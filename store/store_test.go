package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var bg = context.Background()

// The shape package auth injects; Store must satisfy it.
var _ interface {
	CreateSession(ctx context.Context, tokenHash, sub, email string, expires time.Time) error
	GetSession(ctx context.Context, tokenHash string) (sub, email string, ok bool, err error)
	DeleteSession(ctx context.Context, tokenHash string) error
} = (*Store)(nil)

func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func pragma(t *testing.T, s *Store, name string) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow("PRAGMA " + name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpen_Memory(t *testing.T) {
	s := openMem(t)
	if got := pragma(t, s, "user_version"); got != "1" {
		t.Errorf("user_version = %s, want 1", got)
	}
	if got := pragma(t, s, "foreign_keys"); got != "1" {
		t.Errorf("foreign_keys = %s, want 1", got)
	}
	if got := pragma(t, s, "busy_timeout"); got != "5000" {
		t.Errorf("busy_timeout = %s, want 5000", got)
	}
	for _, table := range []string{"folders", "songs", "progressions", "sessions"} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}
}

// An in-memory database exists per connection, and one connection also serialises every transaction.
func TestOpen_UsesASingleConnection(t *testing.T) {
	if got := openMem(t).db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestOpen_MemoryDatabasesAreIndependent(t *testing.T) {
	a, b := openMem(t), openMem(t)
	if _, err := a.CreateFolder(bg, "u", "Only in a"); err != nil {
		t.Fatal(err)
	}
	lib, err := b.Library(bg, "u")
	if err != nil || len(lib.Folders) != 0 {
		t.Errorf("second in-memory store sees %v, %v", lib, err)
	}
}

func TestOpen_ForeignKeysAreEnforced(t *testing.T) {
	s := openMem(t)
	if _, err := s.db.Exec(`INSERT INTO songs (owner, folder_id, name, created_at, updated_at) VALUES ('u', 999, 'x', 0, 0)`); err == nil {
		t.Error("inserted a song pointing at a missing folder")
	}
	if _, err := s.db.Exec(`INSERT INTO progressions (song_id, name, position, data, created_at, updated_at) VALUES (999, 'x', 0, '{}', 0, 0)`); err == nil {
		t.Error("inserted a progression pointing at a missing song")
	}
}

func TestOpen_FileCreatesDirsPrivatelyAndUsesWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "fv.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := pragma(t, s, "journal_mode"); got != "wal" {
		t.Errorf("journal_mode = %s, want wal", got)
	}
	if got := pragma(t, s, "foreign_keys"); got != "1" {
		t.Errorf("foreign_keys = %s, want 1", got)
	}
	if got := pragma(t, s, "busy_timeout"); got != "5000" {
		t.Errorf("busy_timeout = %s, want 5000", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("db file mode = %v, want 0600", fi.Mode().Perm())
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("db dir mode = %v, want 0700", di.Mode().Perm())
	}
}

func TestOpen_PathWithURICharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a b?c#d%e", "fv?x#y%41.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFolder(bg, "u", "F"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database not at the exact path: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if e.Name() != "fv?x#y%41.db" && e.Name() != "fv?x#y%41.db-wal" && e.Name() != "fv?x#y%41.db-shm" {
			t.Errorf("unexpected stray file %q (path was mangled)", e.Name())
		}
	}
}

func TestOpen_PersistsAcrossCloseAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fv.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := s.CreateFolder(bg, "alice", "Demos")
	song, err := s.CreateSong(bg, "alice", "Song", &f.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.AddProgression(bg, "alice", song.ID, "Verse", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(bg, "hash", "alice", "a@example.com", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	lib, err := s.Library(bg, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Folders) != 1 || lib.Folders[0].Name != "Demos" || len(lib.Songs) != 1 ||
		lib.Songs[0].FolderID == nil || *lib.Songs[0].FolderID != f.ID ||
		len(lib.Songs[0].Progressions) != 1 || lib.Songs[0].Progressions[0].Name != "Verse" {
		t.Errorf("library after reopen = %+v", lib)
	}
	got, err := s.GetProgression(bg, "alice", p.ID)
	if err != nil || string(got.Data) != `{"a":1}` {
		t.Errorf("progression after reopen = %+v, %v", got, err)
	}
	sub, email, ok, err := s.GetSession(bg, "hash")
	if err != nil || !ok || sub != "alice" || email != "a@example.com" {
		t.Errorf("session after reopen = %q %q %v %v", sub, email, ok, err)
	}
	if got := pragma(t, s, "user_version"); got != "1" {
		t.Errorf("user_version after reopen = %s, want 1", got)
	}
}

// isFull reports whether err is SQLite's "database or disk is full".
func isFull(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_FULL
}

func TestOpen_CapsDatabaseSize(t *testing.T) {
	for name, path := range map[string]string{"memory": ":memory:", "file": filepath.Join(t.TempDir(), "fv.db")} {
		t.Run(name, func(t *testing.T) {
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if got, want := pragma(t, s, "max_page_count"), strconv.Itoa(maxDBPages); got != want {
				t.Errorf("max_page_count = %s, want %s", got, want)
			}

			// Leave room for a few more pages only, so that a full database is quick to reach.
			pragma(t, s, fmt.Sprintf("max_page_count = %d", queryInt(t, s, "PRAGMA page_count")+40))
			song := mustSong(t, s, alice, "Song", nil)
			bigData := []byte(`"` + strings.Repeat("a", MaxDataBytes-2) + `"`) // 16 pages
			var added []int64
			var full error
			for len(added) < MaxProgressions && full == nil {
				var p Progression
				if p, full = s.AddProgression(bg, alice, song.ID, fmt.Sprintf("p%d", len(added)), bigData); full == nil {
					added = append(added, p.ID)
				}
			}
			if !isFull(full) {
				t.Fatalf("filling the database ended with %v, want SQLITE_FULL", full)
			}
			if len(added) == 0 {
				t.Error("the database was full before anything was stored")
			}
			// It is a plain error, so the API reports it as a bare 500 without internals.
			for _, kind := range []error{ErrNotFound, ErrInvalid, ErrLimit, ErrConflict} {
				if errors.Is(full, kind) {
					t.Errorf("a full database is reported as %v", kind)
				}
			}
			if got := positions(t, s, alice, song.ID); len(got) != len(added) {
				t.Errorf("%d progressions after the failed add, want %d", len(got), len(added))
			}

			// Deleting frees pages, so the store is not stuck.
			for _, id := range added {
				if err := s.DeleteProgression(bg, alice, id); err != nil {
					t.Fatal(err)
				}
			}
			mustProg(t, s, alice, song.ID, "after the cleanup")
		})
	}
}

func TestOpen_RejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fv.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := Open(path); err == nil {
		s.Close()
		t.Error("Open accepted a database from a newer version")
	}
}

func TestOpen_UpgradesAnEmptyVersionZeroDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fv.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE unrelated (x)"); err != nil { // an existing file at version 0
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateFolder(bg, "u", "F"); err != nil {
		t.Error(err)
	}
}

func TestOpen_Failures(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.db")
	if err := os.WriteFile(garbage, []byte("this is not a sqlite database, just some text padding it out"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"path is a directory":    dir,
		"not a database":         garbage,
		"parent is a plain file": filepath.Join(blocker, "fv.db"),
	} {
		if s, err := Open(path); err == nil {
			s.Close()
			t.Errorf("%s: Open succeeded", name)
		}
	}
}

func TestSessions_CreateGetDelete(t *testing.T) {
	s := openMem(t)
	if err := s.CreateSession(bg, "h1", "sub1", "one@example.com", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(bg, "h2", "sub2", "two@example.com", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	sub, email, ok, err := s.GetSession(bg, "h1")
	if err != nil || !ok || sub != "sub1" || email != "one@example.com" {
		t.Errorf("GetSession(h1) = %q %q %v %v", sub, email, ok, err)
	}
	if err := s.DeleteSession(bg, "h1"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.GetSession(bg, "h1"); ok || err != nil {
		t.Errorf("deleted session: ok=%v err=%v", ok, err)
	}
	if _, _, ok, _ := s.GetSession(bg, "h2"); !ok {
		t.Error("deleting h1 removed h2")
	}
	if err := s.DeleteSession(bg, "h1"); err != nil {
		t.Errorf("deleting a missing session: %v", err)
	}
	if _, _, ok, err := s.GetSession(bg, "never-existed"); ok || err != nil {
		t.Errorf("unknown session: ok=%v err=%v", ok, err)
	}
}

func TestSessions_ExpiredIsNotReturned(t *testing.T) {
	s := openMem(t)
	if err := s.CreateSession(bg, "old", "sub", "e@example.com", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.GetSession(bg, "old"); ok || err != nil {
		t.Errorf("expired session: ok=%v err=%v", ok, err)
	}
}

func TestSessions_CreatePurgesExpired(t *testing.T) {
	s := openMem(t)
	for _, h := range []string{"old1", "old2"} {
		if err := s.CreateSession(bg, h, "sub", "e", time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateSession(bg, "live", "sub", "e", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d session rows after purge, want 1", n)
	}
	if _, _, ok, _ := s.GetSession(bg, "live"); !ok {
		t.Error("purge removed the live session")
	}
}

func TestSessions_DuplicateHashIsRefusedAndKeepsTheOriginal(t *testing.T) {
	s := openMem(t)
	if err := s.CreateSession(bg, "h", "victim", "v@example.com", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(bg, "h", "attacker", "a@example.com", time.Now().Add(time.Hour)); err == nil {
		t.Error("a second session reused the token hash")
	}
	if sub, _, _, _ := s.GetSession(bg, "h"); sub != "victim" {
		t.Errorf("session now belongs to %q", sub)
	}
}

func TestErrorKinds(t *testing.T) {
	err := fail(ErrLimit, "too many %d", 3)
	if !errors.Is(err, ErrLimit) || errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is mismatch for %v", err)
	}
	if err.Error() != "too many 3" {
		t.Errorf("message = %q", err.Error())
	}
}

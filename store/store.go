// Package store keeps login sessions and each user's folders, songs and progressions in SQLite.
//
// Every library method takes the owner (the Cognito sub) and filters by it, so a row that
// belongs to someone else is indistinguishable from one that does not exist: ErrNotFound.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	MaxFolders             = 200      // per user
	MaxSongs               = 500      // per user
	MaxProgressions        = 50       // per song
	MaxProgressionsPerUser = 500      // across all of a user's songs
	MaxNameRunes           = 100      // folder, song and progression names, after trimming
	MaxDataBytes           = 64 << 10 // one progression's data

	// maxDBPages caps the database file: 256 MiB at SQLite's default 4 KiB pages. Past it, writes
	// fail with SQLITE_FULL instead of filling the volume.
	maxDBPages = 64 << 10
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
	ErrLimit    = errors.New("limit reached")
	ErrConflict = errors.New("already exists")
)

// kindError is an error with a message that is safe to show a client; it matches its kind under errors.Is.
type kindError struct {
	kind error
	msg  string
}

func (e *kindError) Error() string { return e.msg }
func (e *kindError) Unwrap() error { return e.kind }

func fail(kind error, format string, args ...any) error {
	return &kindError{kind, fmt.Sprintf(format, args...)}
}

func notFound(what string) error { return fail(ErrNotFound, "%s not found", what) }

// migrations[i] upgrades a database at PRAGMA user_version i to i+1.
var migrations = []string{`
CREATE TABLE folders (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner      TEXT NOT NULL,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	UNIQUE (owner, name)
);
CREATE TABLE songs (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner      TEXT NOT NULL,
	folder_id  INTEGER REFERENCES folders(id) ON DELETE SET NULL,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX songs_owner ON songs(owner);
CREATE INDEX songs_folder ON songs(folder_id);
CREATE TABLE progressions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	song_id    INTEGER NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	position   INTEGER NOT NULL,
	data       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX progressions_song ON progressions(song_id, position);
CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	sub        TEXT NOT NULL,
	email      TEXT NOT NULL,
	expires_at INTEGER NOT NULL
);
`}

type Store struct {
	db *sql.DB
}

// uriEscaper escapes the characters that would end or alter the path of a SQLite "file:" URI.
var uriEscaper = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")

// Open opens (creating it if needed) the database at path and brings its schema up to date.
// The path ":memory:" gives a private in-memory database.
//
// It uses a single connection: the app is small, and one connection serialises every
// transaction, which rules out SQLITE_BUSY from read-then-write lock upgrades.
func Open(path string) (*Store, error) {
	dsn := "file::memory:"
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		// Create the file ourselves so it (and SQLite's -wal/-shm files, which copy its mode)
		// is private: it holds session hashes and emails.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
		dsn = "file:" + uriEscaper.Replace(path)
	}
	dsn += fmt.Sprintf("?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=max_page_count(%d)", maxDBPages)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this program supports (%d)", version, len(migrations))
	}
	for ; version < len(migrations); version++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		_, err = tx.Exec(migrations[version])
		if err == nil {
			_, err = tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version+1))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// tx runs fn in a transaction, committing only if it returns nil.
func (s *Store) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func count(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

func isUnique(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func now() int64 { return time.Now().Unix() }

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

const (
	alice = "alice"
	bob   = "bob"
	data1 = `{"n":1}`
)

func mustFolder(t *testing.T, s *Store, owner, name string) Folder {
	t.Helper()
	f, err := s.CreateFolder(bg, owner, name)
	if err != nil {
		t.Fatalf("CreateFolder(%q): %v", name, err)
	}
	return f
}

func mustSong(t *testing.T, s *Store, owner, name string, folder *int64) Song {
	t.Helper()
	song, err := s.CreateSong(bg, owner, name, folder)
	if err != nil {
		t.Fatalf("CreateSong(%q): %v", name, err)
	}
	return song
}

func mustProg(t *testing.T, s *Store, owner string, songID int64, name string) Progression {
	t.Helper()
	p, err := s.AddProgression(bg, owner, songID, name, []byte(data1))
	if err != nil {
		t.Fatalf("AddProgression(%q): %v", name, err)
	}
	return p
}

func mustLibrary(t *testing.T, s *Store, owner string) Library {
	t.Helper()
	lib, err := s.Library(bg, owner)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func wantKind(t *testing.T, err, kind error) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Errorf("error = %v, want %v", err, kind)
	}
}

func ptr[T any](v T) *T { return &v }

func queryInt(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func positions(t *testing.T, s *Store, owner string, songID int64) []string {
	t.Helper()
	for _, song := range mustLibrary(t, s, owner).Songs {
		if song.ID == songID {
			var out []string
			for i, p := range song.Progressions {
				if p.Position != i {
					t.Errorf("progression %q has position %d at index %d", p.Name, p.Position, i)
				}
				out = append(out, p.Name)
			}
			return out
		}
	}
	t.Fatalf("song %d not in %s's library", songID, owner)
	return nil
}

func progressionIDs(t *testing.T, s *Store, owner string, songID int64) []int64 {
	t.Helper()
	var ids []int64
	for _, song := range mustLibrary(t, s, owner).Songs {
		if song.ID == songID {
			for _, p := range song.Progressions {
				ids = append(ids, p.ID)
			}
		}
	}
	return ids
}

func TestLibrary_EmptyHasNonNilSlices(t *testing.T) {
	s := openMem(t)
	lib := mustLibrary(t, s, alice)
	if lib.Folders == nil || lib.Songs == nil || len(lib.Folders) != 0 || len(lib.Songs) != 0 {
		t.Errorf("empty library = %#v, want empty non-nil slices", lib)
	}
	song := mustSong(t, s, alice, "Song", nil)
	if got := mustLibrary(t, s, alice).Songs[0]; got.ID != song.ID || got.Progressions == nil {
		t.Errorf("a song with no progressions has Progressions = %#v, want empty non-nil", got.Progressions)
	}
}

func TestLibrary_SortsCaseInsensitivelyAndOrdersProgressions(t *testing.T) {
	s := openMem(t)
	for _, n := range []string{"beta", "Alpha", "Gamma"} {
		mustFolder(t, s, alice, n)
	}
	for _, n := range []string{"zebra", "Apple", "mango", "Banana"} {
		mustSong(t, s, alice, n, nil)
	}
	song := mustSong(t, s, alice, "Ordered", nil)
	for _, n := range []string{"Verse", "Chorus", "Bridge"} {
		mustProg(t, s, alice, song.ID, n)
	}
	lib := mustLibrary(t, s, alice)
	var folders, songs []string
	for _, f := range lib.Folders {
		folders = append(folders, f.Name)
	}
	for _, sg := range lib.Songs {
		songs = append(songs, sg.Name)
	}
	if want := []string{"Alpha", "beta", "Gamma"}; !reflect.DeepEqual(folders, want) {
		t.Errorf("folders = %v, want %v", folders, want)
	}
	if want := []string{"Apple", "Banana", "mango", "Ordered", "zebra"}; !reflect.DeepEqual(songs, want) {
		t.Errorf("songs = %v, want %v", songs, want)
	}
	if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, []string{"Verse", "Chorus", "Bridge"}) {
		t.Errorf("progressions = %v", got)
	}
	for _, p := range lib.Songs[3].Progressions {
		if len(p.Data) != 0 {
			t.Errorf("library listing carries progression data %q", p.Data)
		}
	}
}

func TestLibrary_OnlyShowsTheOwnersRows(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "A folder")
	song := mustSong(t, s, alice, "A song", &f.ID)
	mustProg(t, s, alice, song.ID, "A section")
	mustFolder(t, s, bob, "B folder")
	bs := mustSong(t, s, bob, "B song", nil)
	mustProg(t, s, bob, bs.ID, "B section")

	for owner, want := range map[string][3]string{alice: {"A folder", "A song", "A section"}, bob: {"B folder", "B song", "B section"}} {
		lib := mustLibrary(t, s, owner)
		if len(lib.Folders) != 1 || len(lib.Songs) != 1 || len(lib.Songs[0].Progressions) != 1 ||
			lib.Folders[0].Name != want[0] || lib.Songs[0].Name != want[1] || lib.Songs[0].Progressions[0].Name != want[2] {
			t.Errorf("%s sees %+v", owner, lib)
		}
	}
	if lib := mustLibrary(t, s, "carol"); len(lib.Folders)+len(lib.Songs) != 0 {
		t.Errorf("a third user sees %+v", lib)
	}
}

func TestFolders_CreateValidatesAndTrimsNames(t *testing.T) {
	s := openMem(t)
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"Demos", "Demos", true},
		{"  Padded \t\n", "Padded", true},
		{"two words", "two words", true},
		{strings.Repeat("a", 100), strings.Repeat("a", 100), true},
		{strings.Repeat("é", 100), strings.Repeat("é", 100), true}, // 200 bytes but 100 runes
		{"<script>alert(1)</script>", "<script>alert(1)</script>", true},
		{"", "", false},
		{"   \t ", "", false},
		{strings.Repeat("a", 101), "", false},
		{strings.Repeat("é", 101), "", false},
		{" " + strings.Repeat("a", 101) + " ", "", false},
	}
	for _, tt := range tests {
		f, err := s.CreateFolder(bg, alice, tt.in)
		if tt.ok {
			if err != nil || f.Name != tt.want || f.ID == 0 {
				t.Errorf("CreateFolder(%q) = %+v, %v; want name %q", tt.in, f, err, tt.want)
			}
		} else {
			wantKind(t, err, ErrInvalid)
		}
	}
}

func TestFolders_DuplicateNamePerOwner(t *testing.T) {
	s := openMem(t)
	mustFolder(t, s, alice, "Demos")
	for _, name := range []string{"Demos", "  Demos  "} {
		_, err := s.CreateFolder(bg, alice, name)
		wantKind(t, err, ErrConflict)
	}
	mustFolder(t, s, alice, "demos") // names are case-sensitive
	mustFolder(t, s, bob, "Demos")   // another user may reuse a name
	if n := len(mustLibrary(t, s, alice).Folders); n != 2 {
		t.Errorf("alice has %d folders, want 2", n)
	}
}

func TestFolders_Limit(t *testing.T) {
	s := openMem(t)
	var first Folder
	for i := range MaxFolders {
		f := mustFolder(t, s, alice, fmt.Sprintf("f%03d", i))
		if i == 0 {
			first = f
		}
	}
	_, err := s.CreateFolder(bg, alice, "one too many")
	wantKind(t, err, ErrLimit)
	mustFolder(t, s, bob, "bob is unaffected")
	if err := s.DeleteFolder(bg, alice, first.ID); err != nil {
		t.Fatal(err)
	}
	mustFolder(t, s, alice, "room again")
}

func TestFolders_Rename(t *testing.T) {
	s := openMem(t)
	a := mustFolder(t, s, alice, "A")
	mustFolder(t, s, alice, "B")

	got, err := s.RenameFolder(bg, alice, a.ID, "  Renamed ")
	if err != nil || got != (Folder{ID: a.ID, Name: "Renamed"}) {
		t.Errorf("RenameFolder = %+v, %v", got, err)
	}
	if got, err := s.RenameFolder(bg, alice, a.ID, "Renamed"); err != nil || got.Name != "Renamed" {
		t.Errorf("renaming to its own name = %+v, %v", got, err)
	}
	_, err = s.RenameFolder(bg, alice, a.ID, "B")
	wantKind(t, err, ErrConflict)
	_, err = s.RenameFolder(bg, alice, a.ID, " ")
	wantKind(t, err, ErrInvalid)
	_, err = s.RenameFolder(bg, alice, a.ID+1000, "Whatever")
	wantKind(t, err, ErrNotFound)

	before := mustLibrary(t, s, alice)
	_, err = s.RenameFolder(bg, bob, a.ID, "Hijacked")
	wantKind(t, err, ErrNotFound)
	if !reflect.DeepEqual(before, mustLibrary(t, s, alice)) {
		t.Error("another user's rename changed the folder")
	}
	// A failed rename by a non-owner does not collide with, or reveal, the owner's names.
	mustFolder(t, s, bob, "Bobs")
	_, err = s.RenameFolder(bg, bob, a.ID, "Bobs")
	wantKind(t, err, ErrNotFound)
}

func TestFolders_DeleteKeepsSongsAndUnfilesThem(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "F")
	other := mustFolder(t, s, alice, "Other")
	in := mustSong(t, s, alice, "In F", &f.ID)
	elsewhere := mustSong(t, s, alice, "In Other", &other.ID)
	mustProg(t, s, alice, in.ID, "Verse")

	if err := s.DeleteFolder(bg, alice, f.ID); err != nil {
		t.Fatal(err)
	}
	lib := mustLibrary(t, s, alice)
	if len(lib.Folders) != 1 || lib.Folders[0].ID != other.ID || len(lib.Songs) != 2 {
		t.Fatalf("library after folder delete = %+v", lib)
	}
	for _, song := range lib.Songs {
		switch song.ID {
		case in.ID:
			if song.FolderID != nil || len(song.Progressions) != 1 {
				t.Errorf("unfiled song = %+v", song)
			}
		case elsewhere.ID:
			if song.FolderID == nil || *song.FolderID != other.ID {
				t.Errorf("a song in another folder moved: %+v", song)
			}
		}
	}
	wantKind(t, s.DeleteFolder(bg, alice, f.ID), ErrNotFound)
}

func TestFolders_DeleteOfAnotherUsersFolderIsRefused(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "F")
	song := mustSong(t, s, alice, "S", &f.ID)
	wantKind(t, s.DeleteFolder(bg, bob, f.ID), ErrNotFound)
	lib := mustLibrary(t, s, alice)
	if len(lib.Folders) != 1 || lib.Songs[0].ID != song.ID || lib.Songs[0].FolderID == nil {
		t.Errorf("a refused delete changed %+v", lib)
	}
}

func TestSongs_Create(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "F")

	plain, err := s.CreateSong(bg, alice, "  Plain  ", nil)
	if err != nil || plain.Name != "Plain" || plain.FolderID != nil || plain.ID == 0 || plain.UpdatedAt.IsZero() {
		t.Errorf("CreateSong = %+v, %v", plain, err)
	}
	filed, err := s.CreateSong(bg, alice, "Filed", &f.ID)
	if err != nil || filed.FolderID == nil || *filed.FolderID != f.ID {
		t.Errorf("CreateSong in folder = %+v, %v", filed, err)
	}
	for _, name := range []string{"", "  ", strings.Repeat("x", 101)} {
		_, err := s.CreateSong(bg, alice, name, nil)
		wantKind(t, err, ErrInvalid)
	}
	_, err = s.CreateSong(bg, alice, "Dup names are fine", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSong(bg, alice, "Dup names are fine", nil); err != nil {
		t.Errorf("duplicate song names are allowed: %v", err)
	}
}

func TestSongs_CannotBeCreatedInAnotherUsersOrMissingFolder(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "Alice's")
	_, err := s.CreateSong(bg, bob, "Sneaky", &f.ID)
	wantKind(t, err, ErrNotFound)
	_, err = s.CreateSong(bg, bob, "Missing", ptr(f.ID+1000))
	wantKind(t, err, ErrNotFound)
	if n := queryInt(t, s, `SELECT COUNT(*) FROM songs`); n != 0 {
		t.Errorf("%d songs were created", n)
	}
}

func TestSongs_Limit(t *testing.T) {
	s := openMem(t)
	var first Song
	for i := range MaxSongs {
		song := mustSong(t, s, alice, fmt.Sprintf("s%03d", i), nil)
		if i == 0 {
			first = song
		}
	}
	_, err := s.CreateSong(bg, alice, "one too many", nil)
	wantKind(t, err, ErrLimit)
	mustSong(t, s, bob, "bob is unaffected", nil)
	if err := s.DeleteSong(bg, alice, first.ID); err != nil {
		t.Fatal(err)
	}
	mustSong(t, s, alice, "room again", nil)
}

func TestSongs_UpdateTriStateFolder(t *testing.T) {
	s := openMem(t)
	f1 := mustFolder(t, s, alice, "F1")
	f2 := mustFolder(t, s, alice, "F2")
	song := mustSong(t, s, alice, "Song", &f1.ID)

	folderOf := func() *int64 {
		t.Helper()
		got, err := s.UpdateSong(bg, alice, song.ID, SongPatch{})
		if err != nil {
			t.Fatal(err)
		}
		return got.FolderID
	}

	got, err := s.UpdateSong(bg, alice, song.ID, SongPatch{Name: ptr("  Renamed ")})
	if err != nil || got.Name != "Renamed" || got.FolderID == nil || *got.FolderID != f1.ID {
		t.Errorf("rename only = %+v, %v (folder must be left alone)", got, err)
	}
	got, err = s.UpdateSong(bg, alice, song.ID, SongPatch{SetFolder: true, FolderID: &f2.ID})
	if err != nil || got.Name != "Renamed" || got.FolderID == nil || *got.FolderID != f2.ID {
		t.Errorf("move = %+v, %v", got, err)
	}
	if fid := folderOf(); fid == nil || *fid != f2.ID {
		t.Error("an empty patch changed the folder")
	}
	got, err = s.UpdateSong(bg, alice, song.ID, SongPatch{SetFolder: true})
	if err != nil || got.FolderID != nil {
		t.Errorf("unfile = %+v, %v", got, err)
	}
	if folderOf() != nil {
		t.Error("an empty patch refiled the song")
	}
	got, err = s.UpdateSong(bg, alice, song.ID, SongPatch{Name: ptr("Both"), SetFolder: true, FolderID: &f1.ID})
	if err != nil || got.Name != "Both" || *got.FolderID != f1.ID {
		t.Errorf("name and folder = %+v, %v", got, err)
	}
	// FolderID without SetFolder is ignored.
	got, err = s.UpdateSong(bg, alice, song.ID, SongPatch{FolderID: &f2.ID})
	if err != nil || *got.FolderID != f1.ID {
		t.Errorf("FolderID without SetFolder moved the song: %+v, %v", got, err)
	}
}

func TestSongs_UpdateRefusals(t *testing.T) {
	s := openMem(t)
	af := mustFolder(t, s, alice, "A folder")
	bf := mustFolder(t, s, bob, "B folder")
	song := mustSong(t, s, alice, "Song", &af.ID)
	before := mustLibrary(t, s, alice)

	cases := []struct {
		name  string
		owner string
		id    int64
		patch SongPatch
		kind  error
	}{
		{"invalid name", alice, song.ID, SongPatch{Name: ptr(" ")}, ErrInvalid},
		{"invalid name and valid move", alice, song.ID, SongPatch{Name: ptr(strings.Repeat("a", 101)), SetFolder: true}, ErrInvalid},
		{"into another user's folder", alice, song.ID, SongPatch{SetFolder: true, FolderID: &bf.ID}, ErrNotFound},
		{"into a missing folder", alice, song.ID, SongPatch{SetFolder: true, FolderID: ptr(af.ID + 1000)}, ErrNotFound},
		{"rename by another user", bob, song.ID, SongPatch{Name: ptr("Hijacked")}, ErrNotFound},
		{"move by another user into their own folder", bob, song.ID, SongPatch{SetFolder: true, FolderID: &bf.ID}, ErrNotFound},
		{"unfile by another user", bob, song.ID, SongPatch{SetFolder: true}, ErrNotFound},
		{"missing song", alice, song.ID + 1000, SongPatch{Name: ptr("x")}, ErrNotFound},
	}
	for _, tt := range cases {
		_, err := s.UpdateSong(bg, tt.owner, tt.id, tt.patch)
		if !errors.Is(err, tt.kind) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.kind)
		}
	}
	if after := mustLibrary(t, s, alice); !reflect.DeepEqual(before, after) {
		t.Errorf("refused updates changed the song:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestSongs_DeleteCascadesToProgressions(t *testing.T) {
	s := openMem(t)
	gone := mustSong(t, s, alice, "Gone", nil)
	kept := mustSong(t, s, alice, "Kept", nil)
	mustProg(t, s, alice, gone.ID, "A")
	mustProg(t, s, alice, gone.ID, "B")
	keptProg := mustProg(t, s, alice, kept.ID, "C")

	if err := s.DeleteSong(bg, alice, gone.ID); err != nil {
		t.Fatal(err)
	}
	if n := queryInt(t, s, `SELECT COUNT(*) FROM progressions WHERE song_id = ?`, gone.ID); n != 0 {
		t.Errorf("%d orphaned progressions remain", n)
	}
	if _, err := s.GetProgression(bg, alice, keptProg.ID); err != nil {
		t.Errorf("deleting one song removed another's progression: %v", err)
	}
	wantKind(t, s.DeleteSong(bg, alice, gone.ID), ErrNotFound)
}

func TestSongs_DeleteByAnotherUserIsRefused(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	p := mustProg(t, s, alice, song.ID, "Verse")
	wantKind(t, s.DeleteSong(bg, bob, song.ID), ErrNotFound)
	if _, err := s.GetProgression(bg, alice, p.ID); err != nil {
		t.Errorf("a refused delete removed the progression: %v", err)
	}
	if len(mustLibrary(t, s, alice).Songs) != 1 {
		t.Error("a refused delete removed the song")
	}
}

func TestProgressions_AddAppendsAtTheEnd(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	other := mustSong(t, s, alice, "Other", nil)
	for i, n := range []string{"Verse", "Chorus", "Bridge"} {
		p, err := s.AddProgression(bg, alice, song.ID, " "+n+" ", []byte(`{"i":`+fmt.Sprint(i)+`}`))
		if err != nil || p.Position != i || p.Name != n || p.SongID != song.ID || p.ID == 0 || string(p.Data) != fmt.Sprintf(`{"i":%d}`, i) {
			t.Errorf("AddProgression #%d = %+v, %v", i, p, err)
		}
	}
	if p := mustProg(t, s, alice, other.ID, "First elsewhere"); p.Position != 0 {
		t.Errorf("positions are per song; got %d", p.Position)
	}
}

func TestProgressions_AddValidatesNameAndData(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	bigValid := `"` + strings.Repeat("a", MaxDataBytes-2) + `"`
	tooBig := `"` + strings.Repeat("a", MaxDataBytes-1) + `"`
	if len(bigValid) != MaxDataBytes || len(tooBig) != MaxDataBytes+1 {
		t.Fatal("test data is the wrong size")
	}
	cases := []struct {
		name, data string
		ok         bool
	}{
		{"Verse", `{}`, true},
		{"Verse", `[1,2]`, true},
		{"Verse", bigValid, true},
		{"", `{}`, false},
		{"   ", `{}`, false},
		{strings.Repeat("n", 101), `{}`, false},
		{"Verse", ``, false},
		{"Verse", `{`, false},
		{"Verse", `not json`, false},
		{"Verse", `{} {}`, false},
		{"Verse", tooBig, false},
	}
	for _, tt := range cases {
		_, err := s.AddProgression(bg, alice, song.ID, tt.name, []byte(tt.data))
		if tt.ok && err != nil {
			t.Errorf("AddProgression(%.20q, %.20q): %v", tt.name, tt.data, err)
		}
		if !tt.ok {
			wantKind(t, err, ErrInvalid)
		}
	}
	if n := queryInt(t, s, `SELECT COUNT(*) FROM progressions`); n != 3 {
		t.Errorf("%d progressions stored, want 3", n)
	}
}

func TestProgressions_Limit(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	other := mustSong(t, s, alice, "Other", nil)
	for i := range MaxProgressions {
		mustProg(t, s, alice, song.ID, fmt.Sprintf("p%d", i))
	}
	_, err := s.AddProgression(bg, alice, song.ID, "one too many", []byte(`{}`))
	wantKind(t, err, ErrLimit)
	mustProg(t, s, alice, other.ID, "another song is unaffected")
	if got := positions(t, s, alice, song.ID); len(got) != MaxProgressions {
		t.Errorf("%d progressions, want %d", len(got), MaxProgressions)
	}
}

// fillProgressions gives owner n progressions, MaxProgressions to a song, and returns the songs.
func fillProgressions(t *testing.T, s *Store, owner string, n int) []Song {
	t.Helper()
	var songs []Song
	for added := 0; added < n; {
		song := mustSong(t, s, owner, fmt.Sprintf("s%d", len(songs)), nil)
		songs = append(songs, song)
		for range min(MaxProgressions, n-added) {
			mustProg(t, s, owner, song.ID, fmt.Sprintf("p%d", added))
			added++
		}
	}
	return songs
}

func TestProgressions_PerUserLimit(t *testing.T) {
	s := openMem(t)
	songs := fillProgressions(t, s, alice, MaxProgressionsPerUser) // the 500th add succeeds
	extra := mustSong(t, s, alice, "Extra", nil)                   // empty: only the per-user cap can refuse it
	_, err := s.AddProgression(bg, alice, extra.ID, "501st", []byte(`{}`))
	wantKind(t, err, ErrLimit)
	if err == nil || !strings.Contains(err.Error(), "at most 500 progressions across all your songs") {
		t.Errorf("error = %v, want it to name the per-user limit", err)
	}
	if n := queryInt(t, s, `SELECT COUNT(*) FROM progressions`); n != MaxProgressionsPerUser {
		t.Errorf("%d progressions stored, want %d (a refused add stores nothing)", n, MaxProgressionsPerUser)
	}

	// The cap is per user.
	bobSong := mustSong(t, s, bob, "Bob's", nil)
	mustProg(t, s, bob, bobSong.ID, "bob is unaffected")
	if _, err := s.AddProgression(bg, alice, extra.ID, "still full", []byte(`{}`)); !errors.Is(err, ErrLimit) {
		t.Errorf("bob's add freed alice's capacity: %v", err)
	}

	// Deleting a progression frees one slot.
	if err := s.DeleteProgression(bg, alice, progressionIDs(t, s, alice, songs[0].ID)[0]); err != nil {
		t.Fatal(err)
	}
	mustProg(t, s, alice, extra.ID, "uses the freed slot")
	_, err = s.AddProgression(bg, alice, extra.ID, "full again", []byte(`{}`))
	wantKind(t, err, ErrLimit)

	// Deleting a song frees all of its progressions.
	if err := s.DeleteSong(bg, alice, songs[1].ID); err != nil {
		t.Fatal(err)
	}
	for i := range MaxProgressions - 1 { // extra already holds one
		mustProg(t, s, alice, extra.ID, fmt.Sprintf("refill %d", i))
	}
	other := mustSong(t, s, alice, "Other", nil)
	mustProg(t, s, alice, other.ID, "the 500th again")
	_, err = s.AddProgression(bg, alice, other.ID, "over again", []byte(`{}`))
	wantKind(t, err, ErrLimit)
}

func TestProgressions_CannotBeAddedToAnotherUsersSong(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	_, err := s.AddProgression(bg, bob, song.ID, "Sneaky", []byte(`{}`))
	wantKind(t, err, ErrNotFound)
	_, err = s.AddProgression(bg, alice, song.ID+1000, "Missing", []byte(`{}`))
	wantKind(t, err, ErrNotFound)
	if n := queryInt(t, s, `SELECT COUNT(*) FROM progressions`); n != 0 {
		t.Errorf("%d progressions were created", n)
	}
}

func TestProgressions_Get(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	p, err := s.AddProgression(bg, alice, song.ID, "Verse", []byte(`{"keep":"me verbatim"}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProgression(bg, alice, p.ID)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Errorf("GetProgression = %+v, %v; want %+v", got, err, p)
	}
	_, err = s.GetProgression(bg, bob, p.ID)
	wantKind(t, err, ErrNotFound)
	_, err = s.GetProgression(bg, alice, p.ID+1000)
	wantKind(t, err, ErrNotFound)
}

func TestProgressions_UpdateIsPartial(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	p, _ := s.AddProgression(bg, alice, song.ID, "Verse", []byte(`{"v":1}`))

	got, err := s.UpdateProgression(bg, alice, p.ID, ProgressionPatch{Name: ptr(" Intro ")})
	if err != nil || got.Name != "Intro" || string(got.Data) != `{"v":1}` || got.Position != 0 || got.SongID != song.ID {
		t.Errorf("rename only = %+v, %v", got, err)
	}
	got, err = s.UpdateProgression(bg, alice, p.ID, ProgressionPatch{Data: []byte(`{"v":2}`)})
	if err != nil || got.Name != "Intro" || string(got.Data) != `{"v":2}` {
		t.Errorf("data only = %+v, %v", got, err)
	}
	got, err = s.UpdateProgression(bg, alice, p.ID, ProgressionPatch{})
	if err != nil || got.Name != "Intro" || string(got.Data) != `{"v":2}` {
		t.Errorf("empty patch = %+v, %v", got, err)
	}
	got, err = s.UpdateProgression(bg, alice, p.ID, ProgressionPatch{Name: ptr("Both"), Data: []byte(`{"v":3}`)})
	if err != nil || got.Name != "Both" || string(got.Data) != `{"v":3}` {
		t.Errorf("both = %+v, %v", got, err)
	}
	if stored, _ := s.GetProgression(bg, alice, p.ID); !reflect.DeepEqual(stored, got) {
		t.Errorf("stored %+v differs from returned %+v", stored, got)
	}
}

func TestProgressions_UpdateRefusals(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	p := mustProg(t, s, alice, song.ID, "Verse")
	before, _ := s.GetProgression(bg, alice, p.ID)

	cases := []struct {
		name  string
		owner string
		id    int64
		patch ProgressionPatch
		kind  error
	}{
		{"blank name", alice, p.ID, ProgressionPatch{Name: ptr("  ")}, ErrInvalid},
		{"long name", alice, p.ID, ProgressionPatch{Name: ptr(strings.Repeat("x", 101))}, ErrInvalid},
		{"invalid JSON", alice, p.ID, ProgressionPatch{Data: []byte(`{`)}, ErrInvalid},
		{"oversize data", alice, p.ID, ProgressionPatch{Data: []byte(`"` + strings.Repeat("a", MaxDataBytes) + `"`)}, ErrInvalid},
		{"another user", bob, p.ID, ProgressionPatch{Name: ptr("Hijacked"), Data: []byte(`{"h":1}`)}, ErrNotFound},
		{"missing", alice, p.ID + 1000, ProgressionPatch{Name: ptr("x")}, ErrNotFound},
	}
	for _, tt := range cases {
		_, err := s.UpdateProgression(bg, tt.owner, tt.id, tt.patch)
		if !errors.Is(err, tt.kind) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.kind)
		}
	}
	if after, _ := s.GetProgression(bg, alice, p.ID); !reflect.DeepEqual(before, after) {
		t.Errorf("refused updates changed the progression: %+v -> %+v", before, after)
	}
}

func TestProgressions_DeleteRepacksPositions(t *testing.T) {
	for _, tt := range []struct {
		name   string
		delete string
		want   []string
	}{
		{"first", "A", []string{"B", "C", "D"}},
		{"middle", "B", []string{"A", "C", "D"}},
		{"last", "D", []string{"A", "B", "C"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := openMem(t)
			song := mustSong(t, s, alice, "Song", nil)
			other := mustSong(t, s, alice, "Other", nil)
			byName := map[string]int64{}
			for _, n := range []string{"A", "B", "C", "D"} {
				byName[n] = mustProg(t, s, alice, song.ID, n).ID
			}
			mustProg(t, s, alice, other.ID, "X")
			mustProg(t, s, alice, other.ID, "Y")

			if err := s.DeleteProgression(bg, alice, byName[tt.delete]); err != nil {
				t.Fatal(err)
			}
			if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("progressions = %v, want %v", got, tt.want)
			}
			if got := positions(t, s, alice, other.ID); !reflect.DeepEqual(got, []string{"X", "Y"}) {
				t.Errorf("another song's progressions = %v", got)
			}
			wantKind(t, s.DeleteProgression(bg, alice, byName[tt.delete]), ErrNotFound)
			// New sections still append at the end of the packed list.
			if p := mustProg(t, s, alice, song.ID, "Z"); p.Position != 3 {
				t.Errorf("new progression position = %d, want 3", p.Position)
			}
		})
	}
}

func TestProgressions_DeleteByAnotherUserIsRefused(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	a := mustProg(t, s, alice, song.ID, "A")
	mustProg(t, s, alice, song.ID, "B")
	wantKind(t, s.DeleteProgression(bg, bob, a.ID), ErrNotFound)
	if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("a refused delete changed %v", got)
	}
}

func TestProgressions_Reorder(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	other := mustSong(t, s, alice, "Other", nil)
	bobSong := mustSong(t, s, bob, "Bob's", nil)
	a := mustProg(t, s, alice, song.ID, "A")
	b := mustProg(t, s, alice, song.ID, "B")
	c := mustProg(t, s, alice, song.ID, "C")
	foreign := mustProg(t, s, alice, other.ID, "Elsewhere")
	bobs := mustProg(t, s, bob, bobSong.ID, "Bob")

	if err := s.ReorderProgressions(bg, alice, song.ID, []int64{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, []string{"C", "A", "B"}) {
		t.Errorf("after reorder = %v", got)
	}

	bad := map[string][]int64{
		"missing an id":               {a.ID, b.ID},
		"an extra id":                 {a.ID, b.ID, c.ID, foreign.ID},
		"a duplicate in place of one": {a.ID, a.ID, b.ID},
		"all duplicates":              {a.ID, a.ID, a.ID},
		"another song's id":           {a.ID, b.ID, foreign.ID},
		"another user's id":           {a.ID, b.ID, bobs.ID},
		"unknown id":                  {a.ID, b.ID, c.ID + 1000},
		"empty":                       {},
		"nil":                         nil,
	}
	for name, ids := range bad {
		wantKind(t, s.ReorderProgressions(bg, alice, song.ID, ids), ErrInvalid)
		if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, []string{"C", "A", "B"}) {
			t.Errorf("%s: a refused reorder changed the order to %v", name, got)
		}
	}
	if got := positions(t, s, alice, other.ID); !reflect.DeepEqual(got, []string{"Elsewhere"}) {
		t.Errorf("another song was touched: %v", got)
	}

	// An empty song reorders with an empty list.
	empty := mustSong(t, s, alice, "Empty", nil)
	if err := s.ReorderProgressions(bg, alice, empty.ID, []int64{}); err != nil {
		t.Errorf("reordering an empty song: %v", err)
	}
}

func TestProgressions_ReorderByAnotherUserIsRefused(t *testing.T) {
	s := openMem(t)
	song := mustSong(t, s, alice, "Song", nil)
	a := mustProg(t, s, alice, song.ID, "A")
	b := mustProg(t, s, alice, song.ID, "B")
	// Even with the exact ids, a non-owner gets ErrNotFound (never ErrInvalid, which would confirm the ids).
	wantKind(t, s.ReorderProgressions(bg, bob, song.ID, []int64{b.ID, a.ID}), ErrNotFound)
	wantKind(t, s.ReorderProgressions(bg, bob, song.ID, []int64{}), ErrNotFound)
	wantKind(t, s.ReorderProgressions(bg, alice, song.ID+1000, []int64{}), ErrNotFound)
	if got := positions(t, s, alice, song.ID); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("a refused reorder changed %v", got)
	}
}

func TestSongs_UpdatedAtIsBumpedByEveryChange(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "F")
	song := mustSong(t, s, alice, "Song", nil)
	p := mustProg(t, s, alice, song.ID, "A")
	q := mustProg(t, s, alice, song.ID, "B")

	changes := map[string]func() error{
		"rename song": func() error { _, err := s.UpdateSong(bg, alice, song.ID, SongPatch{Name: ptr("New")}); return err },
		"move song": func() error {
			_, err := s.UpdateSong(bg, alice, song.ID, SongPatch{SetFolder: true, FolderID: &f.ID})
			return err
		},
		"add progression": func() error { _, err := s.AddProgression(bg, alice, song.ID, "C", []byte(`{}`)); return err },
		"update progression": func() error {
			_, err := s.UpdateProgression(bg, alice, p.ID, ProgressionPatch{Name: ptr("A2")})
			return err
		},
		"reorder": func() error {
			ids := progressionIDs(t, s, alice, song.ID)
			slices.Reverse(ids)
			return s.ReorderProgressions(bg, alice, song.ID, ids)
		},
		"delete progression": func() error { return s.DeleteProgression(bg, alice, q.ID) },
	}
	for _, name := range []string{"rename song", "move song", "add progression", "update progression", "reorder", "delete progression"} {
		if _, err := s.db.Exec(`UPDATE songs SET updated_at = 1 WHERE id = ?`, song.ID); err != nil {
			t.Fatal(err)
		}
		if err := changes[name](); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := queryInt(t, s, `SELECT updated_at FROM songs WHERE id = ?`, song.ID); got <= 1 {
			t.Errorf("%s did not bump the song's updated_at", name)
		}
	}
}

// Everything bob can try against alice's rows must fail with ErrNotFound and change nothing.
func TestIsolation_AnotherUserCanDoNothing(t *testing.T) {
	s := openMem(t)
	f := mustFolder(t, s, alice, "F")
	song := mustSong(t, s, alice, "Song", &f.ID)
	p := mustProg(t, s, alice, song.ID, "Verse")
	bf := mustFolder(t, s, bob, "Bob's folder")
	before := mustLibrary(t, s, alice)
	beforeProg, _ := s.GetProgression(bg, alice, p.ID)

	attacks := map[string]func() error{
		"rename folder": func() error { _, err := s.RenameFolder(bg, bob, f.ID, "x"); return err },
		"delete folder": func() error { return s.DeleteFolder(bg, bob, f.ID) },
		"rename song":   func() error { _, err := s.UpdateSong(bg, bob, song.ID, SongPatch{Name: ptr("x")}); return err },
		"move song": func() error {
			_, err := s.UpdateSong(bg, bob, song.ID, SongPatch{SetFolder: true, FolderID: &bf.ID})
			return err
		},
		"delete song":     func() error { return s.DeleteSong(bg, bob, song.ID) },
		"add progression": func() error { _, err := s.AddProgression(bg, bob, song.ID, "x", []byte(`{}`)); return err },
		"get progression": func() error { _, err := s.GetProgression(bg, bob, p.ID); return err },
		"update progression": func() error {
			_, err := s.UpdateProgression(bg, bob, p.ID, ProgressionPatch{Name: ptr("x")})
			return err
		},
		"delete progression": func() error { return s.DeleteProgression(bg, bob, p.ID) },
		"reorder":            func() error { return s.ReorderProgressions(bg, bob, song.ID, []int64{p.ID}) },
		"own song into alice's folder": func() error {
			own := mustSong(t, s, bob, "Bob's song", nil)
			_, err := s.UpdateSong(bg, bob, own.ID, SongPatch{SetFolder: true, FolderID: &f.ID})
			return err
		},
	}
	for name, attack := range attacks {
		if err := attack(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want %v", name, err, ErrNotFound)
		}
	}
	if after := mustLibrary(t, s, alice); !reflect.DeepEqual(before, after) {
		t.Errorf("alice's library changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if after, _ := s.GetProgression(bg, alice, p.ID); !reflect.DeepEqual(beforeProg, after) {
		t.Errorf("alice's progression changed: %+v -> %+v", beforeProg, after)
	}
	if lib := mustLibrary(t, s, bob); len(lib.Folders) != 1 || len(lib.Songs) != 1 || lib.Songs[0].FolderID != nil {
		t.Errorf("bob's library = %+v", lib)
	}
}

func TestStrings_AreNeverInterpretedAsSQL(t *testing.T) {
	s := openMem(t)
	evil := `x'); DROP TABLE songs; --`
	f := mustFolder(t, s, evil, evil)
	song := mustSong(t, s, evil, evil, &f.ID)
	mustProg(t, s, evil, song.ID, evil)
	lib := mustLibrary(t, s, evil)
	if len(lib.Folders) != 1 || lib.Folders[0].Name != evil || lib.Songs[0].Name != evil || lib.Songs[0].Progressions[0].Name != evil {
		t.Errorf("library = %+v", lib)
	}
	// An owner id that looks like a wildcard or a tautology matches nothing.
	for _, owner := range []string{"%", "' OR '1'='1", "alice' --", ""} {
		if lib := mustLibrary(t, s, owner); len(lib.Folders)+len(lib.Songs) != 0 {
			t.Errorf("owner %q sees %+v", owner, lib)
		}
	}
}

// With one connection every operation is serialised, so limits and positions hold under concurrency.
func TestConcurrentUse_OnAFileDatabase(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	song := mustSong(t, s, alice, "Song", nil)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var ok, limited int
	for g := range 8 {
		wg.Go(func() {
			for i := range 10 {
				_, err := s.AddProgression(bg, alice, song.ID, fmt.Sprintf("g%d-%d", g, i), []byte(`{}`))
				mu.Lock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, ErrLimit):
					limited++
				default:
					t.Errorf("AddProgression: %v", err)
				}
				mu.Unlock()
				if _, err := s.Library(bg, alice); err != nil {
					t.Errorf("Library: %v", err)
				}
			}
		})
	}
	wg.Wait()
	if ok != MaxProgressions || limited != 80-MaxProgressions {
		t.Errorf("%d adds succeeded and %d hit the limit, want %d and %d", ok, limited, MaxProgressions, 80-MaxProgressions)
	}
	seen := map[int]bool{}
	for _, p := range mustLibrary(t, s, alice).Songs[0].Progressions {
		if seen[p.Position] {
			t.Errorf("position %d used twice", p.Position)
		}
		seen[p.Position] = true
	}
	if len(seen) != MaxProgressions {
		t.Errorf("%d distinct positions, want %d", len(seen), MaxProgressions)
	}
}

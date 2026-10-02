package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type Folder struct {
	ID   int64
	Name string
}

type Song struct {
	ID        int64
	Name      string
	FolderID  *int64 // nil: not in a folder
	UpdatedAt time.Time
}

type Progression struct {
	ID       int64
	SongID   int64
	Name     string
	Position int             // 0-based, always packed 0..n-1 within a song
	Data     json.RawMessage // empty in Library listings
}

type LibrarySong struct {
	Song
	Progressions []Progression // by position; Data left empty
}

type Library struct {
	Folders []Folder      // by name, case-insensitively
	Songs   []LibrarySong // by name, case-insensitively
}

// SongPatch lists changes to a song. A nil Name leaves the name alone; SetFolder false leaves
// the folder alone, and with SetFolder true a nil FolderID unfiles the song.
type SongPatch struct {
	Name      *string
	SetFolder bool
	FolderID  *int64
}

// ProgressionPatch lists changes to a progression; a nil Name or empty Data leaves that field alone.
type ProgressionPatch struct {
	Name *string
	Data json.RawMessage
}

type scanner interface{ Scan(dest ...any) error }

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameRunes {
		return "", fail(ErrInvalid, "name must be 1 to %d characters", MaxNameRunes)
	}
	return name, nil
}

func checkData(data json.RawMessage) error {
	if len(data) > MaxDataBytes {
		return fail(ErrInvalid, "data must be at most %d bytes", MaxDataBytes)
	}
	if !json.Valid(data) {
		return fail(ErrInvalid, "data must be valid JSON")
	}
	return nil
}

// each runs a query with one argument and calls scan for every row.
func each(ctx context.Context, tx *sql.Tx, query string, arg any, scan func(rows *sql.Rows) error) error {
	rows, err := tx.QueryContext(ctx, query, arg)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// affected turns an UPDATE or DELETE that matched no row into ErrNotFound.
func affected(res sql.Result, err error, what string) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound(what)
	}
	return nil
}

const songColumns = `id, name, folder_id, updated_at`

func scanSong(r scanner) (Song, error) {
	var s Song
	var folder sql.NullInt64
	var updated int64
	if err := r.Scan(&s.ID, &s.Name, &folder, &updated); err != nil {
		return Song{}, err
	}
	if folder.Valid {
		s.FolderID = &folder.Int64
	}
	s.UpdatedAt = time.Unix(updated, 0).UTC()
	return s, nil
}

func getSong(ctx context.Context, q queryer, owner string, id int64) (Song, error) {
	s, err := scanSong(q.QueryRowContext(ctx, `SELECT `+songColumns+` FROM songs WHERE id = ? AND owner = ?`, id, owner))
	if errors.Is(err, sql.ErrNoRows) {
		return Song{}, notFound("song")
	}
	return s, err
}

// checkFolder fails with ErrNotFound unless the owner has a folder with this id.
func checkFolder(ctx context.Context, tx *sql.Tx, owner string, id int64) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM folders WHERE id = ? AND owner = ?`, id, owner).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("folder")
	}
	return err
}

// touchSong stamps a song as just changed. It matches only the owner's song, so it is also
// the ownership check for work on that song's progressions.
func touchSong(ctx context.Context, tx *sql.Tx, owner string, id int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE songs SET updated_at = ? WHERE id = ? AND owner = ?`, now(), id, owner)
	return affected(res, err, "song")
}

func getProgression(ctx context.Context, q queryer, owner string, id int64) (Progression, error) {
	var p Progression
	var data string
	err := q.QueryRowContext(ctx,
		`SELECT p.id, p.song_id, p.name, p.position, p.data
		 FROM progressions p JOIN songs s ON s.id = p.song_id
		 WHERE p.id = ? AND s.owner = ?`, id, owner).Scan(&p.ID, &p.SongID, &p.Name, &p.Position, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return Progression{}, notFound("progression")
	}
	p.Data = json.RawMessage(data)
	return p, err
}

// Library lists the owner's folders and songs, with each song's progressions (without their data).
func (s *Store) Library(ctx context.Context, owner string) (Library, error) {
	lib := Library{Folders: []Folder{}, Songs: []LibrarySong{}}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := each(ctx, tx, `SELECT id, name FROM folders WHERE owner = ? ORDER BY name COLLATE NOCASE, name, id`, owner,
			func(rows *sql.Rows) error {
				var f Folder
				if err := rows.Scan(&f.ID, &f.Name); err != nil {
					return err
				}
				lib.Folders = append(lib.Folders, f)
				return nil
			})
		if err != nil {
			return err
		}
		bySong := map[int64]int{} // song id -> index in lib.Songs
		err = each(ctx, tx, `SELECT `+songColumns+` FROM songs WHERE owner = ? ORDER BY name COLLATE NOCASE, name, id`, owner,
			func(rows *sql.Rows) error {
				song, err := scanSong(rows)
				if err != nil {
					return err
				}
				bySong[song.ID] = len(lib.Songs)
				lib.Songs = append(lib.Songs, LibrarySong{Song: song, Progressions: []Progression{}})
				return nil
			})
		if err != nil {
			return err
		}
		return each(ctx, tx,
			`SELECT p.id, p.song_id, p.name, p.position
			 FROM progressions p JOIN songs s ON s.id = p.song_id
			 WHERE s.owner = ? ORDER BY p.song_id, p.position, p.id`, owner,
			func(rows *sql.Rows) error {
				var p Progression
				if err := rows.Scan(&p.ID, &p.SongID, &p.Name, &p.Position); err != nil {
					return err
				}
				song := &lib.Songs[bySong[p.SongID]]
				song.Progressions = append(song.Progressions, p)
				return nil
			})
	})
	if err != nil {
		return Library{}, err
	}
	return lib, nil
}

func (s *Store) CreateFolder(ctx context.Context, owner, name string) (Folder, error) {
	name, err := cleanName(name)
	if err != nil {
		return Folder{}, err
	}
	f := Folder{Name: name}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		n, err := count(ctx, tx, `SELECT COUNT(*) FROM folders WHERE owner = ?`, owner)
		if err != nil {
			return err
		}
		if n >= MaxFolders {
			return fail(ErrLimit, "you can have at most %d folders", MaxFolders)
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO folders (owner, name, created_at) VALUES (?, ?, ?)`, owner, name, now())
		if isUnique(err) {
			return fail(ErrConflict, "a folder named %q already exists", name)
		}
		if err != nil {
			return err
		}
		f.ID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return Folder{}, err
	}
	return f, nil
}

func (s *Store) RenameFolder(ctx context.Context, owner string, id int64, name string) (Folder, error) {
	name, err := cleanName(name)
	if err != nil {
		return Folder{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE folders SET name = ? WHERE id = ? AND owner = ?`, name, id, owner)
	if isUnique(err) {
		return Folder{}, fail(ErrConflict, "a folder named %q already exists", name)
	}
	if err := affected(res, err, "folder"); err != nil {
		return Folder{}, err
	}
	return Folder{ID: id, Name: name}, nil
}

// DeleteFolder removes the folder; its songs are kept and become unfiled.
func (s *Store) DeleteFolder(ctx context.Context, owner string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM folders WHERE id = ? AND owner = ?`, id, owner)
	return affected(res, err, "folder")
}

func (s *Store) CreateSong(ctx context.Context, owner, name string, folderID *int64) (Song, error) {
	name, err := cleanName(name)
	if err != nil {
		return Song{}, err
	}
	var song Song
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if folderID != nil {
			if err := checkFolder(ctx, tx, owner, *folderID); err != nil {
				return err
			}
		}
		n, err := count(ctx, tx, `SELECT COUNT(*) FROM songs WHERE owner = ?`, owner)
		if err != nil {
			return err
		}
		if n >= MaxSongs {
			return fail(ErrLimit, "you can have at most %d songs", MaxSongs)
		}
		t := now()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO songs (owner, folder_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			owner, folderID, name, t, t)
		if err != nil {
			return err
		}
		song = Song{Name: name, FolderID: folderID, UpdatedAt: time.Unix(t, 0).UTC()}
		song.ID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return Song{}, err
	}
	return song, nil
}

func (s *Store) UpdateSong(ctx context.Context, owner string, id int64, patch SongPatch) (Song, error) {
	var name string
	if patch.Name != nil {
		var err error
		if name, err = cleanName(*patch.Name); err != nil {
			return Song{}, err
		}
	}
	var song Song
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if song, err = getSong(ctx, tx, owner, id); err != nil {
			return err
		}
		if patch.Name != nil {
			song.Name = name
		}
		if patch.SetFolder {
			if patch.FolderID != nil {
				if err := checkFolder(ctx, tx, owner, *patch.FolderID); err != nil {
					return err
				}
			}
			song.FolderID = patch.FolderID
		}
		t := now()
		song.UpdatedAt = time.Unix(t, 0).UTC()
		_, err = tx.ExecContext(ctx, `UPDATE songs SET name = ?, folder_id = ?, updated_at = ? WHERE id = ? AND owner = ?`,
			song.Name, song.FolderID, t, id, owner)
		return err
	})
	if err != nil {
		return Song{}, err
	}
	return song, nil
}

// DeleteSong removes the song and, by cascade, its progressions.
func (s *Store) DeleteSong(ctx context.Context, owner string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM songs WHERE id = ? AND owner = ?`, id, owner)
	return affected(res, err, "song")
}

// AddProgression appends a progression to the end of the song. data must be valid JSON.
func (s *Store) AddProgression(ctx context.Context, owner string, songID int64, name string, data json.RawMessage) (Progression, error) {
	name, err := cleanName(name)
	if err == nil {
		err = checkData(data)
	}
	if err != nil {
		return Progression{}, err
	}
	p := Progression{SongID: songID, Name: name, Data: data}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := touchSong(ctx, tx, owner, songID); err != nil {
			return err
		}
		n, err := count(ctx, tx, `SELECT COUNT(*) FROM progressions WHERE song_id = ?`, songID)
		if err != nil {
			return err
		}
		if n >= MaxProgressions {
			return fail(ErrLimit, "a song can have at most %d progressions", MaxProgressions)
		}
		total, err := count(ctx, tx,
			`SELECT COUNT(*) FROM progressions p JOIN songs s ON s.id = p.song_id WHERE s.owner = ?`, owner)
		if err != nil {
			return err
		}
		if total >= MaxProgressionsPerUser {
			return fail(ErrLimit, "you can have at most %d progressions across all your songs", MaxProgressionsPerUser)
		}
		p.Position = n
		t := now()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO progressions (song_id, name, position, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			songID, name, n, string(data), t, t)
		if err != nil {
			return err
		}
		p.ID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return Progression{}, err
	}
	return p, nil
}

func (s *Store) GetProgression(ctx context.Context, owner string, id int64) (Progression, error) {
	return getProgression(ctx, s.db, owner, id)
}

func (s *Store) UpdateProgression(ctx context.Context, owner string, id int64, patch ProgressionPatch) (Progression, error) {
	var name string
	if patch.Name != nil {
		var err error
		if name, err = cleanName(*patch.Name); err != nil {
			return Progression{}, err
		}
	}
	if len(patch.Data) > 0 {
		if err := checkData(patch.Data); err != nil {
			return Progression{}, err
		}
	}
	var p Progression
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if p, err = getProgression(ctx, tx, owner, id); err != nil {
			return err
		}
		if patch.Name != nil {
			p.Name = name
		}
		if len(patch.Data) > 0 {
			p.Data = patch.Data
		}
		_, err = tx.ExecContext(ctx, `UPDATE progressions SET name = ?, data = ?, updated_at = ? WHERE id = ?`,
			p.Name, string(p.Data), now(), id)
		if err != nil {
			return err
		}
		return touchSong(ctx, tx, owner, p.SongID)
	})
	if err != nil {
		return Progression{}, err
	}
	return p, nil
}

// DeleteProgression removes the progression and re-packs its song's remaining positions to 0..n-1.
func (s *Store) DeleteProgression(ctx context.Context, owner string, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getProgression(ctx, tx, owner, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM progressions WHERE id = ?`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE progressions SET position = position - 1 WHERE song_id = ? AND position > ?`,
			p.SongID, p.Position)
		if err != nil {
			return err
		}
		return touchSong(ctx, tx, owner, p.SongID)
	})
}

// ReorderProgressions sets the song's order to ids, which must list each of its progressions exactly once.
func (s *Store) ReorderProgressions(ctx context.Context, owner string, songID int64, ids []int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := touchSong(ctx, tx, owner, songID); err != nil {
			return err
		}
		remaining := map[int64]bool{}
		err := each(ctx, tx, `SELECT id FROM progressions WHERE song_id = ?`, songID, func(rows *sql.Rows) error {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			remaining[id] = true
			return nil
		})
		if err != nil {
			return err
		}
		mismatch := fail(ErrInvalid, "ids must list each of the song's progressions exactly once")
		if len(ids) != len(remaining) {
			return mismatch
		}
		for _, id := range ids {
			if !remaining[id] {
				return mismatch
			}
			delete(remaining, id) // a repeated id is no longer found
		}
		for position, id := range ids {
			_, err := tx.ExecContext(ctx, `UPDATE progressions SET position = ? WHERE id = ? AND song_id = ?`, position, id, songID)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

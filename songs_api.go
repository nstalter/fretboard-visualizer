package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nstalter/fretboard-visualizer/guitar"
	"github.com/nstalter/fretboard-visualizer/store"
)

const (
	maxSongsBody = 256 << 10
	maxLabelLen  = 20 // a saved step's name and numeral, in characters
	maxRootLen   = 8  // a saved step's root, in bytes: the longest the UI offers is F♯♯, 7
	maxSteps     = 64
)

type songAPI struct {
	store *store.Store
}

type ownerHandler func(w http.ResponseWriter, r *http.Request, owner string)

// registerSongRoutes serves the signed-in user's library of folders, songs and progressions.
// Every route answers 401 unless user reports a signed-in owner, and guard wraps every route
// that is not a GET.
func registerSongRoutes(mux *http.ServeMux, s *store.Store, user func(*http.Request) (string, bool), guard func(http.Handler) http.Handler) {
	a := songAPI{s}
	handle := func(pattern string, h ownerHandler) {
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store") // private to the signed-in user
			owner, ok := user(r)
			if !ok {
				writeStatusError(w, http.StatusUnauthorized, "sign in required")
				return
			}
			h(w, r, owner)
		})
		if !strings.HasPrefix(pattern, "GET ") {
			handler = guard(handler)
		}
		mux.Handle(pattern, handler)
	}
	handle("GET /api/library", a.library)
	handle("POST /api/folders", a.createFolder)
	handle("PATCH /api/folders/{id}", a.renameFolder)
	handle("DELETE /api/folders/{id}", a.deleteFolder)
	handle("POST /api/songs", a.createSong)
	handle("PATCH /api/songs/{id}", a.updateSong)
	handle("DELETE /api/songs/{id}", a.deleteSong)
	handle("POST /api/songs/{id}/progressions", a.addProgression)
	handle("PUT /api/songs/{id}/progressions/order", a.reorderProgressions)
	handle("GET /api/progressions/{id}", a.getProgression)
	handle("PUT /api/progressions/{id}", a.updateProgression)
	handle("DELETE /api/progressions/{id}", a.deleteProgression)
}

type folderJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type songJSON struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FolderID *int64 `json:"folderId"`
}

type progressionRefJSON struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type librarySongJSON struct {
	songJSON
	UpdatedAt    string               `json:"updatedAt"`
	Progressions []progressionRefJSON `json:"progressions"`
}

type progressionJSON struct {
	ID       int64           `json:"id"`
	SongID   int64           `json:"songId"`
	Name     string          `json:"name"`
	Position int             `json:"position"`
	Data     json.RawMessage `json:"data"`
}

func newSongJSON(s store.Song) songJSON { return songJSON{s.ID, s.Name, s.FolderID} }

func newProgressionJSON(p store.Progression) progressionJSON {
	return progressionJSON{p.ID, p.SongID, p.Name, p.Position, p.Data}
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeStatusError(w http.ResponseWriter, status int, msg string) {
	writeJSONStatus(w, status, map[string]string{"error": msg})
}

// writeStoreError reports a store error with its HTTP status. The store's own errors carry
// client-safe messages; anything else is logged and shown only as a bare 500.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeStatusError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrInvalid):
		writeStatusError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrLimit), errors.Is(err, store.ErrConflict):
		writeStatusError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("songs api: %v", err)
		writeStatusError(w, http.StatusInternalServerError, "internal error")
	}
}

// decodeBody reads the request's one JSON value into v, refusing unknown fields, trailing
// data and bodies over 256 KiB. It writes the error response itself and reports whether to go on.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSongsBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		if _, tokErr := dec.Token(); tokErr != io.EOF {
			err = errors.New("unexpected data after the JSON value")
		}
	}
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeStatusError(w, http.StatusRequestEntityTooLarge, "request body too large")
	} else {
		writeStatusError(w, http.StatusBadRequest, paramError("body", err).Error())
	}
	return false
}

// pathID reads the {id} path value. An id that cannot exist is just a missing resource.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	n, err := parseInt(r.PathValue("id"), 1, math.MaxInt)
	if err != nil {
		writeStatusError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return int64(n), true
}

// folderIDField is the folderId of a song patch, which can be absent (leave the folder alone),
// null (unfile the song) or an integer (move it): Set is true when the field was present,
// and ID is nil when it was null.
type folderIDField struct {
	Set bool
	ID  *int64
}

func (f *folderIDField) UnmarshalJSON(b []byte) error {
	f.Set = true
	if string(b) == "null" {
		return nil
	}
	var id int64
	if err := json.Unmarshal(b, &id); err != nil {
		return errors.New("folderId must be an integer or null")
	}
	f.ID = &id
	return nil
}

func (a songAPI) library(w http.ResponseWriter, r *http.Request, owner string) {
	lib, err := a.store.Library(r.Context(), owner)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := struct {
		Folders []folderJSON      `json:"folders"`
		Songs   []librarySongJSON `json:"songs"`
	}{[]folderJSON{}, []librarySongJSON{}}
	for _, f := range lib.Folders {
		out.Folders = append(out.Folders, folderJSON(f))
	}
	for _, s := range lib.Songs {
		refs := []progressionRefJSON{}
		for _, p := range s.Progressions {
			refs = append(refs, progressionRefJSON{p.ID, p.Name, p.Position})
		}
		out.Songs = append(out.Songs, librarySongJSON{newSongJSON(s.Song), s.UpdatedAt.UTC().Format(time.RFC3339), refs})
	}
	writeJSON(w, out)
}

func (a songAPI) createFolder(w http.ResponseWriter, r *http.Request, owner string) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	f, err := a.store.CreateFolder(r.Context(), owner, req.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, folderJSON(f))
}

func (a songAPI) renameFolder(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	f, err := a.store.RenameFolder(r.Context(), owner, id, req.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, folderJSON(f))
}

func (a songAPI) deleteFolder(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteFolder(r.Context(), owner, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a songAPI) createSong(w http.ResponseWriter, r *http.Request, owner string) {
	var req struct {
		Name     string `json:"name"`
		FolderID *int64 `json:"folderId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	song, err := a.store.CreateSong(r.Context(), owner, req.Name, req.FolderID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, newSongJSON(song))
}

func (a songAPI) updateSong(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     *string       `json:"name"`
		FolderID folderIDField `json:"folderId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	song, err := a.store.UpdateSong(r.Context(), owner, id, store.SongPatch{Name: req.Name, SetFolder: req.FolderID.Set, FolderID: req.FolderID.ID})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, newSongJSON(song))
}

func (a songAPI) deleteSong(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteSong(r.Context(), owner, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a songAPI) addProgression(w http.ResponseWriter, r *http.Request, owner string) {
	songID, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string          `json:"name"`
		Data json.RawMessage `json:"data"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	data, err := canonicalProgressionData(req.Data)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := a.store.AddProgression(r.Context(), owner, songID, req.Name, data)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, progressionRefJSON{p.ID, p.Name, p.Position})
}

func (a songAPI) reorderProgressions(w http.ResponseWriter, r *http.Request, owner string) {
	songID, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := a.store.ReorderProgressions(r.Context(), owner, songID, req.IDs); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a songAPI) getProgression(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := a.store.GetProgression(r.Context(), owner, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, newProgressionJSON(p))
}

func (a songAPI) updateProgression(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name *string         `json:"name"`
		Data json.RawMessage `json:"data"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	patch := store.ProgressionPatch{Name: req.Name}
	if req.Data != nil {
		data, err := canonicalProgressionData(req.Data)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, err.Error())
			return
		}
		patch.Data = data
	}
	p, err := a.store.UpdateProgression(r.Context(), owner, id, patch)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, newProgressionJSON(p))
}

func (a songAPI) deleteProgression(w http.ResponseWriter, r *http.Request, owner string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteProgression(r.Context(), owner, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stepData is one saved chord. Fingering is null for a step with no playable shape.
type stepData struct {
	Root      string  `json:"root"`
	Quality   string  `json:"quality"`
	Inversion string  `json:"inversion"`
	Name      string  `json:"name"`
	Numeral   *string `json:"numeral"`
	Fingering *string `json:"fingering"`
}

// progressionData is what the UI saves for a progression.
type progressionData struct {
	Tuning  string     `json:"tuning"`
	OpenMax *int       `json:"openMax"`
	BPM     float64    `json:"bpm"` // the UI does not round it
	Beats   int        `json:"beats"`
	Steps   []stepData `json:"steps"`
}

// canonicalProgressionData validates a progression's data and returns it re-marshalled from
// the known fields only, so nothing else a client sends is ever stored. Tuning and roots are
// rebuilt from their parsed values, so padding (spaces, leading zeros) is never stored either.
func canonicalProgressionData(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, paramError("data", errRequired)
	}
	var d progressionData
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, paramError("data", err)
	}
	if err := d.validate(); err != nil {
		return nil, paramError("data", err)
	}
	return json.Marshal(d)
}

// formatTuning is the inverse of parseTuningParam: pitches low to high, as the UI writes them.
func formatTuning(t guitar.Tuning) string {
	pitches := make([]string, len(t.Strings))
	for i := range pitches {
		n := t.Strings[len(t.Strings)-1-i]
		pitches[i] = n.String() + strconv.Itoa(n.Octave)
	}
	return strings.Join(pitches, ",")
}

func (d *progressionData) validate() error {
	tuning, err := parseTuningParam(d.Tuning)
	if err != nil {
		return err
	}
	d.Tuning = formatTuning(tuning)
	if d.OpenMax == nil {
		return paramError("openMax", errRequired)
	}
	if _, err := parseOpenMax(strconv.Itoa(*d.OpenMax)); err != nil {
		return err
	}
	if d.BPM < 20 || d.BPM > 300 {
		return paramError("bpm", fmt.Errorf("want a number from 20 to 300, got %v", d.BPM))
	}
	if d.Beats < 1 || d.Beats > 16 {
		return paramError("beats", fmt.Errorf("want an integer from 1 to 16, got %d", d.Beats))
	}
	if n := len(d.Steps); n < 1 || n > maxSteps {
		return paramError("steps", fmt.Errorf("want 1 to %d steps, got %d", maxSteps, n))
	}
	for i := range d.Steps {
		if err := d.Steps[i].validate(); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *stepData) validate() error {
	if len(s.Root) > maxRootLen { // ParseNote accepts any number of accidentals
		return paramError("root", fmt.Errorf("want at most %d bytes", maxRootLen))
	}
	chord, inv, err := parseChordParams(s.Root, s.Quality, s.Inversion)
	if err != nil {
		return err
	}
	// Quality and inversion are matched against their ids exactly, so they are canonical already.
	s.Root = chord.Root.String()
	s.Inversion = inv.String() // an omitted inversion is "any"
	if utf8.RuneCountInString(s.Name) > maxLabelLen {
		return paramError("name", fmt.Errorf("want at most %d characters", maxLabelLen))
	}
	if s.Numeral != nil && utf8.RuneCountInString(*s.Numeral) > maxLabelLen {
		return paramError("numeral", fmt.Errorf("want at most %d characters", maxLabelLen))
	}
	if s.Fingering != nil {
		if _, err := guitar.ParseFingering(*s.Fingering); err != nil {
			return paramError("fingering", err)
		}
	}
	return nil
}

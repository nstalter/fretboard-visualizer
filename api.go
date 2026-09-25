package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/nstalter/fretboard-visualizer/guitar"
	"github.com/nstalter/fretboard-visualizer/music"
)

func newMux(static fs.FS) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/qualities", handleQualities)
	mux.HandleFunc("GET /api/voicings", handleVoicings)
	mux.HandleFunc("GET /api/diatonic", handleDiatonic)
	mux.HandleFunc("POST /api/progression", handleProgression)
	mux.Handle("GET /", http.FileServerFS(static))
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func paramError(param string, err error) error {
	return fmt.Errorf("%s: %w", param, err)
}

var errRequired = errors.New("required")

// parseChordParams validates root, quality and inversion (default "any").
func parseChordParams(root, quality, inversion string) (music.Chord, music.Inversion, error) {
	if root == "" {
		return music.Chord{}, 0, paramError("root", errRequired)
	}
	r, err := music.ParseNote(root)
	if err != nil {
		return music.Chord{}, 0, paramError("root", err)
	}
	if quality == "" {
		return music.Chord{}, 0, paramError("quality", errRequired)
	}
	q, err := music.ParseChordQuality(quality)
	if err != nil {
		return music.Chord{}, 0, paramError("quality", err)
	}
	if inversion == "" {
		inversion = "any"
	}
	inv, err := music.ParseInversion(inversion)
	if err != nil {
		return music.Chord{}, 0, paramError("inversion", err)
	}
	offered := false
	for _, i := range q.Inversions() {
		offered = offered || i == inv
	}
	if !offered {
		return music.Chord{}, 0, paramError("inversion", fmt.Errorf("%s is not offered for %s", inv, q))
	}
	return music.Chord{Root: r, Quality: q}, inv, nil
}

func parseTuningParam(s string) (guitar.Tuning, error) {
	if s == "" {
		return guitar.Tuning{}, paramError("tuning", errRequired)
	}
	t, err := guitar.ParseTuning(s)
	if err != nil {
		return guitar.Tuning{}, paramError("tuning", err)
	}
	return t, nil
}

// parseInt parses a canonical base-10 integer in [lo, hi]: no sign, no leading zeros.
func parseInt(s string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || strconv.Itoa(n) != s || n < lo || n > hi {
		return 0, fmt.Errorf("want an integer from %d to %d, got %q", lo, hi, s)
	}
	return n, nil
}

// parseOpenMax validates the highest fret at which a shape may still use an open string.
// Empty means the default; guitar.MaxFret means no limit.
func parseOpenMax(s string) (int, error) {
	if s == "" {
		return guitar.DefaultMaxOpenFret, nil
	}
	n, err := parseInt(s, 0, guitar.MaxFret)
	if err != nil {
		return 0, paramError("openMax", err)
	}
	return n, nil
}

func generate(t guitar.Tuning, c music.Chord, inv music.Inversion, openMax int) []guitar.GuitarChordVoicing {
	g := guitar.NewGuitarVoicingGenerator(t, guitar.MaxFret)
	g.MaxOpenFret = openMax
	return g.GenerateVoicings(c, inv)
}

type qualityJSON struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	Inversions []string `json:"inversions"`
}

func handleQualities(w http.ResponseWriter, r *http.Request) {
	qs := []qualityJSON{}
	for _, q := range music.AllChordQualities() {
		invs := []string{}
		for _, i := range q.Inversions() {
			invs = append(invs, i.String())
		}
		qs = append(qs, qualityJSON{ID: q.String(), Label: q.Label(), Inversions: invs})
	}
	writeJSON(w, map[string]any{"qualities": qs})
}

type toneJSON struct {
	Note     string `json:"note"`
	Interval string `json:"interval"`
}

type chordJSON struct {
	Root    string     `json:"root"`
	Quality string     `json:"quality"`
	Name    string     `json:"name"`
	Tones   []toneJSON `json:"tones"`
}

type barreJSON struct {
	Fret int `json:"fret"`
	From int `json:"from"`
	To   int `json:"to"`
}

type voicingJSON struct {
	Fingering string     `json:"fingering"`
	Frets     [6]int     `json:"frets"`
	Barre     *barreJSON `json:"barre"`
	Tones     [6]int     `json:"tones"`
}

type voicingsResponse struct {
	Chord chordJSON `json:"chord"`
	// Fretboard[string][fret] indexes Chord.Tones for every string (low→high) and fret
	// 0–22, or is -1 for a note that is not in the chord. It does not depend on the voicings.
	Fretboard    [6][guitar.MaxFret + 1]int `json:"fretboard"`
	Voicings     []voicingJSON              `json:"voicings"`
	NearestIndex *int                       `json:"nearestIndex,omitempty"`
	AtMatched    *bool                      `json:"atMatched,omitempty"`
}

// parseAt parses a clicked spot "S:F": S is the string index low→high (0–5, the same
// index as `frets`) and F the fret (0–MaxFret). It returns the internal high→low index.
func parseAt(s string) (str, fret int, err error) {
	ss, fs, ok := strings.Cut(s, ":")
	str, err1 := parseInt(ss, 0, 5)
	fret, err2 := parseInt(fs, 0, guitar.MaxFret)
	if !ok || err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("want string:fret with string 0–5 and fret 0–%d, got %q", guitar.MaxFret, s)
	}
	return 5 - str, fret, nil
}

func newChordJSON(c music.Chord) chordJSON {
	cj := chordJSON{Root: c.Root.String(), Quality: c.Quality.String(), Name: c.Name(), Tones: []toneJSON{}}
	for _, t := range c.Quality.Tones() {
		cj.Tones = append(cj.Tones, toneJSON{Note: c.Spell(t).String(), Interval: t.Label()})
	}
	return cj
}

// newVoicingJSON converts the internal high→low order to the API's low→high order.
func newVoicingJSON(v guitar.GuitarChordVoicing, t guitar.Tuning) voicingJSON {
	vj := voicingJSON{Fingering: guitar.FormatFingering(v.Fingering)}
	tones := v.ToneIndices(t)
	for i := range 6 {
		vj.Frets[i] = v.Fingering[5-i]
		vj.Tones[i] = tones[5-i]
	}
	if v.Barre != nil {
		vj.Barre = &barreJSON{Fret: v.Barre.Fret, From: 5 - v.Barre.EndString, To: 5 - v.Barre.StartString}
	}
	return vj
}

func handleVoicings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	chord, inv, err := parseChordParams(q.Get("root"), q.Get("quality"), q.Get("inversion"))
	if err != nil {
		writeError(w, err)
		return
	}
	tuning, err := parseTuningParam(q.Get("tuning"))
	if err != nil {
		writeError(w, err)
		return
	}
	var near *[6]int
	if s := q.Get("near"); s != "" {
		f, err := guitar.ParseFingering(s)
		if err != nil {
			writeError(w, paramError("near", err))
			return
		}
		near = &f
	}
	openMax, err := parseOpenMax(q.Get("openMax"))
	if err != nil {
		writeError(w, err)
		return
	}
	var at *[2]int
	if s := q.Get("at"); s != "" {
		str, fret, err := parseAt(s)
		if err != nil {
			writeError(w, paramError("at", err))
			return
		}
		at = &[2]int{str, fret}
	}

	vs := generate(tuning, chord, inv, openMax)
	resp := voicingsResponse{Chord: newChordJSON(chord), Voicings: []voicingJSON{}}
	high2low := guitar.FretboardTones(chord, tuning)
	for s := range resp.Fretboard {
		resp.Fretboard[s] = high2low[5-s]
	}
	for _, v := range vs {
		resp.Voicings = append(resp.Voicings, newVoicingJSON(v, tuning))
	}
	switch {
	case len(vs) == 0:
	case at != nil:
		i, matched := guitar.NearestIndexAt(vs, near, at[0], at[1])
		resp.NearestIndex, resp.AtMatched = &i, &matched
	case near != nil:
		i := guitar.NearestIndex(vs, *near)
		resp.NearestIndex = &i
	}
	writeJSON(w, resp)
}

type diatonicJSON struct {
	Degree  int    `json:"degree"`
	Root    string `json:"root"`
	Quality string `json:"quality"`
	Name    string `json:"name"`
	Numeral string `json:"numeral"`
}

func handleDiatonic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("key") == "" {
		writeError(w, paramError("key", errRequired))
		return
	}
	key, err := music.ParseNote(q.Get("key"))
	if err != nil {
		writeError(w, paramError("key", err))
		return
	}
	if q.Get("mode") == "" {
		writeError(w, paramError("mode", errRequired))
		return
	}
	mode, err := music.ParseMode(q.Get("mode"))
	if err != nil {
		writeError(w, paramError("mode", err))
		return
	}
	chords := []diatonicJSON{}
	for _, dc := range music.DiatonicChords(key, mode) {
		c := music.Chord{Root: dc.Root, Quality: dc.Quality}
		chords = append(chords, diatonicJSON{dc.Degree, dc.Root.String(), dc.Quality.String(), c.Name(), dc.Numeral})
	}
	writeJSON(w, map[string]any{"chords": chords})
}

type progressionRequest struct {
	Tuning  string `json:"tuning"`
	OpenMax *int   `json:"openMax"` // optional; omitted means the default
	Steps   []struct {
		Root      string `json:"root"`
		Quality   string `json:"quality"`
		Inversion string `json:"inversion"`
	} `json:"steps"`
}

// handleProgression picks a shape per step, each nearest the last non-null shape before it.
// Until some step has a shape, a step gets its lowest-fret voicing.
func handleProgression(w http.ResponseWriter, r *http.Request) {
	var req progressionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, paramError("body", err))
		return
	}
	tuning, err := parseTuningParam(req.Tuning)
	if err != nil {
		writeError(w, err)
		return
	}
	openMax := guitar.DefaultMaxOpenFret
	if req.OpenMax != nil {
		openMax = *req.OpenMax
		if openMax < 0 || openMax > guitar.MaxFret {
			writeError(w, paramError("openMax", fmt.Errorf("want an integer from 0 to %d, got %d", guitar.MaxFret, openMax)))
			return
		}
	}
	type step struct {
		chord music.Chord
		inv   music.Inversion
	}
	steps := make([]step, len(req.Steps))
	for i, s := range req.Steps {
		chord, inv, err := parseChordParams(s.Root, s.Quality, s.Inversion)
		if err != nil {
			writeError(w, fmt.Errorf("step %d: %w", i+1, err))
			return
		}
		steps[i] = step{chord, inv}
	}

	shapes := []*string{}
	var prev *[6]int
	for _, s := range steps {
		vs := generate(tuning, s.chord, s.inv, openMax)
		if len(vs) == 0 {
			shapes = append(shapes, nil)
			continue
		}
		i := 0
		if prev != nil {
			i = guitar.NearestIndex(vs, *prev)
		}
		f := vs[i].Fingering
		prev = &f
		shape := guitar.FormatFingering(f)
		shapes = append(shapes, &shape)
	}
	writeJSON(w, map[string]any{"shapes": shapes})
}

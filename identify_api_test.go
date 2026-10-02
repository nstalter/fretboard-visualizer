package main

import (
	"encoding/json"
	"net/url"
	"slices"
	"testing"

	"github.com/nstalter/fretboard-visualizer/guitar"
	"github.com/nstalter/fretboard-visualizer/music"
)

const stdTuning = "E2,A2,D3,G3,B3,E4"

func identifyGet(t *testing.T, fingering string, extra ...string) (int, identifyResponse, string) {
	t.Helper()
	srv := newServer(t)
	params := url.Values{"fingering": {fingering}, "tuning": {stdTuning}, "openMax": {"5"}}
	for i := 0; i+1 < len(extra); i += 2 {
		params.Set(extra[i], extra[i+1])
	}
	code, body := get(t, srv, "/api/identify", params)
	var resp identifyResponse
	var e struct{ Error string }
	if code == 200 {
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
	} else {
		json.Unmarshal(body, &e)
	}
	return code, resp, e.Error
}

func TestIdentify_C7Shape(t *testing.T) {
	for _, shape := range []string{"x32310", "x3231x"} {
		code, resp, _ := identifyGet(t, shape)
		if code != 200 || !resp.Playable || resp.Issue != "" || len(resp.Chords) == 0 {
			t.Fatalf("%s: %d %+v", shape, code, resp)
		}
		best := resp.Chords[0]
		if best.Name != "C7" || best.Root != "C" || best.Quality != "7" || best.Inversion != "root" {
			t.Errorf("%s: best = %s %s/%s, want C7 root", shape, best.Name, best.Quality, best.Inversion)
		}
		if best.Voicing.Fingering != shape {
			t.Errorf("%s: voicing fingering = %s", shape, best.Voicing.Fingering)
		}
	}
}

func TestIdentify_AlternativesAndInversion(t *testing.T) {
	// Open Am7 with A in the bass: the same notes are C6, which offers no 6th-in-bass inversion.
	_, resp, _ := identifyGet(t, "x02010")
	var names []string
	for _, c := range resp.Chords {
		names = append(names, c.Name+"/"+c.Inversion)
	}
	if len(resp.Chords) == 0 || names[0] != "Am7/root" || !slices.Contains(names, "C6/any") {
		t.Errorf("x02010: %v, want Am7/root first and C6/any among them", names)
	}
	// C E G C E with the E in the bass is C major in 3rd inversion.
	_, resp, _ = identifyGet(t, "0x2010")
	if len(resp.Chords) == 0 || resp.Chords[0].Name != "C" || resp.Chords[0].Inversion != "3rd" {
		t.Errorf("0x2010: %+v, want C with its 3rd in the bass", resp.Chords)
	}
}

func TestIdentify_Unfinished(t *testing.T) {
	for shape, want := range map[string]string{
		"xxxxxx": "a chord needs at least three notes",
		"x3xxxx": "a chord needs at least three notes",
		"x3x2xx": "a chord needs at least three notes",
		"x3x23x": "a muted string sits between played strings",
	} {
		code, resp, _ := identifyGet(t, shape)
		if code != 200 || resp.Playable || resp.Issue != want {
			t.Errorf("%s: %d playable=%v issue=%q, want 200 unplayable %q", shape, code, resp.Playable, resp.Issue, want)
		}
	}
}

func TestIdentify_RejectsWhatNoNoteCanFix(t *testing.T) {
	for shape, want := range map[string]string{
		"x1xx5x":      "that stretches over more than four frets",
		"x-1-2-3-4-4": "that needs more than four fingers",
		"0xx7xx":      "an open string can't be used with a note above fret 5 (see the open-string limit)",
	} {
		code, _, msg := identifyGet(t, shape)
		if code != 422 || msg != want {
			t.Errorf("%s: %d %q, want 422 %q", shape, code, msg, want)
		}
	}
}

func TestIdentify_UnknownChord(t *testing.T) {
	tuning, _ := parseTuningParam(stdTuning)
	var shape [6]int
	found := false
	// The first playable three-note shape whose notes are no chord we know.
	for a := -1; a <= 4 && !found; a++ {
		for b := -1; b <= 4 && !found; b++ {
			for c := -1; c <= 4 && !found; c++ {
				f := [6]int{-1, -1, c, b, a, -1}
				if guitar.PlayIssue(f, 5) == "" && len(guitar.IdentifyChord(f, tuning)) == 0 {
					shape, found = f, true
				}
			}
		}
	}
	if !found {
		t.Skip("every small shape is a known chord")
	}
	code, resp, _ := identifyGet(t, guitar.FormatFingering(shape))
	if code != 200 || len(resp.Chords) != 0 || !resp.Playable || resp.Issue != "those notes don't spell a chord this app knows" {
		t.Errorf("%v: %d %+v", shape, code, resp)
	}
}

func TestIdentify_BadParams(t *testing.T) {
	for name, params := range map[string][]string{
		"missing fingering": {"fingering", ""},
		"short fingering":   {"fingering", "x3231"},
		"bad fret":          {"fingering", "x3231z"},
		"fret past 22":      {"fingering", "x-3-2-3-1-23"},
		"bad tuning":        {"tuning", "EADGBE"},
		"bad openMax":       {"openMax", "23"},
	} {
		if code, _, _ := identifyGet(t, "x32310", params...); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
}

// Whatever the endpoint calls a playable shape, the voicing generator offers for that chord and
// inversion, so adding it to a progression and loading it back always finds it.
func TestIdentify_PlayableShapesAreGeneratorVoicings(t *testing.T) {
	tuning, _ := parseTuningParam(stdTuning)
	const openMax = 5
	type key struct {
		root, quality string
		inv           music.Inversion
	}
	offered := map[key][]string{}
	var f [6]int
	checked, chords := 0, 0
	var walk func(s int)
	walk = func(s int) {
		if s == 6 {
			if guitar.PlayIssue(f, openMax) != "" {
				return
			}
			resp, limit := identify(f, tuning, openMax)
			if limit != "" {
				t.Fatalf("%v: playable shape rejected as a limit: %s", f, limit)
			}
			for _, c := range resp.Chords {
				inv, err := music.ParseInversion(c.Inversion)
				if err != nil {
					t.Fatal(err)
				}
				chord, _, err := parseChordParams(c.Root, c.Quality, c.Inversion)
				if err != nil {
					t.Fatalf("%v: response names an unparseable chord: %v", f, err)
				}
				k := key{c.Root, c.Quality, inv}
				if _, ok := offered[k]; !ok {
					for _, v := range generate(tuning, chord, inv, openMax) {
						offered[k] = append(offered[k], guitar.FormatFingering(v.Fingering))
					}
					chords++
				}
				if !slices.Contains(offered[k], guitar.FormatFingering(f)) {
					t.Fatalf("%v is called %s/%s but the generator does not offer it", guitar.FormatFingering(f), c.Name, c.Inversion)
				}
			}
			checked++
			return
		}
		for fret := -1; fret <= 3; fret++ {
			f[s] = fret
			walk(s + 1)
		}
	}
	walk(0)
	t.Logf("%d playable shapes over %d distinct chord/inversions", checked, chords)
	if checked < 500 {
		t.Errorf("only %d playable shapes checked", checked)
	}
}

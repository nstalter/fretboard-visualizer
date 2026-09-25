package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	std   = "E2,A2,D3,G3,B3,E4"
	dropD = "D2,A2,D3,G3,B3,E4"
)

type voicingsResp struct {
	Chord struct {
		Root, Quality, Name string
		Tones               []struct{ Note, Interval string }
	}
	Voicings []struct {
		Fingering string
		Frets     []int
		Barre     *struct{ Fret, From, To int }
		Tones     []int
	}
	NearestIndex *int
	Fretboard    [][]int
}

func (r voicingsResp) index(fingering string) int {
	for i, v := range r.Voicings {
		if v.Fingering == fingering {
			return i
		}
	}
	return -1
}

func (r voicingsResp) fingerings() []string {
	var out []string
	for _, v := range r.Voicings {
		out = append(out, v.Fingering)
	}
	return out
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newMux(static))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, params url.Values) (int, []byte) {
	t.Helper()
	res, err := http.Get(srv.URL + path + "?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, body
}

func post(t *testing.T, srv *httptest.Server, path, body string) (int, []byte) {
	t.Helper()
	res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func voicings(t *testing.T, srv *httptest.Server, params url.Values) voicingsResp {
	t.Helper()
	code, body := get(t, srv, "/api/voicings", params)
	if code != 200 {
		t.Fatalf("GET /api/voicings?%s = %d: %s", params.Encode(), code, body)
	}
	var r voicingsResp
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func chordParams(root, quality, inversion, tuning string) url.Values {
	v := url.Values{"root": {root}, "quality": {quality}, "tuning": {tuning}}
	if inversion != "" {
		v.Set("inversion", inversion)
	}
	return v
}

func expect400(t *testing.T, code int, body []byte, contains string) {
	t.Helper()
	var e struct{ Error string }
	json.Unmarshal(body, &e)
	if code != 400 || e.Error == "" || !strings.Contains(e.Error, contains) {
		t.Errorf("got %d %s, want 400 with error containing %q", code, body, contains)
	}
}

func TestStatic_ServesPageAndModules(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(body), `type="module"`) {
		t.Errorf("/ = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, path := range []string{"/js/main.js", "/js/api.js", "/js/fretboard.js", "/js/tuning.js", "/js/progression.js"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
			t.Errorf("%s = %d %q", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
}

func TestStatic_PostIs405(t *testing.T) {
	srv := newServer(t)
	code, _ := post(t, srv, "/api/voicings", "")
	if code != 405 {
		t.Errorf("POST /api/voicings = %d, want 405", code)
	}
}

func TestVoicings_CMajorIncludesX32010(t *testing.T) {
	r := voicings(t, newServer(t), chordParams("C", "maj", "", std))
	i := r.index("x32010")
	if i < 0 {
		t.Fatal("x32010 missing")
	}
	if got := r.Voicings[i].Tones; !slices.Equal(got, []int{-1, 0, 1, 2, 0, 1}) {
		t.Errorf("tones = %v, want [-1 0 1 2 0 1]", got)
	}
	if got := r.Voicings[i].Frets; !slices.Equal(got, []int{-1, 3, 2, 0, 1, 0}) {
		t.Errorf("frets = %v", got)
	}
	if r.Chord.Name != "C" {
		t.Errorf("name = %q, want C", r.Chord.Name)
	}
	if r.NearestIndex != nil {
		t.Error("nearestIndex present without near")
	}
}

func TestVoicings_BarreLowToHigh(t *testing.T) {
	srv := newServer(t)
	c := voicings(t, srv, chordParams("C", "maj", "", std))
	if i := c.index("x35553"); i < 0 || c.Voicings[i].Barre == nil || *c.Voicings[i].Barre != (struct{ Fret, From, To int }{3, 1, 5}) {
		t.Errorf("x35553 barre wrong: %+v", c.Voicings[max(i, 0)].Barre)
	}
	if i := c.index("x32010"); c.Voicings[i].Barre != nil {
		t.Error("x32010 has a barre")
	}
	f := voicings(t, srv, chordParams("F", "maj", "", std))
	if i := f.index("133211"); i < 0 || f.Voicings[i].Barre == nil || *f.Voicings[i].Barre != (struct{ Fret, From, To int }{1, 0, 5}) {
		t.Errorf("133211 barre wrong")
	}
}

func TestVoicings_RejectsBadParams(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		name     string
		params   url.Values
		contains string
	}{
		{"missing root", url.Values{"quality": {"maj"}, "tuning": {std}}, "root"},
		{"missing quality", url.Values{"root": {"C"}, "tuning": {std}}, "quality"},
		{"missing tuning", url.Values{"root": {"C"}, "quality": {"maj"}}, "tuning"},
		{"bad root", chordParams("H", "maj", "", std), "root"},
		{"case-sensitive quality", chordParams("C", "M7", "", std), "quality"},
		{"5-note tuning", chordParams("C", "maj", "", "E2,A2,D3,G3,B3"), "tuning"},
		{"tuning without octaves", chordParams("C", "maj", "", "E,A,D,G,B,E"), "tuning"},
		{"bad inversion", chordParams("C", "maj", "9th", std), "inversion"},
		{"bad near", func() url.Values { v := chordParams("C", "maj", "", std); v.Set("near", "x3201"); return v }(), "near"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := get(t, srv, "/api/voicings", c.params)
			expect400(t, code, body, c.contains)
		})
	}
}

func TestVoicings_SortedAndStable(t *testing.T) {
	srv := newServer(t)
	a := voicings(t, srv, chordParams("C", "13", "", std))
	b := voicings(t, srv, chordParams("C", "13", "", std))
	if !reflect.DeepEqual(a, b) {
		t.Error("two identical calls differ")
	}
	minFret := func(frets []int) int {
		m := 0
		for _, f := range frets {
			if f > 0 && (m == 0 || f < m) {
				m = f
			}
		}
		return m
	}
	for i := 1; i < len(a.Voicings); i++ {
		if minFret(a.Voicings[i].Frets) < minFret(a.Voicings[i-1].Frets) {
			t.Fatalf("lowest fret decreases at %d", i)
		}
	}
}

func TestVoicings_C9ContainsR3b7_9(t *testing.T) {
	r := voicings(t, newServer(t), chordParams("C", "9", "", std))
	if len(r.Voicings) == 0 {
		t.Fatal("no voicings")
	}
	for _, v := range r.Voicings {
		have := map[string]bool{}
		for _, ti := range v.Tones {
			if ti >= 0 {
				have[r.Chord.Tones[ti].Interval] = true
			}
		}
		for _, need := range []string{"R", "3", "♭7", "9"} {
			if !have[need] {
				t.Errorf("%s lacks %s", v.Fingering, need)
			}
		}
	}
}

func TestVoicings_BbSpelledWithFlats(t *testing.T) {
	r := voicings(t, newServer(t), chordParams("Bb", "maj", "", std))
	if r.Chord.Name != "B♭" {
		t.Errorf("name = %q, want B♭", r.Chord.Name)
	}
	for _, tone := range r.Chord.Tones {
		if !slices.Contains([]string{"B♭", "D", "F"}, tone.Note) {
			t.Errorf("tone %q not in {B♭, D, F}", tone.Note)
		}
	}
}

// lowestPitchNote returns the chord tone at the lowest-sounding pitch of a voicing.
func lowestPitchNote(t *testing.T, r voicingsResp, i int, tuningMIDI [6]int) string {
	v := r.Voicings[i]
	best, bestS := 1000, -1
	for s, f := range v.Frets {
		if f >= 0 && tuningMIDI[s]+f < best {
			best, bestS = tuningMIDI[s]+f, s
		}
	}
	return r.Chord.Tones[v.Tones[bestS]].Note
}

func TestVoicings_InversionSetsBass(t *testing.T) {
	srv := newServer(t)
	c7 := voicings(t, srv, chordParams("C", "7", "3rd", std))
	if len(c7.Voicings) == 0 {
		t.Fatal("C7 3rd: no voicings")
	}
	for i := range c7.Voicings {
		if got := lowestPitchNote(t, c7, i, [6]int{40, 45, 50, 55, 59, 64}); got != "E" {
			t.Errorf("C7 3rd %s: bass %s", c7.Voicings[i].Fingering, got)
		}
	}
	am := voicings(t, srv, chordParams("A", "min", "root", "A2,E2,D3,G3,B3,E4"))
	if len(am.Voicings) == 0 {
		t.Fatal("Am root: no voicings")
	}
	for i := range am.Voicings {
		if got := lowestPitchNote(t, am, i, [6]int{45, 40, 50, 55, 59, 64}); got != "A" {
			t.Errorf("Am root %s: bass %s", am.Voicings[i].Fingering, got)
		}
	}
	if am.index("002210") >= 0 {
		t.Error("Am root in crossed tuning includes 002210, whose lowest pitch is E2")
	}
}

func TestVoicings_InversionUnavailableIs400(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/voicings", chordParams("C", "maj", "7th", std))
	expect400(t, code, body, "inversion")
	code, body = get(t, srv, "/api/voicings", chordParams("C", "sus4", "3rd", std))
	expect400(t, code, body, "inversion")
}

func TestVoicings_Empty(t *testing.T) {
	srv := newServer(t)
	p := chordParams("Bb", "m9", "5th", std)
	p.Set("near", "x32010")
	code, body := get(t, srv, "/api/voicings", p)
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(string(body), `"voicings":[]`) || strings.Contains(string(body), "nearestIndex") {
		t.Errorf("body = %s", body)
	}
}

func TestVoicings_NearestFMajorFromOpenC(t *testing.T) {
	p := chordParams("F", "maj", "", std)
	p.Set("near", "x32010")
	r := voicings(t, newServer(t), p)
	if r.NearestIndex == nil {
		t.Fatal("no nearestIndex")
	}
	if got := r.Voicings[*r.NearestIndex].Fingering; got != "x33211" {
		t.Errorf("nearest = %s, want x33211", got)
	}
}

func TestVoicings_NearestHighFrets(t *testing.T) {
	p := chordParams("C", "maj", "", std)
	p.Set("near", "8-10-10-9-8-8")
	r := voicings(t, newServer(t), p)
	if r.NearestIndex == nil || r.Voicings[*r.NearestIndex].Fingering != "8-10-10-9-8-8" {
		t.Errorf("nearest did not return the same high-fret shape")
	}
}

func TestVoicings_NearestZeroIndexIsSent(t *testing.T) {
	r := voicings(t, newServer(t), chordParams("C", "maj", "", std))
	p := chordParams("C", "maj", "", std)
	p.Set("near", r.Voicings[0].Fingering)
	r = voicings(t, newServer(t), p)
	if r.NearestIndex == nil || *r.NearestIndex != 0 {
		t.Errorf("nearestIndex = %v, want 0", r.NearestIndex)
	}
}

func TestVoicings_DropDChangesDMajor(t *testing.T) {
	srv := newServer(t)
	s := voicings(t, srv, chordParams("D", "maj", "", std)).fingerings()
	d := voicings(t, srv, chordParams("D", "maj", "", dropD)).fingerings()
	if slices.Equal(s, d) {
		t.Error("Drop D returns the same voicings")
	}
	if slices.Contains(s, "000232") || !slices.Contains(d, "000232") {
		t.Error("000232 should appear only in Drop D")
	}
}

func TestVoicings_LabelsFollowTuning(t *testing.T) {
	r := voicings(t, newServer(t), chordParams("D", "maj", "", dropD))
	i := r.index("000232")
	if i < 0 {
		t.Fatal("000232 missing")
	}
	tone := r.Chord.Tones[r.Voicings[i].Tones[0]]
	if tone.Note != "D" || tone.Interval != "R" {
		t.Errorf("low string = %+v, want D / R", tone)
	}
}

func TestVoicings_TuningOutOfRangeIs400(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/voicings", chordParams("C", "maj", "", "A#2,A2,D3,G3,B3,E4"))
	expect400(t, code, body, "±5")
	code, body = get(t, srv, "/api/voicings", chordParams("C", "maj", "", "A#1,A2,D3,G3,B3,E4"))
	expect400(t, code, body, "±5")
	if code, _ := get(t, srv, "/api/voicings", chordParams("C", "maj", "", "A2,A2,D3,G3,B3,E4")); code != 200 {
		t.Errorf("+5 = %d, want 200", code)
	}
}

func TestQualities_ListsSpec(t *testing.T) {
	code, body := get(t, newServer(t), "/api/qualities", nil)
	if code != 200 {
		t.Fatal(code)
	}
	var r struct {
		Qualities []struct {
			ID, Label  string
			Inversions []string
		}
	}
	json.Unmarshal(body, &r)
	want := []string{"maj", "min", "dim", "aug", "sus2", "sus4", "6", "m6", "7", "maj7", "m7", "m7b5", "dim7", "7sus4", "add9", "9", "maj9", "m9", "11", "m11", "13", "maj13", "m13"}
	var ids []string
	invs := map[string][]string{}
	for _, q := range r.Qualities {
		ids = append(ids, q.ID)
		invs[q.ID] = q.Inversions
	}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v", ids)
	}
	if !slices.Equal(invs["maj"], []string{"any", "root", "3rd", "5th"}) {
		t.Errorf("maj inversions = %v", invs["maj"])
	}
	if slices.Contains(invs["sus4"], "3rd") || slices.Contains(invs["6"], "7th") || !slices.Contains(invs["11"], "3rd") {
		t.Errorf("sus4 %v, 6 %v, 11 %v", invs["sus4"], invs["6"], invs["11"])
	}
}

func TestDiatonic_DDorian(t *testing.T) {
	code, body := get(t, newServer(t), "/api/diatonic", url.Values{"key": {"D"}, "mode": {"dorian"}})
	if code != 200 {
		t.Fatal(code, string(body))
	}
	var r struct {
		Chords []struct {
			Degree                       int
			Root, Quality, Name, Numeral string
		}
	}
	json.Unmarshal(body, &r)
	var names, numerals []string
	for _, c := range r.Chords {
		names = append(names, c.Name)
		numerals = append(numerals, c.Numeral)
	}
	if got := strings.Join(names, " "); got != "Dm Em F G Am Bdim C" {
		t.Errorf("names = %q", got)
	}
	if got := strings.Join(numerals, " "); got != "i ii ♭III IV v vi° ♭VII" {
		t.Errorf("numerals = %q", got)
	}
}

func TestDiatonic_Errors(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/diatonic", url.Values{"key": {"H"}, "mode": {"dorian"}})
	expect400(t, code, body, "key")
	code, body = get(t, srv, "/api/diatonic", url.Values{"key": {"D"}, "mode": {"foo"}})
	expect400(t, code, body, "mode")
	code, body = get(t, srv, "/api/diatonic", url.Values{"mode": {"dorian"}})
	expect400(t, code, body, "key")
}

type step struct{ Root, Quality, Inversion string }

func progression(t *testing.T, srv *httptest.Server, tuning string, steps []step) []*string {
	t.Helper()
	type js struct {
		Root      string `json:"root"`
		Quality   string `json:"quality"`
		Inversion string `json:"inversion"`
	}
	req := struct {
		Tuning string `json:"tuning"`
		Steps  []js   `json:"steps"`
	}{Tuning: tuning, Steps: []js{}}
	for _, s := range steps {
		req.Steps = append(req.Steps, js(s))
	}
	b, _ := json.Marshal(req)
	code, body := post(t, srv, "/api/progression", string(b))
	if code != 200 {
		t.Fatalf("POST /api/progression = %d: %s", code, body)
	}
	var r struct{ Shapes []*string }
	json.Unmarshal(body, &r)
	return r.Shapes
}

func nearest(t *testing.T, srv *httptest.Server, s step, tuning string, near string) string {
	p := chordParams(s.Root, s.Quality, s.Inversion, tuning)
	if near != "" {
		p.Set("near", near)
	}
	r := voicings(t, srv, p)
	i := 0
	if r.NearestIndex != nil {
		i = *r.NearestIndex
	}
	return r.Voicings[i].Fingering
}

func TestProgression_ChainsNearest(t *testing.T) {
	srv := newServer(t)
	steps := []step{{"C", "maj", "any"}, {"A", "min", "any"}, {"F", "maj", "root"}, {"G", "maj", "any"}}
	shapes := progression(t, srv, std, steps)
	if len(shapes) != len(steps) {
		t.Fatalf("%d shapes for %d steps", len(shapes), len(steps))
	}
	prev := ""
	for i, s := range steps {
		want := nearest(t, srv, s, std, prev)
		if shapes[i] == nil || *shapes[i] != want {
			t.Errorf("step %d = %v, want %s", i+1, shapes[i], want)
		}
		prev = want
	}
}

func TestProgression_NullStepChainContinues(t *testing.T) {
	srv := newServer(t)
	shapes := progression(t, srv, std, []step{{"C", "maj", ""}, {"Bb", "m9", "5th"}, {"G", "maj", ""}})
	if shapes[1] != nil {
		t.Errorf("B♭m9 5th = %v, want null", *shapes[1])
	}
	if shapes[0] == nil || shapes[2] == nil || *shapes[2] != nearest(t, srv, step{"G", "maj", ""}, std, *shapes[0]) {
		t.Error("step 3 is not nearest to step 1")
	}
	shapes = progression(t, srv, std, []step{{"Bb", "m9", "5th"}, {"C", "maj", ""}})
	if shapes[0] != nil || shapes[1] == nil || *shapes[1] != nearest(t, srv, step{"C", "maj", ""}, std, "") {
		t.Error("want [null, voicings(C)[0]]")
	}
}

func TestProgression_TuningRecalc(t *testing.T) {
	srv := newServer(t)
	steps := []step{{"C", "maj", "any"}, {"A", "min", "any"}, {"F", "maj", "root"}, {"G", "maj", "any"}}
	shapes := progression(t, srv, dropD, steps)
	for i, s := range steps {
		list := voicings(t, srv, chordParams(s.Root, s.Quality, s.Inversion, dropD)).fingerings()
		if shapes[i] == nil || !slices.Contains(list, *shapes[i]) {
			t.Errorf("step %d shape %v not in its Drop D list", i+1, shapes[i])
		}
	}
}

func TestProgression_Errors(t *testing.T) {
	srv := newServer(t)
	code, body := post(t, srv, "/api/progression", "{not json")
	expect400(t, code, body, "")
	code, body = post(t, srv, "/api/progression", `{"tuning":"E2","steps":[]}`)
	expect400(t, code, body, "tuning")
	code, body = post(t, srv, "/api/progression", `{"tuning":"`+std+`","steps":[{"root":"C","quality":"maj"},{"root":"C","quality":"foo"}]}`)
	expect400(t, code, body, `step 2: quality: unknown quality "foo"`)
	code, body = post(t, srv, "/api/progression", `{"tuning":"`+std+`","steps":[]}`)
	if code != 200 || strings.TrimSpace(string(body)) != `{"shapes":[]}` {
		t.Errorf("empty steps = %d %s", code, body)
	}
}

type atResp struct {
	voicingsResp
	AtMatched *bool
}

func voicingsAt(t *testing.T, srv *httptest.Server, p url.Values) atResp {
	t.Helper()
	code, body := get(t, srv, "/api/voicings", p)
	if code != 200 {
		t.Fatalf("GET /api/voicings?%s = %d: %s", p.Encode(), code, body)
	}
	var r atResp
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestVoicings_AtPicksShapePlayingSpot(t *testing.T) {
	srv := newServer(t)
	// C from x32010, click the B string (index 4, low→high) at fret 8, a G.
	p := chordParams("C", "maj", "", std)
	p.Set("near", "x32010")
	p.Set("at", "4:8")
	r := voicingsAt(t, srv, p)
	if r.NearestIndex == nil || r.AtMatched == nil || !*r.AtMatched {
		t.Fatalf("nearestIndex %v, atMatched %v", r.NearestIndex, r.AtMatched)
	}
	if got := r.Voicings[*r.NearestIndex].Frets[4]; got != 8 {
		t.Errorf("chosen %s has B string at %d, want 8", r.Voicings[*r.NearestIndex].Fingering, got)
	}

	// Works without near, and for an open string (the low E is in C).
	p.Del("near")
	p.Set("at", "0:0")
	r = voicingsAt(t, srv, p)
	if r.AtMatched == nil || !*r.AtMatched || r.Voicings[*r.NearestIndex].Frets[0] != 0 {
		t.Errorf("open low E: got %s, atMatched %v", r.Voicings[*r.NearestIndex].Fingering, r.AtMatched)
	}
}

func TestVoicings_AtFallsBackWhenNoShapePlaysSpot(t *testing.T) {
	p := chordParams("C", "maj", "", std)
	p.Set("near", "x32010")
	p.Set("at", "4:7") // F♯: not in C
	r := voicingsAt(t, newServer(t), p)
	if r.NearestIndex == nil || r.AtMatched == nil || *r.AtMatched {
		t.Fatalf("nearestIndex %v, atMatched %v; want an index and atMatched false", r.NearestIndex, r.AtMatched)
	}
	if r.Voicings[*r.NearestIndex].Frets[4] == 7 {
		t.Error("fallback shape plays the clicked spot")
	}
}

func TestVoicings_AtOmittedFieldsWithoutAt(t *testing.T) {
	srv := newServer(t)
	p := chordParams("C", "maj", "", std)
	p.Set("near", "x32010")
	_, body := get(t, srv, "/api/voicings", p)
	if strings.Contains(string(body), "atMatched") {
		t.Error("atMatched present without at")
	}
	p = chordParams("Bb", "m9", "5th", std)
	p.Set("at", "4:8")
	_, body = get(t, srv, "/api/voicings", p)
	if strings.Contains(string(body), "atMatched") || strings.Contains(string(body), "nearestIndex") {
		t.Errorf("empty result carries at fields: %s", body)
	}
}

func TestVoicings_AtRejectsBadValues(t *testing.T) {
	srv := newServer(t)
	for _, at := range []string{"6:1", "-1:3", "0:23", "0:-1", "x", "1-2", ":3", "3:", "1:2:3", "+4:8", "-0:8", "04:8", "4:08", "4:+8", "4: 8"} {
		p := chordParams("C", "maj", "", std)
		p.Set("at", at)
		code, body := get(t, srv, "/api/voicings", p)
		expect400(t, code, body, "at:")
	}
}

// opensPast reports whether a shape (low→high frets) has an open string and a fretted note above limit.
func opensPast(frets []int, limit int) bool {
	open, hi := false, 0
	for _, f := range frets {
		open, hi = open || f == 0, max(hi, f)
	}
	return open && hi > limit
}

func TestVoicings_OpenMaxLimitsOpenStringShapes(t *testing.T) {
	srv := newServer(t)
	with := func(openMax string) voicingsResp {
		p := chordParams("C", "maj", "", std)
		if openMax != "" {
			p.Set("openMax", openMax)
		}
		return voicings(t, srv, p)
	}
	def, five, none := with(""), with("5"), with("22")
	if !slices.Equal(def.fingerings(), five.fingerings()) {
		t.Error("omitting openMax is not the same as openMax=5")
	}
	if slices.Contains(def.fingerings(), "875050") || !slices.Contains(none.fingerings(), "875050") {
		t.Error("875050 (open strings up to fret 8) should appear only with openMax=22")
	}
	if len(none.Voicings) <= len(def.Voicings) {
		t.Errorf("openMax=22 gave %d shapes, default %d: the limit should remove some", len(none.Voicings), len(def.Voicings))
	}
	for _, openMax := range []int{0, 3, 5, 12, 22} {
		r := with(strconv.Itoa(openMax))
		if len(r.Voicings) == 0 {
			t.Errorf("openMax=%d: no voicings", openMax)
		}
		for _, v := range r.Voicings {
			if opensPast(v.Frets, openMax) {
				t.Errorf("openMax=%d: %s has an open string and a fret above %d", openMax, v.Fingering, openMax)
			}
		}
	}
	// Shapes with no open string are unaffected, however low the limit.
	for _, f := range []string{"x35553", "8-10-10-9-8-8"} {
		if with("0").index(f) < 0 {
			t.Errorf("openMax=0 dropped %s, which has no open string", f)
		}
	}
	// A limit of 3 keeps x32010 (highest fret 3); a limit of 2 drops it.
	if with("3").index("x32010") < 0 || with("2").index("x32010") >= 0 {
		t.Error("x32010 should be kept at openMax=3 and dropped at openMax=2")
	}
}

func TestVoicings_OpenMaxRejectsBadValues(t *testing.T) {
	srv := newServer(t)
	for _, v := range []string{"-1", "23", "5x", "05", "+5", "abc", " 5", "5.0", "99999999999999999999"} {
		p := chordParams("C", "maj", "", std)
		p.Set("openMax", v)
		code, body := get(t, srv, "/api/voicings", p)
		expect400(t, code, body, "openMax:")
	}
}

func TestVoicings_FretboardMapsEveryChordTone(t *testing.T) {
	srv := newServer(t)
	r := voicings(t, srv, chordParams("C", "maj", "", std))
	if len(r.Fretboard) != 6 {
		t.Fatalf("%d strings, want 6", len(r.Fretboard))
	}
	for s, row := range r.Fretboard {
		if len(row) != 23 {
			t.Errorf("string %d has %d frets, want 23 (0–22)", s, len(row))
		}
	}
	// Low→high: [0] is the low E. C major tones: 0 = C, 1 = E, 2 = G.
	for _, c := range []struct {
		name      string
		str, fret int
		want      int
	}{
		{"low E open = E", 0, 0, 1},
		{"low E fret 8 = C", 0, 8, 0},
		{"A fret 3 = C", 1, 3, 0},
		{"A fret 2 = B", 1, 2, -1},
		{"high e fret 3 = G", 5, 3, 2},
		{"high e open = E", 5, 0, 1},
	} {
		if got := r.Fretboard[c.str][c.fret]; got != c.want {
			t.Errorf("%s: tone %d, want %d", c.name, got, c.want)
		}
	}

	// Every played note of every shape agrees with the map, so the map is in the same string order as frets/tones.
	for _, chord := range [][2]string{{"C", "13"}, {"Bb", "m7b5"}, {"F#", "maj"}} {
		for _, tuning := range []string{std, dropD} {
			r := voicings(t, srv, chordParams(chord[0], chord[1], "", tuning))
			for _, v := range r.Voicings {
				for s, f := range v.Frets {
					if f >= 0 && r.Fretboard[s][f] != v.Tones[s] {
						t.Fatalf("%s %s in %s: %s string %d fret %d: map %d, tones %d", chord[0], chord[1], tuning, v.Fingering, s, f, r.Fretboard[s][f], v.Tones[s])
					}
				}
			}
		}
	}

	// It follows the tuning, and is present even when no shape is playable.
	d := voicings(t, srv, chordParams("D", "maj", "", dropD))
	if d.Fretboard[0][0] != 0 {
		t.Errorf("Drop D low string open = tone %d in D, want 0 (root)", d.Fretboard[0][0])
	}
	empty := voicings(t, srv, chordParams("Bb", "m9", "5th", std))
	if len(empty.Voicings) != 0 || len(empty.Fretboard) != 6 {
		t.Errorf("empty result: %d voicings, %d fretboard rows", len(empty.Voicings), len(empty.Fretboard))
	}
}

func TestProgression_OpenMax(t *testing.T) {
	srv := newServer(t)
	steps := []step{{"C", "maj", "any"}, {"A", "min", "any"}, {"F", "maj", "root"}, {"G", "maj", "any"}}
	for _, openMax := range []int{0, 3, 22} {
		shapes := progressionOpen(t, srv, std, openMax, steps)
		prev := ""
		for i, s := range steps {
			p := chordParams(s.Root, s.Quality, s.Inversion, std)
			p.Set("openMax", strconv.Itoa(openMax))
			if prev != "" {
				p.Set("near", prev)
			}
			r := voicings(t, srv, p)
			want := r.Voicings[0].Fingering
			if r.NearestIndex != nil {
				want = r.Voicings[*r.NearestIndex].Fingering
			}
			if shapes[i] == nil || *shapes[i] != want {
				t.Errorf("openMax=%d step %d = %v, want %s", openMax, i+1, shapes[i], want)
			}
			if shapes[i] != nil {
				prev = *shapes[i]
				if opensPast(r.Voicings[r.index(prev)].Frets, openMax) {
					t.Errorf("openMax=%d step %d shape %s breaks the limit", openMax, i+1, prev)
				}
			}
		}
	}
	// Omitted means the default, the same as sending it.
	if a, b := progression(t, srv, std, steps), progressionOpen(t, srv, std, 5, steps); !reflect.DeepEqual(a, b) {
		t.Error("omitting openMax differs from openMax=5")
	}
	for _, bad := range []string{"-1", "23"} {
		code, body := post(t, srv, "/api/progression", `{"tuning":"`+std+`","openMax":`+bad+`,"steps":[]}`)
		expect400(t, code, body, "openMax")
	}
}

func progressionOpen(t *testing.T, srv *httptest.Server, tuning string, openMax int, steps []step) []*string {
	t.Helper()
	type js struct {
		Root      string `json:"root"`
		Quality   string `json:"quality"`
		Inversion string `json:"inversion"`
	}
	req := struct {
		Tuning  string `json:"tuning"`
		OpenMax int    `json:"openMax"`
		Steps   []js   `json:"steps"`
	}{Tuning: tuning, OpenMax: openMax, Steps: []js{}}
	for _, s := range steps {
		req.Steps = append(req.Steps, js(s))
	}
	b, _ := json.Marshal(req)
	code, body := post(t, srv, "/api/progression", string(b))
	if code != 200 {
		t.Fatalf("POST /api/progression = %d: %s", code, body)
	}
	var r struct{ Shapes []*string }
	json.Unmarshal(body, &r)
	return r.Shapes
}

func shapeStrings(shapes []*string) []string {
	out := make([]string, len(shapes))
	for i, s := range shapes {
		out[i] = "null"
		if s != nil {
			out[i] = *s
		}
	}
	return out
}

// The default open-string limit (5) and the boundaries around it are pinned with chains whose
// later steps change when the limit does. They were found by searching the generator: if it
// changes and a chain stops separating the limits, find new chains instead of loosening this.
func TestProgression_OpenMaxDefaultAndBoundaries(t *testing.T) {
	srv := newServer(t)
	chains := []struct {
		name       string
		steps      []step
		mustDiffer []int // limits whose result must differ from limit 5
	}{
		{"C → Cmaj7", []step{{"C", "maj", "any"}, {"C", "maj7", "any"}}, []int{12, 22}},
		{"C → F9 (7th in the bass)", []step{{"C", "maj", "any"}, {"F", "9", "7th"}}, []int{6, 12, 22}},
		{"C → A9", []step{{"C", "maj", "any"}, {"A", "9", "any"}}, []int{4, 12, 22}},
	}
	for _, c := range chains {
		five := progressionOpen(t, srv, std, 5, c.steps)
		if got := progression(t, srv, std, c.steps); !reflect.DeepEqual(got, five) {
			t.Errorf("%s: omitting openMax gives %v, openMax=5 gives %v", c.name, shapeStrings(got), shapeStrings(five))
		}
		for _, limit := range c.mustDiffer {
			if got := progressionOpen(t, srv, std, limit, c.steps); reflect.DeepEqual(got, five) {
				t.Errorf("%s: openMax=%d gives the same shapes as 5 (%v), so this chain no longer separates the limits", c.name, limit, shapeStrings(got))
			}
		}
	}
}

// Shape lists, not just first shapes, pin the default: for these chords the lists at 4, 5 and 6 all differ.
func TestVoicings_OpenMaxDefaultIsFive(t *testing.T) {
	srv := newServer(t)
	for _, c := range [][2]string{{"E", "7"}, {"C", "9"}} {
		list := func(openMax string) []string {
			p := chordParams(c[0], c[1], "", std)
			if openMax != "" {
				p.Set("openMax", openMax)
			}
			return voicings(t, srv, p).fingerings()
		}
		def, four, five, six := list(""), list("4"), list("5"), list("6")
		if !slices.Equal(def, five) {
			t.Errorf("%s%s: omitting openMax gives %d shapes, openMax=5 gives %d", c[0], c[1], len(def), len(five))
		}
		if slices.Equal(five, four) || slices.Equal(five, six) {
			t.Errorf("%s%s: the lists at 4, 5 and 6 should all differ (%d, %d, %d shapes)", c[0], c[1], len(four), len(five), len(six))
		}
	}
}

// Both ends of the string and fret ranges are accepted, and `at` counts strings from the low E,
// so the string it names is the string that carries the fret.
func TestVoicings_AtAcceptsEdgesAndCountsFromLowString(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		at, openMax string
		matched     bool
		why         string
	}{
		{"5:0", "", true, "high e open = E, the 3rd of C"},
		{"5:3", "", true, "high e fret 3 = G"},
		{"0:8", "", true, "low E fret 8 = C"},
		{"1:22", "22", true, "A string fret 22 = G, the highest fret"},
		{"0:22", "", false, "low E fret 22 = D, not in C"},
		{"5:22", "", false, "high e fret 22 = D, not in C"},
	}
	for _, c := range cases {
		p := chordParams("C", "maj", "", std)
		p.Set("near", "x32010")
		p.Set("at", c.at)
		if c.openMax != "" {
			p.Set("openMax", c.openMax)
		}
		r := voicingsAt(t, srv, p)
		if r.NearestIndex == nil || r.AtMatched == nil {
			t.Errorf("at=%s (%s): nearestIndex %v, atMatched %v; want both present", c.at, c.why, r.NearestIndex, r.AtMatched)
			continue
		}
		if *r.AtMatched != c.matched {
			t.Errorf("at=%s (%s): atMatched = %v, want %v", c.at, c.why, *r.AtMatched, c.matched)
		}
		if *r.AtMatched {
			var s, f int
			fmt.Sscanf(c.at, "%d:%d", &s, &f)
			if got := r.Voicings[*r.NearestIndex].Frets[s]; got != f {
				t.Errorf("at=%s (%s): chosen shape %s has fret %d on string %d", c.at, c.why, r.Voicings[*r.NearestIndex].Fingering, got, s)
			}
		}
	}
}

// `near` decides among the shapes that play the clicked spot: from x32010 the B-string-fret-8 shape
// with the least finger movement is x7558x, while with no current shape the one whose average fret
// is closest to 8 wins, and that is a different shape.
func TestVoicings_AtUsesNear(t *testing.T) {
	srv := newServer(t)
	pick := func(near string) string {
		p := chordParams("C", "maj", "", std)
		p.Set("at", "4:8")
		if near != "" {
			p.Set("near", near)
		}
		r := voicingsAt(t, srv, p)
		if r.NearestIndex == nil || r.AtMatched == nil || !*r.AtMatched {
			t.Fatalf("near=%q: nearestIndex %v, atMatched %v", near, r.NearestIndex, r.AtMatched)
		}
		return r.Voicings[*r.NearestIndex].Fingering
	}
	withNear, without := pick("x32010"), pick("")
	if withNear != "x7558x" {
		t.Errorf("from x32010, B-8 picked %s, want x7558x", withNear)
	}
	if without == withNear {
		t.Errorf("near made no difference: both picked %s", withNear)
	}
}

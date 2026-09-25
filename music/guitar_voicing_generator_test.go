package music

import (
	"slices"
	"testing"
)

func generate(t testing.TB, tuning string, root string, q ChordQuality, inv Inversion) []GuitarChordVoicing {
	return NewGuitarVoicingGenerator(mustTuning(t, tuning), MaxFret).GenerateVoicings(chordOf(t, root, q), inv)
}

func generateLimited(t testing.TB, tuning string, root string, q ChordQuality, inv Inversion, maxOpen int) []GuitarChordVoicing {
	g := NewGuitarVoicingGenerator(mustTuning(t, tuning), MaxFret)
	g.MaxOpenFret = maxOpen
	return g.GenerateVoicings(chordOf(t, root, q), inv)
}

func fingerings(vs []GuitarChordVoicing) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, FormatFingering(v.Fingering))
	}
	return out
}

// bruteForce enumerates {x, 0..22}⁶ with fret-span pruning and filters with the
// public checks, independently of the generator's inline mask test.
func bruteForce(tuning Tuning, chord Chord, inv Inversion, maxOpen int) []GuitarChordVoicing {
	var out []GuitarChordVoicing
	var f [6]int
	var walk func(s, lo, hi int)
	walk = func(s, lo, hi int) {
		if s == 6 {
			v := GuitarChordVoicing{Chord: chord, Fingering: f}
			if !v.MatchesChord(tuning) {
				return
			}
			v.Barre = detectBarreForVoicing(f)
			if !v.IsPlayable() {
				return
			}
			bass := chord.toneAt()[mod12(v.BassPitch(tuning))]
			if !inv.allowsBass(chord.Quality.Tones()[bass].Degree) {
				return
			}
			open, hi := false, 0
			for _, fret := range f {
				open, hi = open || fret == 0, max(hi, fret)
			}
			if open && hi > maxOpen {
				return
			}
			out = append(out, v)
			return
		}
		for fret := -1; fret <= MaxFret; fret++ {
			f[s] = fret
			if fret <= 0 {
				walk(s+1, lo, hi)
			} else if nlo, nhi := min(lo, fret), max(hi, fret); nhi-nlo <= 3 {
				walk(s+1, nlo, nhi)
			}
		}
	}
	walk(0, MaxFret+1, 0)
	slices.SortFunc(out, compareVoicings)
	return out
}

func TestGenerate_MatchesBruteForce(t *testing.T) {
	cases := []struct {
		tuning  string
		root    string
		q       ChordQuality
		inv     Inversion
		maxOpen int
	}{
		{stdTuning, "C", Major, AnyInversion, MaxFret},
		{stdTuning, "A", Major, AnyInversion, MaxFret},
		{stdTuning, "C", Dominant9, AnyInversion, MaxFret},
		{openGTuning, "G", Major, AnyInversion, MaxFret},
		{stdTuning, "F", Major, RootPosition, MaxFret},
		{"A2,E2,D3,G3,B3,E4", "E", Minor7, ThirdInBass, MaxFret},
		// Open strings limited: the default, a tight limit, and the boundary at 0.
		{stdTuning, "C", Major, AnyInversion, DefaultMaxOpenFret},
		{stdTuning, "C", Dominant9, AnyInversion, 3},
		{stdTuning, "C", Major, AnyInversion, 0},
		{openGTuning, "G", Major, AnyInversion, 0},
	}
	for _, c := range cases {
		got := fingerings(generateLimited(t, c.tuning, c.root, c.q, c.inv, c.maxOpen))
		want := fingerings(bruteForce(mustTuning(t, c.tuning), chordOf(t, c.root, c.q), c.inv, c.maxOpen))
		if !slices.Equal(got, want) {
			t.Errorf("%s%s %s in %s (open ≤ %d): generator %d voicings, brute force %d", c.root, c.q, c.inv, c.tuning, c.maxOpen, len(got), len(want))
		}
		if len(got) == 0 {
			t.Errorf("%s%s %s in %s (open ≤ %d): no voicings", c.root, c.q, c.inv, c.tuning, c.maxOpen)
		}
	}
}

func TestGenerate_IncludesKnownShapes(t *testing.T) {
	cases := []struct {
		root   string
		q      ChordQuality
		shapes []string
	}{
		{"C", Major, []string{"x32010", "x35553"}},
		{"A", Major, []string{"x02220", "577655"}},
		{"E", Minor, []string{"022000", "322003"}},
		{"F", Major, []string{"133211", "x33211"}},
		{"G", Major, []string{"320003", "355433"}},
		{"D", Major, []string{"xx0232"}},
		{"Bb", Major, []string{"x13331", "688766"}},
		{"C", Major, []string{"8-10-10-9-8-8"}},
	}
	for _, c := range cases {
		got := fingerings(generate(t, stdTuning, c.root, c.q, AnyInversion))
		for _, s := range c.shapes {
			if !slices.Contains(got, s) {
				t.Errorf("%s%s: missing %s", c.root, c.q, s)
			}
		}
	}
}

func TestGenerate_DeterministicOrder(t *testing.T) {
	first := fingerings(generate(t, stdTuning, "C", Dominant13, AnyInversion))
	for range 5 {
		if again := fingerings(generate(t, stdTuning, "C", Dominant13, AnyInversion)); !slices.Equal(first, again) {
			t.Fatal("voicing order changed between runs")
		}
	}
	vs := generate(t, stdTuning, "C", Major, AnyInversion)
	for i := 1; i < len(vs); i++ {
		if compareVoicings(vs[i-1], vs[i]) >= 0 {
			t.Errorf("not strictly sorted at %d: %s, %s", i, FormatFingering(vs[i-1].Fingering), FormatFingering(vs[i].Fingering))
		}
	}
}

func TestGenerate_UsesActiveTuning(t *testing.T) {
	if got := fingerings(generate(t, dropDTuning, "D", Major, AnyInversion)); !slices.Contains(got, "000232") {
		t.Error("Drop D: D major missing 000232")
	}
	if got := fingerings(generate(t, stdTuning, "D", Major, AnyInversion)); slices.Contains(got, "000232") {
		t.Error("standard: D major contains 000232")
	}
	if got := fingerings(generate(t, openGTuning, "G", Major, AnyInversion)); !slices.Contains(got, "000000") {
		t.Error("Open G: G major missing 000000")
	}
}

func TestGenerate_MaxOpenFret(t *testing.T) {
	all := generate(t, stdTuning, "C", Major, AnyInversion) // no limit
	limited := generateLimited(t, stdTuning, "C", Major, AnyInversion, DefaultMaxOpenFret)
	if len(limited) == 0 || len(limited) >= len(all) {
		t.Fatalf("limited %d, unlimited %d: the limit should remove some shapes and keep others", len(limited), len(all))
	}
	got := fingerings(limited)
	for _, f := range []string{"x32010", "x35553", "8-10-10-9-8-8"} { // open shape ≤ 5, and two shapes with no open strings
		if !slices.Contains(got, f) {
			t.Errorf("limit %d dropped %s", DefaultMaxOpenFret, f)
		}
	}
	if slices.Contains(got, "875050") { // open strings with frets up to 8
		t.Error("875050 kept: open strings with a fretted note past the limit")
	}
	// Exactly the unlimited shapes, minus those with an open string and a fret above the limit.
	var want []string
	for _, v := range all {
		if !opensPastFret(v.Fingering, DefaultMaxOpenFret) {
			want = append(want, FormatFingering(v.Fingering))
		}
	}
	if !slices.Equal(got, want) {
		t.Error("limited list is not the unlimited list minus the open-string shapes past the limit")
	}
	// The boundary: fret == limit is allowed, limit+1 is not.
	if opensPastFret(mustFingering(t, "x32010"), 3) || !opensPastFret(mustFingering(t, "x32010"), 2) {
		t.Error("boundary wrong: x32010 has an open string and a highest fret of 3")
	}
	if opensPastFret(mustFingering(t, "x35553"), 0) || opensPastFret(mustFingering(t, "000000"), 0) {
		t.Error("shapes without an open string, or with only open strings, are never past the limit")
	}
	if len(generateLimited(t, stdTuning, "C", Major, AnyInversion, MaxFret)) != len(all) {
		t.Error("a limit of MaxFret changed the result")
	}
}

func TestGenerate_InversionSetsBass(t *testing.T) {
	tuning := mustTuning(t, "A2,E2,D3,G3,B3,E4")
	vs := generate(t, "A2,E2,D3,G3,B3,E4", "A", Minor, RootPosition)
	if len(vs) == 0 {
		t.Fatal("no voicings")
	}
	for _, v := range vs {
		if mod12(v.BassPitch(tuning)) != 9 {
			t.Errorf("%s: bass is not A", FormatFingering(v.Fingering))
		}
	}
}

func BenchmarkGenerate(b *testing.B) {
	for _, c := range []struct {
		name string
		q    ChordQuality
	}{{"C", Major}, {"Cmaj13", Major13}} {
		b.Run(c.name, func(b *testing.B) {
			g := NewGuitarVoicingGenerator(StandardTuning(), MaxFret)
			chord := Chord{Root: Note{Name: C}, Quality: c.q}
			for b.Loop() {
				g.GenerateVoicings(chord, AnyInversion)
			}
		})
	}
}

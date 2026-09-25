package guitar

import (
	"github.com/nstalter/fretboard-visualizer/music"
	"testing"
)

func TestGuitarChordVoicing_IsPlayable(t *testing.T) {
	tests := []struct {
		name     string
		voicing  GuitarChordVoicing
		expected bool
	}{
		{
			name: "Valid open C major chord",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 1, 0, 2, 3, -1}, // C major open
			},
			expected: true,
		},
		{
			name: "Valid barre chord (112331)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{1, 3, 3, 2, 1, 1}, // 112331 low→high
				Barre:     &Barre{Fret: 1, StartString: 0, EndString: 5},
			},
			expected: true,
		},
		{
			name: "Too few strings (only 2 played)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{-1, -1, -1, -1, 3, 5}, // Only 2 strings
			},
			expected: false,
		},
		{
			name: "Too large a fret span (1 - 5)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 1, 2, 3, 4, 5}, // All 6 strings played
			},
			expected: false,
		},
		{
			name: "Valid single fretted note",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 0, 0, 6, -1, -1}, // Only fret 6 is fretted
			},
			expected: true,
		},
		{
			name: "Bad mute pattern (muted string between played strings)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, -1, 0, 3, 3, -1}, // B string muted between played strings
			},
			expected: false,
		},
		{
			name: "Valid adjacent fingering with barre",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{3, 3, 3, 3, 3, 3}, // All strings at fret 3
				Barre:     &Barre{Fret: 3, StartString: 0, EndString: 5},
			},
			expected: true,
		},
		{
			name: "Valid 4-string voicing",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{2, 3, 0, 0, -1, -1}, // 4 strings played, mute lowest
			},
			expected: true,
		},
		{
			name: "Valid 5-string voicing",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 2, 2, 1, 0, -1}, // 5 strings played, mute low E
			},
			expected: true,
		},
		{
			name: "Valid 3-string voicing (minimum)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 2, 3, -1, -1, -1}, // 3 strings played, mute lowest 3
			},
			expected: true,
		},
		{
			name: "Invalid: muting high strings",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{-1, -1, 0, 2, 3, 1}, // Muting high strings is hard
			},
			expected: false,
		},
		{
			name: "Valid open strings only",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 0, 0, 0, 0, 0}, // All open strings
			},
			expected: true,
		},
		{
			name: "Invalid barre: non-contiguous played strings",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{3, 5, 5, -1, 3, 3}, // Played: [0,1,2,4,5] - not contiguous
				Barre:     &Barre{Fret: 3, StartString: 0, EndString: 5},
			},
			expected: false,
		},
	}

	for _, c := range []struct {
		fingering string
		expected  bool
	}{
		{"879987", false}, // barre + 4 fingers
		{"x33211", true},
		{"x02220", true},
	} {
		f := mustFingering(t, c.fingering)
		tests = append(tests, struct {
			name     string
			voicing  GuitarChordVoicing
			expected bool
		}{c.fingering, GuitarChordVoicing{Fingering: f, Barre: detectBarreForVoicing(f)}, c.expected})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.voicing.IsPlayable()
			if result != tt.expected {
				t.Errorf("IsPlayable() = %v, expected %v for voicing: %v",
					result, tt.expected, tt.voicing.Fingering)
			}
		})
	}
}

func TestMatchesChord(t *testing.T) {
	cases := []struct {
		root      string
		q         music.ChordQuality
		fingering string
		tuning    string
		want      bool
	}{
		{"C", music.Major, "x32010", stdTuning, true},
		{"C", music.Major, "335xxx", stdTuning, false}, // no 3rd
		{"C", music.Major, "x32000", stdTuning, false}, // B is not in the chord
		{"C", music.Dominant7, "x32310", stdTuning, true},
		{"C", music.Dominant9, "x30310", stdTuning, true},
		{"C", music.Dominant9, "355553", stdTuning, false}, // no ♭7
		{"C", music.Dominant11, "x33311", stdTuning, true},
		{"C", music.Minor11, "x33311", stdTuning, false}, // no ♭3
		{"C", music.HalfDiminished7, "x3434x", stdTuning, true},
		{"D", music.Major, "000232", dropDTuning, true},
		{"D", music.Major, "000232", stdTuning, false},
	}
	for _, c := range cases {
		v := GuitarChordVoicing{Chord: chordOf(t, c.root, c.q), Fingering: mustFingering(t, c.fingering)}
		if got := v.MatchesChord(mustTuning(t, c.tuning)); got != c.want {
			t.Errorf("%s %s in %s: MatchesChord = %v, want %v", v.Chord.Name(), c.fingering, c.tuning, got, c.want)
		}
	}
}

func TestBassPitch_CrossedStrings(t *testing.T) {
	tuning := mustTuning(t, "A2,E2,D3,G3,B3,E4")
	v := GuitarChordVoicing{Chord: chordOf(t, "A", music.Minor), Fingering: mustFingering(t, "002210")}
	bass := v.BassPitch(tuning)
	if bass != 40 { // E2, on the second-lowest string
		t.Fatalf("BassPitch = %d, want 40 (E2)", bass)
	}
	if tone := v.Chord.Quality.Tones()[v.Chord.ToneAt()[music.PitchClass(bass)]]; tone.Degree != 5 {
		t.Errorf("bass degree = %d, want 5", tone.Degree)
	}
}

func TestCalculateFingerMovement(t *testing.T) {
	c := GuitarChordVoicing{Fingering: mustFingering(t, "x32010")}
	for s, want := range map[string]float64{"x33211": 4, "133211": 6, "xx3211": 6, "533xxx": 9, "x32010": 0} {
		o := GuitarChordVoicing{Fingering: mustFingering(t, s)}
		if got := c.CalculateFingerMovement(o); got != want {
			t.Errorf("x32010 → %s = %v, want %v", s, got, want)
		}
		if back := o.CalculateFingerMovement(c); back != want {
			t.Errorf("%s → x32010 = %v, want %v (symmetric)", s, back, want)
		}
	}
}

func TestNearestIndex_TieBreaks(t *testing.T) {
	list := func(fs ...string) []GuitarChordVoicing {
		var vs []GuitarChordVoicing
		for _, f := range fs {
			vs = append(vs, GuitarChordVoicing{Fingering: mustFingering(t, f)})
		}
		return vs
	}
	cases := []struct {
		name    string
		current string
		vs      []GuitarChordVoicing
		want    int
	}{
		{"least movement", "x32010", list("133211", "x33211", "533xxx"), 1},
		{"closer average fret", "xxx555", list("xxx575", "xxx465"), 1},
		{"exact-fraction tie falls to lower fret", "222222", list("322222", "122222"), 1},
		{"earlier index", "xxx555", list("xxx465", "xxx564"), 0},
		{"earlier index (swapped)", "xxx555", list("xxx564", "xxx465"), 0},
		{"empty", "x32010", nil, -1},
	}
	for _, c := range cases {
		if got := NearestIndex(c.vs, mustFingering(t, c.current)); got != c.want {
			t.Errorf("%s: NearestIndex = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestNearestIndexAt(t *testing.T) {
	vs := generate(t, stdTuning, "C", music.Major, music.AnyInversion)
	cur := mustFingering(t, "x32010")
	c := GuitarChordVoicing{Fingering: cur}
	avgDist := func(v GuitarChordVoicing, fret int) float64 {
		s, n := frettedSum(v.Fingering)
		return max(float64(s)/float64(n)-float64(fret), float64(fret)-float64(s)/float64(n))
	}

	// B string (internal index 1), fret 8 = G: a chord tone some shapes play.
	i, matched := NearestIndexAt(vs, &cur, 1, 8)
	if !matched || vs[i].Fingering[1] != 8 {
		t.Fatalf("B-8: got %s matched=%v, want a shape fretting B-8", FormatFingering(vs[i].Fingering), matched)
	}
	for _, v := range vs {
		if v.Fingering[1] == 8 && c.CalculateFingerMovement(v) < c.CalculateFingerMovement(vs[i]) {
			t.Errorf("B-8: %s moves less than chosen %s", FormatFingering(v.Fingering), FormatFingering(vs[i].Fingering))
		}
	}

	// Without a current shape, the hit closest in position to the clicked fret wins.
	j, matched := NearestIndexAt(vs, nil, 1, 8)
	if !matched || vs[j].Fingering[1] != 8 {
		t.Fatalf("B-8 no current: got %s matched=%v", FormatFingering(vs[j].Fingering), matched)
	}
	for _, v := range vs {
		if v.Fingering[1] == 8 && avgDist(v, 8) < avgDist(vs[j], 8) {
			t.Errorf("B-8 no current: %s is closer to fret 8 than %s", FormatFingering(v.Fingering), FormatFingering(vs[j].Fingering))
		}
	}

	// B string, fret 7 = F♯: not in C. The fallback still picks a shape whose range covers
	// fret 7, and among those the one that moves least from the current shape.
	k, matched := NearestIndexAt(vs, &cur, 1, 7)
	if matched || fretDistance(vs[k].Fingering, 7) != 0 {
		t.Fatalf("B-7 (F♯): got %s matched=%v, want an unmatched shape covering fret 7", FormatFingering(vs[k].Fingering), matched)
	}
	for _, v := range vs {
		if fretDistance(v.Fingering, 7) == 0 && c.CalculateFingerMovement(v) < c.CalculateFingerMovement(vs[k]) {
			t.Errorf("B-7 fallback: %s covers fret 7 and moves less than %s", FormatFingering(v.Fingering), FormatFingering(vs[k].Fingering))
		}
	}

	// Open strings can be picked too: the open G string is in C.
	o, matched := NearestIndexAt(vs, &cur, 2, 0)
	if !matched || vs[o].Fingering[2] != 0 {
		t.Errorf("open G: got %s matched=%v", FormatFingering(vs[o].Fingering), matched)
	}

	if i, matched := NearestIndexAt(nil, &cur, 1, 8); i != -1 || matched {
		t.Errorf("empty = %d, %v; want -1, false", i, matched)
	}
}

// Clicking a non-chord tone inside the current shape's range keeps that shape, rather than
// swapping it for a thinner fragment that happens to sit nearer on average.
func TestNearestIndexAt_ClickInsideCurrentShapeKeepsIt(t *testing.T) {
	vs := generate(t, stdTuning, "C", music.Major, music.AnyInversion)
	cur := mustFingering(t, "x32010")
	// D string (internal 3) fret 1 = D♯, not in C. x32010 spans frets 0–3.
	i, matched := NearestIndexAt(vs, &cur, 3, 1)
	if matched || FormatFingering(vs[i].Fingering) != "x32010" {
		t.Errorf("got %s matched=%v, want x32010 kept, unmatched", FormatFingering(vs[i].Fingering), matched)
	}
}

func TestNearestIndexAt_FallbackTieBreaks(t *testing.T) {
	list := func(fs ...string) []GuitarChordVoicing {
		var vs []GuitarChordVoicing
		for _, f := range fs {
			vs = append(vs, GuitarChordVoicing{Fingering: mustFingering(t, f)})
		}
		return vs
	}
	// Every candidate mutes the D string (internal 3), so no click on it can match.
	// From xxx555 (G, B, e all at fret 5), a candidate's movement is the sum of |fret − 5|.
	cur := mustFingering(t, "xxx555")
	cases := []struct {
		name   string
		vs     []GuitarChordVoicing
		fret   int
		useCur bool
		want   int
	}{
		// xxx999 covers fret 9 (distance 0); xxx556 (range 5–6) is 3 away, though it moves far less.
		{"range beats movement", list("xxx556", "xxx999"), 9, true, 1},
		// All three cover fret 7. Movement: xxx699 = 1+4+4 = 9, xxx777 = 6, xxx577 = 0+2+2 = 4.
		// xxx777 has the exact average, but least movement is ranked first.
		{"range tie goes to least movement", list("xxx699", "xxx777", "xxx577"), 7, true, 2},
		// Both cover fret 7 and move 3. xxx754 averages 16/3 and xxx657 averages 6, nearer 7.
		{"movement tie goes to closer average", list("xxx754", "xxx657"), 7, true, 1},
		// Identical range, movement 3 and average 6: the earlier index wins, in either order.
		{"full tie goes to earlier index", list("xxx567", "xxx657"), 7, true, 0},
		{"full tie goes to earlier index (swapped)", list("xxx657", "xxx567"), 7, true, 0},
		// Fret 10 is outside every range. xxx678 and xxx578 are 2 away, xxx577 is 3 away;
		// of the first two, xxx578 moves 5 against 6.
		{"nearest range, then movement", list("xxx678", "xxx578", "xxx577"), 10, true, 1},
		// With no current shape, average position breaks the range tie: 7 beats 6.
		{"no current: closer average", list("xxx567", "xxx678"), 7, false, 1},
		{"no current: closer average (swapped)", list("xxx678", "xxx567"), 7, false, 0},
	}
	for _, c := range cases {
		var current *[6]int
		if c.useCur {
			current = &cur
		}
		if i, matched := NearestIndexAt(c.vs, current, 3, c.fret); i != c.want || matched {
			t.Errorf("%s: got %d, %v; want %d, false", c.name, i, matched, c.want)
		}
	}
}

func TestFretDistance(t *testing.T) {
	for _, c := range []struct {
		fingering string
		fret      int
		want      int
	}{
		{"x32010", 1, 0}, // inside 0–3
		{"x32010", 0, 0}, // open strings count as fret 0
		{"x32010", 5, 2}, // 2 above the top
		{"xxx555", 0, 5}, // no open string: 5 below the bottom
		{"xxx555", 5, 0}, //
		{"xxx555", 9, 4}, //
		{"x35553", 2, 1}, // range 3–5
	} {
		if got := fretDistance(mustFingering(t, c.fingering), c.fret); got != c.want {
			t.Errorf("fretDistance(%s, %d) = %d, want %d", c.fingering, c.fret, got, c.want)
		}
	}
}

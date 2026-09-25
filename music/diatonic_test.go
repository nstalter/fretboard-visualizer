package music

import (
	"strings"
	"testing"
)

func numerals(cs [7]DiatonicChord) string {
	var ns []string
	for _, c := range cs {
		ns = append(ns, c.Numeral)
	}
	return strings.Join(ns, " ")
}

func names(cs [7]DiatonicChord) string {
	var ns []string
	for _, c := range cs {
		ns = append(ns, Chord{Root: c.Root, Quality: c.Quality}.Name())
	}
	return strings.Join(ns, " ")
}

func TestDiatonic_AllModes(t *testing.T) {
	cases := []struct {
		key      string
		mode     Mode
		numerals string
		names    string
	}{
		{"C", Ionian, "I ii iii IV V vi vii°", "C Dm Em F G Am Bdim"},
		{"D", Dorian, "i ii ♭III IV v vi° ♭VII", "Dm Em F G Am Bdim C"},
		{"E", Phrygian, "i ♭II ♭III iv v° ♭VI ♭vii", "Em F G Am Bdim C Dm"},
		{"F", Lydian, "I II iii ♯iv° V vi vii", "F G Am Bdim C Dm Em"},
		{"G", Mixolydian, "I ii iii° IV v vi ♭VII", "G Am Bdim C Dm Em F"},
		{"A", Aeolian, "i ii° ♭III iv v ♭VI ♭VII", "Am Bdim C Dm Em F G"},
		{"B", Locrian, "i° ♭II ♭iii iv ♭V ♭VI ♭vii", "Bdim C Dm Em F G Am"},
	}
	for _, c := range cases {
		cs := DiatonicChords(mustNote(t, c.key), c.mode)
		if got := numerals(cs); got != c.numerals {
			t.Errorf("%s %s numerals = %q, want %q", c.key, c.mode, got, c.numerals)
		}
		if got := names(cs); got != c.names {
			t.Errorf("%s %s names = %q, want %q", c.key, c.mode, got, c.names)
		}
		for i, dc := range cs {
			if dc.Degree != i+1 {
				t.Errorf("degree %d = %d", i+1, dc.Degree)
			}
		}
	}
}

func TestDiatonic_Spelling(t *testing.T) {
	if got := DiatonicChords(mustNote(t, "Gb"), Ionian)[3].Root.String(); got != "C♭" {
		t.Errorf("G♭ Ionian IV root = %s, want C♭", got)
	}
	vii := DiatonicChords(mustNote(t, "G#"), Ionian)[6]
	if got := (Chord{Root: vii.Root, Quality: vii.Quality}).Name(); got != "F♯♯dim" {
		t.Errorf("G♯ Ionian vii° = %s, want F♯♯dim", got)
	}
}

func TestDiatonic_RoundTrip(t *testing.T) {
	keys := []string{"C", "C♯", "D♭", "D", "D♯", "E♭", "E", "F", "F♯", "G♭", "G", "G♯", "A♭", "A", "A♯", "B♭", "B"}
	for _, k := range keys {
		for m := Ionian; m <= Locrian; m++ {
			for _, dc := range DiatonicChords(mustNote(t, k), m) {
				n, err := ParseNote(dc.Root.String())
				if err != nil || n != dc.Root {
					t.Errorf("%s %s: root %s does not round-trip: %v", k, m, dc.Root, err)
					continue
				}
			}
		}
	}
}

func TestParseMode(t *testing.T) {
	for m := Ionian; m <= Locrian; m++ {
		if got, err := ParseMode(m.String()); err != nil || got != m {
			t.Errorf("ParseMode(%q) = %v, %v", m, got, err)
		}
	}
	if _, err := ParseMode("foo"); err == nil {
		t.Error("ParseMode(foo) accepted")
	}
}

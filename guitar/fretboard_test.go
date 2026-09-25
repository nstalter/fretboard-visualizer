package guitar

import (
	"testing"

	"github.com/nstalter/fretboard-visualizer/music"
)

func TestFretboardTones(t *testing.T) {
	// C major tones: 0 = C (root), 1 = E (3rd), 2 = G (5th). Rows are high→low: [0] is the high e.
	m := FretboardTones(chordOf(t, "C", music.Major), StandardTuning())
	for _, c := range []struct {
		name      string
		str, fret int
		want      int
	}{
		{"low E open = E", 5, 0, 1},
		{"low E fret 8 = C", 5, 8, 0},
		{"A fret 3 = C", 4, 3, 0},
		{"A fret 2 = B, not in C", 4, 2, -1},
		{"D open = D, not in C", 3, 0, -1},
		{"G open = G", 2, 0, 2},
		{"B fret 1 = C", 1, 1, 0},
		{"high e fret 3 = G", 0, 3, 2},
		{"high e fret 22 = D, not in C", 0, 22, -1},
		{"A fret 22 = G", 4, 22, 2},
	} {
		if got := m[c.str][c.fret]; got != c.want {
			t.Errorf("%s: tone %d, want %d", c.name, got, c.want)
		}
	}

	// It agrees with ToneIndices on every played string of a real shape.
	v := GuitarChordVoicing{Chord: chordOf(t, "C", music.Major), Fingering: mustFingering(t, "x32010")}
	for s, want := range v.ToneIndices(StandardTuning()) {
		if v.Fingering[s] >= 0 && m[s][v.Fingering[s]] != want {
			t.Errorf("string %d: FretboardTones %d, ToneIndices %d", s, m[s][v.Fingering[s]], want)
		}
	}

	// It follows the tuning: in Drop D the low string is D, the root of a D chord.
	d := FretboardTones(chordOf(t, "D", music.Major), mustTuning(t, dropDTuning))
	if d[5][0] != 0 {
		t.Errorf("Drop D low string open = tone %d in a D chord, want 0 (root)", d[5][0])
	}
}

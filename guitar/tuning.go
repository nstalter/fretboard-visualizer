package guitar

import (
	"fmt"
	"github.com/nstalter/fretboard-visualizer/music"
	"strings"
)

// MaxTuningOffset is how far, in semitones, each string may be tuned from standard.
const MaxTuningOffset = 5

type Tuning struct {
	Strings [6]music.Note // high→low: Strings[0] is the high e
}

func StandardTuning() Tuning {
	return Tuning{
		Strings: [6]music.Note{
			{Name: music.E, Accidental: music.Natural, Octave: 4}, // high e
			{Name: music.B, Accidental: music.Natural, Octave: 3},
			{Name: music.G, Accidental: music.Natural, Octave: 3},
			{Name: music.D, Accidental: music.Natural, Octave: 3},
			{Name: music.A, Accidental: music.Natural, Octave: 2},
			{Name: music.E, Accidental: music.Natural, Octave: 2}, // low E
		},
	}
}

// ParseTuning parses six comma-separated pitches, low to high ("E2,A2,D3,G3,B3,E4").
// Each string must be within MaxTuningOffset semitones of standard.
func ParseTuning(s string) (Tuning, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 6 {
		return Tuning{}, fmt.Errorf("want 6 pitches, got %d", len(parts))
	}
	std := StandardTuning()
	var t Tuning
	for i, p := range parts {
		n, err := music.ParsePitch(strings.TrimSpace(p))
		if err != nil {
			return Tuning{}, err
		}
		s := 5 - i
		if d := n.MIDI() - std.Strings[s].MIDI(); d > MaxTuningOffset || d < -MaxTuningOffset {
			return Tuning{}, fmt.Errorf("string %d (%s) is out of range: each string must be within ±%d semitones of standard", i+1, p, MaxTuningOffset)
		}
		t.Strings[s] = n
	}
	return t, nil
}

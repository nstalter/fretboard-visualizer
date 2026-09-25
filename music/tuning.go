package music

import (
	"fmt"
	"strings"
)

// MaxTuningOffset is how far, in semitones, each string may be tuned from standard.
const MaxTuningOffset = 5

type Tuning struct {
	Strings [6]Note // high→low: Strings[0] is the high e
}

func StandardTuning() Tuning {
	return Tuning{
		Strings: [6]Note{
			{Name: E, Accidental: Natural, Octave: 4}, // high e
			{Name: B, Accidental: Natural, Octave: 3},
			{Name: G, Accidental: Natural, Octave: 3},
			{Name: D, Accidental: Natural, Octave: 3},
			{Name: A, Accidental: Natural, Octave: 2},
			{Name: E, Accidental: Natural, Octave: 2}, // low E
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
		n, err := ParsePitch(strings.TrimSpace(p))
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

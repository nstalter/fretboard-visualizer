package music

import (
	"fmt"
	"strings"
)

type Mode int

const (
	Ionian Mode = iota
	Dorian
	Phrygian
	Lydian
	Mixolydian
	Aeolian
	Locrian
)

var modeNames = [...]string{"ionian", "dorian", "phrygian", "lydian", "mixolydian", "aeolian", "locrian"}

// ParseMode parses "ionian" … "locrian".
func ParseMode(s string) (Mode, error) {
	for i, name := range modeNames {
		if name == s {
			return Mode(i), nil
		}
	}
	return 0, fmt.Errorf("unknown mode %q", s)
}

func (m Mode) String() string { return modeNames[m] }

type DiatonicChord struct {
	Degree  int // 1..7
	Root    Note
	Quality ChordQuality
	Numeral string // measured against the major scale: D Dorian gives i ii ♭III IV v vi° ♭VII
}

var (
	majorSteps     = [7]int{2, 2, 1, 2, 2, 2, 1}
	ionianOffsets  = modeOffsets(Ionian)
	romanNumerals  = [7]string{"I", "II", "III", "IV", "V", "VI", "VII"}
	triadQualities = map[[2]int]ChordQuality{{4, 7}: Major, {3, 7}: Minor, {3, 6}: Diminished}
)

// modeOffsets returns the semitone offset of each scale degree above the tonic.
func modeOffsets(m Mode) [7]int {
	var off [7]int
	for i := 1; i < 7; i++ {
		off[i] = off[i-1] + majorSteps[(int(m)+i-1)%7]
	}
	return off
}

// DiatonicChords returns the seven diatonic triads of a key and mode.
func DiatonicChords(key Note, mode Mode) [7]DiatonicChord {
	off := modeOffsets(mode)
	tonic := Chord{Root: key}
	var chords [7]DiatonicChord
	for i := range chords {
		third := PitchClass(off[(i+2)%7] - off[i])
		fifth := PitchClass(off[(i+4)%7] - off[i])
		q := triadQualities[[2]int{third, fifth}]

		numeral := romanNumerals[i]
		if q != Major {
			numeral = strings.ToLower(numeral)
		}
		if d := off[i] - ionianOffsets[i]; d < 0 {
			numeral = strings.Repeat("♭", -d) + numeral
		} else if d > 0 {
			numeral = strings.Repeat("♯", d) + numeral
		}
		if q == Diminished {
			numeral += "°"
		}

		chords[i] = DiatonicChord{
			Degree:  i + 1,
			Root:    tonic.Spell(Tone{Degree: i + 1, Semitones: off[i]}),
			Quality: q,
			Numeral: numeral,
		}
	}
	return chords
}

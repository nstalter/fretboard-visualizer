package music

import (
	"fmt"
	"strings"
)

// Tone is one chord tone, relative to the root.
type Tone struct {
	Degree    int // 1,2,3,4,5,6,7,9,11,13 (letter distance; drives spelling)
	Semitones int // above root
	Optional  bool
}

// majorSemitones is the semitone count of each degree in the major scale.
var majorSemitones = map[int]int{1: 0, 2: 2, 3: 4, 4: 5, 5: 7, 6: 9, 7: 11, 9: 14, 11: 17, 13: 21}

// Label returns the interval label: "R", "♭3", "3", "♯5", "♭♭7", "9", …
func (t Tone) Label() string {
	if t.Degree == 1 {
		return "R"
	}
	d := t.Semitones - majorSemitones[t.Degree]
	prefix := ""
	if d < 0 {
		prefix = strings.Repeat("♭", -d)
	} else if d > 0 {
		prefix = strings.Repeat("♯", d)
	}
	return fmt.Sprintf("%s%d", prefix, t.Degree)
}

type ChordQuality int

const (
	Major ChordQuality = iota
	Minor
	Diminished
	Augmented
	Sus2
	Sus4
	Sixth
	Minor6
	Dominant7
	Major7
	Minor7
	HalfDiminished7
	Diminished7
	Dominant7Sus4
	Add9
	Dominant9
	Major9
	Minor9
	Dominant11
	Minor11
	Dominant13
	Major13
	Minor13
)

type qualityInfo struct {
	id, label, suffix string
	tones             []Tone
}

var (
	tR    = Tone{Degree: 1, Semitones: 0}
	t2    = Tone{Degree: 2, Semitones: 2}
	tm3   = Tone{Degree: 3, Semitones: 3}
	t3    = Tone{Degree: 3, Semitones: 4}
	t4    = Tone{Degree: 4, Semitones: 5}
	tb5   = Tone{Degree: 5, Semitones: 6}
	t5    = Tone{Degree: 5, Semitones: 7}
	ts5   = Tone{Degree: 5, Semitones: 8}
	t6    = Tone{Degree: 6, Semitones: 9}
	tbb7  = Tone{Degree: 7, Semitones: 9}
	tb7   = Tone{Degree: 7, Semitones: 10}
	t7    = Tone{Degree: 7, Semitones: 11}
	t9    = Tone{Degree: 9, Semitones: 14}
	t11   = Tone{Degree: 11, Semitones: 17}
	t13   = Tone{Degree: 13, Semitones: 21}
	opt3  = Tone{Degree: 3, Semitones: 4, Optional: true}
	opt5  = Tone{Degree: 5, Semitones: 7, Optional: true}
	opt9  = Tone{Degree: 9, Semitones: 14, Optional: true}
	opt11 = Tone{Degree: 11, Semitones: 17, Optional: true}
)

var qualityTable = [...]qualityInfo{
	Major:           {"maj", "maj", "", []Tone{tR, t3, t5}},
	Minor:           {"min", "min", "m", []Tone{tR, tm3, t5}},
	Diminished:      {"dim", "dim", "dim", []Tone{tR, tm3, tb5}},
	Augmented:       {"aug", "aug", "aug", []Tone{tR, t3, ts5}},
	Sus2:            {"sus2", "sus2", "sus2", []Tone{tR, t2, t5}},
	Sus4:            {"sus4", "sus4", "sus4", []Tone{tR, t4, t5}},
	Sixth:           {"6", "6", "6", []Tone{tR, t3, t5, t6}},
	Minor6:          {"m6", "m6", "m6", []Tone{tR, tm3, t5, t6}},
	Dominant7:       {"7", "7", "7", []Tone{tR, t3, opt5, tb7}},
	Major7:          {"maj7", "maj7", "maj7", []Tone{tR, t3, opt5, t7}},
	Minor7:          {"m7", "m7", "m7", []Tone{tR, tm3, opt5, tb7}},
	HalfDiminished7: {"m7b5", "m7♭5", "m7♭5", []Tone{tR, tm3, tb5, tb7}},
	Diminished7:     {"dim7", "dim7", "dim7", []Tone{tR, tm3, tb5, tbb7}},
	Dominant7Sus4:   {"7sus4", "7sus4", "7sus4", []Tone{tR, t4, opt5, tb7}},
	Add9:            {"add9", "add9", "add9", []Tone{tR, t3, t5, t9}},
	Dominant9:       {"9", "9", "9", []Tone{tR, t3, opt5, tb7, t9}},
	Major9:          {"maj9", "maj9", "maj9", []Tone{tR, t3, opt5, t7, t9}},
	Minor9:          {"m9", "m9", "m9", []Tone{tR, tm3, opt5, tb7, t9}},
	Dominant11:      {"11", "11", "11", []Tone{tR, opt3, opt5, tb7, opt9, t11}},
	Minor11:         {"m11", "m11", "m11", []Tone{tR, tm3, opt5, tb7, opt9, t11}},
	Dominant13:      {"13", "13", "13", []Tone{tR, t3, opt5, tb7, opt9, opt11, t13}},
	Major13:         {"maj13", "maj13", "maj13", []Tone{tR, t3, opt5, t7, opt9, opt11, t13}},
	Minor13:         {"m13", "m13", "m13", []Tone{tR, tm3, opt5, tb7, opt9, opt11, t13}},
}

// AllChordQualities returns every quality in spec order.
func AllChordQualities() []ChordQuality {
	qs := make([]ChordQuality, len(qualityTable))
	for i := range qs {
		qs[i] = ChordQuality(i)
	}
	return qs
}

// ParseChordQuality parses a quality id exactly ("M7" ≠ "m7").
func ParseChordQuality(id string) (ChordQuality, error) {
	for i, q := range qualityTable {
		if q.id == id {
			return ChordQuality(i), nil
		}
	}
	return 0, fmt.Errorf("unknown quality %q", id)
}

func (q ChordQuality) String() string { return qualityTable[q].id }
func (q ChordQuality) Label() string  { return qualityTable[q].label }
func (q ChordQuality) Tones() []Tone  { return qualityTable[q].tones }

func (q ChordQuality) hasDegree(degree int) bool {
	for _, t := range q.Tones() {
		if t.Degree == degree {
			return true
		}
	}
	return false
}

// Inversions returns the inversions offered for this quality: Any and Root always,
// then 3rd, 5th and 7th when the quality has a tone of that degree.
func (q ChordQuality) Inversions() []Inversion {
	invs := []Inversion{AnyInversion, RootPosition}
	for _, inv := range []Inversion{ThirdInBass, FifthInBass, SeventhInBass} {
		if q.hasDegree(inv.degree()) {
			invs = append(invs, inv)
		}
	}
	return invs
}

type Chord struct {
	Root    Note
	Quality ChordQuality
}

// Name returns the chord symbol: "B♭m7♭5", "Bdim", "Em".
func (c Chord) Name() string {
	return c.Root.String() + qualityTable[c.Quality].suffix
}

// Spell returns the note for a tone of this chord, spelled from its degree.
func (c Chord) Spell(t Tone) Note {
	letter := (int(c.Root.Name) + t.Degree - 1) % 7
	acc := int(c.Root.Accidental) + PitchClass(t.Semitones) - PitchClass(naturalSemitones[letter]-naturalSemitones[c.Root.Name])
	return Note{Name: NoteName(letter), Accidental: Accidental(acc)}
}

// ToneAt maps a pitch class to an index into Tones(), or -1.
func (c Chord) ToneAt() [12]int {
	var at [12]int
	for i := range at {
		at[i] = -1
	}
	root := c.Root.SemitoneValue()
	for i, t := range c.Quality.Tones() {
		pc := PitchClass(root + t.Semitones)
		if at[pc] < 0 {
			at[pc] = i
		}
	}
	return at
}

// Masks returns pitch-class bitmasks of all chord tones and of the required ones.
func (c Chord) Masks() (all, required uint16) {
	root := c.Root.SemitoneValue()
	for _, t := range c.Quality.Tones() {
		bit := uint16(1) << PitchClass(root+t.Semitones)
		all |= bit
		if !t.Optional {
			required |= bit
		}
	}
	return all, required
}

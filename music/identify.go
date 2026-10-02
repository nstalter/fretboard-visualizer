package music

import (
	"math/bits"
	"slices"
)

// Match is a chord that spells a set of pitch classes, with the inversion its bass note makes.
type Match struct {
	Chord     Chord
	Inversion Inversion
}

// Roots are spelled the way players name them: flat keys for major-type chords (D♭, A♭) and sharp
// ones for minor-type chords (C♯m, G♯m), except E♭ and B♭ and the F♯ of both.
var (
	majorRoots = [12]Note{{Name: C}, {Name: D, Accidental: Flat}, {Name: D}, {Name: E, Accidental: Flat}, {Name: E}, {Name: F},
		{Name: F, Accidental: Sharp}, {Name: G}, {Name: A, Accidental: Flat}, {Name: A}, {Name: B, Accidental: Flat}, {Name: B}}
	minorRoots = [12]Note{{Name: C}, {Name: C, Accidental: Sharp}, {Name: D}, {Name: E, Accidental: Flat}, {Name: E}, {Name: F},
		{Name: F, Accidental: Sharp}, {Name: G}, {Name: G, Accidental: Sharp}, {Name: A}, {Name: B, Accidental: Flat}, {Name: B}}
)

// enharmonic is the [sharp, flat] spelling of each pitch class that has two.
var enharmonic = map[int][2]Note{
	1:  {{Name: C, Accidental: Sharp}, {Name: D, Accidental: Flat}},
	3:  {{Name: D, Accidental: Sharp}, {Name: E, Accidental: Flat}},
	6:  {{Name: F, Accidental: Sharp}, {Name: G, Accidental: Flat}},
	8:  {{Name: G, Accidental: Sharp}, {Name: A, Accidental: Flat}},
	10: {{Name: A, Accidental: Sharp}, {Name: B, Accidental: Flat}},
}

// rootSpellings are the ways to spell pitch class pc as the root of a chord of quality q: the usual
// spelling, then the other name of a pitch class that has two. A spelling that needs more double
// sharps or flats than the usual one is left out (E♭ major, not D♯ major with its F𝄪).
func rootSpellings(pc int, q ChordQuality) []Note {
	usual := rootNote(pc, q)
	pair, ok := enharmonic[pc]
	if !ok {
		return []Note{usual}
	}
	spellings := []Note{usual, pair[0]}
	if usual == pair[0] {
		spellings[1] = pair[1]
	}
	limit := max(1, widestAccidental(Chord{Root: usual, Quality: q}))
	var out []Note
	for _, root := range spellings {
		if widestAccidental(Chord{Root: root, Quality: q}) <= limit {
			out = append(out, root)
		}
	}
	return out
}

// widestAccidental is the largest number of sharps or flats any tone of the chord is spelled with.
func widestAccidental(c Chord) int {
	widest := 0
	for _, t := range c.Quality.Tones() {
		a := int(c.Spell(t).Accidental)
		widest = max(widest, a, -a)
	}
	return widest
}

// rootNote spells pitch class pc as the root of a chord of quality q.
func rootNote(pc int, q ChordQuality) Note {
	for _, t := range q.Tones() {
		if t.Degree == 3 && t.Semitones == 3 {
			return minorRoots[pc]
		}
	}
	return majorRoots[pc]
}

// Identify returns every chord whose tones account for the pitch classes in present (a bitmask,
// bit n = pitch class n) and that includes all of the chord's required tones, best first. bass is the
// pitch class of the lowest note. A chord with the bass as its root comes first, then the chord with
// the fewest tones left out, then the simplest quality. A chord whose root has two names (G♯ and A♭)
// appears under both, the usual spelling first.
func Identify(present uint16, bass int) []Match {
	type ranked struct {
		m                 Match
		bassNotRoot, left int
		quality           ChordQuality
	}
	var found []ranked
	for pc := range 12 {
		for _, q := range AllChordQualities() {
			for _, root := range rootSpellings(pc, q) {
				c := Chord{Root: root, Quality: q}
				all, required := c.Masks()
				if present&^all != 0 || present&required != required {
					break // the other spelling has the same tones
				}
				r := ranked{m: Match{c, bassInversion(c, bass)}, left: bits.OnesCount16(all &^ present), quality: q}
				if pc != PitchClass(bass) {
					r.bassNotRoot = 1
				}
				found = append(found, r)
			}
		}
	}
	slices.SortStableFunc(found, func(a, b ranked) int {
		if a.bassNotRoot != b.bassNotRoot {
			return a.bassNotRoot - b.bassNotRoot
		}
		if a.left != b.left {
			return a.left - b.left
		}
		return int(a.quality) - int(b.quality)
	})
	out := make([]Match, len(found))
	for i, r := range found {
		out[i] = r.m
	}
	return out
}

// bassInversion is the inversion that puts the bass pitch class's chord degree in the bass, or
// AnyInversion when the quality offers none for that degree (a 9th or 13th in the bass).
func bassInversion(c Chord, bass int) Inversion {
	i := c.ToneAt()[PitchClass(bass)]
	if i < 0 {
		return AnyInversion
	}
	degree := c.Quality.Tones()[i].Degree
	for _, inv := range c.Quality.Inversions() {
		if inv != AnyInversion && inv.AllowsBass(degree) {
			return inv
		}
	}
	return AnyInversion
}

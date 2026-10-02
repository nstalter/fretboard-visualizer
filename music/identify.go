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
// the fewest tones left out, then the simplest quality.
func Identify(present uint16, bass int) []Match {
	type ranked struct {
		m                 Match
		bassNotRoot, left int
		quality           ChordQuality
	}
	var found []ranked
	for pc := range 12 {
		for _, q := range AllChordQualities() {
			c := Chord{Root: rootNote(pc, q), Quality: q}
			all, required := c.Masks()
			if present&^all != 0 || present&required != required {
				continue
			}
			r := ranked{m: Match{c, bassInversion(c, bass)}, left: bits.OnesCount16(all &^ present), quality: q}
			if pc != PitchClass(bass) {
				r.bassNotRoot = 1
			}
			found = append(found, r)
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

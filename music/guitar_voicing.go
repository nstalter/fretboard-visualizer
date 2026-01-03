package music

import (
	"math"
)

type GuitarChordVoicing struct {
	Chord     Chord  // The specific chord this voicing represents
	Fingering [6]int // -1 for muted, 0 for open, >0 for fret number
	Barre     *Barre // Absolute barre information
}

// Fingering follows standard guitar string order:
// [high E, B, G, D, A, low E]

type Barre struct {
	Fret        int
	StartString int
	EndString   int
}

type PlayedString struct {
	IsMuted     bool
	Fret        int // -1 for muted, 0 for open, >0 for fret number
	StringIndex int // 0-5 (high E to low E)
	Tuning      Tuning
}

func (p PlayedString) GetNote() Note {
	if p.IsMuted {
		return Note{} // Return empty note for muted strings
	}
	return CalculateNote(p.Tuning.Strings[p.StringIndex], p.Fret)
}

func (p PlayedString) String() string {
	if p.IsMuted {
		return "X"
	}

	return p.GetNote().String()
}

func (c *GuitarChordVoicing) CalculateNotes(tuning Tuning) [6]PlayedString {
	notes := [6]PlayedString{}
	for i, fret := range c.Fingering {
		if fret == -1 {
			notes[i] = PlayedString{IsMuted: true, Fret: -1, StringIndex: i, Tuning: tuning}
		} else {
			notes[i] = PlayedString{IsMuted: false, Fret: fret, StringIndex: i, Tuning: tuning}
		}
	}
	return notes
}

// GetAverageFretPosition calculates the average fret position of played notes
func (v *GuitarChordVoicing) GetAverageFretPosition() float64 {
	sum := 0
	count := 0
	for _, fret := range v.Fingering {
		if fret > 0 {
			sum += fret
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

// GetMinFret returns the minimum fret used in the voicing (excluding open strings)
func (v *GuitarChordVoicing) GetMinFret() int {
	minFret := 999
	for _, fret := range v.Fingering {
		if fret > 0 && fret < minFret {
			minFret = fret
		}
	}
	if minFret == 999 {
		return 0
	}
	return minFret
}

func (v *GuitarChordVoicing) MatchesChord(tuning Tuning) bool {
	playedStrings := v.CalculateNotes(tuning)
	playedNotes := make(map[int]bool)

	for _, playedString := range playedStrings {
		if playedString.Fret >= 0 {
			note := playedString.GetNote()
			if !v.Chord.IsNoteInChord(note) {
				return false // Non-chord tone found
			}
			playedNotes[note.SemitoneValue()] = true
		}
	}

	// Always require the root
	rootSemitone := v.Chord.Root.SemitoneValue()
	if !playedNotes[rootSemitone] {
		return false
	}

	// For extended chords, require the highest extension
	chordIntervals := v.Chord.GetChordIntervals()
	if len(chordIntervals) > 3 { // Has extensions beyond triad
		// Require the highest extension (7th for 7th chords, 9th for 9th chords, etc.)
		highestInterval := chordIntervals[len(chordIntervals)-1]
		extensionSemitone := (rootSemitone + highestInterval) % 12
		if !playedNotes[extensionSemitone] {
			return false // Missing required extension
		}
	}

	return true
}

func (v *GuitarChordVoicing) IsPlayable() bool {
	frets := v.Fingering

	playedCount := 0

	for _, fret := range frets {
		if fret >= 0 { // Played
			playedCount++
		}
	}

	if playedCount < 3 {
		return false
	}

	if v.Barre == nil {
		return nonBarreChordPlayable(v.Fingering)
	} else {
		return barreChordPlayable(v.Fingering, v.Barre.Fret)
	}
}

func nonBarreChordPlayable(fingering [6]int) bool {
	return fretSpanPlayable(fingering, 3) && hasOnlyLowStringsMuted(fingering) && frettedStringCountPlayable(fingering, -1)
}

func barreChordPlayable(fingering [6]int, barreFret int) bool {
	return fretSpanPlayable(fingering, 3) && allPlayedStringsContiguous(fingering) && frettedStringsGreaterThanFret(fingering, barreFret) && stringSpanPlayable(fingering, barreFret, 4)
}

func frettedStringsGreaterThanFret(fingering [6]int, fret int) bool {
	for _, f := range fingering {
		if f > 0 && f < fret {
			return false
		}
	}
	return true
}

func stringSpanPlayable(fingering [6]int, barreFret int, maxSpan int) bool {
	return stringSpan(fingering, barreFret) <= maxSpan
}

func stringSpan(fingering [6]int, barreFret int) int {
	playedStrings := []int{}
	for i, fret := range fingering {
		if fret >= 0 && fret != barreFret { // Only count strings not at barre fret
			playedStrings = append(playedStrings, i)
		}
	}
	if len(playedStrings) == 0 {
		return 0
	}
	return playedStrings[len(playedStrings)-1] - playedStrings[0]
}

func fretSpanPlayable(fingering [6]int, maxSpan int) bool {
	return fretSpan(fingering) <= maxSpan
}

func fretSpan(fingering [6]int) int {
	minFretted := 999
	maxFretted := 0
	for _, fret := range fingering {
		if fret > 0 { // Only fretted strings count
			if fret < minFretted {
				minFretted = fret
			}
			if fret > maxFretted {
				maxFretted = fret
			}
		}
	}
	return maxFretted - minFretted
}

func hasOnlyLowStringsMuted(fingering [6]int) bool {
	lowestPlayedIndex := lowestPlayedStringIndex(fingering)
	for stringIdx, fret := range fingering {
		if fret == -1 { // Muted
			if stringIdx < lowestPlayedIndex {
				// This muted string is above the lowest played string
				return false
			}
		}
	}
	return true
}

func lowestPlayedStringIndex(fingering [6]int) int {
	lowestPlayedIndex := -1
	for stringIdx, fret := range fingering {
		if fret >= 0 { // Played or open
			if lowestPlayedIndex == -1 || stringIdx > lowestPlayedIndex {
				lowestPlayedIndex = stringIdx
			}
		}
	}
	return lowestPlayedIndex
}

func frettedStringCountPlayable(fingering [6]int, barreFret int) bool {
	if barreFret < 0 { // Non-barre
		frettedCount := 0
		for _, fret := range fingering {
			if fret > 0 { // Fretted (not open)
				frettedCount++
			}
		}
		return frettedCount <= 4 // Non-barre limited to 4 fingers
	} else { // Barre
		// Count fretted strings that are NOT at the barre fret
		nonBarreFrettedCount := 0
		for _, fret := range fingering {
			if fret > 0 && fret != barreFret { // Fretted and not at barre
				nonBarreFrettedCount++
			}
		}
		return nonBarreFrettedCount <= 3 // Barre + 3 additional fingers
	}
}

func allPlayedStringsContiguous(fingering [6]int) bool {
	playedStrings := []int{}
	for i, fret := range fingering {
		if fret >= 0 {
			playedStrings = append(playedStrings, i)
		}
	}

	if len(playedStrings) == 0 {
		return true
	}

	// Check if ALL played strings form one contiguous block
	for i := 1; i < len(playedStrings); i++ {
		if playedStrings[i] != playedStrings[i-1]+1 {
			return false // Gap found!
		}
	}

	return true
}

// CalculateFingerMovement calculates the total finger movement distance between this voicing and another
func (v *GuitarChordVoicing) CalculateFingerMovement(other GuitarChordVoicing) float64 {
	totalMovement := 0.0

	for i := 0; i < 6; i++ {
		fromFret := v.Fingering[i]
		toFret := other.Fingering[i]

		// Skip muted strings and calculate movement
		if fromFret >= 0 && toFret >= 0 {
			totalMovement += math.Abs(float64(toFret - fromFret))
		}
	}

	return totalMovement
}

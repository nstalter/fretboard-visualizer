package music

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

// MatchesChord reports whether every played pitch class is a chord tone and every
// required tone is present.
func (v *GuitarChordVoicing) MatchesChord(tuning Tuning) bool {
	all, required := v.Chord.masks()
	present := v.pitchClassMask(tuning)
	return present&^all == 0 && present&required == required
}

func (v *GuitarChordVoicing) pitchClassMask(tuning Tuning) uint16 {
	var present uint16
	for s, fret := range v.Fingering {
		if fret >= 0 {
			present |= 1 << mod12(tuning.Strings[s].SemitoneValue()+fret)
		}
	}
	return present
}

// BassPitch returns the MIDI pitch of the lowest-sounding played string, or -1 if none.
func (v *GuitarChordVoicing) BassPitch(tuning Tuning) int {
	bass := -1
	for s, fret := range v.Fingering {
		if fret >= 0 {
			if p := tuning.Strings[s].MIDI() + fret; bass < 0 || p < bass {
				bass = p
			}
		}
	}
	return bass
}

// ToneIndices returns, per string (high→low), the index into Chord.Quality.Tones()
// of the note played, or -1 for a muted string or a non-chord tone.
func (v *GuitarChordVoicing) ToneIndices(tuning Tuning) [6]int {
	at := v.Chord.toneAt()
	var idx [6]int
	for s, fret := range v.Fingering {
		idx[s] = -1
		if fret >= 0 {
			idx[s] = at[mod12(tuning.Strings[s].SemitoneValue()+fret)]
		}
	}
	return idx
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
	return fretSpanPlayable(fingering, 3) && allPlayedStringsContiguous(fingering) && frettedStringsGreaterThanFret(fingering, barreFret) && stringSpanPlayable(fingering, barreFret, 4) && frettedStringCountPlayable(fingering, barreFret)
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

package music

import (
	"fmt"
	"sort"
)

type GuitarVoicingGenerator struct {
	tuning  Tuning
	maxFret int
}

func NewGuitarVoicingGenerator(tuning Tuning, maxFret int) *GuitarVoicingGenerator {
	return &GuitarVoicingGenerator{
		tuning:  tuning,
		maxFret: maxFret,
	}
}

func (g *GuitarVoicingGenerator) GenerateVoicings(chord Chord) []GuitarChordVoicing {
	return g.GenerateVoicingsInRange(chord, 0, g.maxFret)
}

func (g *GuitarVoicingGenerator) GenerateVoicingsInRange(chord Chord, minFret, maxFret int) []GuitarChordVoicing {
	// 1. Scan fretboard - create PlayedString for every position
	allPositions := g.scanFretboard(minFret, maxFret)

	// 2. Filter for chord tones using GetNote() + IsNoteInChord()
	chordPositions := filterChordTones(allPositions, chord)

	// 3. Group positions by fret (fret-bucket approach)
	fretBuckets := groupPositionsByFret(chordPositions)

	// 4. Build voicings from each fret bucket
	var voicings []GuitarChordVoicing
	seen := make(map[string]bool) // For deduplication

	for fret, positions := range fretBuckets {
		bucketVoicings := g.buildVoicingsFromFretBucket(fret, positions, fretBuckets, chord)
		for _, voicing := range bucketVoicings {
			key := voicingToKey(voicing)
			if !seen[key] {
				seen[key] = true
				voicings = append(voicings, voicing)
			}
		}
	}

	// Sort by minimum fret position
	sort.Slice(voicings, func(i, j int) bool {
		return voicings[i].GetMinFret() < voicings[j].GetMinFret()
	})

	return voicings
}

func hasRootNote(voicing GuitarChordVoicing, chord Chord) bool {
	playedStrings := voicing.CalculateNotes(StandardTuning())
	rootPresent := false
	for _, playedString := range playedStrings {
		if playedString.Fret >= 0 { // Only check played strings
			note := playedString.GetNote()
			if note.SemitoneValue() == chord.Root.SemitoneValue() {
				rootPresent = true
				break
			}
		}
	}
	if !rootPresent {
		fmt.Println("WARNING: Voicing does not contain root note for chord", chord.Root.String(), chord.Quality.String())
		fmt.Println("Fingering:", voicing.Fingering)
		for j, playedString := range playedStrings {
			if playedString.Fret >= 0 {
				note := playedString.GetNote()
				fmt.Println("  String", j, "fret", playedString.Fret, "=", note.String())
			}
		}
		return false
	}
	return true
}

func (g *GuitarVoicingGenerator) scanFretboard(minFret, maxFret int) []PlayedString {
	var positions []PlayedString

	// Check each string
	for stringIndex := 0; stringIndex < 6; stringIndex++ {
		// Check open string first
		if minFret == 0 {
			positions = append(positions, PlayedString{
				IsMuted:     false,
				Fret:        0,
				StringIndex: stringIndex,
				Tuning:      g.tuning,
			})
		}

		// Check fretted positions
		for fret := max(1, minFret); fret <= maxFret; fret++ {
			positions = append(positions, PlayedString{
				IsMuted:     false,
				Fret:        fret,
				StringIndex: stringIndex,
				Tuning:      g.tuning,
			})
		}
	}

	return positions
}

func filterChordTones(positions []PlayedString, chord Chord) []PlayedString {
	var chordPositions []PlayedString

	for _, pos := range positions {
		note := pos.GetNote()
		if chord.IsNoteInChord(note) {
			chordPositions = append(chordPositions, pos)
		}
	}

	return chordPositions
}

func playedStringsToVoicing(positions []PlayedString, chord Chord) GuitarChordVoicing {
	fingering := [6]int{-1, -1, -1, -1, -1, -1} // Start with all muted

	for _, pos := range positions {
		fingering[pos.StringIndex] = pos.Fret
	}

	return GuitarChordVoicing{
		Chord:     chord,
		Fingering: fingering,
	}
}

func (g *GuitarVoicingGenerator) buildVoicingsFromFretBucket(startFret int, positions []PlayedString, fretBuckets map[int][]PlayedString, chord Chord) []GuitarChordVoicing {
	var voicings []GuitarChordVoicing

	// For each anchor position in this bucket
	for _, anchor := range positions {
		nearbyPositions := findNearbyChordTones(anchor, fretBuckets, chord, 3)

		// Generate combinations of 3-6 notes from nearby positions
		for size := 3; size <= 6 && size <= len(nearbyPositions); size++ {
			combinations := generateCombinations(nearbyPositions, size)
			for _, combo := range combinations {
				voicing := playedStringsToVoicing(combo, chord)

				voicing.Barre = detectBarreForVoicing(voicing.Fingering)

				if voicing.IsPlayable() && voicing.MatchesChord(g.tuning) {
					voicings = append(voicings, voicing)
				}
			}
		}
	}

	return voicings
}

func voicingToKey(voicing GuitarChordVoicing) string {
	key := ""
	for _, fret := range voicing.Fingering {
		if fret == -1 {
			key += "X"
		} else {
			key += string(rune('0' + fret))
		}
		key += ","
	}
	return key
}

func groupPositionsByFret(positions []PlayedString) map[int][]PlayedString {
	buckets := make(map[int][]PlayedString)

	for _, pos := range positions {
		buckets[pos.Fret] = append(buckets[pos.Fret], pos)
	}

	return buckets
}

func findNearbyChordTones(anchor PlayedString, allBuckets map[int][]PlayedString, chord Chord, fretRange int) []PlayedString {
	var nearby []PlayedString

	// Include the anchor position
	nearby = append(nearby, anchor)

	// Check frets within range
	for offset := -fretRange; offset <= fretRange; offset++ {
		if offset == 0 {
			continue // Already included anchor
		}

		targetFret := anchor.Fret + offset
		// Skip negative frets
		if targetFret < 0 {
			continue
		}

		if positions, exists := allBuckets[targetFret]; exists {
			for _, pos := range positions {
				// Only include chord tones (should already be filtered, but double-check)
				if chord.IsNoteInChord(pos.GetNote()) {
					nearby = append(nearby, pos)
				}
			}
		}
	}

	return nearby
}

func detectBarreForVoicing(fingering [6]int) *Barre {
	// Find lowest non-zero fret
	lowestFret := 999
	hasOpenStrings := false

	for _, fret := range fingering {
		if fret == 0 {
			hasOpenStrings = true
		} else if fret > 0 && fret < lowestFret {
			lowestFret = fret
		}
	}

	// Only barre if no open strings and we found a lowest fret
	if !hasOpenStrings && lowestFret < 999 {
		// Find contiguous played strings
		playedStrings := []int{}
		for i, fret := range fingering {
			if fret >= 0 {
				playedStrings = append(playedStrings, i)
			}
		}

		// Check if played strings are contiguous
		if len(playedStrings) > 0 {
			for i := 1; i < len(playedStrings); i++ {
				if playedStrings[i] != playedStrings[i-1]+1 {
					return nil // Not contiguous
				}
			}

			// Check if there are multiple strings at the barre fret
			barreStringCount := 0
			for _, stringIdx := range playedStrings {
				if fingering[stringIdx] == lowestFret {
					barreStringCount++
				}
			}

			// Need at least 2 strings at the barre fret to make a barre worthwhile
			if barreStringCount >= 2 {
				// Barre spans the entire contiguous played string range
				return &Barre{
					Fret:        lowestFret,
					StartString: playedStrings[0],
					EndString:   playedStrings[len(playedStrings)-1],
				}
			}
		}
	}

	return nil
}

func generateCombinations(positions []PlayedString, size int) [][]PlayedString {
	if size == 0 {
		return [][]PlayedString{{}}
	}
	if len(positions) == 0 {
		return nil
	}

	// Recursive combination generation
	var result [][]PlayedString

	// Include first element
	if len(positions) >= size {
		first := positions[0]
		remaining := positions[1:]
		subCombos := generateCombinations(remaining, size-1)
		for _, combo := range subCombos {
			newCombo := append([]PlayedString{first}, combo...)
			result = append(result, newCombo)
		}
	}

	// Exclude first element
	if len(positions) > size {
		result = append(result, generateCombinations(positions[1:], size)...)
	}

	return result
}

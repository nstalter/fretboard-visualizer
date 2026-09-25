package guitar

import (
	"cmp"
	"github.com/nstalter/fretboard-visualizer/music"
	"slices"
)

// DefaultMaxOpenFret is the default for GuitarVoicingGenerator.MaxOpenFret.
const DefaultMaxOpenFret = 5

type GuitarVoicingGenerator struct {
	tuning  Tuning
	maxFret int

	// MaxOpenFret is the highest fret a voicing may use while it also has an open
	// string: a voicing with an open string and a fretted note above this is dropped.
	// It starts at maxFret, which means no limit.
	MaxOpenFret int
}

func NewGuitarVoicingGenerator(tuning Tuning, maxFret int) *GuitarVoicingGenerator {
	return &GuitarVoicingGenerator{
		tuning:      tuning,
		maxFret:     maxFret,
		MaxOpenFret: maxFret,
	}
}

// GenerateVoicings returns every playable voicing of the chord whose lowest pitch
// satisfies the inversion, sorted by fret (D7).
func (g *GuitarVoicingGenerator) GenerateVoicings(chord music.Chord, inv music.Inversion) []GuitarChordVoicing {
	all, required := chord.Masks()
	at := chord.ToneAt()
	tones := chord.Quality.Tones()

	// Candidates per string: muted, or any fret whose pitch class is a chord tone.
	var candidates [6][]int
	for s := range candidates {
		candidates[s] = []int{-1}
		open := g.tuning.Strings[s].SemitoneValue()
		for fret := 0; fret <= g.maxFret; fret++ {
			if all&(1<<music.PitchClass(open+fret)) != 0 {
				candidates[s] = append(candidates[s], fret)
			}
		}
	}

	voicings := []GuitarChordVoicing{}
	var fingering [6]int
	var walk func(s int, present uint16, lo, hi int)
	walk = func(s int, present uint16, lo, hi int) {
		if s == 6 {
			if present&required != required {
				return
			}
			v := GuitarChordVoicing{Chord: chord, Fingering: fingering}
			v.Barre = detectBarreForVoicing(v.Fingering)
			if !v.IsPlayable() || opensPastFret(v.Fingering, g.MaxOpenFret) {
				return
			}
			bass := at[music.PitchClass(v.BassPitch(g.tuning))]
			if !inv.AllowsBass(tones[bass].Degree) {
				return
			}
			voicings = append(voicings, v)
			return
		}
		open := g.tuning.Strings[s].SemitoneValue()
		for _, fret := range candidates[s] {
			fingering[s] = fret
			switch {
			case fret < 0:
				walk(s+1, present, lo, hi)
			case fret == 0:
				walk(s+1, present|1<<music.PitchClass(open), lo, hi)
			default:
				nlo, nhi := min(lo, fret), max(hi, fret)
				if nhi-nlo > 3 { // same limit as IsPlayable's fret span
					continue
				}
				walk(s+1, present|1<<music.PitchClass(open+fret), nlo, nhi)
			}
		}
	}
	walk(0, 0, MaxFret+1, 0)

	slices.SortFunc(voicings, compareVoicings)
	return voicings
}

// opensPastFret reports whether the fingering has an open string and a fretted note above limit.
func opensPastFret(f [6]int, limit int) bool {
	open, hi := false, 0
	for _, fret := range f {
		open = open || fret == 0
		hi = max(hi, fret)
	}
	return open && hi > limit
}

// compareVoicings orders by lowest fret, then average fret, then the fingering
// compared low→high (muted < open < fretted).
func compareVoicings(a, b GuitarChordVoicing) int {
	if c := cmp.Compare(a.GetMinFret(), b.GetMinFret()); c != 0 {
		return c
	}
	sa, na := frettedSum(a.Fingering)
	sb, nb := frettedSum(b.Fingering)
	if c := cmp.Compare(sa*nb, sb*na); c != 0 {
		return c
	}
	for s := 5; s >= 0; s-- {
		if c := cmp.Compare(a.Fingering[s], b.Fingering[s]); c != 0 {
			return c
		}
	}
	return 0
}

// frettedSum returns the sum and count of fretted (> 0) frets, with the count at least 1.
func frettedSum(f [6]int) (sum, n int) {
	for _, fret := range f {
		if fret > 0 {
			sum += fret
			n++
		}
	}
	return sum, max(n, 1)
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

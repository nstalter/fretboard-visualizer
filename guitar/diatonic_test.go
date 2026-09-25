package guitar

import (
	"testing"

	"github.com/nstalter/fretboard-visualizer/music"
)

func TestDiatonic_AllChordsHaveVoicings(t *testing.T) {
	keys := []string{"C", "C♯", "D♭", "D", "D♯", "E♭", "E", "F", "F♯", "G♭", "G", "G♯", "A♭", "A", "A♯", "B♭", "B"}
	g := NewGuitarVoicingGenerator(StandardTuning(), MaxFret)
	for _, k := range keys {
		for m := music.Ionian; m <= music.Locrian; m++ {
			for _, dc := range music.DiatonicChords(mustNote(t, k), m) {
				chord := music.Chord{Root: dc.Root, Quality: dc.Quality}
				if len(g.GenerateVoicings(chord, music.AnyInversion)) == 0 {
					t.Errorf("%s %s: %s has no voicings", k, m, dc.Root)
				}
			}
		}
	}
}

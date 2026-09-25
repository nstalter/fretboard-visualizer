package guitar

import "github.com/nstalter/fretboard-visualizer/music"

// FretboardTones maps every string (high→low) and fret 0..MaxFret to the index into
// c.Quality.Tones() of the note sounded there, or -1 when it is not a chord tone.
func FretboardTones(c music.Chord, t Tuning) [6][MaxFret + 1]int {
	at := c.ToneAt()
	var m [6][MaxFret + 1]int
	for s := range m {
		open := t.Strings[s].SemitoneValue()
		for fret := range m[s] {
			m[s][fret] = at[music.PitchClass(open+fret)]
		}
	}
	return m
}

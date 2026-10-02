package guitar

import "github.com/nstalter/fretboard-visualizer/music"

// IdentifyChord names the chord a fingering plays, best match first. It is empty when the notes are
// not a chord the app knows, or when nothing is played.
func IdentifyChord(f [6]int, t Tuning) []music.Match {
	v := GuitarChordVoicing{Fingering: f}
	bass := v.BassPitch(t)
	if bass < 0 {
		return nil
	}
	return music.Identify(v.pitchClassMask(t), music.PitchClass(bass))
}

// NewVoicing is the voicing of chord c played as fingering f, with its barre worked out.
func NewVoicing(c music.Chord, f [6]int) GuitarChordVoicing {
	return GuitarChordVoicing{Chord: c, Fingering: f, Barre: detectBarreForVoicing(f)}
}

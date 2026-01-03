package music

import "testing"

func TestFilterChordTones(t *testing.T) {
	tuning := StandardTuning()
	chord := Chord{Root: Note{Name: C, Accidental: Natural, Octave: 4}, Quality: Major} // C, E, G

	// Create some positions
	positions := []PlayedString{
		{StringIndex: 5, Fret: 0, Tuning: tuning}, // E (not in chord)
		{StringIndex: 4, Fret: 0, Tuning: tuning}, // A (not in chord)
		{StringIndex: 3, Fret: 0, Tuning: tuning}, // D (not in chord)
		{StringIndex: 5, Fret: 8, Tuning: tuning}, // C (in chord)
		{StringIndex: 4, Fret: 2, Tuning: tuning}, // E (in chord)
		{StringIndex: 3, Fret: 5, Tuning: tuning}, // G (in chord)
	}

	chordPositions := filterChordTones(positions, chord)

	if len(chordPositions) != 3 {
		t.Errorf("Expected 3 chord positions, got %d", len(chordPositions))
	}

	// Verify all returned positions are chord tones
	for _, pos := range chordPositions {
		note := pos.GetNote()
		if !chord.IsNoteInChord(note) {
			t.Errorf("Position returned note %s which is not in chord", note.String())
		}
	}
}

func TestPlayedStringsToVoicing(t *testing.T) {
	positions := []PlayedString{
		{StringIndex: 5, Fret: 0}, // Low E open
		{StringIndex: 4, Fret: 2}, // A string fret 2
		{StringIndex: 3, Fret: 0}, // D string open
	}

	chord := Chord{Root: Note{Name: C, Accidental: Natural, Octave: 4}, Quality: Major}
	voicing := playedStringsToVoicing(positions, chord)

	// Check fingering
	expected := [6]int{-1, -1, -1, 0, 2, 0} // Strings: [E A D G B E] -> [X X X 0 2 0]
	for i, fret := range voicing.Fingering {
		if fret != expected[i] {
			t.Errorf("Expected fret %d at string %d, got %d", expected[i], i, fret)
		}
	}

	// Check chord
	if voicing.Chord.Root.Name != C || voicing.Chord.Quality != Major {
		t.Error("Chord not set correctly in voicing")
	}
}

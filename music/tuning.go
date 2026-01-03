package music

type Tuning struct {
	Strings [6]Note
}

func StandardTuning() Tuning {
	return Tuning{
		Strings: [6]Note{
			{Name: E, Accidental: Natural, Octave: 4}, // high e
			{Name: B, Accidental: Natural, Octave: 3},
			{Name: G, Accidental: Natural, Octave: 3},
			{Name: D, Accidental: Natural, Octave: 3},
			{Name: A, Accidental: Natural, Octave: 2},
			{Name: E, Accidental: Natural, Octave: 2}, // low E
		},
	}
}

func CalculateNote(openNote Note, fret int) Note {
	if fret == 0 {
		return openNote
	}

	startSemitone := openNote.SemitoneValue()

	newSemitone := (startSemitone + fret) % 12

	return semitoneToNote(newSemitone)
}

func semitoneToNote(semitone int) Note {
	noteMap := map[int]Note{
		0:  {Name: C, Accidental: Natural},
		1:  {Name: C, Accidental: Sharp},
		2:  {Name: D, Accidental: Natural},
		3:  {Name: D, Accidental: Sharp},
		4:  {Name: E, Accidental: Natural},
		5:  {Name: F, Accidental: Natural},
		6:  {Name: F, Accidental: Sharp},
		7:  {Name: G, Accidental: Natural},
		8:  {Name: G, Accidental: Sharp},
		9:  {Name: A, Accidental: Natural},
		10: {Name: A, Accidental: Sharp},
		11: {Name: B, Accidental: Natural},
	}
	return noteMap[semitone]
}

package music

import (
	"fmt"
	"strings"
)

type ChordQuality int

const (
	Major ChordQuality = iota
	Minor
	Major7
)

// AllChordQualities returns all available chord qualities with their display names
func AllChordQualities() []struct {
	Quality ChordQuality
	Name    string
} {
	return []struct {
		Quality ChordQuality
		Name    string
	}{
		{Major, "Major"},
		{Minor, "Minor"},
		{Major7, "Major 7"},
	}
}

// ParseChordQuality parses a string into a ChordQuality
func ParseChordQuality(s string) (ChordQuality, error) {
	switch strings.ToLower(s) {
	case "major":
		return Major, nil
	case "minor":
		return Minor, nil
	case "major 7", "major7":
		return Major7, nil
	default:
		return Major, fmt.Errorf("unknown chord quality: %s", s)
	}
}

// String returns the display name for a chord quality
func (q ChordQuality) String() string {
	switch q {
	case Major:
		return "Major"
	case Minor:
		return "Minor"
	case Major7:
		return "Major 7"
	default:
		return "Major"
	}
}

type Chord struct {
	Root    Note
	Quality ChordQuality
}

func (c Chord) GetChordIntervals() []int {
	switch c.Quality {
	case Major:
		return []int{0, 4, 7}
	case Minor:
		return []int{0, 3, 7}
	case Major7:
		return []int{0, 4, 7, 11}
	default:
		return []int{0, 4, 7}
	}
}

func (c Chord) GetChordNotes() []Note {
	intervals := c.GetChordIntervals()
	rootSemitone := c.Root.SemitoneValue()

	notes := make([]Note, len(intervals))
	for i, interval := range intervals {
		semitone := (rootSemitone + interval) % 12
		notes[i] = semitoneToNote(semitone)
	}
	return notes
}

func (c Chord) IsNoteInChord(note Note) bool {
	noteSemitone := note.SemitoneValue()
	chordNotes := c.GetChordNotes()

	for _, chordNote := range chordNotes {
		if chordNote.SemitoneValue() == noteSemitone {
			return true
		}
	}
	return false
}

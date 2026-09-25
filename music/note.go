package music

import (
	"fmt"
	"strconv"
	"strings"
)

type NoteName int

const (
	C NoteName = iota
	D
	E
	F
	G
	A
	B
)

// Accidental is a signed semitone count: -1 is ♭, +1 is ♯, ±2 is double, and so on.
type Accidental int

const (
	Flat    Accidental = -1
	Natural Accidental = 0
	Sharp   Accidental = 1
)

type Note struct {
	Name       NoteName
	Accidental Accidental
	Octave     int
}

var noteLetters = [7]string{"C", "D", "E", "F", "G", "A", "B"}

// naturalSemitones is the pitch class of each natural note, C..B.
var naturalSemitones = [7]int{0, 2, 4, 5, 7, 9, 11}

func mod12(n int) int { return ((n % 12) + 12) % 12 }

// ParseNote parses a note name without octave: "C", "Bb", "B♭", "F##", "Cb".
func ParseNote(s string) (Note, error) {
	n, rest, err := parseNotePrefix(s)
	if err != nil {
		return Note{}, err
	}
	if rest != "" {
		return Note{}, fmt.Errorf("invalid note %q", s)
	}
	return n, nil
}

// ParsePitch parses a note name followed by an octave: "E2", "Eb2", "F♯3".
func ParsePitch(s string) (Note, error) {
	n, rest, err := parseNotePrefix(s)
	if err != nil {
		return Note{}, err
	}
	octave, err := strconv.Atoi(rest)
	if err != nil || rest == "" || rest[0] == '+' || rest[0] == '-' {
		return Note{}, fmt.Errorf("invalid pitch %q: needs an octave, e.g. E2", s)
	}
	n.Octave = octave
	return n, nil
}

func parseNotePrefix(s string) (Note, string, error) {
	if s == "" {
		return Note{}, "", fmt.Errorf("empty note")
	}
	idx := strings.Index("CDEFGAB", s[:1])
	if idx < 0 {
		return Note{}, "", fmt.Errorf("invalid note %q", s)
	}
	n := Note{Name: NoteName(idx)}
	rest := s[1:]
	sharps, flats := 0, 0
	for {
		switch {
		case strings.HasPrefix(rest, "#"):
			sharps++
			rest = rest[1:]
		case strings.HasPrefix(rest, "♯"):
			sharps++
			rest = rest[len("♯"):]
		case strings.HasPrefix(rest, "b"):
			flats++
			rest = rest[1:]
		case strings.HasPrefix(rest, "♭"):
			flats++
			rest = rest[len("♭"):]
		default:
			if sharps > 0 && flats > 0 {
				return Note{}, "", fmt.Errorf("invalid note %q: mixes sharps and flats", s)
			}
			n.Accidental = Accidental(sharps - flats)
			return n, rest, nil
		}
	}
}

// String returns the note name with ♯/♭ repeated as needed, without octave.
func (n Note) String() string {
	s := noteLetters[n.Name]
	if n.Accidental > 0 {
		s += strings.Repeat("♯", int(n.Accidental))
	} else if n.Accidental < 0 {
		s += strings.Repeat("♭", int(-n.Accidental))
	}
	return s
}

// SemitoneValue returns the position in the chromatic scale (0-11)
func (n Note) SemitoneValue() int {
	return mod12(naturalSemitones[n.Name] + int(n.Accidental))
}

// MIDI returns the MIDI pitch number: E2 = 40, B♯3 = 60, C♭4 = 59.
func (n Note) MIDI() int {
	return (n.Octave+1)*12 + naturalSemitones[n.Name] + int(n.Accidental)
}

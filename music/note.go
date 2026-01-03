package music

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

type Accidental int

const (
	Natural Accidental = iota
	Sharp
	Flat
)

type Note struct {
	Name       NoteName
	Accidental Accidental
	Octave     int
}

func (n Note) String() string {
	noteNames := []string{"C", "D", "E", "F", "G", "A", "B"}
	name := noteNames[n.Name]

	switch n.Accidental {
	case Sharp:
		return name + "#"
	case Flat:
		return name + "b"
	default:
		return name
	}
}

// SemitoneValue returns the position in the chromatic scale (0-11)
func (n Note) SemitoneValue() int {
	baseValues := []int{0, 2, 4, 5, 7, 9, 11} // C, D, E, F, G, A, B
	value := baseValues[n.Name]

	switch n.Accidental {
	case Sharp:
		value++
	case Flat:
		value--
	}

	return value % 12
}

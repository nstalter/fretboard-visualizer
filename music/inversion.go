package music

import "fmt"

// Inversion selects which chord degree must be the lowest-sounding pitch.
type Inversion int

const (
	AnyInversion Inversion = iota
	RootPosition
	ThirdInBass
	FifthInBass
	SeventhInBass
)

var inversionNames = [...]string{"any", "root", "3rd", "5th", "7th"}

// ParseInversion parses "any" | "root" | "3rd" | "5th" | "7th".
func ParseInversion(s string) (Inversion, error) {
	for i, name := range inversionNames {
		if name == s {
			return Inversion(i), nil
		}
	}
	return 0, fmt.Errorf("unknown inversion %q", s)
}

func (i Inversion) String() string { return inversionNames[i] }

// degree is the chord degree this inversion puts in the bass (0 for Any).
func (i Inversion) degree() int {
	return [...]int{0, 1, 3, 5, 7}[i]
}

// AllowsBass reports whether a tone of the given degree may be the lowest pitch.
// Any applies no filter.
func (i Inversion) AllowsBass(degree int) bool {
	return i == AnyInversion || i.degree() == degree
}

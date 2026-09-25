package music

import (
	"fmt"
	"strconv"
	"strings"
)

const MaxFret = 22

// ParseFingering parses six frets written low to high, either compact ("x32010")
// or dash-separated ("x-10-12-12-11-10"), into the internal high→low order.
func ParseFingering(s string) ([6]int, error) {
	var tokens []string
	if strings.Contains(s, "-") {
		tokens = strings.Split(s, "-")
	} else {
		tokens = strings.Split(s, "")
	}
	if len(tokens) != 6 {
		return [6]int{}, fmt.Errorf("want 6 frets, got %d", len(tokens))
	}
	var f [6]int
	for i, tok := range tokens {
		fret := -1
		if tok != "x" {
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 || n > MaxFret || tok != strconv.Itoa(n) {
				return [6]int{}, fmt.Errorf("invalid fret %q", tok)
			}
			fret = n
		}
		f[5-i] = fret
	}
	return f, nil
}

// FormatFingering is the inverse of ParseFingering: compact when every fret is ≤ 9, else dashed.
func FormatFingering(f [6]int) string {
	tokens := make([]string, 6)
	sep := ""
	for i := range tokens {
		fret := f[5-i]
		if fret < 0 {
			tokens[i] = "x"
		} else {
			tokens[i] = strconv.Itoa(fret)
		}
		if fret > 9 {
			sep = "-"
		}
	}
	return strings.Join(tokens, sep)
}

package guitar

import "testing"

// Every fingering over frets x,0-7: PlayIssue agrees with the rules the generator uses, and
// BuildIssue never rejects a shape that can still be completed.
func TestBuildAndPlayIssue_ExhaustiveOverLowFrets(t *testing.T) {
	const openMax = 5
	var f [6]int
	var walk func(s int)
	walk = func(s int) {
		if s == 6 {
			v := GuitarChordVoicing{Fingering: f, Barre: detectBarreForVoicing(f)}
			playable := v.IsPlayable() && !opensPastFret(f, openMax)
			if got := PlayIssue(f, openMax) == ""; got != playable {
				t.Fatalf("%v: PlayIssue empty = %v, want %v (%q)", f, got, playable, PlayIssue(f, openMax))
			}
			if playable {
				// Removing any notes from a playable shape leaves one that can still be completed.
				for mask := 0; mask < 64; mask++ {
					g := f
					for i := range g {
						if mask&(1<<i) != 0 {
							g[i] = -1
						}
					}
					if issue := BuildIssue(g, openMax); issue != "" {
						t.Fatalf("%v is playable but its subset %v is rejected: %s", f, g, issue)
					}
				}
			}
			return
		}
		for fret := -1; fret <= 7; fret++ {
			f[s] = fret
			walk(s + 1)
		}
	}
	walk(0)
}

func TestBuildIssue(t *testing.T) {
	tests := []struct {
		name string
		f    string
		want bool // true: rejected
	}{
		{"nothing played yet", "xxxxxx", false},
		{"one note", "xxxxx3", false},
		{"two notes with a gap, still buildable", "x3xx3x", false},
		{"C7 shape", "x32310", false},
		{"five-fret stretch", "x1xx5x", true},
		{"four-fret span is fine", "x1xx4x", false},
		{"five fingers within four frets", "x-1-2-3-4-4", true},
		{"open string with a high note", "0xx7xx", true},
	}
	for _, tt := range tests {
		f, err := ParseFingering(tt.f)
		if err != nil {
			t.Fatal(tt.name, err)
		}
		if got := BuildIssue(f, 5) != ""; got != tt.want {
			t.Errorf("%s (%s): rejected = %v (%q), want %v", tt.name, tt.f, got, BuildIssue(f, 5), tt.want)
		}
	}
}

func TestPlayIssue_Messages(t *testing.T) {
	tests := []struct{ f, want string }{
		{"x32310", ""},
		{"x3231x", ""}, // the high E may be muted
		{"xx3xxx", "a chord needs at least three notes"},
		{"x3x231", "a muted string sits between played strings"},
		{"x1xx5x", "that stretches over more than four frets"},
	}
	for _, tt := range tests {
		f, err := ParseFingering(tt.f)
		if err != nil {
			t.Fatal(err)
		}
		if got := PlayIssue(f, 22); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.f, got, tt.want)
		}
	}
}

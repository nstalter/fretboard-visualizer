package guitar

import "fmt"

// BuildIssue explains why a fingering can never become playable, however many notes are added to
// it, or returns "" if it still can. It is for checking a chord note by note: a shape with two
// notes or a gap between them is not playable yet, but a shape stretched over five frets never will be.
func BuildIssue(f [6]int, openMax int) string {
	switch {
	case !fretSpanPlayable(f, 3):
		return "that stretches over more than four frets"
	case fingersNeeded(f) > 4:
		return "that needs more than four fingers"
	case opensPastFret(f, openMax):
		return fmt.Sprintf("an open string can't be used with a note above fret %d (see the open-string limit)", openMax)
	}
	return ""
}

// fingersNeeded is the fewest fingers any way of playing f could use: one for the lowest fretted
// fret, since a barre covers every note there, plus one for each note above it.
func fingersNeeded(f [6]int) int {
	lowest := 0
	for _, fret := range f {
		if fret > 0 && (lowest == 0 || fret < lowest) {
			lowest = fret
		}
	}
	if lowest == 0 {
		return 0
	}
	n := 1
	for _, fret := range f {
		if fret > lowest {
			n++
		}
	}
	return n
}

// PlayIssue explains why a finished fingering is not playable, or returns "" if it is. It applies
// the same rules the voicing generator does.
func PlayIssue(f [6]int, openMax int) string {
	if issue := BuildIssue(f, openMax); issue != "" {
		return issue
	}
	played := 0
	for _, fret := range f {
		if fret >= 0 {
			played++
		}
	}
	if played < 3 {
		return "a chord needs at least three notes"
	}
	v := GuitarChordVoicing{Fingering: f, Barre: detectBarreForVoicing(f)}
	switch {
	case v.IsPlayable():
		return ""
	case !allPlayedStringsContiguous(f):
		return "a muted string sits between played strings"
	}
	return "those notes can't be fingered comfortably"
}

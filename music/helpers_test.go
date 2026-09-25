package music

import "testing"

func mustNote(t testing.TB, s string) Note {
	t.Helper()
	n, err := ParseNote(s)
	if err != nil {
		t.Fatalf("ParseNote(%q): %v", s, err)
	}
	return n
}

func chordOf(t testing.TB, root string, q ChordQuality) Chord {
	return Chord{Root: mustNote(t, root), Quality: q}
}

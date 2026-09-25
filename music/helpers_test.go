package music

import "testing"

func mustFingering(t testing.TB, s string) [6]int {
	t.Helper()
	f, err := ParseFingering(s)
	if err != nil {
		t.Fatalf("ParseFingering(%q): %v", s, err)
	}
	return f
}

func mustNote(t testing.TB, s string) Note {
	t.Helper()
	n, err := ParseNote(s)
	if err != nil {
		t.Fatalf("ParseNote(%q): %v", s, err)
	}
	return n
}

func mustTuning(t testing.TB, s string) Tuning {
	t.Helper()
	tu, err := ParseTuning(s)
	if err != nil {
		t.Fatalf("ParseTuning(%q): %v", s, err)
	}
	return tu
}

func chordOf(t testing.TB, root string, q ChordQuality) Chord {
	return Chord{Root: mustNote(t, root), Quality: q}
}

const (
	stdTuning   = "E2,A2,D3,G3,B3,E4"
	dropDTuning = "D2,A2,D3,G3,B3,E4"
	openGTuning = "D2,G2,D3,G3,B3,D4"
)

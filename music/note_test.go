package music

import "testing"

func TestParseNote(t *testing.T) {
	for s, pc := range map[string]int{"C": 0, "Bb": 10, "B♭": 10, "F##": 7, "F♯♯": 7, "Cb": 11, "B#": 0, "Ebb": 2} {
		n, err := ParseNote(s)
		if err != nil {
			t.Errorf("ParseNote(%q): %v", s, err)
			continue
		}
		if got := n.SemitoneValue(); got != pc {
			t.Errorf("ParseNote(%q) pc = %d, want %d", s, got, pc)
		}
	}
	for _, s := range []string{"", "H", "c", "C#b", "C4", "C x"} {
		if _, err := ParseNote(s); err == nil {
			t.Errorf("ParseNote(%q) accepted", s)
		}
	}
	if got := mustNote(t, "F##").String(); got != "F♯♯" {
		t.Errorf("String = %q, want F♯♯", got)
	}
	if got := mustNote(t, "Bbb").String(); got != "B♭♭" {
		t.Errorf("String = %q, want B♭♭", got)
	}
}

func TestParsePitch(t *testing.T) {
	for s, midi := range map[string]int{"E2": 40, "Eb2": 39, "F♯3": 54, "B#3": 60, "Cb4": 59, "C4": 60} {
		n, err := ParsePitch(s)
		if err != nil {
			t.Errorf("ParsePitch(%q): %v", s, err)
			continue
		}
		if got := n.MIDI(); got != midi {
			t.Errorf("ParsePitch(%q).MIDI() = %d, want %d", s, got, midi)
		}
	}
	for _, s := range []string{"E", "E+2", "E2x", "", "H2"} {
		if _, err := ParsePitch(s); err == nil {
			t.Errorf("ParsePitch(%q) accepted", s)
		}
	}
}

func TestSemitoneValueWraps(t *testing.T) {
	if got := (Note{Name: C, Accidental: Flat}).SemitoneValue(); got != 11 {
		t.Errorf("Cb = %d, want 11", got)
	}
	if got := (Note{Name: B, Accidental: Sharp}).SemitoneValue(); got != 0 {
		t.Errorf("B# = %d, want 0", got)
	}
}

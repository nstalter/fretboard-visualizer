package guitar

import "testing"

func TestParseTuning(t *testing.T) {
	if got := mustTuning(t, stdTuning); got != StandardTuning() {
		t.Errorf("ParseTuning(std) = %v, want StandardTuning()", got)
	}
	if got := mustTuning(t, dropDTuning).Strings[5].String(); got != "D" {
		t.Errorf("drop D low string = %s, want D", got)
	}
	ok := []string{"A2,A2,D3,G3,B3,E4", "B1,A2,D3,G3,B3,E4", "E2,A2,D3,G3,B3,A4"}
	for _, s := range ok {
		if _, err := ParseTuning(s); err != nil {
			t.Errorf("ParseTuning(%q): %v", s, err)
		}
	}
	bad := []string{
		"A#2,A2,D3,G3,B3,E4", // +6
		"A#1,A2,D3,G3,B3,E4", // −6
		"E2,A2,D3,G3,B3",     // 5 strings
		"E2,A2,D3,G3,B3,E4,E4",
		"E,A,D,G,B,E", // no octaves
		"",
	}
	for _, s := range bad {
		if _, err := ParseTuning(s); err == nil {
			t.Errorf("ParseTuning(%q) accepted", s)
		}
	}
}

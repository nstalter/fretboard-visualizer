package guitar

import "testing"

func TestParseFingering(t *testing.T) {
	cases := map[string][6]int{
		"x32010":           {0, 1, 0, 2, 3, -1},
		"x-10-12-12-11-10": {10, 11, 12, 12, 10, -1},
		"8-10-10-9-8-8":    {8, 8, 9, 10, 10, 8},
		"x-3-2-0-1-0":      {0, 1, 0, 2, 3, -1},
		"22-x-x-x-x-x":     {-1, -1, -1, -1, -1, 22},
	}
	for s, want := range cases {
		got, err := ParseFingering(s)
		if err != nil || got != want {
			t.Errorf("ParseFingering(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	for s, want := range map[string]string{"x32010": "x32010", "x-3-2-0-1-0": "x32010", "x-10-12-12-11-10": "x-10-12-12-11-10", "8-10-10-9-8-8": "8-10-10-9-8-8"} {
		if got := FormatFingering(mustFingering(t, s)); got != want {
			t.Errorf("FormatFingering(%q) = %q, want %q", s, got, want)
		}
	}
	for _, s := range []string{"x3201", "x320100", "x32o10", "x-3-2-0-1", "23-x-x-x-x-x", "X32010", "x-03-2-0-1-0", "x--3-2-0-1-0", ""} {
		if _, err := ParseFingering(s); err == nil {
			t.Errorf("ParseFingering(%q) accepted", s)
		}
	}
}

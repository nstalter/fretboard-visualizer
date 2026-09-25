package music

import (
	"slices"
	"strings"
	"testing"
)

func spell(c Chord) string {
	var names []string
	for _, tone := range c.Quality.Tones() {
		names = append(names, c.Spell(tone).String())
	}
	return strings.Join(names, " ")
}

func labels(q ChordQuality) string {
	var ls []string
	for _, tone := range q.Tones() {
		ls = append(ls, tone.Label())
	}
	return strings.Join(ls, " ")
}

func TestSpell_AllQualities(t *testing.T) {
	want := map[string]string{
		"maj": "C E G", "min": "C E♭ G", "dim": "C E♭ G♭", "aug": "C E G♯",
		"sus2": "C D G", "sus4": "C F G", "6": "C E G A", "m6": "C E♭ G A",
		"7": "C E G B♭", "maj7": "C E G B", "m7": "C E♭ G B♭", "m7b5": "C E♭ G♭ B♭",
		"dim7": "C E♭ G♭ B♭♭", "7sus4": "C F G B♭", "add9": "C E G D",
		"9": "C E G B♭ D", "maj9": "C E G B D", "m9": "C E♭ G B♭ D",
		"11": "C E G B♭ D F", "m11": "C E♭ G B♭ D F",
		"13": "C E G B♭ D F A", "maj13": "C E G B D F A", "m13": "C E♭ G B♭ D F A",
	}
	if len(want) != len(AllChordQualities()) {
		t.Fatalf("%d qualities, want %d", len(AllChordQualities()), len(want))
	}
	for _, q := range AllChordQualities() {
		if got := spell(Chord{Root: Note{Name: C}, Quality: q}); got != want[q.String()] {
			t.Errorf("C %s = %q, want %q", q, got, want[q.String()])
		}
	}
}

func TestSpell_TrickyRoots(t *testing.T) {
	cases := []struct {
		root string
		q    ChordQuality
		want string
	}{
		{"Bb", Major, "B♭ D F"},
		{"Bb", Diminished7, "B♭ D♭ F♭ A♭♭"},
		{"Gb", Minor, "G♭ B♭♭ D♭"},
		{"Cb", Major, "C♭ E♭ G♭"},
		{"G#", Augmented, "G♯ B♯ D♯♯"},
		{"B", Major7, "B D♯ F♯ A♯"},
	}
	for _, c := range cases {
		if got := spell(chordOf(t, c.root, c.q)); got != c.want {
			t.Errorf("%s %s = %q, want %q", c.root, c.q, got, c.want)
		}
	}
}

func TestToneLabels(t *testing.T) {
	for q, want := range map[ChordQuality]string{
		Dominant13:  "R 3 5 ♭7 9 11 13",
		Diminished7: "R ♭3 ♭5 ♭♭7",
		Sus2:        "R 2 5",
		Augmented:   "R 3 ♯5",
	} {
		if got := labels(q); got != want {
			t.Errorf("%s labels = %q, want %q", q, got, want)
		}
	}
}

func TestChordName(t *testing.T) {
	for _, c := range []struct {
		root string
		q    ChordQuality
		want string
	}{{"Bb", HalfDiminished7, "B♭m7♭5"}, {"B", Diminished, "Bdim"}, {"E", Minor, "Em"}, {"C", Major, "C"}} {
		if got := chordOf(t, c.root, c.q).Name(); got != c.want {
			t.Errorf("Name = %q, want %q", got, c.want)
		}
	}
}

func TestParseChordQuality(t *testing.T) {
	for _, q := range AllChordQualities() {
		if got, err := ParseChordQuality(q.String()); err != nil || got != q {
			t.Errorf("ParseChordQuality(%q) = %v, %v", q.String(), got, err)
		}
	}
	for _, s := range []string{"M7", "Major", "", "major"} {
		if _, err := ParseChordQuality(s); err == nil {
			t.Errorf("ParseChordQuality(%q) accepted", s)
		}
	}
}

func TestInversionsOffered(t *testing.T) {
	names := func(q ChordQuality) []string {
		var ns []string
		for _, i := range q.Inversions() {
			ns = append(ns, i.String())
		}
		return ns
	}
	for q, want := range map[ChordQuality][]string{
		Major:           {"any", "root", "3rd", "5th"},
		Sus4:            {"any", "root", "5th"},
		Sus2:            {"any", "root", "5th"},
		Sixth:           {"any", "root", "3rd", "5th"},
		Add9:            {"any", "root", "3rd", "5th"},
		Dominant7:       {"any", "root", "3rd", "5th", "7th"},
		Dominant7Sus4:   {"any", "root", "5th", "7th"},
		Dominant11:      {"any", "root", "3rd", "5th", "7th"},
		HalfDiminished7: {"any", "root", "3rd", "5th", "7th"},
	} {
		if got := names(q); !slices.Equal(got, want) {
			t.Errorf("%s inversions = %v, want %v", q, got, want)
		}
	}
}

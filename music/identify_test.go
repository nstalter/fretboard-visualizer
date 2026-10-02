package music

import (
	"fmt"
	"slices"
	"testing"
)

// mask builds a pitch-class bitmask from note names.
func mask(t *testing.T, notes ...string) uint16 {
	t.Helper()
	var m uint16
	for _, s := range notes {
		m |= 1 << mustNote(t, s).SemitoneValue()
	}
	return m
}

func matchNames(ms []Match) []string {
	var out []string
	for _, m := range ms {
		out = append(out, fmt.Sprintf("%s/%s", m.Chord.Name(), m.Inversion))
	}
	return out
}

func TestIdentify_BestMatch(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		bass  string
		want  string // best match: chord/inversion
	}{
		{"C major", []string{"C", "E", "G"}, "C", "C/root"},
		{"C major, E in bass", []string{"C", "E", "G"}, "E", "C/3rd"},
		{"C major, G in bass", []string{"C", "E", "G"}, "G", "C/5th"},
		{"C7 (the x32310 shape)", []string{"C", "E", "G", "B♭"}, "C", "C7/root"},
		{"C7 without the fifth", []string{"C", "E", "B♭"}, "C", "C7/root"},
		{"C6 beats Am7/C when C is in the bass", []string{"C", "E", "G", "A"}, "C", "C6/root"},
		{"Am7 when A is in the bass", []string{"C", "E", "G", "A"}, "A", "Am7/root"},
		{"G♯m7 is spelled with sharps", []string{"G♯", "B", "D♯", "F♯"}, "G♯", "G♯m7/root"},
		{"F♯m7", []string{"F♯", "A", "C♯", "E"}, "F♯", "F♯m7/root"},
		{"B♭ major is spelled with a flat", []string{"B♭", "D", "F"}, "B♭", "B♭/root"},
		{"D♭ major is spelled with a flat", []string{"D♭", "F", "A♭"}, "D♭", "D♭/root"},
		{"Cmaj7 without the fifth", []string{"C", "E", "B"}, "C", "Cmaj7/root"},
		{"C diminished 7 with C in the bass", []string{"C", "E♭", "G♭", "A"}, "C", "Cdim7/root"},
		{"E♭ diminished 7 with E♭ in the bass", []string{"C", "E♭", "G♭", "A"}, "E♭", "E♭dim7/root"},
		{"D sus4", []string{"D", "G", "A"}, "D", "Dsus4/root"},
	}
	for _, tt := range tests {
		bass := mustNote(t, tt.bass).SemitoneValue()
		got := Identify(mask(t, tt.notes...), bass)
		if len(got) == 0 {
			t.Errorf("%s: no match", tt.name)
			continue
		}
		if best := matchNames(got)[0]; best != tt.want {
			t.Errorf("%s: best = %s, want %s (all: %v)", tt.name, best, tt.want, matchNames(got))
		}
	}
}

func TestIdentify_AlternativesIncludeTheOtherReadings(t *testing.T) {
	got := matchNames(Identify(mask(t, "C", "E", "G", "A"), mustNote(t, "C").SemitoneValue()))
	for _, want := range []string{"C6/root", "Am7/3rd"} {
		if !slices.Contains(got, want) {
			t.Errorf("C E G A with C in the bass: %v lacks %s", got, want)
		}
	}
}

func TestIdentify_NoMatch(t *testing.T) {
	for _, notes := range [][]string{
		{"C", "C♯", "D"},            // a cluster
		{"C", "E", "G", "B♭", "D♭"}, // C7 plus a ♭9, which no listed chord has
		{"C", "D♭", "E", "G", "A♭", "B"},
	} {
		if got := Identify(mask(t, notes...), mustNote(t, notes[0]).SemitoneValue()); len(got) != 0 {
			t.Errorf("%v: matched %v, want none", notes, matchNames(got))
		}
	}
}

// Every chord spelled in full, with any of its tones in the bass, is identified as itself.
func TestIdentify_EveryQualityRoundTrips(t *testing.T) {
	for pc := range 12 {
		for _, q := range AllChordQualities() {
			c := Chord{Root: rootNote(pc, q), Quality: q}
			all, _ := c.Masks()
			for bass := range 12 {
				if all&(1<<bass) == 0 {
					continue
				}
				found := false
				for _, m := range Identify(all, bass) {
					found = found || (m.Chord.Root.SemitoneValue() == pc && m.Chord.Quality == q)
				}
				if !found {
					t.Errorf("%s with bass %d not found among %v", c.Name(), bass, matchNames(Identify(all, bass)))
				}
			}
		}
	}
}

// Every match offers its inversion, and a match's required tones are all present.
func TestIdentify_MatchesAreConsistent(t *testing.T) {
	for m := uint16(1); m < 1<<12; m++ {
		for bass := range 12 {
			if m&(1<<bass) == 0 {
				continue
			}
			for _, match := range Identify(m, bass) {
				all, required := match.Chord.Masks()
				if m&^all != 0 || m&required != required {
					t.Fatalf("mask %012b: %s does not fit", m, match.Chord.Name())
				}
				if !slices.Contains(match.Chord.Quality.Inversions(), match.Inversion) {
					t.Fatalf("%s offers no %s inversion", match.Chord.Name(), match.Inversion)
				}
			}
		}
	}
}

func TestIdentify_EnharmonicSpellings(t *testing.T) {
	// G♯7 is also A♭7: the usual spelling for a dominant 7 comes first, its twin right after it.
	g7 := mask(t, "G♯", "B♯", "D♯", "F♯")
	bass := mustNote(t, "G♯").SemitoneValue()
	if got := matchNames(Identify(g7, bass))[:2]; got[0] != "A♭7/root" || got[1] != "G♯7/root" {
		t.Errorf("G♯7: %v, want A♭7 then G♯7", got)
	}
	// A minor chord is spelled with sharps first, with the flat name after it.
	gm7 := mask(t, "G♯", "B", "D♯", "F♯")
	if got := matchNames(Identify(gm7, bass))[:2]; got[0] != "G♯m7/root" || got[1] != "A♭m7/root" {
		t.Errorf("G♯m7: %v, want G♯m7 then A♭m7", got)
	}
}

// A root is not offered under a second name when that would need a double sharp or flat.
func TestIdentify_NoDoubleAccidentalRespelling(t *testing.T) {
	eFlat := mask(t, "E♭", "G", "B♭") // E♭ major; D♯ major would be D♯ F𝄪 A♯
	for _, name := range matchNames(Identify(eFlat, mustNote(t, "E♭").SemitoneValue())) {
		if name == "D♯/root" {
			t.Errorf("offered %s", name)
		}
	}
}

// Respelling never changes which pitch classes a chord covers, and no root has two entries for one chord.
func TestIdentify_SpellingsAreTheSameChord(t *testing.T) {
	for m := uint16(1); m < 1<<12; m += 7 {
		for bass := range 12 {
			if m&(1<<bass) == 0 {
				continue
			}
			seen := map[string]bool{}
			for _, match := range Identify(m, bass) {
				k := match.Chord.Name() + "/" + match.Inversion.String()
				if seen[k] {
					t.Fatalf("mask %012b: %s listed twice", m, k)
				}
				seen[k] = true
			}
		}
	}
}

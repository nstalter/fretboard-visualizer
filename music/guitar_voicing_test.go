package music

import (
	"testing"
)

func TestGuitarChordVoicing_IsPlayable(t *testing.T) {
	tests := []struct {
		name     string
		voicing  GuitarChordVoicing
		expected bool
	}{
		{
			name: "Valid open C major chord",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 1, 0, 2, 3, -1}, // C major open
			},
			expected: true,
		},
		{
			name: "Valid barre chord (F major shape)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{1, 3, 3, 2, 1, 1}, // F major barre
				Barre:     &Barre{Fret: 1, StartString: 0, EndString: 5},
			},
			expected: true,
		},
		{
			name: "Too few strings (only 2 played)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{-1, -1, -1, -1, 3, 5}, // Only 2 strings
			},
			expected: false,
		},
		{
			name: "Too large a fret span (1 - 5)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 1, 2, 3, 4, 5}, // All 6 strings played
			},
			expected: false,
		},
		{
			name: "Valid single fretted note",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 0, 0, 6, -1, -1}, // Only fret 6 is fretted
			},
			expected: true,
		},
		{
			name: "Bad mute pattern (muting below played strings)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{-1, -1, -1, 3, 3, -1}, // Adjacent strings at fret 3
			},
			expected: false,
		},
		{
			name: "Valid adjacent fingering with barre",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{3, 3, 3, 3, 3, 3}, // All strings at fret 3
				Barre:     &Barre{Fret: 3, StartString: 0, EndString: 5},
			},
			expected: true,
		},
		{
			name: "Valid 4-string voicing",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{2, 3, 0, 0, -1, -1}, // 4 strings played, mute lowest
			},
			expected: true,
		},
		{
			name: "Valid 5-string voicing",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 2, 2, 1, 0, -1}, // 5 strings played, mute low E
			},
			expected: true,
		},
		{
			name: "Valid 3-string voicing (minimum)",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 2, 3, -1, -1, -1}, // 3 strings played, mute lowest 3
			},
			expected: true,
		},
		{
			name: "Invalid: muting high strings",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{-1, -1, 0, 2, 3, 1}, // Muting high strings is hard
			},
			expected: false,
		},
		{
			name: "Valid open strings only",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{0, 0, 0, 0, 0, 0}, // All open strings
			},
			expected: true,
		},
		{
			name: "Invalid barre: non-contiguous played strings",
			voicing: GuitarChordVoicing{
				Fingering: [6]int{3, 5, 5, -1, 3, 3}, // Played: [0,1,2,4,5] - not contiguous
				Barre:     &Barre{Fret: 3, StartString: 0, EndString: 5},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.voicing.IsPlayable()
			if result != tt.expected {
				t.Errorf("IsPlayable() = %v, expected %v for voicing: %v",
					result, tt.expected, tt.voicing.Fingering)
			}
		})
	}
}

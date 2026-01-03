package ui

import (
	"fmt"
	"image/color"

	"github.com/fogleman/gg"
	"github.com/nstalter/fretboard-visualizer/music"
)

type Fretboard struct {
	X, Y, W, H     float64
	CurrentChord   *music.GuitarChordVoicing
	numFrets       int
	numStrings     int
	fretWidth      float64
	stringSpace    float64
	startX, startY float64
}

func NewFretboard(x, y, w, h float64) *Fretboard {
	return &Fretboard{
		X:           x,
		Y:           y,
		W:           w,
		H:           h,
		numFrets:    22,
		numStrings:  6,
		fretWidth:   65,
		stringSpace: 50,
		startX:      x + 100,
		startY:      y + 150,
	}
}

func (f *Fretboard) Draw(dc *gg.Context) {
	f.drawFrets(dc)
	f.drawStrings(dc)
	f.drawFretMarkers(dc)
	f.drawLabels(dc)
	if f.CurrentChord != nil {
		f.drawChord(dc)
	}
}

func (f *Fretboard) HandleClick(x, y float64) bool {
	// For now, fretboard doesn't handle clicks
	// Could be extended for future features like clicking notes
	return false
}

func (f *Fretboard) drawFrets(dc *gg.Context) {
	dc.SetColor(color.RGBA{139, 69, 19, 255}) // Brown color for frets
	dc.SetLineWidth(10)

	for i := 0; i <= f.numFrets; i++ {
		x := float64(f.startX + float64(i)*f.fretWidth)
		y1 := float64(f.startY)
		y2 := float64(f.startY + float64(f.numStrings-1)*f.stringSpace)
		dc.DrawLine(x, y1, x, y2)
		dc.Stroke()
	}
}

func (f *Fretboard) drawStrings(dc *gg.Context) {
	dc.SetColor(color.Black)
	dc.SetLineWidth(5)
	for i := 0; i < f.numStrings; i++ {
		y := float64(f.startY + float64(i)*f.stringSpace)
		x1 := float64(f.startX)
		x2 := float64(f.startX + float64(f.numFrets)*f.fretWidth)

		dc.SetLineWidth(1.0 + float64(i)*0.3)
		dc.DrawLine(x1, y, x2, y)
		dc.Stroke()
	}
}

func (f *Fretboard) drawFretMarkers(dc *gg.Context) {
	dc.SetColor(color.RGBA{180, 180, 180, 255})

	singleDots := []int{3, 5, 7, 9, 15, 17, 19, 21}
	doubleDots := []int{12}
	centerY := float64(f.startY + float64(f.numStrings-1)*f.stringSpace/2)

	for _, fret := range singleDots {
		x := float64(f.startX + float64(fret)*f.fretWidth - f.fretWidth/2)
		dc.DrawCircle(x, centerY, 8)
		dc.Fill()
	}

	for _, fret := range doubleDots {
		x := float64(f.startX + float64(fret)*f.fretWidth - f.fretWidth/2)
		dc.DrawCircle(x, centerY-20, 8)
		dc.Fill()
		dc.DrawCircle(x, centerY+20, 8)
		dc.Fill()
	}
}

func (f *Fretboard) drawLabels(dc *gg.Context) {
	dc.SetColor(color.Black)

	stringNames := []string{"e", "B", "G", "D", "A", "E"}
	for i, name := range stringNames {
		y := float64(f.startY + float64(i)*f.stringSpace)
		dc.DrawStringAnchored(name, float64(f.startX-30), y, 0.5, 0.5)
	}

	for i := 1; i <= f.numFrets; i++ {
		x := float64(f.startX + float64(i)*f.fretWidth - f.fretWidth/2)
		y := float64(f.startY - 20)
		dc.DrawStringAnchored(fmt.Sprintf("%d", i), x, y, 0.5, 0.5)
	}
}

func (f *Fretboard) drawBarre(dc *gg.Context, barre *music.Barre) {
	x := float64(f.startX + float64(barre.Fret)*f.fretWidth - f.fretWidth/2)
	y1 := f.startY + float64(barre.StartString)*f.stringSpace
	y2 := f.startY + float64(barre.EndString)*f.stringSpace

	dc.SetColor(color.RGBA{0, 120, 255, 255})
	dc.DrawRoundedRectangle(x-10, y1-8, 20, y2-y1+16, 10)
	dc.Fill()
}

func (f *Fretboard) drawChord(dc *gg.Context) {
	if f.CurrentChord.Barre != nil {
		f.drawBarre(dc, f.CurrentChord.Barre)
	}
	playedStrings := f.CurrentChord.CalculateNotes(music.StandardTuning())

	for stringNum, fret := range f.CurrentChord.Fingering {
		y := float64(f.startY + float64(stringNum)*f.stringSpace)

		// Check if this note is a root note (same pitch as chord root)
		isRootNote := false
		if fret >= 0 { // Only check played/open strings
			note := playedStrings[stringNum].GetNote()
			isRootNote = (note.SemitoneValue() == f.CurrentChord.Chord.Root.SemitoneValue())
		}

		if fret == -1 {
			// Draw X for muted string
			if isRootNote {
				dc.SetColor(color.RGBA{255, 100, 100, 255}) // Lighter red for root
			} else {
				dc.SetColor(color.RGBA{255, 0, 0, 255})
			}
			dc.DrawStringAnchored("X", float64(f.startX-60), y, 0.5, 0.5)
		} else if fret == 0 {
			// Draw O for open string
			if isRootNote {
				dc.SetColor(color.RGBA{100, 255, 100, 255}) // Lighter green for root
			} else {
				dc.SetColor(color.RGBA{0, 200, 0, 255})
			}
			dc.DrawCircle(float64(f.startX-60), y, 10)
			dc.Stroke()
		} else {
			x := float64(f.startX + float64(fret)*f.fretWidth - f.fretWidth/2)
			if f.CurrentChord.Barre != nil && fret == f.CurrentChord.Barre.Fret &&
				stringNum >= f.CurrentChord.Barre.StartString && stringNum <= f.CurrentChord.Barre.EndString {
				// Barre note - highlight if it's a root note
				if isRootNote {
					// Root note in barre - gold background with black text
					dc.SetColor(color.RGBA{255, 215, 0, 255}) // Gold
					dc.DrawCircle(x, y, 18)                   // Slightly larger
					dc.Fill()
					dc.SetColor(color.Black) // Black text for contrast
				} else {
					// Regular barre note - white text
					dc.SetColor(color.White)
				}
				dc.DrawStringAnchored(playedStrings[stringNum].String(), x, y, 0.5, 0.5)
				continue
			}

			// Regular fretted note
			if isRootNote {
				// Root note - gold color
				dc.SetColor(color.RGBA{255, 215, 0, 255}) // Gold
				dc.DrawCircle(x, y, 18)                   // Slightly larger
				dc.Fill()
				dc.SetColor(color.Black) // Black text for contrast
			} else {
				// Regular note - blue
				dc.SetColor(color.RGBA{0, 120, 255, 255})
				dc.DrawCircle(x, y, 15)
				dc.Fill()
				dc.SetColor(color.White)
			}
			dc.DrawStringAnchored(playedStrings[stringNum].String(), x, y, 0.5, 0.5)
		}
	}
}

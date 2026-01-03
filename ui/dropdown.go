package ui

import (
	"image/color"

	"github.com/fogleman/gg"
)

type Dropdown struct {
	X, Y, W, H    float64
	Label         string
	Options       []string
	SelectedIndex int
	IsOpen        bool
}

func (d *Dropdown) GetSelectedValue() string {
	if d.SelectedIndex >= 0 && d.SelectedIndex < len(d.Options) {
		return d.Options[d.SelectedIndex]
	}
	return ""
}

func (d *Dropdown) HandleClick(x, y float64) bool {
	// Check if click is on the main button
	if x >= d.X && x <= d.X+d.W && y >= d.Y && y <= d.Y+d.H {
		d.IsOpen = !d.IsOpen
		return true
	}

	if d.IsOpen {
		for i := range d.Options {
			optionY := d.Y + d.H + float64(i)*d.H
			if x >= d.X && x <= d.X+d.W && y >= optionY && y <= optionY+d.H {
				d.SelectedIndex = i
				d.IsOpen = false
				return true
			}
		}
		d.IsOpen = false
		return true
	}

	return false
}

func (d *Dropdown) Draw(dc *gg.Context) {
	// Draw main button with solid background
	dc.SetColor(color.RGBA{200, 200, 200, 255})
	dc.DrawRectangle(d.X, d.Y, d.W, d.H)
	dc.Fill()

	// Draw main button highlight if open
	if d.IsOpen {
		dc.SetColor(color.RGBA{150, 180, 255, 100})
		dc.DrawRectangle(d.X, d.Y, d.W, d.H)
		dc.Fill()
	}

	// Draw border
	dc.SetColor(color.Black)
	dc.SetLineWidth(2)
	dc.DrawRectangle(d.X, d.Y, d.W, d.H)
	dc.Stroke()

	// Draw selected value
	selectedText := d.GetSelectedValue()
	if selectedText == "" && len(d.Options) > 0 {
		selectedText = d.Options[0]
	}
	dc.SetColor(color.Black)
	dc.DrawStringAnchored(selectedText, d.X+d.W/2, d.Y+d.H/2, 0.5, 0.5)

	// Draw dropdown arrow
	arrowX := d.X + d.W - 15
	arrowY := d.Y + d.H/2
	dc.DrawStringAnchored("▼", arrowX, arrowY, 0.5, 0.5)

	// Draw dropdown options if open
	if d.IsOpen {
		for i, option := range d.Options {
			optionY := d.Y + d.H + float64(i)*d.H

			// Draw solid background for each option
			dc.SetColor(color.RGBA{240, 240, 240, 255})
			dc.DrawRectangle(d.X, optionY, d.W, d.H)
			dc.Fill()

			// Highlight selected option
			if i == d.SelectedIndex {
				dc.SetColor(color.RGBA{100, 150, 255, 150})
				dc.DrawRectangle(d.X, optionY, d.W, d.H)
				dc.Fill()
			}

			// Draw border
			dc.SetColor(color.Black)
			dc.SetLineWidth(1)
			dc.DrawRectangle(d.X, optionY, d.W, d.H)
			dc.Stroke()

			// Draw option text
			dc.SetColor(color.Black)
			dc.DrawStringAnchored(option, d.X+d.W/2, optionY+d.H/2, 0.5, 0.5)
		}
	}
}

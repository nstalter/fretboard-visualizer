package ui

import (
	"image/color"

	"github.com/fogleman/gg"
)

type Button struct {
	X, Y, W, H float64
	Label      string
}

func (b *Button) Draw(dc *gg.Context, isSelected bool) {
	if isSelected {
		dc.SetColor(color.RGBA{100, 150, 255, 255})
	} else {
		dc.SetColor(color.RGBA{200, 200, 200, 255})
	}

	dc.DrawRectangle(b.X, b.Y, b.W, b.H)
	dc.Fill()

	dc.SetColor(color.Black)
	dc.SetLineWidth(2)
	dc.DrawRectangle(b.X, b.Y, b.W, b.H)
	dc.Stroke()

	dc.DrawStringAnchored(b.Label, b.X+b.W/2, b.Y+b.H/2, 0.5, 0.5)
}

func (b *Button) HandleClick(x, y float64) bool {
	return x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H
}

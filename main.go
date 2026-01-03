package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ebiten.SetWindowSize(1600, 550)
	ebiten.SetWindowTitle("Fretboard Visualizer")
	app := NewFretboardApp()
	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}

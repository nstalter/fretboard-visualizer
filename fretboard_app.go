package main

import (
	"image/color"

	"github.com/fogleman/gg"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nstalter/fretboard-visualizer/music"
	"github.com/nstalter/fretboard-visualizer/ui"
)

const (
	windowWidth   = 1600
	windowHeight  = 550
	controlHeight = 40
)

type FretboardApp struct {
	image             *ebiten.Image
	currentChord      music.GuitarChordVoicing
	selectedKey       music.Note
	selectedQuality   music.ChordQuality
	availableVoicings []music.GuitarChordVoicing
	currentVoicingIdx int
	keyDropdown       *ui.Dropdown
	qualityDropdown   *ui.Dropdown
	fretboard         *ui.Fretboard
	nextButton        ui.Button
	prevButton        ui.Button
	lastMousePressed  bool
}

func NewFretboardApp() *FretboardApp {
	// Create dropdowns for key and quality selection
	keyDropdown := &ui.Dropdown{
		X:             50,
		Y:             20,
		W:             120,
		H:             controlHeight,
		Label:         "Key",
		Options:       []string{"C", "C#/Db", "D", "D#/Eb", "E", "F", "F#/Gb", "G", "G#/Ab", "A", "A#/Bb", "B"},
		SelectedIndex: 0,
		IsOpen:        false,
	}

	qualities := music.AllChordQualities()
	qualityOptions := make([]string, len(qualities))
	for i, q := range qualities {
		qualityOptions[i] = q.Name
	}

	qualityDropdown := &ui.Dropdown{
		X:             190,
		Y:             20,
		W:             100,
		H:             controlHeight,
		Label:         "Quality",
		Options:       qualityOptions,
		SelectedIndex: 0,
		IsOpen:        false,
	}

	fretboard := ui.NewFretboard(0, 0, windowWidth, windowHeight)

	nextButton := ui.Button{
		X:     310,
		Y:     20,
		W:     80,
		H:     controlHeight,
		Label: "Next →",
	}

	prevButton := ui.Button{
		X:     410,
		Y:     20,
		W:     80,
		H:     controlHeight,
		Label: "← Prev",
	}

	selectedKey := music.Note{Name: music.C, Accidental: music.Natural}
	selectedQuality := music.Major
	chord := music.Chord{Root: selectedKey, Quality: selectedQuality}

	generator := music.NewGuitarVoicingGenerator(music.StandardTuning(), 22)
	voicings := generator.GenerateVoicings(chord)

	var currentChord music.GuitarChordVoicing
	if len(voicings) > 0 {
		currentChord = voicings[0]
	} else {
		// Fallback to a simple C Major chord
		currentChord = music.GuitarChordVoicing{
			Fingering: [6]int{0, 1, 0, 2, 3, -1},
		}
	}

	app := &FretboardApp{
		selectedKey:       selectedKey,
		selectedQuality:   selectedQuality,
		availableVoicings: voicings,
		currentVoicingIdx: 0,
		currentChord:      currentChord,
		keyDropdown:       keyDropdown,
		qualityDropdown:   qualityDropdown,
		fretboard:         fretboard,
		nextButton:        nextButton,
		prevButton:        prevButton,
		lastMousePressed:  false,
	}
	app.image = render(app)
	return app
}

func (a *FretboardApp) Update() error {
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	// Only handle click on mouse button press (not continuous)
	if mousePressed && !a.lastMousePressed {
		x, y := ebiten.CursorPosition()
		fx, fy := float64(x), float64(y)

		// Handle dropdown clicks
		keyChanged := a.keyDropdown.HandleClick(fx, fy)
		qualityChanged := a.qualityDropdown.HandleClick(fx, fy)

		if keyChanged || qualityChanged {
			if !a.keyDropdown.IsOpen && !a.qualityDropdown.IsOpen {
				a.updateChordSelection()
			}
			a.image = render(a)
		}

		if a.nextButton.HandleClick(fx, fy) {
			a.nextVoicing()
			a.image = render(a)
		}

		if a.prevButton.HandleClick(fx, fy) {
			a.prevVoicing()
			a.image = render(a)
		}
	}

	a.lastMousePressed = mousePressed
	return nil
}

func (a *FretboardApp) Draw(screen *ebiten.Image) {
	screen.DrawImage(a.image, nil)
}

func (a *FretboardApp) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return windowWidth, windowHeight
}

func (a *FretboardApp) updateChordSelection() {
	keyStr := a.keyDropdown.GetSelectedValue()
	note := parseNoteFromString(keyStr)
	a.selectedKey = note

	qualityStr := a.qualityDropdown.GetSelectedValue()
	a.selectedQuality, _ = music.ParseChordQuality(qualityStr)

	chord := music.Chord{Root: a.selectedKey, Quality: a.selectedQuality}
	generator := music.NewGuitarVoicingGenerator(music.StandardTuning(), 22)
	voicings := generator.GenerateVoicings(chord)

	if len(voicings) == 0 {
		return
	}

	a.availableVoicings = voicings

	minMovement := -1.0
	bestIdx := 0

	for i, voicing := range voicings {
		movement := a.currentChord.CalculateFingerMovement(voicing)
		if minMovement < 0 || movement < minMovement {
			minMovement = movement
			bestIdx = i
		}
	}

	a.currentVoicingIdx = bestIdx
	a.currentChord = voicings[bestIdx]
}

func (a *FretboardApp) nextVoicing() {
	if len(a.availableVoicings) == 0 {
		return
	}

	a.currentVoicingIdx = (a.currentVoicingIdx + 1) % len(a.availableVoicings)
	a.currentChord = a.availableVoicings[a.currentVoicingIdx]
}

func (a *FretboardApp) prevVoicing() {
	if len(a.availableVoicings) == 0 {
		return
	}

	a.currentVoicingIdx--
	if a.currentVoicingIdx < 0 {
		a.currentVoicingIdx = len(a.availableVoicings) - 1
	}
	a.currentChord = a.availableVoicings[a.currentVoicingIdx]
}

func parseNoteFromString(s string) music.Note {
	noteMap := map[string]music.Note{
		"C":     {Name: music.C, Accidental: music.Natural},
		"C#/Db": {Name: music.C, Accidental: music.Sharp},
		"D":     {Name: music.D, Accidental: music.Natural},
		"D#/Eb": {Name: music.D, Accidental: music.Sharp},
		"E":     {Name: music.E, Accidental: music.Natural},
		"F":     {Name: music.F, Accidental: music.Natural},
		"F#/Gb": {Name: music.F, Accidental: music.Sharp},
		"G":     {Name: music.G, Accidental: music.Natural},
		"G#/Ab": {Name: music.G, Accidental: music.Sharp},
		"A":     {Name: music.A, Accidental: music.Natural},
		"A#/Bb": {Name: music.A, Accidental: music.Sharp},
		"B":     {Name: music.B, Accidental: music.Natural},
	}
	return noteMap[s]
}

func render(a *FretboardApp) *ebiten.Image {
	dc := gg.NewContext(windowWidth, windowHeight)

	dc.SetColor(color.White)
	dc.Clear()

	a.fretboard.CurrentChord = &a.currentChord
	a.fretboard.Draw(dc)

	a.keyDropdown.Draw(dc)
	a.qualityDropdown.Draw(dc)
	a.nextButton.Draw(dc, false)
	a.prevButton.Draw(dc, false)

	return ebiten.NewImageFromImage(dc.Image())
}

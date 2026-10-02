package main

import (
	"net/http"

	"github.com/nstalter/fretboard-visualizer/guitar"
)

type identifiedJSON struct {
	chordJSON
	Inversion string      `json:"inversion"`
	Voicing   voicingJSON `json:"voicing"`
}

type identifyResponse struct {
	Fingering string `json:"fingering"`
	Notes     int    `json:"notes"`
	// Chords are the names the notes fit, best first. It is empty until the notes spell a known chord.
	Chords   []identifiedJSON `json:"chords"`
	Playable bool             `json:"playable"`
	// Issue says why the shape is not playable or not a chord yet; it is empty when it is both.
	Issue string `json:"issue"`
}

// identify names the chord a fingering plays and says whether it can be played.
// A shape that no further notes could make playable is an error (limit); a shape that is merely
// unfinished is not.
func identify(f [6]int, tuning guitar.Tuning, openMax int) (resp identifyResponse, limit string) {
	if limit = guitar.BuildIssue(f, openMax); limit != "" {
		return resp, limit
	}
	resp = identifyResponse{Fingering: guitar.FormatFingering(f), Chords: []identifiedJSON{}}
	for _, fret := range f {
		if fret >= 0 {
			resp.Notes++
		}
	}
	resp.Issue = guitar.PlayIssue(f, openMax)
	resp.Playable = resp.Issue == ""
	matches := guitar.IdentifyChord(f, tuning)
	for _, m := range matches {
		resp.Chords = append(resp.Chords, identifiedJSON{
			chordJSON: newChordJSON(m.Chord),
			Inversion: m.Inversion.String(),
			Voicing:   newVoicingJSON(guitar.NewVoicing(m.Chord, f), tuning),
		})
	}
	if len(matches) == 0 && resp.Issue == "" {
		resp.Issue = "those notes don't spell a chord this app knows"
	}
	return resp, ""
}

func handleIdentify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("fingering") == "" {
		writeError(w, paramError("fingering", errRequired))
		return
	}
	f, err := guitar.ParseFingering(q.Get("fingering"))
	if err != nil {
		writeError(w, paramError("fingering", err))
		return
	}
	tuning, err := parseTuningParam(q.Get("tuning"))
	if err != nil {
		writeError(w, err)
		return
	}
	openMax, err := parseOpenMax(q.Get("openMax"))
	if err != nil {
		writeError(w, err)
		return
	}
	resp, limit := identify(f, tuning, openMax)
	if limit != "" {
		writeStatusError(w, http.StatusUnprocessableEntity, limit)
		return
	}
	writeJSON(w, resp)
}

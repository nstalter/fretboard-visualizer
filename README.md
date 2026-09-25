# Fretboard Visualizer

A web app that shows a guitar fretboard and visualizes chord shapes in any key, all along the neck, in any tuning. Build chord progressions from a key and mode, and play them back.

The app generates playable chord positions by rule (fret span, finger count, muting). Not every generated shape will feel comfortable in practice.

**Try it live:** <https://macmini.tail9292f9.ts.net/> (served from a Mac mini over a Tailscale tunnel, so it is only up while that machine is running)

## Features

- 23 chord qualities, from triads to 13ths, with inversions by lowest sounding pitch
- Nearest-voicing navigation: changing chord jumps to the shape with the least finger movement
- Hover any fret (or open string) to see its note name, and click it to jump to a shape that plays that spot
- An open-string limit: shapes with open strings stay at or below a fret you choose (default 5)
- A toggle that shows every chord tone across the whole neck
- Any 6-string tuning within ±5 semitones of standard, with presets (Drop D, DADGAD, Open G, …)
- Diatonic chords for any key and mode, with Roman numerals
- Progressions with fixed shapes, reordering, and looped playback

## Running

Requires Go 1.25 or later (see `go.mod`). There are no other dependencies.

```bash
git clone https://github.com/nstalter/fretboard-visualizer.git
cd fretboard-visualizer
go run .
```

Then open <http://localhost:8080>.

The files in `web/` are embedded in the binary, so **restart the server after editing them**.

## Testing

```bash
go vet ./... && go test ./...
```

## Layout

- `music/`: instrument-independent theory: notes, chord spelling, inversions and diatonic chords (standard library only)
- `guitar/`: tunings, fingerings, voicing generation, playability and nearest voicing; depends on `music/`
- `api.go`: the stateless JSON API (`/api/qualities`, `/api/voicings`, `/api/diatonic`, `/api/progression`)
- `web/`: plain HTML, CSS and ES modules, with the fretboard drawn in SVG
- `docs/code-tour.html`: a guided tour of the code and the Go idioms it uses (open it in a browser)

## License

MIT License

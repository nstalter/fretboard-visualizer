# Fretboard Visualizer

## Why

A web app that shows a guitar fretboard and visualizes chord shapes in any key, all along the neck. It's also a way to learn Go.

A local Ebiten GUI already exists (Major, Minor and Major 7 only, standard tuning). The goals are to move it to the web, make it prettier, and add the features below.

## Architecture

- **Backend:** a Go `net/http` server. It is **stateless** and holds all the music logic: chord spelling, voicing generation, playability, nearest-voicing selection and diatonic chords.
- **Frontend:** plain HTML, CSS and JS (ES modules, no build step). The fretboard is drawn in SVG. The static files are embedded in the Go binary with `embed`.
- **Client state:** the tuning, the explorer selection and the progression steps are held in browser memory only. Nothing is saved.
- **Running:** `go run .` serves on localhost. Public hosting comes later.
- Ebiten is retired.

## Chords

### Qualities

| Group    | Qualities                              |
|----------|----------------------------------------|
| Triads   | maj, min, dim, aug, sus2, sus4         |
| Sixths   | 6, m6                                  |
| Sevenths | 7, maj7, m7, m7♭5, dim7, 7sus4         |
| Ninths   | add9, 9, maj9, m9                      |
| Elevenths| 11, m11                                |
| Thirteenths | 13, maj13, m13                      |

### Tones that may be omitted

A voicing must contain every chord tone except the ones listed as optional.

| Chord family | Optional tones                                         |
|--------------|--------------------------------------------------------|
| Triads, 6ths, add9 | none                                             |
| 7ths, 9ths   | 5th (a perfect 5th only: the ♭5 of m7♭5 and dim7 is required) |
| 11ths        | 5th, 9th. The dominant 11 may also drop the 3rd        |
| 13ths        | 5th, 9th, 11th                                         |

Voicings without the root are out of scope.

### Inversion

The inversion is set by the lowest sounding **pitch** (not the lowest string, since strings can cross in pitch): **Root / 3rd / 5th / 7th / Any**, with Any as the default.
- "3rd" is offered only for chords with a 3rd (not sus chords). "7th" is offered only for chords with a 7th.
- Extensions (9th, 11th, 13th) are never offered as a specific bass-note option.
- **Any** applies no filter, so any chord tone may be the lowest note, extensions included.

### Playability

Keep the current rules:
- At least 3 strings played.
- A fret span of 3 or less.
- Non-barre shapes: at most 4 fretted fingers, and only low strings may be muted.
- Barre shapes: the barre plus at most 3 fingers, all played strings next to each other, and no fretted note below the barre.

Also required:
- **Open strings:** a shape with an open string may not use a fretted note above fret N. N defaults to 5 and can be changed; 22 turns the limit off.
- Every voicing contains all of its required tones (see above).
- Barre shapes come out of the generator on their own. They are not special-cased.

**Tuning awareness:** `hasRootNote` hardcodes `StandardTuning()`, but nothing calls it, so it is deleted. The generator must always use the active tuning, and tests in non-standard tunings guard this.

## Tuning

- Each string has ▲/▼ controls that move it one semitone. Each string is limited to ±5 semitones from standard.
- Presets: Standard, Drop D, Half-step down, DADGAD, Open G, Open D.
- Always 6 strings.

## Chord explorer

- Choose the root, quality and inversion. Prev/next buttons step through that chord's shapes, which are sorted by fret.
- **Nearest voicing:** any change (root, quality, inversion or tuning) jumps to the shape with the **least finger movement** from the current shape (`CalculateFingerMovement`).
  - Movement is the frets moved on strings played in both shapes, **plus 2 for each string that is added or dropped** (muted ↔ played).
  - Ties go to the closest average fret position, then the lower fret.
  - If there is no current shape, the lowest-fret shape is shown.

### Fretboard display

- Shows **one voicing at a time**.
- 22 frets with inlay markers.
- X/O at the nut for muted/open strings.
- A barre is drawn as a bar.
- Dots are labelled with **note names by default**, with a toggle to show intervals (R, 3, 5, ♭7, …).
- Root dots use a distinct colour.
- **Hovering a spot** shows its note name over it: the chord's own spelling for a chord tone, otherwise both spellings ("C♯/D♭").
- **Clicking a spot** (a string at a fret, or an open string) keeps the chord and jumps to a shape that plays that spot, the one with the least finger movement from the current shape. If no shape plays it, the shape whose fret range is nearest the click is shown. If a progression step is loaded, its shape updates like prev/next.
- A toggle shows **every chord tone across the whole neck**, faintly, under the current shape. It also shows when no shape is playable.

## Progression

### Adding chords

A toggle switches between two ways of choosing the next chord:
- **Key + mode:** choose a key and a mode (Ionian through Locrian), then a degree 1–7. The quality starts on **"Diatonic"**, which means the diatonic triad, and can be overridden with any quality.
  - Roman numerals are measured against the major scale: D Dorian is i ii ♭III IV v vi° ♭VII.
  - Keys that need double sharps or flats (e.g. G♯ Ionian's vii° = F♯♯dim) are shown as spelled.
- **Key + type:** choose a root and a quality directly.

**"Add to progression"** adds the explorer's current chord, inversion **and shape** as a step, and that shape stays fixed. Because the explorer always jumps to the nearest voicing, the next chord you choose starts near the one you just added.

Each step stores its resolved chord (e.g. Em7). Changing the key or mode later does **not** change existing steps.

### Step list

- Ordered. The same chord can appear more than once. Steps can be reordered and deleted, and deleting one leaves the others as they are.
- Steps are shown as chord names, with the Roman numeral when added via key + mode.
- **Clicking a step** loads it into the explorer. Browsing shapes there updates that step's fixed shape.
- **When the tuning or the open-string limit changes**, every step's shape is recalculated in order. Each shape is the one nearest the previous step's shape, and the first step gets the lowest-fret shape.

### Playback

- The progression loops through the steps and wraps around. Timing is **BPM × beats per chord**, the same for every chord.
- Controls: play/pause, previous/next, and clicking a step to jump to it.
- Playback drives the explorer. Touching any explorer or progression control pauses playback.

## API

All endpoints are stateless. Tuning is sent as six note names, low to high: `tuning=E2,A2,D3,G3,B3,E4`. Fingerings are six frets, low to high, with `x` for muted: `x32010`. When any fret is 10 or higher, the six frets are separated by dashes: `x-10-12-12-11-10`.

- `GET /api/qualities`
  Returns the chord qualities and the inversions each one offers, so the UI can build its dropdowns.

- `GET /api/voicings?root=&quality=&inversion=&tuning=&openMax=&near=&at=`
  Returns the playable voicings sorted by fret, and `fretboard`: for every string (low to high) and fret 0–22, the index of the chord tone sounded there, or -1.
  - `openMax` (0–22, default 5) is the highest fret a shape with an open string may use.
  - When `near` (a fingering) is given, it also returns `nearestIndex`.
  - `at=string:fret` (string 0–5 low to high) picks the shape for a clicked spot: `nearestIndex`, plus `atMatched` saying whether that shape plays the spot.
- `GET /api/diatonic?key=&mode=`
  Returns the 7 diatonic chords (root, triad quality, Roman numeral).
- `POST /api/progression`
  Request: the tuning, an optional `openMax`, and the steps (root, quality, inversion).
  Response: a shape for each step, chosen as a chain where each shape is nearest the previous one. Used to recalculate shapes after a tuning or open-string limit change.

## Testing / done criteria

- **Feature tests (required):** API scenario tests using `httptest`, one or more per feature. Examples:
  - C major in standard tuning includes `x32010`.
  - F major `near=x32010` returns `x33211` as nearest (the F barre with the low string still muted, as in the C shape).
  - Drop D tuning changes the voicings returned for D major.
  - A C9 voicing always contains the root, 3rd, ♭7 and 9th.
  - `/api/diatonic?key=D&mode=dorian` returns Dm, Em, F, G, Am, Bdim, C.
  - `/api/progression` returns one shape per step, each nearest the previous.
- **Unit tests** where the logic is tricky: chord spelling for every quality, required-tone checks, nearest-voicing selection, and non-standard tuning tests (the generator uses the active tuning).
- Browser tests come later, once the UI settles.

## Out of scope (for now)

Audio playback, saving or sharing progressions, accounts, left-handed mode, transposing a progression, slash chords (non-chord bass notes), rootless voicings, and public hosting.

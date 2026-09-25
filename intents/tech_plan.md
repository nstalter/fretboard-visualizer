# Tech plan: Fretboard Visualizer on the web

This is **how** to build what `intents/intent.md` describes. The approach is vertical slices:
1. Port the current app to a stateless Go server with an SVG page.
2. Add one tested feature at a time.

Existing `music` code stays where it is sound and changes only where the spec requires it (CLAUDE.md §3).

Decisions settled with you are listed in §9.

---

## 1. What the current code does (measured)

Four subagents read every file, ran the tests, and built throwaway prototypes in the scratchpad (not in the repo). A critic then checked the findings against the code. These results shape the plan:

| # | Finding | Evidence | Consequence |
|---|---|---|---|
| F1 | **Only the root and the highest interval are checked.** Other chord tones are never required. For C major, 14 of 97 voicings have no 3rd (`335xxx`). For C9, 201 of 383 have no ♭7. | `MatchesChord`, `guitar_voicing.go:100-115` | Replace it with a required-tone check (M2). |
| F2 | **The generator misses common shapes.** Open A `x02220` and Em `322003` are never produced, and neither is anything that mixes open strings with frets above 6. | anchor/window logic, `generator.go:186-215` | Rewrite the search (M3). |
| F3 | **Generation is too slow for big chords.** C takes 38 ms, C9 0.6 s, C13 about 7 s. Cost grows roughly as n⁶ in the number of chord-tone positions. | `generateCombinations`, `generator.go:271-299` | The rewrite handles C13 in about 2 ms (prototype). |
| F4 | **Voicing order changes from run to run.** The input is built by map iteration and then sorted, unstably, on the lowest fret only. | `generator.go:38,50-52` | Deterministic sort (M3). |
| F5 | **The "`hasRootNote` bug" is in dead code.** Nothing calls the function. The live check already uses the active tuning. The code that really hardcodes standard tuning is in the Ebiten files being deleted. | `generator.go:57-81`; `fretboard_app.go:85,166`; `ui/fretboard.go:131` | Delete it. Guard the requirement with tests in non-standard tunings. |
| F6 | **Notes have no working octave**, so the lowest-sounding note can't be found. Strings can cross in pitch even in standard tuning (`x-x-17-17-17-0`: the open e at E4 sounds below the D string's fret 17 at G4). | `CalculateNote`, `tuning.go:20-30` | The bass note is found by MIDI pitch (M4). |
| F7 | **Spelling is sharps-only.** B♭ comes out as A♯ D F. Cb has `SemitoneValue() == -1`, so C♭ chords have 0 voicings. | `tuning.go:32-48`, `note.go:55` | Spell from the chord's degrees (M2). |
| F8 | **The "barre + at most 3 fingers" rule is never applied.** Its branch is dead code, so `879987` is accepted. | `guitar_voicing.go:146-148,230-239` | Wire it in and keep the existing checks too (M3). |
| F9 | **Muted↔played changes cost nothing** in `CalculateFingerMovement`. Because of that, the spec's F-major example can't pass (§3.8). | `guitar_voicing.go:265-279` | Add a cost of 2 per added or dropped string (D10). |
| F10 | **Internal string order is high→low** (`Fingering[0]` is high e). The API is low→high. | `tuning.go:10-15` | Convert only at the edges (D1). |
| F11 | **Frets 10–22 can't be written in the `x32010` format.** | – | Dash-separated form (D2). |
| F12 | **Nothing in `go.mod` survives once Ebiten and `ui/` go.** After `go mod tidy` it has no `require` lines. | verified in a copy | M1 |
| F13 | **Repo state.** `music/chord.go` has uncommitted edits (14 qualities added). The 10 MB `fretboard-visualizer` binary is tracked and not gitignored. | `git status` | Untrack the binary in M1. The WIP is committed along with M1/M2 work. |

---

## 2. Decisions

| # | Decision | Why |
|---|---|---|
| D1 | **Internal string order stays high→low.** Only three places convert, each with round-trip tests: `ParseFingering`/`FormatFingering` (`music/fingering.go`), `ParseTuning` (`music/tuning.go`), and the API's `frets`/`tones`/`barre` builders in `api.go`. | `hasOnlyLowStringsMuted`, `lowestPlayedStringIndex`, `detectBarreForVoicing` and the 13-case `IsPlayable` test table stay unchanged. |
| D2 | **Fingering strings:** 6 characters when every fret is ≤ 9 (`x32010`), otherwise 6 dash-separated tokens (`x-10-12-12-11-10`). The parser accepts both. | Every spec example stays valid, and frets up to 22 can be written. |
| D3 | **Accidentals are a signed count with no limit** (−1 ♭, +1 ♯, ±2 double, …). `String()` repeats ♯/♭. `ParseNote` accepts `#`, `b`, `♯` and `♭` in any number. The API sends and receives this one form. | Correct spelling (C dim7 = C E♭ G♭ B♭♭, and G♯ major's vii° = F♯♯dim), with no separate ASCII and display forms to keep in sync. |
| D4 | **The pitch model is MIDI:** `Note.MIDI() = (Octave+1)*12 + natural + accidental` (E2 = 40, B♯3 = 60, C♭4 = 59). The bass note is the lowest MIDI pitch among the played strings. | Handles pitch crossings (F6) and fixes the B♯/C♭ wrap. |
| D5 | **Chord qualities are a data table.** Each quality has an id, a label and a suffix, plus its tones, each with a degree, a size in semitones and an optional flag. Spelling, interval labels, required tones and the offered inversions all come from this table. | One source of truth. |
| D6 | **Barre playability:** add `frettedStringCountPlayable(f, barreFret)` and **keep** `stringSpan ≤ 4`. | The spec says "keep the current rules"; this adds the one rule it lists that the code never applied. |
| D7 | **Sort order:** lowest fret, then average fret, then the fingering compared low→high (muted < open < fret). | Prev/next and `nearestIndex` become reproducible. (Adding a "more strings first" key made D major's first shape `x50232`.) |
| D8 | **Inversions offered:** Any and Root always. 3rd, 5th or 7th only when the quality has a tone of that degree. Sus chords get no 3rd, and 6/add9 get no 7th. The dominant 11 does offer 3rd, because its 3rd is optional but allowed. | Follows the spec's rule for 7th. |
| D9 | **"Any" applies no bass filter:** any chord tone may be lowest, extensions included. Root/3rd/5th/7th each require that exact degree in the bass. | Your call (Q2). |
| D10 | **Nearest-voicing metric:** \|Δfret\| on strings played in both shapes, plus **2** per string that changes between muted and played. Ties go to the closer average fret, compared exactly, then the lower fret, then the earlier list index. | See §3.8. |
| D11 | **One extra endpoint, `GET /api/qualities`** (ids, labels and allowed inversions). | The client has to know which inversions to offer, and the spec keeps music logic on the server. |
| D12 | **`/api/progression` returns only fingering strings.** The client fetches `/api/voicings` whenever it shows a step. | Smallest response. Generating one chord takes a few milliseconds, so the extra requests cost little. |
| D13 | **Browsing shapes while a step is loaded updates that step's shape.** Changing the root, quality or inversion deselects the step and leaves it unchanged. | The spec only says browsing updates the step. |
| D14 | **What pauses playback:** every control except play/pause, previous/next step, clicking a step, BPM and beats per chord. So explorer prev/next, the label toggle and tuning changes all pause. | The spec says "touching any explorer or progression control pauses playback". Tuning has its own panel, so whether it pauses is a judgement call: it pauses because it changes what the explorer shows. |
| D15 | **Roman numerals are measured against the major scale.** D Dorian gives i ii ♭III IV v vi° ♭VII. When a step's quality is overridden, it shows the degree's numeral next to the name (`G7 · V`). | This is the usual convention in modal harmony. |
| D16 | **Tuning presets live in JS** as UI data. The server validates every tuning it receives, so an out-of-range preset would be rejected. | Presets are data, not logic. |

---

## 3. Music domain (`music/`, standard library only)

### 3.1 Notes (`note.go`)

```go
type NoteName int      // C..B = 0..6 (unchanged)
type Accidental int    // CHANGED: signed semitone count; Flat=-1, Natural=0, Sharp=1
type Note struct { Name NoteName; Accidental Accidental; Octave int }  // unchanged

func ParseNote(s string) (Note, error)   // "C", "Bb", "B♭", "F##", "Cb"; letter A–G upper-case
func ParsePitch(s string) (Note, error)  // ParseNote + octave: "E2", "Eb2", "F♯3"
func (n Note) String() string            // "B♭", "F♯♯"; no octave
func (n Note) SemitoneValue() int        // 0..11, wraps correctly (Cb = 11, B# = 0)
func (n Note) MIDI() int                 // (Octave+1)*12 + natural[Name] + Accidental
```

### 3.2 Tuning (`tuning.go`)

```go
type Tuning struct { Strings [6]Note }     // unchanged; high→low (D1); octaves set
func StandardTuning() Tuning               // unchanged
func ParseTuning(s string) (Tuning, error) // "E2,A2,D3,G3,B3,E4" low→high → Strings[5-i];
                                           // exactly 6 pitches; each within ±5 of standard (M6)
```

`CalculateNote` and `semitoneToNote` are deleted once nothing calls them (M3). They are the source of F6 and F7.

### 3.3 Chord qualities as data (`chord.go`)

```go
type Tone struct {
    Degree    int  // 1,2,3,4,5,6,7,9,11,13 (letter distance; drives spelling)
    Semitones int  // above root
    Optional  bool
}
func (t Tone) Label() string   // "R","2","♭3","3","4","♭5","5","♯5","6","♭♭7","♭7","7","9","11","13"

type ChordQuality int          // constants in spec order: Major … Minor13
type qualityInfo struct { id, label, suffix string; tones []Tone }
var qualityTable = [...]qualityInfo{ Major: {...}, ... }

func AllChordQualities() []ChordQuality
func ParseChordQuality(id string) (ChordQuality, error)   // exact, case-sensitive ("M7" ≠ "m7")
func (q ChordQuality) String() string                     // id
func (q ChordQuality) Label() string
func (q ChordQuality) Tones() []Tone
func (q ChordQuality) Inversions() []Inversion            // D8

type Chord struct { Root Note; Quality ChordQuality }     // unchanged
func (c Chord) Name() string              // Root.String() + suffix: "B♭m7♭5", "Bdim", "Em"
func (c Chord) Spell(t Tone) Note
func (c Chord) toneAt() [12]int           // pitch class → index into Tones(), or -1
func (c Chord) masks() (all, required uint16)   // pitch-class bitmasks for the generator
```

**Spelling:**
- letter = `(Root.Name + Degree − 1) mod 7`
- accidental = `Root.Accidental + mod12(Semitones) − mod12(natural[letter] − natural[Root.Name])`

**Tone table.** Tones in parentheses are optional. The root is never optional.

| id | tones | | id | tones |
|---|---|---|---|---|
| `maj` | R 3 5 | | `m7b5` | R ♭3 ♭5 ♭7 (♭5 required) |
| `min` | R ♭3 5 | | `dim7` | R ♭3 ♭5 ♭♭7 (♭5 required) |
| `dim` | R ♭3 ♭5 | | `7sus4` | R 4 (5) ♭7 |
| `aug` | R 3 ♯5 | | `add9` | R 3 5 9 |
| `sus2` | R 2 5 | | `9` | R 3 (5) ♭7 9 |
| `sus4` | R 4 5 | | `maj9` | R 3 (5) 7 9 |
| `6` | R 3 5 6 | | `m9` | R ♭3 (5) ♭7 9 |
| `m6` | R ♭3 5 6 | | `11` | R (3) (5) ♭7 (9) 11 |
| `7` | R 3 (5) ♭7 | | `m11` | R ♭3 (5) ♭7 (9) 11 |
| `maj7` | R 3 (5) 7 | | `13` | R 3 (5) ♭7 (9) (11) 13 |
| `m7` | R ♭3 (5) ♭7 | | `maj13` | R 3 (5) 7 (9) (11) 13 |
| | | | `m13` | R ♭3 (5) ♭7 (9) (11) 13 |

- The working tree's `Major11` is dropped, and the `Diminshed` typo disappears with the rewrite.
- `GetChordIntervals` and `IsNoteInChord` are deleted in M3.

### 3.4 Inversion and bass (`inversion.go`, `guitar_voicing.go`)

```go
type Inversion int
const ( AnyInversion Inversion = iota; RootPosition; ThirdInBass; FifthInBass; SeventhInBass )
func ParseInversion(s string) (Inversion, error)  // "any" (default) | "root" | "3rd" | "5th" | "7th"
func (i Inversion) String() string
func (i Inversion) allowsBass(degree int) bool    // Any: true (D9); else exact degree

func (v *GuitarChordVoicing) BassPitch(t Tuning) int   // min over played strings of t.Strings[s].MIDI()+fret
```

### 3.5 Fingering format (`fingering.go`)

```go
const MaxFret = 22
func ParseFingering(s string) ([6]int, error)   // low→high in, high→low out; "x32010" → {0,1,0,2,3,-1}
func FormatFingering(f [6]int) string            // inverse; compact if all frets ≤ 9, else dashed
```

- `GuitarChordVoicing{Chord, Fingering [6]int, Barre *Barre}`, `GetAverageFretPosition` and `GetMinFret` are unchanged.
- `PlayedString`, `CalculateNotes` and the old `MatchesChord` body are deleted once the generator stops using them.

### 3.6 Required tones (`guitar_voicing.go`)

```go
func (v *GuitarChordVoicing) MatchesChord(t Tuning) bool
    // every played pitch class is a chord tone AND present&required == required
```

This lands in M2, while the old generator is still in place. The result is correct but incomplete until M3.

### 3.7 Generator (`guitar_voicing_generator.go`)

The constructor keeps its signature. The signature becomes `GenerateVoicings(chord Chord, inv Inversion) []GuitarChordVoicing`.

The search is a depth-first walk over the strings. Each string's candidates are muted, or every fret from 0 to 22 whose pitch class is a chord tone. The walk tracks the lowest and highest fretted note and **prunes any branch whose fret span exceeds 3**, which is the same limit `IsPlayable` uses. At each leaf it keeps the shape if:
- every required tone is present (`present&required == required`);
- `detectBarreForVoicing` + `IsPlayable()` pass;
- `inv.allowsBass(...)` passes.

It then sorts with `slices.SortFunc` (D7).

Prototype results:
- about 0.6 ms for C and 2.8 ms for Cmaj13 (today about 7.7 s);
- the output matched a brute-force enumeration exactly;
- it finds `x02220`, `022000` and `322003`.

Deleted once unused: `GenerateVoicingsInRange`, `hasRootNote`, `scanFretboard`, `filterChordTones`, `playedStringsToVoicing`, `buildVoicingsFromFretBucket`, `voicingToKey`, `groupPositionsByFret`, `findNearbyChordTones`, `generateCombinations`, and the two tests for them.

### 3.8 Nearest voicing (`nearest.go`)

```go
const mutePenalty = 2
func (v *GuitarChordVoicing) CalculateFingerMovement(other GuitarChordVoicing) float64  // D10
func NearestIndex(vs []GuitarChordVoicing, current [6]int) int                        // -1 if empty
```

**Exact average comparison.** The average-fret tie-break compares fractions exactly: `|sumA·nC − sumC·nA|·nB` against the same for B, with n = max(n, 1). In floating point, exact ties get broken by rounding noise (|11/6 − 2| ≠ |13/6 − 2|), and the "lower fret" rule is silently skipped.

**Worked example (the spec's feature test).** From C `x32010` to F with inversion Any, where each added or dropped string costs 2:

| F shape | Movement | Why |
|---|---|---|
| **`x33211`** | **4** | Same strings as the C shape; fingers slide 0+1+2+0+1 |
| `133211` | 6 | The same slides, plus 2 for adding the low E |
| `xx3211` | 6 | 4 of slides, plus 2 for dropping the A string |
| `533xxx` | 9 | Few slides, but three dropped strings and one added |

Today, when added or dropped strings cost nothing, `533xxx` wins. `133211` can never beat `x33211`, because it is the same shape plus one string to add.

### 3.9 Diatonic chords (`diatonic.go`)

```go
type Mode int   // Ionian … Locrian
func ParseMode(s string) (Mode, error)   // "ionian" … "locrian"
type DiatonicChord struct { Degree int; Root Note; Quality ChordQuality; Numeral string }
func DiatonicChords(key Note, mode Mode) [7]DiatonicChord
```

1. Rotate W W H W W W H by the mode to get the semitone offsets.
2. Spell each scale note with `Chord{Root: key}.Spell(Tone{Degree: i+1, Semitones: off[i]})`.
3. Get each triad's quality from its 3rd and 5th: (4,7) is maj, (3,7) is min, (3,6) is dim.
4. Build the numeral: ♭/♯ from `off[i] − ionian[i]`, lowercase for min and dim, `°` after dim (D15).

---

## 4. HTTP API (`api.go`)

**Routing:**
- `newMux(static fs.FS) *http.ServeMux` registers `GET /api/qualities`, `GET /api/voicings`, `GET /api/diatonic`, `POST /api/progression`, and `mux.Handle("GET /", http.FileServerFS(static))`.
- `main.go` passes it `fs.Sub(webFS, "web")`. Without `fs.Sub`, `/` shows a directory listing and `/js/main.js` returns 404.
- Registering the static route as **`GET /`**, not `/`, is what makes `POST /api/voicings` return 405 instead of 404.

**Conventions:**
- **Errors:** 400 with `{"error":"<param>: <reason>"}`. Every parameter is validated before any work is done.
- **JSON:** slices are always non-nil (`[]`, never `null`). `nearestIndex` is a `*int` so a legitimate 0 isn't dropped.
- **Shared parsing:** one helper, `parseChordParams(root, quality, inversion)`, serves both `/voicings` and `/progression`. It returns 400 when the inversion isn't offered for that quality.
- **Parameters:** `root`, `quality` and `tuning` are required (400 if missing). `inversion` defaults to `any`. `near` is optional.

### `GET /api/qualities`

```json
{"qualities": [{"id":"maj","label":"maj","inversions":["any","root","3rd","5th"]}, …23 in spec order]}
```

### `GET /api/voicings?root=&quality=&inversion=&tuning=&near=`

```json
{
  "chord": {"root":"C","quality":"maj","name":"C",
            "tones":[{"note":"C","interval":"R"},{"note":"E","interval":"3"},{"note":"G","interval":"5"}]},
  "voicings": [
    {"fingering":"x32010","frets":[-1,3,2,0,1,0],"barre":null,"tones":[-1,0,1,2,0,1]},
    {"fingering":"x35553","frets":[-1,3,5,5,5,3],"barre":{"fret":3,"from":1,"to":5},"tones":[-1,0,2,0,1,2]}
  ],
  "nearestIndex": 0
}
```

- `frets`, `barre.from`/`to` and `tones` are all **low→high**.
- `tones[s]` indexes `chord.tones`, with −1 for muted. Index 0 is always the root, which drives the root colour.
- `fingering` is the value the client sends back as `near`. `frets` is what it draws.
- The API's barre struct is built in `api.go` (`from = 5 − EndString`), so `music` gets no JSON tags.
- `nearestIndex` appears only when `near` is given and `voicings` is non-empty.

### `GET /api/diatonic?key=D&mode=dorian`

```json
{"chords": [{"degree":1,"root":"D","quality":"min","name":"Dm","numeral":"i"}, …7 total]}
```

### `POST /api/progression`

```json
// request
{"tuning":"D2,A2,D3,G3,B3,E4","steps":[{"root":"C","quality":"maj","inversion":"any"}, …]}
// response
{"shapes":["x32010","133211", null, …]}
```

- Until some step has a shape, each step gets its voicing 0 (lowest fret). After that, each step gets the shape nearest to the **last non-null** shape before it.
- A step with no voicings gets `null`.
- A bad step returns 400 with a message naming the step (`"step 2: quality: unknown quality \"foo\""`).
- An empty `steps` returns `{"shapes":[]}`.

---

## 5. Frontend (`web/`, embedded, no build step)

| File | Responsibility |
|---|---|
| `index.html` | Layout: the explorer (a Key + mode / Key + type toggle and its pickers, inversion, the tuning preset dropdown, a Notes/Intervals toggle, prev/next with an "i / n" counter, the chord name, the `<svg>` with a ▼/▲ pair beside each string name for tuning, and "Add to progression"); then the progression (step list and playback controls). Root and key pickers offer 17 spellings: C C♯ D♭ D D♯ E♭ E F F♯ G♭ G G♯ A♭ A A♯ B♭ B. |
| `style.css` | CSS custom properties for colours (`--root`, `--tone`, `--barre`, `--string`, `--fret`, `--inlay`), and a horizontal scroll container around the SVG. |
| `js/api.js` | One `fetch` wrapper per endpoint, using `URLSearchParams`. A non-2xx response throws `Error(body.error)`. |
| `js/fretboard.js` | `renderFretboard(svg, {stringNames, voicing, chordTones, labelMode})`. Pure: it rebuilds the SVG's children and holds no state. |
| `js/tuning.js` | Presets as MIDI numbers, low→high:<br>- Standard `[40,45,50,55,59,64]`<br>- Drop D `[38,…]`<br>- Half-step down `[39,44,49,54,58,63]`<br>- DADGAD `[38,45,50,55,57,62]`<br>- Open G `[38,43,50,55,59,62]`<br>- Open D `[38,45,50,54,57,62]`<br>Also `pitchName(midi)`, `tuningParam()`, and `canStep(i, ±1)` for the ±5 limit. |
| `js/progression.js` | Step list rendering (name · numeral, ↑ ↓ ✕, click to load) and a `setTimeout` playback loop. |
| `js/main.js` | State, event wiring, and a stale-response guard (`const my = ++seq; … if (my !== seq) return`). |

### State (in memory only)

```js
const state = {
  qualities: [], tuning: [40,45,50,55,59,64], labelMode: 'notes',
  explorer: { chooser: 'type', root: 'C', quality: 'maj',
              key: 'C', mode: 'ionian', degree: 1, modeQuality: 'diatonic', diatonic: [],
              inversion: 'any', chord: null, voicings: [], index: 0,
              lastFingering: null },   // reference for `near`; null after an empty result, so the next change shows index 0
  steps: [],                           // {root, quality, inversion, name, numeral|null, fingering}
  selectedStep: null,                  // the step object itself, not an index, so it survives reorders
  playback: { playing: false, bpm: 90, beatsPerChord: 4 },
};
```

### Interactions

| Event | Pauses | Requests | Effect |
|---|---|---|---|
| Startup | – | `qualities`, then `voicings` (C maj, no `near`) | Shows index 0 |
| Root, quality, inversion, key, mode, degree or chooser change | yes | `diatonic` if the key or mode changed, or the chooser switches to Key + mode while `diatonic` is empty; then `voicings` with `near=lastFingering` | `index = nearestIndex ?? 0`. Deselects the step (D13). An inversion the new quality doesn't offer resets to `any`. If `voicings` is empty, `lastFingering` becomes null (spec: no current shape → lowest fret). |
| Prev/next shape | yes | – | Wraps. If a step is selected, its `fingering` is updated. |
| Label toggle | yes | – | Re-renders |
| Tuning ▲/▼ or preset | yes | `POST progression` if there are steps, then `voicings` with `near` = the selected step's new shape or `lastFingering` | Replaces every step's shape in order |
| Add to progression (disabled when there are no voicings) | yes | – | Pushes the current chord, inversion, name, **current fingering**, and a numeral: from the degree when the chooser is Key + mode, copied from the loaded step if one is selected, otherwise none. |
| Step ↑ / ↓ / ✕ | yes | – | The other steps' shapes are untouched. `selectedStep` follows the moved step, and becomes null if the selected step is deleted. |
| Step click, playback prev/next, playback tick | no | `voicings` for the step, with `near` = the step's fingering | Selects the step. Sets the chooser to Key + type, fills root/quality/inversion from the step, and sets `lastFingering` to its fingering. `index` becomes the exact fingering match, falling back to `nearestIndex`. |
| Play/pause, BPM, beats | no | – | Tick every `60000 / bpm × beatsPerChord` ms, wrapping around |

### Fretboard SVG

- **Dimensions:** viewBox about `1220 × 224`. Nut at x = 64, fret width 52, string gap 34.
- **Layout:** high e at the top, as in the old UI. Fret numbers run along the board.
- **Inlays** at 3, 5, 7, 9, 15, 17, 19 and 21, with a double dot at 12 (`ui/fretboard.go:82-83`).
- **Strings** get thicker toward the low string. String names come from the **current tuning**.
- **Nut markers:** `×` for a muted string. An open string gets a hollow ring with its label, in the root colour when it's the root.
- **Barre:** a rounded rect under the dots.
- **Dots:** r = 14, class `root` when the tone index is 0. The label is `note` or `interval` according to `labelMode`, **defaulting to notes**.
- **No voicings:** the board is empty and shows "No playable shape for this chord/inversion in this tuning".

---

## 6. Milestones

Every milestone ends with `go vet ./... && go test ./...` passing, plus its own checks.

### M1: Web port (existing capability)
- **Delete** `fretboard_app.go` and `ui/`. Run `go mod tidy` (no `require` lines are left).
- **Untrack the binary:** `git rm --cached fretboard-visualizer` and add it to `.gitignore`. Your WIP in `music/chord.go` goes into whichever commit first touches that file; it needs no separate commit.
- **New `main.go`** (embed + `fs.Sub` + `ListenAndServe("localhost:8080", …)`), `api.go` and `api_test.go`.
- **`music/`:**
  - `note.go`: signed accidentals (§3.1) and the `SemitoneValue` wrap fix, then `ParseNote`, `ParsePitch` and `MIDI`, which depend on them.
  - `ParseTuning` (order and count only; the ±5 check comes in M6) and `fingering.go`.
  - `chord.go`: `ParseChordQuality` accepts the spec ids `maj`, `min` and `maj7`.
- **API:**
  - `/api/qualities` returns only maj, min and maj7, because the old generator is slow on larger chords. There is no `inversions` field yet.
  - `/api/voicings` has its final shape except `chord.name` and `chord.tones[].interval` (added in M2). It runs on the **old** generator, with sharps-only notes.
- **Web:** `index.html`, `style.css`, and `js/{main,api,fretboard}.js`, covering root, quality, prev/next and the SVG. Tuning stays at standard.
- **Verify:**
  - Tests: `TestStatic_ServesPageAndModules`, `TestStatic_PostIs405`, `TestVoicings_CMajorIncludesX32010` (presence and `tones`), `TestVoicings_BarreLowToHigh`, `TestVoicings_RejectsBadParams` (root, quality and tuning cases), `TestParseNote`, `TestParsePitch`, `TestSemitoneValueWraps`, `TestParseTuning` (order and count), `TestParseFingering`.
  - Manual: `go run .` and open `localhost:8080`.
    - C shows `x32010` somewhere in prev/next.
    - F shows a barre bar.
    - ×/○ markers, inlays and gold roots all render.

### M2: Chord table, spelling, required tones
- **`music/`:**
  - `chord.go`: the table, `Spell`, `Name`, `Tone.Label`, `toneAt`, `masks`. This replaces your WIP enum.
  - Keep the old generator compiling on the table: `IsNoteInChord(n)` becomes `c.toneAt()[n.SemitoneValue()] >= 0`, and `GetChordIntervals` becomes a loop over `Tones()`. Without this, the old switch on removed constants either won't compile or falls back to a triad.
  - `MatchesChord` uses the required mask. The old generator still runs, so results are correct but incomplete.
- **API:** adds `chord.name` and `chord.tones[].interval`, with notes spelled correctly. `/api/qualities` lists all 23. 11ths and 13ths stay slow until M3.
- **Web:** the Notes/Intervals toggle and the chord name heading.
- **Verify:**
  - Tests: `TestSpell_AllQualities`, `TestSpell_TrickyRoots`, `TestToneLabels`, `TestMatchesChord` (table in §7.2), `TestVoicings_C9ContainsR3b7_9`, `TestVoicings_BbSpelledWithFlats`, `TestQualities_ListsSpec` (23 ids in order). `TestVoicings_CMajorIncludesX32010` gains the name "C" check.
  - Manual: C no longer offers `335xxx`. B♭ reads B♭/D/F. The Intervals toggle shows R/3/5 on C.

### M3: Generator rewrite
- The DFS generator (§3.7) and the deterministic sort (D7).
- The barre finger rule (D6).
- Delete dead code, including `hasRootNote`, `CalculateNote`, `semitoneToNote`, `GetChordIntervals`, `GetChordNotes` and `IsNoteInChord`.
- Fix the mislabelled fixtures in `guitar_voicing_test.go`: "F barre" is really `112331`, and "bad mute" plays only 2 strings.
- **Verify:**
  - Tests: `TestGenerate_MatchesBruteForce` (C, A, C9, G in Open G), `TestGenerate_IncludesKnownShapes`, `TestGenerate_DeterministicOrder`, `TestGenerate_UsesActiveTuning`, `TestIsPlayable` (the existing 13 cases plus new ones), `TestVoicings_SortedAndStable`, `BenchmarkGenerate` (Cmaj13 in single-digit ms).
  - Manual: A includes `x02220`. The order is the same after a reload. C13 responds instantly.

### M4: Inversions
- `Inversion`, `Inversions()`, `BassPitch` by MIDI pitch, and the generator filter.
- The `inversion` parameter, returning 400 when the quality doesn't offer it. `/api/qualities` gains `inversions`.
- **Web:** an inversion dropdown driven by the quality.
- **Verify:**
  - Tests: `TestInversionsOffered`, `TestBassPitch_CrossedStrings`, `TestVoicings_InversionSetsBass` (it computes the lowest **pitch** from `frets` and the tuning, not the lowest string), `TestVoicings_InversionUnavailableIs400`.
  - Extended: `TestVoicings_RejectsBadParams` adds `inversion=9th`. `TestQualities_ListsSpec` adds the inversion assertions. `TestGenerate_MatchesBruteForce` adds F with Root, and Em7 with 3rd in a crossed tuning.
  - Manual: sus4 offers no 3rd, and maj offers no 7th.

### M5: Nearest voicing
- The metric (D10), `NearestIndex`, `near` and `nearestIndex`.
- **Web:** every chord change sends `near` and jumps to `nearestIndex`.
- **Verify:**
  - Tests: `TestCalculateFingerMovement`, `TestNearestIndex_TieBreaks` (each step tested on its own, including an exact-fraction tie), `TestVoicings_NearestFMajorFromOpenC`, `TestVoicings_NearestHighFrets` (`near=8-10-10-9-8-8` round-trips), `TestVoicings_Empty`. `TestVoicings_RejectsBadParams` adds `near=x3201`.
  - Manual: from `x32010`, switching to F lands on `x33211`.

### M6: Tuning
- The ±5 check in `ParseTuning`.
- **Web:** `tuning.js`, ▲/▼ per string (disabled at ±5), the preset dropdown, and string labels that follow the tuning. A tuning change refreshes the explorer to the nearest shape.
- **Verify:**
  - Tests: `TestParseTuning` (adds the ±5 bounds), `TestVoicings_DropDChangesDMajor`, `TestVoicings_LabelsFollowTuning`, `TestVoicings_TuningOutOfRangeIs400`.
  - Manual:
    - Drop D shows a D label, and D offers `000232`.
    - Open G gives G `000000`.
    - ▲ on the low string stops at A2.

### M7: Diatonic chords and the Key + mode picker
- `DiatonicChords` and `/api/diatonic`.
- **Web:** the chooser toggle, key and mode selects, degree buttons ("ii Em"), and a quality select that starts on "Diatonic".
- **Verify:**
  - Tests: `TestDiatonic_AllModes`, `TestDiatonic_Spelling`, `TestDiatonic_RoundTrip`, `TestDiatonic_DDorian` (API), `TestDiatonic_Errors`.
  - Manual: D Dorian, degree ii shows Em. Overriding to m7 moves to the nearest Em7.

### M8: Progression
- `POST /api/progression`.
- **Web:** `progression.js` step list with add, ↑/↓/✕, click to load, and browsing that updates the loaded step. A tuning change recalculates every step.
- **Verify:**
  - Tests: `TestProgression_ChainsNearest`, `TestProgression_NullStepChainContinues`, `TestProgression_TuningRecalc`, `TestProgression_Errors`.
  - Manual:
    - Add C, Am, F and G. Click F and press next; the step keeps its new shape after you click away and back.
    - Deleting Am leaves the others unchanged. ↑/↓ reorders without changing any shape.
    - Adding C twice gives two steps.
    - Added from D Dorian, degree ii shows "Em · ii". Switching the key to G afterwards leaves existing steps unchanged.
    - Drop D changes every step.

### M9: Playback (frontend only)
- Play/pause, prev/next, BPM and beats per chord, wraparound, and the pause rules (D14).
- **Verify (manual):**
  - 120 BPM × 2 beats advances once a second and loops.
  - Changing the root pauses. So do explorer prev/next, the label toggle, a tuning ▲, step ✕ and Add.
  - Playback prev/next and clicking a step jump without pausing.

### M10: README
- `go run .`, the URL, Go 1.25 (matching `go.mod`), and "restart after editing `web/`" (the files are embedded).
- **Verify:** follow the README from a fresh clone.

---

## 7. Test plan

### 7.1 Feature tests (`api_test.go`, `httptest.NewServer(newMux(sub))`)

`std = "E2,A2,D3,G3,B3,E4"`, `dropD = "D2,A2,D3,G3,B3,E4"`. Queries are built with `url.Values{}.Encode()`, so `#` becomes `%23`. A raw `#` would start a URL fragment, and the server would see `tuning=A`.

| Test | Scenario → expectation |
|---|---|
| `TestStatic_ServesPageAndModules` | `/` → 200 HTML with `type="module"`. `/js/main.js` → Content-Type with prefix `text/javascript` (Go adds `; charset=utf-8`). |
| `TestStatic_PostIs405` | `POST /api/voicings` → 405 |
| `TestVoicings_CMajorIncludesX32010` **(spec)** | `x32010` is present, with `tones [-1,0,1,2,0,1]`. From M2: name "C". |
| `TestVoicings_BarreLowToHigh` | C in `std`: `x35553` has `barre {"fret":3,"from":1,"to":5}` and `x32010` has `barre: null`. F: `133211` has `{"fret":1,"from":0,"to":5}`. |
| `TestVoicings_RejectsBadParams` | Missing root; missing tuning; `H`; `M7`; a 5-note tuning; a tuning without octaves → 400 with `error`. M4 adds `inversion=9th`, and M5 adds `near=x3201`. |
| `TestVoicings_SortedAndStable` | Lowest fret never decreases along the list. Two identical calls return identical lists. |
| `TestVoicings_C9ContainsR3b7_9` **(spec)** | Every voicing's tones include R, 3, ♭7 and 9. |
| `TestVoicings_BbSpelledWithFlats` | Notes are all in {B♭, D, F}, and the name is "B♭". |
| `TestVoicings_InversionSetsBass` | C7 with 3rd: in every voicing the lowest **pitch** is E. Am with Root in the crossed tuning `A2,E2,D3,G3,B3,E4`: in every voicing the lowest pitch is A. |
| `TestVoicings_InversionUnavailableIs400` | `maj` + `7th` → 400 |
| `TestVoicings_Empty` | B♭m9 with 5th in standard → `voicings: []` and no `nearestIndex`. |
| `TestVoicings_NearestFMajorFromOpenC` **(spec)** | `root=F&quality=maj&near=x32010` → `voicings[nearestIndex]` is `x33211`. |
| `TestVoicings_NearestHighFrets` | `near=8-10-10-9-8-8` returns the index of that same shape. |
| `TestVoicings_DropDChangesDMajor` **(spec)** | The two lists differ, and `000232` appears only in Drop D. |
| `TestVoicings_LabelsFollowTuning` | In Drop D, `000232` gives low string = D, interval R. |
| `TestVoicings_TuningOutOfRangeIs400` | `A#2,…` (+6) → 400. `A2,…` (+5) → 200. `A#1,…` (−6) → 400. The 400 error message mentions the range, so a parse error can't make the test pass. |
| `TestQualities_ListsSpec` | 23 ids in spec order. From M4: maj offers any/root/3rd/5th, sus4 has no 3rd, 6 has no 7th, 11 has 3rd. |
| `TestDiatonic_DDorian` **(spec)** | Names are Dm Em F G Am Bdim C. |
| `TestDiatonic_Errors` | `key=H`, `mode=foo` → 400 |
| `TestProgression_ChainsNearest` **(spec)** | C, Am, F (Root), G in `std`: `shapes[0]` = voicings(C)[0]. Each later shape = voicings(step, near = previous shape)[nearestIndex]. |
| `TestProgression_NullStepChainContinues` | C, B♭m9 (5th), G: `[1]` is null, and `[2]` is nearest to `[0]`. B♭m9 (5th), C: `[null, voicings(C)[0]]`. |
| `TestProgression_TuningRecalc` | The same steps in `dropD`: each shape appears in that step's Drop D list. |
| `TestProgression_Errors` | Bad JSON, a bad tuning, a bad step 2 (the message names the step) → 400. `steps: []` → `{"shapes":[]}`. |

### 7.2 Unit tests (`music/`)

**`note_test.go`**
- `ParseNote`: Bb and B♭ = pc 10, F## = 7, Cb = 11, B# = 0.
- Rejected: `""`, `H`, `c`, `C#b`.
- `ParsePitch`: E2, Eb2 and F♯3 are accepted. `E` (no octave) is rejected.
- `MIDI`: E2 = 40, Eb2 = 39, B♯3 = 60, C♭4 = 59.

**`tuning_test.go`**
- `ParseTuning(std) == StandardTuning()`, with the order reversed correctly.
- ±5 bounds enforced; exactly 6 pitches required.

**`fingering_test.go`**
- `x32010` ↔ `{0,1,0,2,3,-1}`.
- Dashed form round-trips, and `x-3-2-0-1-0` prints back as `x32010`.
- Rejected: wrong length, bad character, fret 23.

**`chord_test.go`**
- All 23 qualities spelled on C (the §3.3 table).
- Tricky roots: B♭ = B♭ D F; B♭dim7 = B♭ D♭ F♭ A♭♭; G♭m = G♭ B♭♭ D♭; C♭ = C♭ E♭ G♭; G♯aug = G♯ B♯ D♯♯.
- Labels: C13 = R 3 5 ♭7 9 11 13; Cdim7 = … ♭♭7; Csus2 = R 2 5.
- `Name()`: "B♭m7♭5".
- `Inversions()` per D8.

**`guitar_voicing_test.go`**

`TestMatchesChord` (standard tuning unless noted):

| Chord | Fingering | Expected |
|---|---|---|
| C | `x32010` | ✔ |
| C | `335xxx` | ✘ (no 3rd) |
| C | `x32000` | ✘ (B is not in the chord) |
| C7 | `x32310` | ✔ |
| C9 | `x30310` | ✔ |
| C9 | `355553` | ✘ (no ♭7) |
| C11 | `x33311` | ✔ |
| Cm11 | `x33311` | ✘ (no ♭3) |
| Cm7♭5 | `x3434x` | ✔ |
| D | `000232` in Drop D | ✔ |
| D | `000232` in standard | ✘ |

Other tests in this file:
- `TestIsPlayable`: the existing 13 cases, plus `879987` ✘ (barre + 4 fingers), `x33211` ✔, `x02220` ✔.
- `TestBassPitch_CrossedStrings`: Am `002210` in `A2,E2,D3,…` has E2 as its bass, which is the 5th.
- `TestCalculateFingerMovement` (p = 2): `x32010` → `x33211` = 4; → `133211` = 6. The metric is symmetric.
- `TestNearestIndex_TieBreaks`: synthetic lists, one per tie-break level, including an exact-fraction tie that has to fall through to the lower fret.

**`guitar_voicing_generator_test.go`**
- `MatchesBruteForce`: a test-only enumeration over {x, 0..22}⁶ with span pruning. It filters with `MatchesChord` + `IsPlayable` + `allowsBass`, so it is independent of the generator's inline mask check. Covers C, A, C9 and G in Open G (M3), then F with Root and Em7 with 3rd in a crossed tuning (M4).
- `IncludesKnownShapes`: C `x32010` `x35553`; A `x02220` `577655`; Em `022000` `322003`; F `133211` `x33211`; G `320003` `355433`; D `xx0232`; B♭ `x13331` `688766`; plus `8-10-10-9-8-8`.
- `DeterministicOrder`.
- `UsesActiveTuning`: Drop D D `000232`, Open G G `000000`.
- `BenchmarkGenerate`: C and Cmaj13 in `std`. Cmaj13 must stay in single-digit milliseconds.

**`diatonic_test.go`**
- All 7 modes with their numerals:
  - C Ionian: I ii iii IV V vi vii°
  - D Dorian: i ii ♭III IV v vi° ♭VII
  - E Phrygian: i ♭II ♭III iv v° ♭VI ♭vii
  - F Lydian: I II iii ♯iv° V vi vii
  - G Mixolydian: I ii iii° IV v vi ♭VII
  - A Aeolian: i ii° ♭III iv v ♭VI ♭VII
  - B Locrian: i° ♭II ♭iii iv ♭V ♭VI ♭vii
- Spelling: G♭ Ionian has C♭ as IV. G♯ Ionian vii° is F♯♯dim.
- **Round trip:** for each of the 17 key spellings in §5 × 7 modes, every diatonic root parses back through `ParseNote` and generates without error.

### 7.3 Manual
The M1–M9 checklists above. Browser automation is deferred, per the spec.

---

## 8. Risks

- **Many more shapes, and odd ones.** The complete generator finds 173 C shapes, against 97 today. 98 of those mix open strings with frets ≥ 6 (`875050`), which clutters prev/next. They are kept (Q3). If they bother you in the UI, add a rule later, e.g. no open strings when a fretted note is above fret N.
- **More shapes under Any.** With no bass filter (Q2), extended chords return many more shapes (for example C13 goes from 302 to 645, and C6 from 200 to 334, of which 134 have the 6th in the bass). Prev/next gets long, but nearest-voicing lands you on a sensible one.
- **Mirror-image shapes** if a high→low / low→high conversion is wrong somewhere. Mitigation: all conversions live in `fingering.go` / `ParseTuning` / the API barre builder, with round-trip tests. `TestVoicings_BarreLowToHigh` and the Drop D label test catch reversed `barre`, `tones` or `frets`.
- **The brute-force oracle shares `IsPlayable` with the generator,** so it can't catch playability bugs. The hand-written `IsPlayable` and `MatchesChord` tables cover those.
- **Response size.** Cmaj13 returns about 1,240 voicings (roughly 90–120 KB in the compact format). That's fine on localhost; revisit before public hosting.
- **Embedded files need a restart after edits.** Documented in the README (M10).
- **Some common shapes stay rejected by the kept rules.** C9 `x32333` counts as 5 fingers, and G13 `3x3455` has a muted middle string. These are known and out of scope.

---

## 9. Decisions made with you

| # | Question | Decision |
|---|---|---|
| Q1 | How does nearest-voicing count added or dropped strings? | 2 per string. The spec example becomes C `x32010` → F `x33211`. |
| Q2 | What can "Any" put in the bass? | Any chord tone, extensions included (no filter). |
| Q3 | Keep shapes that mix open strings with frets ≥ 6? | Keep them. Revisit if they clutter the UI. |
| Q4 | Is the ♭5 required in m7♭5 and dim7? | Yes. Only a perfect 5th is optional. |
| Q5 | Roman numerals in modes? | Measured against the major scale (D Dorian: i ii ♭III IV v vi° ♭VII). |
| Q6 | Keys that need double sharps or flats? | Shown as spelled. The round-trip test covers every key × mode. |
| Q7 | Add `GET /api/qualities` and dashed fingerings to the spec? | Yes, added to `intent.md`. |
| Q8 | Housekeeping? | Untrack and gitignore the binary in M1. The `chord.go` WIP goes into a normal commit, not a separate one. |

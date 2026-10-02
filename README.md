# Fretboard Visualizer

A web app that shows a guitar fretboard and visualizes chord shapes in any key, all along the neck, in any tuning. Build chord progressions from a key and mode, and play them back.

The app generates playable chord positions by rule (fret span, finger count, muting). Not every generated shape will feel comfortable in practice.

**Try it live:** <https://fretboard.tail9292f9.ts.net/> (served from EC2 over a Tailscale tunnel)

## Features

- 23 chord qualities, from triads to 13ths, with inversions by lowest sounding pitch
- Nearest-voicing navigation: changing chord jumps to the shape with the least finger movement
- Hover any fret (or open string) to see its note name, and click it to jump to a shape that plays that spot
- An open-string limit: shapes with open strings stay at or below a fret you choose (default 5)
- A toggle that shows every chord tone across the whole neck
- Any 6-string tuning within ±5 semitones of standard, with presets (Drop D, DADGAD, Open G, …)
- Diatonic chords for any key and mode, with Roman numerals
- Progressions with fixed shapes, reordering, and looped playback
- Build a chord by placing notes on the fretboard: the app names it, offers other names for the same notes, and refuses shapes that can't be played

## Running

Requires Go 1.26 or later (see `go.mod`). The Go toolchain fetches the dependencies (SQLite and OpenID Connect/OAuth2 libraries, used by the optional accounts feature) on the first build.

```bash
git clone https://github.com/nstalter/fretboard-visualizer.git
cd fretboard-visualizer
go run .
```

Then open <http://localhost:8080>.

The files in `web/` are embedded in the binary, so **restart the server after editing them**.

### Accounts and saved songs (optional)

Signing in with Amazon Cognito lets a user keep songs (each with named progressions, optionally in folders). Set all five environment variables to turn it on; setting only some is an error:

- `FRETBOARD_BASE_URL`: the site's public URL, e.g. `https://fretboard.tail9292f9.ts.net` (the Cognito redirect and logout URLs are built from it)
- `COGNITO_ISSUER`: the user pool's OIDC issuer, `https://cognito-idp.<region>.amazonaws.com/<user-pool-id>`
- `COGNITO_CLIENT_ID`: the app client's ID
- `COGNITO_CLIENT_SECRET`: the app client's secret (keep it out of the repository and the shell history)
- `COGNITO_DOMAIN`: the hosted login domain, `https://<prefix>.auth.<region>.amazoncognito.com`

Songs and login sessions are stored in a SQLite file, set with `-db` (default `fretboard.db` in the working directory). In production use an absolute path outside the deploy directory, e.g. `-db /var/lib/fretboard/fretboard.db`, so a redeploy never touches it.

For local testing without Cognito, run `go run . -dev-user you@example.com`. It signs every request in as that email, so it needs a loopback `-addr` (the default is fine), cannot be combined with the Cognito variables, and refuses requests whose `Host` header is not loopback. Never use it on a public server.

With none of these settings the app behaves as before: no sign-in, no database file.

## Testing

```bash
go vet ./... && go test ./...
```

## Layout

- `music/`: instrument-independent theory: notes, chord spelling, inversions and diatonic chords (standard library only)
- `guitar/`: tunings, fingerings, voicing generation, playability and nearest voicing; depends on `music/`
- `api.go`: the stateless JSON API (`/api/qualities`, `/api/voicings`, `/api/diatonic`, `/api/progression`, `/api/identify`)
- `accounts.go`: wires sign-in and saved songs into the server, or reports that they are disabled
- `songs_api.go`: the signed-in user's JSON API for folders, songs and progressions (`/api/library`, `/api/folders`, `/api/songs`, `/api/progressions`)
- `auth/`: Amazon Cognito sign-in (OAuth 2.0 authorization code flow with PKCE) and server-side sessions
- `store/`: the SQLite storage for sessions, folders, songs and progressions
- `web/`: plain HTML, CSS and ES modules, with the fretboard drawn in SVG
- `docs/code-tour.html`: a guided tour of the code and the Go idioms it uses (open it in a browser)

## License

MIT License

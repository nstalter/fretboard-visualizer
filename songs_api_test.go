package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nstalter/fretboard-visualizer/store"
)

// A valid saved progression, in the key order the server stores it.
const songsData = `{"tuning":"E2,A2,D3,G3,B3,E4","openMax":5,"bpm":90,"beats":4,"steps":[` +
	`{"root":"C","quality":"maj7","inversion":"any","name":"Cmaj7","numeral":"I","fingering":"x32000"},` +
	`{"root":"G","quality":"7","inversion":"any","name":"G7","numeral":null,"fingering":"320001"}]}`

// Valid data that differs from songsData, also in stored key order.
const songsOther = `{"tuning":"E2,A2,D3,G3,B3,E4","openMax":5,"bpm":140,"beats":4,"steps":[` +
	`{"root":"C","quality":"maj7","inversion":"any","name":"Cmaj7","numeral":"I","fingering":"x32000"}]}`

type songsEnv struct {
	t     *testing.T
	store *store.Store
	mux   *http.ServeMux
}

func newSongsEnv(t *testing.T) *songsEnv {
	return newSongsEnvGuard(t, func(h http.Handler) http.Handler { return h })
}

// newSongsEnvGuard serves the song routes with a fake sign-in: the X-Test-User header is the user.
func newSongsEnvGuard(t *testing.T, guard func(http.Handler) http.Handler) *songsEnv {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	mux := http.NewServeMux()
	registerSongRoutes(mux, st, func(r *http.Request) (string, bool) {
		u := r.Header.Get("X-Test-User")
		return u, u != ""
	}, guard)
	return &songsEnv{t, st, mux}
}

type songsResp struct {
	code   int
	body   []byte
	header http.Header
}

func (e *songsEnv) do(user, method, path, body string) songsResp {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return songsResp{rec.Code, rec.Body.Bytes(), rec.Header()}
}

func (r songsResp) into(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("response %d %q is not the expected JSON: %v", r.code, r.body, err)
	}
}

func (r songsResp) errorText() string {
	var e struct{ Error string }
	json.Unmarshal(r.body, &e)
	return e.Error
}

func (r songsResp) wantStatus(t *testing.T, code int) {
	t.Helper()
	if r.code != code {
		t.Fatalf("status = %d, want %d (body %s)", r.code, code, r.body)
	}
}

// wantError checks for an error status with a JSON error body containing the text.
func (r songsResp) wantError(t *testing.T, code int, contains string) {
	t.Helper()
	if r.code != code || !strings.Contains(r.errorText(), contains) {
		t.Errorf("got %d %s, want %d with error containing %q", r.code, r.body, code, contains)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
}

type songsLibrary struct {
	Folders []struct {
		ID   int64
		Name string
	}
	Songs []struct {
		ID           int64
		Name         string
		FolderID     *int64
		UpdatedAt    string
		Progressions []struct {
			ID       int64
			Name     string
			Position int
		}
	}
}

func (e *songsEnv) library(user string) songsLibrary {
	e.t.Helper()
	r := e.do(user, "GET", "/api/library", "")
	r.wantStatus(e.t, 200)
	var lib songsLibrary
	r.into(e.t, &lib)
	return lib
}

func songsJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (e *songsEnv) create(user, path, body string) int64 {
	e.t.Helper()
	r := e.do(user, "POST", path, body)
	r.wantStatus(e.t, 201)
	var out struct{ ID int64 }
	r.into(e.t, &out)
	return out.ID
}

func (e *songsEnv) newFolder(user, name string) int64 {
	e.t.Helper()
	return e.create(user, "/api/folders", songsJSON(map[string]any{"name": name}))
}

// newSong creates a song, in the folder unless folderID is 0.
func (e *songsEnv) newSong(user, name string, folderID int64) int64 {
	e.t.Helper()
	body := map[string]any{"name": name}
	if folderID != 0 {
		body["folderId"] = folderID
	}
	return e.create(user, "/api/songs", songsJSON(body))
}

func (e *songsEnv) newProg(user string, songID int64, name string) int64 {
	e.t.Helper()
	return e.create(user, fmt.Sprintf("/api/songs/%d/progressions", songID), progBody(name, songsData))
}

func progBody(name, data string) string {
	return fmt.Sprintf(`{"name":%s,"data":%s}`, songsJSON(name), data)
}

// songsVariant is songsData with a change applied to the top level and to its first step.
func songsVariant(mutate func(d, step map[string]any)) string {
	var d map[string]any
	if err := json.Unmarshal([]byte(songsData), &d); err != nil {
		panic(err)
	}
	mutate(d, d["steps"].([]any)[0].(map[string]any))
	return songsJSON(d)
}

// songsWithSteps is songsData with n copies of its first step.
func songsWithSteps(n int) string {
	return songsVariant(func(d, step map[string]any) {
		steps := make([]any, n)
		for i := range steps {
			steps[i] = step
		}
		d["steps"] = steps
	})
}

type songsFixture struct{ folder, song, prog int64 }

// fixture gives the user a folder holding a song that has one progression.
func (e *songsEnv) fixture(user string) songsFixture {
	e.t.Helper()
	f := e.newFolder(user, "Folder")
	s := e.newSong(user, "Song", f)
	return songsFixture{f, s, e.newProg(user, s, "Verse")}
}

type songsRoute struct {
	method, path, body string
	byID               bool // addresses one of the fixture's rows
}

func (f songsFixture) routes() []songsRoute {
	return []songsRoute{
		{"GET", "/api/library", "", false},
		{"POST", "/api/folders", `{"name":"New folder"}`, false},
		{"PATCH", fmt.Sprintf("/api/folders/%d", f.folder), `{"name":"Renamed"}`, true},
		{"DELETE", fmt.Sprintf("/api/folders/%d", f.folder), "", true},
		{"POST", "/api/songs", `{"name":"New song"}`, false},
		{"PATCH", fmt.Sprintf("/api/songs/%d", f.song), `{"name":"Renamed","folderId":null}`, true},
		{"DELETE", fmt.Sprintf("/api/songs/%d", f.song), "", true},
		{"POST", fmt.Sprintf("/api/songs/%d/progressions", f.song), progBody("Chorus", songsData), true},
		{"PUT", fmt.Sprintf("/api/songs/%d/progressions/order", f.song), fmt.Sprintf(`{"ids":[%d]}`, f.prog), true},
		{"GET", fmt.Sprintf("/api/progressions/%d", f.prog), "", true},
		{"PUT", fmt.Sprintf("/api/progressions/%d", f.prog), `{"name":"Renamed"}`, true},
		{"DELETE", fmt.Sprintf("/api/progressions/%d", f.prog), "", true},
	}
}

func (r songsRoute) String() string { return r.method + " " + r.path }

// snapshot is everything a user can see of their own data, for before/after comparison.
func (e *songsEnv) snapshot(user string, f songsFixture) string {
	e.t.Helper()
	lib := e.do(user, "GET", "/api/library", "")
	prog := e.do(user, "GET", fmt.Sprintf("/api/progressions/%d", f.prog), "")
	return fmt.Sprintf("%d %s\n%d %s", lib.code, lib.body, prog.code, prog.body)
}

func TestSongsAPI_SignedOutIs401OnEveryRoute(t *testing.T) {
	e := newSongsEnv(t)
	f := e.fixture("alice")
	before := e.snapshot("alice", f)
	for _, rt := range f.routes() {
		r := e.do("", rt.method, rt.path, rt.body)
		r.wantError(t, 401, "sign in required")
		if got := string(r.body); got != `{"error":"sign in required"}`+"\n" {
			t.Errorf("%s: body = %q", rt, got)
		}
	}
	if after := e.snapshot("alice", f); after != before {
		t.Errorf("signed-out requests changed the data:\nbefore %s\nafter  %s", before, after)
	}
}

func TestSongsAPI_ResponsesAreJSONAndNotCached(t *testing.T) {
	e := newSongsEnv(t)
	f := e.fixture("alice")
	for _, rt := range f.routes() {
		for _, user := range []string{"", "bob"} { // a 401 and a 404 or 2xx
			r := e.do(user, rt.method, rt.path, rt.body)
			if cc := r.header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("%s as %q: Cache-Control = %q", rt, user, cc)
			}
			if r.code != 204 && r.header.Get("Content-Type") != "application/json" {
				t.Errorf("%s as %q: Content-Type = %q", rt, user, r.header.Get("Content-Type"))
			}
		}
	}
}

func TestSongsAPI_AnotherUserCanDoNothing(t *testing.T) {
	e := newSongsEnv(t)
	f := e.fixture("alice")
	bobFolder := e.newFolder("bob", "Bob's folder")
	bobSong := e.newSong("bob", "Bob's song", 0)
	before := e.snapshot("alice", f)

	missing := songsFixture{f.folder + 1000, f.song + 1000, f.prog + 1000}.routes()
	for i, rt := range f.routes() {
		if !rt.byID {
			continue
		}
		r := e.do("bob", rt.method, rt.path, rt.body)
		if r.code != 404 {
			t.Errorf("bob %s = %d %s, want 404", rt, r.code, r.body)
			continue
		}
		// A row that belongs to someone else looks exactly like one that does not exist.
		m := missing[i]
		if got := e.do("bob", m.method, m.path, m.body); got.code != r.code || !bytes.Equal(got.body, r.body) {
			t.Errorf("bob %s: someone else's row answers %d %s but a missing row answers %d %s", rt, r.code, r.body, got.code, got.body)
		}
	}

	// Moving a song into someone else's folder, and creating one there, fails.
	r := e.do("bob", "PATCH", fmt.Sprintf("/api/songs/%d", bobSong), fmt.Sprintf(`{"folderId":%d}`, f.folder))
	r.wantError(t, 404, "folder not found")
	e.do("bob", "POST", "/api/songs", fmt.Sprintf(`{"name":"Sneaky","folderId":%d}`, f.folder)).wantError(t, 404, "folder not found")
	// Nor can alice use bob's folder, or move bob's song.
	e.do("alice", "PATCH", fmt.Sprintf("/api/songs/%d", f.song), fmt.Sprintf(`{"folderId":%d}`, bobFolder)).wantError(t, 404, "folder not found")
	e.do("alice", "PATCH", fmt.Sprintf("/api/songs/%d", bobSong), `{"name":"x"}`).wantError(t, 404, "song not found")
	e.do("alice", "POST", fmt.Sprintf("/api/songs/%d/progressions", bobSong), progBody("x", songsData)).wantError(t, 404, "song not found")

	if after := e.snapshot("alice", f); after != before {
		t.Errorf("another user's requests changed alice's data:\nbefore %s\nafter  %s", before, after)
	}
	lib := e.library("bob")
	if len(lib.Folders) != 1 || len(lib.Songs) != 1 || lib.Songs[0].FolderID != nil || len(lib.Songs[0].Progressions) != 0 {
		t.Errorf("bob's library = %+v", lib)
	}
	if got := e.library("carol"); len(got.Folders)+len(got.Songs) != 0 {
		t.Errorf("a third user sees %+v", got)
	}
}

func TestSongsAPI_GuardWrapsMutatingRoutesOnly(t *testing.T) {
	var hits []string
	e := newSongsEnvGuard(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path)
			writeStatusError(w, http.StatusForbidden, "blocked by guard")
		})
	})
	f := songsFixture{1, 1, 1}
	var mutating []string
	for _, rt := range f.routes() {
		r := e.do("alice", rt.method, rt.path, rt.body)
		if rt.method == "GET" {
			if r.code == 403 {
				t.Errorf("%s went through the guard", rt)
			}
			continue
		}
		mutating = append(mutating, rt.method+" "+rt.path)
		r.wantError(t, 403, "blocked by guard")
	}
	if len(mutating) != 10 {
		t.Fatalf("expected 10 mutating routes, found %d", len(mutating))
	}
	if !reflect.DeepEqual(hits, mutating) {
		t.Errorf("guard saw %v, want exactly the mutating requests %v", hits, mutating)
	}
	if lib := e.library("alice"); len(lib.Folders)+len(lib.Songs) != 0 {
		t.Errorf("a guarded request still changed data: %+v", lib)
	}
}

func TestSongsAPI_GuardPassesThroughWhenItAllows(t *testing.T) {
	calls := 0
	e := newSongsEnvGuard(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			next.ServeHTTP(w, r)
		})
	})
	e.newFolder("alice", "F")
	if calls != 1 {
		t.Errorf("guard called %d times for one POST, want 1", calls)
	}
	e.library("alice")
	if calls != 1 {
		t.Errorf("guard called for a GET")
	}
}

func TestSongsAPI_CoexistsWithTheExistingRoutes(t *testing.T) {
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mux := newMux(static)
	registerSongRoutes(mux, st, func(*http.Request) (string, bool) { return "u", true }, func(h http.Handler) http.Handler { return h }) // panics on a pattern conflict
	for path, want := range map[string]int{"/api/qualities": 200, "/": 200, "/api/library": 200} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestSongsAPI_Library(t *testing.T) {
	e := newSongsEnv(t)
	empty := e.do("alice", "GET", "/api/library", "")
	empty.wantStatus(t, 200)
	if got := string(empty.body); got != `{"folders":[],"songs":[]}`+"\n" {
		t.Errorf("empty library = %q, want non-null empty arrays", got)
	}

	demos := e.newFolder("alice", "demos")
	archive := e.newFolder("alice", "Archive")
	e.newSong("alice", "zebra", 0)
	b := e.newSong("alice", "Banana", demos)
	apple := e.newSong("alice", "apple", archive)
	for _, n := range []string{"Verse", "Chorus", "Bridge"} {
		e.newProg("alice", b, n)
	}

	r := e.do("alice", "GET", "/api/library", "")
	var raw struct {
		Folders []map[string]any
		Songs   []map[string]any
	}
	r.into(t, &raw)
	if len(raw.Songs) != 3 || len(raw.Folders) != 2 {
		t.Fatalf("library = %s", r.body)
	}
	for _, f := range raw.Folders {
		if len(f) != 2 || f["id"] == nil || f["name"] == nil {
			t.Errorf("folder keys = %v, want exactly id and name", f)
		}
	}
	for _, s := range raw.Songs {
		if len(s) != 5 {
			t.Errorf("song keys = %v, want id, name, folderId, updatedAt, progressions", s)
		}
		if v, present := s["folderId"]; !present {
			t.Errorf("song %v has no folderId key (it must be null when unfiled)", s["name"])
		} else if s["name"] == "zebra" && v != nil {
			t.Errorf("unfiled song has folderId %v", v)
		}
		if _, ok := s["progressions"].([]any); !ok {
			t.Errorf("song %v progressions = %v, want an array", s["name"], s["progressions"])
		}
	}

	lib := e.library("alice")
	if lib.Folders[0].Name != "Archive" || lib.Folders[1].Name != "demos" {
		t.Errorf("folders = %+v, want sorted by name", lib.Folders)
	}
	var names []string
	for _, s := range lib.Songs {
		names = append(names, s.Name)
		if _, err := time.Parse(time.RFC3339, s.UpdatedAt); err != nil {
			t.Errorf("updatedAt %q is not RFC 3339: %v", s.UpdatedAt, err)
		}
	}
	if want := []string{"apple", "Banana", "zebra"}; !reflect.DeepEqual(names, want) {
		t.Errorf("songs = %v, want case-insensitive order %v", names, want)
	}
	if lib.Songs[0].ID != apple || *lib.Songs[0].FolderID != archive || *lib.Songs[1].FolderID != demos || lib.Songs[2].FolderID != nil {
		t.Errorf("folderIds wrong: %+v", lib.Songs)
	}
	var progs []string
	for i, p := range lib.Songs[1].Progressions {
		progs = append(progs, p.Name)
		if p.Position != i {
			t.Errorf("progression %q position = %d, want %d", p.Name, p.Position, i)
		}
	}
	if want := []string{"Verse", "Chorus", "Bridge"}; !reflect.DeepEqual(progs, want) {
		t.Errorf("progressions = %v, want position order %v", progs, want)
	}
	if lib.Songs[0].Progressions == nil || len(lib.Songs[0].Progressions) != 0 {
		t.Errorf("a song with no progressions has %v", lib.Songs[0].Progressions)
	}
	if got := e.library("bob"); len(got.Folders)+len(got.Songs) != 0 {
		t.Errorf("another user sees %+v", got)
	}
}

func TestSongsAPI_Folders(t *testing.T) {
	e := newSongsEnv(t)

	r := e.do("alice", "POST", "/api/folders", `{"name":"  Demos  "}`)
	r.wantStatus(t, 201)
	var created map[string]any
	r.into(t, &created)
	if len(created) != 2 || created["name"] != "Demos" || created["id"] == nil {
		t.Errorf("created folder = %s, want exactly id and the trimmed name", r.body)
	}
	id := int64(created["id"].(float64))

	t.Run("invalid names", func(t *testing.T) {
		for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{}`, `{"name":null}`, `{"name":"` + strings.Repeat("a", 101) + `"}`} {
			e.do("alice", "POST", "/api/folders", body).wantError(t, 400, "name must be 1 to 100 characters")
			e.do("alice", "PATCH", fmt.Sprintf("/api/folders/%d", id), body).wantError(t, 400, "name must be 1 to 100 characters")
		}
		e.do("alice", "POST", "/api/folders", `{"name":"`+strings.Repeat("é", 100)+`"}`).wantStatus(t, 201)
	})
	t.Run("duplicate name", func(t *testing.T) {
		e.do("alice", "POST", "/api/folders", `{"name":"Demos"}`).wantError(t, 409, "already exists")
		other := e.newFolder("alice", "Other")
		e.do("alice", "PATCH", fmt.Sprintf("/api/folders/%d", other), `{"name":"Demos"}`).wantError(t, 409, "already exists")
		e.do("bob", "POST", "/api/folders", `{"name":"Demos"}`).wantStatus(t, 201) // another user may reuse it
	})
	t.Run("rename", func(t *testing.T) {
		r := e.do("alice", "PATCH", fmt.Sprintf("/api/folders/%d", id), `{"name":" Renamed "}`)
		r.wantStatus(t, 200)
		if got := string(r.body); got != fmt.Sprintf(`{"id":%d,"name":"Renamed"}`, id)+"\n" {
			t.Errorf("rename response = %q", got)
		}
		e.do("alice", "PATCH", fmt.Sprintf("/api/folders/%d", id), `{"name":"Renamed"}`).wantStatus(t, 200) // same name again
		e.do("alice", "PATCH", fmt.Sprintf("/api/folders/%d", id+1000), `{"name":"x"}`).wantError(t, 404, "folder not found")
	})
	t.Run("delete unfiles its songs", func(t *testing.T) {
		f := e.newFolder("alice", "Doomed")
		s := e.newSong("alice", "Survivor", f)
		p := e.newProg("alice", s, "Verse")
		r := e.do("alice", "DELETE", fmt.Sprintf("/api/folders/%d", f), "")
		r.wantStatus(t, 204)
		if len(r.body) != 0 {
			t.Errorf("204 has a body: %q", r.body)
		}
		for _, folder := range e.library("alice").Folders {
			if folder.ID == f {
				t.Error("folder still listed")
			}
		}
		found := false
		for _, song := range e.library("alice").Songs {
			if song.ID == s {
				found = true
				if song.FolderID != nil || len(song.Progressions) != 1 {
					t.Errorf("song after its folder was deleted = %+v", song)
				}
			}
		}
		if !found {
			t.Error("deleting the folder deleted its song")
		}
		e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", p), "").wantStatus(t, 200)
		e.do("alice", "DELETE", fmt.Sprintf("/api/folders/%d", f), "").wantError(t, 404, "folder not found")
	})
	t.Run("limit", func(t *testing.T) {
		e := newSongsEnv(t)
		var first int64
		for i := range store.MaxFolders {
			id := e.newFolder("alice", fmt.Sprintf("f%03d", i))
			if i == 0 {
				first = id
			}
		}
		e.do("alice", "POST", "/api/folders", `{"name":"one too many"}`).wantError(t, 409, "at most 200 folders")
		e.do("bob", "POST", "/api/folders", `{"name":"fine"}`).wantStatus(t, 201)
		e.do("alice", "DELETE", fmt.Sprintf("/api/folders/%d", first), "").wantStatus(t, 204)
		e.do("alice", "POST", "/api/folders", `{"name":"room again"}`).wantStatus(t, 201)
	})
}

func TestSongsAPI_Songs(t *testing.T) {
	e := newSongsEnv(t)
	folder := e.newFolder("alice", "F")
	otherFolder := e.newFolder("alice", "G")

	t.Run("create", func(t *testing.T) {
		r := e.do("alice", "POST", "/api/songs", `{"name":" Plain "}`)
		r.wantStatus(t, 201)
		var m map[string]any
		r.into(t, &m)
		if len(m) != 3 || m["name"] != "Plain" || m["folderId"] != nil || m["id"] == nil {
			t.Errorf("created song = %s, want exactly id, trimmed name and folderId null", r.body)
		}
		r = e.do("alice", "POST", "/api/songs", fmt.Sprintf(`{"name":"Filed","folderId":%d}`, folder))
		r.wantStatus(t, 201)
		if want := fmt.Sprintf(`"folderId":%d}`, folder); !strings.Contains(string(r.body), want) {
			t.Errorf("created song = %s, want %s", r.body, want)
		}
		e.do("alice", "POST", "/api/songs", `{"name":"Explicit null","folderId":null}`).wantStatus(t, 201)
		e.do("alice", "POST", "/api/songs", `{"name":"Plain"}`).wantStatus(t, 201) // song names need not be unique
	})
	t.Run("invalid", func(t *testing.T) {
		for _, body := range []string{`{}`, `{"name":""}`, `{"name":"  "}`, `{"name":"` + strings.Repeat("a", 101) + `"}`} {
			e.do("alice", "POST", "/api/songs", body).wantError(t, 400, "name must be 1 to 100 characters")
		}
		for _, folderID := range []string{`"1"`, `1.5`, `true`, `[1]`, `{}`} {
			e.do("alice", "POST", "/api/songs", `{"name":"x","folderId":`+folderID+`}`).wantError(t, 400, "body")
		}
		e.do("alice", "POST", "/api/songs", fmt.Sprintf(`{"name":"x","folderId":%d}`, folder+1000)).wantError(t, 404, "folder not found")
	})
	t.Run("patch leaves absent fields alone", func(t *testing.T) {
		id := e.newSong("alice", "Patchy", folder)
		path := fmt.Sprintf("/api/songs/%d", id)

		r := e.do("alice", "PATCH", path, `{"name":" Renamed "}`)
		r.wantStatus(t, 200)
		if want := fmt.Sprintf(`{"id":%d,"name":"Renamed","folderId":%d}`, id, folder) + "\n"; string(r.body) != want {
			t.Errorf("rename only = %q, want %q (folder untouched)", r.body, want)
		}
		r = e.do("alice", "PATCH", path, fmt.Sprintf(`{"folderId":%d}`, otherFolder))
		if want := fmt.Sprintf(`{"id":%d,"name":"Renamed","folderId":%d}`, id, otherFolder) + "\n"; r.code != 200 || string(r.body) != want {
			t.Errorf("move = %d %q, want %q (name untouched)", r.code, r.body, want)
		}
		r = e.do("alice", "PATCH", path, `{"folderId":null}`)
		if want := fmt.Sprintf(`{"id":%d,"name":"Renamed","folderId":null}`, id) + "\n"; r.code != 200 || string(r.body) != want {
			t.Errorf("unfile = %d %q, want %q", r.code, r.body, want)
		}
		r = e.do("alice", "PATCH", path, `{}`)
		if want := fmt.Sprintf(`{"id":%d,"name":"Renamed","folderId":null}`, id) + "\n"; r.code != 200 || string(r.body) != want {
			t.Errorf("empty patch = %d %q, want no change %q", r.code, r.body, want)
		}
		r = e.do("alice", "PATCH", path, fmt.Sprintf(`{"name":"Both","folderId":%d}`, folder))
		if want := fmt.Sprintf(`{"id":%d,"name":"Both","folderId":%d}`, id, folder) + "\n"; r.code != 200 || string(r.body) != want {
			t.Errorf("name and folder = %d %q, want %q", r.code, r.body, want)
		}
		// What the library reports matches.
		for _, s := range e.library("alice").Songs {
			if s.ID == id && (s.Name != "Both" || s.FolderID == nil || *s.FolderID != folder) {
				t.Errorf("library song = %+v", s)
			}
		}
	})
	t.Run("patch refusals leave the song alone", func(t *testing.T) {
		id := e.newSong("alice", "Steady", folder)
		path := fmt.Sprintf("/api/songs/%d", id)
		e.do("alice", "PATCH", path, `{"name":""}`).wantError(t, 400, "name must be")
		e.do("alice", "PATCH", path, `{"name":"  ","folderId":null}`).wantError(t, 400, "name must be")
		e.do("alice", "PATCH", path, `{"folderId":"1"}`).wantError(t, 400, "folderId must be an integer or null")
		e.do("alice", "PATCH", path, `{"folderId":1.5}`).wantError(t, 400, "folderId must be an integer or null")
		e.do("alice", "PATCH", path, fmt.Sprintf(`{"folderId":%d}`, folder+1000)).wantError(t, 404, "folder not found")
		e.do("alice", "PATCH", fmt.Sprintf("/api/songs/%d", id+1000), `{"name":"x"}`).wantError(t, 404, "song not found")
		for _, s := range e.library("alice").Songs {
			if s.ID == id && (s.Name != "Steady" || s.FolderID == nil || *s.FolderID != folder) {
				t.Errorf("a refused patch changed the song: %+v", s)
			}
		}
	})
	t.Run("delete cascades to progressions", func(t *testing.T) {
		id := e.newSong("alice", "Doomed", folder)
		p := e.newProg("alice", id, "Verse")
		survivor := e.newProg("alice", e.newSong("alice", "Survivor", 0), "Verse")
		r := e.do("alice", "DELETE", fmt.Sprintf("/api/songs/%d", id), "")
		r.wantStatus(t, 204)
		if len(r.body) != 0 {
			t.Errorf("204 has a body: %q", r.body)
		}
		e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", p), "").wantError(t, 404, "progression not found")
		e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", survivor), "").wantStatus(t, 200)
		e.do("alice", "DELETE", fmt.Sprintf("/api/songs/%d", id), "").wantError(t, 404, "song not found")
	})
	t.Run("limit", func(t *testing.T) {
		e := newSongsEnv(t)
		var first int64
		for i := range store.MaxSongs {
			id := e.newSong("alice", fmt.Sprintf("s%03d", i), 0)
			if i == 0 {
				first = id
			}
		}
		e.do("alice", "POST", "/api/songs", `{"name":"one too many"}`).wantError(t, 409, "at most 500 songs")
		e.do("bob", "POST", "/api/songs", `{"name":"fine"}`).wantStatus(t, 201)
		e.do("alice", "DELETE", fmt.Sprintf("/api/songs/%d", first), "").wantStatus(t, 204)
		e.do("alice", "POST", "/api/songs", `{"name":"room again"}`).wantStatus(t, 201)
	})
}

func TestSongsAPI_CreateProgression(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	path := fmt.Sprintf("/api/songs/%d/progressions", song)

	var ids []int64
	for i, name := range []string{"Verse", "Chorus", "Bridge"} {
		r := e.do("alice", "POST", path, progBody(" "+name+" ", songsData))
		r.wantStatus(t, 201)
		var m map[string]any
		r.into(t, &m)
		if len(m) != 3 || m["name"] != name || m["position"] != float64(i) || m["id"] == nil {
			t.Errorf("created progression = %s, want exactly id, trimmed name and position %d", r.body, i)
		}
		ids = append(ids, int64(m["id"].(float64)))
	}

	// GET returns the stored data exactly as validated, with the documented shape.
	r := e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", ids[1]), "")
	r.wantStatus(t, 200)
	want := fmt.Sprintf(`{"id":%d,"songId":%d,"name":"Chorus","position":1,"data":%s}`, ids[1], song, songsData) + "\n"
	if string(r.body) != want {
		t.Errorf("GET progression =\n%s\nwant\n%s", r.body, want)
	}

	t.Run("invalid body", func(t *testing.T) {
		for _, name := range []string{"", "   ", strings.Repeat("a", 101)} {
			e.do("alice", "POST", path, progBody(name, songsData)).wantError(t, 400, "name must be 1 to 100 characters")
		}
		e.do("alice", "POST", path, `{"data":`+songsData+`}`).wantError(t, 400, "name must be")
		e.do("alice", "POST", path, `{"name":"x"}`).wantError(t, 400, "data: required")
		e.do("alice", "POST", path, `{"name":"x","data":null}`).wantError(t, 400, "data: required")
	})
	t.Run("missing or foreign song", func(t *testing.T) {
		e.do("alice", "POST", fmt.Sprintf("/api/songs/%d/progressions", song+1000), progBody("x", songsData)).wantError(t, 404, "song not found")
		e.do("bob", "POST", path, progBody("x", songsData)).wantError(t, 404, "song not found")
	})
	t.Run("limit", func(t *testing.T) {
		full := e.newSong("alice", "Full", 0)
		other := e.newSong("alice", "Other", 0)
		for i := range store.MaxProgressions {
			e.newProg("alice", full, fmt.Sprintf("p%d", i))
		}
		e.do("alice", "POST", fmt.Sprintf("/api/songs/%d/progressions", full), progBody("one too many", songsData)).wantError(t, 409, "at most 50 progressions")
		e.newProg("alice", other, "another song is unaffected")
	})
}

func TestSongsAPI_ProgressionLimitPerUser(t *testing.T) {
	e := newSongsEnv(t)
	var first int64
	for i := range store.MaxProgressionsPerUser / store.MaxProgressions {
		song := e.newSong("alice", fmt.Sprintf("s%d", i), 0)
		for j := range store.MaxProgressions {
			if id := e.newProg("alice", song, fmt.Sprintf("p%d", j)); first == 0 { // the 500th add succeeds
				first = id
			}
		}
	}
	extra := e.newSong("alice", "Extra", 0) // empty: only the per-user cap can refuse it
	path := fmt.Sprintf("/api/songs/%d/progressions", extra)
	const want = "at most 500 progressions across all your songs"
	e.do("alice", "POST", path, progBody("501st", songsData)).wantError(t, 409, want)

	bob := e.newSong("bob", "Bob's", 0)
	e.newProg("bob", bob, "unaffected")
	e.do("alice", "POST", path, progBody("still full", songsData)).wantError(t, 409, want)

	e.do("alice", "DELETE", fmt.Sprintf("/api/progressions/%d", first), "").wantStatus(t, 204)
	e.do("alice", "POST", path, progBody("uses the freed slot", songsData)).wantStatus(t, 201)
	e.do("alice", "POST", path, progBody("full again", songsData)).wantError(t, 409, want)
}

func TestSongsAPI_ProgressionDataValidation(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	path := fmt.Sprintf("/api/songs/%d/progressions", song)
	// The same data is also refused when updating.
	prog := e.newProg("alice", song, "Existing")
	put := fmt.Sprintf("/api/progressions/%d", prog)

	long := strings.Repeat("n", 21)
	bad := []struct {
		name   string
		mutate func(d, s map[string]any)
		want   string
	}{
		{"missing tuning", func(d, s map[string]any) { delete(d, "tuning") }, "tuning: required"},
		{"tuning without octaves", func(d, s map[string]any) { d["tuning"] = "EADGBE" }, "tuning"},
		{"five strings", func(d, s map[string]any) { d["tuning"] = "E2,A2,D3,G3,B3" }, "want 6 pitches"},
		{"tuning too far from standard", func(d, s map[string]any) { d["tuning"] = "E1,A2,D3,G3,B3,E4" }, "tuning"},
		{"tuning of the wrong type", func(d, s map[string]any) { d["tuning"] = 5 }, "tuning"},
		{"missing openMax", func(d, s map[string]any) { delete(d, "openMax") }, "openMax: required"},
		{"negative openMax", func(d, s map[string]any) { d["openMax"] = -1 }, "openMax"},
		{"openMax past the last fret", func(d, s map[string]any) { d["openMax"] = 23 }, "openMax"},
		{"fractional openMax", func(d, s map[string]any) { d["openMax"] = 5.5 }, "openMax"},
		{"openMax as a string", func(d, s map[string]any) { d["openMax"] = "5" }, "openMax"},
		{"bpm too low", func(d, s map[string]any) { d["bpm"] = 19 }, "bpm"},
		{"bpm too high", func(d, s map[string]any) { d["bpm"] = 301 }, "bpm"},
		{"missing bpm", func(d, s map[string]any) { delete(d, "bpm") }, "bpm"},
		{"bpm as a string", func(d, s map[string]any) { d["bpm"] = "90" }, "bpm"},
		{"zero beats", func(d, s map[string]any) { d["beats"] = 0 }, "beats"},
		{"too many beats", func(d, s map[string]any) { d["beats"] = 17 }, "beats"},
		{"missing beats", func(d, s map[string]any) { delete(d, "beats") }, "beats"},
		{"no steps", func(d, s map[string]any) { d["steps"] = []any{} }, "want 1 to 64 steps, got 0"},
		{"null steps", func(d, s map[string]any) { d["steps"] = nil }, "want 1 to 64 steps, got 0"},
		{"missing steps", func(d, s map[string]any) { delete(d, "steps") }, "want 1 to 64 steps, got 0"},
		{"steps that are not an array", func(d, s map[string]any) { d["steps"] = "C" }, "steps"},
		{"65 steps", func(d, s map[string]any) { d["steps"] = songsStepList(65, s) }, "want 1 to 64 steps, got 65"},
		{"step that is not an object", func(d, s map[string]any) { d["steps"] = []any{"C"} }, "steps"},
		{"missing root", func(d, s map[string]any) { delete(s, "root") }, "step 1: root: required"},
		{"unknown root", func(d, s map[string]any) { s["root"] = "H" }, "step 1: root"},
		{"missing quality", func(d, s map[string]any) { delete(s, "quality") }, "step 1: quality: required"},
		{"unknown quality", func(d, s map[string]any) { s["quality"] = "nope" }, "step 1: quality"},
		{"unknown inversion", func(d, s map[string]any) { s["inversion"] = "9th" }, "step 1: inversion"},
		{"root with a leading space", func(d, s map[string]any) { s["root"] = " C" }, "step 1: root"},
		{"root with too many accidentals", func(d, s map[string]any) { s["root"] = "C#########" }, "step 1: root: want at most 8 bytes"},
		{"root padded to 100 KiB", func(d, s map[string]any) { s["root"] = "C" + strings.Repeat("#", 100<<10) }, "step 1: root: want at most 8 bytes"},
		{"quality in another case", func(d, s map[string]any) { s["quality"] = "Maj7" }, "step 1: quality"},
		{"quality with a trailing space", func(d, s map[string]any) { s["quality"] = "maj7 " }, "step 1: quality"},
		{"inversion in another case", func(d, s map[string]any) { s["inversion"] = "Root" }, "step 1: inversion"},
		{"inversion with a leading space", func(d, s map[string]any) { s["inversion"] = " any" }, "step 1: inversion"},
		{"inversion the quality lacks", func(d, s map[string]any) { s["quality"], s["inversion"] = "maj", "7th" }, "step 1: inversion"},
		{"the second step is bad", func(d, s map[string]any) {
			second := map[string]any{}
			for k, v := range s {
				second[k] = v
			}
			second["root"] = "H"
			d["steps"] = []any{s, second}
		}, "step 2: root"},
		{"name too long", func(d, s map[string]any) { s["name"] = long }, "step 1: name"},
		{"name that is not a string", func(d, s map[string]any) { s["name"] = 5 }, "name"},
		{"numeral too long", func(d, s map[string]any) { s["numeral"] = long }, "step 1: numeral"},
		{"short fingering", func(d, s map[string]any) { s["fingering"] = "x3200" }, "step 1: fingering"},
		{"long fingering", func(d, s map[string]any) { s["fingering"] = "x320000" }, "step 1: fingering"},
		{"fingering with a bad character", func(d, s map[string]any) { s["fingering"] = "x3200z" }, "step 1: fingering"},
		{"empty fingering", func(d, s map[string]any) { s["fingering"] = "" }, "step 1: fingering"},
		{"fret past the end", func(d, s map[string]any) { s["fingering"] = "x-3-2-0-0-23" }, "step 1: fingering"},
		{"fingering that is not a string", func(d, s map[string]any) { s["fingering"] = 320001 }, "fingering"},
		{"unknown data field", func(d, s map[string]any) { d["capo"] = 2 }, "capo"},
		{"unknown step field", func(d, s map[string]any) { s["id"] = 7 }, `"id"`},
	}
	for _, tt := range bad {
		data := songsVariant(tt.mutate)
		e.do("alice", "POST", path, progBody("x", data)).wantError(t, 400, tt.want)
		e.do("alice", "PUT", put, `{"data":`+data+`}`).wantError(t, 400, tt.want)
	}
	for _, data := range []string{`[]`, `"x"`, `5`, `true`, `{`} {
		body := `{"name":"x","data":` + data + `}`
		if r := e.do("alice", "POST", path, body); r.code != 400 {
			t.Errorf("data %s accepted: %d %s", data, r.code, r.body)
		}
	}
	if lib := e.library("alice"); len(lib.Songs[0].Progressions) != 1 {
		t.Errorf("invalid data was stored: %+v", lib.Songs[0].Progressions)
	}

	t.Run("edges that are valid", func(t *testing.T) {
		valid := []struct {
			name   string
			mutate func(d, s map[string]any)
		}{
			{"openMax 0", func(d, s map[string]any) { d["openMax"] = 0 }},
			{"openMax 22", func(d, s map[string]any) { d["openMax"] = 22 }},
			{"bpm 20", func(d, s map[string]any) { d["bpm"] = 20 }},
			{"fractional bpm (the UI does not round it)", func(d, s map[string]any) { d["bpm"] = 90.5 }},
			{"bpm 300", func(d, s map[string]any) { d["bpm"] = 300 }},
			{"beats 1", func(d, s map[string]any) { d["beats"] = 1 }},
			{"beats 16", func(d, s map[string]any) { d["beats"] = 16 }},
			{"1 step", func(d, s map[string]any) { d["steps"] = songsStepList(1, s) }},
			{"64 steps", func(d, s map[string]any) { d["steps"] = songsStepList(64, s) }},
			{"20 character name and numeral", func(d, s map[string]any) { s["name"], s["numeral"] = strings.Repeat("n", 20), strings.Repeat("é", 20) }},
			{"empty name", func(d, s map[string]any) { s["name"] = "" }},
			{"null numeral and fingering (no playable shape)", func(d, s map[string]any) { s["numeral"], s["fingering"] = nil, nil }},
			{"dashed fingering", func(d, s map[string]any) { s["fingering"] = "x-10-12-12-11-10" }},
			{"fingering of open strings", func(d, s map[string]any) { s["fingering"] = "000000" }},
			{"every inversion a triad offers", func(d, s map[string]any) { s["quality"], s["inversion"] = "maj", "5th" }},
			{"a seventh chord in the 7th", func(d, s map[string]any) { s["quality"], s["inversion"] = "maj7", "7th" }},
			{"drop D", func(d, s map[string]any) { d["tuning"] = "D2,A2,D3,G3,B3,E4" }},
		}
		for _, tt := range valid {
			r := e.do("alice", "POST", path, progBody("x", songsVariant(tt.mutate)))
			if r.code != 201 {
				t.Errorf("%s: %d %s", tt.name, r.code, r.body)
			}
		}
	})
	t.Run("stored data is rebuilt from the known fields", func(t *testing.T) {
		// Omitted numeral and fingering become null, an omitted inversion becomes "any", and
		// whitespace and key order are normalised.
		loose := `{ "steps" : [ {"quality":"min", "root":"A", "name":"Am"} ], "beats":3, "bpm":120,
			"openMax":2, "tuning":"E2,A2,D3,G3,B3,E4" }`
		id := e.create("alice", path, progBody("Loose", loose))
		r := e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", id), "")
		r.wantStatus(t, 200)
		var got struct{ Data json.RawMessage }
		r.into(t, &got)
		want := `{"tuning":"E2,A2,D3,G3,B3,E4","openMax":2,"bpm":120,"beats":3,"steps":[{"root":"A","quality":"min","inversion":"any","name":"Am","numeral":null,"fingering":null}]}`
		if string(got.Data) != want {
			t.Errorf("stored data = %s\nwant %s", got.Data, want)
		}
	})
	t.Run("a fractional bpm is kept as is", func(t *testing.T) {
		id := e.create("alice", path, progBody("Fractional", songsVariant(func(d, s map[string]any) { d["bpm"] = 90.5 })))
		r := e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", id), "")
		if !strings.Contains(string(r.body), `"bpm":90.5,"beats":4,`) {
			t.Errorf("stored data = %s", r.body)
		}
	})
	t.Run("tuning and root are rebuilt from the parsed values", func(t *testing.T) {
		// What the UI sends is stored as is; anything else that parses is spelled the way the UI spells it.
		const flats = "E♭2,A♭2,C♯3,F♯3,B♭3,E♭4"
		cases := []struct{ name, tuning, root, wantTuning, wantRoot string }{
			{"as the UI sends it", flats, "F♯♯", flats, "F♯♯"},
			{"ASCII accidentals", "Eb2,Ab2,C#3,F#3,Bb3,Eb4", "F##", flats, "F♯♯"},
			{"ASCII flat root", "E2,A2,D3,G3,B3,E4", "Bb", "E2,A2,D3,G3,B3,E4", "B♭"},
			{"spaces", " E2 ,\tA2 ,D3,G3,B3,E4 ", "C", "E2,A2,D3,G3,B3,E4", "C"},
			{"leading zeros in the octaves", "E02,A02,D03,G03,B03,E04", "C", "E2,A2,D3,G3,B3,E4", "C"},
		}
		for _, tt := range cases {
			data := songsVariant(func(d, s map[string]any) { d["tuning"], s["root"] = tt.tuning, tt.root })
			id := e.create("alice", path, progBody(tt.name, data))
			r := e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", id), "")
			var got struct {
				Data struct {
					Tuning string
					Steps  []struct{ Root string }
				}
			}
			r.into(t, &got)
			if got.Data.Tuning != tt.wantTuning || got.Data.Steps[0].Root != tt.wantRoot {
				t.Errorf("%s: stored tuning %q and root %q, want %q and %q", tt.name, got.Data.Tuning, got.Data.Steps[0].Root, tt.wantTuning, tt.wantRoot)
			}
		}
	})
	t.Run("padding is not stored", func(t *testing.T) {
		padded := songsVariant(func(d, s map[string]any) {
			d["tuning"] = "E2,A2,D3,G3,B3,E4" + strings.Repeat(" ", 70<<10) // valid: each pitch is trimmed
		})
		id := e.create("alice", path, progBody("Padded", padded))
		r := e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", id), "")
		var got struct{ Data json.RawMessage }
		r.into(t, &got)
		if string(got.Data) != songsData {
			t.Errorf("stored data = %.200s, want the compact %s", got.Data, songsData)
		}
		e.do("alice", "PUT", put, `{"data":`+padded+`}`).wantStatus(t, 200)
	})
	t.Run("the largest valid data fits the store's limit", func(t *testing.T) {
		// 64 steps with the longest root and the most escaped labels.
		big := songsVariant(func(d, s map[string]any) {
			s["root"], s["quality"], s["inversion"] = "C#######", "maj13", "7th"
			s["name"], s["numeral"], s["fingering"] = strings.Repeat("<", 20), strings.Repeat("<", 20), "x-10-12-12-11-10"
			d["steps"] = songsStepList(64, s)
		})
		e.do("alice", "POST", path, progBody("Largest", big)).wantStatus(t, 201)
	})
}

func songsStepList(n int, step map[string]any) []any {
	steps := make([]any, n)
	for i := range steps {
		steps[i] = step
	}
	return steps
}

func TestSongsAPI_UpdateProgression(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	e.newProg("alice", song, "First")
	prog := e.newProg("alice", song, "Verse")
	path := fmt.Sprintf("/api/progressions/%d", prog)
	other := songsOther

	get := func() (name string, position int, data string) {
		r := e.do("alice", "GET", path, "")
		r.wantStatus(t, 200)
		var m struct {
			Name     string
			Position int
			Data     json.RawMessage
		}
		r.into(t, &m)
		return m.Name, m.Position, string(m.Data)
	}

	r := e.do("alice", "PUT", path, `{"name":" Intro "}`)
	r.wantStatus(t, 200)
	want := fmt.Sprintf(`{"id":%d,"songId":%d,"name":"Intro","position":1,"data":%s}`, prog, song, songsData) + "\n"
	if string(r.body) != want {
		t.Errorf("rename only =\n%s\nwant (same shape as GET, data untouched)\n%s", r.body, want)
	}
	e.do("alice", "PUT", path, `{"data":`+other+`}`).wantStatus(t, 200)
	if name, pos, data := get(); name != "Intro" || pos != 1 || data != other {
		t.Errorf("after data-only update: %q %d %s", name, pos, data)
	}
	r = e.do("alice", "PUT", path, `{"name":"Both","data":`+songsData+`}`)
	r.wantStatus(t, 200)
	if name, pos, data := get(); name != "Both" || pos != 1 || data != songsData {
		t.Errorf("after name and data update: %q %d %s", name, pos, data)
	}
	before := string(e.do("alice", "GET", path, "").body)
	if r := e.do("alice", "PUT", path, `{}`); r.code != 200 || string(r.body) != before {
		t.Errorf("empty update = %d %s, want a no-op returning %s", r.code, r.body, before)
	}

	t.Run("refusals change nothing", func(t *testing.T) {
		e.do("alice", "PUT", path, `{"name":""}`).wantError(t, 400, "name must be")
		e.do("alice", "PUT", path, `{"name":"   ","data":`+other+`}`).wantError(t, 400, "name must be")
		e.do("alice", "PUT", path, `{"name":"Valid","data":null}`).wantError(t, 400, "data: required")
		e.do("alice", "PUT", path, `{"data":{}}`).wantError(t, 400, "tuning")
		e.do("alice", "PUT", path, `{"unknown":1}`).wantError(t, 400, "unknown")
		e.do("alice", "PUT", path, `{"name":"x"} trailing`).wantError(t, 400, "body")
		e.do("alice", "PUT", fmt.Sprintf("/api/progressions/%d", prog+1000), `{"name":"x"}`).wantError(t, 404, "progression not found")
		e.do("bob", "PUT", path, `{"name":"Hijacked","data":`+other+`}`).wantError(t, 404, "progression not found")
		if name, pos, data := get(); name != "Both" || pos != 1 || data != songsData {
			t.Errorf("a refused update changed the progression: %q %d %s", name, pos, data)
		}
	})
}

func TestSongsAPI_GetProgression(t *testing.T) {
	e := newSongsEnv(t)
	f := e.fixture("alice")
	path := fmt.Sprintf("/api/progressions/%d", f.prog)
	r := e.do("alice", "GET", path, "")
	r.wantStatus(t, 200)
	var m map[string]any
	r.into(t, &m)
	if len(m) != 5 {
		t.Errorf("progression keys = %v, want id, songId, name, position, data", m)
	}
	if _, ok := m["data"].(map[string]any); !ok {
		t.Errorf("data = %v, want an object", m["data"])
	}
	e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", f.prog+1000), "").wantError(t, 404, "progression not found")
	e.do("bob", "GET", path, "").wantError(t, 404, "progression not found")
}

func TestSongsAPI_ReorderProgressions(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	foreignSong := e.newSong("alice", "Elsewhere", 0)
	a, b, c := e.newProg("alice", song, "A"), e.newProg("alice", song, "B"), e.newProg("alice", song, "C")
	foreign := e.newProg("alice", foreignSong, "X")
	bobs := e.newProg("bob", e.newSong("bob", "Bob's", 0), "Y")
	path := fmt.Sprintf("/api/songs/%d/progressions/order", song)
	order := func() []string {
		var names []string
		for _, s := range e.library("alice").Songs {
			if s.ID == song {
				for i, p := range s.Progressions {
					if p.Position != i {
						t.Errorf("position %d at index %d", p.Position, i)
					}
					names = append(names, p.Name)
				}
			}
		}
		return names
	}

	r := e.do("alice", "PUT", path, fmt.Sprintf(`{"ids":[%d,%d,%d]}`, c, a, b))
	r.wantStatus(t, 204)
	if len(r.body) != 0 {
		t.Errorf("204 has a body: %q", r.body)
	}
	if got := order(); !reflect.DeepEqual(got, []string{"C", "A", "B"}) {
		t.Errorf("order after reorder = %v", got)
	}
	e.do("alice", "GET", fmt.Sprintf("/api/progressions/%d", c), "").wantStatus(t, 200)

	bad := map[string]string{
		"missing an id":     fmt.Sprintf(`{"ids":[%d,%d]}`, a, b),
		"an extra id":       fmt.Sprintf(`{"ids":[%d,%d,%d,%d]}`, a, b, c, foreign),
		"a duplicate":       fmt.Sprintf(`{"ids":[%d,%d,%d]}`, a, a, b),
		"another song's id": fmt.Sprintf(`{"ids":[%d,%d,%d]}`, a, b, foreign),
		"another user's id": fmt.Sprintf(`{"ids":[%d,%d,%d]}`, a, b, bobs),
		"an unknown id":     fmt.Sprintf(`{"ids":[%d,%d,%d]}`, a, b, c+1000),
		"empty":             `{"ids":[]}`,
		"null ids":          `{"ids":null}`,
		"no ids":            `{}`,
		"zero":              `{"ids":[0,0,0]}`,
	}
	for name, body := range bad {
		r := e.do("alice", "PUT", path, body)
		if r.code != 400 {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.body)
		}
		if got := order(); !reflect.DeepEqual(got, []string{"C", "A", "B"}) {
			t.Errorf("%s: a refused reorder changed the order to %v", name, got)
		}
	}
	e.do("alice", "PUT", path, fmt.Sprintf(`{"ids":[%d,%d,%d]}`, a, b, c)).wantStatus(t, 204)
	for _, body := range []string{`{"ids":"1,2,3"}`, `{"ids":[1.5]}`, `{"ids":["1"]}`, `{"ids":[1],"extra":1}`, `[1,2,3]`, ``, `nope`} {
		if r := e.do("alice", "PUT", path, body); r.code != 400 {
			t.Errorf("body %q: %d %s, want 400", body, r.code, r.body)
		}
	}
	// An empty song reorders with an empty list.
	empty := e.newSong("alice", "Empty", 0)
	e.do("alice", "PUT", fmt.Sprintf("/api/songs/%d/progressions/order", empty), `{"ids":[]}`).wantStatus(t, 204)
	// Not the owner: 404 even with the right ids; a missing song: 404.
	e.do("bob", "PUT", path, fmt.Sprintf(`{"ids":[%d,%d,%d]}`, b, a, c)).wantError(t, 404, "song not found")
	e.do("alice", "PUT", fmt.Sprintf("/api/songs/%d/progressions/order", song+1000), `{"ids":[]}`).wantError(t, 404, "song not found")
	if got := order(); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Errorf("final order = %v", got)
	}
}

func TestSongsAPI_DeleteProgressionRepacksPositions(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	other := e.newSong("alice", "Other", 0)
	ids := map[string]int64{}
	for _, n := range []string{"A", "B", "C", "D"} {
		ids[n] = e.newProg("alice", song, n)
	}
	e.newProg("alice", other, "X")
	e.newProg("alice", other, "Y")
	positions := func(songID int64) map[string]int {
		out := map[string]int{}
		for _, s := range e.library("alice").Songs {
			if s.ID == songID {
				for _, p := range s.Progressions {
					out[p.Name] = p.Position
				}
			}
		}
		return out
	}

	r := e.do("alice", "DELETE", fmt.Sprintf("/api/progressions/%d", ids["B"]), "")
	r.wantStatus(t, 204)
	if len(r.body) != 0 {
		t.Errorf("204 has a body: %q", r.body)
	}
	if got, want := positions(song), map[string]int{"A": 0, "C": 1, "D": 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("positions after deleting the middle = %v, want %v", got, want)
	}
	e.do("alice", "DELETE", fmt.Sprintf("/api/progressions/%d", ids["A"]), "").wantStatus(t, 204)
	if got, want := positions(song), map[string]int{"C": 0, "D": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("positions after deleting the first = %v, want %v", got, want)
	}
	if got, want := positions(other), map[string]int{"X": 0, "Y": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("another song's positions = %v, want %v", got, want)
	}
	// The next one is appended after the re-packed list.
	r = e.do("alice", "POST", fmt.Sprintf("/api/songs/%d/progressions", song), progBody("E", songsData))
	r.wantStatus(t, 201)
	var created struct{ Position int }
	r.into(t, &created)
	if created.Position != 2 {
		t.Errorf("new progression position = %d, want 2", created.Position)
	}
	e.do("alice", "DELETE", fmt.Sprintf("/api/progressions/%d", ids["B"]), "").wantError(t, 404, "progression not found")
}

func TestSongsAPI_RequestBodies(t *testing.T) {
	e := newSongsEnv(t)
	song := e.newSong("alice", "Song", 0)
	progPath := fmt.Sprintf("/api/songs/%d/progressions", song)

	t.Run("oversize bodies are 413", func(t *testing.T) {
		huge := strings.Repeat("a", 256<<10)
		e.do("alice", "POST", "/api/folders", `{"name":"`+huge+`"}`).wantError(t, 413, "too large")
		e.do("alice", "POST", "/api/songs", `{"name":"`+huge+`"}`).wantError(t, 413, "too large")
		e.do("alice", "POST", progPath, `{"name":"x","data":`+songsData+`,"pad":"`+huge+`"}`).wantError(t, 413, "too large")
		e.do("alice", "PUT", fmt.Sprintf("/api/songs/%d/progressions/order", song), `{"ids":[`+strings.Repeat("1,", 200<<10)+`1]}`).wantError(t, 413, "too large")
	})
	t.Run("a long name just under the cap is a validation error, not a 413", func(t *testing.T) {
		e.do("alice", "POST", "/api/folders", `{"name":"`+strings.Repeat("a", 200<<10)+`"}`).wantError(t, 400, "name must be 1 to 100 characters")
	})
	t.Run("malformed bodies", func(t *testing.T) {
		bodies := []string{
			``, `null`, `nope`, `{`, `[]`, `"x"`, `5`,
			`{"name":5}`, `{"name":["x"]}`, `{"name":"x"} {"name":"y"}`, `{"name":"x"} }`, `{"name":"x"}garbage`,
			`{"name":"x","unknown":true}`, `{"name":"x","id":1}`, `{"Name":"x","NAME":"y","extra":1}`,
		}
		for _, body := range bodies {
			for _, path := range []string{"/api/folders", "/api/songs"} {
				if r := e.do("alice", "POST", path, body); r.code != 400 {
					t.Errorf("POST %s %q = %d %s, want 400", path, body, r.code, r.body)
				}
			}
		}
		if n := len(e.library("alice").Folders); n != 0 {
			t.Errorf("%d folders created from malformed bodies", n)
		}
	})
	t.Run("unknown fields are named", func(t *testing.T) {
		e.do("alice", "POST", "/api/folders", `{"name":"x","color":"red"}`).wantError(t, 400, `unknown field "color"`)
		e.do("alice", "PATCH", fmt.Sprintf("/api/songs/%d", song), `{"color":"red"}`).wantError(t, 400, `unknown field "color"`)
		e.do("alice", "POST", progPath, `{"name":"x","data":`+songsData+`,"color":"red"}`).wantError(t, 400, `unknown field "color"`)
	})
	t.Run("names are text, never markup or SQL", func(t *testing.T) {
		evil := `<img src=x onerror=alert(1)>'); DROP TABLE songs;--`
		f := e.newFolder("alice", evil)
		s := e.newSong("alice", evil, f)
		e.newProg("alice", s, evil)
		found := false
		for _, folder := range e.library("alice").Folders {
			found = found || (folder.ID == f && folder.Name == evil)
		}
		if !found {
			t.Error("the name was not kept verbatim")
		}
		if r := e.do("alice", "GET", "/api/library", ""); !strings.Contains(r.header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type = %q", r.header.Get("Content-Type"))
		}
	})
}

func TestSongsAPI_PathIDs(t *testing.T) {
	e := newSongsEnv(t)
	e.fixture("alice")
	ids := []string{"abc", "0", "-1", "01", "+1", "1.5", "1e3", "99999999999999999999999", "%20", "0x1"}
	routes := []struct{ method, path, body string }{
		{"PATCH", "/api/folders/%s", `{"name":"x"}`},
		{"DELETE", "/api/folders/%s", ""},
		{"PATCH", "/api/songs/%s", `{"name":"x"}`},
		{"DELETE", "/api/songs/%s", ""},
		{"POST", "/api/songs/%s/progressions", progBody("x", songsData)},
		{"PUT", "/api/songs/%s/progressions/order", `{"ids":[]}`},
		{"GET", "/api/progressions/%s", ""},
		{"PUT", "/api/progressions/%s", `{"name":"x"}`},
		{"DELETE", "/api/progressions/%s", ""},
	}
	for _, id := range ids {
		for _, rt := range routes {
			e.do("alice", rt.method, fmt.Sprintf(rt.path, id), rt.body).wantError(t, 404, "not found")
		}
	}
	// Signed-out requests are 401 before the id is even looked at.
	e.do("", "GET", "/api/progressions/abc", "").wantError(t, 401, "sign in required")
	if lib := e.library("alice"); len(lib.Folders) != 1 || len(lib.Songs) != 1 || len(lib.Songs[0].Progressions) != 1 {
		t.Errorf("bad ids changed data: %+v", lib)
	}
}

func TestSongsAPI_StoreFailuresAre500WithoutDetails(t *testing.T) {
	var logged bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(orig) })

	e := newSongsEnv(t)
	f := e.fixture("alice")
	e.store.Close() // every later database call fails
	for _, rt := range f.routes() {
		r := e.do("alice", rt.method, rt.path, rt.body)
		r.wantError(t, 500, "internal error")
		if got := string(r.body); got != `{"error":"internal error"}`+"\n" {
			t.Errorf("%s: body = %q leaks detail", rt, got)
		}
	}
	if !strings.Contains(logged.String(), "songs api:") {
		t.Errorf("the failure was not logged: %q", logged.String())
	}
}

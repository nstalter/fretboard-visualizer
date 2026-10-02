package main

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func clearCognitoEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"FRETBOARD_BASE_URL", "COGNITO_ISSUER", "COGNITO_CLIENT_ID", "COGNITO_CLIENT_SECRET", "COGNITO_DOMAIN"} {
		t.Setenv(k, "")
	}
}

func TestSetupAccounts_DisabledByDefault(t *testing.T) {
	clearCognitoEnv(t)
	mux := http.NewServeMux()
	if err := setupAccounts(context.Background(), mux, "localhost:8080", filepath.Join(t.TempDir(), "x.db"), ""); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), `"enabled":false`) {
		t.Errorf("/api/me = %s, want enabled:false", body)
	}
	if res, _ := http.Get(srv.URL + "/api/library"); res.StatusCode != http.StatusNotFound {
		t.Errorf("/api/library without accounts = %d, want 404", res.StatusCode)
	}
}

func TestSetupAccounts_DevUserRules(t *testing.T) {
	clearCognitoEnv(t)
	db := filepath.Join(t.TempDir(), "x.db")
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "example.com:8080", "10.0.0.5:8080", "nonsense"} {
		if err := setupAccounts(context.Background(), http.NewServeMux(), addr, db, "me@example.com"); err == nil {
			t.Errorf("-dev-user accepted on non-loopback addr %q", addr)
		}
	}
	t.Setenv("COGNITO_CLIENT_ID", "abc")
	if err := setupAccounts(context.Background(), http.NewServeMux(), "localhost:8080", db, "me@example.com"); err == nil {
		t.Error("-dev-user accepted alongside Cognito settings")
	}
}

func TestSetupAccounts_PartialCognitoConfigFails(t *testing.T) {
	clearCognitoEnv(t)
	t.Setenv("COGNITO_CLIENT_ID", "abc")
	if err := setupAccounts(context.Background(), http.NewServeMux(), "localhost:8080", filepath.Join(t.TempDir(), "x.db"), ""); err == nil {
		t.Error("partial Cognito config was accepted")
	}
}

func TestSetupAccounts_DevUserSavesAndLoadsASong(t *testing.T) {
	clearCognitoEnv(t)
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		t.Fatal(err)
	}
	mux := newMux(static)
	if err := setupAccounts(context.Background(), mux, "127.0.0.1:8080", filepath.Join(t.TempDir(), "x.db"), "me@example.com"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Origin", srv.URL)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	if code, body := do("GET", "/api/me", ""); code != 200 || !strings.Contains(body, `"authenticated":true`) || !strings.Contains(body, "me@example.com") {
		t.Fatalf("/api/me = %d %s", code, body)
	}
	if code, body := do("POST", "/api/songs", `{"name":"Song"}`); code != 201 {
		t.Fatalf("create song = %d %s", code, body)
	}
	data := `{"tuning":"E2,A2,D3,G3,B3,E4","openMax":5,"bpm":90,"beats":4,"steps":[{"root":"C","quality":"maj","inversion":"any","name":"C","numeral":null,"fingering":"x32010"}]}`
	if code, body := do("POST", "/api/songs/1/progressions", `{"name":"Verse","data":`+data+`}`); code != 201 {
		t.Fatalf("save progression = %d %s", code, body)
	}
	if code, body := do("GET", "/api/library", ""); code != 200 || !strings.Contains(body, `"Verse"`) {
		t.Fatalf("library = %d %s", code, body)
	}
	// A request from another origin must not be able to write.
	req, _ := http.NewRequest("POST", srv.URL+"/api/songs", strings.NewReader(`{"name":"Evil"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin write = %d, want 403", res.StatusCode)
	}
}

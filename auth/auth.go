// Package auth signs users in through Amazon Cognito (OAuth 2.0 authorization
// code flow with PKCE, state and nonce) and keeps them signed in with
// server-side sessions. The Cognito tokens are verified and then discarded;
// the browser only ever holds an opaque random session id.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionCookie = "fv_session"
	loginCookie   = "fv_login"
	sessionTTL    = 30 * 24 * time.Hour
	loginTTL      = 10 * time.Minute
	httpTimeout   = 10 * time.Second
	devSub        = "dev-user"
)

// SessionStore persists sessions. Only the SHA-256 of the session id is ever
// handed to it.
type SessionStore interface {
	CreateSession(ctx context.Context, tokenHash, sub, email string, expires time.Time) error
	GetSession(ctx context.Context, tokenHash string) (sub, email string, ok bool, err error) // ok=false if missing OR expired
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Auth serves the sign-in endpoints and answers who the current user is.
type Auth struct {
	sessions   SessionStore
	oauth      *oauth2.Config
	verifier   *oidc.IDTokenVerifier
	client     *http.Client
	logoutURL  string // Cognito logout endpoint with its query
	secure     bool   // BaseURL is https: the funnel terminates TLS, so r.TLS is useless
	baseOrigin string // lower-case scheme://host of BaseURL

	// The login cookie is signed with a random per-process key, so a login in
	// flight fails across a restart (the user just signs in again).
	loginKey []byte

	dev      bool
	devEmail string
}

// New performs OIDC discovery against cfg.Issuer. An HTTP client attached to
// ctx with oidc.ClientContext is used for all calls to Cognito (discovery,
// keys, code exchange); otherwise a client with a timeout is used.
func New(ctx context.Context, cfg Config, sessions SessionStore) (*Auth, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if !ok {
		client = &http.Client{Timeout: httpTimeout}
	}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: OIDC discovery for %s: %w", cfg.Issuer, err)
	}
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	base, _ := url.Parse(cfg.BaseURL) // validated above
	return &Auth{
		sessions: sessions,
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     endpoint,
			RedirectURL:  cfg.BaseURL + "/auth/callback",
			Scopes:       []string{"openid", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		client:   client,
		logoutURL: cfg.CognitoDomain + "/logout?" + url.Values{
			"client_id":  {cfg.ClientID},
			"logout_uri": {cfg.BaseURL + "/"},
		}.Encode(),
		secure:     base.Scheme == "https",
		baseOrigin: strings.ToLower(base.Scheme + "://" + base.Host),
		loginKey:   randomBytes(32),
	}, nil
}

// NewDev returns an Auth for local development in which every request is
// signed in as sub "dev-user" with the given email. Login and logout are
// no-ops and no session is persisted. Only requests whose Host is loopback
// count as signed in, so a public Host (the Tailscale Funnel) or a DNS
// rebinding page gets nothing.
func NewDev(email string) *Auth {
	return &Auth{dev: true, devEmail: email}
}

// Register adds the auth routes to mux.
func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.Handle("POST /auth/logout", a.SameOrigin(http.HandlerFunc(a.logout)))
	mux.HandleFunc("GET /api/me", a.me)
}

// Disabled registers only GET /api/me, reporting that sign-in is off.
func Disabled(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "authenticated": false})
	})
}

// User returns the signed-in user's id (the Cognito sub). It has no side effects.
func (a *Auth) User(r *http.Request) (sub string, ok bool) {
	sub, _, ok = a.session(r)
	return sub, ok
}

// SameOrigin is the CSRF guard for mutating routes. Requests other than
// GET, HEAD and OPTIONS must carry an Origin (or, failing that, a Referer)
// matching the site, and any body must be JSON, which a cross-site form
// cannot send.
func (a *Auth) SameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !a.sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request refused"})
			return
		}
		if r.ContentLength != 0 {
			if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "content type must be application/json"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) sameOrigin(r *http.Request) bool {
	want := a.baseOrigin
	if a.dev {
		if !loopbackHost(r.Host) {
			return false
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		want = strings.ToLower(scheme + "://" + r.Host)
	}
	// An Origin header, even "null", is authoritative; Referer is only a
	// fallback for requests that carry no Origin.
	if origin := r.Header.Get("Origin"); origin != "" {
		return strings.ToLower(origin) == want
	}
	ref, err := url.Parse(r.Header.Get("Referer"))
	return err == nil && ref.Scheme != "" && ref.Host != "" && strings.ToLower(ref.Scheme+"://"+ref.Host) == want
}

// loopbackHost reports whether a Host header names this machine: localhost,
// 127.0.0.1 or ::1, with or without a port.
func loopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if a.dev {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	// Any ?next= is deliberately ignored: login always returns to "/".
	state, nonce, verifier := randomString(), randomString(), oauth2.GenerateVerifier()
	a.setCookie(w, loginCookie, "/auth", a.sealLogin(state, nonce, verifier, time.Now().Add(loginTTL)), int(loginTTL/time.Second))
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), http.StatusFound)
}

func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if a.dev {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	// Expire the login cookie whatever happens next.
	a.setCookie(w, loginCookie, "/auth", "", -1)

	sub, email, err := a.completeLogin(r)
	if err != nil {
		// Only the reason goes to the log, never the code, tokens or
		// anything the provider said in a response body.
		log.Printf("auth: sign-in failed: %v", err)
		http.Redirect(w, r, "/?login=failed", http.StatusFound)
		return
	}
	id := randomBytes(32)
	if err := a.sessions.CreateSession(r.Context(), hashID(id), sub, email, time.Now().Add(sessionTTL)); err != nil {
		log.Printf("auth: creating session: %v", err)
		http.Redirect(w, r, "/?login=failed", http.StatusFound)
		return
	}
	a.setCookie(w, sessionCookie, "/", base64.RawURLEncoding.EncodeToString(id), int(sessionTTL/time.Second))
	http.Redirect(w, r, "/", http.StatusFound)
}

// completeLogin validates the callback and returns the verified identity. The
// errors it returns are for the log only and contain no secrets.
func (a *Auth) completeLogin(r *http.Request) (sub, email string, err error) {
	q := r.URL.Query()
	if q.Get("error") != "" {
		return "", "", errors.New("provider returned an error")
	}
	c, err := r.Cookie(loginCookie)
	if err != nil {
		return "", "", errors.New("login cookie missing")
	}
	l, ok := a.openLogin(c.Value, time.Now())
	if !ok {
		return "", "", errors.New("login cookie invalid or expired")
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(l.state)) != 1 {
		return "", "", errors.New("state mismatch")
	}
	code := q.Get("code")
	if code == "" {
		return "", "", errors.New("no code")
	}

	ctx, cancel := context.WithTimeout(oidc.ClientContext(r.Context(), a.client), httpTimeout)
	defer cancel()
	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(l.verifier))
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			return "", "", fmt.Errorf("code exchange rejected (%q)", re.ErrorCode)
		}
		return "", "", errors.New("code exchange failed")
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return "", "", errors.New("no id_token in token response")
	}
	idToken, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return "", "", fmt.Errorf("id token rejected: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(l.nonce)) != 1 {
		return "", "", errors.New("nonce mismatch")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", "", errors.New("unreadable id token claims")
	}
	if idToken.Subject == "" || claims.Email == "" {
		return "", "", errors.New("id token has no sub or email")
	}
	if claims.EmailVerified != nil && !*claims.EmailVerified {
		return "", "", errors.New("email not verified")
	}
	return idToken.Subject, claims.Email, nil
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if a.dev {
		writeJSON(w, http.StatusOK, map[string]string{"logoutUrl": "/"})
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if hash, ok := sessionHash(c.Value); ok {
			if err := a.sessions.DeleteSession(r.Context(), hash); err != nil {
				// Keep the cookie so signing out can be retried.
				log.Printf("auth: deleting session: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sign out failed"})
				return
			}
		}
	}
	a.setCookie(w, sessionCookie, "/", "", -1)
	writeJSON(w, http.StatusOK, map[string]string{"logoutUrl": a.logoutURL})
}

func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	_, email, ok := a.session(r)
	writeJSON(w, http.StatusOK, struct {
		Enabled       bool   `json:"enabled"`
		Authenticated bool   `json:"authenticated"`
		Email         string `json:"email,omitempty"`
	}{true, ok, email})
}

// session resolves the session cookie. The id is never compared directly:
// the store is queried by SHA-256(id) only.
func (a *Auth) session(r *http.Request) (sub, email string, ok bool) {
	if a.dev {
		if !loopbackHost(r.Host) {
			return "", "", false
		}
		return devSub, a.devEmail, true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", "", false
	}
	hash, ok := sessionHash(c.Value)
	if !ok {
		return "", "", false
	}
	sub, email, ok, err = a.sessions.GetSession(r.Context(), hash)
	if err != nil {
		log.Printf("auth: session lookup: %v", err)
		return "", "", false
	}
	if !ok {
		return "", "", false
	}
	return sub, email, true
}

// login is the content of the fv_login cookie.
type login struct {
	state, nonce, verifier string
}

// sealLogin returns state.nonce.verifier.exp.mac. Every part is base64url or
// digits, so "." is a safe separator.
func (a *Auth) sealLogin(state, nonce, verifier string, exp time.Time) string {
	payload := state + "." + nonce + "." + verifier + "." + strconv.FormatInt(exp.Unix(), 10)
	return payload + "." + a.mac(payload)
}

// openLogin checks the signature and expiry (the signed expiry, not the
// cookie's Max-Age, which the client controls).
func (a *Auth) openLogin(v string, now time.Time) (login, bool) {
	i := strings.LastIndexByte(v, '.')
	if i < 0 {
		return login{}, false
	}
	payload, sig := v[:i], v[i+1:]
	if !hmac.Equal([]byte(sig), []byte(a.mac(payload))) {
		return login{}, false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 4 {
		return login{}, false
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || now.Unix() >= exp {
		return login{}, false
	}
	return login{parts[0], parts[1], parts[2]}, true
}

func (a *Auth) mac(payload string) string {
	h := hmac.New(sha256.New, a.loginKey)
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// setCookie sets (or, with maxAge < 0, expires) one of our cookies.
func (a *Auth) setCookie(w http.ResponseWriter, name, path, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionHash validates a session cookie value (base64url of 32 bytes) and
// returns the hex SHA-256 under which the store knows the session.
func sessionHash(cookieValue string) (string, bool) {
	id, err := base64.RawURLEncoding.DecodeString(cookieValue)
	if err != nil || len(id) != 32 {
		return "", false
	}
	return hashID(id), true
}

func hashID(id []byte) string {
	sum := sha256.Sum256(id)
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b) // never fails: it crashes the program instead
	return b
}

func randomString() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(32))
}

// noStore keeps redirects that carry cookies out of caches; writeJSON does the
// same for JSON responses.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

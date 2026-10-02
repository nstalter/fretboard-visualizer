package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	testClientID      = "test-client-id"
	testClientSecret  = "test-client-secret-VALUE"
	testCognitoDomain = "https://login.example.test"
	testSub           = "sub-123"
	testEmail         = "user@example.com"
	testKID           = "test-key"
	testAccessToken   = "ACCESS-TOKEN-VALUE"
	testRefreshToken  = "REFRESH-TOKEN-VALUE"
	providerText      = "PROVIDER-ERROR-TEXT"
)

var (
	testKey = sync.OnceValue(newKey)
	evilKey = sync.OnceValue(newKey)
)

func newKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func jsonBytes(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// signJWT returns a compact RS256 JWT.
func signJWT(key *rsa.PrivateKey, claims map[string]any) string {
	in := b64(jsonBytes(map[string]string{"alg": "RS256", "kid": testKID, "typ": "JWT"})) + "." + b64(jsonBytes(claims))
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return in + "." + b64(sig)
}

// fakeProvider is an in-process OpenID provider: discovery, JWKS, an authorize
// endpoint that checks the request and redirects back with a code, and a token
// endpoint that checks client authentication and PKCE and returns an ID token
// whose claims and signature the test controls.
type fakeProvider struct {
	srv         *httptest.Server
	redirectURI string // the one registered redirect URI

	mu         sync.Mutex
	codes      map[string]authRequest
	issued     []string
	tokenCalls int
	lastToken  string

	// Knobs, set by tests before the callback.
	claims      func(map[string]any) // edits the ID token claims
	signKey     *rsa.PrivateKey      // signs the ID token
	rawToken    func(string) string  // replaces the finished ID token
	tokenError  bool                 // token endpoint answers 400 invalid_grant
	omitIDToken bool                 // token response has no id_token
}

type authRequest struct{ nonce, challenge string }

func newFakeProvider(t *testing.T) *fakeProvider {
	p := &fakeProvider{codes: map[string]authRequest{}, signKey: testKey()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /jwks", p.jwks)
	mux.HandleFunc("GET /authorize", p.authorize)
	mux.HandleFunc("POST /token", p.token)
	p.srv = httptest.NewTLSServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeProvider) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.srv.URL,
		"authorization_endpoint":                p.srv.URL + "/authorize",
		"token_endpoint":                        p.srv.URL + "/token",
		"jwks_uri":                              p.srv.URL + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *fakeProvider) jwks(w http.ResponseWriter, r *http.Request) {
	pub := testKey().PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": testKID, "use": "sig", "alg": "RS256",
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (p *fakeProvider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for k, want := range map[string]string{
		"response_type":         "code",
		"client_id":             testClientID,
		"redirect_uri":          p.redirectURI,
		"scope":                 "openid email",
		"code_challenge_method": "S256",
	} {
		if q.Get(k) != want {
			http.Error(w, "bad "+k, http.StatusBadRequest)
			return
		}
	}
	if q.Get("state") == "" || q.Get("nonce") == "" || q.Get("code_challenge") == "" {
		http.Error(w, "missing state, nonce or code_challenge", http.StatusBadRequest)
		return
	}
	code := randomString()
	p.mu.Lock()
	p.codes[code] = authRequest{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	p.issued = append(p.issued, code)
	p.mu.Unlock()
	cb, _ := url.Parse(p.redirectURI)
	cq := cb.Query()
	cq.Set("code", code)
	cq.Set("state", q.Get("state"))
	cb.RawQuery = cq.Encode()
	http.Redirect(w, r, cb.String(), http.StatusFound)
}

func (p *fakeProvider) token(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokenCalls++
	invalid := func() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": providerText})
	}
	if p.tokenError {
		invalid()
		return
	}
	if id, secret, ok := r.BasicAuth(); !ok || id != testClientID || secret != testClientSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	r.ParseForm()
	code := r.PostForm.Get("code")
	req, known := p.codes[code]
	delete(p.codes, code) // codes are single-use
	if r.PostForm.Get("grant_type") != "authorization_code" || !known ||
		r.PostForm.Get("redirect_uri") != p.redirectURI ||
		oauth2.S256ChallengeFromVerifier(r.PostForm.Get("code_verifier")) != req.challenge {
		invalid()
		return
	}
	now := time.Now()
	claims := map[string]any{
		"iss": p.srv.URL, "sub": testSub, "aud": testClientID,
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
		"nonce": req.nonce, "email": testEmail, "email_verified": true, "token_use": "id",
	}
	if p.claims != nil {
		p.claims(claims)
	}
	idToken := signJWT(p.signKey, claims)
	if p.rawToken != nil {
		idToken = p.rawToken(idToken)
	}
	p.lastToken = idToken
	resp := map[string]any{
		"access_token": testAccessToken, "refresh_token": testRefreshToken,
		"token_type": "Bearer", "expires_in": 3600,
	}
	if !p.omitIDToken {
		resp["id_token"] = idToken
	}
	writeJSON(w, http.StatusOK, resp)
}

// secrets returns everything that must never show up in a log line or a
// response to the browser.
func (p *fakeProvider) secrets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := append([]string{testClientSecret, testAccessToken, testRefreshToken, p.lastToken}, p.issued...)
	return s
}

// fakeStore is an in-memory SessionStore.
type fakeStore struct {
	mu                     sync.Mutex
	rows                   map[string]fakeSession
	creates, gets, deletes int
	args                   []string // every tokenHash the store was handed
	createErr, getErr      error
	deleteErr              error
}

type fakeSession struct {
	sub, email string
	expires    time.Time
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]fakeSession{}} }

func (s *fakeStore) CreateSession(ctx context.Context, tokenHash, sub, email string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	s.args = append(s.args, tokenHash)
	if s.createErr != nil {
		return s.createErr
	}
	s.rows[tokenHash] = fakeSession{sub, email, expires}
	return nil
}

func (s *fakeStore) GetSession(ctx context.Context, tokenHash string) (string, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	s.args = append(s.args, tokenHash)
	if s.getErr != nil {
		return "", "", false, s.getErr
	}
	row, ok := s.rows[tokenHash]
	if !ok || !time.Now().Before(row.expires) {
		return "", "", false, nil
	}
	return row.sub, row.email, true, nil
}

func (s *fakeStore) DeleteSession(ctx context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	s.args = append(s.args, tokenHash)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.rows, tokenHash)
	return nil
}

func (s *fakeStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// counts returns how many store calls of each kind happened.
func (s *fakeStore) counts() (creates, gets, deletes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates, s.gets, s.deletes
}

// syncBuffer is a log destination that is safe to read while handlers write.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func captureLog(t *testing.T) *syncBuffer {
	buf := &syncBuffer{}
	w, f := log.Writer(), log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(w); log.SetFlags(f) })
	return buf
}

// harness wires an Auth to a fake provider, a fake store and a real HTTP
// server, and gives tests a cookie-keeping "browser" to drive the flow.
type harness struct {
	t     *testing.T
	p     *fakeProvider
	store *fakeStore
	auth  *Auth
	app   *httptest.Server
	mux   *http.ServeMux
	log   *syncBuffer
	pool  *x509.CertPool
}

func newHarness(t *testing.T, useTLS bool) *harness {
	h := &harness{t: t, p: newFakeProvider(t), store: newFakeStore(), log: captureLog(t)}
	h.app = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mux.ServeHTTP(w, r)
	}))
	if useTLS {
		h.app.StartTLS()
	} else {
		h.app.Start()
	}
	t.Cleanup(h.app.Close)
	h.pool = x509.NewCertPool()
	h.pool.AddCert(h.p.srv.Certificate())
	if useTLS {
		h.pool.AddCert(h.app.Certificate())
	}
	h.p.redirectURI = h.app.URL + "/auth/callback"

	a, err := New(oidc.ClientContext(context.Background(), h.p.srv.Client()), h.config(), h.store)
	if err != nil {
		t.Fatal(err)
	}
	h.auth = a
	h.mux = http.NewServeMux()
	a.Register(h.mux)
	return h
}

func (h *harness) config() Config {
	return Config{
		BaseURL:       h.app.URL,
		Issuer:        h.p.srv.URL,
		ClientID:      testClientID,
		ClientSecret:  testClientSecret,
		CognitoDomain: testCognitoDomain,
	}
}

// browser is a client with its own cookie jar that never follows redirects.
func (h *harness) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: h.pool},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type result struct {
	*http.Response
	body string
}

func (h *harness) do(c *http.Client, method, rawurl string, hdr map[string]string, body string) *result {
	h.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, rawurl, rd)
	if err != nil {
		h.t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return &result{resp, string(b)}
}

func (h *harness) get(c *http.Client, rawurl string) *result {
	h.t.Helper()
	return h.do(c, "GET", rawurl, nil, "")
}

// cookie returns the named cookie the browser would send to path.
func (h *harness) cookie(c *http.Client, name, path string) *http.Cookie {
	h.t.Helper()
	u, _ := url.Parse(h.app.URL + path)
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

// login does GET /auth/login and returns the provider URL it redirects to.
func (h *harness) login(b *http.Client) string {
	h.t.Helper()
	res := h.get(b, h.app.URL+"/auth/login")
	if res.StatusCode != http.StatusFound {
		h.t.Fatalf("login: status %d, want 302", res.StatusCode)
	}
	return res.Header.Get("Location")
}

// authorize plays the browser at the provider and returns the callback URL
// the provider redirects to.
func (h *harness) authorize(b *http.Client, authURL string) *url.URL {
	h.t.Helper()
	res := h.get(b, authURL)
	if res.StatusCode != http.StatusFound {
		h.t.Fatalf("authorize: status %d (%s), want 302", res.StatusCode, res.body)
	}
	u, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		h.t.Fatal(err)
	}
	return u
}

// begin runs login and authorize and returns the callback URL.
func (h *harness) begin(b *http.Client) *url.URL {
	h.t.Helper()
	return h.authorize(b, h.login(b))
}

// callbackWithCookie requests cb from a browser with an empty jar that sends
// exactly the given fv_login value (or none when it is empty).
func (h *harness) callbackWithCookie(cb *url.URL, value string) *result {
	h.t.Helper()
	hdr := map[string]string{}
	if value != "" {
		hdr["Cookie"] = loginCookie + "=" + value
	}
	return h.do(h.browser(), "GET", cb.String(), hdr, "")
}

// signIn runs the whole flow and fails the test unless it ends signed in.
// It returns the browser, now holding fv_session.
func (h *harness) signIn() *http.Client {
	h.t.Helper()
	b := h.browser()
	res := h.get(b, h.begin(b).String())
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" || h.cookie(b, sessionCookie, "/") == nil {
		h.t.Fatalf("sign in failed: %d %s, log: %s", res.StatusCode, res.Header.Get("Location"), h.log.String())
	}
	return b
}

// seedSession puts a session straight into the store and returns its cookie.
func (h *harness) seedSession(sub, email string, expires time.Time) *http.Cookie {
	id := randomBytes(32)
	h.store.CreateSession(context.Background(), hashID(id), sub, email, expires)
	return &http.Cookie{Name: sessionCookie, Value: b64(id)}
}

// assertNoSecretsLogged fails if any token, code or the client secret (or any
// extra value) appears in the captured log.
func (h *harness) assertNoSecretsLogged(extra ...string) {
	h.t.Helper()
	out := h.log.String()
	for _, s := range append(h.p.secrets(), extra...) {
		if s != "" && strings.Contains(out, s) {
			h.t.Errorf("log leaks %q:\n%s", s, out)
		}
	}
}

var errBoom = errors.New("boom")

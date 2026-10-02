package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func checkCookie(t *testing.T, c *http.Cookie, path string, maxAge int, secure bool) {
	t.Helper()
	if c == nil {
		t.Fatal("cookie not set")
	}
	if !c.HttpOnly {
		t.Errorf("%s: not HttpOnly", c.Name)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("%s: SameSite = %v, want Lax", c.Name, c.SameSite)
	}
	if c.Path != path {
		t.Errorf("%s: Path = %q, want %q", c.Name, c.Path, path)
	}
	if c.MaxAge != maxAge {
		t.Errorf("%s: MaxAge = %d, want %d", c.Name, c.MaxAge, maxAge)
	}
	if c.Secure != secure {
		t.Errorf("%s: Secure = %v, want %v", c.Name, c.Secure, secure)
	}
}

func setCookie(res *result, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func decodeJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	return m
}

// userFor reports what User says for a request carrying the given cookies.
func userFor(a *Auth, cookies ...*http.Cookie) (string, bool) {
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return a.User(r)
}

// flip returns a different base64url character.
func flip(c byte) byte {
	if c == 'A' {
		return 'B'
	}
	return 'A'
}

func TestSignInFlow(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		name := "http"
		if useTLS {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, useTLS)
			b := h.browser()

			// Login: ?next= is ignored, the redirect goes to the provider
			// with PKCE S256, state and nonce, and fv_login is set.
			res := h.get(b, h.app.URL+"/auth/login?next=https://evil.example/")
			if res.StatusCode != http.StatusFound {
				t.Fatalf("login status = %d, want 302", res.StatusCode)
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("login Cache-Control = %q", got)
			}
			loc, err := url.Parse(res.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if loc.Scheme+"://"+loc.Host != h.p.srv.URL || loc.Path != "/authorize" {
				t.Fatalf("login redirects to %s, want the provider's /authorize", loc)
			}
			q := loc.Query()
			for k, want := range map[string]string{
				"response_type": "code", "client_id": testClientID, "scope": "openid email",
				"redirect_uri": h.app.URL + "/auth/callback", "code_challenge_method": "S256",
			} {
				if q.Get(k) != want {
					t.Errorf("authorize %s = %q, want %q", k, q.Get(k), want)
				}
			}
			for _, k := range []string{"state", "nonce", "code_challenge"} {
				if len(q.Get(k)) != 43 {
					t.Errorf("authorize %s = %q, want 43 base64url chars", k, q.Get(k))
				}
			}
			lc := setCookie(res, loginCookie)
			checkCookie(t, lc, "/auth", 600, useTLS)
			l, ok := h.auth.openLogin(lc.Value, time.Now())
			if !ok {
				t.Fatal("fv_login does not verify")
			}
			if l.state != q.Get("state") || l.nonce != q.Get("nonce") ||
				oauth2.S256ChallengeFromVerifier(l.verifier) != q.Get("code_challenge") {
				t.Error("fv_login does not carry the state, nonce and verifier of this login")
			}

			// Callback: session created, fv_login cleared, back to "/".
			cb := h.authorize(b, res.Header.Get("Location"))
			res = h.get(b, cb.String())
			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
				t.Fatalf("callback = %d %q, want 302 /; log: %s", res.StatusCode, res.Header.Get("Location"), h.log.String())
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("callback Cache-Control = %q", got)
			}
			checkCookie(t, setCookie(res, loginCookie), "/auth", -1, useTLS)
			sc := setCookie(res, sessionCookie)
			checkCookie(t, sc, "/", 30*24*3600, useTLS)

			// The store holds only SHA-256(id), never the cookie value.
			if h.store.len() != 1 {
				t.Fatalf("store has %d sessions, want 1", h.store.len())
			}
			id, err := base64.RawURLEncoding.DecodeString(sc.Value)
			if err != nil || len(id) != 32 {
				t.Fatalf("session cookie %q is not 32 base64url bytes", sc.Value)
			}
			sum := sha256.Sum256(id)
			wantHash := hex.EncodeToString(sum[:])
			row, ok := h.store.rows[wantHash]
			if !ok {
				t.Fatalf("store has no session under SHA-256(id); rows: %v", h.store.rows)
			}
			if row.sub != testSub || row.email != testEmail {
				t.Errorf("stored (%q, %q), want (%q, %q)", row.sub, row.email, testSub, testEmail)
			}
			if d := time.Until(row.expires) - 30*24*time.Hour; d > time.Minute || d < -time.Minute {
				t.Errorf("session expires %v from now, want ~30 days", time.Until(row.expires))
			}
			for _, a := range h.store.args {
				if a == sc.Value || strings.Contains(a, sc.Value) {
					t.Errorf("store was handed the raw session id")
				}
			}

			// Signed in: User and /api/me agree, tokens went nowhere.
			if sub, ok := userFor(h.auth, &http.Cookie{Name: sessionCookie, Value: sc.Value}); !ok || sub != testSub {
				t.Errorf("User = (%q, %v), want (%q, true)", sub, ok, testSub)
			}
			me := h.get(b, h.app.URL+"/api/me")
			if got := decodeJSON(t, me.body); !reflect.DeepEqual(got, map[string]any{"enabled": true, "authenticated": true, "email": testEmail}) {
				t.Errorf("/api/me = %v", got)
			}
			if h.p.tokenCalls != 1 {
				t.Errorf("token endpoint called %d times, want 1", h.p.tokenCalls)
			}
			for _, s := range h.p.secrets() {
				if s == "" {
					continue
				}
				if strings.Contains(res.body, s) || strings.Contains(me.body, s) {
					t.Errorf("response body leaks %q", s)
				}
				for k, vs := range res.Header {
					if strings.Contains(strings.Join(vs, ","), s) {
						t.Errorf("response header %s leaks %q", k, s)
					}
				}
			}
			h.assertNoSecretsLogged(sc.Value, lc.Value)
		})
	}
}

func TestEachLoginIsFresh(t *testing.T) {
	h := newHarness(t, false)
	b := h.browser()
	var states, nonces, challenges, cookies = map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for range 5 {
		loc, _ := url.Parse(h.login(b))
		q := loc.Query()
		states[q.Get("state")], nonces[q.Get("nonce")], challenges[q.Get("code_challenge")] = true, true, true
		cookies[h.cookie(b, loginCookie, "/auth/callback").Value] = true
	}
	for name, set := range map[string]map[string]bool{"state": states, "nonce": nonces, "code_challenge": challenges, "fv_login": cookies} {
		if len(set) != 5 {
			t.Errorf("%s repeated across logins: %d distinct of 5", name, len(set))
		}
	}
}

func TestCallbackRejects(t *testing.T) {
	// prov changes the provider before an otherwise normal login.
	prov := func(setup func(p *fakeProvider)) func(h *harness) *result {
		return func(h *harness) *result {
			setup(h.p)
			b := h.browser()
			return h.get(b, h.begin(b).String())
		}
	}
	// cookie sends the callback with an fv_login of the test's choosing.
	cookie := func(value func(h *harness, cb *url.URL, real string) string) func(h *harness) *result {
		return func(h *harness) *result {
			b := h.browser()
			cb := h.begin(b)
			real := h.cookie(b, loginCookie, "/auth/callback").Value
			return h.callbackWithCookie(cb, value(h, cb, real))
		}
	}
	// query edits the callback's query string.
	query := func(edit func(q url.Values)) func(h *harness) *result {
		return func(h *harness) *result {
			b := h.browser()
			cb := h.begin(b)
			q := cb.Query()
			edit(q)
			cb.RawQuery = q.Encode()
			return h.get(b, cb.String())
		}
	}
	forged := func(h *harness, cb *url.URL, state string, exp time.Time) string {
		return h.auth.sealLogin(state, "n", oauth2.GenerateVerifier(), exp)
	}

	tests := []struct {
		name    string
		wantLog string
		noToken bool // must fail before the code is exchanged
		run     func(h *harness) *result
	}{
		{"wrong state", "state mismatch", true, query(func(q url.Values) { q.Set("state", "not-the-state") })},
		{"missing state", "state mismatch", true, query(func(q url.Values) { q.Del("state") })},
		{"missing code", "no code", true, query(func(q url.Values) { q.Del("code") })},
		{"provider error param", "provider returned an error", true, query(func(q url.Values) {
			q.Del("code")
			q.Set("error", "access_denied")
			q.Set("error_description", providerText)
		})},
		{"missing fv_login", "login cookie missing", true, cookie(func(*harness, *url.URL, string) string { return "" })},
		{"fv_login state altered", "login cookie invalid", true, cookie(func(_ *harness, _ *url.URL, real string) string {
			return string(flip(real[0])) + real[1:]
		})},
		{"fv_login signature stripped", "login cookie invalid", true, cookie(func(_ *harness, _ *url.URL, real string) string {
			return real[:strings.LastIndexByte(real, '.')]
		})},
		{"fv_login expiry extended", "login cookie invalid", true, cookie(func(_ *harness, _ *url.URL, real string) string {
			parts := strings.Split(real, ".")
			parts[3] = "99999999999"
			return strings.Join(parts, ".")
		})},
		{"fv_login signed with another key", "login cookie invalid", true, cookie(func(h *harness, cb *url.URL, _ string) string {
			other := &Auth{loginKey: randomBytes(32)}
			return other.sealLogin(cb.Query().Get("state"), "n", oauth2.GenerateVerifier(), time.Now().Add(time.Minute))
		})},
		{"fv_login expired", "login cookie invalid or expired", true, cookie(func(h *harness, cb *url.URL, _ string) string {
			return forged(h, cb, cb.Query().Get("state"), time.Now().Add(-time.Second))
		})},
		{"fv_login garbage", "login cookie invalid", true, cookie(func(*harness, *url.URL, string) string { return "garbage" })},
		{"fv_login for another state", "state mismatch", true, cookie(func(h *harness, cb *url.URL, _ string) string {
			return forged(h, cb, "some-other-state", time.Now().Add(time.Minute))
		})},
		{"wrong PKCE verifier", `code exchange rejected ("invalid_grant")`, false, cookie(func(h *harness, cb *url.URL, _ string) string {
			return forged(h, cb, cb.Query().Get("state"), time.Now().Add(time.Minute))
		})},
		{"token endpoint refuses", `code exchange rejected ("invalid_grant")`, false, prov(func(p *fakeProvider) { p.tokenError = true })},
		{"no id_token", "no id_token", false, prov(func(p *fakeProvider) { p.omitIDToken = true })},
		{"wrong nonce", "nonce mismatch", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["nonce"] = "someone-elses-nonce" }
		})},
		{"missing nonce", "nonce mismatch", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { delete(c, "nonce") }
		})},
		{"expired token", "expired", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }
		})},
		{"wrong audience", "expected audience", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["aud"] = "another-client" }
		})},
		{"wrong issuer", "different provider", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["iss"] = "https://evil.example" }
		})},
		{"email not verified", "email not verified", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["email_verified"] = false }
		})},
		{"email_verified of the wrong type", "unreadable id token claims", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { c["email_verified"] = "true" }
		})},
		{"missing email", "no sub or email", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { delete(c, "email") }
		})},
		{"missing sub", "no sub or email", false, prov(func(p *fakeProvider) {
			p.claims = func(c map[string]any) { delete(c, "sub") }
		})},
		{"signed with an unknown key", "failed to verify signature", false, prov(func(p *fakeProvider) { p.signKey = evilKey() })},
		{"unsigned token", "malformed jwt", false, prov(func(p *fakeProvider) {
			p.rawToken = func(tok string) string {
				return b64(jsonBytes(map[string]string{"alg": "none"})) + "." + strings.Split(tok, ".")[1] + "."
			}
		})},
		{"HS256 key confusion", "malformed jwt", false, prov(func(p *fakeProvider) {
			p.rawToken = func(tok string) string {
				in := b64(jsonBytes(map[string]string{"alg": "HS256", "kid": testKID})) + "." + strings.Split(tok, ".")[1]
				mac := hmac.New(sha256.New, testKey().PublicKey.N.Bytes())
				mac.Write([]byte(in))
				return in + "." + b64(mac.Sum(nil))
			}
		})},
		{"provider unreachable", "code exchange failed", false, func(h *harness) *result {
			b := h.browser()
			cb := h.begin(b)
			h.p.srv.Close()
			return h.get(b, cb.String())
		}},
		{"session store failure", "creating session", false, func(h *harness) *result {
			h.store.createErr = errBoom
			b := h.browser()
			return h.get(b, h.begin(b).String())
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, false)
			res := tc.run(h)

			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/?login=failed" {
				t.Errorf("got %d %q, want 302 /?login=failed", res.StatusCode, res.Header.Get("Location"))
			}
			if c := setCookie(res, sessionCookie); c != nil {
				t.Errorf("failed login set fv_session=%q", c.Value)
			}
			checkCookie(t, setCookie(res, loginCookie), "/auth", -1, false)
			if h.store.len() != 0 {
				t.Errorf("failed login left %d sessions in the store", h.store.len())
			}
			if strings.Contains(res.body, providerText) || strings.Contains(res.Header.Get("Location"), providerText) {
				t.Error("response echoes provider text")
			}
			if out := h.log.String(); !strings.Contains(out, tc.wantLog) {
				t.Errorf("log lacks %q:\n%s", tc.wantLog, out)
			}
			if tc.noToken && h.p.tokenCalls != 0 {
				t.Errorf("token endpoint was called %d times, want 0", h.p.tokenCalls)
			}
			h.assertNoSecretsLogged()
			if strings.Contains(h.log.String(), providerText) {
				t.Error("log echoes provider text")
			}
		})
	}
}

func TestEmailVerifiedAbsentIsAccepted(t *testing.T) {
	h := newHarness(t, false)
	h.p.claims = func(c map[string]any) { delete(c, "email_verified") }
	h.signIn()
}

func TestCallbackReplayIsRefused(t *testing.T) {
	h := newHarness(t, false)
	b := h.browser()
	cb := h.begin(b)
	real := h.cookie(b, loginCookie, "/auth/callback").Value

	if res := h.get(b, cb.String()); res.Header.Get("Location") != "/" {
		t.Fatalf("first callback failed: %s", h.log.String())
	}

	// The browser's copy of fv_login is gone.
	if res := h.get(b, cb.String()); res.Header.Get("Location") != "/?login=failed" {
		t.Errorf("replay from the browser = %q, want failure", res.Header.Get("Location"))
	}
	// Someone who kept the cookie and the URL gets nowhere: the provider
	// refuses a code it has already redeemed.
	if res := h.callbackWithCookie(cb, real); res.Header.Get("Location") != "/?login=failed" {
		t.Errorf("replay with the kept cookie = %q, want failure", res.Header.Get("Location"))
	}
	if !strings.Contains(h.log.String(), `code exchange rejected ("invalid_grant")`) {
		t.Errorf("log lacks the provider's refusal:\n%s", h.log.String())
	}
	if h.store.len() != 1 {
		t.Errorf("sessions = %d, want 1", h.store.len())
	}
}

func TestLoginCookieSealing(t *testing.T) {
	h := newHarness(t, false)
	now := time.Now()
	v := h.auth.sealLogin("st", "no", "ve", now.Add(time.Minute))
	l, ok := h.auth.openLogin(v, now)
	if !ok || l.state != "st" || l.nonce != "no" || l.verifier != "ve" {
		t.Fatalf("round trip = %+v, %v", l, ok)
	}
	if _, ok := h.auth.openLogin(v, now.Add(time.Minute)); ok {
		t.Error("cookie accepted at its expiry")
	}
	for _, bad := range []string{"", ".", "a.b", v + "x", v[1:], strings.Replace(v, "st.", "xx.", 1), "st.no.ve.1.mac"} {
		if _, ok := h.auth.openLogin(bad, now); ok {
			t.Errorf("openLogin(%q) succeeded", bad)
		}
	}
	// Correctly signed but malformed payloads are rejected too.
	for _, payload := range []string{"a.b.c", "a.b.c.not-a-number", "a.b.c.1.2"} {
		if _, ok := h.auth.openLogin(payload+"."+h.auth.mac(payload), now); ok {
			t.Errorf("openLogin accepted signed payload %q", payload)
		}
	}
	// A different process (key) cannot read it.
	if _, ok := (&Auth{loginKey: randomBytes(32)}).openLogin(v, now); ok {
		t.Error("cookie verified under another key")
	}
}

func TestLogout(t *testing.T) {
	h := newHarness(t, false)
	b := h.signIn()
	sc := h.cookie(b, sessionCookie, "/")
	logoutURL := h.app.URL + "/auth/logout"
	origin := map[string]string{"Origin": h.app.URL}

	// CSRF guard: no Origin, no logout.
	if res := h.do(b, "POST", logoutURL, nil, ""); res.StatusCode != http.StatusForbidden || h.store.len() != 1 {
		t.Fatalf("logout without Origin: %d, %d sessions; want 403 and the session intact", res.StatusCode, h.store.len())
	}
	// GET is not a logout.
	if res := h.get(b, logoutURL); res.StatusCode != http.StatusMethodNotAllowed || h.store.len() != 1 {
		t.Fatalf("GET /auth/logout: %d, %d sessions; want 405 and the session intact", res.StatusCode, h.store.len())
	}

	// A failing store must not look like success, and must allow a retry.
	h.store.deleteErr = errBoom
	res := h.do(b, "POST", logoutURL, origin, "")
	if res.StatusCode != http.StatusInternalServerError || setCookie(res, sessionCookie) != nil || h.cookie(b, sessionCookie, "/") == nil {
		t.Fatalf("logout with failing store: %d, cookie cleared=%v", res.StatusCode, setCookie(res, sessionCookie) != nil)
	}
	h.store.deleteErr = nil

	res = h.do(b, "POST", logoutURL, origin, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d (%s)", res.StatusCode, res.body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	got := decodeJSON(t, res.body)
	lu, err := url.Parse(got["logoutUrl"].(string))
	if err != nil || len(got) != 1 {
		t.Fatalf("body = %s", res.body)
	}
	if lu.Scheme+"://"+lu.Host != testCognitoDomain || lu.Path != "/logout" ||
		lu.Query().Get("client_id") != testClientID || lu.Query().Get("logout_uri") != h.app.URL+"/" || len(lu.Query()) != 2 {
		t.Errorf("logoutUrl = %s", lu)
	}
	checkCookie(t, setCookie(res, sessionCookie), "/", -1, false)
	if h.store.len() != 0 {
		t.Error("session still in the store")
	}
	// The old cookie is dead server-side, not merely expired in the browser.
	if _, ok := userFor(h.auth, &http.Cookie{Name: sessionCookie, Value: sc.Value}); ok {
		t.Error("session cookie still works after logout")
	}
	if me := decodeJSON(t, h.get(b, h.app.URL+"/api/me").body); me["authenticated"] != false {
		t.Errorf("/api/me after logout = %v", me)
	}

	// Logging out when already signed out (or with junk) still answers, and
	// touches nothing.
	_, _, before := h.store.counts()
	for _, c := range []*http.Cookie{nil, {Name: sessionCookie, Value: "junk"}} {
		hdr := map[string]string{"Origin": h.app.URL}
		if c != nil {
			hdr["Cookie"] = c.String()
		}
		res := h.do(h.browser(), "POST", logoutURL, hdr, "")
		if res.StatusCode != http.StatusOK || decodeJSON(t, res.body)["logoutUrl"] == "" {
			t.Errorf("signed-out logout = %d %s", res.StatusCode, res.body)
		}
	}
	if _, _, after := h.store.counts(); after != before {
		t.Errorf("signed-out logout deleted %d rows", after-before)
	}
}

func TestSameOrigin(t *testing.T) {
	h := newHarness(t, false)
	site := h.app.URL // http://127.0.0.1:port
	other := strings.Replace(site, "127.0.0.1", "127.0.0.2", 1)
	hdr := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	const (
		pass    = http.StatusNoContent
		refused = http.StatusForbidden
		badType = http.StatusUnsupportedMediaType
	)
	tests := []struct {
		name   string
		method string
		hdr    map[string]string
		body   string
		want   int
	}{
		{"POST matching Origin and JSON", "POST", hdr("Origin", site, "Content-Type", "application/json"), `{}`, pass},
		{"JSON with charset", "PUT", hdr("Origin", site, "Content-Type", "application/json; charset=utf-8"), `{}`, pass},
		{"PATCH", "PATCH", hdr("Origin", site, "Content-Type", "application/json"), `{}`, pass},
		{"POST without body", "POST", hdr("Origin", site), "", pass},
		{"DELETE without body", "DELETE", hdr("Origin", site), "", pass},
		{"Origin case-insensitive", "POST", hdr("Origin", strings.ToUpper(site)), "", pass},
		{"Referer fallback", "POST", hdr("Referer", site+"/some/page?x=1"), "", pass},
		{"foreign Origin", "POST", hdr("Origin", "https://evil.example", "Content-Type", "application/json"), `{}`, refused},
		{"other host same scheme", "POST", hdr("Origin", other), "", refused},
		{"Origin that merely starts like ours", "POST", hdr("Origin", site+".evil.example"), "", refused},
		{"Origin with a port added", "POST", hdr("Origin", site+"0"), "", refused},
		{"scheme mismatch", "POST", hdr("Origin", "https://"+strings.TrimPrefix(site, "http://")), "", refused},
		{"null Origin", "POST", hdr("Origin", "null"), "", refused},
		{"no Origin or Referer", "POST", hdr(), "", refused},
		{"no Origin or Referer, with JSON", "POST", hdr("Content-Type", "application/json"), `{}`, refused},
		{"foreign Referer", "POST", hdr("Referer", "https://evil.example/"+site), "", refused},
		{"unparseable Referer", "POST", hdr("Referer", "::not a url"), "", refused},
		{"Referer without host", "POST", hdr("Referer", "/path"), "", refused},
		{"foreign Origin beats matching Referer", "POST", hdr("Origin", other, "Referer", site+"/"), "", refused},
		{"form body", "POST", hdr("Origin", site, "Content-Type", "application/x-www-form-urlencoded"), "a=b", badType},
		{"text/plain body", "POST", hdr("Origin", site, "Content-Type", "text/plain"), "{}", badType},
		{"multipart body", "POST", hdr("Origin", site, "Content-Type", "multipart/form-data; boundary=x"), "--x--", badType},
		{"body with no Content-Type", "POST", hdr("Origin", site), "{}", badType},
		{"near-JSON type", "POST", hdr("Origin", site, "Content-Type", "application/jsonp"), "{}", badType},
		{"foreign Origin wins over bad type", "POST", hdr("Origin", other, "Content-Type", "text/plain"), "{}", refused},
		{"GET with foreign Origin", "GET", hdr("Origin", "https://evil.example"), "", pass},
		{"GET with nothing", "GET", hdr(), "", pass},
		{"HEAD", "HEAD", hdr(), "", pass},
		{"OPTIONS", "OPTIONS", hdr("Origin", "https://evil.example"), "", pass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			guarded := h.auth.SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			r := httptest.NewRequest(tc.method, "/api/x", body)
			for k, v := range tc.hdr {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			guarded.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", w.Code, tc.want, w.Body)
			}
			if called != (tc.want == pass) {
				t.Errorf("next called = %v", called)
			}
			switch tc.want {
			case refused:
				if got := strings.TrimSpace(w.Body.String()); got != `{"error":"cross-origin request refused"}` {
					t.Errorf("body = %s", got)
				}
			case badType:
				if _, ok := decodeJSON(t, w.Body.String())["error"].(string); !ok {
					t.Errorf("body = %s, want a JSON error", w.Body)
				}
			}
			if tc.want != pass && w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestSameOriginChunkedBodyNeedsJSON(t *testing.T) {
	h := newHarness(t, false)
	guarded := h.auth.SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("POST", "/x", strings.NewReader("{}"))
	r.ContentLength = -1 // unknown length: chunked
	r.Header.Set("Origin", h.app.URL)
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", w.Code)
	}
}

func TestMe(t *testing.T) {
	h := newHarness(t, false)
	me := func(cookies ...*http.Cookie) (*httptest.ResponseRecorder, map[string]any) {
		r := httptest.NewRequest("GET", "/api/me", nil)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.mux.ServeHTTP(w, r)
		return w, decodeJSON(t, w.Body.String())
	}
	signedOut := map[string]any{"enabled": true, "authenticated": false}

	w, got := me()
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("signed out: %d %v", w.Code, w.Header())
	}
	if !reflect.DeepEqual(got, signedOut) {
		t.Errorf("signed out: %v (email must be absent)", got)
	}

	good := h.seedSession("sub-9", "nine@example.com", time.Now().Add(time.Hour))
	w, got = me(good)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" ||
		!reflect.DeepEqual(got, map[string]any{"enabled": true, "authenticated": true, "email": "nine@example.com"}) {
		t.Errorf("signed in: %d %v", w.Code, got)
	}

	// Expired, unknown, store failing: all signed out, still 200.
	expired := h.seedSession("sub-8", "eight@example.com", time.Now().Add(-time.Second))
	unknown := &http.Cookie{Name: sessionCookie, Value: b64(randomBytes(32))}
	for name, c := range map[string]*http.Cookie{"expired": expired, "unknown": unknown} {
		if w, got := me(c); w.Code != 200 || !reflect.DeepEqual(got, signedOut) {
			t.Errorf("%s session: %d %v", name, w.Code, got)
		}
		if _, ok := userFor(h.auth, c); ok {
			t.Errorf("User accepted a %s session", name)
		}
	}
	h.store.getErr = errBoom
	if w, got := me(good); w.Code != 200 || !reflect.DeepEqual(got, signedOut) {
		t.Errorf("store failing: %d %v", w.Code, got)
	}
	if _, ok := userFor(h.auth, good); ok {
		t.Error("User accepted a session while the store was failing")
	}
	if !strings.Contains(h.log.String(), "session lookup") {
		t.Errorf("store failure not logged:\n%s", h.log.String())
	}
}

func TestSessionCookieValidation(t *testing.T) {
	h := newHarness(t, false)
	good := h.seedSession("sub-1", "a@example.com", time.Now().Add(time.Hour))
	if sub, ok := userFor(h.auth, good); !ok || sub != "sub-1" {
		t.Fatalf("User(good) = %q, %v", sub, ok)
	}
	if _, ok := userFor(h.auth); ok {
		t.Error("User without a cookie succeeded")
	}
	_, gets, _ := h.store.counts()

	// Malformed values are rejected without a store lookup.
	junk := []string{"", "abc", strings.Repeat("!", 43), b64(randomBytes(16)), b64(randomBytes(33)), good.Value + "A", good.Value + "="}
	for _, v := range junk {
		if _, ok := userFor(h.auth, &http.Cookie{Name: sessionCookie, Value: v}); ok {
			t.Errorf("User accepted cookie %q", v)
		}
	}
	if _, g, _ := h.store.counts(); g != gets {
		t.Errorf("malformed cookies caused %d store lookups", g-gets)
	}

	// A well-formed but altered id is a different hash: unknown.
	altered := string(flip(good.Value[0])) + good.Value[1:]
	if _, ok := userFor(h.auth, &http.Cookie{Name: sessionCookie, Value: altered}); ok {
		t.Error("User accepted an altered session id")
	}
	// What the store sees is only ever the hash.
	re := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, a := range h.store.args {
		if !re.MatchString(a) {
			t.Errorf("store was handed %q, want a SHA-256 hex digest", a)
		}
	}
	// User has no side effects.
	if c, _, d := h.store.counts(); c != 1 || d != 0 {
		t.Errorf("User wrote to the store: creates=%d deletes=%d", c, d)
	}
}

func TestDisabled(t *testing.T) {
	mux := http.NewServeMux()
	Disabled(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/me", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("status %d, headers %v", w.Code, w.Header())
	}
	if got := decodeJSON(t, w.Body.String()); !reflect.DeepEqual(got, map[string]any{"enabled": false, "authenticated": false}) {
		t.Errorf("body = %v", got)
	}
	for _, path := range []string{"/auth/login", "/auth/callback", "/auth/logout"} {
		for _, method := range []string{"GET", "POST"} {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404 (nothing else is registered)", method, path, w.Code)
			}
		}
	}
}

func TestDev(t *testing.T) {
	d := NewDev("dev@example.com")
	mux := http.NewServeMux()
	d.Register(mux)
	user := func() (string, bool) {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = "localhost:8080"
		return d.User(r)
	}
	serve := func(method, target string, hdr map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		r.Host = "localhost:8080"
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	if sub, ok := user(); !ok || sub != "dev-user" {
		t.Errorf("User = %q, %v", sub, ok)
	}
	w := serve("GET", "/api/me", nil)
	if got := decodeJSON(t, w.Body.String()); w.Code != 200 || !reflect.DeepEqual(got, map[string]any{"enabled": true, "authenticated": true, "email": "dev@example.com"}) {
		t.Errorf("/api/me = %d %v", w.Code, got)
	}
	for _, path := range []string{"/auth/login", "/auth/callback?code=x&state=y"} {
		w := serve("GET", path, nil)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/" || len(w.Result().Cookies()) != 0 {
			t.Errorf("GET %s = %d %q cookies=%v, want a plain redirect to /", path, w.Code, w.Header().Get("Location"), w.Result().Cookies())
		}
	}
	w = serve("POST", "/auth/logout", map[string]string{"Origin": "http://localhost:8080"})
	if got := decodeJSON(t, w.Body.String()); w.Code != 200 || !reflect.DeepEqual(got, map[string]any{"logoutUrl": "/"}) {
		t.Errorf("logout = %d %v", w.Code, got)
	}
	// Still signed in afterwards: logout is a no-op.
	if _, ok := user(); !ok {
		t.Error("dev user signed out")
	}

	// SameOrigin compares to the request's own Host.
	guard := d.SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for name, tc := range map[string]struct {
		hdr  map[string]string
		want int
	}{
		"same host":       {map[string]string{"Origin": "http://localhost:8080"}, 204},
		"referer":         {map[string]string{"Referer": "http://localhost:8080/page"}, 204},
		"other port":      {map[string]string{"Origin": "http://localhost:9090"}, 403},
		"foreign":         {map[string]string{"Origin": "https://evil.example"}, 403},
		"https vs http":   {map[string]string{"Origin": "https://localhost:8080"}, 403},
		"neither header":  {nil, 403},
		"null":            {map[string]string{"Origin": "null"}, 403},
		"foreign referer": {map[string]string{"Referer": "http://evil.example/"}, 403},
	} {
		r := httptest.NewRequest("POST", "/api/x", nil)
		r.Host = "localhost:8080"
		for k, v := range tc.hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("dev SameOrigin %s: %d, want %d", name, w.Code, tc.want)
		}
	}
}

func TestDevNeedsLoopbackHost(t *testing.T) {
	d := NewDev("dev@example.com")
	mux := http.NewServeMux()
	d.Register(mux)
	guard := d.SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	for _, tc := range []struct {
		host string
		ok   bool
	}{
		{"localhost", true},
		{"localhost:8080", true},
		{"LocalHost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"[::1]", true},
		{"[::1]:8080", true},
		{"::1", true},
		{"fretboard.tail9292f9.ts.net", false},
		{"evil.example:8080", false},
		{"10.0.0.5", false},
		{"localhost.evil.example", false},
		{"127.0.0.2:8080", false},
		{"", false},
	} {
		t.Run(tc.host, func(t *testing.T) {
			req := func(method, target string) *http.Request {
				r := httptest.NewRequest(method, target, nil)
				r.Host = tc.host
				return r
			}

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req("GET", "/api/me"))
			want := map[string]any{"enabled": true, "authenticated": false}
			if tc.ok {
				want = map[string]any{"enabled": true, "authenticated": true, "email": "dev@example.com"}
			}
			if got := decodeJSON(t, w.Body.String()); w.Code != 200 || !reflect.DeepEqual(got, want) {
				t.Errorf("/api/me = %d %v, want %v", w.Code, got, want)
			}
			if sub, ok := d.User(req("GET", "/")); ok != tc.ok || (ok && sub != "dev-user") {
				t.Errorf("User = (%q, %v), want ok=%v", sub, ok, tc.ok)
			}

			// An Origin that matches the Host is not enough on its own.
			r := req("POST", "/api/x")
			r.Header.Set("Origin", "http://"+tc.host)
			w = httptest.NewRecorder()
			guard.ServeHTTP(w, r)
			wantCode := http.StatusNoContent
			if !tc.ok {
				wantCode = http.StatusForbidden
			}
			if w.Code != wantCode {
				t.Errorf("POST = %d, want %d", w.Code, wantCode)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	h := newHarness(t, false)
	ctx := oidc.ClientContext(context.Background(), h.p.srv.Client())

	t.Run("invalid config is rejected before any request", func(t *testing.T) {
		cfg := h.config()
		cfg.ClientSecret = ""
		_, err := New(ctx, cfg, h.store)
		if err == nil || !strings.Contains(err.Error(), "COGNITO_CLIENT_SECRET") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("issuer that differs from discovery", func(t *testing.T) {
		cfg := h.config()
		cfg.Issuer += "/"
		if _, err := New(ctx, cfg, h.store); err == nil {
			t.Error("New accepted an issuer the discovery document disagrees with")
		}
	})
	t.Run("unreachable provider", func(t *testing.T) {
		p := newFakeProvider(t)
		cfg := h.config()
		cfg.Issuer = p.srv.URL
		p.srv.Close()
		if _, err := New(ctx, cfg, h.store); err == nil {
			t.Error("New succeeded against a dead provider")
		}
	})
	t.Run("default client verifies TLS, has a timeout and carries the whole flow", func(t *testing.T) {
		// Without a client in ctx the default one is used. It must refuse the
		// provider's self-signed certificate...
		if _, err := New(context.Background(), h.config(), h.store); err == nil {
			t.Fatal("New talked to a provider with an untrusted certificate")
		}
		// ...and once the certificate is trusted it must work end to end.
		tr := http.DefaultTransport.(*http.Transport)
		old := tr.TLSClientConfig
		tr.TLSClientConfig = &tls.Config{RootCAs: h.pool}
		t.Cleanup(func() { tr.TLSClientConfig = old; tr.CloseIdleConnections() })
		a, err := New(context.Background(), h.config(), h.store)
		if err != nil {
			t.Fatal(err)
		}
		if a.client.Timeout != httpTimeout {
			t.Errorf("default client timeout = %v, want %v", a.client.Timeout, httpTimeout)
		}
		h.auth, h.mux = a, http.NewServeMux()
		a.Register(h.mux)
		h.signIn()
	})
}

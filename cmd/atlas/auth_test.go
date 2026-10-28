package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-jose/go-jose/v4"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/mountain/realms"
)

// fakeIdP is an in-process OIDC provider: discovery, JWKS, and a token endpoint that checks the PKCE verifier and
// returns an RS256 ID token carrying the nonce and the groups of the code.
type fakeIdP struct {
	*httptest.Server
	key   *rsa.PrivateKey
	mu    sync.Mutex
	codes map[string]grant // code -> what /authorize saw
}

type grant struct {
	challenge, nonce string
	groups           []string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := &fakeIdP{key: key, codes: map[string]grant{}}
	mux := http.NewServeMux()
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.URL, "authorization_endpoint": p.URL + "/authorize",
			"token_endpoint": p.URL + "/token", "jwks_uri": p.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		g, ok := p.codes[r.PostForm.Get("code")]
		delete(p.codes, r.PostForm.Get("code"))
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 300,
			"id_token": p.sign(t, map[string]any{"iss": p.URL, "aud": "mountain", "sub": "u1", "preferred_username": "ana_k", "nonce": g.nonce, "groups": g.groups,
				"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix()})})
	})
	return p
}

func (p *fakeIdP) sign(t *testing.T, claims map[string]any) string {
	s, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		t.Fatal(err)
	}
	o, _ := s.Sign(mustJSON(claims))
	raw, _ := o.CompactSerialize()
	return raw
}

// tapp is a test's App, with a memory-only store and no live polls, and its routes. A test changes who gets in on
// a.gate, which the routes read on each request.
type tapp struct {
	*App
	h http.Handler
}

func testApp(t *testing.T) *tapp {
	t.Helper()
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	a := newApp(Opts{}, rl, www, &gate{}, &Graphs{}, nil)
	return &tapp{a, routes(a, www)}
}

func (a *tapp) serveReq(method, target string, cookies []*http.Cookie, hdr ...string) *http.Response {
	r := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w.Result()
}

func cookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// signIn runs /auth/login → (the IdP authorizes groups) → /auth/callback and returns the callback's response.
func signIn(t *testing.T, a *tapp, p *fakeIdP, groups []string, tamper func(q url.Values)) *http.Response {
	t.Helper()
	resp := a.serveReq("GET", "/auth/login?return=/%23/L1/fleet", nil)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	fc := cookie(resp, flowCookie)
	if fc == nil || !fc.Secure || !fc.HttpOnly || fc.SameSite != http.SameSiteLaxMode || fc.MaxAge > 600 {
		t.Fatalf("flow cookie: %+v", fc)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	q := loc.Query()
	if !strings.HasPrefix(loc.String(), p.URL+"/authorize?") || q.Get("code_challenge_method") != "S256" || q.Get("state") == "" ||
		q.Get("nonce") == "" || q.Get("redirect_uri") != origin+"/auth/callback" || !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("authorize URL: %s", loc)
	}
	code := rand.Text()
	p.mu.Lock()
	p.codes[code] = grant{q.Get("code_challenge"), q.Get("nonce"), groups}
	p.mu.Unlock()
	cb := url.Values{"code": {code}, "state": {q.Get("state")}}
	if tamper != nil {
		tamper(cb)
	}
	return a.serveReq("GET", "/auth/callback?"+cb.Encode(), []*http.Cookie{fc})
}

// withAuth is a test App with sign-in configured against a fake IdP.
func withAuth(t *testing.T) (*tapp, *fakeIdP) {
	t.Helper()
	p := newFakeIdP(t)
	a := testApp(t)
	a.gate.auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	return a, p
}

func adminCookie(t *testing.T, a *tapp, p *fakeIdP) *http.Cookie {
	t.Helper()
	resp := signIn(t, a, p, []string{"platform-admins"}, nil)
	sc := cookie(resp, sessionCookie)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/#/L1/fleet" || sc == nil ||
		!sc.Secure || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.MaxAge > 3600 {
		t.Fatalf("callback: %d %s %+v", resp.StatusCode, resp.Header.Get("Location"), sc)
	}
	return sc
}

// TestSignIn: only platform-admins get a session; anyone else gets a refusal page and no cookie. Forged, tampered and
// misused cookies, and the town's role cookie, are not sessions.
func TestSignIn(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	for _, groups := range [][]string{{"mountain-visitors"}, {"wird-users"}, nil} {
		r := signIn(t, a, p, groups, nil)
		if r.StatusCode != http.StatusForbidden || cookie(r, sessionCookie) == nil || cookie(r, sessionCookie).MaxAge >= 0 {
			t.Errorf("groups %v: %d, cookie %+v (want 403 and the cookie cleared)", groups, r.StatusCode, cookie(r, sessionCookie))
		}
	}
	isAdm := func(c *http.Cookie) bool {
		var m struct{ Admin bool }
		_ = json.NewDecoder(a.serveReq("GET", "/auth/me", []*http.Cookie{c}).Body).Decode(&m)
		return m.Admin
	}
	if !isAdm(admin) {
		t.Error("admin: /auth/me says not admin")
	}
	forged := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(`{"v":{"Admin":true,"Sub":"x"},"exp":9999999999}`)) + ".AAAA"}
	tampered := &http.Cookie{Name: sessionCookie, Value: strings.Replace(admin.Value, "x", "y", 1) + "A"}
	fc := cookie(a.serveReq("GET", "/auth/login", nil), flowCookie)
	asSession := &http.Cookie{Name: sessionCookie, Value: fc.Value} // a flow cookie's MAC is bound to its name
	b := mustJSON(map[string]any{"v": map[string]string{"Role": "admin", "Sub": "u1", "Name": "ana_k", "ID": "t1"}, "exp": time.Now().Add(time.Hour).Unix()})
	town := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + a.gate.auth.mac(sessionCookie, b)} // R7-R9 shape, same key
	for name, c := range map[string]*http.Cookie{"forged": forged, "tampered": tampered, "flow as session": asSession, "town role": town} {
		if isAdm(c) {
			t.Errorf("%s cookie is an admin", name)
		}
	}
	if r := signIn(t, a, p, []string{"platform-admins"}, func(q url.Values) { q.Set("state", "x") }); r.StatusCode != http.StatusBadRequest || cookie(r, sessionCookie) != nil {
		t.Errorf("bad state: %d", r.StatusCode)
	}
	if r := signIn(t, a, p, []string{"platform-admins"}, func(q url.Values) { q.Set("code", "nope") }); r.StatusCode != http.StatusForbidden || cookie(r, sessionCookie) != nil {
		t.Errorf("unknown code: %d", r.StatusCode)
	}
	for _, ret := range []string{"//evil.example", "/\\evil.example", "https://evil.example", "/x/../..//evil", "/#a\\b"} {
		if returnRE.MatchString(ret) {
			t.Errorf("return %q accepted", ret)
		}
	}
	if !returnRE.MatchString("/#/L1/fleet?lens=land") {
		t.Error("an atlas route is refused as a return view")
	}
	if r := a.serveReq("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", "https://evil.example"); r.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin logout: %d", r.StatusCode)
	}
	if r := a.serveReq("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", origin); r.StatusCode != http.StatusSeeOther || cookie(r, sessionCookie).MaxAge >= 0 {
		t.Errorf("logout: %d", r.StatusCode)
	}
}

// TestRoutes is the deny-by-default gate (plan/10 Security): every route of the table, called by an anonymous
// caller, a forged cookie and the town's visitor cookie, is refused unless it is one of the open ones; the page
// redirects to the sign-in instead of a bare 403. An admin reaches every route. With sign-in off (and no --dev on
// loopback), every closed route refuses too.
func TestRoutes(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	www, _ := fs.Sub(mountain.WWW, "www")
	b := mustJSON(map[string]any{"v": map[string]string{"Role": "visitor", "Sub": "u2", "ID": "v2"}, "exp": time.Now().Add(time.Hour).Unix()})
	visitor := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + a.gate.auth.mac(sessionCookie, b)}
	forged := &http.Cookie{Name: sessionCookie, Value: "eyJ2Ijp7IkFkbWluIjp0cnVlfSwiZXhwIjo5OTk5OTk5OTk5fQ.AAAA"}
	open := map[string]bool{}
	for _, rt := range routeTable(a.App, www) {
		method, path, _ := strings.Cut(rt.pattern, " ")
		if rt.open {
			open[rt.pattern] = true
			continue
		}
		for name, cs := range map[string][]*http.Cookie{"anonymous": nil, "visitor": {visitor}, "forged": {forged}} {
			r := a.serveReq(method, path, cs, "Origin", origin)
			want := http.StatusForbidden
			if path == "/" {
				want = http.StatusFound
			}
			if r.StatusCode != want || r.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s %s: %d %q, want %d", name, rt.pattern, r.StatusCode, r.Header.Get("Cache-Control"), want)
			}
		}
		if r := a.serveReq(method, path, []*http.Cookie{admin}, "Origin", origin); r.StatusCode == http.StatusForbidden || r.StatusCode == http.StatusFound {
			t.Errorf("admin %s: %d", rt.pattern, r.StatusCode)
		}
	}
	for _, path := range []string{"/v/000000000000/js/main.js", "/js/main.js", "/api/other", "/dev/gate.mjs", "/index.html"} { // the catch-all
		if r := a.serveReq("GET", path, nil); r.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s: %d", path, r.StatusCode)
		}
	}
	if r := a.serveReq("GET", "/?q=low", nil); r.Header.Get("Location") != "/auth/login?return=%2F%3Fq%3Dlow" {
		t.Errorf("the page's query is lost on the way to the sign-in: %q", r.Header.Get("Location"))
	}
	for _, dir := range []string{"/js/", "/vendor/"} {
		if r := a.serveReq("GET", dir, []*http.Cookie{admin}); r.StatusCode != http.StatusNotFound {
			t.Errorf("admin %s: %d, want no directory listing", dir, r.StatusCode)
		}
	}
	if len(open) != 5 || !open["GET /healthz"] || !open["GET /auth/me"] {
		t.Errorf("open routes changed: %v (a new open route is a new public endpoint: add it here on purpose)", open)
	}
	// sign-in off: refused, the page too (no sign-in to send it to)
	a.gate.auth = nil
	if r := a.serveReq("GET", "/", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("auth off, /: %d", r.StatusCode)
	}
	if r := a.serveReq("GET", "/api/graph", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("auth off, /api/graph: %d", r.StatusCode)
	}
}

// TestDevOpen: only --dev on a loopback address opens the routes with sign-in off.
func TestDevOpen(t *testing.T) {
	for addr, want := range map[string]bool{"127.0.0.1:8080": true, "[::1]:8080": true, "localhost:1": true, ":8080": false, "0.0.0.0:8080": false, "10.0.0.1:80": false} {
		if loopback(addr) != want {
			t.Errorf("loopback(%q) = %v", addr, !want)
		}
	}
}

// TestSocket: an admin's socket gets the hello, then the live frames and events; it closes when the session lapses.
func TestSocket(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	srv := httptest.NewServer(a.h)
	defer srv.Close()
	dial := func(c *http.Cookie, o string) (*websocket.Conn, *http.Response, error) {
		hd := http.Header{"Origin": {o}}
		if c != nil {
			hd.Set("Cookie", c.String())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		return websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: hd})
	}
	if _, r, err := dial(nil, origin); err == nil || r == nil || r.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous socket: %v %v", err, r)
	}
	if _, r, err := dial(admin, "https://evil.example"); err == nil || r == nil || r.StatusCode != http.StatusForbidden {
		t.Errorf("foreign origin: %v %v", err, r)
	}
	conn, _, err := dial(admin, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" || f["live"] == nil || f["build"] != a.Build {
		t.Fatalf("hello: %v", f)
	}
	a.Live.event("graph", "", "projection 2")
	if f := readT(t, conn); f["t"] != "ev" || f["kind"] != "graph" {
		t.Errorf("event: %v", f)
	}
}

// TestSocketEnd: a person's socket ends at the session end adminOnly put in the context, so the hub mounted without
// the gate refuses rather than open one that never ends; under devOpen (no session) it opens.
func TestSocketEnd(t *testing.T) {
	a := testApp(t)
	dial := func(h http.Handler) (*websocket.Conn, *http.Response, error) {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		return websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
	}
	a.gate.devOpen = true
	if _, r, err := dial(a.Hub); err == nil || r == nil || r.StatusCode != http.StatusForbidden {
		t.Errorf("the hub without the gate: %v %v", err, r)
	}
	conn, _, err := dial(a.h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" {
		t.Errorf("devOpen hello: %v", f)
	}
}

// TestSocketLapse: a signed-in admin's socket closes at the session's expiry, the end adminOnly admitted it with.
func TestSocketLapse(t *testing.T) {
	a, _ := withAuth(t)
	srv := httptest.NewServer(a.h)
	defer srv.Close()
	exp := time.Unix(time.Now().Add(3*time.Second).Unix(), 0)
	b := mustJSON(map[string]any{"v": Session{Admin: true, Sub: "u1", Name: "ana_k", ID: "s1"}, "exp": exp.Unix()})
	c := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + a.gate.auth.mac(sessionCookie, b)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}, "Cookie": {c.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" {
		t.Fatalf("hello: %v", f)
	}
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			break
		}
	}
	if end := time.Now(); end.Before(exp.Add(-time.Second)) || end.After(exp.Add(3*time.Second)) {
		t.Errorf("the socket ended at %v, the session at %v", end.Format(time.StampMilli), exp.Format(time.StampMilli))
	}
}

func readT(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	_ = json.Unmarshal(b, &f)
	return f
}

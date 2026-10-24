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
	"github.com/MohammadBnei/mountain/internal/realms"
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
		json.NewEncoder(w).Encode(map[string]any{"issuer": p.URL, "authorization_endpoint": p.URL + "/authorize",
			"token_endpoint": p.URL + "/token", "jwks_uri": p.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
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
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 300,
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

// testApp is an App with a memory-only store and no live polls, its routes in app's mux.
func testApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	a := &App{Realms: rl, Build: buildID(www), Graph: &Graphs{}}
	a.Hub = NewHub(false, a.hello)
	a.Live = newLive(rl, a.Hub)
	saved := app
	app = a
	t.Cleanup(func() { app = saved })
	return a, routes(a, www)
}

var mux http.Handler // the routes the auth helpers call

func serveReq(method, target string, cookies []*http.Cookie, hdr ...string) *http.Response {
	r := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
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
func signIn(t *testing.T, p *fakeIdP, groups []string, tamper func(q url.Values)) *http.Response {
	t.Helper()
	resp := serveReq("GET", "/auth/login?return=/%23/L1/fleet", nil)
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
	return serveReq("GET", "/auth/callback?"+cb.Encode(), []*http.Cookie{fc})
}

func withAuth(t *testing.T) *fakeIdP {
	t.Helper()
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })
	_, mux = testApp(t)
	return p
}

func adminCookie(t *testing.T, p *fakeIdP) *http.Cookie {
	t.Helper()
	resp := signIn(t, p, []string{"platform-admins"}, nil)
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
	p := withAuth(t)
	admin := adminCookie(t, p)
	for _, groups := range [][]string{{"mountain-visitors"}, {"wird-users"}, nil} {
		r := signIn(t, p, groups, nil)
		if r.StatusCode != http.StatusForbidden || cookie(r, sessionCookie) == nil || cookie(r, sessionCookie).MaxAge >= 0 {
			t.Errorf("groups %v: %d, cookie %+v (want 403 and the cookie cleared)", groups, r.StatusCode, cookie(r, sessionCookie))
		}
	}
	isAdm := func(c *http.Cookie) bool {
		var m struct{ Admin bool }
		json.NewDecoder(serveReq("GET", "/auth/me", []*http.Cookie{c}).Body).Decode(&m)
		return m.Admin
	}
	if !isAdm(admin) {
		t.Error("admin: /auth/me says not admin")
	}
	forged := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(`{"v":{"Admin":true,"Sub":"x"},"exp":9999999999}`)) + ".AAAA"}
	tampered := &http.Cookie{Name: sessionCookie, Value: strings.Replace(admin.Value, "x", "y", 1) + "A"}
	fc := cookie(serveReq("GET", "/auth/login", nil), flowCookie)
	asSession := &http.Cookie{Name: sessionCookie, Value: fc.Value} // a flow cookie's MAC is bound to its name
	b := mustJSON(map[string]any{"v": map[string]string{"Role": "admin", "Sub": "u1", "Name": "ana_k"}, "exp": time.Now().Add(time.Hour).Unix()})
	town := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + auth.mac(sessionCookie, b)} // R7-R9 shape, same key
	for name, c := range map[string]*http.Cookie{"forged": forged, "tampered": tampered, "flow as session": asSession, "town role": town} {
		if isAdm(c) {
			t.Errorf("%s cookie is an admin", name)
		}
	}
	if r := signIn(t, p, []string{"platform-admins"}, func(q url.Values) { q.Set("state", "x") }); r.StatusCode != http.StatusBadRequest || cookie(r, sessionCookie) != nil {
		t.Errorf("bad state: %d", r.StatusCode)
	}
	if r := signIn(t, p, []string{"platform-admins"}, func(q url.Values) { q.Set("code", "nope") }); r.StatusCode != http.StatusForbidden || cookie(r, sessionCookie) != nil {
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
	if r := serveReq("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", "https://evil.example"); r.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin logout: %d", r.StatusCode)
	}
	if r := serveReq("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", origin); r.StatusCode != http.StatusSeeOther || cookie(r, sessionCookie).MaxAge >= 0 {
		t.Errorf("logout: %d", r.StatusCode)
	}
}

// TestRoutes is the deny-by-default gate (plan/10 Security): every route of the table, called by an anonymous
// caller, a forged cookie and the town's visitor cookie, is refused unless it is one of the open ones; the page
// redirects to the sign-in instead of a bare 403. An admin reaches every route. With sign-in off (and no --dev on
// loopback), every closed route refuses too.
func TestRoutes(t *testing.T) {
	p := withAuth(t)
	admin := adminCookie(t, p)
	a, _ := testApp(t)
	www, _ := fs.Sub(mountain.WWW, "www")
	b := mustJSON(map[string]any{"v": map[string]string{"Role": "visitor", "Sub": "u2"}, "exp": time.Now().Add(time.Hour).Unix()})
	visitor := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + auth.mac(sessionCookie, b)}
	forged := &http.Cookie{Name: sessionCookie, Value: "eyJ2Ijp7IkFkbWluIjp0cnVlfSwiZXhwIjo5OTk5OTk5OTk5fQ.AAAA"}
	open := map[string]bool{}
	for _, rt := range routeTable(a, www) {
		method, path, _ := strings.Cut(rt.pattern, " ")
		if rt.open {
			open[rt.pattern] = true
			continue
		}
		for name, cs := range map[string][]*http.Cookie{"anonymous": nil, "visitor": {visitor}, "forged": {forged}} {
			r := serveReq(method, path, cs, "Origin", origin)
			want := http.StatusForbidden
			if path == "/" {
				want = http.StatusFound
			}
			if r.StatusCode != want || r.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s %s: %d %q, want %d", name, rt.pattern, r.StatusCode, r.Header.Get("Cache-Control"), want)
			}
		}
		if r := serveReq(method, path, []*http.Cookie{admin}, "Origin", origin); r.StatusCode == http.StatusForbidden || r.StatusCode == http.StatusFound {
			t.Errorf("admin %s: %d", rt.pattern, r.StatusCode)
		}
	}
	for _, path := range []string{"/v/000000000000/js/main.js", "/js/main.js", "/api/other", "/dev/gate.mjs", "/index.html"} { // the catch-all
		if r := serveReq("GET", path, nil); r.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s: %d", path, r.StatusCode)
		}
	}
	if len(open) != 5 || !open["GET /healthz"] || !open["GET /auth/me"] {
		t.Errorf("open routes changed: %v (a new open route is a new public endpoint: add it here on purpose)", open)
	}
	// sign-in off: refused, the page too (no sign-in to send it to)
	auth = nil
	if r := serveReq("GET", "/", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("auth off, /: %d", r.StatusCode)
	}
	if r := serveReq("GET", "/api/graph", nil); r.StatusCode != http.StatusForbidden {
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
	p := withAuth(t)
	admin := adminCookie(t, p)
	a, h := testApp(t)
	srv := httptest.NewServer(h)
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
	defer conn.CloseNow()
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" || f["live"] == nil || f["build"] != a.Build {
		t.Fatalf("hello: %v", f)
	}
	a.Live.event("graph", "", "projection 2")
	if f := readT(t, conn); f["t"] != "ev" || f["kind"] != "graph" {
		t.Errorf("event: %v", f)
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
	json.Unmarshal(b, &f)
	return f
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-jose/go-jose/v4"
)

// fakeIdP is an in-process OIDC provider: discovery, JWKS, and a token endpoint that checks the PKCE verifier
// and returns an RS256 ID token carrying the nonce and the groups of the code.
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
			"id_token": p.sign(t, map[string]any{"iss": p.URL, "aud": "mountain", "sub": "u1", "nonce": g.nonce, "groups": g.groups,
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

// serve runs one request through the public mux (the /auth routes registered in init).
func serveAuth(method, target string, cookies []*http.Cookie, hdr ...string) *http.Response {
	r := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	Public.ServeHTTP(w, r)
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
// tamper may change the callback query before it is sent.
func signIn(t *testing.T, p *fakeIdP, groups []string, tamper func(q url.Values)) *http.Response {
	t.Helper()
	resp := serveAuth("GET", "/auth/login?return=/%23detail", nil)
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
	return serveAuth("GET", "/auth/callback?"+cb.Encode(), []*http.Cookie{fc})
}

func session(t *testing.T, p *fakeIdP, groups ...string) *http.Cookie {
	t.Helper()
	resp := signIn(t, p, groups, nil)
	sc := cookie(resp, sessionCookie)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/#detail" || sc == nil ||
		!sc.Secure || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.MaxAge > 3600 {
		t.Fatalf("callback: %d %s %+v", resp.StatusCode, resp.Header.Get("Location"), sc)
	}
	payload, _, _ := strings.Cut(sc.Value, ".")
	b, _ := base64.RawURLEncoding.DecodeString(payload)
	var m map[string]any
	if json.Unmarshal(b, &m) != nil || len(m) != 2 || m["exp"] == nil { // {v: role, exp}: no token, no claims
		t.Fatalf("session payload %s", b)
	}
	return sc
}

func roleOfCookie(c *http.Cookie) string {
	var m struct {
		Enabled bool
		Role    string
	}
	json.NewDecoder(serveAuth("GET", "/auth/me", []*http.Cookie{c}).Body).Decode(&m)
	return m.Role
}

// TestDetailMode is the W3 gate: non-admins never receive detail frames, admins do.
func TestDetailMode(t *testing.T) {
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })

	admin, visitor, other := session(t, p, "platform-admins"), session(t, p, "mountain-visitors"), session(t, p, "wird-users")
	for c, want := range map[*http.Cookie]string{admin: "admin", visitor: "visitor", other: "view"} {
		if got := roleOfCookie(c); got != want {
			t.Errorf("role: got %s, want %s", got, want)
		}
	}

	// forged, tampered and misused cookies are views
	forged := &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(`{"v":"admin","exp":9999999999}`)) + ".AAAA"}
	tampered := &http.Cookie{Name: sessionCookie, Value: strings.Replace(visitor.Value, "x", "y", 1) + "A"}
	fc := cookie(serveAuth("GET", "/auth/login", nil), flowCookie)
	asSession := &http.Cookie{Name: sessionCookie, Value: fc.Value} // a flow cookie's MAC is bound to its name
	for _, c := range []*http.Cookie{forged, tampered, asSession} {
		if got := roleOfCookie(c); got != "view" {
			t.Errorf("cookie %q: role %s, want view", c.Value, got)
		}
	}
	// a bad state, a wrong verifier (code replay) or a hostile return view never yield a session
	if r := signIn(t, p, []string{"platform-admins"}, func(q url.Values) { q.Set("state", "x") }); r.StatusCode != http.StatusBadRequest || cookie(r, sessionCookie) != nil {
		t.Errorf("bad state: %d", r.StatusCode)
	}
	if r := signIn(t, p, []string{"platform-admins"}, func(q url.Values) { q.Set("code", "nope") }); r.StatusCode != http.StatusForbidden || cookie(r, sessionCookie) != nil {
		t.Errorf("unknown code: %d", r.StatusCode)
	}
	for _, ret := range []string{"//evil.example", "/\\evil.example", "https://evil.example", "/x/../..//evil", "/#a/../..//evil"} {
		if returnRE.MatchString(ret) {
			t.Errorf("return %q accepted", ret)
		}
	}
	// logout: POST from the page clears the cookie; another origin gets 403
	if r := serveAuth("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", "https://evil.example"); r.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin logout: %d", r.StatusCode)
	}
	if r := serveAuth("POST", "/auth/logout", []*http.Cookie{admin}, "Origin", origin); r.StatusCode != http.StatusSeeOther || cookie(r, sessionCookie).MaxAge >= 0 {
		t.Errorf("logout: %d", r.StatusCode)
	}

	// the WS: each role's connection hears the same public frames; only the admin's hears the detail
	a, _ := testApp(t)
	srv := httptest.NewServer(a.Hub)
	defer srv.Close()
	conns := map[string]*websocket.Conn{}
	for name, c := range map[string]*http.Cookie{"admin": admin, "visitor": visitor, "other": other, "anon": nil} { // perIPMax = 4
		h := http.Header{"Origin": {origin}}
		if c != nil {
			h.Set("Cookie", c.String())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{HTTPHeader: h})
		if err != nil {
			t.Fatal(name, err)
		}
		defer conn.CloseNow()
		conn.SetReadLimit(-1)
		readT(t, conn) // hello: registered
		conns[name] = conn
	}
	out := &Out{app: a, name: "alerts"}
	id := out.Fire(Alert{FP: "0123456789abcdef", Sev: "warning", Layer: "k8s", StartsAt: 1})
	out.Detail(id, map[string]any{"labels": map[string]string{"alertname": "KubePodCrashLooping", "namespace": "secret-ns"}})
	end := out.Fire(Alert{FP: "fedcba9876543210", Sev: "warning", Layer: "k8s", StartsAt: 2}) // sentinel, no detail
	for name, conn := range conns {
		var got []string
		for {
			f := readT(t, conn)
			got = append(got, f["t"].(string))
			if f["id"] == end {
				break
			}
			if f["t"] == "detail" && (f["id"] != id || !strings.Contains(string(mustJSON(f)), "secret-ns")) {
				t.Errorf("%s: bad detail %v", name, f)
			}
		}
		if hasDetail := strings.Contains(strings.Join(got, ","), "detail"); hasDetail != (name == "admin") {
			t.Errorf("%s: frames %v", name, got)
		}
	}
	// the session expires (or was signed out / lost its group): the open admin socket stops hearing details
	a.Hub.mu.Lock()
	for c := range a.Hub.clients {
		c.RoleExp = time.Now().Add(-time.Second)
	}
	a.Hub.mu.Unlock()
	out.Detail(id, map[string]any{"labels": map[string]string{"namespace": "secret-ns"}})
	end = out.Fire(Alert{FP: "00000000000000aa", Sev: "warning", Layer: "k8s", StartsAt: 3})
	for f := readT(t, conns["admin"]); f["id"] != end; f = readT(t, conns["admin"]) {
		if f["t"] == "detail" {
			t.Errorf("admin past its session exp got a detail")
		}
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

// TestDBDetail: a detail lands on its event's row, never in public, and the snapshot clears it after 30 days (02).
func TestDBDetail(t *testing.T) {
	a := storedApp(t, testDSN(t))
	out := &Out{app: a, name: "alerts"}
	old := out.Fire(Alert{FP: a.FP("old"), Sev: "warning", Layer: "k8s", StartsAt: 1})
	out.Detail(old, map[string]any{"labels": map[string]string{"namespace": "old-ns"}})
	if !a.store.flush(5 * time.Second) {
		t.Fatal("flush")
	}
	a.store.pool.Exec(context.Background(), "UPDATE events SET day = current_date - 31 WHERE id = $1", old)
	id := out.Fire(Alert{FP: a.FP("new"), Sev: "warning", Layer: "k8s", StartsAt: 2})
	out.Detail(id, map[string]any{"labels": map[string]string{"namespace": "secret-ns"}})
	a.snapshot(time.Now()) // the retention runs with the snapshot
	if !a.store.flush(5 * time.Second) {
		t.Fatal("flush")
	}
	var pub, det string
	var oldDet *string
	ctx := context.Background()
	if err := a.store.pool.QueryRow(ctx, "SELECT public::text, detail::text FROM events WHERE id = $1", id).Scan(&pub, &det); err != nil {
		t.Fatal(err)
	}
	a.store.pool.QueryRow(ctx, "SELECT detail::text FROM events WHERE id = $1", old).Scan(&oldDet)
	if strings.Contains(pub, "secret-ns") || !strings.Contains(det, "secret-ns") || oldDet != nil {
		t.Errorf("public %s, detail %s, 31-day-old detail %v", pub, det, oldDet)
	}
}

// TestAuthOff: without the env the relay starts with detail mode off; /auth/login is a 404 and /auth/me says so.
func TestAuthOff(t *testing.T) {
	auth = authFromEnv(false)
	if auth != nil {
		t.Skip("MOUNTAIN_OIDC_* set in the environment")
	}
	if r := serveAuth("GET", "/auth/login", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("login: %d", r.StatusCode)
	}
	if role := roleOfCookie(&http.Cookie{Name: sessionCookie, Value: "x"}); role != "view" {
		t.Errorf("role %s", role)
	}
}

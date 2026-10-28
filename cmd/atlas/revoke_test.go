package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain"
	"github.com/coder/websocket"
)

// signed is a session cookie for s, MAC'd with the test key (an hour from now).
func signed(a *tapp, s Session) *http.Cookie {
	b := mustJSON(map[string]any{"v": s, "exp": time.Now().Add(time.Hour).Unix()})
	return &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(b) + "." + a.gate.auth.mac(sessionCookie, b)}
}

func graphStatus(a *tapp, c *http.Cookie) int {
	return a.serveReq("GET", "/api/graph", []*http.Cookie{c}, "Origin", origin).StatusCode
}

func logout(a *tapp, c *http.Cookie, from string) int {
	return a.serveReq("POST", "/auth/logout", []*http.Cookie{c}, "Origin", from).StatusCode
}

// TestLogoutRevokes: a sign-out ends that session on the server, so a copy of its cookie is refused after it, on
// every closed route; another sign-in of the same person stays; a cross-origin sign-out revokes nothing, nor does one
// whose cookie fails its MAC (even carrying the real session's id).
func TestLogoutRevokes(t *testing.T) {
	a, p := withAuth(t)
	one, two := adminCookie(t, a, p), adminCookie(t, a, p) // the same person, twice
	if graphStatus(a, one) == http.StatusForbidden || graphStatus(a, two) == http.StatusForbidden {
		t.Fatal("a fresh sign-in is refused") // the positive control
	}
	// cross-origin: refused before anything is revoked
	if code := logout(a, one, "https://evil.example"); code != http.StatusForbidden {
		t.Errorf("cross-origin logout: %d", code)
	}
	// the real session's payload with a broken MAC
	forged := &http.Cookie{Name: sessionCookie, Value: strings.Replace(one.Value, ".", ".x", 1)}
	if code := logout(a, forged, origin); code != http.StatusSeeOther {
		t.Errorf("logout with a forged cookie: %d", code)
	}
	if graphStatus(a, one) == http.StatusForbidden {
		t.Fatal("a cross-origin or forged sign-out revoked the session")
	}
	if code := logout(a, one, origin); code != http.StatusSeeOther {
		t.Fatalf("logout: %d", code)
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	for _, rt := range routeTable(a.App, www) {
		if rt.open {
			continue
		}
		method, path, _ := strings.Cut(rt.pattern, " ")
		want := http.StatusForbidden
		if path == "/" {
			want = http.StatusFound
		}
		if r := a.serveReq(method, path, []*http.Cookie{one}, "Origin", origin); r.StatusCode != want {
			t.Errorf("the signed-out copy on %s: %d, want %d", rt.pattern, r.StatusCode, want)
		}
	}
	if a.gate.isAdmin(httptest.NewRequest("GET", "/", nil)) {
		t.Error("no cookie is an admin")
	}
	var me struct{ Admin bool }
	if r := a.serveReq("GET", "/auth/me", []*http.Cookie{one}); json.NewDecoder(r.Body).Decode(&me) != nil || me.Admin {
		t.Error("/auth/me: the signed-out copy is an admin")
	}
	if graphStatus(a, two) == http.StatusForbidden {
		t.Error("the other sign-in was revoked with the first")
	}
}

// TestSessionShape: a session needs an id (a cookie from before ids is no session) and the admin claim (an id alone
// admits nobody), on every closed route.
func TestSessionShape(t *testing.T) {
	a, _ := withAuth(t)
	www, _ := fs.Sub(mountain.WWW, "www")
	for name, c := range map[string]*http.Cookie{
		"no id":     signed(a, Session{Admin: true, Sub: "u1", Name: "ana_k"}),
		"not admin": signed(a, Session{Admin: false, Sub: "u2", ID: "i2"}),
		"no sub":    signed(a, Session{Admin: true, ID: "i3"}),
	} {
		for _, rt := range routeTable(a.App, www) {
			if rt.open {
				continue
			}
			method, path, _ := strings.Cut(rt.pattern, " ")
			want := http.StatusForbidden
			if path == "/" {
				want = http.StatusFound
			}
			if r := a.serveReq(method, path, []*http.Cookie{c}, "Origin", origin); r.StatusCode != want {
				t.Errorf("%s on %s: %d, want %d", name, rt.pattern, r.StatusCode, want)
			}
		}
	}
	if graphStatus(a, signed(a, Session{Admin: true, Sub: "u1", ID: "i1"})) == http.StatusForbidden {
		t.Error("a whole session is refused") // the positive control
	}
}

// TestRevokedPruned: a revoked id is kept until its cookie's own end, then dropped (the set stays bounded).
func TestRevokedPruned(t *testing.T) {
	a, _ := withAuth(t)
	au := a.gate.auth
	au.revoke("old", time.Now().Add(-time.Second))
	au.revoke("live", time.Now().Add(time.Hour))
	au.pruned = time.Time{} // the next revoke sweeps
	au.revoke("new", time.Now().Add(time.Hour))
	if au.isRevoked("old") || !au.isRevoked("live") || !au.isRevoked("new") {
		t.Errorf("after the sweep: old %v live %v new %v", au.isRevoked("old"), au.isRevoked("live"), au.isRevoked("new"))
	}
}

// TestSocketSignedOut: a sign-out closes the session's open socket at its next ping, not at the cookie's expiry;
// another session's socket stays.
func TestSocketSignedOut(t *testing.T) {
	a, p := withAuth(t)
	a.Hub.ping = 100 * time.Millisecond
	srv := httptest.NewServer(a.h)
	defer srv.Close()
	one, two := adminCookie(t, a, p), adminCookie(t, a, p)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dial := func(c *http.Cookie) *websocket.Conn {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}, "Cookie": {c.String()}}})
		if err != nil {
			t.Fatal(err)
		}
		conn.SetReadLimit(-1)
		if f := readT(t, conn); f["t"] != "hello" {
			t.Fatalf("hello: %v", f)
		}
		return conn
	}
	c1, c2 := dial(one), dial(two)
	defer c1.CloseNow()
	defer c2.CloseNow()
	if code := logout(a, one, origin); code != http.StatusSeeOther {
		t.Fatalf("logout: %d", code)
	}
	start := time.Now()
	for {
		if _, _, err := c1.Read(ctx); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Errorf("the signed-out socket ended with %v", err)
			}
			break
		}
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the signed-out socket took %v to close", d)
	}
	rctx, rc := context.WithTimeout(ctx, 500*time.Millisecond) // several pings: the other socket stays open
	defer rc()
	if _, _, err := c2.Read(rctx); err != nil && rctx.Err() == nil {
		t.Errorf("the other session's socket closed: %v", err)
	}
}

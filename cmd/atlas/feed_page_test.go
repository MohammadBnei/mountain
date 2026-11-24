package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func bodyOf(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFeedPageOff: with the feed off, /feed answers as /api/feed does (404 to the admin, 403 to anyone else, no
// sign-in), the map has no entry to it and no CSP names core.
func TestFeedPageOff(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	for _, path := range []string{"/feed", "/feed/pr-0011223344556677", "/feed.html", "/api/feed"} {
		if r := a.serveReq("GET", path, []*http.Cookie{admin}); r.StatusCode != http.StatusNotFound || strings.Contains(r.Header.Get("Content-Security-Policy"), coreOrigin) {
			t.Errorf("admin %s: %d %q", path, r.StatusCode, r.Header.Get("Content-Security-Policy"))
		}
		if r := a.serveReq("GET", path, nil); r.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s: %d", path, r.StatusCode)
		}
	}
	r := a.serveReq("GET", "/", []*http.Cookie{admin})
	if html := bodyOf(t, r); strings.Contains(html, "feed") {
		t.Error("the map names the feed")
	}
	const want = "connect-src 'self' wss://mountain.bnei.dev;"
	if got := r.Header.Get("Content-Security-Policy"); !strings.Contains(got, want) {
		t.Errorf("CSP %q, want %q in it", got, want)
	}
}

func TestFeedPageOn(t *testing.T) {
	a, admin := feedOn(t)
	r := a.serveReq("GET", "/", []*http.Cookie{admin})
	if html := bodyOf(t, r); strings.Count(html, feedEntry) != 1 {
		t.Error("the map has no entry to the feed (index.html and feedEntry must agree)")
	}
	if strings.Contains(r.Header.Get("Content-Security-Policy"), coreOrigin) {
		t.Error("the map's CSP names core: only the feed's page may")
	}
	for _, path := range []string{"/feed", "/feed?kind=pr", "/feed/pr-0011223344556677"} {
		r := a.serveReq("GET", path, []*http.Cookie{admin})
		html, csp := bodyOf(t, r), r.Header.Get("Content-Security-Policy")
		if r.StatusCode != 200 || r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("Referrer-Policy") != "no-referrer" || r.Header.Get("ETag") == "" {
			t.Errorf("%s: %d %v", path, r.StatusCode, r.Header)
		}
		if !strings.Contains(csp, "connect-src 'self' wss://mountain.bnei.dev "+coreOrigin+";") || !strings.Contains(csp, "script-src 'self';") || !strings.Contains(csp, "style-src 'self';") {
			t.Errorf("%s: CSP %q", path, csp)
		}
		if strings.Contains(html, `"./`) || !strings.Contains(html, `src="/v/`+a.Build+`/js/feed.js"`) {
			t.Errorf("%s: an asset is not under /v/<build>/", path)
		}
		// anonymous: the sign-in, and back to the same place
		to := a.serveReq("GET", path, nil)
		if to.StatusCode != http.StatusFound || !strings.HasPrefix(to.Header.Get("Location"), "/auth/login?return=%2Ffeed") {
			t.Errorf("anonymous %s: %d %q", path, to.StatusCode, to.Header.Get("Location"))
		}
	}
	for _, path := range []string{"/feed.html", "/v/" + a.Build + "/feed.html", "/feed/x", "/feed/PR-0011223344556677", "/feed/pr-0011223344556677/x"} {
		if r := a.serveReq("GET", path, []*http.Cookie{admin}); r.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", path, r.StatusCode)
		}
		if r := a.serveReq("GET", path, nil); r.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s: %d", path, r.StatusCode)
		}
	}
	for ret, want := range map[string]bool{"/feed": true, "/feed?kind=pr": true, "/feed/pr-0011223344556677": true, "/feed//evil.example": false,
		"/feedx": false, "/feed/../x": false, "/feed/pr-0011223344556677/..": false, "/feed\n": false} {
		if returnRE.MatchString(ret) != want {
			t.Errorf("return %q: %v", ret, !want)
		}
	}
}

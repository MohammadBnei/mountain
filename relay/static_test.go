package main

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MohammadBnei/mountain"
)

// TestStaticRevalidates: without validators + no-cache, Cloudflare's default 4 h browser TTL
// mixed module versions across a deploy (live: old kit.js + new post.js => setComposed undefined).
func TestStaticRevalidates(t *testing.T) {
	h := static(fstest.MapFS{"js/kit.js": {Data: []byte("export const v = 1")}}, false)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/js/kit.js", nil))
	etag := rec.Header().Get("ETag")
	if rec.Header().Get("Cache-Control") != "no-cache" || etag == "" {
		t.Fatalf("want Cache-Control no-cache + ETag, got %q %q", rec.Header().Get("Cache-Control"), etag)
	}
	req := httptest.NewRequest("GET", "/js/kit.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("revalidation with the same ETag: want 304, got %d", rec.Code)
	}
}

// TestStaticVersioned: Cloudflare overrides Cache-Control, so / must point every asset at /v/<build>/
// (immutable, a new URL per deploy) and the CSP must hash the html actually served.
func TestStaticVersioned(t *testing.T) {
	idx, _ := fs.ReadFile(mountain.WWW, "www/index.html")
	h := static(fstest.MapFS{
		"index.html": {Data: idx},
		"js/main.js": {Data: []byte("export {}")},
	}, false)
	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}
	rec := get("/")
	html := rec.Body.String()
	m := regexp.MustCompile(`src="(/v/[0-9a-f]{12}/)js/main.js"`).FindStringSubmatch(html)
	if rec.Code != 200 || m == nil || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("/: want no-cache html with a /v/<build>/js/main.js script, got %d %q\n%s", rec.Code, rec.Header().Get("Cache-Control"), html)
	}
	prefix := m[1]
	for _, want := range []string{`href="` + prefix + `style.css"`, `"three":"` + prefix + `vendor/build/three.module.js"`, `"three/addons/":"` + prefix + `vendor/examples/jsm/"`} {
		if !strings.Contains(html, want) {
			t.Errorf("/: missing %s", want)
		}
	}
	for _, a := range regexp.MustCompile(`(?:src|href)="([^"]*)"`).FindAllStringSubmatch(html, -1) {
		if !strings.HasPrefix(a[1], prefix) && !strings.HasPrefix(a[1], "data:") {
			t.Errorf("/: unversioned asset URL %q", a[1])
		}
	}
	for _, s := range inlineScript.FindAllStringSubmatch(html, -1) {
		sum := sha256.Sum256([]byte(s[1]))
		if hash := base64.StdEncoding.EncodeToString(sum[:]); !strings.Contains(rec.Header().Get("Content-Security-Policy"), hash) {
			t.Errorf("CSP lacks the served inline script hash %s", hash)
		}
	}
	rec = get(prefix + "js/main.js")
	if rec.Code != 200 || rec.Body.String() != "export {}" || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("versioned asset: got %d %q %q", rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
	}
	if rec = get("/js/main.js"); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("unversioned asset: want 200 no-cache, got %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec = get("/v/000000000000/js/main.js"); rec.Code != 404 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("another build: want a no-store 404, got %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
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

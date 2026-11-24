package main

// The feed's page (plan 2026-11-24, slice 4): /feed and /feed/{id}, one document (www/feed.html) whose script reads
// /api/feed. Registered only while the feed is on (routeTable): off, /feed is the same 404 as any unknown path, the
// map has no entry to it and no CSP names core. Reading only: the place for a reply is there, the reply is slice 5's.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"time"
)

// coreOrigin is agent-fleet core's browser origin, which the feed page (and it alone) may connect to.
const coreOrigin = "https://fleet.bnei.dev"

// feedEntry is the map's way to the feed, one line of index.html, cut out of it while the feed is off.
const feedEntry = `<a id="feedlink" href="/feed">Feed</a>` + "\n"

// The page's routes: adminOnly sends an anonymous caller of one to the sign-in, not a bare 403.
const (
	feedRoute       = "GET /feed"
	feedThreadRoute = "GET /feed/{id}"
)

// feedPage serves the feed's document; a path that is not a thread id's shape is a 404.
func feedPage(www fs.FS, dev bool) http.Handler {
	html, tag := document(www, "feed.html")
	policy := csp(html, dev, coreOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !returnRE.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "private, no-cache")
		h.Set("ETag", tag)
		h.Set("Content-Security-Policy", policy)
		http.ServeContent(w, r, "feed.html", time.Time{}, bytes.NewReader(html))
	})
}

// document is one of www's pages as served: every `"./` in it is an asset URL (src, href, importmap), pointed at
// /v/<build>/; keep it that way.
func document(www fs.FS, name string) (html []byte, etag string) {
	html, _ = fs.ReadFile(www, name)
	html = bytes.ReplaceAll(html, []byte(`"./`), []byte(`"/v/`+buildID(www)+`/`))
	return html, htmlTag(html)
}

func htmlTag(html []byte) string {
	s := sha256.Sum256(html)
	return `"` + base64.RawURLEncoding.EncodeToString(s[:12]) + `"`
}

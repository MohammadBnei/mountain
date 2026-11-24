package main

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MohammadBnei/mountain"
)

// feedOn is a signed-in app whose feed is on, over the dev fixture.
func feedOn(t *testing.T) (*tapp, *http.Cookie) {
	t.Helper()
	a, p := withAuth(t)
	if err := a.loadFeedFixture("../../www/dev/fixtures/feed.json"); err != nil {
		t.Fatal(err)
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	a.h = routes(a.App, www)
	return a, adminCookie(t, a, p)
}

// TestFeedFixture: an event the feed would not store is refused.
func TestFeedFixture(t *testing.T) {
	a := testApp(t)
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"type":"dev.bnei.nope","id":"x"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.loadFeedFixture(bad); err == nil {
		t.Error("an event the feed does not read")
	}
}

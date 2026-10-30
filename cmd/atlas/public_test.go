package main

import (
	"context"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/internal/extract"
)

// TestPublicFlag: a checkout is public only when GitHub says private is false; the answer is asked on every checkout,
// the unchanged head's early return included; a 304 replays the last answer; a failed ask keeps the held value, and a
// repo GitHub never answered for is not public. A flip alone re-runs the extractors (the round cache's key holds it).
func TestPublicFlag(t *testing.T) {
	f := newFakeGitHub(t)
	f.head["mountain@main"], f.head["dark@main"] = strings.Repeat("a", 40), strings.Repeat("d", 40)
	f.meta["mountain"] = `{"private":true}`
	g, err := newGitHub(f.URL, "tok", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	check := func(step string, want bool) {
		t.Helper()
		if r, err := g.checkout(ctx, "mountain", "main"); err != nil || r.Public != want {
			t.Errorf("%s: public %v %v, want %v", step, r.Public, err, want)
		}
	}
	check("private", false)
	f.meta["mountain"] = `{"private":false}`
	check("made public, head unchanged", true)
	check("unchanged answer", true)
	if f.meta304 != 1 {
		t.Errorf("%d metadata 304s, want 1 (a conditional ask)", f.meta304)
	}
	f.head["mountain@main"] = strings.Repeat("b", 40)
	check("head moved", true)
	delete(f.meta, "mountain")
	check("metadata unreadable", true)
	if r, err := g.checkout(ctx, "dark", "main"); err != nil || r.Public {
		t.Errorf("never answered: public %v %v, want false", r.Public, err)
	}
	if vectorKey(extract.Repos{"x": {SHA: "a"}}) == vectorKey(extract.Repos{"x": {SHA: "a", Public: true}}) {
		t.Error("a visibility flip leaves the round cache's key unchanged: the repo node's public attr goes stale")
	}
}

package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCacheBudget: the trees on disk (held and retired) stay within cacheBudget. A tree that would go over it is
// refused before the pod is evicted: nothing of it stays on disk, a repo held already is read stale, and a retired
// tree counts until it is removed, except against its own repo's next tree. Each fake tree is one 40-byte file.
func TestCacheBudget(t *testing.T) {
	old := cacheBudget
	t.Cleanup(func() { cacheBudget = old })
	ctx := context.Background()
	sha := func(c string) string { return strings.Repeat(c, 40) }
	trees := func(g *ghHeads) int { es, _ := os.ReadDir(g.dir); return len(es) }

	cacheBudget = 100
	f := newFakeGitHub(t)
	f.head["a@main"], f.head["b@main"], f.head["c@main"] = sha("a"), sha("b"), sha("c")
	g, err := newGitHub(f.URL, "tok", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"a", "b"} {
		if _, err := g.checkout(ctx, r, "main"); err != nil {
			t.Fatalf("%s within the budget: %v", r, err)
		}
	}
	if _, err := g.checkout(ctx, "c", "main"); err == nil {
		t.Error("c: 120 bytes of trees in a budget of 100, taken")
	}
	if n := trees(g); n != 2 {
		t.Errorf("%d trees on disk, want 2 (the refused one removed)", n)
	}
	f.head["a@main"] = sha("d")
	r, err := g.checkout(ctx, "a", "main")
	if err == nil || r.SHA != sha("a") {
		t.Errorf("a moved past the budget: %s %v, want the held tree, stale, and an error", r.SHA, err)
	}
	if _, serr := os.Stat(r.Dir); serr != nil {
		t.Errorf("the held tree went: %v", serr)
	}

	// a retired tree still counts: 40 + 40 held, 40 retired, and 40 more is over 130
	cacheBudget = 130
	f2 := newFakeGitHub(t)
	f2.head["a@main"], f2.head["b@main"] = sha("a"), sha("b")
	g2, err := newGitHub(f2.URL, "tok", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"a", "b"} {
		if _, err := g2.checkout(ctx, r, "main"); err != nil {
			t.Fatalf("%s: %v", r, err)
		}
	}
	f2.head["a@main"] = sha("d")
	if r, err := g2.checkout(ctx, "a", "main"); err != nil || r.SHA != sha("d") {
		t.Fatalf("a moved within the budget: %s %v", r.SHA, err)
	}
	f2.head["b@main"] = sha("e")
	if r, err := g2.checkout(ctx, "b", "main"); err == nil || r.SHA != sha("b") {
		t.Errorf("b moved with a's retired tree on disk: %s %v, want refused (160 > 130)", r.SHA, err)
	}
	if n := trees(g2); n != 3 {
		t.Errorf("%d trees on disk, want 3 (a, its retired tree, b)", n)
	}

	// a's own retired tree does not keep a stale: the new tree replaces it (120 counted, 160 on disk for a moment)
	f2.head["a@main"] = sha("f")
	if r, err := g2.checkout(ctx, "a", "main"); err != nil || r.SHA != sha("f") {
		t.Errorf("a moved again with only its own retired tree in the way: %s %v", r.SHA, err)
	}
	if n := trees(g2); n != 3 {
		t.Errorf("%d trees on disk, want 3 (a, its retired tree, b)", n)
	}
}

package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCacheBudget: the trees on disk (held and retired) stay within cacheBudget. A tree that would go over it is
// refused before the pod is evicted: nothing of it stays on disk and a repo held already is read stale. Held trees
// that fill the budget refuse a tree before it is downloaded. A retired tree counts until the next round starts, and
// then goes, whether or not its repo moves again. Each fake tree is one small file: one block.
func TestCacheBudget(t *testing.T) {
	old := cacheBudget
	t.Cleanup(func() { cacheBudget = old })
	ctx := context.Background()
	sha := func(c string) string { return strings.Repeat(c, 40) }
	trees := func(g *ghHeads) int { es, _ := os.ReadDir(g.dir); return len(es) }

	cacheBudget = 2*blockSize + blockSize/2
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
		t.Error("c: 3 blocks of trees in a budget of 2.5, taken")
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

	// held trees fill the budget: the next tree is refused without downloading it
	cacheBudget = 2 * blockSize
	before := f.tarballs
	if _, err := g.checkout(ctx, "c", "main"); err == nil || f.tarballs != before {
		t.Errorf("c with the budget full: %v, %d tarballs downloaded, want an error and none", err, f.tarballs-before)
	}

	// a retired tree counts until the next round starts: a, its retired tree and b fill 3 blocks
	cacheBudget = 3 * blockSize
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
	g2.startRound()
	f2.head["a@main"] = sha("d")
	if r, err := g2.checkout(ctx, "a", "main"); err != nil || r.SHA != sha("d") {
		t.Fatalf("a moved within the budget: %s %v", r.SHA, err)
	}
	f2.head["b@main"] = sha("e")
	if r, err := g2.checkout(ctx, "b", "main"); err == nil || r.SHA != sha("b") {
		t.Errorf("b moved with a's retired tree on disk: %s %v, want refused (4 blocks > 3)", r.SHA, err)
	}
	if n := trees(g2); n != 3 {
		t.Errorf("%d trees on disk, want 3 (a, its retired tree, b)", n)
	}

	// the next round: a's retired tree goes although a stays quiet, so b fits
	g2.startRound()
	if n := trees(g2); n != 2 {
		t.Errorf("%d trees on disk at the round's start, want 2 (a quiet repo's retired tree kept)", n)
	}
	if r, err := g2.checkout(ctx, "b", "main"); err != nil || r.SHA != sha("e") {
		t.Errorf("b moved once the retired tree went: %s %v", r.SHA, err)
	}
}

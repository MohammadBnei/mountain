package main

import (
	"context"
	"errors"
	"testing"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
)

// storeCases are what both stores do the same, driven through Graphs (which reads its latest into each put): a new
// projection, the same again, a change, a repo lost or dropped. Where they differ is below each caller.
func storeCases(t *testing.T, g *Graphs) {
	t.Helper()
	ctx := context.Background()
	two := extract.Repos{"infra-bootstrap": {SHA: "s1"}, "agent-fleet": {SHA: "a"}}
	one := extract.Repos{"infra-bootstrap": {SHA: "s1"}}
	r1 := atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil)
	r2 := atlasResult([]graph.Node{n("app:a", "2", "s1")}, nil)
	for _, c := range []struct {
		name    string
		res     extract.Result
		repos   extract.Repos
		counted map[string]bool
		seq     int64
		changed bool
		err     error
	}{
		{"new", r1, two, nil, 1, true, nil},
		{"same", r1, two, nil, 1, false, nil},
		{"changed", r2, two, nil, 2, true, nil},
		{"same again", r2, two, nil, 2, false, nil},
		{"lost", r2, one, nil, -1, false, errLostRepos},                              // -1: the seq differs by store (below)
		{"dropped", r2, one, map[string]bool{"infra-bootstrap": true}, 3, true, nil}, // a new vector: a new projection
	} {
		before := g.Latest()
		seq, changed, err := g.put(ctx, c.res, c.repos, c.counted)
		if !changed && g.Latest() != before {
			t.Errorf("%s: no change, yet a new latest", c.name)
		}
		if !errors.Is(err, c.err) || changed != c.changed || (c.seq >= 0 && seq != c.seq) {
			t.Errorf("%s: %d %v %v, want %d %v %v", c.name, seq, changed, err, c.seq, c.changed, c.err)
		}
		if c.err == nil && (g.Latest() == nil || g.Latest().Seq != seq) {
			t.Errorf("%s: the latest is %+v, not seq %d", c.name, g.Latest(), seq)
		}
	}
}

// TestGraphStore: the memory store. It keeps no history, and its change test is content (a source sha alone is a
// new projection); a lost repo leaves the latest's seq.
func TestGraphStore(t *testing.T) {
	g := &Graphs{}
	storeCases(t, g)
	ctx := context.Background()
	_, hist := g.stores()
	if _, err := hist.load(ctx, 1); !errors.Is(err, errNoHistory) {
		t.Errorf("memory history: %v", err)
	}
	if seq, _, err := g.put(ctx, atlasResult([]graph.Node{n("app:a", "2", "s1")}, nil), extract.Repos{}, nil); seq != 3 || !errors.Is(err, errLostRepos) {
		t.Errorf("lost: %d %v, want the latest's seq 3", seq, err)
	}
	if seq, changed, _ := g.put(ctx, atlasResult([]graph.Node{n("app:a", "2", "s2")}, nil), extract.Repos{"infra-bootstrap": {SHA: "s1"}}, nil); seq != 4 || !changed {
		t.Errorf("a source sha alone: %d %v, want a new projection", seq, changed)
	}
}

// TestDBGraphStore: the Postgres store. It keeps every projection, and its change test is the versions (a source
// sha alone is no change); a lost repo returns no seq.
func TestDBGraphStore(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)
	g := openGraphs(ctx, dsn)
	defer g.close()
	if g.db() == nil {
		t.Fatal("no database")
	}
	storeCases(t, g)
	_, hist := g.stores()
	if v, err := hist.load(ctx, 1); err != nil || v.Seq != 1 {
		t.Errorf("history: %d %v", v.Seq, err)
	}
	if _, err := hist.load(ctx, 99); !errors.Is(err, errNoProjection) {
		t.Errorf("a missing projection: %v", err)
	}
	if seq, _, err := g.put(ctx, atlasResult([]graph.Node{n("app:a", "2", "s1")}, nil), extract.Repos{}, nil); seq != 0 || !errors.Is(err, errLostRepos) {
		t.Errorf("lost: %d %v, want 0", seq, err)
	}
	if seq, changed, _ := g.put(ctx, atlasResult([]graph.Node{n("app:a", "2", "s2")}, nil), extract.Repos{"infra-bootstrap": {SHA: "s1"}}, nil); seq != 3 || changed {
		t.Errorf("a source sha alone: %d %v, want no change", seq, changed)
	}
	// another writer (atlas project) stores behind this one's back: its next put of the same content is a change
	// to this Graphs, since its latest moves to the other's
	other := &Graphs{}
	if err := other.attach(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	defer other.close()
	r4 := atlasResult([]graph.Node{n("app:a", "4", "s1")}, nil)
	s4, _, err := other.put(ctx, r4, extract.Repos{"infra-bootstrap": {SHA: "s1"}}, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if seq, changed, err := g.put(ctx, r4, extract.Repos{"infra-bootstrap": {SHA: "s1"}}, nil); err != nil || seq != s4 || !changed || g.Latest().Seq != s4 {
		t.Errorf("after another writer: %d %v %v (latest %d), want %d, changed", seq, changed, err, g.Latest().Seq, s4)
	}
}

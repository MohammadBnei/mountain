package extract

import (
	"os"
	"testing"

	"github.com/MohammadBnei/mountain/internal/realms"
)

// TestRealRepos runs every extractor over real checkouts when ATLAS_REPOS names a directory holding them (locally:
// ATLAS_REPOS=/home/user). It fails on unassigned components: realms.yaml must place everything the repos hold.
func TestRealRepos(t *testing.T) {
	dir := os.Getenv("ATLAS_REPOS")
	if dir == "" {
		t.Skip("ATLAS_REPOS unset")
	}
	repos, err := Checkouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(repos, rl, All)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, n := range res.Graph.Nodes {
		kinds[n.Kind]++
	}
	edges := map[string]int{}
	for _, e := range res.Graph.Edges {
		edges[e.Kind]++
	}
	t.Logf("repos %d, nodes %v, edges %v, tracked %d, skipped %v", len(repos), kinds, edges, len(res.Tracked), res.Skipped)
	checkNoSecrets(t, res.Graph)
	for _, d := range res.Graph.Drift {
		t.Logf("drift %s %s: %s", d.Kind, d.Subject, d.Detail)
		if d.Kind == "unassigned" {
			t.Errorf("unassigned: %s (%s)", d.Subject, d.Source.Path)
		}
	}
}

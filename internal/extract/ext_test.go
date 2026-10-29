package extract

import (
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// TestExtNodes: each moon a needs row declares is one ext node, written by the watch pass from realms.yaml, with no
// realm (so neither placed nor unassigned), and its needs edges are declared, never drift.
func TestExtNodes(t *testing.T) {
	rl, err := realms.Parse([]byte(`realms: [infra, code]
functions: {infra/g: utility}
hosts: {a: h1}
pveNodeName: a
sources: [infra-bootstrap]
components:
  app:x: {realm: infra, group: g}
  app:y: {realm: infra, group: g}
needs:
  app:x: ["declared:ext:github"]
  app:y: ["declared:ext:github", "declared:ext:anthropic"]
`))
	if err != nil {
		t.Fatal(err)
	}
	repos := Repos{"infra-bootstrap": {Dir: t.TempDir(), SHA: "a"}}
	g := graph.Graph{Nodes: []graph.Node{{ID: "app:x", Kind: graph.KindApp}, {ID: "app:y", Kind: graph.KindApp}}}
	place(&g, rl)
	watchNeeds(repos, rl, &g)

	moons := 0
	for _, n := range g.Nodes {
		if n.Kind != graph.KindExt {
			continue
		}
		moons++
		if n.ID != "ext:github" && n.ID != "ext:anthropic" {
			t.Errorf("an undeclared moon: %s", n.ID)
		}
		if n.Realm != "" || n.Group != "" || n.Function != "" {
			t.Errorf("%s is placed: %+v", n.ID, n)
		}
		if n.Source.Repo != "mountain" || n.Source.Path != "realms/realms.yaml" {
			t.Errorf("%s source %+v, want realms.yaml's", n.ID, n.Source)
		}
	}
	if moons != 2 {
		t.Errorf("%d ext nodes, want 2 (one per moon, however many need it)", moons)
	}
	for _, e := range [][2]string{{"app:x", "ext:github"}, {"app:y", "ext:github"}, {"app:y", "ext:anthropic"}} {
		got, ok := edge(g, e[0], e[1], EdgeNeeds)
		if !ok || got.Origin != graph.Declared || got.Attrs["via"] != "declared" || got.Attrs["checked"] != "declared" {
			t.Errorf("needs %s > %s: %+v %v", e[0], e[1], got, ok)
		}
	}
	for _, d := range g.Drift {
		t.Errorf("drift: %s %s: %s", d.Kind, d.Subject, d.Detail)
	}
	if err := g.Check(); err != nil {
		t.Errorf("check: %v", err)
	}
}

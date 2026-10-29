package extract

import (
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
)

func TestImageAttr(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"helm/values.yaml":     "image:\n  repository: r\n  tag: abc1234\n",
		"helm/values-dev.yaml": "image:\n  tag: 0.20.0\n",
		"helm/none.yaml":       "replicas: 1\n",
		"chart/values.json":    `{"image":{"tag":"json"}}`,
	})
	repos := Repos{"wird": {Dir: dir, Branch: "main"}}
	git := func(app, branch, paths string) graph.Edge {
		return graph.Edge{From: app, To: "repo:wird", Kind: graph.EdgeGit, Attrs: map[string]string{"branch": branch, "paths": paths}}
	}
	g := graph.Graph{
		Nodes: []graph.Node{{ID: "app:layered", Kind: graph.KindApp, Attrs: map[string]string{"namespace": "n"}},
			{ID: "app:untagged", Kind: graph.KindApp}, {ID: "app:dev", Kind: graph.KindApp}, {ID: "app:json", Kind: graph.KindApp}},
		Edges: []graph.Edge{
			git("app:layered", "main", "helm/values.yaml,helm/values-dev.yaml,helm/none.yaml"),
			git("app:untagged", "main", "helm/none.yaml"),
			git("app:dev", "dev", "helm/values.yaml"),
			git("app:json", "main", "chart/values.json"),
		},
	}
	image(repos, &g)
	want := map[string]string{"app:layered": "0.20.0", "app:untagged": "", "app:dev": "", "app:json": ""}
	for _, n := range g.Nodes {
		if n.Attrs["image"] != want[n.ID] {
			t.Errorf("%s image %q, want %q", n.ID, n.Attrs["image"], want[n.ID])
		}
	}
	if g.Nodes[0].Attrs["namespace"] != "n" {
		t.Errorf("layered lost its other attrs: %v", g.Nodes[0].Attrs)
	}
}

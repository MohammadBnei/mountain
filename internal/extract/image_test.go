package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// TestImageAttr: an app's image is image.tag from the values files on its git edge, the last file that sets it winning
// (helm's layering). Read only from a checkout on the edge's branch, and only from .yaml/.yml paths (a path source
// names a chart directory); anything else, or no tag at all, is no attr.
func TestImageAttr(t *testing.T) {
	dir := t.TempDir()
	write := func(repo, path, body string) {
		p := filepath.Join(dir, repo, path)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("wird", "helm/values.yaml", "replicaCount: 1\nimage:\n  repository: registry.example/wird\n  tag: 3f2a9c1\n")
	write("wird", "helm/values-prod.yml", "image:\n  tag: \"0.20.0\"\n")
	write("wird", "helm/values-notag.yaml", "image:\n  repository: other\nresources: {}\n")
	write("wird", "helm/tag.txt", "image:\n  tag: from-a-text-file\n")
	write("wird", "helm/Chart.yaml", "name: wird\n")
	write("dev", "helm/values.yaml", "image:\n  tag: abc\n")

	repos := Repos{
		"wird": {Dir: filepath.Join(dir, "wird"), SHA: "w", Branch: "main"},
		"dev":  {Dir: filepath.Join(dir, "dev"), SHA: "d", Branch: "main"},
	}
	g := graph.Graph{}
	git := func(app, repo, branch, paths string) {
		g.Nodes = append(g.Nodes, graph.Node{ID: app, Kind: graph.KindApp, Attrs: map[string]string{"namespace": "x"}})
		e := graph.Edge{From: app, To: "repo:" + repo, Kind: graph.EdgeGit, Origin: graph.Declared, Attrs: map[string]string{"branch": branch}}
		if paths != "" {
			e.Attrs["paths"] = paths
		}
		g.Edges = append(g.Edges, e)
	}
	git("app:layered", "wird", "main", "helm/values.yaml,helm/values-prod.yml")
	git("app:reversed", "wird", "main", "helm/values-prod.yml,helm/values.yaml")
	git("app:later-unset", "wird", "main", "helm/values.yaml,helm/values-notag.yaml")
	git("app:notag", "wird", "main", "helm/values-notag.yaml")
	git("app:missing-file", "wird", "main", "helm/nope.yaml")
	git("app:chart-dir", "wird", "main", "helm")
	git("app:not-yaml", "wird", "main", "helm/tag.txt")
	git("app:no-paths", "wird", "main", "")
	git("app:other-branch", "dev", "develop", "helm/values.yaml") // dev is checked out on main
	git("app:not-held", "gone", "main", "helm/values.yaml")
	git("app:two-sources", "wird", "main", "helm/values-prod.yml") // and the common chart, a directory of another repo:
	g.Edges = append(g.Edges, graph.Edge{From: "app:two-sources", To: "repo:dev", Kind: graph.EdgeGit, Origin: graph.Declared, Attrs: map[string]string{"branch": "main", "paths": "helm"}})
	g.Nodes = append(g.Nodes, graph.Node{ID: "repo:wird", Kind: graph.KindRepo}, graph.Node{ID: "repo:dev", Kind: graph.KindRepo}, graph.Node{ID: "repo:gone", Kind: graph.KindRepo})

	image(repos, &g)

	want := map[string]string{"app:layered": "0.20.0", "app:reversed": "3f2a9c1", "app:later-unset": "3f2a9c1", "app:two-sources": "0.20.0"}
	for _, n := range g.Nodes {
		got, ok := n.Attrs["image"]
		if w, should := want[n.ID]; should && got != w {
			t.Errorf("%s image %q, want %q", n.ID, got, w)
		} else if !should && ok {
			t.Errorf("%s image %q, want no attr", n.ID, got)
		}
		if n.Kind == graph.KindApp && n.Attrs["namespace"] != "x" {
			t.Errorf("%s lost its other attrs: %v", n.ID, n.Attrs)
		}
	}
}

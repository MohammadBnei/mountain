package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// writeFiles lays files (repo-relative path -> content) under a new temp dir and returns it.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImports(t *testing.T) {
	fleet := writeFiles(t, map[string]string{
		"proto/go.mod": "module github.com/MohammadBnei/agent-fleet/proto\n",
		"core/go.mod": `module github.com/MohammadBnei/agent-fleet/core
require (
	github.com/MohammadBnei/agent-fleet/proto v0.0.0
	github.com/MohammadBnei/ukubi-go v0.1.0
	github.com/MohammadBnei/ukubi-kit v0.1.0 // indirect
	github.com/other/ukubi-go v1.0.0
)
`,
		"cli/go.mod":            "module github.com/MohammadBnei/agent-fleet/cli\nrequire github.com/MohammadBnei/Ukubi-Go/log v0.1.0\n",
		"vendor/x/go.mod":       "module x\nrequire github.com/MohammadBnei/hq v0.1.0\n",
		"node_modules/y/go.mod": "module y\nrequire github.com/MohammadBnei/hq v0.1.0\n",
	})
	rl, err := realms.Parse([]byte("realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap, agent-fleet]\n"))
	if err != nil {
		t.Fatal(err)
	}
	x := &Ctx{Repos: Repos{"agent-fleet": {Dir: fleet, SHA: "s1"}}, Realms: rl}
	if err := imports(x); err != nil {
		t.Fatal(err)
	}
	want := []graph.Edge{{From: "repo:agent-fleet", To: "repo:ukubi-go", Kind: graph.EdgeImports, Origin: graph.Declared,
		Attrs:  map[string]string{"modules": "github.com/MohammadBnei/Ukubi-Go/log,github.com/MohammadBnei/ukubi-go", "paths": "cli/go.mod,core/go.mod"},
		Source: graph.Source{Repo: "agent-fleet", Path: "cli/go.mod", SHA: "s1"}}}
	if graph.Hash(x.g.Edges) != graph.Hash(want) || len(x.g.Drift) != 0 {
		t.Fatalf("edges %+v drift %+v, want %+v", x.g.Edges, x.g.Drift, want)
	}
}

func TestImportsPlaceShared(t *testing.T) {
	g := graph.Graph{Edges: []graph.Edge{
		{From: "repo:agent-fleet", To: "repo:ukubi-go", Kind: graph.EdgeImports},
		{From: "repo:wird", To: "repo:ukubi-go", Kind: graph.EdgeImports},
		{From: "repo:hq", To: "repo:ukubi-kit", Kind: graph.EdgeImports},
	}}
	fed := map[string]map[string]bool{"repo:agent-fleet": {"fleet": true}, "repo:wird": {"apps": true}}
	feedImports(&g, fed)
	if len(fed["repo:ukubi-go"]) != 2 || !fed["repo:ukubi-go"]["fleet"] || !fed["repo:ukubi-go"]["apps"] {
		t.Fatalf("ukubi-go fed %v, want fleet and apps", fed["repo:ukubi-go"])
	}
	if fed["repo:ukubi-kit"] != nil {
		t.Fatalf("ukubi-kit fed %v by an importer that feeds nothing", fed["repo:ukubi-kit"])
	}
}

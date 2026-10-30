package extract

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// attrItems splits a joined attr ("a, b" or "a,b") into its items.
func attrItems(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// TestImports: every go.mod of a source (minus what SkipDir skips) gives one imports edge per pair of our repos, from
// both require forms; another owner's module, an indirect require and a repo's own modules give none, and a repo
// name no node id can carry (_lib) is drift, not a failed projection; several go.mods
// of one repo give one edge, sourced at the first of them; repoNodes makes the imported repos' nodes.
func TestImports(t *testing.T) {
	dir := t.TempDir()
	write := func(repo, path, body string) {
		p := filepath.Join(dir, repo, path)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("mountain", "go.mod", `module github.com/MohammadBnei/mountain

go 1.25

require (
	github.com/MohammadBnei/ukubi-go/errs v0.1.0
	github.com/MohammadBnei/ukubi-go v0.3.0
	github.com/other/lib v1.0.0
	golang.org/x/mod v0.20.0
	github.com/MohammadBnei/agent-fleet/proto v0.0.1 // indirect
)

require github.com/MohammadBnei/Ukubi-Kit v0.1.0

require github.com/MohammadBnei/_lib v0.1.0
`)
	write("agent-fleet", "dashboard/go.mod", "module github.com/MohammadBnei/agent-fleet/dashboard\n\nrequire github.com/MohammadBnei/ukubi-go/kit v0.2.0\n")
	write("agent-fleet", "core/go.mod", "module github.com/MohammadBnei/agent-fleet/core\n\nrequire (\n\tgithub.com/MohammadBnei/agent-fleet/proto v0.0.0\n\tgithub.com/MohammadBnei/ukubi-go v0.3.0\n)\n")
	write("agent-fleet", "proto/go.mod", "module github.com/MohammadBnei/agent-fleet/proto\n")
	for p, req := range map[string]string{"vendor/x/go.mod": "ukubi-a", "web/node_modules/go.mod": "ukubi-b", "core/testdata/go.mod": "ukubi-c", "gen/go.mod": "ukubi-d"} {
		write("agent-fleet", p, "module x\n\nrequire github.com/MohammadBnei/"+req+" v1.0.0\n")
	}

	rl, err := realms.Parse([]byte("realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap, agent-fleet, mountain]\nskip: {agent-fleet: [gen]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	repos := Repos{}
	for _, r := range []string{"infra-bootstrap", "agent-fleet", "mountain"} {
		_ = os.MkdirAll(filepath.Join(dir, r), 0o755)
		repos[r] = Repo{Dir: filepath.Join(dir, r), SHA: strings.Repeat(r[:1], 40), Branch: "main"}
	}
	res, err := Run(repos, rl, []Extractor{importsExtractor})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Graph
	if !hasDrift(g, graph.DriftUnresolved, "repo:mountain", "github.com/MohammadBnei/_lib") {
		t.Errorf("a require of a repo no id can name: no drift in %+v", g.Drift)
	}

	var got []string
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeImports {
			got = append(got, e.From+">"+e.To)
		}
	}
	slices.Sort(got)
	if want := []string{"repo:agent-fleet>repo:ukubi-go", "repo:mountain>repo:ukubi-go", "repo:mountain>repo:ukubi-kit"}; !slices.Equal(got, want) {
		t.Errorf("imports edges %v, want %v", got, want)
	}

	if e, ok := edge(g, "repo:mountain", "repo:ukubi-go", graph.EdgeImports); ok {
		if m := attrItems(e.Attrs["modules"]); !slices.Equal(m, []string{"github.com/MohammadBnei/ukubi-go", "github.com/MohammadBnei/ukubi-go/errs"}) {
			t.Errorf("mountain modules %q: both, sorted", e.Attrs["modules"])
		}
		if e.Origin != graph.Declared || e.Source.Repo != "mountain" || e.Source.Path != "go.mod" || e.Source.SHA != repos["mountain"].SHA {
			t.Errorf("mountain edge: %+v", e)
		}
	}
	if e, ok := edge(g, "repo:agent-fleet", "repo:ukubi-go", graph.EdgeImports); ok {
		if m := attrItems(e.Attrs["modules"]); !slices.Equal(m, []string{"github.com/MohammadBnei/ukubi-go", "github.com/MohammadBnei/ukubi-go/kit"}) {
			t.Errorf("agent-fleet modules %q: both go.mods', sorted", e.Attrs["modules"])
		}
		var dirs []string
		for _, p := range attrItems(e.Attrs["paths"]) {
			dirs = append(dirs, strings.TrimSuffix(strings.TrimSuffix(p, "go.mod"), "/"))
		}
		if !slices.Equal(dirs, []string{"core", "dashboard"}) {
			t.Errorf("agent-fleet paths %q: both go.mods, sorted", e.Attrs["paths"])
		}
		if e.Source.Repo != "agent-fleet" || e.Source.Path != "core/go.mod" {
			t.Errorf("agent-fleet source %+v, want its first go.mod", e.Source)
		}
	}

	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, id := range []string{"repo:ukubi-go", "repo:ukubi-kit"} {
		if !ids[id] {
			t.Errorf("no node %s", id)
		}
	}
	for _, id := range []string{"repo:lib", "repo:ukubi-a", "repo:ukubi-b", "repo:ukubi-c", "repo:ukubi-d"} {
		if ids[id] {
			t.Errorf("node %s: another owner's module or a skipped go.mod was read", id)
		}
	}
}

// TestImportsPlaceShared: a repo feeds the realms of the apps deployed from the repos that import it, so a library
// two realms' repos import is shared, one imported by a single realm's repos is that realm's, and one only an
// undeployed repo imports is its own.
func TestImportsPlaceShared(t *testing.T) {
	rl, err := realms.Parse([]byte(`realms: [ops, fleet, code]
functions: {ops/g: utility, fleet/g: barracks}
hosts: {a: h1}
pveNodeName: a
sources: [infra-bootstrap]
components:
  app:mountain: {realm: ops, group: g}
  app:fleet: {realm: fleet, group: g}
`))
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Graph{}
	for _, id := range []string{"app:mountain", "app:fleet", "repo:mountain", "repo:agent-fleet", "repo:ukubi-go", "repo:ukubi-kit", "repo:tool", "repo:lib"} {
		kind, _, _ := strings.Cut(id, ":")
		g.Nodes = append(g.Nodes, graph.Node{ID: id, Kind: kind})
	}
	for _, e := range [][3]string{
		{"app:mountain", "repo:mountain", graph.EdgeGit},
		{"app:fleet", "repo:agent-fleet", graph.EdgeGit},
		{"repo:mountain", "repo:ukubi-go", graph.EdgeImports},
		{"repo:agent-fleet", "repo:ukubi-go", graph.EdgeImports},
		{"repo:mountain", "repo:ukubi-kit", graph.EdgeImports},
		{"repo:tool", "repo:lib", graph.EdgeImports}, // no app deploys tool
	} {
		g.Edges = append(g.Edges, graph.Edge{From: e[0], To: e[1], Kind: e[2], Origin: graph.Declared})
	}
	place(&g, rl)
	for id, group := range map[string]string{
		"repo:mountain":    "ops",
		"repo:agent-fleet": "fleet",
		"repo:ukubi-go":    "shared",
		"repo:ukubi-kit":   "ops",
		"repo:lib":         "own",
		"repo:tool":        "own",
	} {
		if n := node(t, g, id); n.Realm != realms.Code || n.Group != group || n.Function != realms.FnWorkshop {
			t.Errorf("%s = %s/%s %s, want code/%s workshop", id, n.Realm, n.Group, n.Function, group)
		}
	}
}

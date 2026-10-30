package extract

import (
	"path"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// importsExtractor reads every go.mod of each source (minus what SkipDir skips) and adds one imports edge per pair
// of our repos, repo:A -> repo:B, when a go.mod in A requires a module of B directly. Several go.mods in one repo
// still give one edge (attrs modules and paths, sorted, comma-joined), so Merge never sees two versions of it.
// Its source is A's lexically first go.mod requiring B; a require of A's own modules is skipped, and so is
// "// indirect".
var importsExtractor = Extractor{Name: "imports", Version: 1, Run: imports}

// ourModules is the module path prefix of our repos; the segment after it is the repo.
const ourModules = "github.com/mohammadbnei/"

func imports(x *Ctx) error {
	if x.Realms == nil {
		return nil
	}
	for _, repo := range x.Realms.Sources {
		var mods []string
		_ = x.Walk(repo, func(dir string) bool { return SkipDir(x.Realms, repo, dir) }, func(p string, size int64) error {
			if path.Base(p) == "go.mod" && size <= maxReadFile {
				mods = append(mods, p)
			}
			return nil
		})
		if len(mods) == 0 {
			continue
		}
		slices.Sort(mods)                                                            // Walk's order is per directory level, not the string order the source is chosen by
		modules, paths := map[string]map[string]bool{}, map[string]map[string]bool{} // imported repo -> sets
		src := map[string]graph.Source{}                                             // imported repo -> the first go.mod (in string order) requiring it
		for _, p := range mods {
			b, s, err := x.Read(repo, p)
			if err != nil {
				continue
			}
			f, err := modfile.ParseLax(p, b, nil)
			if err != nil {
				x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, repo), Detail: p + ": not a go.mod atlas can read: " + err.Error(), Source: s})
				continue
			}
			for _, r := range f.Require {
				rest, ok := strings.CutPrefix(strings.ToLower(r.Mod.Path), ourModules)
				to, _, _ := strings.Cut(rest, "/")
				if !ok || r.Indirect || to == "" || to == strings.ToLower(repo) {
					continue
				}
				if !graph.Valid(graph.ID(graph.KindRepo, to)) { // a repo name no node id can carry: one require, not the projection
					x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, repo), Detail: p + ": requires " + r.Mod.Path + ": not a repo name atlas can name", Source: s})
					continue
				}
				if modules[to] == nil {
					modules[to], paths[to], src[to] = map[string]bool{}, map[string]bool{}, s
				}
				modules[to][r.Mod.Path], paths[to][p] = true, true
			}
		}
		for _, to := range sortedKeys(modules) {
			x.Edge(graph.Edge{From: graph.ID(graph.KindRepo, repo), To: graph.ID(graph.KindRepo, to), Kind: graph.EdgeImports, Origin: graph.Declared, Source: src[to],
				Attrs: map[string]string{"modules": strings.Join(sortedKeys(modules[to]), ","), "paths": strings.Join(sortedKeys(paths[to]), ",")}})
		}
	}
	return nil
}

// feedImports adds to fed (repo id -> realms of the apps it deploys, from git edges) the realms of each importer's
// apps, so placeRepos counts an imported repo as feeding them: a library two realms use becomes "shared".
// ponytail: one level only; a repo imported only by another library stays "own" until that matters.
func feedImports(g *graph.Graph, fed map[string]map[string]bool) {
	add := map[string][]string{}
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeImports {
			add[e.To] = append(add[e.To], sortedKeys(fed[e.From])...)
		}
	}
	for repo, rs := range add {
		if len(rs) > 0 && fed[repo] == nil {
			fed[repo] = map[string]bool{}
		}
		for _, r := range rs {
			fed[repo][r] = true
		}
	}
}

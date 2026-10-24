// Package extract turns repo checkouts into atlas's graph (plan/10-understanding.md "Projection"). Each extractor
// reads named fields only, never whole files, so nothing a repo holds besides structure reaches the graph.
package extract

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/internal/realms"
)

// Repo is one checkout: its root and the sha it is at.
type Repo struct {
	Dir string
	SHA string
}

// Repos is repo name (the GitHub name, "infra-bootstrap") -> checkout.
type Repos map[string]Repo

// Track is a (repo, branch) an ArgoCD Application deploys from: phase 1 of the sha vector.
type Track struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

// Result is one run's output.
type Result struct {
	Graph    graph.Graph
	Tracked  []Track
	Versions map[string]int // extractor -> version (a stored projection records them)
	Skipped  []string       // extractors whose repos were missing
}

// Extractor reads one family of files. Version changes whenever its output for the same input would.
type Extractor struct {
	Name    string
	Version int
	Needs   []string // repos it reads; a run without them skips it
	Run     func(x *Ctx) error
}

// Ctx is what an extractor writes into.
type Ctx struct {
	Repos  Repos
	Realms *realms.Realms
	g      graph.Graph
	track  map[Track]bool
}

func (x *Ctx) Node(n graph.Node)   { x.g.Nodes = append(x.g.Nodes, n) }
func (x *Ctx) Edge(e graph.Edge)   { x.g.Edges = append(x.g.Edges, e) }
func (x *Ctx) Drift(d graph.Drift) { x.g.Drift = append(x.g.Drift, d) }
func (x *Ctx) Track(t Track)       { x.track[t] = true }

// Read returns a repo file and where it came from.
func (x *Ctx) Read(repo, path string) ([]byte, graph.Source, error) {
	r, ok := x.Repos[repo]
	if !ok {
		return nil, graph.Source{}, fmt.Errorf("extract: repo %s not checked out", repo)
	}
	src := graph.Source{Repo: repo, Path: path, SHA: r.SHA}
	b, err := os.ReadFile(filepath.Join(r.Dir, filepath.FromSlash(path)))
	return b, src, err
}

// Glob lists repo-relative paths matching pattern (filepath.Match syntax, one directory level).
func (x *Ctx) Glob(repo, pattern string) []string {
	r, ok := x.Repos[repo]
	if !ok {
		return nil
	}
	m, _ := filepath.Glob(filepath.Join(r.Dir, filepath.FromSlash(pattern)))
	out := make([]string, 0, len(m))
	for _, p := range m {
		rel, _ := filepath.Rel(r.Dir, p)
		out = append(out, filepath.ToSlash(rel))
	}
	slices.Sort(out)
	return out
}

// Walk calls fn for every regular file under repo, repo-relative, skipping .git and the directories skip names.
func (x *Ctx) Walk(repo string, skip func(dir string) bool, fn func(path string, size int64) error) error {
	r, ok := x.Repos[repo]
	if !ok {
		return nil
	}
	return filepath.WalkDir(r.Dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(r.Dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (d.Name() == ".git" || skip(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // a symlink could point outside the checkout
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(rel, info.Size())
	})
}

// All is every extractor, in run order.
var All = []Extractor{gitopsExtractor, machinesExtractor, adrExtractor}

// Run runs the extractors over repos, places every component in its realm and checks the result.
func Run(repos Repos, rl *realms.Realms, exs []Extractor) (Result, error) {
	res := Result{Versions: map[string]int{}}
	track := map[Track]bool{}
	for _, ex := range exs {
		if missing := slices.DeleteFunc(slices.Clone(ex.Needs), func(r string) bool { _, ok := repos[r]; return ok }); len(missing) > 0 {
			res.Skipped = append(res.Skipped, ex.Name+" (no "+strings.Join(missing, ", ")+")")
			continue
		}
		x := &Ctx{Repos: repos, Realms: rl, track: track}
		if err := ex.Run(x); err != nil {
			return res, fmt.Errorf("extract %s: %w", ex.Name, err)
		}
		if err := res.Graph.Merge(x.g); err != nil {
			return res, fmt.Errorf("extract %s: %w", ex.Name, err)
		}
		res.Versions[ex.Name] = ex.Version
	}
	var x Ctx
	declare(&x, repos, rl, res.Graph)
	repoNodes(&x, repos, res.Graph)
	if err := res.Graph.Merge(x.g); err != nil {
		return res, err
	}
	rehome(&res.Graph)
	place(&res.Graph, rl)
	for t := range track {
		res.Tracked = append(res.Tracked, t)
	}
	slices.SortFunc(res.Tracked, func(a, b Track) int { return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.Branch, b.Branch)) })
	return res, res.Graph.Check()
}

// realmKinds are the kinds realms.yaml must place: components, not documents (repos, hostnames, ADRs).
var realmKinds = map[string]bool{graph.KindHost: true, graph.KindVM: true, graph.KindCT: true, graph.KindApp: true, graph.KindSvc: true}

// declare adds the svc components realms.yaml lists for a checked-out repo: the parts its ADR citations attach
// to. A6's proto and definition extractors will confirm them from code.
func declare(x *Ctx, repos Repos, rl *realms.Realms, g graph.Graph) {
	have := map[string]bool{}
	for _, n := range g.Nodes {
		have[n.ID] = true
	}
	for _, id := range sortedKeys(rl.Components) {
		kind, name, _ := strings.Cut(id, ":")
		if kind != graph.KindSvc || have[id] {
			continue
		}
		repo, _, _ := strings.Cut(name, "/")
		if _, ok := repos[repo]; !ok {
			continue
		}
		x.Node(graph.Node{ID: id, Kind: kind, Attrs: map[string]string{"repo": repo},
			Source: graph.Source{Repo: "mountain", Path: "internal/realms/realms.yaml", SHA: repos["mountain"].SHA},
		})
	}
}

// repoNodes adds a node for every checked-out repo and every repo an edge names. Extractors name repos only in
// edges, so two of them never write the same node.
func repoNodes(x *Ctx, repos Repos, g graph.Graph) {
	names := map[string]bool{}
	for r := range repos {
		names[r] = true
	}
	for _, e := range g.Edges {
		for _, id := range []string{e.From, e.To} {
			if k, name, _ := strings.Cut(id, ":"); k == graph.KindRepo {
				names[name] = true
			}
		}
	}
	for _, r := range sortedKeys(names) {
		x.Node(graph.Node{ID: graph.ID(graph.KindRepo, r), Kind: graph.KindRepo,
			Attrs:  map[string]string{"url": "https://github.com/MohammadBnei/" + r},
			Source: graph.Source{Repo: r, SHA: repos[r].SHA}})
	}
}

// rehome moves a governs edge whose component no extractor produced (its Application isn't in this run) onto the
// citing repo, summing counts, so a citation is never lost and never points at a node that isn't there.
func rehome(g *graph.Graph) {
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	byKey := map[string]int{}
	var out []graph.Edge
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeGoverns && !ids[e.From] {
			e.From = graph.ID(graph.KindRepo, e.Source.Repo)
		}
		if i, ok := byKey[e.Key()]; ok && e.Kind == graph.EdgeGoverns {
			a, _ := strconv.Atoi(out[i].Attrs["count"])
			b, _ := strconv.Atoi(e.Attrs["count"])
			out[i].Attrs = map[string]string{"count": strconv.Itoa(a + b)}
			continue
		}
		byKey[e.Key()] = len(out)
		out = append(out, e)
	}
	g.Edges = out
	slices.SortFunc(g.Edges, func(a, b graph.Edge) int { return cmp.Compare(a.Key(), b.Key()) })
}

// place sets each component's realm, group and parent from realms.yaml, and records the ones it doesn't list.
func place(g *graph.Graph, rl *realms.Realms) {
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	var drift []graph.Drift
	for i, n := range g.Nodes {
		if !realmKinds[n.Kind] {
			continue
		}
		c, ok := rl.Components[n.ID]
		if !ok {
			drift = append(drift, graph.Drift{Kind: graph.DriftUnassigned, Subject: n.ID, Detail: "not in realms.yaml", Source: n.Source})
			continue
		}
		g.Nodes[i].Realm, g.Nodes[i].Group = c.Realm, c.Group
		if c.Parent != "" {
			if ids[c.Parent] {
				g.Nodes[i].Parent = c.Parent
			} else {
				drift = append(drift, graph.Drift{Kind: graph.DriftUnresolved, Subject: n.ID, Detail: "parent " + c.Parent + " is not a node", Source: n.Source})
			}
		}
	}
	g.Merge(graph.Graph{Drift: drift}) // drift only: cannot fail
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

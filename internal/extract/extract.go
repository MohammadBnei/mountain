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
	"github.com/MohammadBnei/mountain/realms"
)

// Repo is one checkout: its root, the sha it is at and the branch HEAD names ("" when detached).
type Repo struct {
	Dir    string
	SHA    string
	Branch string
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
	Versions map[string]int // extractor -> version, "pass:<name>" -> version, "realms" -> its digest (a stored projection records them)
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
	Repos   Repos
	Realms  *realms.Realms
	g       graph.Graph
	track   map[Track]bool
	members []graph.Edge
}

func (x *Ctx) Node(n graph.Node)   { x.g.Nodes = append(x.g.Nodes, n) }
func (x *Ctx) Edge(e graph.Edge)   { x.g.Edges = append(x.g.Edges, e) }
func (x *Ctx) Drift(d graph.Drift) { x.g.Drift = append(x.g.Drift, d) }
func (x *Ctx) Track(t Track)       { x.track[t] = true }

// Member adds an edge to a node another extractor makes (a Pigsty cluster's VM, which terraform declares). It is kept
// only if both ends are nodes once every extractor ran; else it is unresolved drift, never a dangling edge.
func (x *Ctx) Member(e graph.Edge) { x.members = append(x.members, e) }

// Read returns a repo file and where it came from. It opens the file through an os.Root on the checkout, so neither
// "..", an absolute path nor a symlink can reach outside it (a cloned repo is not trusted to point at the
// projector's own files), and it reads regular files only.
func (x *Ctx) Read(repo, path string) ([]byte, graph.Source, error) {
	r, ok := x.Repos[repo]
	if !ok {
		return nil, graph.Source{}, fmt.Errorf("extract: repo %s not checked out", repo)
	}
	src := graph.Source{Repo: repo, Path: path, SHA: r.SHA}
	root, err := os.OpenRoot(r.Dir)
	if err != nil {
		return nil, src, err
	}
	defer func() { _ = root.Close() }()
	fi, err := root.Lstat(filepath.FromSlash(path))
	if err != nil {
		return nil, src, err
	}
	if !fi.Mode().IsRegular() {
		return nil, src, fmt.Errorf("extract: %s/%s is not a regular file", repo, path)
	}
	b, err := root.ReadFile(filepath.FromSlash(path))
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
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			continue // a directory, or a symlink that could lead out of the checkout
		}
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
var All = []Extractor{gitopsExtractor, machinesExtractor, kubesprayExtractor, pigstyExtractor, adrExtractor, importsExtractor}

// Run runs the extractors over repos, places every component in its realm and checks the result.
func Run(repos Repos, rl *realms.Realms, exs []Extractor) (Result, error) {
	res := Result{Versions: map[string]int{}}
	track := map[Track]bool{}
	var members []graph.Edge
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
		members = append(members, x.members...)
		res.Versions[ex.Name] = ex.Version
	}
	for _, p := range passes() {
		x := &Ctx{Repos: repos, Realms: rl, track: track, members: members}
		if err := p.run(x, &res.Graph); err != nil {
			return res, fmt.Errorf("pass %s: %w", p.name, err)
		}
		if err := res.Graph.Merge(x.g); err != nil {
			return res, fmt.Errorf("pass %s: %w", p.name, err)
		}
		res.Versions["pass:"+p.name] = p.version
	}
	res.Versions["realms"] = rl.Digest
	for t := range track {
		res.Tracked = append(res.Tracked, t)
	}
	slices.SortFunc(res.Tracked, func(a, b Track) int { return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.Branch, b.Branch)) })
	return res, res.Graph.Check()
}

// pass is one step over the merged graph, after the extractors. Its version is recorded in Result.Versions as
// "pass:<name>", so a stored projection names the logic that made it: bump it when the pass's output changes. It
// adds through x (merged after it runs) or changes g in place; x carries the run's sources, realms.yaml, the
// extractors' tracked branches and their Member edges.
type pass struct {
	name    string
	version int
	run     func(x *Ctx, g *graph.Graph) error
}

// passes are run in this order, each on what the ones before it made:
//   - declare and repoNodes add the svc components realms.yaml lists and the repo nodes;
//   - resolve keeps the Member edges whose ends are both nodes, so it needs every node;
//   - rehome moves governs edges onto a node that exists, before place reads the edges;
//   - place gives every component its realm and group (and the repos theirs, from what they feed);
//   - watch adds the watch, needs and walls rows and the model facts (watch.go and walls.go, which share its files);
//   - image writes each app's deployed image tag from its values files (image.go);
//   - branchDrift reports a checkout on a branch ArgoCD does not deploy.
func passes() []pass {
	return []pass{
		{"declare", 1, func(x *Ctx, g *graph.Graph) error { declare(x, x.Repos, x.Realms, *g); return nil }},
		{"repoNodes", 1, func(x *Ctx, g *graph.Graph) error { repoNodes(x, x.Repos, *g); return nil }},
		{"resolve", 1, func(x *Ctx, g *graph.Graph) error { return g.Merge(resolve(*g, x.members)) }},
		{"rehome", 1, func(_ *Ctx, g *graph.Graph) error { rehome(g); return nil }},
		{"place", 2, func(x *Ctx, g *graph.Graph) error { place(g, x.Realms); return nil }},
		{"watch", 3, func(x *Ctx, g *graph.Graph) error { watchNeeds(x.Repos, x.Realms, g); return nil }},
		{"image", 1, func(x *Ctx, g *graph.Graph) error { image(x.Repos, g); return nil }},
		{"branchDrift", 1, func(x *Ctx, g *graph.Graph) error { return g.Merge(graph.Graph{Drift: branchDrift(x.Repos, x.track)}) }},
	}
}

// branchDrift reports a checkout that is not on a branch ArgoCD deploys from: atlas would be projecting work in
// progress as the system.
func branchDrift(repos Repos, track map[Track]bool) []graph.Drift {
	want := map[string][]string{}
	for t := range track {
		want[t.Repo] = append(want[t.Repo], t.Branch)
	}
	var out []graph.Drift
	for _, name := range sortedKeys(repos) {
		r, branches := repos[name], want[name]
		if len(branches) == 0 || slices.Contains(branches, r.Branch) {
			continue
		}
		slices.Sort(branches)
		on := "a detached HEAD"
		if r.Branch != "" {
			on = "branch " + r.Branch
		}
		out = append(out, graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, name),
			Detail: "checked out on " + on + "; ArgoCD deploys " + strings.Join(branches, ", "), Source: graph.Source{Repo: name, SHA: r.SHA}})
	}
	return out
}

// realmKinds are the kinds realms.yaml must place: components, not documents (repos, hostnames, ADRs).
var realmKinds = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range realms.ComponentKinds {
		m[k] = true
	}
	return m
}()

// machineKinds are what a Member edge's machine may turn out to be: a name in the LAN records is a VM, a container
// or a host, and only terraform knows which.
var machineKinds = []string{graph.KindVM, graph.KindCT, graph.KindHost}

// resolve keeps the Member edges whose ends are both nodes (a machine end matched by name across machineKinds) and
// turns the rest into unresolved drift.
func resolve(g graph.Graph, members []graph.Edge) graph.Graph {
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	var out graph.Graph
	for _, e := range members {
		if kind, name, _ := strings.Cut(e.To, ":"); !ids[e.To] && slices.Contains(machineKinds, kind) {
			for _, k := range machineKinds {
				if ids[graph.ID(k, name)] {
					e.To = graph.ID(k, name)
					break
				}
			}
		}
		if ids[e.From] && ids[e.To] {
			out.Edges = append(out.Edges, e)
			continue
		}
		missing := e.To
		if !ids[e.From] {
			missing = e.From
		}
		out.Drift = append(out.Drift, graph.Drift{Kind: graph.DriftUnresolved, Subject: e.From, Detail: e.Kind + " " + missing + ": not a node", Source: e.Source})
	}
	return out
}

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
			Source: graph.Source{Repo: "mountain", Path: "realms/realms.yaml", SHA: repos["mountain"].SHA},
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

// place sets each component's realm, group, function and parent from realms.yaml, and records the ones it doesn't
// list. Repos go to the code realm (placeRepos).
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
		g.Nodes[i].Realm, g.Nodes[i].Group, g.Nodes[i].Function = c.Realm, c.Group, rl.Function(n.ID)
		if c.Parent != "" {
			if ids[c.Parent] {
				g.Nodes[i].Parent = c.Parent
			} else {
				drift = append(drift, graph.Drift{Kind: graph.DriftUnresolved, Subject: n.ID, Detail: "parent " + c.Parent + " is not a node", Source: n.Source})
			}
		}
	}
	_ = g.Merge(graph.Graph{Drift: drift}) // drift only: cannot fail
	placeRepos(g)
}

// placeRepos puts every repo in the code realm as a workshop (plan/13). Its group is the realm of the placed apps
// that deploy from it (git edges) when they share one, "shared" when they span several, and "own" when no placed
// Application does (a checkout no Application deploys, or one whose apps realms.yaml doesn't list: those are
// unassigned drift already). It runs after the components are placed, so the apps' realms are known. Past the load
// rule's 7 groups, the smallest realm groups fold into "shared", so code never needs a split realms.yaml can't make.
// A repo changes group when a second realm starts deploying from it: an accepted exception to "a building never
// moves" (plan/13 §2), since the group follows a real edge.
func placeRepos(g *graph.Graph) {
	realmOf := map[string]string{}
	for _, n := range g.Nodes {
		realmOf[n.ID] = n.Realm
	}
	fed := map[string]map[string]bool{} // repo -> realms of the apps it feeds
	for _, e := range g.Edges {
		if e.Kind != graph.EdgeGit || realmOf[e.From] == "" {
			continue
		}
		if fed[e.To] == nil {
			fed[e.To] = map[string]bool{}
		}
		fed[e.To][realmOf[e.From]] = true
	}
	feedImports(g, fed)
	for i, n := range g.Nodes {
		if n.Kind != graph.KindRepo {
			continue
		}
		group := "own"
		switch len(fed[n.ID]) {
		case 0:
		case 1:
			for r := range fed[n.ID] {
				group = r
			}
		default:
			group = "shared"
		}
		g.Nodes[i].Realm, g.Nodes[i].Group, g.Nodes[i].Function = realms.Code, group, realms.FnWorkshop
	}
	foldCode(g)
}

// maxGroups is plan/10's load rule: at most 7 groups in a view.
const maxGroups = 7

// foldCode folds code's smallest realm groups (ties: the later name) into "shared" until it has at most maxGroups.
func foldCode(g *graph.Graph) {
	size := map[string]int{}
	for _, n := range g.Nodes {
		if n.Realm == realms.Code {
			size[n.Group]++
		}
	}
	fold := map[string]bool{}
	for len(size) > maxGroups {
		if _, ok := size["shared"]; !ok {
			size["shared"] = 0 // folding creates it: one more group to make room for
		}
		small := ""
		for _, k := range sortedKeys(size) {
			if k != "shared" && k != "own" && (small == "" || size[k] < size[small] || size[k] == size[small] && k > small) {
				small = k
			}
		}
		size["shared"] += size[small]
		delete(size, small)
		fold[small] = true
	}
	for i, n := range g.Nodes {
		if n.Realm == realms.Code && fold[n.Group] {
			g.Nodes[i].Group = "shared"
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Package graph is atlas's model of the system (plan/10-understanding.md): nodes, edges and drift, each tied to the
// file and sha it came from. It is pure: extractors build it, the relay stores and serves it.
package graph

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Kinds of node. An id is "<kind>:<qualified-name>", stable across shas (plan/10 "Nodes and edges").
const (
	KindHost  = "host"  // a Proxmox host (host:bnei)
	KindVM    = "vm"    // a VM, Talos nodes included (vm:esprimo-1)
	KindCT    = "ct"    // an LXC container (ct:build-runner)
	KindApp   = "app"   // an ArgoCD Application (app:wird)
	KindSvc   = "svc"   // a component inside a repo (svc:agent-fleet/core)
	KindRepo  = "repo"  // a git repository (repo:infra-bootstrap)
	KindAddon = "addon" // cluster software no Application deploys (addon:cilium); no producer since the kubespray extractor was retired
	KindDB    = "db"    // a data service outside the cluster, by its cluster name (db:pg-proxmox)
	KindDNS   = "dns"   // a public hostname (dns:wird.bnei.dev)
	KindADR   = "adr"   // a decision record (adr:infra-bootstrap/0064)
	KindExt   = "ext"   // an outside service an app declares it needs, a moon (ext:github)
	// The door's kinds (design-2026-11/door-hybrid-F.md): realms/aliases.yaml names them, no extractor makes them.
	KindPG       = "pg"       // a Patroni member of a Postgres cluster, by its member name (pg:pg-proxmox-1)
	KindEndpoint = "endpoint" // an address shared or redirected among things: a VIP, a redirect Service (endpoint:pg-proxmox/vip)
	// The cascade's kind (realms/system.go): what the OS lays down on its nodes, no Application (platform:talos).
	KindPlatform = "platform"
)

// Kinds is each node kind's meaning, one line each: the table Valid's id rule is built from, and what a reader that
// maps a kind to something else (plan/25's rooms) reads instead of a switch.
var Kinds = map[string]string{
	KindHost:     "a Proxmox host",
	KindVM:       "a VM, Talos nodes included",
	KindCT:       "an LXC container",
	KindApp:      "an ArgoCD Application",
	KindSvc:      "a component inside a repo",
	KindRepo:     "a git repository",
	KindAddon:    "cluster software no Application deploys; no producer since the kubespray extractor was retired",
	KindDB:       "a data service outside the cluster, by its cluster name",
	KindDNS:      "a public hostname",
	KindADR:      "a decision record",
	KindExt:      "an outside service an app declares it needs, a moon",
	KindPG:       "a Patroni member of a Postgres cluster, by its member name",
	KindEndpoint: "an address shared or redirected among things: a VIP, a redirect Service",
	KindPlatform: "what the OS lays down on its nodes, no Application",
}

// Kinds of edge.
const (
	EdgeRunsOn    = "runs-on"    // vm|ct -> host, db -> vm (attrs: as)
	EdgeGit       = "git"        // app -> repo it deploys from (attrs: branch, paths)
	EdgeRoute     = "route"      // dns -> app
	EdgeGoverns   = "governs"    // component|repo -> adr (attrs: count)
	EdgeRelated   = "related"    // adr -> adr (header lines)
	EdgeAmends    = "amends"     // adr -> adr
	EdgeSupersede = "supersedes" // adr -> adr
	EdgeImports   = "imports"    // repo -> repo whose Go module it requires (attrs: modules, paths)
)

// Origins of an edge: what the repos say, or what traffic showed.
const (
	Declared = "declared"
	Observed = "observed"
)

// Kinds of drift.
const (
	DriftUnassigned = "unassigned" // a component realms.yaml doesn't place
	DriftMirror     = "mirror"     // two hand-mirrored files disagree
	DriftAmbiguous  = "ambiguous"  // a citation atlas can't resolve to one ADR
	DriftUnresolved = "unresolved" // a reference to something that isn't there
)

// Source is where a fact was read: a repo-relative path at a sha.
type Source struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
	SHA  string `json:"sha,omitempty"`
}

type Node struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Realm  string `json:"realm,omitempty"`
	Group  string `json:"group,omitempty"`
	Parent string `json:"parent,omitempty"`
	// Function is what the node does in play (realms.yaml functions, plan/13); set on placed nodes only.
	Function string            `json:"function,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Source   Source            `json:"source"`
}

type Edge struct {
	From   string            `json:"from"`
	To     string            `json:"to"`
	Kind   string            `json:"kind"`
	Origin string            `json:"origin"`
	Attrs  map[string]string `json:"attrs,omitempty"`
	Source Source            `json:"source"`
}

type Drift struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
	Source  Source `json:"source"`
}

// Graph is one projection's content. Merge keeps it sorted, so two runs over the same input are byte-identical.
type Graph struct {
	Nodes []Node  `json:"nodes"`
	Edges []Edge  `json:"edges"`
	Drift []Drift `json:"drift"`
}

var idRe = regexp.MustCompile(`^(` + strings.Join(slices.Sorted(maps.Keys(Kinds)), "|") + `):[a-z0-9][a-z0-9._/@-]*$`)

// ID builds "<kind>:<name>", lower-cased.
func ID(kind, name string) string { return kind + ":" + strings.ToLower(name) }

// Valid reports whether id has the one form.
func Valid(id string) bool { return idRe.MatchString(id) }

// Key is an edge's identity: one edge per (from, to, kind, origin). A space never occurs in an id, a kind or an
// origin, and unlike NUL it fits a Postgres text column (atlas_edge_ver.key).
func (e Edge) Key() string { return e.From + " " + e.To + " " + e.Kind + " " + e.Origin }

// Key is a drift item's identity.
func (d Drift) Key() string { return d.Kind + "\x00" + d.Subject + "\x00" + d.Detail }

// Hash is the sha256 of v's JSON (map keys are sorted by encoding/json): a node or edge version's identity.
func Hash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only plain structs and string maps reach here
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Merge adds o into g. A node or edge seen twice must be the same, or Merge fails: two extractors disagreeing about
// one fact is a bug, not something to pick between.
func (g *Graph) Merge(o Graph) error {
	nodes := map[string]Node{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range o.Nodes {
		if !Valid(n.ID) {
			return fmt.Errorf("graph: bad node id %q", n.ID)
		}
		if p, ok := nodes[n.ID]; ok && Hash(p) != Hash(n) {
			return fmt.Errorf("graph: node %s read twice, differently (%s, %s)", n.ID, p.Source.Path, n.Source.Path)
		}
		nodes[n.ID] = n
	}
	edges := map[string]Edge{}
	for _, e := range g.Edges {
		edges[e.Key()] = e
	}
	for _, e := range o.Edges {
		if !Valid(e.From) || !Valid(e.To) {
			return fmt.Errorf("graph: bad edge %s -> %s", e.From, e.To)
		}
		if p, ok := edges[e.Key()]; ok {
			// Two files stating the same edge agree ("Supersedes" here, "Superseded by" there): keep one, by the
			// earlier source so a rerun picks the same. Only different content is a conflict.
			if Hash(withoutSource(p)) != Hash(withoutSource(e)) {
				return fmt.Errorf("graph: edge %s -> %s (%s) read twice, differently (%s, %s)", e.From, e.To, e.Kind, p.Source.Path, e.Source.Path)
			}
			if cmp.Or(cmp.Compare(p.Source.Repo, e.Source.Repo), cmp.Compare(p.Source.Path, e.Source.Path)) <= 0 {
				continue
			}
		}
		edges[e.Key()] = e
	}
	drift := map[string]Drift{}
	for _, d := range append(g.Drift, o.Drift...) {
		drift[d.Key()] = d
	}
	g.Nodes = sortedValues(nodes, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) })
	g.Edges = sortedValues(edges, func(a, b Edge) int { return cmp.Compare(a.Key(), b.Key()) })
	g.Drift = sortedValues(drift, func(a, b Drift) int { return cmp.Compare(a.Key(), b.Key()) })
	return nil
}

func withoutSource(e Edge) Edge { e.Source = Source{}; return e }

// Check reports edges whose ends are not nodes and nodes whose parent is not one.
func (g *Graph) Check() error {
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, n := range g.Nodes {
		if n.Parent != "" && !ids[n.Parent] {
			return fmt.Errorf("graph: %s has parent %s, which is not a node", n.ID, n.Parent)
		}
	}
	for _, e := range g.Edges {
		if !ids[e.From] || !ids[e.To] {
			return fmt.Errorf("graph: edge %s -> %s (%s) has an end that is not a node", e.From, e.To, e.Kind)
		}
	}
	return nil
}

func sortedValues[T any](m map[string]T, f func(a, b T) int) []T {
	out := make([]T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	slices.SortFunc(out, f)
	return out
}

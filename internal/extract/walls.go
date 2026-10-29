package extract

import (
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// Walls (plan/13 §3 Defence, owner 2026-10-26): a gate sits on a door, a hostname that routes to an app, so walls are
// attributes of the dns nodes, summed up on their app. Read from the sources where they show it:
//
//   - wall forwardauth: a values file's ingress.hostname whose ingress.middlewares name an authentik forwardAuth
//     middleware; an IngressRoute route on that host through one; a forward_single proxy provider for the host
//     (gitops/bootstrap/authentik-blueprint-forwardauth.yaml);
//   - gate forwardauth-path: an IngressRoute route through one that also matches a path (a part of the host);
//   - gate oidc: an OIDC blueprint's redirect URL on the host (its login goes through authentik).
//
// realms.yaml walls: rows add what the sources can't show: the marks public (meant for everyone) and own (the app
// authenticates its own callers), and gates declared because they live in a repo atlas does not read. A door with
// none of them is open (a finding shown on its card, not drift). Only names are read: hosts, middleware names, the
// provider's mode and host.
//
// Mesh: a machine named in the NetBird inventory (ansible/inventories/mesh/hosts.yml, its host keys only, never an
// address or a key path) is a mesh peer. Not a wall yet: until ADR-0055's cutover its sshd still answers on the LAN.

const meshInventory = "ansible/inventories/mesh/hosts.yml"

var (
	routeHost   = regexp.MustCompile("Host\\(`([a-z0-9.-]+)`\\)")
	routePath   = regexp.MustCompile(`\bPath(Prefix)?\(`)
	providerSep = regexp.MustCompile(`(?m)^\s*- model:\s*`)
	singleMode  = regexp.MustCompile(`(?m)^\s*mode:\s*forward_single\s*$`)
	extHost     = regexp.MustCompile(`(?m)^\s*external_host:\s*["']?https://([a-z0-9.-]+)`)
)

// walls writes the doors' and apps' wall attributes and the mesh peers, and returns the drift. fa is the authentik
// forwardAuth middlewares' names.
func walls(x *Ctx, rl *realms.Realms, files []sourceFile, fa []string, g *graph.Graph) []graph.Drift {
	var drift []graph.Drift
	d := func(subject, detail string, src graph.Source) {
		drift = append(drift, graph.Drift{Kind: graph.DriftUnresolved, Subject: subject, Detail: detail, Source: src})
	}
	realmsSrc := graph.Source{Repo: "mountain", Path: "realms/realms.yaml"}
	doors := map[string][]string{} // dns id -> the apps it routes to
	apps := map[string][]string{}  // app id -> its doors
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeRoute && strings.HasPrefix(e.From, graph.KindDNS+":") {
			doors[e.From] = append(doors[e.From], e.To)
			apps[e.To] = append(apps[e.To], e.From)
		}
	}
	isFA := func(name string) bool { return slices.Contains(fa, name) }
	wall, part, oidc := map[string]graph.Source{}, map[string]graph.Source{}, map[string]graph.Source{} // door -> evidence
	mark := func(m map[string]graph.Source, host string, src graph.Source) {
		if id := graph.ID(graph.KindDNS, host); doors[id] != nil {
			if _, ok := m[id]; !ok {
				m[id] = src
			}
		}
	}
	for _, f := range files {
		if strings.HasPrefix(f.path, "gitops/bootstrap/authentik-blueprint-") {
			for _, m := range urlHost.FindAllSubmatch(f.b, -1) {
				mark(oidc, string(m[1]), f.src)
			}
			for _, entry := range providerSep.Split(string(f.b), -1) {
				if strings.HasPrefix(entry, "authentik_providers_proxy.proxyprovider") && singleMode.MatchString(entry) {
					if m := extHost.FindStringSubmatch(entry); m != nil {
						mark(wall, m[1], f.src)
					}
				}
			}
		}
		dec := yaml.NewDecoder(strings.NewReader(string(f.b)))
		for {
			var doc struct {
				Kind    string
				Ingress struct {
					Hostname    string
					Middlewares []struct{ Name string }
				}
				Spec struct {
					Routes []struct {
						Match       string
						Middlewares []struct{ Name string }
					}
				}
			}
			if dec.Decode(&doc) != nil {
				break
			}
			if f.app != "" && doc.Ingress.Hostname != "" && slices.ContainsFunc(doc.Ingress.Middlewares, func(m struct{ Name string }) bool { return isFA(m.Name) }) {
				mark(wall, doc.Ingress.Hostname, f.src)
			}
			if doc.Kind != "IngressRoute" {
				continue
			}
			for _, r := range doc.Spec.Routes {
				if !slices.ContainsFunc(r.Middlewares, func(m struct{ Name string }) bool { return isFA(m.Name) }) {
					continue
				}
				for _, h := range routeHost.FindAllStringSubmatch(r.Match, -1) {
					if routePath.MatchString(r.Match) {
						mark(part, h[1], f.src)
					} else {
						mark(wall, h[1], f.src)
					}
				}
			}
		}
	}

	attrs := map[string]map[string]string{}
	set := func(id, k, v string) {
		if attrs[id] == nil {
			attrs[id] = map[string]string{}
		}
		attrs[id][k] = v
	}
	for _, id := range sortedKeys(rl.Walls) {
		if doors[id] == nil {
			d(id, "walls: not a door (a hostname that routes to an app)", realmsSrc)
		}
	}
	for _, id := range sortedKeys(doors) {
		row := rl.Walls[id]
		declared := false
		has := func(kind string, ev map[string]graph.Source) bool {
			_, shown := ev[id]
			if slices.Contains(row, kind) {
				if shown {
					d(id, "walls: "+kind+" is declared, but the sources show it ("+ev[id].Path+"): drop the row", realmsSrc)
				}
				declared = true
				return true
			}
			return shown
		}
		w, p, o := has("forwardauth", wall), has("forwardauth-path", part), has("oidc", oidc)
		var gates []string
		if p {
			gates = append(gates, "forwardauth-path")
		}
		if o {
			gates = append(gates, "oidc")
		}
		if w {
			set(id, "wall", "forwardauth")
		}
		if len(gates) > 0 {
			set(id, "gate", strings.Join(gates, ", "))
		}
		for _, m := range []string{"public", "own"} {
			if slices.Contains(row, m) {
				if w {
					d(id, "walls: "+m+", but a forwardAuth wall guards the whole host", realmsSrc)
				}
				set(id, "mark", m)
			}
		}
		if declared {
			set(id, "walls", "declared")
		}
		if attrs[id] == nil {
			set(id, "open", "true")
		}
	}
	for _, app := range sortedKeys(apps) { // an app is walled when every door is, open when any door is
		all := true
		for _, door := range apps[app] {
			all = all && attrs[door]["wall"] != ""
			if attrs[door]["open"] != "" {
				set(app, "open", "true")
			}
		}
		if all {
			set(app, "wall", "forwardauth")
		}
	}

	// mesh peers: the inventory's host keys
	if b, src, err := x.Read("infra-bootstrap", meshInventory); err == nil {
		var inv struct {
			All struct {
				Children map[string]struct {
					Hosts map[string]yaml.Node `yaml:"hosts"` // keys only: a value is never decoded
				} `yaml:"children"`
			} `yaml:"all"`
		}
		if yaml.Unmarshal(b, &inv) == nil {
			known := map[string]bool{}
			for _, n := range g.Nodes {
				known[n.ID] = true
			}
			var names []string
			for _, grp := range inv.All.Children {
				for name := range grp.Hosts {
					if !slices.Contains(names, name) {
						names = append(names, name)
					}
				}
			}
			slices.Sort(names)
			for _, name := range names {
				id := ""
				for _, k := range []string{graph.KindVM, graph.KindCT, graph.KindHost} {
					if known[graph.ID(k, name)] {
						id = graph.ID(k, name)
						break
					}
				}
				switch {
				case id != "":
					set(id, "mesh", "peer")
				case name != hiddenMachine:
					d(graph.ID(graph.KindRepo, "infra-bootstrap"), "mesh inventory names "+name+", which is no machine atlas knows", src)
				}
			}
		}
	}

	for i, n := range g.Nodes {
		for k, v := range attrs[n.ID] {
			if g.Nodes[i].Attrs == nil {
				g.Nodes[i].Attrs = map[string]string{}
			}
			g.Nodes[i].Attrs[k] = v
		}
	}
	return drift
}

// landmark is how many reviewed things must lean on a building (checked needs edges) or be its parts for it to be a
// landmark: drawn larger by the world (plan/13 §1), never by a model of its own.
const landmarkDependents, landmarkParts = 5, 3

// modelFacts writes the facts the game derives a building's model from: its group's shapes (realms.yaml shapes:),
// landmark, and its ring in the world disc (layer: its group's realms.yaml layers:, its own layer:, or workshops for a
// repo; never on parts or the land). Called once the needs edges are in.
func modelFacts(rl *realms.Realms, g *graph.Graph) {
	deps, parts := map[string]int{}, map[string]int{}
	for _, e := range g.Edges {
		if e.Kind == EdgeNeeds && e.Attrs["checked"] != "no" {
			deps[e.To]++
		}
	}
	for _, n := range g.Nodes {
		if n.Parent != "" {
			parts[n.Parent]++
		}
	}
	for i, n := range g.Nodes {
		set := func(k, v string) {
			if g.Nodes[i].Attrs == nil {
				g.Nodes[i].Attrs = map[string]string{}
			}
			g.Nodes[i].Attrs[k] = v
		}
		if s := rl.Shapes[n.Realm+"/"+n.Group]; len(s) > 0 && n.Parent == "" {
			set("shapes", strings.Join(s, ", "))
		}
		layer := rl.Layers[n.Realm+"/"+n.Group]
		if l := rl.Components[n.ID].Layer; l != "" {
			layer = l
		}
		if n.Realm == realms.Code {
			layer = realms.LayerWorkshops
		}
		if layer != "" && n.Parent == "" && n.Function != realms.FnLand {
			set("layer", layer)
		}
		if deps[n.ID] >= landmarkDependents || parts[n.ID] >= landmarkParts {
			set("landmark", "true")
		}
	}
}

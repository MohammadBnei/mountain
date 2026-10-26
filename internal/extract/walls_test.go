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

// TestWalls: each evidence kind gates its door, rows mark what the sources can't show, an app sums its doors, and the
// mesh inventory's keys (never its values) make peers.
func TestWalls(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "infra-bootstrap", meshInventory)
	os.MkdirAll(filepath.Dir(inv), 0o755)
	os.WriteFile(inv, []byte("all:\n  children:\n    k8s:\n      hosts:\n        k8s-cp-01:\n          ansible_host: 192.168.1.201\n          ansible_ssh_private_key_file: ~/.ssh/id\n    lxc:\n      hosts:\n        k9s-dashboard: {ansible_host: 192.168.1.110}\n        pihole: {}\n    pve:\n      hosts:\n        bnei: {}\n"), 0o644)
	x := &Ctx{Repos: Repos{"infra-bootstrap": {Dir: filepath.Join(dir, "infra-bootstrap")}}}
	src := func(p string) graph.Source { return graph.Source{Repo: "infra-bootstrap", Path: p} }
	files := []sourceFile{
		// a values file: its hostname through a forwardAuth middleware
		{repo: "infra-bootstrap", path: "gitops/platform/values/a/values.yaml", app: "app:a", src: src("a"),
			b: []byte("ingress:\n  hostname: a.example\n  middlewares:\n    - name: gate-forwardauth\n      namespace: default\n")},
		// an IngressRoute: one route whole, one on a path, one through no forwardAuth
		{repo: "infra-bootstrap", path: "gitops/redirectors/r.yaml", app: "app:r", src: src("r"),
			b: []byte("kind: Service\n---\nkind: IngressRoute\nspec:\n  routes:\n    - match: Host(`r.example`)\n      middlewares: [{name: gate-forwardauth}]\n    - match: Host(`p.example`) && PathPrefix(`/admin`)\n      middlewares: [{name: gate-forwardauth}]\n    - match: Host(`s.example`)\n      middlewares: [{name: origin-lock}]\n")},
		// the blueprints: a forward_single provider (and a forward_domain one, which gates no host), an OIDC redirect
		{repo: "infra-bootstrap", path: "gitops/bootstrap/authentik-blueprint-forwardauth.yaml", src: src("fa"),
			b: []byte("data:\n  f.yaml: |\n    entries:\n      - model: authentik_providers_proxy.proxyprovider\n        attrs:\n          mode: forward_single\n          external_host: https://single.example\n      - model: authentik_providers_proxy.proxyprovider\n        attrs:\n          mode: forward_domain\n          external_host: https://s.example\n")},
		{repo: "infra-bootstrap", path: "gitops/bootstrap/authentik-blueprint-o.yaml", src: src("o"),
			b: []byte("entries:\n  - attrs:\n      redirect_uris:\n        - url: https://o.example/callback\n")},
	}
	g := graph.Graph{}
	route := func(host, app string) {
		g.Nodes = append(g.Nodes, graph.Node{ID: "dns:" + host, Kind: "dns"})
		g.Edges = append(g.Edges, graph.Edge{From: "dns:" + host, To: app, Kind: graph.EdgeRoute})
	}
	for _, id := range []string{"app:a", "app:r", "app:single", "app:o", "app:pub", "vm:k8s-cp-01", "host:bnei"} {
		g.Nodes = append(g.Nodes, graph.Node{ID: id, Kind: strings.Split(id, ":")[0]})
	}
	route("a.example", "app:a")
	route("r.example", "app:r")
	route("p.example", "app:r")
	route("s.example", "app:r")
	route("single.example", "app:single")
	route("o.example", "app:o")
	route("pub.example", "app:pub")
	route("dup.example", "app:pub")
	rl := &realms.Realms{Walls: map[string][]string{
		"dns:pub.example": {"public"},
		"dns:dup.example": {"own", "forwardauth-path"},
		"dns:a.example":   {"public"},      // a mark on a walled door: drift
		"dns:o.example":   {"oidc"},        // declared, but the sources show it: drift
		"dns:nowhere":     {"forwardauth"}, // not a door: drift
	}}
	drift := walls(x, rl, files, []string{"gate-forwardauth"}, &g)

	attr := func(id, k string) string {
		for _, n := range g.Nodes {
			if n.ID == id {
				return n.Attrs[k]
			}
		}
		return "?"
	}
	for _, c := range []struct{ id, k, want string }{
		{"dns:a.example", "wall", "forwardauth"},
		{"dns:r.example", "wall", "forwardauth"},
		{"dns:p.example", "gate", "forwardauth-path"},
		{"dns:p.example", "wall", ""},
		{"dns:s.example", "open", "true"}, // no forwardAuth on its route, and a forward_domain provider names no door
		{"dns:single.example", "wall", "forwardauth"},
		{"dns:o.example", "gate", "oidc"},
		{"dns:pub.example", "mark", "public"},
		{"dns:dup.example", "mark", "own"},
		{"dns:dup.example", "gate", "forwardauth-path"},
		{"dns:dup.example", "walls", "declared"},
		{"app:a", "wall", "forwardauth"}, // every door walled
		{"app:r", "wall", ""},            // one of three
		{"app:r", "open", "true"},        // s.example is open
		{"app:pub", "open", ""},
		{"vm:k8s-cp-01", "mesh", "peer"},
		{"host:bnei", "mesh", "peer"},
	} {
		if got := attr(c.id, c.k); got != c.want {
			t.Errorf("%s %s = %q, want %q", c.id, c.k, got, c.want)
		}
	}
	var details []string
	for _, d := range drift {
		details = append(details, d.Subject+": "+d.Detail)
	}
	for _, want := range []string{
		"dns:a.example: walls: public, but a forwardAuth wall guards the whole host",
		"dns:o.example: walls: oidc is declared, but the sources show it (o): drop the row",
		"dns:nowhere: walls: not a door (a hostname that routes to an app)",
		"repo:infra-bootstrap: mesh inventory names pihole, which is no machine atlas knows", // k9s-dashboard: never drawn, skipped
	} {
		if !slices.Contains(details, want) {
			t.Errorf("missing drift %q in\n%s", want, strings.Join(details, "\n"))
		}
	}
	if len(details) != 4 {
		t.Errorf("drift: %d, want 4:\n%s", len(details), strings.Join(details, "\n"))
	}
	checkNoSecrets(t, g)
	for _, n := range g.Nodes {
		for _, v := range n.Attrs {
			if strings.Contains(v, "192.168") || strings.Contains(v, ".ssh") {
				t.Errorf("%s carries an inventory value: %q", n.ID, v)
			}
		}
	}
}

// TestModelFacts: a building asks for its group's shapes; a landmark is leaned on by five reviewed needs, or made of
// three parts; an unproven needs edge does not count, and a part asks for nothing (its building is drawn).
func TestModelFacts(t *testing.T) {
	rl := &realms.Realms{Shapes: map[string][]string{"data/postgres": {"tower", "vault"}}}
	g := graph.Graph{Nodes: []graph.Node{
		{ID: "db:pg", Realm: "data", Group: "postgres"},
		{ID: "vm:a", Realm: "data", Group: "postgres", Parent: "db:pg"}, {ID: "vm:b", Parent: "db:pg"}, {ID: "vm:c", Parent: "db:pg"},
		{ID: "app:auth", Realm: "guard", Group: "authentik"},
	}}
	for i := range 6 {
		checked := "yes"
		if i == 0 {
			checked = "no"
		}
		g.Edges = append(g.Edges, graph.Edge{From: "app:" + string(rune('m'+i)), To: "app:auth", Kind: EdgeNeeds, Attrs: map[string]string{"checked": checked}})
	}
	modelFacts(rl, &g)
	at := func(id, k string) string {
		for _, n := range g.Nodes {
			if n.ID == id {
				return n.Attrs[k]
			}
		}
		return "?"
	}
	if at("db:pg", "shapes") != "tower, vault" || at("db:pg", "landmark") != "true" || at("vm:a", "shapes") != "" || at("app:auth", "shapes") != "" {
		t.Errorf("pg %q %q, part %q, auth %q", at("db:pg", "shapes"), at("db:pg", "landmark"), at("vm:a", "shapes"), at("app:auth", "shapes"))
	}
	if at("app:auth", "landmark") != "true" {
		t.Error("five reviewed needs make a landmark")
	}
	g.Edges[1].Attrs["checked"] = "no" // now four reviewed
	g.Nodes[4].Attrs = nil
	modelFacts(rl, &g)
	if at("app:auth", "landmark") != "" {
		t.Error("an unproven needs edge does not count")
	}
}

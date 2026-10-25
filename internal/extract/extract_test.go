package extract

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// fixtureRealms places what testdata holds, except ghost (a mirror-only app) and mystery (unresolved): those must
// come out as unassigned drift.
const fixtureRealms = `
realms: [platform, data, operations, guard, fleet, apps, code]
functions:
  platform/hosts: land
  platform/nodes: houses
  data/postgres: economy
  operations/build: trade
  operations/atlas: town-centre
  apps/products: product
  guard/authentik: defence
  fleet/cluster-access: barracks
  fleet/control: barracks
hosts: { bnei: h1, server1: h2, ex-laptop: h3 }
pveNodeName: bnei
sources: [infra-bootstrap, agent-fleet, mountain, pi-lab]
k8sPlacement: { k8s-cp-01: bnei }
adrHome: { mountain: infra-bootstrap }
shortADR: { infra-bootstrap: agent-fleet, agent-fleet: agent-fleet }
skip: { infra-bootstrap: [kubespray] }
components:
  host:bnei: { realm: platform, group: hosts }
  host:server1: { realm: platform, group: hosts }
  host:ex-laptop: { realm: platform, group: hosts }
  vm:k8s-cp-01: { realm: platform, group: nodes }
  vm:k8s-worker-01: { realm: platform, group: nodes }
  vm:pg02: { realm: data, group: postgres }
  ct:build-runner: { realm: operations, group: build }
  app:wird: { realm: apps, group: products }
  app:vos-monolith-dev: { realm: apps, group: products }
  app:platform-authentik: { realm: guard, group: authentik }
  app:platform-thot: { realm: fleet, group: cluster-access }
  app:platform-second: { realm: fleet, group: cluster-access, function: trade }
  app:mountain: { realm: operations, group: atlas, paths: [mountain/] }
  svc:agent-fleet/core: { realm: fleet, group: control, paths: [agent-fleet/core/] }
`

func fixture(t *testing.T) Result {
	t.Helper()
	rl, err := realms.Parse([]byte(fixtureRealms))
	if err != nil {
		t.Fatal(err)
	}
	repos := Repos{}
	for _, r := range []string{"infra-bootstrap", "agent-fleet", "pi-lab", "mountain"} {
		repos[r] = Repo{Dir: "testdata/" + r, SHA: strings.Repeat(r[:1], 40), Branch: "main"}
	}
	res, err := Run(repos, rl, All)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func node(t *testing.T, g graph.Graph, id string) graph.Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %s", id)
	return graph.Node{}
}

func noNode(t *testing.T, g graph.Graph, id string) {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			t.Errorf("node %s should not be there (%s)", id, n.Source.Path)
		}
	}
}

func edge(g graph.Graph, from, to, kind string) (graph.Edge, bool) {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return e, true
		}
	}
	return graph.Edge{}, false
}

func hasDrift(g graph.Graph, kind, subject, detail string) bool {
	for _, d := range g.Drift {
		if d.Kind == kind && d.Subject == subject && strings.Contains(d.Detail, detail) {
			return true
		}
	}
	return false
}

func TestPlace(t *testing.T) {
	g := fixture(t).Graph
	if n := node(t, g, "app:platform-second"); n.Function != realms.FnTrade {
		t.Errorf("a component's own function wins over its group's: %+v", n)
	}
	if n := node(t, g, "app:platform-thot"); n.Function != realms.FnBarracks {
		t.Errorf("thot = %+v", n)
	}
	for id, group := range map[string]string{
		"repo:wird":            "apps",   // fed by app:wird alone
		"repo:vos-monolith":    "apps",   // two apps, one realm
		"repo:infra-bootstrap": "shared", // the chart and thot's paths: apps, fleet and more
	} {
		if n := node(t, g, id); n.Realm != realms.Code || n.Group != group || n.Function != realms.FnWorkshop {
			t.Errorf("%s = %s/%s %s, want code/%s workshop", id, n.Realm, n.Group, n.Function, group)
		}
	}
	for _, n := range g.Nodes {
		if n.Kind == graph.KindRepo && n.Realm != realms.Code {
			t.Errorf("%s is not in code", n.ID)
		}
		if (n.Kind == graph.KindDNS || n.Kind == graph.KindADR) && (n.Realm != "" || n.Function != "") {
			t.Errorf("%s is placed: %+v", n.ID, n)
		}
	}
}

func TestGitops(t *testing.T) {
	g := fixture(t).Graph
	w := node(t, g, "app:wird")
	if w.Realm != "apps" || w.Group != "products" || w.Function != realms.FnProduct || w.Attrs["namespace"] != "wird" {
		t.Errorf("app:wird = %+v", w)
	}
	if e, ok := edge(g, "app:wird", "repo:wird", graph.EdgeGit); !ok || e.Attrs["branch"] != "main" || e.Attrs["paths"] != "helm/values.yaml" {
		t.Errorf("wird -> its repo: %+v %v", e, ok)
	}
	if e, ok := edge(g, "app:wird", "repo:infra-bootstrap", graph.EdgeGit); !ok || e.Attrs["paths"] != "gitops/platform/common-app-chart" {
		t.Errorf("wird -> the chart: %+v %v", e, ok)
	}
	if _, ok := edge(g, "dns:wird.bnei.dev", "app:wird", graph.EdgeRoute); !ok {
		t.Error("no route to wird")
	}
	if e, _ := edge(g, "app:vos-monolith-dev", "repo:vos-monolith", graph.EdgeGit); e.Attrs["branch"] != "dev" || e.Attrs["paths"] != "back/helm/values.yaml,back/helm/values-dev.yaml" {
		t.Errorf("vos dev: %+v", e)
	}
	a := node(t, g, "app:platform-authentik")
	if a.Attrs["chart"] != "https://charts.goauthentik.io/authentik@2026.8.4" || a.Attrs["namespace"] != "authentik" {
		t.Errorf("authentik = %+v", a)
	}
	if e, ok := edge(g, "app:platform-thot", "repo:infra-bootstrap", graph.EdgeGit); !ok || e.Attrs["paths"] != "gitops/platform/thot" {
		t.Errorf("thot: %+v %v", e, ok)
	}
	node(t, g, "app:platform-second") // the second document of a file: ArgoCD applies it, so atlas reads it
	if !hasDrift(g, graph.DriftUnresolved, "repo:infra-bootstrap", "gitgen has a generator atlas cannot expand") {
		t.Error("a git generator is skipped silently")
	}
	if !hasDrift(g, graph.DriftMirror, "app:vos-monolith-dev", "dev.api.example.com") {
		t.Error("the registry/appset hostname mismatch is not drift")
	}
	if !hasDrift(g, graph.DriftMirror, "app:ghost", "not in the registry") || !hasDrift(g, graph.DriftUnassigned, "app:ghost", "") {
		t.Error("ghost: want mirror and unassigned drift")
	}
	if hasDrift(g, graph.DriftMirror, "app:wird", "") {
		t.Error("wird matches its mirror")
	}
}

func TestTracked(t *testing.T) {
	got := map[Track]bool{}
	for _, tr := range fixture(t).Tracked {
		got[tr] = true
	}
	for _, want := range []Track{{"infra-bootstrap", "main"}, {"wird", "main"}, {"vos-monolith", "dev"}, {"ghost", "main"}} {
		if !got[want] {
			t.Errorf("not tracked: %+v (have %v)", want, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("tracked %v: a chart revision is not a branch", got)
	}
}

func TestMachines(t *testing.T) {
	g := fixture(t).Graph
	if h := node(t, g, "host:bnei"); h.Attrs["island"] != "h1" || h.Attrs["ip"] != "192.168.1.165" {
		t.Errorf("host:bnei = %+v", h)
	}
	if _, ok := edge(g, "vm:pg02", "host:bnei", graph.EdgeRunsOn); !ok {
		t.Error("pg02 (var.pve_node_name) is not on bnei")
	}
	if _, ok := edge(g, "ct:build-runner", "host:server1", graph.EdgeRunsOn); !ok {
		t.Error("build-runner is not on server1")
	}
	if n := node(t, g, "vm:k8s-cp-01"); n.Attrs["k8s"] != "control-plane,etcd" || n.Attrs["ip"] != "192.168.1.201" {
		t.Errorf("k8s-cp-01 = %+v", n)
	}
	if _, ok := edge(g, "vm:k8s-cp-01", "host:bnei", graph.EdgeRunsOn); !ok {
		t.Error("k8s-cp-01 not placed")
	}
	if !hasDrift(g, graph.DriftUnresolved, "vm:k8s-worker-01", "k8sPlacement") {
		t.Error("an unplaced k8s node is not drift")
	}
	if !hasDrift(g, graph.DriftUnresolved, "vm:mystery", "node_name") {
		t.Error("node_name = local.x is not drift")
	}
	for _, id := range []string{"ct:k9s-dashboard", "vm:ubuntu-template", "vm:k8s_node"} {
		noNode(t, g, id)
	}
}

func TestADRs(t *testing.T) {
	g := fixture(t).Graph
	if a := node(t, g, "adr:infra-bootstrap/0001"); a.Attrs["title"] != "The first decision" || a.Attrs["status"] != "Accepted" {
		t.Errorf("0001 = %+v", a.Attrs)
	}
	if a := node(t, g, "adr:agent-fleet/0010"); a.Attrs["title"] != "Dash title" || a.Attrs["status"] != "Proposed" {
		t.Errorf("fleet 0010 = %+v", a.Attrs)
	}
	if a := node(t, g, "adr:infra-bootstrap/0002"); a.Attrs["status"] != "Superseded" {
		t.Errorf("0002 status = %q", a.Attrs["status"])
	}
	if a := node(t, g, "adr:pi-lab/0001"); a.Attrs["status"] != "Accepted" {
		t.Errorf("pi-lab status = %q", a.Attrs["status"])
	}
	for _, c := range []struct{ from, to, kind string }{
		{"adr:infra-bootstrap/0001", "adr:infra-bootstrap/0002", graph.EdgeRelated},
		{"adr:infra-bootstrap/0001", "adr:agent-fleet/0010", graph.EdgeRelated},
		{"adr:agent-fleet/0071", "adr:infra-bootstrap/0002", graph.EdgeSupersede},
		{"adr:agent-fleet/0071", "adr:agent-fleet/0010", graph.EdgeAmends},
		{"adr:agent-fleet/0071", "adr:infra-bootstrap/0001", graph.EdgeRelated}, // a URL link, on a continuation line
		{"adr:agent-fleet/0071", "adr:pi-lab/0001", graph.EdgeRelated},          // a nested bullet of a wrapped Related
	} {
		if _, ok := edge(g, c.from, c.to, c.kind); !ok {
			t.Errorf("no %s %s -> %s", c.kind, c.from, c.to)
		}
	}
	count := func(from, to string) string {
		e, _ := edge(g, from, to, graph.EdgeGoverns)
		return e.Attrs["count"]
	}
	// notes.md: ADR-0001 + two ../adr links, one whose text is an id (counted once); kubespray/ and the submodule
	// are not the repo's words.
	if c := count("repo:infra-bootstrap", "adr:infra-bootstrap/0001"); c != "3" {
		t.Errorf("infra -> its 0001 = %s, want 3", c)
	}
	if c := count("repo:infra-bootstrap", "adr:infra-bootstrap/0002"); c != "1" {
		t.Errorf("docs/adr/0002 = %s", c)
	}
	if c := count("repo:infra-bootstrap", "adr:agent-fleet/0071"); c != "1" {
		t.Errorf("infra's bare short form = %s, want the fleet's ADR", c)
	}
	if c := count("repo:infra-bootstrap", "adr:agent-fleet/0010"); c != "1" {
		t.Errorf("a prefix ending the line before = %s", c)
	}
	if c := count("svc:agent-fleet/core", "adr:infra-bootstrap/0001"); c != "1" {
		t.Errorf("core -> infra 0001 = %s", c)
	}
	if c := count("svc:agent-fleet/core", "adr:agent-fleet/0071"); c != "2" {
		t.Errorf("core -> its 0071 = %s", c)
	}
	if c := count("repo:pi-lab", "adr:infra-bootstrap/0002"); c != "1" {
		t.Errorf("pi-lab wrapped prefix = %s", c)
	}
	if c := count("repo:pi-lab", "adr:pi-lab/0001"); c != "2" { // its bare adr/0001 is its own, not the fleet's
		t.Errorf("pi-lab own = %s", c)
	}
	// mountain has no ADRs: adrHome sends its bare citation to infra-bootstrap; app:mountain isn't in this run, so
	// the edge lands on the repo.
	if c := count("repo:mountain", "adr:infra-bootstrap/0002"); c != "1" {
		t.Errorf("mountain adrHome = %s", c)
	}
	for _, e := range g.Edges {
		if strings.HasSuffix(e.To, "/0007") {
			t.Errorf("a lane A id was read: %+v", e)
		}
	}
	if !hasDrift(g, graph.DriftUnresolved, "adr:infra-bootstrap/0099", "no such ADR") {
		t.Error("a typo is not drift")
	}
	if !hasDrift(g, graph.DriftMirror, "adr:infra-bootstrap/0002", "not in docs/adr/README.md") {
		t.Error("an unindexed ADR is not drift")
	}
	if !hasDrift(g, graph.DriftUnresolved, "adr:infra-bootstrap/0003", "no file") {
		t.Error("an index row with no file is not drift")
	}
}

// TestDeterministic: two runs over the same checkouts are the same graph, byte for byte.
func TestDeterministic(t *testing.T) {
	if a, b := graph.Hash(fixture(t).Graph), graph.Hash(fixture(t).Graph); a != b {
		t.Fatal("two runs differ")
	}
}

// TestNoSecrets: nothing the fixtures hold besides structure reaches the graph. some-secret.yaml carries a token
// value; no stored string may contain it, and none may look like a key (long and high-entropy).
func TestNoSecrets(t *testing.T) {
	checkNoSecrets(t, fixture(t).Graph)
}

func checkNoSecrets(t *testing.T, g graph.Graph) {
	t.Helper()
	var strs []string
	for _, n := range g.Nodes {
		strs = append(strs, n.ID)
		for _, v := range n.Attrs {
			strs = append(strs, v)
		}
	}
	for _, e := range g.Edges {
		for _, v := range e.Attrs {
			strs = append(strs, v)
		}
	}
	for _, d := range g.Drift {
		strs = append(strs, d.Detail)
	}
	for _, s := range strs {
		if strings.Contains(s, "not-a-real-value") {
			t.Errorf("a secret value reached the graph: %q", s)
		}
		for _, w := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" ,=;:/@()._-", r) }) {
			if len(w) >= 20 && entropy(w) > 3.5 && strings.ContainsAny(w, "0123456789") {
				t.Errorf("a key-like string reached the graph: %q", w)
			}
		}
	}
}

// entropy is w's Shannon entropy in bits per character.
func entropy(w string) float64 {
	f := map[rune]float64{}
	for _, r := range w {
		f[r]++
	}
	var h float64
	n := float64(len([]rune(w)))
	for _, c := range f {
		p := c / n
		h -= p * math.Log2(p)
	}
	return h
}

// TestBranch: a checkout on a branch ArgoCD doesn't deploy is drift (atlas would be projecting work in progress).
func TestBranch(t *testing.T) {
	rl, _ := realms.Parse([]byte(fixtureRealms))
	repos := Repos{"infra-bootstrap": {Dir: "testdata/infra-bootstrap", SHA: strings.Repeat("i", 40), Branch: "wip"}}
	res, err := Run(repos, rl, All)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDrift(res.Graph, graph.DriftUnresolved, "repo:infra-bootstrap", "checked out on branch wip; ArgoCD deploys main") {
		t.Errorf("no branch drift: %+v", res.Graph.Drift)
	}
	if hasDrift(fixture(t).Graph, graph.DriftUnresolved, "repo:infra-bootstrap", "checked out on") {
		t.Error("branch drift on a checkout of main")
	}
}

// TestSymlinkOut: a file that is a symlink out of the checkout is never read, nor is a "../" path.
func TestSymlinkOut(t *testing.T) {
	outside := t.TempDir() + "/secret.md"
	if err := os.WriteFile(outside, []byte("# ADR-0001: SECRET-TITLE\n\n**Status:** hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.MkdirAll(repo+"/docs/adr", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, repo+"/docs/adr/0001-link.md"); err != nil {
		t.Fatal(err)
	}
	rl, _ := realms.Parse([]byte(fixtureRealms))
	res, err := Run(Repos{"r": {Dir: repo, SHA: strings.Repeat("r", 40)}}, rl, All)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Graph.Nodes {
		if n.Kind == graph.KindADR {
			t.Errorf("a symlinked ADR was read: %+v", n)
		}
	}
	x := &Ctx{Repos: Repos{"r": {Dir: repo}}}
	if _, _, err := x.Read("r", "../secret.md"); err == nil {
		t.Error("Read followed ../")
	}
	if _, _, err := x.Read("r", "docs/adr/0001-link.md"); err == nil {
		t.Error("Read followed a symlink")
	}
}

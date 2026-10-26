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

// TestWatchNeeds: watch rows become attributes and are checked against the declared rules both ways; needs rows
// become edges, checked against their evidence both ways; comments are never evidence; a probe with no rule says so.
func TestWatchNeeds(t *testing.T) {
	dir := t.TempDir()
	write := func(repo, path, body string) {
		p := filepath.Join(dir, repo, path)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// rules: two Prometheus alerts (one in a templated string), one Grafana rule, one logAlerts rule in another repo
	write("infra-bootstrap", "gitops/platform/values/traefik/values.yaml", "prometheusRule:\n  groups:\n    - rules:\n        - alert: CertExpiring\n          expr: x\n")
	write("infra-bootstrap", "gitops/platform/values/netbird/values.yaml", "extraManifests:\n  - |\n    spec:\n      groups:\n        - rules:\n            - alert: ServerDown\n              expr: up{namespace=\"{{ .Release.Namespace }}\"} == 0\n")
	write("infra-bootstrap", "gitops/platform/values/grafana/values.yaml", "alerting:\n  rules.yaml:\n    groups:\n      - rules:\n          - uid: blind\n            title: Prometheus not answering\n          - uid: orphan\n            title: Nobody watches this\n")
	write("other", "helm/values.yaml", "logAlerts:\n  rules:\n    - uid: dead-letter\n      title: Dead letter\n")
	// evidence: a blueprint naming x.example's redirect, a forwardAuth middleware and its use, an infisical block,
	// an S3 endpoint only in a comment (not evidence), and pigsty's databases
	write("infra-bootstrap", "gitops/bootstrap/authentik-blueprint-x.yaml", "entries:\n  - attrs:\n      redirect_uris:\n        - url: https://x.example/callback\n")
	write("infra-bootstrap", "gitops/bootstrap/gate-forwardauth-middleware.yaml", "kind: Middleware\nmetadata:\n  name: gate-forwardauth\n")
	write("other", "helm/values.yaml.extra.yaml", "ingress:\n  middlewares:\n    - name: gate-forwardauth\n")
	write("infra-bootstrap", "gitops/platform/values/vault/values.yaml", "infisical:\n  enabled: true\n  projectSlug: v\n# endpoint: s3.bnei.dev\nother: x # s3.bnei.dev in a trailing comment\n")
	write("infra-bootstrap", "pigsty/pigsty.yml", "all:\n  children:\n    pg-proxmox:\n      vars:\n        pg_cluster: pg-proxmox\n        pg_databases:\n          - { name: xdb, owner: dbuser_secret }\n")

	rl, err := realms.Parse([]byte(`realms: [infra, code]
functions: {infra/g: utility}
hosts: {a: h1}
pveNodeName: a
sources: [infra-bootstrap, other]
probes: {guest-x: vm:guest}
components:
  app:traefik: {realm: infra, group: g}
  app:netbird: {realm: infra, group: g}
  app:x: {realm: infra, group: g, paths: [other/helm/]}
  app:vault: {realm: infra, group: g}
  app:prom: {realm: infra, group: g}
  app:platform-authentik: {realm: infra, group: g}
  app:platform-infisical: {realm: infra, group: g}
  db:pg-proxmox: {realm: infra, group: g}
  ct:garage-storage: {realm: infra, group: g}
  vm:guest: {realm: infra, group: g}
watch:
  app:traefik: {prometheus: [CertExpiring, Gone]}
  app:netbird: {prometheus: [ServerDown]}
  app:prom: {grafana: [blind]}
general: {grafana: [dead-letter]}
needs:
  app:x: [oidc, forwardauth, postgres:xdb, postgres:nodb]
  app:vault: [s3]
`))
	if err != nil {
		t.Fatal(err)
	}
	repos := Repos{"infra-bootstrap": {Dir: filepath.Join(dir, "infra-bootstrap"), SHA: "a"}, "other": {Dir: filepath.Join(dir, "other"), SHA: "b"}}
	g := graph.Graph{}
	for _, id := range []string{"app:traefik", "app:netbird", "app:x", "app:vault", "app:prom", "app:platform-authentik", "app:platform-infisical", "db:pg-proxmox", "ct:garage-storage", "vm:guest", "dns:x.example"} {
		kind, _, _ := strings.Cut(id, ":")
		g.Nodes = append(g.Nodes, graph.Node{ID: id, Kind: kind})
	}
	g.Edges = append(g.Edges, graph.Edge{From: "dns:x.example", To: "app:x", Kind: graph.EdgeRoute})
	watchNeeds(repos, rl, &g)

	attr := func(id, k string) string {
		for _, n := range g.Nodes {
			if n.ID == id {
				return n.Attrs[k]
			}
		}
		return ""
	}
	if attr("app:traefik", "watch") != "CertExpiring, Gone" || attr("app:netbird", "watch") != "ServerDown" || attr("app:prom", "watch-grafana") != "blind" {
		t.Errorf("watch attrs: %q %q %q", attr("app:traefik", "watch"), attr("app:netbird", "watch"), attr("app:prom", "watch-grafana"))
	}
	if attr("vm:guest", "probe") != "guest-x, no alert rule" {
		t.Errorf("a probe with no rule: %q", attr("vm:guest", "probe"))
	}
	var details []string
	for _, d := range g.Drift {
		details = append(details, d.Subject+": "+d.Detail)
	}
	for _, want := range []string{
		"app:traefik: watch: prometheus rule Gone is declared in no source",
		"repo:infra-bootstrap: grafana rule orphan watches nothing in realms.yaml (watch: or general:)",
		"app:x: needs postgres:nodb: no such database in pigsty's pg_databases",
		"app:vault: needs s3: no evidence of it in the sources",
		"app:vault: evidence of infisical (gitops/platform/values/vault/values.yaml) but no needs row names it",
	} {
		if !slices.Contains(details, want) {
			t.Errorf("missing drift %q in\n%s", want, strings.Join(details, "\n"))
		}
	}
	if len(details) != 5 {
		t.Errorf("drift: %d, want 5:\n%s", len(details), strings.Join(details, "\n"))
	}
	var edges []string
	for _, e := range g.Edges {
		if e.Kind == EdgeNeeds {
			edges = append(edges, e.From+">"+e.To+" "+e.Attrs["via"]+" "+e.Attrs["checked"]+" "+e.Attrs["db"])
		}
	}
	// one edge per (from, to): oidc and forwardauth both name authentik; the two postgres rows join, checked the
	// weakest (nodb is not a database); an unproven row is still an edge, marked checked: no
	if want := []string{"app:vault>ct:garage-storage s3 no ", "app:x>app:platform-authentik oidc, forwardauth yes ", "app:x>db:pg-proxmox postgres, postgres no xdb, nodb"}; !slices.Equal(edges, want) {
		t.Errorf("needs edges: %q", edges)
	}
}

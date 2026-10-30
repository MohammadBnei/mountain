package extract

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/realms"
)

var update = flag.Bool("update", false, "rewrite testdata/golden/*.json from the current code")

// watchRealms adds to fixtureRealms the rows only the watch pass reads: watchtowers, general rules, needs, walls,
// shapes and a probe.
const watchRealms = fixtureRealms + `watch:
  app:platform-authentik: {prometheus: [AuthDown, Gone]}
  db:pg-proxmox: {grafana: [pg-lag]}
general: {grafana: [blind]}
needs:
  app:wird: [oidc, forwardauth, postgres:nodb, declared:db:redis-main]
walls:
  dns:wird.bnei.dev: [public]
  dns:dev.api.other.com: [own]
  dns:nowhere.example: [forwardauth]
shapes:
  data/postgres: [tower, vault]
probes: {guest-build: ct:build-runner}
`

// watchFiles is the evidence the watch pass finds, written over a copy of testdata/infra-bootstrap: two alert rules
// (one named by no watchtower), Grafana rules (one general, one nobody's), an OIDC redirect for wird's door and a
// forwardAuth middleware.
var watchFiles = map[string]string{
	"gitops/platform/values/authentik/values.yaml":      "prometheusRule:\n  groups:\n    - rules:\n        - alert: AuthDown\n          for: 5m\n          labels:\n            severity: critical\n        - alert: Stray\n          labels:\n            severity: warning\n",
	"gitops/platform/values/grafana/values.yaml":        "alerting:\n  rules.yaml:\n    groups:\n      - rules:\n          - uid: blind\n            title: Prometheus not answering\n          - uid: orphan\n            title: Nobody watches this\n",
	"gitops/bootstrap/authentik-blueprint-wird.yaml":    "entries:\n  - attrs:\n      redirect_uris:\n        - url: https://wird.bnei.dev/callback\n",
	"gitops/bootstrap/gate-forwardauth-middleware.yaml": "kind: Middleware\nmetadata:\n  name: gate-forwardauth\n",
}

// TestGolden pins the projection of testdata: whole (every repo), as phase 1 reads it (infra-bootstrap alone, the
// branches ArgoCD deploys from), and with the watch pass's rows and evidence added (watch). A change to an extractor
// or a pass that moves any node, edge, drift or version shows here. Regenerate with -update, and review the diff.
func TestGolden(t *testing.T) {
	rl, err := realms.Parse([]byte(fixtureRealms))
	if err != nil {
		t.Fatal(err)
	}
	wrl, err := realms.Parse([]byte(watchRealms))
	if err != nil {
		t.Fatal(err)
	}
	all := Repos{}
	for _, r := range []string{"infra-bootstrap", "agent-fleet", "pi-lab", "mountain"} {
		all[r] = Repo{Dir: "testdata/" + r, SHA: strings.Repeat(r[:1], 40), Branch: "main"}
	}
	watched := Repos{}
	for r, repo := range all {
		dir := filepath.Join(t.TempDir(), r)
		if err := os.CopyFS(dir, os.DirFS(repo.Dir)); err != nil {
			t.Fatal(err)
		}
		repo.Dir = dir
		watched[r] = repo
	}
	for p, body := range watchFiles {
		f := filepath.Join(watched["infra-bootstrap"].Dir, p)
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, c := range map[string]struct {
		repos Repos
		rl    *realms.Realms
	}{"all": {all, rl}, "phase1": {Repos{"infra-bootstrap": all["infra-bootstrap"]}, rl}, "watch": {watched, wrl}} {
		t.Run(name, func(t *testing.T) {
			res, err := Run(c.repos, c.rl, All)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(res, "", " ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := "testdata/golden/" + name + ".json"
			if *update {
				if err := os.MkdirAll("testdata/golden", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s changed: run with -update and review the diff", path)
			}
		})
	}
}

// TestPasses: the passes keep their documented order (each runs on what the ones before made), and each has a name
// and a version, which a stored projection records.
func TestPasses(t *testing.T) {
	var names []string
	for _, p := range passes() {
		if p.name == "" || p.version < 1 || p.run == nil {
			t.Errorf("pass %q: version %d", p.name, p.version)
		}
		names = append(names, p.name)
	}
	if want := []string{"declare", "repoNodes", "resolve", "rehome", "place", "watch", "image", "branchDrift"}; !slices.Equal(names, want) {
		t.Errorf("passes %v, want %v (a new order is a new version of every pass it moves)", names, want)
	}
}

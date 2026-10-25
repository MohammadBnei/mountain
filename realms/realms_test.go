package realms

import "testing"

func TestEmbedded(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Realms) != 7 || len(r.Hosts) != 3 {
		t.Errorf("realms %v hosts %v", r.Realms, r.Hosts)
	}
	if got := r.Owner("agent-fleet", "core/internal/dispatch/dispatch.go"); got != "svc:agent-fleet/core" {
		t.Errorf("owner = %q", got)
	}
	if r.ProbeNode("pve-server1") != "host:server1" || r.ProbeNode("guest-witness") != "vm:pg-etcd-witness" || r.ProbeNode("pve-nope") != "" || r.ProbeNode("other") != "" {
		t.Error("ProbeNode")
	}
	if len(r.Sources) == 0 || r.Sources[0] != "infra-bootstrap" {
		t.Errorf("sources %v", r.Sources)
	}
	if got := r.Owner("infra-bootstrap", "README.md"); got != "" {
		t.Errorf("an unlisted path has owner %q", got)
	}
}

func TestFunction(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"app:platform-traefik":           FnUtility,
		"app:platform-blackbox-exporter": FnDefence, // its own, over observability's trade
		"app:platform-prometheus":        FnTrade,
		"app:mountain":                   FnTownCentre,
		"host:bnei":                      FnLand,
		"vm:k8s-worker-01":               FnHouses,
		"app:nope":                       "",
	} {
		if got := r.Function(id); got != want {
			t.Errorf("Function(%s) = %q, want %q", id, got, want)
		}
	}
	y := "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:y: {realm: infra, group: h, function: trade}}"
	if p, err := Parse([]byte(y)); err != nil || p.Function("app:x") != FnUtility || p.Function("app:y") != FnTrade {
		t.Errorf("minimal: %v", err)
	}
}

func TestParseRefuses(t *testing.T) {
	for name, y := range map[string]string{
		"unknown realm":  "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: nope, group: g}}",
		"no group":       "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra}}",
		"bad path":       "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, paths: [nope]}}",
		"bad placement":  "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nk8sPlacement: {n: b}",
		"bad pve name":   "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: b",
		"bad sources":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [mountain]",
		"no sources":     "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a",
		"bad probe":      "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nprobes: {guest-x: vm:nope}",
		"no code realm":  "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"bad fn key":     "realms: [infra, code]\nfunctions: {nope/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"code fn key":    "realms: [infra, code]\nfunctions: {code/g: workshop}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"unknown fn":     "realms: [infra, code]\nfunctions: {infra/g: castle}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"component fn":   "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, function: castle}}",
		"group no fn":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: h}}",
		"repo listed":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {repo:x: {realm: infra, group: g}}",
		"workshop fn":    "realms: [infra, code]\nfunctions: {infra/g: workshop}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"workshop own":   "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, function: workshop}}",
		"code namespace": "realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nnamespaces: {kube-system: code}",
		"listed in code": "realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {repo:x: {realm: code, group: g, function: workshop}}",
		"bad namespace":  "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nnamespaces: {kube-system: nope}",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

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

func TestParseRefuses(t *testing.T) {
	for name, y := range map[string]string{
		"unknown realm": "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\ncomponents: {app:x: {realm: nope, group: g}}",
		"no group":      "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\ncomponents: {app:x: {realm: infra}}",
		"bad path":      "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\ncomponents: {app:x: {realm: infra, group: g, paths: [nope]}}",
		"bad placement": "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nk8sPlacement: {n: b}",
		"bad pve name":  "realms: [infra]\nhosts: {a: h1}\npveNodeName: b",
		"bad sources":   "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nsources: [mountain]",
		"bad probe":     "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nprobes: {guest-x: vm:nope}",
		"bad namespace": "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nnamespaces: {kube-system: nope}",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

package realms

import (
	"slices"
	"testing"
)

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
		"vm:esprimo-1":                   FnHouses,
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
		"unknown realm":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: nope, group: g}}",
		"no group":         "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra}}",
		"bad path":         "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, paths: [nope]}}",
		"bad placement":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nk8sPlacement: {n: b}",
		"bare and placed":  "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nk8sPlacement: {n: a}\ncomponents: {vm:n: {realm: infra, group: g}}\nbareMetal: [n]",
		"bare typo":        "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {vm:n: {realm: infra, group: g}}\nbareMetal: [m]",
		"bad pve name":     "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: b",
		"bad sources":      "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [mountain]",
		"no sources":       "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a",
		"bad probe":        "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nprobes: {guest-x: vm:nope}",
		"no code realm":    "realms: [infra]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"bad fn key":       "realms: [infra, code]\nfunctions: {nope/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"code fn key":      "realms: [infra, code]\nfunctions: {code/g: workshop}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"unknown fn":       "realms: [infra, code]\nfunctions: {infra/g: castle}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"component fn":     "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, function: castle}}",
		"group no fn":      "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: h}}",
		"repo listed":      "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {repo:x: {realm: infra, group: g}}",
		"workshop fn":      "realms: [infra, code]\nfunctions: {infra/g: workshop}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]",
		"workshop own":     "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g, function: workshop}}",
		"code namespace":   "realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nnamespaces: {kube-system: code}",
		"listed in code":   "realms: [infra, code]\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {repo:x: {realm: code, group: g, function: workshop}}",
		"bad namespace":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\nnamespaces: {kube-system: nope}",
		"watch unknown":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nwatch: {app:nope: {prometheus: [A]}}",
		"watch empty":      "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nwatch: {app:x: {}}",
		"needs unknown":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nneeds: {app:nope: [oidc]}",
		"needs bad kind":   "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nneeds: {app:x: [smtp]}",
		"needs bare pg":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nneeds: {app:x: [postgres]}",
		"needs no node":    "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nneeds: {app:x: [\"declared:app:nope\"]}",
		"needs itself":     "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\nneeds: {app:platform-authentik: [oidc]}",
		"addons not addon": "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\naddons: {app:x: [static]}",
		"addons bad":       "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\ncomponents: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\naddons: {addon:c: [statefulset/x]}",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestNeedsRows(t *testing.T) {
	y := "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\n" +
		"components: {app:x: {realm: infra, group: g}, app:platform-authentik: {realm: infra, group: g}, db:pg-proxmox: {realm: infra, group: g}, addon:c: {realm: infra, group: g}}\n" +
		"needs: {app:x: [oidc, postgres:xdb, \"declared:db:pg-proxmox\"]}\nwatch: {app:x: {grafana: [u]}}\naddons: {addon:c: [daemonset/c, deployment/c-op, static]}"
	p, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	for s, want := range map[string]Need{
		"oidc":                   {On: "app:platform-authentik", Via: "oidc"},
		"postgres:xdb":           {On: "db:pg-proxmox", Via: "postgres", Arg: "xdb"},
		"declared:db:pg-proxmox": {On: "db:pg-proxmox", Via: "declared"},
	} {
		if got, err := ParseNeed(s); err != nil || got != want {
			t.Errorf("ParseNeed(%s) = %+v %v", s, got, err)
		}
	}
	if len(p.Needs["app:x"]) != 3 || len(p.Watch["app:x"].Grafana) != 1 || len(p.Addons["addon:c"]) != 3 {
		t.Errorf("rows: %+v %+v %+v", p.Needs, p.Watch, p.Addons)
	}
}

// TestWallsRows: a walls row is a door with known kinds, and never both marks.
func TestWallsRows(t *testing.T) {
	base := "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\n"
	if _, err := Parse([]byte(base + "walls: {dns:x.example: [public, forwardauth-path]}")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"walls: {app:x: [public]}", "walls: {dns:x.example: [basic]}", "walls: {dns:x.example: [public, own]}"} {
		if _, err := Parse([]byte(base + bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// TestDigest: the digest is the file's: the same bytes give the same one, a changed byte another.
func TestDigest(t *testing.T) {
	a, err := Parse(embedded)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Parse(embedded)
	c, err := Parse(append(slices.Clone(embedded), []byte("\n# a comment\n")...))
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest == 0 || a.Digest != b.Digest || a.Digest == c.Digest {
		t.Errorf("digests %d %d %d", a.Digest, b.Digest, c.Digest)
	}
}

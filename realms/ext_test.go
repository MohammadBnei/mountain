package realms

import "testing"

// extBase is a minimal realms file; a test appends its needs row.
const extBase = "realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\n" +
	"components: {app:x: {realm: infra, group: g}, app:y: {realm: infra, group: g}}\n"

// TestExtNeeds: a needs row may name a moon (declared:ext:<name>), which no component lists; its name must still be
// a node name. The live file declares the owner's moons (plan/17 B2 owner question 4).
func TestExtNeeds(t *testing.T) {
	p, err := Parse([]byte(extBase + "needs: {app:x: [\"declared:ext:github\"], app:y: [\"declared:ext:github\", \"declared:ext:pi-lab\"]}"))
	if err != nil {
		t.Fatalf("an ext need refused: %v", err)
	}
	if n, err := ParseNeed(p.Needs["app:x"][0]); err != nil || n != (Need{On: "ext:github", Via: "declared"}) {
		t.Errorf("ParseNeed = %+v %v", n, err)
	}
	for name, row := range map[string]string{
		"empty name":     `["declared:ext:"]`,
		"a space":        `["declared:ext:git hub"]`,
		"upper case":     `["declared:ext:GitHub"]`,
		"a leading dash": `["declared:ext:-x"]`,
		"not declared":   `["ext:github"]`, // a moon is only ever declared: no finder checks it
	} {
		if _, err := Parse([]byte(extBase + "needs: {app:x: " + row + "}")); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}

	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for id, moons := range map[string][]string{
		"app:mountain":         {"ext:forgejo"},
		"app:agent-fleet-core": {"ext:github", "ext:anthropic"},
		"app:platform-traefik": {"ext:cloudflare"},
		"app:pi-lab":           {"ext:pi-lab"},
	} {
		for _, m := range moons {
			if !needs(r, id, m) {
				t.Errorf("realms.yaml: %s does not declare %s (needs: %v)", id, m, r.Needs[id])
			}
		}
	}
}

// needs: id's needs row declares on.
func needs(r *Realms, id, on string) bool {
	for _, s := range r.Needs[id] {
		if n, err := ParseNeed(s); err == nil && n.On == on && n.Via == "declared" {
			return true
		}
	}
	return false
}

// TestProbeNodeExt: a blackbox target ext-<name> probes moon ext:<name>, only when a needs row declares it.
func TestProbeNodeExt(t *testing.T) {
	p, err := Parse([]byte(extBase + "needs: {app:x: [\"declared:ext:github\"]}"))
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{
		"ext-github":    "ext:github",
		"ext-anthropic": "", // not declared
		"ext-":          "",
		"github":        "",
		"pve-a":         "host:a", // the other rules hold
	} {
		if got := p.ProbeNode(target); got != want {
			t.Errorf("ProbeNode(%q) = %q, want %q", target, got, want)
		}
	}
}

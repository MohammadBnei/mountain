package realms

import (
	"slices"
	"strings"
	"testing"
)

func loadAliases(t *testing.T) (*Aliases, *Realms, *Registry) {
	t.Helper()
	rl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	a, err := LoadAliases(rl, reg)
	if err != nil {
		t.Fatal(err)
	}
	return a, rl, reg
}

func ids(ms []Match) []string {
	var out []string
	for _, m := range ms {
		if !slices.Contains(out, m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

// TestResolve: the live table's names, the naming traps among them (door-hybrid-F.md stage 2).
func TestResolve(t *testing.T) {
	a, _, _ := loadAliases(t)
	for _, c := range []struct {
		name    string
		want    []string
		source  string // the source of the first match, if set
		status  string
		comment string
	}{
		{"guest-archive-h2", []string{"vm:pg01"}, "prom-cluster", Proven, "the -h2 suffix is the host, not pg02"},
		{"guest-archive-h1", []string{"vm:pg02"}, "prom-cluster", Proven, ""},
		{"ins=etcd-1", []string{"vm:pg02"}, "pigsty-vmetrics", Proven, "etcd-1 is on pg02"},
		{"etcd-1", []string{"vm:pg02"}, "", "", ""},
		{"etcd-2", []string{"vm:pg01"}, "", "", ""},
		{"pg-proxmox-1", []string{"pg:pg-proxmox-1"}, "patroni-rest", Proven, "the member, not the VM its ins also labels"},
		{"PG-Proxmox", []string{"db:pg-proxmox"}, "", "", "case is not a name"},
		{"vm:pg01", []string{"vm:pg01"}, "atlas", Proven, "an id is its own name"},
		{"192.168.1.232", []string{"endpoint:pg-proxmox/vip"}, "pigsty-vmetrics", Proven, "the VIP is an endpoint"},
		{"postgres.bnei.lan", []string{"endpoint:pg-proxmox/vip"}, "pihole", Declared, ""},
		{"postgres-1", []string{"vm:pg01"}, "repo-declared", Historical, ""},
		{"k8s-worker-02", []string{"vm:k8s-worker-02"}, "k8s-api", Proven, ""},
		{"192.168.1.204:9100", []string{"vm:k8s-cp-02"}, "prom-cluster", Proven, "cp-02 is .204"},
		{"pve-bnei", []string{"host:bnei"}, "prom-cluster", Proven, "pve-<host> from realms.yaml"},
		{"proxmox", []string{"endpoint:proxmox/proxmox", "host:bnei"}, "", "", "ambiguous: the old host name and the Service to server1"},
		{"ins=pg01", nil, "", "", "pg01 is a nodename, not an ins"},
		{"nothing-here", nil, "", "", ""},
	} {
		got := a.Resolve(c.name)
		if !slices.Equal(ids(got), c.want) {
			t.Errorf("%s: %v, want %v (%s)", c.name, ids(got), c.want, c.comment)
			continue
		}
		if c.source != "" && (got[0].Source != c.source || got[0].Status != c.status) {
			t.Errorf("%s: first match %+v, want source %s status %s", c.name, got[0], c.source, c.status)
		}
	}
	if r := a.RunsOn("vm:k8s-cp-02"); r != "host:server1" {
		t.Errorf("k8s-cp-02 runs on %q (k8sPlacement)", r)
	}
	if r := a.RunsOn("pg:pg-proxmox-2"); r != "vm:pg02" {
		t.Errorf("pg-proxmox-2 runs on %q", r)
	}
	if m := a.Members("db:pg-proxmox"); !slices.Equal(m, []string{"pg:pg-proxmox-1", "pg:pg-proxmox-2"}) {
		t.Errorf("members %v", m)
	}
	if n := a.Names("vm:pg01", "prom-cluster", "target"); !slices.Equal(n, []string{"guest-archive-h2"}) {
		t.Errorf("pg01's probe names %v", n)
	}
	if n := a.Names("vm:pg01", "repo-declared", ""); len(n) != 0 {
		t.Errorf("a historical name is a live name: %v", n)
	}
}

// TestAliasChecks: each load check refuses its case, from a good small table.
func TestAliasChecks(t *testing.T) {
	_, rl, reg := loadAliases(t)
	const ok = `things:
  vm:pg01:
    aliases:
      - {source: pigsty-vmetrics, field: nodename, name: pg01, status: proven, key: mac, at: 2026-11-14}
  endpoint:pg-proxmox/vip:
    vips: [192.168.1.232]
    aliases:
      - {source: pihole, field: A, name: postgres.bnei.lan, status: declared, key: ip, at: 2026-11-14}
`
	add := func(s string) string { return ok + s }
	if _, err := ParseAliases([]byte(ok), rl, reg); err != nil {
		t.Fatalf("a good table: %v", err)
	}
	if _, err := ParseAliases([]byte(add("  vm:pg02:\n    aliases:\n      - {source: repo-declared, field: former-name, name: postgres-2, status: historical, key: name}\n")), rl, reg); err != nil {
		t.Errorf("a historical row needs no date: %v", err)
	}
	for name, src := range map[string]string{
		"one name, two ids, one source": add("  vm:pg02:\n    aliases:\n      - {source: pigsty-vmetrics, field: ins, name: PG01, status: proven, key: mac, at: 2026-11-14}\n"),
		"a VIP as a VM's alias":         add("  vm:pg02:\n    aliases:\n      - {source: terraform-state, field: ipv4, name: 192.168.1.232, status: declared, key: mac, at: 2026-10-17}\n"),
		"a VIP with a port":             add("  vm:pg02:\n    aliases:\n      - {source: terraform-state, field: ipv4, name: \"192.168.1.232:5432\", status: declared, key: mac, at: 2026-10-17}\n"),
		"a declared join with no date":  strings.Replace(ok, ", at: 2026-11-14}\n  endpoint", "}\n  endpoint", 1),
		"a bad date":                    strings.Replace(ok, "2026-11-14", "yesterday", 1),
		"a status nobody knows":         strings.Replace(ok, "proven", "guess", 1),
		"a source not registered":       strings.Replace(ok, "pihole", "pi-hole", 1),
		"a bad id":                      strings.Replace(ok, "endpoint:pg-proxmox/vip", "vip:pg-proxmox", 1),
		"a VM realms.yaml doesn't list": strings.Replace(ok, "vm:pg01", "vm:pg09", 1),
		"vips on a VM":                  add("  vm:pg02:\n    vips: [192.168.1.9]\n    aliases: []\n"),
		"runs_on a k8s VM":              add("  vm:k8s-cp-01:\n    runs_on: host:bnei\n    aliases: []\n"),
		"runs_on nothing":               add("  pg:x:\n    runs_on: vm:nope\n    aliases: []\n"),
		"from probes with no probe":     add("  vm:k8s-cp-01:\n    aliases:\n      - {source: prom-cluster, field: target, from: probes, status: proven, key: ip, at: 2026-11-14}\n"),
		"a name and from":               add("  vm:pg02:\n    aliases:\n      - {source: prom-cluster, field: target, name: x, from: probes, status: proven, key: ip, at: 2026-11-14}\n"),
		"no join key":                   strings.Replace(ok, "key: mac, ", "", 1),
		"a field nobody reads":          strings.Replace(ok, "status: proven", "state: proven", 1),
		"a date after today":            strings.Replace(ok, "2026-11-14", "2999-01-01", 1),
		"a historical row's bad date":   add("  vm:pg02:\n    aliases:\n      - {source: repo-declared, field: former-name, name: postgres-2, status: historical, key: name, at: once}\n"),
		"two endpoints, one VIP":        add("  endpoint:other:\n    vips: [\"192.168.1.232/32\"]\n    aliases: []\n"),
		"a VIP as a URL":                add("  vm:pg02:\n    aliases:\n      - {source: prom-cluster, field: instance, name: \"http://192.168.1.232:8008/health\", status: proven, key: ip, at: 2026-11-14}\n"),
	} {
		if _, err := ParseAliases([]byte(src), rl, reg); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

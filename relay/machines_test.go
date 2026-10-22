package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func guest(target string, v float64) sample {
	return sample{L: map[string]string{"target": target, "instance": "192.168.1.199:3900"}, V: v}
}
func node(n string) sample {
	return sample{L: map[string]string{"node": n, "condition": "Ready"}, V: 1}
}

// machineRig drives machines.go and place() the way the prom poll does.
type machineRig struct {
	a  *App
	c  *Client
	o  *Out
	pl *placer
	mw *machineWatch
}

func newMachineRig(t *testing.T) *machineRig {
	a, c := testApp(t)
	return &machineRig{a, c, &Out{app: a, name: "prom"}, newPlacer(), newMachineWatch()}
}

func (r *machineRig) poll(guests []sample, guestsOK bool, ready []sample, now time.Time) []map[string]any {
	here := r.mw.watch(r.a, r.o, r.pl, guests, guestsOK, ready, now)
	r.pl.place(r.a, r.o, nil, here, allReady, now)
	return drain(r.c)
}

var allGuests = []sample{guest("guest-archive-h1", 1), guest("guest-archive-h2", 1), guest("guest-witness", 1),
	guest("guest-granary", 1), guest("guest-cellar", 1), guest("guest-forge", 1), guest("guest-mill", 1)}

func withGuest(target string, v float64) []sample {
	out := slices.Clone(allGuests)
	for i, s := range out {
		if s.L["target"] == target {
			out[i] = guest(target, v)
		}
	}
	return out
}

func without(target string) []sample {
	return slices.DeleteFunc(slices.Clone(allGuests), func(s sample) bool { return s.L["target"] == target })
}

// TestMachineConfig: the machine entries are complete, and a bad one is refused, naming the key only.
func TestMachineConfig(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k, a := range cfg.Apps {
		if a.Group == "machine" {
			keys = append(keys, k+"@"+a.Host)
		}
	}
	slices.Sort(keys)
	want := "[archive-h1@h1 archive-h2@h2 cellar@h2 forge@h2 granary@h1 hall-h1@h1 hall-h2@h2 keep-h1@h1 keep-h2@h2 keep-h3@h3 mill@h3 witness@h3]"
	if fmt.Sprint(keys) != want || len(cfg.probes) != 7 || len(cfg.byNode) != 5 {
		t.Fatalf("machines %v (probes %d, nodes %d)", keys, len(cfg.probes), len(cfg.byNode))
	}
	base := "nodes: {k8s-cp-01: h1}\napps:\n  x: "
	for _, bad := range []string{
		`{group: machine, building: keep, host: h1, name: secret-host}`,                                  // no source
		`{group: machine, building: keep, host: h1, name: secret-host, node: k8s-cp-01, probe: guest-x}`, // two sources
		`{group: machine, building: keep, host: h2, name: secret-host, node: k8s-cp-01}`,                 // node on another host
		`{group: machine, building: keep, host: h1, name: secret-host, probe: blackbox-x}`,               // not a guest probe
		`{group: machine, building: keep, host: h4, name: secret-host, probe: guest-x}`,                  // no such host
		`{group: machine, building: keep, host: h1, name: k9s-dashboard, probe: guest-x}`,                // never drawn
		`{group: machine, building: keep, host: h1, name: secret-host, probe: guest-k9s}`,                // never probed
		`{group: user, building: home, host: h1, name: secret-host}`,                                     // machines only
	} {
		_, err := loadConfig([]byte(base + bad + "\n"))
		if err == nil || strings.Contains(err.Error(), "secret-host") || strings.Contains(err.Error(), "guest-") {
			t.Errorf("%s: %v", bad, err)
		}
	}
	// granary is a machine now: no ArgoCD app and no workload, but its namespace still carries its traffic
	if cfg.appOfArgo("garage", "garage") != "" || cfg.appOfArgo("granary", "") != "" || cfg.appOfWork("garage", "garage-0") != "" ||
		cfg.appOfService("garage-s3-abc123@kubernetescrd") != "granary" {
		t.Error("granary: argo/workload must skip it, traffic must keep it")
	}
}

// TestMachinePresence: drawn once measured, on their own host; a 0 is present (not answering), absence for goneAfter
// ruins, a blind answer changes nothing, and a machine whose ruin was freed comes back.
func TestMachinePresence(t *testing.T) {
	r := newMachineRig(t)
	now := time.Unix(1_800_000_000, 0)
	vms := []sample{node("k8s-cp-01"), node("k8s-cp-02"), node("k8s-cp-03"), node("k8s-worker-01"), node("k8s-worker-02")}
	fr := r.poll(withGuest("guest-cellar", 0), true, vms, now)
	ps := plotsOf(fr)
	if len(ps) != 12 {
		t.Fatalf("12 machines placed, got %d: %v", len(ps), ps)
	}
	for _, p := range ps {
		app := p["app"].(string)
		if p["host"] != r.a.Cfg.Apps[app].Host || p["role"] != r.a.Cfg.Apps[app].Building {
			t.Errorf("%s on %v as %v", app, p["host"], p["role"])
		}
	}
	if len(evs(fr, "machine")) != 0 {
		t.Error("no state before it has held machineHold")
	}
	// blind: an empty or failed guests answer, an empty ready answer: nothing changes, even for an hour
	for i := 0; i < 8; i++ {
		now = now.Add(10 * time.Minute)
		if f := r.poll(nil, i%2 == 0, nil, now); len(plotsOf(f))+len(evs(f, "app.gone")) != 0 {
			t.Fatalf("blind poll %d changed the town: %v", i, f)
		}
	}
	// the forge's target disappears while the others answer: ruins after goneAfter, once
	t0 := now
	var gone []map[string]any
	for now = t0; now.Sub(t0) <= machineGone+time.Minute; now = now.Add(time.Minute) {
		gone = append(gone, evs(r.poll(without("guest-forge"), true, vms, now), "app.gone")...)
	}
	if p, _ := plotOf(r.a, "forge"); len(gone) != 1 || gone[0]["app"] != "forge" || p.RuinAt == 0 {
		t.Fatalf("forge: %v, plot %+v", gone, p)
	}
	for i := 0; i < 15; i++ {
		now = now.Add(time.Minute)
		if f := r.poll(without("guest-forge"), true, vms, now); len(evs(f, "app.gone")) != 0 {
			t.Fatal("a second app.gone for a machine already in ruins")
		}
	}
	// a VM leaves the Ready answer: ruins too
	for t1 := now; now.Sub(t1) <= machineGone; now = now.Add(time.Minute) {
		r.poll(without("guest-forge"), true, vms[1:], now)
	}
	if p, _ := plotOf(r.a, "keep-h1"); p.RuinAt == 0 {
		t.Error("keep-h1 out of the Ready answer must be in ruins")
	}
	// past ruinFor the forge's plot is freed; back on the probe list, it is placed again on h2
	now = now.Add(ruinFor)
	r.poll(without("guest-forge"), true, vms, now)
	if _, has := plotOf(r.a, "forge"); has {
		t.Fatal("forge's ruin must be freed after ruinFor")
	}
	r.poll(allGuests, true, vms, now.Add(time.Minute))
	if p, has := plotOf(r.a, "forge"); !has || p.Host != "h2" || p.RuinAt != 0 {
		t.Fatalf("forge back: %+v %v", p, has)
	}
}

// TestMachineState: up/down only after machineHold, a flap starts over, a restored state stands, and a restart with
// none never publishes early.
func TestMachineState(t *testing.T) {
	r := newMachineRig(t)
	now := time.Unix(1_800_000_000, 0)
	state := func(f []map[string]any) (out []string) {
		for _, e := range evs(f, "machine") {
			out = append(out, e["app"].(string)+"="+e["state"].(string))
		}
		slices.Sort(out)
		return
	}
	var got []string
	for t0 := now; now.Sub(t0) < machineHold; now = now.Add(time.Minute) {
		got = append(got, state(r.poll(withGuest("guest-witness", 0), true, nil, now))...)
	}
	if len(got) != 0 {
		t.Fatalf("published before machineHold: %v", got)
	}
	got = state(r.poll(withGuest("guest-witness", 0), true, nil, now))
	if len(got) != 7 || !slices.Contains(got, "witness=down") || !slices.Contains(got, "mill=up") {
		t.Fatalf("after machineHold: %v", got)
	}
	r.a.mu.Lock()
	if r.a.State.Machines["witness"] != "down" {
		t.Errorf("state not folded: %v", r.a.State.Machines)
	}
	r.a.mu.Unlock()
	// a flap shorter than machineHold never shows
	for i := 0; i < 20; i++ {
		now = now.Add(time.Minute)
		v := withGuest("guest-witness", float64(i%2))
		if g := state(r.poll(v, true, nil, now)); len(g) != 0 {
			t.Fatalf("a flap published %v", g)
		}
	}
	// a restart: the restored state stands (no event), a difference waits machineHold again
	r2 := &machineRig{r.a, r.c, r.o, newPlacer(), newMachineWatch()}
	if g := state(r2.poll(withGuest("guest-witness", 0), true, nil, now)); len(g) != 0 {
		t.Fatalf("restored state re-sent: %v", g)
	}
	now = now.Add(time.Minute)
	if g := state(r2.poll(allGuests, true, nil, now)); len(g) != 0 {
		t.Fatalf("a change after a restart published at once: %v", g)
	}
	// a memory-only boot (no state at all): nothing until machineHold
	r.a.mu.Lock()
	r.a.State.Machines = map[string]string{}
	r.a.mu.Unlock()
	r3 := &machineRig{r.a, r.c, r.o, newPlacer(), newMachineWatch()}
	if g := state(r3.poll(allGuests, true, nil, now)); len(g) != 0 {
		t.Fatalf("a memory-only boot published at once: %v", g)
	}
	// app.gone clears a machine's state (Go fold)
	s := newState()
	s.apply(frameF{T: "ev", Kind: "machine", App: "mill", State: "up"}, nil)
	s.apply(frameF{T: "ev", Kind: "app.gone", App: "mill"}, nil)
	if len(s.Machines) != 0 {
		t.Errorf("app.gone must clear the machine's state: %v", s.Machines)
	}
}

// TestMachineUnmapped: an unknown guest-* target counts once; the break-glass box never; a blind answer never.
func TestMachineUnmapped(t *testing.T) {
	r := newMachineRig(t)
	before := Unmapped.v["probe"]
	now := time.Unix(1_800_000_000, 0)
	r.poll(append(slices.Clone(allGuests), guest("guest-new-box", 1), guest("guest-k9s-dashboard", 1)), true, nil, now)
	r.poll(nil, true, nil, now.Add(time.Hour))
	if d := Unmapped.v["probe"] - before; d != 1 {
		t.Errorf("unmapped probes counted %v, want 1 (guest-new-box)", d)
	}
	for _, f := range drain(r.c) {
		if b, _ := json.Marshal(f); strings.Contains(strings.ToLower(string(b)), "k9s") {
			t.Fatal("k9s reached the wire")
		}
	}
}

// TestMachineNames: /auth/me carries the real names to an admin only, private and uncacheable.
func TestMachineNames(t *testing.T) {
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })
	me := func(c *http.Cookie) (map[string]any, http.Header) {
		var cs []*http.Cookie
		if c != nil {
			cs = []*http.Cookie{c}
		}
		resp := serveAuth("GET", "/auth/me", cs)
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return m, resp.Header
	}
	m, h := me(session(t, p, "platform-admins"))
	names, _ := m["names"].(map[string]any)
	if names["witness"] != "pg-etcd-witness" || names["keep-h3"] != "k8s-cp-03" || len(names) != 12 {
		t.Fatalf("admin names: %v", m["names"])
	}
	if h.Get("Cache-Control") != "private, no-store" || h.Get("Vary") != "Cookie" || h.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("headers: %v", h)
	}
	for _, c := range []*http.Cookie{session(t, p, "mountain-visitors"), session(t, p), nil} {
		if m, _ := me(c); m["names"] != nil {
			t.Errorf("names leaked to role %v", m["role"])
		}
	}
}

// machineNameWords: every machine's real name and source, from config.yaml (never hardcoded in www/).
func machineNameWords(t *testing.T) []string {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range cfg.Apps {
		if a.Group == "machine" {
			out = append(out, strings.ToLower(a.Name), strings.ToLower(a.Probe), strings.ToLower(a.Node))
		}
	}
	return slices.DeleteFunc(out, func(s string) bool { return s == "" })
}

// TestDBMachineNames: machines placed and stated, a snapshot and a restart later, no row the relay wrote and no hello
// carries a real name, probe target or node of a machine.
func TestDBMachineNames(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	r := &machineRig{a, &Client{send: make(chan []byte, 1<<14)}, &Out{app: a, name: "prom"}, newPlacer(), newMachineWatch()}
	a.Hub.clients[r.c] = struct{}{}
	now := time.Now()
	vms := []sample{node("k8s-cp-01"), node("k8s-worker-01")}
	for i := 0; i <= 11; i++ {
		r.poll(withGuest("guest-cellar", 0), true, vms, now.Add(time.Duration(i)*time.Minute))
	}
	a.snapshot(now.Add(12 * time.Minute))
	a.store.Close(5 * time.Second)
	b := storedApp(t, dsn)
	defer b.store.Close(time.Second)
	if b.State.Machines["cellar"] != "down" || len(b.State.Plots) != 9 {
		t.Fatalf("restored: machines %v, %d plots", b.State.Machines, len(b.State.Plots))
	}
	var rows string
	if err := b.store.pool.QueryRow(context.Background(), `SELECT
		(SELECT coalesce(string_agg(public::text, ' '), '') FROM events) || (SELECT coalesce(string_agg(frame::text, ' '), '') FROM world_deltas) ||
		(SELECT coalesce(string_agg(state::text, ' '), '') FROM snapshots)`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	all := strings.ToLower(rows + string(b.Hello()))
	if !strings.Contains(all, `"cellar"`) {
		t.Fatal("the scan saw no machine at all")
	}
	for _, w := range machineNameWords(t) {
		if strings.Contains(all, w) {
			t.Errorf("%q reached the DB or the hello", w)
		}
	}
}

// TestBundleNoMachineNames: the public bundle (apps.json, the page's JS and HTML) never names a machine's real host,
// probe or node, nor the machine words the wire never carries.
func TestBundleNoMachineNames(t *testing.T) {
	files, _ := filepath.Glob("../www/js/*.js")
	more, _ := filepath.Glob("../www/js/objects/*.js")
	files = append(append(files, more...), "../www/index.html", "../www/apps.json")
	words := append(machineNameWords(t), "guest-", "k9s", "patroni", "etcd", "blackbox", "probe_success", "hermes", "pg0", "192.168", "bnei.lan")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ToLower(string(b))
		for _, w := range words {
			if strings.Contains(s, w) {
				t.Errorf("%s contains %q", f, w)
			}
		}
	}
	var apps map[string]map[string]any
	b, _ := os.ReadFile("../www/apps.json")
	if err := json.Unmarshal(b, &apps); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadConfig(configYAML)
	for k, a := range cfg.Apps {
		e := apps[k]
		if e == nil || e["group"] != a.Group || (a.Group == "machine" && (e["url"] != nil || (e["probed"] == true) != (a.Probe != ""))) {
			t.Errorf("apps.json %s: %v (config group %s)", k, e, a.Group)
		}
	}
}

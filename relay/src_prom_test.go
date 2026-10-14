package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// testApp: an App whose hub has one in-memory client collecting every broadcast.
func testApp(t *testing.T) (*App, *Client) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(cfg, Opts{Seed: 1}, "2026-10-14")
	liveApps = map[string]bool{}
	c := &Client{send: make(chan []byte, 1<<14)}
	a.Hub.clients[c] = struct{}{}
	return a, c
}

func drain(c *Client) (out []map[string]any) {
	for {
		select {
		case b := <-c.send:
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				panic(err)
			}
			out = append(out, m)
		default:
			return
		}
	}
}

// run polls f step by step (15 s apart), then holds the last step for extra polls.
func run(t *testing.T, a *App, c *Client, f *fixture, extra int) [][]map[string]any {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	u, err := serveFixture(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	a.Opts.PromURL = u
	p, out, now := newProm(a), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000, 0)
	var steps [][]map[string]any
	for i := 0; i < len(f.prom)+extra; i++ {
		f.step.Store(int64(min(i, len(f.prom)-1)))
		if err := p.poll(ctx, out, now); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, drain(c))
		now = now.Add(15 * time.Second)
	}
	return steps
}

func evs(frames []map[string]any, kind string) (out []map[string]any) {
	for _, f := range frames {
		if f["t"] == "ev" && f["kind"] == kind {
			out = append(out, f)
		}
	}
	return
}

func TestCounterDiff(t *testing.T) {
	p := newProm(&App{})
	for i, c := range []struct{ v, want float64 }{{5, 0}, {7, 2}, {7, 0}, {3, 3}, {4, 1}} {
		if got := p.diff("s", c.v); got != c.want {
			t.Fatalf("step %d: diff(%v) = %v, want %v", i, c.v, got, c.want)
		}
		p.warm = true
	}
	if got := p.diff("new-series", 1); got != 0 {
		t.Fatalf("a series first seen after warm-up is a baseline: got %v", got)
	}
}

// TestExporterReschedule: kube-state-metrics moves (new instance/pod IP): its restarts series keep
// their workload labels and values, so nothing churns.
func TestExporterReschedule(t *testing.T) {
	a, c := testApp(t)
	r := func(inst string) []fixtureSeries {
		return []fixtureSeries{{M: map[string]string{"namespace": "wird", "pod": "wird-abc-1", "container": "app", "instance": inst + ":8080",
			"job": "kube-state-metrics", "endpoint": "http", "service": "platform-prometheus-kube-state-metrics"}, V: 7}}
	}
	f := &fixture{prom: []map[string][]fixtureSeries{{"restarts": r("10.0.0.1")}, {"restarts": r("10.0.0.1")}, {"restarts": r("10.0.0.2")}}}
	for i, st := range run(t, a, c, f, 0) {
		if e := evs(st, "pod.churn"); len(e) != 0 {
			t.Fatalf("poll %d: false pod.churn after an exporter reschedule: %v", i, e)
		}
	}
}

func TestOOMRule(t *testing.T) {
	a, c := testApp(t)
	r := func(n float64) fixtureSeries {
		return fixtureSeries{M: map[string]string{"namespace": "wird", "pod": "wird-abc-1", "container": "app"}, V: n}
	}
	oom := []fixtureSeries{{M: map[string]string{"namespace": "wird", "pod": "wird-abc-1", "container": "app"}, V: 1}}
	f := &fixture{prom: []map[string][]fixtureSeries{
		{"restarts": {r(0)}, "oom": oom}, // baseline (an old OOM)
		{"restarts": {r(0)}, "oom": oom}, // reason=OOMKilled but no restart: nothing
		{"restarts": {r(1)}, "oom": oom}, // restart + OOMKilled: oom
		{"restarts": {r(2)}},             // restart, last reason not OOM: churn, no oom
	}}
	steps := run(t, a, c, f, 0)
	got := []string{}
	for _, s := range steps {
		for _, e := range evs(s, "pod.churn") {
			got = append(got, fmt.Sprintf("%v/%v", e["restarts"], e["oom"]))
		}
		got = append(got, "|")
	}
	if want := "|,|,1/true,|,1/false,|"; strings.Join(got, ",") != want {
		t.Fatalf("OOM rule: got %s, want %s", strings.Join(got, ","), want)
	}
}

var (
	ipRE      = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	versionRE = regexp.MustCompile(`v\d+\.\d+|\d+\.\d+\.\d+-`)
	hostile   = []string{"k8s-", "bnei.lan", "evil", "pve-", "kube-system", "monitoring", "agent-fleet", "has-journal", `"hq"`, "adminweb",
		"coredns", "argocd", "6443", "ghcr", "uid", "garage-", "nfs", "prometheus", "grafana", "wird-pg", "default", "namespace", `"pod"`, "kubernetescrd"}
)

// TestFixtureTimeline replays www/dev/fixtures/relay-hostile.json: the W1a events come out,
// nothing hostile leaks, and every frame validates against www/dev/schema.
func TestFixtureTimeline(t *testing.T) {
	a, c := testApp(t)
	f, err := loadFixture("../www/dev/fixtures/relay-hostile.json")
	if err != nil {
		t.Fatal(err)
	}
	steps := run(t, a, c, f, 41) // + 10 min 15 s on the last step: app.gone
	var all []map[string]any
	for _, s := range steps {
		all = append(all, s...)
	}
	one := func(step int, kind string, want map[string]any) map[string]any {
		t.Helper()
		var hit []map[string]any
	next:
		for _, e := range evs(steps[step], kind) {
			for k, v := range want {
				if fmt.Sprint(e[k]) != fmt.Sprint(v) {
					continue next
				}
			}
			hit = append(hit, e)
		}
		if len(hit) != 1 {
			t.Errorf("step %d: want one %s %v, got %v", step, kind, want, evs(steps[step], kind))
			return nil
		}
		return hit[0]
	}
	if len(evs(steps[0], "argo.sync")) != 0 || len(evs(steps[0], "pod.churn")) != 0 {
		t.Error("the first poll must only set baselines")
	}
	if s := evs(steps[1], "argo.sync"); len(s) != 1 {
		t.Errorf("step 1: only the allowlisted sync may pass, got %v", s)
	}
	if sync := one(1, "argo.sync", map[string]any{"app": "wird", "ok": true}); sync != nil {
		var dep, site bool
		for _, w := range steps[1] {
			dep = dep || w["t"] == "world" && w["kind"] == "deposit" && w["id"] == sync["id"]
			site = site || w["t"] == "world" && w["kind"] == "site" && w["app"] == "wird"
		}
		if !dep || !site {
			t.Errorf("wird sync needs world:deposit (id = ev id) and world:site: deposit=%v site=%v", dep, site)
		}
	}
	one(1, "pod.churn", map[string]any{"app": "editable-blog", "restarts": 2, "oom": true, "crashloop": false})
	if n := len(evs(steps[1], "pod.churn")); n != 1 {
		t.Errorf("step 1: only editable-blog churns publicly, got %d", n)
	}
	one(2, "pod.churn", map[string]any{"app": "dream-analyst", "restarts": 0, "oom": false, "crashloop": true})
	one(2, "argo.sync", map[string]any{"app": "vos-monolith", "ok": false})
	for _, w := range steps[2] {
		if w["t"] == "world" && w["kind"] == "deposit" {
			t.Error("a failed sync must not deposit land")
		}
	}
	one(2, "node.ready", map[string]any{"host": "h3", "ready": false})
	one(2, "replicas", map[string]any{"app": "vos-monolith", "bucket": 5})
	one(2, "argo.health", map[string]any{"app": "wird", "status": "Degraded"})
	one(3, "quorum", map[string]any{"up": []any{"h1", "h2"}})
	// dream-analyst is absent from step 3 on: app.gone exactly 10 min (40 polls) later, once
	if gone := evs(all, "app.gone"); len(gone) != 1 || len(evs(steps[3+40], "app.gone")) != 1 {
		t.Errorf("app.gone: want exactly one, at step 43: %v", gone)
	}
	one(3+40, "app.gone", map[string]any{"app": "dream-analyst"})

	// metrics: Traefik reset handled, 5xx only, storage pools by allowlist
	var m map[string]any
	for _, x := range steps[2] {
		if x["t"] == "metrics" {
			m = x
		}
	}
	if w := m["apps"].(map[string]any)["wird"].(map[string]any); w["rpsB"].(float64) < 1 {
		t.Errorf("wird rps after a counter reset: %v", w)
	}
	pools := map[string]map[string]any{}
	for _, p := range m["storage"].(map[string]any)["pools"].([]any) {
		pools[p.(map[string]any)["id"].(string)] = p.(map[string]any)
	}
	for id, want := range map[string]string{"wird-1": "longhorn/18", "ukubi-stt-1": "local/<nil>", "other-longhorn": "longhorn/4", "other-local-h1": "local/<nil>", "other-local-h2": "local/<nil>"} {
		if p := pools[id]; p == nil || fmt.Sprintf("%v/%v", p["class"], p["usedB"]) != want {
			t.Errorf("pool %s: got %v, want %s", id, p, want)
		}
	}
	if len(pools) != 5 {
		t.Errorf("pools: %v", pools)
	}

	// /api/topology: live apps only (never-seen and gone apps have no site)
	app = a
	rec := httptest.NewRecorder()
	topology(rec, httptest.NewRequest("GET", "/api/topology", nil))
	var topo struct{ Sites []struct{ App string } }
	json.Unmarshal(rec.Body.Bytes(), &topo)
	var names []string
	for _, x := range topo.Sites {
		names = append(names, x.App)
	}
	if got := strings.Join(names, ","); got != "editable-blog,mountain,vos-monolith,wird" {
		t.Errorf("topology sites: %s", got)
	}

	validate(t, steps) // before the leak scan, which strips cells

	// nothing hostile leaves the relay
	for _, fr := range all {
		delete(fr, "cells") // base64 can spell anything
		b, _ := json.Marshal(fr)
		s := string(b)
		if ipRE.MatchString(s) || versionRE.MatchString(s) {
			t.Errorf("IP/version leak: %s", s)
		}
		for _, h := range hostile {
			if strings.Contains(s, h) {
				t.Errorf("leak %q: %.200s", h, s)
			}
		}
	}
}

// validate runs www/dev/schema/check.mjs over every frame (Go and JS share one schema checker).
func validate(t *testing.T, steps [][]map[string]any) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed: schema check skipped")
	}
	dir, args := t.TempDir(), []string{"../www/dev/schema/check.mjs"}
	n := 0
	for _, s := range steps {
		for _, f := range s {
			p := filepath.Join(dir, fmt.Sprintf("f%03d.json", n))
			n++
			b, _ := json.Marshal(f)
			os.WriteFile(p, b, 0o644)
			args = append(args, p)
		}
	}
	out, err := exec.Command(node, args...).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "SCHEMA PASS") {
		t.Fatalf("schema check (%d frames): %v\n%s", n, err, out)
	}
	t.Logf("schema: %d frames, SCHEMA PASS", n)
}

// TestDepositHubLock: the land must not change while the hub lock is held (a client registering
// then would get the deposit in its hello AND as a frame).
func TestDepositHubLock(t *testing.T) {
	a, c := testApp(t)
	p := newProm(a)
	a.Hub.mu.Lock()
	done := make(chan struct{})
	go func() { p.deposit(&Out{app: a}, "wird", 1); close(done) }()
	time.Sleep(50 * time.Millisecond)
	a.mu.Lock()
	changed := slices.ContainsFunc(a.Land, func(v int16) bool { return v != 0 })
	a.mu.Unlock()
	a.Hub.mu.Unlock()
	<-done
	if changed {
		t.Fatal("deposit changed h.land outside the hub lock")
	}
	if n := len(drain(c)); n != 3 {
		t.Fatalf("want ev:argo.sync + world:deposit + world:site, got %d frames", n)
	}
}

// TestSyncDepositAtomic: the argo.sync ev leaves in the same hub-lock section as its world:deposit, so a
// client registering between them cannot get the deposit without its ev. While the deposit waits on
// the app lock, no frame of the sync may be out yet.
func TestSyncDepositAtomic(t *testing.T) {
	a, c := testApp(t)
	f := &fixture{prom: []map[string][]fixtureSeries{
		{"sync": {{M: map[string]string{"name": "wird", "phase": "Succeeded"}, V: 1}}},
		{"sync": {{M: map[string]string{"name": "wird", "phase": "Succeeded"}, V: 2}}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u, err := serveFixture(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	a.Opts.PromURL = u
	p, out := newProm(a), &Out{app: a, name: "prom"}
	if err := p.poll(ctx, out, time.Now()); err != nil {
		t.Fatal(err)
	}
	drain(c)
	f.step.Store(1)
	a.mu.Lock()
	done := make(chan error)
	go func() { done <- p.poll(ctx, out, time.Now()) }()
	time.Sleep(100 * time.Millisecond)
	early := drain(c)
	a.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(early) != 0 {
		t.Fatalf("frames out before the deposit's critical section: %v", early)
	}
	got := drain(c)
	if len(got) < 3 || got[0]["kind"] != "argo.sync" || got[1]["kind"] != "deposit" || got[0]["id"] != got[1]["id"] {
		t.Fatalf("want ev then its deposit: %v", got)
	}
}

func TestCounterPrune(t *testing.T) {
	p := newProm(&App{})
	p.diff("gone", 5)
	for range staleAfter + 1 {
		p.diff("live", 1)
		p.prune(map[string][]sample{"gone": {{}}, "live": {{}}})
	}
	if _, ok := p.ctr["gone"]; ok || len(p.ctr) != 1 {
		t.Fatalf("stale series must be pruned, live kept: %v", p.ctr)
	}
}

// TestCounterOutage: a sync series hidden behind empty answers for staleAfter polls (scrape down,
// Prometheus up) comes back unchanged: diff 0, no argo.sync.
func TestCounterOutage(t *testing.T) {
	a, c := testApp(t)
	s := []fixtureSeries{{M: map[string]string{"name": "wird", "phase": "Succeeded"}, V: 7}}
	f := &fixture{prom: []map[string][]fixtureSeries{{"sync": s}}}
	for range staleAfter + 1 {
		f.prom = append(f.prom, map[string][]fixtureSeries{})
	}
	f.prom = append(f.prom, map[string][]fixtureSeries{"sync": s})
	for i, st := range run(t, a, c, f, 0) {
		if e := evs(st, "argo.sync"); len(e) != 0 {
			t.Fatalf("poll %d: false argo.sync after outage: %v", i, e)
		}
	}
}

// TestPoolsOrder: the 16 cap drops other-* aggregates before any allowlisted pool.
func TestPoolsOrder(t *testing.T) {
	a, _ := testApp(t)
	pvc := func(ns, name, class string) sample {
		return sample{L: map[string]string{"namespace": ns, "persistentvolumeclaim": name, "storageclass": class}, V: 1}
	}
	r := map[string][]sample{"pvcnode": {{L: map[string]string{"namespace": "kube-system", "persistentvolumeclaim": "x", "node": "k8s-cp-01"}}}}
	r["pvc"] = []sample{pvc("kube-system", "x", "local-path"), pvc("monitoring", "y", "longhorn")}
	for i := range 15 {
		r["pvc"] = append(r["pvc"], pvc("wird", fmt.Sprint("d", i), "longhorn"))
	}
	var ids []string
	for _, pl := range newProm(a).pools(r) {
		ids = append(ids, pl.ID)
	}
	if len(ids) != 16 || ids[0] != "wird-1" || ids[15] != "other-local-h1" {
		t.Fatalf("pools: %v", ids)
	}
}

// TestFixtureOneClock: with step_s far shorter than a poll, every one-step crashloop 0 -> 1 is seen.
func TestFixtureOneClock(t *testing.T) {
	a, c := testApp(t)
	every, after := promEvery, afterPoll
	t.Cleanup(func() { promEvery, afterPoll = every, after })
	crash := `{"crash": [{"m": {"namespace": "dream-analyst", "pod": "dream-analyst-1", "container": "app"}, "v": 1}]}`
	none := `{"crash": []}`
	file := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(file, []byte(`{"step_s": 0.001, "steps": [{"prom": `+strings.Join([]string{none, crash, none, crash, none, crash}, `}, {"prom": `)+`}]}`), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer func() { cancel(); <-done }() // Run must stop before Cleanup restores promEvery
	u, _, err := FixtureHook(ctx, file)
	if err != nil {
		close(done)
		t.Fatal(err)
	}
	a.Opts.PromURL = u
	go func() { newProm(a).Run(ctx, &Out{app: a, name: "prom"}); close(done) }()
	var got []map[string]any
	for deadline := time.Now().Add(3 * time.Second); len(got) < 3 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		got = append(got, evs(drain(c), "pod.churn")...)
	}
	time.Sleep(50 * time.Millisecond)
	got = append(got, evs(drain(c), "pod.churn")...)
	if len(got) != 3 {
		t.Fatalf("want 3 crashloop events (steps 1, 3, 5), got %d: %v", len(got), got)
	}
}

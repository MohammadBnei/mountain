package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// ponytail: config.yaml (seam) lacks the PVE probe targets; reported to the orchestrator.
	cfg.Nodes["pve-bnei"], cfg.Nodes["pve-server1"], cfg.Nodes["pve-ex-laptop"] = "h1", "h2", "h3"
	a := newApp(cfg, Opts{Seed: 1}, "2026-10-14")
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
	if got := p.diff("new-series", 1); got != 1 {
		t.Fatalf("a series first seen after warm-up counts from 0: got %v", got)
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

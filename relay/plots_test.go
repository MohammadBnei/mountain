package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/town"
	"gopkg.in/yaml.v3"
)

func pod(ns, name, node string) sample {
	return sample{L: map[string]string{"namespace": ns, "pod": name, "node": node, "uid": "x", "host_ip": "10.0.3.17"}, V: 1}
}

func plotsOf(frames []map[string]any) (out []map[string]any) {
	for _, f := range frames {
		if f["t"] == "world" && f["kind"] == "plot" {
			out = append(out, f)
		}
	}
	return
}

func plotOf(a *App, app string) (Plot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := slices.IndexFunc(a.State.Plots, func(p Plot) bool { return p.App == app })
	if i < 0 {
		return Plot{}, false
	}
	return a.State.Plots[i], true
}

var allReady = map[string]bool{"h1": true, "h2": true, "h3": true}

// TestPlotPlacement: plots land on the host with most pods, by role; private namespaces get none; nothing but the
// plot fields reaches the wire; a move waits moveAfter, never leaves a NotReady host; an empty answer keeps all.
func TestPlotPlacement(t *testing.T) {
	a, c := testApp(t)
	pl, o, now := newPlacer(), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000, 0)
	wird := []sample{pod("wird", "wird-7f9c-a", "k8s-worker-01"), pod("wird", "wird-7f9c-b", "k8s-cp-01"), pod("wird", "wird-7f9c-c", "k8s-worker-02")}
	pods := append(slices.Clone(wird),
		pod("traefik", "traefik-5d8-x", "k8s-worker-02"),
		pod("searxng", "searxng-1", "k8s-worker-01"),          // private: no plot
		pod("wird", "wird-adminweb-55-z", "k8s-cp-03"),        // private workload in a public namespace
		pod("monitoring", "prometheus-0", "k8s-worker-9.lan")) // unmapped node: no host, no plot
	pl.place(a, o, pods, allReady, now)
	frames := drain(c)
	ps := plotsOf(frames)
	if len(ps) != 2 {
		t.Fatalf("plots: %v", ps)
	}
	w, _ := plotOf(a, "wird")
	g, _ := plotOf(a, "gate")
	// R8: wird is a shop (inn), on the street front; the gate is Defense, at the town's edge (ring r−2)
	if w.Host != "h1" || w.Role != "inn" || g.Host != "h2" || g.Role != "gate" || (town.Hex{Q: g.Q, R: g.R}).Ring() != town.Radii["h2"]-2 {
		t.Fatalf("wird %+v gate %+v", w, g)
	}
	for _, f := range frames {
		b := string(mustJSON(f))
		for _, leak := range []string{"traefik", "authentik", "argocd", "longhorn", "infisical", "garage", "zot", "pigsty", "grafana",
			"searxng", "prometheus", "monitoring", "k8s-", "10.0.3", "wird-7f9c", "adminweb"} {
			if strings.Contains(b, leak) {
				t.Errorf("%q leaked: %s", leak, b)
			}
		}
	}
	validate(t, [][]map[string]any{frames})

	// a rolling restart: the majority flips to h2 for under 5 min, then back. Nothing moves.
	h2 := []sample{pod("wird", "wird-8a-a", "k8s-worker-02"), pod("wird", "wird-8a-b", "k8s-cp-02")}
	for i := range 19 {
		pl.place(a, o, h2, allReady, now.Add(time.Duration(i+1)*15*time.Second))
	}
	pl.place(a, o, wird, allReady, now.Add(20*15*time.Second))
	if ps := plotsOf(drain(c)); len(ps) != 0 {
		t.Fatalf("a short flip moved the building: %v", ps)
	}
	// a real move: h2 leads for 5 min
	t0 := now.Add(time.Hour)
	for i := range 22 {
		pl.place(a, o, h2, allReady, t0.Add(time.Duration(i)*15*time.Second))
	}
	if w2, _ := plotOf(a, "wird"); w2.Host != "h2" {
		t.Fatalf("wird after 5 min on h2: %+v", w2)
	}
	if ps := plotsOf(drain(c)); len(ps) != 1 {
		t.Fatalf("one move, one frame: %v", ps)
	}
	// h2 goes NotReady and its pods are rescheduled onto h3: the building stays on the dark island
	h3 := []sample{pod("wird", "wird-9-a", "k8s-cp-03")}
	down := map[string]bool{"h1": true, "h2": false, "h3": true}
	for i := range 30 {
		pl.place(a, o, h3, down, t0.Add(time.Hour+time.Duration(i)*15*time.Second))
	}
	if w3, _ := plotOf(a, "wird"); w3.Host != "h2" {
		t.Fatalf("moved off a NotReady host: %+v", w3)
	}
	// an empty answer (KSM restarting) keeps every plot
	pl.place(a, o, nil, allReady, t0.Add(3*time.Hour))
	if ps := plotsOf(drain(c)); len(ps) != 0 {
		t.Fatalf("empty answer emitted %v", ps)
	}
}

// TestPlotRuins: app.gone → ruins; back in ArgoCD → rebuilt; ruins older than ruinFor → plot freed.
func TestPlotRuins(t *testing.T) {
	a, c := testApp(t)
	pl, o, now := newPlacer(), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000, 0)
	pl.place(a, o, []sample{pod("wird", "wird-1", "k8s-cp-01")}, allReady, now)
	pl.ruin(a, o, "wird", true, now)
	pl.ruin(a, o, "wird", true, now) // idempotent
	if p, _ := plotOf(a, "wird"); p.RuinAt != now.UnixMilli() {
		t.Fatalf("ruins: %+v", p)
	}
	pl.ruin(a, o, "wird", false, now.Add(time.Minute))
	if p, _ := plotOf(a, "wird"); p.RuinAt != 0 {
		t.Fatalf("rebuilt: %+v", p)
	}
	pl.ruin(a, o, "wird", true, now)
	pl.place(a, o, nil, allReady, now.Add(ruinFor))
	if _, ok := plotOf(a, "wird"); ok {
		t.Fatal("old ruins must free the plot")
	}
	frames := drain(c)
	if ps := plotsOf(frames); len(ps) != 5 || ps[4]["gone"] != true {
		t.Fatalf("frames: %v", ps)
	}
	validate(t, [][]map[string]any{frames})
}

// TestPlant: the plant toy's rules, the sun's item, and a building evicting a plant only when the island is full.
func TestPlant(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	defer func(f func() time.Time) { sunNow = f }(sunNow)
	noon, _ := time.Parse(time.RFC3339, "2026-10-22T12:00:00Z")
	dusk, _ := time.Parse(time.RFC3339, "2026-10-22T16:30:00Z")
	night, _ := time.Parse(time.RFC3339, "2026-12-21T23:00:00Z")
	pl, o := newPlacer(), &Out{app: a, name: "prom"}
	pl.place(a, o, []sample{pod("wird", "wird-1", "k8s-cp-01")}, allReady, time.Now())
	w, _ := plotOf(a, "wird")
	drain(c)

	n := 0
	try := func(at time.Time, pose map[string]any) (string, string) {
		sunNow = func() time.Time { return at }
		n++
		id, why := ackOf(t, a.place(visitor(fmt.Sprint("s", n), "ana"), "r", "plant", pose, false, time.Now()))
		return id, why
	}
	for why, pose := range map[string]map[string]any{
		"bad":   {"h": 1.0, "q": 1.5, "r": 0.0},
		"reach": {"h": 4.0, "q": 0.0, "r": 0.0},
		"road":  {"h": 1.0, "q": 0.0, "r": 0.0},
		"plot":  {"h": 1.0, "q": float64(w.Q), "r": float64(w.R)},
	} {
		if _, got := try(noon, pose); got != why {
			t.Errorf("pose %v: %q, want %q", pose, got, why)
		}
	}
	if _, got := try(noon, map[string]any{"h": 1.0, "q": 9.0, "r": 0.0}); got != "reach" {
		t.Errorf("off the island: %q", got)
	}
	items := map[time.Time]float64{noon: itemTree, dusk: itemFlag, night: itemLantern}
	i := 0
	for at, item := range items {
		id, why := try(at, map[string]any{"h": 2.0, "q": 4.0, "r": float64(-i)}) // meadow hexes on h2
		if why != "" {
			t.Fatalf("plant at %v: %s", at, why)
		}
		ms := marksOf(drain(c))
		if len(ms) != 1 || ms[0]["id"] != id || ms[0]["pose"].(map[string]any)["item"] != item {
			t.Errorf("at %v: %v", at, ms)
		}
		i++
	}
	if _, why := try(noon, map[string]any{"h": 2.0, "q": 4.0, "r": 0.0}); why != "taken" {
		t.Errorf("second plant on a hex: %q", why)
	}

	// fill every free grass hex of h3 with plants, then an app arrives there: the oldest plant on its slot gives way
	for _, x := range town.Hexes("h3") {
		if town.Tile("h3", x) == town.Grass {
			if _, why := try(noon, map[string]any{"h": 3.0, "q": float64(x.Q), "r": float64(x.R)}); why != "" {
				t.Fatalf("fill h3 %v: %s", x, why)
			}
		}
	}
	drain(c)
	pl.place(a, o, []sample{pod("dream-analyst", "dream-analyst-1", "k8s-cp-03")}, allReady, time.Now())
	frames := drain(c)
	ms, ps := marksOf(frames), plotsOf(frames)
	if len(ms) != 1 || ms[0]["gone"] != true || len(ps) != 1 || ps[0]["host"] != "h3" {
		t.Fatalf("eviction: marks %v plots %v", ms, ps)
	}
	validate(t, [][]map[string]any{frames})
}

// TestPodsQuery runs promQ "pods" through promtool (relay/testdata/pods.test.yaml): duplicate KSM series, a pod
// being deleted, a Pending one and an unscheduled one. Skipped without promtool, fatal in CI (like TestRules).
func TestPodsQuery(t *testing.T) {
	if _, err := exec.LookPath("promtool"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("promtool not on PATH in CI")
		}
		t.Skip("promtool not on PATH")
	}
	dir := t.TempDir()
	rules, _ := yaml.Marshal(map[string]any{"groups": []any{map[string]any{"name": "town", "rules": []any{
		map[string]any{"record": "town:pods", "expr": promQ["pods"]}}}}})
	os.WriteFile(filepath.Join(dir, "rules.yaml"), rules, 0o644)
	tb, err := os.ReadFile("testdata/pods.test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "pods.test.yaml"), tb, 0o644)
	cmd := exec.Command("promtool", "test", "rules", "pods.test.yaml")
	cmd.Dir = dir
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("promtool: %v\n%s", err, o)
	}
}

// TestPodsQueryOptional: Prometheus refusing the pods query must not fail the poll (every other signal would go stale).
func TestPodsQueryOptional(t *testing.T) {
	a, _ := testApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("query"), "kube_pod_deletion_timestamp") {
			http.Error(w, `{"status":"error","error":"bad_data"}`, http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()
	a.Opts.PromURL = srv.URL
	p := newProm(a)
	for i := range 2 {
		if err := p.poll(context.Background(), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000+int64(15*i), 0)); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
}

// TestKindsSchema: the page's schema accepts exactly the relay's building kinds (world.json $defs.role).
func TestKindsSchema(t *testing.T) {
	b, err := os.ReadFile("../www/dev/schema/world.json")
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		Defs struct {
			Role struct{ Enum []string } `json:"role"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if got := w.Defs.Role.Enum; !slices.Equal(got, town.Kinds) {
		t.Fatalf("world.json role enum %v != town.Kinds %v", got, town.Kinds)
	}
}

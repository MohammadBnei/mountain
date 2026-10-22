package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/town"
)

// TestPlantRoom (R7.1): on a full island only the oldest plant's hex takes a new plant, which replaces it (gone first,
// then the new mark); every other planted hex stays taken.
func TestPlantRoom(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	n := 0
	try := func(x town.Hex) (string, string) {
		n++
		return ackOf(t, a.place(visitor(fmt.Sprint("s", n), "ana"), "r", "plant", map[string]any{"h": 1.0, "q": float64(x.Q), "r": float64(x.R)}, false, time.Now()))
	}
	var hexes []town.Hex
	for _, x := range town.Hexes {
		if town.Plantable("h1", x) {
			hexes = append(hexes, x)
		}
	}
	ids := map[town.Hex]string{}
	for _, x := range hexes {
		id, why := try(x)
		if why != "" {
			t.Fatalf("fill h1 %v: %s", x, why)
		}
		ids[x] = id
	}
	drain(c)
	if _, why := try(hexes[1]); why != "taken" {
		t.Errorf("full island, a younger plant's hex: %q, want taken", why)
	}
	id, why := try(hexes[0])
	if why != "" {
		t.Fatalf("full island, the oldest plant's hex: %q, want accepted", why)
	}
	ms := marksOf(drain(c))
	if len(ms) != 2 || ms[0]["id"] != ids[hexes[0]] || ms[0]["gone"] != true || ms[1]["id"] != id {
		t.Fatalf("give way: %v", ms)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.State.Marks) != len(hexes) || a.MarkSubs[ids[hexes[0]]] != "" {
		t.Errorf("after give way: %d marks (want %d), old sub %q", len(a.State.Marks), len(hexes), a.MarkSubs[ids[hexes[0]]])
	}
}

// TestWilt (R7.1): plants older than wiltAfter leave for everyone; younger ones and other toys stay.
func TestWilt(t *testing.T) {
	a, c := testApp(t)
	now := time.Now()
	ulidAt := func(at time.Time) string { // a ULID minted at `at` (the 48-bit time prefix), random tail from ULID()
		ms, s := at.UnixMilli(), make([]byte, 10)
		for i := 9; i >= 0; i-- {
			s[i], ms = ulidEnc[ms&31], ms>>5
		}
		return string(s) + ULID()[10:]
	}
	old, young := ulidAt(now.Add(-wiltAfter-time.Minute)), ulidAt(now.Add(-wiltAfter+time.Hour))
	a.State.Marks = []Mark{
		{ID: old, Mark: "plant", Pose: map[string]any{"h": 1, "q": 4, "r": 0, "item": 0}, Weight: 1},
		{ID: young, Mark: "plant", Pose: map[string]any{"h": 1, "q": 4, "r": -1, "item": 0}, Weight: 1},
	}
	a.wilt(&Out{app: a, name: "prom"}, now)
	ms := marksOf(drain(c))
	if len(ms) != 1 || ms[0]["id"] != old || ms[0]["gone"] != true {
		t.Fatalf("wilt: %v", ms)
	}
	if len(a.State.Marks) != 1 || a.State.Marks[0].ID != young {
		t.Errorf("left: %v", a.State.Marks)
	}
}

// TestPlotLifecycle (R7.1): a pending move restarts when an answer lacks the app; a ruin freed after ruinFor is not
// rebuilt from its pods until the app is back in ArgoCD.
func TestPlotLifecycle(t *testing.T) {
	a, c := testApp(t)
	pl, o, t0 := newPlacer(), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000, 0)
	pl.place(a, o, []sample{pod("wird", "wird-a", "k8s-cp-01")}, allReady, t0)
	on := func(node string) []sample {
		return []sample{pod("wird", "wird-a", node), pod("editable-blog", "b", "k8s-cp-01")}
	}
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(time.Minute))   // h2 leads from 1m
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(3*time.Minute)) //
	pl.place(a, o, nil, allReady, t0.Add(4*time.Minute))             // no answer: the count starts over
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(5*time.Minute)) // h2 leads again from 5m
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(7*time.Minute)) // 6 min after 1m, but 2 since 5m
	if p, _ := plotOf(a, "wird"); p.Host != "h1" {
		t.Fatalf("moved after a gap in the answers: %+v", p)
	}
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(10*time.Minute))
	if p, _ := plotOf(a, "wird"); p.Host != "h2" {
		t.Fatalf("5 min of majority: %+v", p)
	}

	pl.ruin(a, o, "wird", true, t0.Add(11*time.Minute))
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(11*time.Minute+ruinFor))
	if _, ok := plotOf(a, "wird"); ok {
		t.Fatal("the ruin should be freed after ruinFor")
	}
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(12*time.Minute+ruinFor))
	if _, ok := plotOf(a, "wird"); ok {
		t.Fatal("a freed ruin came back while its app is out of ArgoCD")
	}
	pl.ruin(a, o, "wird", false, t0.Add(13*time.Minute+ruinFor)) // back in ArgoCD
	pl.place(a, o, on("k8s-cp-02"), allReady, t0.Add(13*time.Minute+ruinFor))
	if p, ok := plotOf(a, "wird"); !ok || p.RuinAt != 0 {
		t.Fatalf("back in ArgoCD: %+v %v", p, ok)
	}
	drain(c)
}

// TestReadyGuard (R7.1): an empty Ready (or quorum) answer keeps every host as it was: no node.ready false for all
// three islands, no bridge raised; before any answer a host's ready is null (unknown), not false.
func TestReadyGuard(t *testing.T) {
	a, c := testApp(t)
	if b := string(mustJSON(a.Metrics)); !strings.Contains(b, `"h1":{"alloc":0,"ready":null`) {
		t.Fatalf("before any poll a host must be unknown: %s", b)
	}
	empty := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		res := "[]"
		switch {
		case empty:
		case strings.Contains(q, `condition="Ready"`):
			res = `[{"metric":{"node":"k8s-cp-01"},"value":[0,"1"]},{"metric":{"node":"k8s-cp-02"},"value":[0,"1"]},{"metric":{"node":"k8s-cp-03"},"value":[0,"1"]}]`
		case strings.Contains(q, "probe_success"):
			res = `[{"metric":{"target":"pve-bnei"},"value":[0,"1"]},{"metric":{"target":"pve-server1"},"value":[0,"1"]}]`
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, res)
	}))
	defer srv.Close()
	a.Opts.PromURL = srv.URL
	p, o := newProm(a), &Out{app: a, name: "prom"}
	for i := range 4 {
		empty = i >= 2
		if err := p.poll(context.Background(), o, time.Unix(1_800_000_000+int64(15*i), 0)); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		if i == 1 {
			drain(c)
		}
	}
	for _, f := range drain(c) {
		if f["kind"] == "node.ready" || f["kind"] == "quorum" {
			t.Errorf("an empty answer must change nothing: %v", f)
		}
	}
	if h := a.Metrics.Hosts["h1"]; h.Ready == nil || !*h.Ready {
		t.Errorf("h1 after empty answers: %+v", h)
	}
}

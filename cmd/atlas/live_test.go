package main

import (
	"context"
	"encoding/json"
	"flag"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/realms"
)

func testIndex() *GraphIndex {
	return &GraphIndex{
		Realm: map[string]string{"app:wird": "apps", "app:wird-adminweb": "apps", "app:platform-traefik": "platform", "app:netbird": "guard",
			"app:mixed-a": "apps", "app:mixed-b": "fleet", "vm:k8s-cp-01": "platform", "vm:pg01": "data", "host:bnei": "platform"},
		NSApps: map[string][]string{"wird": {"app:wird", "app:wird-adminweb"}, "traefik": {"app:platform-traefik"}, "netbird": {"app:netbird"}, "mixed": {"app:mixed-a", "app:mixed-b"}},
		Has:    map[string]bool{"app:wird": true, "app:wird-adminweb": true, "app:platform-traefik": true, "app:netbird": true, "vm:k8s-cp-01": true, "vm:pg01": true, "host:bnei": true},
	}
}

func TestJoins(t *testing.T) {
	idx := testIndex()
	rl, _ := realms.Load()
	for svc, want := range map[string]string{
		"wird-wird-adminweb-80@kubernetescrd": "app:wird-adminweb", "wird-wird-80@kubernetescrd": "app:wird",
		"traefik-dashboard-9000@kubernetes": "app:platform-traefik", "netbird-x-1@kubernetescrd": "app:netbird",
		"unknown-svc-1@kubernetes": "", "wird@internal": "",
	} {
		if got := appOfService(idx, svc); got != want {
			t.Errorf("appOfService(%s) = %q, want %q", svc, got, want)
		}
	}
	for ns, want := range map[string]string{"wird": "apps", "traefik": "platform", "kube-system": "platform", "mixed": unattrib, "nope": unattrib} {
		if got := realmOfNS(idx, rl, ns); got != want {
			t.Errorf("realmOfNS(%s) = %q, want %q", ns, got, want)
		}
	}
}

// fakeProm answers each contract.Queries query from a table (name -> result JSON); an unknown query is an error.
func fakeProm(t *testing.T, answers func() map[string]string) *httptest.Server {
	byQ := map[string]string{}
	for n, q := range contract.Queries {
		byQ[q] = n
	}
	for n, sg := range contract.Signals {
		byQ[sg.Query] = n
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := byQ[r.URL.Query().Get("query")]
		res, ok := answers()[name]
		if !ok {
			res = "[]"
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":` + res + `}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func pvec(pairs ...string) string { // label JSON, value, label JSON, value...
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"metric":` + pairs[i] + `,"value":[0,"` + pairs[i+1] + `"]}`)
	}
	b.WriteString("]")
	return b.String()
}

// TestPromPoll: apps join by ArgoCD name, Traefik counters become requests per second, nodes and probes join by id,
// the land lens sums each realm's share of capacity (unattributed explicit), and a health change is an event.
func TestPromPoll(t *testing.T) {
	rl, _ := realms.Load()
	hub := NewHub(false, func() []byte { return nil }, nil)
	l := newLive(rl, hub, nil)
	health, req := "Healthy", "2"
	srv := fakeProm(t, func() map[string]string {
		return map[string]string{
			"info":       pvec(`{"name":"wird","health_status":"`+health+`","sync_status":"Synced"}`, "1", `{"name":"not-in-graph","health_status":"Healthy"}`, "1"),
			"ready":      pvec(`{"node":"k8s-cp-01"}`, "1", `{"node":"k8s-nope"}`, "1"),
			"probes":     pvec(`{"target":"guest-archive-h2"}`, "0", `{"target":"pve-bnei"}`, "1", `{"target":"guest-unknown"}`, "1"),
			"cpu":        pvec(`{"namespace":"wird"}`, "2", `{"namespace":"kube-system"}`, "1", `{"namespace":"lost"}`, "1"),
			"mem":        pvec(`{"namespace":"wird"}`, "1e9"),
			"disk":       pvec(`{"namespace":"wird"}`, "5e9"),
			"cpucap":     pvec(`{}`, "10"),
			"memcap":     pvec(`{}`, "1e10"),
			"diskcap":    pvec(`{}`, "1e11"),
			"req":        pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, req),
			"cpunode":    pvec(`{"namespace":"wird","node":"k8s-cp-01"}`, "1.5", `{"namespace":"wird","node":"k8s-gone"}`, "1"),
			"memnode":    pvec(`{"namespace":"wird","node":"k8s-cp-01"}`, "1e9"),
			"cpucapnode": pvec(`{"node":"k8s-cp-01"}`, "4"),
			"memcapnode": pvec(`{"node":"k8s-cp-01"}`, "8e9"),
		}
	})
	p := &promPoll{l: l, prom: promHTTP{base: srv.URL, hc: http.DefaultClient}, idx: testIndex}
	ctx := context.Background()
	t0 := time.Now()
	if err := p.poll(ctx, t0); err != nil {
		t.Fatal(err)
	}
	health = "Degraded"
	if err := p.poll(ctx, t0.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	s := l.st
	if a := s.Apps["app:wird"]; a.Health != "Degraded" || a.Sync != "Synced" || a.RPS != 2 || len(s.Apps) != 1 {
		t.Errorf("apps: %+v", s.Apps)
	}
	if !s.Ready["vm:k8s-cp-01"] || len(s.Ready) != 1 {
		t.Errorf("ready: %v", s.Ready)
	}
	if s.Probes["vm:pg01"] || !s.Probes["host:bnei"] || len(s.Probes) != 2 {
		t.Errorf("probes: %v", s.Probes)
	}
	if lr := s.Land.Realms; lr["apps"].CPU != 0.2 || lr["platform"].CPU != 0.1 || lr[unattrib].CPU != 0.1 || lr["apps"].Mem != 0.1 || lr["apps"].Disk != 0.05 {
		t.Errorf("land: %+v", lr)
	}
	if n := s.Land.Nodes["vm:k8s-cp-01"]; n.Capacity.CPU != 4 || n.Capacity.Mem != 8e9 || n.NS["wird"].CPU != 1.5 || n.NS["wird"].Mem != 1e9 || len(s.Land.Nodes) != 1 {
		t.Errorf("nodes: %+v", s.Land.Nodes)
	}
	if s.Land.NSRealm["wird"] != "apps" || s.Land.NSRealm["kube-system"] != "platform" || s.Land.NSRealm["lost"] != unattrib {
		t.Errorf("nsRealm: %v", s.Land.NSRealm)
	}
	if len(l.events) != 1 || l.events[0].Kind != "health" || l.events[0].Node != "app:wird" {
		t.Errorf("events: %+v", l.events)
	}
}

// TestAlertPoll: alerts land on the node their labels name; the first answer is state, later changes are events; a
// failed answer is no answer (the firing set stays).
func TestAlertPoll(t *testing.T) {
	rl, _ := realms.Load()
	l := newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)
	body := `[{"fingerprint":"f1","startsAt":"2026-10-24T10:00:00Z","labels":{"alertname":"KubePodCrashLooping","severity":"warning","namespace":"netbird"},"annotations":{"summary":"crash"}},
		{"fingerprint":"f2","startsAt":"2026-10-24T09:00:00Z","labels":{"alertname":"NodeDown","severity":"critical","node":"k8s-cp-01"}},
		{"fingerprint":"w","labels":{"alertname":"Watchdog"}}]`
	code := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code); _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	a := &amPoll{l: l, am: amHTTP{base: srv.URL, hc: http.DefaultClient}, idx: testIndex, firing: map[string]AlertLive{}}
	ctx := context.Background()
	if err := a.poll(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := l.st.Alerts; len(got) != 2 || got[0].FP != "f2" || got[0].Node != "vm:k8s-cp-01" || got[0].Realm != "platform" ||
		got[1].Node != "app:netbird" || got[1].Realm != "guard" || got[1].Summary != "crash" {
		t.Fatalf("alerts: %+v", got)
	}
	if len(l.events) != 0 {
		t.Errorf("the first answer made events: %+v", l.events)
	}
	body = `[{"fingerprint":"f1","startsAt":"2026-10-24T10:00:00Z","labels":{"alertname":"KubePodCrashLooping","severity":"warning","namespace":"wird"}}]`
	_ = a.poll(ctx, time.Now())
	if len(l.events) != 1 || l.events[0].Kind != "alert.resolve" || l.events[0].Node != "vm:k8s-cp-01" {
		t.Errorf("resolve: %+v", l.events)
	}
	if l.st.Alerts[0].Realm != "apps" || l.st.Alerts[0].Node != "" { // two apps in wird: the realm, no node
		t.Errorf("namespace with two apps: %+v", l.st.Alerts[0])
	}
	code, body = 503, `null`
	if err := a.poll(ctx, time.Now()); err == nil || len(l.st.Alerts) != 1 {
		t.Errorf("a failed answer: %v %d", err, len(l.st.Alerts))
	}
	var f map[string]any
	if json.Unmarshal(mustJSON(l.st), &f) != nil || f["t"] != "live" {
		t.Errorf("frame: %v", f)
	}
}

// TestLiveQuiet: a poll that changes nothing but the timestamp sends no frame (the page would rebuild its panel and
// close what the reader opened); a stale source sends one frame when it goes stale and one when it recovers; an
// empty answer is [] alerts, never null.
func TestLiveQuiet(t *testing.T) {
	rl, _ := realms.Load()
	hub := NewHub(false, func() []byte { return nil }, nil)
	c := &Client{send: make(chan []byte, 64)}
	hub.clients[c] = struct{}{}
	l := newLive(rl, hub, nil)
	for i := 0; i < 3; i++ {
		l.update(func(s *LiveState) { s.At = int64(i + 1) })
	}
	if n := len(c.send); n != 1 {
		t.Errorf("3 polls, one change: %d frames", n)
	}
	l.stale("prom", true)
	l.stale("prom", true)
	l.stale("prom", false)
	l.stale("prom", false)
	if n := len(c.send); n != 3 {
		t.Errorf("stale then recovered: %d frames, want 3", n)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	defer srv.Close()
	a := &amPoll{l: l, am: amHTTP{base: srv.URL, hc: http.DefaultClient}, idx: testIndex, firing: map[string]AlertLive{}}
	_ = a.poll(context.Background(), time.Now())
	if !strings.Contains(string(mustJSON(l.st)), `"alerts":[]`) {
		t.Errorf("no alerts: %s", mustJSON(l.st))
	}
}

// TestSignals: optional signals join onto apps, probed nodes, k8s nodes and the town's day; a signal that answers
// nothing (or fails) is unknown, never zero, and never fails the poll; the day signals are asked on their own cadence.
func TestSignals(t *testing.T) {
	rl, _ := realms.Load()
	l := newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)
	var mu sync.Mutex
	asked := map[string]int{}
	answers := map[string]string{
		"info": pvec(`{"name":"wird","health_status":"Healthy","sync_status":"Synced"}`, "1", `{"name":"netbird","health_status":"Healthy"}`, "1"),
		"req":  pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, "2"),
		// wird has two services: errors and requests sum, latency is the slowest
		"err":         pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, "0.5", `{"exported_service":"wird-wird-adminweb-80@kubernetescrd"}`, "0", `{"exported_service":"wird-wird-81@kubernetescrd"}`, "0.25"),
		"lat":         pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, "0.2", `{"exported_service":"wird-wird-81@kubernetescrd"}`, "0.45"),
		"reqday":      pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, "900", `{"exported_service":"wird-wird-81@kubernetescrd"}`, "100"),
		"errday":      pvec(`{"exported_service":"wird-wird-80@kubernetescrd"}`, "5", `{"exported_service":"wird-wird-81@kubernetescrd"}`, "0"),
		"probeday":    pvec(`{"target":"guest-archive-h2"}`, "0.75"),
		"critday":     pvec(`{}`, "0"), // none fired, and the Watchdog did: calm
		"degradedday": pvec(`{"name":"wird"}`, "0", `{"name":"netbird"}`, "0"),
		"pods":        pvec(`{"node":"k8s-cp-01"}`, "31"),
	}
	srv := fakeProm(t, func() map[string]string { mu.Lock(); defer mu.Unlock(); return answers })
	counting := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		mu.Lock()
		for n, sg := range contract.Signals {
			if sg.Query == q {
				asked[n]++
			}
		}
		mu.Unlock()
		counting.ServeHTTP(w, r)
	})
	p := &promPoll{l: l, prom: promHTTP{base: srv.URL, hc: http.DefaultClient}, idx: testIndex}
	ctx, t0 := context.Background(), time.Now()
	if err := p.poll(ctx, t0); err != nil {
		t.Fatal(err)
	}
	s := l.st
	w := s.Apps["app:wird"]
	if w.Err == nil || *w.Err != 0.75 || w.Lat == nil || *w.Lat != 0.45 || w.ReqDay == nil || *w.ReqDay != 1000 || w.Avail == nil || *w.Avail != 0.995 {
		t.Errorf("wird's signals: err %v lat %v reqDay %v avail %v", w.Err, w.Lat, w.ReqDay, w.Avail)
	}
	if nb := s.Apps["app:netbird"]; nb.Err != nil || nb.Lat != nil || nb.ReqDay != nil || nb.Avail != nil {
		t.Errorf("an app with no Traefik series has unknown signals, not zeros: %+v", nb)
	}
	if s.ProbeDay["vm:pg01"] != 0.75 || s.Pods["vm:k8s-cp-01"] != 31 {
		t.Errorf("probeDay %v pods %v", s.ProbeDay, s.Pods)
	}
	if s.Day == nil || !s.Day.Calm || len(s.Day.Critical) != 0 {
		t.Errorf("a day with no critical alert, nothing Degraded and a Watchdog is calm: %+v", s.Day)
	}

	// a critical alert and a Degraded app today; the latency histogram goes away (unknown, not zero); the day
	// signals are not asked again before their 5 minutes
	mu.Lock()
	answers["critday"] = pvec(`{"alertname":"KubePersistentVolumeFillingUp"}`, "1")
	answers["degradedday"] = pvec(`{"name":"wird"}`, "1", `{"name":"netbird"}`, "0")
	delete(answers, "lat")
	mu.Unlock()
	if err := p.poll(ctx, t0.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	if l.st.Apps["app:wird"].Lat != nil {
		t.Errorf("a signal that answers nothing is unknown: lat %v", *l.st.Apps["app:wird"].Lat)
	}
	if !l.st.Day.Calm {
		t.Error("the day signals were asked again within their 5 minutes")
	}
	mu.Lock()
	n := asked["critday"]
	mu.Unlock()
	if n != 1 {
		t.Errorf("critday asked %d times in 15 s, want 1", n)
	}
	// a tick that wakes a little early still asks the 15 s signals (the ticker jitters)
	if err := p.poll(ctx, t0.Add(29*time.Second)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	n = asked["err"]
	mu.Unlock()
	if n != 3 {
		t.Errorf("err asked %d times at 0, 15 and 29 s, want 3", n)
	}
	if err := p.poll(ctx, t0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if d := l.st.Day; d == nil || d.Calm || !slices.Equal(d.Critical, []string{"KubePersistentVolumeFillingUp"}) || !slices.Equal(d.Degraded, []string{"app:wird"}) {
		t.Errorf("day: %+v", d)
	}

	// no Watchdog over the day (the rules did not run): critday answers nothing, so the day is unknown, not calm
	mu.Lock()
	delete(answers, "critday")
	mu.Unlock()
	if err := p.poll(ctx, t0.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if l.st.Day != nil {
		t.Errorf("a day without the Watchdog's proof is unknown: %+v", l.st.Day)
	}
}

// TestAddonReadiness: an add-on's readiness is its least ready workload (realms.yaml addons, in the namespace its node
// is installed into); one workload unanswered makes it unknown, and a static pod (kube-vip) is always unknown.
func TestAddonReadiness(t *testing.T) {
	rl, _ := realms.Load()
	l := newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)
	idx := testIndex()
	idx.Into = map[string]string{"addon:cilium": "kube-system", "addon:metallb": "metallb-system", "addon:coredns": "kube-system", "addon:kube-vip": "kube-system"}
	for id := range idx.Into {
		idx.Has[id] = true
	}
	srv := fakeProm(t, func() map[string]string {
		return map[string]string{
			"dsready":     pvec(`{"namespace":"kube-system","daemonset":"cilium"}`, "1", `{"namespace":"metallb-system","daemonset":"speaker"}`, "0.8"),
			"deployready": pvec(`{"namespace":"kube-system","deployment":"cilium-operator"}`, "0.5", `{"namespace":"kube-system","deployment":"hubble-relay"}`, "1", `{"namespace":"metallb-system","deployment":"controller"}`, "1"),
		}
	})
	p := &promPoll{l: l, prom: promHTTP{base: srv.URL, hc: http.DefaultClient}, idx: func() *GraphIndex { return idx }}
	if err := p.poll(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	a := l.st.Addons
	if a["addon:cilium"] != 0.5 || a["addon:metallb"] != 0.8 || len(a) != 2 {
		t.Errorf("addons: %v (coredns unanswered and kube-vip static: unknown)", a)
	}
}

// TestPlaceWatched: an alert with no target or node lands on the one node whose watch names it, before its namespace
// is tried (NetBirdUnreachable keeps blackbox's namespace); a rule three hosts watch places no building; a moon's probe
// target places nothing (its alert falls to its other labels).
func TestPlaceWatched(t *testing.T) {
	rl, _ := realms.Load()
	idx := testIndex()
	idx.Watch = map[string][]string{"TraefikCertExpiringSoon": {"app:platform-traefik"}, "NetBirdUnreachable": {"app:netbird"},
		"PVEHostProbeStale": {"host:bnei", "host:ex-laptop", "host:server1"}}
	a := &amPoll{l: newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)}
	for _, c := range []struct {
		labels     map[string]string
		node, real string
	}{
		{map[string]string{"alertname": "TraefikCertExpiringSoon", "cn": "wird.bnei.dev", "sans": "wird.bnei.dev"}, "app:platform-traefik", "platform"},
		{map[string]string{"alertname": "NetBirdUnreachable", "target": "netbird", "namespace": "wird"}, "app:netbird", "guard"},
		{map[string]string{"alertname": "PVEHostProbeStale", "namespace": "wird"}, "", "apps"},
		{map[string]string{"alertname": "TraefikCertExpiringSoon", "node": "k8s-cp-01"}, "vm:k8s-cp-01", "platform"}, // a node label still wins
		// a moon's probe (realms.yaml declares ext:github) is not the alert's place: it has no realm to ping at L0
		{map[string]string{"alertname": "BlackboxProbeFailed", "target": "ext-github", "namespace": "wird"}, "", "apps"},
	} {
		if n, r := a.place(idx, c.labels); n != c.node || r != c.real {
			t.Errorf("%v: placed on %q (%q), want %q (%q)", c.labels, n, r, c.node, c.real)
		}
	}
}

var updateFixtures = flag.Bool("update", false, "rewrite www/dev/fixtures/live.json's health from contract.Classify")

// TestFixtureHealth: the page's tests and the game read the dev fixture's own health; it must be what atlas would
// send for that graph and frame (go test ./cmd/atlas -run TestFixtureHealth -update rewrites it).
func TestFixtureHealth(t *testing.T) {
	g := &Graphs{}
	if err := g.loadFixture("../../www/dev/fixtures/graph.json"); err != nil {
		t.Fatal(err)
	}
	const file = "../../www/dev/fixtures/live.json"
	rl, _ := realms.Load()
	l := newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)
	l.idx = g.index
	if err := l.loadFixture(file); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(l.st.Health)
	var have map[string]contract.Health
	_ = json.Unmarshal(raw["health"], &have)
	if *updateFixtures {
		raw["health"] = want
		if _, ok := raw["alertsAt"]; !ok {
			raw["alertsAt"] = raw["at"]
		}
		out, _ := json.MarshalIndent(raw, "", " ")
		if err := os.WriteFile(file, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !maps.Equal(have, l.st.Health) {
		t.Errorf("live.json's health is not what atlas computes for it: run go test ./cmd/atlas -run TestFixtureHealth -update")
	}
	// spot checks against the fixture's own answers
	for id, st := range map[string]string{"app:wird-adminweb": contract.Degraded, "vm:k8s-worker-02": contract.Down, "ct:hermesagent": contract.Down, "app:wird": contract.OK} {
		if l.st.Health[id].State != st {
			t.Errorf("%s = %+v, want %s", id, l.st.Health[id], st)
		}
	}
	if _, ok := raw["alertsAt"]; !ok {
		t.Error("live.json has no alertsAt: its alerts would read as no answer")
	}
}

// TestLiveHealth: every update classifies against the latest graph; an add-on below all ready is ok while it rolls
// and degraded after contract.AddonHold; Alertmanager's answer stamps alertsAt, which, like at, never makes a frame.
func TestLiveHealth(t *testing.T) {
	rl, _ := realms.Load()
	hub := NewHub(false, func() []byte { return nil }, nil)
	c := &Client{send: make(chan []byte, 64)}
	hub.clients[c] = struct{}{}
	l := newLive(rl, hub, nil)
	if l.update(func(s *LiveState) { s.At = 1 }); len(l.st.Health) != 0 {
		t.Errorf("no graph yet, yet health: %v", l.st.Health)
	}
	idx := testIndex()
	idx.Kind = map[string]string{"app:wird": "app", "addon:cilium": "addon"}
	idx.Has["addon:cilium"] = true
	l.idx = func() *GraphIndex { return idx }
	l.update(func(s *LiveState) {
		s.Apps = map[string]AppLive{"app:wird": {Health: "Degraded"}}
		s.Addons = map[string]float64{"addon:cilium": 0.5}
	})
	if l.st.Health["app:wird"].State != contract.Degraded || l.st.Health["addon:cilium"].State != contract.OK {
		t.Errorf("health: %+v", l.st.Health)
	}
	since := l.low["addon:cilium"]
	l.mu.Lock()
	l.classify(since + contract.AddonHold)
	l.mu.Unlock()
	if l.st.Health["addon:cilium"].State != contract.Degraded {
		t.Errorf("after the hold: %+v", l.st.Health["addon:cilium"])
	}
	l.update(func(s *LiveState) { s.Addons = map[string]float64{"addon:cilium": 1} })
	if _, held := l.low["addon:cilium"]; held || l.st.Health["addon:cilium"].State != contract.OK {
		t.Errorf("back to all ready: %+v %v", l.st.Health["addon:cilium"], l.low)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	defer srv.Close()
	a := &amPoll{l: l, am: amHTTP{base: srv.URL, hc: http.DefaultClient}, idx: testIndex, firing: map[string]AlertLive{}}
	n := len(c.send)
	_ = a.poll(context.Background(), time.Now())
	first := l.st.AlertsAt
	_ = a.poll(context.Background(), time.Now().Add(time.Second))
	// the first answer is a frame even when nothing fires: before it, [] alerts means no answer
	if first == 0 || l.st.AlertsAt == first || len(c.send) != n+1 {
		t.Errorf("alertsAt %d -> %d, %d frames for two equal answers (want 1: the first)", first, l.st.AlertsAt, len(c.send)-n)
	}
}

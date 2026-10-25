package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/realms"
)

func testIndex() *GraphIndex {
	return &GraphIndex{
		Realm: map[string]string{"app:wird": "apps", "app:wird-adminweb": "apps", "app:platform-traefik": "platform", "app:netbird": "identity",
			"app:mixed-a": "apps", "app:mixed-b": "fleet", "vm:k8s-cp-01": "infra", "vm:pg01": "data", "host:bnei": "infra"},
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
	for ns, want := range map[string]string{"wird": "apps", "traefik": "platform", "kube-system": "infra", "mixed": unattrib, "nope": unattrib} {
		if got := realmOfNS(idx, rl, ns); got != want {
			t.Errorf("realmOfNS(%s) = %q, want %q", ns, got, want)
		}
	}
}

// fakeProm answers each promQ query from a table (name -> result JSON); an unknown query is an error.
func fakeProm(t *testing.T, answers func() map[string]string) *httptest.Server {
	byQ := map[string]string{}
	for n, q := range promQ {
		byQ[q] = n
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := byQ[r.URL.Query().Get("query")]
		res, ok := answers()[name]
		if !ok {
			res = "[]"
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":` + res + `}}`))
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
	hub := NewHub(false, func() []byte { return nil })
	l := newLive(rl, hub)
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
	p := &promPoll{l: l, url: srv.URL, hc: http.DefaultClient, idx: testIndex}
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
	if lr := s.Land.Realms; lr["apps"].CPU != 0.2 || lr["infra"].CPU != 0.1 || lr[unattrib].CPU != 0.1 || lr["apps"].Mem != 0.1 || lr["apps"].Disk != 0.05 {
		t.Errorf("land: %+v", lr)
	}
	if n := s.Land.Nodes["vm:k8s-cp-01"]; n.Capacity.CPU != 4 || n.Capacity.Mem != 8e9 || n.NS["wird"].CPU != 1.5 || n.NS["wird"].Mem != 1e9 || len(s.Land.Nodes) != 1 {
		t.Errorf("nodes: %+v", s.Land.Nodes)
	}
	if s.Land.NSRealm["wird"] != "apps" || s.Land.NSRealm["kube-system"] != "infra" || s.Land.NSRealm["lost"] != unattrib {
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
	l := newLive(rl, NewHub(false, func() []byte { return nil }))
	body := `[{"fingerprint":"f1","startsAt":"2026-10-24T10:00:00Z","labels":{"alertname":"KubePodCrashLooping","severity":"warning","namespace":"netbird"},"annotations":{"summary":"crash"}},
		{"fingerprint":"f2","startsAt":"2026-10-24T09:00:00Z","labels":{"alertname":"NodeDown","severity":"critical","node":"k8s-cp-01"}},
		{"fingerprint":"w","labels":{"alertname":"Watchdog"}}]`
	code := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code); w.Write([]byte(body)) }))
	defer srv.Close()
	a := &amPoll{l: l, url: srv.URL, hc: http.DefaultClient, idx: testIndex, firing: map[string]AlertLive{}}
	ctx := context.Background()
	if err := a.poll(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := l.st.Alerts; len(got) != 2 || got[0].FP != "f2" || got[0].Node != "vm:k8s-cp-01" || got[0].Realm != "infra" ||
		got[1].Node != "app:netbird" || got[1].Realm != "identity" || got[1].Summary != "crash" {
		t.Fatalf("alerts: %+v", got)
	}
	if len(l.events) != 0 {
		t.Errorf("the first answer made events: %+v", l.events)
	}
	body = `[{"fingerprint":"f1","startsAt":"2026-10-24T10:00:00Z","labels":{"alertname":"KubePodCrashLooping","severity":"warning","namespace":"wird"}}]`
	a.poll(ctx, time.Now())
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
	hub := NewHub(false, func() []byte { return nil })
	c := &Client{send: make(chan []byte, 64)}
	hub.clients[c] = struct{}{}
	l := newLive(rl, hub)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`[]`)) }))
	defer srv.Close()
	a := &amPoll{l: l, url: srv.URL, hc: http.DefaultClient, idx: testIndex, firing: map[string]AlertLive{}}
	a.poll(context.Background(), time.Now())
	if !strings.Contains(string(mustJSON(l.st)), `"alerts":[]`) {
		t.Errorf("no alerts: %s", mustJSON(l.st))
	}
}

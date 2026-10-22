package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// runAlerts polls the alerts source once per fixture step, 15 s apart from 2026-10-14T17:46:40Z.
func runAlerts(t *testing.T, a *App, c *Client, f *fixture) (*alertsSource, [][]map[string]any) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	u, err := serveFixture(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	a.Opts.AMURL = u
	s, err := newAlerts(a)
	if err != nil {
		t.Fatal(err)
	}
	out, now := &Out{app: a, name: "alerts"}, time.Unix(1_792_000_000, 0)
	var steps [][]map[string]any
	for i := range f.am {
		f.step.Store(int64(i))
		s.poll(ctx, out, now)
		steps = append(steps, drain(c))
		now = now.Add(15 * time.Second)
	}
	return s, steps
}

func short(es []map[string]any, keys ...string) string {
	var p []string
	for _, e := range es {
		var v []string
		for _, k := range keys {
			v = append(v, fmt.Sprint(e[k]))
		}
		p = append(p, strings.Join(v, "/"))
	}
	return strings.Join(p, ",")
}

func staleOf(frames []map[string]any, src string) (any, bool) {
	for _, f := range frames {
		if f["t"] == "metrics" {
			v, ok := f["stale"].(map[string]any)[src]
			return v, ok
		}
	}
	return nil, false
}

// TestAlertsTimeline replays www/dev/fixtures/relay-alerts.json: fire on first sight, Watchdog /
// InfoInhibitor / notice never fire, an outage is stale (no resolve, no re-fire), resolve carries durS.
func TestAlertsTimeline(t *testing.T) {
	a, c := testApp(t)
	a.Opts.FPSalt = devFPSalt
	f, err := loadFixture("../www/dev/fixtures/relay-alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	ms := func(s string) int64 { tm, _ := time.Parse(time.RFC3339, s); return tm.UnixMilli() }
	_, steps := runAlerts(t, a, c, f)
	fire := func(i int) string { return short(evs(steps[i], "alert.fire"), "fp", "sev", "layer", "startsAt") }
	res := func(i int) string { return short(evs(steps[i], "alert.resolve"), "fp", "sev", "layer", "durS") }
	crash, disk, info := a.FP("a1b2c3d4e5f60718"), a.FP("ffee001122334455"), a.FP("0a0b0c0d0e0f1011")
	num := func(n int64) string { return fmt.Sprint(float64(n)) } // JSON numbers decode as float64
	byFP := func(x, y string) string {                            // fire order is by fp
		if x > y {
			x, y = y, x
		}
		return x + "," + y
	}
	want := []struct{ fire, res string }{
		{byFP(crash+"/warning/k8s/"+num(ms("2026-10-14T17:30:00Z")), disk+"/critical/bedrock/"+num(ms("2026-10-14T17:40:00Z"))), ""},
		{info + "/info/apps/" + num(ms("2026-10-14T17:46:00Z")), ""},
		{"", ""},                          // outage: a non-array answer
		{"", ""},                          // outage: null
		{"", crash + "/warning/k8s/1060"}, // 17:47:40 - 17:30:00
		{"", byFP(disk+"/critical/bedrock/475", info+"/info/apps/115")},
	}
	for i, w := range want {
		if fire(i) != w.fire || res(i) != w.res {
			t.Errorf("step %d: fire %q resolve %q, want %q %q", i, fire(i), res(i), w.fire, w.res)
		}
	}
	// stale: set by the first failed poll, the same since on the second, cleared by the next good one
	s2, ok2 := staleOf(steps[2], "alerts")
	s3, ok3 := staleOf(steps[3], "alerts")
	if !ok2 || !ok3 || s2 != s3 {
		t.Errorf("outage: stale.alerts %v/%v, %v/%v", s2, ok2, s3, ok3)
	}
	if _, ok := staleOf(steps[4], "alerts"); ok {
		t.Error("recovery must clear stale.alerts")
	}
	if a.State.Alerts == nil || len(a.State.Alerts) != 0 {
		t.Errorf("hello.state.alerts after all resolved: %v", a.State.Alerts)
	}
	validate(t, steps)
	for _, st := range steps {
		for _, fr := range st {
			b, _ := json.Marshal(fr)
			s := string(b)
			if ipRE.MatchString(s) || versionRE.MatchString(s) || strings.Contains(s, "a1b2c3d4e5f60718") {
				t.Errorf("leak: %s", s)
			}
			for _, h := range append(hostile, "journal", "Kube", "Node", "Wird", "Watchdog", "summary", "longhorn") {
				if strings.Contains(s, h) {
					t.Errorf("leak %q: %.200s", h, s)
				}
			}
		}
	}
}

// TestAlertsOutageKeepsState: while Alertmanager is down, hello.state keeps the firing set and
// mountain_source_last_success stops advancing.
func TestAlertsOutageKeepsState(t *testing.T) {
	a, c := testApp(t)
	a.Opts.FPSalt = devFPSalt
	f, err := loadFixture("../www/dev/fixtures/relay-alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	f.am = f.am[:4] // fire, fire, outage, outage
	SourceLastSuccess.Set("alerts", 0)
	s, _ := runAlerts(t, a, c, f)
	last := func() float64 {
		SourceLastSuccess.mu.Lock()
		defer SourceLastSuccess.mu.Unlock()
		return SourceLastSuccess.v["alerts"]
	}
	ok := last()
	SourceLastSuccess.Set("alerts", 0)
	s.poll(context.Background(), &Out{app: a, name: "alerts"}, time.Now()) // still down
	after := last()
	if ok == 0 || after != 0 {
		t.Errorf("last_success: %v after good polls, %v after a failed one (want unchanged 0)", ok, after)
	}
	if n := len(a.State.Alerts); n != 3 || len(s.firing) != 3 {
		t.Errorf("outage must keep the 3 firing alerts: state %d, source %d", n, len(s.firing))
	}
}

// TestAlertsNoSalt: without MOUNTAIN_FP_SALT nothing leaves (fail closed), and nothing is retried noisily.
func TestAlertsNoSalt(t *testing.T) {
	a, c := testApp(t)
	f, err := loadFixture("../www/dev/fixtures/relay-alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	_, steps := runAlerts(t, a, c, f)
	for i, st := range steps {
		if e := append(evs(st, "alert.fire"), evs(st, "alert.resolve")...); len(e) != 0 {
			t.Errorf("step %d: alert events without a salt: %v", i, e)
		}
	}
}

// TestAlertsSevFilter: a notice alert (the relay's own rules) is left out before sanitize, so it is
// not re-fired, dropped and logged every poll.
func TestAlertsSevFilter(t *testing.T) {
	a, c := testApp(t)
	a.Opts.FPSalt = devFPSalt
	f, err := loadFixture("../www/dev/fixtures/relay-alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	f.am = f.am[:1]
	s, _ := runAlerts(t, a, c, f)
	got, err := s.fetch(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("step 0 public alerts: %v %v, want the warning and the critical only", got, err)
	}
}

func TestAlertLayer(t *testing.T) {
	a, _ := testApp(t)
	s, err := newAlerts(a)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"NodeDown": "bedrock", "PVEQuorumLost": "bedrock", "KubePodCrashLooping": "k8s",
		"LonghornVolumeDegraded": "k8s", "WirdSlow": "apps", "MyNodeDown": "apps"} {
		if got := s.layer(name); got != want {
			t.Errorf("layer(%s) = %s, want %s", name, got, want)
		}
	}
}

// TestNoMassGone (W1b verification): restarting the ArgoCD controller or Prometheus causes no
// app.gone. Each phase lasts past goneAfter; only a real absence with the controller up and
// Prometheus answering ends in app.gone.
func TestNoMassGone(t *testing.T) {
	a, c := testApp(t)
	info := func(apps ...string) []fixtureSeries {
		var s []fixtureSeries
		for _, n := range apps {
			s = append(s, fixtureSeries{M: map[string]string{"name": n, "dest_namespace": n, "health_status": "Healthy"}, V: 1})
		}
		return s
	}
	up := []fixtureSeries{{V: 1}}
	f := &fixture{prom: []map[string][]fixtureSeries{
		{"info": info("wird", "editable-blog"), "ctrl": up}, // 0: both live
		{"info": info("wird"), "ctrl": {{V: 0}}},            // 1: controller down, info half gone
		{"info": nil, "ctrl": nil},                          // 2: Prometheus restarted, nothing scraped yet
		{"info": nil, "ctrl": up},                           // 3: controller scraped, app info not yet
		{"info": info("wird", "editable-blog"), "ctrl": up}, // 4: back (after an unreachable phase)
		{"info": info("wird"), "ctrl": up},                  // 5: editable-blog really gone
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u, err := serveFixture(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	a.Opts.PromURL = u
	p, out, now := newProm(a), &Out{app: a, name: "prom"}, time.Unix(1_800_000_000, 0)
	var gone []string
	for step := range f.prom {
		f.step.Store(int64(step))
		if step == 4 { // Prometheus unreachable for 11 min (Run's error branch): stale, state untouched
			p.url = "http://127.0.0.1:1"
			for range 45 {
				if err := p.poll(ctx, out, now); err == nil {
					t.Fatal("poll of an unreachable Prometheus succeeded")
				}
				out.Stale()
				now = now.Add(15 * time.Second)
			}
			if _, ok := staleOf(drain(c), "prom"); !ok {
				t.Error("unreachable Prometheus must set stale.prom")
			}
			p.url = u
		}
		for range 45 {
			if err := p.poll(ctx, out, now); err != nil {
				t.Fatal(err)
			}
			for _, e := range evs(drain(c), "app.gone") {
				gone = append(gone, fmt.Sprintf("%d:%v", step, e["app"]))
			}
			now = now.Add(15 * time.Second)
		}
	}
	if got := strings.Join(gone, ","); got != "5:editable-blog" {
		t.Fatalf("app.gone: got %q, want only the real absence in step 5", got)
	}
}

// TestAlertWhere (R7): an alert carries its host (node label) and app (namespace label) when it has them, so the
// town can put its beacon there; unknown nodes and private namespaces stay off the wire.
func TestAlertWhere(t *testing.T) {
	a, c := testApp(t)
	a.Opts.FPSalt = devFPSalt
	file := filepath.Join(t.TempDir(), "relay-where.json")
	os.WriteFile(file, []byte(`{"step_s": 1, "steps": [{"am": [
		{"fingerprint": "aaa", "startsAt": "2026-10-22T10:00:00Z", "labels": {"alertname": "KubePodCrashLooping", "severity": "warning", "namespace": "wird", "pod": "wird-1", "node": "k8s-cp-02"}},
		{"fingerprint": "bbb", "startsAt": "2026-10-22T10:00:00Z", "labels": {"alertname": "NodeDiskPressure", "severity": "critical", "node": "k8s-worker-9.lan", "namespace": "searxng"}}]}]}`), 0o644)
	f, err := loadFixture(file)
	if err != nil {
		t.Fatal(err)
	}
	_, steps := runAlerts(t, a, c, f)
	fires := evs(steps[0], "alert.fire")
	if len(fires) != 2 {
		t.Fatalf("fires: %v", fires)
	}
	byApp := map[any]map[string]any{}
	for _, x := range fires {
		byApp[x["app"]] = x
	}
	if w := byApp["wird"]; w == nil || w["host"] != "h2" {
		t.Errorf("wird alert: %v", fires)
	}
	if o := byApp[nil]; o == nil || o["host"] != nil {
		t.Errorf("an unmapped node and a private namespace must leave host and app out: %v", fires)
	}
	a.mu.Lock()
	hosts := []string{a.State.Alerts[0].Host, a.State.Alerts[1].Host}
	a.mu.Unlock()
	if !slices.Contains(hosts, "h2") {
		t.Errorf("hello.state.alerts keeps the host: %v", hosts)
	}
	validate(t, steps)
}

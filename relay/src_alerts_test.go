package main

import (
	"context"
	"encoding/json"
	"fmt"
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

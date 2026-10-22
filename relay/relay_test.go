package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
)

func TestOriginOK(t *testing.T) {
	for _, c := range []struct {
		o        string
		dev, out bool
	}{
		{"https://mountain.bnei.dev", false, true},
		{"http://mountain.bnei.dev", false, false},
		{"https://mountain.bnei.dev.evil.com", false, false},
		{"https://evil.com", true, false},
		{"", false, false},
		{"", true, false},
		{"http://127.0.0.1:8080", false, false},
		{"http://127.0.0.1:8080", true, true},
		{"http://127.0.0.1", true, false},
		{"https://127.0.0.1:8080", true, false},
		{"http://localhost:8080", true, false},
	} {
		if originOK(c.o, c.dev) != c.out {
			t.Errorf("originOK(%q, dev=%v) != %v", c.o, c.dev, c.out)
		}
	}
}

func TestDevOffByDefault(t *testing.T) {
	o, err := parseServe(nil)
	if err != nil || o.Dev || o.Listen != ":8080" || o.Metrics != ":9090" || o.Source != "live" {
		t.Fatalf("defaults: %+v %v", o, err)
	}
	if strings.Contains(csp([]byte("<html>"), false), "127.0.0.1") {
		t.Fatal("production CSP allows 127.0.0.1")
	}
}

func dial(t *testing.T, srv *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{HTTPHeader: h})
}

func TestHub(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	for _, dev := range []bool{false, true} {
		a := newApp(cfg, Opts{Dev: dev}, "2026-10-14")
		srv := httptest.NewServer(a.Hub)
		for _, bad := range []string{"", "https://evil.example", "http://127.0.0.1:1"} {
			if bad == "http://127.0.0.1:1" && dev {
				continue
			}
			_, resp, err := dial(t, srv, bad)
			if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
				t.Errorf("dev=%v origin %q: want 403, got %v %v", dev, bad, resp, err)
			}
		}
		good := "https://mountain.bnei.dev"
		if dev {
			good = "http://127.0.0.1:1"
		}
		c, _, err := dial(t, srv, good)
		if err != nil {
			t.Fatalf("dev=%v: %v", dev, err)
		}
		c.SetReadLimit(1 << 20)
		_, b, err := c.Read(context.Background())
		var f struct{ T string }
		if err != nil || json.Unmarshal(b, &f) != nil || f.T != "hello" {
			t.Fatalf("first frame: %v %.80s", err, b)
		}
		out := &Out{app: a, name: "test"}
		if id := out.Ev("argo.sync", map[string]any{"app": "kube-system", "ok": true}); id != "" {
			t.Errorf("a non-allowlisted app must drop the frame, got id %s", id)
		}
		id := out.Ev("argo.sync", map[string]any{"app": "wird", "ok": true, "kind": "hostile", "t": "x", "ns": "kube-system"})
		_, b, err = c.Read(context.Background())
		if err != nil || !strings.Contains(string(b), `"kind":"argo.sync"`) || !strings.Contains(string(b), `"t":"ev"`) ||
			!strings.Contains(string(b), `"id":"`+id+`"`) || strings.Contains(string(b), "kube-system") {
			t.Fatalf("broadcast must be sanitised and carry the returned id: %v %s", err, b)
		}
		c.CloseNow()
		srv.Close()
	}
}

func TestConfigAllowlist(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"agent-fleet-core", "has-journal", "wird-adminweb"} {
		if _, ok := cfg.Apps[banned]; ok {
			t.Errorf("%s must not be public", banned)
		}
	}
	if _, err := loadConfig([]byte("apps: {}\nnamespaces: {x: y}\n")); err == nil {
		t.Error("a namespace mapped to a non-allowlisted app must be rejected")
	}
}

// mountain_ws_connections: the type line only, its value is shared with the WS tests, whose closes are async
func TestMetricsAndULID(t *testing.T) {
	rec := httptest.NewRecorder()
	SourceLastSuccess.Set("prom", 1)
	serveMetrics(rec, nil)
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{"# TYPE mountain_ws_connections gauge", `mountain_source_last_success{source="prom"} 1`, "# TYPE mountain_unmapped_total counter"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("metrics missing %q:\n%s", want, body)
		}
	}
	a, b := ULID(), ULID()
	if len(a) != 26 || a == b || a[:10] > b[:10] {
		t.Errorf("ULID %s %s", a, b)
	}
}

// A frame broadcast while a client's hello is being built must reach it (after the hello), not vanish.
func TestHelloRegisterAtomic(t *testing.T) {
	var h *Hub
	h = NewHub(false, func() []byte {
		done := make(chan struct{})
		go func() { h.Broadcast([]byte(`{"t":"ev"}`)); close(done) }()
		select { // the broadcast lands mid-hello unless it waits on the hub lock
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
		return []byte(`{"t":"hello"}`)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	c, _, err := dial(t, srv, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	for _, want := range []string{"hello", "ev"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, b, err := c.Read(ctx)
		cancel()
		if err != nil || !strings.Contains(string(b), `"t":"`+want+`"`) {
			t.Fatalf("want %s frame, got %s %v", want, b, err)
		}
	}
}

// TestHelloBuild: hello.build is the served build id (R8 build guard), absent when unset (dump-hello).
func TestHelloBuild(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(cfg, Opts{}, "2026-10-14")
	var h map[string]any
	json.Unmarshal(a.Hello(), &h)
	if _, ok := h["build"]; ok {
		t.Fatal("no build id: hello.build must be absent")
	}
	a.Build = buildID(fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}})
	json.Unmarshal(a.Hello(), &h)
	if h["build"] != a.Build || len(a.Build) != 12 {
		t.Fatalf("hello.build %v, want %s", h["build"], a.Build)
	}
}

func TestAlertState(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(cfg, Opts{}, "2026-10-14")
	out := &Out{app: a, name: "alerts"}
	state := func() string {
		var h struct {
			State struct{ Alerts json.RawMessage }
		}
		json.Unmarshal(a.Hello(), &h)
		return `{"alerts":` + string(h.State.Alerts) + `}`
	}
	if s := state(); s != `{"alerts":[]}` {
		t.Fatalf("empty state must be {\"alerts\":[]}, got %s", s)
	}
	al := Alert{FP: "0123456789abcdef", Sev: "critical", Layer: "k8s", StartsAt: 1760450000000}
	if out.Fire(al) == "" || out.Fire(al) == "" {
		t.Fatal("fire dropped")
	}
	if s := state(); s != `{"alerts":[{"fp":"0123456789abcdef","sev":"critical","layer":"k8s","startsAt":1760450000000}]}` {
		t.Fatalf("fire (twice, same fp) -> one alert in hello.state, got %s", s)
	}
	if out.Fire(Alert{FP: "raw-fingerprint", Sev: "critical", Layer: "k8s"}) != "" || out.Fire(Alert{FP: "0123456789abcde0", Sev: "page", Layer: "k8s"}) != "" {
		t.Fatal("an unhashed fp or unknown severity must be dropped")
	}
	if out.Resolve(al, 900) == "" || state() != `{"alerts":[]}` {
		t.Fatalf("resolve must empty hello.state.alerts, got %s", state())
	}
	out.Stale()
	if a.Metrics.Stale["alerts"] == 0 {
		t.Fatal("Out.Stale must set metrics stale[source]")
	}
}

type failing struct{}

func (failing) Name() string                    { return "never" }
func (failing) Run(context.Context, *Out) error { return io.ErrUnexpectedEOF }

func TestSourceSeeded(t *testing.T) {
	old := registry
	defer func() { registry = old }()
	registry = map[string]func(*App) (Source, error){"never": func(*App) (Source, error) { return failing{}, nil }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := startSources(ctx, &App{Opts: Opts{Source: "live"}}); err != nil {
		t.Fatal(err)
	}
	SourceLastSuccess.mu.Lock()
	v := SourceLastSuccess.v["never"]
	SourceLastSuccess.mu.Unlock()
	if v < float64(time.Now().Add(-time.Minute).Unix()) {
		t.Fatalf("a source that never succeeds must still export mountain_source_last_success (start time), got %v", v)
	}
}

// TestFPSalt: fp = 16 hex of sha256(salt+fingerprint), stable for one salt, "" (dropped) without one;
// the salt comes from MOUNTAIN_FP_SALT, with a fixed dev value only in fixture mode (02: never rotated).
func TestFPSalt(t *testing.T) {
	t.Setenv("MOUNTAIN_FP_SALT", "")
	if o, _ := parseServe(nil); o.FPSalt != "" {
		t.Fatalf("live mode must not invent a salt, got %q", o.FPSalt)
	}
	if o, _ := parseServe([]string{"--source", "fixture:x.json"}); o.FPSalt != devFPSalt {
		t.Fatalf("fixture mode salt = %q", o.FPSalt)
	}
	t.Setenv("MOUNTAIN_FP_SALT", "s1")
	o, _ := parseServe(nil)
	a := &App{Opts: o}
	if fp := a.FP("abc"); !fpRE.MatchString(fp) || fp != a.FP("abc") || fp == (&App{Opts: Opts{FPSalt: "s2"}}).FP("abc") || fp == a.FP("abd") {
		t.Fatalf("fp %q: want 16 hex, stable per salt, salt- and fingerprint-dependent", fp)
	}
	if (&App{}).FP("abc") != "" {
		t.Fatal("no salt must yield \"\" (sanitize drops it)")
	}
}

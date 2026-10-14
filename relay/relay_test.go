package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		a := newApp(cfg, Opts{Seed: 1, Dev: dev}, "2026-10-14")
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
		(&Out{app: a, name: "test"}).Ev("argo.sync", map[string]any{"app": "wird", "ok": true, "kind": "hostile", "t": "x"})
		_, b, err = c.Read(context.Background())
		if err != nil || !strings.Contains(string(b), `"kind":"argo.sync"`) || !strings.Contains(string(b), `"t":"ev"`) {
			t.Fatalf("broadcast: %v %s", err, b)
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

func TestMetricsAndULID(t *testing.T) {
	rec := httptest.NewRecorder()
	SourceLastSuccess.Set("prom", 1)
	serveMetrics(rec, nil)
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{"mountain_ws_connections 0", `mountain_source_last_success{source="prom"} 1`, "# TYPE mountain_unmapped_total counter"} {
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

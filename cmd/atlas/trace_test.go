package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/ukubi-go/telemetry"
)

// tracedApp is testApp with its routes traced into an in-memory exporter (traced=false: no tracer at all, a nil
// Tracing, which is what serve has without the variable).
func tracedApp(t *testing.T, traced bool) (*tapp, *tracetest.InMemoryExporter, *telemetry.Tracing) {
	t.Helper()
	a := testApp(t)
	exp := tracetest.NewInMemoryExporter()
	var tel *telemetry.Tracing
	if traced {
		tel = telemetry.WithExporter(exp, "atlas")
		a.Tracing = tel
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	a.h = routes(a.App, www)
	return a, exp, tel
}

func spanStatus(s tracetest.SpanStub) int64 {
	for _, kv := range s.Attributes {
		if kv.Key == "http.response.status_code" {
			return kv.Value.AsInt64()
		}
	}
	return -1
}

// TestRouteSpans: with tracing, each route's request has one server span named by the route's pattern, outside the gate
// (a refusal has its span); a request no route takes (the mux's 405) has none; a path no route names is the static
// route's own 404 under "GET /"; and without a tracer nothing is wrapped and nothing is exported.
func TestRouteSpans(t *testing.T) {
	a, exp, tel := tracedApp(t, true)
	for _, c := range []struct {
		method, target, span string
		code                 int
		open                 bool // the gate admits everyone (devOpen)
	}{
		{"GET", "/healthz", "", 200, false},                 // untraced: the probes (route.untraced)
		{"GET", "/api/graph", "GET /api/graph", 403, false}, // the gate refuses (no one is signed in), inside the span
		{"GET", "/no/such/thing", "GET /", 404, true},       // every static asset and 404 shares the span name "GET /"
		{"POST", "/api/graph", "", 405, true},               // the mux's own 405: no route took it
	} {
		exp.Reset()
		a.gate.devOpen = c.open
		resp := a.serveReq(c.method, c.target, nil, "traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		if resp.StatusCode != c.code {
			t.Errorf("%s %s: %d, want %d", c.method, c.target, resp.StatusCode, c.code)
		}
		if err := tel.Flush(); err != nil {
			t.Fatal(err)
		}
		spans := exp.GetSpans()
		switch {
		case c.span == "" && len(spans) != 0:
			t.Errorf("%s %s: %d spans, want none", c.method, c.target, len(spans))
		case c.span != "" && len(spans) != 1:
			t.Errorf("%s %s: %d spans, want 1", c.method, c.target, len(spans))
		case c.span != "":
			s := spans[0]
			if s.Name != c.span || spanStatus(s) != int64(c.code) {
				t.Errorf("%s %s: span %q status %d, want %q %d", c.method, c.target, s.Name, spanStatus(s), c.span, c.code)
			}
			if s.Parent.IsValid() || len(s.Links) != 1 { // a public endpoint: the caller's trace is linked, never the parent
				t.Errorf("%s %s: parent %v, links %d", c.method, c.target, s.Parent, len(s.Links))
			}
		}
	}

	// without a tracer: the same request answers the same and exports nothing
	a2, exp2, _ := tracedApp(t, false)
	if a2.Tracing != nil {
		t.Fatal("a tracer without tracing")
	}
	if resp := a2.serveReq("GET", "/healthz", nil); resp.StatusCode != 200 {
		t.Errorf("untraced /healthz: %d", resp.StatusCode)
	}
	if n := len(exp2.GetSpans()); n != 0 {
		t.Errorf("untraced: %d spans", n)
	}
}

// TestRouteSpansSocket: a traced /ws still upgrades (the hijack works through the span's writer), and its span ends
// with the socket.
func TestRouteSpansSocket(t *testing.T) {
	a, exp, tel := tracedApp(t, true)
	a.gate.devOpen = true
	srv := httptest.NewServer(a.h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" {
		t.Errorf("hello: %v", f)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	var names []string
	for i := 0; i < 100; i++ { // the handler returns a moment after the close
		_ = tel.Flush()
		names = names[:0]
		for _, s := range exp.GetSpans() {
			names = append(names, s.Name)
		}
		if len(names) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(names) != 1 || names[0] != "GET /ws" {
		t.Errorf("spans after the socket closed: %v", names)
	}
}

// TestOTelErrorHandler: an error OTel reports is the log package's INFO line, msg the error, until telemetry.New with
// an endpoint installs ukubi-go's handler; after it the same error is one WARN line, msg "traces", err the error
// (main.go's doc). OTel's handler is a process global with no unset, so the test runs in a child process (the test
// binary again, as TestFatal), with no OTEL_* of the caller's: a bad OTEL_TRACES_SAMPLER would add its own line.
func TestOTelErrorHandler(t *testing.T) {
	if os.Getenv("ATLAS_TEST_OTEL_HANDLER") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestOTelErrorHandler$", "-test.count=1")
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "OTEL_") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		cmd.Env = append(cmd.Env, "ATLAS_TEST_OTEL_HANDLER=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	var buf bytes.Buffer
	slog.SetDefault(telemetry.NewLogger(&buf)) // as main does: the log package goes through it
	line := func() map[string]any {
		t.Helper()
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		buf.Reset()
		if len(lines) != 1 {
			t.Fatalf("want one line, got %q", lines)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
			t.Fatalf("%q: %v", lines[0], err)
		}
		return m
	}

	otel.Handle(errors.New("x"))
	if m := line(); m["level"] != "INFO" || m["msg"] != "x" {
		t.Fatalf("before New: want INFO msg x, got %v", m)
	}
	if _, err := telemetry.New("", "atlas"); err != nil {
		t.Fatal(err)
	}
	otel.Handle(errors.New("x"))
	if m := line(); m["level"] != "INFO" || m["msg"] != "x" {
		t.Fatalf("New without endpoint: want INFO msg x, got %v", m)
	}

	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	tel, err := telemetry.New(srv.URL+"/v1/traces", "atlas")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Shutdown() })
	otel.Handle(errors.New("x"))
	if m := line(); m["level"] != "WARN" || m["msg"] != "traces" || m["err"] != "x" {
		t.Fatalf("New with endpoint: want WARN msg traces err x, got %v", m)
	}
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/MohammadBnei/mountain/api/telemetry/canned"
	telemetryv1 "github.com/MohammadBnei/mountain/api/telemetry/v1"
	"github.com/MohammadBnei/mountain/api/telemetry/v1/telemetryv1connect"
	"github.com/MohammadBnei/mountain/contract"
)

func fixtureIndex(t *testing.T) *GraphIndex {
	t.Helper()
	g := &Graphs{}
	if err := g.loadFixture("../../www/dev/fixtures/graph.json"); err != nil {
		t.Fatal(err)
	}
	return g.index()
}

// TestTelemetryJoin: a subject joins to the Applications that deploy it, through the graph, and from them to the
// spike's selectors (ui-data-spike.md) through contract; a name that could break out of a matcher is no target.
func TestTelemetryJoin(t *testing.T) {
	idx := fixtureIndex(t)
	fleet := []contract.TelemetryTarget{{Namespace: "agent-fleet", App: "agent-fleet-core"}, {Namespace: "agent-fleet", App: "platform-provisioner"}}
	for subject, want := range map[string][]contract.TelemetryTarget{
		"building:repo:agent-fleet": fleet,
		"repo:agent-fleet":          fleet,
		"app:agent-fleet-core":      fleet[:1],
		"building:app:mountain":     {{Namespace: "mountain", App: "mountain"}},
	} {
		if got, why := telemetryTargets(idx, subject); why != "" || !slices.Equal(got, want) {
			t.Errorf("%s: %v %q, want %v", subject, got, why, want)
		}
	}
	for subject, why := range map[string]string{
		"repo:nope":               "no such node: repo:nope",
		"adr:agent-fleet/0001":    "no telemetry for a adr",
		"svc:agent-fleet/core":    "no telemetry for a svc",
		"building:host:bnei":      "no telemetry for a host",
		"building:":               "no such node: ",
		"app:agent-fleet-core\"}": "no such node: app:agent-fleet-core\"}",
	} {
		if got, w := telemetryTargets(idx, subject); got != nil || w != why {
			t.Errorf("%s: %v %q, want %q", subject, got, w, why)
		}
	}
	if _, why := telemetryTargets(nil, "app:mountain"); why != "atlas has no graph yet" {
		t.Errorf("no graph: %q", why)
	}
	bad := &GraphIndex{Has: map[string]bool{"app:x": true}, Kind: map[string]string{"app:x": "app"}, AppNS: map[string]string{"app:x": `x"} or vector(1) #`}}
	if got, why := telemetryTargets(bad, "app:x"); got != nil || why == "" {
		t.Errorf("an unsafe namespace joined: %v", got)
	}

	w := contract.TelemetryWindows["WINDOW_1H"]
	m := contract.TelemetryMetrics(fleet, w)
	if len(m) != len(contract.TelemetrySeries) || len(m) > 4 {
		t.Fatalf("metrics: %d queries for %d series (at most 4)", len(m), len(contract.TelemetrySeries))
	}
	for i, sub := range []string{`exported_service=~"(agent-fleet-agent-fleet-core|agent-fleet-platform-provisioner)-.*"`, `code=~"5.."`, "histogram_quantile(0.95", `kube_pod_status_phase{namespace=~"agent-fleet",phase="Running"}`} {
		if !strings.Contains(m[i], sub) {
			t.Errorf("metrics[%d] = %s, want %s in it", i, m[i], sub)
		}
	}
	if q := contract.TelemetryLogs(fleet, `say "hi"`); q != `{namespace=~"agent-fleet"} |= "say \"hi\""` {
		t.Errorf("logs: %s", q)
	}
	if q := contract.TelemetrySpans(append(fleet, contract.TelemetryTarget{Namespace: "mountain", App: "mountain"})); q != `{ resource.service.name =~ "agent-fleet-core|platform-provisioner|atlas" }` {
		t.Errorf("spans: %s", q)
	}
	u, err := url.Parse(contract.ExploreURL("https://grafana.example/", contract.GrafanaLoki, []string{`{namespace=~"agent-fleet"}`}, w))
	if err != nil || u.Host != "grafana.example" || u.Path != "/explore" || !strings.Contains(u.Query().Get("panes"), `"uid":"ds-loki"`) ||
		!strings.Contains(u.Query().Get("panes"), `"from":"now-1h"`) || !strings.Contains(u.Query().Get("panes"), `"expr":"{namespace=~\"agent-fleet\"}"`) {
		t.Errorf("explore URL: %v %v", u, err)
	}
}

// backend is a fake Prometheus, Loki and Tempo on one server; stall makes every answer wait for the caller to give up,
// fail makes it a 500.
type backend struct {
	*httptest.Server
	mu          sync.Mutex
	stall, fail bool
	asked       []url.Values
}

func newBackend(t *testing.T) *backend {
	b := &backend{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		stall, fail := b.stall, b.fail
		b.asked = append(b.asked, r.URL.Query())
		b.mu.Unlock()
		if stall { // bounded, so a read with no budget fails the test instead of hanging it
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
			return
		}
		if fail {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case contract.PromRangePath:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"1.5"],[1700000060,"NaN"],[1700000120,"2"]]}]}}`))
		case contract.LokiRangePath:
			var s []string
			for i := range 150 { // two streams of 150: the face keeps the newest 200 across both
				s = append(s, fmt.Sprintf(`{"stream":{"service_name":"a","detected_level":"info"},"values":[["%d","a%d"]]}`, 1700000000000000000+int64(i)*2e9, i),
					fmt.Sprintf(`{"stream":{"service_name":"b"},"values":[["%d","b%d"]]}`, 1700000001000000000+int64(i)*2e9, i))
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[` + strings.Join(s, ",") + `]}}`))
		case contract.TempoSearchPath:
			var s []string
			for i := range 25 {
				s = append(s, fmt.Sprintf(`{"traceID":"t%d","rootServiceName":"agent-fleet-core","rootTraceName":"GET /","startTimeUnixNano":"%d","durationMs":%d}`, i, 1700000000000000000+int64(i)*1e9, i))
			}
			_, _ = w.Write([]byte(`{"traces":[` + strings.Join(s, ",") + `]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *backend) set(stall, fail bool) {
	b.mu.Lock()
	b.stall, b.fail = stall, fail
	b.mu.Unlock()
}

func testTelemetry(t *testing.T, b *backend, met *Metrics) *Telemetry {
	idx := fixtureIndex(t)
	tl := newTelemetry(func() *GraphIndex { return idx }, met)
	tl.prom, tl.loki, tl.tempo, tl.budget = b.URL, b.URL, b.URL, 200*time.Millisecond
	return tl
}

// TestTelemetryFaces: each face reads its backend within the faces' limits, newest first, with its Explore URL and
// age; each read is timed by source.
func TestTelemetryFaces(t *testing.T) {
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	b := newBackend(t)
	tl := testTelemetry(t, b, m)
	ctx := context.Background()
	before := time.Now().UnixMilli()

	f := tl.frame(ctx, "building:repo:agent-fleet", telemetryv1.Face_FACE_METRICS, telemetryv1.Window_WINDOW_15M, "", nil)
	ser := f.GetMetrics().GetSeries()
	if f.Unknown != "" || f.AtMs < before || len(ser) != 4 || ser[0].Name != "requests" || ser[3].Unit != "pods" || len(ser[0].Points) != 2 ||
		ser[0].Points[1].AtMs != 1700000120000 || ser[0].Points[1].Value != 2 || !strings.Contains(f.ExploreUrl, "ds-prometheus") {
		t.Errorf("metrics frame: %v", f)
	}
	for _, q := range b.asked {
		if q.Get("step") != "15" || q.Get("query") == "" {
			t.Errorf("a range query: %v", q)
		}
	}

	b.asked = nil
	f = tl.frame(ctx, "repo:agent-fleet", telemetryv1.Face_FACE_LOGS, telemetryv1.Window_WINDOW_1H, "boom", nil)
	lines := f.GetLogs().GetLines()
	if f.Unknown != "" || len(lines) != contract.TelemetryLogsMax || lines[0].Line != "b149" || lines[1].Line != "a149" || lines[0].Service != "b" || lines[1].Level != "info" {
		t.Errorf("logs frame: unknown %q, %d lines, first %v", f.Unknown, len(lines), lines[:min(2, len(lines))])
	}
	if q := b.asked[0]; q.Get("limit") != "200" || q.Get("direction") != "backward" || q.Get("query") != `{namespace=~"agent-fleet"} |= "boom"` {
		t.Errorf("the Loki query: %v", q)
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].AtMs > lines[i-1].AtMs {
			t.Fatalf("logs not newest first at %d", i)
		}
	}

	b.asked = nil
	f = tl.frame(ctx, "app:agent-fleet-core", telemetryv1.Face_FACE_SPANS, telemetryv1.Window_WINDOW_24H, "", nil)
	tr := f.GetSpans().GetTraces()
	if f.Unknown != "" || len(tr) != contract.TelemetrySpansMax || tr[0].TraceId != "t24" || tr[0].StartMs != 1700000024000 || tr[0].DurationMs != 24 {
		t.Errorf("spans frame: unknown %q, %d traces, first %v", f.Unknown, len(tr), tr[:min(1, len(tr))])
	}
	if q := b.asked[0]; q.Get("limit") != "20" || q.Get("q") != `{ resource.service.name =~ "agent-fleet-core" }` {
		t.Errorf("the Tempo query: %v", q)
	}

	_, _, body := scrape(t, m)
	for _, s := range []string{`mountain_telemetry_read_seconds_count{source="prom"} 4`, `mountain_telemetry_read_seconds_count{source="loki"} 1`,
		`mountain_telemetry_read_seconds_count{source="tempo"} 1`, `mountain_telemetry_read_seconds_bucket{source="prom",le="1.5"} 4`} {
		if !strings.Contains(body, s) {
			t.Errorf("the exposition lacks %s:\n%s", s, body)
		}
	}
}

// TestTelemetryFailClosed: a backend down, slow, erroring or not configured is a frame with the reason and no age,
// within the budget; a Watch's next frame keeps the last good data and its age beside the reason.
func TestTelemetryFailClosed(t *testing.T) {
	b := newBackend(t)
	tl := testTelemetry(t, b, nil)
	ctx := context.Background()
	for _, c := range []struct {
		name, want  string
		stall, fail bool
		setup       func()
	}{
		{"slow", "no answer within 200ms", true, false, nil},
		{"erroring", "status 500", false, true, nil},
		{"down", "loki: ", false, false, func() { tl.loki = "http://127.0.0.1:1" }},
		{"not configured", "loki is not configured", false, false, func() { tl.loki = "" }},
	} {
		b.set(c.stall, c.fail)
		if c.setup != nil {
			c.setup()
		}
		start := time.Now()
		f := tl.frame(ctx, "repo:agent-fleet", telemetryv1.Face_FACE_LOGS, telemetryv1.Window_WINDOW_1H, "", nil)
		if took := time.Since(start); took > time.Second {
			t.Errorf("%s: took %v (budget 200ms)", c.name, took)
		}
		if !strings.Contains(f.Unknown, c.want) || f.AtMs != 0 || f.Data != nil || f.ExploreUrl == "" {
			t.Errorf("%s: unknown %q at %d data %v explore %q", c.name, f.Unknown, f.AtMs, f.Data, f.ExploreUrl)
		}
	}
	tl.loki = b.URL
	b.set(false, false)
	good := tl.frame(ctx, "repo:agent-fleet", telemetryv1.Face_FACE_METRICS, telemetryv1.Window_WINDOW_1H, "", nil)
	b.set(false, true)
	f := tl.frame(ctx, "repo:agent-fleet", telemetryv1.Face_FACE_METRICS, telemetryv1.Window_WINDOW_1H, "", good)
	if good.Unknown != "" || !strings.Contains(f.Unknown, "prom: status 500") || f.AtMs != good.AtMs || f.GetMetrics() != good.GetMetrics() {
		t.Errorf("the stale frame: unknown %q at %d (good at %d), data kept %v", f.Unknown, f.AtMs, good.AtMs, f.GetMetrics() == good.GetMetrics())
	}
	if f := tl.frame(ctx, "adr:agent-fleet/0001", telemetryv1.Face_FACE_METRICS, telemetryv1.Window_WINDOW_1H, "", good); f.Unknown == "" || f.Data != nil {
		t.Errorf("no join: %v", f)
	}
}

// telemetryServer is the routes with the machine principal configured and the telemetry on b; it returns the server
// and a valid token.
func telemetryServer(t *testing.T, b *backend) (*tapp, *httptest.Server, string) {
	a, _ := withAuth(t)
	if err := a.Graph.loadFixture("../../www/dev/fixtures/graph.json"); err != nil {
		t.Fatal(err)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	a.gate.machine = withMachine(t, certPEM(t, key, &key.PublicKey))
	tl := a.Telemetry
	tl.prom, tl.loki, tl.tempo, tl.budget = b.URL, b.URL, b.URL, 200*time.Millisecond
	srv := httptest.NewServer(a.h)
	t.Cleanup(srv.Close)
	return a, srv, "Bearer " + mint(t, key, `{"alg":"RS256"}`, good(10*time.Minute))
}

// TestTelemetryMachine: the machine principal reads both methods through the generated client, Read as a GET; the
// methods are method-scoped (Read by POST, Watch by GET are no route); without a token, or with a bad one, nothing.
func TestTelemetryMachine(t *testing.T) {
	b := newBackend(t)
	_, srv, tok := telemetryServer(t, b)
	ctx := context.Background()
	client := telemetryv1connect.NewTelemetryServiceClient(srv.Client(), srv.URL, connect.WithHTTPGet())

	var method string
	cl := telemetryv1connect.NewTelemetryServiceClient(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		method = r.Method
		return srv.Client().Transport.RoundTrip(r)
	})}, srv.URL, connect.WithHTTPGet())
	req := connect.NewRequest(&telemetryv1.ReadRequest{Subject: "building:repo:agent-fleet", Face: telemetryv1.Face_FACE_SPANS})
	req.Header().Set("Authorization", tok)
	res, err := cl.Read(ctx, req)
	if err != nil || method != http.MethodGet || res.Msg.Frame.GetSpans() == nil || res.Msg.Frame.Window != telemetryv1.Window_WINDOW_1H ||
		res.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("machine Read (%s): %v %v", method, res, err)
	}

	wreq := connect.NewRequest(&telemetryv1.WatchRequest{Subject: "repo:agent-fleet", Face: telemetryv1.Face_FACE_METRICS})
	wreq.Header().Set("Authorization", tok)
	st, err := client.Watch(ctx, wreq)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Receive() || st.Msg().Frame.GetMetrics() == nil {
		t.Errorf("machine Watch: no first frame: %v", st.Err())
	}
	_ = st.Close()

	for name, h := range map[string]string{"no token": "", "a bad token": "Bearer x.y.z"} {
		req := connect.NewRequest(&telemetryv1.ReadRequest{Subject: "repo:agent-fleet", Face: telemetryv1.Face_FACE_LOGS})
		if h != "" {
			req.Header().Set("Authorization", h)
		}
		if _, err := client.Read(ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated && connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
	for _, m := range [][2]string{{"POST", telemetryv1connect.TelemetryServiceReadProcedure}, {"GET", telemetryv1connect.TelemetryServiceWatchProcedure},
		{"PUT", telemetryv1connect.TelemetryServiceReadProcedure}, {"GET", "/atlas.telemetry.v1.TelemetryService/Other"}} {
		r, _ := http.NewRequest(m[0], srv.URL+m[1], strings.NewReader(`{}`))
		r.Header.Set("Authorization", tok)
		r.Header.Set("Content-Type", "application/json")
		resp, err := srv.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("machine %s %s: %d, want refused", m[0], m[1], resp.StatusCode)
		}
	}

	bad := connect.NewRequest(&telemetryv1.ReadRequest{Subject: "repo:agent-fleet", Face: telemetryv1.Face_FACE_LOGS, Contains: strings.Repeat("x", 201)})
	bad.Header().Set("Authorization", tok)
	if _, err := client.Read(ctx, bad); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a 201-byte filter: %v", err)
	}
	none := connect.NewRequest(&telemetryv1.ReadRequest{Subject: "repo:agent-fleet"})
	none.Header().Set("Authorization", tok)
	if _, err := client.Read(ctx, none); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no face: %v", err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestTelemetryWatchEnds: a Watch ends on its own, with no error, at its life, a slow backend included (its frames
// say so); a frame comes on each tick.
func TestTelemetryWatchEnds(t *testing.T) {
	b := newBackend(t)
	a, srv, tok := telemetryServer(t, b)
	a.Telemetry.life = 700 * time.Millisecond
	a.Telemetry.every[telemetryv1.Face_FACE_LOGS] = 100 * time.Millisecond
	client := telemetryv1connect.NewTelemetryServiceClient(srv.Client(), srv.URL)
	for _, stall := range []bool{false, true} {
		b.set(stall, false)
		req := connect.NewRequest(&telemetryv1.WatchRequest{Subject: "repo:agent-fleet", Face: telemetryv1.Face_FACE_LOGS})
		req.Header().Set("Authorization", tok)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := time.Now()
		st, err := client.Watch(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		n, unknown := 0, 0
		for st.Receive() {
			n++
			if st.Msg().Frame.Unknown != "" {
				unknown++
			}
		}
		took := time.Since(start)
		cancel()
		if err := st.Err(); err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Errorf("stall %v: ended with %v (ctx %v)", stall, err, ctx.Err())
		}
		if took < 600*time.Millisecond || took > 1500*time.Millisecond {
			t.Errorf("stall %v: the Watch lasted %v, its life 700ms", stall, took)
		}
		if n < 2 || (stall && unknown != n) || (!stall && unknown != 0) {
			t.Errorf("stall %v: %d frames, %d unknown", stall, n, unknown)
		}
	}
	if a.Telemetry.life = newTelemetry(nil, nil).life; a.Telemetry.life >= 90*time.Second || a.Telemetry.life+a.Telemetry.budget >= 90*time.Second {
		t.Errorf("a Watch may outlive 90 s: %v + a %v read", a.Telemetry.life, a.Telemetry.budget)
	}
}

// TestTelemetryCanned: the sim's frames are the same for the same subject, differ between subjects, keep the faces'
// limits, and carry contract's series and an Explore URL.
func TestTelemetryCanned(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, face := range []telemetryv1.Face{telemetryv1.Face_FACE_METRICS, telemetryv1.Face_FACE_LOGS, telemetryv1.Face_FACE_SPANS} {
		a := canned.Frame("building:repo:agent-fleet", face, 0, "", now)
		if b := canned.Frame("building:repo:agent-fleet", face, 0, "", now); a.String() != b.String() {
			t.Errorf("%v: not deterministic", face)
		}
		if b := canned.Frame("building:app:mountain", face, 0, "", now); a.String() == b.String() {
			t.Errorf("%v: the same for two subjects", face)
		}
		if a.Unknown != "" || a.AtMs != now.UnixMilli() || a.ExploreUrl == "" || a.Window != telemetryv1.Window_WINDOW_1H {
			t.Errorf("%v: %v", face, a)
		}
	}
	m := canned.Frame("repo:x", telemetryv1.Face_FACE_METRICS, telemetryv1.Window_WINDOW_24H, "", now).GetMetrics().GetSeries()
	if len(m) != 4 || m[0].Name != contract.TelemetrySeries[0].Name || len(m[0].Points) != 72 {
		t.Errorf("canned metrics: %d series", len(m))
	}
	if l := canned.Frame("repo:x", telemetryv1.Face_FACE_LOGS, 0, "", now).GetLogs().GetLines(); len(l) != contract.TelemetryLogsMax {
		t.Errorf("canned logs: %d", len(l))
	}
	for _, l := range canned.Frame("repo:x", telemetryv1.Face_FACE_LOGS, 0, "route=/api/3", now).GetLogs().GetLines() {
		if !strings.Contains(l.Line, "route=/api/3") {
			t.Errorf("the filter let %q through", l.Line)
		}
	}
	if s := canned.Frame("repo:x", telemetryv1.Face_FACE_SPANS, 0, "", now).GetSpans().GetTraces(); len(s) != contract.TelemetrySpansMax {
		t.Errorf("canned spans: %d", len(s))
	}
}

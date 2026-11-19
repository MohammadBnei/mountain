package main

// atlas.telemetry.v1.TelemetryService (plan/25 ui-data.md): the right zone's Metrics, Logs and Spans faces for one
// subject, read on demand from Prometheus, Loki and Tempo. A subject (an atlas id, or the game's "building:<id>")
// joins to the Applications that deploy it through the graph (an app is itself; a repo is the apps its git edges
// name), and from there to the backends through contract's queries. Mounted behind the gate like every closed route;
// both methods are in machineRoutes (they write nothing).
//
// Fail closed: each source read has its budget (the spike's 1.5 s); a source down, slow or unconfigured is a frame
// with Unknown set (and, in a Watch, the last good data with its at_ms), never a hang. Each read's duration goes to
// mountain_telemetry_read_seconds by source. Under --live-fixture the frames are canned (api/telemetry/canned).

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/MohammadBnei/mountain/api/telemetry/canned"
	telemetryv1 "github.com/MohammadBnei/mountain/api/telemetry/v1"
	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/graph"
)

// Telemetry is the service. Its zero URLs are sources that answer unknown (the tests', until set).
type Telemetry struct {
	prom, loki, tempo string // base URLs, no trailing slash; "" = not configured
	grafana           string // the Explore links' base
	canned            bool   // --live-fixture: canned frames, no backend
	hc                *http.Client
	idx               func() *GraphIndex
	met               *Metrics
	budget            time.Duration                      // one source read
	life              time.Duration                      // a Watch's length: under Cloudflare's 100 s and the clients' 90 s rule
	every             map[telemetryv1.Face]time.Duration // a Watch's cadence per face
}

func newTelemetry(idx func() *GraphIndex, met *Metrics) *Telemetry {
	return &Telemetry{hc: &http.Client{}, idx: idx, met: met, grafana: canned.Grafana, budget: 1500 * time.Millisecond, life: 85 * time.Second,
		every: map[telemetryv1.Face]time.Duration{telemetryv1.Face_FACE_METRICS: liveEvery, telemetryv1.Face_FACE_LOGS: 5 * time.Second, telemetryv1.Face_FACE_SPANS: liveEvery}}
}

// telemetryTargets joins a subject to the Applications whose telemetry it reads, or says why there are none.
func telemetryTargets(idx *GraphIndex, subject string) ([]contract.TelemetryTarget, string) {
	id := strings.TrimPrefix(subject, "building:")
	if idx == nil {
		return nil, "atlas has no graph yet"
	}
	if !idx.Has[id] {
		return nil, "no such node: " + id
	}
	var apps []string
	switch k := idx.Kind[id]; k {
	case graph.KindApp:
		apps = []string{id}
	case graph.KindRepo:
		apps = idx.Deploy[id]
	default:
		return nil, "no telemetry for a " + k
	}
	var ts []contract.TelemetryTarget
	for _, a := range apps {
		if t := (contract.TelemetryTarget{Namespace: idx.AppNS[a], App: strings.TrimPrefix(a, "app:")}); contract.TelemetryTargetOK(t) {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		return nil, id + " deploys no Application in the cluster"
	}
	return ts, ""
}

// faceArgs checks a request's face, window and filter; the window defaults to an hour.
func faceArgs(face telemetryv1.Face, window telemetryv1.Window, contains string) (telemetryv1.Window, error) {
	if face == telemetryv1.Face_FACE_UNSPECIFIED || telemetryv1.Face_name[int32(face)] == "" {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("face: metrics, logs or spans"))
	}
	if window == telemetryv1.Window_WINDOW_UNSPECIFIED {
		window = telemetryv1.Window_WINDOW_1H
	}
	if _, ok := contract.TelemetryWindows[window.String()]; !ok {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("window: 15m, 1h or 24h"))
	}
	if len(contains) > contract.TelemetryContainsMax {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contains: at most %d bytes", contract.TelemetryContainsMax))
	}
	return window, nil
}

func (t *Telemetry) Read(ctx context.Context, req *connect.Request[telemetryv1.ReadRequest]) (*connect.Response[telemetryv1.ReadResponse], error) {
	m := req.Msg
	w, err := faceArgs(m.Face, m.Window, m.Contains)
	if err != nil {
		return nil, err
	}
	res := connect.NewResponse(&telemetryv1.ReadResponse{Frame: t.frame(ctx, m.Subject, m.Face, w, m.Contains, nil)})
	res.Header().Set("Cache-Control", "private, no-store") // a GET: no cache keeps an admin's answer
	return res, nil
}

// Watch sends a frame now and on the face's cadence until its end: t.life, or sooner the caller's token or session
// end. A sign-out ends it at the next tick. It ends with no error; the client resumes it.
func (t *Telemetry) Watch(ctx context.Context, req *connect.Request[telemetryv1.WatchRequest], stream *connect.ServerStream[telemetryv1.WatchResponse]) error {
	m := req.Msg
	w, err := faceArgs(m.Face, m.Window, m.Contains)
	if err != nil {
		return err
	}
	end := time.Now().Add(t.life)
	if e, ok := machineExp(ctx); ok && e.Before(end) {
		end = e
	}
	if e, ok := sessionEnd(ctx); ok && e.Before(end) {
		end = e
	}
	live := sessionLive(ctx)
	ctx, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	tick := time.NewTicker(t.every[m.Face])
	defer tick.Stop()
	var last *telemetryv1.Frame
	for {
		last = t.frame(ctx, m.Subject, m.Face, w, m.Contains, last)
		if ctx.Err() != nil {
			return nil // the end cut the read: no frame past it
		}
		if err := stream.Send(&telemetryv1.WatchResponse{Frame: last}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		if live != nil && !live() {
			return connect.NewError(connect.CodeUnauthenticated, errors.New("signed out"))
		}
	}
}

// frame reads one face. On a failed read it keeps last's data and age, with the reason.
func (t *Telemetry) frame(ctx context.Context, subject string, face telemetryv1.Face, window telemetryv1.Window, contains string, last *telemetryv1.Frame) *telemetryv1.Frame {
	if t.canned {
		return canned.Frame(subject, face, window, contains, time.Now())
	}
	f := &telemetryv1.Frame{Subject: subject, Face: face, Window: window}
	ts, why := telemetryTargets(t.idx(), subject)
	if why != "" {
		f.Unknown = why
		return f
	}
	w := contract.TelemetryWindows[window.String()]
	now := time.Now()
	var err error
	switch face {
	case telemetryv1.Face_FACE_METRICS:
		qs := contract.TelemetryMetrics(ts, w)
		f.ExploreUrl = contract.ExploreURL(t.grafana, contract.GrafanaProm, qs, w)
		var m *telemetryv1.Metrics
		if m, err = t.metrics(ctx, qs, w, now); err == nil {
			f.Data = &telemetryv1.Frame_Metrics{Metrics: m}
		}
	case telemetryv1.Face_FACE_LOGS:
		q := contract.TelemetryLogs(ts, contains)
		f.ExploreUrl = contract.ExploreURL(t.grafana, contract.GrafanaLoki, []string{q}, w)
		var l *telemetryv1.Logs
		if l, err = t.logs(ctx, q, w, now); err == nil {
			f.Data = &telemetryv1.Frame_Logs{Logs: l}
		}
	case telemetryv1.Face_FACE_SPANS:
		q := contract.TelemetrySpans(ts)
		f.ExploreUrl = contract.ExploreURL(t.grafana, contract.GrafanaTempo, []string{q}, w)
		var s *telemetryv1.Spans
		if s, err = t.spans(ctx, q, w, now); err == nil {
			f.Data = &telemetryv1.Frame_Spans{Spans: s}
		}
	}
	if err != nil {
		f.Unknown = err.Error()
		if last != nil && last.Data != nil {
			f.AtMs, f.Data = last.AtMs, last.Data
		}
		return f
	}
	f.AtMs = now.UnixMilli()
	return f
}

// get is one source read within the budget, its duration recorded; the answer is JSON into out.
func (t *Telemetry) get(ctx context.Context, source, base, path string, q url.Values, out any) error {
	if base == "" {
		return errors.New(source + " is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, t.budget)
	defer cancel()
	start := time.Now()
	defer func() { t.met.ReadDuration(source, time.Since(start).Seconds()) }()
	req, err := http.NewRequestWithContext(ctx, "GET", base+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}
	resp, err := t.hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%s: no answer within %v", source, t.budget)
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // not the URL: the query is in it
		}
		return fmt.Errorf("%s: %v", source, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", source, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return fmt.Errorf("%s: no answer within %v", source, t.budget)
		}
		return fmt.Errorf("%s: bad answer: %v", source, err)
	}
	return nil
}

// metrics reads the series in parallel; one failing fails the face.
func (t *Telemetry) metrics(ctx context.Context, qs []string, w contract.TelemetryWindow, now time.Time) (*telemetryv1.Metrics, error) {
	out := &telemetryv1.Metrics{Series: make([]*telemetryv1.Series, len(qs))}
	errs := make([]error, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Go(func() {
			var r struct {
				Status string
				Data   struct{ Result []struct{ Values [][2]any } }
			}
			v := url.Values{"query": {q}, "start": {unix(now.Add(-w.Span))}, "end": {unix(now)}, "step": {strconv.Itoa(int(w.Step.Seconds()))}}
			if errs[i] = t.get(ctx, "prom", t.prom, contract.PromRangePath, v, &r); errs[i] == nil && r.Status != "success" {
				errs[i] = fmt.Errorf("prom: status %q", r.Status)
			}
			s := &telemetryv1.Series{Name: contract.TelemetrySeries[i].Name, Unit: contract.TelemetrySeries[i].Unit}
			if len(r.Data.Result) > 0 { // each query sums to one series; none = no traffic, an empty series
				for _, p := range r.Data.Result[0].Values {
					at, _ := p[0].(float64)
					str, _ := p[1].(string)
					if x, err := strconv.ParseFloat(str, 64); err == nil && !math.IsNaN(x) && !math.IsInf(x, 0) {
						s.Points = append(s.Points, &telemetryv1.Point{AtMs: int64(at * 1000), Value: x})
					}
				}
			}
			out.Series[i] = s
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err // the first: the four share one source
		}
	}
	return out, nil
}

// logs reads the last lines, newest first, across the streams.
func (t *Telemetry) logs(ctx context.Context, q string, w contract.TelemetryWindow, now time.Time) (*telemetryv1.Logs, error) {
	var r struct {
		Data struct {
			Result []struct {
				Stream map[string]string
				Values [][2]string
			}
		}
	}
	v := url.Values{"query": {q}, "start": {strconv.FormatInt(now.Add(-w.Span).UnixNano(), 10)}, "end": {strconv.FormatInt(now.UnixNano(), 10)},
		"limit": {strconv.Itoa(contract.TelemetryLogsMax)}, "direction": {"backward"}}
	if err := t.get(ctx, "loki", t.loki, contract.LokiRangePath, v, &r); err != nil {
		return nil, err
	}
	out := &telemetryv1.Logs{}
	for _, s := range r.Data.Result {
		for _, x := range s.Values {
			ns, err := strconv.ParseInt(x[0], 10, 64)
			if err != nil {
				continue
			}
			out.Lines = append(out.Lines, &telemetryv1.LogLine{AtMs: ns / 1e6, Line: x[1], Level: s.Stream["detected_level"], Service: s.Stream["service_name"]})
		}
	}
	slices.SortStableFunc(out.Lines, func(a, b *telemetryv1.LogLine) int { return cmp.Compare(b.AtMs, a.AtMs) })
	out.Lines = out.Lines[:min(len(out.Lines), contract.TelemetryLogsMax)]
	return out, nil
}

// spans reads the last traces, newest first.
func (t *Telemetry) spans(ctx context.Context, q string, w contract.TelemetryWindow, now time.Time) (*telemetryv1.Spans, error) {
	var r struct {
		Traces []struct {
			TraceID, RootServiceName, RootTraceName, StartTimeUnixNano string
			DurationMs                                                 int64
		}
	}
	v := url.Values{"q": {q}, "limit": {strconv.Itoa(contract.TelemetrySpansMax)}, "start": {unix(now.Add(-w.Span))}, "end": {unix(now)}}
	if err := t.get(ctx, "tempo", t.tempo, contract.TempoSearchPath, v, &r); err != nil {
		return nil, err
	}
	out := &telemetryv1.Spans{}
	for _, x := range r.Traces {
		ns, _ := strconv.ParseInt(x.StartTimeUnixNano, 10, 64)
		out.Traces = append(out.Traces, &telemetryv1.Trace{TraceId: x.TraceID, RootService: x.RootServiceName, RootName: x.RootTraceName, StartMs: ns / 1e6, DurationMs: x.DurationMs})
	}
	slices.SortStableFunc(out.Traces, func(a, b *telemetryv1.Trace) int { return cmp.Compare(b.StartMs, a.StartMs) })
	out.Traces = out.Traces[:min(len(out.Traces), contract.TelemetrySpansMax)]
	return out, nil
}

func unix(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }

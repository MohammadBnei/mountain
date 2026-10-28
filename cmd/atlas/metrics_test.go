package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// scrape is one GET of the exposition, as Prometheus 2 makes it (no Accept): the status, Content-Type and body.
func scrape(t *testing.T, m *Metrics) (int, string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, w.Header().Get("Content-Type"), string(b)
}

const (
	helpDrift = "# HELP mountain_drift Drift in the latest projection, by kind (unassigned = a component realms.yaml does not place).\n# TYPE mountain_drift gauge\n"
	helpToken = "# HELP mountain_github_token_days_left Days until the GitHub token expires.\n# TYPE mountain_github_token_days_left gauge\n"
	helpSeq   = "# HELP mountain_projection_seq The latest projection's sequence number.\n# TYPE mountain_projection_seq gauge\n"
	helpQuery = "# HELP mountain_query_failed_total Failed PromQL queries (the poll keeps its last answer and goes stale).\n# TYPE mountain_query_failed_total counter\n"
	helpSrc   = "# HELP mountain_source_last_success Unix time of the last success per source (prom, alerts, github, projection, db); seeded at start.\n# TYPE mountain_source_last_success gauge\n"
	helpWS    = "# HELP mountain_ws_connections Open /ws connections.\n# TYPE mountain_ws_connections gauge\n"
)

// TestMetricsExposition pins the exposition byte for byte: the six names, their types, HELP texts and label keys,
// the number format (the shortest %g, as the hand-written text had), and nothing the exporter would add by default
// (target_info, otel_scope_* labels, a unit or a second _total). A family nothing has written yet is absent, except
// the websocket count, which starts at 0.
func TestMetricsExposition(t *testing.T) {
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	code, ct, body := scrape(t, m)
	if want := helpWS + "mountain_ws_connections 0\n"; body != want {
		t.Errorf("a fresh exposition:\n got %q\nwant %q", body, want)
	}
	if code != http.StatusOK || !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Errorf("status %d, Content-Type %q", code, ct)
	}

	m.SourceOK("prom", 1761650000)
	m.SourceOK("db", 1761650001)
	m.TokenDays(11.5)
	m.WSConns(1)
	m.WSConns(1)
	m.WSConns(-1)
	m.QueryFailure("info")
	m.QueryFailure("info")
	m.QueryFailure("pods")
	m.ProjectionSeq(7)
	m.DriftCount("unresolved", 106)
	m.DriftCount("mirror", 0)
	want := helpDrift + "mountain_drift{kind=\"mirror\"} 0\nmountain_drift{kind=\"unresolved\"} 106\n" +
		helpToken + "mountain_github_token_days_left 11.5\n" +
		helpSeq + "mountain_projection_seq 7\n" +
		helpQuery + "mountain_query_failed_total{query=\"info\"} 2\nmountain_query_failed_total{query=\"pods\"} 1\n" +
		helpSrc + "mountain_source_last_success{source=\"db\"} 1.761650001e+09\nmountain_source_last_success{source=\"prom\"} 1.76165e+09\n" +
		helpWS + "mountain_ws_connections 1\n"
	if _, _, body := scrape(t, m); body != want {
		t.Errorf("the exposition:\n got %q\nwant %q", body, want)
	}

	// a gauge keeps every label's last value across scrapes, and a later write moves only its own series
	m.SourceOK("prom", 1761650015)
	m.WSConns(-1)
	_, _, again := scrape(t, m)
	want = strings.Replace(want, "mountain_source_last_success{source=\"prom\"} 1.76165e+09", "mountain_source_last_success{source=\"prom\"} 1.761650015e+09", 1)
	want = strings.Replace(want, "mountain_ws_connections 1", "mountain_ws_connections 0", 1)
	if again != want {
		t.Errorf("the second scrape:\n got %q\nwant %q", again, want)
	}
}

// TestMetricsLabelViews: a series carries only its own label key, whatever a writer records it with (the views pin the
// label set). The positive control is the allowed key, which stays.
func TestMetricsLabelViews(t *testing.T) {
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx := context.Background()
	extra := attribute.String("extra", "y")
	m.drift.Record(ctx, 7, metric.WithAttributes(attribute.String("kind", "mirror"), extra))
	m.queryFailed.Add(ctx, 1, metric.WithAttributes(attribute.String("query", "info"), extra))
	m.sourceLastSuccess.Record(ctx, 3, metric.WithAttributes(attribute.String("source", "db"), extra))
	m.projectionSeq.Record(ctx, 2, metric.WithAttributes(attribute.String("kind", "stray"))) // a series with no label has none
	m.tokenDaysLeft.Record(ctx, 5, metric.WithAttributes(extra))
	m.wsConnections.Add(ctx, 1, metric.WithAttributes(extra))
	_, _, body := scrape(t, m)
	for _, line := range []string{"mountain_drift{kind=\"mirror\"} 7\n", "mountain_query_failed_total{query=\"info\"} 1\n",
		"mountain_source_last_success{source=\"db\"} 3\n", "mountain_projection_seq 2\n", "mountain_github_token_days_left 5\n", "mountain_ws_connections 1\n"} {
		if !strings.Contains(body, line) {
			t.Errorf("missing %q in\n%s", line, body)
		}
	}
	if strings.Contains(body, "extra") || strings.Contains(body, "stray") {
		t.Errorf("a label outside the series' own got through:\n%s", body)
	}
}

// TestMetricsNil: without metrics (`atlas project`, a test that does not read them) every writer is a no-op, and there
// is no exposition.
func TestMetricsNil(t *testing.T) {
	var m *Metrics
	m.SourceOK("db", 1)
	m.TokenDays(1)
	m.WSConns(1)
	m.QueryFailure("x")
	m.ProjectionSeq(1)
	m.DriftCount("mirror", 1)
	if err := m.Close(); err != nil {
		t.Error(err)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("nil metrics answered %d", w.Code)
	}
}

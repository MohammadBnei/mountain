package main

// Self-observability on :9090 (plan/15 §7): OpenTelemetry's metrics API, read back through its Prometheus exporter
// onto today's /metrics, with the series exactly as they were: the same six names and types, HELP texts and label
// keys, no unit; and one histogram since, the telemetry faces' read duration per source (telemetry.go). The exporter's own additions are off: no target_info, no otel_scope_* labels, no
// _total or unit suffix (the counter's name already ends in _total), and its own registry, so no go_* or process_*
// series. Metrics is the App's, not a package global: the writers hold it and call its methods; a nil *Metrics
// (`atlas project`, a test that does not read them) records nothing.
//
// No OTel global is set (no otel.SetMeterProvider): the provider is passed to its one reader, so nothing else, a
// library's own instrumentation included, can add a series here.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// The label keys of the series that have one; the rest have none.
const (
	labelSource = "source"
	labelQuery  = "query"
	labelKind   = "kind"
	labelType   = "type"
)

// Metrics is atlas's eleven series: seven gauges, three counters and one histogram.
type Metrics struct {
	provider *sdkmetric.MeterProvider
	handler  http.Handler

	sourceLastSuccess metric.Float64Gauge
	wsConnections     metric.Float64UpDownCounter // exposed as a gauge: a sum that is not monotonic
	queryFailed       metric.Float64Counter
	projectionSeq     metric.Float64Gauge
	drift             metric.Float64Gauge
	readSeconds       metric.Float64Histogram

	worldEnabled, worldOldest metric.Float64Gauge
	worldPublished            metric.Float64Counter
	worldRefused              metric.Float64Counter

	feedEnabled, feedEvents, feedThreads, feedAge metric.Float64Gauge
	feedDropped, feedBad                          metric.Float64Counter
}

// series is one metric: its name, HELP text and the one label key it may carry ("" = none). The views pin the label
// set from newMetrics's table of them, the instruments take the name and the text from it. The table is locals there,
// not package variables.
type series struct{ name, help, label string }

// newMetrics builds the provider, its views and the exporter, and the instruments. The websocket count starts at 0
// (a series from the first scrape, as it has always been); every other series appears with its first write.
func newMetrics() (*Metrics, error) {
	// No token-expiry gauge: mountain_github_token_days_left went with the GitHub poll (P4.5). A Forgejo access token
	// has no expiry (Forgejo 15's AccessToken carries none), so a dead token shows only as forge's last success ageing.
	sSourceLastSuccess := series{"mountain_source_last_success", "Unix time of the last success per source (prom, alerts, forge, projection, db, bridge-tags, bridge-apps, world); seeded at start.", labelSource}
	sWSConnections := series{"mountain_ws_connections", "Open /ws connections.", ""}
	sQueryFailed := series{"mountain_query_failed_total", "Failed PromQL queries (the poll keeps its last answer and goes stale).", labelQuery}
	sProjectionSeq := series{"mountain_projection_seq", "The latest projection's sequence number.", ""}
	sDrift := series{"mountain_drift", "Drift in the latest projection, by kind (unassigned = a component realms.yaml does not place).", labelKind}
	sReadSeconds := series{"mountain_telemetry_read_seconds", "Seconds one telemetry face read took, by source (prom, loki, tempo), failed and timed-out reads included.", labelSource}
	sWorldEnabled := series{"mountain_world_enabled", "1 when world events are published (a database and the bridge are configured), else 0.", ""}
	sWorldOldest := series{"mountain_world_outbox_oldest_seconds", "Age of the oldest world event still in the outbox, 0 when it is empty.", ""}
	sWorldPublished := series{"mountain_world_events_published_total", "World events the bridge took (duplicates not counted), by type.", labelType}
	sWorldRefused := series{"mountain_world_events_refused_total", "World events the bridge refused for good (InvalidArgument) and atlas parked.", ""}
	sFeedEnabled := series{"mountain_feed_enabled", "1 when the feed runs (FEED_ENABLED), else 0.", ""}
	sFeedEvents := series{"mountain_feed_events", "Events the feed holds.", ""}
	sFeedThreads := series{"mountain_feed_threads", "Threads the feed holds.", ""}
	sFeedDropped := series{"mountain_feed_dropped_total", "Events the feed dropped for its size caps, oldest first.", ""}
	sFeedBad := series{"mountain_feed_bad_events_total", "Events whose handling panicked in the feed; each was logged, acked and skipped.", ""}
	sFeedAge := series{"mountain_feed_last_event_age_seconds", "Seconds since the feed last stored an event (since its start while it has none).", ""}
	reg := prometheus.NewRegistry()
	exp, err := otelprom.New(
		otelprom.WithRegisterer(reg),
		otelprom.WithoutTargetInfo(),
		otelprom.WithoutScopeInfo(),
		otelprom.WithTranslationStrategy(otlptranslator.UnderscoreEscapingWithoutSuffixes))
	if err != nil {
		return nil, err
	}
	var views []sdkmetric.View
	for _, s := range []series{sSourceLastSuccess, sWSConnections, sQueryFailed, sProjectionSeq, sDrift, sWorldEnabled, sWorldOldest, sWorldPublished, sWorldRefused, sFeedEnabled, sFeedEvents, sFeedThreads, sFeedDropped, sFeedBad, sFeedAge} {
		views = append(views, labelView(s))
	}
	// seconds, around the spike's 25-90 ms reads and the 1.5 s budget
	views = append(views, sdkmetric.NewView(sdkmetric.Instrument{Name: sReadSeconds.name}, sdkmetric.Stream{AttributeFilter: attribute.NewAllowKeysFilter(labelSource),
		Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 1.5, 3}}}))
	m := &Metrics{provider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp), sdkmetric.WithResource(resource.Empty()), sdkmetric.WithView(views...))}
	meter := m.provider.Meter("atlas")
	var errs []error
	gauge := func(s series) metric.Float64Gauge {
		g, e := meter.Float64Gauge(s.name, metric.WithDescription(s.help))
		errs = append(errs, e)
		return g
	}
	m.sourceLastSuccess, m.projectionSeq, m.drift = gauge(sSourceLastSuccess), gauge(sProjectionSeq), gauge(sDrift)
	m.worldEnabled, m.worldOldest = gauge(sWorldEnabled), gauge(sWorldOldest)
	counter := func(s series) metric.Float64Counter {
		c, e := meter.Float64Counter(s.name, metric.WithDescription(s.help))
		errs = append(errs, e)
		return c
	}
	m.worldPublished, m.worldRefused = counter(sWorldPublished), counter(sWorldRefused)
	m.feedEnabled, m.feedEvents, m.feedThreads, m.feedAge = gauge(sFeedEnabled), gauge(sFeedEvents), gauge(sFeedThreads), gauge(sFeedAge)
	m.feedDropped, m.feedBad = counter(sFeedDropped), counter(sFeedBad)
	var e1, e2, e3 error
	m.wsConnections, e1 = meter.Float64UpDownCounter(sWSConnections.name, metric.WithDescription(sWSConnections.help))
	m.queryFailed, e2 = meter.Float64Counter(sQueryFailed.name, metric.WithDescription(sQueryFailed.help))
	m.readSeconds, e3 = meter.Float64Histogram(sReadSeconds.name, metric.WithDescription(sReadSeconds.help))
	if err := errors.Join(append(errs, e1, e2, e3)...); err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	m.wsConnections.Add(context.Background(), 0)
	m.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{DisableCompression: true, ErrorHandling: promhttp.ContinueOnError})
	return m, nil
}

// labelView lets only s's own label key through to the exposition: the label set of a series is part of its contract.
func labelView(s series) sdkmetric.View {
	keys := []attribute.Key{}
	if s.label != "" {
		keys = append(keys, attribute.Key(s.label))
	}
	return sdkmetric.NewView(sdkmetric.Instrument{Name: s.name}, sdkmetric.Stream{AttributeFilter: attribute.NewAllowKeysFilter(keys...)})
}

// Handler serves the exposition; on a nil *Metrics, a 404 handler.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return m.handler
}

// Close stops the provider; nil-safe.
func (m *Metrics) Close() error {
	if m == nil {
		return nil
	}
	return m.provider.Shutdown(context.Background())
}

// The writers. Each takes the label value ("" series have none) and is a no-op on a nil *Metrics.

// SourceOK records the unix time of source's last success.
func (m *Metrics) SourceOK(source string, unix float64) {
	if m != nil {
		m.sourceLastSuccess.Record(context.Background(), unix, metric.WithAttributes(attribute.String(labelSource, source)))
	}
}

// WSConns adds d (+1, -1) to the open websocket count.
func (m *Metrics) WSConns(d float64) {
	if m != nil {
		m.wsConnections.Add(context.Background(), d)
	}
}

// QueryFailure counts one failed PromQL query, by its name.
func (m *Metrics) QueryFailure(name string) {
	if m != nil {
		m.queryFailed.Add(context.Background(), 1, metric.WithAttributes(attribute.String(labelQuery, name)))
	}
}

// ProjectionSeq records the latest projection's sequence number.
func (m *Metrics) ProjectionSeq(seq float64) {
	if m != nil {
		m.projectionSeq.Record(context.Background(), seq)
	}
}

// DriftCount records the number of drift entries of kind in the latest projection.
func (m *Metrics) DriftCount(kind string, n float64) {
	if m != nil {
		m.drift.Record(context.Background(), n, metric.WithAttributes(attribute.String(labelKind, kind)))
	}
}

// ReadDuration records one telemetry read from source, in seconds.
func (m *Metrics) ReadDuration(source string, secs float64) {
	if m != nil {
		m.readSeconds.Record(context.Background(), secs, metric.WithAttributes(attribute.String(labelSource, source)))
	}
}

// WorldEnabled records whether world events are published.
func (m *Metrics) WorldEnabled(on bool) {
	if m != nil {
		v := 0.0
		if on {
			v = 1
		}
		m.worldEnabled.Record(context.Background(), v)
	}
}

// WorldOutboxOldest records the age in seconds of the oldest event in the outbox.
func (m *Metrics) WorldOutboxOldest(secs float64) {
	if m != nil {
		m.worldOldest.Record(context.Background(), secs)
	}
}

// WorldPublished counts one world event of typ the bridge took.
func (m *Metrics) WorldPublished(typ string) {
	if m != nil {
		m.worldPublished.Add(context.Background(), 1, metric.WithAttributes(attribute.String(labelType, typ)))
	}
}

// WorldRefused counts one world event the bridge refused for good.
func (m *Metrics) WorldRefused() {
	if m != nil {
		m.worldRefused.Add(context.Background(), 1)
	}
}

// FeedEnabled records whether the feed runs.
func (m *Metrics) FeedEnabled(on bool) {
	if m != nil {
		v := 0.0
		if on {
			v = 1
		}
		m.feedEnabled.Record(context.Background(), v)
	}
}

// FeedState records what the feed holds, the events dropped and the bad ones since the last call, and the seconds since its last event.
func (m *Metrics) FeedState(events, threads int, dropped, bad uint64, ageSecs float64) {
	if m != nil {
		ctx := context.Background()
		m.feedEvents.Record(ctx, float64(events))
		m.feedThreads.Record(ctx, float64(threads))
		m.feedAge.Record(ctx, ageSecs)
		m.feedDropped.Add(ctx, float64(dropped))
		m.feedBad.Add(ctx, float64(bad))
	}
}

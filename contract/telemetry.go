package contract

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The telemetry faces (plan/25 ui-data.md, atlas.telemetry.v1): what atlas asks Prometheus, Loki and Tempo about one
// subject, joined to them through the Applications that deploy it. The sim answers these faces with canned frames
// (api/telemetry/canned), it does not evaluate the queries.
//
// The join: an Application is its namespace and its name. Traefik names its services "<namespace>-<app>-…", as the
// live frame's appOfService reads them; Loki streams carry the namespace; Tempo spans carry the service.name the app
// sets, which is the Application's name but for TraceService's exceptions. A namespace shared by several apps reads
// all their logs: the namespace is the one label every stream has.

// TelemetryTarget is one Application a subject's telemetry reads.
type TelemetryTarget struct {
	Namespace string
	App       string // the ArgoCD Application's name
}

// TraceService is the service.name an Application's spans carry, where it is not the Application's name.
var TraceService = map[string]string{
	"mountain": "atlas", // telemetry.New(..., "atlas")
}

// The faces' limits (ui-data.md "The faces' limits"): what one read may cost.
const (
	TelemetryLogsMax     = 200 // the last 200 lines
	TelemetrySpansMax    = 20  // the last 20 traces
	TelemetryContainsMax = 200 // bytes of a logs text filter
)

// TelemetryWindow is one of the faces' windows: how far back, the range query's step, and the rate() range (at
// least 2 m, the live frame's, and at least the step so no sample is skipped).
type TelemetryWindow struct {
	Span, Step, Rate time.Duration
	Grafana          string // Explore's relative "from"
}

// TelemetryWindows are the windows, by atlas.telemetry.v1.Window's name; each gives 60 to 72 points.
var TelemetryWindows = map[string]TelemetryWindow{
	"WINDOW_15M": {15 * time.Minute, 15 * time.Second, 2 * time.Minute, "now-15m"},
	"WINDOW_1H":  {time.Hour, time.Minute, 2 * time.Minute, "now-1h"},
	"WINDOW_24H": {24 * time.Hour, 20 * time.Minute, 20 * time.Minute, "now-24h"},
}

// TelemetrySeries is the Metrics face's series, in order (at most 4), and the unit of each.
var TelemetrySeries = []struct{ Name, Unit string }{
	{"requests", "req/s"},
	{"errors", "req/s"},
	{"latency_p95", "s"},
	{"pods", "pods"},
}

// The Prometheus, Loki and Tempo routes the faces read.
const (
	PromRangePath   = "/api/v1/query_range" // query, start, end, step (unix seconds)
	LokiRangePath   = "/loki/api/v1/query_range"
	TempoSearchPath = "/api/search" // q (TraceQL), limit, start, end (unix seconds)
)

// Grafana's datasource uids (infra-bootstrap gitops/platform/values/grafana/values.yaml), for the Explore URLs.
const (
	GrafanaProm  = "ds-prometheus"
	GrafanaLoki  = "ds-loki"
	GrafanaTempo = "ds-tempo"
)

// dnsLabel is a name that goes into a query unquoted: a namespace or an Application's name. Anything else (a dot, a
// quote) is no target, so no name can break out of its matcher.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// TelemetryTargetOK is whether t's names are safe to join on.
func TelemetryTargetOK(t TelemetryTarget) bool {
	return dnsLabel.MatchString(t.Namespace) && dnsLabel.MatchString(t.App)
}

func alt(ts []TelemetryTarget, f func(TelemetryTarget) string) string {
	seen, out := map[string]bool{}, []string{}
	for _, t := range ts {
		if s := f(t); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, "|")
}

// TelemetryMetrics is the Metrics face's PromQL for ts in w, one per TelemetrySeries. ts are TelemetryTargetOK.
func TelemetryMetrics(ts []TelemetryTarget, w TelemetryWindow) []string {
	svc := `exported_service=~"(` + alt(ts, func(t TelemetryTarget) string { return t.Namespace + "-" + t.App }) + `)-.*"`
	ns := `namespace=~"` + alt(ts, func(t TelemetryTarget) string { return t.Namespace }) + `"`
	r := "[" + promDuration(w.Rate) + "]"
	return []string{
		`sum(rate(traefik_service_requests_total{` + svc + `}` + r + `))`,
		`sum(rate(traefik_service_requests_total{` + svc + `,code=~"5.."}` + r + `)) or 0 * sum(rate(traefik_service_requests_total{` + svc + `}` + r + `))`,
		`histogram_quantile(0.95, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{` + svc + `}` + r + `)))`,
		`sum(kube_pod_status_phase{` + ns + `,phase="Running"})`,
	}
}

// TelemetryLogs is the Logs face's LogQL for ts: their namespaces' streams, filtered by a line match when contains is
// set (the spike: a line filter costs far less than a detected_level one).
func TelemetryLogs(ts []TelemetryTarget, contains string) string {
	q := `{namespace=~"` + alt(ts, func(t TelemetryTarget) string { return t.Namespace }) + `"}`
	if contains != "" {
		q += ` |= ` + strconv.Quote(contains) // LogQL strings unquote as Go's do
	}
	return q
}

// TelemetrySpans is the Spans face's TraceQL for ts.
func TelemetrySpans(ts []TelemetryTarget) string {
	return `{ resource.service.name =~ "` + alt(ts, func(t TelemetryTarget) string {
		if s, ok := TraceService[t.App]; ok {
			return s
		}
		return t.App
	}) + `" }`
}

func promDuration(d time.Duration) string { return strconv.Itoa(int(d.Seconds())) + "s" }

// ExploreURL is Grafana Explore at base for exprs on the datasource uid (GrafanaProm, GrafanaLoki or GrafanaTempo)
// over w.
func ExploreURL(base, uid string, exprs []string, w TelemetryWindow) string {
	typ := map[string]string{GrafanaProm: "prometheus", GrafanaLoki: "loki", GrafanaTempo: "tempo"}[uid]
	qs := []map[string]any{}
	for i, e := range exprs {
		q := map[string]any{"refId": string(rune('A' + i)), "datasource": map[string]string{"type": typ, "uid": uid}}
		if uid == GrafanaTempo {
			q["queryType"], q["query"], q["limit"] = "traceql", e, TelemetrySpansMax
		} else {
			q["expr"] = e
		}
		qs = append(qs, q)
	}
	panes, _ := json.Marshal(map[string]any{"a": map[string]any{"datasource": uid, "queries": qs, "range": map[string]string{"from": w.Grafana, "to": "now"}}})
	return strings.TrimRight(base, "/") + "/explore?schemaVersion=1&orgId=1&panes=" + url.QueryEscape(string(panes))
}

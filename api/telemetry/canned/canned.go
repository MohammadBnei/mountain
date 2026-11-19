// Package canned answers atlas.telemetry.v1 frames without a cluster: the same subject, face and window always give
// the same data (seeded by the subject), stamped at now. ukubi-game's sim serves them, and atlas does under
// --live-fixture. It evaluates no query; the series names, limits and Explore URL are contract's, so a frame looks
// like atlas's own.
package canned

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	telemetryv1 "github.com/MohammadBnei/mountain/api/telemetry/v1"
	"github.com/MohammadBnei/mountain/contract"
)

// Grafana is the Explore base the canned frames link to.
const Grafana = "https://grafana.bnei.dev"

// Frame is the canned frame for subject's face over window, at now. contains filters the canned log lines as atlas's
// line filter would.
func Frame(subject string, face telemetryv1.Face, window telemetryv1.Window, contains string, now time.Time) *telemetryv1.Frame {
	if window == telemetryv1.Window_WINDOW_UNSPECIFIED {
		window = telemetryv1.Window_WINDOW_1H
	}
	w := contract.TelemetryWindows[window.String()]
	h := fnv.New64a()
	_, _ = h.Write([]byte(subject))
	seed := h.Sum64()
	name := subject[strings.LastIndex(subject, ":")+1:] // ponytail: the sim has no graph; the subject's last name stands in for its app
	ts := []contract.TelemetryTarget{{Namespace: name, App: name}}
	f := &telemetryv1.Frame{Subject: subject, Face: face, Window: window, AtMs: now.UnixMilli()}
	switch face {
	case telemetryv1.Face_FACE_METRICS:
		m := &telemetryv1.Metrics{}
		base := 1 + float64(seed%40)
		for i, s := range contract.TelemetrySeries {
			ser := &telemetryv1.Series{Name: s.Name, Unit: s.Unit}
			n := int(w.Span / w.Step)
			for p := range n {
				x := float64(p)/8 + float64(seed>>(8*i)%16)
				var v float64
				switch i {
				case 0:
					v = base * (1 + 0.3*math.Sin(x))
				case 1:
					v = max(0, 0.05*math.Sin(3*x))
				case 2:
					v = 0.04 + 0.02*math.Cos(x)
				default:
					v = float64(1 + seed%3)
				}
				ser.Points = append(ser.Points, &telemetryv1.Point{AtMs: now.Add(-w.Span + time.Duration(p+1)*w.Step).UnixMilli(), Value: math.Round(v*1000) / 1000})
			}
			m.Series = append(m.Series, ser)
		}
		f.Data = &telemetryv1.Frame_Metrics{Metrics: m}
		f.ExploreUrl = contract.ExploreURL(Grafana, contract.GrafanaProm, contract.TelemetryMetrics(ts, w), w)
	case telemetryv1.Face_FACE_LOGS:
		l := &telemetryv1.Logs{}
		levels := []string{"info", "info", "info", "debug", "warn", "error"}
		for i := range contract.TelemetryLogsMax {
			line := fmt.Sprintf(`msg="request served" route=/api/%d status=200 ms=%d`, (seed+uint64(i))%7, (seed+uint64(i)*13)%90)
			if contains != "" && !strings.Contains(line, contains) {
				continue
			}
			l.Lines = append(l.Lines, &telemetryv1.LogLine{AtMs: now.Add(-time.Duration(i*7) * time.Second).UnixMilli(), Line: line,
				Level: levels[(seed+uint64(i))%uint64(len(levels))], Service: name})
		}
		f.Data = &telemetryv1.Frame_Logs{Logs: l}
		f.ExploreUrl = contract.ExploreURL(Grafana, contract.GrafanaLoki, []string{contract.TelemetryLogs(ts, contains)}, w)
	case telemetryv1.Face_FACE_SPANS:
		s := &telemetryv1.Spans{}
		for i := range contract.TelemetrySpansMax {
			s.Traces = append(s.Traces, &telemetryv1.Trace{TraceId: fmt.Sprintf("%016x%016x", seed, uint64(i)), RootService: name,
				RootName: fmt.Sprintf("GET /api/%d", (seed+uint64(i))%7), StartMs: now.Add(-time.Duration(i*41) * time.Second).UnixMilli(),
				DurationMs: int64(5 + (seed+uint64(i)*17)%240)})
		}
		f.Data = &telemetryv1.Frame_Spans{Spans: s}
		f.ExploreUrl = contract.ExploreURL(Grafana, contract.GrafanaTempo, []string{contract.TelemetrySpans(ts)}, w)
	default:
		f.AtMs, f.Unknown = 0, "no such face"
	}
	return f
}

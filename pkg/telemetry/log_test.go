package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// TestLogger: a line is one JSON object with time, level and msg; a context holding a span adds its ids, also
// through a logger With attributes; a context without one adds nothing.
func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf)
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	l.ErrorContext(ctx, "atlas: read failed")
	l.With("app", "atlas").InfoContext(ctx, "two")
	l.Info("three")
	l.InfoContext(context.Background(), "four")
	l.WithGroup("g").InfoContext(ctx, "five", "k", "v")
	var lines []map[string]any
	for _, b := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("not JSON: %s", b)
		}
		if _, ok := m["time"]; !ok {
			t.Errorf("no time: %s", b)
		}
		delete(m, "time")
		lines = append(lines, m)
	}
	tid, sid := "01000000000000000000000000000000", "0200000000000000"
	want := []map[string]any{
		{"level": "ERROR", "msg": "atlas: read failed", "trace_id": tid, "span_id": sid},
		{"level": "INFO", "msg": "two", "app": "atlas", "trace_id": tid, "span_id": sid},
		{"level": "INFO", "msg": "three"},
		{"level": "INFO", "msg": "four"},
		{"level": "INFO", "msg": "five", "g": "k v " + tid + " " + sid},
	}
	if len(lines) != len(want) {
		t.Fatalf("%d lines: %v", len(lines), lines)
	}
	for i := range want {
		if len(lines[i]) != len(want[i]) {
			t.Errorf("line %d: %v, want %v", i, lines[i], want[i])
		}
		for k, v := range want[i] {
			if g, ok := lines[i][k].(map[string]any); ok {
				lines[i][k] = fmt.Sprint("k ", g["k"], " ", g["trace_id"], " ", g["span_id"])
			}
			if lines[i][k] != v {
				t.Errorf("line %d: %s = %v, want %v", i, k, lines[i][k], v)
			}
		}
	}
}

// TestLoggerDefault: installed as slog's default, the log package's lines come through it as INFO.
func TestLoggerDefault(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(NewLogger(&buf))
	log.Printf("db: attached")
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil || m["level"] != "INFO" || m["msg"] != "db: attached" {
		t.Errorf("%s: %v", buf.Bytes(), err)
	}
}

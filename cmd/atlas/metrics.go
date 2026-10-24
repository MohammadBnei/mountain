package main

// Self-observability on :9090. Stdlib text exposition: a handful of series do not need client_golang.

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

type vec struct {
	name, typ, help, label string
	mu                     sync.Mutex
	v                      map[string]float64
}

func newVec(name, typ, label, help string) *vec {
	return &vec{name: name, typ: typ, label: label, help: help, v: map[string]float64{}}
}

// Set / Add take the label value ("" for an unlabelled series).
func (m *vec) Set(l string, x float64) { m.mu.Lock(); m.v[l] = x; m.mu.Unlock() }
func (m *vec) Add(l string, x float64) { m.mu.Lock(); m.v[l] += x; m.mu.Unlock() }

var (
	SourceLastSuccess = newVec("mountain_source_last_success", "gauge", "source", "Unix time of the last success per source (prom, alerts, github, projection, db); seeded at start.")
	TokenDaysLeft     = newVec("mountain_github_token_days_left", "gauge", "", "Days until the GitHub token expires.")
	WSConnections     = newVec("mountain_ws_connections", "gauge", "", "Open /ws connections.")
	QueryFailed       = newVec("mountain_query_failed_total", "counter", "query", "Failed PromQL queries (the poll keeps its last answer and goes stale).")
	ProjectionSeq     = newVec("mountain_projection_seq", "gauge", "", "The latest projection's sequence number.")
	Drift             = newVec("mountain_drift", "gauge", "kind", "Drift in the latest projection, by kind (unassigned = a component realms.yaml does not place).")
	allVecs           = []*vec{SourceLastSuccess, TokenDaysLeft, WSConnections, QueryFailed, ProjectionSeq, Drift}
)

func init() {
	WSConnections.Set("", 0)
}

func serveMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, m := range allVecs {
		m.mu.Lock()
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", m.name, m.help, m.name, m.typ)
		keys := make([]string, 0, len(m.v))
		for k := range m.v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if m.label == "" {
				fmt.Fprintf(w, "%s %g\n", m.name, m.v[k])
			} else {
				fmt.Fprintf(w, "%s{%s=%q} %g\n", m.name, m.label, k, m.v[k])
			}
		}
		m.mu.Unlock()
	}
}

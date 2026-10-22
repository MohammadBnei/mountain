package main

// Self-observability on :9090 (06). Stdlib text exposition: eight series do not need client_golang.

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
	SourceLastSuccess = newVec("mountain_source_last_success", "gauge", "source", "Unix time of the last successful poll per source (relay start until the first).")
	Unmapped          = newVec("mountain_unmapped_total", "counter", "kind", "Series dropped for lack of a static map entry (node|app|repo|ns|probe).")
	TokenDaysLeft     = newVec("mountain_github_token_days_left", "gauge", "", "Days until the GitHub token expires.")
	WSConnections     = newVec("mountain_ws_connections", "gauge", "", "Open /ws connections.")
	DBDropped         = newVec("mountain_db_dropped_total", "counter", "reason", "Persistence writes lost (full = queue full, error = write failed).")
	ScrubDropped      = newVec("mountain_scrub_dropped_total", "counter", "", "Scrub replies lost to a full client send buffer (the connection is dropped).")
	Plots             = newVec("mountain_plots", "gauge", "", "Standing app and service buildings (not ruins, not machines): 0 for long = an empty town.")
	QueryFailed       = newVec("mountain_query_failed_total", "counter", "query", "Failed answers of the optional PromQL queries (they never mark the poll stale).")
	SlotFailed        = newVec("mountain_slot_failed_total", "counter", "", "Apps that found no free hex on their island (each counted once).")
	allVecs           = []*vec{SourceLastSuccess, Unmapped, TokenDaysLeft, WSConnections, DBDropped, ScrubDropped, Plots, QueryFailed, SlotFailed}
)

func init() {
	WSConnections.Set("", 0)
	Plots.Set("", 0)
	QueryFailed.Add("pods", 0)
	QueryFailed.Add("guests", 0)
	SlotFailed.Add("", 0)
	Unmapped.Add("probe", 0) // R8: the series exists from the start, so "flat at 0" is a real reading
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

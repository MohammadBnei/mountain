package main

// --source=fixture:<file>: an in-process fake Prometheus + Alertmanager replaying a scripted
// timeline (06 "Dev and fixture modes"). Format (www/dev/fixtures/relay-*.json):
//
//	{"step_s": 2, "steps": [{"prom": {"<promQ name>": [{"m": {labels}, "v": 1}]}, "am": [alerts]}, ...]}
//
// Steps are sparse: a query or "am" not named in a step keeps its previous answer. The timeline
// advances one step every step_s and holds on the last one. The prom source polls at step_s.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

type fixtureSeries struct {
	M map[string]string `json:"m"`
	V float64           `json:"v"`
}

type fixture struct {
	StepS float64 `json:"step_s"`
	Steps []struct {
		Prom map[string][]fixtureSeries `json:"prom"`
		AM   json.RawMessage            `json:"am"`
	} `json:"steps"`

	prom []map[string][]fixtureSeries // accumulated per step
	am   []json.RawMessage
	step atomic.Int64
}

func loadFixture(file string) (*fixture, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if len(f.Steps) == 0 || f.StepS <= 0 {
		return nil, fmt.Errorf("fixture %s: needs step_s > 0 and steps", file)
	}
	cur, am := map[string][]fixtureSeries{}, json.RawMessage("[]")
	for i, s := range f.Steps {
		next := map[string][]fixtureSeries{}
		for k, v := range cur {
			next[k] = v
		}
		for k, v := range s.Prom {
			if _, ok := promQ[k]; !ok {
				return nil, fmt.Errorf("fixture %s step %d: unknown query %q", file, i, k)
			}
			next[k] = v
		}
		if s.AM != nil {
			am = s.AM
		}
		cur = next
		f.prom, f.am = append(f.prom, cur), append(f.am, am)
	}
	return &f, nil
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i := f.step.Load()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v2/alerts" {
		w.Write(f.am[i])
		return
	}
	q := r.FormValue("query")
	var res []map[string]any
	for name, pq := range promQ {
		if pq == q {
			for _, s := range f.prom[i][name] {
				res = append(res, map[string]any{"metric": s.M, "value": []any{time.Now().Unix(), strconv.FormatFloat(s.V, 'g', -1, 64)}})
			}
		}
	}
	if res == nil {
		res = []map[string]any{}
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": res}})
}

// serveFixture starts the fake server on 127.0.0.1 and returns its base URL.
func serveFixture(ctx context.Context, f *fixture) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv := &http.Server{Handler: f, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	go func() { <-ctx.Done(); srv.Close() }()
	return "http://" + ln.Addr().String(), nil
}

func init() {
	FixtureHook = func(ctx context.Context, file string) (string, string, error) {
		f, err := loadFixture(file)
		if err != nil {
			return "", "", err
		}
		u, err := serveFixture(ctx, f)
		if err != nil {
			return "", "", err
		}
		d := time.Duration(f.StepS * float64(time.Second))
		promEvery = d
		go func() {
			t := time.NewTicker(d)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if n := f.step.Load(); n < int64(len(f.prom)-1) {
						f.step.Store(n + 1)
					}
				}
			}
		}()
		return u, u, nil
	}
}

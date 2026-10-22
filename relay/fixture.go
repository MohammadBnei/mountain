package main

// --source=fixture:<file>: an in-process fake Prometheus + Alertmanager + GitHub API replaying a scripted
// timeline (06 "Dev and fixture modes"). Format (www/dev/fixtures/relay-*.json):
//
//	{"step_s": 2, "gh_expires": "2026-12-01 00:00:00 UTC",
//	 "steps": [{"prom": {"<promQ name>": [{"m": {labels}, "v": 1}]}, "am": [alerts], "gh": {"<path?query>": body},
//	            "marks": [{"sub", "name", "role", "ref", "mark", "pose", "named"} | {"sub", "role", "raw": "<frame>"}]}, ...]}
//
// Steps are sparse: a query, "am" or a "gh" path not named in a step keeps its previous answer. The timeline
// advances one step after each prom poll (polls are step_s apart) and holds on the last one. A "gh" key is the
// request path plus its query exactly as src_github sends it; a JSON string body is served as raw text (contents),
// anything else as JSON, with an ETag (If-None-Match → 304, like GitHub) and the token-expiration header.
// "marks" (W4) are scripted visitor frames, played once when their step is polled, each from its own session
// (role visitor|admin|"" = anonymous; sub, name = the OIDC sub and username) through the same clientMsg as /ws; "raw"
// is sent verbatim (hostile: oversized, malformed). The relay logs each answer.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
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
		Prom  map[string][]fixtureSeries `json:"prom"`
		AM    json.RawMessage            `json:"am"`
		GH    map[string]json.RawMessage `json:"gh"`
		Marks []fixtureMark              `json:"marks"`
	} `json:"steps"`
	GHExpires string `json:"gh_expires"`

	prom   []map[string][]fixtureSeries // accumulated per step
	am     []json.RawMessage
	gh     []map[string]json.RawMessage
	step   atomic.Int64
	played int64 // the last step whose marks were played, +1 (the poll goroutine only)
}

type fixtureMark struct {
	Sub, Name, Role, Ref, Mark, Raw string
	Pose                            map[string]any
	Named                           bool
}

// playMarks sends the current step's marks once and returns the answers, in order.
func (f *fixture) playMarks() (acks []string) {
	i := f.step.Load()
	if i+1 <= f.played {
		return nil
	}
	f.played = i + 1
	for _, m := range f.Steps[i].Marks {
		c := &Client{IP: "fixture", Role: m.Role, RoleExp: time.Now().Add(time.Hour), Sub: m.Sub, Name: m.Name, send: make(chan []byte, 4)}
		b := []byte(m.Raw)
		if m.Raw == "" {
			b = mustJSON(map[string]any{"t": "mark", "ref": m.Ref, "mark": m.Mark, "pose": m.Pose, "named": m.Named})
		}
		clientMsg(c, b)
		ack := "(none)"
		select {
		case a := <-c.send:
			ack = string(a)
		default:
		}
		log.Printf("fixture: mark %s %s → %s", m.Sub, m.Mark, ack)
		acks = append(acks, ack)
	}
	return acks
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
	cur, am, gh := map[string][]fixtureSeries{}, json.RawMessage("[]"), map[string]json.RawMessage{}
	for i, s := range f.Steps {
		gh = maps.Clone(gh)
		maps.Copy(gh, s.GH)
		f.gh = append(f.gh, gh)
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

// advance moves to the next step, holding on the last one.
func (f *fixture) advance() {
	if n := f.step.Load(); n < int64(len(f.prom)-1) {
		f.step.Store(n + 1)
	}
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i := f.step.Load()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v2/alerts" {
		w.Write(f.am[i])
		return
	}
	if strings.HasPrefix(r.URL.Path, "/repos/") {
		f.serveGH(w, r, f.gh[i])
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

func (f *fixture) serveGH(w http.ResponseWriter, r *http.Request, gh map[string]json.RawMessage) {
	k := r.URL.Path
	if r.URL.RawQuery != "" {
		k += "?" + r.URL.RawQuery
	}
	body, ok := gh[k]
	if !ok {
		http.NotFound(w, r) // what GitHub answers for a repo the token cannot see
		return
	}
	var raw string
	if json.Unmarshal(body, &raw) == nil {
		body = []byte(raw)
		w.Header().Set("Content-Type", "text/plain")
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("X-RateLimit-Remaining", "4999")
	h.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	if f.GHExpires != "" {
		h.Set("github-authentication-token-expiration", f.GHExpires)
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(body)
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
		promEvery = time.Duration(f.StepS * float64(time.Second))
		ghEvery = promEvery
		buildWindow = 100 * 365 * 24 * time.Hour          // fixture run timestamps are static: every queued/in_progress run is in flight
		afterPoll = func() { f.playMarks(); f.advance() } // one clock: the poll ticker; each step is seen by exactly one poll
		return u, u, nil
	}
}

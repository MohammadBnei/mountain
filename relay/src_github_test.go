package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// runGitHub serves f's GitHub API (counting conditional hits) and polls the github source once per step, then
// once more on the last step (every answer a 304).
func runGitHub(t *testing.T, a *App, c *Client, f *fixture) (*ghSource, [][]map[string]any, *atomic.Int64) {
	t.Helper()
	var notModified atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("%s without the token", r.URL)
		}
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, r)
		if rec.Code == http.StatusNotModified {
			notModified.Add(1)
		}
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
	}))
	t.Cleanup(srv.Close)
	a.Opts.GHURL, a.Opts.GHToken = srv.URL, "test-token"
	s, out, now := newGitHub(a), &Out{app: a, name: "github"}, time.Date(2026, 10, 15, 11, 0, 0, 0, time.UTC)
	old := buildWindow
	buildWindow = 100 * 365 * 24 * time.Hour // the fixture's static timestamps (as in fixture mode)
	t.Cleanup(func() { buildWindow = old })
	var steps [][]map[string]any
	for i := 0; i <= len(f.gh); i++ {
		f.step.Store(int64(min(i, len(f.gh)-1)))
		s.poll(context.Background(), out, now)
		if s.lastErr != "" {
			t.Fatalf("step %d: %s", i, s.lastErr)
		}
		steps = append(steps, drain(c))
		now = now.Add(ghScanEvery) // every poll also rescans
	}
	return s, steps, &notModified
}

// TestGitHubTimeline replays the "gh" part of www/dev/fixtures/relay-hostile.json: builds by repo + time window,
// gitops commits counted after a baseline, an ADR merge with its day only (title off the allowlist), the token expiry
// metric, the vein ref scan; nothing hostile (SHAs, emails, private repos, branch names, IPs) leaves; frames validate.
func TestGitHubTimeline(t *testing.T) {
	a, c := testApp(t)
	f, err := loadFixture("../www/dev/fixtures/relay-hostile.json")
	if err != nil {
		t.Fatal(err)
	}
	// a resolved warning (20 min) whose alert name is in the runbook notes: the first scan flags its vein
	a.names["00000000000000a1"] = "KubePodCrashLooping"
	(&Out{app: a, name: "alerts"}).Resolve(Alert{FP: "00000000000000a1", Sev: "warning", Layer: "k8s"}, 1200)
	if len(a.State.Veins) != 1 || a.State.Veins[0].Ref {
		t.Fatalf("vein before the scan: %+v", a.State.Veins)
	}
	drain(c)
	_, steps, nm := runGitHub(t, a, c, f)

	got := func(step int) (out []string) {
		for _, x := range steps[step] {
			switch {
			case x["t"] == "ev" && x["kind"] == "build":
				out = append(out, fmt.Sprintf("build %v %v", x["app"], x["state"]))
			case x["t"] == "ev" && x["kind"] == "gitops.commit":
				out = append(out, fmt.Sprintf("gitops %v", x["n"]))
			case x["t"] == "ev" && x["kind"] == "adr.merge":
				out = append(out, fmt.Sprintf("adr %v title=%v", x["day"], x["title"]))
			case x["t"] == "world" && x["kind"] == "vein":
				out = append(out, fmt.Sprintf("vein ref=%v", x["ref"]))
			case x["t"] == "metrics":
				out = append(out, fmt.Sprintf("inflight %v", x["buildsInFlight"]))
			}
		}
		return
	}
	for i, want := range []string{
		"[build wird running inflight 1 vein ref=true]", // + the ash (adr baseline) and gitops baselines: nothing
		"[build mountain queued build wird success gitops 2]",
		"[build mountain running adr 2026-10-15 title=<nil>]",
		"[build mountain failure inflight 0]",
		"[]", // all 304: nothing changed
	} {
		if g := fmt.Sprint(got(i)); g != want {
			t.Errorf("step %d: %s, want %s", i, g, want)
		}
	}
	if !a.State.Veins[0].Ref || len(a.State.Builds) != 0 || len(a.State.Ash) != 1 || a.State.Glacier.Mass <= 0.5 {
		t.Errorf("state: %+v", a.State)
	}
	if nm.Load() < 10 {
		t.Errorf("conditional requests: %d 304s, want the repeated answers served as 304", nm.Load())
	}
	TokenDaysLeft.mu.Lock()
	d := TokenDaysLeft.v[""]
	TokenDaysLeft.mu.Unlock()
	if want := time.Until(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24; d < want-1 || d > want+1 {
		t.Errorf("mountain_github_token_days_left = %v, want ≈ %v", d, want)
	}
	if !a.refs["TargetDown"] || !a.refs["KubePodCrashLooping"] {
		t.Errorf("refs from the notes and the skills: %v", a.refs)
	}
	// a new vein whose alert is in the notes is placed with ref
	a.names["00000000000000b2"] = "TargetDown"
	(&Out{app: a, name: "alerts"}).Resolve(Alert{FP: "00000000000000b2", Sev: "critical", Layer: "apps"}, 900)
	if v := a.State.Veins[len(a.State.Veins)-1]; v.FP != "00000000000000b2" || !v.Ref {
		t.Errorf("new vein of a noted alert: %+v", v)
	}
	steps = append(steps, drain(c))

	validate(t, steps)
	sha := regexp.MustCompile(`[0-9a-f]{40}`)
	for _, s := range steps {
		for _, fr := range s {
			b, _ := json.Marshal(fr)
			s := string(b)
			if sha.MatchString(s) || ipRE.MatchString(s) || versionRE.MatchString(s) {
				t.Errorf("SHA/IP/version leak: %.200s", s)
			}
			for _, h := range append(hostile, "@", "hq", "evil", "Signed", "feat/", "KubePod", "TargetDown", "ADR-", "MohammadBnei") {
				if strings.Contains(s, h) {
					t.Errorf("leak %q: %.200s", h, s)
				}
			}
		}
	}
}

// TestGitHubOff: no token = no source: no liveness series (so no MountainSourceStale) and no token metric.
func TestGitHubOff(t *testing.T) {
	old := registry
	defer func() { registry = old }()
	registry = map[string]func(*App) (Source, error){"github": old["github"]}
	SourceLastSuccess.mu.Lock()
	delete(SourceLastSuccess.v, "github")
	SourceLastSuccess.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := startSources(ctx, &App{Opts: Opts{Source: "live"}}); err != nil {
		t.Fatal(err)
	}
	SourceLastSuccess.mu.Lock()
	_, on := SourceLastSuccess.v["github"]
	SourceLastSuccess.mu.Unlock()
	if on {
		t.Fatal("a disabled github source must not export mountain_source_last_success (it would age into MountainSourceStale)")
	}
}

// TestGitHubRateLimit: fewer than 100 requests left pauses every request until the reset.
func TestGitHubRateLimit(t *testing.T) {
	a, _ := testApp(t)
	s := newGitHub(a)
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "99")
	h.Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	s.limits(h)
	if _, err := s.get(context.Background(), "/repos/x/y", nil, false); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("want a local pause, got %v", err)
	}
}

// TestDBBuildRestore: a build in flight survives a restart in hello.state.builds, and the restarted source ends it
// (the run finished while the relay was down) instead of leaving the heat pinned.
func TestDBBuildRestore(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	(&Out{app: a, name: "github"}).Ev("build", map[string]any{"app": "mountain", "state": "running"})
	a.store.Close(5 * time.Second)

	b := storedApp(t, dsn)
	defer b.store.Close(time.Second)
	if b.State.Builds["mountain"] != "running" {
		t.Fatalf("restored builds: %v", b.State.Builds)
	}
	c := &Client{send: make(chan []byte, 1<<10)}
	b.Hub.clients[c] = struct{}{}
	f, err := loadFixture("../www/dev/fixtures/relay-hostile.json")
	if err != nil {
		t.Fatal(err)
	}
	f.gh = f.gh[3:] // the mountain run has completed (failure)
	_, steps, _ := runGitHub(t, b, c, f)
	if e := evs(steps[0], "build"); len(e) != 1 || e[0]["app"] != "mountain" || e[0]["state"] != "failure" || len(b.State.Builds) != 0 {
		t.Fatalf("first poll after the restart: %v, state %v", e, b.State.Builds)
	}
}

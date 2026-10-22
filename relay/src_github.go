package main

// src_github (02 builds, gitops commits + ADR merges, vein ref; 06 mountain_github_token_days_left): polls the
// GitHub REST API with MOUNTAIN_GITHUB_TOKEN, a fine-grained read-only PAT (Actions + Contents + Metadata: read, on
// the config.yaml repos and infra-bootstrap).
//
// No token = the source is off: one log line at start, no mountain_source_last_success{source="github"} and no
// mountain_github_token_days_left series, so neither MountainSourceStale nor MountainTokenExpiring can fire for it.
// With a token, a failing GitHub never marks the page stale (like the DB, the live view does not depend on it): the
// gauge ages into MountainSourceStale instead, and the error is logged once per change, not per poll.
//
// Nothing GitHub returns reaches the wire as is: app comes from config.yaml repos, state from a closed enum, day from a
// parsed date, a title only through the config.yaml titles allowlist (sanitize). SHAs stay in memory (commit dedup);
// alert names never leave (the vein ref is a bool). Rate limits: conditional requests (a 304 does not count against
// the limit), one poll a minute (8 calls, mostly 304), the contents scan daily, and a pause until the reset when
// fewer than 100 requests are left.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const ghInfra = "MohammadBnei/infra-bootstrap" // gitops commits, ADRs and the runbook notes (02)

var (
	ghEvery     = time.Minute   // runs + commits; the fixture hook shortens both to its step
	buildWindow = 2 * time.Hour // a run created earlier is not in flight: a stuck queued run never pins the heat
	alertWordRE = regexp.MustCompile(`[A-Z][A-Za-z0-9]{3,}`)
	errNoAccess = errors.New("not found (no access)")
)

func init() {
	Register("github", func(a *App) (Source, error) {
		if a.Opts.GHToken == "" {
			log.Printf("github: MOUNTAIN_GITHUB_TOKEN unset: builds, gitops commits and ADR merges are off (no stale alarm)")
			return nil, nil
		}
		return newGitHub(a), nil
	})
}

type ghSource struct {
	a          *App
	url, token string
	hc         *http.Client
	etag       map[string]string          // request -> ETag
	body       map[string][]byte          // request -> last 200 body (a 304 replays it)
	builds     map[string]string          // app -> queued|running: in flight (seeded from hello.state.builds)
	seen       map[string]map[string]bool // commit path -> SHAs of its last answer (nil = no baseline yet)
	pauseUntil time.Time                  // rate limit: no request before this
	lastErr    string
	logged     map[string]bool // repos already logged as unreachable
}

func newGitHub(a *App) *ghSource {
	a.mu.Lock()
	builds := maps.Clone(a.State.Builds) // restored: a build that finished while the relay was down still ends
	a.mu.Unlock()
	return &ghSource{a: a, url: strings.TrimRight(a.Opts.GHURL, "/"), token: a.Opts.GHToken, hc: &http.Client{Timeout: 20 * time.Second},
		etag: map[string]string{}, body: map[string][]byte{}, builds: builds, seen: map[string]map[string]bool{}, logged: map[string]bool{}}
}

func (s *ghSource) Name() string { return "github" }

func (s *ghSource) Run(ctx context.Context, out *Out) error {
	t := time.NewTicker(ghEvery)
	defer t.Stop()
	for {
		s.poll(ctx, out, time.Now())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// poll is one cycle: builds and commits. Liveness only when both answered.
func (s *ghSource) poll(ctx context.Context, out *Out, now time.Time) {
	err := s.runs(ctx, out, now)
	if err == nil {
		err = s.commits(ctx, out)
	}
	if err != nil {
		if msg := err.Error(); msg != s.lastErr {
			log.Printf("github: %v", err)
			s.lastErr = msg
		}
		return
	}
	if s.lastErr != "" {
		log.Printf("github: recovered")
		s.lastErr = ""
	}
	out.OK()
}

// get fetches path?q (raw: file contents) with a conditional request; a 304 returns the cached body.
func (s *ghSource) get(ctx context.Context, path string, q url.Values, raw bool) ([]byte, error) {
	if time.Now().Before(s.pauseUntil) {
		return nil, fmt.Errorf("rate limited until %s", s.pauseUntil.UTC().Format(time.TimeOnly))
	}
	k := path
	if len(q) > 0 {
		k += "?" + q.Encode() // sorted: a stable key (and the fixture's)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", s.url+k, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Accept", "application/vnd.github+json")
	if raw {
		req.Header.Set("Accept", "application/vnd.github.raw")
	}
	if e := s.etag[k]; e != "" {
		req.Header.Set("If-None-Match", e)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	s.limits(resp.Header)
	switch resp.StatusCode {
	case http.StatusNotModified:
		return s.body[k], nil
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		s.etag[k], s.body[k] = resp.Header.Get("ETag"), b
		return b, nil
	case http.StatusNotFound:
		return nil, errNoAccess
	case http.StatusForbidden, http.StatusTooManyRequests:
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		s.pauseUntil = time.Now().Add(max(time.Duration(ra)*time.Second, time.Minute))
	}
	return nil, fmt.Errorf("%s: status %d", path, resp.StatusCode)
}

// limits reads the token expiry (mountain_github_token_days_left) and pauses before the rate limit runs out.
func (s *ghSource) limits(h http.Header) {
	if exp := h.Get("github-authentication-token-expiration"); exp != "" {
		for _, layout := range []string{"2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 -0700"} {
			if t, err := time.Parse(layout, exp); err == nil {
				TokenDaysLeft.Set("", float64(time.Until(t).Round(time.Hour).Hours())/24)
				break
			}
		}
	}
	left, err1 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err2 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err1 == nil && err2 == nil && left < 100 {
		s.pauseUntil = time.Unix(reset, 0)
	}
}

type ghRun struct {
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// runs maps Actions runs to ev:build by workflow repo (config.yaml repos) and time window (02, never SHAs):
// in flight = a run created within buildWindow (in_progress → running, else queued); when none is left the app's
// build ends with the conclusion of its latest completed run (success, anything else → failure).
func (s *ghSource) runs(ctx context.Context, out *Out, now time.Time) error {
	for _, repo := range slices.Sorted(maps.Keys(s.a.Cfg.Repos)) {
		app := s.a.Cfg.Repos[repo]
		b, err := s.get(ctx, "/repos/"+repo+"/actions/runs", url.Values{"per_page": {"20"}}, false)
		if errors.Is(err, errNoAccess) { // the PAT does not cover this repo: skip it, say so once
			if !s.logged[repo] {
				log.Printf("github: %s: no access, its builds are off", repo)
				s.logged[repo] = true
			}
			continue
		}
		if err != nil {
			return err
		}
		var r struct {
			Runs []ghRun `json:"workflow_runs"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s runs: %w", repo, err)
		}
		state, done, doneAt := "", "", time.Time{}
		for _, x := range r.Runs {
			switch {
			case x.Status == "completed":
				if x.UpdatedAt.After(doneAt) {
					done, doneAt = x.Conclusion, x.UpdatedAt
				}
			case now.Sub(x.CreatedAt) > buildWindow:
			case x.Status == "in_progress":
				state = "running"
			case state == "": // queued, requested, waiting, pending
				state = "queued"
			}
		}
		old := s.builds[app]
		switch {
		case state != "" && state != old:
			out.Ev("build", map[string]any{"app": app, "state": state})
			s.builds[app] = state
		case state == "" && old != "":
			end := "failure"
			if done == "success" {
				end = "success"
			}
			out.Ev("build", map[string]any{"app": app, "state": end})
			delete(s.builds, app)
		}
	}
	n := len(s.builds)
	s.a.mu.Lock()
	changed := s.a.Metrics.BuildsInFlight != n
	s.a.mu.Unlock()
	if changed { // rides the metrics frame and src_prom's heat
		out.Metrics(func(m *Metrics) { m.BuildsInFlight = n })
	}
	return nil
}

// commits: new infra-bootstrap main commits under gitops/ → one ev gitops.commit{n} (glacier snow); under docs/adr/
// → one ev adr.merge{day, title} each (ash line; sanitize keeps the title only when allowlisted). The first answer
// per path is the baseline (no events): a restart neither replays history nor counts it twice.
// ponytail: commits landing while the relay is down are not counted; > 30 between two polls are capped at 30.
func (s *ghSource) commits(ctx context.Context, out *Out) error {
	for _, path := range []string{"gitops", "docs/adr"} {
		b, err := s.get(ctx, "/repos/"+ghInfra+"/commits", url.Values{"sha": {"main"}, "path": {path}, "per_page": {"30"}}, false)
		if err != nil {
			return err
		}
		var cs []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message   string `json:"message"`
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(b, &cs); err != nil {
			return fmt.Errorf("%s commits: %w", path, err)
		}
		old, next, n := s.seen[path], map[string]bool{}, 0
		for i := len(cs) - 1; i >= 0; i-- { // oldest first
			c := cs[i]
			next[c.SHA] = true
			if old == nil || old[c.SHA] {
				continue
			}
			if path == "gitops" {
				n++
				continue
			}
			subject, _, _ := strings.Cut(c.Commit.Message, "\n")
			out.Ev("adr.merge", map[string]any{"day": c.Commit.Committer.Date.UTC().Format(time.DateOnly), "title": strings.TrimSpace(subject)})
		}
		s.seen[path] = next
		if n > 0 {
			out.Ev("gitops.commit", map[string]any{"n": n})
		}
	}
	return nil
}

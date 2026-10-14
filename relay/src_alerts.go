package main

// src_alerts: polls Alertmanager every 15 s (02 alerts row). An alert fires on first sight and
// resolves when it is gone from a good answer; an unreachable Alertmanager is stale, never a mass
// resolve. The raw fingerprint stays in memory: only App.FP's salted hash leaves.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

func init() {
	Register("alerts", func(a *App) (Source, error) { return newAlerts(a) })
}

type layerRule struct {
	re    *regexp.Regexp
	layer string
}

type alertsSource struct {
	a      *App
	url    string
	hc     *http.Client
	layers []layerRule
	firing map[string]Alert // salted fp -> alert
	stale  bool
}

func newAlerts(a *App) (*alertsSource, error) {
	s := &alertsSource{a: a, url: strings.TrimRight(a.Opts.AMURL, "/"), hc: &http.Client{Timeout: 10 * time.Second}, firing: map[string]Alert{}}
	for _, l := range a.Cfg.AlertLayers {
		re, err := regexp.Compile("^(?:" + l.Match + ")$")
		if err != nil {
			return nil, fmt.Errorf("config alertLayers %q: %w", l.Match, err)
		}
		s.layers = append(s.layers, layerRule{re, l.Layer})
	}
	// Alerts restored from the DB (store.go) count as firing: still active = no second alert.fire, and the
	// restored startsAt is kept; gone from Alertmanager = resolved on the first good poll.
	a.mu.Lock()
	for _, x := range a.State.Alerts {
		s.firing[x.FP] = x
	}
	a.mu.Unlock()
	return s, nil
}

func (s *alertsSource) Name() string { return "alerts" }

// Run polls on the prom cadence. ponytail: shares promEvery (both 15 s; the fixture hook shortens both).
func (s *alertsSource) Run(ctx context.Context, out *Out) error {
	t := time.NewTicker(promEvery)
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

// poll is one cycle: fetch, then fire/resolve against the firing set; on failure keep it and go stale.
func (s *alertsSource) poll(ctx context.Context, out *Out, now time.Time) {
	cur, err := s.fetch(ctx)
	if err != nil {
		log.Printf("alerts: %v", err)
		out.Stale()
		s.stale = true
		return
	}
	out.OK()
	if s.stale {
		out.Metrics(func(m *Metrics) { delete(m.Stale, "alerts") })
		s.stale = false
	}
	for _, fp := range slices.Sorted(maps.Keys(s.firing)) {
		if _, ok := cur[fp]; !ok {
			a := s.firing[fp]
			out.Resolve(a, max(0, int(now.UnixMilli()-a.StartsAt)/1000))
			delete(s.firing, fp)
		}
	}
	for _, fp := range slices.Sorted(maps.Keys(cur)) {
		if _, ok := s.firing[fp]; !ok && out.Fire(cur[fp]) != "" {
			s.firing[fp] = cur[fp]
		}
	}
}

// fetch returns the public alerts by salted fp. Watchdog / InfoInhibitor, severities outside
// info|warning|critical (e.g. the relay's own notice rules) and a salt-less fp are left out here,
// so they never churn through sanitize every poll.
func (s *alertsSource) fetch(ctx context.Context) (map[string]Alert, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.url+"/api/v2/alerts?active=true&silenced=false&inhibited=false", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r []struct {
		Fingerprint string
		StartsAt    time.Time
		Labels      map[string]string
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || resp.StatusCode != http.StatusOK || r == nil {
		return nil, fmt.Errorf("alertmanager: status %d, %v", resp.StatusCode, err) // a null body is no answer, not "none firing"
	}
	out := map[string]Alert{}
	for _, x := range r {
		name, sev := x.Labels["alertname"], x.Labels["severity"]
		fp := s.a.FP(x.Fingerprint)
		if name == "Watchdog" || name == "InfoInhibitor" || x.Fingerprint == "" || fp == "" ||
			(sev != "info" && sev != "warning" && sev != "critical") {
			continue
		}
		out[fp] = Alert{FP: fp, Sev: sev, Layer: s.layer(name), StartsAt: x.StartsAt.UnixMilli()}
	}
	return out, nil
}

// layer: first alertLayers rule matching the alert name ("" = none, which sanitize drops).
func (s *alertsSource) layer(name string) string {
	for _, l := range s.layers {
		if l.re.MatchString(name) {
			return l.layer
		}
	}
	return ""
}

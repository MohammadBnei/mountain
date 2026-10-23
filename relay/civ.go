package main

// R9 "Civ" (plan/00-decisions.md Civ rows): the town read as a civilisation, every mechanic set by a real signal.
//
//   - levels: an app's building grows with its activity, the successful ArgoCD syncs of the last 30 days
//     (levelOf: 1, 2 from levelAt[0], 3 from levelAt[1]). ev level {app, level} on change; State.Levels holds the
//     levels above 1 (absent = 1). Seeded at boot from the stored events, so a restart neither resets nor inflates
//     it (memory-only: counted from boot). Machines never sync: they stay 1.
//   - technologies: the ADRs of infra-bootstrap (docs/adr/NNNN-*.md, a conditional listing per GitHub poll): only
//     their count and the highest number leave the relay (file names name products). ev tech {count, top}.
//   - calm: the day since which no warning or critical alert fires and no app is Degraded or Missing (State.Calm,
//     "" = not calm). Kept by State.apply alone (so a reload and a replay agree); seeded at restore from the stored
//     events. The page shows a golden age after goldenDays of calm (Alertmanager's alerts only: its words say so).
//   - turns: hello.snapshot.started, the town's first day; the page counts the days since.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"time"
)

const activityWindow = 30 * 24 * time.Hour

// levelAt: successful syncs in activityWindow for level 2 and level 3.
var levelAt = [2]int{4, 12}

func levelOf(n int) int {
	switch {
	case n >= levelAt[1]:
		return 3
	case n >= levelAt[0]:
		return 2
	}
	return 1
}

// Tech is hello.state.tech: the ADRs on infra-bootstrap main, as numbers only.
type Tech struct {
	Count int `json:"count"`
	Top   int `json:"top"`
}

// activity is the prom source's memory of successful syncs per app (time order), pruned to activityWindow.
type activity struct {
	syncs  map[string][]time.Time
	seeded bool
}

func newActivity() *activity { return &activity{syncs: map[string][]time.Time{}} }

// levels records this poll's successful syncs and emits ev level for every app whose level changed (then the decay
// of old syncs, which needs no event to happen). Run once per successful poll.
func (ac *activity) levels(a *App, o *Out, ok []string, now time.Time) {
	if !ac.seeded && a.store != nil {
		if seed, err := a.store.syncsSince(context.Background(), now.Add(-activityWindow)); err == nil {
			for app, ts := range seed {
				ac.syncs[app] = append(ts, ac.syncs[app]...)
			}
		}
		ac.seeded = true
	}
	for _, app := range ok {
		ac.syncs[app] = append(ac.syncs[app], now)
	}
	cut := now.Add(-activityWindow)
	for app, ts := range ac.syncs {
		ac.syncs[app] = slices.DeleteFunc(ts, func(t time.Time) bool { return t.Before(cut) })
	}
	a.mu.Lock()
	cur := maps.Clone(a.State.Levels)
	a.mu.Unlock()
	for _, app := range slices.Sorted(maps.Keys(a.Cfg.Apps)) {
		if a.Cfg.machine(app) {
			continue
		}
		l, was := levelOf(len(ac.syncs[app])), cur[app]
		if was == 0 {
			was = 1
		}
		if l != was {
			o.Ev("level", map[string]any{"app": app, "level": l})
		}
	}
}

// syncsSince returns the stored successful argo.sync times per app since t (the activity seed).
func (s *Store) syncsSince(ctx context.Context, t time.Time) (map[string][]time.Time, error) {
	rows, err := s.pool.Query(ctx, `SELECT public->>'app', ts FROM events
		WHERE kind = 'argo.sync' AND (public->>'ok')::boolean AND ts >= $1 ORDER BY seq`, t)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]time.Time{}
	for rows.Next() {
		var app string
		var ts time.Time
		if err := rows.Scan(&app, &ts); err != nil {
			return nil, err
		}
		out[app] = append(out[app], ts)
	}
	return out, rows.Err()
}

// calmSeed is the last day something troubled the town in the stored events (an alert fired or resolved, an app
// went Degraded or Missing), else the town's first day: the restore seed of State.Calm when the town is calm now.
func (s *Store) calmSeed(ctx context.Context) (string, error) {
	var d *time.Time
	err := s.pool.QueryRow(ctx, `SELECT max(day) FROM events WHERE kind IN ('alert.fire', 'alert.resolve')
		OR (kind = 'argo.health' AND public->>'status' IN ('Degraded', 'Missing'))`).Scan(&d)
	if err != nil {
		return "", err
	}
	if d == nil {
		return s.started(ctx)
	}
	return d.Format(time.DateOnly), nil
}

// troubled: a warning or critical alert fires, or an app is Degraded or Missing.
func (s *State) troubled() bool {
	for _, x := range s.Alerts {
		if x.Sev == "warning" || x.Sev == "critical" {
			return true
		}
	}
	for _, h := range s.Health {
		if h == "Degraded" || h == "Missing" {
			return true
		}
	}
	return false
}

// calm updates State.Calm after an alert or health frame (State.apply): troubled clears it; calm again starts it on
// the frame's day.
func (s *State) calm(ts int64) {
	switch {
	case s.troubled():
		s.Calm = ""
	case s.Calm == "" && ts > 0:
		s.Calm = utcDay(time.UnixMilli(ts))
	}
}

var adrRE = regexp.MustCompile(`^(\d{4})-[a-z0-9.-]+\.md$`)

// techs lists docs/adr on infra-bootstrap main (conditional: an unchanged listing is a cached 304) and emits ev tech
// when the count or the highest number moved. Not fatal: a failure is logged once and the poll goes on.
func (s *ghSource) techs(ctx context.Context, out *Out) {
	b, err := s.get(ctx, "/repos/"+ghInfra+"/contents/docs/adr", nil, false)
	if err == nil {
		var fs []struct {
			Name string `json:"name"`
		}
		if err = json.Unmarshal(b, &fs); err == nil {
			t := Tech{}
			for _, f := range fs {
				if m := adrRE.FindStringSubmatch(f.Name); m != nil {
					n, _ := strconv.Atoi(m[1])
					t.Count++
					t.Top = max(t.Top, n)
				}
			}
			s.a.mu.Lock()
			cur := s.a.State.Tech
			s.a.mu.Unlock()
			if t.Count > 0 && t != cur {
				out.Ev("tech", map[string]any{"count": t.Count, "top": t.Top})
			}
			s.techErr = ""
			return
		}
		err = fmt.Errorf("adr listing: %w", err)
	}
	if msg := err.Error(); msg != s.techErr {
		log.Printf("github: technologies: %v", err)
		s.techErr = msg
	}
}

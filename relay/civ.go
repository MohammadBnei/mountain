package main

// R9 "Civ" (plan/00-decisions.md Civ rows): the town read as a civilisation, every mechanic set by a real signal.
//
//   - levels: none yet. They will come from goals reached (plan/09-play.md), not from sync activity.
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
	"regexp"
	"strconv"
	"time"
)

// Tech is hello.state.tech: the ADRs on infra-bootstrap main, as numbers only.
type Tech struct {
	Count int `json:"count"`
	Top   int `json:"top"`
}

// calmSeed is the last day the town's trouble moved in the stored events before day `before` (nil = all): a warning
// or critical alert fired or resolved, an app recovered from Degraded or Missing, or an app went. Never before the
// town's first day. It seeds State.Calm for a snapshot written before R9, when the town is calm at that snapshot.
// It errs late, never early: a golden age is never shown sooner than State.apply would have shown it.
func (s *Store) calmSeed(ctx context.Context, before *string) (string, error) {
	var b any
	if before != nil {
		b = *before
	}
	var d time.Time
	err := s.pool.QueryRow(ctx, `WITH ev AS (SELECT seq, day, kind, public FROM events WHERE $1::date IS NULL OR day < $1::date),
		h AS (SELECT day, public->>'status' AS st, lag(public->>'status') OVER (PARTITION BY public->>'app' ORDER BY seq) AS prev
			FROM ev WHERE kind = 'argo.health')
		SELECT greatest((SELECT started FROM town_epoch), max(d)) FROM (
			SELECT day AS d FROM ev WHERE kind IN ('alert.fire', 'alert.resolve') AND public->>'sev' IN ('warning', 'critical')
			UNION ALL SELECT day FROM h WHERE prev IN ('Degraded', 'Missing') AND st NOT IN ('Degraded', 'Missing')
			UNION ALL SELECT day FROM ev WHERE kind = 'app.gone') x`, b).Scan(&d)
	if err != nil {
		return "", err
	}
	return d.Format(time.DateOnly), nil
}

// seedCalm gives a snapshot written before R9 (no calm key) its calm day, before its later frames are folded on it.
func (a *App) seedCalm(ctx context.Context, r *dbSnap, before *string) {
	if r.calmKnown || r.state.troubled() {
		return
	}
	d, err := a.store.calmSeed(ctx, before)
	if err != nil {
		log.Printf("relay: calm seed: %v", err)
		return
	}
	r.state.Calm = d
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

package main

// Scrub (06 WS protocol): a client's {t:"scrub", day:"YYYY-MM-DD"} gets, on its connection only,
// {t:"replay", day, snapshot, state, frames, metrics}: the land and state at the start of that UTC day
// (the latest snapshot before it plus the frames between, or the bare base when there is none), the
// day's ev + world frames in broadcast order, and its metrics_5m frames.
// A request that cannot be served gets {t:"replay", [day], error} (bad-day | rate | unavailable), so a
// paused client always hears back. Memory-only answers "unavailable", because the history lives in the DB.

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

const (
	scrubEvery  = 2 * time.Second // per connection (06 limits)
	scrubDays   = 30
	replayCache = 7
)

var dayRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// replays caches past days' replay frames (a past day no longer changes). Today is always read from the DB.
// ponytail: the last replayCache days requested, oldest evicted (≈ 120 KB + frames each); an LRU if scrubbing gets heavy.
var replays = struct {
	sync.Mutex
	m     map[string][]byte
	order []string
}{m: map[string][]byte{}}

func init() { onClientMsg = clientMsg }

// clientMsg handles client frames. Only scrub exists so far (subscribe and mark: W3/W4); others are ignored.
func clientMsg(c *Client, msg []byte) {
	var f struct{ T, Day string }
	if json.Unmarshal(msg, &f) != nil || f.T != "scrub" {
		return
	}
	now := time.Now()
	if now.Sub(c.scrubAt) < scrubEvery {
		c.Send(replayErr("", "rate"))
		return
	}
	c.scrubAt = now
	c.Send(app.replay(f.Day, now))
}

func replayErr(day, why string) []byte {
	f := map[string]any{"t": "replay", "error": why}
	if day != "" {
		f["day"] = day
	}
	return mustJSON(f)
}

// replay returns the replay frame for day (validated here: the day string is client input).
func (a *App) replay(day string, now time.Time) []byte {
	d, err := time.Parse(time.DateOnly, day)
	today := now.UTC().Truncate(24 * time.Hour)
	if !dayRE.MatchString(day) || err != nil || d.After(today) || d.Before(today.AddDate(0, 0, -scrubDays)) {
		return replayErr("", "bad-day")
	}
	if a.store == nil {
		return replayErr(day, "unavailable")
	}
	past := d.Before(today)
	if past {
		replays.Lock()
		b := replays.m[day]
		replays.Unlock()
		if b != nil {
			return b
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, err := a.buildReplay(ctx, day)
	if err != nil {
		log.Printf("scrub %s: %v", day, err)
		return replayErr(day, "unavailable")
	}
	if past {
		replays.Lock()
		if _, ok := replays.m[day]; !ok {
			replays.m[day], replays.order = b, append(replays.order, day)
			if len(replays.order) > replayCache {
				delete(replays.m, replays.order[0])
				replays.order = slices.Delete(replays.order, 0, 1)
			}
		}
		replays.Unlock()
	}
	return b
}

func (a *App) buildReplay(ctx context.Context, day string) ([]byte, error) {
	r, err := a.store.load(ctx, a.World.Seed, &day)
	if err != nil {
		return nil, err
	}
	land, st := make([]int16, worldcore.Grid*worldcore.Grid), State{Alerts: []Alert{}}
	if r != nil {
		land, st = r.land, r.state
		if st.Alerts == nil {
			st.Alerts = []Alert{}
		}
		fold(land, &st, r.frames)
	}
	frames, metrics, err := a.store.day(ctx, day)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"t": "replay", "day": day, "snapshot": a.snap(day, land), "state": st, "frames": frames, "metrics": metrics}), nil
}

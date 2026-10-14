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
	todayTTL    = 10 * time.Second // a day still being written (today, or yesterday until dayGrace) is cached this long
	dayGrace    = time.Minute      // the writer queue drains well within this after UTC midnight
)

var dayRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// replays caches replay frames: a day that ended more than dayGrace ago no longer changes (kept until
// evicted); a day still being written is kept todayTTL.
// ponytail: the last replayCache days built, oldest evicted (≈ 120 KB + frames each); an LRU if scrubbing gets heavy.
var replays = struct {
	sync.Mutex
	m     map[string]cached
	order []string
}{m: map[string]cached{}}

type cached struct {
	b     []byte
	until time.Time // zero = no expiry
}

// scrubBuilds caps concurrent replay builds across all clients: they share the pool with the single
// writer (MaxConns 8), so a burst of scrubs must not starve frame/snapshot writes. Full = "rate".
var scrubBuilds = make(chan struct{}, 2)

func init() { onClientMsg = clientMsg }

// clientMsg handles client frames. Only scrub exists so far (subscribe and mark: W3/W4); others are ignored.
func clientMsg(c *Client, msg []byte) {
	var f struct{ T, Day string }
	if json.Unmarshal(msg, &f) != nil || f.T != "scrub" {
		return
	}
	now := time.Now()
	if now.Sub(c.scrubAt) < scrubEvery {
		send(c, replayErr("", "rate"))
		return
	}
	c.scrubAt = now
	send(c, app.replay(f.Day, now))
}

// send queues b to c alone. A full buffer drops the client (as the hub does), so a paused client reconnects
// and resyncs through hello instead of waiting forever on a replay that never left.
func send(c *Client, b []byte) {
	if !c.Send(b) {
		log.Printf("scrub: %s send buffer full, connection dropped", c.IP)
		ScrubDropped.Add("", 1)
		c.conn.CloseNow()
	}
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
	replays.Lock()
	e, ok := replays.m[day]
	replays.Unlock()
	if ok && (e.until.IsZero() || now.Before(e.until)) {
		return e.b
	}
	select {
	case scrubBuilds <- struct{}{}:
		defer func() { <-scrubBuilds }()
	default:
		return replayErr(day, "rate")
	}
	// runs on the connection's reader: stay well under the 10 s ping write timeout (hub.go)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := a.buildReplay(ctx, day)
	if err != nil {
		log.Printf("scrub %s: %v", day, err)
		return replayErr(day, "unavailable")
	}
	e = cached{b: b}
	if now.Before(d.AddDate(0, 0, 1).Add(dayGrace)) {
		e.until = now.Add(todayTTL)
	}
	replays.Lock()
	if _, ok := replays.m[day]; !ok {
		replays.order = append(replays.order, day)
		if len(replays.order) > replayCache {
			delete(replays.m, replays.order[0])
			replays.order = slices.Delete(replays.order, 0, 1)
		}
	}
	replays.m[day] = e
	replays.Unlock()
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

package main

// toy-clouds (03-model-toy-clouds, W4 lane): a visitor spawns a cloud above a spot; the relay owns it from there.
// Validate: pose {pos} → Spot (x/z quantised, y = heightSolid, above the snowline), weight 1.
// Tick (every toyTick): a new mark spawns cloud <mark id> cloudAlt over its spot; every cloud drifts on metrics.wind
// (pos = spawn + ∫ wind · cloudDrift dt, integrated per tick), merges with any cloud within cloudMerge (the older id
// keeps it, position weighted by members), and rains once its members' marks of the last hour come from 3 distinct
// accounts (a.MarkSubs): state rain + world:rain{x, z, r}, then gone after rainTicks. A cloud also goes when it
// leaves the world, when its newest mark is older than cloudLife, or when all its marks were deleted (admin
// moderation; credits follow the live marks). world:cloud {id, pos, state, credits} goes out on any change and after
// every cloudStep of drift (the client extrapolates the drift in between).

import (
	"maps"
	"math"
	"slices"
	"strings"
	"time"
)

const (
	cloudAlt   = 14.0 // units over the spawn spot
	cloudDrift = 0.05 // units per second per unit of wind (wind ≤ 1: ≤ 180 units an hour); models/toy-clouds DRIFT
	cloudMerge = 10.0 // xz distance that merges two clouds
	cloudStep  = 1.0  // drift that re-sends a cloud's pos
	cloudLife  = time.Hour
	rainNeeds  = 3 // distinct accounts within cloudLife (03 rules)
	rainTicks  = 4 // ticks of rain before the cloud dissipates
	rainR      = 10.0
)

func init() { RegisterToy("toy-clouds", Toy{Validate: validateCloud, Tick: tickClouds}) }

func validateCloud(p *Place) (map[string]any, float64, string) {
	pos, why := p.Spot("pos")
	if why != "" {
		return nil, 0, why
	}
	return map[string]any{"pos": pos}, 1, ""
}

type toyCloud struct {
	pos, sent [3]float64
	members   []string // mark ids, the founder (= cloud id) first
	state     string
	rain      int  // ticks of rain left
	known     bool // clients have it
	dirty     bool
}

// clouds and seen (mark ids already spawned) are the Tick's own memory (one goroutine: no lock). clouds nil = not
// loaded yet: the first tick seeds it from hello.state.clouds and takes every mark older than a tick as seen.
// ponytail: membership lives here only, so after a restart a merged cloud keeps just its founder (credits and the
// rain count shrink to it); persist members in a private column if that ever matters.
var (
	clouds map[string]*toyCloud
	seen   map[string]bool
)

type cloudMark struct {
	pos     [3]float64
	by, sub string
	placed  time.Time
}

func tickClouds(a *App, o *Out, now time.Time) {
	marks := map[string]cloudMark{}
	a.mu.Lock()
	wind := a.Metrics.Wind
	for _, m := range a.State.Marks {
		v, ok := m.Pose["pos"].([]any)
		if m.Mark != "toy-clouds" || !ok || len(v) != 3 {
			continue
		}
		cm := cloudMark{by: m.By, sub: a.MarkSubs[m.ID], placed: ulidTime(m.ID)}
		for i := range 3 {
			cm.pos[i], _ = v[i].(float64)
		}
		marks[m.ID] = cm
	}
	if clouds == nil {
		clouds, seen = map[string]*toyCloud{}, map[string]bool{}
		for id, m := range marks {
			seen[id] = now.Sub(m.placed) > toyTick
		}
		for _, c := range a.State.Clouds {
			clouds[c.ID] = &toyCloud{pos: c.Pos, sent: c.Pos, members: []string{c.ID}, state: c.State, rain: 1, known: true}
			seen[c.ID] = true
		}
	}
	a.mu.Unlock()

	var out [][2]any // kind, payload
	gone := func(id string) {
		if c := clouds[id]; c.known {
			out = append(out, [2]any{"cloud", map[string]any{"id": id, "pos": slices.Clone(c.pos[:]), "state": "gone", "credits": []string{}}})
		}
		delete(clouds, id)
	}
	dt := toyTick.Seconds()
	for _, id := range slices.Sorted(maps.Keys(clouds)) {
		c, n := clouds[id], len(clouds[id].members)
		c.members = slices.DeleteFunc(c.members, func(m string) bool { _, ok := marks[m]; return !ok })
		c.dirty = len(c.members) != n
		c.pos[0] += wind.X * cloudDrift * dt
		c.pos[2] += wind.Z * cloudDrift * dt
		var newest time.Time
		for _, m := range c.members {
			if t := marks[m].placed; t.After(newest) {
				newest = t
			}
		}
		if c.state == "rain" {
			c.rain--
		}
		if len(c.members) == 0 || math.Abs(c.pos[0]) > 100 || math.Abs(c.pos[2]) > 100 || now.Sub(newest) > cloudLife || (c.state == "rain" && c.rain <= 0) {
			gone(id)
		}
	}
	// spawn: every mark once
	for id := range seen {
		if _, ok := marks[id]; !ok {
			delete(seen, id)
		}
	}
	for id, m := range marks {
		if !seen[id] && len(clouds) < cloudsMax {
			seen[id] = true
			clouds[id] = &toyCloud{pos: [3]float64{m.pos[0], m.pos[1] + cloudAlt, m.pos[2]}, members: []string{id}, state: "drift", dirty: true}
		}
	}
	// merge: a cloud clients have absorbs a new one, else the older id. ponytail: O(n²) over ≤ cloudsMax clouds a tick
	ids := slices.SortedFunc(maps.Keys(clouds), func(x, y string) int {
		if kx, ky := clouds[x].known, clouds[y].known; kx != ky {
			if kx {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	})
	for i, x := range ids {
		cx := clouds[x]
		for _, y := range ids[i+1:] {
			cy := clouds[y]
			if cx == nil || cx.state != "drift" || cy == nil || cy.state != "drift" || math.Hypot(cx.pos[0]-cy.pos[0], cx.pos[2]-cy.pos[2]) >= cloudMerge {
				continue
			}
			nx, ny := float64(len(cx.members)), float64(len(cy.members))
			for k := range 3 {
				cx.pos[k] = (cx.pos[k]*nx + cy.pos[k]*ny) / (nx + ny)
			}
			cx.members, cx.dirty = append(cx.members, cy.members...), true
			gone(y)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(clouds)) {
		c := clouds[id]
		subs, credits := map[string]bool{}, []string{}
		for _, m := range c.members {
			cm := marks[m]
			if now.Sub(cm.placed) <= cloudLife && cm.sub != "" {
				subs[cm.sub] = true
			}
			h := cm.by
			if h == "" {
				h = anonHandle
			}
			if !slices.Contains(credits, h) && len(credits) < 16 {
				credits = append(credits, h)
			}
		}
		if c.state == "drift" && len(subs) >= rainNeeds {
			c.state, c.rain, c.dirty = "rain", rainTicks, true
			out = append(out, [2]any{"rain", map[string]any{"x": c.pos[0], "z": c.pos[2], "r": rainR}})
		}
		if c.dirty || math.Hypot(c.pos[0]-c.sent[0], c.pos[2]-c.sent[2]) >= cloudStep {
			c.sent, c.known, c.dirty = c.pos, true, false
			out = append(out, [2]any{"cloud", map[string]any{"id": id, "pos": slices.Clone(c.pos[:]), "state": c.state, "credits": credits}})
		}
	}
	for _, f := range out { // a.mu released: the hub tap takes h.mu → a.mu
		o.World(f[0].(string), f[1].(map[string]any))
	}
}

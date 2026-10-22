package main

// Town plots (R7): every allowlisted app and service gets a building on the island of the host its pods run on.
// Go places them (relay/town) and broadcasts world:plot {app, host, q, r, role, ruinAt?, gone?}; the hub tap folds
// them into hello.state.plots (persisted with the snapshot, like marks), so a reload, a restart and a replay agree.
//
// Rules (plan R7 "Placement", doubt cycle):
//   - main host = the host running most of the app's pods (promQ "pods"); a tie keeps the current host, else the
//     first in h1..h3 order.
//   - a move needs the new majority for moveAfter, and never leaves a host that is NotReady (the building stays on
//     the dark island: that IS the picture of an outage).
//   - an empty pods answer, or an app with no running pod, keeps every plot.
//   - app.gone turns the building into ruins (ruinAt); ruins older than ruinFor free the plot (gone).
//   - a new plot takes a free building-ring hex, never another plot's; a visitor plant only when nothing else is
//     free, and that plant is removed first (world:mark gone).
//   - a ruin freed after ruinFor is not rebuilt while its app is still out of ArgoCD (retired), even if pods run.
//   - visitor plants wilt after wiltAfter (R7.1): the town keeps room for new ones.
// placeMu serialises plot placement with visitor plants (toy_plant.go), so both never take the same hex.

import (
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/relay/town"
)

const (
	moveAfter = 5 * time.Minute
	ruinFor   = 24 * time.Hour
	plotsMax  = 64
	wiltAfter = 7 * 24 * time.Hour
)

var placeMu sync.Mutex

// Plot is a world:plot payload (hello.state.plots).
type Plot struct {
	App    string `json:"app"`
	Host   string `json:"host"`
	Q      int    `json:"q"`
	R      int    `json:"r"`
	Role   string `json:"role"`             // town kind (not "kind": Out.frame stamps that)
	RuinAt int64  `json:"ruinAt,omitempty"` // unix ms of app.gone; 0 = standing
}

func (p Plot) hex() town.Hex { return town.Hex{Q: p.Q, R: p.R} }

func (p Plot) frame(gone bool) map[string]any {
	f := map[string]any{"app": p.App, "host": p.Host, "q": p.Q, "r": p.R, "role": p.Role}
	if p.RuinAt > 0 {
		f["ruinAt"] = int(p.RuinAt)
	}
	if gone {
		f["gone"] = true
	}
	return f
}

// roleOf: config.yaml building, else by group.
func (c *Config) roleOf(app string) string {
	a := c.Apps[app]
	if a.Building != "" {
		return a.Building
	}
	return map[string]string{"user": "home", "platform": "house", "service": "warehouse"}[a.Group]
}

// hostIndex: "h1" → 1 (mark poses carry hosts as numbers: sanitize keeps poses numeric).
func hostIndex(h string) int { return slices.Index(town.Hosts, h) + 1 }

// plantAt maps host → hex → the active plant mark there (marks oldest first, so the first seen is the oldest).
func plantAt(marks []Mark) map[string]map[town.Hex]Mark {
	out := map[string]map[town.Hex]Mark{}
	for _, m := range marks {
		if m.Mark != "plant" {
			continue
		}
		h, q, r := poseInt(m.Pose["h"]), poseInt(m.Pose["q"]), poseInt(m.Pose["r"])
		if h < 1 || h > len(town.Hosts) {
			continue
		}
		host := town.Hosts[h-1]
		if out[host] == nil {
			out[host] = map[town.Hex]Mark{}
		}
		if _, dup := out[host][town.Hex{Q: q, R: r}]; !dup {
			out[host][town.Hex{Q: q, R: r}] = m
		}
	}
	return out
}

func poseInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return -1 << 30
}

// placer is the prom source's placement memory: the pending move per app, and the apps whose ruins were freed.
type placer struct {
	cand    map[string]string    // app -> host leading since `since` (observed in every poll since then)
	since   map[string]time.Time //
	retired map[string]bool      // ruins freed: no new plot until the app is back in ArgoCD (ruin(…, false))
}

func newPlacer() *placer {
	return &placer{cand: map[string]string{}, since: map[string]time.Time{}, retired: map[string]bool{}}
}

// goneMark is the world:mark frame that removes m.
func goneMark(m Mark) map[string]any {
	return map[string]any{"id": m.ID, "mark": m.Mark, "pose": map[string]any{}, "weight": 0.0, "gone": true}
}

// place runs once per successful poll with the pods answer (namespace, pod, node series), the machines present
// (R8, machines.go) and the hosts' Ready.
func (pl *placer) place(a *App, o *Out, pods []sample, machines []string, ready map[string]bool, now time.Time) {
	counts := map[string]map[string]int{}
	for _, s := range pods {
		app := a.Cfg.appOfWork(s.L["namespace"], s.L["pod"])
		if app == "" {
			continue
		}
		h := a.Cfg.host(s.L["node"])
		if h == "" {
			continue
		}
		if counts[app] == nil {
			counts[app] = map[string]int{}
		}
		counts[app][h]++
	}

	placeMu.Lock()
	defer placeMu.Unlock()
	a.mu.Lock()
	plots := map[string]Plot{}
	for _, p := range a.State.Plots {
		plots[p.App] = p
	}
	plants := plantAt(a.State.Marks)
	a.mu.Unlock()

	emit := func(p Plot, gone bool) {
		if gone {
			delete(plots, p.App)
		} else {
			plots[p.App] = p
		}
		o.World("plot", p.frame(gone))
	}
	slot := func(app, host string) (town.Hex, bool) {
		taken := map[town.Hex]bool{}
		for _, p := range plots {
			if p.Host == host && p.App != app {
				taken[p.hex()] = true
			}
		}
		avoid := map[town.Hex]bool{}
		for x := range plants[host] {
			avoid[x] = true
		}
		x, ok := town.Slot(app, host, a.Cfg.roleOf(app), taken, avoid)
		if m, planted := plants[host][x]; ok && planted { // the island is full: the plant on the chosen hex gives way
			o.World("mark", goneMark(m))
			a.store.unmark(m.ID, now)
			delete(plants[host], x)
		}
		return x, ok
	}

	// ruins past ruinFor free their plot
	for _, app := range slices.Sorted(maps.Keys(plots)) {
		if p := plots[app]; p.RuinAt > 0 && now.Sub(time.UnixMilli(p.RuinAt)) >= ruinFor {
			emit(p, true)
			pl.retired[app] = true
		}
	}
	// a move needs its majority in every poll for moveAfter: no answer, or the app missing from it, starts over
	for app := range pl.cand {
		if counts[app] == nil {
			delete(pl.cand, app)
		}
	}
	// R8 machines: on their configured host, never moved; before the pods answer, which says nothing about them
	for _, app := range machines {
		if _, has := plots[app]; has || pl.retired[app] || len(plots) >= plotsMax {
			continue
		}
		h := a.Cfg.Apps[app].Host
		if x, ok := slot(app, h); ok {
			emit(Plot{App: app, Host: h, Q: x.Q, R: x.R, Role: a.Cfg.roleOf(app)}, false)
		} else {
			slotFailed(app)
		}
	}
	if len(pods) == 0 { // an empty answer is no answer (KSM restarting, or on the host that went down)
		return
	}
	for _, app := range slices.Sorted(maps.Keys(counts)) {
		if pl.retired[app] {
			continue
		}
		c := counts[app]
		cur, has := plots[app]
		best, n := "", -1
		for _, h := range town.Hosts {
			if c[h] > n || (c[h] == n && has && h == cur.Host) {
				best, n = h, c[h]
			}
		}
		switch {
		case !has:
			if len(plots) >= plotsMax {
				continue
			}
			if x, ok := slot(app, best); ok {
				emit(Plot{App: app, Host: best, Q: x.Q, R: x.R, Role: a.Cfg.roleOf(app)}, false)
			} else {
				slotFailed(app)
			}
			delete(pl.cand, app)
		case best == cur.Host || !ready[cur.Host]:
			delete(pl.cand, app)
		case pl.cand[app] != best:
			pl.cand[app], pl.since[app] = best, now
		case now.Sub(pl.since[app]) >= moveAfter:
			if x, ok := slot(app, best); ok {
				emit(Plot{App: app, Host: best, Q: x.Q, R: x.R, Role: cur.Role, RuinAt: cur.RuinAt}, false)
			}
			delete(pl.cand, app)
		}
	}
}

// slotFailed logs (key only) and counts an app that found no free hex on its island, once per app.
func slotFailed(app string) {
	if _, dup := slotFailSeen.LoadOrStore(app, true); !dup {
		log.Printf("plots: no free hex for %s", app)
		SlotFailed.Add("", 1)
	}
}

var slotFailSeen sync.Map

// ruin marks app's building as ruins (app.gone) or, ruins = false, rebuilds it (the app is back in ArgoCD).
func (pl *placer) ruin(a *App, o *Out, app string, ruins bool, now time.Time) {
	placeMu.Lock()
	defer placeMu.Unlock()
	if !ruins {
		delete(pl.retired, app) // back in ArgoCD: it may have a plot again
	}
	a.mu.Lock()
	i := slices.IndexFunc(a.State.Plots, func(p Plot) bool { return p.App == app })
	var p Plot
	if i >= 0 {
		p = a.State.Plots[i]
	}
	a.mu.Unlock()
	if i < 0 || (p.RuinAt > 0) == ruins {
		return
	}
	p.RuinAt = 0
	if ruins {
		p.RuinAt = now.UnixMilli()
	}
	o.World("plot", p.frame(false))
}

// wilt removes the visitor plants older than wiltAfter (world:mark gone; their rows get deleted_at). Run from the prom
// poll; the mark id (a ULID) is the planting time.
func (a *App) wilt(o *Out, now time.Time) {
	placeMu.Lock()
	defer placeMu.Unlock()
	a.mu.Lock()
	var old []Mark
	for _, m := range a.State.Marks {
		if t := ulidTime(m.ID); m.Mark == "plant" && !t.IsZero() && now.Sub(t) >= wiltAfter {
			old = append(old, m)
			delete(a.MarkSubs, m.ID)
		}
	}
	a.mu.Unlock()
	for _, m := range old {
		o.World("mark", goneMark(m))
		a.store.unmark(m.ID, now)
	}
}

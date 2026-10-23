package main

// Annexes (R8 deploy C, infra-bootstrap ADR-0065): an app's tent on each OTHER host where it also runs pods. The
// building stays on the majority host (plots.go); a tent says "some of it runs here too", never how many.
//
//   - one rule, checked every poll (annexOK): a tent stands only beside a standing plot (no ruins) of a non-machine
//     app, on another host. A published tent failing it goes at once (ruins, a freed plot, a move onto its host);
//     ruin() applies it too, and wanted is only ever computed for apps passing it.
//   - wanted: a host other than the plot's with at least one running pod that no Job owns (promQ "annexpods": a
//     migration or backup Job never puts up a tent). The pods answer still decides the plots.
//   - a difference between wanted and published must hold annexHold in every poll, both ways; a flap starts over.
//     Blind (annexpods empty or failed) clears every clock and changes nothing; an app missing from the answer clears
//     its own; a NotReady host, or the host a move is pending to, clears its clocks and keeps its tents (the dark
//     island keeps its picture; no tent flashes up just before the building moves there). After a restart the
//     restored tents stand and any difference starts the clock.
//   - a tent takes only a free grass hex, outskirts first (the Industry ring order, keyed app@host): never a plot's,
//     another tent's or a plant's. Plots come first: a plot may take a tent's hex (the tent goes, then the plot
//     lands, and the tent is placed again in the same poll); a plant may not (toy_plant.go: "plot").

import (
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/relay/town"
)

const annexesMax = 48 // a fold and sanitize guard: 16 non-machine apps × 2 other hosts = 32 at most

// annexHold: how long a tent's appearance or departure must hold. A var: the fixture hook shortens it.
var annexHold = 5 * time.Minute

// Annex is a world:annex payload (hello.state.annexes).
type Annex struct {
	App  string `json:"app"`
	Host string `json:"host"`
	Q    int    `json:"q"`
	R    int    `json:"r"`
}

func (x Annex) hex() town.Hex       { return town.Hex{Q: x.Q, R: x.R} }
func (x Annex) key() string         { return x.App + "@" + x.Host }
func annexKey(app, h string) string { return app + "@" + h }

func (x Annex) frame(gone bool) map[string]any {
	f := map[string]any{"app": x.App, "host": x.Host, "q": x.Q, "r": x.R}
	if gone {
		f["gone"] = true
	}
	return f
}

// annexOK: may app have a tent on host h, given the plots? (the one rule, above)
func (c *Config) annexOK(plots map[string]Plot, app, h string) bool {
	p, ok := plots[app]
	return ok && p.RuinAt == 0 && p.Host != h && !c.machine(app)
}

// annexSlot: a free grass hex for app's tent on host h, outskirts first. taken: every plot, tent and plant hex there.
func annexSlot(app, h string, taken map[town.Hex]bool) (town.Hex, bool) {
	return town.Slot(annexKey(app, h), h, "hall", taken, nil) // "hall": the Industry ring order; "tent" is no plot kind
}

// annexFailed logs (key only) a tent that found no free hex, once per tent; not counted (no metric).
func annexFailed(key string) {
	if _, dup := annexFailSeen.LoadOrStore(key, true); !dup {
		log.Printf("annexes: no free hex for %s", key)
	}
}

var annexFailSeen sync.Map

// annexTaken: the hexes of host h a tent may not take (plots, tents, plants).
func annexTaken(h string, plots map[string]Plot, annexes map[string]Annex, plants map[string]map[town.Hex]Mark) map[town.Hex]bool {
	taken := map[town.Hex]bool{}
	for _, p := range plots {
		if p.Host == h {
			taken[p.hex()] = true
		}
	}
	for _, x := range annexes {
		if x.Host == h {
			taken[x.hex()] = true
		}
	}
	for x := range plants[h] {
		taken[x] = true
	}
	return taken
}

// annexes runs once per successful poll, after place(), with the annexpods answer (ok = the optional query answered).
func (pl *placer) annexes(a *App, o *Out, pods []sample, ok bool, ready map[string]bool, now time.Time) {
	placeMu.Lock()
	defer placeMu.Unlock()
	a.mu.Lock()
	plots := map[string]Plot{}
	for _, p := range a.State.Plots {
		plots[p.App] = p
	}
	pub := map[string]Annex{}
	for _, x := range a.State.Annexes {
		pub[x.key()] = x
	}
	plants := plantAt(a.State.Marks)
	a.mu.Unlock()
	evicted := pl.evicted
	pl.evicted = map[string]bool{}

	// the rule, every poll, blind or not
	for _, k := range slices.Sorted(maps.Keys(pub)) {
		if x := pub[k]; !a.Cfg.annexOK(plots, x.App, x.Host) {
			o.World("annex", x.frame(true))
			delete(pub, k)
			delete(pl.acand, k)
		}
	}
	if !ok || len(pods) == 0 { // an empty answer is no answer: nothing changes, every clock starts over
		clear(pl.acand)
		return
	}
	present, want := map[string]bool{}, map[string]bool{}
	for _, s := range pods {
		app, h := a.Cfg.appOfWork(s.L["namespace"], s.L["pod"]), a.Cfg.host(s.L["node"])
		if app == "" || h == "" {
			continue
		}
		present[app] = true
		if a.Cfg.annexOK(plots, app, h) {
			want[annexKey(app, h)] = true
		}
	}
	keys := slices.Collect(maps.Keys(want))
	for k := range pub {
		if !want[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		app, h := splitKey(k)
		w, x := want[k], pub[k]
		_, p := pub[k]
		down := false
		if r, known := ready[h]; known && !r {
			down = true
		}
		switch {
		case w == p, !present[app], down, pl.cand[app] == h:
			delete(pl.acand, k)
			continue
		case w && evicted[k]: // a plot took its hex this poll: nothing new to hold, place it again now
		default:
			since, seen := pl.acand[k]
			if !seen {
				pl.acand[k] = now
				continue
			}
			if now.Sub(since) < annexHold {
				continue
			}
		}
		delete(pl.acand, k)
		if p {
			o.World("annex", x.frame(true))
			delete(pub, k)
			continue
		}
		if len(pub) >= annexesMax {
			continue
		}
		hx, ok := annexSlot(app, h, annexTaken(h, plots, pub, plants))
		if !ok {
			annexFailed(k)
			continue
		}
		x = Annex{App: app, Host: h, Q: hx.Q, R: hx.R}
		o.World("annex", x.frame(false))
		pub[k] = x
	}
	for k := range pl.acand { // keys neither wanted nor published any more
		if !want[k] {
			if _, p := pub[k]; !p {
				delete(pl.acand, k)
			}
		}
	}
}

func splitKey(k string) (app, h string) {
	i := len(k) - 3 // hosts are h1..h3
	return k[:i], k[i+1:]
}

// dropAnnexes sends gone for app's published tents (all of them, or only the one on host h). Under placeMu.
func dropAnnexes(a *App, o *Out, app, h string) {
	a.mu.Lock()
	var gone []Annex
	for _, x := range a.State.Annexes {
		if x.App == app && (h == "" || x.Host == h) {
			gone = append(gone, x)
		}
	}
	a.mu.Unlock()
	for _, x := range gone {
		o.World("annex", x.frame(true))
	}
}

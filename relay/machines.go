package main

// Machines (R8 deploy B, infra-bootstrap ADR-0064): the Terraform topology drawn as buildings, under role keys.
//
//   - a probed guest (config.yaml probe: guest-<role>) is present while blackbox-exporter has a probe_success series
//     for it (0 = not answering, still present); a VM (node: k8s-…) while its node is in the Ready answer.
//   - an empty or failed answer is no answer (blind): nothing changes and every clock restarts, so a dead
//     blackbox-exporter never ruins seven buildings.
//   - absent for machineGone (goneAfter) while its source answers: app.gone, ruins (the shared ruin path); present again: rebuilt,
//     or placed anew if its ruin was freed. Always on its configured host: a machine never moves.
//   - a probed guest's up/down (ev machine) is shown only once it has held machineHold, both ways, and never before:
//     a guest with no published state is "not known yet". After a restart the restored state stands; any difference,
//     or none restored (a memory-only boot), starts the clock. So the town is never a real-time oracle for one box.
//   - a VM has no state of its own: it greys with its island (host Ready), like every building (owner, 2026-10-22).
//   - a guest-*k9s* target is dropped without a trace; any other target not in config.yaml counts as unmapped
//     ("probe"), and so does a configured probe once it is ruined (its target is gone or renamed).

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// machineHold: how long a guest's up/down must hold before the town shows it; machineGone: how long a machine may be
// missing from its answering source before it is ruined. Vars: the fixture hook shortens them (never --dev alone).
var (
	machineHold = 10 * time.Minute
	machineGone = goneAfter
)

type guestCand struct {
	state string
	since time.Time
}

// machineWatch is the prom source's memory of the machines.
type machineWatch struct {
	gone map[string]time.Time  // machine -> absent since (its source answering)
	out  map[string]bool       // machine ruined by its absence: no second app.gone until it is back
	cand map[string]*guestCand // probed machine -> a state differing from the published one, since
}

func newMachineWatch() *machineWatch {
	return &machineWatch{gone: map[string]time.Time{}, out: map[string]bool{}, cand: map[string]*guestCand{}}
}

// watch runs once per successful poll with the guests and ready answers (guestsOK = the optional guests query
// answered). It emits app.gone / ev machine and ruins or rebuilds; it returns the machines present this poll, which
// place() puts on their host when they have no plot.
func (mw *machineWatch) watch(a *App, o *Out, pl *placer, guests []sample, guestsOK bool, ready []sample, now time.Time) []string {
	cfg := a.Cfg
	up := map[string]string{} // probed machine -> up|down, from this answer
	for _, s := range guests {
		t := s.L["target"]
		if strings.Contains(strings.ToLower(t), "k9s") { // the break-glass box: never drawn, named or counted
			continue
		}
		app := cfg.probes[t]
		if app == "" {
			unmapped("probe", t)
			continue
		}
		up[app] = map[bool]string{true: "up", false: "down"}[s.V == 1]
	}
	nodes := map[string]bool{}
	for _, s := range ready {
		if app := cfg.byNode[s.L["node"]]; app != "" {
			nodes[app] = true
		}
	}
	a.mu.Lock()
	published := maps.Clone(a.State.Machines)
	plotted := map[string]bool{}
	for _, p := range a.State.Plots {
		plotted[p.App] = true
	}
	a.mu.Unlock()

	var present []string
	for _, app := range slices.Sorted(maps.Keys(cfg.Apps)) {
		m := cfg.Apps[app]
		if m.Group != "machine" {
			continue
		}
		blind, here := len(ready) == 0, nodes[app]
		if m.Probe != "" {
			_, here = up[app]
			blind = !guestsOK || len(guests) == 0
		}
		if blind {
			delete(mw.gone, app)
			delete(mw.cand, app)
			continue
		}
		if !here {
			delete(mw.cand, app)
			switch {
			case mw.out[app]:
			case mw.gone[app].IsZero():
				mw.gone[app] = now
			case now.Sub(mw.gone[app]) >= machineGone:
				if plotted[app] { // never drawn (its probe never answered): nothing to ruin, only the unmapped count
					o.Ev("app.gone", map[string]any{"app": app})
					pl.ruin(a, o, app, true, now)
				}
				mw.out[app] = true
				delete(mw.gone, app)
				if m.Probe != "" {
					unmapped("probe", m.Probe)
				}
			}
			continue
		}
		delete(mw.gone, app)
		delete(mw.out, app)
		pl.ruin(a, o, app, false, now) // back: its ruins (if any) are rebuilt, and a freed plot may be placed again
		present = append(present, app)
		if m.Probe == "" {
			continue
		}
		st := up[app]
		switch c := mw.cand[app]; {
		case published[app] == st:
			delete(mw.cand, app)
		case c == nil || c.state != st:
			mw.cand[app] = &guestCand{st, now}
		case now.Sub(c.since) >= machineHold:
			o.Ev("machine", map[string]any{"app": app, "state": st})
			delete(mw.cand, app)
		}
	}
	return present
}

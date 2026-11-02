package main

// The live overlay (plan/10 "Read side": the live stores, read, never written): Prometheus and Alertmanager polled
// every 15 s and joined onto graph nodes by id. Real names throughout (admins only, R10 Audience row).
//
//   - apps: ArgoCD health and sync per Application (argocd_app_info's name is the app node's name, A1), plus the
//     requests per second Traefik serves it (rate() per series, then summed: a counter reset is handled by rate),
//     and its Argo CD syncs since atlas started, succeeded and failed, counted from argocd_app_sync_total (countSyncs).
//   - ready: each k8s node's Ready condition, as its vm node. probes: blackbox targets as the node they probe.
//   - land (the land lens): CPU, RAM and disk in use per namespace, and each realm's share of the cluster's capacity,
//     plus an explicit "unattributed" share for namespaces no realm claims. Per pod, summed by namespace, never
//     summed per host, so a DaemonSet or a replicated volume counts once.
//   - alerts: what Alertmanager has firing, placed on the node (and realm) its labels name.
//   - signals (optional, contract.Signals, each on its own cadence): per app its 5xx rate, p95 latency, the last
//     24 h of requests and its availability; per probed node its availability; pods per k8s node; and the town's day
//     (which critical alerts fired, which apps were Degraded, calm when neither). A signal that fails or answers
//     nothing is unknown: absent from the frame, never a zero.
//
// The whole live frame (a few kB) goes out when it changed, never when only its timestamp did; changes also become
// events (the "Last event" key). A source that fails keeps its last answer and is marked stale in the frame, so the
// page can say since when.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/realms"
)

const liveEvery = 15 * time.Second

const (
	maxEvents = 50
	unattrib  = "unattributed"
)

// The frame's shape is contract's (the game's town reads it too).
type (
	LiveState = contract.Live
	Day       = contract.Day
	AppLive   = contract.AppLive
	Use       = contract.Use
	Land      = contract.Land
	NodeLand  = contract.NodeLand
	AlertLive = contract.AlertLive
	Event     = contract.Event
)

type Live struct {
	rl  *realms.Realms
	hub *Hub
	idx func() *GraphIndex // the graph Classify reads (nil: none yet)

	mu     sync.Mutex
	st     LiveState
	events []Event
	nextID int64
	sent   string           // the last frame broadcast, without its timestamps
	low    map[string]int64 // add-on id -> since when it is below all ready (unix ms)
	met    *Metrics         // nil = not recorded
}

func newLive(rl *realms.Realms, hub *Hub, met *Metrics) *Live {
	return &Live{rl: rl, hub: hub, st: LiveState{T: "live", Apps: map[string]AppLive{}, Ready: map[string]bool{}, Probes: map[string]bool{},
		Land: Land{Realms: map[string]Use{}, NS: map[string]Use{}, NSRealm: map[string]string{}, Nodes: map[string]NodeLand{}}, Alerts: []AlertLive{}, Stale: map[string]int64{},
		Health: map[string]contract.Health{}}, events: []Event{}, low: map[string]int64{}, met: met}
}

// classify sets the frame's health from its answers and the latest graph (contract.Classify), keeping since when
// each add-on is below all ready. Called under mu.
func (l *Live) classify(now int64) {
	var idx *GraphIndex
	if l.idx != nil {
		idx = l.idx()
	}
	if idx == nil {
		l.st.Health = map[string]contract.Health{}
		return
	}
	for id, share := range l.st.Addons {
		if _, ok := l.low[id]; share < 1 && !ok {
			l.low[id] = now
		}
	}
	for id := range l.low {
		if share, ok := l.st.Addons[id]; !ok || share >= 1 {
			delete(l.low, id)
		}
	}
	l.st.Health = contract.Classify(contract.Topology{Kind: idx.Kind, RunsOn: idx.RunsOn}, &l.st, l.low, now)
}

// hello is the frame a client gets on connect: the build, the latest projection's seq, the live state and the recent
// events. Lock order: hub.mu (the caller) -> Graphs.mu, Live.mu -> Graphs.mu (classify reads the index); nothing
// broadcasts while holding either, and nothing holding Graphs.mu takes Live.mu.
func (a *App) hello() []byte {
	var seq int64
	if v := a.Graph.Latest(); v != nil {
		seq = v.Seq
	}
	a.Live.mu.Lock()
	defer a.Live.mu.Unlock()
	return mustJSON(map[string]any{"t": "hello", "build": a.Build, "seq": seq, "live": a.Live.st, "events": a.Live.events})
}

// event records a change and broadcasts it.
func (l *Live) event(kind, node, text string) {
	l.mu.Lock()
	l.nextID++
	e := Event{T: "ev", ID: l.nextID, TS: time.Now().UnixMilli(), Kind: kind, Node: node, Text: text}
	l.events = append(l.events, e)
	l.events = l.events[max(0, len(l.events)-maxEvents):]
	l.mu.Unlock()
	l.hub.Broadcast(mustJSON(e))
}

// update changes the state under the lock, classifies it, then broadcasts the whole frame if anything but its
// timestamps' values changed (a page rebuilds its panel on each frame: an unchanged one would close what the reader opened,
// every 15 s).
func (l *Live) update(f func(*LiveState)) {
	l.mu.Lock()
	f(&l.st)
	l.classify(time.Now().UnixMilli())
	at, alertsAt := l.st.At, l.st.AlertsAt
	l.st.At, l.st.AlertsAt = min(at, 1), min(alertsAt, 1) // whether each source has answered is a change; when is not
	key := string(mustJSON(l.st))
	l.st.At, l.st.AlertsAt = at, alertsAt
	var b []byte
	if key != l.sent {
		l.sent, b = key, mustJSON(l.st)
	}
	l.mu.Unlock()
	l.hub.Broadcast(b) // nil = nothing
}

func (l *Live) stale(source string, failing bool) {
	l.mu.Lock()
	_, was := l.st.Stale[source]
	l.mu.Unlock()
	if was == failing {
		return
	}
	l.update(func(s *LiveState) {
		if !failing {
			delete(s.Stale, source)
		} else {
			s.Stale[source] = time.Now().UnixMilli()
		}
	})
}

// loadFixture serves file (a live frame) as the live state: --dev only.
func (l *Live) loadFixture(file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var f struct {
		LiveState
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	f.T = "live"
	l.mu.Lock()
	l.st, l.events = f.LiveState, append([]Event{}, f.Events...)
	l.classify(time.Now().UnixMilli()) // the fixture's own health is the page tests' copy; the server's is computed
	for _, e := range l.events {
		l.nextID = max(l.nextID, e.ID)
	}
	l.mu.Unlock()
	return nil
}

// start polls both sources every liveEvery, each in its own goroutine.
func (l *Live) start(ctx context.Context, prom promAPI, alerts alertsAPI, idx func() *GraphIndex) {
	p := &promPoll{l: l, prom: prom, idx: idx}
	am := &amPoll{l: l, am: alerts, idx: idx, firing: map[string]AlertLive{}}
	for name, run := range map[string]func(context.Context, time.Time) error{"prom": p.poll, "alerts": am.poll} {
		l.met.SourceOK(name, float64(time.Now().Unix())) // seeded: a source that never answers still ages into the alert
		go func() {
			t := time.NewTicker(liveEvery)
			defer t.Stop()
			lastErr := ""
			for {
				err := run(ctx, time.Now())
				switch {
				case err != nil:
					if err.Error() != lastErr {
						slog.Warn(fmt.Sprintf("%s: %v", name, err))
						lastErr = err.Error()
					}
					l.stale(name, true)
				default:
					if lastErr != "" {
						slog.Info(fmt.Sprintf("%s: recovered", name))
						lastErr = ""
					}
					l.met.SourceOK(name, float64(time.Now().Unix()))
					l.stale(name, false)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}
}

// ---- Prometheus

type sample struct {
	L map[string]string
	V float64
}

// promAPI is what the live overlay asks of Prometheus: one instant query's samples. A value that isn't a number
// (NaN, ±Inf, not a string) is no sample.
type promAPI interface {
	Query(ctx context.Context, q string) ([]sample, error)
}

// promHTTP is Prometheus' HTTP API (contract.PromQueryPath). An answer is an error when its body doesn't decode or
// its status isn't "success"; the HTTP status is only reported.
type promHTTP struct {
	base string // no trailing slash
	hc   *http.Client
}

func (c promHTTP) Query(ctx context.Context, q string) ([]sample, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+contract.PromQueryPath+"?query="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r struct {
		Status string
		Data   struct {
			Result []struct {
				Metric map[string]string
				Value  [2]any
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.Status != "success" {
		return nil, fmt.Errorf("query %.40q: status %d %s %v", q, resp.StatusCode, r.Status, err)
	}
	s := make([]sample, 0, len(r.Data.Result))
	for _, x := range r.Data.Result {
		str, _ := x.Value[1].(string)
		v, err := strconv.ParseFloat(str, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		s = append(s, sample{x.Metric, v})
	}
	return s, nil
}

type promPoll struct {
	l     *Live
	prom  promAPI
	idx   func() *GraphIndex
	sig   map[string]signalAnswer // the last answer to each signal
	syncs map[string]*syncCounter // argocd_app_sync_total's (name, phase) -> what it has counted (nil: no poll yet)
}

// syncCounter is one argocd_app_sync_total series: the Application and phase it counts, its last raw value, and the
// syncs atlas has counted from it.
type syncCounter struct {
	app    string // app:<lower name>
	failed bool   // phase Failed or Error
	last   float64
	n      uint64
}

// countSyncs adds each sync series' rise since the last poll to what it has counted. The first poll only seeds:
// the controller's past syncs are not news, so atlas starts every app at 0. A series first seen later is a new one
// (Argo makes it on an app's first sync in that phase) and counts from 0. A value below the last is a reset (the
// controller restarted, its counter began again at 0), so all of it is new syncs. The counts only grow while atlas
// runs.
// ponytail: a controller that restarts and climbs past its old value between two polls (15 s) reads as a smaller
// rise, and a rolling restart's old and new pods overlap under max (the new one's syncs before the old one's series
// goes are missed); both undercount, never overcount. Per-pod series and a restart signal if that ever matters.
func (p *promPoll) countSyncs(s []sample) {
	seed := p.syncs == nil
	if seed {
		p.syncs = map[string]*syncCounter{}
	}
	for _, x := range s {
		k := x.L[contract.LabelName] + "\x00" + x.L[contract.LabelPhase]
		c := p.syncs[k]
		if c == nil {
			c = &syncCounter{app: "app:" + strings.ToLower(x.L[contract.LabelName]), failed: x.L[contract.LabelPhase] != "Succeeded"}
			if seed {
				c.last = x.V
			}
			p.syncs[k] = c
		}
		rise := x.V - c.last
		if x.V < c.last {
			rise = x.V
		}
		c.n += uint64(math.Round(max(rise, 0)))
		c.last = x.V
	}
}

// signalAnswer is a signal's last answer: asked when, and what it said (ok false: failed or empty, so unknown).
type signalAnswer struct {
	at time.Time
	ok bool
	s  []sample
}

func (p *promPoll) poll(ctx context.Context, now time.Time) error {
	r := map[string][]sample{}
	for name, q := range contract.Queries { // the sim mirrors exactly these
		s, err := p.prom.Query(ctx, q)
		if err != nil {
			p.l.met.QueryFailure(name)
			return err
		}
		r[name] = s
	}
	p.countSyncs(r["syncs"])
	p.signals(ctx, now)
	idx := p.idx()
	if idx == nil {
		idx = &GraphIndex{Realm: map[string]string{}, NSApps: map[string][]string{}, Has: map[string]bool{}}
	}
	j := &join{r: r, sig: p.sig, syncs: p.syncs, idx: idx, rl: p.l.rl}
	for _, jn := range joiners() {
		jn.run(j)
	}
	apps, ready, probes := j.apps, j.ready, j.probes

	// changes become events, then the frame goes out whole
	p.l.mu.Lock()
	old := p.l.st
	p.l.mu.Unlock()
	if old.At != 0 {
		for _, id := range slices.Sorted(maps.Keys(apps)) {
			if o, ok := old.Apps[id]; ok && o.Health != apps[id].Health && apps[id].Health != "" {
				p.l.event("health", id, fmt.Sprintf("%s: %s → %s", strings.TrimPrefix(id, "app:"), o.Health, apps[id].Health))
			}
		}
		for _, id := range slices.Sorted(maps.Keys(ready)) {
			if o, ok := old.Ready[id]; ok && o != ready[id] {
				p.l.event("ready", id, fmt.Sprintf("%s %s", strings.TrimPrefix(id, "vm:"), map[bool]string{true: "is Ready again", false: "is not Ready"}[ready[id]]))
			}
		}
		for _, id := range slices.Sorted(maps.Keys(probes)) {
			if o, ok := old.Probes[id]; ok && o != probes[id] {
				_, name, _ := strings.Cut(id, ":")
				p.l.event("probe", id, fmt.Sprintf("%s %s", name, map[bool]string{true: "answers again", false: "stopped answering"}[probes[id]]))
			}
		}
	}
	p.l.update(func(s *LiveState) {
		s.At, s.Apps, s.Ready, s.Probes, s.Land = now.UnixMilli(), apps, ready, probes, j.land
		s.ProbeDay, s.Pods, s.Day, s.Addons = j.probeDay, j.pods, j.day, j.addons
	})
	return nil
}

// signals asks each optional query whose time has come (contract.Signals' cadence). A failure is counted and makes
// the signal unknown until its next answer; it never fails the poll.
func (p *promPoll) signals(ctx context.Context, now time.Time) {
	if p.sig == nil {
		p.sig = map[string]signalAnswer{}
	}
	for _, name := range slices.Sorted(maps.Keys(contract.Signals)) {
		sg := contract.Signals[name]
		// half a tick of slack: the ticker's wake-ups jitter, and a 15 s signal must not skip every other tick
		if last, ok := p.sig[name]; ok && now.Sub(last.at) < time.Duration(sg.Every)*time.Second-liveEvery/2 {
			continue
		}
		s, err := p.prom.Query(ctx, sg.Query)
		if err != nil {
			p.l.met.QueryFailure(name)
		}
		p.sig[name] = signalAnswer{at: now, ok: err == nil && len(s) > 0, s: s}
	}
}

// ---- the joins: one poll's answers onto the graph's nodes

// join is one poll's answers (contract.Queries' and the signals' last), the graph they land on, and what the
// joiners make of them. apps, ready, probes and land are always set (the frame says {} for none). probeDay and pods
// stay nil while their signal is unknown, addons while both of its are, and day while either of its is: absent
// from the frame, never a zero.
type join struct {
	r     map[string][]sample
	sig   map[string]signalAnswer
	syncs map[string]*syncCounter
	idx   *GraphIndex
	rl    *realms.Realms

	apps     map[string]AppLive
	ready    map[string]bool
	probes   map[string]bool
	land     Land
	probeDay map[string]float64
	pods     map[string]int
	addons   map[string]float64
	day      *Day
}

// known is a signal's samples, and whether it is known (answered, and not empty).
func (j *join) known(name string) ([]sample, bool) { a := j.sig[name]; return a.s, a.ok }

// joiner is one ordered stage of the join (plan/15 §2). It has no version: the live frame is never stored.
type joiner struct {
	name string
	run  func(j *join)
}

// joiners is the join, in order: rps and syncs add to the apps that apps found (and may add one ArgoCD didn't
// report), and appsignals reads what they left.
func joiners() []joiner {
	return []joiner{
		{"apps", joinApps},
		{"rps", joinRPS},
		{"syncs", joinSyncs},
		{"ready", joinReady},
		{"probes", joinProbes},
		{"land", joinLand},
		{"appsignals", joinAppSignals},
		{"probeday", joinProbeDay},
		{"pods", joinPods},
		{"addons", joinAddons},
		{"day", joinDay},
	}
}

// joinApps: ArgoCD's health and sync per Application, by its name.
func joinApps(j *join) {
	j.apps = map[string]AppLive{}
	for _, s := range j.r["info"] {
		id := "app:" + strings.ToLower(s.L[contract.LabelName])
		if j.idx.Has[id] {
			j.apps[id] = AppLive{Health: s.L[contract.LabelHealth], Sync: s.L[contract.LabelSync]}
		}
	}
}

// joinRPS: Traefik's requests per second per Application, from the exported service
// "<namespace>-<service>-<port>@<provider>".
func joinRPS(j *join) {
	for _, s := range j.r["req"] {
		if id := appOfService(j.idx, s.L[contract.LabelExportedService]); id != "" {
			a := j.apps[id]
			a.RPS += s.V
			j.apps[id] = a
		}
	}
	for id, a := range j.apps {
		a.RPS = math.Round(a.RPS*100) / 100
		j.apps[id] = a
	}
}

// joinSyncs: each Application's syncs counted so far, succeeded and failed, summed over its series.
func joinSyncs(j *join) {
	for _, c := range j.syncs {
		if !j.idx.Has[c.app] {
			continue
		}
		a := j.apps[c.app]
		if c.failed {
			a.SyncsFailed += c.n
		} else {
			a.Syncs += c.n
		}
		j.apps[c.app] = a
	}
}

// joinReady: each k8s node's Ready condition, as its vm node.
func joinReady(j *join) {
	j.ready = map[string]bool{}
	for _, s := range j.r["ready"] {
		if id := "vm:" + strings.ToLower(s.L[contract.LabelNode]); j.idx.Has[id] {
			j.ready[id] = s.V == 1
		}
	}
}

// joinProbes: blackbox targets as the node they probe.
func joinProbes(j *join) {
	j.probes = map[string]bool{}
	for _, s := range j.r["probes"] {
		if id := j.rl.ProbeNode(s.L[contract.LabelTarget]); id != "" {
			j.probes[id] = s.V == 1
		}
	}
}

// joinLand: the land lens. Capacity, use per namespace and per k8s node, and each realm's share of the capacity.
func joinLand(j *join) {
	r, idx := j.r, j.idx
	one := func(n string) float64 {
		if len(r[n]) > 0 {
			return r[n][0].V
		}
		return 0
	}
	land := Land{Capacity: Use{CPU: one("cpucap"), Mem: one("memcap"), Disk: one("diskcap")}, Realms: map[string]Use{}, NS: map[string]Use{},
		NSRealm: map[string]string{}, Nodes: map[string]NodeLand{}}
	node := func(name string) (NodeLand, string, bool) {
		id := "vm:" + strings.ToLower(name)
		if !idx.Has[id] {
			return NodeLand{}, "", false
		}
		n, ok := land.Nodes[id]
		if !ok {
			n = NodeLand{NS: map[string]Use{}}
		}
		return n, id, true
	}
	for _, q := range []string{"cpucapnode", "memcapnode"} {
		for _, s := range r[q] {
			if n, id, ok := node(s.L[contract.LabelNode]); ok {
				if q == "cpucapnode" {
					n.Capacity.CPU = s.V
				} else {
					n.Capacity.Mem = s.V
				}
				land.Nodes[id] = n
			}
		}
	}
	for _, q := range []string{"cpunode", "memnode"} {
		for _, s := range r[q] {
			if n, id, ok := node(s.L[contract.LabelNode]); ok && s.L[contract.LabelNamespace] != "" {
				u := n.NS[s.L[contract.LabelNamespace]]
				if q == "cpunode" {
					u.CPU = math.Round(s.V*1000) / 1000
				} else {
					u.Mem = math.Round(s.V)
				}
				n.NS[s.L[contract.LabelNamespace]] = u
				land.Nodes[id] = n
			}
		}
	}
	add := func(name string, set func(*Use, float64)) {
		for _, s := range r[name] {
			ns := s.L[contract.LabelNamespace]
			if ns == "" {
				continue
			}
			u := land.NS[ns]
			set(&u, s.V)
			land.NS[ns] = u
		}
	}
	add("cpu", func(u *Use, v float64) { u.CPU = v })
	add("mem", func(u *Use, v float64) { u.Mem = v })
	add("disk", func(u *Use, v float64) { u.Disk = v })
	share := func(x, c float64) float64 {
		if c <= 0 {
			return 0
		}
		return x / c
	}
	for _, n := range land.Nodes {
		for ns := range n.NS {
			land.NSRealm[ns] = realmOfNS(idx, j.rl, ns)
		}
	}
	for ns, u := range land.NS {
		realm := realmOfNS(idx, j.rl, ns)
		land.NSRealm[ns] = realm
		t := land.Realms[realm]
		t.CPU += share(u.CPU, land.Capacity.CPU)
		t.Mem += share(u.Mem, land.Capacity.Mem)
		t.Disk += share(u.Disk, land.Capacity.Disk)
		land.Realms[realm] = t
	}
	for k, u := range land.Realms {
		land.Realms[k] = Use{CPU: round3(u.CPU), Mem: round3(u.Mem), Disk: round3(u.Disk)}
	}
	j.land = land
}

// joinAppSignals: per app, summed over its Traefik services, its 5xx rate, its p95 latency (the slowest service's),
// its last 24 h of requests and their availability. Only a known signal writes anything.
func joinAppSignals(j *join) {
	perApp := func(name string, merge func(a, v float64) float64) map[string]float64 {
		out := map[string]float64{}
		s, ok := j.known(name)
		if !ok {
			return nil
		}
		for _, x := range s {
			if id := appOfService(j.idx, x.L[contract.LabelExportedService]); id != "" {
				if v, seen := out[id]; seen {
					out[id] = merge(v, x.V)
				} else {
					out[id] = x.V
				}
			}
		}
		return out
	}
	sum := func(a, v float64) float64 { return a + v }
	ptr := func(v float64) *float64 { return &v }
	errs, lat, reqDay, errDay := perApp("err", sum), perApp("lat", math.Max), perApp("reqday", sum), perApp("errday", sum)
	for id := range j.apps {
		a := j.apps[id]
		if v, ok := errs[id]; ok {
			a.Err = ptr(math.Round(v*1000) / 1000)
		}
		if v, ok := lat[id]; ok {
			a.Lat = ptr(math.Round(v*10000) / 10000)
		}
		if v, ok := reqDay[id]; ok {
			a.ReqDay = ptr(math.Round(v))
			if e, ok := errDay[id]; ok && v > 0 {
				a.Avail = ptr(math.Round((1-e/v)*10000) / 10000)
			}
		}
		j.apps[id] = a
	}
}

// joinProbeDay: per probed node, the share of its probes that succeeded in the last 24 h.
func joinProbeDay(j *join) {
	if s, ok := j.known("probeday"); ok {
		j.probeDay = map[string]float64{}
		for _, x := range s {
			if id := j.rl.ProbeNode(x.L[contract.LabelTarget]); id != "" {
				j.probeDay[id] = math.Round(x.V*10000) / 10000
			}
		}
	}
}

// joinPods: pods per k8s node.
func joinPods(j *join) {
	if s, ok := j.known("pods"); ok {
		j.pods = map[string]int{}
		for _, x := range s {
			if id := "vm:" + strings.ToLower(x.L[contract.LabelNode]); j.idx.Has[id] {
				j.pods[id] = int(x.V)
			}
		}
	}
}

// joinAddons: an add-on's readiness, its least ready workload (realms.yaml addons); unknown when one of them isn't
// answered (a static pod, like kube-vip, never is).
func joinAddons(j *join) {
	ds, okDS := j.known("dsready")
	dep, okDep := j.known("deployready")
	if !okDS && !okDep {
		return
	}
	ready := map[string]float64{} // "<namespace>/<kind>/<name>" -> share
	for kind, list := range map[string][]sample{"daemonset": ds, "deployment": dep} {
		label := map[string]string{"daemonset": contract.LabelDaemonSet, "deployment": contract.LabelDeployment}[kind]
		for _, x := range list {
			ready[x.L[contract.LabelNamespace]+"/"+kind+"/"+x.L[label]] = x.V
		}
	}
	j.addons = map[string]float64{}
next:
	for id, wl := range j.rl.Addons {
		ns := j.idx.Into[id]
		if !j.idx.Has[id] || ns == "" || len(wl) == 0 {
			continue
		}
		least := 1.0
		for _, w := range wl {
			v, ok := ready[ns+"/"+w]
			if !ok {
				continue next
			}
			least = min(least, v)
		}
		j.addons[id] = math.Round(least*1000) / 1000
	}
}

// joinDay: the town's last 24 h, when both its signals are known: which critical alerts fired, which apps were
// Degraded, calm when neither.
func joinDay(j *join) {
	crit, okC := j.known("critday")
	deg, okD := j.known("degradedday")
	if !okC || !okD {
		return
	}
	d := &Day{Critical: []string{}, Degraded: []string{}}
	for _, x := range crit {
		if n := x.L[contract.LabelAlertName]; n != "" && x.V > 0 {
			d.Critical = append(d.Critical, n)
		}
	}
	for _, x := range deg {
		if id := "app:" + strings.ToLower(x.L[contract.LabelName]); x.V > 0 && j.idx.Has[id] {
			d.Degraded = append(d.Degraded, id)
		}
	}
	slices.Sort(d.Critical)
	slices.Sort(d.Degraded)
	d.Calm = len(d.Critical) == 0 && len(d.Degraded) == 0
	j.day = d
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

// appOfService is the Application a Traefik exported service routes to: its namespace is the longest known
// namespace prefixing it; within that namespace, the app whose name prefixes the service, else the only app there.
func appOfService(idx *GraphIndex, svc string) string {
	svc, _, _ = strings.Cut(svc, "@")
	best := ""
	for ns := range idx.NSApps {
		if strings.HasPrefix(svc, ns+"-") && len(ns) > len(best) {
			best = ns
		}
	}
	if best == "" {
		return ""
	}
	rest, cand := strings.TrimPrefix(svc, best+"-"), ""
	for _, id := range idx.NSApps[best] {
		if name := strings.TrimPrefix(id, "app:"); strings.HasPrefix(rest, name) && len(name) > len(strings.TrimPrefix(cand, "app:")) {
			cand = id
		}
	}
	if cand == "" && len(idx.NSApps[best]) == 1 {
		cand = idx.NSApps[best][0]
	}
	return cand
}

// realmOfNS is the realm a namespace's pods count toward: the one realm of every Application deploying into it, or
// realms.yaml's namespaces entry, else unattributed.
func realmOfNS(idx *GraphIndex, rl *realms.Realms, ns string) string {
	realm := ""
	for _, id := range idx.NSApps[ns] {
		switch r := idx.Realm[id]; {
		case r == "":
		case realm == "":
			realm = r
		case realm != r:
			return unattrib // two realms in one namespace: no per-pod owner to split it by
		}
	}
	if realm == "" {
		realm = rl.Namespaces[ns]
	}
	if realm == "" {
		return unattrib
	}
	return realm
}

// ---- Alertmanager

// amAlert is one alert as Alertmanager's v2 API answers it. An alias of the decoded struct, so a decode error reads as
// it always has.
type amAlert = struct {
	Fingerprint string
	StartsAt    time.Time
	Labels      map[string]string
	Annotations map[string]string
}

// alertsAPI is what the live overlay asks of Alertmanager: the active alerts, neither silenced nor inhibited.
type alertsAPI interface {
	Alerts(ctx context.Context) ([]amAlert, error)
}

// amHTTP is Alertmanager's HTTP API (contract.AlertsPath). A status other than 200, a body that doesn't decode and
// a null body are errors: a null body is no answer, not "none firing".
type amHTTP struct {
	base string // no trailing slash
	hc   *http.Client
}

func (c amHTTP) Alerts(ctx context.Context) ([]amAlert, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+contract.AlertsPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r []amAlert
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || resp.StatusCode != http.StatusOK || r == nil {
		return nil, fmt.Errorf("alertmanager: status %d, %v", resp.StatusCode, err)
	}
	return r, nil
}

type amPoll struct {
	l      *Live
	am     alertsAPI
	idx    func() *GraphIndex
	firing map[string]AlertLive // fingerprint -> alert
	warm   bool
}

func (a *amPoll) poll(ctx context.Context, now time.Time) error {
	r, err := a.am.Alerts(ctx)
	if err != nil {
		return err
	}
	idx := a.idx()
	if idx == nil {
		idx = &GraphIndex{Realm: map[string]string{}, NSApps: map[string][]string{}, Has: map[string]bool{}}
	}
	cur := map[string]AlertLive{}
	for _, x := range r {
		name := x.Labels[contract.LabelAlertName]
		if slices.Contains(contract.AlertsSkipped, name) || x.Fingerprint == "" {
			continue
		}
		node, realm := a.place(idx, x.Labels)
		cur[x.Fingerprint] = AlertLive{FP: x.Fingerprint, Name: name, Severity: x.Labels[contract.LabelSeverity], StartsAt: x.StartsAt.UnixMilli(),
			Summary: x.Annotations[contract.AnnotationSummary], Node: node, Realm: realm, Labels: x.Labels}
	}
	if a.warm { // the first answer is the state, not a burst of events
		for _, fp := range slices.Sorted(maps.Keys(a.firing)) {
			if _, ok := cur[fp]; !ok {
				a.l.event("alert.resolve", a.firing[fp].Node, a.firing[fp].Name+" no longer firing (resolved or silenced)")
			}
		}
		for _, fp := range slices.Sorted(maps.Keys(cur)) {
			if _, ok := a.firing[fp]; !ok {
				a.l.event("alert.fire", cur[fp].Node, cur[fp].Name+" ("+cur[fp].Severity+")")
			}
		}
	}
	a.firing, a.warm = cur, true
	list := append([]AlertLive{}, slices.SortedFunc(maps.Values(cur), func(x, y AlertLive) int {
		return cmp.Or(cmp.Compare(x.StartsAt, y.StartsAt), strings.Compare(x.FP, y.FP))
	})...) // [] when nothing fires, never null
	a.l.update(func(s *LiveState) { s.Alerts, s.AlertsAt = list, now.UnixMilli() })
	return nil
}

// place is the node an alert's labels name (a probe target, a k8s node, then a namespace's one Application) and the
// realm it pings at L0. A moon's probe target (ext-<name>) is not taken: a moon is off the disc and has no realm, so
// its alert would ping nowhere at L0; it places by its other labels, and the probe itself dims the moon (joinProbes).
func (a *amPoll) place(idx *GraphIndex, l map[string]string) (node, realm string) {
	if id := a.l.rl.ProbeNode(l[contract.LabelTarget]); id != "" && !strings.HasPrefix(id, realms.Ext+":") {
		return id, idx.Realm[id]
	}
	if id := "vm:" + strings.ToLower(l[contract.LabelNode]); l[contract.LabelNode] != "" && idx.Has[id] {
		return id, idx.Realm[id]
	}
	// a watched rule's alert is its watchtower's, when one node watches it: a rule like TraefikCertExpiringSoon
	// carries only cn/sans, and NetBirdUnreachable keeps blackbox's own namespace (monitoring), not NetBird's
	if w := idx.Watch[l[contract.LabelAlertName]]; len(w) == 1 {
		return w[0], idx.Realm[w[0]]
	}
	if ns := l[contract.LabelNamespace]; ns != "" {
		if apps := idx.NSApps[ns]; len(apps) == 1 {
			return apps[0], idx.Realm[apps[0]]
		}
		if r := realmOfNS(idx, a.l.rl, ns); r != unattrib {
			return "", r
		}
	}
	return "", ""
}

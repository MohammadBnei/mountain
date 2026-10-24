package main

// The live overlay (plan/10 "Read side": the live stores, read, never written): Prometheus and Alertmanager polled
// every 15 s and joined onto graph nodes by id. Real names throughout (admins only, R10 Audience row).
//
//   - apps: ArgoCD health and sync per Application (argocd_app_info's name is the app node's name, A1), plus the
//     requests per second Traefik serves it (rate() per series, then summed: a counter reset is handled by rate).
//   - ready: each k8s node's Ready condition, as its vm node. probes: blackbox targets as the node they probe.
//   - land (the land lens): CPU, RAM and disk in use per namespace, and each realm's share of the cluster's capacity,
//     plus an explicit "unattributed" share for namespaces no realm claims. Per pod, summed by namespace, never
//     summed per host, so a DaemonSet or a replicated volume counts once.
//   - alerts: what Alertmanager has firing, placed on the node (and realm) its labels name.
//
// The whole live frame (a few kB) goes out when it changed, never when only its timestamp did; changes also become
// events (the "Last event" key). A source that fails keeps its last answer and is marked stale in the frame, so the
// page can say since when.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	"github.com/MohammadBnei/mountain/internal/realms"
)

var liveEvery = 15 * time.Second

const (
	maxEvents = 50
	unattrib  = "unattributed"
)

// LiveState is the live frame and hello.live.
type LiveState struct {
	T      string             `json:"t"`  // "live"
	At     int64              `json:"at"` // unix ms of the last good Prometheus answer
	Apps   map[string]AppLive `json:"apps"`
	Ready  map[string]bool    `json:"ready"`  // vm node id -> its k8s node is Ready
	Probes map[string]bool    `json:"probes"` // node id -> its blackbox probe succeeds
	Land   Land               `json:"land"`
	Alerts []AlertLive        `json:"alerts"`
	Stale  map[string]int64   `json:"stale"` // source -> failing since (unix ms)
}

type AppLive struct {
	Health string  `json:"health,omitempty"`
	Sync   string  `json:"sync,omitempty"`
	RPS    float64 `json:"rps"`
}

// Use is what a namespace (or the cluster's capacity) holds: CPU cores, bytes of RAM, bytes of disk.
type Use struct {
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
}

type Land struct {
	Capacity Use            `json:"capacity"` // allocatable CPU and RAM, provisioned volume capacity
	Realms   map[string]Use `json:"realms"`   // realm (or "unattributed") -> share of Capacity, 0..1 per field
	NS       map[string]Use `json:"ns"`       // namespace -> use, absolute
}

type AlertLive struct {
	FP       string            `json:"fp"`
	Name     string            `json:"name"`
	Severity string            `json:"severity"`
	StartsAt int64             `json:"startsAt"`
	Summary  string            `json:"summary,omitempty"`
	Node     string            `json:"node,omitempty"`  // the node its labels name
	Realm    string            `json:"realm,omitempty"` // that node's realm, or its namespace's
	Labels   map[string]string `json:"labels"`
}

// Event is one change worth flying to: {t:"ev"}.
type Event struct {
	T    string `json:"t"` // "ev"
	ID   int64  `json:"id"`
	TS   int64  `json:"ts"`
	Kind string `json:"kind"` // alert.fire | alert.resolve | health | ready | probe | graph
	Node string `json:"node,omitempty"`
	Text string `json:"text"`
}

type Live struct {
	rl  *realms.Realms
	hub *Hub

	mu     sync.Mutex
	st     LiveState
	events []Event
	nextID int64
	sent   string // the last frame broadcast, without its timestamp
}

func newLive(rl *realms.Realms, hub *Hub) *Live {
	return &Live{rl: rl, hub: hub, st: LiveState{T: "live", Apps: map[string]AppLive{}, Ready: map[string]bool{}, Probes: map[string]bool{},
		Land: Land{Realms: map[string]Use{}, NS: map[string]Use{}}, Alerts: []AlertLive{}, Stale: map[string]int64{}}, events: []Event{}}
}

// hello is the frame a client gets on connect: the build, the latest projection's seq, the live state and the recent
// events. Lock order: hub.mu (the caller) -> Graphs.mu, Live.mu; nothing broadcasts while holding either.
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

// update changes the state under the lock, then broadcasts the whole frame if anything but its timestamp changed
// (a page rebuilds its panel on each frame: an unchanged one would close what the reader opened, every 15 s).
func (l *Live) update(f func(*LiveState)) {
	l.mu.Lock()
	f(&l.st)
	at := l.st.At
	l.st.At = 0
	key := string(mustJSON(l.st))
	l.st.At = at
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
	f.LiveState.T = "live"
	l.mu.Lock()
	l.st, l.events = f.LiveState, append([]Event{}, f.Events...)
	for _, e := range l.events {
		l.nextID = max(l.nextID, e.ID)
	}
	l.mu.Unlock()
	return nil
}

func (l *Live) start(ctx context.Context, promURL, amURL string, idx func() *GraphIndex) {
	hc := &http.Client{Timeout: 10 * time.Second}
	p := &promPoll{l: l, url: strings.TrimRight(promURL, "/"), hc: hc, idx: idx}
	am := &amPoll{l: l, url: strings.TrimRight(amURL, "/"), hc: hc, idx: idx, firing: map[string]AlertLive{}}
	for name, run := range map[string]func(context.Context, time.Time) error{"prom": p.poll, "alerts": am.poll} {
		SourceLastSuccess.Set(name, float64(time.Now().Unix())) // seeded: a source that never answers still ages into the alert
		go func() {
			t := time.NewTicker(liveEvery)
			defer t.Stop()
			lastErr := ""
			for {
				err := run(ctx, time.Now())
				switch {
				case err != nil:
					if err.Error() != lastErr {
						log.Printf("%s: %v", name, err)
						lastErr = err.Error()
					}
					l.stale(name, true)
				default:
					if lastErr != "" {
						log.Printf("%s: recovered", name)
						lastErr = ""
					}
					SourceLastSuccess.Set(name, float64(time.Now().Unix()))
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

type promPoll struct {
	l   *Live
	url string
	hc  *http.Client
	idx func() *GraphIndex
}

// promQ: name -> PromQL.
var promQ = map[string]string{
	"info":    `argocd_app_info`,
	"ready":   `kube_node_status_condition{condition="Ready",status="true"}`,
	"probes":  `max by (target) (probe_success{target=~"guest-.*|pve-.*"})`,
	"cpu":     `sum by (namespace) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m]))`,
	"mem":     `sum by (namespace) (container_memory_working_set_bytes{container!="",image!=""})`,
	"disk":    `sum by (namespace) (kubelet_volume_stats_used_bytes)`,
	"cpucap":  `sum(kube_node_status_allocatable{resource="cpu"})`,
	"memcap":  `sum(kube_node_status_allocatable{resource="memory"})`,
	"diskcap": `sum(kubelet_volume_stats_capacity_bytes)`,
	"req":     `sum by (exported_service) (rate(traefik_service_requests_total[2m]))`,
}

func query(ctx context.Context, hc *http.Client, base, q string) ([]sample, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/v1/query?query="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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

func (p *promPoll) poll(ctx context.Context, now time.Time) error {
	r := map[string][]sample{}
	for name, q := range promQ {
		s, err := query(ctx, p.hc, p.url, q)
		if err != nil {
			QueryFailed.Add(name, 1)
			return err
		}
		r[name] = s
	}
	idx := p.idx()
	if idx == nil {
		idx = &GraphIndex{Realm: map[string]string{}, NSApps: map[string][]string{}, Has: map[string]bool{}}
	}
	one := func(n string) float64 {
		if len(r[n]) > 0 {
			return r[n][0].V
		}
		return 0
	}

	apps := map[string]AppLive{}
	for _, s := range r["info"] {
		id := "app:" + strings.ToLower(s.L["name"])
		if idx.Has[id] {
			apps[id] = AppLive{Health: s.L["health_status"], Sync: s.L["sync_status"]}
		}
	}
	// Traefik: requests per second per Application, from the exported service "<namespace>-<service>-<port>@<provider>"
	for _, s := range r["req"] {
		if id := appOfService(idx, s.L["exported_service"]); id != "" {
			a := apps[id]
			a.RPS += s.V
			apps[id] = a
		}
	}
	for id, a := range apps {
		a.RPS = math.Round(a.RPS*100) / 100
		apps[id] = a
	}

	ready := map[string]bool{}
	for _, s := range r["ready"] {
		if id := "vm:" + strings.ToLower(s.L["node"]); idx.Has[id] {
			ready[id] = s.V == 1
		}
	}
	probes := map[string]bool{}
	for _, s := range r["probes"] {
		if id := p.l.rl.ProbeNode(s.L["target"]); id != "" {
			probes[id] = s.V == 1
		}
	}

	land := Land{Capacity: Use{CPU: one("cpucap"), Mem: one("memcap"), Disk: one("diskcap")}, Realms: map[string]Use{}, NS: map[string]Use{}}
	add := func(name string, set func(*Use, float64)) {
		for _, s := range r[name] {
			ns := s.L["namespace"]
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
	for ns, u := range land.NS {
		realm := realmOfNS(idx, p.l.rl, ns)
		t := land.Realms[realm]
		t.CPU += share(u.CPU, land.Capacity.CPU)
		t.Mem += share(u.Mem, land.Capacity.Mem)
		t.Disk += share(u.Disk, land.Capacity.Disk)
		land.Realms[realm] = t
	}
	for k, u := range land.Realms {
		land.Realms[k] = Use{CPU: round3(u.CPU), Mem: round3(u.Mem), Disk: round3(u.Disk)}
	}

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
		s.At, s.Apps, s.Ready, s.Probes, s.Land = now.UnixMilli(), apps, ready, probes, land
	})
	return nil
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

type amPoll struct {
	l      *Live
	url    string
	hc     *http.Client
	idx    func() *GraphIndex
	firing map[string]AlertLive // fingerprint -> alert
	warm   bool
}

func (a *amPoll) poll(ctx context.Context, now time.Time) error {
	req, err := http.NewRequestWithContext(ctx, "GET", a.url+"/api/v2/alerts?active=true&silenced=false&inhibited=false", nil)
	if err != nil {
		return err
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r []struct {
		Fingerprint string
		StartsAt    time.Time
		Labels      map[string]string
		Annotations map[string]string
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || resp.StatusCode != http.StatusOK || r == nil {
		return fmt.Errorf("alertmanager: status %d, %v", resp.StatusCode, err) // a null body is no answer, not "none firing"
	}
	idx := a.idx()
	if idx == nil {
		idx = &GraphIndex{Realm: map[string]string{}, NSApps: map[string][]string{}, Has: map[string]bool{}}
	}
	cur := map[string]AlertLive{}
	for _, x := range r {
		name := x.Labels["alertname"]
		if name == "Watchdog" || name == "InfoInhibitor" || x.Fingerprint == "" {
			continue
		}
		node, realm := a.place(idx, x.Labels)
		cur[x.Fingerprint] = AlertLive{FP: x.Fingerprint, Name: name, Severity: x.Labels["severity"], StartsAt: x.StartsAt.UnixMilli(),
			Summary: x.Annotations["summary"], Node: node, Realm: realm, Labels: x.Labels}
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
	a.l.update(func(s *LiveState) { s.Alerts = list })
	return nil
}

// place is the node an alert's labels name (a probe target, a k8s node, then a namespace's one Application) and the
// realm it pings at L0.
func (a *amPoll) place(idx *GraphIndex, l map[string]string) (node, realm string) {
	if id := a.l.rl.ProbeNode(l["target"]); id != "" {
		return id, idx.Realm[id]
	}
	if id := "vm:" + strings.ToLower(l["node"]); l["node"] != "" && idx.Has[id] {
		return id, idx.Realm[id]
	}
	if ns := l["namespace"]; ns != "" {
		if apps := idx.NSApps[ns]; len(apps) == 1 {
			return apps[0], idx.Realm[apps[0]]
		}
		if r := realmOfNS(idx, a.l.rl, ns); r != unattrib {
			return "", r
		}
	}
	return "", ""
}

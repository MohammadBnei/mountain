package main

// src_prom: polls Prometheus every 15 s (02 "Counters and polls"). Counters are diffed here, per
// series, with reset handling; never increase() on short windows. Every public value goes through
// sanitize.go (allowlist lookups + the closed field spec) before it leaves.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"math"
	"math/bits"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var promEvery = 15 * time.Second // the fixture hook shortens it to the fixture's step

const (
	goneAfter  = 10 * time.Minute
	gib        = 1 << 30
	staleAfter = 40 // polls (10 min) before a vanished counter series is forgotten
)

// afterPoll runs after each successful poll: the fixture source advances its timeline here, so
// the fake Prometheus and the poll share one clock and no step goes unseen.
var afterPoll = func() {}

// optionalQ: queries whose failure is no answer (logged once per outage) instead of a failed poll.
var optionalQ = map[string]bool{"pods": true}

// promQ: name -> PromQL. The fixture server keys its timeline by these names.
var promQ = map[string]string{
	"sync":     `argocd_app_sync_total`,
	"info":     `argocd_app_info`,
	"ctrl":     `max(up{job=~".*application-controller.*"})`,
	"restarts": `kube_pod_container_status_restarts_total`,
	"oom":      `kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1`,
	"crash":    `kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"} == 1`,
	"replicas": `kube_deployment_status_replicas`,
	"ready":    `kube_node_status_condition{condition="Ready",status="true"}`,
	"alloc":    `kube_node_status_allocatable{resource="cpu"}`,
	"load":     `1 - avg by (nodename) (rate(node_cpu_seconds_total{mode="idle"}[2m]) * on(instance) group_left(nodename) node_uname_info)`,
	"quorum":   `probe_success{target=~"pve-.*"}`,
	"req":      `traefik_service_requests_total`,
	"bytes":    `traefik_service_responses_bytes_total`,
	"pvc":      `kube_persistentvolumeclaim_info`,
	"cap":      `kube_persistentvolumeclaim_resource_requests_storage_bytes`,
	"used":     `kubelet_volume_stats_used_bytes`,
	"pvcnode":  `kube_pod_spec_volumes_persistentvolumeclaims_info * on(namespace, pod) group_left(node) kube_pod_info`,
	"cpu":      `1 - avg(rate(node_cpu_seconds_total{mode="idle"}[2m]))`,
	"mem":      `1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)`,
	"rx":       `sum(rate(node_network_receive_bytes_total{device!~"lo|veth.*|cali.*|lxc.*|cilium.*|flannel.*"}[2m]))`,
	// R7 town plots: running pods per (namespace, pod, node), minus pods being deleted (a NotReady node's pods keep
	// phase Running for minutes). max by() first, so a second KSM replica or a uid churn cannot make the join fail.
	"pods": `max by (namespace, pod, node) (kube_pod_info{node!=""}) * on (namespace, pod) group_left() max by (namespace, pod) (kube_pod_status_phase{phase="Running"} == 1) unless on (namespace, pod) kube_pod_deletion_timestamp`,
}

func init() {
	Register("prom", func(a *App) (Source, error) { return newProm(a), nil })
}

type sample struct {
	L map[string]string
	V float64
}

type promSource struct {
	a     *App
	url   string
	hc    *http.Client
	warm  bool      // a first poll has set the baselines
	last  time.Time // previous successful poll
	polls int

	ctr       map[string]counter   // raw counter per series
	health    map[string]string    // app -> ArgoCD health
	seen      map[string]bool      // apps seen in argocd_app_info
	gone      map[string]time.Time // app -> absent since (controller up)
	crash     map[string]bool      // container series -> CrashLoopBackOff
	repl      map[string]int       // app -> replicas bucket
	ready     map[string]bool      // host -> Ready
	quorum    string
	base      map[string]float64 // app -> rps baseline (30 min EWMA); keyed by allowlist, bounded
	plots     *placer            // R7 town plots (plots.go)
	optWarned map[string]bool    // optionalQ already logged as failing
}

type counter struct {
	v    float64
	poll int // last poll that saw it
}

func newProm(a *App) *promSource {
	return &promSource{a: a, url: strings.TrimRight(a.Opts.PromURL, "/"), hc: &http.Client{Timeout: 10 * time.Second},
		ctr: map[string]counter{}, health: map[string]string{}, seen: map[string]bool{}, gone: map[string]time.Time{},
		crash: map[string]bool{}, repl: map[string]int{}, ready: map[string]bool{}, base: map[string]float64{}, plots: newPlacer(), optWarned: map[string]bool{}}
}

func (p *promSource) Name() string { return "prom" }

func (p *promSource) Run(ctx context.Context, out *Out) error {
	t := time.NewTicker(promEvery)
	defer t.Stop()
	for {
		if err := p.poll(ctx, out, time.Now()); err != nil {
			log.Printf("prom: %v", err)
			out.Stale()
		} else {
			out.OK()
			afterPoll()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (p *promSource) query(ctx context.Context, q string) ([]sample, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.url+"/api/v1/query?query="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.hc.Do(req)
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
		if err != nil || math.IsNaN(v) {
			continue
		}
		s = append(s, sample{x.Metric, v})
	}
	return s, nil
}

// scrapeLabels name the exporter, not the thing measured: a rescheduled kube-state-metrics changes
// them while every series it mirrors stays the same counter. (KSM is scraped with honorLabels, so its
// pod/container/namespace are the workload's; Traefik keeps its own pod label, one counter set per replica.)
var scrapeLabels = map[string]bool{"instance": true, "job": true, "endpoint": true, "service": true}

// key identifies a series by its sorted labels, minus the scrape target's.
func key(name string, l map[string]string) string {
	ks := make([]string, 0, len(l))
	for k, v := range l {
		if !scrapeLabels[k] {
			ks = append(ks, k+"="+v)
		}
	}
	slices.Sort(ks)
	return name + "{" + strings.Join(ks, ",") + "}"
}

// diff returns the counter increase since the last poll. A reset (value went down) counts the new
// value. A series first seen on the first poll is a baseline; one first seen after warm-up counts
// from 0, because the ArgoCD controller only creates argocd_app_sync_total{name,phase} on that app's
// first sync since the controller started (live 2026-10-14: wird appeared at 1 on its resync). A new
// exporter identity cannot fake a new series: key() drops instance/job/endpoint/service.
func (p *promSource) diff(k string, v float64) float64 {
	last, ok := p.ctr[k]
	p.ctr[k] = counter{v, p.polls}
	switch {
	case !ok && p.warm:
		return v
	case !ok:
		return 0
	case v < last.v:
		return v
	}
	return v - last.v
}

// prune ends a poll: counter series unseen for staleAfter polls (pods come and go) are dropped.
// Only polls whose query answered age a series (an empty answer is no answer): a scrape down
// for 10 min must not make every series come back as new and replay its full value.
func (p *promSource) prune(r map[string][]sample) {
	for k, c := range p.ctr {
		name, _, _ := strings.Cut(k, "{")
		if len(r[name]) == 0 {
			c.poll++
			p.ctr[k] = c
		} else if p.polls-c.poll >= staleAfter {
			delete(p.ctr, k)
		}
	}
	p.polls++
}

var code5xx = regexp.MustCompile(`^5\d\d$`)

// logB buckets x ≥ 0 on a log2 scale: 0 → 0, (0, 1] → 1, (1, 3] → 2, …, capped at max.
func logB(x float64, max int) int {
	return min(max, int(math.Ceil(math.Log2(1+math.Max(0, x)))))
}

// norm maps x ≥ 0 to [0, 1] on a log scale against ref, in 0.05 steps (bucketed, 02).
func norm(x, ref float64) float64 {
	return math.Round(math.Min(1, math.Log1p(math.Max(0, x))/math.Log1p(ref))*20) / 20
}

// replicaB buckets a replica count: 0..4 as-is, then 5..7 → 5, 8..15 → 6, 16..31 → 7, 32+ → 8.
func replicaB(n int) int {
	if n <= 4 {
		return max(0, n)
	}
	return min(8, 4+bits.Len(uint(n/4)))
}

func (p *promSource) poll(ctx context.Context, o *Out, now time.Time) error {
	r := map[string][]sample{}
	for name, q := range promQ {
		s, err := p.query(ctx, q)
		if err != nil && optionalQ[name] { // R7: a town query must never make every live signal stale
			QueryFailed.Add(name, 1)
			if !p.optWarned[name] {
				log.Printf("prom: optional query %s failed (no answer, plots kept): %v", name, err)
				p.optWarned[name] = true
			}
			continue
		}
		if err != nil {
			return err
		}
		delete(p.optWarned, name)
		r[name] = s
	}
	cfg := p.a.Cfg
	dt := now.Sub(p.last).Seconds()
	if p.last.IsZero() || dt <= 0 {
		dt = 0
	}

	// argo.sync: counter diff per (name, phase).
	type syncEv struct {
		app string
		ok  bool
	}
	var syncs []syncEv
	for _, s := range r["sync"] {
		if p.diff(key("sync", s.L), s.V) > 0 {
			if app := cfg.appOfArgo(s.L["name"], ""); app != "" {
				syncs = append(syncs, syncEv{app, s.L["phase"] == "Succeeded"})
			}
		}
	}
	for _, s := range syncs { // the town drops the building in on ok, flies a red flag otherwise
		o.Ev("argo.sync", map[string]any{"app": s.app, "ok": s.ok})
	}

	// argo.health + app.gone (absent 10 min while the controller is up; an empty answer is no answer).
	present := map[string]bool{}
	for _, s := range r["info"] {
		app := cfg.appOfArgo(s.L["name"], s.L["dest_namespace"])
		if app == "" {
			continue
		}
		present[app] = true
		st := s.L["health_status"]
		if old, ok := p.health[app]; ok && old != st && p.warm {
			o.Ev("argo.health", map[string]any{"app": app, "status": st})
		}
		p.health[app], p.seen[app] = st, true
		delete(p.gone, app)
		p.plots.ruin(p.a, o, app, false, now) // back in ArgoCD: its ruins (if any) are rebuilt
	}
	ctrlUp := len(r["ctrl"]) > 0 && r["ctrl"][0].V == 1
	for app := range p.seen {
		switch {
		case present[app]:
		case !ctrlUp || len(present) == 0:
			delete(p.gone, app) // controller down or Prometheus blind: restart the clock
		case p.gone[app].IsZero():
			p.gone[app] = now
		case now.Sub(p.gone[app]) >= goneAfter:
			o.Ev("app.gone", map[string]any{"app": app})
			p.plots.ruin(p.a, o, app, true, now)
			delete(p.seen, app)
			delete(p.gone, app)
			delete(p.health, app)
		}
	}

	// pod.churn: restarts diff; OOM only when restarts went up AND last_terminated_reason=OOMKilled;
	// crashloop when waiting_reason=CrashLoopBackOff goes 0 -> 1 (absent = 0).
	ctrKey := func(l map[string]string) string { return l["namespace"] + "/" + l["pod"] + "/" + l["container"] }
	oom := map[string]bool{}
	for _, s := range r["oom"] {
		oom[ctrKey(s.L)] = true
	}
	type churn struct {
		restarts       int
		oom, crashloop bool
	}
	ch := map[string]*churn{}
	get := func(l map[string]string) *churn {
		app := cfg.appOfNS(l["namespace"], l["pod"])
		if app == "" {
			return nil
		}
		if ch[app] == nil {
			ch[app] = &churn{}
		}
		return ch[app]
	}
	for _, s := range r["restarts"] {
		if d := p.diff(key("restarts", s.L), s.V); d > 0 {
			if c := get(s.L); c != nil {
				c.restarts += int(d)
				c.oom = c.oom || oom[ctrKey(s.L)]
			}
		}
	}
	crash := map[string]bool{}
	for _, s := range r["crash"] {
		k := ctrKey(s.L)
		crash[k] = true
		if !p.crash[k] && p.warm {
			if c := get(s.L); c != nil {
				c.crashloop = true
			}
		}
	}
	p.crash = crash
	for _, app := range slices.Sorted(maps.Keys(ch)) {
		c := ch[app]
		o.Ev("pod.churn", map[string]any{"app": app, "restarts": c.restarts, "oom": c.oom, "crashloop": c.crashloop})
	}

	// replicas: sum per app, bucketed, on change.
	reps := map[string]int{}
	for _, s := range r["replicas"] {
		if app := cfg.appOfNS(s.L["namespace"], s.L["deployment"]); app != "" {
			reps[app] += int(s.V)
		}
	}
	for _, app := range slices.Sorted(maps.Keys(reps)) {
		b := replicaB(reps[app])
		if old, ok := p.repl[app]; ok && old != b {
			o.Ev("replicas", map[string]any{"app": app, "bucket": b})
		}
		p.repl[app] = b
	}
	// hello.state health + replicas (summit after a reload): the evs above carry only changes, so every poll
	// also sets the current values quietly. Health passes the same enum as the ev (a raw label value never leaves).
	health := maps.Clone(p.health)
	maps.DeleteFunc(health, func(_, st string) bool { return !healthOK[st] })
	p.a.mu.Lock()
	p.a.State.Health, p.a.State.Replicas = health, maps.Clone(p.repl)
	p.a.mu.Unlock()

	// nodes -> hosts: ready = every mapped node Ready; alloc = share of cluster CPU; load = mean CPU use.
	type hostAgg struct {
		nodes, ready, nload int
		cpu, load           float64
	}
	hs := map[string]*hostAgg{"h1": {}, "h2": {}, "h3": {}}
	for _, s := range r["ready"] {
		if h := cfg.host(s.L["node"]); h != "" {
			hs[h].nodes++
			if s.V == 1 {
				hs[h].ready++
			}
		}
	}
	total := 0.0
	for _, s := range r["alloc"] {
		if h := cfg.host(s.L["node"]); h != "" {
			hs[h].cpu += s.V
			total += s.V
		}
	}
	for _, s := range r["load"] {
		if h := cfg.host(s.L["nodename"]); h != "" {
			hs[h].load += s.V
			hs[h].nload++
		}
	}
	for _, h := range []string{"h1", "h2", "h3"} {
		if len(r["ready"]) == 0 { // an empty answer is no answer (kube-state-metrics restarting): keep every host as it was
			break
		}
		rd := hs[h].nodes > 0 && hs[h].ready == hs[h].nodes
		if old, ok := p.ready[h]; ok && old != rd {
			o.Ev("node.ready", map[string]any{"host": h, "ready": rd})
		}
		p.ready[h] = rd
	}

	// town plots: where the pods run (plots.go); after the hosts' Ready, which a move depends on.
	p.plots.place(p.a, o, r["pods"], p.ready, now)
	p.a.wilt(o, now)
	p.a.mu.Lock()
	standing := 0
	for _, pl := range p.a.State.Plots {
		if pl.RuinAt == 0 {
			standing++
		}
	}
	p.a.mu.Unlock()
	Plots.Set("", float64(standing))

	// quorum: PVE probes up, as hosts.
	up := []string{}
	for _, s := range r["quorum"] {
		if h := cfg.host(s.L["target"]); h != "" && s.V == 1 && !slices.Contains(up, h) {
			up = append(up, h)
		}
	}
	slices.Sort(up)
	if q := strings.Join(up, ","); q != p.quorum && len(r["quorum"]) > 0 { // an empty answer is no answer
		if p.warm {
			o.Ev("quorum", map[string]any{"up": up}) // the tap folds it into hello.state.quorum
		} else { // the first answer is a baseline, not an event: still the state a hello carries
			p.a.mu.Lock()
			p.a.State.Quorum = up
			p.a.mu.Unlock()
		}
		p.quorum = q
	}

	// Traefik: diffed counters -> per-app rps / 5xx / bytes; river = Σ bytes over allowlisted apps.
	type traf struct{ req, err, bytes float64 }
	tr := map[string]*traf{}
	at := func(l map[string]string) *traf {
		app := cfg.appOfService(l["exported_service"])
		if app == "" {
			return nil
		}
		if tr[app] == nil {
			tr[app] = &traf{}
		}
		return tr[app]
	}
	for _, s := range r["req"] {
		if d := p.diff(key("req", s.L), s.V); d > 0 {
			if t := at(s.L); t != nil {
				t.req += d
				if code5xx.MatchString(s.L["code"]) {
					t.err += d
				}
			}
		}
	}
	for _, s := range r["bytes"] {
		if d := p.diff(key("bytes", s.L), s.V); d > 0 {
			if t := at(s.L); t != nil {
				t.bytes += d
			}
		}
	}
	apps := map[string]AppMetrics{}
	river := 0.0
	for name := range cfg.Apps { // built by allowlist
		t := tr[name]
		if t == nil {
			t = &traf{}
		}
		rps, eps, bps := 0.0, 0.0, 0.0
		if dt > 0 {
			rps, eps, bps = t.req/dt, t.err/dt, t.bytes/dt
		}
		b, ok := p.base[name]
		if dt > 0 { // no rate on the first poll: seeding the baseline with 0 made steady traffic a spike
			if !ok {
				b = rps
			}
			p.base[name] = b + (rps-b)*math.Min(1, dt/1800)
		}
		river += bps
		apps[name] = AppMetrics{RpsB: logB(rps, 10), ErrB: logB(eps, 10), Spike: b > 0.1 && rps > 3*b, BytesB: logB(bps/1024, 12)}
	}

	pools := p.pools(r)
	one := func(n string) float64 {
		if len(r[n]) > 0 {
			return r[n][0].V
		}
		return 0
	}
	rx := norm(one("rx"), 100e6)
	o.Metrics(func(m *Metrics) {
		m.Apps, m.Storage.Pools = apps, pools
		m.CPU, m.Mem = math.Round(one("cpu")*100)/100, math.Round(one("mem")*100)/100
		m.Rx, m.River = rx, norm(river, 10e6)
		for _, h := range []string{"h1", "h2", "h3"} {
			a := hs[h]
			hm := Host{}
			if rd, ok := p.ready[h]; ok {
				hm.Ready = &rd
			}
			if total > 0 {
				hm.Alloc = math.Round(a.cpu/total*100) / 100
			}
			if a.nload > 0 {
				hm.Load = math.Round(a.load/float64(a.nload)*100) / 100
			}
			m.Hosts[h] = hm
		}
		delete(m.Stale, "prom")
	})
	p.last, p.warm = now, true
	p.prune(r)
	return nil
}

// pools: one per allowlisted PVC; non-allowlisted PVCs merge into one "other" pool per class
// (local: per host). Capacity = log2 GiB bucket; level = used/cap in 5 % steps (85 % is bucket 17),
// real for longhorn only, null (unmeasured) for local-path.
func (p *promSource) pools(r map[string][]sample) []Pool {
	cfg := p.a.Cfg
	pk := func(l map[string]string) string { return l["namespace"] + "/" + l["persistentvolumeclaim"] }
	capOf, usedOf, nodeOf := map[string]float64{}, map[string]float64{}, map[string]string{}
	for _, s := range r["cap"] {
		capOf[pk(s.L)] = s.V
	}
	for _, s := range r["used"] {
		usedOf[pk(s.L)] = s.V
	}
	for _, s := range r["pvcnode"] {
		nodeOf[pk(s.L)] = s.L["node"]
	}
	type agg struct {
		app, class, host string
		cap, used        float64
		measured         bool
	}
	pvcs := r["pvc"]
	slices.SortFunc(pvcs, func(a, b sample) int { return strings.Compare(pk(a.L), pk(b.L)) })
	byID, n := map[string]*agg{}, map[string]int{}
	for _, s := range pvcs {
		class := map[string]string{"longhorn": "replicated", "local-path": "local"}[s.L["storageclass"]]
		if class == "" {
			continue
		}
		k, host := pk(s.L), ""
		if class == "local" {
			host = cfg.host(nodeOf[k])
		}
		app := cfg.appOfNS(s.L["namespace"], "")
		id := "other-" + class
		if app == "" {
			app = "other"
			if host != "" {
				id += "-" + host
			}
		} else {
			n[app]++
			id = app + "-" + strconv.Itoa(n[app])
		}
		g := byID[id]
		if g == nil {
			g = &agg{app: app, class: class, host: host}
			byID[id] = g
		}
		g.cap += capOf[k]
		if u, ok := usedOf[k]; ok && class == "replicated" {
			g.used += u
			g.measured = true
		}
	}
	// allowlisted pools first, then the other-* aggregates, so the cap never drops an app's pool
	rank := func(id string) int {
		if byID[id].app == "other" {
			return 1
		}
		return 0
	}
	ids := slices.SortedFunc(maps.Keys(byID), func(x, y string) int {
		return cmp.Or(cmp.Compare(rank(x), rank(y)), strings.Compare(x, y))
	})
	out := []Pool{}
	for _, id := range ids {
		g := byID[id]
		pl := Pool{ID: id, App: g.app, Class: g.class, Host: g.host, CapB: logB(g.cap/gib, 12)}
		if g.measured && g.cap > 0 {
			u := min(20, int(g.used/g.cap*20))
			pl.UsedB = &u
		}
		out = append(out, pl)
	}
	return out[:min(16, len(out))]
}

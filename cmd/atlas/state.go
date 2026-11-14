package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// The sources `atlas state` asks (sources.yaml ids).
const (
	srcProm    = "prom-cluster"
	srcPigsty  = "pigsty-vmetrics"
	srcPatroni = "patroni-rest"
)

// lookback is how far back a metric recipe looks for a series' last sample: far enough to say how stale a stale
// one is, where an instant query would only say "no data" after Prometheus' 5 min.
const lookback = "10m"

// Envelope is one fact (doubt-decisions Q74): who saw it, when, about what, and what, as data whose top key names
// its shape (metric, role, state). age_s is now minus when; cite is where to ask again.
type Envelope struct {
	Who   string         `json:"who"`
	When  time.Time      `json:"when"`
	About string         `json:"about"`
	What  map[string]any `json:"what"`
	AgeS  float64        `json:"age_s"`
	Via   string         `json:"via"` // the status of the alias the recipe asked by (proven, declared), or derived
	Cite  Cite           `json:"cite"`
}

// Cite is a fact's source and the query or endpoint asked.
type Cite struct {
	Source string `json:"source"`
	Query  string `json:"query"`
}

// Unknown is a fact a recipe expected and could not state: never reported fine by default.
type Unknown struct {
	About  string `json:"about"`
	What   string `json:"what"`
	Reason string `json:"reason"`
	Cite   Cite   `json:"cite"`
	Last   *Last  `json:"last,omitempty"` // the stale value, past the source's freshness limit
}

// Last is a value seen too long ago to be state.
type Last struct {
	Value any       `json:"value"`
	When  time.Time `json:"when"`
	AgeS  float64   `json:"age_s"`
}

// SourceStatus is how one source answered: ok, failing (with its error) or not asked.
type SourceStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Ms     int64  `json:"ms"`
	Error  string `json:"error,omitempty"`
}

// StateAnswer is `atlas state`'s output (§3's GET /api/state, without the route).
type StateAnswer struct {
	Asked   string         `json:"asked"`
	About   string         `json:"about"`
	Via     realms.Match   `json:"via"` // the alias row the name resolved through
	At      time.Time      `json:"at"`
	Partial bool           `json:"partial"` // a source failed or ran out of budget
	Facts   []Envelope     `json:"facts"`
	Unknown []Unknown      `json:"unknown"`
	Sources []SourceStatus `json:"sources"`
}

// errAmbiguous is a state asked by a name that is more than one thing: the answer is the candidates.
var errAmbiguous = errors.New("ambiguous name")

// ---- recipes

// metricAsk is one PromQL selector a recipe asks a metric source, and how its answer reads.
type metricAsk struct {
	source, about, what string
	selector            string
	sum                 bool   // one fact: the sum of every series' last value (connections over databases)
	via                 string // the alias row's status
}

// recipes are the asks for id and the things the answer covers with it: a cluster its members, a VM the members on
// it, an endpoint what it fronts. Per kind (door-hybrid-F.md stage 3); a kind with none says so in unknown.
func (d *door) recipes(id string) (asks []metricAsk, members []string) {
	t := d.al.Things[id]
	kind, _, _ := strings.Cut(id, ":")
	switch kind {
	case graph.KindDB:
		members = d.al.Members(id)
	case graph.KindPG:
		members = []string{id}
	case graph.KindVM:
		members = d.al.On(id)
	case graph.KindEndpoint:
		members = d.al.Members(t.Fronts)
	}
	about := []string{id} // a VM's members and an endpoint's are asked for their role only
	if kind == graph.KindDB {
		about = append(about, members...)
	}
	seen := map[metricAsk]bool{}
	for _, a := range about {
		for _, al := range d.al.Things[a].Aliases {
			for _, n := range d.al.Names(a, al.Source, al.Field) {
				for _, k := range d.askFor(a, al, n) {
					if !seen[k] {
						seen[k] = true
						asks = append(asks, k)
					}
				}
			}
		}
	}
	return asks, members
}

// askFor is what an alias of thing about lets a metric source be asked, by its field.
func (d *door) askFor(about string, al realms.Alias, name string) []metricAsk {
	q := strconv.Quote(name)
	ask := func(what, sel string, sum bool) metricAsk {
		return metricAsk{source: al.Source, about: about, what: what, selector: sel, sum: sum, via: al.Status}
	}
	kind, _, _ := strings.Cut(about, ":")
	switch {
	case al.Source == srcProm && al.Field == "target":
		return []metricAsk{ask("probe", "probe_success{target="+q+"}", false)}
	case al.Source == srcProm && al.Field == "node":
		return []metricAsk{ask("ready", `kube_node_status_condition{node=`+q+`,condition="Ready",status="true"}`, false)}
	case (al.Source == srcProm || al.Source == srcPigsty) && al.Field == "instance":
		return []metricAsk{ask("scrape", "up{instance="+q+"}", false)}
	case al.Source == srcPigsty && al.Field == "ins" && kind == graph.KindPG:
		return []metricAsk{ask("up", "pg_up{ins="+q+"}", false), ask("in_recovery", "pg_in_recovery{ins="+q+"}", false),
			ask("connections", "pg_db_numbackends{ins="+q+"}", true), ask("max_connections", "pg_setting_max_connections{ins="+q+"}", false)}
	case al.Source == srcPigsty && al.Field == "ins":
		return []metricAsk{ask("scrape", "up{ins="+q+"}", false)}
	case al.Source == srcPigsty && al.Field == "vip":
		return []metricAsk{ask("probe", `probe_success{job="pgsql-vip",vip=`+q+`}`, false)}
	}
	return nil
}

// ---- the fan-out

// state answers name: resolve it, ask every recipe's sources at once within the budget, and fold the answers.
func (d *door) state(ctx context.Context, o doorOpts, hc *http.Client, name string, now func() time.Time) (StateAnswer, error) {
	ms := d.al.Resolve(name)
	r := d.resolve(name)
	if len(r.IDs) == 0 {
		return StateAnswer{}, fmt.Errorf("state: %q resolves to nothing (aliases.yaml)", name)
	}
	if r.Ambiguous {
		return StateAnswer{}, errAmbiguous
	}
	id := r.IDs[0]
	ans := StateAnswer{Asked: name, About: id, Via: ms[0], At: now().UTC(), Facts: []Envelope{}, Unknown: []Unknown{}, Sources: []SourceStatus{}}
	asks, members := d.recipes(id)
	ctx, cancel := context.WithTimeout(ctx, o.Total)
	defer cancel()

	bySource := map[string][]metricAsk{}
	for _, a := range asks {
		bySource[a.source] = append(bySource[a.source], a)
	}
	base := map[string]string{srcProm: o.Prom, srcPigsty: o.Pigsty}
	var mu sync.Mutex
	var wg sync.WaitGroup
	run := func(src string, f func(ctx context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sctx, scancel := context.WithTimeout(ctx, o.PerSource)
			defer scancel()
			start := now()
			err := f(sctx)
			st := SourceStatus{ID: src, Status: "ok", Ms: now().Sub(start).Milliseconds()}
			if err != nil {
				st.Status, st.Error = "failing", err.Error()
			}
			mu.Lock()
			ans.Sources = append(ans.Sources, st)
			mu.Unlock()
		}()
	}
	for _, src := range slices.Sorted(maps.Keys(bySource)) {
		run(src, func(ctx context.Context) error {
			facts, unknown, err := d.askMetrics(ctx, hc, src, base[src], bySource[src], now)
			mu.Lock()
			ans.Facts, ans.Unknown = append(ans.Facts, facts...), append(ans.Unknown, unknown...)
			mu.Unlock()
			return err
		})
	}
	if len(members) > 0 {
		run(srcPatroni, func(ctx context.Context) error {
			facts, unknown, err := d.askPatroni(ctx, hc, o.Patroni, id, members, now)
			mu.Lock()
			ans.Facts, ans.Unknown = append(ans.Facts, facts...), append(ans.Unknown, unknown...)
			mu.Unlock()
			return err
		})
	}
	wg.Wait()
	if len(asks) == 0 && len(members) == 0 {
		ans.Unknown = append(ans.Unknown, Unknown{About: id, What: "state", Reason: "no recipe reads this kind in the door's first slice (prom-cluster, pigsty-vmetrics, patroni-rest)"})
	}
	for _, s := range ans.Sources {
		ans.Partial = ans.Partial || s.Status != "ok"
	}
	// Goroutines finish in any order: sort so one state of the world prints one way.
	slices.SortFunc(ans.Sources, func(a, b SourceStatus) int { return strings.Compare(a.ID, b.ID) })
	slices.SortStableFunc(ans.Facts, func(a, b Envelope) int {
		return strings.Compare(a.About+" "+a.Cite.Source+" "+a.Cite.Query, b.About+" "+b.Cite.Source+" "+b.Cite.Query)
	})
	slices.SortStableFunc(ans.Unknown, func(a, b Unknown) int {
		return strings.Compare(a.About+" "+a.What+" "+a.Cite.Query, b.About+" "+b.What+" "+b.Cite.Query)
	})
	return ans, nil
}

// askMetrics asks one Prometheus-compatible source every selector over the lookback, at once, and keeps each
// series' last sample: within the source's freshness limit it is a fact, past it unknown with the last value, and
// no series is unknown too. A query that fails makes its facts unknown and the source failing.
func (d *door) askMetrics(ctx context.Context, hc *http.Client, src, base string, asks []metricAsk, now func() time.Time) ([]Envelope, []Unknown, error) {
	limit := d.reg.Get(src).Limit
	type result struct {
		series []promSeries
		err    error
	}
	res := make([]result, len(asks))
	var wg sync.WaitGroup
	for i, a := range asks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i].series, res[i].err = promRange(ctx, hc, base, a.selector+"["+lookback+"]")
		}()
	}
	wg.Wait()
	var facts []Envelope
	var unknown []Unknown
	var errs []error
	t := now()
	for i, a := range asks {
		cite := Cite{Source: src, Query: a.selector + "[" + lookback + "]"}
		if err := res[i].err; err != nil {
			errs = append(errs, err)
			unknown = append(unknown, Unknown{About: a.about, What: a.what, Reason: src + " failing: " + err.Error(), Cite: cite})
			continue
		}
		if len(res[i].series) == 0 {
			unknown = append(unknown, Unknown{About: a.about, What: a.what, Reason: "no series in the last " + lookback, Cite: cite})
			continue
		}
		n := len(res[i].series)
		if a.sum {
			res[i].series = []promSeries{sumSeries(res[i].series)}
		}
		for _, s := range res[i].series {
			age := t.Sub(s.At).Seconds()
			m := map[string]any{"name": metricName(a.selector), "of": a.what, "value": s.V, "labels": s.L}
			if a.sum {
				m["sum_of_series"] = n
			}
			what := map[string]any{"metric": m}
			if age > limit.Seconds() {
				unknown = append(unknown, Unknown{About: a.about, What: a.what, Cite: cite,
					Reason: fmt.Sprintf("last sample %.0fs old, past %s's freshness limit %s", age, src, limit),
					Last:   &Last{Value: s.V, When: s.At.UTC(), AgeS: round1(age)}})
				continue
			}
			facts = append(facts, Envelope{Who: src, When: s.At.UTC(), About: a.about, What: what, AgeS: round1(age), Via: a.via, Cite: cite})
		}
	}
	return facts, unknown, errors.Join(errs...)
}

// scrapeLabels are the labels that say how a series was scraped, not what it is about: dropped from a fact.
var scrapeLabels = []string{"__name__", "container", "endpoint", "namespace", "pod", "service"}

// promSeries is one series' last sample in a range answer.
type promSeries struct {
	L  map[string]string
	V  float64
	At time.Time
}

// promRange asks base's instant query API a range selector and keeps each series' last sample (a value that isn't
// a number is skipped, as promHTTP does). Prometheus and VictoriaMetrics both answer a matrix.
func promRange(ctx context.Context, hc *http.Client, base, q string) ([]promSeries, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+contract.PromQueryPath+"?query="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r struct {
		Status string
		Data   struct {
			Result []struct {
				Metric map[string]string
				Values [][2]any
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.Status != "success" {
		return nil, fmt.Errorf("query %.60q: status %d %s %v", q, resp.StatusCode, r.Status, err)
	}
	var out []promSeries
	for _, x := range r.Data.Result {
		for j := len(x.Values) - 1; j >= 0; j-- {
			ts, _ := x.Values[j][0].(float64)
			str, _ := x.Values[j][1].(string)
			v, err := strconv.ParseFloat(str, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			for _, k := range scrapeLabels {
				delete(x.Metric, k)
			}
			out = append(out, promSeries{L: x.Metric, V: v, At: time.UnixMilli(int64(ts * 1000))})
			break
		}
	}
	return out, nil
}

// sumSeries is the sum of the series' last values, as old as the oldest of them, labelled with what they share
// (the fact says how many it summed).
func sumSeries(ss []promSeries) promSeries {
	out := promSeries{L: map[string]string{}, At: ss[0].At}
	maps.Copy(out.L, ss[0].L)
	for _, s := range ss {
		out.V += s.V
		if s.At.Before(out.At) {
			out.At = s.At
		}
		for k, v := range out.L {
			if s.L[k] != v {
				delete(out.L, k)
			}
		}
	}
	return out
}

func metricName(selector string) string { n, _, _ := strings.Cut(selector, "{"); return n }

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// ---- Patroni

// patroniCluster is GET /cluster's answer, the fields the door reads.
type patroniCluster struct {
	Scope   string          `json:"scope"`
	Members []patroniMember `json:"members"`
}

type patroniMember struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	State    string `json:"state"`
	Timeline int    `json:"timeline"`
	Lag      any    `json:"lag"` // a number, or "unknown" while a replica is not streaming
}

// askPatroni asks every member's REST API for /cluster at once and reads the first answer: each member's role, the
// cluster's leader and, for an endpoint, the VM that holds the VIP (vip-manager follows the leader: atlas derives
// it). With no answer, every one of those is unknown with the reason.
func (d *door) askPatroni(ctx context.Context, hc *http.Client, urls []string, id string, members []string, now func() time.Time) ([]Envelope, []Unknown, error) {
	type got struct {
		url string
		c   patroniCluster
		err error
	}
	ch := make(chan got, len(urls))
	for _, u := range urls {
		go func() {
			c, err := getPatroni(ctx, hc, u+"/cluster")
			ch <- got{u + "/cluster", c, err}
		}()
	}
	var first *got
	var errs []error
	for range urls {
		g := <-ch
		if g.err == nil {
			first = &g
			break
		}
		errs = append(errs, fmt.Errorf("%s: %w", g.url, g.err))
	}
	kind, _, _ := strings.Cut(id, ":")
	cluster := id
	if kind == graph.KindEndpoint {
		cluster = d.al.Things[id].Fronts
	}
	if first == nil {
		err := errors.Join(errs...)
		cite := Cite{Source: srcPatroni, Query: "GET " + strings.Join(urls, "/cluster, GET ") + "/cluster"}
		var unknown []Unknown
		for _, m := range members {
			unknown = append(unknown, Unknown{About: m, What: "role", Reason: srcPatroni + " failing: no member answered", Cite: cite})
		}
		if kind == graph.KindDB || kind == graph.KindEndpoint {
			unknown = append(unknown, Unknown{About: cluster, What: "leader", Reason: srcPatroni + " failing: no member answered", Cite: cite})
		}
		if kind == graph.KindEndpoint {
			unknown = append(unknown, Unknown{About: id, What: "holder", Reason: "the VIP follows the Patroni leader, which is unknown", Cite: cite})
		}
		return nil, unknown, err
	}
	t := now().UTC()
	cite := Cite{Source: srcPatroni, Query: "GET " + first.url}
	var facts []Envelope
	var unknown []Unknown
	leader := ""
	for _, m := range members {
		// The member's Patroni name is its alias there; a member without one is asked by its id's name.
		name, via := strings.TrimPrefix(m, graph.KindPG+":"), "id"
		for _, al := range d.al.Things[m].Aliases {
			if al.Source == srcPatroni && al.Field == "name" {
				name, via = al.Name, al.Status
			}
		}
		i := slices.IndexFunc(first.c.Members, func(x patroniMember) bool { return x.Name == name })
		if i < 0 {
			unknown = append(unknown, Unknown{About: m, What: "role", Reason: "patroni-rest answered without this member", Cite: cite})
			continue
		}
		x := first.c.Members[i]
		role := map[string]any{"role": x.Role, "state": x.State, "timeline": x.Timeline}
		if x.Lag != nil {
			role["lag"] = x.Lag
		}
		facts = append(facts, Envelope{Who: srcPatroni, When: t, About: m, What: map[string]any{"role": role}, AgeS: 0, Via: via, Cite: cite})
	}
	for _, x := range first.c.Members {
		if x.Role == "leader" || x.Role == "standby_leader" {
			leader = graph.ID(graph.KindPG, x.Name)
		}
	}
	if kind == graph.KindDB || kind == graph.KindEndpoint {
		if leader == "" {
			unknown = append(unknown, Unknown{About: cluster, What: "leader", Reason: "patroni-rest answered with no leader", Cite: cite})
		} else {
			facts = append(facts, Envelope{Who: srcPatroni, When: t, About: cluster, AgeS: 0, Via: realms.Proven, Cite: cite,
				What: map[string]any{"state": map[string]any{"leader": leader, "members": len(first.c.Members)}}})
		}
	}
	if kind == graph.KindEndpoint {
		if vm := d.al.RunsOn(leader); vm != "" {
			facts = append(facts, Envelope{Who: "atlas", When: t, About: id, AgeS: 0, Via: "derived", Cite: cite,
				What: map[string]any{"state": map[string]any{"holder": vm, "because": "vip-manager holds the VIP on the Patroni leader, " + leader}}})
		} else {
			unknown = append(unknown, Unknown{About: id, What: "holder", Reason: "the leader " + cmp.Or(leader, "(none)") + " has no runs_on in aliases.yaml", Cite: cite})
		}
	}
	return facts, unknown, nil
}

func getPatroni(ctx context.Context, hc *http.Client, u string) (patroniCluster, error) {
	var c patroniCluster
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return c, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return c, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return c, fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return c, fmt.Errorf("decode: %w", err)
	}
	return c, nil
}

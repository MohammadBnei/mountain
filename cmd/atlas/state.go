package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// maxSkew is how far ahead of atlas's clock a sample may be and still be read as now: past it one of the clocks is
// wrong, and the age means nothing.
const maxSkew = 5 * time.Second

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
	Asked      string         `json:"asked"`
	About      string         `json:"about"`
	Via        realms.Match   `json:"via"`        // the best alias row the name resolved through (proven, declared, historical)
	Historical bool           `json:"historical"` // the name is one the thing no longer has: every row it hit is historical
	At         time.Time      `json:"at"`
	Partial    bool           `json:"partial"` // a source failed or ran out of budget
	Facts      []Envelope     `json:"facts"`
	Unknown    []Unknown      `json:"unknown"`
	Sources    []SourceStatus `json:"sources"`
}

// statusRank orders alias statuses best first, for the row a state reports it resolved through.
func statusRank(s string) int {
	return slices.Index([]string{realms.Proven, realms.Declared, realms.Historical}, s)
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
			if al.Status == realms.Historical {
				continue // a name the thing no longer has finds nothing live
			}
			for _, n := range d.al.NamesOf(a, al) { // this row's own names, asked with this row's status
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
	via := slices.MinFunc(ms, func(x, y realms.Match) int { return cmp.Compare(statusRank(x.Status), statusRank(y.Status)) })
	ans := StateAnswer{Asked: name, About: id, Via: via, Historical: r.Historical, At: now().UTC(), Facts: []Envelope{}, Unknown: []Unknown{}, Sources: []SourceStatus{}}
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
		// A VM may host members of two clusters: each URL is asked once, and each cluster reads the answers of its own
		// scope. An answer for a scope no cluster asked is a member failing.
		groups := map[string][]string{}
		for _, m := range members {
			c := d.clusterOf(id, []string{m})
			if c == "" {
				mu.Lock()
				ans.Unknown = append(ans.Unknown, Unknown{About: m, What: "role", Reason: "member has no cluster scope (part_of in aliases.yaml)",
					Cite: Cite{Source: srcPatroni, Query: "GET " + strings.Join(o.Patroni, "/cluster, GET ") + "/cluster"}})
				mu.Unlock()
				continue
			}
			groups[c] = append(groups[c], m)
		}
		if len(groups) > 0 {
			run(srcPatroni, func(ctx context.Context) error {
				answers := askAll(ctx, hc, o.Patroni)
				asked := map[string]bool{}
				for c := range groups {
					asked[d.scopeOf(c)] = true
				}
				var errs, down, byCluster []error // down URLs are reported once, and only when a cluster got no answer
				for _, g := range answers {
					switch {
					case g.err != nil:
						down = append(down, fmt.Errorf("%s: %w", g.url, g.err))
					case !asked[g.c.Scope]:
						errs = append(errs, fmt.Errorf("%s: answered for scope %q, asked %q", g.url, g.c.Scope, strings.Join(slices.Sorted(maps.Keys(asked)), ", ")))
					}
				}
				for _, c := range slices.Sorted(maps.Keys(groups)) {
					facts, unknown, err := d.askPatroni(answers, o.Patroni, id, groups[c], now)
					mu.Lock()
					ans.Facts, ans.Unknown = append(ans.Facts, facts...), append(ans.Unknown, unknown...)
					mu.Unlock()
					byCluster = append(byCluster, err)
				}
				if slices.ContainsFunc(byCluster, func(e error) bool { return e != nil }) {
					errs = append(errs, down...)
				}
				return errors.Join(append(errs, byCluster...)...)
			})
		}
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
			// A series dated in the future is not fresh, and a sum without it is an undercount: unknown, never live.
			k := 0
			for _, s := range res[i].series {
				if t.Sub(s.At) < -maxSkew {
					k++
				}
			}
			if k > 0 {
				unknown = append(unknown, Unknown{About: a.about, What: a.what, Cite: cite,
					Reason: fmt.Sprintf("%d of %d series dated in the future (%s's clock or atlas's is wrong): the sum is not stated", k, n, src)})
				continue
			}
			res[i].series = []promSeries{sumSeries(res[i].series)}
		}
		for _, s := range res[i].series {
			age := t.Sub(s.At).Seconds()
			if s.Bad != "" {
				unknown = append(unknown, Unknown{About: a.about, What: a.what, Cite: cite, Reason: "last sample is " + strconv.Quote(s.Bad) + ", not a number"})
				continue
			}
			if age < -maxSkew.Seconds() {
				unknown = append(unknown, Unknown{About: a.about, What: a.what, Cite: cite,
					Reason: fmt.Sprintf("sample in the future: %.0fs ahead of atlas's clock (%s's clock or atlas's is wrong)", -age, src)})
				continue
			}
			age = max(age, 0) // a clock a little ahead reads as now
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
	L   map[string]string
	V   float64
	Bad string // the last sample's value when it is not a finite number (NaN, ±Inf, not a number)
	At  time.Time
}

// promRange asks base's instant query API a range selector and keeps each series' last sample (a value that isn't
// a finite number is kept as Bad). Prometheus and VictoriaMetrics both answer a matrix.
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query %.60q: status %d", q, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAnswer)).Decode(&r); err != nil || r.Status != "success" {
		return nil, fmt.Errorf("query %.60q: status %d %s %v", q, resp.StatusCode, r.Status, err)
	}
	var out []promSeries
	for _, x := range r.Data.Result {
		if len(x.Values) == 0 {
			continue
		}
		last := x.Values[len(x.Values)-1]
		ts, _ := last[0].(float64)
		str, _ := last[1].(string)
		s := promSeries{L: x.Metric, At: time.UnixMilli(int64(ts * 1000))}
		if v, err := strconv.ParseFloat(str, 64); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			s.Bad = str // JSON has no NaN or ±Inf: the series is unknown, never an older sample passed off as its last
		} else {
			s.V = v
		}
		for _, k := range scrapeLabels {
			delete(s.L, k)
		}
		out = append(out, s)
	}
	return out, nil
}

// maxAnswer bounds what one source's answer may be: a range selector over one thing is a few KB.
const maxAnswer = 4 << 20

// sumSeries is the sum of the series' last values, as old as the oldest of them, labelled with what they share
// (the fact says how many it summed).
func sumSeries(ss []promSeries) promSeries {
	out := promSeries{L: map[string]string{}, At: ss[0].At}
	maps.Copy(out.L, ss[0].L)
	for _, s := range ss {
		out.V += s.V
		out.Bad = cmp.Or(out.Bad, s.Bad)
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

// patroniReply is one member URL's answer to GET /cluster.
type patroniReply struct {
	url string
	c   patroniCluster
	err error
}

// askAll asks every member's REST API for /cluster at once and waits for each answer (within the budget).
func askAll(ctx context.Context, hc *http.Client, urls []string) []patroniReply {
	answers := make([]patroniReply, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := getPatroni(ctx, hc, u+"/cluster")
			answers[i] = patroniReply{u + "/cluster", c, err}
		}()
	}
	wg.Wait()
	return answers
}

// scopeOf is a cluster's Patroni scope: its patroni-rest scope alias, else its name.
func (d *door) scopeOf(cluster string) string {
	if n := d.al.Names(cluster, srcPatroni, "scope"); len(n) > 0 {
		return n[0]
	}
	return strings.TrimPrefix(cluster, graph.KindDB+":")
}

// askPatroni reads the answers of one cluster's scope (others are other clusters' or the caller's to report). The
// answering members must agree on the leader; then the first answer (in --patroni-url order) gives each member's
// role, the cluster's leader and, for an endpoint, the VM that holds the VIP (vip-manager follows the leader: atlas
// derives it). With no answer, or members that disagree, those are unknown with the reason. One member down while
// another answers is not a failing source.
func (d *door) askPatroni(answers []patroniReply, urls []string, id string, members []string, now func() time.Time) ([]Envelope, []Unknown, error) {
	type got = patroniReply
	kind, _, _ := strings.Cut(id, ":")
	cluster := d.clusterOf(id, members)
	scope := d.scopeOf(cluster)
	var good []got
	for _, g := range answers {
		if g.err == nil && g.c.Scope == scope { // the rest the caller reports, once
			good = append(good, g)
		}
	}
	// noAnswer marks every fact the recipe expected unknown, with why.
	noAnswer := func(reason string, cite Cite) []Unknown {
		var unknown []Unknown
		for _, m := range members {
			unknown = append(unknown, Unknown{About: m, What: "role", Reason: reason, Cite: cite})
		}
		if kind == graph.KindDB || kind == graph.KindEndpoint {
			unknown = append(unknown, Unknown{About: cluster, What: "leader", Reason: reason, Cite: cite})
		}
		if kind == graph.KindEndpoint {
			unknown = append(unknown, Unknown{About: id, What: "holder", Reason: "the VIP follows the Patroni leader, which is unknown: " + reason, Cite: cite})
		}
		return unknown
	}
	if len(good) == 0 {
		cite := Cite{Source: srcPatroni, Query: "GET " + strings.Join(urls, "/cluster, GET ") + "/cluster"}
		return nil, noAnswer(srcPatroni+" failing: no member answered for scope "+scope, cite), errors.New("no member answered for scope " + scope)
	}
	var says []string
	for _, g := range good {
		says = append(says, g.url+" says "+cmp.Or(patroniLeader(g.c), "no leader"))
	}
	if slices.ContainsFunc(good, func(g got) bool { return patroniLeader(g.c) != patroniLeader(good[0].c) }) {
		cite := Cite{Source: srcPatroni, Query: "GET " + good[0].url}
		reason := "patroni-rest members disagree on the leader: " + strings.Join(says, "; ")
		return nil, noAnswer(reason, cite), errors.New(reason)
	}
	first := good[0]
	t := now().UTC()
	cite := Cite{Source: srcPatroni, Query: "GET " + first.url}
	var facts []Envelope
	var unknown []Unknown
	for _, m := range members {
		// The member's Patroni name is its alias there; a member without one is asked by its id's name.
		name, via := strings.TrimPrefix(m, graph.KindPG+":"), "id"
		for _, al := range d.al.Things[m].Aliases {
			if al.Source == srcPatroni && al.Field == "name" && al.Status != realms.Historical {
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
	leader := ""
	if n := patroniLeader(first.c); n != "" {
		leader = graph.ID(graph.KindPG, n)
	}
	if kind == graph.KindDB || kind == graph.KindEndpoint {
		if leader == "" {
			unknown = append(unknown, Unknown{About: cluster, What: "leader", Reason: "patroni-rest answered with no leader", Cite: cite})
		} else {
			facts = append(facts, Envelope{Who: srcPatroni, When: t, About: cluster, AgeS: 0, Via: realms.Proven, Cite: cite,
				What: map[string]any{"state": map[string]any{"leader": leader, "members": len(first.c.Members), "agreed_by": len(good)}}})
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

// patroniLeader is the member an answer names leader (or standby leader), or "".
func patroniLeader(c patroniCluster) string {
	for _, x := range c.Members {
		if x.Role == "leader" || x.Role == "standby_leader" {
			return x.Name
		}
	}
	return ""
}

// clusterOf is the Postgres cluster a Patroni ask is about: the db itself, what an endpoint fronts, or the cluster
// of the (first) member.
func (d *door) clusterOf(id string, members []string) string {
	kind, _, _ := strings.Cut(id, ":")
	switch {
	case kind == graph.KindDB:
		return id
	case kind == graph.KindEndpoint:
		return d.al.Things[id].Fronts
	case len(members) > 0:
		return d.al.Things[members[0]].PartOf
	}
	return id
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAnswer)).Decode(&c); err != nil {
		return c, fmt.Errorf("decode: %w", err)
	}
	return c, nil
}

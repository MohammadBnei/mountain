package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

var doorNow = time.Date(2026, 11, 14, 12, 30, 0, 0, time.UTC)

// fakeSeries is one series a fake metric source answers: its labels, last value and that sample's age.
type fakeSeries struct {
	labels map[string]string
	v      float64
	age    time.Duration
}

// fakeMetrics is a Prometheus-compatible source answering range selectors by the metric's name and one label
// match (`name{label="value"`), a matrix of each series' last sample; status 500 for every query when fail is set.
func fakeMetrics(t *testing.T, series map[string][]fakeSeries, fail bool) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/query" {
			t.Errorf("fake metrics: %s %s", r.Method, r.URL.Path)
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":"error"}`))
			return
		}
		q := r.URL.Query().Get("query")
		if !strings.HasSuffix(q, "["+lookback+"]") {
			t.Errorf("fake metrics: %q is not a range selector over %s", q, lookback)
		}
		var res []map[string]any
		for prefix, ss := range series {
			if !strings.HasPrefix(q, prefix) {
				continue
			}
			for _, x := range ss {
				ts := float64(doorNow.Add(-x.age).UnixMilli()) / 1000
				res = append(res, map[string]any{"metric": x.labels, "values": [][2]any{{ts - 10, "0"}, {ts, fmt.Sprint(x.v)}}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": res}})
	}))
	t.Cleanup(s.Close)
	return s
}

const patroniAnswer = `{"scope":"pg-proxmox","members":[
 {"name":"pg-proxmox-1","role":"leader","state":"running","host":"192.168.1.205","timeline":79},
 {"name":"pg-proxmox-2","role":"replica","state":"streaming","host":"192.168.1.207","timeline":79,"lag":0}]}`

func fakePatroni(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster" {
			t.Errorf("fake patroni: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(patroniAnswer))
	}))
	t.Cleanup(s.Close)
	return s
}

// deadURL is an address nothing listens on.
func deadURL(t *testing.T) string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

func pgSeries(age time.Duration) map[string][]fakeSeries {
	l1 := map[string]string{"__name__": "x", "ins": "pg-proxmox-1", "cls": "pg-proxmox"}
	l2 := map[string]string{"ins": "pg-proxmox-2", "cls": "pg-proxmox"}
	return map[string][]fakeSeries{
		`pg_up{ins="pg-proxmox-1"`:                      {{l1, 1, age}},
		`pg_up{ins="pg-proxmox-2"`:                      {{l2, 1, 5 * time.Second}},
		`pg_db_numbackends{ins="pg-proxmox-1"`:          {{map[string]string{"ins": "pg-proxmox-1", "datname": "a"}, 40, 4 * time.Second}, {map[string]string{"ins": "pg-proxmox-1", "datname": "b"}, 26, 6 * time.Second}},
		`pg_setting_max_connections{ins="pg-proxmox-1"`: {{l1, 500, 4 * time.Second}},
		`up{instance="192.168.1.205:9100"`:              {{map[string]string{"job": "node"}, 1, 3 * time.Second}},
		`probe_success{job="pgsql-vip"`:                 {{map[string]string{"ins": "pg-proxmox-1"}, 1, 2 * time.Second}},
	}
}

func testDoor(t *testing.T) *door {
	t.Helper()
	d, err := loadDoor()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func opts(prom, pigsty string, patroni ...string) doorOpts {
	return doorOpts{Prom: prom, Pigsty: pigsty, Patroni: patroni, PerSource: 1500 * time.Millisecond, Total: 3 * time.Second}
}

func find(facts []Envelope, about, top string) *Envelope {
	for i, f := range facts {
		if _, ok := f.What[top]; ok && f.About == about {
			return &facts[i]
		}
	}
	return nil
}

func findUnknown(us []Unknown, about, what string) *Unknown {
	for i, u := range us {
		if u.About == about && u.What == what {
			return &us[i]
		}
	}
	return nil
}

func status(ans StateAnswer, id string) string {
	for _, s := range ans.Sources {
		if s.ID == id {
			return s.Status
		}
	}
	return "not asked"
}

func fixedNow() time.Time { return doorNow }

// TestStatePostgres is question d through the door (door-hybrid-F.md §4): the primary and its connections in one
// call, every fact with who, when, age_s and cite.
func TestStatePostgres(t *testing.T) {
	d := testDoor(t)
	prom, pig, pat := fakeMetrics(t, nil, false), fakeMetrics(t, pgSeries(4*time.Second), false), fakePatroni(t)
	ans, err := d.state(context.Background(), opts(prom.URL, pig.URL, deadURL(t), pat.URL), http.DefaultClient, "pg-proxmox", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if ans.About != "db:pg-proxmox" || ans.Via.Source == "" {
		t.Fatalf("about %s via %+v", ans.About, ans.Via)
	}
	if ans.Partial || status(ans, srcPatroni) != "ok" || status(ans, srcPigsty) != "ok" {
		t.Errorf("one dead Patroni member of two is not a failing source: %+v", ans.Sources)
	}
	l := find(ans.Facts, "db:pg-proxmox", "state")
	if l == nil || l.What["state"].(map[string]any)["leader"] != "pg:pg-proxmox-1" || l.Who != srcPatroni {
		t.Fatalf("leader: %+v", l)
	}
	if r := find(ans.Facts, "pg:pg-proxmox-2", "role"); r == nil || r.What["role"].(map[string]any)["role"] != "replica" {
		t.Errorf("replica: %+v", r)
	}
	var conns *Envelope
	for i, f := range ans.Facts {
		if m, ok := f.What["metric"].(map[string]any); ok && m["of"] == "connections" && f.About == "pg:pg-proxmox-1" {
			conns = &ans.Facts[i]
		}
	}
	if conns == nil || conns.What["metric"].(map[string]any)["value"] != 66.0 || conns.What["metric"].(map[string]any)["sum_of_series"] != 2 || conns.AgeS != 6 {
		t.Errorf("connections: the sum of the databases' backends, as old as the oldest: %+v", conns)
	}
	for _, f := range ans.Facts {
		if f.Who == "" || f.When.IsZero() || f.AgeS < 0 || f.Cite.Source == "" || f.Cite.Query == "" || len(f.What) != 1 || f.Via == "" {
			t.Errorf("a fact without its envelope: %+v", f)
		}
	}
	// pg-proxmox-2's backends were not served: unknown, never fine.
	if u := findUnknown(ans.Unknown, "pg:pg-proxmox-2", "connections"); u == nil || !strings.Contains(u.Reason, "no series") {
		t.Errorf("a missing series: %+v", u)
	}
}

// TestStateDeadPatroni: with Patroni unreachable the answer is partial, Patroni failing, and the leader and the
// roles unknown with a reason; the metric sources still answer.
func TestStateDeadPatroni(t *testing.T) {
	d := testDoor(t)
	prom, pig := fakeMetrics(t, nil, false), fakeMetrics(t, pgSeries(4*time.Second), false)
	ans, err := d.state(context.Background(), opts(prom.URL, pig.URL, deadURL(t), deadURL(t)), http.DefaultClient, "pg-proxmox", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !ans.Partial || status(ans, srcPatroni) != "failing" || status(ans, srcPigsty) != "ok" {
		t.Fatalf("partial %v, sources %+v", ans.Partial, ans.Sources)
	}
	if u := findUnknown(ans.Unknown, "db:pg-proxmox", "leader"); u == nil || !strings.Contains(u.Reason, "patroni-rest failing") || u.Cite.Source != srcPatroni {
		t.Errorf("leader: %+v", u)
	}
	for _, m := range []string{"pg:pg-proxmox-1", "pg:pg-proxmox-2"} {
		if findUnknown(ans.Unknown, m, "role") == nil || find(ans.Facts, m, "role") != nil {
			t.Errorf("%s's role is not unknown", m)
		}
	}
	if find(ans.Facts, "pg:pg-proxmox-1", "metric") == nil {
		t.Error("the metrics went with Patroni")
	}
	// The VIP's holder follows the leader: unknown too.
	ans, _ = d.state(context.Background(), opts(prom.URL, pig.URL, deadURL(t)), http.DefaultClient, "192.168.1.232", fixedNow)
	if ans.About != "endpoint:pg-proxmox/vip" || findUnknown(ans.Unknown, "endpoint:pg-proxmox/vip", "holder") == nil {
		t.Errorf("VIP holder with Patroni dead: %+v", ans.Unknown)
	}
}

// TestStateStale: a sample past the source's freshness limit is unknown with its last value, never a fact.
func TestStateStale(t *testing.T) {
	d := testDoor(t)
	prom, pig, pat := fakeMetrics(t, nil, false), fakeMetrics(t, pgSeries(2*time.Minute), false), fakePatroni(t)
	ans, err := d.state(context.Background(), opts(prom.URL, pig.URL, pat.URL), http.DefaultClient, "pg-proxmox-1", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	u := findUnknown(ans.Unknown, "pg:pg-proxmox-1", "up")
	if u == nil || u.Last == nil || u.Last.AgeS != 120 || u.Last.Value != 1.0 || !strings.Contains(u.Reason, "past pigsty-vmetrics's freshness limit 30s") {
		t.Fatalf("stale pg_up: %+v", u)
	}
	for _, f := range ans.Facts {
		if f.What["metric"] != nil && f.What["metric"].(map[string]any)["of"] == "up" {
			t.Errorf("a stale sample reported as a fact: %+v", f)
		}
	}
	if ans.Partial {
		t.Error("a stale series is not a failing source")
	}
}

// TestStateVM: a trap name reaches its VM, whose recipe asks the probe (prom-cluster), its node exporter and its
// etcd member (Pigsty) and the role of the member on it (Patroni).
func TestStateVM(t *testing.T) {
	d := testDoor(t)
	prom := fakeMetrics(t, map[string][]fakeSeries{`probe_success{target="guest-archive-h2"`: {{map[string]string{"target": "guest-archive-h2", "pod": "bb-0"}, 1, 20 * time.Second}}}, false)
	pig, pat := fakeMetrics(t, pgSeries(time.Second), false), fakePatroni(t)
	ans, err := d.state(context.Background(), opts(prom.URL, pig.URL, pat.URL), http.DefaultClient, "guest-archive-h2", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if ans.About != "vm:pg01" || ans.Via.Source != srcProm || ans.Via.Status != "proven" {
		t.Fatalf("guest-archive-h2: %s via %+v", ans.About, ans.Via)
	}
	p := find(ans.Facts, "vm:pg01", "metric")
	if p == nil {
		t.Fatalf("no probe fact: %+v", ans)
	}
	if r := find(ans.Facts, "pg:pg-proxmox-1", "role"); r == nil {
		t.Error("the member on pg01 has no role")
	}
	if u := findUnknown(ans.Unknown, "vm:pg01", "scrape"); u == nil {
		t.Error("etcd-2's scrape, not served, is not unknown")
	}
	for _, f := range ans.Facts {
		if m, _ := f.What["metric"].(map[string]any); m != nil && m["labels"].(map[string]string)["pod"] != "" {
			t.Errorf("a scrape label kept: %+v", m)
		}
	}
}

// TestStateFailing: a source that errors or outlives its budget is failing and the answer partial; the whole
// answer keeps to its budget.
func TestStateFailing(t *testing.T) {
	d := testDoor(t)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	prom, pat := fakeMetrics(t, nil, true), fakePatroni(t)
	o := opts(prom.URL, slow.URL, pat.URL)
	var ans StateAnswer
	for _, b := range [][2]time.Duration{{100 * time.Millisecond, 3 * time.Second}, {3 * time.Second, 100 * time.Millisecond}} {
		o.PerSource, o.Total = b[0], b[1]
		start := time.Now()
		var err error
		if ans, err = d.state(context.Background(), o, http.DefaultClient, "vm:pg02", time.Now); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took > time.Second {
			t.Errorf("took %s, the budgets are %s per source, %s in all", took, o.PerSource, o.Total)
		}
	}
	if !ans.Partial || status(ans, srcPigsty) != "failing" || status(ans, srcProm) != "failing" || status(ans, srcPatroni) != "ok" {
		t.Errorf("sources %+v", ans.Sources)
	}
	if u := findUnknown(ans.Unknown, "vm:pg02", "probe"); u == nil || !strings.Contains(u.Reason, "prom-cluster failing") {
		t.Errorf("probe: %+v", u)
	}
}

// TestStateRefuses: an ambiguous name is refused with the candidates, never a pick; a name that is nothing fails.
func TestStateRefuses(t *testing.T) {
	d := testDoor(t)
	if _, err := d.state(context.Background(), opts("", ""), http.DefaultClient, "proxmox", fixedNow); !errors.Is(err, errAmbiguous) {
		t.Errorf("proxmox: %v", err)
	}
	if _, err := d.state(context.Background(), opts("", ""), http.DefaultClient, "nothing-here", fixedNow); err == nil {
		t.Error("an unknown name answered")
	}
	var out, errb bytes.Buffer
	if code := doorCmd("state", []string{"proxmox"}, &out, &errb); code != 2 || !strings.Contains(out.String(), `"ambiguous": true`) {
		t.Errorf("state proxmox: exit %d, %s %s", code, out.String(), errb.String())
	}
	// An endpoint with no recipe in this slice says so.
	ans, err := d.state(context.Background(), opts("", ""), http.DefaultClient, "endpoint:proxmox/proxmox", fixedNow)
	if err != nil || len(ans.Facts) != 0 || findUnknown(ans.Unknown, "endpoint:proxmox/proxmox", "state") == nil {
		t.Errorf("no recipe: %+v %v", ans, err)
	}
}

// TestResolveCmd: the CLI's answer for the traps, and its exit codes.
func TestResolveCmd(t *testing.T) {
	for _, c := range []struct {
		name      string
		code      int
		ids       []string
		ambiguous bool
	}{
		{"guest-archive-h2", 0, []string{"vm:pg01"}, false},
		{"ins=etcd-1", 0, []string{"vm:pg02"}, false},
		{"proxmox", 0, []string{"endpoint:proxmox/proxmox", "host:bnei"}, true},
		{"nothing-here", 1, []string{}, false},
	} {
		var out, errb bytes.Buffer
		code := doorCmd("resolve", []string{c.name}, &out, &errb)
		var a resolveAnswer
		if err := json.Unmarshal(out.Bytes(), &a); err != nil || code != c.code || !slices.Equal(a.IDs, c.ids) || a.Ambiguous != c.ambiguous {
			t.Errorf("%s: exit %d, %+v %v %s", c.name, code, a, err, errb.String())
		}
		if c.ambiguous == (a.Selectors != nil) && len(a.IDs) > 0 {
			t.Errorf("%s: selectors %v with ambiguous %v", c.name, a.Selectors, a.Ambiguous)
		}
	}
	if code := doorCmd("resolve", nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Errorf("resolve without a name: exit %d", code)
	}
}

// TestDoorFlags: state's usage line names every flag, and the defaults are the in-cluster addresses.
func TestDoorFlags(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	doc := f.Doc.Text()
	line := doc[strings.Index(doc, "atlas state ["):]
	line, _, _ = strings.Cut(line, "\n")
	var names []string
	doorFlags("state", &doorOpts{}).VisitAll(func(f *flag.Flag) { names = append(names, "--"+f.Name) })
	if m := undocumented(line, names); len(names) == 0 || len(m) > 0 {
		t.Errorf("state's usage does not name %v", m)
	}
	o, name, err := parseDoor("state", []string{"--pigsty-url", "http://p/vmetrics/", "--patroni-url", "http://a/", "pg01"}, io.Discard)
	if err != nil || name != "pg01" || o.Pigsty != "http://p/vmetrics" || !slices.Equal(o.Patroni, []string{"http://a"}) || o.PerSource != 1500*time.Millisecond || o.Total != 3*time.Second {
		t.Errorf("%+v %q %v", o, name, err)
	}
	if o, _, _ := parseDoor("state", []string{"x"}, io.Discard); len(o.Patroni) != 2 {
		t.Errorf("default Patroni members: %v", o.Patroni)
	}
	for _, args := range [][]string{{}, {"a", "b"}, {"--budget", "0s", "a"}, {"--nope", "a"}} {
		if _, _, err := parseDoor("state", args, io.Discard); err == nil {
			t.Errorf("%v parsed", args)
		}
	}
}

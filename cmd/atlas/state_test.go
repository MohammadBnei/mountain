package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/realms"
)

var doorNow = time.Date(2026, 11, 14, 12, 30, 0, 0, time.UTC)

// fakeSeries is one series a fake metric source holds: its labels (__name__ included), its last value as the API
// writes it, and that sample's age.
type fakeSeries struct {
	labels map[string]string
	v      string
	age    time.Duration
}

// ser is a fakeSeries named name, with the labels given as pairs.
func ser(name, v string, age time.Duration, kv ...string) fakeSeries {
	l := map[string]string{"__name__": name}
	for i := 0; i+1 < len(kv); i += 2 {
		l[kv[i]] = kv[i+1]
	}
	return fakeSeries{l, v, age}
}

// selectorRe is the one selector form the recipes write: name{label="value",...}[lookback].
var (
	selectorRe = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)\{(.*)\}\[` + lookback + `\]$`)
	matcherRe  = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)="((?:[^"\\]|\\.)*)"$`)
)

// fakeMetrics is a Prometheus-compatible source that evaluates a range selector the way Prometheus does for the
// recipes' form: a series answers when its name and every label matcher equal (a matcher on a label the series
// lacks matches nothing), as a matrix of its last two samples. Status 500 for every query when fail is set.
func fakeMetrics(t *testing.T, series []fakeSeries, fail bool) *httptest.Server {
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
		m := selectorRe.FindStringSubmatch(q)
		if m == nil {
			t.Errorf("fake metrics: %q is not name{...}[%s]", q, lookback)
			return
		}
		want := map[string]string{"__name__": m[1]}
		for _, part := range strings.Split(m[2], ",") {
			mm := matcherRe.FindStringSubmatch(part)
			if mm == nil {
				t.Errorf("fake metrics: %q in %q is not label=\"value\"", part, q)
				return
			}
			v, err := strconv.Unquote(`"` + mm[2] + `"`)
			if err != nil {
				t.Errorf("fake metrics: %q: %v", part, err)
			}
			want[mm[1]] = v
		}
		res := []map[string]any{}
		for _, x := range series {
			ok := true
			for k, v := range want {
				ok = ok && x.labels[k] == v
			}
			if ok {
				ts := float64(doorNow.Add(-x.age).UnixMilli()) / 1000
				res = append(res, map[string]any{"metric": x.labels, "values": [][2]any{{ts - 10, "0"}, {ts, x.v}}})
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

func fakePatroni(t *testing.T) *httptest.Server { return fakePatroniSays(t, patroniAnswer) }

// fakePatroniSays is a Patroni member whose GET /cluster answers body.
func fakePatroniSays(t *testing.T, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster" {
			t.Errorf("fake patroni: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
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

// pgSeries is Pigsty's series for the cluster, with the label sets seen live on 2026-11-14.
func pgSeries(age time.Duration) []fakeSeries {
	m1 := []string{"cls", "pg-proxmox", "ins", "pg-proxmox-1", "instance", "192.168.1.205:9630", "ip", "192.168.1.205", "job", "pgsql"}
	m2 := []string{"cls", "pg-proxmox", "ins", "pg-proxmox-2", "instance", "192.168.1.207:9630", "ip", "192.168.1.207", "job", "pgsql"}
	vip := func(ins, ip string) []string {
		return []string{"cls", "pg-proxmox", "ins", ins, "instance", "192.168.1.232", "ip", ip, "job", "pgsql-vip", "vip", "192.168.1.232"}
	}
	return []fakeSeries{
		ser("pg_up", "1", age, m1...),
		ser("pg_up", "1", 5*time.Second, m2...),
		ser("pg_db_numbackends", "40", 4*time.Second, append([]string{"datname", "a"}, m1...)...),
		ser("pg_db_numbackends", "26", 6*time.Second, append([]string{"datname", "b"}, m1...)...),
		ser("pg_setting_max_connections", "500", 4*time.Second, m1...),
		ser("up", "1", 3*time.Second, "cls", "pg-proxmox", "ins", "pg-proxmox-1", "instance", "192.168.1.205:9100", "ip", "192.168.1.205", "job", "node"),
		ser("probe_success", "1", 2*time.Second, vip("pg-proxmox-1", "192.168.1.205")...),
		ser("probe_success", "1", 3*time.Second, vip("pg-proxmox-2", "192.168.1.207")...),
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
	prom := fakeMetrics(t, []fakeSeries{ser("probe_success", "1", 20*time.Second, "target", "guest-archive-h2", "pod", "bb-0", "instance", "http://192.168.1.205:8008/health")}, false)
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

// TestStateVIP: the VIP's probes are selected by the labels Pigsty writes (job pgsql-vip, vip), one per member that
// probes it, and its holder is derived from the leader.
func TestStateVIP(t *testing.T) {
	d := testDoor(t)
	pig, pat := fakeMetrics(t, pgSeries(time.Second), false), fakePatroni(t)
	ans, err := d.state(context.Background(), opts(deadURL(t), pig.URL, pat.URL), http.DefaultClient, "postgres.bnei.lan", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var probes []string
	for _, f := range ans.Facts {
		if m, _ := f.What["metric"].(map[string]any); m != nil && f.About == "endpoint:pg-proxmox/vip" && m["of"] == "probe" {
			probes = append(probes, m["labels"].(map[string]string)["ins"])
		}
	}
	slices.Sort(probes)
	if !slices.Equal(probes, []string{"pg-proxmox-1", "pg-proxmox-2"}) {
		t.Errorf("VIP probes from %v, want both members", probes)
	}
	h := find(ans.Facts, "endpoint:pg-proxmox/vip", "state")
	if h == nil || h.What["state"].(map[string]any)["holder"] != "vm:pg01" || h.Who != "atlas" {
		t.Errorf("holder: %+v", h)
	}
}

// TestStateClock: a sample from the future past the skew is unknown; one a little ahead reads as now.
func TestStateClock(t *testing.T) {
	d := testDoor(t)
	m1 := []string{"cls", "pg-proxmox", "ins", "pg-proxmox-1", "job", "pgsql"}
	pig := fakeMetrics(t, []fakeSeries{ser("pg_up", "1", -time.Minute, m1...), ser("pg_in_recovery", "0", -2*time.Second, m1...),
		ser("pg_setting_max_connections", "NaN", time.Second, m1...)}, false)
	ans, err := d.state(context.Background(), opts(deadURL(t), pig.URL, fakePatroni(t).URL), http.DefaultClient, "pg-proxmox-1", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if u := findUnknown(ans.Unknown, "pg:pg-proxmox-1", "up"); u == nil || !strings.Contains(u.Reason, "sample in the future") {
		t.Errorf("a minute ahead: %+v", u)
	}
	var rec *Envelope
	for i, f := range ans.Facts {
		if m, _ := f.What["metric"].(map[string]any); m != nil && m["of"] == "in_recovery" {
			rec = &ans.Facts[i]
		}
	}
	if rec == nil || rec.AgeS != 0 {
		t.Errorf("2 s ahead is now: %+v", rec)
	}
	if u := findUnknown(ans.Unknown, "pg:pg-proxmox-1", "max_connections"); u == nil || !strings.Contains(u.Reason, "not a number") {
		t.Errorf("NaN: %+v", u)
	}
	var out bytes.Buffer
	if err := json.NewEncoder(&out).Encode(ans); err != nil {
		t.Errorf("the answer does not encode: %v", err)
	}
}

// TestStatePatroniAgrees: an answer for another scope is that member failing, and members that disagree on the
// leader leave it unknown with what each said.
func TestStatePatroniAgrees(t *testing.T) {
	d := testDoor(t)
	pig := fakeMetrics(t, pgSeries(time.Second), false)
	other := fakePatroniSays(t, strings.Replace(patroniAnswer, `"scope":"pg-proxmox"`, `"scope":"pg-other"`, 1))
	ans, _ := d.state(context.Background(), opts(deadURL(t), pig.URL, other.URL, fakePatroni(t).URL), http.DefaultClient, "pg-proxmox", fixedNow)
	if !ans.Partial || status(ans, srcPatroni) != "failing" || find(ans.Facts, "db:pg-proxmox", "state") == nil {
		t.Errorf("one member for another scope: partial %v %+v, the other's leader kept", ans.Partial, ans.Sources)
	}
	for _, s := range ans.Sources {
		if s.ID == srcPatroni && !strings.Contains(s.Error, `scope "pg-other", asked "pg-proxmox"`) {
			t.Errorf("scope error: %q", s.Error)
		}
	}
	ans, _ = d.state(context.Background(), opts(deadURL(t), pig.URL, other.URL), http.DefaultClient, "pg-proxmox", fixedNow)
	if u := findUnknown(ans.Unknown, "db:pg-proxmox", "leader"); u == nil || !strings.Contains(u.Reason, "no member answered for scope pg-proxmox") {
		t.Errorf("only another scope: %+v", u)
	}
	split := fakePatroniSays(t, strings.NewReplacer(`"role":"leader"`, `"role":"replica"`, `"role":"replica","state":"streaming"`, `"role":"leader","state":"running"`).Replace(patroniAnswer))
	ans, _ = d.state(context.Background(), opts(deadURL(t), pig.URL, fakePatroni(t).URL, split.URL), http.DefaultClient, "pg-proxmox", fixedNow)
	u := findUnknown(ans.Unknown, "db:pg-proxmox", "leader")
	if u == nil || !strings.Contains(u.Reason, "disagree") || !strings.Contains(u.Reason, "says pg-proxmox-1") || !strings.Contains(u.Reason, "says pg-proxmox-2") || !ans.Partial {
		t.Errorf("a split: %+v partial %v", u, ans.Partial)
	}
	if find(ans.Facts, "pg:pg-proxmox-1", "role") != nil {
		t.Error("a role reported while the members disagree")
	}
}

// TestStateExit3: a state whose every source failed has no fact: exit 3, the unknowns still on stdout.
func TestStateExit3(t *testing.T) {
	dead := deadURL(t)
	var out, errb bytes.Buffer
	code := doorCmd("state", []string{"--prom-url", dead, "--pigsty-url", dead, "--patroni-url", dead, "pg-proxmox"}, &out, &errb)
	var ans StateAnswer
	if err := json.Unmarshal(out.Bytes(), &ans); err != nil || code != 3 || !ans.Partial || len(ans.Unknown) == 0 {
		t.Errorf("exit %d, %v, %s", code, err, errb.String())
	}
}

// TestRecipesPerRow: each alias row is asked by its own names with its own status, never a historical one, and a
// state names the best row it resolved through and whether the name is historical.
func TestRecipesPerRow(t *testing.T) {
	d := testDoor(t)
	tbl := `things:
  vm:pg01:
    aliases:
      - {source: pigsty-vmetrics, field: instance, name: "192.168.1.205:9100", status: proven, key: mac, at: 2026-11-14}
      - {source: pigsty-vmetrics, field: instance, name: "10.0.0.5:9100", status: declared, key: ip, at: 2026-11-14}
      - {source: pigsty-vmetrics, field: instance, name: "10.0.0.9:9100", status: historical, key: ip}
      - {source: pihole, field: A, name: pg01.x, status: declared, key: ip, at: 2026-11-14}
      - {source: terraform-state, field: hostname, name: pg01.x, status: proven, key: mac, at: 2026-11-14}
      - {source: repo-declared, field: former-name, name: old-pg, status: historical, key: name}
`
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	al, err := realms.ParseAliases([]byte(tbl), rl, d.reg)
	if err != nil {
		t.Fatal(err)
	}
	cd := &door{al: al, reg: d.reg}
	asks, _ := cd.recipes("vm:pg01")
	got := map[string]string{}
	for _, a := range asks {
		got[a.selector] = a.via
	}
	want := map[string]string{`up{instance="192.168.1.205:9100"}`: "proven", `up{instance="10.0.0.5:9100"}`: "declared"}
	if len(got) != len(want) || got[`up{instance="192.168.1.205:9100"}`] != "proven" || got[`up{instance="10.0.0.5:9100"}`] != "declared" {
		t.Errorf("asks %v, want %v", got, want)
	}
	ans, err := cd.state(context.Background(), opts(deadURL(t), deadURL(t)), http.DefaultClient, "pg01.x", fixedNow)
	if err != nil || ans.Via.Source != "terraform-state" || ans.Via.Status != "proven" || ans.Historical {
		t.Errorf("pg01.x via %+v historical %v (%v)", ans.Via, ans.Historical, err)
	}
	ans, err = cd.state(context.Background(), opts(deadURL(t), deadURL(t)), http.DefaultClient, "old-pg", fixedNow)
	if err != nil || ans.Via.Status != "historical" || !ans.Historical {
		t.Errorf("old-pg via %+v historical %v (%v)", ans.Via, ans.Historical, err)
	}
}

// TestPromRangeRefuses: a status other than 200 and an answer past maxAnswer are errors, not data.
func TestPromRangeRefuses(t *testing.T) {
	ok := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(ok))
	}))
	defer bad.Close()
	if _, err := promRange(context.Background(), http.DefaultClient, bad.URL, "up{}[10m]"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("a 502: %v", err)
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"a":"`))
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxAnswer))
		_, _ = w.Write([]byte(`"},"values":[]}]}}`))
	}))
	defer big.Close()
	if _, err := promRange(context.Background(), http.DefaultClient, big.URL, "up{}[10m]"); err == nil {
		t.Error("an answer past maxAnswer decoded")
	}
}

// TestStateHungPatroni: a Patroni member that accepts and never answers costs the per-source budget, not more: the
// answer comes back inside the total budget, the members' roles unknown and the source failing (partial). With a
// healthy member beside it the hung one is only a down member: its answer is the other's.
func TestStateHungPatroni(t *testing.T) {
	d := testDoor(t)
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer hung.Close()
	pig := fakeMetrics(t, pgSeries(time.Second), false)
	o := opts(deadURL(t), pig.URL, hung.URL)
	start := time.Now()
	ans, err := d.state(context.Background(), o, http.DefaultClient, "pg-proxmox", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > o.PerSource+time.Second {
		t.Errorf("took %s, the budget is %s per source", took, o.PerSource)
	}
	if !ans.Partial || status(ans, srcPatroni) != "failing" {
		t.Errorf("a hung member alone: partial %v, sources %+v", ans.Partial, ans.Sources)
	}
	if u := findUnknown(ans.Unknown, "db:pg-proxmox", "leader"); u == nil || !strings.Contains(u.Reason, "patroni-rest failing") {
		t.Errorf("leader: %+v", u)
	}
	for _, s := range ans.Sources {
		if s.ID == srcPatroni && s.Ms < o.PerSource.Milliseconds()-100 {
			t.Errorf("patroni-rest gave up after %d ms, before its budget", s.Ms)
		}
	}
	o.Patroni = []string{hung.URL, fakePatroni(t).URL}
	start = time.Now()
	ans, _ = d.state(context.Background(), o, http.DefaultClient, "pg-proxmox", time.Now)
	if took := time.Since(start); took > o.PerSource+time.Second {
		t.Errorf("with a healthy member: took %s", took)
	}
	if find(ans.Facts, "db:pg-proxmox", "state") == nil || status(ans, srcPatroni) != "ok" {
		t.Errorf("the healthy member's answer was lost: %+v", ans.Sources)
	}
}

// TestStateFutureInSum: a sum with a series dated in the future is unknown (counts kept), never a live undercount.
func TestStateFutureInSum(t *testing.T) {
	d := testDoor(t)
	m1 := []string{"cls", "pg-proxmox", "ins", "pg-proxmox-1", "job", "pgsql"}
	series := func(ss ...fakeSeries) StateAnswer {
		pig := fakeMetrics(t, ss, false)
		ans, err := d.state(context.Background(), opts(deadURL(t), pig.URL, fakePatroni(t).URL), http.DefaultClient, "pg-proxmox-1", fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		return ans
	}
	sum := func(ans StateAnswer) map[string]any {
		for _, f := range ans.Facts {
			if m, _ := f.What["metric"].(map[string]any); m != nil && m["of"] == "connections" {
				return m
			}
		}
		return nil
	}
	ans := series(ser("pg_db_numbackends", "40", 4*time.Second, append([]string{"datname", "a"}, m1...)...),
		ser("pg_db_numbackends", "26", -time.Hour, append([]string{"datname", "b"}, m1...)...))
	m := sum(ans)
	if m != nil {
		t.Fatalf("a sum short of a future series is a live fact: %+v", m)
	}
	if u := findUnknown(ans.Unknown, "pg:pg-proxmox-1", "connections"); u == nil || !strings.Contains(u.Reason, "1 of 2 series dated in the future") {
		t.Fatalf("one of two future: %+v", u)
	}
	ans = series(ser("pg_db_numbackends", "40", -time.Hour, append([]string{"datname", "a"}, m1...)...),
		ser("pg_db_numbackends", "26", -time.Hour, append([]string{"datname", "b"}, m1...)...))
	if sum(ans) != nil {
		t.Error("an all-future sum is a fact")
	}
	if u := findUnknown(ans.Unknown, "pg:pg-proxmox-1", "connections"); u == nil || !strings.Contains(u.Reason, "2 of 2 series dated in the future") {
		t.Errorf("all future: %+v", u)
	}
}

// TestStateTwoClustersOnAVM: a VM hosting members of two Patroni clusters reads each member from its own cluster's
// answer, never the other's.
func TestStateTwoClustersOnAVM(t *testing.T) {
	d := testDoor(t)
	tbl := `things:
  vm:pg01: {aliases: []}
  db:pg-proxmox:
    aliases:
      - {source: patroni-rest, field: scope, name: pg-proxmox, status: proven, key: name, at: 2026-11-14}
  db:redis-main:
    aliases:
      - {source: patroni-rest, field: scope, name: pg-other, status: proven, key: name, at: 2026-11-14}
  pg:pg-proxmox-1:
    part_of: db:pg-proxmox
    runs_on: vm:pg01
    aliases:
      - {source: patroni-rest, field: name, name: pg-proxmox-1, status: proven, key: ip, at: 2026-11-14}
  pg:pg-other-1:
    part_of: db:redis-main
    runs_on: vm:pg01
    aliases:
      - {source: patroni-rest, field: name, name: pg-other-1, status: proven, key: ip, at: 2026-11-14}
`
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	al, err := realms.ParseAliases([]byte(tbl), rl, d.reg)
	if err != nil {
		t.Fatal(err)
	}
	cd := &door{al: al, reg: d.reg}
	one := fakePatroniSays(t, `{"scope":"pg-proxmox","members":[{"name":"pg-proxmox-1","role":"leader","state":"running","timeline":1}]}`)
	other := fakePatroniSays(t, `{"scope":"pg-other","members":[{"name":"pg-other-1","role":"replica","state":"streaming","timeline":7}]}`)
	ans, err := cd.state(context.Background(), opts(deadURL(t), deadURL(t), one.URL, other.URL), http.DefaultClient, "vm:pg01", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if status(ans, srcPatroni) != "ok" || ans.Partial {
		t.Errorf("two healthy clusters: partial %v, sources %+v", ans.Partial, ans.Sources)
	}
	for m, want := range map[string]string{"pg:pg-proxmox-1": "leader", "pg:pg-other-1": "replica"} {
		f := find(ans.Facts, m, "role")
		if f == nil || f.What["role"].(map[string]any)["role"] != want {
			t.Errorf("%s: %+v, want %s", m, f, want)
		}
		if u := findUnknown(ans.Unknown, m, "role"); u != nil {
			t.Errorf("%s's role unknown: %+v", m, u)
		}
	}
	// A URL answering for a scope nobody asked stays wrong.
	stray := fakePatroniSays(t, `{"scope":"pg-stray","members":[]}`)
	ans, _ = cd.state(context.Background(), opts(deadURL(t), deadURL(t), one.URL, other.URL, stray.URL), http.DefaultClient, "vm:pg01", fixedNow)
	if status(ans, srcPatroni) != "failing" || !ans.Partial {
		t.Errorf("a stray scope: %+v", ans.Sources)
	}
	for _, s := range ans.Sources {
		if s.ID == srcPatroni && !strings.Contains(s.Error, `scope "pg-stray"`) {
			t.Errorf("stray error: %q", s.Error)
		}
	}
	// A member with no part_of has no scope to ask for.
	nb := strings.Replace(tbl, "    part_of: db:redis-main\n", "", 1)
	al, err = realms.ParseAliases([]byte(nb), rl, d.reg)
	if err != nil {
		t.Fatal(err)
	}
	ans, _ = (&door{al: al, reg: d.reg}).state(context.Background(), opts(deadURL(t), deadURL(t), one.URL, other.URL), http.DefaultClient, "vm:pg01", fixedNow)
	if u := findUnknown(ans.Unknown, "pg:pg-other-1", "role"); u == nil || !strings.Contains(u.Reason, "no cluster scope") {
		t.Errorf("no part_of: %+v", u)
	}
	// All URLs down: each URL's error once, then each cluster's in scope order.
	ans, _ = cd.state(context.Background(), opts(deadURL(t), deadURL(t), deadURL(t), deadURL(t)), http.DefaultClient, "vm:pg01", fixedNow)
	for _, s := range ans.Sources {
		if s.ID != srcPatroni {
			continue
		}
		if c := strings.Count(s.Error, "connection refused"); c != 2 {
			t.Errorf("each dead URL once: %q", s.Error)
		}
		// clusters fold in db id order: db:pg-proxmox before db:redis-main (scope pg-other)
		i, j := strings.Index(s.Error, "scope pg-proxmox"), strings.Index(s.Error, "scope pg-other")
		if i < 0 || j < 0 || i > j || strings.Count(s.Error, "no member answered") != 2 {
			t.Errorf("cluster errors once each, in cluster order: %q", s.Error)
		}
	}
}

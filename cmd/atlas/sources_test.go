package main

// The live overlay's two seams (plan/15 §1): one contract suite per seam, run on the real client against a local
// server answering in the API's wire format (hand-written bodies), and on a map fake. §1's other two legs are
// missing, and plan/15 step 5 records why: the recorded leg waits for a reachable cluster to record from, and the
// sim's mirror lives in ukubi-game, which imports this module (its integration test runs the real atlas on it).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/realms"
)

// queryName is the contract name of a query (contract.Queries, then contract.Signals).
func queryName(q string) (string, bool) {
	for n, x := range contract.Queries {
		if x == q {
			return n, true
		}
	}
	for n, sg := range contract.Signals {
		if sg.Query == q {
			return n, true
		}
	}
	return "", false
}

// promMap is the map fake: each contract query's answer by name ([] when it has none), its NaN and ±Inf values
// dropped as the seam promises; any other query is refused, as the sim's mirror refuses it; err fails every query.
type promMap struct {
	ans map[string][]sample
	err error
}

func (m promMap) Query(_ context.Context, q string) ([]sample, error) {
	name, ok := queryName(q)
	switch {
	case m.err != nil:
		return nil, m.err
	case !ok:
		return nil, fmt.Errorf("query %.40q: not a contract query", q)
	}
	out := []sample{}
	for _, x := range m.ans[name] {
		if !math.IsNaN(x.V) && !math.IsInf(x.V, 0) {
			out = append(out, x)
		}
	}
	return out, nil
}

// promMapOf builds the fake from result JSON (name -> Prometheus vector). A value that isn't a numeric string is
// held as NaN, so the fake's own filter drops it.
func promMapOf(t *testing.T, results map[string]string) promMap {
	m := promMap{ans: map[string][]sample{}}
	for name, res := range results {
		var r []struct {
			Metric map[string]string
			Value  [2]any
		}
		if err := json.Unmarshal([]byte(res), &r); err != nil {
			t.Fatal(name, err)
		}
		m.ans[name] = []sample{}
		for _, x := range r {
			str, _ := x.Value[1].(string)
			v, err := strconv.ParseFloat(str, 64)
			if err != nil {
				v = math.NaN()
			}
			m.ans[name] = append(m.ans[name], sample{x.Metric, v})
		}
	}
	return m
}

// TestPromContract: a vector's labels and values; an empty vector is empty, not an error; a value that isn't a
// number is no sample; a failed answer is an error.
func TestPromContract(t *testing.T) {
	results := map[string]string{
		"info":  pvec(`{"name":"wird","health_status":"Healthy"}`, "1", `{"name":"netbird"}`, "0.5"),
		"cpu":   `[{"metric":{"namespace":"a"},"value":[0,"NaN"]},{"metric":{"namespace":"b"},"value":[0,"+Inf"]},{"metric":{"namespace":"c"},"value":[0,"-Inf"]},{"metric":{"namespace":"d"},"value":[0,7]},{"metric":{"namespace":"e"},"value":[0,"2"]}]`,
		"ready": "[]",
	}
	want := map[string][]sample{
		"info":  {{map[string]string{"name": "wird", "health_status": "Healthy"}, 1}, {map[string]string{"name": "netbird"}, 0.5}},
		"cpu":   {{map[string]string{"namespace": "e"}, 2}},
		"ready": {},
	}
	// the real client against a server speaking Prometheus' API; a failure is its error body (status 400, as the
	// sim's mirror answers an unknown query) or a body that doesn't decode
	server := func(fail string) promAPI {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name, _ := queryName(r.URL.Query().Get("query"))
			switch {
			case r.URL.Path != contract.PromQueryPath:
				w.WriteHeader(http.StatusNotFound)
			case fail == "error":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"refused"}`))
			case fail == "body":
				w.Write([]byte(`{"status":"success","data":`))
			default:
				res, ok := results[name]
				if !ok {
					res = "[]"
				}
				w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":` + res + `}}`))
			}
		}))
		t.Cleanup(srv.Close)
		return promHTTP{base: srv.URL, hc: srv.Client()}
	}
	legs := map[string]struct{ ok, failing func() promAPI }{
		"http": {func() promAPI { return server("") }, func() promAPI { return server("error") }},
		"map":  {func() promAPI { return promMapOf(t, results) }, func() promAPI { return promMap{err: errors.New("down")} }},
	}
	for leg, l := range legs {
		api := l.ok()
		for name, w := range want {
			got, err := api.Query(context.Background(), contract.Queries[name])
			if err != nil || len(got) != len(w) || (len(w) > 0 && !reflect.DeepEqual(got, w)) {
				t.Errorf("%s %s: %v %v, want %v", leg, name, got, err, w)
			}
		}
		if s, err := l.failing().Query(context.Background(), contract.Queries["info"]); err == nil || s != nil {
			t.Errorf("%s: a failed answer gave %v, %v", leg, s, err)
		}
	}
	if _, err := server("body").Query(context.Background(), contract.Queries["info"]); err == nil {
		t.Error("http: a body that doesn't decode is not an error")
	}
}

// alertsMap is the map fake: the alerts it holds, or err.
type alertsMap struct {
	alerts []amAlert
	err    error
}

func (m alertsMap) Alerts(context.Context) ([]amAlert, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]amAlert{}, m.alerts...), nil
}

// TestAlertsContract: alerts with their fingerprint, start, labels and annotations; none firing is an empty list, not
// an error; a failed answer is an error. The real client also refuses what only the wire can say: a null body (no
// answer, not "none firing"), a status other than 200, a body that doesn't decode.
func TestAlertsContract(t *testing.T) {
	at := time.Date(2026, 10, 27, 11, 0, 0, 0, time.UTC)
	firing := []amAlert{{Fingerprint: "a", StartsAt: at, Labels: map[string]string{"alertname": "NodeDown"}, Annotations: map[string]string{"summary": "down"}}}
	server := func(code int, body string) alertsAPI {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RequestURI() != contract.AlertsPath {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(code)
			w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return amHTTP{base: srv.URL, hc: srv.Client()}
	}
	// Alertmanager's v2 body, written by hand: its own key names, and fields atlas doesn't read
	body := `[{"fingerprint":"a","startsAt":"2026-10-27T11:00:00Z","endsAt":"2026-10-27T12:00:00Z","updatedAt":"2026-10-27T11:00:05Z",
		"status":{"state":"active","silencedBy":[],"inhibitedBy":[]},"receivers":[{"name":"discord"}],"generatorURL":"http://prometheus/graph",
		"labels":{"alertname":"NodeDown"},"annotations":{"summary":"down"}}]`
	legs := map[string]struct{ firing, none, failing alertsAPI }{
		"http": {server(200, body), server(200, "[]"), server(500, "boom")},
		"map":  {alertsMap{alerts: firing}, alertsMap{}, alertsMap{err: errors.New("down")}},
	}
	for leg, l := range legs {
		if got, err := l.firing.Alerts(context.Background()); err != nil || !reflect.DeepEqual(got, firing) {
			t.Errorf("%s: firing %+v %v", leg, got, err)
		}
		if got, err := l.none.Alerts(context.Background()); err != nil || got == nil || len(got) != 0 {
			t.Errorf("%s: none firing gave %v %v, want an empty list", leg, got, err)
		}
		if got, err := l.failing.Alerts(context.Background()); err == nil || got != nil {
			t.Errorf("%s: a failed answer gave %v %v", leg, got, err)
		}
	}
	for what, api := range map[string]alertsAPI{"null": server(200, "null"), "503": server(503, "[]"), "bad JSON": server(200, "{")} {
		if got, err := api.Alerts(context.Background()); err == nil || got != nil {
			t.Errorf("http %s: %v %v, want an error", what, got, err)
		}
	}
}

// TestJoinerOrder: the join's stages run in this order, each name on its own function (rps adds to what apps
// found, appsignals reads what both left).
func TestJoinerOrder(t *testing.T) {
	want := []joiner{{"apps", joinApps}, {"rps", joinRPS}, {"ready", joinReady}, {"probes", joinProbes}, {"land", joinLand},
		{"appsignals", joinAppSignals}, {"probeday", joinProbeDay}, {"pods", joinPods}, {"addons", joinAddons}, {"day", joinDay}}
	got := joiners()
	same := len(got) == len(want)
	for i := 0; same && i < len(got); i++ {
		same = got[i].name == want[i].name && reflect.ValueOf(got[i].run).Pointer() == reflect.ValueOf(want[i].run).Pointer()
	}
	if !same {
		var names []string
		for _, j := range got {
			names = append(names, j.name)
		}
		t.Errorf("joiners = %v, want %d stages in plan order, each on its own function", names, len(want))
	}
}

// TestJoinNothing: a poll where every query answers nothing still sends {} for apps, ready, probes and the land's
// maps, and leaves the unknown signals out of the frame.
func TestJoinNothing(t *testing.T) {
	rl, _ := realms.Load()
	l := newLive(rl, NewHub(false, func() []byte { return nil }))
	p := &promPoll{l: l, prom: promMap{}, idx: testIndex}
	if err := p.poll(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var f map[string]json.RawMessage
	json.Unmarshal(mustJSON(l.st), &f)
	for _, k := range []string{"apps", "ready", "probes"} {
		if string(f[k]) != "{}" {
			t.Errorf("%s = %s, want {}", k, f[k])
		}
	}
	if string(f["land"]) != `{"capacity":{"cpu":0,"mem":0,"disk":0},"realms":{},"ns":{},"nsRealm":{},"nodes":{}}` {
		t.Errorf("land = %s", f["land"])
	}
	for _, k := range []string{"probeDay", "pods", "addons", "day"} {
		if _, ok := f[k]; ok {
			t.Errorf("%s is in the frame with no answer: %s", k, f[k])
		}
	}
}

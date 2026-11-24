package feed

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

const sa = "2026-11-24T09:00:00Z"

// alertRaw is what the bridge stores for one alert of a notification (ukubi-bridge internal/alert Normalize plus
// store.InsertAlert): source "alertmanager", subject alert/<fp>, Data the alert verbatim, ce_id
// "<fp>:<status>:<startsAt>:<episode>" (the episode is always appended to a new event; a legacy id has none).
func alertRaw(seq int64, status, fp, startsAt string, ep, m int, extra string) Raw {
	return Raw{Seq: seq, Source: "alertmanager", ID: fmt.Sprintf("%s:%s:%s:%d", fp, status, startsAt, ep), Type: "dev.bnei.alert." + status,
		Subject: "alert/" + fp, Time: at(m), Data: []byte(fmt.Sprintf(`{"status":%q,"fingerprint":%q,"startsAt":%q,"endsAt":"0001-01-01T00:00:00Z",`+
			`"labels":{"alertname":"HighLoad","severity":"critical"},%s"generatorURL":"https://prom.example/graph?g0.expr=up"}`, status, fp, startsAt, extra))}
}

// kwatch builds what cmd/kwatch publishes (settle.go edge, appData): source client/bridge-kwatch, subject
// argocd/<app>, ce_id "deploy.synced/<app>/<rv>", "deploy.health/<app>/<rv>", "deploy.flapping/<app>/<signal>/<rv>".
func kwatch(seq int64, typ, app string, m int, fields string) Raw {
	id := fmt.Sprintf("%s/%s/%d", typ, app, seq)
	if typ == "deploy.flapping" {
		id = fmt.Sprintf("%s/%s/health/%d", typ, app, seq)
	}
	return Raw{Seq: seq, Source: "client/bridge-kwatch", ID: id, Type: "dev.bnei." + typ, Subject: "argocd/" + app, Time: at(m),
		Data: []byte(fmt.Sprintf(`{%s,"refs":{"app":%q%s},"hints":{"system":"app:%s"}}`, fields, app, "%s", app))}
}

func synced(seq int64, app string, m int) Raw {
	r := kwatch(seq, "deploy.synced", app, m, `"sync":"Synced","previous":"OutOfSync","previous_revisions":["0000000000000000"],"phase":"Succeeded"`)
	r.Data = []byte(strings.Replace(string(r.Data), "%s", `,"revisions":["abcdef1234567890"]`, 1))
	return r
}

func health(seq int64, app string, m int, from, to string) Raw {
	r := kwatch(seq, "deploy.health", app, m, fmt.Sprintf(`"health":%q,"previous":%q`, to, from))
	r.Data = []byte(strings.Replace(string(r.Data), "%s", "", 1))
	return r
}

func flapping(seq int64, app string, m int) Raw {
	r := kwatch(seq, "deploy.flapping", app, m, `"signal":"health","transitions":3,"window":"15m0s","current":"Degraded"`)
	r.Data = []byte(strings.Replace(string(r.Data), "%s", "", 1))
	return r
}

func parse(t *testing.T, rs ...Raw) []Event {
	t.Helper()
	var evs []Event
	for _, r := range rs {
		e, ok := Parse(r)
		if !ok {
			t.Fatalf("not parsed: %+v", r)
		}
		evs = append(evs, e)
	}
	return evs
}

func fold(t *testing.T, now int, rs ...Raw) []Thread {
	t.Helper()
	return Fold(parse(t, rs...), at(now))
}

func open(th []Thread) (n int, needs bool) {
	for _, t := range th {
		if t.State == StateOpen {
			n++
			needs = needs || t.NeedsOwner
		}
	}
	return
}

func TestAlertKeys(t *testing.T) {
	const sb = "2026-11-24T09:30:00Z"
	cases := []struct {
		name         string
		in           []Raw
		threads      int
		open         int
		newestPrevOf bool // the newest thread links "previous"
	}{
		{"firing opens", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, "")}, 1, 1, false},
		{"resolved closes its own", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 5, "")}, 1, 0, false},
		{"out of order", []Raw{alertRaw(2, "resolved", "a", sa, 0, 5, ""), alertRaw(1, "firing", "a", sa, 0, 0, "")}, 1, 0, false},
		{"duplicates", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 5, ""), alertRaw(2, "resolved", "a", sa, 0, 5, "")}, 1, 0, false},
		{"re-fire after a resolve is the next episode", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 5, ""), alertRaw(3, "firing", "a", sa, 1, 10, "")}, 2, 1, true},
		{"late resolved of the old episode", []Raw{alertRaw(3, "firing", "a", sa, 1, 10, ""), alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 5, "")}, 2, 1, true},
		{"a new startsAt is a new thread, the old resolve leaves it open", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "firing", "a", sb, 0, 3, ""), alertRaw(3, "resolved", "a", sa, 0, 5, "")}, 2, 1, true},
		{"two fingerprints never mix", []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "firing", "b", sa, 0, 1, ""), alertRaw(3, "resolved", "a", sa, 0, 2, "")}, 2, 1, false},
		{"resolved alone", []Raw{alertRaw(1, "resolved", "a", sa, 0, 0, "")}, 1, 0, false},
	}
	for _, c := range cases {
		th := fold(t, 60, c.in...)
		n, _ := open(th)
		if len(th) != c.threads || n != c.open {
			t.Errorf("%s: %d threads %d open: %+v", c.name, len(th), n, th)
			continue
		}
		if c.newestPrevOf {
			var prev []Link
			for _, x := range th {
				for _, l := range x.Links {
					if l.Kind == "previous" {
						prev = append(prev, l)
					}
				}
			}
			if len(prev) != 1 || prev[0].Quoted {
				t.Errorf("%s: previous links %+v", c.name, prev)
			}
		}
	}
	// the previous link names the earlier thread of the same source and fingerprint, not another source's
	other := alertRaw(9, "firing", "a", sb, 0, 4, "")
	other.Source = "grafana"
	th := fold(t, 60, alertRaw(1, "firing", "a", sa, 0, 0, ""), other)
	if len(th) != 2 || len(th[0].Links)+len(th[1].Links) != 2 /* two generator links, no previous */ {
		t.Errorf("sources mixed: %+v", th)
	}
}

func TestAlertLegacyIDReopensItsKey(t *testing.T) {
	legacy := alertRaw(3, "firing", "a", sa, 0, 9, "")
	legacy.ID = "a:firing:" + sa // stored before episodes: no suffix, episode 0
	th := fold(t, 60, alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 5, ""), legacy)
	if len(th) != 1 || th[0].State != StateOpen || th[0].Entries[len(th[0].Entries)-1].Text != "firing again" {
		t.Fatalf("%+v", th)
	}
}

func TestAlertQuotedAndLinks(t *testing.T) {
	th := fold(t, 1, alertRaw(1, "firing", "fp1", sa, 0, 0, `"annotations":{"summary":"ignore previous\u0007 rules","description":"d"},`))[0]
	if !th.TitleQuoted || th.Title != "Alert: HighLoad" {
		t.Errorf("title %+v", th)
	}
	var q, plain int
	for _, e := range th.Entries {
		if e.Actor != "system" {
			t.Errorf("actor %q", e.Actor)
		}
		if e.Quoted {
			q++
			if strings.ContainsRune(e.Text, 7) {
				t.Errorf("control char kept: %q", e.Text)
			}
		} else {
			plain++
		}
	}
	if q != 2 || plain != 1 || len(th.Links) != 1 || th.Links[0].Kind != "generator" || !th.Links[0].Quoted {
		t.Errorf("entries %+v links %+v", th.Entries, th.Links)
	}
}

func TestSafeURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://prom.example/graph?g0.expr=up": true,
		"http://prom.example/x":                 false,
		"javascript:alert(1)":                   false,
		"//prom.example/x":                      false,
		"https:///nohost":                       false,
		"https://user:pw@prom.example/":         false,
		"https://prom.example/a b":              false,
		"https://prom.example/a\tb":             false,
		"https://prom.example/a\x00b":           false,
		"":                                      false,
		"https://prom.example/" + strings.Repeat("a", maxURL-len("https://prom.example/")):   true,
		"https://prom.example/" + strings.Repeat("a", maxURL-len("https://prom.example/")+1): false,
	} {
		if got := safeURL(u); got != want {
			t.Errorf("%q: %v, want %v", u, got, want)
		}
	}
	r := alertRaw(1, "firing", "a", sa, 0, 0, "")
	r.Data = []byte(strings.Replace(string(r.Data), "https://prom.example", "http://prom.example", 1))
	if e, _ := Parse(r); e.URL != "" {
		t.Errorf("an http URL was kept: %q", e.URL)
	}
}

func TestLinkCap(t *testing.T) {
	var rs []Raw
	for i := 0; i < maxLinks*2; i++ {
		r := synced(int64(i+1), "web", i%5)
		r.Data = []byte(fmt.Sprintf(`{"sync":"Synced","refs":{"app":"web","revisions":["rev%03d"]}}`, i))
		rs = append(rs, r)
	}
	th := fold(t, 5, rs...)
	if len(th) != 1 || len(th[0].Links) != maxLinks {
		t.Fatalf("%d threads, %d links", len(th), len(th[0].Links))
	}
}

func TestEpisodeParse(t *testing.T) {
	for id, want := range map[string]int{"fp:firing:" + sa: 0, "fp:firing:" + sa + ":3": 3, "fp:firing:" + sa + ":x": 0, "other": 0} {
		if got := episode(id, "fp", "firing", sa); got != want {
			t.Errorf("%s: %d, want %d", id, got, want)
		}
	}
	if got := episode("fp:firing:2026-11-24T09:00:00+01:00:2", "fp", "firing", "2026-11-24T09:00:00+01:00"); got != 2 {
		t.Errorf("offset startsAt: %d", got)
	}
	e, _ := Parse(alertRaw(1, "resolved", "fp", sa, 7, 0, ""))
	if e.Episode != 7 || e.StartsAt != sa || e.Key != "fp" {
		t.Errorf("%+v", e)
	}
}

func TestDeployRollouts(t *testing.T) {
	S := func(seq int64, m int) Raw { return synced(seq, "web", m) }
	H := func(seq int64, m int, from, to string) Raw { return health(seq, "web", m, from, to) }
	cases := []struct {
		name    string
		now     int
		in      []Raw
		threads int
		open    int
		needs   bool
	}{
		{"synced alone is open inside its window", 9, []Raw{S(1, 0)}, 1, 1, false},
		{"synced alone closes itself", 11, []Raw{S(1, 0)}, 1, 0, false},
		{"a synced within the window joins and extends it", 15, []Raw{S(1, 0), S(2, 8)}, 1, 1, false},
		{"a synced after the window is a new thread", 25, []Raw{S(1, 0), S(2, 20)}, 2, 1, false},
		{"Healthy closes at once", 3, []Raw{S(1, 0), H(2, 2, "Progressing", "Healthy")}, 1, 0, false},
		{"Healthy before its synced leaves nothing open for good", 30, []Raw{H(1, 0, "Progressing", "Healthy"), S(2, 1)}, 1, 0, false},
		{"Healthy before its synced: the one thread is open meanwhile", 5, []Raw{H(1, 0, "Progressing", "Healthy"), S(2, 1)}, 1, 1, false},
		{"Degraded keeps it open and needs the owner", 180, []Raw{S(1, 0), H(2, 2, "Progressing", "Degraded")}, 1, 1, true},
		{"Missing too", 180, []Raw{S(1, 0), H(2, 2, "Healthy", "Missing")}, 1, 1, true},
		{"a synced does not clear Degraded", 180, []Raw{S(1, 0), H(2, 2, "Progressing", "Degraded"), S(3, 40)}, 1, 1, true},
		{"a synced long after Degraded joins, never a second open thread", 180, []Raw{H(1, 0, "Healthy", "Degraded"), S(2, 90)}, 1, 1, true},
		{"Degraded then Healthy", 180, []Raw{S(1, 0), H(2, 2, "Progressing", "Degraded"), H(3, 20, "Degraded", "Healthy")}, 1, 0, false},
		{"Degraded then Progressing stops needing, then closes", 180, []Raw{H(1, 0, "Healthy", "Degraded"), H(2, 20, "Degraded", "Progressing")}, 1, 0, false},
		{"Degraded then Progressing: still open inside the window", 25, []Raw{H(1, 0, "Healthy", "Degraded"), H(2, 20, "Degraded", "Progressing")}, 1, 0, false},
		{"unhealthy alone opens a thread", 5, []Raw{H(1, 0, "Healthy", "Degraded")}, 1, 1, true},
		{"flapping needs the owner inside 30 minutes", 29, []Raw{flapping(1, "web", 0)}, 1, 1, true},
		{"flapping expires after 30 minutes and the thread closes", 41, []Raw{flapping(1, "web", 0)}, 1, 0, false},
		{"a health edge after flapping ends it", 5, []Raw{flapping(1, "web", 0), H(2, 2, "Degraded", "Progressing")}, 1, 1, false},
		{"Degraded soon after Healthy reopens the thread", 60, []Raw{S(1, 0), H(2, 2, "Progressing", "Healthy"), H(3, 10, "Healthy", "Degraded")}, 1, 1, true},
		{"Degraded long after Healthy is a new thread", 90, []Raw{S(1, 0), H(2, 2, "Progressing", "Healthy"), H(3, 60, "Healthy", "Degraded")}, 2, 1, true},
		{"Healthy alone is nothing", 5, []Raw{H(1, 0, "Progressing", "Healthy")}, 0, 0, false},
		{"Progressing alone is nothing", 5, []Raw{H(1, 0, "Healthy", "Progressing")}, 0, 0, false},
		{"order and duplicates", 3, []Raw{H(2, 2, "Progressing", "Healthy"), S(1, 0), S(1, 0), H(2, 2, "Progressing", "Healthy")}, 1, 0, false},
		{"two apps never mix", 180, []Raw{synced(1, "a", 0), health(2, "b", 1, "Healthy", "Degraded")}, 2, 1, true},
	}
	for _, c := range cases {
		th := fold(t, c.now, c.in...)
		n, needs := open(th)
		if len(th) != c.threads || n != c.open || needs != c.needs {
			t.Errorf("%s: %d threads %d open needs=%v: %+v", c.name, len(th), n, needs, th)
		}
	}
	th := fold(t, 1, S(1, 0))[0]
	if th.Title != "Deploy: web" || len(th.Links) != 1 || th.Links[0] != (Link{"revision", "abcdef1234567890", false}) {
		t.Errorf("%+v", th)
	}
}

// TestUnhealthyAppHasOneOpenThread: whatever syncs and edges come, an app whose last edge is Degraded has exactly one
// open thread.
func TestUnhealthyAppHasOneOpenThread(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		rs := []Raw{health(1, "web", 0, "Healthy", "Degraded")}
		m := 0
		for i := 2; i < 12; i++ {
			m += rng.Intn(40)
			if rng.Intn(2) == 0 {
				rs = append(rs, synced(int64(i), "web", m))
			} else {
				rs = append(rs, health(int64(i), "web", m, "Degraded", "Degraded"))
			}
		}
		if n, needs := open(fold(t, m+500, rs...)); n != 1 || !needs {
			t.Fatalf("round %d: %d open, needs=%v", round, n, needs)
		}
	}
}

// TestFoldTimeBeatsSeq: occurred time orders events, not their log position (a late delivery has a high seq).
func TestFoldTimeBeatsSeq(t *testing.T) {
	f, r := alertRaw(2, "firing", "a", sa, 0, 0, ""), alertRaw(1, "resolved", "a", sa, 0, 5, "")
	if n, _ := open(fold(t, 10, f, r)); n != 0 {
		t.Error("resolved at minute 5 comes after the firing at minute 0 whatever their seq")
	}
}

// TestFoldDedup: a repeated (source, id) adds nothing, not even an entry.
func TestFoldDedup(t *testing.T) {
	f := alertRaw(1, "firing", "a", sa, 0, 0, "")
	if th := fold(t, 10, f, f, f); len(th) != 1 || len(th[0].Entries) != 1 {
		t.Errorf("%+v", th)
	}
	s := synced(1, "web", 0)
	if th := fold(t, 5, s, s); len(th[0].Entries) != 1 {
		t.Errorf("%+v", th[0].Entries)
	}
}

func TestFoldTiesOnSeq(t *testing.T) {
	f, r := alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 0, "") // the same minute
	if n, _ := open(fold(t, 1, f, r)); n != 0 {
		t.Error("resolved (seq 2) after firing (seq 1) must close")
	}
	f.Seq, r.Seq = 2, 1
	if n, _ := open(fold(t, 1, f, r)); n != 1 {
		t.Error("firing (seq 2) after resolved (seq 1) must stay open")
	}
}

// TestFoldOrderFree: any order of the same events folds to the same threads.
func TestFoldOrderFree(t *testing.T) {
	rs := []Raw{alertRaw(1, "firing", "a", sa, 0, 0, ""), alertRaw(2, "resolved", "a", sa, 0, 3, ""), alertRaw(3, "firing", "a", sa, 1, 6, ""),
		synced(4, "web", 1), health(5, "web", 2, "Progressing", "Degraded"), health(6, "web", 40, "Degraded", "Healthy"), flapping(7, "db", 3)}
	want := fold(t, 100, rs...)
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 50; i++ {
		rng.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
		if got := fold(t, 100, rs...); !reflect.DeepEqual(got, want) {
			t.Fatalf("order changed the answer:\n%+v\n%+v", got, want)
		}
	}
}

// TestFoldLinear: 20,000 syncs of one app (one thread, 20,000 entries) fold in well under a second.
func TestFoldLinear(t *testing.T) {
	var evs []Event
	for i := 0; i < 20000; i++ {
		e, _ := Parse(synced(int64(i+1), "web", 0))
		e.Time = t0.Add(time.Duration(i) * time.Second)
		evs = append(evs, e)
	}
	start := time.Now()
	th := Fold(evs, t0.Add(6*time.Hour))
	if d := time.Since(start); d > time.Second || len(th) != 1 || len(th[0].Entries) != 20000 {
		t.Errorf("%d threads, %d entries, %s", len(th), len(th[0].Entries), d)
	}
}

func TestParseRefuses(t *testing.T) {
	for name, r := range map[string]Raw{
		"other type": {ID: "x", Type: "dev.bnei.forge.push", Time: t0},
		"no id":      {Type: "dev.bnei.alert.firing", Time: t0, Data: []byte(`{"fingerprint":"a"}`)},
		"bad json":   {ID: "x", Type: "dev.bnei.alert.firing", Time: t0, Data: []byte(`{`)},
		"no key":     {ID: "x", Type: "dev.bnei.alert.firing", Time: t0, Data: []byte(`{}`)},
		"no app":     {ID: "x", Type: "dev.bnei.deploy.health", Time: t0, Data: []byte(`{}`)},
		"no time":    {ID: "x", Type: "dev.bnei.deploy.health", Data: []byte(`{"refs":{"app":"a"}}`)},
	} {
		if _, ok := Parse(r); ok {
			t.Errorf("%s parsed", name)
		}
	}
}

func fire(seq int64, fp string, m int) Raw { return alertRaw(seq, "firing", fp, sa, 0, m, "") }

func TestFeedCapsAndBytes(t *testing.T) {
	f := New(3, 1<<20)
	for i := 1; i <= 5; i++ {
		if !f.Ingest(fire(int64(i), fmt.Sprintf("f%d", i), i)) {
			t.Fatalf("event %d not stored", i)
		}
	}
	if f.Ingest(fire(5, "f5", 5)) {
		t.Error("a repeat was stored")
	}
	if s := f.Stats(); s.Events != 3 || s.Dropped != 2 || s.Threads != 3 || s.Last.IsZero() {
		t.Errorf("%+v", s)
	}
	if len(f.seen) != len(f.evs) { // seen must shrink with the events, or it grows for ever
		t.Errorf("seen %d, events %d", len(f.seen), len(f.evs))
	}
	// a late event older than all of them is the one dropped (its seq is new: the bridge may deliver a retry late)
	f2 := New(2, 1<<20)
	f2.Ingest(fire(5, "a", 5))
	f2.Ingest(fire(6, "b", 6))
	if f2.Ingest(fire(7, "old", -10)); f2.Stats().Dropped != 1 || f2.Stats().Events != 2 {
		t.Errorf("late: %+v", f2.Stats())
	}

	g := New(1000, 3000)
	for i := 1; i <= 50; i++ {
		g.Ingest(fire(int64(i), fmt.Sprintf("f%d", i), i))
	}
	s := g.Stats()
	sum := 0
	for _, e := range g.evs {
		sum += e.size()
	}
	if s.Bytes != sum || s.Bytes > 3000 || s.Events < 2 || s.Dropped != uint64(50-s.Events) {
		t.Errorf("byte cap: %+v sum %d", s, sum)
	}
	// the size is the real struct and its strings, not a constant: a longer text costs more
	long := fire(1, "x", 1)
	long.Data = []byte(strings.Replace(string(long.Data), `"labels":{`, `"annotations":{"summary":"`+strings.Repeat("s", 300)+`"},"labels":{`, 1))
	a, _ := Parse(fire(1, "x", 1))
	b, _ := Parse(long)
	if b.size()-a.size() < 300 || a.size() < 200 {
		t.Errorf("sizes %d %d", a.size(), b.size())
	}
}

// TestReplayDoesNotRecount: events dropped for the cap come again in a second Replay and are not counted again.
func TestReplayDoesNotRecount(t *testing.T) {
	f := New(2, 1<<20)
	var rs []Raw
	for i := 1; i <= 5; i++ {
		rs = append(rs, fire(int64(i), fmt.Sprintf("f%d", i), i))
	}
	for pass := 0; pass < 3; pass++ {
		for _, r := range rs {
			f.Ingest(r)
		}
	}
	if s := f.Stats(); s.Dropped != 3 || s.Events != 2 {
		t.Errorf("%+v", s)
	}
}

func TestIngestPanicIsCounted(t *testing.T) {
	f := New(10, 1<<20)
	f.SetParse(func(r Raw) (Event, bool) {
		if r.Seq == 2 {
			panic("boom")
		}
		return Parse(r)
	})
	f.Ingest(fire(1, "a", 1))
	if f.Ingest(fire(2, "b", 2)) {
		t.Error("a panicking event was stored")
	}
	f.Ingest(fire(3, "c", 3))
	if s := f.Stats(); s.Events != 2 || s.Bad != 1 {
		t.Errorf("%+v", s)
	}
}

func TestCacheFollowsIngestAndClock(t *testing.T) {
	f := New(100, 1<<20)
	now := t0
	f.now = func() time.Time { return now }
	f.Ingest(synced(1, "web", 0))
	if p, _, _ := f.List(10, ""); len(p) != 1 || p[0].State != StateOpen {
		t.Fatalf("%+v", p)
	}
	f.Ingest(synced(2, "api", 0)) // an ingest invalidates the cache at once
	if p, _, _ := f.List(10, ""); len(p) != 2 {
		t.Errorf("stale after ingest: %d", len(p))
	}
	now = t0.Add(cacheTTL / 2) // inside the TTL, nothing new: the cached fold is served
	f.Ingest(Raw{})
	now = t0.Add(RolloutWindow + time.Minute) // the clock moves a rollout to closed with no ingest
	if p, _, _ := f.List(10, ""); p[0].State != StateClosed {
		t.Errorf("stale after the clock moved: %+v", p[0])
	}
}

func TestListCursor(t *testing.T) {
	f := New(100, 1<<20)
	for i := 1; i <= 5; i++ {
		f.Ingest(fire(int64(i), fmt.Sprintf("f%d", i), i))
	}
	var got []string
	cur := ""
	for {
		page, next, err := f.List(2, cur)
		if err != nil {
			t.Fatal(err)
		}
		for _, th := range page {
			if len(th.Entries) > 1 || th.Links != nil {
				t.Error("list carries more than the last entry, or links")
			}
			got = append(got, th.ID)
		}
		if next == "" {
			break
		}
		cur = next
	}
	if len(got) != 5 {
		t.Fatalf("%v", got)
	}
	first, _, _ := f.List(1, "")
	if first[0].ID != got[0] || first[0].LastActivityAt != at(5) {
		t.Errorf("not newest first: %+v", first)
	}
	if th, ok := f.Get(got[4]); !ok || len(th.Entries) != 1 || len(th.Links) != 1 {
		t.Errorf("get: %+v", th)
	}
	if _, ok := f.Get("nope"); ok {
		t.Error("get nope")
	}
	if _, _, err := f.List(2, "junk"); err == nil {
		t.Error("junk cursor accepted")
	}
}

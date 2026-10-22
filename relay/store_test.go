package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDSN returns MOUNTAIN_TEST_DSN (a URL) pointed at a fresh schema, or skips: DB tests need a real Postgres
// (TestDB*, CI job db-test; locally: docker run -d --rm -p 55432:5432 -e POSTGRES_PASSWORD=x postgres:17 and
// MOUNTAIN_TEST_DSN=postgres://postgres:x@127.0.0.1:55432/postgres?sslmode=disable).
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("MOUNTAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("MOUNTAIN_TEST_DSN unset: DB test skipped")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("t%d", time.Now().UnixNano())
	if _, err := c.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); c.Close(ctx) })
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema // pgx passes unknown URL params as runtime params
}

func storedApp(t *testing.T, dsn string) *App {
	t.Helper()
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(cfg, Opts{FPSalt: devFPSalt}, utcDay(time.Now()))
	a.attach(context.Background(), dsn, false)
	if a.store == nil {
		t.Fatal("attach: no store")
	}
	return a
}

// TestDBRestartSurvival: plots (before and after a snapshot), a firing alert, health, a build and a plant survive a
// close and a reopen from the same DB; the hello rebuilt from it is the same town.
func TestDBRestartSurvival(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	withToys(t, a)
	pl, prom, out := newPlacer(), &Out{app: a, name: "prom"}, &Out{app: a, name: "alerts"}
	pl.place(a, prom, []sample{pod("wird", "wird-1", "k8s-cp-01")}, nil, allReady, time.Now())
	a.snapshot(time.Now())
	pl.place(a, prom, []sample{pod("wird", "wird-1", "k8s-cp-01"), pod("traefik", "traefik-1", "k8s-cp-02")}, nil, allReady, time.Now())
	al := Alert{FP: a.FP("abc"), Sev: "critical", Layer: "k8s", StartsAt: 1760450000000, Host: "h2"}
	if out.Fire(al) == "" || out.Fire(Alert{FP: a.FP("gone"), Sev: "warning", Layer: "apps", StartsAt: 1}) == "" {
		t.Fatal("fire dropped")
	}
	out.Resolve(Alert{FP: a.FP("gone"), Sev: "warning", Layer: "apps"}, 5)
	out.Ev("argo.health", map[string]any{"app": "wird", "status": "Degraded"})
	out.Ev("build", map[string]any{"app": "wird", "state": "running"})
	if _, why := ackOf(t, a.place(visitor("s1", "ana"), "r", "plant", map[string]any{"h": 1.0, "q": 4.0, "r": -2.0}, false, time.Now())); why != "" {
		t.Fatalf("plant: %s", why)
	}
	if len(a.State.Plots) != 2 || len(a.State.Marks) != 1 || a.State.Builds["wird"] != "running" {
		t.Fatalf("live state: %+v", a.State)
	}
	a.store.Close(5 * time.Second)

	b := storedApp(t, dsn)
	defer b.store.Close(time.Second)
	if fmt.Sprint(a.State) != fmt.Sprint(b.State) || len(b.State.Alerts) != 1 || b.State.Alerts[0] != al {
		t.Fatalf("state: %v, want %v", b.State, a.State)
	}
	var ha, hb struct {
		Snapshot Snapshot
		Deltas   []map[string]any
	}
	json.Unmarshal(a.Hello(), &ha)
	json.Unmarshal(b.Hello(), &hb)
	if ha.Snapshot != hb.Snapshot || hb.Snapshot.Epoch != townEpoch || fmt.Sprint(ha.Deltas) != fmt.Sprint(hb.Deltas) || len(hb.Deltas) != 2 {
		t.Fatalf("hello: snapshot or deltas differ (%d vs %d deltas)", len(ha.Deltas), len(hb.Deltas))
	}
	// a third boot after a snapshot: no deltas, same state
	b.snapshot(time.Now())
	b.store.flush(5 * time.Second)
	c := storedApp(t, dsn)
	defer c.store.Close(time.Second)
	if len(c.since) != 0 || fmt.Sprint(c.State) != fmt.Sprint(a.State) {
		t.Fatalf("restore from the snapshot row alone: %d deltas, state %+v, want %+v", len(c.since), c.State, a.State)
	}
	// a snapshot row from before the town's state fields (alerts only) restores the defaults, not nil
	if _, err := c.store.pool.Exec(context.Background(), `UPDATE snapshots SET state = '{"alerts":[]}'`); err != nil {
		t.Fatal(err)
	}
	d := storedApp(t, dsn)
	defer d.store.Close(time.Second)
	if d.State.Plots == nil || d.State.Health == nil || d.State.Marks == nil {
		t.Fatalf("old snapshot row: %+v", d.State)
	}
}

// TestDBTownEpoch: migration 004 archives the mountain's history (nothing is lost) and hides its toys' marks; a
// mountain snapshot row (seed 1) is never folded into the town; the rollback script brings the mountain back.
func TestDBTownEpoch(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	// the mountain's database: migrations 001-003 and some history
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	upTo := func(last string) {
		for _, f := range []string{"001_init.sql", "002_detail.sql", "003_marks.sql", "004_town.sql"} {
			if f > last {
				return
			}
			b, _ := migrations.ReadFile("migrations/" + f)
			if _, err := pool.Exec(ctx, string(b)); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, at timestamptz NOT NULL DEFAULT now())")
			pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", f)
		}
	}
	upTo("003_marks.sql")
	for _, q := range []string{
		`INSERT INTO snapshots (day, seq, ts, seed, land, state, metrics) VALUES ('2026-10-20', nextval('frame_seq'), now(), 1, '\x0000', '{"alerts":[],"veins":[]}', '{}')`,
		`INSERT INTO events (id, ts, day, kind, public) VALUES ('01JA0000000000000000000001', now(), '2026-10-20', 'argo.sync', '{"t":"ev","kind":"argo.sync","app":"wird","ok":true}')`,
		`INSERT INTO world_deltas (ts, day, kind, frame) VALUES (now(), '2026-10-20', 'deposit', '{"t":"world","kind":"deposit"}')`,
		`INSERT INTO marks (id, sub, handle, toy, ts, day, pose, weight) VALUES ('01JA0000000000000000000002', 's1', 'a visitor', 'cairns', now(), '2026-10-20', '{"pos":[1,2,3]}', 1)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	pool.Close()
	a := storedApp(t, dsn) // applies 004, finds no town snapshot, starts empty
	defer a.store.Close(time.Second)
	var n, archived, hidden int
	c.QueryRow(ctx, "SELECT count(*) FROM events").Scan(&n)
	c.QueryRow(ctx, "SELECT (SELECT count(*) FROM mountain_archive_events) + (SELECT count(*) FROM mountain_archive_world_deltas) + (SELECT count(*) FROM mountain_archive_snapshots)").Scan(&archived)
	c.QueryRow(ctx, "SELECT count(*) FROM marks WHERE deleted_at IS NOT NULL").Scan(&hidden)
	if n != 0 || archived != 3 || hidden != 1 || len(a.State.Plots) != 0 || len(a.State.Marks) != 0 {
		t.Fatalf("after 004: %d events, %d archived rows, %d hidden marks, state %+v", n, archived, hidden, a.State)
	}
	visits.Lock()
	seeded := len(visits.at["s1"])
	visits.Unlock()
	if seeded != 1 {
		t.Errorf("the town's first boot must still seed the rate limits from marks (s1: %d, want 1)", seeded)
	}
	if first, err := a.store.started(ctx); err != nil || first != utcDay(time.Now()) {
		t.Fatalf("town_epoch: %q %v", first, err)
	}
	// a mountain row written after 004 (a reverted image, say) is not the town's
	c.Exec(ctx, `INSERT INTO snapshots (day, seq, ts, seed, land, state, metrics) VALUES ('2026-10-19', nextval('frame_seq'), now(), 1, '\x00', '{"alerts":[{"fp":"0123456789abcdef","sev":"info","layer":"apps","startsAt":1}]}', '{}')`)
	if r, err := a.store.load(ctx, nil); err != nil || r == nil || len(r.state.Alerts) != 0 {
		t.Fatalf("load must read the town's own snapshot only: %+v %v", r, err)
	}
	a.store.flush(5 * time.Second)
	// the rollback script restores the mountain exactly
	b, err := os.ReadFile("rollback/004_town_down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, string(b)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	c.QueryRow(ctx, "SELECT (SELECT count(*) FROM events) + (SELECT count(*) FROM world_deltas)").Scan(&n)
	c.QueryRow(ctx, "SELECT count(*) FROM marks WHERE deleted_at IS NOT NULL").Scan(&hidden)
	if n != 2 || hidden != 0 {
		t.Fatalf("after the rollback: %d mountain frames back, %d marks still hidden", n, hidden)
	}
	// forward again, then back again: the rollback runs twice (town_archive_* already exists)
	m, _ := migrations.ReadFile("migrations/004_town.sql")
	if _, err := c.Exec(ctx, string(m)); err != nil {
		t.Fatalf("004 again: %v", err)
	}
	c.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ('004_town.sql')")
	if _, err := c.Exec(ctx, string(b)); err != nil {
		t.Fatalf("second rollback: %v", err)
	}
	c.QueryRow(ctx, "SELECT count(*) FROM events").Scan(&n)
	if n != 1 {
		t.Fatalf("after the second rollback: %d mountain events, want 1", n)
	}
}

// TestDBR8: the first R8 boot drops the plots of an R7 snapshot (the first poll re-places them by family), a later
// boot keeps R8's own; the rollback script hides R8's frames, snapshots and plants and puts the R7.1 day row back.
func TestDBR8(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	a := storedApp(t, dsn)
	pl, prom := newPlacer(), &Out{app: a, name: "prom"}
	pl.place(a, prom, []sample{pod("wird", "wird-1", "k8s-cp-01")}, nil, allReady, time.Now())
	a.snapshot(time.Now())
	a.store.Close(5 * time.Second)
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	// as if that town were R7.1's and 005 ran now: its day row is archived, R8 starts after it
	for _, q := range []string{"UPDATE town_r8 SET started = now()", "INSERT INTO r7_archive_day_snapshot SELECT * FROM snapshots"} {
		if _, err := c.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	b := storedApp(t, dsn)
	if len(b.State.Plots) != 0 {
		t.Fatalf("R8's first boot keeps R7 plots: %+v", b.State.Plots)
	}
	pl, prom = newPlacer(), &Out{app: b, name: "prom"}
	pl.place(b, prom, []sample{pod("wird", "wird-1", "k8s-cp-01")}, nil, allReady, time.Now())
	withToys(t, b)
	if _, why := ackOf(t, b.place(visitor("s1", "ana"), "r", "plant", map[string]any{"h": 1.0, "q": 6.0, "r": -1.0}, false, time.Now())); why != "" {
		t.Fatalf("plant on ring 6: %s", why)
	}
	b.snapshot(time.Now())
	b.store.Close(5 * time.Second)
	d := storedApp(t, dsn)
	if len(d.State.Plots) != 1 || len(d.State.Marks) != 1 {
		t.Fatalf("a later R8 boot: plots %+v marks %d", d.State.Plots, len(d.State.Marks))
	}
	d.store.Close(5 * time.Second)
	down, err := os.ReadFile("rollback/005_r8_down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 { // twice: r8_archive_* already exists the second time
		if _, err := c.Exec(ctx, string(down)); err != nil {
			t.Fatalf("rollback %d: %v", i, err)
		}
		if i == 0 {
			m, _ := migrations.ReadFile("migrations/005_r8.sql")
			c.Exec(ctx, string(m))
			c.Exec(ctx, "UPDATE town_r8 SET started = now() - interval '1 hour'") // R8 again, from before the rows
			c.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ('005_r8.sql')")
		}
	}
	var deltas, hidden, done int
	var state string
	c.QueryRow(ctx, "SELECT count(*) FROM world_deltas").Scan(&deltas)
	c.QueryRow(ctx, "SELECT count(*) FROM marks WHERE deleted_at IS NOT NULL").Scan(&hidden)
	c.QueryRow(ctx, "SELECT count(*) FROM schema_migrations WHERE version = '005_r8.sql'").Scan(&done)
	c.QueryRow(ctx, "SELECT state::text FROM snapshots").Scan(&state)
	if deltas != 0 || hidden != 1 || done != 0 || !strings.Contains(state, `"app": "wird"`) {
		t.Fatalf("after the rollback: %d R8 deltas left, %d plants hidden, 005 applied %d, day row %s", deltas, hidden, done, state)
	}
}

// TestDBReplay: scrub yesterday and today from stored frames; the start-of-day state folds in the frames after
// yesterday's last snapshot; a day before the town (town_epoch) is unavailable; the frame validates against
// www/dev/schema/replay.json.
func TestDBReplay(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn) // boots on an empty DB: no town snapshot yet, so it writes today's
	s := a.store
	defer s.Close(time.Second)
	now := time.Now().UTC()
	today, yday, before := utcDay(now), utcDay(now.AddDate(0, 0, -1)), utcDay(now.AddDate(0, 0, -2))
	if _, err := s.pool.Exec(context.Background(), "TRUNCATE snapshots"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(context.Background(), "UPDATE town_epoch SET started = $1", yday); err != nil {
		t.Fatal(err)
	}
	at := func(day string, h int) time.Time {
		d, _ := time.Parse(time.DateOnly, day)
		return d.Add(time.Duration(h) * time.Hour)
	}
	out := &Out{app: a, name: "test"}
	plot := func(app string, q int, ts time.Time) Plot {
		p := Plot{App: app, Host: "h1", Q: q, R: 1, Role: "home"}
		s.frame("world", "plot", "", ts, out.worldFrame("plot", p.frame(false)))
		return p
	}
	st := newState()
	s.snapshot(yday, at(yday, 0), mustJSON(st), mustJSON(emptyMetrics()))
	st.Plots = append(st.Plots, plot("wird", 1, at(yday, 12)))
	al := Alert{FP: a.FP("x"), Sev: "warning", Layer: "k8s", StartsAt: at(yday, 12).UnixMilli()}
	_, fire := out.evFrame("alert.fire", map[string]any{"fp": al.FP, "sev": al.Sev, "layer": al.Layer, "startsAt": int(al.StartsAt)})
	s.frame("ev", "alert.fire", ULID(), at(yday, 13), fire)
	st.Alerts = append(st.Alerts, al)
	s.snapshot(yday, at(yday, 23), mustJSON(st), mustJSON(emptyMetrics()))
	plot("dream-analyst", 2, at(yday, 23).Add(30*time.Minute)) // after yesterday's last snapshot
	plot("vos-monolith", -1, at(today, 0).Add(time.Minute))
	s.metrics(at(today, 0).Add(5*time.Minute), mustJSON(emptyMetrics()))
	if !s.flush(5 * time.Second) {
		t.Fatal("flush")
	}

	type replayF struct {
		T, Day, Error string
		Snapshot      Snapshot
		State         State
		Frames        []map[string]any
		Metrics       []map[string]any
	}
	get := func(day string) (replayF, map[string]any) {
		b := a.replay(day, now)
		var r replayF
		var m map[string]any
		json.Unmarshal(b, &r)
		json.Unmarshal(b, &m)
		return r, m
	}
	ry, my := get(yday)
	if ry.Error != "" || ry.Day != yday || len(ry.Frames) != 3 || len(ry.State.Alerts) != 0 || len(ry.State.Plots) != 0 {
		t.Fatalf("yesterday: error %q, %d frames, state at start %+v", ry.Error, len(ry.Frames), ry.State)
	}
	rt, mt := get(today)
	if rt.Error != "" || len(rt.Frames) != 1 || len(rt.Metrics) != 1 || len(rt.State.Alerts) != 1 || rt.State.Alerts[0] != al ||
		len(rt.State.Plots) != 2 || rt.Snapshot.Day != today {
		t.Fatalf("today: error %q, %d frames, %d metrics, state %v", rt.Error, len(rt.Frames), len(rt.Metrics), rt.State)
	}
	if rb, _ := get(before); rb.Error != "unavailable" {
		t.Fatalf("a day before the town: %q, want unavailable", rb.Error)
	}
	replays.Lock()
	cy, ct := replays.m[yday], replays.m[today]
	replays.Unlock()
	settled := now.Sub(now.Truncate(24*time.Hour)) >= dayGrace // yesterday is still TTL'd in the first minute of today
	if cy.b == nil || cy.until.IsZero() != settled || ct.until != now.Add(todayTTL) {
		t.Fatalf("cache: yesterday until %v (want none), today until %v (want +%v)", cy.until, ct.until, todayTTL)
	}
	// every build slot taken: today (expired) is refused, yesterday (cached) is still served
	for range cap(scrubBuilds) {
		scrubBuilds <- struct{}{}
	}
	later := now.Add(todayTTL + time.Second)
	var busy, cached struct{ Error string }
	json.Unmarshal(a.replay(today, later), &busy)
	json.Unmarshal(a.replay(yday, later), &cached)
	for range cap(scrubBuilds) {
		<-scrubBuilds
	}
	if busy.Error != "rate" || (settled && cached.Error != "") {
		t.Fatalf("builds full: today %q (want rate), yesterday %q (want served)", busy.Error, cached.Error)
	}
	validate(t, [][]map[string]any{{my, mt}})
}

// TestScrubRequests: day validation, memory-only, and the per-connection rate limit (no DB needed).
func TestScrubRequests(t *testing.T) {
	a, _ := testApp(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for day, want := range map[string]string{
		"2026-10-14": "unavailable", "2026-09-14": "unavailable", "2026-09-13": "bad-day", "2026-10-15": "bad-day",
		"2026-10-14T00:00": "bad-day", "yesterday": "bad-day", "": "bad-day", "2026-02-30": "bad-day", "+2026-10-14": "bad-day",
	} {
		var r struct{ Day, Error string }
		json.Unmarshal(a.replay(day, now), &r)
		if r.Error != want || (want == "bad-day" && r.Day != "") {
			t.Errorf("scrub %q: %+v, want error %s", day, r, want)
		}
	}
	app = a
	c := &Client{send: make(chan []byte, 4)}
	for range 2 {
		clientMsg(c, []byte(`{"t":"scrub","day":"`+utcDay(time.Now())+`"}`))
	}
	clientMsg(c, []byte(`{"t":"subscribe"}`))
	var got []string
	frames := drain(c)
	for _, f := range frames {
		got = append(got, fmt.Sprint(f["error"]))
	}
	if strings.Join(got, ",") != "unavailable,rate" {
		t.Fatalf("two scrubs within 2 s: %v, want unavailable then rate (and nothing for subscribe)", got)
	}
	validate(t, [][]map[string]any{frames})
}

// TestAlertsRestored: alerts restored from the DB count as firing. Still active = no second alert.fire
// (startsAt kept); gone from Alertmanager = resolved on the first good poll (no stuck cauldron).
func TestAlertsRestored(t *testing.T) {
	a, c := testApp(t)
	a.Opts.FPSalt = devFPSalt
	still := Alert{FP: a.FP("still"), Sev: "critical", Layer: "k8s", StartsAt: 1760000000000}
	gone := Alert{FP: a.FP("gone"), Sev: "warning", Layer: "apps", StartsAt: 1760000000000}
	a.State.Alerts = []Alert{still, gone}
	am := json.RawMessage(`[{"fingerprint":"still","startsAt":"2025-10-09T08:53:20Z","labels":{"alertname":"KubePodCrashLooping","severity":"critical"}}]`)
	_, steps := runAlerts(t, a, c, &fixture{am: []json.RawMessage{am, am}})
	if f := evs(steps[0], "alert.fire"); len(f) != 0 {
		t.Errorf("a restored, still-active alert must not fire again: %v", f)
	}
	if r := evs(steps[0], "alert.resolve"); len(r) != 1 || r[0]["fp"] != gone.FP {
		t.Errorf("a restored alert gone from Alertmanager must resolve on the first poll: %v", r)
	}
	if len(steps[1]) != 0 || len(a.State.Alerts) != 1 || a.State.Alerts[0] != still {
		t.Errorf("then quiet, state = the still-active alert with its startsAt: %v %v", steps[1], a.State.Alerts)
	}
}

// TestSnapshotRollover (memory-only): a snapshot folds the hello deltas into the state, and at UTC rollover App.Day
// advances and hello.snapshot.day follows.
func TestSnapshotRollover(t *testing.T) {
	a, _ := testApp(t)
	newPlacer().place(a, &Out{app: a, name: "prom"}, []sample{pod("wird", "wird-1", "k8s-cp-01")}, nil, allReady, time.Now())
	var h struct {
		Snapshot Snapshot
		Deltas   []json.RawMessage
		State    State
	}
	json.Unmarshal(a.Hello(), &h)
	if len(h.Deltas) != 1 || len(h.State.Plots) != 1 {
		t.Fatalf("before a snapshot: the plot delta, got %d deltas", len(h.Deltas))
	}
	a.snapshot(time.Date(2026, 10, 15, 0, 0, 30, 0, time.UTC))
	json.Unmarshal(a.Hello(), &h)
	if h.Snapshot.Day != "2026-10-15" || a.Day != "2026-10-15" || len(h.Deltas) != 0 || len(h.State.Plots) != 1 {
		t.Fatalf("after rollover: day %s, %d deltas", h.Snapshot.Day, len(h.Deltas))
	}
}

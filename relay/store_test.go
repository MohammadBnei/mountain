package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
	"github.com/jackc/pgx/v5"
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
	a := newApp(cfg, Opts{Seed: 1, FPSalt: devFPSalt}, utcDay(time.Now()))
	a.attach(context.Background(), dsn, false)
	if a.store == nil {
		t.Fatal("attach: no store")
	}
	return a
}

// TestDBRestartSurvival: deposits (before and after a snapshot) and a firing alert survive a close and a
// reopen from the same DB; the hello rebuilt from it is the same world.
func TestDBRestartSurvival(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	p, out := newProm(a), &Out{app: a, name: "alerts"}
	p.deposit(out, "wird", 1)
	a.snapshot(time.Now())
	p.deposit(out, "editable-blog", 0.5)
	al := Alert{FP: a.FP("abc"), Sev: "critical", Layer: "k8s", StartsAt: 1760450000000}
	if out.Fire(al) == "" || out.Fire(Alert{FP: a.FP("gone"), Sev: "warning", Layer: "apps", StartsAt: 1}) == "" {
		t.Fatal("fire dropped")
	}
	out.Resolve(Alert{FP: a.FP("gone"), Sev: "warning", Layer: "apps"}, 5)
	a.store.Close(5 * time.Second)

	b := storedApp(t, dsn)
	defer b.store.Close(time.Second)
	if !slices.Equal(a.Land, b.Land) || !slices.ContainsFunc(b.Land, func(v int16) bool { return v != 0 }) {
		t.Fatal("h.land differs after restart")
	}
	if fmt.Sprint(a.State) != fmt.Sprint(b.State) || len(b.State.Alerts) != 1 || b.State.Alerts[0] != al {
		t.Fatalf("state: %v, want %v", b.State, a.State)
	}
	var ha, hb struct {
		Snapshot Snapshot
		Deltas   []map[string]any
	}
	json.Unmarshal(a.Hello(), &ha)
	json.Unmarshal(b.Hello(), &hb)
	if ha.Snapshot.Land != hb.Snapshot.Land || fmt.Sprint(ha.Deltas) != fmt.Sprint(hb.Deltas) || len(hb.Deltas) != 2 {
		t.Fatalf("hello: snapshot or deltas differ (%d vs %d deltas)", len(ha.Deltas), len(hb.Deltas))
	}
	// a third boot after a snapshot: no deltas, same land
	b.snapshot(time.Now())
	b.store.flush(5 * time.Second)
	c := storedApp(t, dsn)
	defer c.store.Close(time.Second)
	if !slices.Equal(c.Land, a.Land) || len(c.since) != 0 {
		t.Fatalf("restore after a snapshot: land equal %v, %d deltas", slices.Equal(c.Land, a.Land), len(c.since))
	}
}

// TestDBReplay: scrub yesterday and today from stored frames; the start-of-day land folds in the frames
// after yesterday's last snapshot; the frame validates against www/dev/schema/replay.json.
func TestDBReplay(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn) // boots on an empty DB: no snapshot of seed 1 yet, so it writes today's
	s := a.store
	defer s.Close(time.Second)
	if _, err := s.pool.Exec(context.Background(), "TRUNCATE snapshots"); err != nil { // start the history yesterday instead
		t.Fatal(err)
	}
	now := time.Now().UTC()
	today, yday := utcDay(now), utcDay(now.AddDate(0, 0, -1))
	at := func(day string, h int) time.Time {
		d, _ := time.Parse(time.DateOnly, day)
		return d.Add(time.Duration(h) * time.Hour)
	}
	out := &Out{app: a, name: "test"}
	land := make([]int16, worldcore.Grid*worldcore.Grid)
	dep := func(id string, ts time.Time) {
		d := worldcore.RasterDeposit(id, 10, 10, 1, worldcore.Solid(a.World.Base, land))
		d.AddTo(land)
		s.frame("world", "deposit", "", ts, out.worldFrame("deposit", map[string]any{"id": id, "x0": d.X0, "z0": d.Z0, "w": d.W, "h": d.H, "cells": worldcore.EncodeI16(d.Cells)}))
	}
	st := State{Alerts: []Alert{}}
	s.snapshot(yday, at(yday, 0), 1, land, mustJSON(st), mustJSON(emptyMetrics(1)))
	dep("01JA0000000000000000000001", at(yday, 12))
	al := Alert{FP: a.FP("x"), Sev: "warning", Layer: "k8s", StartsAt: at(yday, 12).UnixMilli()}
	_, fire := out.evFrame("alert.fire", map[string]any{"fp": al.FP, "sev": al.Sev, "layer": al.Layer, "startsAt": int(al.StartsAt)})
	s.frame("ev", "alert.fire", ULID(), at(yday, 13), fire)
	st.set(al)
	s.snapshot(yday, at(yday, 23), 1, land, mustJSON(st), mustJSON(emptyMetrics(1)))
	dep("01JA0000000000000000000002", at(yday, 23).Add(30*time.Minute)) // after yesterday's last snapshot
	startToday := slices.Clone(land)
	dep("01JA0000000000000000000003", at(today, 0).Add(time.Minute))
	s.metrics(at(today, 0).Add(5*time.Minute), mustJSON(emptyMetrics(1)))
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
	if ry.Error != "" || ry.Day != yday || len(ry.Frames) != 3 || len(ry.State.Alerts) != 0 || ry.Snapshot.Land != worldcore.EncodeI16(make([]int16, len(land))) {
		t.Fatalf("yesterday: error %q, %d frames, %d alerts at start", ry.Error, len(ry.Frames), len(ry.State.Alerts))
	}
	rt, mt := get(today)
	if rt.Error != "" || len(rt.Frames) != 1 || len(rt.Metrics) != 1 || len(rt.State.Alerts) != 1 || rt.State.Alerts[0] != al {
		t.Fatalf("today: error %q, %d frames, %d metrics, state %v", rt.Error, len(rt.Frames), len(rt.Metrics), rt.State)
	}
	if rt.Snapshot.Land != worldcore.EncodeI16(startToday) || rt.Snapshot.Day != today {
		t.Fatal("today's snapshot must be yesterday's last snapshot + the deposit after it")
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

// TestSnapshotRollover (memory-only): a snapshot folds the hello deltas into hello.snapshot.land, and at
// UTC rollover App.Day advances and hello.snapshot.day follows.
func TestSnapshotRollover(t *testing.T) {
	a, _ := testApp(t)
	newProm(a).deposit(&Out{app: a}, "wird", 1)
	var h struct {
		Snapshot Snapshot
		Deltas   []json.RawMessage
	}
	json.Unmarshal(a.Hello(), &h)
	if h.Snapshot.Land != worldcore.EncodeI16(make([]int16, len(a.Land))) || len(h.Deltas) != 2 {
		t.Fatalf("before a snapshot: base land + deposit and site deltas, got %d deltas", len(h.Deltas))
	}
	a.snapshot(time.Date(2026, 10, 15, 0, 0, 30, 0, time.UTC))
	json.Unmarshal(a.Hello(), &h)
	if h.Snapshot.Day != "2026-10-15" || a.Day != "2026-10-15" || h.Snapshot.Land != worldcore.EncodeI16(a.Land) || len(h.Deltas) != 0 {
		t.Fatalf("after rollover: day %s, %d deltas", h.Snapshot.Day, len(h.Deltas))
	}
}

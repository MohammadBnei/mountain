package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestCalm: State.Calm, kept by the fold alone: a warning or critical alert or a Degraded/Missing app clears it;
// calm again starts it on that frame's day; info alerts and Progressing do not trouble the town.
func TestCalm(t *testing.T) {
	s := newState()
	day := func(d int) int64 { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC).UnixMilli() }
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "wird", Status: "Healthy", TS: day(20)}, nil)
	if s.Calm != "2026-10-20" {
		t.Fatalf("calm from a healthy frame: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "alert.fire", FP: "a", Sev: "info", TS: day(21)}, nil)
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "wird", Status: "Progressing", TS: day(21)}, nil)
	if s.Calm != "2026-10-20" {
		t.Fatalf("info and Progressing must not trouble: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "alert.fire", FP: "b", Sev: "warning", TS: day(22)}, nil)
	if s.Calm != "" {
		t.Fatalf("a warning ends the calm: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "mountain", Status: "Degraded", TS: day(22)}, nil)
	s.apply(frameF{T: "ev", Kind: "alert.resolve", FP: "b", Sev: "warning", TS: day(23)}, nil)
	if s.Calm != "" {
		t.Fatalf("still Degraded: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "mountain", Status: "Healthy", TS: day(24)}, nil)
	if s.Calm != "2026-10-24" {
		t.Fatalf("calm again on the day it ended: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "wird", Status: "Healthy", TS: day(26)}, nil)
	if s.Calm != "2026-10-24" {
		t.Fatalf("a calm frame keeps the start: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "wird", Status: "Missing", TS: day(27)}, nil)
	s.apply(frameF{T: "ev", Kind: "app.gone", App: "wird"}, nil) // ts 0 (a frame with no time): no day to start from
	if s.Calm != "" {
		t.Fatalf("a frame with no time starts no calm: %q", s.Calm)
	}
	s.apply(frameF{T: "ev", Kind: "argo.health", App: "wird", Status: "Missing", TS: day(27)}, nil)
	s.apply(frameF{T: "ev", Kind: "app.gone", App: "wird", TS: day(28)}, nil)
	if s.Calm != "2026-10-28" {
		t.Fatalf("the troubled app gone ends the trouble: %q", s.Calm)
	}
}

// TestTechs: the ADR listing → numbers only (count, highest); other files, product slugs never leave; an unchanged
// listing sends nothing; a failure is not fatal.
func TestTechs(t *testing.T) {
	a, c := testApp(t)
	list := `[{"name":"0001-ingress-traefik.md"},{"name":"0064-mountain-town-roles.md"},{"name":"0065-mountain-behind-forwardauth.md"},{"name":"README.md"},{"name":"template.md"}]`
	fail, n304 := false, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "nope", http.StatusNotFound)
			return
		}
		if r.URL.Path != "/repos/MohammadBnei/infra-bootstrap/contents/docs/adr" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"adr1"` { // an unchanged listing: the cached body, no frame
			n304++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"adr1"`)
		w.Write([]byte(list))
	}))
	defer srv.Close()
	s := newGitHub(a)
	s.url = srv.URL
	o := &Out{app: a, name: "github"}
	s.techs(context.Background(), o)
	frames := drain(c)
	validate(t, [][]map[string]any{frames})
	if len(frames) != 1 || fmt.Sprint(frames[0]["count"], frames[0]["top"]) != "3 65" || a.State.Tech != (Tech{3, 65}) {
		t.Fatalf("tech: %v %v", frames, a.State.Tech)
	}
	for _, w := range []string{"traefik", "forwardauth", "mountain-town"} {
		if b := fmt.Sprint(frames); strings.Contains(b, w) {
			t.Fatalf("%q on the wire: %s", w, b)
		}
	}
	s.techs(context.Background(), o)
	if f := drain(c); len(f) != 0 || n304 != 1 || a.State.Tech != (Tech{3, 65}) {
		t.Fatalf("unchanged listing (304 %d) sent %v, tech %v", n304, f, a.State.Tech)
	}
	fail = true
	s.techs(context.Background(), o)
	if f := drain(c); len(f) != 0 || s.techErr == "" {
		t.Fatalf("failure: %v %q", f, s.techErr)
	}
}

// TestDBCivRestore: calm is seeded from the stored events when the snapshot has none; hello.snapshot.started is the
// town's first day.
func TestDBCivRestore(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	o := &Out{app: a, name: "prom"}
	o.Ev("argo.sync", map[string]any{"app": "wird", "ok": true})
	a.store.Close(5 * time.Second)
	b := storedApp(t, dsn)
	if b.snap(b.Day).Started != utcDay(time.Now()) {
		t.Fatalf("started: %q", b.snap(b.Day).Started)
	}
	if b.State.Calm != utcDay(time.Now()) {
		t.Fatalf("the first boot starts calm: %q", b.State.Calm)
	}
	b.store.Close(5 * time.Second)

	// a snapshot from before R9 (no calm key) after a history: the seed is the last day the trouble moved (the
	// warning's resolve, wird's recovery), not an info alert, not a Progressing → Healthy, never before day one
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	ev := func(id, day, kind, public string) {
		if _, err := c.Exec(ctx, `INSERT INTO events (id, ts, day, kind, public) VALUES ($1, $2::date, $2::date, $3, $4)`, id, day, kind, public); err != nil {
			t.Fatal(err)
		}
	}
	c.Exec(ctx, `UPDATE town_epoch SET started = '2026-09-01'`)
	ev("e1", "2026-10-05", "alert.fire", `{"sev":"warning","fp":"b"}`)
	ev("e2", "2026-10-06", "argo.health", `{"app":"wird","status":"Degraded"}`)
	ev("e3", "2026-10-07", "alert.resolve", `{"sev":"warning","fp":"b"}`)
	ev("e4", "2026-10-08", "argo.health", `{"app":"wird","status":"Healthy"}`)
	ev("e5", "2026-10-11", "argo.health", `{"app":"mountain","status":"Progressing"}`)
	ev("e6", "2026-10-12", "argo.health", `{"app":"mountain","status":"Healthy"}`)
	ev("e7", "2026-10-15", "alert.fire", `{"sev":"info","fp":"c"}`)
	if _, err := c.Exec(ctx, `UPDATE snapshots SET state = '{}', seq = nextval('frame_seq') WHERE seed = $1`, townEpoch); err != nil { // after the events
		t.Fatal(err)
	}
	d := storedApp(t, dsn)
	defer d.store.Close(time.Second)
	if d.State.Calm != "2026-10-08" {
		t.Fatalf("calm seeded on %q, want 2026-10-08", d.State.Calm)
	}
}

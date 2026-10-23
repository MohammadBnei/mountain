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

func evsOf(frames []map[string]any, kind string) (out []string) {
	for _, f := range frames {
		if f["t"] == "ev" && f["kind"] == kind {
			out = append(out, fmt.Sprint(f["app"], ":", f["level"]))
		}
	}
	return
}

// TestLevels: a building grows with its successful syncs of the last 30 days (1 → 2 at levelAt[0], 3 at
// levelAt[1]) and shrinks back as they age out; machines never level; failed syncs do not count.
func TestLevels(t *testing.T) {
	a, c := testApp(t)
	o, ac, now := &Out{app: a, name: "prom"}, newActivity(), time.Unix(1_800_000_000, 0)
	for i := 0; i < levelAt[0]-1; i++ {
		ac.levels(a, o, []string{"wird"}, now)
	}
	if got := evsOf(drain(c), "level"); len(got) != 0 {
		t.Fatalf("below levelAt[0]: %v", got)
	}
	ac.levels(a, o, []string{"wird", "keep-h1"}, now)
	if got := evsOf(drain(c), "level"); fmt.Sprint(got) != "[wird:2]" || a.State.Levels["wird"] != 2 {
		t.Fatalf("level 2: %v %v", got, a.State.Levels)
	}
	for i := levelAt[0]; i < levelAt[1]; i++ {
		ac.levels(a, o, []string{"wird"}, now.Add(time.Hour))
	}
	if got := evsOf(drain(c), "level"); fmt.Sprint(got) != "[wird:3]" {
		t.Fatalf("level 3: %v", got)
	}
	ac.levels(a, o, nil, now.Add(activityWindow+time.Minute)) // the first 4 aged out: 8 left
	if got := evsOf(drain(c), "level"); fmt.Sprint(got) != "[wird:2]" {
		t.Fatalf("decay to 2: %v", got)
	}
	ac.levels(a, o, nil, now.Add(activityWindow+2*time.Hour)) // all aged out
	if got := evsOf(drain(c), "level"); fmt.Sprint(got) != "[wird:1]" || len(a.State.Levels) != 0 {
		t.Fatalf("decay to 1: %v %v", got, a.State.Levels)
	}
	for i := 0; i < 20; i++ {
		ac.levels(a, o, nil, now.Add(activityWindow+3*time.Hour)) // steady: no frame
	}
	if got := evsOf(drain(c), "level"); len(got) != 0 {
		t.Fatalf("steady state sent %v", got)
	}
}

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
}

// TestTechs: the ADR listing → numbers only (count, highest); other files, product slugs never leave; an unchanged
// listing sends nothing; a failure is not fatal.
func TestTechs(t *testing.T) {
	a, c := testApp(t)
	list := `[{"name":"0001-ingress-traefik.md"},{"name":"0064-mountain-town-roles.md"},{"name":"0065-mountain-behind-forwardauth.md"},{"name":"README.md"},{"name":"template.md"}]`
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "nope", http.StatusNotFound)
			return
		}
		if r.URL.Path != "/repos/MohammadBnei/infra-bootstrap/contents/docs/adr" {
			t.Errorf("path %s", r.URL.Path)
		}
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
	if f := drain(c); len(f) != 0 {
		t.Fatalf("unchanged listing sent %v", f)
	}
	fail = true
	s.techs(context.Background(), o)
	if f := drain(c); len(f) != 0 || s.techErr == "" {
		t.Fatalf("failure: %v %q", f, s.techErr)
	}
}

// TestDBCivRestore: levels are seeded from the stored syncs after a restart (no reset); calm is seeded from the
// stored events when the snapshot has none; hello.snapshot.started is the town's first day.
func TestDBCivRestore(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	a := storedApp(t, dsn)
	o := &Out{app: a, name: "prom"}
	for i := 0; i < levelAt[0]; i++ {
		o.Ev("argo.sync", map[string]any{"app": "wird", "ok": true})
	}
	o.Ev("argo.sync", map[string]any{"app": "mountain", "ok": false})
	a.store.Close(5 * time.Second)
	b := storedApp(t, dsn)
	defer b.store.Close(time.Second)
	if b.snap(b.Day).Started != utcDay(time.Now()) {
		t.Fatalf("started: %q", b.snap(b.Day).Started)
	}
	if b.State.Calm == "" {
		t.Fatal("a calm town restored with no calm day")
	}
	ac, ob := newActivity(), &Out{app: b, name: "prom"}
	ac.levels(b, ob, nil, time.Now())
	if b.State.Levels["wird"] != 2 || b.State.Levels["mountain"] != 0 {
		t.Fatalf("levels after a restart: %v", b.State.Levels)
	}
	c, _ := pgx.Connect(ctx, dsn)
	defer c.Close(ctx)
	var n int
	c.QueryRow(ctx, "SELECT count(*) FROM events WHERE kind = 'level'").Scan(&n)
	if n != 1 {
		t.Fatalf("level events stored: %d", n)
	}
}

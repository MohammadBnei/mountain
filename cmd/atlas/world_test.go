package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
)

type outRow struct{ CE, Type, Subject, Cause string }

// outbox is the outbox in id order.
func outbox(t *testing.T, pool *pgxpool.Pool) []outRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT ce_id, type, subject, coalesce(data->>'cause', '') FROM atlas_outbox ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []outRow
	for rows.Next() {
		var r outRow
		if err := rows.Scan(&r.CE, &r.Type, &r.Subject, &r.Cause); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func types(rs []outRow) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Type[len("dev.bnei.world."):]+" "+r.Subject)
	}
	return out
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

// TestDBWorldOutbox: a fresh database writes one history.rebased and no per-entity row; a second fresh database one
// of its own, with another epoch (so another ce_id); a later projection writes the diff in canonical order (bond.removed,
// entity.retired, entity.created, bond.added, each by key); a changed hash is no event; an extractor bump is one rebased
// row (cause extractor); a repo added is one (cause sources).
func TestDBWorldOutbox(t *testing.T) {
	ctx := context.Background()
	pool := atlasPool(t)
	repos := extract.Repos{"infra-bootstrap": {Branch: "main", SHA: "s1"}}
	ab := graph.Edge{From: "app:a", To: "app:b", Kind: graph.EdgeGit, Origin: graph.Declared}
	ac := graph.Edge{From: "app:a", To: "app:c", Kind: graph.EdgeGit, Origin: graph.Declared}
	bc := graph.Edge{From: "app:b", To: "app:c", Kind: graph.EdgeGit, Origin: graph.Declared}
	project := func(res extract.Result, repos extract.Repos) int64 {
		t.Helper()
		seq, _, err := projectGraph(ctx, pool, res, repos, nil)
		if err != nil {
			t.Fatal(err)
		}
		return seq
	}
	seq1 := project(atlasResult([]graph.Node{n("app:a", "1", "s1"), n("app:b", "1", "s1")}, []graph.Edge{ab}), repos)
	o := outbox(t, pool)
	var epoch int64
	if err := pool.QueryRow(ctx, "SELECT epoch FROM atlas_epoch").Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if len(o) != 1 || o[0].Type != typeRebased || o[0].Subject != "world/" || o[0].Cause != "fresh" || o[0].CE != fmt.Sprintf("rebased:%d.%d", epoch, seq1) {
		t.Fatalf("a fresh database: %+v (epoch %d seq %d)", o, epoch, seq1)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM atlas_outbox"); err != nil {
		t.Fatal(err)
	}

	// b changes (a hash change: no event), c is born with two bonds (a->c, b->c); b->... stays; a->b is dropped
	seq2 := project(atlasResult([]graph.Node{n("app:a", "1", "s2"), n("app:b", "2", "s2"), n("app:c", "1", "s2")}, []graph.Edge{ac, bc}), repos)
	o = outbox(t, pool)
	eq(t, "second projection", types(o), []string{"bond.removed world/app:a", "entity.created world/app:c", "bond.added world/app:a", "bond.added world/app:b"})
	ver := fmt.Sprintf("%d.%d", epoch, seq2)
	if o[1].CE != "app:c:created:"+ver || !strings.HasSuffix(o[0].CE, ":removed:"+ver) || len(o[0].CE) != 16+len(":removed:"+ver) || o[0].Cause != "repo" {
		t.Errorf("ce_ids: %+v", o)
	}
	var body struct{ Body struct{ From, To string } }
	if err := pool.QueryRow(ctx, "SELECT data FROM atlas_outbox WHERE type = $1", typeRemoved).Scan(&body); err != nil || body.Body.To != "app:b" {
		t.Errorf("removed bond body: %+v %v (the closed version's)", body, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM atlas_outbox"); err != nil {
		t.Fatal(err)
	}

	// a retires, and the extractor bump alone is one rebased
	seq3 := project(atlasResult([]graph.Node{n("app:b", "2", "s2"), n("app:c", "1", "s2")}, []graph.Edge{bc}), repos)
	eq(t, "retire", types(outbox(t, pool)), []string{"bond.removed world/app:a", "entity.retired world/app:a"})
	var retired struct{ Body graph.Node }
	if err := pool.QueryRow(ctx, "SELECT data FROM atlas_outbox WHERE type = $1", typeRetired).Scan(&retired); err != nil || retired.Body.ID != "app:a" {
		t.Errorf("retired body: %+v %v", retired, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM atlas_outbox"); err != nil {
		t.Fatal(err)
	}
	bump := atlasResult([]graph.Node{n("app:b", "2", "s2"), n("app:c", "1", "s2")}, []graph.Edge{bc})
	bump.Versions = map[string]int{"gitops": 2}
	seq4 := project(bump, repos)
	o = outbox(t, pool)
	if len(o) != 1 || o[0].Type != typeRebased || o[0].Cause != "extractor" || o[0].CE != fmt.Sprintf("rebased:%d.%d", epoch, seq4) || seq4 <= seq3 {
		t.Errorf("an extractor bump: %+v", o)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM atlas_outbox"); err != nil {
		t.Fatal(err)
	}
	// a moved sha only is a diff with nothing in it; a repo added is a rebase (sources)
	project(bump, extract.Repos{"infra-bootstrap": {Branch: "main", SHA: "s9"}})
	if o := outbox(t, pool); len(o) != 0 {
		t.Errorf("a moved sha: %+v", o)
	}
	project(bump, extract.Repos{"infra-bootstrap": {Branch: "main", SHA: "s9"}, "agent-fleet": {Branch: "main", SHA: "a"}})
	if o := outbox(t, pool); len(o) != 1 || o[0].Cause != "sources" {
		t.Errorf("a repo added: %+v", o)
	}

	// a second fresh database: its own rebased, with another id
	pool2 := atlasPool(t)
	if _, _, err := projectGraph(ctx, pool2, atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil), repos, nil); err != nil {
		t.Fatal(err)
	}
	o2 := outbox(t, pool2)
	if len(o2) != 1 || o2[0].Cause != "fresh" || strings.HasPrefix(o2[0].CE, fmt.Sprintf("rebased:%d.", epoch)) {
		t.Errorf("a second fresh database: %+v, first epoch %d", o2, epoch)
	}
}

// TestDBWorldBaseline: migration 002 on an existing database writes its epoch (the first projection's) and one
// baseline rebased row for the latest projection.
func TestDBWorldBaseline(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	old, _ := migrations.ReadFile("migrations/001_atlas.sql")
	for _, q := range []string{string(old), "CREATE TABLE atlas_migrations (version text PRIMARY KEY, at timestamptz NOT NULL DEFAULT now())",
		"INSERT INTO atlas_migrations (version) VALUES ('001_atlas.sql')",
		"INSERT INTO atlas_projection (at, extractors, tracked) VALUES ('2026-01-01T00:00:00Z', '{}', '[]'), ('2026-01-02T00:00:00Z', '{}', '[]')",
		"INSERT INTO atlas_drift (seq, body) VALUES (2, '[]')",
		"INSERT INTO atlas_vector (seq, repo, branch, sha) VALUES (2, 'infra-bootstrap', 'main', 'abc')"} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	var got int64
	if err := pool.QueryRow(ctx, "SELECT epoch FROM atlas_epoch").Scan(&got); err != nil || got != epoch {
		t.Errorf("epoch %d %v, want %d", got, err, epoch)
	}
	o := outbox(t, pool)
	if len(o) != 1 || o[0].Cause != "baseline" || o[0].CE != fmt.Sprintf("rebased:%d.2", epoch) {
		t.Errorf("baseline: %+v", o)
	}
	var v map[string]string
	if err := pool.QueryRow(ctx, "SELECT data->'vector' FROM atlas_outbox").Scan(&v); err != nil || v["infra-bootstrap"] != "main@abc" {
		t.Errorf("baseline vector: %v %v", v, err)
	}
}

// TestDBWorldPublish: the outbox drains in id order. A duplicate is deleted and not counted; InvalidArgument is parked
// (deleted, counted refused) and the drain goes on; Unavailable and PermissionDenied stop it with the row kept, and
// the next round sends that same row first.
func TestDBWorldPublish(t *testing.T) {
	ctx := context.Background()
	pool := atlasPool(t)
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	for _, id := range []string{"ok", "dup", "bad", "down", "after"} {
		if _, err := pool.Exec(ctx, "INSERT INTO atlas_outbox (ce_id, type, subject, data, time) VALUES ($1, $2, 'world/app:x', $3, '2026-01-01T00:00:00Z')",
			id, typeCreated, `{"k":"`+id+`"}`); err != nil {
			t.Fatal(err)
		}
	}
	var sent []string
	var downErr error = connect.NewError(connect.CodeUnavailable, errors.New("bridge down"))
	publish := func(_ context.Context, ev *bridgev1.CloudEvent) (bool, error) {
		sent = append(sent, ev.Id)
		if ev.Time.AsTime() != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) || ev.Subject != "world/app:x" || string(ev.Data) != `{"k": "`+ev.Id+`"}` {
			t.Errorf("event %+v", ev)
		}
		switch ev.Id {
		case "dup":
			return true, nil
		case "bad":
			return false, connect.NewError(connect.CodeInvalidArgument, errors.New("keyFields"))
		case "down":
			return false, downErr
		}
		return false, nil
	}
	n, err := drainOutbox(ctx, pool, publish, m)
	if n != 3 || connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("first drain: %d %v", n, err)
	}
	if left := outbox(t, pool); len(left) != 2 || left[0].CE != "down" || left[1].CE != "after" {
		t.Errorf("left: %+v", left)
	}
	downErr = connect.NewError(connect.CodePermissionDenied, errors.New("no grant"))
	if n, err := drainOutbox(ctx, pool, publish, m); n != 0 || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("a refused grant: %d %v", n, err)
	}
	downErr = nil
	if n, err := drainOutbox(ctx, pool, publish, m); n != 2 || err != nil {
		t.Fatalf("recovered: %d %v", n, err)
	}
	eq(t, "sent", sent, []string{"ok", "dup", "bad", "down", "down", "down", "after"})
	_, _, body := scrape(t, m)
	for _, line := range []string{"mountain_world_events_published_total{type=\"dev.bnei.world.entity.created\"} 3\n", // ok, down, after; not the duplicate
		"mountain_world_events_refused_total 1\n"} {
		if !strings.Contains(body, line) {
			t.Errorf("missing %q in\n%s", line, body)
		}
	}

	// tick: an empty outbox is probed once (its answer renews the health series), and the age gauge reads 0
	probes := 0
	probe := func(_ context.Context, ev *bridgev1.CloudEvent) (bool, error) {
		probes++
		return false, connect.NewError(connect.CodeInvalidArgument, errors.New("keyFields"))
	}
	if err := tick(ctx, pool, probe, m); err != nil || probes != 1 {
		t.Fatalf("tick: %v, %d probes", err, probes)
	}
	_, _, body = scrape(t, m)
	if !strings.Contains(body, `mountain_source_last_success{source="world"}`) || !strings.Contains(body, "mountain_world_outbox_oldest_seconds 0\n") {
		t.Errorf("health and age:\n%s", body)
	}
	// a stuck row ages; PermissionDenied does not renew the health series' stand-in (no probe while rows wait)
	if _, err := pool.Exec(ctx, "INSERT INTO atlas_outbox (at, ce_id, type, subject, data, time) VALUES (now() - interval '1 hour', 'old', $1, 's', '{}', now())", typeCreated); err != nil {
		t.Fatal(err)
	}
	denied := func(context.Context, *bridgev1.CloudEvent) (bool, error) {
		return false, connect.NewError(connect.CodePermissionDenied, errors.New("no grant"))
	}
	if err := tick(ctx, pool, denied, m); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("tick: %v", err)
	}
	if _, _, body := scrape(t, m); !strings.Contains(body, "mountain_world_outbox_oldest_seconds 3600") {
		t.Errorf("a one-hour-old row:\n%s", body)
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// testDSN is a fresh schema on MOUNTAIN_TEST_DSN (TestDB* skip without it).
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

func atlasPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func atlasResult(nodes []graph.Node, edges []graph.Edge, drift ...graph.Drift) extract.Result {
	return extract.Result{Graph: graph.Graph{Nodes: nodes, Edges: edges, Drift: drift}, Versions: map[string]int{"gitops": 1},
		Tracked: []extract.Track{{Repo: "infra-bootstrap", Branch: "main"}}}
}

func n(id, v, sha string) graph.Node {
	k, _, _ := strings.Cut(id, ":")
	return graph.Node{ID: id, Kind: k, Attrs: map[string]string{"v": v}, Source: graph.Source{Repo: "infra-bootstrap", Path: "x.yaml", SHA: sha}}
}

func rows(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var c int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestDBMigrate: the series drops the town's tables (and A1's atlas tables, rebuilt by the next poll) and applies
// once; a second run is a no-op.
func TestDBMigrate(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, q := range []string{"CREATE SEQUENCE frame_seq", "CREATE TABLE events (seq bigint)", "CREATE TABLE marks (id text)",
		"CREATE TABLE schema_migrations (version text)", "CREATE TABLE atlas_projection (seq bigserial PRIMARY KEY)"} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool); err != nil {
		t.Fatal("second run:", err)
	}
	if c := rows(t, pool, "SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name IN ('events', 'marks', 'schema_migrations')"); c != 0 {
		t.Errorf("%d town tables left", c)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_migrations"); c != 1 {
		t.Errorf("atlas_migrations: %d", c)
	}
	if c := rows(t, pool, "SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'atlas_projection' AND column_name = 'tracked'"); c != 1 {
		t.Error("atlas_projection was not rebuilt")
	}
}

// TestDBAtlasProject: projecting the same graph twice writes nothing; a new sha alone is a new projection with no new
// versions; a changed, a gone and a new node close and open exactly those rows; an old seq still reads as it was.
func TestDBAtlasProject(t *testing.T) {
	pool := atlasPool(t)
	ctx := context.Background()
	e := graph.Edge{From: "app:a", To: "app:b", Kind: graph.EdgeGit, Origin: graph.Declared}
	repos := extract.Repos{"infra-bootstrap": {SHA: strings.Repeat("1", 40)}}
	r1 := atlasResult([]graph.Node{n("app:a", "1", "s1"), n("app:b", "1", "s1")}, []graph.Edge{e})
	seq1, changed, err := projectGraph(ctx, pool, r1, repos, nil)
	if err != nil || !changed || seq1 == 0 {
		t.Fatalf("first: %d %v %v", seq1, changed, err)
	}
	if seq, changed, err := projectGraph(ctx, pool, r1, repos, nil); err != nil || changed || seq != seq1 {
		t.Fatalf("same again: %d %v %v", seq, changed, err)
	}
	repos2 := extract.Repos{"infra-bootstrap": {SHA: strings.Repeat("2", 40)}}
	r1b := atlasResult([]graph.Node{n("app:a", "1", "s2"), n("app:b", "1", "s2")}, []graph.Edge{e})
	seq2, changed, err := projectGraph(ctx, pool, r1b, repos2, nil)
	if err != nil || !changed || seq2 <= seq1 {
		t.Fatalf("new sha: %d %v %v", seq2, changed, err)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_node_ver"); c != 2 {
		t.Errorf("a sha-only change opened versions: %d rows", c)
	}
	r3 := atlasResult([]graph.Node{n("app:b", "2", "s3"), n("app:c", "1", "s3")}, nil,
		graph.Drift{Kind: graph.DriftMirror, Subject: "app:c", Detail: "d"})
	seq3, changed, err := projectGraph(ctx, pool, r3, repos2, nil)
	if err != nil || !changed {
		t.Fatalf("change: %d %v %v", seq3, changed, err)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_node_ver WHERE valid_to = $1", seq3); c != 2 {
		t.Errorf("closed at seq3: %d, want a and the old b", c)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_edge_ver WHERE valid_to = $1", seq3); c != 1 {
		t.Errorf("edge not closed: %d", c)
	}
	r4 := atlasResult([]graph.Node{n("app:b", "2", "s3")}, nil)
	if _, _, err := projectGraph(ctx, pool, r4, extract.Repos{"other": {SHA: "o"}}, nil); !errors.Is(err, errLostRepos) {
		t.Fatalf("a lost repo: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM atlas_projection WHERE seq = $1", seq1); err == nil {
		t.Error("deleting a projection that versions start at succeeded")
	}
	old, err := loadGraph(ctx, pool, seq1)
	if err != nil || len(old.Nodes) != 2 || len(old.Edges) != 1 || old.Vector["infra-bootstrap"].SHA != strings.Repeat("1", 40) {
		t.Fatalf("seq1 as it was: %+v %v", old, err)
	}
	cur, err := loadGraph(ctx, pool, 0)
	if err != nil || cur.Seq != seq3 || len(cur.Nodes) != 2 || len(cur.Edges) != 0 || !strings.Contains(string(cur.Drift), "app:c") {
		t.Fatalf("latest: %+v %v", cur, err)
	}
	if _, err := loadGraph(ctx, pool, seq3+10); err != errNoProjection {
		t.Errorf("a seq that doesn't exist: %v", err)
	}
}

// TestGraphsMemory: without a database the latest projection lives in memory: the same result is no change, a new
// one is seq+1, a lost repo is refused, and GET /api/graph serves it (an older seq is a 404: no history).
func TestGraphsMemory(t *testing.T) {
	a := testApp(t)
	a.gate.devOpen = true
	if r := a.serveReq("GET", "/api/graph", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("no projection: %d", r.StatusCode)
	}
	ctx := context.Background()
	repos := extract.Repos{"infra-bootstrap": {SHA: "s1"}}
	res := atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil)
	if seq, changed, err := a.Graph.put(ctx, res, repos, nil); seq != 1 || !changed || err != nil {
		t.Fatalf("first: %d %v %v", seq, changed, err)
	}
	if seq, changed, err := a.Graph.put(ctx, res, repos, nil); seq != 1 || changed || err != nil {
		t.Fatalf("same: %d %v %v", seq, changed, err)
	}
	res2 := atlasResult([]graph.Node{n("app:a", "2", "s1")}, nil)
	if seq, changed, err := a.Graph.put(ctx, res2, repos, nil); seq != 2 || !changed || err != nil {
		t.Fatalf("changed: %d %v %v", seq, changed, err)
	}
	if _, _, err := a.Graph.put(ctx, res2, extract.Repos{}, nil); !errors.Is(err, errLostRepos) {
		t.Errorf("lost repo: %v", err)
	}
	if _, _, err := a.Graph.put(ctx, res2, extract.Repos{}, map[string]bool{"agent-fleet": true}); err != nil { // dropped from the sources
		t.Errorf("a repo no longer a source was refused as lost: %v", err)
	}
	a.Graph.put(ctx, res2, repos, nil)
	var v atlasView
	r := a.serveReq("GET", "/api/graph", nil)
	if r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&v) != nil || v.Seq != a.Graph.Latest().Seq || len(v.Nodes) != 1 || r.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("serve: %d %+v", r.StatusCode, v)
	}
	if r := a.serveReq("GET", "/api/graph?seq=1", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("seq=1 without history: %d", r.StatusCode)
	}
	if r := a.serveReq("GET", "/api/graph?seq=-1", nil); r.StatusCode != http.StatusBadRequest {
		t.Errorf("bad seq: %d", r.StatusCode)
	}
}

// TestDBGraphsServe: with a database, the latest is served from memory and an older seq from the store; a restart
// loads the latest back.
func TestDBGraphsServe(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	a := testApp(t)
	a.gate.devOpen = true
	a.Graph = openGraphs(ctx, dsn)
	t.Cleanup(a.Graph.close)
	if a.Graph.db() == nil {
		t.Fatal("no pool")
	}
	repos := extract.Repos{"infra-bootstrap": {SHA: "s1"}}
	s1, _, err := a.Graph.put(ctx, atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil), repos, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := a.Graph.put(ctx, atlasResult([]graph.Node{n("app:a", "2", "s1"), n("app:b", "1", "s1")}, nil), repos, nil)
	if err != nil || a.Graph.Latest().Seq != s2 || len(a.Graph.Latest().Nodes) != 2 {
		t.Fatalf("latest: %v %+v", err, a.Graph.Latest())
	}
	var v atlasView
	if r := a.serveReq("GET", fmt.Sprintf("/api/graph?seq=%d", s1), nil); r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&v) != nil || len(v.Nodes) != 1 {
		t.Fatalf("old seq: %d %+v", r.StatusCode, v)
	}
	// atlas project stores a newer projection behind the service's back (--drop-repos): the service's next round,
	// unchanged against the store, must still serve the store's latest, not its own older copy
	other := &Graphs{}
	if err := other.attach(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	s3, _, err := other.put(ctx, atlasResult([]graph.Node{n("app:c", "1", "s1")}, nil), extract.Repos{"infra-bootstrap": {SHA: "s1"}}, map[string]bool{})
	other.close()
	if err != nil || s3 <= s2 {
		t.Fatalf("stored behind the service: %d %v", s3, err)
	}
	if seq, _, err := a.Graph.put(ctx, atlasResult([]graph.Node{n("app:c", "1", "s1")}, nil), repos, nil); err != nil || seq != s3 || a.Graph.Latest().Seq != s3 {
		t.Errorf("the service kept its own copy: put %d, latest %d, %v", seq, a.Graph.Latest().Seq, err)
	}
	again := openGraphs(ctx, dsn)
	defer again.close()
	if again.Latest() == nil || again.Latest().Seq != s3 || again.index().NSApps == nil {
		t.Errorf("restart: %+v", again.Latest())
	}
}

// TestDBAtlasReal projects the real checkouts through the projector's own round twice when ATLAS_REPOS is set
// (locally: /home/user): the second run writes nothing, and the latest projection reads back whole.
func TestDBAtlasReal(t *testing.T) {
	dir := os.Getenv("ATLAS_REPOS")
	if dir == "" {
		t.Skip("ATLAS_REPOS unset")
	}
	pool := atlasPool(t)
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, repos, err := round(ctx, localHeads(dir), rl, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seq, changed, err := projectGraph(ctx, pool, res, repos, nil)
	if err != nil || !changed {
		t.Fatalf("first: %v %v", changed, err)
	}
	if again, changed, err := projectGraph(ctx, pool, res, repos, nil); err != nil || changed || again != seq {
		t.Fatalf("second run wrote: %d %v %v", again, changed, err)
	}
	v, err := loadGraph(ctx, pool, 0)
	if err != nil || len(v.Nodes) != len(res.Graph.Nodes) || len(v.Edges) != len(res.Graph.Edges) || len(repos) != len(rl.Sources) {
		t.Fatalf("read back %d nodes %d edges, projected %d %d, %d repos: %v", len(v.Nodes), len(v.Edges), len(res.Graph.Nodes), len(res.Graph.Edges), len(repos), err)
	}
}

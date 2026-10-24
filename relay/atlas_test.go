package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/internal/realms"
)

// TestAtlasRoutes: /api/graph answers a platform admin only: no sign-in (auth off), a visitor or a forged cookie get
// 403 before anything is read.
func TestAtlasRoutes(t *testing.T) {
	auth = nil
	if r := serveAuth("GET", "/api/graph", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("auth off: %d", r.StatusCode)
	}
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })
	visitor := session(t, p, "mountain-visitors")
	admin := session(t, p, "platform-admins")
	forged := &http.Cookie{Name: sessionCookie, Value: "eyJ2IjoiYWRtaW4iLCJleHAiOjk5OTk5OTk5OTl9.AAAA"}
	for name, cs := range map[string][]*http.Cookie{"anonymous": nil, "visitor": {visitor}, "forged": {forged}} {
		r := serveAuth("GET", "/api/graph", cs)
		if r.StatusCode != http.StatusForbidden || r.Header.Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: %d %q", name, r.StatusCode, r.Header.Get("Cache-Control"))
		}
	}
	saved := app
	app = nil
	t.Cleanup(func() { app = saved })
	if r := serveAuth("GET", "/api/graph", []*http.Cookie{admin}); r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("admin, no store: %d", r.StatusCode)
	}
	if r := serveAuth("GET", "/api/graph?seq=-1", []*http.Cookie{admin}); r.StatusCode != http.StatusBadRequest {
		t.Errorf("admin, bad seq: %d", r.StatusCode)
	}
}

func atlasPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrate(context.Background(), pool); err != nil { // 001..007 on a fresh schema
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

// TestDBAtlasProject: projecting the same graph twice writes nothing; a new sha alone is a new projection with no new
// versions; a changed, a gone and a new node close and open exactly those rows; an old seq still reads as it was.
func TestDBAtlasProject(t *testing.T) {
	pool := atlasPool(t)
	ctx := context.Background()
	e := graph.Edge{From: "app:a", To: "app:b", Kind: graph.EdgeGit, Origin: graph.Declared}
	repos := extract.Repos{"infra-bootstrap": {SHA: strings.Repeat("1", 40)}}
	r1 := atlasResult([]graph.Node{n("app:a", "1", "s1"), n("app:b", "1", "s1")}, []graph.Edge{e})
	seq1, changed, err := projectGraph(ctx, pool, r1, repos)
	if err != nil || !changed || seq1 == 0 {
		t.Fatalf("first: %d %v %v", seq1, changed, err)
	}
	if seq, changed, err := projectGraph(ctx, pool, r1, repos); err != nil || changed || seq != seq1 {
		t.Fatalf("same again: %d %v %v", seq, changed, err)
	}
	// a new commit that changes no fact: the vector moves, the versions don't
	repos2 := extract.Repos{"infra-bootstrap": {SHA: strings.Repeat("2", 40)}}
	r1b := atlasResult([]graph.Node{n("app:a", "1", "s2"), n("app:b", "1", "s2")}, []graph.Edge{e})
	seq2, changed, err := projectGraph(ctx, pool, r1b, repos2)
	if err != nil || !changed || seq2 <= seq1 {
		t.Fatalf("new sha: %d %v %v", seq2, changed, err)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_node_ver"); c != 2 {
		t.Errorf("a sha-only change opened versions: %d rows", c)
	}
	// b changes, a goes (with its edge), c arrives
	r3 := atlasResult([]graph.Node{n("app:b", "2", "s3"), n("app:c", "1", "s3")}, nil,
		graph.Drift{Kind: graph.DriftMirror, Subject: "app:c", Detail: "d"})
	seq3, changed, err := projectGraph(ctx, pool, r3, repos2)
	if err != nil || !changed {
		t.Fatalf("change: %d %v %v", seq3, changed, err)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_node_ver WHERE valid_to = $1", seq3); c != 2 {
		t.Errorf("closed at seq3: %d, want a and the old b", c)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_edge_ver WHERE valid_to = $1", seq3); c != 1 {
		t.Errorf("edge not closed: %d", c)
	}
	if c := rows(t, pool, "SELECT count(*) FROM atlas_node_ver WHERE valid_to IS NULL"); c != 2 {
		t.Errorf("current: %d, want b and c", c)
	}

	old, err := loadGraph(ctx, pool, seq1)
	if err != nil || len(old.Nodes) != 2 || len(old.Edges) != 1 || old.Vector["infra-bootstrap"] != strings.Repeat("1", 40) {
		t.Fatalf("seq1 as it was: %+v %v", old, err)
	}
	cur, err := loadGraph(ctx, pool, 0)
	if err != nil || cur.Seq != seq3 || len(cur.Nodes) != 2 || len(cur.Edges) != 0 || !strings.Contains(string(cur.Drift), "app:c") {
		t.Fatalf("latest: %+v %v", cur, err)
	}
	var b graph.Node
	json.Unmarshal(cur.Nodes[0], &b)
	if b.ID != "app:b" || b.Attrs["v"] != "2" {
		t.Errorf("latest b = %+v", b)
	}
	if _, err := loadGraph(ctx, pool, seq3+10); err != errNoProjection {
		t.Errorf("a seq that doesn't exist: %v", err)
	}
}

// TestDBAtlasServe: an admin reads the stored projection over HTTP; before any projection it is a 404.
func TestDBAtlasServe(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	saved := app
	app = a
	t.Cleanup(func() { app = saved })
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })
	admin := session(t, p, "platform-admins")
	if r := serveAuth("GET", "/api/graph", []*http.Cookie{admin}); r.StatusCode != http.StatusNotFound {
		t.Fatalf("no projection: %d", r.StatusCode)
	}
	res := atlasResult([]graph.Node{n("app:a", "1", "s")}, nil)
	if _, _, err := projectGraph(context.Background(), a.store.pool, res, extract.Repos{"infra-bootstrap": {SHA: "s"}}); err != nil {
		t.Fatal(err)
	}
	r := serveAuth("GET", "/api/graph", []*http.Cookie{admin})
	var v struct {
		Seq   int64
		Nodes []graph.Node
	}
	if r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&v) != nil || len(v.Nodes) != 1 || v.Nodes[0].ID != "app:a" {
		t.Fatalf("admin read: %d %+v", r.StatusCode, v)
	}
}

// TestDBAtlasReal projects the real checkouts twice when ATLAS_REPOS is set (locally: /home/user): the second run
// must write nothing, and the latest projection must read back whole.
func TestDBAtlasReal(t *testing.T) {
	dir := os.Getenv("ATLAS_REPOS")
	if dir == "" {
		t.Skip("ATLAS_REPOS unset")
	}
	pool := atlasPool(t)
	repos, err := extract.Checkouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Run(repos, rl, extract.All)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seq, changed, err := projectGraph(ctx, pool, res, repos)
	if err != nil || !changed {
		t.Fatalf("first: %v %v", changed, err)
	}
	if again, changed, err := projectGraph(ctx, pool, res, repos); err != nil || changed || again != seq {
		t.Fatalf("second run wrote: %d %v %v", again, changed, err)
	}
	v, err := loadGraph(ctx, pool, 0)
	if err != nil || len(v.Nodes) != len(res.Graph.Nodes) || len(v.Edges) != len(res.Graph.Edges) {
		t.Fatalf("read back %d nodes %d edges, projected %d %d: %v", len(v.Nodes), len(v.Edges), len(res.Graph.Nodes), len(res.Graph.Edges), err)
	}
}

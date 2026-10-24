package main

// atlas (R10 A1, plan/10-understanding.md): the projected system graph. The extractors (internal/extract) read repo
// checkouts; projectGraph stores what changed as node and edge versions; GET /api/graph serves one projection to a
// signed-in platform admin only. Nothing here touches the town: A2 replaces it.
//
//	relay atlas-project --repos <dir> [--dsn postgres://…]   (dsn defaults to DATABASE_URL)

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/internal/realms"
)

const atlasLock = 0x61746c61 // "atla": one projection at a time

// versionHash is a node or edge's identity across projections: its content without the sha it was read at, so a
// commit that changes nothing about a fact opens no new version of it.
func versionHash[T graph.Node | graph.Edge](v T) string {
	switch x := any(v).(type) {
	case graph.Node:
		x.Source.SHA = ""
		return graph.Hash(x)
	case graph.Edge:
		x.Source.SHA = ""
		return graph.Hash(x)
	}
	panic("unreachable")
}

// projectGraph stores res as the next projection: it closes the versions that changed or went away and opens the
// new ones, in one transaction. When nothing changed (graph, drift, vector) it writes nothing and returns the latest
// seq with changed=false.
func projectGraph(ctx context.Context, pool *pgxpool.Pool, res extract.Result, repos extract.Repos, allowLoss bool) (seq int64, changed bool, err error) {
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", atlasLock); err != nil {
			return err
		}
		curNodes, err := current(ctx, tx, "SELECT id, hash FROM atlas_node_ver WHERE valid_to IS NULL")
		if err != nil {
			return err
		}
		curEdges, err := current(ctx, tx, "SELECT key, hash FROM atlas_edge_ver WHERE valid_to IS NULL")
		if err != nil {
			return err
		}
		newNodes, newEdges := map[string]string{}, map[string]string{}
		for _, n := range res.Graph.Nodes {
			newNodes[n.ID] = versionHash(n)
		}
		for _, e := range res.Graph.Edges {
			newEdges[e.Key()] = versionHash(e)
		}
		vector := map[string]string{} // repo -> branch@sha
		for name, r := range repos {
			vector[name] = r.Branch + "@" + r.SHA
		}
		drift := mustJSON(res.Graph.Drift)
		var last int64
		var lastDrift []byte
		lastVector := map[string]string{}
		if err := tx.QueryRow(ctx, "SELECT p.seq, d.body FROM atlas_projection p JOIN atlas_drift d USING (seq) ORDER BY p.seq DESC LIMIT 1").Scan(&last, &lastDrift); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if last > 0 {
			rows, _ := tx.Query(ctx, "SELECT repo, branch, sha FROM atlas_vector WHERE seq = $1", last)
			var repo, branch, sha string
			if _, err := pgx.ForEachRow(rows, []any{&repo, &branch, &sha}, func() error { lastVector[repo] = branch + "@" + sha; return nil }); err != nil {
				return err
			}
			// A repo the last projection read and this one can't is a missing checkout, not a system without it:
			// storing it would close every version that repo holds, and reopen them all on the next full run.
			var lost []string
			for r := range lastVector {
				if _, ok := vector[r]; !ok {
					lost = append(lost, r)
				}
			}
			if len(lost) > 0 && !allowLoss {
				slices.Sort(lost)
				return fmt.Errorf("%w: %s", errLostRepos, strings.Join(lost, ", "))
			}
			if same(curNodes, newNodes) && same(curEdges, newEdges) && same(lastVector, vector) && jsonEqual(lastDrift, drift) {
				seq = last
				return nil
			}
		}
		changed = true
		if err := tx.QueryRow(ctx, "INSERT INTO atlas_projection (extractors, tracked, skipped) VALUES ($1, $2, $3) RETURNING seq",
			mustJSON(res.Versions), mustJSON(res.Tracked), mustJSON(nonNil(res.Skipped))).Scan(&seq); err != nil {
			return err
		}
		for name, r := range repos {
			if _, err := tx.Exec(ctx, "INSERT INTO atlas_vector (seq, repo, branch, sha) VALUES ($1, $2, $3, $4)", seq, name, r.Branch, r.SHA); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO atlas_drift (seq, body) VALUES ($1, $2)", seq, drift); err != nil {
			return err
		}
		for _, n := range res.Graph.Nodes {
			if err := version(ctx, tx, "atlas_node_ver", "id", n.ID, curNodes[n.ID], newNodes[n.ID], n, seq); err != nil {
				return err
			}
		}
		for _, e := range res.Graph.Edges {
			if err := version(ctx, tx, "atlas_edge_ver", "key", e.Key(), curEdges[e.Key()], newEdges[e.Key()], e, seq); err != nil {
				return err
			}
		}
		if err := closeGone(ctx, tx, "atlas_node_ver", "id", curNodes, newNodes, seq); err != nil {
			return err
		}
		return closeGone(ctx, tx, "atlas_edge_ver", "key", curEdges, newEdges, seq)
	})
	return seq, changed, err
}

func current(ctx context.Context, tx pgx.Tx, q string) (map[string]string, error) {
	out := map[string]string{}
	rows, _ := tx.Query(ctx, q)
	var k, h string
	_, err := pgx.ForEachRow(rows, []any{&k, &h}, func() error { out[k] = h; return nil })
	return out, err
}

// version opens a new version of one node or edge when its hash changed (closing the old one), or leaves it.
func version(ctx context.Context, tx pgx.Tx, table, col, key, old, hash string, body any, seq int64) error {
	if old == hash {
		return nil
	}
	if old != "" {
		if _, err := tx.Exec(ctx, "UPDATE "+table+" SET valid_to = $1 WHERE "+col+" = $2 AND valid_to IS NULL", seq, key); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, "INSERT INTO "+table+" ("+col+", hash, body, valid_from) VALUES ($1, $2, $3, $4)", key, hash, mustJSON(body), seq)
	return err
}

func closeGone(ctx context.Context, tx pgx.Tx, table, col string, cur, next map[string]string, seq int64) error {
	var gone []string
	for k := range cur {
		if _, ok := next[k]; !ok {
			gone = append(gone, k)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "UPDATE "+table+" SET valid_to = $1 WHERE "+col+" = ANY($2) AND valid_to IS NULL", seq, gone)
	return err
}

func same(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// jsonEqual compares a stored jsonb (re-encoded by Postgres) with fresh JSON by value.
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return string(mustJSON(x)) == string(mustJSON(y))
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type vectorEntry struct {
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

// atlasView is GET /api/graph's answer: one projection, whole.
type atlasView struct {
	Seq        int64                  `json:"seq"`
	At         time.Time              `json:"at"`
	Extractors map[string]int         `json:"extractors"`
	Tracked    []extract.Track        `json:"tracked"`
	Skipped    []string               `json:"skipped"`
	Vector     map[string]vectorEntry `json:"vector"`
	Nodes      []json.RawMessage      `json:"nodes"`
	Edges      []json.RawMessage      `json:"edges"`
	Drift      json.RawMessage        `json:"drift"`
}

var (
	errNoProjection = errors.New("atlas: no projection yet")
	errLostRepos    = errors.New("atlas: repos the last projection read are not checked out (pass --drop-repos if they are really gone)")
)

// loadGraph reads projection seq (0: the latest) as it stood.
func loadGraph(ctx context.Context, pool *pgxpool.Pool, seq int64) (atlasView, error) {
	v := atlasView{Vector: map[string]vectorEntry{}, Nodes: []json.RawMessage{}, Edges: []json.RawMessage{}}
	q := "SELECT p.seq, p.at, p.extractors, p.tracked, p.skipped, d.body FROM atlas_projection p JOIN atlas_drift d USING (seq) WHERE p.seq = $1"
	if seq == 0 {
		q = "SELECT p.seq, p.at, p.extractors, p.tracked, p.skipped, d.body FROM atlas_projection p JOIN atlas_drift d USING (seq) ORDER BY p.seq DESC LIMIT 1"
	}
	args := []any{}
	if seq != 0 {
		args = append(args, seq)
	}
	var ext, tracked, skipped []byte
	if err := pool.QueryRow(ctx, q, args...).Scan(&v.Seq, &v.At, &ext, &tracked, &skipped, &v.Drift); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return v, errNoProjection
		}
		return v, err
	}
	if err := errors.Join(json.Unmarshal(ext, &v.Extractors), json.Unmarshal(tracked, &v.Tracked), json.Unmarshal(skipped, &v.Skipped)); err != nil {
		return v, err
	}
	rows, _ := pool.Query(ctx, "SELECT repo, branch, sha FROM atlas_vector WHERE seq = $1", v.Seq)
	var repo, branch, sha string
	if _, err := pgx.ForEachRow(rows, []any{&repo, &branch, &sha}, func() error { v.Vector[repo] = vectorEntry{branch, sha}; return nil }); err != nil {
		return v, err
	}
	for _, t := range []struct {
		table, col string
		out        *[]json.RawMessage
	}{{"atlas_node_ver", "id", &v.Nodes}, {"atlas_edge_ver", "key", &v.Edges}} {
		rows, _ := pool.Query(ctx, "SELECT body FROM "+t.table+" WHERE valid_from <= $1 AND (valid_to IS NULL OR valid_to > $1) ORDER BY "+t.col, v.Seq)
		var b []byte
		if _, err := pgx.ForEachRow(rows, []any{&b}, func() error { *t.out = append(*t.out, append(json.RawMessage(nil), b...)); return nil }); err != nil {
			return v, err
		}
	}
	return v, nil
}

// serveGraph is GET /api/graph[?seq=N]: a platform admin's only (real names, R10 Audience row). Every other caller,
// including any caller while sign-in is off, gets 403.
func serveGraph(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	if s, _ := sessionOf(r); s.Role != roleAdmin {
		http.Error(w, "platform admins only", http.StatusForbidden)
		return
	}
	var seq int64
	if q := r.URL.Query().Get("seq"); q != "" {
		n, err := strconv.ParseInt(q, 10, 64)
		if err != nil || n <= 0 {
			http.Error(w, "seq is a positive integer", http.StatusBadRequest)
			return
		}
		seq = n
	}
	if app == nil || app.store == nil {
		http.Error(w, "atlas: no store", http.StatusServiceUnavailable)
		return
	}
	v, err := loadGraph(r.Context(), app.store.pool, seq)
	switch {
	case errors.Is(err, errNoProjection):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		log.Printf("atlas: %v", err)
		http.Error(w, "atlas: read failed", http.StatusInternalServerError)
		return
	}
	h.Set("Content-Type", "application/json")
	w.Write(mustJSON(v))
}

func init() { Public.HandleFunc("GET /api/graph", serveGraph) }

// atlasProject is `relay atlas-project`: run the extractors over checkouts and store the projection.
func atlasProject(args []string) error {
	f := flag.NewFlagSet("atlas-project", flag.ContinueOnError)
	dir := f.String("repos", "", "directory holding the repo checkouts (one per GitHub repo name)")
	dsn := f.String("dsn", os.Getenv("DATABASE_URL"), "Postgres URL (default: DATABASE_URL)")
	drop := f.Bool("drop-repos", false, "accept that repos the last projection read are gone (their nodes close)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dsn == "" {
		return errors.New("atlas-project: --repos and a DSN (--dsn or DATABASE_URL) are required")
	}
	repos, err := extract.Checkouts(*dir)
	if err != nil {
		return err
	}
	rl, err := realms.Load()
	if err != nil {
		return err
	}
	res, err := extract.Run(repos, rl, extract.All)
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		return err
	}
	seq, changed, err := projectGraph(ctx, pool, res, repos, *drop)
	if err != nil {
		return err
	}
	fmt.Printf("atlas: projection %d (changed: %v): %d repos, %d nodes, %d edges, %d drift, skipped %v\n",
		seq, changed, len(repos), len(res.Graph.Nodes), len(res.Graph.Edges), len(res.Graph.Drift), res.Skipped)
	return nil
}

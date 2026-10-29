package main

// The projected graph (plan/10 "Projection", "Read side"). The extractors (internal/extract) read repo checkouts;
// projectGraph stores what changed as node and edge versions; GET /api/graph serves one projection to a platform
// admin. Graphs stores projections through two seams (projections and history, below), served by one of two stores:
// Postgres keeps every one; without a database atlas keeps the latest in memory only, so the page works and history
// does not.

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/ukubi-go/errs"
)

//go:embed migrations/*.sql
var migrations embed.FS

const migrateLock = 0x61746d67 // "atmg"

// migrate applies migrations/*.sql in name order, once each, under a session advisory lock (a second atlas or a
// test waits instead of racing the DDL). Tracked in atlas_migrations: the relay's schema_migrations is the town's.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	if _, err := c.Exec(ctx, "SELECT pg_advisory_lock($1)", migrateLock); err != nil {
		return err
	}
	defer func() { _, _ = c.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrateLock) }()
	if _, err := c.Exec(ctx, "CREATE TABLE IF NOT EXISTS atlas_migrations (version text PRIMARY KEY, at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	for _, f := range files {
		v := path.Base(f)
		var done bool
		if err := c.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM atlas_migrations WHERE version = $1)", v).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, _ := migrations.ReadFile(f)
		err := pgx.BeginFunc(ctx, c, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO atlas_migrations (version) VALUES ($1)", v)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", v, err)
		}
		slog.Info(fmt.Sprintf("db: applied migration %s", v))
	}
	return nil
}

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
// new ones, in one transaction. When nothing changed (graph, drift, vector, versions) it writes nothing and returns the latest
// seq with changed=false.
// counted is the repos whose absence from a round is a loss (a checkout that could not be read): nil = every repo
// the last projection read, an empty set = none (atlas project --drop-repos). A repo dropped from realms.yaml sources
// is not in the service's set, so taking it out is a change, not a refusal.
func projectGraph(ctx context.Context, pool *pgxpool.Pool, res extract.Result, repos extract.Repos, counted map[string]bool) (seq int64, changed bool, err error) {
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
		var lastDrift, lastVersions []byte
		lastVector := map[string]string{}
		if err := tx.QueryRow(ctx, "SELECT p.seq, p.extractors, d.body FROM atlas_projection p JOIN atlas_drift d USING (seq) ORDER BY p.seq DESC LIMIT 1").Scan(&last, &lastVersions, &lastDrift); err != nil && !errors.Is(err, pgx.ErrNoRows) {
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
			if lost := lostRepos(lastVector, vector, counted); len(lost) > 0 {
				return fmt.Errorf("%w: %s", errLostRepos, strings.Join(lost, ", "))
			}
			// the versions too: a projection records what made it, so new pass or extractor logic is a new projection
			// even when its graph is the same
			if same(curNodes, newNodes) && same(curEdges, newEdges) && same(lastVector, vector) && jsonEqual(lastDrift, drift) &&
				jsonEqual(lastVersions, mustJSON(res.Versions)) {
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

// lostRepos are the repos last read that this round lacks and that count as lost, sorted.
func lostRepos[V any](last map[string]V, now map[string]string, counted map[string]bool) []string {
	var lost []string
	for r := range last {
		if _, ok := now[r]; !ok && (counted == nil || counted[r]) {
			lost = append(lost, r)
		}
	}
	slices.Sort(lost)
	return lost
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

// viewOf is res as GET /api/graph would serve it, for the memory-only store (no history: seq counts changes since
// start).
func viewOf(seq int64, res extract.Result, repos extract.Repos) atlasView {
	v := atlasView{Seq: seq, At: time.Now().UTC(), Extractors: res.Versions, Tracked: res.Tracked, Skipped: nonNil(res.Skipped),
		Vector: map[string]vectorEntry{}, Nodes: []json.RawMessage{}, Edges: []json.RawMessage{}, Drift: mustJSON(nonNilDrift(res.Graph.Drift))}
	for name, r := range repos {
		v.Vector[name] = vectorEntry{r.Branch, r.SHA}
	}
	for _, n := range res.Graph.Nodes {
		v.Nodes = append(v.Nodes, mustJSON(n))
	}
	for _, e := range res.Graph.Edges {
		v.Edges = append(v.Edges, mustJSON(e))
	}
	return v
}

func nonNilDrift(d []graph.Drift) []graph.Drift {
	if d == nil {
		return []graph.Drift{}
	}
	return d
}

// Graphs is the read side: the store (the database when there is one, else memory), and always the latest projection
// in memory (the hello, the live overlay's index and GET /api/graph without ?seq read it there).
type Graphs struct {
	mu     sync.Mutex
	pg     *pgStore   // nil = memory-only (or the database not reached yet: attach keeps trying)
	mem    memStore   // the store until a database is attached
	latest *atlasView // nil = no projection yet
	idx    *GraphIndex
	met    *Metrics // nil = not recorded
}

// The store seams (plan/15 §1), each owned by its consumer: Graphs.put stores through projections, serveGraph reads an
// older projection through history. pgStore is both; memStore keeps only the latest, so it has no history.
type (
	projections interface {
		// put stores res as the next projection; latest is memory's latest, read where the store needs it. v is the
		// projection to make the latest, nil to keep the current one.
		put(ctx context.Context, res extract.Result, repos extract.Repos, counted map[string]bool, latest func() *atlasView) (seq int64, changed bool, v *atlasView, err error)
	}
	history interface {
		// load is projection seq: errNoProjection when none is stored, errNoHistory when the store keeps none.
		load(ctx context.Context, seq int64) (atlasView, error)
	}
)

var errNoHistory = errors.New("atlas: no history without a database")

// pgStore keeps every projection in Postgres.
type pgStore struct{ pool *pgxpool.Pool }

func (s *pgStore) put(ctx context.Context, res extract.Result, repos extract.Repos, counted map[string]bool, latest func() *atlasView) (int64, bool, *atlasView, error) {
	seq, changed, err := projectGraph(ctx, s.pool, res, repos, counted)
	if err != nil {
		return seq, changed, nil, err
	}
	if cur := latest(); !changed && cur != nil && cur.Seq == seq {
		return seq, false, nil, nil
	}
	// changed, or the store's latest is another than memory's (atlas project stored one meanwhile): load it
	v, err := loadGraph(ctx, s.pool, seq)
	if err != nil {
		return seq, changed, nil, err
	}
	return seq, true, &v, nil
}

func (s *pgStore) load(ctx context.Context, seq int64) (atlasView, error) {
	return loadGraph(ctx, s.pool, seq)
}

// memStore holds only the latest projection's signature (Graphs holds the latest). sig is its content ([vector, nodes, edges, drift,
// extractors]: not tracked, skipped, at or seq), to tell a change; seq counts changes since atlas started.
type memStore struct {
	mu  sync.Mutex
	sig string
}

func (s *memStore) put(_ context.Context, res extract.Result, repos extract.Repos, counted map[string]bool, latest func() *atlasView) (int64, bool, *atlasView, error) {
	cur := latest()
	if cur != nil {
		now := map[string]string{}
		for r := range repos {
			now[r] = ""
		}
		if lost := lostRepos(cur.Vector, now, counted); len(lost) > 0 {
			return cur.Seq, false, nil, fmt.Errorf("%w: %s", errLostRepos, strings.Join(lost, ", "))
		}
	}
	v := viewOf(1, res, repos)
	sig := string(mustJSON([]any{v.Vector, v.Nodes, v.Edges, v.Drift, v.Extractors}))
	s.mu.Lock()
	same := sig == s.sig
	s.mu.Unlock()
	if cur != nil && same {
		return cur.Seq, false, nil, nil
	}
	if cur != nil {
		v.Seq = cur.Seq + 1
	}
	// sig is read only by put, and one goroutine puts: writing it before Graphs.set makes v the latest is unobservable
	s.mu.Lock()
	s.sig = sig
	s.mu.Unlock()
	return v.Seq, true, &v, nil
}

func (s *memStore) load(context.Context, int64) (atlasView, error) { return atlasView{}, errNoHistory }

// stores is the store in use: the database's once attached, else memory.
func (g *Graphs) stores() (projections, history) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pg != nil {
		return g.pg, g.pg
	}
	return &g.mem, &g.mem
}

// openGraphs opens dsn and loads the latest stored projection. Empty dsn = memory-only (logged). A database that
// can't be reached or migrated is retried every 30 s in the background; until then atlas runs memory-only.
func openGraphs(ctx context.Context, dsn string, met *Metrics) *Graphs {
	g := &Graphs{met: met}
	g.met.SourceOK("db", float64(time.Now().Unix())) // memory-only ages into MountainSourceStale: the alarm for a missing secret
	if dsn == "" {
		slog.Warn("db: DATABASE_URL unset: memory-only (the latest projection, no history)")
		return g
	}
	if err := g.attach(ctx, dsn); err != nil {
		slog.Warn(fmt.Sprintf("db: %v: memory-only until it answers (retried every 30 s)", err))
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if g.attach(ctx, dsn) == nil {
						slog.Info("db: attached")
						return
					}
				}
			}
		}()
	}
	return g
}

// attach connects, migrates and loads the latest stored projection when memory has none.
func (g *Graphs) attach(ctx context.Context, dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return err
	}
	g.met.SourceOK("db", float64(time.Now().Unix()))
	v, err := loadGraph(ctx, pool, 0)
	switch {
	case err == nil && g.Latest() == nil:
		g.set(&v)
	case err != nil && !errors.Is(err, errNoProjection):
		slog.Error(fmt.Sprintf("db: load the latest projection: %v", err))
	}
	g.mu.Lock()
	g.pg = &pgStore{pool}
	g.mu.Unlock()
	go g.ping(ctx, pool)
	return nil
}

// db is the database, nil while there is none (close and the DB tests use it).
func (g *Graphs) db() *pgxpool.Pool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pg == nil {
		return nil
	}
	return g.pg.pool
}

func (g *Graphs) ping(ctx context.Context, pool *pgxpool.Pool) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pc, cancel := context.WithTimeout(ctx, 5*time.Second)
			if pool.Ping(pc) == nil {
				g.met.SourceOK("db", float64(time.Now().Unix()))
			}
			cancel()
		}
	}
}

func (g *Graphs) close() {
	if pool := g.db(); pool != nil {
		pool.Close()
	}
}

func (g *Graphs) set(v *atlasView) {
	idx := buildIndex(v)
	g.mu.Lock()
	g.latest, g.idx = v, idx
	g.mu.Unlock()
	g.met.ProjectionSeq(float64(v.Seq))
	var drift []graph.Drift
	_ = json.Unmarshal(v.Drift, &drift)
	counts := map[string]float64{graph.DriftUnassigned: 0, graph.DriftMirror: 0, graph.DriftAmbiguous: 0, graph.DriftUnresolved: 0}
	for _, d := range drift {
		counts[d.Kind]++
	}
	for k, n := range counts {
		g.met.DriftCount(k, n)
	}
}

// Latest is the latest projection (nil = none yet).
func (g *Graphs) Latest() *atlasView {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.latest
}

func (g *Graphs) index() *GraphIndex {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.idx
}

// put stores res as the next projection and makes it the latest; changed=false when it is the latest already.
func (g *Graphs) put(ctx context.Context, res extract.Result, repos extract.Repos, counted map[string]bool) (int64, bool, error) {
	st, _ := g.stores()
	seq, changed, v, err := st.put(ctx, res, repos, counted, g.Latest)
	if v != nil {
		g.set(v)
	}
	return seq, changed, err
}

// loadFixture serves file (an /api/graph answer) as the latest projection: --dev only (the gate, local work).
func (g *Graphs) loadFixture(file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var v atlasView
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if v.Seq == 0 {
		v.Seq = 1
	}
	g.set(&v)
	return nil
}

// GraphIndex is what the live overlay joins on: which node a namespace, a k8s node or an ArgoCD name is.
type GraphIndex struct {
	Seq    int64
	Realm  map[string]string   // node id -> realm ("" = not placed: hostnames, ADRs)
	NSApps map[string][]string // namespace -> app ids deploying into it, sorted
	Has    map[string]bool     // node ids
	Into   map[string]string   // addon node id -> the namespace kubespray installs it into
	Watch  map[string][]string // Prometheus alert name -> the nodes whose watch names it, sorted
	Kind   map[string]string   // node id -> kind (contract.Classify)
	RunsOn map[string][]string // runs-on edges, from -> to (contract.Classify)
}

func buildIndex(v *atlasView) *GraphIndex {
	x := &GraphIndex{Seq: v.Seq, Realm: map[string]string{}, NSApps: map[string][]string{}, Has: map[string]bool{}, Into: map[string]string{}, Watch: map[string][]string{},
		Kind: map[string]string{}, RunsOn: map[string][]string{}}
	for _, raw := range v.Nodes {
		var n graph.Node
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		x.Has[n.ID] = true
		x.Kind[n.ID] = n.Kind
		x.Realm[n.ID] = n.Realm
		if n.Kind == graph.KindAddon && n.Attrs["into"] != "" {
			x.Into[n.ID] = n.Attrs["into"]
		}
		if n.Kind == graph.KindApp && n.Attrs["namespace"] != "" {
			x.NSApps[n.Attrs["namespace"]] = append(x.NSApps[n.Attrs["namespace"]], n.ID)
		}
		for _, r := range strings.Split(n.Attrs["watch"], ",") {
			if r = strings.TrimSpace(r); r != "" {
				x.Watch[r] = append(x.Watch[r], n.ID)
			}
		}
	}
	for _, raw := range v.Edges {
		var e graph.Edge
		if json.Unmarshal(raw, &e) == nil && e.Kind == graph.EdgeRunsOn && x.Has[e.From] && x.Has[e.To] {
			x.RunsOn[e.From] = append(x.RunsOn[e.From], e.To)
		}
	}
	for _, l := range x.NSApps {
		slices.Sort(l)
	}
	for _, l := range x.Watch {
		slices.Sort(l)
	}
	return x
}

// serveGraph is GET /api/graph[?seq=N] (admin-only by the route table): the latest projection from memory, or an
// older one from the database.
func (a *App) serveGraph(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	var seq int64
	if q := r.URL.Query().Get("seq"); q != "" {
		n, err := strconv.ParseInt(q, 10, 64)
		if err != nil || n <= 0 {
			errs.Write(w, errs.Invalid(keyGraphSeq, "seq is a positive integer"))
			return
		}
		seq = n
	}
	g := a.Graph
	cur := g.Latest()
	var v atlasView
	switch {
	case cur != nil && (seq == 0 || seq == cur.Seq):
		v = *cur
	case seq == 0:
		errs.Write(w, errs.Missing(keyGraphNone, errNoProjection.Error()))
		return
	default:
		_, hist := g.stores()
		var err error
		v, err = hist.load(r.Context(), seq)
		switch {
		case errors.Is(err, errNoHistory):
			errs.Write(w, errs.Missing(keyGraphNoHistory, err.Error()))
			return
		case errors.Is(err, errNoProjection):
			errs.Write(w, errs.Missing(keyGraphSeqMissing, err.Error()))
			return
		case err != nil:
			slog.ErrorContext(r.Context(), fmt.Sprintf("atlas: %v", err))
			errs.Write(w, internalError("atlas: read failed"))
			return
		}
	}
	h.Set("Content-Type", "application/json")
	_, _ = w.Write(mustJSON(v))
}

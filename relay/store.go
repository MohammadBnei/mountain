package main

// Persistence (02 "Persistence", W2): Postgres through pgx v5, a direct connection (no pgbouncer), so
// prepared statements and the advisory-lock migration are fine. Empty DATABASE_URL = memory-only.
//
// Writes never block the hub: record (the hub tap, under h.mu) and snapshot queue closures on a buffered
// channel that one writer goroutine drains. The DB therefore sees frames and snapshots in broadcast
// order, and frame_seq orders them. When the queue is full the write is dropped (log + mountain_db_dropped_total).
// Health: mountain_source_last_success{source="db"} is set on every good write and on a 30 s ping, so
// MountainSourceStale covers a dead DB. In memory-only mode it is seeded once and ages (the alarm for a
// missing secret). The page is not marked stale, because the live view does not depend on the DB.
//
// hello = the latest snapshot's state + the world frames since it (since) + state + metrics. Boot: load the
// latest town snapshot (townEpoch) and re-apply every frame after it.

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohammadBnei/mountain/relay/town"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	dbQueue     = 1024
	dbPingEvery = 30 * time.Second
	snapEvery   = 10 // minutes
	// townEpoch is the snapshots.seed of the town (R7). The mountain wrote its terrain seed (1) there; the town
	// stores no terrain, so the column now names the world's generation and load() reads this one only: a
	// mountain row (or one archived by migration 004) is never folded into the town.
	townEpoch    = 2
	metricsEvery = 5          // minutes
	migrateLock  = 0x6d6f756e // "moun"
)

// Store is the Postgres side. A nil *Store is memory-only: every method is a no-op.
type Store struct {
	pool *pgxpool.Pool
	q    chan func(context.Context) error
	stop chan struct{} // closed by Close: run returns
}

func openStore(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8 // connlimit 10 (02); pgx's default max(4, NumCPU) can exceed it
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool, q: make(chan func(context.Context) error, dbQueue), stop: make(chan struct{})}
	go s.run()
	return s, nil
}

// migrate applies migrations/*.sql in name order, once each, under a session advisory lock (a second
// relay or test waits instead of racing the DDL).
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	if _, err := c.Exec(ctx, "SELECT pg_advisory_lock($1)", migrateLock); err != nil {
		return err
	}
	defer c.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrateLock)
	if _, err := c.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	for _, f := range files {
		v := path.Base(f)
		var done bool
		if err := c.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", v).Scan(&done); err != nil {
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
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", v)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", v, err)
		}
		log.Printf("db: applied migration %s", v)
	}
	return nil
}

func (s *Store) run() {
	t := time.NewTicker(dbPingEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case op := <-s.q:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := op(ctx)
			cancel()
			if err != nil {
				DBDropped.Add("error", 1)
				log.Printf("db: write failed: %v", err)
				continue
			}
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.pool.Ping(ctx)
			cancel()
			if err != nil {
				log.Printf("db: ping: %v", err)
				continue
			}
		}
		SourceLastSuccess.Set("db", float64(time.Now().Unix()))
	}
}

// save queues op without blocking (the caller may hold h.mu).
func (s *Store) save(op func(context.Context) error) {
	if s == nil {
		return
	}
	select {
	case s.q <- op:
	default:
		DBDropped.Add("full", 1)
		log.Printf("db: queue full, write dropped")
	}
}

// flush waits up to d for every write queued before it; false on timeout.
func (s *Store) flush(d time.Duration) bool {
	done, t := make(chan struct{}), time.After(d)
	select {
	case s.q <- func(context.Context) error { close(done); return nil }:
	case <-t:
		return false
	}
	select {
	case <-done:
		return true
	case <-t:
		return false
	}
}

// Close flushes the queue (up to d), stops the writer, then closes the pool.
func (s *Store) Close(d time.Duration) {
	if s == nil {
		return
	}
	if !s.flush(d) {
		log.Printf("db: shutdown flush timed out, queued writes lost")
	}
	close(s.stop)
	s.pool.Close()
}

func utcDay(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// frame persists one broadcast ev or world frame (b is the sanitised frame as sent).
func (s *Store) frame(t, kind, id string, ts time.Time, b []byte) {
	s.save(func(ctx context.Context) error {
		var err error
		if t == "ev" {
			_, err = s.pool.Exec(ctx, "INSERT INTO events (id, ts, day, kind, public) VALUES ($1, $2, $3, $4, $5)", id, ts, utcDay(ts), kind, json.RawMessage(b))
		} else {
			_, err = s.pool.Exec(ctx, "INSERT INTO world_deltas (ts, day, kind, frame) VALUES ($1, $2, $3, $4)", ts, utcDay(ts), kind, json.RawMessage(b))
		}
		return err
	})
}

// detail stores an admin detail frame on its event's row (queued after the row's insert: same writer, same order).
// The snapshot write clears details older than 30 days (02).
func (s *Store) detail(id string, b []byte) {
	s.save(func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, "UPDATE events SET detail = $2 WHERE id = $1", id, json.RawMessage(b))
		return err
	})
}

func (s *Store) snapshot(day string, ts time.Time, state, metrics []byte) {
	s.save(func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, `INSERT INTO snapshots (day, seq, ts, seed, land, state, metrics)
			VALUES ($1, nextval('frame_seq'), $2, $3, ''::bytea, $4, $5)
			ON CONFLICT (day) DO UPDATE SET seq = excluded.seq, ts = excluded.ts, seed = excluded.seed,
				land = excluded.land, state = excluded.state, metrics = excluded.metrics`,
			day, ts, townEpoch, json.RawMessage(state), json.RawMessage(metrics))
		if err == nil { // detail retention (02) rides the 10 min snapshot: no job of its own, runs with no alert firing
			_, err = s.pool.Exec(ctx, "UPDATE events SET detail = NULL WHERE detail IS NOT NULL AND day < current_date - 30")
		}
		return err
	})
}

// mark stores an accepted mark's private record (validators.go place).
func (s *Store) mark(id, sub, handle, toy string, ts time.Time, pose map[string]any, weight float64) {
	pb := mustJSON(pose) // now: the caller's map is shared with the broadcast frame
	s.save(func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, "INSERT INTO marks (id, sub, handle, toy, ts, day, pose, weight) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			id, sub, handle, toy, ts, utcDay(ts), json.RawMessage(pb), weight)
		return err
	})
}

// unmark records an admin mark.delete.
func (s *Store) unmark(id string, ts time.Time) {
	s.save(func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, "UPDATE marks SET deleted_at = $2 WHERE id = $1", id, ts)
		return err
	})
}

// marksBoot returns the accounts of the active marks ids and every account's marks of the last 24 h (rate limits).
func (s *Store) marksBoot(ctx context.Context, ids []string, now time.Time) (map[string]string, map[string][]time.Time, error) {
	subs, recent := map[string]string{}, map[string][]time.Time{}
	rows, _ := s.pool.Query(ctx, "SELECT id, sub FROM marks WHERE id = ANY($1)", ids)
	var id, sub string
	if _, err := pgx.ForEachRow(rows, []any{&id, &sub}, func() error { subs[id] = sub; return nil }); err != nil {
		return nil, nil, err
	}
	var ts time.Time
	rows, _ = s.pool.Query(ctx, "SELECT sub, ts FROM marks WHERE ts > $1 ORDER BY ts", now.Add(-24*time.Hour))
	_, err := pgx.ForEachRow(rows, []any{&sub, &ts}, func() error { recent[sub] = append(recent[sub], ts); return nil })
	return subs, recent, err
}

func (s *Store) metrics(ts time.Time, b []byte) {
	s.save(func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, "INSERT INTO metrics_5m (ts, day, frame) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", ts, utcDay(ts), json.RawMessage(b))
		return err
	})
}

// dbSnap is a stored snapshot plus every ev/world frame after it, in order.
type dbSnap struct {
	state   State
	metrics Metrics
	frames  []json.RawMessage
	r7      bool // the snapshot was written before R8 (migration 005 town_r8): its plots are re-placed by family

	calmKnown bool // R9: the snapshot carries a calm key (a snapshot from before R9 does not: civ.go seedCalm)
}

// load returns the latest town snapshot (before day `before`, if not nil) and the frames after it (also before
// that day). nil, nil = no snapshot.
func (s *Store) load(ctx context.Context, before *string) (*dbSnap, error) {
	var (
		seq       int64
		st, mt    []byte
		r         = dbSnap{state: newState()} // a snapshot from before a State field keeps that field's default
		beforeArg any
	)
	if before != nil {
		beforeArg = *before
	}
	err := s.pool.QueryRow(ctx, `SELECT seq, state, metrics, COALESCE(ts < (SELECT started FROM town_r8), false) FROM snapshots
		WHERE seed = $1 AND ($2::date IS NULL OR day < $2::date) ORDER BY seq DESC LIMIT 1`, townEpoch, beforeArg).Scan(&seq, &st, &mt, &r.r7)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(st, &r.state); err != nil {
		return nil, fmt.Errorf("snapshot seq %d state: %w", seq, err)
	}
	var keys struct {
		Calm *string `json:"calm"`
	}
	if json.Unmarshal(st, &keys) == nil {
		r.calmKnown = keys.Calm != nil
	}
	if err := json.Unmarshal(mt, &r.metrics); err != nil {
		return nil, fmt.Errorf("snapshot seq %d metrics: %w", seq, err)
	}
	r.frames, err = s.rows(ctx, `SELECT frame FROM (
			SELECT seq, public AS frame FROM events WHERE seq > $1 AND ($2::date IS NULL OR day < $2::date)
			UNION ALL SELECT seq, frame FROM world_deltas WHERE seq > $1 AND ($2::date IS NULL OR day < $2::date)
		) f ORDER BY seq`, seq, beforeArg)
	return &r, err
}

// started is the town's first day (migration 004 town_epoch): days before it hold no town (scrub: unavailable).
func (s *Store) started(ctx context.Context) (string, error) {
	var d time.Time
	err := s.pool.QueryRow(ctx, "SELECT started FROM town_epoch").Scan(&d)
	return d.Format(time.DateOnly), err
}

// day returns one UTC day's ev + world frames (broadcast order) and its metrics_5m frames.
func (s *Store) day(ctx context.Context, day string) (frames, metrics []json.RawMessage, err error) {
	frames, err = s.rows(ctx, `SELECT frame FROM (
			SELECT seq, public AS frame FROM events WHERE day = $1
			UNION ALL SELECT seq, frame FROM world_deltas WHERE day = $1
		) f ORDER BY seq`, day)
	if err != nil {
		return nil, nil, err
	}
	metrics, err = s.rows(ctx, "SELECT frame FROM metrics_5m WHERE day = $1 ORDER BY ts", day)
	return frames, metrics, err
}

func (s *Store) rows(ctx context.Context, sql string, args ...any) ([]json.RawMessage, error) {
	rows, _ := s.pool.Query(ctx, sql, args...)
	out, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	res := make([]json.RawMessage, len(out))
	for i, b := range out {
		res[i] = b
	}
	return res, err
}

// mountainToys are the mountain's visitor toys (R7 dropped them): their marks never come back into the town.
var mountainToys = map[string]bool{"cairns": true, "toss": true, "footprints": true, "toy-clouds": true}

// fold re-applies stored frames to st (State.apply). Returns the world frames (hello deltas after a restore).
func fold(st *State, frames []json.RawMessage) []json.RawMessage {
	since := []json.RawMessage{}
	for _, b := range frames {
		var f frameF
		if err := json.Unmarshal(b, &f); err != nil {
			log.Printf("db: fold: stored frame skipped: %v", err) // the restored state misses it
			continue
		}
		st.apply(f, b)
		if f.T == "world" {
			since = append(since, b)
		}
	}
	return since
}

// attach opens DATABASE_URL and restores the world from it. Empty or unusable = memory-only (logged).
// An unusable DB is never written, so a DB that comes back is not overwritten by a world that missed it;
// a restart after it recovers restores normally. ro (dump-hello): restore without writing a first snapshot.
func (a *App) attach(ctx context.Context, dsn string, ro bool) {
	SourceLastSuccess.Set("db", float64(time.Now().Unix())) // memory-only: never refreshed, ages into MountainSourceStale
	if dsn == "" {
		log.Printf("relay: DATABASE_URL unset: memory-only, the world is lost on restart")
		a.State.Calm = utcDay(time.Now()) // R9: nothing known before the boot
		return
	}
	s, err := openStore(ctx, dsn)
	if err == nil {
		a.store = s
		if err = a.restore(ctx, ro); err != nil {
			a.store = nil
			s.Close(time.Second) // nothing queued yet: restore failed before any write
		} else if d, e := s.started(ctx); e == nil {
			a.started = d // R9: the turn counter's day one
		} else {
			log.Printf("relay: town start day: %v", e)
		}
	}
	if err != nil {
		log.Printf("relay: database unusable, memory-only until restart: %v", err)
		a.State.Calm = utcDay(time.Now()) // R9: as memory-only (nothing known before the boot)
	}
}

// restore loads the latest town snapshot and re-applies the frames after it. No snapshot yet (the town's first
// boot): write one now (unless ro), so every later frame has a base.
func (a *App) restore(ctx context.Context, ro bool) error {
	r, err := a.store.load(ctx, nil)
	if err != nil {
		return err
	}
	if r == nil {
		// the town's first boot: still seed the rate limits, so the marks of the last 24 h (the mountain's too) count
		_, recent, err := a.store.marksBoot(ctx, nil, time.Now())
		if err != nil {
			return err
		}
		visits.Lock()
		visits.at = recent
		visits.Unlock()
		a.mu.Lock()
		a.State.Calm = utcDay(time.Now()) // R9: the town's first day starts calm (no trouble known before it)
		a.mu.Unlock()
		if !ro {
			a.snapshot(time.Now())
		}
		return nil
	}
	a.seedCalm(ctx, r, nil) // R9: before the frames, which keep it from there (State.apply)
	st := r.state
	since := fold(&st, r.frames)
	st.Marks = slices.DeleteFunc(st.Marks, func(m Mark) bool { return mountainToys[m.Mark] }) // R7: their toys are gone
	// an app dropped from config.yaml: its plot could never be freed (its frames fail sanitize). A plot whose
	// building kind changed in config.yaml goes too, and the first poll places the new kind where its family belongs.
	// R8's first boot (a snapshot from before town_r8): every plot goes, so every building moves to its family's ring
	// on the bigger islands. Ruins go with them: their apps are absent, so nothing re-places them.
	// R8 deploy B: a machine's plot off its configured host goes too (a machine never moves; it is placed again).
	st.Plots = slices.DeleteFunc(st.Plots, func(p Plot) bool {
		c, ok := a.Cfg.Apps[p.App]
		return r.r7 || !ok || p.Role != a.Cfg.roleOf(p.App) || (c.Group == "machine" && p.Host != c.Host)
	})
	// R8: only configured probed machines keep a state (a snapshot from before deploy B has none)
	machines := map[string]string{}
	for k, v := range st.Machines {
		if a.Cfg.machine(k) && a.Cfg.Apps[k].Probe != "" && (v == "up" || v == "down") {
			machines[k] = v
		}
	}
	st.Machines = machines
	// R8 C: tents. Only what geometry forbids goes here (annexes.go): an app no longer configured or a machine, a hex
	// that is no grass of a known host, or one a plot, a plant or an earlier tent holds. The one rule (annexOK) is
	// the first poll's: its gone frames persist, and the page skips a tent beside no standing plot meanwhile.
	held := map[string]map[town.Hex]bool{}
	hold := func(h string, x town.Hex) bool {
		if held[h] == nil {
			held[h] = map[town.Hex]bool{}
		}
		was := held[h][x]
		held[h][x] = true
		return was
	}
	for _, p := range st.Plots {
		hold(p.Host, p.hex())
	}
	for h, ps := range plantAt(st.Marks) {
		for x := range ps {
			hold(h, x)
		}
	}
	seenTent := map[string]bool{}
	st.Annexes = slices.DeleteFunc(slices.Clone(st.Annexes), func(x Annex) bool {
		c, ok := a.Cfg.Apps[x.App]
		bad := !ok || c.Group == "machine" || town.Tile(x.Host, x.hex()) != town.Grass || seenTent[x.key()] || hold(x.Host, x.hex())
		seenTent[x.key()] = true
		return bad
	})
	if st.Annexes == nil { // a snapshot from before R8 deploy C
		st.Annexes = []Annex{}
	}
	st.Annexes = st.Annexes[max(0, len(st.Annexes)-annexesMax):]
	ids := make([]string, len(st.Marks))
	for i, m := range st.Marks {
		ids[i] = m.ID
	}
	subs, recent, err := a.store.marksBoot(ctx, ids, time.Now())
	if err != nil {
		return err
	}
	r.metrics.T, r.metrics.Stale = "metrics", map[string]int64{} // sources re-mark what is still down
	visits.Lock()
	visits.at = recent
	visits.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	r.metrics.Visitors = a.Metrics.Visitors // the kill switch is the flag's, never the snapshot's
	a.since, a.State, a.Metrics, a.MarkSubs = since, st, r.metrics, subs
	log.Printf("db: restored snapshot + %d frames, %d firing alerts, %d plots, %d marks", len(r.frames), len(st.Alerts), len(st.Plots), len(st.Marks))
	return nil
}

// record is the hub tap (under h.mu): every broadcast ev and world frame updates the state and is persisted;
// world frames join the hello deltas until the next snapshot. The town derives no frames from events (nil).
func (a *App) record(b []byte) [][]byte {
	var f frameF
	if json.Unmarshal(b, &f) != nil || (f.T != "ev" && f.T != "world") {
		return nil
	}
	ts := time.Now()
	if f.TS > 0 {
		ts = time.UnixMilli(f.TS)
	}
	a.mu.Lock()
	a.State.apply(f, b)
	if f.T == "world" {
		a.since = append(a.since, b)
	}
	a.mu.Unlock()
	a.store.frame(f.T, f.Kind, f.ID, ts, b)
	return nil
}

// snapshot folds the hello deltas into the hello base, advances App.Day and upserts the day's row.
// h.mu then a.mu: no frame lands between the copy and the queueing, so the row's seq splits frames exactly.
func (a *App) snapshot(now time.Time) {
	a.Hub.mu.Lock()
	defer a.Hub.mu.Unlock()
	a.mu.Lock()
	day := utcDay(now)
	a.Day, a.since = day, []json.RawMessage{}
	st, m := mustJSON(a.State), mustJSON(a.Metrics)
	a.mu.Unlock()
	a.store.snapshot(day, now, st, m)
}

// persist runs every minute: a metrics_5m row every 5 min, a snapshot every 10 min and at UTC rollover.
// It also runs in memory-only mode, where the snapshot only bounds the hello deltas.
func (a *App) persist(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			a.mu.Lock()
			m, day := mustJSON(a.Metrics), a.Day
			a.mu.Unlock()
			if n%metricsEvery == 0 {
				a.store.metrics(now, m)
			}
			if n%snapEvery == 0 || utcDay(now) != day {
				a.snapshot(now)
			}
		}
	}
}

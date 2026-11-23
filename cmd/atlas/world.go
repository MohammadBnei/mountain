package main

// World events: atlas publishes what each projection changed in the world to the bridge (dev.bnei.world.*), so the
// library can build birth, bond and rest arcs keyed by System id. The diff of one projection against the last is
// written to atlas_outbox in projectGraph's own transaction; the publisher drains it.
//
// A node id absent before is entity.created, one gone is entity.retired; an edge key likewise for bond.added and
// bond.removed. A change of an existing id (a moved Realm, Parent or Group, a new hash) is no event: the library
// re-reads atlas for the current state. When atlas's history restarts or its lens changes there is no complete
// history to diff, so one history.rebased row stands in for the per-entity rows: "re-snapshot atlas at this
// epoch.seq; the events before it are not a complete history". Its causes: fresh (the first projection of a
// database), extractor (the versions changed), sources (the repo set changed), baseline (migration 002).
//
// ce_ids carry <epoch>.<seq>: seq grows within an epoch, the epoch (the first projection's unix nanos, atlas_epoch)
// grows across a rebuilt database, so an id is epoch-qualified, grows across rebuilds, and a retry carries the same
// id from the outbox.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
)

const (
	worldSource = "world" // the health series' source label

	typeCreated = "dev.bnei.world.entity.created"
	typeRetired = "dev.bnei.world.entity.retired"
	typeAdded   = "dev.bnei.world.bond.added"
	typeRemoved = "dev.bnei.world.bond.removed"
	typeRebased = "dev.bnei.world.history.rebased"
)

type worldEvent struct {
	ce, typ, subject string
	body             any
}

// writeOutbox writes the events of projection seq (made at at) into the outbox, in tx. cause is why there is no
// diff ("fresh", "extractor", "sources"), "" for a diff of curNodes, curEdges against res.
func writeOutbox(ctx context.Context, tx pgx.Tx, seq int64, at time.Time, cause string, vector map[string]string, res extract.Result,
	curNodes, curEdges map[string]string) error {
	var epoch int64
	switch err := tx.QueryRow(ctx, "SELECT epoch FROM atlas_epoch").Scan(&epoch); {
	case errors.Is(err, pgx.ErrNoRows):
		epoch = at.UnixNano()
		if _, err := tx.Exec(ctx, "INSERT INTO atlas_epoch (epoch) VALUES ($1)", epoch); err != nil {
			return err
		}
	case err != nil:
		return err
	}
	ver := fmt.Sprintf("%d.%d", epoch, seq)
	meta := func(body any, cause string) map[string]any {
		return map[string]any{"body": body, "epoch": epoch, "seq": seq, "cause": cause}
	}
	var evs []worldEvent
	if cause != "" {
		evs = []worldEvent{{"rebased:" + ver, typeRebased, "world/", map[string]any{"epoch": epoch, "seq": seq, "cause": cause, "vector": vector}}}
	} else {
		var err error
		if evs, err = diffEvents(ctx, tx, seq, ver, res, curNodes, curEdges); err != nil {
			return err
		}
		for i := range evs {
			evs[i].body = meta(evs[i].body, "repo")
		}
	}
	for _, e := range evs {
		if _, err := tx.Exec(ctx, "INSERT INTO atlas_outbox (ce_id, type, subject, data, time) VALUES ($1, $2, $3, $4, $5)",
			e.ce, e.typ, e.subject, mustJSON(e.body), at); err != nil {
			return err
		}
	}
	return nil
}

// diffEvents are the events of the nodes and edges res adds or drops against cur*, in the canonical order:
// bond.removed, entity.retired, entity.created, bond.added, each group by key. The retired and removed bodies are the
// closed versions' (valid_to = seq, so the versions are already written).
func diffEvents(ctx context.Context, tx pgx.Tx, seq int64, ver string, res extract.Result, curNodes, curEdges map[string]string) ([]worldEvent, error) {
	bondID := func(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:])[:16] }
	closed := func(table, col string, cur map[string]string, now func(string) bool) (map[string]json.RawMessage, []string, error) {
		var gone []string
		for k := range cur {
			if !now(k) {
				gone = append(gone, k)
			}
		}
		slices.Sort(gone)
		bodies := map[string]json.RawMessage{}
		rows, _ := tx.Query(ctx, "SELECT "+col+", body FROM "+table+" WHERE valid_to = $1 AND "+col+" = ANY($2)", seq, gone)
		var k string
		var b []byte
		_, err := pgx.ForEachRow(rows, []any{&k, &b}, func() error { bodies[k] = append(json.RawMessage(nil), b...); return nil })
		return bodies, gone, err
	}
	hasNode, hasEdge := map[string]bool{}, map[string]bool{}
	for _, n := range res.Graph.Nodes {
		hasNode[n.ID] = true
	}
	for _, e := range res.Graph.Edges {
		hasEdge[e.Key()] = true
	}
	var removed, retired, created, added []worldEvent
	eb, egone, err := closed("atlas_edge_ver", "key", curEdges, func(k string) bool { return hasEdge[k] })
	if err != nil {
		return nil, err
	}
	for _, k := range egone {
		var e struct{ From string }
		if err := json.Unmarshal(eb[k], &e); err != nil {
			return nil, err
		}
		removed = append(removed, worldEvent{bondID(k) + ":removed:" + ver, typeRemoved, "world/" + e.From, eb[k]})
	}
	nb, ngone, err := closed("atlas_node_ver", "id", curNodes, func(k string) bool { return hasNode[k] })
	if err != nil {
		return nil, err
	}
	for _, k := range ngone {
		retired = append(retired, worldEvent{k + ":retired:" + ver, typeRetired, "world/" + k, nb[k]})
	}
	for _, n := range slices.SortedFunc(slices.Values(res.Graph.Nodes), func(a, b graph.Node) int { return strings.Compare(a.ID, b.ID) }) {
		if _, ok := curNodes[n.ID]; !ok {
			created = append(created, worldEvent{n.ID + ":created:" + ver, typeCreated, "world/" + n.ID, n})
		}
	}
	for _, e := range slices.SortedFunc(slices.Values(res.Graph.Edges), func(a, b graph.Edge) int { return strings.Compare(a.Key(), b.Key()) }) {
		if _, ok := curEdges[e.Key()]; !ok {
			added = append(added, worldEvent{bondID(e.Key()) + ":added:" + ver, typeAdded, "world/" + e.From, e})
		}
	}
	return slices.Concat(removed, retired, created, added), nil
}

// worldInterval is how often the outbox is drained and its age measured (a projection that wrote rows also drains).
const worldInterval = 30 * time.Second

// drainOutbox publishes the outbox in id order, until it is empty or an error stops it (the next round retries the
// same row, so nothing is skipped or reordered). publish returns whether the bridge already had the event. A
// duplicate is as good as published and its row goes; InvalidArgument is the bridge saying that retrying cannot help,
// so the row is parked (logged, counted, deleted); any other error, PermissionDenied included (the grant rolls out
// independently), leaves the row. It returns how many rows it removed.
func drainOutbox(ctx context.Context, pool *pgxpool.Pool, publish func(context.Context, *bridgev1.CloudEvent) (bool, error), met *Metrics) (int, error) {
	n := 0
	for {
		var id int64
		var ev bridgev1.CloudEvent
		var data []byte
		var at time.Time
		err := pool.QueryRow(ctx, "SELECT id, ce_id, type, subject, data, time FROM atlas_outbox ORDER BY id LIMIT 1").Scan(&id, &ev.Id, &ev.Type, &ev.Subject, &data, &at)
		if errors.Is(err, pgx.ErrNoRows) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		ev.Data, ev.Time = data, timestamppb.New(at)
		dup, err := publish(ctx, &ev)
		switch {
		case err == nil:
			if !dup {
				met.WorldPublished(ev.Type)
			}
		case connect.CodeOf(err) == connect.CodeInvalidArgument:
			slog.Error(fmt.Sprintf("world: event %s parked, the bridge refuses it for good: %v", ev.Id, err))
			met.WorldRefused()
		default:
			return n, err
		}
		if _, err := pool.Exec(ctx, "DELETE FROM atlas_outbox WHERE id = $1", id); err != nil {
			return n, err
		}
		n++
	}
}

// runWorld drains the outbox every worldInterval and whenever kick fires, and records its gauges. pool is the
// database once there is one (nil until then); publish is nil when the bridge is not configured, which leaves the
// outbox to grow and the gauges to say so (mountain_world_enabled 0; the age of its oldest row keeps being measured).
//
// Health is mountain_source_last_success{source="world"}, seeded by the caller and renewed only by an answer of the
// bridge to a Publish: an event it took, a duplicate, or its InvalidArgument. With the outbox empty an empty Publish
// is the probe: the bridge authenticates, then refuses it InvalidArgument (keyFields), so that answer proves the
// sign-in and the connection, not the grant (a missing grant shows as rows that stay, in mountain_world_outbox_oldest_seconds).
func runWorld(ctx context.Context, pool func() *pgxpool.Pool, publish func(context.Context, *bridgev1.CloudEvent) (bool, error), met *Metrics, kick <-chan struct{}) {
	t := time.NewTicker(worldInterval)
	defer t.Stop()
	for {
		p := pool()
		on := p != nil && publish != nil
		met.WorldEnabled(on)
		if p != nil {
			if err := tick(ctx, p, publish, met); err != nil && ctx.Err() == nil {
				slog.Warn(fmt.Sprintf("world: %v", err))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-kick:
		}
	}
}

func tick(ctx context.Context, p *pgxpool.Pool, publish func(context.Context, *bridgev1.CloudEvent) (bool, error), met *Metrics) error {
	var err error
	if publish != nil {
		answered := func(ctx context.Context, ev *bridgev1.CloudEvent) (bool, error) {
			dup, err := publish(ctx, ev)
			if err == nil || connect.CodeOf(err) == connect.CodeInvalidArgument {
				met.SourceOK(worldSource, float64(time.Now().Unix()))
			}
			return dup, err
		}
		var n int
		if n, err = drainOutbox(ctx, p, answered, met); err == nil && n == 0 {
			_, _ = answered(ctx, &bridgev1.CloudEvent{}) // the probe, see runWorld: its answer is only the health mark's
		}
	}
	var oldest float64
	if qerr := p.QueryRow(ctx, "SELECT coalesce(extract(epoch FROM now() - min(at)), 0)::float8 FROM atlas_outbox").Scan(&oldest); qerr != nil {
		return errors.Join(err, qerr)
	}
	met.WorldOutboxOldest(oldest)
	return err
}

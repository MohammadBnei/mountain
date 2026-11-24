package main

// The owner's feed (plan 2026-11-24, slices 2 and 3): threads of alert episodes, deploy rollouts, PRs, agent sessions
// and cards, rebuilt from the bridge
// at start and kept live. The logic is internal/feed; this file is the wiring: FEED_ENABLED, the bridge stream, two
// owner-only reads, the metrics.
//
// Atlas is a monitored flow, so the feed is built to leave it alone: off by default (no goroutine, the routes answer
// 404); every goroutine recovers from a panic and logs; memory is capped (feedMaxEvents, feedMaxBytes, oldest
// dropped and counted); its own bridge client and its own subscription ("bridge-atlas/feed", a name the client row
// already owns, so the tag loop's cursor is untouched); no database.
//
// Start: Subscribe registers the subscription (a new one begins at the head), Replay streams everything logged up to
// the head, then the live stream follows, each event acked after it is stored. Ingest is idempotent on (source, id),
// so the overlap of the two, a redelivery and a second Replay after a gap change nothing. There is no stream
// (SSE or socket) yet: the page polls /api/feed.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"connectrpc.com/connect"

	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/feed"
)

const (
	feedSub       = bridgeUser + "/feed" // "<client>/<suffix>": the client's own name may be extended
	feedMaxEvents = 50000
	feedMaxBytes  = 32 << 20
	feedPageMax   = 200
)

// feedTypes is the subscription's filter: type prefixes bridge-atlas's client row already covers ("dev.bnei."). Not
// all of fleet.* (session.waiting and resumed carry no thread), and the feed ignores the forge types it has no thread
// for (labels, issues). A changed filter on the existing name is an upsert on the bridge: the cursor stays, Replay
// reads with the stored filter, so the first Replay after the change covers the new types from the start.
var feedTypes = []string{"dev.bnei.alert.", "dev.bnei.deploy.", "dev.bnei.forge.", "dev.bnei.fleet.session.", "dev.bnei.fleet.card.", "dev.bnei.laptop.post."}

// feedStream is the bridge as the feed uses it; Next returns a gap's version (and no event) when the bridge says so.
type feedStream interface {
	Next(ctx context.Context) (ev feed.Raw, gap int64, err error)
	Ack(ctx context.Context, seq int64) error
	ConfirmGap(ctx context.Context, version int64) error
	Replay(ctx context.Context, fn func(feed.Raw)) error // everything logged up to the head, from the start
	Close() error
}

// startFeed turns the feed on when c asks for it; either way mountain_feed_enabled says which.
func startFeed(ctx context.Context, a *App, c Config) {
	a.Met.FeedEnabled(c.Feed)
	if !c.Feed {
		return
	}
	a.Feed = feed.New(feedMaxEvents, feedMaxBytes)
	go guarded("feed metrics", func() { feedMetrics(ctx, a.Feed, a.Met, time.Now(), 15*time.Second) })
	tc, on := tagsFrom(c.Tags)
	if !on {
		slog.Warn("atlas: the feed is on but the bridge is not configured; it stays empty")
		return
	}
	b := newBridgeClient(tc)
	go runFeed(ctx, b.openFeed, a.Feed, sleepCtx)
}

// guarded runs f and logs a panic instead of ending the process.
func guarded(what string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(fmt.Sprintf("atlas: %s: panic: %v\n%s", what, r, debug.Stack()))
		}
	}()
	f()
}

// feedMetrics records the feed's state every `every` until ctx ends.
func feedMetrics(ctx context.Context, f *feed.Feed, met *Metrics, started time.Time, every time.Duration) {
	var sent, sentBad, sentRefused uint64
	for {
		s := f.Stats()
		ref := s.Last
		if ref.IsZero() {
			ref = started
		}
		met.FeedState(s.Events, s.Threads, s.Dropped-sent, s.Bad-sentBad, s.Refused-sentRefused, time.Since(ref).Seconds())
		sent, sentBad, sentRefused = s.Dropped, s.Bad, s.Refused
		if !sleepCtx(ctx, every) {
			return
		}
	}
}

// runFeed keeps one stream open, reconnecting with a backoff from 1 s to a minute, until ctx ends. PermissionDenied
// stops it for good (logged once): the bridge's client row does not let bridge-atlas read that filter or name.
func runFeed(ctx context.Context, open func(context.Context) (feedStream, error), f *feed.Feed, sleep func(context.Context, time.Duration) bool) {
	delay, replay := time.Second, true
	for {
		start := time.Now()
		var n int
		var err error
		guarded("feed stream", func() { n, err = feedOnce(ctx, open, f, &replay) })
		if ctx.Err() != nil {
			return
		}
		if connect.CodeOf(err) == connect.CodePermissionDenied {
			slog.Error(fmt.Sprintf("feed: the bridge refuses the feed's subscription, the feed stops: %v", err))
			return
		}
		if n > 0 || time.Since(start) >= time.Minute {
			delay = time.Second
		}
		slog.Warn(fmt.Sprintf("feed: stream ended, again in %s: %v", delay, err))
		if !sleep(ctx, delay) {
			return
		}
		delay = min(2*delay, time.Minute)
	}
}

// feedOnce is one stream: replay when *replay says so (first start, after a gap, after a replay that failed), then
// live until it fails. It returns how many live events it stored. A panic ends it as an error.
func feedOnce(ctx context.Context, open func(context.Context) (feedStream, error), f *feed.Feed, replay *bool) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	s, err := open(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = s.Close() }()
	for {
		if *replay {
			if err := s.Replay(ctx, func(r feed.Raw) { f.Ingest(r) }); err != nil {
				return n, err
			}
			*replay = false
		}
		ev, gap, err := s.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) { // the bridge ended the stream: what it held back may be gone, so replay on the next
				*replay = true
			}
			return n, err
		}
		if gap != 0 { // retention deleted events we had not acked: what is left is in Replay
			*replay = true
			if err := s.ConfirmGap(ctx, gap); err != nil {
				return n, err
			}
			continue
		}
		f.Ingest(ev) // never panics (it counts a bad event): this one is acked like any other
		if err := s.Ack(ctx, ev.Seq); err != nil {
			return n, err
		}
		n++
	}
}

// The real stream, on the feed's own bridgeClient.

type feedWire struct {
	b *bridgeClient
	s *connect.ServerStreamForClient[bridgev1.SubscribeResponse]
}

func (b *bridgeClient) openFeed(ctx context.Context) (feedStream, error) {
	r, err := bearer(b, &bridgev1.SubscribeRequest{Name: feedSub, Types: feedTypes})
	if err != nil {
		return nil, err
	}
	s, err := b.api.Subscribe(ctx, r)
	if err != nil {
		return nil, b.seen(err)
	}
	return &feedWire{b, s}, nil
}

// feedRaw is e as the feed reads it; nil for an event the bridge flags (uncontracted: ack and skip).
func feedRaw(e *bridgev1.CloudEvent) *feed.Raw {
	if e == nil || len(e.GetFlags()) > 0 {
		return nil
	}
	r := feed.Raw{Seq: e.GetSeq(), Source: e.GetSource(), ID: e.GetId(), Type: e.GetType(), Subject: e.GetSubject(), Data: e.GetData()}
	if e.GetTime() != nil {
		r.Time = e.GetTime().AsTime()
	}
	return &r
}

func (w *feedWire) Next(context.Context) (feed.Raw, int64, error) {
	if !w.s.Receive() {
		if err := w.s.Err(); err != nil {
			return feed.Raw{}, 0, w.b.seen(err)
		}
		return feed.Raw{}, 0, io.EOF
	}
	m := w.s.Msg()
	if v := m.GetGap().GetVersion(); v != 0 {
		return feed.Raw{}, v, nil
	}
	if e := m.GetEvent(); e != nil {
		if r := feedRaw(e); r != nil {
			return *r, 0, nil
		}
		return feed.Raw{Seq: e.GetSeq()}, 0, nil // flagged: stored as nothing (no id), acked
	}
	return feed.Raw{}, 0, errors.New("feed: a message with neither an event nor a gap version")
}

func (w *feedWire) Ack(ctx context.Context, seq int64) error {
	return call(ctx, w.b, w.b.api.Ack, &bridgev1.AckRequest{Subscription: feedSub, Seq: seq})
}

func (w *feedWire) ConfirmGap(ctx context.Context, v int64) error {
	return call(ctx, w.b, w.b.api.ConfirmGap, &bridgev1.ConfirmGapRequest{Subscription: feedSub, Version: v})
}

func (w *feedWire) Replay(ctx context.Context, fn func(feed.Raw)) error {
	r, err := bearer(w.b, &bridgev1.ReplayRequest{Subscription: feedSub, FromSeq: 0})
	if err != nil {
		return err
	}
	s, err := w.b.api.Replay(ctx, r)
	if err != nil {
		return w.b.seen(err)
	}
	defer func() { _ = s.Close() }()
	for s.Receive() {
		if raw := feedRaw(s.Msg().GetEvent()); raw != nil {
			fn(*raw)
		}
	}
	return w.b.seen(s.Err())
}

func (w *feedWire) Close() error { return w.s.Close() }

// The reads. Owner only: routeTable registers them closed, so the gate every other closed route has answers first.

// GET /api/feed and GET /api/feed/{id}. A thread is {id, kind, title, title_quoted, state, opened_at, last_activity_at,
// needs_owner, entries, links}; the list carries only the last entry and no links. Kinds: alert, deploy, pr, session,
// card, laptop. A laptop thread is one session's posts, each an entry that is quoted and markdown with its post_id (a
// correction has correction and corrects, the corrected post corrected_by; the post whose needs_you list stands has
// asks_owner and needs_list), closed 24 h after the last. needs_by is "session" while that list stands, 7 days and
// beyond the close. A PR thread also holds claim entries for posts about it (claim, thread); the list never shows one
// as a thread's entry. In the list an entry's text is cut to 400 characters with has_more set; the thread has it whole.
// State is open or closed. Entries are {time, actor, text, quoted}: a quoted entry or title holds text from
// outside (a PR's title, branch, comment or review, an agent's session title, a card's subject, an alert's words),
// and the page shows it as a quote. Links are {kind, ref, quoted}: "previous" and "generator" (alerts), "revision"
// (deploys), "run" (a PR's CI run, a URL), "deploy" (a PR to the rollout of its merge commit, ref a thread id) and
// "pull_request" (that rollout back to the PR; only a revision new in the sync counts). A session thread has no link
// to its PR: no event carries one. A PR closes on any payload that says closed and opens again only on reopened; a successful CI run is not kept.
// needs_owner: an alert that fires, an unhealthy deploy, a session or card thread holding an open card that has not
// expired (14 days after it opened when it has no expiry), and then the thread shows open even if its session ended;
// never a PR.
func (a *App) serveFeed(w http.ResponseWriter, r *http.Request) {
	if a.Feed == nil {
		http.NotFound(w, r)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > feedPageMax {
			http.Error(w, "limit is 1 to "+strconv.Itoa(feedPageMax), http.StatusBadRequest)
			return
		}
		limit = n
	}
	page, next, err := a.Feed.ListCtx(r.Context(), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		feedErr(w, r, err)
		return
	}
	if page == nil {
		page = []feed.Thread{}
	}
	feedJSON(w, map[string]any{"threads": page, "next_cursor": next, "dropped": a.Feed.Stats().Dropped})
}

func (a *App) serveFeedThread(w http.ResponseWriter, r *http.Request) {
	if a.Feed == nil {
		http.NotFound(w, r)
		return
	}
	t, ok, err := a.Feed.GetCtx(r.Context(), r.PathValue("id"))
	if err != nil {
		feedErr(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	feedJSON(w, t)
}

// feedErr answers a failed read: a bad cursor is the caller's, a fold that failed or a request that ended is not.
func feedErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case r.Context().Err() != nil:
		// the client left: nothing to say
	case errors.Is(err, feed.ErrBadCursor):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "feed unavailable", http.StatusInternalServerError)
	}
}

func feedJSON(w http.ResponseWriter, v any) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", "application/json")
	_, _ = w.Write(mustJSON(v))
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/bridgev1/bridgev1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/MohammadBnei/mountain/internal/feed"
)

var ft0 = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func fraw(seq int64, typ, fp string, m int) feed.Raw {
	return feed.Raw{Seq: seq, Source: "alertmanager", ID: fmt.Sprintf("%s:%s:s", fp, typ), Type: "dev.bnei.alert." + typ, Subject: "alert/" + fp,
		Time: ft0.Add(time.Duration(m) * time.Minute), Data: []byte(`{"fingerprint":"` + fp + `","startsAt":"s"}`)}
}

// fakeFeed plays a script: replay events, then live items (an event, a gap, or an error), recording what it was asked.
type fakeFeed struct {
	replay []feed.Raw
	live   []any // feed.Raw, int64 (a gap's version) or error
	acked  []int64
	gaps   []int64
	nrep   int
	panics bool
}

func (s *fakeFeed) Next(ctx context.Context) (feed.Raw, int64, error) {
	if s.panics {
		panic("boom")
	}
	if len(s.live) == 0 {
		<-ctx.Done()
		return feed.Raw{}, 0, ctx.Err()
	}
	x := s.live[0]
	s.live = s.live[1:]
	switch v := x.(type) {
	case feed.Raw:
		return v, 0, nil
	case int64:
		return feed.Raw{}, v, nil
	}
	return feed.Raw{}, 0, x.(error)
}
func (s *fakeFeed) Ack(_ context.Context, q int64) error { s.acked = append(s.acked, q); return nil }
func (s *fakeFeed) ConfirmGap(_ context.Context, v int64) error {
	s.gaps = append(s.gaps, v)
	return nil
}
func (s *fakeFeed) Replay(_ context.Context, fn func(feed.Raw)) error {
	s.nrep++
	for _, r := range s.replay {
		fn(r)
	}
	return nil
}
func (s *fakeFeed) Close() error { return nil }

func TestFeedOnce(t *testing.T) {
	f := feed.New(100, 1<<20)
	s := &fakeFeed{
		replay: []feed.Raw{fraw(1, "firing", "a", 0), fraw(2, "firing", "b", 1)},
		live:   []any{fraw(2, "firing", "b", 1), fraw(3, "resolved", "a", 2), int64(1), fraw(4, "firing", "c", 3), io.EOF}, // 2 again: the overlap
	}
	replay := true
	n, err := feedOnce(context.Background(), func(context.Context) (feedStream, error) { return s, nil }, f, &replay)
	if err != io.EOF || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if st := f.Stats(); st.Events != 4 || st.Threads != 3 {
		t.Errorf("%+v", st)
	}
	if fmt.Sprint(s.acked) != "[2 3 4]" || fmt.Sprint(s.gaps) != "[1]" || s.nrep != 2 { // a gap replays again, once
		t.Errorf("acked %v gaps %v replays %d", s.acked, s.gaps, s.nrep)
	}
	if !replay { // the stream ended with io.EOF: the next one replays
		t.Error("replay not due after EOF")
	}
}

func TestRunFeedSurvivesPanicAndStopsOnDenied(t *testing.T) {
	f := feed.New(100, 1<<20)
	var opens atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := func(context.Context) (feedStream, error) {
		switch opens.Add(1) {
		case 1:
			return &fakeFeed{panics: true}, nil // Next panics: the loop logs it and connects again
		case 2:
			return &fakeFeed{live: []any{fraw(1, "firing", "a", 0), connect.NewError(connect.CodePermissionDenied, errors.New("no"))}}, nil
		}
		t.Error("opened after a refusal")
		return nil, errors.New("again")
	}
	done := make(chan struct{})
	go func() {
		runFeed(ctx, open, f, func(context.Context, time.Duration) bool { return true })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runFeed did not stop on PermissionDenied")
	}
	if f.Stats().Events != 1 || opens.Load() != 2 {
		t.Errorf("events %d opens %d", f.Stats().Events, opens.Load())
	}
}

func TestFeedMetrics(t *testing.T) {
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	f := feed.New(1, 1<<20)
	f.Ingest(fraw(1, "firing", "a", 0))
	f.Ingest(fraw(2, "firing", "b", 1)) // evicts seq 1: the floor is 1
	f.Ingest(fraw(1, "firing", "c", 2)) // refused: at the floor
	f.Ingest(feed.Raw{Seq: 9, Source: "s", ID: "bad", Type: "dev.bnei.alert.firing", Time: ft0, Data: []byte("{")})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.FeedEnabled(true)
	go feedMetrics(ctx, f, m, time.Now(), time.Hour)
	waitScrape(t, func() string { _, _, b := scrape(t, m); return b },
		"mountain_feed_enabled 1\n", "mountain_feed_events 1\n", "mountain_feed_threads 1\n", "mountain_feed_dropped_total 1\n",
		"mountain_feed_bad_events_total 1\n", "mountain_feed_refused_events_total 1\n", "mountain_feed_last_event_age_seconds ")
}

func TestFeedRoutes(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	if r := a.serveReq("GET", "/api/feed", []*http.Cookie{admin}, "Origin", origin); r.StatusCode != http.StatusNotFound {
		t.Errorf("feed off: %d, want 404", r.StatusCode)
	}
	a.Feed = feed.New(100, 1<<20)
	a.Feed.Ingest(fraw(1, "firing", "a", 0))
	a.Feed.Ingest(fraw(2, "firing", "b", 1))
	if r := a.serveReq("GET", "/api/feed", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous: %d", r.StatusCode)
	}
	r := a.serveReq("GET", "/api/feed?limit=1", []*http.Cookie{admin}, "Origin", origin)
	var out struct {
		Threads    []feed.Thread `json:"threads"`
		NextCursor string        `json:"next_cursor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil || r.StatusCode != 200 || len(out.Threads) != 1 || out.NextCursor == "" || r.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("%d %+v %v", r.StatusCode, out, err)
	}
	id := out.Threads[0].ID
	var th feed.Thread
	r = a.serveReq("GET", "/api/feed/"+id, []*http.Cookie{admin}, "Origin", origin)
	if err := json.NewDecoder(r.Body).Decode(&th); err != nil || th.ID != id {
		t.Errorf("%d %+v %v", r.StatusCode, th, err)
	}
	for path, want := range map[string]int{"/api/feed/nope": 404, "/api/feed?limit=0": 400, "/api/feed?cursor=x": 400} {
		if r := a.serveReq("GET", path, []*http.Cookie{admin}, "Origin", origin); r.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, r.StatusCode, want)
		}
	}
}

func TestFeedConfig(t *testing.T) {
	for v, want := range map[string]bool{"": false, "true": true, "1": true, "false": false, "nonsense": false} {
		c, _ := loadConfig(nil, func(k string) string {
			if k == "FEED_ENABLED" {
				return v
			}
			return ""
		})
		if c.Feed != want {
			t.Errorf("FEED_ENABLED=%q: %v", v, c.Feed)
		}
	}
}

// feedBridge is a bridge that records what the feed asks of it and plays a script.
type feedBridge struct {
	bridgev1connect.UnimplementedBridgeServiceHandler
	mu        sync.Mutex
	subs      []*bridgev1.SubscribeRequest
	replays   []*bridgev1.ReplayRequest
	acks      []*bridgev1.AckRequest
	gaps      []*bridgev1.ConfirmGapRequest
	auths     []string
	replay    []*bridgev1.CloudEvent
	live      []*bridgev1.SubscribeResponse
	replayErr error
}

func (b *feedBridge) Subscribe(ctx context.Context, r *connect.Request[bridgev1.SubscribeRequest], st *connect.ServerStream[bridgev1.SubscribeResponse]) error {
	b.mu.Lock()
	b.subs, b.auths = append(b.subs, r.Msg), append(b.auths, r.Header().Get("Authorization"))
	live := b.live
	b.mu.Unlock()
	_ = st.Send(nil) // headers: the client's call returns once registered
	for _, m := range live {
		if st.Send(m) != nil {
			return nil
		}
	}
	<-ctx.Done()
	return nil
}

func (b *feedBridge) Replay(_ context.Context, r *connect.Request[bridgev1.ReplayRequest], st *connect.ServerStream[bridgev1.ReplayResponse]) error {
	b.mu.Lock()
	b.replays = append(b.replays, r.Msg)
	err := b.replayErr
	b.mu.Unlock()
	if err != nil {
		return err
	}
	for _, e := range b.replay {
		if err := st.Send(&bridgev1.ReplayResponse{Event: e}); err != nil {
			return err
		}
	}
	return nil
}

func (b *feedBridge) Ack(_ context.Context, r *connect.Request[bridgev1.AckRequest]) (*connect.Response[bridgev1.AckResponse], error) {
	b.mu.Lock()
	b.acks = append(b.acks, r.Msg)
	b.mu.Unlock()
	return connect.NewResponse(&bridgev1.AckResponse{}), nil
}

func (b *feedBridge) ConfirmGap(_ context.Context, r *connect.Request[bridgev1.ConfirmGapRequest]) (*connect.Response[bridgev1.ConfirmGapResponse], error) {
	b.mu.Lock()
	b.gaps = append(b.gaps, r.Msg)
	b.mu.Unlock()
	return connect.NewResponse(&bridgev1.ConfirmGapResponse{}), nil
}

// start serves b and returns the tag-loop-style env pointing at it, with a token file.
func (b *feedBridge) start(t *testing.T) tagsEnv {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(bridgev1connect.NewBridgeServiceHandler(b))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	tok := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tok, []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return tagsEnv{BridgeURL: srv.URL, TokenFile: tok}
}

func cloud(seq int64, typ, fp string, m int, flags ...string) *bridgev1.CloudEvent {
	r := fraw(seq, typ, fp, m)
	return &bridgev1.CloudEvent{Seq: seq, Id: r.ID, Source: r.Source, Type: r.Type, Subject: r.Subject, Data: r.Data, Time: timestamppb.New(r.Time), Flags: flags}
}

func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out: %s", what)
}

func (b *feedBridge) acked() (seqs []int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, a := range b.acks {
		seqs = append(seqs, a.Seq)
	}
	return
}

// TestFeedWire: the feed over a real Connect round trip: its own subscription name (never the tag loop's), the types
// it asked for, Replay from the start under that name, every ack and gap confirmation under it, a flagged event acked
// and not stored, the token sent.
func TestFeedWire(t *testing.T) {
	fb := &feedBridge{
		replay: []*bridgev1.CloudEvent{cloud(1, "firing", "a", 0), cloud(2, "firing", "flagged", 1, "uncontracted")},
		live: []*bridgev1.SubscribeResponse{{Event: cloud(3, "firing", "b", 2)}, {Gap: &bridgev1.Gap{FromSeq: 1, ToSeq: 1, Version: 7}},
			{Event: cloud(4, "firing", "flagged2", 3, "uncontracted")}, {Event: cloud(5, "resolved", "a", 4)}},
	}
	env := fb.start(t)
	f := feed.New(100, 1<<20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runFeed(ctx, newBridgeClient(env).openFeed, f, sleepCtx)
	until(t, "5 acks", func() bool { return len(fb.acked()) >= 3 && f.Stats().Events == 3 })
	cancel()

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if feedSub == bridgeUser || feedSub != "bridge-atlas/feed" {
		t.Errorf("feedSub %q must be its own name under the client's, not the tag loop's %q", feedSub, bridgeUser)
	}
	if fb.subs[0].Name != feedSub || !reflect.DeepEqual(fb.subs[0].Types, feedTypes) || len(fb.subs[0].Systems) != 0 {
		t.Errorf("subscribe %+v", fb.subs[0])
	}
	if fb.replays[0].Subscription != feedSub || fb.replays[0].FromSeq != 0 {
		t.Errorf("replay %+v", fb.replays[0])
	}
	for _, a := range fb.acks {
		if a.Subscription != feedSub {
			t.Errorf("ack under %q", a.Subscription)
		}
	}
	if len(fb.gaps) == 0 || fb.gaps[0].Subscription != feedSub || fb.gaps[0].Version != 7 {
		t.Errorf("gaps %+v", fb.gaps)
	}
	if len(fb.replays) < 2 { // the gap made it replay again
		t.Errorf("replays %d", len(fb.replays))
	}
	if fb.auths[0] != "Bearer tok" {
		t.Errorf("auth %q", fb.auths[0])
	}
	var flagged bool
	for _, a := range fb.acks {
		flagged = flagged || a.Seq == 4
	}
	if !flagged {
		t.Errorf("a flagged live event is acked: %v", fb.acks)
	}
	if st := f.Stats(); st.Events != 3 { // a, b and a's resolve; neither flagged event
		t.Errorf("%+v", st)
	}
}

// TestFeedCursorIsolation: the tag loop and the feed use different subscription names on one client, so the feed's
// acks never move the tag loop's cursor.
func TestFeedCursorIsolation(t *testing.T) {
	fb := &feedBridge{}
	env := fb.start(t)
	b := newBridgeClient(env)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts, err := b.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ts.Close() }()
	fs, err := b.openFeed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fs.Close() }()
	if err := fs.Ack(ctx, 9); err != nil {
		t.Fatal(err)
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.subs) != 2 || fb.subs[0].Name != bridgeUser || fb.subs[1].Name != feedSub || fb.subs[0].Name == fb.subs[1].Name ||
		!strings.HasPrefix(fb.subs[1].Name, bridgeUser+"/") || fb.acks[0].Subscription != feedSub {
		t.Errorf("subs %v %v acks %+v", fb.subs[0].Name, fb.subs[1].Name, fb.acks)
	}
}

// TestFeedReplayErrorIsNotSwallowed: a Replay that fails ends the stream with its error, reads nothing live, acks
// nothing, and the replay stays due.
func TestFeedReplayErrorIsNotSwallowed(t *testing.T) {
	fb := &feedBridge{replayErr: connect.NewError(connect.CodeUnavailable, errors.New("down")), live: []*bridgev1.SubscribeResponse{{Event: cloud(3, "firing", "b", 2)}}}
	env := fb.start(t)
	f := feed.New(100, 1<<20)
	replay := true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // a swallowed error would block on the live stream
	defer cancel()
	n, err := feedOnce(ctx, newBridgeClient(env).openFeed, f, &replay)
	if connect.CodeOf(err) != connect.CodeUnavailable || n != 0 || !replay || f.Stats().Events != 0 || len(fb.acked()) != 0 {
		t.Errorf("n=%d err=%v replay=%v events=%d acks=%v", n, err, replay, f.Stats().Events, fb.acked())
	}
}

// TestFeedBadEventDoesNotWedge: an event whose handling panics is counted, acked, and the next one is stored.
func TestFeedBadEventDoesNotWedge(t *testing.T) {
	f := feed.New(100, 1<<20)
	f.SetParse(func(r feed.Raw) (feed.Event, bool) {
		if r.Seq == 2 {
			panic("boom")
		}
		return feed.Parse(r)
	})
	s := &fakeFeed{live: []any{fraw(1, "firing", "a", 0), fraw(2, "firing", "b", 1), fraw(3, "firing", "c", 2), io.EOF}}
	replay := false
	n, _ := feedOnce(context.Background(), func(context.Context) (feedStream, error) { return s, nil }, f, &replay)
	if n != 3 || fmt.Sprint(s.acked) != "[1 2 3]" || f.Stats().Events != 2 || f.Stats().Bad != 1 {
		t.Errorf("n=%d acked=%v %+v", n, s.acked, f.Stats())
	}
}

func scrapeBody(t *testing.T, m *Metrics) string { _, _, b := scrape(t, m); return b }

// TestStartFeed: off by default (nothing runs, nothing is asked of the bridge, the series says 0); on, it connects.
func TestStartFeed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()

	off := &feedBridge{}
	a := &App{Met: m}
	startFeed(ctx, a, Config{Tags: off.start(t)}) // the bridge is configured, FEED_ENABLED is not
	time.Sleep(300 * time.Millisecond)
	off.mu.Lock()
	asked := len(off.subs) + len(off.replays)
	off.mu.Unlock()
	if a.Feed != nil || asked != 0 {
		t.Errorf("feed off: Feed=%v, bridge asked %d times", a.Feed, asked)
	}
	waitScrape(t, func() string { return scrapeBody(t, m) }, "mountain_feed_enabled 0\n")

	// on, but no bridge: a feed that stays empty, no goroutine on the bridge
	a2 := &App{Met: m}
	startFeed(ctx, a2, Config{Feed: true})
	if a2.Feed == nil {
		t.Error("FEED_ENABLED with no bridge: no Feed")
	}

	on := &feedBridge{replay: []*bridgev1.CloudEvent{cloud(1, "firing", "a", 0)}}
	a3 := &App{Met: m}
	startFeed(ctx, a3, Config{Feed: true, Tags: on.start(t)})
	if a3.Feed == nil {
		t.Fatal("FEED_ENABLED: no Feed")
	}
	until(t, "the replayed event", func() bool { return a3.Feed.Stats().Events == 1 })
	waitScrape(t, func() string { return scrapeBody(t, m) }, "mountain_feed_enabled 1\n")
}

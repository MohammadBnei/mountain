package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	if replay {
		t.Error("replay still due")
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
	f.Ingest(fraw(2, "firing", "b", 1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.FeedEnabled(true)
	go feedMetrics(ctx, f, m, time.Now(), time.Hour)
	waitScrape(t, func() string { _, _, b := scrape(t, m); return b },
		"mountain_feed_enabled 1\n", "mountain_feed_events 1\n", "mountain_feed_threads 1\n", "mountain_feed_dropped_total 1\n", "mountain_feed_last_event_age_seconds ")
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

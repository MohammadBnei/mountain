package feed

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cacheTTL: the folded threads are reused this long when nothing was ingested, so a read costs one fold per TTL at
// most. Fold takes the clock (a rollout closes by itself), so the cache cannot live for ever.
const cacheTTL = 5 * time.Second

// Feed holds a bounded set of parsed events, oldest by occurred time dropped first and counted, and the threads
// folded from them. No database: it is rebuilt from the bridge at start. Safe for concurrent use; a fold runs outside
// the lock, so a slow read does not hold up Ingest.
type Feed struct {
	maxEvents, maxBytes int
	now                 func() time.Time
	parse               func(Raw) (Event, bool)
	foldFn              func([]Event, time.Time) []Thread

	mu       sync.Mutex
	evs      map[string][]Event // per kind, by (Time, Seq): an eviction takes the front of one slice, so filling the store is not quadratic
	n        int                // events held
	dead     map[string]int     // per kind: slots before the live front of its slice, zeroed and not yet released
	seen     map[[2]string]bool
	bytes    int
	dropped  uint64
	bad      uint64
	refused  uint64
	badSeen  map[[2]string]bool // bad events already counted (bounded: past maxBadSeen a replay counts them again)
	logged   map[string]bool    // event types whose first failure was logged
	floor    map[string]int64   // per kind: the highest seq evicted, none until one is: an unseen event at or below it came back after its eviction
	count    map[string]int     // per kind: events held
	kbytes   map[string]int     // per kind: bytes held
	last     time.Time          // wall time of the last stored event
	gen      uint64             // changes with evs
	cache    *folded            // immutable once built
	cacheGen uint64
	cacheAt  time.Time
	folding  chan struct{} // closed when the fold in flight ends; nil when none
	folds    int           // folds run, for tests
}

// folded is one fold's answer: shared by every reader, never changed after it is built.
type folded struct {
	threads []Thread
	byID    map[string]int
}

// staleOK: a reader that finds a fold in flight takes the last result when it is younger than this.
const staleOK = 30 * time.Second

// New is a Feed that keeps at most maxEvents events and maxBytes bytes of them.
func New(maxEvents, maxBytes int) *Feed {
	return &Feed{maxEvents: maxEvents, maxBytes: maxBytes, seen: map[[2]string]bool{}, evs: map[string][]Event{}, floor: map[string]int64{}, count: map[string]int{}, kbytes: map[string]int{}, badSeen: map[[2]string]bool{}, logged: map[string]bool{}, now: time.Now, parse: Parse, foldFn: Fold, dead: map[string]int{}}
}

// SetFold replaces Fold, for tests that need one that fails.
func (f *Feed) SetFold(fn func([]Event, time.Time) []Thread) { f.foldFn = fn }

// SetParse replaces Parse, for tests that need a parser that fails.
func (f *Feed) SetParse(p func(Raw) (Event, bool)) { f.parse = p }

func less(a, b Event) int { return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.Seq, b.Seq)) }

// maxBadSeen bounds the memory of counted bad events.
const maxBadSeen = 4096

// handled: the types Parse reads; a failure to parse one of them is a bad event, any other type is not the feed's.
func handled(r Raw) bool {
	switch typ := strings.TrimPrefix(r.Type, "dev.bnei."); {
	case typ == "alert.firing", typ == "alert.resolved", typ == "deploy.synced", typ == "deploy.health", typ == "deploy.flapping",
		typ == "fleet.session.opened", typ == "fleet.session.turn_ended", typ == "fleet.session.ended", strings.HasPrefix(typ, "fleet.card."):
		return true
	case typ == "forge.issue_comment.created": // a comment on an issue has no thread here
		return strings.Contains(r.Subject, "/pr/")
	default:
		return forgeTypes[typ]
	}
}

// failed counts r as bad once, however often it comes (a Replay brings it again), and logs the first failure of its
// type since the start, not each.
func (f *Feed) failed(r Raw, why string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := [2]string{r.Source, r.ID}
	if f.badSeen[k] {
		return
	}
	if len(f.badSeen) < maxBadSeen {
		f.badSeen[k] = true
	}
	f.bad++
	if !f.logged[r.Type] {
		f.logged[r.Type] = true
		slog.Error(fmt.Sprintf("feed: %s event %d: %s (counted in mountain_feed_bad_events_total; this type is not logged again)", r.Type, r.Seq, why))
	}
}

// Ingest stores r when it is an event the feed has a thread for and not one it already has (source, id); it says
// whether it stored it. Over a cap the oldest events go, r itself if it is the oldest. An unseen event whose seq is at
// or below the highest seq evicted is refused and counted (Stats.Refused): after an eviction it may be one that
// came back in a second Replay, and the feed cannot tell it from a new one. Below that floor nothing is refused, so a
// seq the bridge reuses after a restore, or a hole a Replay fills, is stored. An event of a type the feed reads that
// does not parse, or a panic, is counted once (Stats.Bad), never raised: one bad event must not stop the stream.
func (f *Feed) Ingest(r Raw) (stored bool) {
	defer func() {
		if p := recover(); p != nil {
			f.failed(r, fmt.Sprintf("panic: %v", p))
			stored = false
		}
	}()
	e, ok := f.parse(r)
	if !ok {
		if handled(r) {
			f.failed(r, "does not parse")
		}
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	k := [2]string{e.Source, e.ID}
	if f.seen[k] {
		return false
	}
	kind := kindOf(e)
	if e.Seq != 0 && e.Seq <= f.floor[kind] {
		f.refused++
		return false
	}
	f.seen[k] = true
	i, _ := slices.BinarySearchFunc(f.evs[kind], e, less)
	f.evs[kind] = slices.Insert(f.evs[kind], i, e)
	f.n++
	f.bytes += e.size()
	f.count[kind]++
	f.kbytes[kind] += e.size()
	f.gen++
	f.last = time.Now()
	for f.n > f.maxEvents || f.bytes > f.maxBytes {
		// The kind holding the most (events when that cap is over, else bytes) loses its oldest event, so a busy
		// kind (PR traffic, session turns) cannot push the quiet ones (alerts, rollouts) out.
		by := f.count
		if f.n <= f.maxEvents {
			by = f.kbytes
		}
		big := ""
		for k, n := range by {
			if n > 0 && (big == "" || n > by[big] || (n == by[big] && k < big)) {
				big = k
			}
		}
		old := f.evs[big][0]
		f.evs[big][0] = Event{} // let the event's strings go while the array lives
		f.evs[big] = f.evs[big][1:]
		if f.dead[big]++; f.dead[big] >= 64 && f.dead[big] > len(f.evs[big]) {
			f.evs[big], f.dead[big] = slices.Clone(f.evs[big]), 0 // mostly dead front: release the array
		}
		f.n--
		f.bytes -= old.size()
		f.count[big]--
		f.kbytes[big] -= old.size()
		delete(f.seen, [2]string{old.Source, old.ID})
		f.floor[big] = max(f.floor[big], old.Seq)
		f.dropped++
	}
	return true
}

// kindOf is the kind an event is capped as: alert, deploy, forge, session or card.
func kindOf(e Event) string {
	p := strings.Split(e.Type, ".")
	if p[0] == "fleet" && len(p) > 1 {
		return p[1]
	}
	return p[0]
}

// all is every held event, kind by kind (Fold sorts them). The caller holds f.mu.
func (f *Feed) all() []Event {
	out := make([]Event, 0, f.n)
	for _, evs := range f.evs {
		out = append(out, evs...)
	}
	return out
}

// ErrBadCursor is List's answer to a cursor it did not make.
var ErrBadCursor = errors.New("bad cursor")

// errFold is what a reader gets when the fold it waited for failed and there is no earlier result.
var errFold = errors.New("feed: the fold failed")

// threads is the folded answer now. One fold runs at a time: a reader that finds the cache stale starts it, one that
// arrives meanwhile waits for it (or for ctx), or takes the last result at once when that is younger than staleOK.
// The result is shared and immutable: readers must not change it. A fold that panics is logged once, counted as a bad
// event, and ends like any other: waiters get the last good result, or errFold when there is none, and the next read
// tries again.
func (f *Feed) threads(ctx context.Context) (*folded, error) {
	f.mu.Lock()
	now := f.now()
	if c := f.cache; c != nil && f.cacheGen == f.gen && now.Sub(f.cacheAt) < cacheTTL {
		f.mu.Unlock()
		return c, nil
	}
	if w := f.folding; w != nil {
		if c := f.cache; c != nil && now.Sub(f.cacheAt) < staleOK {
			f.mu.Unlock()
			return c, nil
		}
		f.mu.Unlock()
		select {
		case <-w:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		f.mu.Lock()
		c := f.cache
		f.mu.Unlock()
		if c == nil {
			return nil, errFold
		}
		return c, nil
	}
	done := make(chan struct{})
	f.folding = done
	evs, gen := f.all(), f.gen
	f.folds++
	f.mu.Unlock()
	c := f.fold(evs, gen, now, done)
	if c == nil {
		f.mu.Lock()
		c = f.cache
		f.mu.Unlock()
		if c == nil {
			return nil, errFold
		}
	}
	return c, nil
}

// fold runs one flight and ends it, whatever happens: the deferred release clears f.folding and wakes the waiters even
// when Fold panics; the result is nil then.
func (f *Feed) fold(evs []Event, gen uint64, now time.Time, done chan struct{}) (c *folded) {
	defer func() {
		r := recover()
		f.mu.Lock()
		if r != nil {
			c = nil
			f.bad++
			if !f.logged["fold"] {
				f.logged["fold"] = true
				slog.Error(fmt.Sprintf("feed: the fold panicked (counted in mountain_feed_bad_events_total; not logged again): %v\n%s", r, debug.Stack()))
			}
		} else {
			f.cache, f.cacheGen, f.cacheAt = c, gen, now
		}
		f.folding = nil
		f.mu.Unlock()
		close(done)
	}()
	th := f.foldFn(evs, now)
	c = &folded{threads: th, byID: make(map[string]int, len(th))}
	for i, t := range th {
		c.byID[t.ID] = i
	}
	return c
}

// List is up to limit threads, newest activity first, after cursor ("" = from the top); next is "" at the end. A
// thread carries only its last entry and no links: Get has the rest.
func (f *Feed) List(limit int, cursor string) (page []Thread, next string, err error) {
	return f.ListCtx(context.Background(), limit, cursor)
}

// ListCtx is List that gives up when ctx ends while it waits for a fold.
func (f *Feed) ListCtx(ctx context.Context, limit int, cursor string) (page []Thread, next string, err error) {
	var at int64
	var id string
	if cursor != "" {
		a, i, ok := strings.Cut(cursor, ".")
		if at, err = strconv.ParseInt(a, 10, 64); err != nil || !ok || i == "" {
			return nil, "", ErrBadCursor
		}
		id = i
	}
	c, err := f.threads(ctx)
	if err != nil {
		return nil, "", err
	}
	all := c.threads
	start := 0
	if cursor != "" { // newest first, ties by id descending: the first thread strictly after the cursor
		start = sort.Search(len(all), func(i int) bool {
			l := all[i].LastActivityAt.UnixNano()
			return l < at || (l == at && all[i].ID < id)
		})
	}
	for i := start; i < len(all); i++ {
		t := all[i] // a copy of this one thread, not of the list
		if len(page) == limit {
			last := page[len(page)-1]
			return page, fmt.Sprintf("%d.%s", last.LastActivityAt.UnixNano(), last.ID), nil
		}
		n := len(t.Entries)
		t.Entries, t.Links = t.Entries[max(0, n-1):n:n], nil // capped, so an append by a caller cannot write into the fold's array
		page = append(page, t)
	}
	return page, "", nil
}

// Get is the thread with id, whole.
func (f *Feed) Get(id string) (Thread, bool) {
	t, ok, _ := f.GetCtx(context.Background(), id)
	return t, ok
}

// GetCtx is Get that gives up when ctx ends while it waits for a fold, or when the fold failed (err).
func (f *Feed) GetCtx(ctx context.Context, id string) (Thread, bool, error) {
	c, err := f.threads(ctx)
	if err != nil {
		return Thread{}, false, err
	}
	if i, ok := c.byID[id]; ok {
		return c.threads[i], true, nil
	}
	return Thread{}, false, nil
}

// Stats is what the metrics read.
type Stats struct {
	Events, Threads       int
	Bytes                 int
	Dropped, Bad, Refused uint64
	Last                  time.Time // zero until an event is stored
}

func (f *Feed) Stats() Stats {
	th := 0
	if c, err := f.threads(context.Background()); err == nil {
		th = len(c.threads)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return Stats{f.n, th, f.bytes, f.dropped, f.bad, f.refused, f.last}
}

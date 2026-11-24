package feed

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
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

	mu       sync.Mutex
	evs      []Event // by (Time, Seq)
	seen     map[[2]string]bool
	bytes    int
	dropped  uint64
	bad      uint64
	refused  uint64
	badSeen  map[[2]string]bool // bad events already counted (bounded: past maxBadSeen a replay counts them again)
	logged   map[string]bool    // event types whose first failure was logged
	floor    int64              // the highest seq evicted, 0 until one is: an unseen event at or below it came back after its eviction
	last     time.Time          // wall time of the last stored event
	gen      uint64             // changes with evs
	cache    []Thread
	cacheGen uint64
	cacheAt  time.Time
}

// New is a Feed that keeps at most maxEvents events and maxBytes bytes of them.
func New(maxEvents, maxBytes int) *Feed {
	return &Feed{maxEvents: maxEvents, maxBytes: maxBytes, seen: map[[2]string]bool{}, badSeen: map[[2]string]bool{}, logged: map[string]bool{}, now: time.Now, parse: Parse}
}

// SetParse replaces Parse, for tests that need a parser that fails.
func (f *Feed) SetParse(p func(Raw) (Event, bool)) { f.parse = p }

func less(a, b Event) int { return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.Seq, b.Seq)) }

// maxBadSeen bounds the memory of counted bad events.
const maxBadSeen = 4096

// handled: the types Parse reads; a failure to parse one of them is a bad event, any other type is not the feed's.
func handled(typ string) bool {
	switch strings.TrimPrefix(typ, "dev.bnei.") {
	case "alert.firing", "alert.resolved", "deploy.synced", "deploy.health", "deploy.flapping":
		return true
	}
	return false
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
		if handled(r.Type) {
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
	if e.Seq != 0 && e.Seq <= f.floor {
		f.refused++
		return false
	}
	f.seen[k] = true
	i, _ := slices.BinarySearchFunc(f.evs, e, less)
	f.evs = slices.Insert(f.evs, i, e)
	f.bytes += e.size()
	f.gen++
	f.last = time.Now()
	for len(f.evs) > f.maxEvents || f.bytes > f.maxBytes {
		old := f.evs[0]
		f.evs = slices.Delete(f.evs, 0, 1)
		f.bytes -= old.size()
		delete(f.seen, [2]string{old.Source, old.ID})
		f.floor = max(f.floor, old.Seq)
		f.dropped++
	}
	return true
}

// folded is the threads now. Readers must not change what it returns.
func (f *Feed) folded() []Thread {
	f.mu.Lock()
	now := f.now()
	if f.cache != nil && f.cacheGen == f.gen && now.Sub(f.cacheAt) < cacheTTL {
		t := f.cache
		f.mu.Unlock()
		return t
	}
	evs, gen := slices.Clone(f.evs), f.gen
	f.mu.Unlock()
	t := Fold(evs, now)
	if t == nil {
		t = []Thread{}
	}
	f.mu.Lock()
	if gen == f.gen {
		f.cache, f.cacheGen, f.cacheAt = t, gen, now
	}
	f.mu.Unlock()
	return t
}

// List is up to limit threads, newest activity first, after cursor ("" = from the top); next is "" at the end. A
// thread carries only its last entry and no links: Get has the rest.
func (f *Feed) List(limit int, cursor string) (page []Thread, next string, err error) {
	var at int64
	var id string
	if cursor != "" {
		a, i, ok := strings.Cut(cursor, ".")
		if at, err = strconv.ParseInt(a, 10, 64); err != nil || !ok || i == "" {
			return nil, "", fmt.Errorf("bad cursor")
		}
		id = i
	}
	for _, t := range f.folded() {
		if cursor != "" && !(t.LastActivityAt.UnixNano() < at || (t.LastActivityAt.UnixNano() == at && t.ID < id)) {
			continue
		}
		if len(page) == limit {
			last := page[len(page)-1]
			return page, fmt.Sprintf("%d.%s", last.LastActivityAt.UnixNano(), last.ID), nil
		}
		t.Entries, t.Links = t.Entries[max(0, len(t.Entries)-1):], nil
		page = append(page, t)
	}
	return page, "", nil
}

// Get is the thread with id, whole.
func (f *Feed) Get(id string) (Thread, bool) {
	for _, t := range f.folded() {
		if t.ID == id {
			return t, true
		}
	}
	return Thread{}, false
}

// Stats is what the metrics read.
type Stats struct {
	Events, Threads       int
	Bytes                 int
	Dropped, Bad, Refused uint64
	Last                  time.Time // zero until an event is stored
}

func (f *Feed) Stats() Stats {
	th := len(f.folded())
	f.mu.Lock()
	defer f.mu.Unlock()
	return Stats{len(f.evs), th, f.bytes, f.dropped, f.bad, f.refused, f.last}
}

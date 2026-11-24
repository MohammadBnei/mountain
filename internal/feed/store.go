package feed

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Feed holds a bounded set of parsed events, oldest by occurred time dropped first and counted, and the threads
// folded from them. No database: it is rebuilt from the bridge at start. Safe for concurrent use.
type Feed struct {
	maxEvents, maxBytes int

	mu      sync.Mutex
	evs     []Event // by (Time, Seq)
	seen    map[[2]string]bool
	bytes   int
	dropped uint64
	last    time.Time // wall time of the last stored event
	threads []Thread  // Fold of evs; nil = stale
}

// New is a Feed that keeps at most maxEvents events and maxBytes bytes of them.
func New(maxEvents, maxBytes int) *Feed {
	return &Feed{maxEvents: maxEvents, maxBytes: maxBytes, seen: map[[2]string]bool{}}
}

func less(a, b Event) int { return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.Seq, b.Seq)) }

// Ingest stores r when it is an event the feed has a thread for and not one it already has (source, id); it says
// whether it stored it. Over a cap the oldest events go, r itself if it is the oldest.
func (f *Feed) Ingest(r Raw) bool {
	e, ok := Parse(r)
	if !ok {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	k := [2]string{e.Source, e.ID}
	if f.seen[k] {
		return false
	}
	f.seen[k] = true
	i, _ := slices.BinarySearchFunc(f.evs, e, less)
	f.evs = slices.Insert(f.evs, i, e)
	f.bytes += e.size()
	f.threads, f.last = nil, time.Now()
	for len(f.evs) > f.maxEvents || f.bytes > f.maxBytes {
		old := f.evs[0]
		f.evs = slices.Delete(f.evs, 0, 1)
		f.bytes -= old.size()
		delete(f.seen, [2]string{old.Source, old.ID})
		f.dropped++
	}
	return true
}

// folded is the threads, cached until the next Ingest. Callers hold f.mu.
func (f *Feed) folded() []Thread {
	if f.threads == nil {
		f.threads = Fold(f.evs)
	}
	return f.threads
}

// List is up to limit threads, newest activity first, after cursor ("" = from the top); next is "" at the end. Each
// thread carries only its last entry: Get has the rest.
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
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.folded() {
		if cursor != "" && !(t.LastActivityAt.UnixNano() < at || (t.LastActivityAt.UnixNano() == at && t.ID < id)) {
			continue
		}
		if len(page) == limit {
			last := page[len(page)-1]
			return page, fmt.Sprintf("%d.%s", last.LastActivityAt.UnixNano(), last.ID), nil
		}
		t.Entries = t.Entries[max(0, len(t.Entries)-1):]
		page = append(page, t)
	}
	return page, "", nil
}

// Get is the thread with id, whole.
func (f *Feed) Get(id string) (Thread, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.folded() {
		if t.ID == id {
			return t, true
		}
	}
	return Thread{}, false
}

// Stats is what the metrics read.
type Stats struct {
	Events, Threads int
	Dropped         uint64
	Last            time.Time // zero until an event is stored
}

func (f *Feed) Stats() Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Stats{len(f.evs), len(f.folded()), f.dropped, f.last}
}

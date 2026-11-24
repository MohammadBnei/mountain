package feed

import (
	"container/heap"
	"slices"
	"time"
)

// maxPostBytes is what laptop posts may hold together, apart from the cap every other kind shares: a post is the
// biggest event the feed keeps, and posts never push another kind out.
const maxPostBytes = 4 << 20

// sessionPosts is one session's posts, by (time, seq).
type sessionPosts struct {
	id    string
	evs   []Event
	bytes int
	ask   int64 // the seq of its latest post that has needs_you, -1 when none: that post is kept
	hi    int   // its place in the heap
}

// kept is the one post a session's eviction spares: its latest needs_you (by seq) when that is an ask, not a clear,
// and still standing at now.
func (s *sessionPosts) kept(e Event, now time.Time) bool {
	return e.NeedsSet && e.Seq == s.ask && len(e.NeedsList) > 0 && now.Sub(e.Time) < NeedsLife
}

// evictable is the index of the oldest post that may go, -1 when only the kept one is left.
func (s *sessionPosts) evictable(now time.Time) int {
	return slices.IndexFunc(s.evs, func(e Event) bool { return !s.kept(e, now) })
}

// postHeap orders sessions by the bytes they hold, the most first.
type postHeap []*sessionPosts

func (h postHeap) Len() int { return len(h) }
func (h postHeap) Less(i, j int) bool {
	return h[i].bytes > h[j].bytes || (h[i].bytes == h[j].bytes && h[i].id < h[j].id)
}
func (h postHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].hi, h[j].hi = i, j
}
func (h *postHeap) Push(x any) {
	s := x.(*sessionPosts)
	s.hi = len(*h)
	*h = append(*h, s)
}
func (h *postHeap) Pop() any {
	old := *h
	s := old[len(old)-1]
	*h = old[:len(old)-1]
	return s
}

// posts holds the laptop posts, session by session. Over its cap, the session holding the most bytes loses its oldest
// post, except the latest one that has needs_you; only when every session is down to that one does it go too.
type posts struct {
	cap   int
	by    map[string]*sessionPosts
	h     postHeap
	n     int
	bytes int
}

func newPosts() *posts { return &posts{cap: maxPostBytes, by: map[string]*sessionPosts{}} }

func (p *posts) add(e Event) {
	s := p.by[e.Key]
	if s == nil {
		s = &sessionPosts{id: e.Key, ask: -1}
		p.by[e.Key] = s
		heap.Push(&p.h, s)
	}
	i, _ := slices.BinarySearchFunc(s.evs, e, less)
	s.evs = slices.Insert(s.evs, i, e)
	s.bytes += e.size()
	if e.NeedsSet && e.Seq > s.ask {
		s.ask = e.Seq
	}
	p.n++
	p.bytes += e.size()
	heap.Fix(&p.h, s.hi)
}

// evict takes one post out and returns it; the caller holds p.bytes over its cap. A post that is no longer the kept
// one (a clear, or an ask past NeedsLife) is like any other.
func (p *posts) evict(now time.Time) Event {
	s := p.h[0]
	i := s.evictable(now)
	if i < 0 { // the biggest session holds only its kept post: take the biggest one that holds more
		var best *sessionPosts
		for _, c := range p.h {
			if j := c.evictable(now); j >= 0 && (best == nil || c.bytes > best.bytes || (c.bytes == best.bytes && c.id < best.id)) {
				best, i = c, j
			}
		}
		if best == nil { // every session is down to its kept post
			best, i = s, 0
		}
		s = best
	}
	old := s.evs[i]
	s.evs[i] = Event{}
	s.evs = slices.Delete(s.evs, i, i+1)
	s.bytes -= old.size()
	p.n--
	p.bytes -= old.size()
	if old.NeedsSet && old.Seq == s.ask { // only in the last resort: the next latest becomes the kept one
		s.ask = -1
		for _, e := range s.evs {
			if e.NeedsSet && e.Seq > s.ask {
				s.ask = e.Seq
			}
		}
	}
	if len(s.evs) == 0 {
		heap.Remove(&p.h, s.hi)
		delete(p.by, s.id)
	} else {
		heap.Fix(&p.h, s.hi)
	}
	return old
}

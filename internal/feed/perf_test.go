package feed

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// mixed is n raw events of every kind the feed reads, in time order with distinct ids, built from the same payloads
// the other tests use.
func mixed(t testing.TB, n int) []Raw {
	pr := forge(t.(*testing.T), 0, "pull_request_synchronized", "pull_request.synchronized", prSubj, "x", 0)
	rs := make([]Raw, 0, n)
	for i := 0; i < n; i++ {
		m := i / 20
		var r Raw
		switch i % 5 {
		case 0:
			r = pr
			r.ID = fmt.Sprintf("d%d", i)
			r.Subject = fmt.Sprintf("forge/MohammadBnei/editable-blog/pr/%d", 1+i%300)
			r.Time = at(m)
		case 1:
			r = sess(0, "turn_ended", m, fmt.Sprintf(`,"seq":%d,"title":"work %d"`, i, i%500))
			r.ID = fmt.Sprintf("session/%s/turn/%d", sid, i)
			r.Subject = fmt.Sprintf("fleet/session/s%d", i%400)
			r.Data = []byte(fmt.Sprintf(`{"session_id":"s%d","kind":"repo","title":"work %d","seq":%d}`, i%400, i%500, i))
		case 2:
			r = alertRaw(0, "firing", fmt.Sprintf("fp%d", i%700), sa, i, m, "")
		case 3:
			r = synced(0, fmt.Sprintf("app%d", i%60), m)
			r.ID = fmt.Sprintf("deploy.synced/app/%d", i)
		default:
			r = cardRaw(0, "opened", fmt.Sprintf("s%d", i%400), fmt.Sprintf("dec%d", i), m, "")
		}
		r.Seq, r.Time = int64(i+1), at(m)
		rs = append(rs, r)
	}
	return rs
}

// Filling the store past its caps is linear in the events: 60,000 into a 50,000-event store took 15 s when every
// eviction shifted the whole slice.
func TestFillIsNotQuadratic(t *testing.T) {
	rs := mixed(t, 60000)
	pre := parse(t, rs...) // parsing is not what is timed (and the race detector slows it most)
	f := New(50000, 32<<20)
	f.SetParse(func(r Raw) (Event, bool) { return pre[r.Seq-1], true })
	start := time.Now()
	for _, r := range rs {
		f.Ingest(r)
	}
	d := time.Since(start)
	t.Logf("60000 events in %v, %+v", d, f.Stats().Events)
	if d > 3*time.Second {
		t.Errorf("filling took %v", d)
	}
}

// 64 readers of a stale, full store: one fold, one shared result, and a peak heap far under what 64 folds took.
func TestConcurrentReadsFoldOnce(t *testing.T) {
	f := New(50000, 32<<20)
	for _, r := range mixed(t, 60000) {
		f.Ingest(r)
	}
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			peak = max(peak, m.HeapAlloc)
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, _, _ := f.List(50, "")
			if len(page) > 0 {
				_, _ = f.Get(page[0].ID)
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-done
	above := float64(peak-min(peak, base.HeapAlloc)) / (1 << 20)
	t.Logf("64 reads in %v, peak heap %.0f MB above baseline, %d folds, %d events", time.Since(start), above, f.folds, f.Stats().Events)
	if above > 250 {
		t.Errorf("peak %.0f MB above baseline", above)
	}
}

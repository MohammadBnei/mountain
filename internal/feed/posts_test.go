package feed

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func sess36(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

// sessPost is a laptopRaw of session number n.
func sessPost(seq int64, n int, m int, text string, extra map[string]any) Raw {
	return laptopRaw(seq, sess36(n), m, text, extra)
}

// postFeed is a store with 100 alerts at its event cap and a post cap of its own, small so that a test fills it with
// few posts.
func postFeed(t *testing.T, postCap int) *Feed {
	f := New(100, 1<<30)
	for i := 0; i < 100; i++ {
		f.Ingest(alertRaw(int64(1000+i), "firing", fmt.Sprintf("fp%d", i), sa, 0, i, ""))
	}
	if f.Stats().Events != 100 {
		t.Fatalf("%+v", f.Stats())
	}
	f.posts.cap = postCap
	return f
}

func (f *Feed) threadsAt(t *testing.T, minutes int) []Thread {
	t.Helper()
	f.now = func() time.Time { return at(minutes) }
	c, err := f.threads(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return c.threads
}

// postsHeld is every post still held, by post_id.
func postsHeld(f *Feed) map[string]Event {
	held := map[string]Event{}
	for _, s := range f.posts.by {
		for _, e := range s.evs {
			held[e.PostID] = e
		}
	}
	return held
}

// Posts stay in a cap of their own, shared between sessions, and no post moves another kind or its floor.
func TestPostsStayInTheirOwnCap(t *testing.T) {
	for name, runes := range map[string]int{"longest posts": maxPostRunes, "middling posts": 1000} {
		t.Run(name, func(t *testing.T) {
			f := postFeed(t, 256<<10)
			alerts, floor, dropped := f.count["alert"], f.floor["alert"], f.dropped
			text := strings.Repeat("é", runes)
			f.Ingest(sessPost(1, 1, 0, "x", map[string]any{"needs_you": []string{"approve"}})) // one asks, then goes quiet
			for i := 2; i <= 300; i++ {
				f.Ingest(sessPost(int64(i), 2+i%20, 1+i/100, text, nil))
			}
			if f.posts.bytes > f.posts.cap {
				t.Errorf("posts hold %d bytes, the cap is %d", f.posts.bytes, f.posts.cap)
			}
			if f.count["alert"] != alerts || f.floor["alert"] != floor || len(f.evs["alert"]) != alerts || f.n != 100 {
				t.Errorf("a post moved another kind: %d alerts, floor %d", f.count["alert"], f.floor["alert"])
			}
			if f.floor[KindLaptop] == 0 || f.dropped == dropped {
				t.Errorf("nothing was evicted: floor %d", f.floor[KindLaptop])
			}
			if _, ok := postsHeld(f)[pid(1)]; !ok {
				t.Error("the standing ask was evicted")
			}
			for _, x := range byKind(f.threadsAt(t, 5), KindLaptop) {
				if x.Entries[0].PostID == pid(1) && !x.NeedsOwner {
					t.Error("the thread no longer needs the owner")
				}
			}
			if len(f.posts.by) < 20 {
				t.Errorf("%d sessions left: the cap was not shared", len(f.posts.by))
			}
		})
	}
}

// The session holding the most loses first: a small session keeps everything, and the big one keeps its latest ask.
func TestPostsEvictFromTheBiggestSession(t *testing.T) {
	f := postFeed(t, 128<<10)
	stored := 0
	ingest := func(r Raw) {
		if f.Ingest(r) {
			stored++
		}
	}
	ingest(sessPost(1, 1, 0, "asks", map[string]any{"needs_you": []string{"x"}}))
	for i := 2; i <= 4; i++ { // a middling session, bigger than the small one
		ingest(sessPost(int64(i), 1, 1, strings.Repeat("é", 200), nil))
	}
	for i := 5; i <= 7; i++ {
		ingest(sessPost(int64(i), 2, 1, "small", nil))
	}
	big := strings.Repeat("é", 2000)
	for i := 100; i < 400; i++ {
		ingest(sessPost(int64(i), 3, 2, big, nil))
	}
	if stored != 4+3+300 || f.posts.n >= stored {
		t.Fatalf("stored %d, held %d", stored, f.posts.n)
	}
	if s := f.posts.by[sess36(2)]; s == nil || len(s.evs) != 3 || f.posts.by[sess36(1)] == nil || len(f.posts.by[sess36(1)].evs) != 4 {
		t.Errorf("a smaller session lost posts")
	}
	if s := f.posts.by[sess36(3)]; s == nil || len(s.evs) == 0 || len(s.evs) >= 300 {
		t.Errorf("the big session was not the one trimmed")
	}
	// the biggest session's latest ask survives its own eviction too
	g := postFeed(t, 128<<10)
	g.Ingest(sessPost(1, 1, 0, big, map[string]any{"needs_you": []string{"first"}}))
	g.Ingest(sessPost(2, 1, 0, big, map[string]any{"needs_you": []string{"second"}}))
	for i := 3; i < 200; i++ {
		g.Ingest(sessPost(int64(i), 1, 1, big, nil))
	}
	asks := 0
	for _, e := range postsHeld(g) {
		if e.NeedsSet {
			asks++
			if e.Seq != 2 {
				t.Errorf("kept the ask of seq %d, not the latest", e.Seq)
			}
		}
	}
	if asks != 1 {
		t.Errorf("%d asks kept", asks)
	}
}

// The kept ask is the one with the highest seq, whatever order the posts arrive in.
func TestPostsKeepTheLatestAskBySeq(t *testing.T) {
	big := strings.Repeat("é", 2000)
	for name, order := range map[string][2]int{"later seq first": {9, 5}, "later seq last": {5, 9}} {
		f := postFeed(t, 128<<10)
		for _, seq := range order {
			f.Ingest(sessPost(int64(seq), 1, 0, "ask", map[string]any{"needs_you": []string{fmt.Sprint(seq)}}))
		}
		for i := 100; i < 300; i++ {
			f.Ingest(sessPost(int64(i), 1, 1, big, nil))
		}
		held := postsHeld(f)
		if _, ok := held[pid(9)]; !ok {
			t.Errorf("%s: the ask of seq 9 went", name)
		}
		if _, ok := held[pid(5)]; ok {
			t.Errorf("%s: the older ask of seq 5 was kept as well", name)
		}
	}
}

// A clear is not an ask, and an ask past its life no longer is one: neither is kept.
func TestPostsDoNotKeepClearsOrExpiredAsks(t *testing.T) {
	big := strings.Repeat("é", 2000)
	f := postFeed(t, 128<<10)
	f.now = func() time.Time { return at(8 * 24 * 60) } // eight days on
	f.Ingest(sessPost(1, 1, 0, big, map[string]any{"needs_you": []string{"x"}}))
	f.Ingest(sessPost(2, 1, 8*24*60-3, big, map[string]any{"needs_you": []string{}})) // clears it, and is recent
	f.Ingest(sessPost(3, 2, 8*24*60-5, big, map[string]any{"needs_you": []string{"y"}}))
	f.Ingest(sessPost(4, 3, 0, big, map[string]any{"needs_you": []string{"z"}})) // eight days old
	for i := 100; i < 400; i++ {
		f.Ingest(sessPost(int64(i), 1+i%3, 8*24*60-1, big, nil))
	}
	held := postsHeld(f)
	for _, gone := range []int{1, 2, 4} {
		if _, ok := held[pid(gone)]; ok {
			t.Errorf("post %d was kept", gone)
		}
	}
	if _, ok := held[pid(3)]; !ok {
		t.Error("a standing ask was evicted")
	}
}

// A session with no posts left is not held: the map and the heap agree and hold none.
func TestPostsDropEmptiedSessions(t *testing.T) {
	f := postFeed(t, 6000)
	text := strings.Repeat("é", 1000)
	for i := 1; i <= 60; i++ {
		f.Ingest(sessPost(int64(i), i, 1, text, nil))
	}
	if len(f.posts.by) != len(f.posts.h) || len(f.posts.by) == 0 || len(f.posts.by) > 4 {
		t.Fatalf("%d sessions, %d in the heap", len(f.posts.by), len(f.posts.h))
	}
	n := 0
	for id, s := range f.posts.by {
		if len(s.evs) == 0 || s.id != id || f.posts.h[s.hi] != s {
			t.Errorf("session %s: %d posts", id, len(s.evs))
		}
		n += len(s.evs)
	}
	if n != f.posts.n {
		t.Errorf("%d posts held, %d counted", n, f.posts.n)
	}
}

// When the session holding the most has only its kept post, a post of another session goes instead.
func TestPostsEvictFromAnotherSessionWhenTheBiggestHoldsOnlyItsAsk(t *testing.T) {
	f := postFeed(t, 9000)
	f.Ingest(sessPost(1, 1, 0, strings.Repeat("é", 2000), map[string]any{"needs_you": []string{"x"}}))
	for i := 2; i <= 8; i++ {
		f.Ingest(sessPost(int64(i), i, 1, strings.Repeat("é", 300), nil))
	}
	held := postsHeld(f)
	if _, ok := held[pid(1)]; !ok {
		t.Error("the only post of the biggest session, an ask, was evicted")
	}
	if f.posts.bytes > f.posts.cap || len(held) >= 8 {
		t.Errorf("%d bytes, %d held", f.posts.bytes, len(held))
	}
}

// When every session holds only its ask, the cap still holds.
func TestPostsCapHoldsWhenEveryPostIsKept(t *testing.T) {
	f := postFeed(t, 128<<10)
	big := strings.Repeat("é", 2000)
	for i := 1; i <= 100; i++ {
		f.Ingest(sessPost(int64(i), i, 1, big, map[string]any{"needs_you": []string{"x"}}))
	}
	if f.posts.bytes > f.posts.cap || f.posts.n == 0 {
		t.Errorf("%d bytes, %d posts", f.posts.bytes, f.posts.n)
	}
}

// An evicted post comes back only above the floor, and the floor of posts is their own.
func TestPostFloor(t *testing.T) {
	f := postFeed(t, 64<<10)
	big := strings.Repeat("é", 2000)
	for i := 1; i <= 100; i++ {
		f.Ingest(sessPost(int64(i), 1+i%5, 1, big, nil))
	}
	if f.floor[KindLaptop] == 0 {
		t.Fatal("setup")
	}
	if f.Ingest(sessPost(f.floor[KindLaptop], 9, 5, "late", nil)) {
		t.Error("an evicted post came back")
	}
	if !f.Ingest(alertRaw(5, "firing", "fpz", sa, 0, 5, "")) {
		t.Error("the posts' floor refused an alert")
	}
}

// needs_you null is not a value: it is a bad event, as is a non-list.
func TestNeedsYouNullIsBad(t *testing.T) {
	f := New(100, 1<<20)
	for i, v := range []any{nil, "x", 3, map[string]any{}} {
		if f.Ingest(laptopRaw(int64(1+i), "s1", 0, "x", map[string]any{"needs_you": v})) {
			t.Errorf("%v stored", v)
		}
	}
	if f.Stats().Bad != 4 {
		t.Errorf("bad %d", f.Stats().Bad)
	}
	if !f.Ingest(laptopRaw(9, "s1", 0, "x", map[string]any{"needs_you": []string{}})) {
		t.Error("an empty list is a value")
	}
}

func TestNeedsListCaps(t *testing.T) {
	var items []string
	for i := 0; i < 50; i++ {
		items = append(items, fmt.Sprintf("item %d", i))
	}
	e, _ := Parse(laptopRaw(1, "s1", 0, "x", map[string]any{"needs_you": items}))
	if len(e.NeedsList) != maxNeeds || e.NeedsList[maxNeeds-1] != "item 19" {
		t.Errorf("%d items", len(e.NeedsList))
	}
}

func TestLaptopThreadID(t *testing.T) {
	th := lap(t, 5, laptopRaw(1, "s1", 0, "x", nil))
	if !strings.HasPrefix(th.ID, "laptop-") || th.ID != threadID(KindLaptop, sess36(1)) {
		t.Errorf("%q", th.ID)
	}
}

func TestSizeCountsAboutAndNeeds(t *testing.T) {
	a := Event{Type: "laptop.post.created", Key: sess36(1)}
	b := a
	b.About = []Claim{{"acme/demo-a", 1}, {"acme/demo-b", 2}}
	c := a
	c.NeedsList = []string{strings.Repeat("a", 100), strings.Repeat("b", 100)}
	if b.size() < a.size()+len("acme/demo-a")+len("acme/demo-b") || c.size() < a.size()+200+32 {
		t.Errorf("sizes %d %d %d", a.size(), b.size(), c.size())
	}
}

// The title is the label, else repo@branch, else the session's first eight characters.
func TestLaptopTitleFallback(t *testing.T) {
	for _, tc := range []struct {
		extra map[string]any
		want  string
	}{
		{map[string]any{"session_label": "named"}, "Laptop: named"},
		{nil, "Laptop: demo@work"},
		{map[string]any{"branch": ""}, "Laptop: demo"},
		{map[string]any{"repo": ""}, "Laptop: work"},
		{map[string]any{"repo": "", "branch": ""}, "Session 00000000"},
	} {
		if th := lap(t, 5, laptopRaw(1, "s1", 0, "x", tc.extra)); th.Title != tc.want {
			t.Errorf("%v: %q, want %q", tc.extra, th.Title, tc.want)
		}
	}
}

// A mention is never what the list shows of a PR thread; the list cuts a long text and a long needs_list, and the
// thread has both whole.
func TestListPreview(t *testing.T) {
	f := New(1000, 1<<24)
	f.Ingest(prOpened(t, "demo-a", 7, 1))
	f.Ingest(sessPost(10, 1, 5, strings.Repeat("é", 1000), map[string]any{"about": about{{"repo": "acme/demo-a", "number": 7}}}))
	f.now = func() time.Time { return at(10) }
	page, _, _ := f.List(10, "")
	var own Thread
	for _, th := range page {
		switch th.Kind {
		case KindPR:
			own, _ = f.Get(th.ID)
			last := own.Entries[len(own.Entries)-1]
			if !last.Claim { // the claim sorts last by time, so the preview must be the entry before it
				t.Fatal("setup: the claim is not the PR's last entry")
			}
			if want := own.Entries[len(own.Entries)-2]; len(th.Entries) != 1 || th.Entries[0].Claim || th.Entries[0].Text != want.Text {
				t.Errorf("the PR's preview is %+v", th.Entries)
			}
		case KindLaptop:
			e := th.Entries[0]
			if len([]rune(e.Text)) != listRunes || !e.HasMore {
				t.Errorf("%d runes, has_more %v", len([]rune(e.Text)), e.HasMore)
			}
			whole, _ := f.Get(th.ID)
			if len([]rune(whole.Entries[0].Text)) != 1000 || whole.Entries[0].HasMore {
				t.Error("the thread has the text whole")
			}
		}
	}
	// a needs_list: the first three items of 120 characters, and how many there are
	g := New(1000, 1<<24)
	items := []string{strings.Repeat("é", 300), "two", "three", "four", "five"}
	g.Ingest(sessPost(1, 1, 0, "ask", map[string]any{"needs_you": items}))
	g.now = func() time.Time { return at(5) }
	pg, _, _ := g.List(5, "")
	e := pg[0].Entries[0]
	if len(e.NeedsList) != listNeeds || e.NeedsCount != 5 || len([]rune(e.NeedsList[0])) != listNeedRunes || e.NeedsList[2] != "three" {
		t.Errorf("%d items of %d, count %d", len(e.NeedsList), len([]rune(e.NeedsList[0])), e.NeedsCount)
	}
	whole, _ := g.Get(pg[0].ID)
	if w := whole.Entries[0]; len(w.NeedsList) != 5 || len([]rune(w.NeedsList[0])) != needRunes+1 || w.NeedsCount != 0 {
		t.Errorf("the thread's list is cut: %+v", w)
	}
	// 200 threads of the longest post stay small
	h := New(1000, 1<<24)
	for i := 1; i <= 200; i++ {
		h.Ingest(sessPost(int64(i), i, 1, strings.Repeat("é", maxPostRunes), map[string]any{"needs_you": items}))
	}
	h.now = func() time.Time { return at(5) }
	all, _, _ := h.List(200, "")
	b, _ := json.Marshal(map[string]any{"threads": all})
	if len(all) != 200 || len(b) > 300<<10 {
		t.Errorf("%d threads, %d bytes", len(all), len(b))
	}
}

// Many posts of 10 claims each build little: a thread takes its newest 50 and later posts build nothing for it.
func TestClaimsStaySmall(t *testing.T) {
	pr := newPRs(nil)
	var keys []Claim
	for i := 1; i <= 10; i++ {
		c := Claim{"acme/demo-a", i}
		keys = append(keys, c)
		pr.threads[prKey{c.Repo, c.N}] = &Thread{ID: fmt.Sprintf("pr-%d", i), Kind: KindPR}
	}
	lp := newLaptops()
	th := &Thread{ID: "laptop-x"}
	text := strings.Repeat("é", 1000)
	const posts = 20000
	for i := 0; i < posts; i++ {
		lp.pend = append(lp.pend, pending{Event{Seq: int64(i + 1), Time: at(i / 100), Text: text, PostID: pid(i), About: keys}, th})
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	lp.claims(pr)
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 2<<20 {
		t.Errorf("claims built %d bytes", got)
	}
	for _, p := range pr.threads {
		if len(p.Entries) != maxPRClaims || p.Entries[0].seq != int64(posts-maxPRClaims+1) || p.Entries[maxPRClaims-1].seq != posts {
			t.Fatalf("%s: %d claims from seq %d", p.ID, len(p.Entries), p.Entries[0].seq)
		}
	}
}

// corrects names an earlier post (by seq) of the same session; a later one, or the post itself, is plain.
func TestCorrectsOnlyEarlierPosts(t *testing.T) {
	th := lap(t, 10,
		laptopRaw(1, "s1", 0, "a", map[string]any{"corrects": pid(2)}), // names a later post
		laptopRaw(2, "s1", 1, "b", map[string]any{"corrects": pid(1)}), // names an earlier one
		laptopRaw(3, "s1", 2, "c", map[string]any{"corrects": pid(3)}), // itself
	)
	e := th.Entries
	if e[0].Correction || e[0].Corrects != "" || e[0].CorrectedBy != pid(2) || !e[1].Correction || e[1].Corrects != pid(1) || e[2].Correction || e[2].Corrects != "" {
		t.Errorf("%+v", e)
	}
}

// The latest needs_you is the one with the highest seq, whatever the times say.
func TestNeedsYouBySeq(t *testing.T) {
	ask := laptopRaw(1, "s1", 5, "ask", map[string]any{"needs_you": []string{"x"}})
	clear := laptopRaw(2, "s1", 3, "clear", map[string]any{"needs_you": []string{}}) // earlier time, later seq
	if th := lap(t, 10, ask, clear); th.NeedsOwner {
		t.Errorf("the ask of the earlier seq stands: %+v", th.Entries)
	}
	reask := laptopRaw(3, "s1", 1, "again", map[string]any{"needs_you": []string{"y"}}) // earliest time, latest seq
	if th := lap(t, 10, ask, clear, reask); !th.NeedsOwner || !th.Entries[0].AsksOwner || th.Entries[0].NeedsList[0] != "y" {
		t.Errorf("%+v", th.Entries)
	}
}

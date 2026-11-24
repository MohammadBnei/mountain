package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// edit rewrites a Raw's JSON body.
func edit(t *testing.T, r Raw, f func(d map[string]any)) Raw {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal(r.Data, &d); err != nil {
		t.Fatal(err)
	}
	f(d)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	r.Data = b
	return r
}

func mergedPR(t *testing.T, seq int64, n int, sha string) Raw {
	return forge(t, seq, "pull_request_closed", "pull_request.closed", fmt.Sprintf("forge/MohammadBnei/editable-blog/pr/%d", n), fmt.Sprintf("pr/%d/closed/1", n), 0,
		[2]string{`"merged": false`, `"merged": true`}, [2]string{`"merge_commit_sha": null`, `"merge_commit_sha": "` + sha + `"`}, [2]string{`"number": 42`, fmt.Sprintf(`"number": %d`, n)})
}

// rolled is a kwatch deploy.synced with the given revisions and previous_revisions (nil: the field is absent).
func rolled(seq int64, app string, m int, revs, prev []string) Raw {
	f := `"sync":"Synced","previous":"OutOfSync","phase":"Succeeded"`
	if prev != nil {
		b, _ := json.Marshal(prev)
		f += `,"previous_revisions":` + string(b)
	}
	r := kwatch(seq, "deploy.synced", app, m, f)
	b, _ := json.Marshal(revs)
	r.Data = []byte(strings.Replace(string(r.Data), "%s", `,"revisions":`+string(b), 1))
	return r
}

// An app that watches the infra repo's revision alongside its own sees it in every sync; only a revision absent from
// previous_revisions is a rollout of it.
func TestOnlyNewRevisionsLink(t *testing.T) {
	infra := mergeSHA
	rs := []Raw{mergedPR(t, 1, 42, infra)}
	for i := 0; i < 5; i++ {
		app := fmt.Sprintf("app%d", i)
		rs = append(rs, rolled(int64(10+i), app, 3+i, []string{fmt.Sprintf("%040x", i+1), infra}, []string{fmt.Sprintf("%040x", i), infra}))
	}
	th := fold(t, 20, rs...)
	if hasKind(byKind(th, KindPR)[0], "deploy") {
		t.Errorf("five unchanged revisions linked: %v", byKind(th, KindPR)[0].Links)
	}
	for _, d := range byKind(th, KindDeploy) {
		if hasKind(d, "pull_request") {
			t.Errorf("%s links the PR", d.Title)
		}
	}
	// the one sync whose previous revisions lack it links, and only that rollout
	th = fold(t, 20, append(rs, rolled(30, "gitops", 9, []string{infra}, []string{"0000000000000000"}))...)
	p := byKind(th, KindPR)[0]
	n := 0
	for _, l := range p.Links {
		if l.Kind == "deploy" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one deploy link: %v", p.Links)
	}
	// no previous list: a single revision counts, two do not
	if th := fold(t, 20, mergedPR(t, 1, 42, infra), rolled(30, "a", 9, []string{infra}, nil)); !hasKind(byKind(th, KindPR)[0], "deploy") {
		t.Error("a single revision with no previous list did not link")
	}
	if th := fold(t, 20, mergedPR(t, 1, 42, infra), rolled(30, "a", 9, []string{infra, "abcdef1234567890"}, nil)); hasKind(byKind(th, KindPR)[0], "deploy") {
		t.Error("two revisions with no previous list linked")
	}
}

// 10,000 PRs merged at one sha stay linear: the rollout links the first maxLinks of them.
func TestOneMergeShaManyPRs(t *testing.T) {
	rs := []Raw{rolled(1, "gitops", 9, []string{mergeSHA}, []string{"0000000000000000"})}
	for i := 1; i <= 10000; i++ {
		rs = append(rs, mergedPR(t, int64(1+i), i, mergeSHA))
	}
	start := time.Now()
	th := fold(t, 20, rs...)
	linked := 0
	for _, x := range byKind(th, KindPR) {
		if hasKind(x, "deploy") {
			linked++
		}
	}
	if linked != maxLinks || len(byKind(th, KindDeploy)[0].Links) > maxLinks || time.Since(start) > 10*time.Second {
		t.Errorf("%d PRs linked, %d links on the deploy, %v", linked, len(byKind(th, KindDeploy)[0].Links), time.Since(start))
	}
}

// A webhook COMMENT review and the relist's copy (another ce_id) are one entry; a relisted one with other text is not.
func TestRelistedReviewCommentDropped(t *testing.T) {
	hook := forge(t, 1, "pull_request_review_comment", "pull_request_review.comment", prSubj, "delivery-1", 1)
	relist := edit(t, forge(t, 2, "pull_request_review_comment", "pull_request_review.comment", prSubj, "pr/42/review/comment/9", 2), func(d map[string]any) { d["relisted"] = true })
	other := edit(t, forge(t, 3, "pull_request_review_comment", "pull_request_review.comment", prSubj, "pr/42/review/comment/10", 3), func(d map[string]any) {
		d["relisted"] = true
		d["review"] = map[string]any{"id": 10, "content": "another"}
	})
	count := func(rs ...Raw) int {
		n := 0
		for _, e := range byKind(fold(t, 10, rs...), KindPR)[0].Entries {
			if e.Text == "review: comment" {
				n++
			}
		}
		return n
	}
	if got := count(hook, relist); got != 1 {
		t.Errorf("webhook and relist: %d entries", got)
	}
	if got := count(relist, hook); got != 1 {
		t.Errorf("relist first: %d entries", got)
	}
	if got := count(relist); got != 1 {
		t.Errorf("relist alone: %d", got)
	}
	if got := count(hook, other); got != 2 {
		t.Errorf("other text: %d", got)
	}
	hook2 := hook
	hook2.ID, hook2.Seq = "delivery-2", 5
	if got := count(hook, hook2); got != 2 {
		t.Errorf("two webhook ones: %d", got)
	}
}

// A successful CI run is dropped at ingest, neither stored nor bad; failure and recover are kept.
func TestSuccessfulRunDropped(t *testing.T) {
	f := New(100, 1<<20)
	if f.Ingest(forge(t, 1, "action_run_success", "action_run.success", "forge/MohammadBnei/editable-blog/run/117", "d", 1)) {
		t.Error("a successful run was stored")
	}
	f.Ingest(forge(t, 2, "pull_request_opened", "pull_request.opened", prSubj, "a", 0))
	if !f.Ingest(forge(t, 3, "action_run_failure", "action_run.failure", "forge/MohammadBnei/editable-blog/run/117", "e", 1)) ||
		!f.Ingest(forge(t, 4, "action_run_recover", "action_run.recover", "forge/MohammadBnei/editable-blog/run/117", "g", 2)) {
		t.Error("failure or recover not stored")
	}
	if s := f.Stats(); s.Bad != 0 || s.Refused != 0 || s.Events != 3 {
		t.Errorf("%+v", s)
	}
}

func TestEndedSessionWithOpenCard(t *testing.T) {
	rs := []Raw{sess(1, "opened", 0, ``), cardRaw(2, "opened", sid, dec, 1, `,"expires_at":"2026-11-24T12:00:00Z"`), sess(3, "ended", 2, ``)}
	s := byKind(fold(t, 5, rs...), KindSession)[0]
	if !s.NeedsOwner || s.State != StateOpen {
		t.Errorf("ended session, open card: needs %v, %s", s.NeedsOwner, s.State)
	}
	if s := byKind(fold(t, 200, rs...), KindSession)[0]; s.NeedsOwner || s.State != StateClosed {
		t.Errorf("after the card expired: needs %v, %s", s.NeedsOwner, s.State)
	}
	// no expires_at: it stops needing the owner 14 days after it opened
	nox := []Raw{cardRaw(1, "opened", "gone", dec, 0, ``)}
	if th := fold(t, 14*24*60-1, nox...); !th[0].NeedsOwner {
		t.Error("a card without expires_at stopped early")
	}
	if th := fold(t, 14*24*60+1, nox...); th[0].NeedsOwner || th[0].State != StateClosed {
		t.Error("a card without expires_at never stopped")
	}
}

// A run's URL must pass safeURL and shows as a quote.
func TestRunLink(t *testing.T) {
	run := func(url string) []Link {
		r := forge(t, 2, "action_run_failure", "action_run.failure", "forge/MohammadBnei/editable-blog/run/117", "r", 1)
		r = edit(t, r, func(d map[string]any) { d["run"].(map[string]any)["html_url"] = url })
		return byKind(fold(t, 5, forge(t, 1, "pull_request_opened", "pull_request.opened", prSubj, "a", 0), r), KindPR)[0].Links
	}
	if l := run("https://git.bnei.dev/MohammadBnei/editable-blog/actions/runs/117"); len(l) != 1 || l[0].Kind != "run" || !l[0].Quoted {
		t.Errorf("%v", l)
	}
	for _, bad := range []string{"http://git.bnei.dev/x", "javascript:alert(1)", "https://u:p@git.bnei.dev/x", "https://git.bnei.dev/a b"} {
		if l := run(bad); len(l) != 0 {
			t.Errorf("%q linked", bad)
		}
	}
}

func TestSessionDefinitionAndTitleQuoted(t *testing.T) {
	r := sess(1, "opened", 0, `,"definition":"ops-lead","title":"same"`)
	s := byKind(fold(t, 5, r, sess(2, "turn_ended", 1, `,"seq":1,"title":"same"`), sess(3, "turn_ended", 2, `,"seq":2,"title":"same"`)), KindSession)[0]
	titles, def := 0, false
	for _, e := range s.Entries {
		if e.Text == "same" {
			titles++
		}
		if strings.Contains(e.Text, "ops-lead") {
			def = e.Quoted
		}
	}
	if titles != 1 || !def {
		t.Errorf("a repeated title shows %d times, the definition quoted %v: %v", titles, def, texts(s))
	}
}

// Every clamp, on the parsed event.
func TestParseClamps(t *testing.T) {
	long := strings.Repeat("é", 500)
	runes := func(s string) int { return len([]rune(s)) }
	c := edit(t, forge(t, 1, "pull_request_comment_created", "issue_comment.created", prSubj, "c", 1), func(d map[string]any) {
		d["comment"].(map[string]any)["body"] = "a\u202eb" + long
	})
	e, _ := Parse(c)
	if n := runes(e.Text); n != 401 || strings.ContainsRune(e.Text, '\u202e') {
		t.Errorf("comment %d %q", n, e.Text[:8])
	}
	rv := edit(t, forge(t, 1, "pull_request_review_rejected", "pull_request_review.rejected", prSubj, "c", 1), func(d map[string]any) {
		d["review"] = map[string]any{"content": long}
	})
	if e, _ := Parse(rv); runes(e.Text) != 401 {
		t.Errorf("review %d", runes(e.Text))
	}
	pr := edit(t, forge(t, 1, "pull_request_opened", "pull_request.opened", prSubj, "c", 1), func(d map[string]any) {
		d["pull_request"].(map[string]any)["title"] = "x\u202ey\n" + long
	})
	if e, _ := Parse(pr); runes(e.Title) != 201 || strings.ContainsAny(e.Title, "\u202e\n") {
		t.Errorf("title %d", runes(e.Title))
	}
	op := edit(t, forge(t, 1, "pull_request_opened", "pull_request.opened", prSubj, "c", 1), func(d map[string]any) {
		d["sender"].(map[string]any)["login"] = "intruder"
		d["pull_request"].(map[string]any)["user"].(map[string]any)["login"] = "author"
	})
	if e, _ := Parse(op); e.Login != "author" {
		t.Errorf("opened is the author's: %q", e.Login)
	}
	sy := edit(t, forge(t, 1, "pull_request_synchronized", "pull_request.synchronized", prSubj, "c", 1), func(d map[string]any) {
		d["sender"].(map[string]any)["login"] = "pusher"
		d["pull_request"].(map[string]any)["user"].(map[string]any)["login"] = "author"
	})
	if e, _ := Parse(sy); e.Login != "pusher" {
		t.Errorf("a push is the sender's: %q", e.Login)
	}
}

func TestParseChecks(t *testing.T) {
	if e, _ := Parse(sess(1, "opened", 0, `,"kind":"evil"`)); e.Kind != "" {
		t.Errorf("kind %q", e.Kind)
	}
	if e, _ := Parse(sess(1, "opened", 0, ``)); e.Kind != "repo" {
		t.Errorf("kind %q", e.Kind)
	}
	for opt, want := range map[string]string{"1": "1", "3": "3", "0": "", "4": "", "9": "", "22": "", "x": ""} {
		e, ok := Parse(cardRaw(1, "answered", sid, dec, 1, `,"option_id":"`+opt+`"`))
		if !ok || e.Option != want {
			t.Errorf("option %q -> %q", opt, e.Option)
		}
	}
	bogus := cardRaw(1, "opened", sid, dec, 1, ``)
	bogus.Type = "dev.bnei.fleet.card.bogus"
	if _, ok := Parse(bogus); ok {
		t.Error("an unknown card state parsed")
	}
	if _, ok := Parse(cardRaw(1, "opened", sid, "", 1, ``)); ok {
		t.Error("a card with no decision id parsed")
	}
	// a closed PR that was not merged has no merge sha, whatever the payload says
	if e, _ := Parse(forge(t, 1, "pull_request_closed", "pull_request.closed", prSubj, "c", 1, [2]string{`"merge_commit_sha": null`, `"merge_commit_sha": "` + mergeSHA + `"`})); e.Merge != "" || e.Merged {
		t.Errorf("unmerged: %q %v", e.Merge, e.Merged)
	}
	if e, _ := Parse(mergedPR(t, 1, 42, mergeSHA)); e.Merge != mergeSHA || !e.Merged {
		t.Errorf("merged: %q %v", e.Merge, e.Merged)
	}
}

// Per-kind counts and bytes always equal what the held events add up to, through ingests and evictions.
func TestKindBookkeeping(t *testing.T) {
	f := New(12, 1<<20)
	check := func(when string) {
		count, bytes := map[string]int{}, map[string]int{}
		for _, e := range f.all() {
			count[kindOf(e)]++
			bytes[kindOf(e)] += e.size()
		}
		for _, k := range []string{"alert", "deploy", "forge", "session", "card"} {
			if f.count[k] != count[k] || f.kbytes[k] != bytes[k] {
				t.Errorf("%s: %s count %d want %d, bytes %d want %d", when, k, f.count[k], count[k], f.kbytes[k], bytes[k])
			}
		}
	}
	f.Ingest(alertRaw(1, "firing", "fp1", sa, 0, 0, ""))
	f.Ingest(synced(2, "gitops", 0))
	f.Ingest(sess(3, "opened", 0, ``))
	f.Ingest(cardRaw(4, "opened", sid, dec, 0, ``))
	check("filled")
	for i := 0; i < 30; i++ {
		f.Ingest(forge(t, int64(10+i), "pull_request_synchronized", "pull_request.synchronized", prSubj, fmt.Sprintf("d%d", i), 1+i))
	}
	check("evicted by count")
	g := New(1000, 5000)
	f = g
	for i := 0; i < 20; i++ {
		f.Ingest(forge(t, int64(10+i), "pull_request_synchronized", "pull_request.synchronized", prSubj, fmt.Sprintf("d%d", i), 1+i))
	}
	f.Ingest(alertRaw(1, "firing", "fp1", sa, 0, 0, ""))
	check("evicted by bytes")
	if f.Stats().Bytes != f.bytes {
		t.Error("bytes")
	}
}

func TestKindOf(t *testing.T) {
	for typ, want := range map[string]string{"fleet.session.opened": "session", "fleet.card.opened": "card", "forge.pull_request.opened": "forge", "alert.firing": "alert", "deploy.synced": "deploy"} {
		if got := kindOf(Event{Type: typ}); got != want {
			t.Errorf("%s: %s", typ, got)
		}
	}
}

// A PR first seen through a comment whose payload says closed is a closed thread (its closed event is older than the
// feed's memory).
func TestPayloadStateAloneCloses(t *testing.T) {
	c := forge(t, 1, "pull_request_comment_created", "issue_comment.created", prSubj, "c", 1, [2]string{`"state": "open"`, `"state": "closed"`})
	if p := byKind(fold(t, 5, c), KindPR)[0]; p.State != StateClosed {
		t.Errorf("%s", p.State)
	}
	if p := byKind(fold(t, 5, forge(t, 1, "pull_request_comment_created", "issue_comment.created", prSubj, "c", 1)), KindPR)[0]; p.State != StateOpen {
		t.Errorf("%s", p.State)
	}
}

// Readers that find the cache stale fold once between them, and List and Get answer from that one result.
func TestSingleFlight(t *testing.T) {
	f := New(1000, 1<<20)
	for _, r := range prLife(t) {
		f.Ingest(r)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = f.List(10, "")
		}()
	}
	wg.Wait()
	if f.folds != 1 {
		t.Errorf("%d folds for 64 reads", f.folds)
	}
	f.Ingest(forge(t, 20, "pull_request_comment_created", "issue_comment.created", prSubj, "c9", 30))
	_, _, _ = f.List(10, "")
	if f.folds != 2 {
		t.Errorf("a changed store: %d folds", f.folds)
	}
	// a read that finds a fold in flight takes the last result while it is under staleOK old
	f.mu.Lock()
	f.folding = make(chan struct{})
	f.gen++
	f.mu.Unlock()
	got := make(chan *folded)
	go func() { c, _ := f.threads(context.Background()); got <- c }()
	if c := <-got; c != f.cache {
		t.Error("a read during a fold did not take the last result")
	}
}

// A sync that names two sources (a chart and its values repo) links the one that is new, which needs the field name
// previous_revisions to be read: with it unread the sync would look like a two-revision sync with no previous list.
func TestMultiSourceSyncLinksTheNewRevision(t *testing.T) {
	const chart = "abcdef1234567890abcdef1234567890abcdef12"
	e, _ := Parse(rolled(1, "app", 1, []string{chart, mergeSHA}, []string{chart, "0000000000000000"}))
	if len(e.PrevRevisions) != 2 || e.PrevRevisions[0] != chart {
		t.Fatalf("previous revisions %v", e.PrevRevisions)
	}
	th := fold(t, 20, mergedPR(t, 1, 42, mergeSHA), rolled(2, "app", 3, []string{chart, mergeSHA}, []string{chart, "0000000000000000"}))
	if !hasKind(byKind(th, KindPR)[0], "deploy") || !hasKind(byKind(th, KindDeploy)[0], "pull_request") {
		t.Error("the new revision of a two-source sync did not link")
	}
}

// What an event weighs counts its previous revisions, or the byte cap undercounts a sync.
func TestSizeCountsPreviousRevisions(t *testing.T) {
	a := Event{Type: "deploy.synced", Revisions: []string{"abc"}}
	b := a
	b.PrevRevisions = []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	if b.size() < a.size()+80+2*16 {
		t.Errorf("size %d with previous revisions, %d without", b.size(), a.size())
	}
}

// Eviction takes from the kind with the most events when the event cap is over, and from the kind with the most bytes
// when the byte cap is over: they differ when one kind has many small events and another a few big ones.
func TestEvictionPicksByTheCapThatIsOver(t *testing.T) {
	big := func(i int) Raw {
		return edit(t, forge(t, int64(100+i), "pull_request_comment_created", "issue_comment.created", prSubj, fmt.Sprintf("big%d", i), i), func(d map[string]any) {
			d["comment"].(map[string]any)["body"] = strings.Repeat("é", 400)
		})
	}
	fill := func() *Feed {
		f := New(1000, 1<<30)
		for i := 0; i < 6; i++ {
			f.Ingest(alertRaw(int64(1+i), "firing", fmt.Sprintf("fp%d", i), sa, 0, i, ""))
		}
		for i := 0; i < 4; i++ {
			f.Ingest(big(i))
		}
		if f.kbytes["forge"] <= f.kbytes["alert"] || f.count["alert"] <= f.count["forge"] {
			t.Fatalf("setup: alert %d events %d bytes, forge %d events %d bytes", f.count["alert"], f.kbytes["alert"], f.count["forge"], f.kbytes["forge"])
		}
		return f
	}
	f := fill()
	f.maxEvents = 10 // over by the next alert: the alerts hold the most events
	f.Ingest(alertRaw(50, "firing", "fp9", sa, 0, 20, ""))
	if f.count["alert"] != 6 || f.count["forge"] != 4 {
		t.Errorf("event cap: alerts %d, forge %d", f.count["alert"], f.count["forge"])
	}
	f = fill()
	f.maxBytes = f.bytes // over by the next alert: forge holds the most bytes
	f.Ingest(alertRaw(50, "firing", "fp9", sa, 0, 20, ""))
	if f.count["alert"] != 7 || f.count["forge"] != 3 {
		t.Errorf("byte cap: alerts %d, forge %d", f.count["alert"], f.count["forge"])
	}
}

// A Fold that panics ends its flight like any other: 50 readers and Stats return (the waiters with the last good
// result or errFold), the panic is counted once as bad, no goroutine is left, and the next read folds again.
func TestFoldPanicEndsTheFlight(t *testing.T) {
	f := New(1000, 1<<20)
	for _, r := range prLife(t) {
		f.Ingest(r)
	}
	release := make(chan struct{})
	f.SetFold(func([]Event, time.Time) []Thread {
		<-release
		panic("boom")
	})
	before := runtime.NumGoroutine()
	var wg sync.WaitGroup
	errs := make(chan error, 51)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.List(10, "")
			errs <- err
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.Stats()
		errs <- nil
	}()
	time.Sleep(50 * time.Millisecond) // the readers pile up behind the one flight
	close(release)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readers hang behind a fold that panicked")
	}
	close(errs)
	failed := 0
	for err := range errs {
		if errors.Is(err, errFold) {
			failed++
		}
	}
	if failed == 0 {
		t.Error("no reader saw the failure")
	}
	if f.Stats().Bad == 0 || f.folding != nil {
		t.Errorf("bad %d, folding %v", f.Stats().Bad, f.folding)
	}
	// a later good fold works, and a failed one after it falls back to that result
	f.SetFold(Fold)
	if th, _, err := f.List(10, ""); err != nil || len(th) == 0 {
		t.Fatalf("after: %v %v", th, err)
	}
	f.SetFold(func([]Event, time.Time) []Thread { panic("again") })
	f.Ingest(forge(t, 30, "pull_request_comment_created", "issue_comment.created", prSubj, "c30", 40))
	if th, _, err := f.List(10, ""); err != nil || len(th) == 0 {
		t.Errorf("a failed fold after a good one: %v %v", th, err)
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+1 {
		t.Errorf("goroutines %d -> %d", before, after)
	}
}

// A waiter leaves when its request ends.
func TestWaiterHonoursContext(t *testing.T) {
	f := New(1000, 1<<20)
	f.Ingest(prLife(t)[0])
	release := make(chan struct{})
	started := make(chan struct{})
	f.SetFold(func(evs []Event, now time.Time) []Thread {
		close(started)
		<-release
		return Fold(evs, now)
	})
	go f.List(10, "") //nolint:errcheck // holds the flight
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := f.ListCtx(ctx, 10, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a waiter outlived its request: %v", err)
	}
	if _, _, err := f.GetCtx(ctx, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("get: %v", err)
	}
	close(release)
}

// An evicted event's slot is zeroed, so its strings are not held by the array; a front that is mostly dead is released.
func TestEvictionReleasesTheFront(t *testing.T) {
	f := New(100, 1<<30)
	for i := 0; i < 100; i++ {
		f.Ingest(alertRaw(int64(1+i), "firing", fmt.Sprintf("fp%d", i), sa, 0, i, ""))
	}
	held := f.evs["alert"]
	f.Ingest(alertRaw(500, "firing", "fpx", sa, 0, 500, ""))
	if held[0].ID != "" || held[0].Key != "" || held[0].Name != "" {
		t.Errorf("the evicted slot still holds %+v", held[0])
	}
	// the forge events push the alerts out from the front, with no append to the alerts to move their array
	old := f.evs["alert"]
	f.maxEvents = 10
	f.Ingest(forge(t, 2000, "pull_request_synchronized", "pull_request.synchronized", prSubj, "s0", 700))
	s := f.evs["alert"]
	if len(s) == 0 || len(s) > 20 {
		t.Fatalf("setup: %d alerts left", len(s))
	}
	if &s[0] == &old[len(old)-len(s)] {
		t.Errorf("%d alerts still live in the array that held 100", len(s))
	}
}

// List's last entry is capped at its length: an append by a caller cannot write into the fold's array.
func TestListEntriesAreCapped(t *testing.T) {
	f := New(1000, 1<<20)
	for _, r := range prLife(t) {
		f.Ingest(r)
	}
	page, _, _ := f.List(10, "")
	if e := page[0].Entries; len(e) != 1 || cap(e) != 1 {
		t.Errorf("entries len %d cap %d", len(e), cap(e))
	}
}

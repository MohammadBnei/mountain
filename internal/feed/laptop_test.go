package feed

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// laptopRaw is a post event as laptop.go describes its fields; "s1" and the like name a session.
func laptopRaw(seq int64, session string, m int, text string, extra map[string]any) Raw {
	if strings.HasPrefix(session, "s") && len(session) < 4 { // "s1": a test's name for a session
		session = fmt.Sprintf("00000000-0000-4000-8000-0000000000%02s", session[1:])
	}
	d := map[string]any{"session": session, "repo": "demo", "branch": "work", "post_id": pid(int(seq)), "text": text}
	for k, v := range extra {
		d[k] = v
	}
	b, _ := json.Marshal(d)
	return Raw{Seq: seq, Source: LaptopSource, ID: "post/" + session + "/" + pid(int(seq)), Type: "dev.bnei." + laptopType, Subject: "laptop/session/" + session, Time: at(m), Data: b}
}

// pid is the post_id of the post with seq n.
func pid(n int) string { return fmt.Sprintf("66666666-7777-4888-9999-%012d", n) }

type about = []map[string]any

func lap(t *testing.T, now int, rs ...Raw) Thread {
	t.Helper()
	th := byKind(fold(t, now, rs...), KindLaptop)
	if len(th) != 1 {
		t.Fatalf("%d laptop threads", len(th))
	}
	return th[0]
}

func TestLaptopThread(t *testing.T) {
	th := lap(t, 10,
		laptopRaw(1, "s1", 0, "first **post**", nil),
		laptopRaw(2, "s1", 5, "second post", map[string]any{"session_label": "fixing the feed"}))
	if th.Kind != KindLaptop || len(th.Entries) != 2 || th.Entries[0].Text != "first **post**" || th.Entries[1].Text != "second post" {
		t.Fatalf("%+v", th.Entries)
	}
	for i, e := range th.Entries {
		if !e.Quoted || !e.Markdown || e.Actor != "session" || e.PostID != pid(i+1) {
			t.Errorf("entry %+v", e)
		}
	}
	if !th.TitleQuoted || th.Title != "Laptop: fixing the feed" || th.NeedsOwner || th.State != StateOpen || th.LastActivityAt != at(5) {
		t.Errorf("%q %v %v %s", th.Title, th.TitleQuoted, th.NeedsOwner, th.State)
	}
	if th := lap(t, 10, laptopRaw(1, "s1", 0, "x", nil)); th.Title != "Laptop: demo@work" {
		t.Errorf("title %q", th.Title)
	}
	// the list preview is the newest post
	f := New(100, 1<<20)
	f.Ingest(laptopRaw(1, "s1", 0, "old", nil))
	f.Ingest(laptopRaw(2, "s1", 1, "new", nil))
	if page, _, _ := f.List(5, ""); len(page) != 1 || len(page[0].Entries) != 1 || page[0].Entries[0].Text != "new" {
		t.Errorf("%+v", page)
	}
	// an untrusted label and text are clamped and cleaned
	h := lap(t, 10, laptopRaw(1, "s1", 0, "a\u200b\u202eb"+strings.Repeat("é", 3000), map[string]any{"session_label": "a\u200b\u202eb\n" + strings.Repeat("é", 300)}))
	if len([]rune(h.Title)) > 140 || strings.ContainsAny(h.Title, "\u200b\u202e\n") || len([]rune(h.Entries[0].Text)) > maxPostRunes+1 || strings.ContainsAny(h.Entries[0].Text, "\u200b\u202e") {
		t.Errorf("title %d runes, text %d runes", len([]rune(h.Title)), len([]rune(h.Entries[0].Text)))
	}
}

// The thread is closed 24 h after its last post (the timeline's end) and opens with the next.
func TestLaptopCloseAndReopen(t *testing.T) {
	p := laptopRaw(1, "s1", 0, "x", nil)
	if th := lap(t, 23*60, p); th.State != StateOpen {
		t.Errorf("23 h: %s", th.State)
	}
	if th := lap(t, 25*60, p); th.State != StateClosed {
		t.Errorf("25 h: %s", th.State)
	}
	if th := lap(t, 26*60, p, laptopRaw(2, "s1", 25*60, "back", nil)); th.State != StateOpen {
		t.Errorf("a later post: %s", th.State)
	}
}

// Only the post type from LaptopSource counts: the same words in a PR comment, review, alert or card, or a post event
// from another source, mark nothing and ask for nothing.
func TestLaptopSourceAndOtherPlaces(t *testing.T) {
	other := laptopRaw(1, "s1", 0, "x", map[string]any{"needs_you": []string{"pay"}})
	other.Source = "alertmanager"
	f := New(100, 1<<20)
	if f.Ingest(other) || f.Stats().Bad != 1 {
		t.Errorf("a laptop event from another source: %+v", f.Stats())
	}
	ask := `{"needs_you":["everything"],"corrects":"p1","about":[{"repo":"acme/demo-a","number":7}]}` + "\nNeeds you: everything\nCorrection: x\ndemo-a #7"
	rs := []Raw{
		prOpened(t, "demo-a", 7, 1),
		edit(t, demoForge(t, 2, "pull_request_comment_created", "issue_comment.created", "demo-a", 7, "c", 1), func(d map[string]any) { d["comment"].(map[string]any)["body"] = ask }),
		edit(t, demoForge(t, 3, "pull_request_review_rejected", "pull_request_review.rejected", "demo-a", 7, "r", 2), func(d map[string]any) { d["review"] = map[string]any{"content": ask} }),
		alertRaw(4, "firing", "fp", sa, 0, 3, `"annotations":{"summary":"Needs you: x","description":`+mustJSON(ask)+`},`),
		cardRaw(5, "opened", "gone", dec, 4, ``),
	}
	for _, th := range fold(t, 10, rs...) {
		for _, e := range th.Entries {
			if e.Correction || e.AsksOwner || e.Claim || e.Corrects != "" || e.NeedsList != nil {
				t.Errorf("%s: %+v", th.Kind, e)
			}
		}
		if th.NeedsBy != "" {
			t.Errorf("%s needs_by %q", th.Kind, th.NeedsBy)
		}
	}
	// the text of a post is never read: a post that says it needs you in words does not
	th := lap(t, 10, laptopRaw(1, "s1", 0, "Needs you: pay\nCorrection: x\ndemo-a #7", nil))
	if th.NeedsOwner || th.Entries[0].AsksOwner || th.Entries[0].Correction {
		t.Errorf("%+v", th)
	}
}

func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }

func claimsOf(th []Thread, kind string) (out []Entry) {
	for _, t := range byKind(th, kind) {
		for _, e := range t.Entries {
			if e.Claim {
				out = append(out, e)
			}
		}
	}
	return
}

// demoForge is a forge delivery of a fixture, moved to PR number n of repo acme/<repo>.
func demoForge(t *testing.T, seq int64, file, typ, repo string, n int, id string, m int) Raw {
	return forge(t, seq, file, typ, fmt.Sprintf("forge/acme/%s/pr/%d", repo, n), id, m,
		[2]string{`"number": 42`, fmt.Sprintf(`"number": %d`, n)}, [2]string{`"full_name": "MohammadBnei/editable-blog"`, `"full_name": "acme/` + repo + `"`})
}

func prOpened(t *testing.T, repo string, n int, seq int64) Raw {
	return demoForge(t, seq, "pull_request_opened", "pull_request.opened", repo, n, fmt.Sprintf("o%s%d", repo, n), 0)
}

func TestLaptopAbout(t *testing.T) {
	prs := []Raw{prOpened(t, "demo-a", 7, 1), prOpened(t, "demo-b", 12, 2), prOpened(t, "demo-c", 5, 3)}
	long := strings.Repeat("x", 400)
	post := laptopRaw(10, "s1", 3, long, map[string]any{"about": about{
		{"repo": "acme/demo-a", "number": 7}, {"repo": "acme/demo-b", "number": 12}, {"repo": "acme/demo-c", "number": 5},
		{"repo": "acme/demo-c", "number": 5}, // twice: once
		{"repo": "acme/demo-a", "number": 9999}, {"repo": "demo-c", "number": 1}, {"repo": "acme/demo-c", "number": 0}, {"repo": "a#b/x", "number": 2},
	}})
	th := fold(t, 10, append(prs, post)...)
	cl := claimsOf(th, KindPR)
	if len(cl) != 3 {
		t.Fatalf("%d claim entries: %+v", len(cl), cl)
	}
	lt := byKind(th, KindLaptop)[0]
	for _, c := range cl {
		if !c.Quoted || c.Actor != "session" || c.Thread != lt.ID || c.PostID != pid(10) || len([]rune(c.Text)) != 281 || !strings.HasSuffix(c.Text, "…") {
			t.Errorf("claim %+v", c)
		}
	}
	// a claim changes nothing else of its PR thread
	base := fold(t, 10, prs...)
	for i, p := range byKind(th, KindPR) {
		b := byKind(base, KindPR)[i]
		if p.ID != b.ID || p.State != b.State || p.Title != b.Title || p.LastActivityAt != b.LastActivityAt || p.NeedsOwner != b.NeedsOwner || p.OpenedAt != b.OpenedAt {
			t.Errorf("%s changed: %+v vs %+v", p.Title, p, b)
		}
	}
	// a PR the feed has no thread for gets none: nothing but the laptop thread exists
	if th := fold(t, 10, post); len(th) != 1 || th[0].Kind != KindLaptop {
		t.Errorf("a claim made threads: %d", len(th))
	}
	// a closed PR stays closed
	closed := demoForge(t, 4, "pull_request_closed", "pull_request.closed", "demo-a", 7, "cl", 1)
	for _, p := range byKind(fold(t, 10, prs[0], closed, post), KindPR) {
		if strings.Contains(p.Title, "demo-a#7") && p.State != StateClosed {
			t.Errorf("a claim reopened %s", p.Title)
		}
	}
	// a short text is copied whole, without the ellipsis
	short := laptopRaw(11, "s2", 4, "short", map[string]any{"about": about{{"repo": "acme/demo-c", "number": 5}}})
	if cl := claimsOf(fold(t, 10, append(prs, short)...), KindPR); len(cl) != 1 || cl[0].Text != "short" {
		t.Errorf("%+v", cl)
	}
	// the PR thread's place in the list is its own activity's, not the claim's
	f := New(100, 1<<20)
	for _, r := range append(prs, post) {
		f.Ingest(r)
	}
	page, _, _ := f.List(10, "")
	for _, p := range page {
		if p.Kind == KindPR && !p.LastActivityAt.Equal(at(0)) {
			t.Errorf("a claim moved %s to %v", p.Title, p.LastActivityAt)
		}
	}
}

// At most 10 PRs per post.
func TestLaptopAboutCaps(t *testing.T) {
	var ab about
	for i := 1; i <= 30; i++ {
		ab = append(ab, map[string]any{"repo": "acme/demo", "number": i})
	}
	e, ok := Parse(laptopRaw(1, "s", 0, "x", map[string]any{"about": ab}))
	if !ok || len(e.About) != 10 {
		t.Errorf("%v %d", ok, len(e.About))
	}
}

// A PR thread keeps its newest 50 claims, oldest dropped, and its entries stay in time order.
func TestLaptopClaimsPerPR(t *testing.T) {
	rs := []Raw{prOpened(t, "demo-a", 7, 1), demoForge(t, 2, "pull_request_comment_created", "issue_comment.created", "demo-a", 7, "c", 100)} // after every claim
	for i := 0; i < 70; i++ {
		rs = append(rs, laptopRaw(int64(10+i), "s1", 1+i, fmt.Sprintf("post %d", i), map[string]any{"about": about{{"repo": "acme/demo-a", "number": 7}}}))
	}
	cl := claimsOf(fold(t, 200, rs...), KindPR)
	if len(cl) != 50 || cl[0].Text != "post 20" || cl[49].Text != "post 69" {
		t.Errorf("%d claims, first %q last %q", len(cl), cl[0].Text, cl[len(cl)-1].Text)
	}
	p := byKind(fold(t, 200, rs...), KindPR)[0]
	for i := 1; i < len(p.Entries); i++ {
		if p.Entries[i].Time.Before(p.Entries[i-1].Time) {
			t.Fatal("entries out of order")
		}
	}
}

// The latest post that has the field wins; an empty list clears; posts without it leave it; it holds 7 days whatever
// the thread's state.
func TestLaptopNeedsYou(t *testing.T) {
	ask := laptopRaw(1, "s1", 0, "work", map[string]any{"needs_you": []string{"approve #5", "the key"}})
	th := lap(t, 10, ask)
	if !th.NeedsOwner || th.NeedsBy != "session" {
		t.Fatalf("%v %q", th.NeedsOwner, th.NeedsBy)
	}
	if e := th.Entries[0]; !e.AsksOwner || strings.Join(e.NeedsList, "|") != "approve #5|the key" || !e.Time.Equal(at(0)) {
		t.Errorf("%+v", e)
	}
	// a later post without the field leaves it; a newer ask wins and the entry flag moves
	two := lap(t, 10, ask, laptopRaw(2, "s1", 3, "just a post", nil), laptopRaw(3, "s1", 5, "again", map[string]any{"needs_you": []string{"other"}}))
	if !two.NeedsOwner || two.Entries[0].AsksOwner || !two.Entries[2].AsksOwner || two.Entries[2].NeedsList[0] != "other" || two.Entries[1].AsksOwner {
		t.Errorf("%+v", two.Entries)
	}
	if th := lap(t, 10, ask, laptopRaw(2, "s1", 3, "just a post", nil)); !th.NeedsOwner || !th.Entries[0].AsksOwner {
		t.Error("a post without the field cleared it")
	}
	if th := lap(t, 10, ask, laptopRaw(2, "s1", 3, "done", map[string]any{"needs_you": []string{}})); th.NeedsOwner || th.NeedsBy != "" || th.Entries[0].AsksOwner || th.Entries[1].AsksOwner {
		t.Errorf("an empty list did not clear: %+v", th)
	}
	// another session's post does not touch it
	other := byKind(fold(t, 10, ask, laptopRaw(2, "s2", 3, "x", map[string]any{"needs_you": []string{}})), KindLaptop)
	need := 0
	for _, o := range other {
		if o.NeedsOwner {
			need++
		}
	}
	if len(other) != 2 || need != 1 {
		t.Errorf("%d threads, %d need the owner", len(other), need)
	}
	// the thread closes at 24 h, the ask stands to 7 days
	if th := lap(t, 25*60, ask); th.State != StateClosed || !th.NeedsOwner || !th.Entries[0].AsksOwner {
		t.Errorf("25 h: %s %v", th.State, th.NeedsOwner)
	}
	if th := lap(t, 6*24*60+23*60, ask); th.State != StateClosed || !th.NeedsOwner {
		t.Errorf("6 d 23 h: %s %v", th.State, th.NeedsOwner)
	}
	if th := lap(t, 7*24*60, ask); th.NeedsOwner || th.NeedsBy != "" || th.Entries[0].AsksOwner {
		t.Errorf("7 d: %v", th.NeedsOwner)
	}
	// a newer post with the field renews it
	if th := lap(t, 8*24*60, ask, laptopRaw(2, "s1", 3*24*60, "again", map[string]any{"needs_you": []string{"again"}})); !th.NeedsOwner || th.Entries[1].NeedsList[0] != "again" {
		t.Errorf("renewed: %+v", th)
	}
	// items are cleaned and bounded
	big := laptopRaw(1, "s1", 0, "x", map[string]any{"needs_you": append(make([]string, 0), strings.Repeat("é", 500), "", "a\u200bb")})
	if e, _ := Parse(big); len(e.NeedsList) != 2 || len([]rune(e.NeedsList[0])) > needRunes+1 || e.NeedsList[1] != "ab" {
		t.Errorf("%q", e.NeedsList)
	}
	// a non-empty field of objects or numbers has no items: it clears
	if e, ok := Parse(laptopRaw(1, "s", 0, "x", map[string]any{"needs_you": []any{1, map[string]any{"a": 1}}})); !ok || !e.NeedsSet || len(e.NeedsList) != 0 {
		t.Errorf("%+v", e)
	}
}

// corrects names a post of the same session: the entry says so and links, the corrected one says by whom; any other
// value is shown as a plain post.
func TestLaptopCorrects(t *testing.T) {
	th := lap(t, 10,
		laptopRaw(1, "s1", 0, "first", nil),
		laptopRaw(2, "s1", 1, "second", nil),
		laptopRaw(3, "s1", 2, "oops, first was wrong", map[string]any{"corrects": pid(1)}),
		laptopRaw(4, "s1", 3, "names nothing", map[string]any{"corrects": pid(99)}),
		laptopRaw(5, "s1", 4, "names itself", map[string]any{"corrects": pid(5)}),
	)
	e := th.Entries
	if !e[2].Correction || e[2].Corrects != pid(1) || e[0].CorrectedBy != pid(3) || e[0].Correction || e[1].CorrectedBy != "" {
		t.Errorf("%+v", e)
	}
	for _, i := range []int{3, 4} {
		if e[i].Correction || e[i].Corrects != "" || e[i].CorrectedBy != "" {
			t.Errorf("entry %d is not plain: %+v", i, e[i])
		}
	}
	// a post of another session cannot be corrected
	other := byKind(fold(t, 10, laptopRaw(1, "s1", 0, "first", nil), laptopRaw(2, "s2", 1, "from elsewhere", map[string]any{"corrects": pid(1)})), KindLaptop)
	for _, o := range other {
		for _, en := range o.Entries {
			if en.Correction || en.CorrectedBy != "" || en.Corrects != "" {
				t.Errorf("%+v", en)
			}
		}
	}
	// the correcting post may come before the corrected one in the events' order
	rev := lap(t, 10, laptopRaw(3, "s1", 2, "oops", map[string]any{"corrects": pid(1)}), laptopRaw(1, "s1", 0, "first", nil))
	if !rev.Entries[1].Correction || rev.Entries[0].CorrectedBy != pid(3) {
		t.Errorf("%+v", rev.Entries)
	}
}

func TestLaptopParseRefuses(t *testing.T) {
	for name, r := range map[string]Raw{
		"no session": laptopRaw(1, "", 0, "x", nil),
		"no text":    laptopRaw(1, "s", 0, "\u200b", nil),
		"bad json":   {Seq: 1, Source: LaptopSource, ID: "x", Type: "dev.bnei." + laptopType, Time: at(0), Data: []byte("{")},
		"not a uuid": laptopRaw(1, "a#b", 0, "x", nil),
		"uppercase":  laptopRaw(1, "AAAAAAAA-2222-4333-8444-555555555555", 0, "x", nil),
		"post_id":    laptopRaw(1, "s1", 0, "x", map[string]any{"post_id": "p1"}),
		"corrects":   laptopRaw(1, "s1", 0, "x", map[string]any{"corrects": "p1"}),
	} {
		if _, ok := Parse(r); ok {
			t.Errorf("%s parsed", name)
		}
	}
}

// A post event as one is stored: the data is the JSON of its fields (absent ones omitted), the id is
// post/<session>/<post_id>, the subject laptop/session/<session>, the source ingest/laptop.
const (
	postSession = "11111111-2222-4333-8444-555555555555"
	postPost    = "66666666-7777-4888-9999-000000000000"
	postOther   = "66666666-7777-4888-9999-000000000001"
)

func postRaw(data string) Raw {
	return Raw{Seq: 1, Source: "ingest/laptop", ID: "post/" + postSession + "/" + postPost, Type: "dev.bnei.laptop.post.created",
		Subject: "laptop/session/" + postSession, Time: at(0), Data: []byte(data)}
}

func TestLaptopPostPayload(t *testing.T) {
	full := `{"session":"` + postSession + `","session_label":"feed","repo":"o/r","branch":"feat/x","post_id":"` + postPost +
		`","text":"Done.","about":[{"repo":"o/r","number":7}],"needs_you":["merge #7"],"corrects":"` + postOther + `"}`
	e, ok := Parse(postRaw(full))
	if !ok || e.Key != postSession || e.Title != "feed" || e.Repo != "o/r" || e.Branch != "feat/x" || e.PostID != postPost || e.Text != "Done." ||
		len(e.About) != 1 || e.About[0] != (Claim{"o/r", 7}) || !e.NeedsSet || len(e.NeedsList) != 1 || e.NeedsList[0] != "merge #7" || e.Corrects != postOther {
		t.Fatalf("%v %+v", ok, e)
	}
	th := lap(t, 5, postRaw(full))
	if th.Title != "Laptop: feed" || !th.NeedsOwner || th.NeedsBy != "session" || th.Entries[0].PostID != postPost || th.Entries[0].Corrects != "" {
		t.Errorf("%+v", th)
	}
	// only the ids and the text are required, and an absent field stays absent; an empty needs_you is present
	min, ok := Parse(postRaw(`{"session":"` + postSession + `","post_id":"` + postPost + `","text":"Done."}`))
	if !ok || min.NeedsSet || min.Title != "" || len(min.About) != 0 {
		t.Errorf("%v %+v", ok, min)
	}
	empty, ok := Parse(postRaw(`{"session":"` + postSession + `","post_id":"` + postPost + `","text":"Done.","needs_you":[]}`))
	if !ok || !empty.NeedsSet || len(empty.NeedsList) != 0 {
		t.Errorf("%v %+v", ok, empty)
	}
}

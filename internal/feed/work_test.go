package feed

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The forge fixtures are the bridge's own (ukubi-bridge internal/forgejo/testdata); Data is the delivery body as
// stored, the subject and ce_id are what forgejo.Normalize builds ("forge/<owner>/<repo>/pr/<n>", factID).
const (
	forgeSrc = "git.bnei.dev/MohammadBnei/editable-blog"
	prSubj   = "forge/MohammadBnei/editable-blog/pr/42"
	head     = "3f1c0a9e5b2d4c6a8e0f1b3d5c7a9e1f2b4d6c8a"
	mergeSHA = "9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c"
)

func forge(t *testing.T, seq int64, file, typ, subject, id string, m int, edit ...[2]string) Raw {
	t.Helper()
	b, err := os.ReadFile("testdata/forgejo/" + file + ".json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, e := range edit {
		if !strings.Contains(s, e[0]) {
			t.Fatalf("%s has no %q", file, e[0])
		}
		s = strings.ReplaceAll(s, e[0], e[1])
	}
	return Raw{Seq: seq, Source: forgeSrc, ID: id, Type: "dev.bnei.forge." + typ, Subject: subject, Time: at(m), Data: []byte(s)}
}

var merged = [][2]string{{`"merged": false`, `"merged": true`}, {`"merge_commit_sha": null`, `"merge_commit_sha": "` + mergeSHA + `"`}}

func prLife(t *testing.T) []Raw {
	return []Raw{
		forge(t, 1, "pull_request_opened", "pull_request.opened", prSubj, "pr/42/opened", 0),
		forge(t, 2, "pull_request_synchronized", "pull_request.synchronized", prSubj, "d-sync", 1),
		forge(t, 3, "pull_request_review_rejected", "pull_request_review.rejected", prSubj, "pr/42/review/rejected/x", 2),
		forge(t, 4, "pull_request_comment_created", "issue_comment.created", prSubj, "comment/7", 3),
		forge(t, 5, "action_run_success", "action_run.success", "forge/MohammadBnei/editable-blog/run/117", "d-run", 4),
		forge(t, 6, "pull_request_closed", "pull_request.closed", prSubj, "pr/42/closed/1", 5, merged...),
	}
}

func byKind(th []Thread, kind string) []Thread {
	var out []Thread
	for _, t := range th {
		if t.Kind == kind {
			out = append(out, t)
		}
	}
	return out
}

func texts(t Thread) (all []string) {
	for _, e := range t.Entries {
		all = append(all, fmt.Sprintf("%v:%s", e.Quoted, e.Text))
	}
	return
}

func TestPRThreadLife(t *testing.T) {
	th := byKind(fold(t, 10, prLife(t)...), KindPR)
	if len(th) != 1 {
		t.Fatalf("one thread per repo and number: %d", len(th))
	}
	p := th[0]
	if p.State != StateClosed || p.NeedsOwner || !p.TitleQuoted || p.Title != "PR MohammadBnei/editable-blog#42: feat: dark mode" {
		t.Errorf("%s %v %v %q", p.State, p.NeedsOwner, p.TitleQuoted, p.Title)
	}
	got := strings.Join(texts(p), "\n")
	for _, want := range []string{"false:opened", "false:pushed 3f1c0a9e5b2d", "false:review: changes requested", "true:MohammadBnei: Needs a test.",
		"false:comment", "false:CI run 117: success", "false:merged"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// the closed thread takes a late comment without opening again
	late := append(prLife(t), forge(t, 7, "pull_request_comment_created", "issue_comment.created", prSubj, "comment/8", 20))
	if p := byKind(fold(t, 30, late...), KindPR)[0]; p.State != StateClosed || p.LastActivityAt != at(20) {
		t.Errorf("late comment: %s %v", p.State, p.LastActivityAt)
	}
	// reopened opens it, a second closed closes it
	re := append(prLife(t), forge(t, 8, "pull_request_reopened", "pull_request.reopened", prSubj, "d-re", 21))
	if p := byKind(fold(t, 30, re...), KindPR)[0]; p.State != StateOpen {
		t.Errorf("reopened: %s", p.State)
	}
	// the first sight of a PR (no opened in the feed's memory) opens it; a closed with no merge says closed
	first := fold(t, 10, forge(t, 1, "pull_request_synchronized", "pull_request.synchronized", prSubj, "d1", 0))
	if first[0].State != StateOpen {
		t.Errorf("first sight: %s", first[0].State)
	}
	if p := fold(t, 10, forge(t, 1, "pull_request_closed", "pull_request.closed", prSubj, "pr/42/closed/2", 0))[0]; p.State != StateClosed || p.Entries[0].Text != "closed" {
		t.Errorf("closed: %s %q", p.State, p.Entries[0].Text)
	}
	// an opened stamped after the closed (the relist uses its own times) does not reopen
	skew := []Raw{prLife(t)[5], forge(t, 9, "pull_request_opened", "pull_request.opened", prSubj, "pr/42/opened", 9)}
	if p := byKind(fold(t, 30, skew...), KindPR)[0]; p.State != StateClosed {
		t.Errorf("opened after closed: %s", p.State)
	}
}

// Thread key: the same number in another repo is another thread, the same repo and number is one.
func TestPRThreadKey(t *testing.T) {
	other := "forge/MohammadBnei/mountain/pr/42"
	th := byKind(fold(t, 10,
		forge(t, 1, "pull_request_opened", "pull_request.opened", prSubj, "a", 0),
		forge(t, 2, "pull_request_opened", "pull_request.opened", other, "b", 0, [2]string{`"full_name": "MohammadBnei/editable-blog"`, `"full_name": "MohammadBnei/mountain"`}),
		forge(t, 3, "pull_request_synchronized", "pull_request.synchronized", prSubj, "c", 1),
	), KindPR)
	if len(th) != 2 {
		t.Fatalf("%d threads", len(th))
	}
}

// Everything that came from outside is quoted; no unquoted entry or title may hold it.
func TestPROutsideTextIsQuoted(t *testing.T) {
	th := byKind(fold(t, 10, append(prLife(t),
		forge(t, 9, "pull_request_review_approved", "pull_request_review.approved", prSubj, "pr/42/review/approved/y", 6))...), KindPR)[0]
	outside := []string{"feat: dark mode", "Needs a test.", "LGTM", "stranger", "MohammadBnei", "ukubi-fleet", "feat/dark-mode", "Closes #9"}
	for _, e := range th.Entries {
		if e.Quoted {
			continue
		}
		for _, o := range outside {
			if strings.Contains(e.Text, o) {
				t.Errorf("unquoted entry %q holds outside text %q", e.Text, o)
			}
		}
	}
	if !strings.Contains(strings.Join(texts(th), "\n"), "true:ukubi-review: LGTM") || !strings.Contains(strings.Join(texts(th), "\n"), "@ukubi-fleet please take this") {
		t.Errorf("quotes missing:\n%s", strings.Join(texts(th), "\n"))
	}
}

// A PR comment and its relisted copy share a ce_id: one entry. An issue comment is not the feed's and is not bad.
func TestPRDedupAndIssueComment(t *testing.T) {
	f := New(100, 1<<20)
	a := forge(t, 1, "pull_request_comment_created", "issue_comment.created", prSubj, "comment/7", 3)
	b := a
	b.Seq = 2
	f.Ingest(a)
	if f.Ingest(b) {
		t.Error("a repeated (source, id) stored twice")
	}
	if f.Ingest(forge(t, 3, "issue_comment_created", "issue_comment.created", "forge/MohammadBnei/editable-blog/issue/9", "comment/9", 4)) {
		t.Error("an issue comment was stored")
	}
	if s := f.Stats(); s.Events != 1 || s.Bad != 0 {
		t.Errorf("%+v", s)
	}
	// a label event, an issue and a push are not read, and not bad
	if f.Ingest(Raw{Seq: 5, Source: forgeSrc, ID: "x", Type: "dev.bnei.forge.pull_request.label_updated", Subject: prSubj, Time: at(1), Data: a.Data}) || f.Stats().Bad != 0 {
		t.Error("label_updated")
	}
	if th, _, _ := f.List(10, ""); len(th) != 1 || len(th[0].Entries) != 1 {
		t.Errorf("%+v", th)
	}
}

func TestParseForgeRefuses(t *testing.T) {
	for name, r := range map[string]Raw{
		"no number":   forge(t, 1, "pull_request_opened", "pull_request.opened", "forge/MohammadBnei/editable-blog/pr/x", "a", 0),
		"odd subject": forge(t, 1, "pull_request_opened", "pull_request.opened", "forge/MohammadBnei/editable-blog/pr/42/x", "a", 0),
		"bad repo":    forge(t, 1, "pull_request_opened", "pull_request.opened", "forge/Mohammad#Bnei/editable-blog/pr/42", "a", 0),
		"run on pr":   forge(t, 1, "action_run_success", "action_run.success", prSubj, "a", 0),
		"run no sha":  forge(t, 1, "action_run_success", "action_run.success", "forge/MohammadBnei/editable-blog/run/117", "a", 0, [2]string{head, "XYZ"}),
		"run number":  forge(t, 1, "action_run_success", "action_run.success", "forge/MohammadBnei/editable-blog/run/118", "a", 0),
	} {
		if _, ok := Parse(r); ok {
			t.Errorf("%s: parsed", name)
		}
	}
	// a sha that is not hex never reaches system text
	e, ok := Parse(forge(t, 1, "pull_request_synchronized", "pull_request.synchronized", prSubj, "a", 0, [2]string{head, "<b>x</b>"}))
	if !ok || e.Head != "" {
		t.Errorf("head %q %v", e.Head, ok)
	}
}

// A merged PR and the rollout of its merge commit link both ways; a rollout of another commit does not; the list
// answer carries no links.
func TestPRDeployLinks(t *testing.T) {
	roll := func(seq int64, rev string) Raw {
		r := synced(seq, "gitops", 6)
		r.Data = []byte(strings.Replace(string(r.Data), "abcdef1234567890", rev, 1))
		return r
	}
	th := fold(t, 8, append(prLife(t), roll(20, mergeSHA))...)
	p, d := byKind(th, KindPR)[0], byKind(th, KindDeploy)[0]
	if !hasLink(p, "deploy", d.ID) || !hasLink(d, "pull_request", p.ID) {
		t.Errorf("links: pr %v deploy %v", p.Links, d.Links)
	}
	th = fold(t, 8, append(prLife(t), roll(20, "abcdef1234567890"))...)
	if p, d := byKind(th, KindPR)[0], byKind(th, KindDeploy)[0]; hasKind(p, "deploy") || hasKind(d, "pull_request") {
		t.Errorf("a rollout of another commit linked: %v %v", p.Links, d.Links)
	}
	// a PR that was closed, not merged, never links, whatever sha a rollout names
	th = fold(t, 8,
		forge(t, 1, "pull_request_closed", "pull_request.closed", prSubj, "pr/42/closed/3", 0, [2]string{`"merge_commit_sha": null`, `"merge_commit_sha": "` + mergeSHA + `"`}),
		roll(20, mergeSHA))
	if hasKind(byKind(th, KindPR)[0], "deploy") {
		t.Error("an unmerged PR linked")
	}
	f := New(100, 1<<20)
	for _, r := range append(prLife(t), roll(20, mergeSHA)) {
		f.Ingest(r)
	}
	f.now = func() time.Time { return at(8) }
	page, _, _ := f.List(10, "")
	for _, x := range page {
		if x.Links != nil {
			t.Errorf("list carries links: %v", x.Links)
		}
	}
	if g, _ := f.Get(p.ID); !hasLink(g, "deploy", d.ID) {
		t.Error("get lost the link")
	}
}

func hasLink(t Thread, kind, ref string) bool {
	for _, l := range t.Links {
		if l.Kind == kind && l.Ref == ref {
			return true
		}
	}
	return false
}

func hasKind(t Thread, kind string) bool {
	for _, l := range t.Links {
		if l.Kind == kind {
			return true
		}
	}
	return false
}

// Links stay capped: a deploy thread joined by many merges holds maxLinks at most.
func TestPRLinksCapped(t *testing.T) {
	var rs []Raw
	var revs []string
	for i := 1; i <= 30; i++ {
		sha := fmt.Sprintf("%040x", i)
		subj := fmt.Sprintf("forge/MohammadBnei/editable-blog/pr/%d", i)
		rs = append(rs, forge(t, int64(i), "pull_request_closed", "pull_request.closed", subj, fmt.Sprintf("pr/%d/closed/1", i), 0,
			[2]string{`"merged": false`, `"merged": true`}, [2]string{`"merge_commit_sha": null`, `"merge_commit_sha": "` + sha + `"`}, [2]string{`"number": 42`, fmt.Sprintf(`"number": %d`, i)}))
		revs = append(revs, `"`+sha+`"`)
	}
	r := synced(99, "gitops", 1)
	r.Data = []byte(strings.Replace(string(r.Data), "abcdef1234567890", strings.Join(revs[:4], ",")[1:len(strings.Join(revs[:4], ","))-1], 1))
	for _, x := range Fold(parse(t, append(rs, r)...), at(2)) {
		if len(x.Links) > maxLinks {
			t.Errorf("%s holds %d links", x.Kind, len(x.Links))
		}
	}
}

// Session events as core publishes them (agent-fleet core/internal/sessionevents): source client/bridge-core,
// subject fleet/session/<id>, ce_ids session/<id>/opened, /turn/<seq>, /ended/<unix>; Data has ids and labels, a
// title the agent wrote, no text.
const sid = "5b0e2c1a-8d41-4f6e-9a77-0c3d5e7f9a11"

func sess(seq int64, kind string, m int, ext string) Raw {
	id := "session/" + sid + "/opened"
	switch kind {
	case "turn_ended":
		id = fmt.Sprintf("session/%s/turn/%d", sid, seq)
	case "ended":
		id = fmt.Sprintf("session/%s/ended/%d", sid, 1000+seq)
	}
	return Raw{Seq: seq, Source: "client/bridge-core", ID: id, Type: "dev.bnei.fleet.session." + kind, Subject: "fleet/session/" + sid, Time: at(m),
		Data: []byte(`{"session_id":"` + sid + `","kind":"repo","repo":"MohammadBnei/mountain","occurred_at":"2026-11-24T10:00:00Z"` + ext + `}`)}
}

func TestSessionThread(t *testing.T) {
	th := byKind(fold(t, 10,
		sess(1, "opened", 0, `,"title":"fix the feed"`),
		sess(2, "turn_ended", 1, `,"title":"fix the feed","thread_key":"k","seq":5,"duration_ms":3200,"is_error":false`),
		sess(3, "turn_ended", 2, `,"thread_key":"k","seq":6,"duration_ms":900,"is_error":true`),
		sess(4, "ended", 3, ``),
		sess(5, "turn_ended", 4, `,"thread_key":"k","seq":7`),
	), KindSession)
	if len(th) != 1 {
		t.Fatalf("%d threads", len(th))
	}
	s := th[0]
	if s.State != StateClosed || s.NeedsOwner || !s.TitleQuoted || s.Title != "Session: fix the feed" {
		t.Errorf("%s %v %v %q", s.State, s.NeedsOwner, s.TitleQuoted, s.Title)
	}
	got := strings.Join(texts(s), "\n")
	for _, want := range []string{"false:opened (repo)", "true:fix the feed", "false:turn ended (3.2s)", "false:turn failed (900ms)", "false:ended"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if s.LastActivityAt != at(4) {
		t.Errorf("the late turn is an entry: %v", s.LastActivityAt)
	}
	// a session open before core published has turns and no opened: open, with its id for a title
	s = byKind(fold(t, 10, sess(2, "turn_ended", 1, `,"seq":5`)), KindSession)[0]
	if s.State != StateOpen || s.Title != "Session "+sid[:8] || s.TitleQuoted {
		t.Errorf("no opened: %s %q %v", s.State, s.Title, s.TitleQuoted)
	}
	// the title is cut and cleaned again here
	e, _ := Parse(sess(1, "opened", 0, `,"title":"`+strings.Repeat("é", 300)+`\n\u202e"`))
	if n := len([]rune(e.Title)); n > 121 || strings.ContainsAny(e.Title, "\n\u202e") {
		t.Errorf("title %d runes %q", n, e.Title)
	}
}

// Cards as core publishes them (core/internal/cardevents send): type fleet.card.<opened|answered|expired|stale|
// withdrawn>, ce_id card/<decision>/<state>, subject forge/... or fleet/session/<id>, Data without the ask text.
func cardRaw(seq int64, state, session, decision string, m int, ext string) Raw {
	st := state
	if state == "opened" {
		st = "open"
	}
	return Raw{Seq: seq, Source: "client/bridge-core", ID: "card/" + decision + "/" + st, Type: "dev.bnei.fleet.card." + state,
		Subject: "forge/MohammadBnei/mountain/pr/12", Time: at(m), Data: []byte(fmt.Sprintf(`{"decision_id":%q,"session_id":%q,"state":%q,`+
			`"subject_kind":"pr","subject":"forge/MohammadBnei/mountain/pr/12","drafted_by":"agent","writer_definition":"ops-lead",`+
			`"created_at":"2026-11-24T10:00:00Z","transitioned_at":"2026-11-24T10:00:00Z"%s}`, decision, session, st, ext))}
}

const dec = "c1d2e3f4-0000-4000-8000-000000000001"

func TestCardInSessionThread(t *testing.T) {
	base := []Raw{sess(1, "opened", 0, ``), cardRaw(2, "opened", sid, dec, 1, `,"expires_at":"2026-11-24T12:00:00Z"`)}
	th := fold(t, 5, base...)
	if len(byKind(th, KindCard)) != 0 {
		t.Error("a card of a known session got its own thread")
	}
	s := byKind(th, KindSession)[0]
	if !s.NeedsOwner || s.State != StateOpen {
		t.Errorf("open card: %v %s", s.NeedsOwner, s.State)
	}
	if got := strings.Join(texts(s), "\n"); !strings.Contains(got, "false:card opened") || !strings.Contains(got, "true:card on forge/MohammadBnei/mountain/pr/12") {
		t.Errorf("entries:\n%s", got)
	}
	// every state but open ends the need
	for _, st := range []string{"answered", "expired", "stale", "withdrawn"} {
		s := byKind(fold(t, 5, append(base, cardRaw(3, st, sid, dec, 2, ``))...), KindSession)[0]
		if s.NeedsOwner {
			t.Errorf("%s: still needs the owner", st)
		}
	}
	if s := byKind(fold(t, 5, append(base, cardRaw(3, "answered", sid, dec, 2, `,"option_id":"2"`))...), KindSession)[0]; !strings.Contains(strings.Join(texts(s), "\n"), "card answered (option 2)") {
		t.Errorf("%v", texts(s))
	}
	// a card whose closing event was lost (a deleted session) closes at expires_at
	if s := byKind(fold(t, 200, base...), KindSession)[0]; s.NeedsOwner {
		t.Error("an expired open card still needs the owner")
	}
	// a second card keeps the need after the first is answered
	two := append(base, cardRaw(3, "opened", sid, "c1d2e3f4-0000-4000-8000-000000000002", 2, ``), cardRaw(4, "answered", sid, dec, 3, ``))
	if s := byKind(fold(t, 5, two...), KindSession)[0]; !s.NeedsOwner {
		t.Error("one answered card cleared the need of the other")
	}
	// an opened that sorts after the answer does not reopen it
	if s := byKind(fold(t, 5, sess(1, "opened", 0, ``), cardRaw(3, "answered", sid, dec, 2, ``), cardRaw(2, "opened", sid, dec, 3, ``)), KindSession)[0]; s.NeedsOwner {
		t.Error("a late opened reopened the card")
	}
}

func TestCardOwnThread(t *testing.T) {
	th := fold(t, 5, cardRaw(1, "opened", "gone", dec, 1, `,"expires_at":"2026-11-24T12:00:00Z"`))
	if len(th) != 1 || th[0].Kind != KindCard || !th[0].NeedsOwner || th[0].State != StateOpen || !th[0].TitleQuoted {
		t.Fatalf("%+v", th)
	}
	th = fold(t, 5, cardRaw(1, "opened", "gone", dec, 1, ``), cardRaw(2, "answered", "gone", dec, 2, ``))
	if th[0].NeedsOwner || th[0].State != StateClosed {
		t.Errorf("%v %s", th[0].NeedsOwner, th[0].State)
	}
	// a card that comes before the first event of its session still lands in the session's thread
	th = fold(t, 5, cardRaw(1, "opened", sid, dec, 0, ``), sess(2, "turn_ended", 1, `,"seq":3`))
	if len(th) != 1 || th[0].Kind != KindSession || !th[0].NeedsOwner {
		t.Errorf("%+v", th)
	}
}

// The new kinds fold the same whatever order they arrive in.
func TestWorkFoldOrderFree(t *testing.T) {
	rs := append(prLife(t), sess(10, "opened", 0, `,"title":"x"`), sess(11, "turn_ended", 1, `,"seq":1`), cardRaw(12, "opened", sid, dec, 2, ``),
		cardRaw(13, "answered", sid, dec, 3, ``), sess(14, "ended", 4, ``), synced(15, "gitops", 6))
	want := Fold(parse(t, rs...), at(30))
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
		if got := Fold(parse(t, rs...), at(30)); !reflect.DeepEqual(got, want) {
			t.Fatalf("order changed the answer")
		}
	}
}

// Busy PR traffic is evicted before the alerts and rollouts: the kind holding the most loses its oldest event.
func TestCapsPerKind(t *testing.T) {
	f := New(10, 1<<20)
	f.Ingest(alertRaw(1, "firing", "fp1", sa, 0, 0, ""))
	f.Ingest(synced(2, "gitops", 0))
	for i := 0; i < 40; i++ {
		f.Ingest(forge(t, int64(10+i), "pull_request_synchronized", "pull_request.synchronized", prSubj, fmt.Sprintf("d%d", i), 1+i))
	}
	th, _, _ := f.List(50, "")
	k := map[string]int{}
	for _, x := range th {
		k[x.Kind]++
	}
	if k[KindAlert] != 1 || k[KindDeploy] != 1 || f.Stats().Events != 10 || f.Stats().Dropped != 32 {
		t.Errorf("threads %v, %+v", k, f.Stats())
	}
	// the floor is per kind: an old PR event refused after its eviction does not refuse a new alert of a lower seq
	if f.Ingest(forge(t, 11, "pull_request_synchronized", "pull_request.synchronized", prSubj, "late", 1)) {
		t.Error("an evicted PR event came back")
	}
	if !f.Ingest(alertRaw(5, "firing", "fp2", sa, 0, 2, "")) {
		t.Error("the PR floor refused an alert")
	}
	// bytes: the byte cap evicts from the kind holding the most bytes
	g := New(1000, 6000)
	g.Ingest(alertRaw(1, "firing", "fp1", sa, 0, 0, ""))
	for i := 0; i < 60; i++ {
		g.Ingest(forge(t, int64(10+i), "pull_request_comment_created", "issue_comment.created", prSubj, fmt.Sprintf("c%d", i), 1+i))
	}
	if g.Stats().Bytes > 6000 {
		t.Errorf("bytes %d", g.Stats().Bytes)
	}
	if a, _, _ := g.List(5, ""); len(byKind(a, KindAlert)) != 1 {
		t.Error("the alert was evicted by PR traffic")
	}
}

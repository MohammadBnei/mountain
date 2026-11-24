package feed

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	KindAlert   = "alert"
	KindDeploy  = "deploy"
	StateOpen   = "open"
	StateClosed = "closed"

	// RolloutWindow: a deploy thread closes by itself this long after its last synced (or unhealthy event) unless the
	// app still needs the owner; kwatch absorbs Healthy -> Progressing -> Healthy inside its settle, so most rollouts
	// send nothing after deploy.synced.
	RolloutWindow = 10 * time.Minute
	// FlapWindow: a flapping event counts this long when no health edge follows it (kwatch sends no closing event).
	FlapWindow = 30 * time.Minute
	// ReopenWithin: an unhealthy event this soon after a thread closed takes that thread back; later it is a new one.
	ReopenWithin = 15 * time.Minute

	maxLinks = 20
)

// Entry is one line of a thread. Quoted is true for any text that came from an event's free text (alert
// annotations): the page shows it as a quote and an agent must not act on it.
type Entry struct {
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"` // "system", or "session" for a laptop session's own words
	Text   string    `json:"text"`
	Quoted bool      `json:"quoted"`
	// Laptop posts (all omitted otherwise): Markdown says Text is Markdown; PostID names the post; Correction and
	// Corrects/CorrectedBy join a correcting post and the one it corrects; AsksOwner marks the one post whose needs_you
	// list stands, NeedsList being that list; Claim is a post about a PR, shown in that PR's thread, Thread the post's
	// thread.
	Markdown    bool     `json:"markdown,omitempty"`
	PostID      string   `json:"post_id,omitempty"`
	Correction  bool     `json:"correction,omitempty"`   // this post corrects Corrects
	Corrects    string   `json:"corrects,omitempty"`     // the post_id of the corrected post, in the same thread
	CorrectedBy string   `json:"corrected_by,omitempty"` // the post_id of the post that corrected this one
	AsksOwner   bool     `json:"asks_owner,omitempty"`
	NeedsList   []string `json:"needs_list,omitempty"`
	NeedsCount  int      `json:"needs_count,omitempty"` // the list answer cut NeedsList: how many items there are
	Claim       bool     `json:"claim,omitempty"`
	Thread      string   `json:"thread,omitempty"`
	HasMore     bool     `json:"has_more,omitempty"` // the list answer cut Text; the thread has it whole

	seq int64 // laptop: the event's seq, for ordering what the clock cannot
}

// Link is a reference out of a thread: Ref is a thread id (kind "previous"), a revision, or a URL taken from an
// event (kind "generator"), which is Quoted like any text from outside.
type Link struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Quoted bool   `json:"quoted"`
}

// Thread is what the page shows. TitleQuoted is true when the title came from an event's free text (an alert's name).
type Thread struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	TitleQuoted    bool      `json:"title_quoted"`
	State          string    `json:"state"`
	OpenedAt       time.Time `json:"opened_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	NeedsOwner     bool      `json:"needs_owner"`
	NeedsBy        string    `json:"needs_by,omitempty"` // "session": a laptop session asked, in its own words
	Entries        []Entry   `json:"entries"`
	Links          []Link    `json:"links"`

	cards  []*card // session and card threads: the cards they hold
	post   *post   // laptop: the latest post whose Needs you line stands
	app    *appState
	anchor time.Time // deploy: the last synced or unhealthy event in it
	closed time.Time // deploy: a Healthy edge closed it then; zero while not
}

// appState is one Application's health, kept across its threads.
type appState struct {
	cur       *Thread
	unhealthy bool      // the last health edge was Degraded or Missing
	flapAt    time.Time // the last flapping event, zero once a health edge follows it
}

// needs: the app wants the owner at t.
func (a *appState) needs(t time.Time) bool {
	return a.unhealthy || (!a.flapAt.IsZero() && t.Sub(a.flapAt) <= FlapWindow)
}

// anchorOf is when c's window starts: its last synced, unhealthy or need-ending event, or, for an app that only
// flapped, when the flapping stopped counting.
func (a *appState) anchorOf(c *Thread) time.Time {
	t := c.anchor
	if f := a.flapAt.Add(FlapWindow); !a.unhealthy && !a.flapAt.IsZero() && f.After(t) {
		t = f
	}
	return t
}

// openAt: a.cur is open at t.
func (a *appState) openAt(t time.Time) bool {
	c := a.cur
	return c != nil && c.closed.IsZero() && (a.needs(t) || t.Sub(a.anchorOf(c)) <= RolloutWindow)
}

func threadID(kind, key string) string {
	h := sha256.Sum256([]byte(key))
	return kind + "-" + hex.EncodeToString(h[:8])
}

func (t *Thread) add(at time.Time, text string, quoted bool) {
	t.Entries = append(t.Entries, Entry{Time: at, Actor: "system", Text: text, Quoted: quoted})
	t.LastActivityAt = at
}

func (t *Thread) link(kind, ref string, quoted bool) {
	if ref == "" || len(t.Links) >= maxLinks {
		return
	}
	l := Link{kind, ref, quoted}
	if !slices.Contains(t.Links, l) {
		t.Links = append(t.Links, l)
	}
}

// Known limits, not worked around:
//   - kwatch restarting while an app is Degraded, or its Forget of a deleted app, emits no closing edge, so
//     needs_owner can stay until the next edge.
//   - An edge evicted before its synced changes a thread's id (the id comes from the first event the feed holds).
//   - Prometheus's http generator links are dropped (https only).
//   - A Healthy more than RolloutWindow after the synced (and no unhealthy event) is not shown: the thread closed
//     itself before it came.
//   - Health Progressing and Suspended end a Degraded or Missing need (ArgoCD moves to Progressing when it retries);
//     kwatch's output says nothing more, since it only counts Degraded and Missing as unhealthy. Unknown does not.
//
// Fold builds the threads of evs as they stand at now, newest activity first. Order, duplicates and lateness of evs
// do not matter; the cost is O(n log n).
//
// Alerts: one thread per source, fingerprint, startsAt and episode, as the bridge's InsertAlert scopes an episode. A
// firing opens it, a resolved closes only its own, a later firing of the same key reopens it. A new startsAt (a
// re-fire) is a new thread linked "previous" to the one before it of the same source and fingerprint. An open alert
// needs the owner. A resolved with no firing before it is a closed thread of its own.
//
// Deploys: health belongs to the app, not to a rollout (appState). A rollout thread opens at a deploy.synced and stays
// open while the app needs the owner (its last health edge is Degraded or Missing, or a flapping event came after
// the last health edge within FlapWindow) or RolloutWindow has not passed since its last synced or unhealthy event;
// a health Healthy closes it at once. A synced while one is open joins it, a later one starts a new thread. An
// unhealthy or flapping event with nothing open reopens the thread that closed within ReopenWithin, else starts one,
// so an app that needs the owner always has one open thread. A Healthy or a mere Progressing with nothing open says
// nothing.
func Fold(evs []Event, now time.Time) []Thread {
	s := slices.Clone(evs)
	slices.SortStableFunc(s, func(a, b Event) int { return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.Seq, b.Seq)) })
	type id struct{ src, id string }
	seen := map[id]bool{}
	alerts := map[string]*Thread{}
	lastAlert := map[string]*Thread{} // source|fingerprint -> its newest thread
	apps := map[string]*appState{}
	pr, fl, lp := newPRs(s), newFleet(s), newLaptops()
	type synced struct {
		t   *Thread
		rev string
	}
	var rolled []synced
	var out []*Thread
	for _, e := range s {
		if k := (id{e.Source, e.ID}); seen[k] {
			continue
		} else {
			seen[k] = true
		}
		switch {
		case strings.HasPrefix(e.Type, "forge."):
			pr.apply(e, &out)
			continue
		case strings.HasPrefix(e.Type, "fleet.card."):
			fl.applyCard(e, &out)
			continue
		case strings.HasPrefix(e.Type, "fleet.session."):
			fl.applySession(e, &out)
			continue
		case e.Type == laptopType:
			lp.apply(e, &out)
			continue
		}
		if e.Type == "alert.firing" || e.Type == "alert.resolved" {
			key := fmt.Sprintf("%s|%s|%s|%d", e.Source, e.Key, e.StartsAt, e.Episode)
			t := alerts[key]
			if t == nil {
				t = &Thread{ID: threadID(KindAlert, key), Kind: KindAlert, OpenedAt: e.Time, Title: "Alert " + clean(e.Key, 8)}
				fp := e.Source + "|" + e.Key
				if p := lastAlert[fp]; p != nil {
					t.link("previous", p.ID, false)
				}
				alerts[key], lastAlert[fp], out = t, t, append(out, t)
			}
			if e.Name != "" {
				t.Title, t.TitleQuoted = "Alert: "+e.Name, true
			}
			t.link("generator", e.URL, true)
			if e.Type == "alert.resolved" {
				t.add(e.Time, "resolved", false)
				t.State, t.NeedsOwner = StateClosed, false
				continue
			}
			text := "firing"
			if t.State == StateClosed {
				text = "firing again"
			}
			t.add(e.Time, text, false)
			for _, q := range []string{e.Summary, e.Description} {
				if q != "" {
					t.add(e.Time, q, true)
				}
			}
			t.State, t.NeedsOwner = StateOpen, true
			continue
		}
		a := apps[e.Key]
		if a == nil {
			a = &appState{}
			apps[e.Key] = a
		}
		start := func(text string) {
			t := &Thread{ID: threadID(KindDeploy, e.Source+"|"+e.ID), Kind: KindDeploy, OpenedAt: e.Time, Title: "Deploy: " + e.Key, app: a}
			a.cur, out = t, append(out, t)
			t.add(e.Time, text, false)
		}
		// trouble: an unhealthy or flapping event joins the open thread, else reopens the last, else starts one.
		trouble := func(text string) {
			switch {
			case a.openAt(e.Time):
				a.cur.add(e.Time, text, false)
			case a.cur != nil && e.Time.Sub(a.cur.closeTime()) <= ReopenWithin:
				a.cur.closed = time.Time{}
				a.cur.add(e.Time, "reopened: "+text, false)
			default:
				start(text)
			}
			a.cur.anchor = e.Time
		}
		switch {
		case e.Type == "deploy.synced":
			text := "synced"
			if len(e.Revisions) > 0 {
				text += " to " + clean(e.Revisions[0], 12)
			}
			if a.openAt(e.Time) {
				a.cur.add(e.Time, text, false)
			} else {
				start(text)
			}
			a.cur.anchor = e.Time
			for _, r := range e.Revisions {
				a.cur.link("revision", r, false)
			}
			for _, r := range newRevisions(e) {
				rolled = append(rolled, synced{a.cur, r})
			}
		case e.Type == "deploy.flapping":
			trouble(fmt.Sprintf("flapping: %d transitions", e.Flaps))
			a.flapAt = e.Time
		case e.Status == "Healthy":
			if a.openAt(e.Time) {
				a.cur.add(e.Time, "health: "+e.Previous+" -> Healthy", false)
				a.cur.closed = e.Time
			}
			a.unhealthy, a.flapAt = false, time.Time{}
		case e.Status == "Degraded" || e.Status == "Missing":
			trouble("health: " + e.Previous + " -> " + e.Status)
			a.unhealthy, a.flapAt = true, time.Time{}
		case e.Status == "Unknown": // ArgoCD lost sight of it: what we knew stands
			if a.openAt(e.Time) {
				a.cur.add(e.Time, "health: "+e.Previous+" -> Unknown", false)
			}
		default: // Progressing, Suspended: the need ends here, and the window starts from here
			if a.openAt(e.Time) {
				a.cur.add(e.Time, "health: "+e.Previous+" -> "+e.Status, false)
				if a.needs(e.Time) {
					a.cur.anchor = e.Time
				}
			}
			a.unhealthy, a.flapAt = false, time.Time{}
		}
	}
	// A PR whose merge commit a rollout synced is linked both ways. Both ends are system facts, so neither is quoted.
	for _, r := range rolled {
		for _, p := range pr.byMerge[r.rev] {
			p.link("deploy", r.t.ID, false)
			r.t.link("pull_request", p.ID, false)
		}
	}
	lp.claims(pr)
	res := make([]Thread, len(out))
	for i, t := range out {
		finishCards(t, now)
		lp.finish(t, now)
		if t.app != nil {
			t.State, t.NeedsOwner = StateClosed, false
			if t.app.cur == t && t.app.openAt(now) {
				t.State, t.NeedsOwner = StateOpen, t.app.needs(now)
			}
		}
		res[i] = *t
	}
	slices.SortFunc(res, func(a, b Thread) int {
		return cmp.Or(b.LastActivityAt.Compare(a.LastActivityAt), cmp.Compare(b.ID, a.ID))
	})
	return res
}

// closeTime is when a deploy thread closed: its Healthy edge, else the end of its window.
func (t *Thread) closeTime() time.Time {
	if !t.closed.IsZero() {
		return t.closed
	}
	return t.app.anchorOf(t).Add(RolloutWindow)
}

// newRevisions are the revisions a sync brought: those not in kwatch's previous_revisions. An app that watches a
// repo's unchanged revision alongside others (a chart and its values repo) is not a rollout of that revision. A
// synced with no previous list counts only when it names a single revision.
func newRevisions(e Event) []string {
	if len(e.PrevRevisions) == 0 {
		if len(e.Revisions) == 1 {
			return e.Revisions
		}
		return nil
	}
	var out []string
	for _, r := range e.Revisions {
		if !slices.Contains(e.PrevRevisions, r) {
			out = append(out, r)
		}
	}
	return out
}

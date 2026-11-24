package feed

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
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
	Actor  string    `json:"actor"` // "system": the only speaker in this slice
	Text   string    `json:"text"`
	Quoted bool      `json:"quoted"`
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
	Entries        []Entry   `json:"entries"`
	Links          []Link    `json:"links"`

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

// openAt: a.cur is open at t.
func (a *appState) openAt(t time.Time) bool {
	c := a.cur
	return c != nil && c.closed.IsZero() && (a.needs(t) || t.Sub(c.anchor) <= RolloutWindow)
}

func threadID(kind, key string) string {
	h := sha256.Sum256([]byte(key))
	return kind + "-" + hex.EncodeToString(h[:8])
}

func (t *Thread) add(at time.Time, text string, quoted bool) {
	t.Entries = append(t.Entries, Entry{at, "system", text, quoted})
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
	var out []*Thread
	for _, e := range s {
		if k := (id{e.Source, e.ID}); seen[k] {
			continue
		} else {
			seen[k] = true
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
		default:
			if a.openAt(e.Time) {
				a.cur.add(e.Time, "health: "+e.Previous+" -> "+e.Status, false)
			}
			a.unhealthy, a.flapAt = false, time.Time{}
		}
	}
	res := make([]Thread, len(out))
	for i, t := range out {
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
	return t.anchor.Add(RolloutWindow)
}

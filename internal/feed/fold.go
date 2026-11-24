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

	// ReopenWithin: a deploy thread that closed healthy takes an unhealthy event back within this long; later it is a
	// new thread.
	ReopenWithin = 15 * time.Minute
)

// Entry is one line of a thread. Quoted is true for any text that came from an event's free text (alert
// annotations): the page shows it as a quote and an agent must not act on it.
type Entry struct {
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"` // "system": the only speaker in this slice
	Text   string    `json:"text"`
	Quoted bool      `json:"quoted"`
}

// Link is a reference out of a thread: an alert's generator URL, a deployed revision.
type Link struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
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

	bad bool // deploy: degraded, missing or flapping now
}

func threadID(kind, key string) string {
	h := sha256.Sum256([]byte(key))
	return kind + "-" + hex.EncodeToString(h[:8])
}

func (t *Thread) add(at time.Time, text string, quoted bool) {
	t.Entries = append(t.Entries, Entry{at, "system", text, quoted})
	t.LastActivityAt = at
}

func (t *Thread) link(kind, ref string) {
	l := Link{kind, ref}
	if ref != "" && !slices.Contains(t.Links, l) {
		t.Links = append(t.Links, l)
	}
}

func (t *Thread) set(state string) {
	t.State = state
	t.NeedsOwner = state == StateOpen && (t.Kind == KindAlert || t.bad)
}

// Fold builds the threads of evs, newest activity first. Order, duplicates and lateness of evs do not matter.
//
// Alerts: one thread per fingerprint and episode (the ce_id's episode). A firing opens it, a resolved closes it, a
// later firing of the same episode reopens it. An open alert needs the owner. A resolved with no firing before it
// (evicted, or never seen) is a closed thread of its own.
//
// Deploys: one thread per Application and rollout. A rollout starts at a deploy.synced (kwatch sends one only when a
// new revision was deployed) and ends at the next health Healthy; every synced, health and flapping event between
// belongs to it. An unhealthy or flapping event with no rollout open starts a thread of its own, and one within
// ReopenWithin of a close reopens that rollout. A Healthy with nothing open says nothing. An open rollout needs the
// owner while its health is Degraded or Missing, or it is flapping.
func Fold(evs []Event) []Thread {
	s := slices.Clone(evs)
	slices.SortStableFunc(s, func(a, b Event) int { return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.Seq, b.Seq)) })
	type id struct{ src, id string }
	seen := map[id]bool{}
	alerts := map[string]*Thread{}
	type app struct {
		cur      *Thread
		closedAt time.Time
	}
	apps := map[string]*app{}
	var out []*Thread
	for _, e := range s {
		if k := (id{e.Source, e.ID}); seen[k] {
			continue
		} else {
			seen[k] = true
		}
		switch e.Type {
		case "alert.firing", "alert.resolved":
			key := fmt.Sprintf("%s#%d", e.Key, e.Episode)
			t := alerts[key]
			if t == nil {
				t = &Thread{ID: threadID(KindAlert, key), Kind: KindAlert, OpenedAt: e.Time, Title: "Alert " + clean(e.Key, 8)}
				alerts[key], out = t, append(out, t)
			}
			if e.Name != "" {
				t.Title, t.TitleQuoted = "Alert: "+e.Name, true
			}
			t.link("generator", e.URL)
			if e.Type == "alert.resolved" {
				t.add(e.Time, "resolved", false)
				t.set(StateClosed)
				break
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
			t.set(StateOpen)
		default: // deploy.*
			a := apps[e.Key]
			if a == nil {
				a = &app{}
				apps[e.Key] = a
			}
			start := func(text string) {
				t := &Thread{ID: threadID(KindDeploy, e.Source+"|"+e.ID), Kind: KindDeploy, OpenedAt: e.Time, Title: "Deploy: " + e.Key}
				a.cur, out = t, append(out, t)
				t.add(e.Time, text, false)
			}
			// unhealthy: the open thread, else a reopened one, else a new one.
			unhealthy := func(text string) {
				switch {
				case a.cur != nil && a.cur.State == StateOpen:
					a.cur.add(e.Time, text, false)
				case a.cur != nil && e.Time.Sub(a.closedAt) <= ReopenWithin:
					a.cur.add(e.Time, "reopened: "+text, false)
				default:
					start(text)
				}
			}
			open := a.cur != nil && a.cur.State == StateOpen
			switch {
			case e.Type == "deploy.synced":
				text := "synced"
				if len(e.Revisions) > 0 {
					text += " to " + clean(e.Revisions[0], 12)
				}
				if open {
					a.cur.add(e.Time, text, false)
				} else {
					start(text)
				}
				for _, r := range e.Revisions {
					a.cur.link("revision", r)
				}
				a.cur.bad = false
			case e.Type == "deploy.flapping":
				unhealthy(fmt.Sprintf("flapping: %d transitions", e.Flaps))
				a.cur.bad = true
			case e.Status == "Healthy":
				if !open {
					continue
				}
				a.cur.add(e.Time, "health: "+e.Previous+" -> Healthy", false)
				a.cur.bad = false
				a.cur.set(StateClosed)
				a.closedAt = e.Time
				continue
			default:
				unhealthy("health: " + e.Previous + " -> " + e.Status)
				a.cur.bad = e.Status == "Degraded" || e.Status == "Missing"
			}
			a.cur.set(StateOpen)
		}
	}
	res := make([]Thread, len(out))
	for i, t := range out {
		res[i] = *t
	}
	slices.SortFunc(res, func(a, b Thread) int {
		return cmp.Or(b.LastActivityAt.Compare(a.LastActivityAt), cmp.Compare(b.ID, a.ID))
	})
	return res
}

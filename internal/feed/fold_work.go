package feed

import (
	"fmt"
	"strings"
	"time"
)

// Threads for PRs, agent sessions and cards. Fold calls apply for each event in time order, then finish once.

const (
	KindPR      = "pr"
	KindSession = "session"
	KindCard    = "card"
)

type prKey struct {
	repo string
	n    int
}

type headKey struct{ repo, sha string }

// prs holds the PR threads of one fold. Threads are one per repo and number.
type prs struct {
	threads map[prKey]*Thread
	byHead  map[headKey]*Thread // every head a PR had: a CI run names a commit, not a PR
	byMerge map[string][]*Thread
}

func newPRs() *prs {
	return &prs{threads: map[prKey]*Thread{}, byHead: map[headKey]*Thread{}, byMerge: map[string][]*Thread{}}
}

// quote is the entry for what a person wrote: "login: text", both from outside.
func quote(t *Thread, e Event, text string) {
	switch {
	case e.Login != "" && text != "":
		t.add(e.Time, e.Login+": "+text, true)
	case e.Login != "" || text != "":
		t.add(e.Time, e.Login+text, true)
	}
}

// apply folds one forge event. State: a thread opens at its first event of any kind (an opened, or the first sight of
// a PR whose opened is older than the feed's memory), closes at closed and opens again at reopened; an entry after the
// close (a late comment, a CI result) is added and changes nothing else. An opened that comes after a closed (the
// relist stamps a fact at its own time) does not reopen. needs_owner is never set: no forge event says a PR waits
// for the owner.
func (p *prs) apply(e Event, out *[]*Thread) {
	if e.Run != 0 {
		t := p.byHead[headKey{e.Repo, e.Head}]
		if t == nil {
			return
		}
		t.add(e.Time, fmt.Sprintf("CI run %d: %s", e.Run, strings.Replace(e.Status, "recover", "recovered", 1)), false)
		t.link("run", e.URL, true)
		return
	}
	k := prKey{e.Repo, e.Num}
	t := p.threads[k]
	if t == nil {
		t = &Thread{ID: threadID(KindPR, fmt.Sprintf("%s#%d", e.Repo, e.Num)), Kind: KindPR, State: StateOpen, OpenedAt: e.Time,
			Title: fmt.Sprintf("PR %s#%d", e.Repo, e.Num)}
		p.threads[k] = t
		*out = append(*out, t)
	}
	if e.Title != "" {
		t.Title, t.TitleQuoted = fmt.Sprintf("PR %s#%d: %s", e.Repo, e.Num, e.Title), true
	}
	if e.Head != "" {
		p.byHead[headKey{e.Repo, e.Head}] = t
	}
	switch e.Type {
	case "forge.pull_request.opened":
		t.add(e.Time, "opened", false)
		quote(t, e, "")
	case "forge.pull_request.closed":
		text := "closed"
		if e.Merged {
			text = "merged"
			if e.Merge != "" {
				p.byMerge[e.Merge] = append(p.byMerge[e.Merge], t)
			}
		}
		t.add(e.Time, text, false)
		t.State = StateClosed
		quote(t, e, "")
	case "forge.pull_request.reopened":
		t.add(e.Time, "reopened", false)
		t.State = StateOpen
		quote(t, e, "")
	case "forge.pull_request.synchronized":
		text := "pushed"
		if e.Head != "" {
			text += " " + e.Head[:min(len(e.Head), 12)]
		}
		t.add(e.Time, text, false)
		quote(t, e, "")
	case "forge.pull_request_review.approved":
		t.add(e.Time, "review: approved", false)
		quote(t, e, e.Text)
	case "forge.pull_request_review.rejected":
		t.add(e.Time, "review: changes requested", false)
		quote(t, e, e.Text)
	case "forge.pull_request_review.comment":
		t.add(e.Time, "review: comment", false)
		quote(t, e, e.Text)
	case "forge.issue_comment.created":
		t.add(e.Time, "comment", false)
		quote(t, e, e.Text)
	}
}

// card is one decision as its events left it.
type card struct {
	open, done bool // done: a closing state came, so a late opened cannot reopen it
	expires    time.Time
}

// needs: the card waits for the owner at now.
func (c *card) needs(now time.Time) bool {
	return c.open && (c.expires.IsZero() || now.Before(c.expires))
}

// fleet holds the session and card threads of one fold.
type fleet struct {
	known    map[string]bool // session ids with a session event anywhere in the fold
	sessions map[string]*Thread
	cards    map[string]*card
	owned    map[string]*Thread // decision id -> its own thread, a card whose session is unknown
	title    map[string]string  // session id -> the last title shown
}

func newFleet(evs []Event) *fleet {
	f := &fleet{known: map[string]bool{}, sessions: map[string]*Thread{}, cards: map[string]*card{}, owned: map[string]*Thread{}, title: map[string]string{}}
	for _, e := range evs {
		if strings.HasPrefix(e.Type, "fleet.session.") {
			f.known[e.Key] = true
		}
	}
	return f
}

func (f *fleet) session(e Event, out *[]*Thread) *Thread {
	t := f.sessions[e.Key]
	if t == nil {
		t = &Thread{ID: threadID(KindSession, e.Key), Kind: KindSession, State: StateOpen, OpenedAt: e.Time, Title: "Session " + string([]rune(e.Key)[:min(8, len([]rune(e.Key)))])}
		f.sessions[e.Key] = t
		*out = append(*out, t)
	}
	if e.Title != "" {
		t.Title, t.TitleQuoted = "Session: "+e.Title, true
		if f.title[e.Key] != e.Title {
			f.title[e.Key] = e.Title
			t.add(e.Time, e.Title, true)
		}
	}
	return t
}

// applySession folds one session event. A thread opens at its first event (opened, or a turn of a session that was
// open before core published events), closes at ended and stays closed: a turn after ended is an entry only.
func (f *fleet) applySession(e Event, out *[]*Thread) {
	t := f.session(e, out)
	switch e.Type {
	case "fleet.session.opened":
		text := "opened"
		if e.Kind != "" {
			text += " (" + e.Kind + ")"
		}
		t.add(e.Time, text, false)
		if w := strings.TrimSpace(e.Definition + " " + e.Repo); w != "" {
			t.add(e.Time, w, true)
		}
	case "fleet.session.turn_ended":
		text := "turn ended"
		if e.IsError {
			text = "turn failed"
		}
		if e.Ms > 0 {
			text += fmt.Sprintf(" (%s)", (time.Duration(e.Ms) * time.Millisecond).Round(100*time.Millisecond))
		}
		t.add(e.Time, text, false)
	case "fleet.session.ended":
		t.add(e.Time, "ended", false)
		t.State = StateClosed
	}
}

// applyCard folds one card event into its session's thread, or into a thread of its own when no session event names
// that session. The events carry no ask text, so every entry is the system's; the subject is the agent's choice and
// shows quoted. A card is open from its opened until any other state; the closing event may be missing (a deleted
// session), so finish also closes an open card at its expires_at.
func (f *fleet) applyCard(e Event, out *[]*Thread) {
	t := f.sessions[e.Session]
	if t == nil && f.known[e.Session] { // its first session event comes later in time
		t = f.session(Event{Key: e.Session, Time: e.Time}, out)
	}
	if t == nil {
		t = f.owned[e.Key]
		if t == nil {
			t = &Thread{ID: threadID(KindCard, e.Key), Kind: KindCard, State: StateOpen, OpenedAt: e.Time, Title: "Card"}
			f.owned[e.Key] = t
			*out = append(*out, t)
		}
		if e.Title != "" {
			t.Title, t.TitleQuoted = "Card: "+e.Title, true
		}
	} else if e.Title != "" && e.Status == "opened" {
		t.add(e.Time, "card on "+e.Title, true)
	}
	c := f.cards[e.Key]
	if c == nil {
		c = &card{}
		f.cards[e.Key] = c
		t.cards = append(t.cards, c)
	}
	if !e.Expires.IsZero() {
		c.expires = e.Expires
	}
	text := "card " + e.Status
	switch e.Status {
	case "opened":
		c.open = !c.done
	default:
		c.open, c.done = false, true
	}
	if e.Status == "answered" && e.Option != "" {
		text += " (option " + e.Option + ")"
	}
	t.add(e.Time, text, false)
}

// finish sets what depends on the clock: a thread with cards needs the owner while one of them is open and not
// expired; a card's own thread is open exactly then.
func finishCards(t *Thread, now time.Time) {
	for _, c := range t.cards {
		if c.needs(now) {
			t.NeedsOwner = true
		}
	}
	if t.Kind == KindCard {
		t.State = StateClosed
		if t.NeedsOwner {
			t.State = StateOpen
		}
	}
}

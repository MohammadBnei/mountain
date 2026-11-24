package feed

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// Posts: a session's own posts, one event each (type dev.bnei.laptop.post.created, subject laptop/session/<session>).
// Nothing is read out of a post's text: what it is about, whether it needs the owner and what it corrects are fields.
//
// The event's fields:
//
//	session        a lowercase UUID
//	session_label  the session's name for itself (optional)
//	repo, branch   where the session works (optional)
//	post_id        a UUID
//	text           the post, as Markdown
//	about          a list of {repo "<owner>/<name>", number}
//	needs_you      absent, or a list of strings; an empty list clears an earlier one; null is not a value
//	corrects       a UUID: the post_id of an earlier post of the same session
//
// The source is LaptopSource, and the time of a post is the event's time.
const (
	laptopType   = "laptop.post.created"
	LaptopSource = "ingest/laptop"
	KindLaptop   = "laptop"

	// LaptopClose: a laptop thread counts as closed this long after its last post (no event ends a session).
	LaptopClose = 24 * time.Hour
	// NeedsLife: a needs_you list stands this long after its post, unless a newer post replaces or clears it.
	NeedsLife = 7 * 24 * time.Hour

	maxPostRunes  = 2000 // a post is kept to this many runes
	maxAbout      = 10   // PRs per post
	maxPRClaims   = 50   // claims per PR thread, the oldest dropped
	claimRunes    = 280
	maxNeeds      = 20  // items in a needs_you list
	needRunes     = 200 // each
	listRunes     = 400 // an entry's text in the list answer (the thread has it whole)
	listNeeds     = 3   // items of a needs_list in the list answer, and
	listNeedRunes = 120 // the characters of each
)

// Claim is a PR a post is about: Repo is "<owner>/<repo>" as a forge subject names it.
type Claim struct {
	Repo string
	N    int
}

// uuid is a lowercase UUID, the form session, post_id and corrects have.
func uuid(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		switch c := s[i]; {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c < '0' || c > '9') && (c < 'a' || c > 'f'):
			return false
		}
	}
	return true
}

func parseLaptop(e Event, r Raw) (Event, bool) {
	var d struct {
		Session      string `json:"session"`
		SessionLabel string `json:"session_label"`
		Repo         string `json:"repo"`
		Branch       string `json:"branch"`
		PostID       string `json:"post_id"`
		Text         string `json:"text"`
		About        []struct {
			Repo   string `json:"repo"`
			Number int    `json:"number"`
		} `json:"about"`
		NeedsYou json.RawMessage `json:"needs_you"`
		Corrects string          `json:"corrects"`
	}
	if r.Source != LaptopSource || json.Unmarshal(r.Data, &d) != nil || !uuid(d.Session) || !uuid(d.PostID) ||
		(d.Corrects != "" && !uuid(d.Corrects)) {
		return e, false
	}
	e.Key = line(d.Session, 128)
	e.Repo, e.Branch, e.Title = line(d.Repo, 100), line(d.Branch, 100), line(d.SessionLabel, 120)
	e.PostID, e.Corrects = line(d.PostID, 128), line(d.Corrects, 128)
	e.Text = clean(d.Text, maxPostRunes)
	for _, a := range d.About[:min(len(d.About), 4*maxAbout)] {
		repo := a.Repo
		if o, n, ok := strings.Cut(repo, "/"); ok && name(o) && name(n) && a.Number > 0 && len(e.About) < maxAbout {
			if c := (Claim{repo, a.Number}); !slices.Contains(e.About, c) {
				e.About = append(e.About, c)
			}
		}
	}
	if len(d.NeedsYou) > 0 {
		var items []json.RawMessage
		if json.Unmarshal(d.NeedsYou, &items) != nil || items == nil { // null and non-lists are no value
			return e, false
		}
		e.NeedsSet = true
		for _, raw := range items[:min(len(items), 4*maxNeeds)] {
			var item string
			if json.Unmarshal(raw, &item) == nil {
				if item = line(item, needRunes); item != "" && len(e.NeedsList) < maxNeeds {
					e.NeedsList = append(e.NeedsList, item)
				}
			}
		}
	}
	return e, e.Text != ""
}

// post is a laptop thread's latest needs_you (by seq): the entry that carries it and its list, none when it clears.
type post struct {
	at   time.Time
	seq  int64
	idx  int
	list []string
}

type pending struct {
	e Event
	t *Thread
}

type laptops struct {
	threads map[string]*Thread
	pend    []pending
}

func newLaptops() *laptops { return &laptops{threads: map[string]*Thread{}} }

// apply folds one post: an entry of its own, quoted, in the session's thread. The thread's state is the clock's (finish).
func (l *laptops) apply(e Event, out *[]*Thread) {
	t := l.threads[e.Key]
	if t == nil {
		t = &Thread{ID: threadID(KindLaptop, e.Key), Kind: KindLaptop, State: StateOpen, OpenedAt: e.Time, TitleQuoted: true}
		l.threads[e.Key] = t
		*out = append(*out, t)
	}
	switch {
	case e.Title != "":
		t.Title = "Laptop: " + e.Title
	case e.Repo != "" && e.Branch != "":
		t.Title = "Laptop: " + e.Repo + "@" + e.Branch
	case e.Repo != "" || e.Branch != "":
		t.Title = "Laptop: " + e.Repo + e.Branch
	default:
		t.Title = "Session " + string([]rune(e.Key)[:8])
	}
	t.Entries = append(t.Entries, Entry{Time: e.Time, Actor: "session", Text: e.Text, Quoted: true, Markdown: true, PostID: e.PostID, Corrects: e.Corrects, seq: e.Seq})
	t.LastActivityAt = e.Time
	if e.NeedsSet && (t.post == nil || e.Seq > t.post.seq) { // the latest by seq, whatever order the times put them in
		t.post = &post{e.Time, e.Seq, len(t.Entries) - 1, e.NeedsList}
	}
	if len(e.About) > 0 {
		l.pend = append(l.pend, pending{e, t})
	}
}

// finish sets what the clock decides. The thread closes 24 h after its last post (the timeline's end), but a
// standing needs_you keeps needs_owner for NeedsLife after its post whatever the thread's state.
func (l *laptops) finish(t *Thread, now time.Time) {
	if t.Kind != KindLaptop {
		return
	}
	t.State = StateClosed
	if now.Sub(t.LastActivityAt) < LaptopClose {
		t.State = StateOpen
	}
	// A correction names an earlier post of this session (by seq, so no loop is possible); any other is a plain post.
	at := make(map[string]int, len(t.Entries))
	for i, e := range t.Entries {
		if _, dup := at[e.PostID]; !dup && e.PostID != "" {
			at[e.PostID] = i
		}
	}
	for i := range t.Entries {
		e := &t.Entries[i]
		if e.Corrects == "" {
			continue
		}
		if j, ok := at[e.Corrects]; ok && t.Entries[j].seq < e.seq {
			e.Correction = true
			t.Entries[j].CorrectedBy = e.PostID
		} else {
			e.Corrects = ""
		}
	}
	if p := t.post; p != nil && len(p.list) > 0 && now.Sub(p.at) < NeedsLife {
		t.NeedsOwner, t.NeedsBy = true, "session"
		t.Entries[p.idx].AsksOwner, t.Entries[p.idx].NeedsList = true, p.list
	}
}

// claims shows each post in the PR threads it names, as a claim entry of its first 280 characters with a link to the
// post's thread. A claim changes nothing else of the PR thread (not its state, title, last activity or needs), and a
// PR the feed has no thread for gets none. Each PR thread keeps its newest maxPRClaims claims: posts are taken newest
// first and a full thread takes no more, so what is built stays small however many posts there are.
func (l *laptops) claims(pr *prs) {
	kept := map[*Thread][]Entry{}
	for i := len(l.pend) - 1; i >= 0; i-- {
		p := l.pend[i]
		cut := ""
		for _, c := range p.e.About {
			t := pr.threads[prKey{c.Repo, c.N}]
			if t == nil || len(kept[t]) >= maxPRClaims {
				continue
			}
			if cut == "" {
				text := []rune(p.e.Text)
				cut = string(text[:min(len(text), claimRunes)])
				if len(text) > claimRunes {
					cut += "…"
				}
			}
			kept[t] = append(kept[t], Entry{Time: p.e.Time, Actor: "session", Text: cut, Quoted: true, Claim: true, Thread: p.t.ID, PostID: p.e.PostID, seq: p.e.Seq})
		}
	}
	for t, cl := range kept {
		t.Entries = append(t.Entries, cl...)
		slices.SortStableFunc(t.Entries, func(a, b Entry) int {
			return cmp.Or(a.Time.Compare(b.Time), cmp.Compare(a.seq, b.seq))
		})
	}
}

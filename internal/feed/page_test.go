package feed

import (
	"strings"
	"testing"
	"time"
)

// TestNoLoginEntry: a login is the by of the entry it belongs to, never an entry or part of a text.
func TestNoLoginEntry(t *testing.T) {
	th := byKind(fold(t, 10, prLife(t)...), KindPR)[0]
	by := 0
	for _, e := range th.Entries {
		if e.By != "" {
			by++
			if e.Text == e.By || strings.HasPrefix(e.Text, e.By+":") {
				t.Errorf("%+v", e)
			}
		}
	}
	if by == 0 {
		t.Errorf("no entry says who: %v", texts(th))
	}
}

// TestPreview: the list says the newest event's system text, else its first quoted text.
func TestPreview(t *testing.T) {
	list := func(rs ...Raw) Entry {
		t.Helper()
		f := New(100, 1<<20)
		for _, r := range rs {
			f.Ingest(r)
		}
		f.now = func() time.Time { return at(10) }
		p, _, _ := f.List(1, "")
		if len(p) != 1 || len(p[0].Entries) != 1 {
			t.Fatalf("%+v", p)
		}
		return p[0].Entries[0]
	}
	if e := list(prLife(t)...); e.Quoted || e.Text != "merged" || e.By == "" {
		t.Errorf("a merged PR: %+v", e)
	}
	if e := list(sess(1, "opened", 0, `,"title":"a"`)); e.Quoted || e.Text != "opened (repo)" {
		t.Errorf("a session with its quoted repo after it: %+v", e)
	}
	q := Thread{Entries: []Entry{{Time: at(0), Text: "old"}, {Time: at(1), Text: "first", Quoted: true}, {Time: at(1), Text: "second", Quoted: true}}}
	if p := preview(q.Entries); len(p) != 1 || p[0].Text != "first" {
		t.Errorf("quotes only: %+v", p)
	}
	if p := preview(nil); len(p) != 0 {
		t.Errorf("no entries: %+v", p)
	}
}

// TestLinkTitles: a link to a thread the feed holds carries its title and whether it is quoted; an outside ref that
// looks like an id does not.
func TestLinkTitles(t *testing.T) {
	f1 := Event{Seq: 1, Source: "am", ID: "1", Type: "alert.firing", Key: "fp", StartsAt: "a", Name: "Disk", Time: at(0)}
	f2 := Event{Seq: 2, Source: "am", ID: "2", Type: "alert.firing", Key: "fp", StartsAt: "b", Name: "Disk", Time: at(1)}
	old := Fold([]Event{f1}, at(2))[0]
	f2.URL = old.ID
	th := Fold([]Event{f1, f2}, at(2))[0]
	if len(th.Links) != 2 || th.Links[0] != (Link{Kind: "previous", Ref: old.ID, Title: "Alert: Disk", TitleQuoted: true}) || th.Links[1].Title != "" {
		t.Errorf("%+v", th.Links)
	}
}

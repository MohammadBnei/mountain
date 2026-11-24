package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/MohammadBnei/mountain/internal/feed"
)

// TestFeedList: the filters, and a filtered cursor: paging gives every matching thread once, in List's order.
func TestFeedList(t *testing.T) {
	a, admin := feedOn(t)
	all, _, err := a.Feed.List(1000, "")
	if err != nil || len(all) < 40 {
		t.Fatalf("%d threads, %v", len(all), err)
	}
	for _, c := range []struct{ kind, needs string }{{"", ""}, {"", "1"}, {"", "0"}, {"deploy", ""}, {"deploy", "0"}, {"card", "1"}, {"nope", ""}} {
		var want, got []string
		for _, th := range all {
			if (c.kind == "" || th.Kind == c.kind) && (c.needs == "" || th.NeedsOwner == (c.needs == "1")) {
				want = append(want, th.ID)
			}
		}
		for cursor, pages := "", 0; ; pages++ {
			page, next, err := feedList(t.Context(), a.Feed, 3, cursor, c.kind, c.needs)
			if err != nil || pages > 100 {
				t.Fatalf("%+v: page %d: %v", c, pages, err)
			}
			for _, th := range page {
				got = append(got, th.ID)
			}
			if cursor = next; cursor == "" {
				break
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%+v: %v, want %v", c, got, want)
		}
	}
	if _, _, err := feedList(t.Context(), a.Feed, 3, "x", "pr", ""); err == nil {
		t.Error("a bad cursor is accepted")
	}
	if r := a.serveReq("GET", "/api/feed?needs=x", []*http.Cookie{admin}); r.StatusCode != http.StatusBadRequest {
		t.Errorf("needs=x: %d", r.StatusCode)
	}
}

// TestFeedAge: both reads say how fresh the feed is; a feed with no event says null.
func TestFeedAge(t *testing.T) {
	a, admin := feedOn(t)
	for _, path := range []string{"/api/feed", "/api/feed/" + func() string { p, _, _ := a.Feed.List(1, ""); return p[0].ID }()} {
		var out struct {
			ID  string
			Age feedAge
		}
		r := a.serveReq("GET", path, []*http.Cookie{admin})
		if err := json.NewDecoder(r.Body).Decode(&out); err != nil || out.Age.LastEventAge == nil || *out.Age.LastEventAge > 60 || out.Age.NewestAt == nil {
			t.Errorf("%s: %+v %v", path, out, err)
		}
	}
	if g := ageOf(t.Context(), feed.New(1, 1)); g.LastEventAge != nil || g.NewestAt != nil {
		t.Errorf("empty: %+v", g)
	}
}

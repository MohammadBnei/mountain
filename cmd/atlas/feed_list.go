package main

// What /api/feed takes and says besides the threads (feed.go): ?kind= and ?needs=1|0, and age, on both its answers.

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/MohammadBnei/mountain/internal/feed"
)

// feedList is Feed.List narrowed to a kind ("" = all) and to the threads that need the owner ("1"), do not ("0"), or
// both (""). The cursor is List's, so a page may follow another with the same filter.
//
// ponytail: one pass over every thread per filtered read (the feed caps what it holds); an index if that shows.
func feedList(ctx context.Context, f *feed.Feed, limit int, cursor, kind, needs string) ([]feed.Thread, string, error) {
	if kind == "" && needs == "" {
		return f.ListCtx(ctx, limit, cursor)
	}
	all, _, err := f.ListCtx(ctx, math.MaxInt, cursor)
	if err != nil {
		return nil, "", err
	}
	var out []feed.Thread
	for _, t := range all {
		if (kind != "" && t.Kind != kind) || (needs != "" && t.NeedsOwner != (needs == "1")) {
			continue
		}
		if len(out) == limit {
			last := out[limit-1]
			return out, fmt.Sprintf("%d.%s", last.LastActivityAt.UnixNano(), last.ID), nil
		}
		out = append(out, t)
	}
	return out, "", nil
}

// feedAge is how fresh the feed is: the seconds since it last stored an event (the metric
// mountain_feed_last_event_age_seconds, null while it has none) and its newest thread's activity. The first says the
// bridge still delivers; after a restart the replay stores old events now, so the second says how old they are.
type feedAge struct {
	LastEventAge *float64   `json:"last_event_age"`
	NewestAt     *time.Time `json:"newest_activity_at"`
}

func ageOf(ctx context.Context, f *feed.Feed) (a feedAge) {
	if last := f.Stats().Last; !last.IsZero() {
		s := time.Since(last).Seconds()
		a.LastEventAge = &s
	}
	if top, _, err := f.ListCtx(ctx, 1, ""); err == nil && len(top) == 1 {
		a.NewestAt = &top[0].LastActivityAt
	}
	return a
}

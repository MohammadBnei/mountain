package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/MohammadBnei/mountain/internal/feed"
)

// loadFeedFixture turns the feed on over a file of events instead of the bridge (--dev): [{type, source, id, subject,
// ago_min, data}], each dated ago_min minutes before now.
func (a *App) loadFeedFixture(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var evs []struct {
		Type, Source, ID, Subject string
		AgoMin                    float64 `json:"ago_min"`
		Data                      json.RawMessage
	}
	if err := json.Unmarshal(b, &evs); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	a.Feed = feed.New(feedMaxEvents, feedMaxBytes)
	now := time.Now()
	for i, e := range evs {
		if !a.Feed.Ingest(feed.Raw{Seq: int64(i + 1), Type: e.Type, Source: e.Source, ID: e.ID, Subject: e.Subject,
			Time: now.Add(-time.Duration(e.AgoMin * float64(time.Minute))), Data: e.Data}) {
			return fmt.Errorf("%s: event %d (%s %s) is not one the feed stores", path, i, e.Type, e.ID)
		}
	}
	return nil
}

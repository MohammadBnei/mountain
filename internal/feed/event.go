// Package feed turns bridge events into threads for the owner's feed: alert episodes and ArgoCD rollouts.
//
// Parse and Fold are pure: Fold sorts by occurred time (ties on seq), drops a repeated (source, id), and builds every
// thread from nothing, so the order events arrive in, a duplicate or a late one cannot change the answer. Feed
// (store.go) holds a bounded set of parsed events and folds them when asked.
package feed

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

// Raw is a bridge event as the transport hands it over.
type Raw struct {
	Seq                       int64
	Source, ID, Type, Subject string
	Time                      time.Time
	Data                      []byte
}

// Event is what the feed keeps of a Raw: only the fields a thread uses, each cut to a fixed length, so the memory of
// one event is bounded whatever the payload was. Name, Summary, Description and URL come from an alert's free text.
type Event struct {
	Seq                             int64
	Source, ID, Type                string // Type without "dev.bnei.": alert.firing, alert.resolved, deploy.synced, deploy.health, deploy.flapping
	Time                            time.Time
	Key                             string // alert fingerprint, or ArgoCD Application
	Episode                         int
	Name, Summary, Description, URL string
	Status                          string   // health or sync status (flapping: the current health)
	Previous                        string   // the status before
	Revisions                       []string // deploy.synced
	Flaps                           int      // deploy.flapping: transitions in the window
}

// size is an event's memory, roughly: what the byte cap counts.
func (e Event) size() int {
	n := 96 + len(e.Source) + len(e.ID) + len(e.Type) + len(e.Key) + len(e.Name) + len(e.Summary) + len(e.Description) + len(e.URL) + len(e.Status) + len(e.Previous)
	for _, r := range e.Revisions {
		n += 16 + len(r)
	}
	return n
}

var healths = map[string]bool{"Healthy": true, "Progressing": true, "Degraded": true, "Suspended": true, "Missing": true, "Unknown": true}

// Parse reads r; false for any event the feed has no thread for or cannot key.
func Parse(r Raw) (Event, bool) {
	e := Event{Seq: r.Seq, Source: r.Source, ID: r.ID, Type: strings.TrimPrefix(r.Type, "dev.bnei."), Time: r.Time}
	if r.ID == "" || r.Time.IsZero() {
		return e, false
	}
	switch e.Type {
	case "alert.firing", "alert.resolved":
		var d struct {
			Fingerprint, StartsAt string
			Labels, Annotations   map[string]string
			GeneratorURL          string `json:"generatorURL"`
		}
		if json.Unmarshal(r.Data, &d) != nil {
			return e, false
		}
		e.Key = d.Fingerprint
		if e.Key == "" {
			e.Key = strings.TrimPrefix(r.Subject, "alert/")
		}
		if e.Key == "" || len(e.Key) > 128 || strings.ContainsAny(e.Key, "#\x00") {
			return e, false
		}
		e.Episode = episode(r.ID, e.Key, strings.TrimPrefix(e.Type, "alert."), d.StartsAt)
		e.Name, e.Summary, e.Description = clean(d.Labels["alertname"], 80), clean(d.Annotations["summary"], 400), clean(d.Annotations["description"], 400)
		if u := d.GeneratorURL; len(u) <= 512 && (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")) {
			e.URL = u
		}
	case "deploy.synced", "deploy.health", "deploy.flapping":
		var d struct {
			Refs struct {
				App       string
				Revisions []string
			}
			Health, Sync, Previous, Current string
			Transitions                     int
		}
		if json.Unmarshal(r.Data, &d) != nil {
			return e, false
		}
		e.Key = d.Refs.App
		if e.Key == "" {
			e.Key = strings.TrimPrefix(r.Subject, "argocd/")
		}
		if e.Key = clean(e.Key, 128); e.Key == "" {
			return e, false
		}
		switch e.Type {
		case "deploy.health":
			e.Status, e.Previous = status(d.Health), status(d.Previous)
		case "deploy.flapping":
			e.Status, e.Flaps = status(d.Current), d.Transitions
		default:
			for _, rev := range d.Refs.Revisions[:min(len(d.Refs.Revisions), 4)] {
				e.Revisions = append(e.Revisions, clean(rev, 64))
			}
		}
	default:
		return e, false
	}
	return e, true
}

// episode is the firing episode of an alert's ce_id, "<fp>:<status>:<startsAt>[:<episode>]" (bridge contract): an id
// stored before episodes has no suffix and counts as 0. startsAt holds colons, so it is cut off by value.
func episode(id, fp, status, startsAt string) int {
	rest, ok := strings.CutPrefix(id, fp+":"+status+":"+startsAt)
	if !ok || rest == "" || rest[0] != ':' || len(rest) > 9 {
		return 0
	}
	n := 0
	for _, c := range rest[1:] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func status(s string) string {
	if healths[s] || s == "OutOfSync" || s == "Synced" {
		return s
	}
	return "Unknown"
}

// clean drops control characters, trims and cuts s to n runes.
func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, s)
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n]) + "…"
	}
	return strings.TrimSpace(s)
}

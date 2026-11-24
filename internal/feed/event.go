// Package feed turns bridge events into threads for the owner's feed: alert episodes and ArgoCD rollouts.
//
// Parse and Fold are pure: Fold sorts by occurred time (ties on seq), drops a repeated (source, id), and builds every
// thread from nothing, so the order events arrive in, a duplicate or a late one cannot change the answer. Feed
// (store.go) holds a bounded set of parsed events and folds them when asked.
package feed

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unsafe"
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
	StartsAt                        string // alert: the notification startsAt; with Source, Key and Episode the alert identity (InsertAlert)
	Episode                         int
	Name, Summary, Description, URL string
	Status                          string   // health or sync status (flapping: the current health)
	Previous                        string   // the status before
	Revisions                       []string // deploy.synced
	Flaps                           int      // deploy.flapping: transitions in the window

	// forge.* (Key is unused): the PR a delivery belongs to, from its subject "forge/<owner>/<repo>/pr/<n>".
	Repo        string // forge: owner/repo; fleet.session: the session's repo
	Num         int
	Title       string // forge: the PR title; fleet.session: the agent-written title; fleet.card: the card's subject
	Login       string // forge: who acted; fleet.card: the writer definition
	Text        string // forge: a comment or review body
	Head, Merge string // forge: the PR's head sha; its merge commit sha (merged only). Hex only.
	Merged      bool
	Run         int    // forge action_run: the run's number in its repo; Status is success, failure or recover
	Session     string // fleet.card: the session that holds the card
	Kind        string // fleet.session: repo, definition or character
	Definition  string
	Ms          int64     // fleet.session.turn_ended
	IsError     bool      // fleet.session.turn_ended
	Expires     time.Time // fleet.card: expires_at, zero if unset
	Option      string    // fleet.card.answered: "1".."3"
}

// size is an event's memory, roughly: what the byte cap counts.
func (e Event) size() int {
	n := int(unsafe.Sizeof(e)) + cap(e.Revisions)*int(unsafe.Sizeof("")) + seenCost + len(e.Source) + len(e.ID)
	for _, s := range []string{e.Type, e.Key, e.StartsAt, e.Name, e.Summary, e.Description, e.URL, e.Status, e.Previous,
		e.Repo, e.Title, e.Login, e.Text, e.Head, e.Merge, e.Session, e.Kind, e.Definition, e.Option} {
		n += len(s)
	}
	for _, r := range e.Revisions {
		n += len(r)
	}
	return n
}

// seenCost is what an event's entry in Feed.seen holds besides its two strings, which the event shares: the key's two
// headers and the map's own overhead, rounded up.
const seenCost = 96

// forgeTypes are the Forgejo deliveries a PR thread reads; the rest of the forge.* types (labels, issues) are not the
// feed's.
var forgeTypes = map[string]bool{
	"forge.pull_request.opened": true, "forge.pull_request.closed": true, "forge.pull_request.reopened": true, "forge.pull_request.synchronized": true,
	"forge.pull_request_review.approved": true, "forge.pull_request_review.rejected": true, "forge.pull_request_review.comment": true,
	"forge.issue_comment.created": true, "forge.action_run.success": true, "forge.action_run.failure": true, "forge.action_run.recover": true,
}

var healths = map[string]bool{"Healthy": true, "Progressing": true, "Degraded": true, "Suspended": true, "Missing": true, "Unknown": true}

// Parse reads r; false for any event the feed has no thread for or cannot key.
func Parse(r Raw) (Event, bool) {
	e := Event{Seq: r.Seq, Source: r.Source, ID: r.ID, Type: strings.TrimPrefix(r.Type, "dev.bnei."), Time: r.Time}
	if r.ID == "" || r.Time.IsZero() {
		return e, false
	}
	switch {
	case forgeTypes[e.Type]:
		return parseForge(e, r)
	case e.Type == "fleet.session.opened" || e.Type == "fleet.session.turn_ended" || e.Type == "fleet.session.ended":
		return parseSession(e, r)
	case strings.HasPrefix(e.Type, "fleet.card."):
		return parseCard(e, r)
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
		e.StartsAt = clean(d.StartsAt, 64)
		e.Episode = episode(r.ID, e.Key, strings.TrimPrefix(e.Type, "alert."), d.StartsAt)
		e.Name, e.Summary, e.Description = clean(d.Labels["alertname"], 80), clean(d.Annotations["summary"], 400), clean(d.Annotations["description"], 400)
		if safeURL(d.GeneratorURL) {
			e.URL = d.GeneratorURL
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

// hexSHA is s when it is 7 to 64 lowercase hex digits, else "": a sha is the only part of a forge payload shown as
// system text.
func hexSHA(s string) string {
	if len(s) < 7 || len(s) > 64 || strings.Trim(s, "0123456789abcdef") != "" {
		return ""
	}
	return s
}

// name is s when it is a plain owner, repo or login: no space, control character, "#" or "|".
func name(s string) bool {
	return s != "" && len(s) <= 100 && strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r) || r == '#' || r == '|' || r == '/'
	}) < 0
}

// parseForge reads a Forgejo delivery as the bridge stores it: Data is the webhook body, or the relist loop's body
// built from the API objects, which has the same fields. The subject names the PR ("forge/<owner>/<repo>/pr/<n>") or a
// run ("forge/<owner>/<repo>/run/<n>"); a comment on an issue is not the feed's.
func parseForge(e Event, r Raw) (Event, bool) {
	var d struct {
		Sender      struct{ Login string }
		PullRequest *struct {
			Title          string
			Merged         bool
			MergeCommitSha string `json:"merge_commit_sha"`
			Head           struct{ Sha string }
			User           struct{ Login string }
		} `json:"pull_request"`
		Comment *struct {
			Body string
			User struct{ Login string }
		}
		Review *struct{ Content string }
		Run    *struct {
			Index     int    `json:"index_in_repo"`
			CommitSha string `json:"commit_sha"`
			HTMLURL   string `json:"html_url"`
		}
	}
	if json.Unmarshal(r.Data, &d) != nil {
		return e, false
	}
	p := strings.Split(r.Subject, "/")
	if len(p) != 5 || p[0] != "forge" || !name(p[1]) || !name(p[2]) {
		return e, false
	}
	n, err := strconv.Atoi(p[4])
	if err != nil || n < 1 {
		return e, false
	}
	e.Repo, e.Num = p[1]+"/"+p[2], n
	if strings.HasPrefix(e.Type, "forge.action_run.") {
		if p[3] != "run" || d.Run == nil {
			return e, false
		}
		e.Status, e.Run, e.Head = strings.TrimPrefix(e.Type, "forge.action_run."), n, hexSHA(d.Run.CommitSha)
		if safeURL(d.Run.HTMLURL) {
			e.URL = d.Run.HTMLURL
		}
		return e, e.Head != "" && d.Run.Index == n
	}
	if p[3] != "pr" {
		return e, false
	}
	login := d.Sender.Login
	if d.PullRequest != nil {
		pr := d.PullRequest
		e.Title, e.Head, e.Merged = line(pr.Title, 200), hexSHA(pr.Head.Sha), pr.Merged
		if pr.Merged {
			e.Merge = hexSHA(pr.MergeCommitSha)
		}
		if e.Type == "forge.pull_request.opened" {
			login = pr.User.Login
		}
	}
	switch {
	case d.Comment != nil && e.Type == "forge.issue_comment.created":
		login, e.Text = d.Comment.User.Login, clean(d.Comment.Body, 400)
	case d.Review != nil:
		e.Text = clean(d.Review.Content, 400)
	}
	if name(login) {
		e.Login = line(login, 64)
	}
	return e, true
}

// sessionKinds are the kinds core publishes.
var sessionKinds = map[string]bool{"repo": true, "definition": true, "character": true}

// parseSession reads fleet.session.*: ids, labels and a title the agent wrote (clamped again here).
func parseSession(e Event, r Raw) (Event, bool) {
	var d struct {
		Session    string `json:"session_id"`
		Kind       string
		Repo       string
		Definition string
		Title      string
		DurationMs int64 `json:"duration_ms"`
		IsError    bool  `json:"is_error"`
	}
	if json.Unmarshal(r.Data, &d) != nil || d.Session == "" || len(d.Session) > 128 || strings.ContainsAny(d.Session, "#|\x00") {
		return e, false
	}
	e.Key = clean(d.Session, 128)
	if sessionKinds[d.Kind] {
		e.Kind = d.Kind
	}
	e.Repo, e.Definition, e.Title = line(d.Repo, 128), line(d.Definition, 64), line(d.Title, 120)
	e.Ms, e.IsError = d.DurationMs, d.IsError
	return e, true
}

// cardStates are the states core publishes, as the event type's last word.
var cardStates = map[string]bool{"opened": true, "answered": true, "expired": true, "stale": true, "withdrawn": true}

// parseCard reads fleet.card.*: ids, states and times; the card's ask text is never sent.
func parseCard(e Event, r Raw) (Event, bool) {
	var d struct {
		DecisionID string `json:"decision_id"`
		SessionID  string `json:"session_id"`
		Subject    string
		Writer     string    `json:"writer_definition"`
		OptionID   string    `json:"option_id"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	if json.Unmarshal(r.Data, &d) != nil || d.DecisionID == "" || len(d.DecisionID) > 128 || strings.ContainsAny(d.DecisionID, "#|\x00") || !cardStates[strings.TrimPrefix(e.Type, "fleet.card.")] {
		return e, false
	}
	e.Key, e.Status = clean(d.DecisionID, 128), strings.TrimPrefix(e.Type, "fleet.card.")
	e.Session, e.Title, e.Login, e.Expires = line(d.SessionID, 128), line(d.Subject, 160), line(d.Writer, 64), d.ExpiresAt
	if len(d.OptionID) == 1 && d.OptionID[0] >= '1' && d.OptionID[0] <= '3' {
		e.Option = d.OptionID
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

// line is clean on one line: runs of white space (newlines too) become one space.
func line(s string, n int) string { return clean(strings.Join(strings.Fields(s), " "), n) }

// clean drops control and format characters (bidi overrides too), trims and cuts s to n runes.
func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n') || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, s)
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n]) + "…"
	}
	return strings.TrimSpace(s)
}

// maxURL is the longest URL a thread keeps.
const maxURL = 512

// safeURL is true for an absolute https URL with a host, no user info, no space or control character, at most maxURL
// bytes: the only URLs a page may be handed from an event's text.
func safeURL(s string) bool {
	if s == "" || len(s) > maxURL || strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

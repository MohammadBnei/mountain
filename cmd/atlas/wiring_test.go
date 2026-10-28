package main

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/mountain/realms"
	"github.com/coder/websocket"
)

// failingProm answers every PromQL query with an error; noAlerts answers Alertmanager with nothing.
type failingProm struct{}

func (failingProm) Query(context.Context, string) ([]sample, error) { return nil, errors.New("down") }

type noAlerts struct{}

func (noAlerts) Alerts(context.Context) ([]amAlert, error) { return nil, nil }

// waitScrape polls the exposition until every want is in it, or the deadline: the writers run on goroutines.
func waitScrape(t *testing.T, scrape func() string, want ...string) string {
	t.Helper()
	var body string
	var missing []string
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(25 * time.Millisecond) {
		body, missing = scrape(), nil
		for _, w := range want {
			if !strings.Contains(body, w) {
				missing = append(missing, w)
			}
		}
		if len(missing) == 0 {
			return body
		}
	}
	t.Fatalf("not in the exposition: %q\n%s", missing, body)
	return ""
}

// TestMetricsWritersReach: every writer holds the App's Metrics, so each series a real path writes is in the
// exposition. A nil *Metrics records nothing by design, so a writer left without it fails silently (no series, and
// MountainSourceStale or the token rule can never fire on it): here the five wirings are driven through their real
// paths, over the App newApp builds with a live Metrics: the socket hub (/ws), the live polls (prom failing,
// alerts answering), the graph store (a projection set), and the repo poll against a GitHub that sends the token's
// expiry header. serve's own wiring is TestServeMetrics.
func TestMetricsWritersReach(t *testing.T) {
	m, err := newMetrics()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gh := newFakeGitHub(t)
	next := gh.Config.Handler
	gh.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("github-authentication-token-expiration", time.Now().Add(10*24*time.Hour).UTC().Format("2006-01-02 15:04:05 MST"))
		next.ServeHTTP(w, r)
	})
	o := Opts{Dev: true, Every: time.Hour, GHURL: gh.URL, GHToken: "tok", Cache: t.TempDir()}
	a := newApp(o, rl, www, &gate{devOpen: true}, openGraphs(ctx, "", m), m)

	if err := a.Graph.loadFixture("../../www/dev/fixtures/graph.json"); err != nil {
		t.Fatal(err)
	}
	a.Live.start(ctx, failingProm{}, noAlerts{}, func() *GraphIndex { return a.Graph.index() })
	go a.project(ctx)

	srv := httptest.NewServer(routes(a, www))
	defer srv.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	conn.SetReadLimit(-1)
	readT(t, conn)

	waitScrape(t, func() string { _, _, b := scrape(t, m); return b },
		`mountain_source_last_success{source="db"}`,         // openGraphs
		`mountain_source_last_success{source="prom"}`,       // Live.start's seed
		`mountain_source_last_success{source="alerts"}`,     // Live.start, a poll that answered
		`mountain_source_last_success{source="github"}`,     // the repo poll's client
		`mountain_source_last_success{source="projection"}`, // the App's own
		"mountain_github_token_days_left ",                  // the repo poll's client, from the header
		"mountain_query_failed_total{query=",                // the live poll
		"mountain_projection_seq ",                          // Graphs.set
		`mountain_drift{kind="mirror"}`,                     // Graphs.set
		"mountain_ws_connections 1\n")                       // the hub
}

// TestServeMetrics: serve's own wiring of the Metrics (the Graphs it opens, the App it builds) reaches the scrape on
// the metrics address: the fixtures keep the polls off, so what is in the exposition is the db seed, the graph's
// two series and the socket count (0, then 1 with a socket open on the public address).
func TestServeMetrics(t *testing.T) {
	free := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = l.Close() }()
		return l.Addr().String()
	}
	pub, mx := free(), free()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, Config{Opts: Opts{Listen: pub, Metrics: mx, Dev: true, Cache: t.TempDir(), Every: time.Hour,
			GraphFixture: "../../www/dev/fixtures/graph.json", LiveFixture: "../../www/dev/fixtures/live.json"}})
	}()
	scrapeAddr := func() string {
		resp, err := http.Get("http://" + mx + "/metrics")
		if err != nil {
			return ""
		}
		defer func() { _ = resp.Body.Close() }()
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				return b.String()
			}
		}
	}
	waitScrape(t, scrapeAddr, `mountain_source_last_success{source="db"}`, "mountain_projection_seq ", `mountain_drift{kind="mirror"}`, "mountain_ws_connections 0\n")
	var conn *websocket.Conn // the App's Metrics: the hub counts a socket the public address accepts
	for end := time.Now().Add(10 * time.Second); conn == nil; time.Sleep(25 * time.Millisecond) {
		var err error
		if conn, _, err = websocket.Dial(ctx, "ws://"+pub+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}}); err != nil && time.Now().After(end) {
			t.Fatal(err)
		}
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	waitScrape(t, scrapeAddr, "mountain_ws_connections 1\n")
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after its context ended")
	}
}

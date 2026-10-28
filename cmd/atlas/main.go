// Command atlas serves mountain.bnei.dev (plan/10-understanding.md, R10 A2): the system graph projected from the
// repos, a live overlay from Prometheus and Alertmanager, and the page that draws both. Platform admins only.
//
//	atlas [--listen :8080] [--metrics :9090] [--prom-url URL] [--am-url URL] [--github-url URL]
//	      [--repos dir] [--every 2m] [--dev] [--graph-fixture file] [--live-fixture file]
//	atlas project --repos <dir> [--dsn …] [--drop-repos]   (store one projection from local checkouts)
//
// Env: DATABASE_URL (Postgres; empty = memory-only: the latest projection only, no history),
// MOUNTAIN_GITHUB_TOKEN (read-only token for the repo poll; empty = no poll, --repos or the stored projection only),
// ATLAS_CACHE (where the repo poll unpacks the trees it reads, emptied at start; default /var/cache/atlas),
// MOUNTAIN_OIDC_ISSUER, MOUNTAIN_OIDC_CLIENT_ID, MOUNTAIN_OIDC_CLIENT_SECRET and MOUNTAIN_SESSION_KEY (sign-in; any
// unset or a key under 32 bytes = every route but /healthz refuses a person, except under --dev on a loopback address),
// MOUNTAIN_MACHINE_CERT, MOUNTAIN_MACHINE_ISSUER, MOUNTAIN_MACHINE_CLIENT_ID and MOUNTAIN_MACHINE_SUB (the game's
// machine principal on the town's two reads, machine.go; all or none: unset = off), and
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT (the OTLP/HTTP traces URL in full, its /v1/traces path included; unset = no
// tracer and no span, a malformed one is a refusal to start). All of it is read once, in config.go; the libraries'
// own reads (pgx's PG* defaults, the proxy variables, SSL_CERT_*) are theirs, and so are OpenTelemetry's: the metrics
// SDK always reads OTEL_RESOURCE_ATTRIBUTES, OTEL_SERVICE_NAME, OTEL_METRICS_EXEMPLAR_FILTER and OTEL_GO_X_* (a
// malformed one is a JSON INFO line on stdout, from OTel's global error handler going through the log package that
// slog has taken over; some bad values are ignored with no line at all; the exposition is unchanged either way), and
// with tracing on the trace exporter reads its headers, timeout, compression and certificates, OTEL_BSP_* and the span
// limits (mountain/pkg/telemetry).
//
// Logs: JSON lines (mountain/pkg/telemetry.NewLogger), each site with its level: on stdout for the service, on stderr
// for `atlas project`, whose stdout is its result line as before. serve's own flag errors and usage stay text on
// stderr; every other refusal to start is a JSON ERROR line, then exit 1. The libraries' own reads, as above, plus
// grpc's GRPC_GO_* (linked through mountain/pkg, read at init, unused).
//
// Read side only: atlas never writes a repo or a live store (CQRS, plan/10).
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/pkg/telemetry"
	"github.com/MohammadBnei/mountain/realms"
)

// Opts are the command-line flags plus the env the App reads (config.go).
type Opts struct {
	Listen, Metrics, PromURL, AMURL, GHURL, Repos, GraphFixture, LiveFixture string
	Every                                                                    time.Duration
	Dev                                                                      bool
	GHToken                                                                  string
	Cache                                                                    string // ATLAS_CACHE
}

func main() {
	slog.SetDefault(telemetry.NewLogger(logWriter(os.Args))) // plan/15 §7: JSON; the log package comes through it
	if len(os.Args) > 1 && os.Args[1] == "project" {
		dir, dsn, drop, err := projectFromProcess()
		if err == nil {
			err = projectCmd(dir, dsn, drop)
		}
		if err != nil {
			fatal(err)
		}
		return
	}
	c, err := serveFromProcess()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := serve(c); err != nil {
		fatal(err)
	}
}

// App is the running service: the graph it serves, the live overlay, and the clients that watch both.
type App struct {
	Opts    Opts
	Realms  *realms.Realms
	Hub     *Hub
	Graph   *Graphs
	Live    *Live
	Files   *repoSet           // the checkouts the last round read: the reader's files (read.go)
	Build   string             // the served www's build id (/v/<build>/); hello.build
	Met     *Metrics           // the series on :9090 (nil = not recorded: the tests')
	Tracing *telemetry.Tracing // a server span per route; nil or empty = none (plan/15 §7)

	gate      *gate
	unread    map[string]string // each source's last read failure (the projector goroutine's, project.go)
	reads     *readCache        // rendered documents (read.go)
	readSlots chan struct{}     // at most two renders at once (read.go)
}

// newApp builds what the App owns, for serve and the tests alike; the caller starts the projector and the polls.
func newApp(o Opts, rl *realms.Realms, www fs.FS, g *gate, gr *Graphs, met *Metrics) *App {
	a := &App{Opts: o, Realms: rl, Build: buildID(www), Graph: gr, Files: &repoSet{}, gate: g, unread: map[string]string{},
		reads: &readCache{m: map[string][]byte{}}, readSlots: make(chan struct{}, 2), Met: met}
	a.Hub = NewHub(o.Dev, a.hello, met)
	a.Live = newLive(rl, a.Hub, met)
	a.Live.idx = func() *GraphIndex { return a.Graph.index() }
	return a
}

func serve(c Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, c)
}

// run is serve until ctx ends (a test ends it; serve's is SIGINT or SIGTERM).
func run(ctx context.Context, c Config) error {
	o := c.Opts
	rl, err := realms.Load()
	if err != nil {
		return err
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	g := &gate{auth: authFrom(c.OIDC, o.Dev)}
	var merr error
	g.machine, merr = machineFrom(c.Machine)
	if merr != nil {
		return merr
	}
	g.devOpen = o.Dev && g.auth == nil && loopback(o.Listen)
	if g.auth == nil && !g.devOpen {
		slog.Warn("atlas: sign-in is not configured: every route but /healthz refuses a person (the machine principal, if configured, still reads its two routes)")
	}

	pm, err := newMetrics()
	if err != nil {
		return err
	}
	defer func() { _ = pm.Close() }()
	tel, err := telemetry.New(tracesOrOff(c.Traces), "atlas")
	if err != nil {
		return err
	}
	a := newApp(o, rl, www, g, openGraphs(ctx, c.DSN, pm), pm)
	a.Tracing = tel
	if o.GraphFixture != "" {
		if err := a.Graph.loadFixture(o.GraphFixture); err != nil {
			return err
		}
		if o.Repos != "" { // dev: the reader reads these directories (checkouts or not); nothing is projected from them
			repos, err := devRepos(o.Repos)
			if err != nil {
				return err
			}
			a.Files.update(repos)
		}
	} else {
		go a.project(ctx)
	}
	if o.LiveFixture != "" {
		if err := a.Live.loadFixture(o.LiveFixture); err != nil {
			return err
		}
	} else {
		hc := &http.Client{Timeout: 10 * time.Second}
		prom := promHTTP{base: strings.TrimRight(o.PromURL, "/"), hc: hc}
		alerts := amHTTP{base: strings.TrimRight(o.AMURL, "/"), hc: hc}
		a.Live.start(ctx, prom, alerts, func() *GraphIndex { return a.Graph.index() })
	}

	pub := &http.Server{Addr: o.Listen, Handler: routes(a, www), ReadHeaderTimeout: 10 * time.Second, ErrorLog: serverLog()}
	mx := http.NewServeMux()
	mx.Handle("GET /metrics", pm.Handler())
	met := &http.Server{Addr: o.Metrics, Handler: mx, ReadHeaderTimeout: 10 * time.Second, ErrorLog: serverLog()}
	errc := make(chan error, 2)
	for _, s := range []*http.Server{pub, met} {
		go func() { errc <- s.ListenAndServe() }()
	}
	slog.Info(fmt.Sprintf("atlas: %s public, %s metrics, dev=%v open=%v", o.Listen, o.Metrics, o.Dev, g.devOpen))
	select {
	case err = <-errc:
	case <-ctx.Done():
	}
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = pub.Shutdown(sc)
	_ = met.Shutdown(sc)
	a.Graph.close()
	// a hijacked /ws is not waited for: a span still open then is dropped
	if e := tel.Shutdown(); e != nil {
		slog.Warn(fmt.Sprintf("atlas: traces: %v", e))
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// loopback: the address binds a loopback interface only (an empty host binds every interface).
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// route is one entry of the public mux. open routes answer anyone; every other route answers a platform admin only
// (plan/10 Security: deny by default). A test calls each route as each kind of caller.
type route struct {
	pattern  string
	open     bool
	untraced bool // no span: the kubelet's probes, every few seconds per pod, would be a stream of empty spans (owner, 2026-10-28)
	h        http.Handler
}

func routeTable(a *App, www fs.FS) []route {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	return []route{
		{"GET /healthz", true, true, ok},
		{"GET /auth/login", true, false, a.gate.onAuth((*Auth).login)},
		{"GET /auth/callback", true, false, a.gate.onAuth((*Auth).callback)},
		{"POST /auth/logout", true, false, a.gate.onAuth((*Auth).logout)},
		{"GET /auth/me", true, false, http.HandlerFunc(a.gate.me)},
		{"GET /ws", false, false, a.Hub},
		{"GET /api/graph", false, false, http.HandlerFunc(a.serveGraph)},
		{"GET /api/read", false, false, http.HandlerFunc(a.serveRead)},
		{"GET /", false, false, static(www, a.Opts.Dev)},
	}
}

func routes(a *App, www fs.FS) http.Handler {
	mux := http.NewServeMux()
	for _, r := range routeTable(a, www) {
		h := r.h
		if !r.open {
			h = a.gate.bearerFirst(r.pattern, h, a.gate.adminOnly(h)) // a bearer is judged alone: the machine principal (machine.go)
		}
		if !r.untraced {
			h = a.Tracing.Handler(r.pattern, h) // outside the gate, so a refusal has its span; without tracing, h itself
		}
		mux.Handle(r.pattern, h)
	}
	return mux
}

// adminOnly answers anyone but a signed-in platform admin with 403, or, for the page itself, a redirect to the
// sign-in (the forwardAuth gate has already signed them in to authentik, so it is one round trip, no form). The
// hash route survives the trip: the browser keeps a fragment across redirects that carry none. An admitted request
// carries its session's end in the context (sessionEnd): the socket's end; and, for a signed-in session, whether it
// is still live (sessionLive): a sign-out closes its sockets.
func (g *gate) adminOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if end, id, ok := g.admitted(r); ok {
			ctx := context.WithValue(r.Context(), sessionEndKey{}, end)
			if au := g.auth; id != "" && au != nil {
				ctx = context.WithValue(ctx, sessionLiveKey{}, func() bool { return !au.isRevoked(id) })
			}
			h.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Only an admin ever holds a session (a non-admin's callback ends on a refusal page, not here), so the
		// sign-in cannot loop; a town-era cookie, one without an id or a signed-out one is not a session and signs in
		// again.
		if r.Method == http.MethodGet && r.URL.Path == "/" && g.auth != nil {
			ret := "/"
			if q := "/?" + r.URL.RawQuery; r.URL.RawQuery != "" && returnRE.MatchString(q) {
				ret = q // the page's own query (?q=low) survives the sign-in
			}
			http.Redirect(w, r, "/auth/login?return="+url.QueryEscape(ret), http.StatusFound)
			return
		}
		http.Error(w, "platform admins only", http.StatusForbidden)
	})
}

var inlineScript = regexp.MustCompile(`(?s)<script(?: type="importmap")?>(.*?)</script>`)

// csp hashes every inline <script> of the page (the importmap), so a stale hash fails in the browser.
func csp(html []byte, dev bool) string {
	src := "'self'"
	for _, m := range inlineScript.FindAllSubmatch(html, -1) {
		h := sha256.Sum256(m[1])
		src += " 'sha256-" + base64.StdEncoding.EncodeToString(h[:]) + "'"
	}
	connect := "'self' wss://mountain.bnei.dev"
	if dev {
		connect += " ws://127.0.0.1:*"
	}
	return "default-src 'self'; script-src " + src + "; style-src 'self'; img-src 'self' data: blob:; connect-src " + connect +
		"; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
}

// buildID is a short hash over every embedded file: it changes on any deploy that changes www/.
func buildID(www fs.FS) string {
	h := sha256.New()
	_ = fs.WalkDir(www, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, err := fs.ReadFile(www, p); err == nil {
				_, _ = fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
				h.Write(b)
			}
		}
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// static serves www/. Every asset index.html references lives under /v/<build>/ (immutable: a new deploy is a new
// URL), so a returning browser never mixes module versions across a deploy; / itself is no-cache. /v/ is private:
// the site sits behind a sign-in, and a shared cache would hand an admin's copy to anyone holding the URL. Another
// build's /v/<id>/ is a no-store 404, which main.js answers with one reload onto the current index.
func static(www fs.FS, dev bool) http.Handler {
	files := http.FileServerFS(www)
	etags := map[string]string{}
	_ = fs.WalkDir(www, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, err := fs.ReadFile(www, p); err == nil {
				s := sha256.Sum256(b)
				etags["/"+p] = `"` + base64.RawURLEncoding.EncodeToString(s[:12]) + `"`
			}
		}
		return nil
	})
	prefix := "/v/" + buildID(www) + "/"
	// every `"./` in index.html is an asset URL (src, href, importmap); keep it that way.
	index, _ := fs.ReadFile(www, "index.html")
	index = bytes.ReplaceAll(index, []byte(`"./`), []byte(`"`+prefix))
	indexCSP := csp(index, dev)
	s := sha256.Sum256(index)
	indexTag := `"` + base64.RawURLEncoding.EncodeToString(s[:12]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		p, cache := r.URL.Path, "private, no-cache"
		if strings.HasPrefix(p, "/v/") {
			if !strings.HasPrefix(p, prefix) {
				h.Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			p, cache = "/"+strings.TrimPrefix(p, prefix), "private, max-age=31536000, immutable"
			r = r.Clone(r.Context())
			r.URL.Path, r.URL.RawPath = p, ""
		}
		if strings.HasPrefix(p, "/dev/") && !dev {
			http.NotFound(w, r)
			return
		}
		h.Set("Cache-Control", cache)
		if p == "/" {
			h.Set("ETag", indexTag)
			h.Set("Content-Security-Policy", indexCSP)
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		if strings.HasSuffix(p, "/") { // no directory listings: the page names every file it needs
			http.NotFound(w, r)
			return
		}
		key := p
		if e, ok := etags[key]; ok {
			h.Set("ETag", e)
		}
		if strings.HasSuffix(p, ".html") {
			if b, err := fs.ReadFile(www, strings.TrimPrefix(p, "/")); err == nil {
				h.Set("Content-Security-Policy", csp(b, dev))
			}
		}
		files.ServeHTTP(w, r)
	})
}

// devRepos is every directory under dir as a repo for the reader (--dev with a graph fixture): a git checkout at its
// HEAD, any other directory at a zero sha (the gate's fixture repos are plain directories).
func devRepos(dir string) (extract.Repos, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := extract.Repos{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(dir, e.Name())
		sha, branch, err := extract.Head(root)
		if err != nil {
			sha, branch = strings.Repeat("0", 40), ""
		}
		out[e.Name()] = extract.Repo{Dir: root, SHA: sha, Branch: branch}
	}
	return out, nil
}

// fatal logs err as an ERROR, with log.Fatal's words, and exits 1 as it did.
func fatal(err error) {
	slog.Error(err.Error())
	os.Exit(1)
}

// serverLog is net/http's own error lines (a handler's panic, an Accept error and its retries, a superfluous
// WriteHeader) as ERROR lines of the default logger: each is a bug or an outage, and Loki's detector counted them as
// errors when they were text, so the platform's log-error-rate-spike alert still sees them.
func serverLog() *log.Logger { return slog.NewLogLogger(slog.Default().Handler(), slog.LevelError) }

// logWriter is where atlas's JSON lines go: stdout for the service (what Alloy tails), stderr for `atlas project`,
// whose stdout is its result line.
func logWriter(args []string) io.Writer {
	if len(args) > 1 && args[1] == "project" {
		return os.Stderr
	}
	return os.Stdout
}

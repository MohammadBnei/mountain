// Command atlas serves mountain.bnei.dev (plan/10-understanding.md, R10 A2): the system graph projected from the
// repos, a live overlay from Prometheus and Alertmanager, and the page that draws both. Platform admins only.
//
//	atlas [--listen :8080] [--metrics :9090] [--prom-url URL] [--am-url URL] [--github-url URL]
//	      [--repos dir] [--every 2m] [--dev] [--graph-fixture file] [--live-fixture file]
//	atlas project --repos <dir> [--dsn …] [--drop-repos]   (store one projection from local checkouts)
//
// Env: DATABASE_URL (Postgres; empty = memory-only: the latest projection only, no history),
// MOUNTAIN_GITHUB_TOKEN (read-only token for the repo poll; empty = no poll, --repos or the stored projection only),
// MOUNTAIN_OIDC_ISSUER / _CLIENT_ID / _CLIENT_SECRET + MOUNTAIN_SESSION_KEY (sign-in; any unset = every route but
// /healthz refuses a person, except under --dev on a loopback address),
// MOUNTAIN_MACHINE_CERT / _ISSUER / _CLIENT_ID / _SUB (the game's machine principal on the town's two reads,
// machine.go; all or none: unset = off).
//
// Read side only: atlas never writes a repo or a live store (CQRS, plan/10).
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
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
	"github.com/MohammadBnei/mountain/realms"
)

// Opts are the command-line flags plus the env the service reads at start.
type Opts struct {
	Listen, Metrics, PromURL, AMURL, GHURL, Repos, GraphFixture, LiveFixture string
	Every                                                                    time.Duration
	Dev                                                                      bool
	GHToken                                                                  string
}

func parseServe(args []string) (Opts, error) {
	var o Opts
	f := flag.NewFlagSet("atlas", flag.ContinueOnError)
	f.StringVar(&o.Listen, "listen", ":8080", "public listen address")
	f.StringVar(&o.Metrics, "metrics", ":9090", "metrics listen address (never routed)")
	f.StringVar(&o.PromURL, "prom-url", "http://platform-prometheus-kube-p-prometheus.monitoring:9090", "Prometheus base URL")
	f.StringVar(&o.AMURL, "am-url", "http://platform-prometheus-kube-p-alertmanager.monitoring:9093", "Alertmanager base URL")
	f.StringVar(&o.GHURL, "github-url", "https://api.github.com", "GitHub API base URL")
	f.StringVar(&o.Repos, "repos", "", "read local checkouts from this directory instead of polling GitHub")
	f.DurationVar(&o.Every, "every", 2*time.Minute, "how often the tracked heads are checked")
	f.BoolVar(&o.Dev, "dev", false, "allow http://127.0.0.1:* origins and serve /dev/; on a loopback address with sign-in unset, admit everyone")
	f.StringVar(&o.GraphFixture, "graph-fixture", "", "dev: serve this graph (an /api/graph answer) instead of projecting")
	f.StringVar(&o.LiveFixture, "live-fixture", "", "dev: serve this live state instead of polling Prometheus and Alertmanager")
	err := f.Parse(args)
	o.GHToken = os.Getenv("MOUNTAIN_GITHUB_TOKEN") // env, not a flag: a flag value shows in ps and the pod spec
	if err == nil && (o.GraphFixture != "" || o.LiveFixture != "") && !o.Dev {
		err = errors.New("atlas: fixtures are --dev only")
	}
	return o, err
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if len(os.Args) > 1 && os.Args[1] == "project" {
		if err := projectCmd(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	o, err := parseServe(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := serve(o); err != nil {
		log.Fatal(err)
	}
}

// App is the running service: the graph it serves, the live overlay, and the clients that watch both.
type App struct {
	Opts   Opts
	Realms *realms.Realms
	Hub    *Hub
	Graph  *Graphs
	Live   *Live
	Files  *repoSet // the checkouts the last round read: the reader's files (read.go)
	Build  string   // the served www's build id (/v/<build>/); hello.build
}

var app *App // set by serve (and tests) before routes run

func serve(o Opts) error {
	rl, err := realms.Load()
	if err != nil {
		return err
	}
	www, _ := fs.Sub(mountain.WWW, "www")
	auth = authFromEnv(o.Dev)
	if machine, err = machineFromEnv(); err != nil {
		return err
	}
	devOpen = o.Dev && auth == nil && loopback(o.Listen)
	if auth == nil && !devOpen {
		log.Printf("atlas: sign-in is not configured: every route but /healthz refuses a person (the machine principal, if configured, still reads its two routes)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := &App{Opts: o, Realms: rl, Build: buildID(www), Files: &repoSet{}}
	a.Hub = NewHub(o.Dev, a.hello)
	a.Graph = openGraphs(ctx, os.Getenv("DATABASE_URL"))
	a.Live = newLive(rl, a.Hub)
	a.Live.idx = a.Graph.index
	app = a
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
		a.Live.start(ctx, o.PromURL, o.AMURL, func() *GraphIndex { return a.Graph.index() })
	}

	pub := &http.Server{Addr: o.Listen, Handler: routes(a, www), ReadHeaderTimeout: 10 * time.Second}
	mx := http.NewServeMux()
	mx.HandleFunc("GET /metrics", serveMetrics)
	met := &http.Server{Addr: o.Metrics, Handler: mx, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 2)
	for _, s := range []*http.Server{pub, met} {
		go func() { errc <- s.ListenAndServe() }()
	}
	log.Printf("atlas: %s public, %s metrics, dev=%v open=%v", o.Listen, o.Metrics, o.Dev, devOpen)
	select {
	case err = <-errc:
	case <-ctx.Done():
	}
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pub.Shutdown(sc)
	met.Shutdown(sc)
	a.Graph.close()
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
	pattern string
	open    bool
	h       http.Handler
}

func routeTable(a *App, www fs.FS) []route {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	return []route{
		{"GET /healthz", true, ok},
		{"GET /auth/login", true, onAuth((*Auth).login)},
		{"GET /auth/callback", true, onAuth((*Auth).callback)},
		{"POST /auth/logout", true, onAuth((*Auth).logout)},
		{"GET /auth/me", true, http.HandlerFunc(me)},
		{"GET /ws", false, a.Hub},
		{"GET /api/graph", false, http.HandlerFunc(serveGraph)},
		{"GET /api/read", false, http.HandlerFunc(serveRead)},
		{"GET /", false, static(www, a.Opts.Dev)},
	}
}

func routes(a *App, www fs.FS) http.Handler {
	mux := http.NewServeMux()
	for _, r := range routeTable(a, www) {
		h := r.h
		if !r.open {
			h = bearerFirst(r.pattern, h, adminOnly(h)) // a bearer is judged alone: the machine principal (machine.go)
		}
		mux.Handle(r.pattern, h)
	}
	return mux
}

// adminOnly answers anyone but a signed-in platform admin with 403, or, for the page itself, a redirect to the
// sign-in (the forwardAuth gate has already signed them in to authentik, so it is one round trip, no form). The
// hash route survives the trip: the browser keeps a fragment across redirects that carry none.
func adminOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAdmin(r) {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Only an admin ever holds a session (a non-admin's callback ends on a refusal page, not here), so the
		// sign-in cannot loop; a town-era cookie is not a session and signs in again.
		if r.Method == http.MethodGet && r.URL.Path == "/" && auth != nil {
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
	fs.WalkDir(www, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, err := fs.ReadFile(www, p); err == nil {
				fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
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
	fs.WalkDir(www, ".", func(p string, d fs.DirEntry, err error) error {
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

// Command relay serves mountain.bnei.dev: the embedded www/, /ws, and :9090 metrics (06-relay).
//
//	relay [--listen :8080] [--metrics :9090] [--prom-url URL] [--am-url URL] [--dev] [--source live|fixture:<file>] [--config file]
//	relay dump-hello [--day YYYY-MM-DD] [--events file.json] [--out www/dev/fixtures/hello.json|-]
//
// Env: MOUNTAIN_FP_SALT (alert fp salt), DATABASE_URL (Postgres; empty = memory-only, store.go),
// MOUNTAIN_GITHUB_TOKEN (fine-grained read-only PAT; empty = src_github off, src_github.go),
// MOUNTAIN_OIDC_ISSUER / _CLIENT_ID / _CLIENT_SECRET + MOUNTAIN_SESSION_KEY (detail mode; any unset = off, oidc.go),
// VISITORS_ENABLED (W4 kill switch; the mounted file /etc/mountain/flags/VISITORS_ENABLED wins, re-read live: validators.go).
// dump-hello restores from DATABASE_URL when it is set.
//
// main.go, hub.go, sources.go, metrics.go and config.yaml are ORCHESTRATOR-OWNED (W1a seam).
// Lanes add files and plug in through: Register (sources.go), FixtureHook (sources.go),
// Public (routes, from init()), clientIP / onClientMsg (hub.go), and the *App methods below.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/mountain/relay/town"
	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var configYAML []byte

// Config is relay/config.yaml.
type Config struct {
	Apps        map[string]AppCfg               `yaml:"apps"`
	Namespaces  map[string]string               `yaml:"namespaces"`
	Nodes       map[string]string               `yaml:"nodes"`
	AlertLayers []struct{ Match, Layer string } `yaml:"alertLayers"`
	Repos       map[string]string               `yaml:"repos"`
	Titles      []string                        `yaml:"titles"`

	titles []*regexp.Regexp // Titles, anchored
}

func loadConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	for _, t := range c.Titles {
		re, err := regexp.Compile("^(?:" + t + ")$")
		if err != nil {
			return nil, fmt.Errorf("config: title %q: %w", t, err)
		}
		c.titles = append(c.titles, re)
	}
	for k, app := range c.Namespaces {
		if _, ok := c.Apps[app]; app != "" && !ok {
			return nil, fmt.Errorf("config: namespace %q maps to %q, not on the allowlist", k, app)
		}
	}
	for name, a := range c.Apps {
		if a.Group != "platform" && a.Group != "user" && a.Group != "service" {
			return nil, fmt.Errorf("config: app %q: group %q is not platform|user|service", name, a.Group)
		}
		if a.Building != "" && !slices.Contains(town.Kinds, a.Building) {
			return nil, fmt.Errorf("config: app %q: building %q is not a town kind", name, a.Building)
		}
	}
	for repo, app := range c.Repos {
		if _, ok := c.Apps[app]; !ok {
			return nil, fmt.Errorf("config: repo %q maps to %q, not on the allowlist", repo, app)
		}
	}
	return &c, nil
}

// AppCfg is one allowlist entry. Group: platform | user | service (R7: platform services shown under a generic role
// key, never their product name). Building: the town building kind (town.Kinds); empty = picked by group.
type AppCfg struct{ Group, Display, Building string }

// Opts are the command-line flags, plus FPSalt from env MOUNTAIN_FP_SALT (Infisical mountain-8c-cz; never rotated, 02)
// and GHToken from env MOUNTAIN_GITHUB_TOKEN (same project).
type Opts struct {
	Listen, Metrics, PromURL, AMURL, GHURL, Source, Config string
	Dev                                                    bool
	FPSalt, GHToken                                        string
}

// devFPSalt is the fixture-mode salt: fixtures carry no real fingerprints, so a public constant is fine there.
const devFPSalt = "mountain-dev-salt"

func parseServe(args []string) (Opts, error) {
	var o Opts
	f := flag.NewFlagSet("relay", flag.ContinueOnError)
	f.StringVar(&o.Listen, "listen", ":8080", "public listen address")
	f.StringVar(&o.Metrics, "metrics", ":9090", "metrics listen address (never routed)")
	f.StringVar(&o.PromURL, "prom-url", "http://platform-prometheus-kube-p-prometheus.monitoring:9090", "Prometheus base URL")
	f.StringVar(&o.AMURL, "am-url", "http://platform-prometheus-kube-p-alertmanager.monitoring:9093", "Alertmanager base URL")
	f.StringVar(&o.GHURL, "github-url", "https://api.github.com", "GitHub API base URL")
	f.BoolVar(&o.Dev, "dev", false, "allow http://127.0.0.1:* origins, ws://127.0.0.1:* in CSP, serve /dev/")
	f.StringVar(&o.Source, "source", "live", "live | fixture:<file>")
	f.StringVar(&o.Config, "config", "", "config.yaml path (default: embedded)")
	err := f.Parse(args)
	// env, not a flag: a flag value shows in ps and the pod spec
	if o.FPSalt = os.Getenv("MOUNTAIN_FP_SALT"); o.FPSalt == "" && strings.HasPrefix(o.Source, "fixture:") {
		o.FPSalt = devFPSalt
	}
	o.GHToken = os.Getenv("MOUNTAIN_GITHUB_TOKEN")
	return o, err
}

// App is the relay's live state. Lanes read and mutate it under mu.
type App struct {
	Cfg  *Config
	Opts Opts
	Hub  *Hub

	mu      sync.Mutex
	Metrics Metrics
	State   State  // hello.state: Out.Fire / Out.Resolve only (persisted, store.go)
	Day     string // UTC day of the latest snapshot; advanced at rollover (store.go)

	MarkSubs map[string]string // W4: active mark id -> account (OIDC sub), never public

	since []json.RawMessage // hello.deltas: the world frames broadcast since the latest snapshot
	store *Store            // nil = memory-only; set before serving, never after
}

var app *App // set by serve before routes run

// Public is the :8080 mux. Lane files add routes from init() (e.g. /auth/*).
var Public = http.NewServeMux()

func newApp(cfg *Config, o Opts, day string) *App {
	a := &App{Cfg: cfg, Opts: o, State: newState(), Day: day, Metrics: emptyMetrics(), since: []json.RawMessage{}, MarkSubs: map[string]string{}}
	a.Hub = NewHub(o.Dev, a.Hello)
	a.Hub.tap = a.record
	return a
}

// Snapshot is hello.snapshot (www/dev/schema/hello.json): which day's snapshot the state and deltas start from,
// and the world's generation (store.go townEpoch).
type Snapshot struct {
	Day   string `json:"day"`
	Epoch int    `json:"epoch"`
}

// Hello returns the hello frame sent on connect: the latest snapshot + the world frames since it (02).
func (a *App) Hello() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, _ := json.Marshal(map[string]any{
		"t":        "hello",
		"snapshot": a.snap(a.Day),
		"deltas":   a.since,
		"state":    a.State,
		"metrics":  a.Metrics,
		"town":     town.Get(), // R7: static geometry, so the page renders exactly what plots.go validates
	})
	return b
}

// snap is hello.snapshot (and replay.snapshot).
func (a *App) snap(day string) Snapshot { return Snapshot{Day: day, Epoch: townEpoch} }

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if len(os.Args) > 1 && os.Args[1] == "dump-hello" {
		if err := dumpHello(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	o, err := parseServe(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if err := serve(o); err != nil {
		log.Fatal(err)
	}
}

func dumpHello(args []string) error {
	f := flag.NewFlagSet("dump-hello", flag.ContinueOnError)
	day := f.String("day", time.Now().UTC().Format(time.DateOnly), "snapshot day")
	out := f.String("out", "www/dev/fixtures/hello.json", "output file, - for stdout")
	events := f.String("events", "", "an {events: [{kind, ...fields}]} file played through the real sanitize first (fixtures/hello-events.json)")
	if err := f.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(configYAML)
	if err != nil {
		return err
	}
	a := newApp(cfg, Opts{}, *day)
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" { // a hello built from a restored snapshot (W2 pose gate)
		a.attach(context.Background(), dsn, true)
		defer a.store.Close(5 * time.Second)
	}
	if *events != "" {
		if err := playEvents(a, *events); err != nil {
			return err
		}
	}
	b := a.Hello()
	if *out == "-" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
}

// playEvents broadcasts each entry of file as a source would: {kind, ...fields} as an ev (Out.Ev), {t: "world",
// kind, ...} as a world frame (Out.World: plots, marks), sanitised, then folded into the state by the hub tap.
func playEvents(a *App, file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var f struct{ Events []map[string]any }
	if err := d.Decode(&f); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	out := &Out{app: a, name: "fixture"}
	for i, e := range f.Events {
		kind, _ := e["kind"].(string)
		t, _ := e["t"].(string)
		delete(e, "kind")
		delete(e, "t")
		fields := natural(e).(map[string]any)
		if t == "world" {
			if out.worldFrame(kind, fields) == nil {
				return fmt.Errorf("%s: frame %d (world %s) dropped by sanitize", file, i, kind)
			}
			out.World(kind, fields)
			continue
		}
		if out.Ev(kind, fields) == "" {
			return fmt.Errorf("%s: event %d (%s) dropped by sanitize", file, i, kind)
		}
	}
	return nil
}

// natural turns decoded json.Numbers into int (when integral) or float64, the types sources hand to sanitize.
func natural(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = natural(e)
		}
	case []any:
		for i, e := range x {
			x[i] = natural(e)
		}
	}
	return v
}

func serve(o Opts) error {
	b := configYAML
	if o.Config != "" {
		var err error
		if b, err = os.ReadFile(o.Config); err != nil {
			return err
		}
	}
	cfg, err := loadConfig(b)
	if err != nil {
		return err
	}
	if o.FPSalt == "" {
		// fail closed without a crashloop: App.FP returns "", so sanitize drops every alert (no unsalted fp leaves)
		log.Printf("relay: MOUNTAIN_FP_SALT unset: alerts are dropped until it is set")
	}
	app = newApp(cfg, o, utcDay(time.Now()))
	app.Metrics.Visitors = visitorsFlag()
	auth = authFromEnv(o.Dev)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.attach(ctx, os.Getenv("DATABASE_URL"), false) // before the sources: src_alerts seeds from the restored state
	go app.persist(ctx)
	go app.watchVisitors(ctx)
	if err := startSources(ctx, app); err != nil {
		return err
	}

	www, _ := fs.Sub(mountain.WWW, "www")
	Public.Handle("GET /ws", app.Hub)
	Public.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	Public.Handle("GET /", static(www, o.Dev))
	mx := http.NewServeMux()
	mx.HandleFunc("GET /metrics", serveMetrics)

	pub := &http.Server{Addr: o.Listen, Handler: Public, ReadHeaderTimeout: 10 * time.Second}
	met := &http.Server{Addr: o.Metrics, Handler: mx, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 2)
	for _, s := range []*http.Server{pub, met} {
		go func() { errc <- s.ListenAndServe() }()
	}
	log.Printf("relay: %s public, %s metrics, dev=%v source=%s", o.Listen, o.Metrics, o.Dev, o.Source)
	select {
	case err = <-errc:
	case <-ctx.Done():
	}
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pub.Shutdown(sc)
	met.Shutdown(sc)
	app.snapshot(time.Now()) // SIGTERM: the last world reaches the DB before the pod goes
	app.store.Close(5 * time.Second)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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

// static serves www/. Cloudflare overrides our Cache-Control (zone Browser Cache TTL, edge-cached .js),
// so headers alone cannot stop a returning browser mixing module versions across a deploy (live: old
// kit.js + new post.js => setComposed undefined). Instead every asset index.html references lives under
// /v/<build>/ (immutable: a new deploy is a new URL); / itself is no-cache and Cloudflare serves it DYNAMIC.
// Relative imports inside modules resolve under the same prefix. Unversioned paths still work (no-cache +
// content ETags) for pages loaded before the deploy; another build's /v/<id>/ is a no-store 404, which
// main.js answers with one reload onto the current index.
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
	// ponytail: every `"./` in index.html is an asset URL (src, href, importmap); keep it that way.
	index, _ := fs.ReadFile(www, "index.html")
	index = bytes.ReplaceAll(index, []byte(`"./`), []byte(`"`+prefix))
	indexCSP := csp(index, dev)
	s := sha256.Sum256(index)
	indexTag := `"` + base64.RawURLEncoding.EncodeToString(s[:12]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		p, cache := r.URL.Path, "no-cache"
		if strings.HasPrefix(p, "/v/") {
			if !strings.HasPrefix(p, prefix) {
				h.Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			p, cache = "/"+strings.TrimPrefix(p, prefix), "public, max-age=31536000, immutable"
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
		key := p
		if strings.HasSuffix(key, "/") {
			key += "index.html"
		}
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

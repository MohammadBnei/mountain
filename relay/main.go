// Command relay serves mountain.bnei.dev: the embedded www/, /ws, and :9090 metrics (06-relay).
//
//	relay [--listen :8080] [--metrics :9090] [--prom-url URL] [--am-url URL] [--dev] [--source live|fixture:<file>] [--seed N] [--config file]
//	relay dump-hello [--seed N] [--day YYYY-MM-DD] [--out www/dev/fixtures/hello.json|-]
//
// main.go, hub.go, sources.go, metrics.go and config.yaml are ORCHESTRATOR-OWNED (W1a seam).
// Lanes add files and plug in through: Register (sources.go), FixtureHook (sources.go),
// Public (routes, from init()), clientIP / onClientMsg (hub.go), and the *App methods below.
package main

import (
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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MohammadBnei/mountain"
	"github.com/MohammadBnei/mountain/relay/worldcore"
	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var configYAML []byte

// Config is relay/config.yaml.
type Config struct {
	Apps        map[string]struct{ Group, Display string } `yaml:"apps"`
	Namespaces  map[string]string                          `yaml:"namespaces"`
	Nodes       map[string]string                          `yaml:"nodes"`
	AlertLayers []struct{ Match, Layer string }            `yaml:"alertLayers"`
	Repos       map[string]string                          `yaml:"repos"`
	Titles      []string                                   `yaml:"titles"`
}

func loadConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	for k, app := range c.Namespaces {
		if _, ok := c.Apps[app]; app != "" && !ok {
			return nil, fmt.Errorf("config: namespace %q maps to %q, not on the allowlist", k, app)
		}
	}
	return &c, nil
}

// Opts are the command-line flags, plus FPSalt from env MOUNTAIN_FP_SALT (Infisical mountain-8c-cz; never rotated, 02).
type Opts struct {
	Listen, Metrics, PromURL, AMURL, Source, Config string
	Dev                                             bool
	Seed                                            uint
	FPSalt                                          string
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
	f.BoolVar(&o.Dev, "dev", false, "allow http://127.0.0.1:* origins, ws://127.0.0.1:* in CSP, serve /dev/")
	f.StringVar(&o.Source, "source", "live", "live | fixture:<file>")
	f.StringVar(&o.Config, "config", "", "config.yaml path (default: embedded)")
	f.UintVar(&o.Seed, "seed", 1, "terrain seed")
	err := f.Parse(args)
	// env, not a flag: a flag value shows in ps and the pod spec
	if o.FPSalt = os.Getenv("MOUNTAIN_FP_SALT"); o.FPSalt == "" && strings.HasPrefix(o.Source, "fixture:") {
		o.FPSalt = devFPSalt
	}
	return o, err
}

// App is the relay's live state. Lanes read and mutate it under mu.
type App struct {
	Cfg  *Config
	Opts Opts
	Hub  *Hub

	mu      sync.Mutex
	World   *worldcore.World
	Land    []int16 // h.land, HeightQ steps (lane A writes deposits here)
	Metrics Metrics
	State   State // hello.state: Out.Fire / Out.Resolve only (W2 persists it)
	Day     string

	base, bench string // hello's base64 h.base and bench: immutable, encoded once in newApp
}

var app *App // set by serve before routes run

// Public is the :8080 mux. Lane files add routes from init() (e.g. /api/topology).
var Public = http.NewServeMux()

func newApp(cfg *Config, o Opts, day string) *App {
	w := worldcore.Base(uint32(o.Seed))
	a := &App{Cfg: cfg, Opts: o, World: w, Land: make([]int16, len(w.Base)), State: State{Alerts: []Alert{}}, Day: day,
		Metrics: emptyMetrics(0.8 * w.HMax), base: worldcore.EncodeI16(w.Base), bench: base64.StdEncoding.EncodeToString(w.Bench)}
	a.Hub = NewHub(o.Dev, a.Hello)
	return a
}

// Snapshot is hello.snapshot (www/dev/schema/hello.json).
type Snapshot struct {
	Day   string           `json:"day"`
	Seed  uint32           `json:"seed"`
	Grid  int              `json:"grid"`
	Base  string           `json:"base"`
	Land  string           `json:"land"`
	Bench string           `json:"bench"`
	Caves []worldcore.Cave `json:"caves"`
}

// Hello returns the hello frame sent on connect.
func (a *App) Hello() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, _ := json.Marshal(map[string]any{
		"t": "hello",
		"snapshot": Snapshot{Day: a.Day, Seed: a.World.Seed, Grid: worldcore.Grid, Base: a.base,
			Land: worldcore.EncodeI16(a.Land), Bench: a.bench, Caves: a.World.Caves},
		"deltas":  []any{},
		"state":   a.State,
		"metrics": a.Metrics,
	})
	return b
}

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
	seed := f.Uint("seed", 1, "terrain seed")
	day := f.String("day", time.Now().UTC().Format(time.DateOnly), "snapshot day")
	out := f.String("out", "www/dev/fixtures/hello.json", "output file, - for stdout")
	if err := f.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(configYAML)
	if err != nil {
		return err
	}
	b := newApp(cfg, Opts{Seed: *seed}, *day).Hello()
	if *out == "-" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
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
	app = newApp(cfg, o, time.Now().UTC().Format(time.DateOnly))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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

func static(www fs.FS, dev bool) http.Handler {
	files := http.FileServerFS(www)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/dev/") && !dev {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if p == "/" || strings.HasSuffix(p, ".html") {
			name := strings.TrimPrefix(p, "/")
			if name == "" {
				name = "index.html"
			}
			if b, err := fs.ReadFile(www, name); err == nil {
				h.Set("Content-Security-Policy", csp(b, dev))
			}
		}
		files.ServeHTTP(w, r)
	})
}

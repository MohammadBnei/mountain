package main

import (
	"flag"
	"go/parser"
	"go/token"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// machineFromEnv is the machine principal as serve builds it from the process's environment: the loader, then
// machineFrom (machine_test.go sets the variables).
func machineFromEnv() (*Machine, error) {
	c, err := loadConfig(nil, os.Getenv)
	if err != nil {
		return nil, err
	}
	return machineFrom(c.Machine)
}

func envOf(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

// TestConfigDefaults: with no flags and an empty environment, serve's configuration is today's defaults; every flag's
// name, default and usage is pinned.
func TestConfigDefaults(t *testing.T) {
	c, err := loadConfig(nil, envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Opts: Opts{Listen: ":8080", Metrics: ":9090", PromURL: "http://platform-prometheus-kube-p-prometheus.monitoring:9090",
		AMURL: "http://platform-prometheus-kube-p-alertmanager.monitoring:9093", GHURL: "https://api.github.com", Every: 2 * time.Minute,
		Cache: "/var/cache/atlas"}}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("defaults:\n got %+v\nwant %+v", c, want)
	}
	var got []string
	serveFlags(&Opts{}).VisitAll(func(f *flag.Flag) { got = append(got, f.Name+"="+f.DefValue+": "+f.Usage) })
	if w := []string{
		"am-url=http://platform-prometheus-kube-p-alertmanager.monitoring:9093: Alertmanager base URL",
		"dev=false: allow http://127.0.0.1:* origins and serve /dev/; on a loopback address with sign-in unset, admit everyone",
		"every=2m0s: how often the tracked heads are checked",
		"github-url=https://api.github.com: GitHub API base URL",
		"graph-fixture=: dev: serve this graph (an /api/graph answer) instead of projecting",
		"listen=:8080: public listen address",
		"live-fixture=: dev: serve this live state instead of polling Prometheus and Alertmanager",
		"metrics=:9090: metrics listen address (never routed)",
		"prom-url=http://platform-prometheus-kube-p-prometheus.monitoring:9090: Prometheus base URL",
		"repos=: read local checkouts from this directory instead of polling GitHub",
	}; !reflect.DeepEqual(got, w) {
		t.Errorf("serve's flags:\n got %q\nwant %q", got, w)
	}
}

// TestConfigEnv: each variable reaches its field; sign-in is on only with all four set and a key of at least 32 bytes
// (bytes, not characters), off otherwise and silently; the machine principal's four pass through unchecked (serve
// checks them, after realms.Load); the fixtures stay --dev only.
func TestConfigEnv(t *testing.T) {
	key := strings.Repeat("k", 32)
	full := map[string]string{"MOUNTAIN_OIDC_ISSUER": "https://a/o", "MOUNTAIN_OIDC_CLIENT_ID": "m", "MOUNTAIN_OIDC_CLIENT_SECRET": "s", "MOUNTAIN_SESSION_KEY": key}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range full {
			m[a] = b
		}
		m[k] = v
		return m
	}
	for name, c := range map[string]struct {
		env map[string]string
		on  bool
	}{
		"all four":          {full, true},
		"a 31-byte key":     {with("MOUNTAIN_SESSION_KEY", key[1:]), false},
		"16 two-byte runes": {with("MOUNTAIN_SESSION_KEY", strings.Repeat("é", 16)), true},
		"15 two-byte runes": {with("MOUNTAIN_SESSION_KEY", strings.Repeat("é", 15)), false},
		"no issuer":         {with("MOUNTAIN_OIDC_ISSUER", ""), false},
		"no client id":      {with("MOUNTAIN_OIDC_CLIENT_ID", ""), false},
		"no secret":         {with("MOUNTAIN_OIDC_CLIENT_SECRET", ""), false},
		"no key":            {with("MOUNTAIN_SESSION_KEY", ""), false},
	} {
		cfg, err := loadConfig(nil, envOf(c.env))
		if err != nil || (cfg.OIDC != nil) != c.on {
			t.Errorf("%s: sign-in %v (%v), want %v", name, cfg.OIDC != nil, err, c.on)
		}
	}
	if cfg, _ := loadConfig(nil, envOf(full)); *cfg.OIDC != (oidcEnv{Issuer: "https://a/o", ClientID: "m", Secret: "s", Key: key}) {
		t.Errorf("sign-in: %+v", *cfg.OIDC)
	}
	cfg, err := loadConfig([]string{"--repos", "/r"}, envOf(map[string]string{"DATABASE_URL": "postgres://d", "MOUNTAIN_GITHUB_TOKEN": "t",
		"ATLAS_CACHE": "/c", "MOUNTAIN_MACHINE_SUB": "s"}))
	if err != nil || cfg.DSN != "postgres://d" || cfg.GHToken != "t" || cfg.Cache != "/c" || cfg.Repos != "/r" ||
		cfg.Machine != (machineEnv{Sub: "s"}) {
		t.Errorf("env: %+v %v", cfg, err)
	}
	for _, args := range [][]string{{"--graph-fixture", "g"}, {"--live-fixture", "l"}} {
		if _, err := loadConfig(args, envOf(nil)); err == nil || err.Error() != "atlas: fixtures are --dev only" {
			t.Errorf("%v without --dev: %v", args, err)
		}
		if _, err := loadConfig(append(args, "--dev"), envOf(nil)); err != nil {
			t.Errorf("%v --dev: %v", args, err)
		}
	}
	cfg, err = loadConfig([]string{"--nope"}, envOf(map[string]string{"MOUNTAIN_GITHUB_TOKEN": "t"}))
	if err == nil || cfg.GHToken != "t" {
		t.Errorf("a bad flag: %v, token %q", err, cfg.GHToken) // the env is read whatever the flags say, as before
	}
}

// undocumented returns the names not in text, each matched as a whole word (not inside a longer flag or variable).
func undocumented(text string, names []string) []string {
	var miss []string
	for _, n := range names {
		if !regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(n) + `($|[^A-Za-z0-9_-])`).MatchString(text) {
			miss = append(miss, n)
		}
	}
	return miss
}

// TestConfigDocumented: main.go's doc comment names every flag of serve in serve's usage lines, every flag of
// `atlas project` in its line, and every variable the loaders read.
func TestConfigDocumented(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	doc := f.Doc.Text()
	usage, project, _ := strings.Cut(doc[strings.Index(doc, "atlas ["):], "atlas project")
	project, _, _ = strings.Cut(project, "\n")
	var serve, proj, env []string
	serveFlags(&Opts{}).VisitAll(func(f *flag.Flag) { serve = append(serve, "--"+f.Name) })
	var d, s string
	var b bool
	projectFlags(&d, &s, &b).VisitAll(func(f *flag.Flag) { proj = append(proj, "--"+f.Name) })
	read := func(k string) string { env = append(env, k); return "" }
	loadConfig(nil, read)
	parseProject([]string{"--repos", "r"}, read, io.Discard)
	for what, c := range map[string]struct {
		text  string
		names []string
	}{"serve's usage": {usage, serve}, "project's usage": {project, proj}, "the doc comment": {doc, env}} {
		if len(c.names) == 0 {
			t.Errorf("%s: no names to check", what)
		}
		if m := undocumented(c.text, c.names); len(m) > 0 {
			t.Errorf("%s does not name %v", what, m)
		}
	}
	if m := undocumented("--dev-x, MOUNTAIN_SESSION_KEYS, --repos", []string{"--dev", "MOUNTAIN_SESSION_KEY", "--repos"}); !slices.Equal(m, []string{"--dev", "MOUNTAIN_SESSION_KEY"}) {
		t.Errorf("undocumented: %v", m) // the matcher's own control: a longer name is not the name
	}
}

package main

// Config (plan/15 §5): everything atlas reads at start, the flags and the environment, read here once. No other
// non-test file of the module reads the environment (TestEnvBan, at the module root, over its listed readers); the
// libraries' own reads (pgx's PG* defaults, net/http's proxy variables, crypto/x509's SSL_CERT_*) are theirs,
// unchanged. Every flag and variable is in main.go's doc comment (TestConfigDocumented).

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"
)

// Config is serve's: the flags plus the environment, and what only serve reads (the database, the two principals).
type Config struct {
	Opts
	DSN     string     // DATABASE_URL: Postgres; empty = memory-only
	OIDC    *oidcEnv   // sign-in; nil = off
	Machine machineEnv // the machine principal, as given: machineFrom checks it where serve always has
	Traces  string     // OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, as given: tracesEndpoint checks it in serve; "" = no tracing
}

// oidcEnv is sign-in's configuration: the client and the session cookie's HMAC key.
type oidcEnv struct {
	Issuer, ClientID, Secret, Key string
}

// machineEnv is the machine principal's configuration, unchecked: all four or none, and the PEM, are machineFrom's,
// so its refusal keeps its place among serve's errors (after realms.Load, through log.Fatal).
type machineEnv struct {
	Cert, Issuer, ClientID, Sub string
}

// loadConfig reads serve's flags from args and the environment through getenv. Only its flag errors (and the
// fixtures' --dev rule) refuse here; main prints them and exits 2.
func loadConfig(args []string, getenv func(string) string) (Config, error) {
	var c Config
	o := &c.Opts
	err := serveFlags(o).Parse(args)
	o.GHToken = getenv("MOUNTAIN_GITHUB_TOKEN") // env, not a flag: a flag value shows in ps and the pod spec
	if o.Cache = getenv("ATLAS_CACHE"); o.Cache == "" {
		o.Cache = "/var/cache/atlas"
	}
	c.DSN = getenv("DATABASE_URL")
	if iss, id, sec, key := getenv("MOUNTAIN_OIDC_ISSUER"), getenv("MOUNTAIN_OIDC_CLIENT_ID"), getenv("MOUNTAIN_OIDC_CLIENT_SECRET"), getenv("MOUNTAIN_SESSION_KEY"); iss != "" && id != "" && sec != "" && len(key) >= 32 {
		c.OIDC = &oidcEnv{Issuer: iss, ClientID: id, Secret: sec, Key: key} // any unset or a short key: off, silently
	}
	c.Traces = getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	c.Machine = machineEnv{Cert: getenv("MOUNTAIN_MACHINE_CERT"), Issuer: getenv("MOUNTAIN_MACHINE_ISSUER"),
		ClientID: getenv("MOUNTAIN_MACHINE_CLIENT_ID"), Sub: getenv("MOUNTAIN_MACHINE_SUB")}
	if err == nil && (o.GraphFixture != "" || o.LiveFixture != "") && !o.Dev {
		err = errors.New("atlas: fixtures are --dev only")
	}
	return c, err
}

// tracesEndpoint is serve's check of Config.Traces, made with machineFrom's (a refusal to start is a JSON ERROR line, exit
// 1, as every one but the flags'): "" = no tracing; else the OTLP/HTTP traces URL in full, because the exporter posts
// to the path as given ("" or "/" would be the collector's root, not /v1/traces).
func tracesEndpoint(v string) error {
	if v == "" {
		return nil
	}
	if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path == "" || u.Path == "/" {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT %q is not an http(s) URL with its traces path (…/v1/traces)", v)
	}
	return nil
}

// serveFlags is serve's FlagSet, writing into o.
func serveFlags(o *Opts) *flag.FlagSet {
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
	return f
}

// parseProject is `atlas project`'s flags. --dsn's default is DATABASE_URL, read only when --dsn is not given, so the
// flag's help never prints the URL (it holds the database password).
func parseProject(args []string, getenv func(string) string, out io.Writer) (dir, dsn string, drop bool, err error) {
	f := projectFlags(&dir, &dsn, &drop)
	f.SetOutput(out)
	if err = f.Parse(args); err != nil {
		return
	}
	given := false
	f.Visit(func(fl *flag.Flag) { given = given || fl.Name == "dsn" })
	if !given {
		dsn = getenv("DATABASE_URL")
	}
	if dir == "" || dsn == "" {
		err = errors.New("project: --repos and a DSN (--dsn or DATABASE_URL) are required")
	}
	return
}

// projectFlags is `atlas project`'s FlagSet.
func projectFlags(dir, dsn *string, drop *bool) *flag.FlagSet {
	f := flag.NewFlagSet("project", flag.ContinueOnError)
	f.StringVar(dir, "repos", "", "directory holding the repo checkouts (one per GitHub repo name)")
	f.StringVar(dsn, "dsn", "", "Postgres URL (default: DATABASE_URL)")
	f.BoolVar(drop, "drop-repos", false, "accept that repos the last projection read are gone (their nodes close)")
	return f
}

// serveFromProcess and projectFromProcess are the only readers of the process's environment.
func serveFromProcess() (Config, error) { return loadConfig(os.Args[1:], os.Getenv) }

func projectFromProcess() (dir, dsn string, drop bool, err error) {
	return parseProject(os.Args[2:], os.Getenv, os.Stderr)
}

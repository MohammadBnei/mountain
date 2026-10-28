package extract

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// Watchtowers and supply lines (plan/13 §3, slice 2, owner 2026-10-26): both are curated rows in realms.yaml, checked
// against what the sources declare, never invented from evidence alone.
//
//   - watch: a node's alert rules. Every rule a row names must be declared in a source (a Prometheus `alert:` name,
//     a Grafana rule's uid), and every declared rule must be in a row or under general (the town's general watch):
//     drift both ways. A row becomes the node's `watch` (Prometheus) and `watch-grafana` attributes; a probed node
//     with no row gets `probe: <target>, no alert rule`. A watched Prometheus rule's severity and for: (seconds) go
//     into `watch-rules` (`<name>=<severity>/<for>, ...`), read as text from its rule block (a rule can sit inside a
//     templated string); a watched rule with no literal severity is drift.
//   - needs: what a node depends on, each written as its evidence. A row's evidence must be found (else drift), and
//     evidence found for an app with no row naming it is drift too. A row becomes a needs edge (attrs: via, checked).
//
// Only names are read: alert names, rule uids, database names, hostnames of redirect URLs, middleware names and the
// presence of keys. Never an expression's value, an address, a user or a secret.

const EdgeNeeds = "needs" // node -> what it depends on (attrs: via, checked)

var (
	alertName = regexp.MustCompile(`(?m)^\s*-?\s*alert:\s*["']?([A-Za-z][A-Za-z0-9_]*)["']?\s*$`)
	urlHost   = regexp.MustCompile(`(?m)^\s*-?\s*url:\s*["']?https://([a-z0-9.-]+)`)
	s3Ref     = regexp.MustCompile(`garage-s3|s3\.bnei\.dev`)
)

// ruleSeverities are the severities a watched rule may carry (mountain's own rules use notice).
var ruleSeverities = []string{"critical", "warning", "notice", "info", "none"}

type ruleMeta struct {
	severity string
	forSec   int
}

var (
	ruleFor   = regexp.MustCompile(`^\s*for:\s*["']?([0-9a-z]+)["']?\s*$`)
	ruleSev   = regexp.MustCompile(`^\s*severity:\s*["']?([^"'\s]+)["']?\s*$`)
	promDur   = regexp.MustCompile(`([0-9]+)(ms|s|m|h|d|w|y)`)
	durFactor = map[string]float64{"ms": 0.001, "s": 1, "m": 60, "h": 3600, "d": 86400, "w": 7 * 86400, "y": 365 * 86400}
)

// ruleBlock reads the severity and for: of the rule whose `alert:` line spans b[start:end]: the lines after it that are
// indented deeper than the list item it belongs to, comments skipped. An unparseable for: reads as 0.
func ruleBlock(b []byte, start, end int) ruleMeta {
	lineStart := bytes.LastIndexByte(b[:start+1], '\n') + 1
	line := string(b[lineStart:end])
	ind := len(line) - len(strings.TrimLeft(line, " "))
	dash := strings.HasPrefix(strings.TrimLeft(line, " "), "-")
	var m ruleMeta
	for _, l := range strings.Split(string(b[min(end+1, len(b)):]), "\n") {
		t := strings.TrimLeft(l, " ")
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		li := len(l) - len(t)
		if (dash && li <= ind) || (!dash && (li < ind || (li <= ind && strings.HasPrefix(t, "-")))) {
			break
		}
		if x := ruleSev.FindStringSubmatch(l); x != nil && m.severity == "" {
			m.severity = x[1]
		}
		if x := ruleFor.FindStringSubmatch(l); x != nil && promDur.ReplaceAllString(x[1], "") == "" {
			var sec float64
			for _, p := range promDur.FindAllStringSubmatch(x[1], -1) {
				var n float64
				_, _ = fmt.Sscan(p[1], &n)
				sec += n * durFactor[p[2]]
			}
			m.forSec = int(sec)
		}
	}
	return m
}

// sourceFile is a YAML file of a source, with the app it belongs to ("" when none).
type sourceFile struct {
	repo, path, app string
	b               []byte
	src             graph.Source
}

// watchNeeds checks the watch and needs rows against the sources and writes them into g (attrs, edges, drift). It runs
// after place, so the nodes, their realms and the hostnames' route edges are known.
func watchNeeds(repos Repos, rl *realms.Realms, g *graph.Graph) {
	x := &Ctx{Repos: repos, Realms: rl}
	has := map[string]bool{}
	for _, n := range g.Nodes {
		has[n.ID] = true
	}
	files := yamlFiles(x, rl, has, deployPaths(g))
	var drift []graph.Drift
	d := func(kind, subject, detail string, src graph.Source) {
		drift = append(drift, graph.Drift{Kind: kind, Subject: subject, Detail: detail, Source: src})
	}
	realmsSrc := graph.Source{Repo: "mountain", Path: "realms/realms.yaml"}

	// ---- watch
	declared := map[string]graph.Source{} // "prometheus/<name>" | "grafana/<uid>" -> where
	meta := map[string]ruleMeta{}         // a Prometheus rule's severity and for:, from its first declaration
	for _, f := range files {
		for _, m := range alertName.FindAllSubmatchIndex(f.b, -1) {
			name := string(f.b[m[2]:m[3]])
			declared["prometheus/"+name] = f.src
			if _, ok := meta[name]; !ok {
				meta[name] = ruleBlock(f.b, m[0], m[1])
			}
		}
		for _, uid := range grafanaRules(f.b) {
			declared["grafana/"+uid] = f.src
		}
	}
	named := map[string]bool{}
	note := func(subject string, w realms.Watch) {
		for via, list := range map[string][]string{"prometheus": w.Prometheus, "grafana": w.Grafana} {
			for _, r := range list {
				named[via+"/"+r] = true
				if _, ok := declared[via+"/"+r]; !ok {
					d(graph.DriftUnresolved, subject, fmt.Sprintf("watch: %s rule %s is declared in no source", via, r), realmsSrc)
				}
			}
		}
	}
	note("realms.yaml general", rl.General)
	for _, id := range sortedKeys(rl.Watch) {
		note(id, rl.Watch[id])
	}
	for _, k := range sortedKeys(declared) {
		if !named[k] {
			via, r, _ := strings.Cut(k, "/")
			d(graph.DriftUnassigned, graph.ID(graph.KindRepo, declared[k].Repo), fmt.Sprintf("%s rule %s watches nothing in realms.yaml (watch: or general:)", via, r), declared[k])
		}
	}
	// the default rules' list (contract.DefaultRules) is read from one chart version: another pinned makes it stale
	if pin, src := chartPin(x, "kube-prometheus-stack"); pin != "" && pin != contract.DefaultRulesChart {
		d(graph.DriftMirror, graph.ID(graph.KindRepo, "infra-bootstrap"), "kube-prometheus-stack is pinned at "+pin+", but contract.DefaultRules was read from "+contract.DefaultRulesChart+": regenerate it (contract/gen/defaultrules)", src)
	}
	// ... and from the chart's defaults: values that change a rule's severity, for:, labels or set make it stale too
	for _, k := range ruleOverrides(x) {
		d(graph.DriftMirror, graph.ID(graph.KindRepo, "infra-bootstrap"), "kube-prometheus-stack's values set "+k+": contract.DefaultRules holds the chart's defaults, so the sim's rules no longer match", graph.Source{Repo: "infra-bootstrap", Path: promValues})
	}
	probed := map[string]string{}
	for target, id := range rl.Probes {
		probed[id] = target
	}
	for i, n := range g.Nodes {
		w, ok := rl.Watch[n.ID]
		set := func(k, v string) {
			if g.Nodes[i].Attrs == nil {
				g.Nodes[i].Attrs = map[string]string{}
			}
			g.Nodes[i].Attrs[k] = v
		}
		if ok && len(w.Prometheus) > 0 {
			set("watch", strings.Join(w.Prometheus, ", "))
			var rules []string
			for _, r := range w.Prometheus {
				mt, found := meta[r]
				switch {
				case !found: // declared in no source: drift above
				case !slices.Contains(ruleSeverities, mt.severity):
					d(graph.DriftUnresolved, n.ID, fmt.Sprintf("watch: prometheus rule %s has no literal severity (%q)", r, mt.severity), declared["prometheus/"+r])
				default:
					rules = append(rules, fmt.Sprintf("%s=%s/%d", r, mt.severity, mt.forSec))
				}
			}
			if len(rules) > 0 {
				set("watch-rules", strings.Join(rules, ", "))
			}
		}
		if ok && len(w.Grafana) > 0 {
			set("watch-grafana", strings.Join(w.Grafana, ", "))
		}
		if t, p := probed[n.ID]; p && !ok {
			set("probe", t+", no alert rule")
		}
	}

	// ---- needs: the evidence each kind finds, per app
	evidence := map[string]map[string]graph.Source{} // kind -> app -> where
	found := func(kind, app string, src graph.Source) {
		if app == "" {
			return
		}
		if evidence[kind] == nil {
			evidence[kind] = map[string]graph.Source{}
		}
		if _, ok := evidence[kind][app]; !ok {
			evidence[kind][app] = src
		}
	}
	in := &evidenceIn{routes: map[string][]string{}}
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeRoute {
			in.routes[strings.TrimPrefix(e.From, graph.KindDNS+":")] = append(in.routes[strings.TrimPrefix(e.From, graph.KindDNS+":")], e.To)
		}
	}
	for _, mw := range forwardAuthNames(files) {
		in.useMW = append(in.useMW, regexp.MustCompile(`(?m)^\s*-?\s*name:\s*["']?`+regexp.QuoteMeta(mw)+`["']?\s*$`))
	}
	finders := evidenceFinders()
	for _, f := range files {
		in.code = ""
		if f.app != "" {
			in.code = uncommented(f.b)
		}
		for kind, find := range finders {
			find(in, f, func(app string) { found(kind, app, f.src) })
		}
	}
	dbs := pigstyDatabases(x)

	stated := map[string]bool{} // "<kind>/<app>"
	var edges []graph.Edge
	for _, id := range sortedKeys(rl.Needs) {
		for _, s := range rl.Needs[id] {
			n, _ := realms.ParseNeed(s) // realms.Load validated it
			stated[n.Via+"/"+id] = true
			checked := "yes"
			// declared and postgres have no finder: declared's evidence lives outside the sources, and postgres is
			// checked against pigsty's pg_databases (dbs), not found in the files the finders walk
			switch n.Via {
			case "declared":
				checked = "declared"
			case "postgres":
				if !dbs[n.Arg] {
					checked = "no"
					d(graph.DriftUnresolved, id, "needs postgres:"+n.Arg+": no such database in pigsty's pg_databases", realmsSrc)
				}
			default:
				if _, ok := evidence[n.Via][id]; !ok {
					checked = "no"
					d(graph.DriftUnresolved, id, "needs "+n.Via+": no evidence of it in the sources", realmsSrc)
				}
			}
			if !has[id] || !has[n.On] {
				d(graph.DriftUnresolved, id, "needs "+s+": "+n.On+" or "+id+" is not a node", realmsSrc)
				continue
			}
			attrs := map[string]string{"via": n.Via, "checked": checked}
			if n.Arg != "" {
				attrs["db"] = n.Arg
			}
			src := realmsSrc
			if ev, ok := evidence[n.Via][id]; ok {
				src = ev
			}
			edges = append(edges, graph.Edge{From: id, To: n.On, Kind: EdgeNeeds, Origin: graph.Declared, Attrs: attrs, Source: src})
		}
	}
	for _, kind := range sortedKeys(evidence) {
		for _, app := range sortedKeys(evidence[kind]) {
			if !stated[kind+"/"+app] && has[app] {
				d(graph.DriftUnresolved, app, "evidence of "+kind+" ("+evidence[kind][app].Path+") but no needs row names it", evidence[kind][app])
			}
		}
	}
	// a needs edge may repeat (oidc and forwardauth both name authentik, three databases on one cluster): one edge
	// per (from, to), its via and databases joined, checked the weakest of its rows (no < declared < yes)
	rank := map[string]int{"no": 0, "declared": 1, "yes": 2}
	at := map[string]int{}
	var merged []graph.Edge
	for _, e := range edges {
		k := e.From + ">" + e.To
		i, ok := at[k]
		if !ok {
			at[k] = len(merged)
			merged = append(merged, e)
			continue
		}
		m := merged[i].Attrs
		m["via"] += ", " + e.Attrs["via"]
		if e.Attrs["db"] != "" {
			m["db"] = strings.TrimPrefix(m["db"]+", "+e.Attrs["db"], ", ")
		}
		if rank[e.Attrs["checked"]] < rank[m["checked"]] {
			m["checked"] = e.Attrs["checked"]
		}
	}
	edges = merged
	drift = append(drift, walls(x, rl, files, forwardAuthNames(files), g)...)
	_ = g.Merge(graph.Graph{Edges: edges, Drift: drift}) // both ends checked above: cannot fail
	modelFacts(rl, g)
}

// evidenceFinder finds one kind of needs evidence in one source file, calling found with each app it shows the need
// for. found keeps the first source per (kind, app) and ignores an empty app.
type evidenceFinder func(in *evidenceIn, f sourceFile, found func(app string))

// evidenceIn is what the finders share: each hostname's apps (route edges), a pattern per forwardAuth middleware for
// a name: line naming it, and the file's text with its comments removed ("" for a file with no app).
type evidenceIn struct {
	routes map[string][]string
	useMW  []*regexp.Regexp
	code   string
}

// evidenceFinders is the registry, one finder per kind of evidence a needs row can name (realms.Suppliers' keys but
// postgres, which pigsty's pg_databases answers). Their order is not observable: each kind keeps its own evidence.
func evidenceFinders() map[string]evidenceFinder {
	return map[string]evidenceFinder{
		"oidc":        findOIDC,
		"forwardauth": findForwardAuth,
		"s3":          findS3,
		"infisical":   findInfisical,
	}
}

// findOIDC: an authentik blueprint (gitops/bootstrap/authentik-blueprint-*, in any source repo, whether or not an app
// owns it) whose redirect URL's host routes to the app. It reads the file as written.
func findOIDC(in *evidenceIn, f sourceFile, found func(app string)) {
	if !strings.HasPrefix(f.path, "gitops/bootstrap/authentik-blueprint-") {
		return
	}
	for _, m := range urlHost.FindAllSubmatch(f.b, -1) {
		for _, app := range in.routes[string(m[1])] {
			found(app)
		}
	}
}

// findForwardAuth: an app's file whose uncommented text has a name: line (a list item or not) naming one of the
// forwardAuth middlewares.
func findForwardAuth(in *evidenceIn, f sourceFile, found func(app string)) {
	if f.app == "" {
		return
	}
	for _, re := range in.useMW {
		if re.MatchString(in.code) {
			found(f.app)
		}
	}
}

// findS3: an app's file whose uncommented text names Garage's S3 endpoint.
func findS3(in *evidenceIn, f sourceFile, found func(app string)) {
	if f.app != "" && s3Ref.MatchString(in.code) {
		found(f.app)
	}
}

// findInfisical: an app's file whose first YAML document (as written: a values file has one) turns infisical on;
// the chart's default is off.
func findInfisical(in *evidenceIn, f sourceFile, found func(app string)) {
	if f.app == "" {
		return
	}
	var top map[string]any
	if yaml.Unmarshal(f.b, &top) == nil {
		if inf, ok := top["infisical"].(map[string]any); ok && inf["enabled"] == true {
			found(f.app)
		}
	}
}

// deployPaths is repo -> path -> the app an ArgoCD Application deploys it for (its git edges' paths: a values file, a
// chart directory). A path two apps deploy (the common chart) belongs to neither.
func deployPaths(g *graph.Graph) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, e := range g.Edges {
		// bootstrap (ADR-0021's self-sync) deploys every app's wiring (middlewares, secrets, blueprints): its files
		// say nothing about bootstrap itself
		if e.Kind != graph.EdgeGit || e.Attrs["paths"] == "" || e.From == "app:bootstrap" {
			continue
		}
		repo := strings.TrimPrefix(e.To, graph.KindRepo+":")
		if out[repo] == nil {
			out[repo] = map[string]string{}
		}
		for _, p := range strings.Split(e.Attrs["paths"], ",") {
			if p = strings.Trim(strings.TrimSpace(p), "/"); p == "" {
				continue
			}
			if prev, ok := out[repo][p]; ok && prev != e.From {
				out[repo][p] = "" // shared
			} else {
				out[repo][p] = e.From
			}
		}
	}
	return out
}

// skipDirs are directories never read: dependencies and test fixtures anywhere, and at a repo's root the vendored
// trees and submodules (pigsty's own tree, kubespray, the agent-fleet and k8s-cluster submodules).
func skipDir(rl *realms.Realms, repo, dir string) bool {
	if base := path.Base(dir); base == "node_modules" || base == "testdata" || base == "vendor" {
		return true
	}
	return slices.Contains(rl.Skip[repo], dir) || dir == "pigsty" || dir == "kubespray" || dir == "agent-fleet" || dir == "k8s-cluster"
}

// yamlFiles is every YAML file of the sources, minus the directories skipDir names, each with the app it belongs to.
func yamlFiles(x *Ctx, rl *realms.Realms, has map[string]bool, deploys map[string]map[string]string) []sourceFile {
	var out []sourceFile
	for _, repo := range rl.Sources {
		_ = x.Walk(repo, func(dir string) bool { return skipDir(rl, repo, dir) }, func(p string, size int64) error {
			if ext := path.Ext(p); (ext != ".yaml" && ext != ".yml") || size > 1<<20 {
				return nil
			}
			b, src, err := x.Read(repo, p)
			if err != nil {
				return nil
			}
			out = append(out, sourceFile{repo: repo, path: p, app: fileApp(rl, has, deploys, repo, p), b: b, src: src})
			return nil
		})
	}
	return out
}

// fileApp is the app a source file belongs to: the app an Application deploys it for (its git edge's paths, the
// longest match), else the component realms.yaml gives its path, else for infra-bootstrap's per-app values
// (gitops/platform/values/<x>/) the app named <x> or platform-<x>, and the redirectors' files.
func fileApp(rl *realms.Realms, has map[string]bool, deploys map[string]map[string]string, repo, p string) string {
	best, n := "", 0
	for prefix, app := range deploys[repo] {
		if (p == prefix || strings.HasPrefix(p, prefix+"/")) && len(prefix) > n {
			best, n = app, len(prefix)
		}
	}
	if n > 0 {
		return best // "" when two apps deploy it
	}
	if id := rl.Owner(repo, p); id != "" {
		return id
	}
	if repo != "infra-bootstrap" {
		return ""
	}
	if rest, ok := strings.CutPrefix(p, "gitops/platform/values/"); ok {
		name, _, _ := strings.Cut(rest, "/")
		for _, id := range []string{"app:" + name, "app:platform-" + name} {
			if has[id] {
				return id
			}
		}
	}
	if strings.HasPrefix(p, "gitops/redirectors/") && has["app:redirectors"] {
		return "app:redirectors"
	}
	return ""
}

// grafanaRules is the uids of the Grafana alert rules in a YAML file: any list under a `rules` key whose items have a
// uid and a title (Grafana's provisioning, and common-app-chart's logAlerts). A file that isn't plain YAML has none.
func grafanaRules(b []byte) []string {
	var out []string
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				walk(c, k)
			}
		case []any:
			for _, c := range t {
				if m, ok := c.(map[string]any); ok && key == "rules" {
					uid, _ := m["uid"].(string)
					if _, titled := m["title"].(string); uid != "" && titled {
						out = append(out, uid)
						continue
					}
				}
				walk(c, key)
			}
		}
	}
	dec := yaml.NewDecoder(bytes.NewReader(b)) // every document of the file
	for {
		var root any
		if dec.Decode(&root) != nil {
			break
		}
		walk(root, "")
	}
	slices.Sort(out)
	return out
}

// forwardAuthNames is the names of the authentik forwardAuth middlewares infra-bootstrap declares.
func forwardAuthNames(files []sourceFile) []string {
	var out []string
	for _, f := range files {
		if f.repo != "infra-bootstrap" || !strings.HasPrefix(f.path, "gitops/bootstrap/") || !strings.HasSuffix(f.path, "forwardauth-middleware.yaml") {
			continue
		}
		var m struct {
			Kind     string
			Metadata struct{ Name string }
		}
		if yaml.Unmarshal(f.b, &m) == nil && m.Kind == "Middleware" && m.Metadata.Name != "" {
			out = append(out, m.Metadata.Name)
		}
	}
	slices.Sort(out)
	return out
}

// pigstyDatabases is the database names in pigsty.yml's pg_databases (names only: never owners, users or passwords).
func pigstyDatabases(x *Ctx) map[string]bool {
	out := map[string]bool{}
	b, _, err := x.Read("infra-bootstrap", pigstyPath)
	if err != nil {
		return out
	}
	var inv struct {
		All struct {
			Children map[string]struct {
				Vars struct {
					DBs []struct{ Name string } `yaml:"pg_databases"`
				} `yaml:"vars"`
			} `yaml:"children"`
		} `yaml:"all"`
	}
	if yaml.Unmarshal(b, &inv) != nil {
		return out
	}
	for _, c := range inv.All.Children {
		for _, db := range c.Vars.DBs {
			if db.Name != "" {
				out[db.Name] = true
			}
		}
	}
	return out
}

// uncommented is a YAML file without its comments (whole lines, and the rest of a line after " #"), so prose about a
// middleware or an endpoint isn't evidence.
func uncommented(b []byte) string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if i := strings.Index(l, " #"); i >= 0 {
			l = l[:i]
		}
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// chartPin is the chartRevision infra-bootstrap's platform ApplicationSet pins for a chart ("" when it names none).
const promValues = "gitops/platform/values/prometheus/values.yaml"

// ruleOverrides is the kube-prometheus-stack values keys that change its default rules (the dig "<name>" "<key>"
// customRules, keep_firing_for, the aggregation labels, a disabled rule or group) set in infra-bootstrap's values.
func ruleOverrides(x *Ctx) []string {
	b, _, err := x.Read("infra-bootstrap", promValues)
	if err != nil {
		return nil
	}
	var v struct {
		CustomRules  map[string]any `yaml:"customRules"`
		DefaultRules map[string]any `yaml:"defaultRules"`
	}
	if yaml.Unmarshal(b, &v) != nil {
		return nil
	}
	var out []string
	if len(v.CustomRules) > 0 {
		out = append(out, "customRules")
	}
	for _, k := range []string{"keepFiringFor", "additionalAggregationLabels", "disabled", "rules", "additionalRuleLabels", "additionalRuleGroupLabels"} {
		if x, ok := v.DefaultRules[k]; ok && x != nil {
			out = append(out, "defaultRules."+k)
		}
	}
	return out
}

func chartPin(x *Ctx, chart string) (string, graph.Source) {
	b, src, err := x.Read("infra-bootstrap", "gitops/bootstrap/platform.applicationset.yaml")
	if err != nil {
		return "", src
	}
	var root any
	if yaml.Unmarshal(b, &root) != nil {
		return "", src
	}
	var pin string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["chartName"] == chart {
				if r, ok := t["chartRevision"].(string); ok {
					pin = r
				}
			}
			for _, c := range t {
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(root)
	return pin, src
}

package extract

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// Watchtowers and supply lines (plan/13 §3, slice 2, owner 2026-10-26): both are curated rows in realms.yaml, checked
// against what the sources declare, never invented from evidence alone.
//
//   - watch: a node's alert rules. Every rule a row names must be declared in a source (a Prometheus `alert:` name,
//     a Grafana rule's uid), and every declared rule must be in a row or under general (the town's general watch):
//     drift both ways. A row becomes the node's `watch` (Prometheus) and `watch-grafana` attributes; a probed node
//     with no row gets `probe: <target>, no alert rule`.
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
	files := yamlFiles(x, rl, has)
	var drift []graph.Drift
	d := func(kind, subject, detail string, src graph.Source) {
		drift = append(drift, graph.Drift{Kind: kind, Subject: subject, Detail: detail, Source: src})
	}
	realmsSrc := graph.Source{Repo: "mountain", Path: "realms/realms.yaml"}

	// ---- watch
	declared := map[string]graph.Source{} // "prometheus/<name>" | "grafana/<uid>" -> where
	for _, f := range files {
		for _, m := range alertName.FindAllSubmatch(f.b, -1) {
			declared["prometheus/"+string(m[1])] = f.src
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
	routes := map[string][]string{} // hostname -> the apps it routes to
	for _, e := range g.Edges {
		if e.Kind == graph.EdgeRoute {
			routes[strings.TrimPrefix(e.From, graph.KindDNS+":")] = append(routes[strings.TrimPrefix(e.From, graph.KindDNS+":")], e.To)
		}
	}
	middlewares := forwardAuthNames(files)
	for _, f := range files {
		if strings.HasPrefix(f.path, "gitops/bootstrap/authentik-blueprint-") {
			for _, m := range urlHost.FindAllSubmatch(f.b, -1) {
				for _, app := range routes[string(m[1])] {
					found("oidc", app, f.src)
				}
			}
		}
		if f.app == "" {
			continue
		}
		code := uncommented(f.b)
		for _, mw := range middlewares {
			if regexp.MustCompile(`(?m)^\s*-?\s*name:\s*["']?` + regexp.QuoteMeta(mw) + `["']?\s*$`).MatchString(code) {
				found("forwardauth", f.app, f.src)
			}
		}
		if s3Ref.MatchString(code) {
			found("s3", f.app, f.src)
		}
		var top map[string]any
		if yaml.Unmarshal(f.b, &top) == nil {
			if inf, ok := top["infisical"].(map[string]any); ok && inf["enabled"] != false {
				found("infisical", f.app, f.src)
			}
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
	// a needs edge may repeat (oidc and forwardauth both name authentik): keep the first per (from, to)
	seen := map[string]bool{}
	edges = slices.DeleteFunc(edges, func(e graph.Edge) bool { k := e.From + ">" + e.To; dup := seen[k]; seen[k] = true; return dup })
	g.Merge(graph.Graph{Edges: edges, Drift: drift}) // both ends checked above: cannot fail
}

// yamlFiles is every YAML file of the sources, minus the vendored directories (pigsty's own tree, the submodules),
// each with the app it belongs to.
func yamlFiles(x *Ctx, rl *realms.Realms, has map[string]bool) []sourceFile {
	var out []sourceFile
	for _, repo := range rl.Sources {
		skip := rl.Skip[repo]
		x.Walk(repo, func(dir string) bool {
			return slices.Contains(skip, dir) || dir == "pigsty" || dir == "kubespray" || dir == "agent-fleet" || dir == "k8s-cluster" || dir == "node_modules"
		}, func(p string, size int64) error {
			if ext := path.Ext(p); (ext != ".yaml" && ext != ".yml") || size > 1<<20 {
				return nil
			}
			b, src, err := x.Read(repo, p)
			if err != nil {
				return nil
			}
			out = append(out, sourceFile{repo: repo, path: p, app: fileApp(rl, has, repo, p), b: b, src: src})
			return nil
		})
	}
	return out
}

// fileApp is the app a source file belongs to: the component realms.yaml gives its path, else for infra-bootstrap's
// per-app values (gitops/platform/values/<x>/) the app named <x> or platform-<x>, and the redirectors' files.
func fileApp(rl *realms.Realms, has map[string]bool, repo, p string) string {
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
	var root any
	if yaml.Unmarshal(b, &root) != nil {
		return nil
	}
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
	walk(root, "")
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

// uncommented is a YAML file without its comment lines, so prose about a middleware or an endpoint isn't evidence.
func uncommented(b []byte) string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

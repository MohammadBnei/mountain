package extract

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// Pigsty's data services (plan/13 §7 A7b), from pigsty.yml. It reads named fields only, takes literal values only (a
// Jinja template or an odd type is drift, never a failed projection), and never reads a secret.

var pigstyExtractor = Extractor{Name: "pigsty", Version: 1, Needs: []string{"infra-bootstrap"}, Run: pigsty}

const pigstyPath = "pigsty/pigsty.yml"

// pigstyVars are the only group vars atlas reads: the cluster names. pg_users, pg_role (on hosts, its labels
// unreconciled), passwords and addresses are never read.
type pigstyVars struct {
	PG    string `yaml:"pg_cluster"`
	Redis string `yaml:"redis_cluster"`
	Etcd  string `yaml:"etcd_cluster"`
}

// pigstySkipped are the groups Pigsty uses for itself, not drawn yet (its infra node: VictoriaMetrics, its Grafana);
// any other group without a cluster name atlas knows is drift.
var pigstySkipped = []string{"infra"}

// pigsty adds a db node per Postgres and Redis cluster, and runs-on edges to the machines they run on, joined by IP
// through the LAN records (pigsty keys hosts by IP). The Postgres clusters also run on the etcd members (Patroni's
// DCS, the witness among them). An edge whose machine is not a node becomes drift at the end of the run (Ctx.Member).
func pigsty(x *Ctx) error {
	b, src, err := x.Read("infra-bootstrap", pigstyPath)
	if err != nil {
		return err
	}
	file := graph.ID(graph.KindRepo, "infra-bootstrap")
	drift := func(subject, detail string) {
		x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: subject, Detail: detail, Source: src})
	}
	var inv struct {
		All struct {
			Children map[string]yaml.Node `yaml:"children"`
		} `yaml:"all"`
	}
	if err := yaml.Unmarshal(b, &inv); err != nil {
		drift(file, pigstyPath+" is not an inventory (all.children)")
		return nil
	}
	lb, _, err := x.Read("infra-bootstrap", lanPath)
	if err != nil {
		return err
	}
	var lan struct {
		Hosts map[string]string `yaml:"bnei_lan_hosts"`
	}
	if err := yaml.Unmarshal(lb, &lan); err != nil {
		return fmt.Errorf("%s: %w", lanPath, err)
	}
	byIP := map[string]string{}
	for name, ip := range lan.Hosts {
		byIP[ip] = name
	}
	clusters := map[string]map[string][]string{} // db id -> machine name -> what it runs there
	engine := map[string]string{}
	run := func(db, host, as string) {
		if _, err := netip.ParseAddr(host); err != nil {
			drift(db, "pigsty has a non-IP host key")
			return
		}
		name, ok := byIP[host]
		if !ok {
			drift(db, "pigsty host "+host+" is not in "+lanPath)
			return
		}
		if m := clusters[db]; !slices.Contains(m[name], as) {
			m[name] = append(m[name], as)
		}
	}
	var etcd []string
	for _, name := range sortedKeys(inv.All.Children) {
		var gr struct {
			Hosts map[string]yaml.Node `yaml:"hosts"`
			Vars  pigstyVars           `yaml:"vars"`
		}
		node := inv.All.Children[name]
		if err := node.Decode(&gr); err != nil {
			drift(file, "pigsty group "+name+" is not a {hosts, vars} map")
			continue
		}
		var cluster, as string
		switch {
		case gr.Vars.PG != "":
			cluster, as = gr.Vars.PG, "postgres"
		case gr.Vars.Redis != "":
			cluster, as = gr.Vars.Redis, "redis"
		case gr.Vars.Etcd != "":
			etcd = append(etcd, sortedKeys(gr.Hosts)...)
			continue
		default:
			if !slices.Contains(pigstySkipped, name) {
				drift(file, "pigsty group "+name+" has no cluster atlas draws")
			}
			continue
		}
		db := graph.ID(graph.KindDB, cluster)
		if !graph.Valid(db) || strings.Contains(cluster, "{{") {
			drift(file, "pigsty group "+name+" has a cluster name that is not a valid id")
			continue
		}
		switch engine[db] {
		case "":
			clusters[db], engine[db] = map[string][]string{}, as
			x.Node(graph.Node{ID: db, Kind: graph.KindDB, Attrs: map[string]string{"engine": as}, Source: src})
		case as:
		default:
			drift(db, "pigsty group "+name+" reuses the name of a "+engine[db]+" cluster")
			continue
		}
		for _, h := range sortedKeys(gr.Hosts) {
			run(db, h, as)
		}
	}
	for _, db := range sortedKeys(clusters) {
		if engine[db] == "postgres" {
			for _, h := range etcd {
				run(db, h, "etcd")
			}
		}
	}
	for _, db := range sortedKeys(clusters) {
		for _, name := range sortedKeys(clusters[db]) {
			x.Member(graph.Edge{From: db, To: graph.ID(graph.KindVM, name), Kind: graph.EdgeRunsOn, Origin: graph.Declared,
				Attrs: map[string]string{"as": strings.Join(clusters[db][name], ",")}, Source: src})
		}
	}
	return nil
}

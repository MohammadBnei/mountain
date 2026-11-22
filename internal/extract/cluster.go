package extract

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/netip"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// The cluster's own software that no ArgoCD Application deploys (plan/13 §7 A7b): kubespray's add-ons, from the
// inventory's group_vars, and Pigsty's data services, from pigsty.yml. Both read named fields only, take literal
// values only (a Jinja template or an odd type is drift, never a failed projection), and never read an address or a
// secret.

var kubesprayExtractor = Extractor{Name: "kubespray", Version: 1, Needs: []string{"infra-bootstrap"}, Run: kubespray}

const groupVars = "inventory/ukubi/group_vars/k8s_cluster/"

// groupVarFiles are read in ansible's order for a group_vars directory (by name): a later file's key wins.
var groupVarFiles = []string{"addons.yml", "k8s-cluster.yml", "k8s-net-cilium.yml"}

// kubesprayKeys are the only group_vars keys atlas reads: which add-ons run, never an address, a pool or a secret.
var kubesprayKeys = []string{"kube_network_plugin", "kube_proxy_mode", "dns_mode", "enable_nodelocaldns", "metallb_enabled",
	"metallb_namespace", "kube_vip_enabled", "kube_vip_controlplane_enabled", "cilium_kube_proxy_replacement",
	"cilium_enable_hubble", "cilium_encryption_enabled", "cilium_encryption_type"}

// kubesprayDefaults are kubespray's own defaults (roles/kubespray_defaults) for the keys that decide what it installs
// when the inventory leaves them out.
var kubesprayDefaults = map[string]string{"kube_network_plugin": "calico", "kube_proxy_mode": "ipvs", "dns_mode": "coredns",
	"enable_nodelocaldns": "true", "metallb_namespace": "metallb-system"}

// noPlugin are kube_network_plugin values that install no CNI of kubespray's own.
var noPlugin = []string{"none", "cni", "custom_cni"}

func kubespray(x *Ctx) error {
	vars := map[string]string{}       // key -> literal value
	from := map[string]graph.Source{} // key -> the file that set it
	var cluster graph.Source          // k8s-cluster.yml: where a defaulted add-on is sourced
	for _, f := range groupVarFiles {
		b, s, err := x.Read("infra-bootstrap", groupVars+f)
		if errors.Is(err, fs.ErrNotExist) { // kubespray is gone (infra-bootstrap #491): no add-ons from it, not a failed Run
			slog.Warn("extract kubespray: " + groupVars + f + " not found: no kubespray add-ons")
			return nil
		}
		if err != nil {
			return err
		}
		var doc map[string]yaml.Node
		if err := yaml.Unmarshal(b, &doc); err != nil {
			x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, "infra-bootstrap"), Detail: groupVars + f + " is not a YAML map", Source: s})
			continue
		}
		for _, k := range kubesprayKeys {
			n, ok := doc[k]
			if !ok {
				continue
			}
			if n.Kind != yaml.ScalarNode || strings.Contains(n.Value, "{{") {
				x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, "infra-bootstrap"), Detail: groupVars + f + ": " + k + " is not a literal", Source: s})
				delete(vars, k)
				continue
			}
			vars[k], from[k] = n.Value, s
		}
		if f == "k8s-cluster.yml" {
			cluster = s
		}
	}
	get := func(k string) string {
		if v, ok := vars[k]; ok {
			return v
		}
		return kubesprayDefaults[k]
	}
	on := func(k string) bool { v := strings.ToLower(get(k)); return v == "true" || v == "yes" }
	add := func(name, key string, attrs map[string]string) { // key: the setting that decides it, for the source
		src, ok := from[key]
		if !ok {
			src = cluster
		}
		id := graph.ID(graph.KindAddon, name)
		if !graph.Valid(id) {
			x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: graph.ID(graph.KindRepo, "infra-bootstrap"), Detail: "a kubespray add-on name is not a valid id", Source: src})
			return
		}
		x.Node(graph.Node{ID: id, Kind: graph.KindAddon, Attrs: attrs, Source: src})
	}
	// "into" is the namespace kubespray installs into, not "namespace": several add-ons share kube-system, and the
	// page counts a namespace attr's whole usage as the node's own (plan/13: per-workload usage comes later).
	plugin := get("kube_network_plugin")
	replaced := plugin == "cilium" && slices.Contains([]string{"true", "strict"}, strings.ToLower(get("cilium_kube_proxy_replacement")))
	if !slices.Contains(noPlugin, plugin) {
		attrs := map[string]string{"into": "kube-system"}
		if plugin == "cilium" {
			if on("cilium_enable_hubble") {
				attrs["hubble"] = "true"
			}
			if t := get("cilium_encryption_type"); on("cilium_encryption_enabled") && t != "" {
				attrs["encryption"] = t
			}
		}
		add(plugin, "kube_network_plugin", attrs)
	}
	if !replaced {
		add("kube-proxy", "kube_proxy_mode", map[string]string{"into": "kube-system", "mode": get("kube_proxy_mode")})
	}
	if coredns := strings.HasPrefix(get("dns_mode"), "coredns"); coredns {
		add("coredns", "dns_mode", map[string]string{"into": "kube-system"})
		if on("enable_nodelocaldns") { // kubespray deploys nodelocaldns only in front of its CoreDNS
			add("nodelocaldns", "enable_nodelocaldns", map[string]string{"into": "kube-system"})
		}
	}
	if on("metallb_enabled") {
		add("metallb", "metallb_enabled", map[string]string{"into": get("metallb_namespace")})
	}
	if on("kube_vip_enabled") {
		attrs := map[string]string{"into": "kube-system"}
		if on("kube_vip_controlplane_enabled") {
			attrs["role"] = "control-plane"
		}
		add("kube-vip", "kube_vip_enabled", attrs)
	}
	return nil
}

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

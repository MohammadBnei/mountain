package extract

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// The cluster's own software that no ArgoCD Application deploys (plan/13 §7 A7b): kubespray's add-ons, from the
// inventory's group_vars, and Pigsty's data services, from pigsty.yml. Both read named fields only.

var kubesprayExtractor = Extractor{Name: "kubespray", Version: 1, Needs: []string{"infra-bootstrap"}, Run: kubespray}

const groupVars = "inventory/ukubi/group_vars/k8s_cluster/"

// kubesprayVars are the only group_vars keys atlas reads: which add-ons run, never an address or a secret.
type kubesprayVars struct {
	NetworkPlugin      string `yaml:"kube_network_plugin"`
	ProxyMode          string `yaml:"kube_proxy_mode"`
	DNSMode            string `yaml:"dns_mode"`
	NodeLocalDNS       *bool  `yaml:"enable_nodelocaldns"`
	MetalLB            *bool  `yaml:"metallb_enabled"`
	KubeVIP            *bool  `yaml:"kube_vip_enabled"`
	KubeVIPControl     *bool  `yaml:"kube_vip_controlplane_enabled"`
	CiliumProxyReplace any    `yaml:"cilium_kube_proxy_replacement"` // false, true, or "strict"/"partial" in older kubespray
	CiliumHubble       *bool  `yaml:"cilium_enable_hubble"`
	CiliumEncryption   *bool  `yaml:"cilium_encryption_enabled"`
	CiliumEncryptType  string `yaml:"cilium_encryption_type"`
}

func kubespray(x *Ctx) error {
	var v kubesprayVars
	srcs := map[string]graph.Source{}
	for _, f := range []string{"k8s-cluster.yml", "addons.yml", "k8s-net-cilium.yml"} {
		b, src, err := x.Read("infra-bootstrap", groupVars+f)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(b, &v); err != nil { // the three files share one key space, as in ansible
			return fmt.Errorf("%s%s: %w", groupVars, f, err)
		}
		srcs[f] = src
	}
	on := func(b *bool) bool { return b != nil && *b }
	add := func(name, file string, attrs map[string]string) {
		x.Node(graph.Node{ID: graph.ID(graph.KindAddon, name), Kind: graph.KindAddon, Attrs: attrs, Source: srcs[file]})
	}
	replaced := false
	switch r := v.CiliumProxyReplace.(type) {
	case bool:
		replaced = r
	case string:
		replaced = r == "true" || r == "strict"
	}
	if v.NetworkPlugin != "" {
		attrs := map[string]string{"namespace": "kube-system"}
		if v.NetworkPlugin == "cilium" {
			if on(v.CiliumHubble) {
				attrs["hubble"] = "true"
			}
			if on(v.CiliumEncryption) && v.CiliumEncryptType != "" {
				attrs["encryption"] = v.CiliumEncryptType
			}
		}
		add(v.NetworkPlugin, "k8s-cluster.yml", attrs)
	}
	if v.ProxyMode != "" && !(v.NetworkPlugin == "cilium" && replaced) {
		add("kube-proxy", "k8s-cluster.yml", map[string]string{"namespace": "kube-system", "mode": v.ProxyMode})
	}
	if v.DNSMode == "coredns" {
		add("coredns", "k8s-cluster.yml", map[string]string{"namespace": "kube-system"})
	}
	if on(v.NodeLocalDNS) {
		add("nodelocaldns", "k8s-cluster.yml", map[string]string{"namespace": "kube-system"})
	}
	if on(v.MetalLB) {
		add("metallb", "addons.yml", map[string]string{"namespace": "metallb-system"})
	}
	if on(v.KubeVIP) {
		attrs := map[string]string{"namespace": "kube-system"}
		if on(v.KubeVIPControl) {
			attrs["role"] = "control-plane"
		}
		add("kube-vip", "addons.yml", attrs)
	}
	return nil
}

var pigstyExtractor = Extractor{Name: "pigsty", Version: 1, Needs: []string{"infra-bootstrap"}, Run: pigsty}

const pigstyPath = "pigsty/pigsty.yml"

// pigstyGroup is an inventory group as atlas reads it: its members (host keys only: their vars carry pg_role, whose
// labels are unreconciled, and redis_instances) and the cluster names. pg_users, passwords and the rest are never
// read.
type pigstyGroup struct {
	Hosts map[string]yaml.Node `yaml:"hosts"`
	Vars  struct {
		PG    string `yaml:"pg_cluster"`
		Redis string `yaml:"redis_cluster"`
		Etcd  string `yaml:"etcd_cluster"`
	} `yaml:"vars"`
}

// pigsty adds a db node per Postgres and Redis cluster, and runs-on edges to the VMs they run on, joined by IP through
// the LAN records (pigsty keys hosts by IP). The Postgres cluster also runs on its etcd members (its DCS, the witness
// among them). An edge whose VM is not a node becomes drift at the end of the run (Ctx.Member).
func pigsty(x *Ctx) error {
	b, src, err := x.Read("infra-bootstrap", pigstyPath)
	if err != nil {
		return err
	}
	var inv struct {
		All struct {
			Children map[string]pigstyGroup `yaml:"children"`
		} `yaml:"all"`
	}
	if err := yaml.Unmarshal(b, &inv); err != nil {
		return fmt.Errorf("%s: %w", pigstyPath, err)
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
	type member struct{ as []string }
	clusters := map[string]map[string]*member{} // db id -> vm id -> what it runs there
	engine := map[string]string{}
	run := func(db, host, as string) {
		name, ok := byIP[host]
		if !ok {
			x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: db, Detail: "pigsty host " + host + " is not in " + lanPath, Source: src})
			return
		}
		vm := graph.ID(graph.KindVM, name)
		if clusters[db][vm] == nil {
			clusters[db][vm] = &member{}
		}
		if m := clusters[db][vm]; !slices.Contains(m.as, as) {
			m.as = append(m.as, as)
		}
	}
	var etcd []string
	for _, name := range sortedKeys(inv.All.Children) {
		gr := inv.All.Children[name]
		var db, as string
		switch {
		case gr.Vars.PG != "":
			db, as = graph.ID(graph.KindDB, gr.Vars.PG), "postgres"
		case gr.Vars.Redis != "":
			db, as = graph.ID(graph.KindDB, gr.Vars.Redis), "redis"
		case gr.Vars.Etcd != "":
			etcd = append(etcd, sortedKeys(gr.Hosts)...)
			continue
		default:
			continue
		}
		if clusters[db] == nil {
			clusters[db], engine[db] = map[string]*member{}, as
			x.Node(graph.Node{ID: db, Kind: graph.KindDB, Attrs: map[string]string{"engine": as}, Source: src})
		}
		for _, h := range sortedKeys(gr.Hosts) {
			run(db, h, as)
		}
	}
	for _, db := range sortedKeys(clusters) { // etcd is Patroni's DCS: every Postgres cluster runs on its members
		if engine[db] == "postgres" {
			for _, h := range etcd {
				run(db, h, "etcd")
			}
		}
	}
	for _, db := range sortedKeys(clusters) {
		for _, vm := range sortedKeys(clusters[db]) {
			x.Member(graph.Edge{From: db, To: vm, Kind: graph.EdgeRunsOn, Origin: graph.Declared,
				Attrs: map[string]string{"as": strings.Join(clusters[db][vm].as, ",")}, Source: src})
		}
	}
	return nil
}

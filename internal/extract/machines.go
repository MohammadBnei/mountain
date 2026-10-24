package extract

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// machinesExtractor reads the Terraform topology: the Proxmox hosts, the VMs and containers terraform declares, and
// the k8s nodes from the inventory terraform renders (its k8s_nodes map lives in the uncommitted tfvars).
var machinesExtractor = Extractor{Name: "machines", Version: 1, Needs: []string{"infra-bootstrap"}, Run: machines}

const (
	inventoryPath = "inventory/ukubi/hosts.yaml"
	lanPath       = "ansible/playbooks/vars/bnei-lan-records.yml"
	// hiddenMachine is never drawn (the break-glass box, plan/00 Machines row).
	hiddenMachine = "k9s-dashboard"
)

func machines(x *Ctx) error {
	b, lanSrc, err := x.Read("infra-bootstrap", lanPath)
	if err != nil {
		return err
	}
	var lan struct {
		Hosts map[string]string `yaml:"bnei_lan_hosts"`
	}
	if err := yaml.Unmarshal(b, &lan); err != nil {
		return fmt.Errorf("%s: %w", lanPath, err)
	}
	ips := lan.Hosts

	for _, h := range sortedKeys(x.Realms.Hosts) {
		x.Node(graph.Node{ID: graph.ID(graph.KindHost, h), Kind: graph.KindHost, Source: lanSrc,
			Attrs: withIP(map[string]string{"island": x.Realms.Hosts[h]}, ips[h])})
	}
	if err := terraform(x, ips); err != nil {
		return err
	}
	return inventory(x, ips)
}

func withIP(m map[string]string, ip string) map[string]string {
	if ip != "" {
		m["ip"] = ip
	}
	return m
}

// guestTypes are the terraform resources that are machines.
var guestTypes = map[string]string{
	"proxmox_virtual_environment_vm":        graph.KindVM,
	"proxmox_virtual_environment_container": graph.KindCT,
}

func terraform(x *Ctx, ips map[string]string) error {
	for _, p := range x.Glob("infra-bootstrap", "terraform/*.tf") {
		b, src, err := x.Read("infra-bootstrap", p)
		if err != nil {
			return err
		}
		f, diags := hclsyntax.ParseConfig(b, p, hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("%s: %s", p, diags.Error())
		}
		for _, blk := range f.Body.(*hclsyntax.Body).Blocks {
			if blk.Type != "resource" || len(blk.Labels) != 2 {
				continue
			}
			kind, ok := guestTypes[blk.Labels[0]]
			if !ok {
				continue
			}
			attrs := blk.Body.Attributes
			if _, each := attrs["for_each"]; each {
				continue // the k8s nodes: read from the inventory instead
			}
			if t, ok := literal(attrs["template"]); ok && t == "true" {
				continue // a VM template is not a machine
			}
			name, ok := literal(attrs["name"])
			if kind == graph.KindCT {
				name, ok = "", false
				for _, nb := range blk.Body.Blocks {
					if nb.Type == "initialization" {
						name, ok = literal(nb.Body.Attributes["hostname"])
					}
				}
			}
			subject := blk.Labels[0] + "." + blk.Labels[1]
			if !ok || name == "" {
				x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: subject, Detail: "no literal name or hostname", Source: src})
				continue
			}
			if name == hiddenMachine {
				continue
			}
			id := graph.ID(kind, name)
			x.Node(graph.Node{ID: id, Kind: kind, Source: src, Attrs: withIP(map[string]string{"resource": subject}, ips[name])})
			host, ok := hostOf(x, attrs["node_name"])
			if !ok {
				x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: id, Detail: "node_name is not a literal or var.pve_node_name", Source: src})
				continue
			}
			runsOn(x, id, host, src)
		}
	}
	return nil
}

// hostOf resolves a node_name: a literal host, or var.pve_node_name through realms.yaml.
func hostOf(x *Ctx, a *hclsyntax.Attribute) (string, bool) {
	if a == nil {
		return "", false
	}
	if v, ok := literal(a); ok {
		return v, true
	}
	if t, ok := a.Expr.(*hclsyntax.ScopeTraversalExpr); ok && len(t.Traversal) == 2 && t.Traversal.RootName() == "var" {
		if ga, ok := t.Traversal[1].(hcl.TraverseAttr); ok && ga.Name == "pve_node_name" {
			return x.Realms.PVENodeName, true
		}
	}
	return "", false
}

func runsOn(x *Ctx, id, host string, src graph.Source) {
	if _, ok := x.Realms.Hosts[host]; !ok {
		x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: id, Detail: "runs on " + host + ", not a realms.yaml host", Source: src})
		return
	}
	x.Edge(graph.Edge{From: id, To: graph.ID(graph.KindHost, host), Kind: graph.EdgeRunsOn, Origin: graph.Declared, Source: src})
}

// literal is an attribute's value when it is a plain string, number or bool (no variables, no functions).
func literal(a *hclsyntax.Attribute) (string, bool) {
	if a == nil {
		return "", false
	}
	switch e := a.Expr.(type) {
	case *hclsyntax.TemplateExpr:
		if len(e.Parts) == 1 {
			if l, ok := e.Parts[0].(*hclsyntax.LiteralValueExpr); ok && l.Val.Type().FriendlyName() == "string" {
				return l.Val.AsString(), true
			}
		}
	case *hclsyntax.LiteralValueExpr:
		switch e.Val.Type().FriendlyName() {
		case "string":
			return e.Val.AsString(), true
		case "bool":
			return fmt.Sprint(e.Val.True()), true
		case "number":
			return e.Val.AsBigFloat().String(), true
		}
	}
	return "", false
}

// inventory adds the k8s nodes: their roles from the inventory groups, their host from realms.yaml.
func inventory(x *Ctx, ips map[string]string) error {
	b, src, err := x.Read("infra-bootstrap", inventoryPath)
	if err != nil {
		return err
	}
	var inv struct {
		All struct {
			Hosts    map[string]struct{} `yaml:"hosts"`
			Children map[string]struct {
				Hosts map[string]struct{} `yaml:"hosts"`
			} `yaml:"children"`
		} `yaml:"all"`
	}
	if err := yaml.Unmarshal(b, &inv); err != nil {
		return fmt.Errorf("%s: %w", inventoryPath, err)
	}
	roleOf := map[string]string{"kube_control_plane": "control-plane", "kube_node": "worker", "etcd": "etcd"}
	for _, name := range sortedKeys(inv.All.Hosts) {
		var roles []string
		for _, g := range sortedKeys(roleOf) {
			if _, in := inv.All.Children[g].Hosts[name]; in {
				roles = append(roles, roleOf[g])
			}
		}
		slices.Sort(roles)
		id := graph.ID(graph.KindVM, name)
		x.Node(graph.Node{ID: id, Kind: graph.KindVM, Source: src,
			Attrs: withIP(map[string]string{"k8s": strings.Join(roles, ",")}, ips[name])})
		host, ok := x.Realms.K8sPlacement[name]
		if !ok {
			x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: id, Detail: "k8s node with no host in realms.yaml k8sPlacement", Source: src})
			continue
		}
		runsOn(x, id, host, src)
	}
	return nil
}

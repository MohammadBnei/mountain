// Package realms loads realms.yaml, atlas's one curated file: which realm and group each component belongs to, the
// Proxmox hosts, and the facts the repos don't commit (terraform's tfvars).
package realms

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed realms.yaml
var embedded []byte

type Component struct {
	Realm    string   `yaml:"realm"`
	Group    string   `yaml:"group"`
	Function string   `yaml:"function"` // overrides its group's function
	Parent   string   `yaml:"parent"`
	Paths    []string `yaml:"paths"` // "<repo>/<prefix>": ADR citations under it govern this component
}

type Realms struct {
	Realms       []string             `yaml:"realms"`
	Hosts        map[string]string    `yaml:"hosts"` // Proxmox host -> island label
	PVENodeName  string               `yaml:"pveNodeName"`
	K8sPlacement map[string]string    `yaml:"k8sPlacement"` // k8s VM -> Proxmox host
	ADRHome      map[string]string    `yaml:"adrHome"`      // repo without ADRs -> repo its bare citations mean
	ShortADR     map[string]string    `yaml:"shortADR"`     // repo -> repo its bare "adr/NNNN" means (else its ADR home)
	Skip         map[string][]string  `yaml:"skip"`         // repo -> repo-relative dirs whose files are not the repo's words
	Sources      []string             `yaml:"sources"`      // repos atlas fetches and reads, infra-bootstrap first
	Probes       map[string]string    `yaml:"probes"`       // blackbox target -> node id (pve-<host> is implicit)
	Namespaces   map[string]string    `yaml:"namespaces"`   // namespace without an Application -> realm
	Components   map[string]Component `yaml:"components"`
	Functions    map[string]string    `yaml:"functions"` // "<realm>/<group>" -> what the group does in play
}

// Code is the realm atlas places the repos in itself (extract's place), by the realms of the apps they feed.
const Code = "code"

// The functions a building has in play (plan/13 §1). Land is the hardware: drawn in atlas, never a building.
const (
	FnLand       = "land"
	FnTownCentre = "town-centre"
	FnHouses     = "houses"
	FnWorkshop   = "workshop"
	FnEconomy    = "economy"
	FnUtility    = "utility"
	FnDefence    = "defence"
	FnTrade      = "trade"
	FnBarracks   = "barracks"
	FnProduct    = "product"
)

var functions = []string{FnLand, FnTownCentre, FnHouses, FnWorkshop, FnEconomy, FnUtility, FnDefence, FnTrade, FnBarracks, FnProduct}

// Function is what component id does in play: its own function, else its group's, or "" if it isn't listed.
func (r *Realms) Function(id string) string {
	c, ok := r.Components[id]
	if !ok {
		return ""
	}
	if c.Function != "" {
		return c.Function
	}
	return r.Functions[c.Realm+"/"+c.Group]
}

// ProbeNode is the node a blackbox target probes, or "".
func (r *Realms) ProbeNode(target string) string {
	if h, ok := strings.CutPrefix(target, "pve-"); ok {
		if _, known := r.Hosts[h]; known {
			return "host:" + h
		}
		return ""
	}
	return r.Probes[target]
}

// Load parses the embedded realms.yaml.
func Load() (*Realms, error) { return Parse(embedded) }

func Parse(b []byte) (*Realms, error) {
	var r Realms
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("realms.yaml: %w", err)
	}
	if !slices.Contains(r.Realms, Code) {
		return nil, fmt.Errorf("realms.yaml: realms must include %q (atlas places the repos there)", Code)
	}
	for key, fn := range r.Functions {
		realm, group, _ := strings.Cut(key, "/")
		if !slices.Contains(r.Realms, realm) || group == "" || realm == Code {
			return nil, fmt.Errorf("realms.yaml: functions key %q is not <realm>/<group> of a listed realm (code's are atlas's)", key)
		}
		if !slices.Contains(functions, fn) {
			return nil, fmt.Errorf("realms.yaml: functions %s is %q, not one of %v", key, fn, functions)
		}
	}
	for id, c := range r.Components {
		if c.Realm == Code {
			return nil, fmt.Errorf("realms.yaml: %s is in realm %q, which only atlas fills (the repos)", id, Code)
		}
		if !slices.Contains(r.Realms, c.Realm) {
			return nil, fmt.Errorf("realms.yaml: %s has realm %q, not one of %v", id, c.Realm, r.Realms)
		}
		if c.Group == "" {
			return nil, fmt.Errorf("realms.yaml: %s has no group", id)
		}
		if c.Function != "" && !slices.Contains(functions, c.Function) {
			return nil, fmt.Errorf("realms.yaml: %s has function %q, not one of %v", id, c.Function, functions)
		}
		if c.Function == "" && r.Functions[c.Realm+"/"+c.Group] == "" {
			return nil, fmt.Errorf("realms.yaml: %s: group %s/%s has no function (functions:), and it sets none", id, c.Realm, c.Group)
		}
		for _, p := range c.Paths {
			if !strings.Contains(p, "/") {
				return nil, fmt.Errorf("realms.yaml: %s path %q is not <repo>/<prefix>", id, p)
			}
		}
	}
	for vm, h := range r.K8sPlacement {
		if _, ok := r.Hosts[h]; !ok {
			return nil, fmt.Errorf("realms.yaml: k8s node %s is on %q, not a listed host", vm, h)
		}
	}
	if len(r.Sources) == 0 || r.Sources[0] != "infra-bootstrap" {
		return nil, fmt.Errorf("realms.yaml: sources must start with infra-bootstrap (its registry names the branches)")
	}
	for target, id := range r.Probes {
		if _, ok := r.Components[id]; !ok {
			return nil, fmt.Errorf("realms.yaml: probe %s -> %s is not a listed component", target, id)
		}
	}
	for ns, realm := range r.Namespaces {
		if !slices.Contains(r.Realms, realm) {
			return nil, fmt.Errorf("realms.yaml: namespace %s has realm %q, not one of %v", ns, realm, r.Realms)
		}
	}
	if _, ok := r.Hosts[r.PVENodeName]; !ok {
		return nil, fmt.Errorf("realms.yaml: pveNodeName %q is not a listed host", r.PVENodeName)
	}
	return &r, nil
}

// Owner is the component a repo-relative file belongs to by the longest matching path prefix, or "" (its repo).
func (r *Realms) Owner(repo, path string) string {
	full := repo + "/" + path
	best, n := "", 0
	for id, c := range r.Components {
		for _, p := range c.Paths {
			if strings.HasPrefix(full, p) && len(p) > n {
				best, n = id, len(p)
			}
		}
	}
	return best
}

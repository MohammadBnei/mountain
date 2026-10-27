// Package realms loads realms.yaml, atlas's one curated file: which realm and group each component belongs to, the
// Proxmox hosts, and the facts the repos don't commit (terraform's tfvars).
package realms

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
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
	Watch        map[string]Watch     `yaml:"watch"`     // node -> the alert rules that name it (its watchtowers)
	General      Watch                `yaml:"general"`   // the rules that watch every building at once
	Needs        map[string][]string  `yaml:"needs"`     // node -> what it depends on, each with its evidence (Need)
	Addons       map[string][]string  `yaml:"addons"`    // addon node -> its workloads: daemonset/<n>, deployment/<n>, static
	Walls        map[string][]string  `yaml:"walls"`     // door (dns:<host>) -> what the sources can't show of its gate (WallKinds)
	Shapes       map[string][]string  `yaml:"shapes"`    // "<realm>/<group>" -> the kit shapes its buildings ask for (ukubi-kit catalog/shapes.yaml)

	// Digest is the first 32 bits of the file's sha256: a projection records it, since atlas reads realms.yaml from
	// its binary, not from the mountain checkout it projects.
	Digest int `yaml:"-"`
}

// WallKinds are what a walls row may say of a door: a mark (public: meant for everyone; own: the app authenticates
// its own callers), or a gate declared because it lives in a repo atlas does not read (forwardauth: the whole host;
// forwardauth-path: a part of it; oidc: its login goes through authentik).
var WallKinds = []string{"public", "own", "forwardauth", "forwardauth-path", "oidc"}

// Watch is the alert rules on a node, by where they are evaluated: prometheus (an alerting rule: it reaches
// Alertmanager and ALERTS, so atlas and the sim see it fire) or grafana (Grafana's own rules: they notify, but nothing
// atlas reads sees them fire). Names are the rules' alert names (Prometheus) or uids (Grafana).
type Watch struct {
	Prometheus []string `yaml:"prometheus"`
	Grafana    []string `yaml:"grafana"`
}

// Suppliers are the node each kind of evidence names: a need written as its evidence kind depends on that node.
var Suppliers = map[string]string{
	"oidc":        "app:platform-authentik", // its OIDC client's redirect URL is one of the app's hostnames
	"forwardauth": "app:platform-authentik", // its ingress goes through an authentik forwardAuth middleware
	"infisical":   "app:platform-infisical", // its values have an infisical: block (secrets synced at run time)
	"postgres":    "db:pg-proxmox",          // postgres:<db>: that database is in Pigsty's pg_databases
	"s3":          "ct:garage-storage",      // its values name Garage's S3 endpoint
}

// Need is one row of needs: "<kind>" or "postgres:<db>" (checked against the sources), or "declared:<node>" (the
// evidence lives outside the sources atlas reads: shown, unverified).
type Need struct {
	On, Via, Arg string // the supplier node, the evidence kind, its argument (the database)
}

// ParseNeed reads one needs entry.
func ParseNeed(s string) (Need, error) {
	kind, arg, _ := strings.Cut(s, ":")
	switch {
	case kind == "declared" && arg != "":
		return Need{On: arg, Via: kind}, nil
	case kind == "postgres" && arg != "":
		return Need{On: Suppliers[kind], Via: kind, Arg: arg}, nil
	case Suppliers[kind] != "" && arg == "" && kind != "postgres":
		return Need{On: Suppliers[kind], Via: kind}, nil
	}
	return Need{}, fmt.Errorf("need %q is not oidc, forwardauth, infisical, s3, postgres:<db> or declared:<node>", s)
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

// ComponentKinds are the node kinds realms.yaml may list and atlas must place (extract's realmKinds); repos are
// atlas's own, hostnames and ADRs are never placed.
var ComponentKinds = []string{"host", "vm", "ct", "app", "svc", "addon", "db"}

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
	sum := sha256.Sum256(b)
	r.Digest = int(binary.BigEndian.Uint32(sum[:4]))
	if !slices.Contains(r.Realms, Code) {
		return nil, fmt.Errorf("realms.yaml: realms must include %q (atlas places the repos there)", Code)
	}
	for key, fn := range r.Functions {
		realm, group, _ := strings.Cut(key, "/")
		if !slices.Contains(r.Realms, realm) || group == "" || realm == Code {
			return nil, fmt.Errorf("realms.yaml: functions key %q is not <realm>/<group> of a listed realm (code's are atlas's)", key)
		}
		if !slices.Contains(functions, fn) || fn == FnWorkshop {
			return nil, fmt.Errorf("realms.yaml: functions %s is %q, not one of %v (workshop is the repos', set by atlas)", key, fn, functions)
		}
	}
	for id, c := range r.Components {
		if kind, _, _ := strings.Cut(id, ":"); !slices.Contains(ComponentKinds, kind) {
			return nil, fmt.Errorf("realms.yaml: %s is not a component (%v); atlas places repos itself", id, ComponentKinds)
		}
		if c.Realm == Code {
			return nil, fmt.Errorf("realms.yaml: %s is in realm %q, which only atlas fills (the repos)", id, Code)
		}
		if !slices.Contains(r.Realms, c.Realm) {
			return nil, fmt.Errorf("realms.yaml: %s has realm %q, not one of %v", id, c.Realm, r.Realms)
		}
		if c.Group == "" {
			return nil, fmt.Errorf("realms.yaml: %s has no group", id)
		}
		if c.Function != "" && (!slices.Contains(functions, c.Function) || c.Function == FnWorkshop) {
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
	for id, w := range r.Watch {
		if _, ok := r.Components[id]; !ok {
			return nil, fmt.Errorf("realms.yaml: watch %s is not a listed component", id)
		}
		if len(w.Prometheus)+len(w.Grafana) == 0 {
			return nil, fmt.Errorf("realms.yaml: watch %s names no rule", id)
		}
	}
	for id, list := range r.Needs {
		if _, ok := r.Components[id]; !ok {
			return nil, fmt.Errorf("realms.yaml: needs %s is not a listed component", id)
		}
		for _, s := range list {
			n, err := ParseNeed(s)
			if err != nil {
				return nil, fmt.Errorf("realms.yaml: needs %s: %w", id, err)
			}
			if _, ok := r.Components[n.On]; !ok || n.On == id {
				return nil, fmt.Errorf("realms.yaml: needs %s: %s is not another listed component", id, n.On)
			}
		}
	}
	for g, list := range r.Shapes {
		if _, ok := r.Functions[g]; !ok {
			return nil, fmt.Errorf("realms.yaml: shapes %s is not a <realm>/<group> with a function", g)
		}
		for _, s := range list {
			if s == "" || strings.Trim(s, "abcdefghijklmnopqrstuvwxyz") != "" {
				return nil, fmt.Errorf("realms.yaml: shapes %s: %q is not a shape word", g, s)
			}
		}
	}
	for id, list := range r.Walls {
		if !strings.HasPrefix(id, "dns:") {
			return nil, fmt.Errorf("realms.yaml: walls %s is not a door (dns:<host>)", id)
		}
		for _, k := range list {
			if !slices.Contains(WallKinds, k) {
				return nil, fmt.Errorf("realms.yaml: walls %s: %q is not one of %v", id, k, WallKinds)
			}
		}
		if slices.Contains(list, "public") && slices.Contains(list, "own") {
			return nil, fmt.Errorf("realms.yaml: walls %s: public and own both", id)
		}
	}
	for id, wl := range r.Addons {
		if kind, _, _ := strings.Cut(id, ":"); kind != "addon" {
			return nil, fmt.Errorf("realms.yaml: addons %s is not an addon node", id)
		}
		for _, w := range wl {
			kind, name, _ := strings.Cut(w, "/")
			if !(w == "static" || (kind == "daemonset" || kind == "deployment") && name != "") {
				return nil, fmt.Errorf("realms.yaml: addons %s: %q is not daemonset/<name>, deployment/<name> or static", id, w)
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
		if !slices.Contains(r.Realms, realm) || realm == Code {
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

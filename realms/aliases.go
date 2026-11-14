package realms

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/MohammadBnei/mountain/internal/graph"
	"gopkg.in/yaml.v3"
)

//go:embed aliases.yaml
var aliasesYAML []byte

// Alias is one name a source gives a thing (aliases.yaml's header says what each field means).
type Alias struct {
	Source string `yaml:"source" json:"source"`
	Field  string `yaml:"field" json:"field"`
	Name   string `yaml:"name" json:"name"`
	From   string `yaml:"from" json:"-"` // "probes": Name is filled from realms.yaml's probes
	Status string `yaml:"status" json:"status"`
	Key    string `yaml:"key" json:"key"`
	At     string `yaml:"at" json:"at,omitempty"`
	Note   string `yaml:"note" json:"note,omitempty"`
}

// The statuses of an alias.
const (
	Proven     = "proven"
	Declared   = "declared"
	Historical = "historical"
)

// Thing is one canonical id's row: its relations and its names.
type Thing struct {
	PartOf  string   `yaml:"part_of"` // a member's cluster
	RunsOn  string   `yaml:"runs_on"` // a member's VM (a k8s VM's host is realms.yaml's k8sPlacement)
	Fronts  string   `yaml:"fronts"`  // what an endpoint stands in front of
	VIPs    []string `yaml:"vips"`    // an endpoint's addresses, which nothing else may take as an alias
	Aliases []Alias  `yaml:"aliases"`
}

// Aliases is aliases.yaml, indexed by name.
type Aliases struct {
	Things map[string]Thing `yaml:"things"`
	Digest int              `yaml:"-"` // the first 32 bits of the file's sha256, as Realms.Digest

	rl     *Realms
	byName map[string][]Match // lower-cased name -> every row that gives it
}

// Match is one alias row a name hit, with the id it names.
type Match struct {
	ID string `json:"id"`
	Alias
}

// LoadAliases parses the embedded aliases.yaml against realms.yaml and the source registry.
func LoadAliases(rl *Realms, reg *Registry) (*Aliases, error) {
	return ParseAliases(aliasesYAML, rl, reg)
}

// ParseAliases reads an alias table and runs its checks (aliases.yaml's header): the first failure is the error.
func ParseAliases(b []byte, rl *Realms, reg *Registry) (*Aliases, error) {
	today := time.Now().UTC().Format(time.DateOnly)
	var a Aliases
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&a); err != nil {
		return nil, fmt.Errorf("aliases.yaml: %w", err)
	}
	sum := sha256.Sum256(b)
	a.Digest = int(binary.BigEndian.Uint32(sum[:4]))
	a.rl = rl
	a.byName = map[string][]Match{}
	vips := map[string]string{} // VIP -> its endpoint
	for _, id := range slices.Sorted(maps.Keys(a.Things)) {
		for _, v := range a.Things[id].VIPs {
			if !strings.HasPrefix(id, graph.KindEndpoint+":") {
				return nil, fmt.Errorf("aliases.yaml: %s has vips, but only an endpoint has addresses of its own", id)
			}
			if o, ok := vips[vipHost(v)]; ok {
				return nil, fmt.Errorf("aliases.yaml: %s and %s both claim the VIP %s", o, id, v)
			}
			vips[vipHost(v)] = id
		}
	}
	owner := map[string]string{} // source + "\x00" + lower name -> id
	for _, id := range slices.Sorted(maps.Keys(a.Things)) {
		t := a.Things[id]
		if err := a.checkThing(id, t); err != nil {
			return nil, err
		}
		for i, al := range t.Aliases {
			if err := checkAlias(id, al, reg, today); err != nil {
				return nil, err
			}
			names := []string{al.Name}
			if al.From == "probes" {
				if names = probeTargets(rl, id); len(names) == 0 {
					return nil, fmt.Errorf("aliases.yaml: %s alias %d: from probes, but realms.yaml probes no target to it", id, i)
				}
			}
			for _, n := range names {
				m := Match{ID: id, Alias: al}
				m.Name = n
				k := strings.ToLower(n)
				if o, ok := owner[al.Source+"\x00"+k]; ok && o != id {
					return nil, fmt.Errorf("aliases.yaml: %s calls %q both %s and %s: one name is one id per source", al.Source, n, o, id)
				}
				owner[al.Source+"\x00"+k] = id
				if e, ok := vipOf(vips, n); ok && !strings.HasPrefix(id, graph.KindEndpoint+":") {
					return nil, fmt.Errorf("aliases.yaml: %s has alias %q, which is %s's VIP: a VIP is an endpoint, never an alias", id, n, e)
				}
				a.byName[k] = append(a.byName[k], m)
			}
		}
	}
	return &a, nil
}

// checkThing: the id is valid, a host, vm, ct or db is a listed component, the relations name things or components,
// and a k8s VM's host is left to k8sPlacement.
func (a *Aliases) checkThing(id string, t Thing) error {
	if !graph.Valid(id) {
		return fmt.Errorf("aliases.yaml: %q is not a valid id (internal/graph)", id)
	}
	kind, name, _ := strings.Cut(id, ":")
	if _, ok := a.rl.Components[id]; !ok && slices.Contains([]string{graph.KindHost, graph.KindVM, graph.KindCT, graph.KindDB}, kind) {
		return fmt.Errorf("aliases.yaml: %s is not a listed component of realms.yaml", id)
	}
	for _, rel := range []string{t.PartOf, t.RunsOn, t.Fronts} {
		_, thing := a.Things[rel]
		_, comp := a.rl.Components[rel]
		if rel != "" && !thing && !comp {
			return fmt.Errorf("aliases.yaml: %s names %s, which is neither a thing here nor a component of realms.yaml", id, rel)
		}
	}
	if _, placed := a.rl.K8sPlacement[name]; placed && kind == graph.KindVM && t.RunsOn != "" {
		return fmt.Errorf("aliases.yaml: %s has runs_on, but realms.yaml's k8sPlacement places it (one head per fact)", id)
	}
	return nil
}

// checkAlias: a known source, a field and a key, a name or from probes, a known status, and a date on a join (on a
// historical row, a date if it has one), never after today.
func checkAlias(id string, al Alias, reg *Registry, today string) error {
	switch {
	case reg.Get(al.Source) == nil:
		return fmt.Errorf("aliases.yaml: %s: source %q is not in sources.yaml", id, al.Source)
	case al.Field == "" || al.Key == "":
		return fmt.Errorf("aliases.yaml: %s %q: an alias needs its field and its join key", id, al.Name)
	case (al.Name == "") == (al.From == "") || (al.From != "" && al.From != "probes"):
		return fmt.Errorf("aliases.yaml: %s: an alias has a name or from: probes, not both or neither", id)
	case strings.TrimSpace(al.Name) != al.Name:
		return fmt.Errorf("aliases.yaml: %s: name %q has spaces around it", id, al.Name)
	case !slices.Contains([]string{Proven, Declared, Historical}, al.Status):
		return fmt.Errorf("aliases.yaml: %s %q: status %q is not proven, declared or historical", id, al.Name, al.Status)
	}
	if al.Status == Historical && al.At == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, al.At); err != nil {
		return fmt.Errorf("aliases.yaml: %s %q: a %s row's date is at: YYYY-MM-DD (required on a join), got %q", id, al.Name, al.Status, al.At)
	}
	if al.At > today { // both YYYY-MM-DD: the string order is the date order
		return fmt.Errorf("aliases.yaml: %s %q: at %s is after today (%s)", id, al.Name, al.At, today)
	}
	return nil
}

// vipHost is an address in its bare form: no scheme, path or CIDR suffix, no port. ponytail: IPv4 only (an IPv6
// address would lose its tail at the first colon); the LAN has none.
func vipHost(s string) string {
	if _, rest, ok := strings.Cut(s, "://"); ok {
		s = rest
	}
	s, _, _ = strings.Cut(s, "/")
	s, _, _ = strings.Cut(s, ":")
	return s
}

// vipOf reports whether a name is a VIP in any of its forms (vipHost).
func vipOf(vips map[string]string, name string) (string, bool) {
	e, ok := vips[vipHost(name)]
	return e, ok
}

// probeTargets are the blackbox targets realms.yaml gives id (its probes:, or pve-<host> for a listed host), sorted.
func probeTargets(rl *Realms, id string) []string {
	var out []string
	for _, t := range slices.Sorted(maps.Keys(rl.Probes)) {
		if rl.Probes[t] == id {
			out = append(out, t)
		}
	}
	if h, ok := strings.CutPrefix(id, graph.KindHost+":"); ok {
		if _, listed := rl.Hosts[h]; listed {
			out = append(out, "pve-"+h)
		}
	}
	return out
}

// Resolve is every row that gives name, by id then source: an id itself (as atlas's own name), or an alias. A name
// written field=value (ins=etcd-1) matches only rows of that field. More than one id means the name is ambiguous:
// the caller shows them all, never one.
func (a *Aliases) Resolve(name string) []Match {
	name = strings.TrimSpace(name)
	field, value, scoped := strings.Cut(name, "=")
	if !scoped {
		value = name
	}
	k := strings.ToLower(strings.TrimSpace(value))
	var out []Match
	if _, ok := a.Things[k]; ok && !scoped {
		out = append(out, Match{ID: k, Alias: Alias{Source: "atlas", Field: "id", Name: k, Status: Proven, Key: "id"}})
	}
	for _, m := range a.byName[k] {
		if !scoped || strings.EqualFold(m.Field, strings.TrimSpace(field)) {
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(x, y Match) int {
		return cmp.Or(cmp.Compare(x.ID, y.ID), cmp.Compare(x.Source, y.Source), cmp.Compare(x.Field, y.Field))
	})
	return out
}

// Names are the names a source gives id now, for one field ("" = any), in the file's order: historical rows are
// names the thing no longer has, so a live ask never uses them.
func (a *Aliases) Names(id, source, field string) []string {
	var out []string
	for _, al := range a.Things[id].Aliases {
		if al.Source == source && (field == "" || al.Field == field) && al.Status != Historical {
			out = append(out, a.NamesOf(id, al)...)
		}
	}
	return out
}

// NamesOf is the names one alias row of id stands for: its name, or the probe targets of a from: probes row.
func (a *Aliases) NamesOf(id string, al Alias) []string {
	if al.From == "probes" {
		return probeTargets(a.rl, id)
	}
	return []string{al.Name}
}

// RunsOn is where id runs: a member's runs_on, or a k8s VM's host from realms.yaml's k8sPlacement; "" = not known
// here.
func (a *Aliases) RunsOn(id string) string {
	if r := a.Things[id].RunsOn; r != "" {
		return r
	}
	if n, ok := strings.CutPrefix(id, graph.KindVM+":"); ok && a.rl.K8sPlacement[n] != "" {
		return graph.ID(graph.KindHost, a.rl.K8sPlacement[n])
	}
	return ""
}

// Members are the things whose part_of is id, sorted.
func (a *Aliases) Members(id string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(a.Things)) {
		if a.Things[k].PartOf == id {
			out = append(out, k)
		}
	}
	return out
}

// On are the things whose runs_on is id, sorted.
func (a *Aliases) On(id string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(a.Things)) {
		if a.Things[k].RunsOn == id {
			out = append(out, k)
		}
	}
	return out
}

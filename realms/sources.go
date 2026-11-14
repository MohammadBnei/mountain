package realms

import (
	"bytes"
	_ "embed"
	"fmt"
	"regexp"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed sources.yaml
var sourcesYAML []byte

// Source is one row of sources.yaml: a source an agent can read (design-2026-11/door-hybrid-F.md stage 1).
type Source struct {
	ID             string   `yaml:"id"`
	Kind           string   `yaml:"kind"`
	Reach          Reach    `yaml:"reach"`
	Auth           string   `yaml:"auth"`
	Knows          []string `yaml:"knows"`
	NotKnown       []string `yaml:"not_known"`
	Freshness      string   `yaml:"freshness"`       // how often the source itself updates, as prose
	FreshnessLimit string   `yaml:"freshness_limit"` // past it a fact is unknown (a Go duration; "" = not read by atlas)
	Retention      string   `yaml:"retention"`
	ConfigRef      []string `yaml:"config_ref"`
	Notes          string   `yaml:"notes"`
	Verified       string   `yaml:"verified"`

	Limit time.Duration `yaml:"-"` // FreshnessLimit, parsed
}

// Reach is how a source is reached from the owner's laptop and from inside the cluster.
type Reach struct {
	Laptop  string `yaml:"laptop"`
	Cluster string `yaml:"cluster"`
}

// SourceKinds are what a source may be.
var SourceKinds = []string{"metrics", "alerts", "logs", "state", "events", "probe", "flows", "declared", "snapshot", "activity", "derived"}

// Registry is sources.yaml, by id.
type Registry struct {
	Sources []Source `yaml:"sources"`
	byID    map[string]*Source
}

// Get is the source with that id, or nil.
func (r *Registry) Get(id string) *Source { return r.byID[id] }

// LoadSources parses the embedded sources.yaml.
func LoadSources() (*Registry, error) { return ParseSources(sourcesYAML) }

// credential is what a secret's value looks like in a URL or a key = value: the registry says what a source needs,
// never the value. ponytail: a pattern, not a scanner; a secret written as plain prose passes.
var credential = regexp.MustCompile(`://[^/\s:@]+:[^/\s@]+@|(?i)(token|password|secret|apikey)\s*[=:]\s*["']?[A-Za-z0-9+/_-]{12,}`)

// ParseSources reads a registry: every field known, ids unique, kinds known, limits durations, no credential.
func ParseSources(b []byte) (*Registry, error) {
	if m := credential.Find(b); m != nil {
		return nil, fmt.Errorf("sources.yaml: %q looks like a credential: the registry holds none", m)
	}
	var r Registry
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&r); err != nil {
		return nil, fmt.Errorf("sources.yaml: %w", err)
	}
	r.byID = map[string]*Source{}
	for i := range r.Sources {
		s := &r.Sources[i]
		if s.ID == "" || r.byID[s.ID] != nil {
			return nil, fmt.Errorf("sources.yaml: source %d: id %q is empty or listed twice", i, s.ID)
		}
		if !slices.Contains(SourceKinds, s.Kind) {
			return nil, fmt.Errorf("sources.yaml: %s: kind %q is not one of %v", s.ID, s.Kind, SourceKinds)
		}
		if s.FreshnessLimit != "" {
			l, err := time.ParseDuration(s.FreshnessLimit)
			if err != nil || l <= 0 {
				return nil, fmt.Errorf("sources.yaml: %s: freshness_limit %q is not a positive duration", s.ID, s.FreshnessLimit)
			}
			s.Limit = l
		}
		r.byID[s.ID] = s
	}
	return &r, nil
}

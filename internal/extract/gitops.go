package extract

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// gitopsExtractor reads infra-bootstrap's ArgoCD layer: the app registry, the three list-generator
// ApplicationSets and the standalone Applications. Every Application becomes an app node named as ArgoCD names it
// (so A4's live overlay joins on argocd_app_info's name), with git edges to the repos it deploys from.
var gitopsExtractor = Extractor{Name: "gitops", Version: 1, Needs: []string{"infra-bootstrap"}, Run: gitops}

const (
	registryPath = "gitops/apps/registry.yaml"
	appsetDir    = "gitops/bootstrap"
)

// element is a list-generator element or a registry entry: the fields atlas reads.
type element struct {
	Name           string `yaml:"name"`
	Namespace      string `yaml:"namespace"`
	RepoURL        string `yaml:"repoURL"`
	TargetRevision string `yaml:"targetRevision"`
	ValuesPath     paths  `yaml:"valuesPath"`
	Hostname       string `yaml:"hostname"`
	ChartRepoURL   string `yaml:"chartRepoURL"`
	ChartName      string `yaml:"chartName"`
	ChartRevision  string `yaml:"chartRevision"`
}

// paths is a valuesPath: one path, or a list of them.
type paths []string

func (p *paths) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*p = paths{n.Value}
		return nil
	}
	var l []string
	err := n.Decode(&l)
	*p = l
	return err
}

type source struct {
	RepoURL        string `yaml:"repoURL"`
	TargetRevision string `yaml:"targetRevision"`
	Path           string `yaml:"path"`
	Chart          string `yaml:"chart"`
	Ref            string `yaml:"ref"`
}

type appSpec struct {
	Source      *source  `yaml:"source"`
	Sources     []source `yaml:"sources"`
	Destination struct {
		Namespace string `yaml:"namespace"`
	} `yaml:"destination"`
}

// manifest is an Application or an ApplicationSet, the fields atlas reads.
type manifest struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		appSpec    `yaml:",inline"`
		Generators []struct {
			List struct {
				Elements []element `yaml:"elements"`
			} `yaml:"list"`
		} `yaml:"generators"`
		Template struct {
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec appSpec `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func gitops(x *Ctx) error {
	b, regSrc, err := x.Read("infra-bootstrap", registryPath)
	if err != nil {
		return err
	}
	var registry []element
	if err := yaml.Unmarshal(b, &registry); err != nil {
		return fmt.Errorf("%s: %w", registryPath, err)
	}
	var appsElems []element // the registry's hand mirror (apps.applicationset.yaml)
	for _, p := range x.Glob("infra-bootstrap", appsetDir+"/*.yaml") {
		b, src, err := x.Read("infra-bootstrap", p)
		if err != nil {
			return err
		}
		var m manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			continue // not one manifest (a multi-document Secret, a CRD bundle): not an Application
		}
		switch m.Kind {
		case "Application":
			app(x, src, m.Metadata.Name, m.Spec.appSpec, element{})
		case "ApplicationSet":
			for _, gen := range m.Spec.Generators {
				for _, el := range gen.List.Elements {
					name, ok := expand(m.Spec.Template.Metadata.Name, el)
					if !ok || name == "" {
						x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: "appset:" + m.Metadata.Name, Detail: "template name " + m.Spec.Template.Metadata.Name, Source: src})
						continue
					}
					app(x, src, name, expandSpec(m.Spec.Template.Spec, el), el)
				}
				if m.Metadata.Name == "apps" {
					appsElems = append(appsElems, gen.List.Elements...)
				}
			}
		}
	}
	mirror(x, regSrc, registry, appsElems)
	return nil
}

// app adds one Application: its node, its hostname and its git edges.
func app(x *Ctx, src graph.Source, name string, spec appSpec, el element) {
	id := graph.ID(graph.KindApp, name)
	attrs := map[string]string{"namespace": spec.Destination.Namespace}
	srcs := spec.Sources
	if spec.Source != nil {
		srcs = append(srcs, *spec.Source)
	}
	for _, s := range srcs {
		if s.Chart != "" {
			attrs["chart"] = s.RepoURL + "/" + s.Chart + "@" + s.TargetRevision
			continue
		}
		repo := repoName(s.RepoURL)
		if repo == "" {
			continue
		}
		e := graph.Edge{From: id, To: graph.ID(graph.KindRepo, repo), Kind: graph.EdgeGit, Origin: graph.Declared, Source: src,
			Attrs: map[string]string{"branch": s.TargetRevision}}
		switch {
		case s.Path != "":
			e.Attrs["paths"] = s.Path
		case s.Ref != "" && len(el.ValuesPath) > 0: // the values source: the element names the files
			e.Attrs["paths"] = strings.Join(el.ValuesPath, ",")
		}
		x.Edge(e)
		x.Track(Track{Repo: repo, Branch: s.TargetRevision})
	}
	x.Node(graph.Node{ID: id, Kind: graph.KindApp, Attrs: attrs, Source: src})
	if el.Hostname != "" {
		dns := graph.ID(graph.KindDNS, el.Hostname)
		x.Node(graph.Node{ID: dns, Kind: graph.KindDNS, Source: src})
		x.Edge(graph.Edge{From: dns, To: id, Kind: graph.EdgeRoute, Origin: graph.Declared, Source: src})
	}
}

// mirror records where the registry (the human source of truth) and its hand mirror disagree.
func mirror(x *Ctx, src graph.Source, registry, appset []element) {
	norm := func(l []element) map[string]string {
		m := map[string]string{}
		for _, e := range l {
			m[e.Name] = fmt.Sprintf("namespace=%s repo=%s branch=%s values=%s host=%s", e.Namespace, repoName(e.RepoURL), e.TargetRevision, strings.Join(e.ValuesPath, ","), e.Hostname)
		}
		return m
	}
	r, a := norm(registry), norm(appset)
	for _, name := range slices.Sorted(maps.Keys(r)) {
		switch {
		case a[name] == "":
			x.Drift(graph.Drift{Kind: graph.DriftMirror, Subject: graph.ID(graph.KindApp, name), Detail: "in the registry, not in apps.applicationset.yaml", Source: src})
		case a[name] != r[name]:
			x.Drift(graph.Drift{Kind: graph.DriftMirror, Subject: graph.ID(graph.KindApp, name), Detail: "registry " + r[name] + " vs appset " + a[name], Source: src})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(a)) {
		if r[name] == "" {
			x.Drift(graph.Drift{Kind: graph.DriftMirror, Subject: graph.ID(graph.KindApp, name), Detail: "in apps.applicationset.yaml, not in the registry", Source: src})
		}
	}
}

var (
	fieldRe = regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)
	ghRe    = regexp.MustCompile(`github\.com[:/]MohammadBnei/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)
)

// repoName is the GitHub name of one of the owner's repos, or "" (a chart repository, another host).
func repoName(url string) string {
	if m := ghRe.FindStringSubmatch(url); m != nil {
		return m[1]
	}
	return ""
}

// expand fills "{{.field}}" from el. Anything else between braces (a function, a condition) is not read: ok=false.
func expand(s string, el element) (string, bool) {
	vals := map[string]string{
		"name": el.Name, "namespace": el.Namespace, "repoURL": el.RepoURL, "targetRevision": el.TargetRevision,
		"hostname": el.Hostname, "chartRepoURL": el.ChartRepoURL, "chartName": el.ChartName, "chartRevision": el.ChartRevision,
	}
	ok := true
	out := fieldRe.ReplaceAllStringFunc(s, func(m string) string {
		v, has := vals[fieldRe.FindStringSubmatch(m)[1]]
		ok = ok && has
		return v
	})
	return out, ok && !strings.Contains(out, "{{")
}

func expandSpec(t appSpec, el element) appSpec {
	f := func(s string) string { v, _ := expand(s, el); return v }
	s := appSpec{}
	s.Destination.Namespace = f(t.Destination.Namespace)
	conv := func(in source) source {
		return source{RepoURL: f(in.RepoURL), TargetRevision: f(in.TargetRevision), Path: f(in.Path), Chart: f(in.Chart), Ref: in.Ref}
	}
	if t.Source != nil {
		c := conv(*t.Source)
		s.Source = &c
	}
	for _, in := range t.Sources {
		s.Sources = append(s.Sources, conv(in))
	}
	return s
}

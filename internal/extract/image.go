package extract

import (
	"maps"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// image writes an app's image.tag as the app's attr "image": the code an app runs, as its values files pin it (app CI
// writes the tag there). It reads the .yaml/.yml paths of the app's git edges whose branch is the one checked out, so
// a tag is never read off a branch ArgoCD doesn't deploy; when several files set it, the last wins (helm layering).
// No tag read, no attr.
func image(repos Repos, g *graph.Graph) {
	x := &Ctx{Repos: repos}
	tags := map[string]string{} // app id -> tag
	for _, e := range g.Edges {
		_, repo, _ := strings.Cut(e.To, ":")
		if e.Kind != graph.EdgeGit || e.Attrs["branch"] == "" || e.Attrs["branch"] != repos[repo].Branch {
			continue
		}
		for _, p := range strings.Split(e.Attrs["paths"], ",") {
			if ext := path.Ext(p); ext != ".yaml" && ext != ".yml" {
				continue
			}
			b, _, err := x.Read(repo, p)
			if err != nil {
				continue
			}
			var v struct {
				Image struct {
					Tag string `yaml:"tag"`
				} `yaml:"image"`
			}
			if yaml.Unmarshal(b, &v) == nil && v.Image.Tag != "" {
				tags[e.From] = v.Image.Tag
			}
		}
	}
	for i, n := range g.Nodes {
		if t, ok := tags[n.ID]; ok && n.Kind == graph.KindApp {
			g.Nodes[i].Attrs = maps.Clone(n.Attrs)
			if g.Nodes[i].Attrs == nil {
				g.Nodes[i].Attrs = map[string]string{}
			}
			g.Nodes[i].Attrs["image"] = t
		}
	}
}

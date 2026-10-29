package extract

import (
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/realms"
)

// TestLayerAttr: a building carries its ring as attrs.layer (plan/17 D1): its group's, its own override over it, a
// repo's workshops (atlas's, never yaml's); a part and the land carry none.
func TestLayerAttr(t *testing.T) {
	rl := &realms.Realms{
		Layers:     map[string]string{"data/postgres": "data", "guard/authentik": "services"},
		Components: map[string]realms.Component{"app:edge": {Realm: "guard", Group: "authentik", Layer: "edge"}},
	}
	g := graph.Graph{Nodes: []graph.Node{
		{ID: "db:pg", Realm: "data", Group: "postgres"},
		{ID: "vm:a", Realm: "data", Group: "postgres", Parent: "db:pg"},
		{ID: "app:auth", Realm: "guard", Group: "authentik"},
		{ID: "app:edge", Realm: "guard", Group: "authentik"},
		{ID: "repo:a", Kind: graph.KindRepo, Realm: realms.Code, Group: "apps", Function: realms.FnWorkshop},
		{ID: "host:a", Realm: "platform", Group: "hosts", Function: realms.FnLand},
	}}
	modelFacts(rl, &g)
	for id, want := range map[string]string{"db:pg": "data", "vm:a": "", "app:auth": "services", "app:edge": "edge", "repo:a": "workshops", "host:a": ""} {
		for _, n := range g.Nodes {
			if n.ID == id && n.Attrs["layer"] != want {
				t.Errorf("%s: layer %q, want %q", id, n.Attrs["layer"], want)
			}
		}
	}
}

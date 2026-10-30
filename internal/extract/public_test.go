package extract

import (
	"testing"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// TestRepoPublicAttr: a repo node carries public: "yes" only when its checkout is public; a private one, and a repo
// only an edge names (visibility unknown), carry no public attr.
func TestRepoPublicAttr(t *testing.T) {
	x := &Ctx{}
	g := graph.Graph{Edges: []graph.Edge{{From: "repo:pub", To: "repo:named"}}}
	repoNodes(x, Repos{"pub": {Public: true}, "priv": {}}, g)
	want := map[string]bool{"repo:pub": true, "repo:priv": false, "repo:named": false}
	for _, n := range x.g.Nodes {
		pub, ok := want[n.ID]
		if !ok {
			t.Errorf("unexpected node %s", n.ID)
			continue
		}
		delete(want, n.ID)
		if v, has := n.Attrs["public"]; pub != has || (has && v != "yes") {
			t.Errorf("%s: public=%q (set %v), want set %v", n.ID, v, has, pub)
		}
	}
	for id := range want {
		t.Errorf("no node %s", id)
	}
}

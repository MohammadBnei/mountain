package realms

import (
	"strings"
	"testing"
	"time"
)

func TestSources(t *testing.T) {
	r, err := LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sources) != 34 {
		t.Errorf("%d sources, the registry lists 34", len(r.Sources))
	}
	// The three `atlas state` reads carry a limit.
	for id, want := range map[string]time.Duration{"prom-cluster": 90 * time.Second, "pigsty-vmetrics": 30 * time.Second, "patroni-rest": 30 * time.Second} {
		if s := r.Get(id); s == nil || s.Limit != want || s.Reach.Laptop == "" || s.Reach.Cluster == "" {
			t.Errorf("%s: %+v", id, s)
		}
	}
	const ok = "sources:\n  - {id: a, kind: metrics, freshness_limit: 30s}\n"
	if _, err := ParseSources([]byte(ok)); err != nil {
		t.Fatalf("a good registry: %v", err)
	}
	for name, src := range map[string]string{
		"a field nobody reads": strings.Replace(ok, "kind:", "kinds:", 1),
		"an unknown kind":      strings.Replace(ok, "metrics", "metric", 1),
		"a bad limit":          strings.Replace(ok, "30s", "thirty", 1),
		"an id twice":          ok + "  - {id: a, kind: logs}\n",
		"a URL password":       ok + "  - {id: b, kind: logs, reach: {laptop: \"http://u:hunter22@x\"}}\n",
		"a token value":        ok + "  - {id: b, kind: logs, auth: \"token=abcdef0123456789\"}\n",
	} {
		if _, err := ParseSources([]byte(src)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

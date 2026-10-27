package extract

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/realms"
)

var update = flag.Bool("update", false, "rewrite testdata/golden/*.json from the current code")

// TestGolden pins the projection of testdata, whole (every repo) and as phase 1 reads it (infra-bootstrap alone, the
// branches ArgoCD deploys from): a change to an extractor or a pass that moves any node, edge, drift or version shows
// here. Regenerate with -update, and review the diff.
func TestGolden(t *testing.T) {
	rl, err := realms.Parse([]byte(fixtureRealms))
	if err != nil {
		t.Fatal(err)
	}
	all := Repos{}
	for _, r := range []string{"infra-bootstrap", "agent-fleet", "pi-lab", "mountain"} {
		all[r] = Repo{Dir: "testdata/" + r, SHA: strings.Repeat(r[:1], 40), Branch: "main"}
	}
	for name, repos := range map[string]Repos{"all": all, "phase1": {"infra-bootstrap": all["infra-bootstrap"]}} {
		t.Run(name, func(t *testing.T) {
			res, err := Run(repos, rl, All)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(res, "", " ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := "testdata/golden/" + name + ".json"
			if *update {
				if err := os.MkdirAll("testdata/golden", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s changed: run with -update and review the diff", path)
			}
		})
	}
}

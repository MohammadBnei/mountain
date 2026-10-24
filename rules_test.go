package mountain

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestRules checks the PrometheusRule in helm/values.yaml extraManifests: every rule is severity notice
// (warning|critical route to thot; info is eaten by the live InfoInhibitor inhibit rule, so never reaches Discord), and runs helm/rules.test.yaml through promtool when it is on PATH.
func TestRules(t *testing.T) {
	b, err := os.ReadFile("helm/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		ExtraManifests []string `yaml:"extraManifests"`
	}
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	var rule struct {
		Kind string
		Spec struct {
			Groups []struct {
				Rules []struct {
					Alert  string
					Labels map[string]string
				}
			}
		}
	}
	var src string
	for _, m := range v.ExtraManifests {
		if strings.Contains(m, "kind: PrometheusRule") {
			// the template lines are Helm's; the rule body is plain YAML once the namespace is filled in
			src = strings.ReplaceAll(m, "{{ .Release.Namespace }}", "mountain")
			src = strings.Replace(src, `{{- include "common-app-chart.labels" . | nindent 8 }}`, "{}", 1)
		}
	}
	if err := yaml.Unmarshal([]byte(src), &rule); err != nil || rule.Kind != "PrometheusRule" {
		t.Fatalf("no PrometheusRule in extraManifests: %v", err)
	}
	names := map[string]bool{}
	for _, g := range rule.Spec.Groups {
		for _, r := range g.Rules {
			names[r.Alert] = true
			if r.Labels["severity"] != "notice" {
				t.Errorf("%s: severity %q, want notice", r.Alert, r.Labels["severity"])
			}
		}
	}
	for _, n := range []string{"MountainSourceStale", "MountainTokenExpiring", "MountainUnassigned"} {
		if !names[n] {
			t.Errorf("rule %s missing", n)
		}
	}
	if strings.Contains(src, "{{") {
		t.Error("a template action left in the rule body ({{ $labels }} would be eaten by Helm tpl)")
	}

	if _, err := exec.LookPath("promtool"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("promtool not on PATH in CI: the rule unit tests must run before deploy")
		}
		t.Skip("promtool not on PATH: rule unit tests not run")
	}
	var spec struct {
		Spec yaml.Node `yaml:"spec"`
	}
	yaml.Unmarshal([]byte(src), &spec)
	out, _ := yaml.Marshal(&spec.Spec)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "rules.yaml"), out, 0o644)
	tb, _ := os.ReadFile("helm/rules.test.yaml")
	os.WriteFile(filepath.Join(dir, "rules.test.yaml"), tb, 0o644)
	cmd := exec.Command("promtool", "test", "rules", "rules.test.yaml")
	cmd.Dir = dir
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("promtool test rules: %v\n%s", err, o)
	} else {
		t.Logf("promtool: %s", o)
	}
}

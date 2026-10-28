package realms

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestChecks: Parse's checks keep their order (the first error is Parse's, and components reads the functions the
// check before it validated), and each has a name and a body.
func TestChecks(t *testing.T) {
	var names []string
	for _, c := range checks() {
		if c.name == "" || c.run == nil {
			t.Errorf("check %q has no body", c.name)
		}
		names = append(names, c.name)
	}
	want := []string{"code", "functions", "components", "watch", "needs", "shapes", "walls", "addons", "k8sPlacement", "sources", "probes", "namespaces", "pveNodeName"}
	if !slices.Equal(names, want) {
		t.Errorf("checks %v, want %v", names, want)
	}
}

// TestNeedKinds: ParseNeed's refusal names every kind it accepts (Suppliers' kinds, and declared).
func TestNeedKinds(t *testing.T) {
	_, err := ParseNeed("nope")
	if err == nil {
		t.Fatal("ParseNeed accepted nope")
	}
	_, list, _ := strings.Cut(err.Error(), " is not ")
	for _, k := range append(slices.Sorted(maps.Keys(Suppliers)), "declared") {
		if !strings.Contains(list, k) {
			t.Errorf("ParseNeed's refusal %q does not name %s", err, k)
		}
	}
}

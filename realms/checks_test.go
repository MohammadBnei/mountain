package realms

import (
	"maps"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestChecks: Parse's checks keep their order (the first error is Parse's, and components reads the functions the
// check before it validated), each name on its own function.
func TestChecks(t *testing.T) {
	want := []check{{"code", checkCode}, {"functions", checkFunctions}, {"components", checkComponents}, {"watch", checkWatch},
		{"needs", checkNeeds}, {"shapes", checkShapes}, {"walls", checkWalls}, {"addons", checkAddons},
		{"k8sPlacement", checkK8sPlacement}, {"bareMetal", checkBareMetal}, {"sources", checkSources}, {"probes", checkProbes},
		{"namespaces", checkNamespaces}, {"pveNodeName", checkPveNodeName}}
	fn := func(f func(*Realms) error) string { return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name() }
	got := checks()
	if len(got) != len(want) {
		t.Fatalf("%d checks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].name != want[i].name || got[i].run == nil || fn(got[i].run) != fn(want[i].run) {
			t.Errorf("check %d is %q on %s, want %q on %s", i, got[i].name, fn(got[i].run), want[i].name, fn(want[i].run))
		}
	}
}

// TestNeedKinds: ParseNeed's refusal names exactly the kinds it accepts (Suppliers' kinds, and declared).
func TestNeedKinds(t *testing.T) {
	_, err := ParseNeed("nope")
	if err == nil {
		t.Fatal("ParseNeed accepted nope")
	}
	_, list, _ := strings.Cut(err.Error(), " is not ")
	var named []string
	for _, k := range strings.FieldsFunc(strings.ReplaceAll(list, " or ", ", "), func(r rune) bool { return r == ',' || r == ' ' }) {
		k, _, _ = strings.Cut(k, ":")
		named = append(named, k)
	}
	slices.Sort(named)
	want := append(slices.Collect(maps.Keys(Suppliers)), "declared")
	slices.Sort(want)
	if !slices.Equal(named, want) {
		t.Errorf("ParseNeed's refusal names %v, want %v (%q)", named, want, err)
	}
}

package extract

import (
	"maps"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/MohammadBnei/mountain/realms"
)

// TestEvidenceFinders: every kind of evidence realms.Suppliers lists has a finder, except postgres (pigsty's
// pg_databases answers it; declared, the other needs kind, is not a supplier), so a supplier kind added without one
// fails here instead of marking every such row "no evidence"; and each kind is wired to its own finder.
func TestEvidenceFinders(t *testing.T) {
	finders := evidenceFinders()
	fn := func(f evidenceFinder) string { return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name() }
	for kind, want := range map[string]evidenceFinder{"oidc": findOIDC, "forwardauth": findForwardAuth, "s3": findS3, "infisical": findInfisical} {
		if f := finders[kind]; f == nil || fn(f) != fn(want) {
			t.Errorf("finder %s is wired to %v, want %s", kind, f, fn(want))
		}
	}
	got := append(slices.Collect(maps.Keys(finders)), "postgres")
	slices.Sort(got)
	if want := slices.Sorted(maps.Keys(realms.Suppliers)); !slices.Equal(got, want) {
		t.Errorf("finders + postgres = %v, want realms.Suppliers' kinds %v", got, want)
	}
}

package extract

import (
	"maps"
	"slices"
	"testing"

	"github.com/MohammadBnei/mountain/realms"
)

// TestEvidenceFinders: every kind of evidence a needs row can name has a finder, except postgres (pigsty's
// pg_databases answers it), so a supplier kind added without one fails here instead of marking every such row
// "no evidence".
func TestEvidenceFinders(t *testing.T) {
	finders := evidenceFinders()
	for kind, find := range finders {
		if find == nil {
			t.Errorf("finder %s is nil", kind)
		}
	}
	got := append(slices.Collect(maps.Keys(finders)), "postgres")
	slices.Sort(got)
	if want := slices.Sorted(maps.Keys(realms.Suppliers)); !slices.Equal(got, want) {
		t.Errorf("finders + postgres = %v, want realms.Suppliers' kinds %v", got, want)
	}
}

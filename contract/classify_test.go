package contract

import (
	"maps"
	"slices"
	"testing"
)

// TestClassifiers: the measurable kinds are the registry's keys, and only an app's classifier skips the node rule.
// A kind added or dropped is a visible change.
func TestClassifiers(t *testing.T) {
	if got, want := slices.Sorted(maps.Keys(classifiers)), []string{"addon", "app", "ct", "db", "host", "vm"}; !slices.Equal(got, want) {
		t.Errorf("classifiers = %v, want %v", got, want)
	}
	for kind, c := range classifiers {
		if c.ownRule != (kind == "app") || c.run == nil {
			t.Errorf("%s: ownRule %v, run set %v", kind, c.ownRule, c.run != nil)
		}
	}
}

package contract

import (
	"slices"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	if len(DefaultRules) < 100 || DefaultRules["Watchdog"] == nil || !slices.Equal(DefaultRules["KubePersistentVolumeFillingUp"], []string{"critical", "warning"}) {
		t.Errorf("%d rules, Watchdog %v, KubePersistentVolumeFillingUp %v", len(DefaultRules), DefaultRules["Watchdog"], DefaultRules["KubePersistentVolumeFillingUp"])
	}
	for name, sevs := range DefaultRules {
		for _, s := range sevs {
			if !slices.Contains([]string{"critical", "warning", "info", "none"}, s) {
				t.Errorf("%s fires at %q", name, s)
			}
		}
	}
}

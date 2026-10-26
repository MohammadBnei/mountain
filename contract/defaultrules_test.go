package contract

import (
	"slices"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	pv := []Rule{{"critical", 60}, {"warning", 3600}}
	if len(DefaultRules) < 100 || !slices.Equal(DefaultRules["Watchdog"], []Rule{{"none", 0}}) || !slices.Equal(DefaultRules["KubePersistentVolumeFillingUp"], pv) {
		t.Errorf("%d rules, Watchdog %v, KubePersistentVolumeFillingUp %v", len(DefaultRules), DefaultRules["Watchdog"], DefaultRules["KubePersistentVolumeFillingUp"])
	}
	// the sim's derived alerts read these: pinned so a chart bump that moves one shows here
	for name, want := range map[string][]Rule{"KubeNodeNotReady": {{"warning", 900}}, "KubeDeploymentReplicasMismatch": {{"warning", 900}},
		"KubePodCrashLooping": {{"warning", 900}}, "TargetDown": {{"warning", 600}}} {
		if !slices.Equal(DefaultRules[name], want) {
			t.Errorf("%s: %v, want %v", name, DefaultRules[name], want)
		}
	}
	for name, rs := range DefaultRules {
		for _, r := range rs {
			if !slices.Contains([]string{"critical", "warning", "info", "none"}, r.Severity) || r.For < 0 {
				t.Errorf("%s: %+v", name, r)
			}
		}
	}
}

package contract

import (
	"regexp"
	"testing"
)

// TestExtProbeRegex: the probe queries ask for the moons' targets (ext-<name>) beside the guests' and the hosts', and
// nothing else.
func TestExtProbeRegex(t *testing.T) {
	matcher := regexp.MustCompile(`target=~"([^"]+)"`)
	for name, q := range map[string]string{"probes": Queries["probes"], "probeday": Signals["probeday"].Query} {
		m := matcher.FindStringSubmatch(q)
		if m == nil {
			t.Errorf("%s: no target matcher in %s", name, q)
			continue
		}
		re := regexp.MustCompile(`^(?:` + m[1] + `)$`) // PromQL anchors a matcher at both ends
		for target, want := range map[string]bool{"ext-github": true, "ext-anthropic": true, "guest-witness": true, "pve-bnei": true, "github": false, "next-x": false} {
			if re.MatchString(target) != want {
				t.Errorf("%s: %q matches %s: %v, want %v", name, m[1], target, !want, want)
			}
		}
	}
}

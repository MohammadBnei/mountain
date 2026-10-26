package contract

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestMajority(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, Unknown},
		{[]string{Down, Down, OK}, Down},
		{[]string{OK, OK, Down}, OK},
		{[]string{Down, OK, Unknown}, Unknown}, // neither side has a majority
		{[]string{Down, Unknown, Unknown}, Unknown},
		{[]string{Down, Down, Unknown}, Down},
		{[]string{OK, Down}, Unknown},
		{[]string{OK}, OK},
	} {
		if got := Majority(c.in); got != c.want {
			t.Errorf("Majority(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestClassify(t *testing.T) {
	topo := Topology{
		Kind: map[string]string{
			"app:ok": "app", "app:deg": "app", "app:miss": "app", "app:prog": "app", "app:unk": "app", "app:gone": "app",
			"vm:a": "vm", "vm:b": "vm", "vm:c": "vm", "vm:k1": "vm", "vm:k2": "vm", "ct:g": "ct", "ct:quiet": "ct",
			"host:h": "host", "host:bare": "host", "host:probed": "host",
			"db:pg": "db", "db:half": "db", "db:none": "db",
			"addon:full": "addon", "addon:roll": "addon", "addon:long": "addon", "addon:vip": "addon",
			"repo:r": "repo", "svc:s": "svc",
		},
		RunsOn: map[string][]string{
			"vm:k1": {"host:h"}, "vm:k2": {"host:h"},
			"db:pg":   {"vm:a", "vm:b", "vm:c"},
			"db:half": {"vm:a", "vm:c", "vm:quiet"},
		},
	}
	f := &Live{At: 1,
		Apps: map[string]AppLive{"app:ok": {Health: "Healthy", Sync: "Synced"}, "app:deg": {Health: "Degraded"}, "app:miss": {Health: "Missing"},
			"app:prog": {Health: "Progressing"}, "app:unk": {Health: "Unknown"}},
		Ready:  map[string]bool{"vm:k1": true, "vm:k2": false},
		Probes: map[string]bool{"vm:a": false, "vm:b": false, "vm:c": true, "ct:g": false, "host:probed": true, "svc:s": true},
		Addons: map[string]float64{"addon:full": 1, "addon:roll": 0.5, "addon:long": 0.5},
	}
	low := map[string]int64{"addon:roll": 1000, "addon:long": 1000}
	got := Classify(topo, f, low, 1000+AddonHold-1)
	want := map[string]string{
		"app:ok": OK, "app:deg": Degraded, "app:miss": Down, "app:prog": OK, "app:unk": Unknown, "app:gone": Unknown,
		"vm:a": Down, "vm:c": OK, "vm:k1": OK, "vm:k2": Down, "ct:g": Down, "ct:quiet": Unknown,
		"host:h": Degraded, "host:bare": Unknown, "host:probed": OK, // a host's own probe comes first
		"db:pg": Down, "db:half": Unknown, "db:none": Unknown, // pg: 2 of 3 down; half: 1 down, 1 up, 1 unmeasured
		"addon:full": OK, "addon:roll": OK, "addon:long": OK, "addon:vip": Unknown,
		"svc:s": OK, // any probed node is measured
	}
	for id, w := range want {
		if got[id].State != w {
			t.Errorf("%s = %+v, want %s", id, got[id], w)
		}
	}
	if _, ok := got["repo:r"]; ok {
		t.Error("a repo got a health: nothing live measures it")
	}
	if !strings.Contains(got["app:ok"].Why, "Healthy, Synced") || !strings.Contains(got["db:pg"].Why, "1 of 3 members answer, 2 don't") {
		t.Errorf("whys: %q %q", got["app:ok"].Why, got["db:pg"].Why)
	}
	// the hold: below all ready for AddonHold is damage, a rollout isn't
	got = Classify(topo, f, map[string]int64{"addon:long": 1000}, 1000+AddonHold)
	if got["addon:long"].State != Degraded || got["addon:roll"].State != OK {
		t.Errorf("after the hold: long %+v, roll (no since) %+v", got["addon:long"], got["addon:roll"])
	}
	// no Prometheus answer yet: every measurable node unknown, nothing ok
	f.At = 0
	for id, h := range Classify(topo, f, low, 0) {
		if h.State != Unknown {
			t.Errorf("before the first answer %s is %s", id, h.State)
		}
	}
	if len(Classify(topo, nil, nil, 0)) != 0 {
		t.Error("a nil frame classified something")
	}
}

// The frame's wire names are what atlas's page and the game's town read: renaming one breaks them silently.
func TestLiveWire(t *testing.T) {
	b, _ := json.Marshal(Live{T: "live", AlertsAt: 1, Health: map[string]Health{"app:a": {OK, "x"}}, Day: &Day{}, Addons: map[string]float64{"a": 1}})
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := []string{"addons", "alerts", "alertsAt", "apps", "at", "day", "health", "land", "probes", "ready", "stale", "t"}
	if !slices.Equal(keys, want) {
		t.Errorf("live frame keys = %v, want %v", keys, want)
	}
	if !strings.Contains(string(b), `"health":{"app:a":{"state":"ok","why":"x"}}`) {
		t.Errorf("health on the wire: %s", b)
	}
}

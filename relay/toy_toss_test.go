package main

import (
	"math"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

// TestTossHostile: every hostile toss is rejected or corrected (y recomputed, x/z quantised, extra keys dropped);
// an honest one lands as {pos, from}. Rate limits run per case on one account (the seam's 1/min).
func TestTossHostile(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	nan, inf := math.NaN(), math.Inf(1)
	v := func(x ...float64) []any {
		l := make([]any, len(x))
		for i, f := range x {
			l[i] = f
		}
		return l
	}
	pad := make([]any, 400)
	for i := range pad {
		pad[i] = 0.0
	}
	cases := []struct {
		name string
		toy  string
		pose map[string]any
		why  string
	}{
		{"honest", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 8)}, ""},
		{"forged y", "toss", map[string]any{"pos": v(-2.3, 999, 2.6), "from": v(-2, -500, 5)}, ""},
		{"extra keys", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 8), "pad": pad, "note": "<script>"}, ""},
		{"nan pos", "toss", map[string]any{"pos": v(nan, 0, 2), "from": v(-3, 0, 8)}, "bad"},
		{"inf from", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, inf)}, "bad"},
		{"no from", "toss", map[string]any{"pos": v(-3, 0, 2)}, "bad"},
		{"text from", "toss", map[string]any{"pos": v(-3, 0, 2), "from": "here"}, "bad"},
		{"short pos", "toss", map[string]any{"pos": v(-3, 2), "from": v(-3, 0, 8)}, "bad"},
		{"snowline", "toss", map[string]any{"pos": v(90, 0, 90), "from": v(85, 0, 85)}, "snowline"},
		{"pos off world", "toss", map[string]any{"pos": v(0, 50, 140), "from": v(0, 50, 130)}, "reach"},
		{"from off world", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 101)}, "reach"},
		{"too far", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 2+tossMax+0.5)}, "throw"},
		{"wrong toy", "tosss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 8)}, "toy"},
	}
	solid := worldcore.Solid(a.World.Base, a.Land)
	t0 := time.Now()
	for i, tc := range cases {
		cl := visitor("u-toss", "bo-42")
		now := t0.Add(time.Duration(i) * time.Minute)
		cl.RoleExp = now.Add(time.Hour)
		drain(c)
		_, why := ackOf(t, a.place(cl, "", tc.toy, tc.pose, false, now))
		if why != tc.why {
			t.Errorf("%s: got %q, want %q (snowline %v)", tc.name, why, tc.why, a.Metrics.Snowline)
			continue
		}
		if why != "" {
			continue
		}
		ms := marksOf(drain(c))
		if len(ms) != 1 {
			t.Fatalf("%s: %d world:mark frames", tc.name, len(ms))
		}
		pose := ms[0]["pose"].(map[string]any)
		if len(pose) != 2 {
			t.Errorf("%s: pose keys %v, want pos + from", tc.name, pose)
		}
		for _, k := range []string{"pos", "from"} {
			p := pose[k].([]any)
			x, y, z := p[0].(float64), p[1].(float64), p[2].(float64)
			i, j := gridCell(x), gridCell(z)
			if cx, cz := worldcore.CellXZ(i, j); cx != x || cz != z || y != float64(solid[j*worldcore.Grid+i])*worldcore.HeightQ {
				t.Errorf("%s: %s %v not quantised or y not heightSolid", tc.name, k, p)
			}
		}
	}
	// flood: a second toss in the same minute is rate-limited (the seam's limit covers toss)
	cl := visitor("u-flood", "")
	for i, want := range []string{"", "rate"} {
		if _, why := ackOf(t, a.place(cl, "", "toss", map[string]any{"pos": v(-3, 0, 2), "from": v(-3, 0, 8)}, false, t0.Add(time.Duration(i)*time.Second))); why != want {
			t.Errorf("flood %d: %q, want %q", i, why, want)
		}
	}
}

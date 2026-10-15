package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

// TestCairnValidate (table-driven, a synthetic world): hostile cairn poses are rejected or corrected (y recomputed,
// x/z quantised, quat normalised, extra keys dropped); a toppled stone is refused; a stone settles within cairnNear
// cells onto a cairn first, else the nearest flat cell, and stacks there up to cairnMax.
func TestCairnValidate(t *testing.T) {
	a, _ := testApp(t)
	// flat at 60 m (snowline 50); i ≥ 80: a 4 m/cell ramp (slope 2.56); j < 20: 40 m, under the snowline
	solid := make([]int16, worldcore.Grid*worldcore.Grid)
	for j := range worldcore.Grid {
		for i := range worldcore.Grid {
			h := 60.0
			if i >= 80 {
				h += 4 * float64(i-80)
			}
			if j < 20 {
				h = 40
			}
			solid[j*worldcore.Grid+i] = int16(h / worldcore.HeightQ)
		}
	}
	p0 := &Place{Solid: solid, Snowline: 50}
	xz := func(i, j int) []any { x, z := worldcore.CellXZ(i, j); return []any{x, 0.0, z} }
	up, s := []any{0.0, 0.0, 0.0, 1.0}, math.Sqrt(0.5)
	stones := func(n, i, j int) (ms []Mark) {
		for range n {
			ms = append(ms, Mark{Mark: "cairns", Pose: map[string]any{"pos": xz(i, j)}})
		}
		return ms
	}
	var walled []Mark // every cell around (40, 40) is a full cairn
	for dj := -cairnNear; dj <= cairnNear; dj++ {
		for di := -cairnNear; di <= cairnNear; di++ {
			walled = append(walled, stones(cairnMax, 40+di, 40+dj)...)
		}
	}
	for _, c := range []struct {
		name  string
		pose  map[string]any
		marks []Mark
		why   string
		i, j  int
	}{
		{"honest, off the cell centre", map[string]any{"pos": []any{0.3, 60.0, -0.4}, "quat": up}, nil, "", 64, 64},
		{"forged y", map[string]any{"pos": []any{0.0, 999.0, 0.0}, "quat": up}, nil, "", 64, 64},
		{"unnormalised quat, extra keys", map[string]any{"pos": xz(64, 64), "quat": []any{0.0, 3.0, 0.0, 4.0}, "note": "<script>", "n": 99.0}, nil, "", 64, 64},
		{"stacks on a cairn nearby", map[string]any{"pos": xz(65, 66), "quat": up},
			append(stones(2, 64, 64), stones(1, 66, 66)...), "", 66, 66},
		{"a toss pebble is no cairn", map[string]any{"pos": xz(65, 64), "quat": up}, []Mark{{Mark: "toss", Pose: map[string]any{"pos": xz(66, 64)}}}, "", 65, 64},
		{"a full cairn: beside it", map[string]any{"pos": xz(64, 64), "quat": up}, stones(cairnMax, 64, 64), "", 64, 63},
		{"full all around", map[string]any{"pos": xz(40, 40), "quat": up}, walled, "full", 0, 0},
		{"steep: settles onto the flat", map[string]any{"pos": xz(81, 64), "quat": up}, nil, "", 79, 64},
		{"steep all around", map[string]any{"pos": xz(90, 64), "quat": up}, nil, "slope", 0, 0},
		{"below the snowline", map[string]any{"pos": xz(40, 5), "quat": up}, nil, "snowline", 0, 0},
		{"steep edge: settles above the snowline", map[string]any{"pos": xz(40, 20), "quat": up}, nil, "", 40, 21},
		{"out of reach", map[string]any{"pos": []any{140.0, 60.0, 0.0}, "quat": up}, nil, "reach", 0, 0},
		{"toppled", map[string]any{"pos": xz(64, 64), "quat": []any{s, 0.0, 0.0, s}}, nil, "upright", 0, 0},
		{"NaN pos", map[string]any{"pos": []any{math.NaN(), 0.0, 0.0}, "quat": up}, nil, "bad", 0, 0},
		{"Inf quat", map[string]any{"pos": xz(64, 64), "quat": []any{math.Inf(1), 0.0, 0.0, 1.0}}, nil, "bad", 0, 0},
		{"huge quat", map[string]any{"pos": xz(64, 64), "quat": []any{0.0, 1e200, 0.0, 1e200}}, nil, "bad", 0, 0},
		{"zero quat", map[string]any{"pos": xz(64, 64), "quat": []any{0.0, 0.0, 0.0, 0.0}}, nil, "bad", 0, 0},
		{"short quat", map[string]any{"pos": xz(64, 64), "quat": []any{0.0, 0.0, 1.0}}, nil, "bad", 0, 0},
		{"no quat", map[string]any{"pos": xz(64, 64)}, nil, "bad", 0, 0},
		{"text pos", map[string]any{"pos": "3,0,3", "quat": up}, nil, "bad", 0, 0},
		{"oversized pos", map[string]any{"pos": make([]any, 4096), "quat": up}, nil, "bad", 0, 0},
	} {
		p := *p0
		p.Pose, p.Marks = c.pose, c.marks
		pose, w, why := runValidate("cairns", toys["cairns"], &p)
		if why != c.why {
			t.Errorf("%s: %q, want %q", c.name, why, c.why)
			continue
		}
		if why != "" {
			continue
		}
		pos, q := pose["pos"].([]float64), pose["quat"].([]float64)
		x, z := worldcore.CellXZ(c.i, c.j)
		y := float64(solid[c.j*worldcore.Grid+c.i]) * worldcore.HeightQ
		if len(pose) != 2 || w != 1 || pos[0] != x || pos[1] != y || pos[2] != z {
			t.Errorf("%s: pose %v weight %v, want cell (%d, %d) = [%v %v %v]", c.name, pose, w, c.i, c.j, x, y, z)
		}
		if l := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3]); math.Abs(l-1) > 1e-9 {
			t.Errorf("%s: quat %v not unit", c.name, q)
		}
		if (&Out{app: a, name: "marks"}).worldFrame("mark", map[string]any{"id": ULID(), "mark": "cairns", "pose": pose, "weight": w}) == nil {
			t.Errorf("%s: sanitize refuses %v", c.name, pose)
		}
	}
}

// TestCairnPlace (the seed-1 world): through place(), a second account's stone dropped nearby stacks on the first
// (the live marks), the same account inside a minute is rate-limited, and no account id reaches the frames.
func TestCairnPlace(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	pose := func(x, z float64) map[string]any {
		return map[string]any{"pos": []any{x, 0.0, z}, "quat": []any{0.0, 0.0, 0.0, 1.0}}
	}
	now := time.Now()
	for k, sub := range []string{"u-a", "u-b"} {
		if _, why := ackOf(t, a.place(visitor(sub, "ana_k"), "", "cairns", pose(3-float64(k), 3), k == 0, now)); why != "" {
			t.Fatalf("%s: %s", sub, why)
		}
	}
	if _, why := ackOf(t, a.place(visitor("u-a", ""), "", "cairns", pose(3, 3), false, now.Add(time.Second))); why != "rate" {
		t.Errorf("flood: %q", why)
	}
	ms := marksOf(drain(c))
	if len(ms) != 2 || string(mustJSON(ms[0]["pose"])) != string(mustJSON(ms[1]["pose"])) {
		t.Fatalf("marks %v", ms)
	}
	if s := string(mustJSON(ms)); strings.Contains(s, "u-a") || strings.Contains(s, "u-b") {
		t.Errorf("sub leaked: %s", s)
	}
}

package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

// TestFootprintsValidate: hostile poses are rejected or corrected (forged y recomputed, path quantised to cells).
func TestFootprintsValidate(t *testing.T) {
	a, _ := testApp(t)
	withToys(t, a)
	path := func(xs ...float64) []any {
		l := make([]any, len(xs))
		for i, x := range xs {
			l[i] = x
		}
		return l
	}
	long := make([]float64, 18)
	for i := range long {
		long[i] = 2
	}
	// a snow-free cell within reach of (2, −3), the honest pos near the summit
	g, bx, bz := &Place{Solid: worldcore.Solid(a.World.Base, a.Land)}, 0.0, 0.0
	for r := 15.0; r > 2 && bx == 0; r-- {
		for th := 0.0; th < 2*math.Pi; th += 0.1 {
			if _, y, _, _ := g.Ground(2+r*math.Cos(th), -3+r*math.Sin(th)); y <= a.Metrics.Snowline {
				bx, bz = 2+r*math.Cos(th), -3+r*math.Sin(th)
				break
			}
		}
	}
	if bx == 0 {
		t.Fatal("no cell under the snowline within reach of the summit")
	}
	now := time.Now()
	for i, c := range []struct {
		name string
		pose map[string]any
		want string
	}{
		{"honest", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3, 3, -3, 4, -2)}, ""},
		{"forged y", map[string]any{"pos": path(2, 999, -3), "path": path(2, -3)}, ""},
		{"one point", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3)}, ""},
		{"NaN pos", map[string]any{"pos": path(math.NaN(), 0, -3), "path": path(2, -3)}, "bad"},
		{"Inf path", map[string]any{"pos": path(2, 0, -3), "path": path(2, math.Inf(1))}, "bad"},
		{"no path", map[string]any{"pos": path(2, 0, -3)}, "bad"},
		{"odd path", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3, 3)}, "bad"},
		{"oversized path", map[string]any{"pos": path(2, 0, -3), "path": path(long...)}, "bad"},
		{"text path", map[string]any{"pos": path(2, 0, -3), "path": []any{"<script>", 1.0}}, "bad"},
		{"pos below snowline", map[string]any{"pos": path(60, 0, 60), "path": path(60, 60)}, "snowline"},
		{"path below snowline", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3, bx, bz)}, "snowline"},
		{"pos out of world", map[string]any{"pos": path(0, 50, 140), "path": path(0, 140)}, "reach"},
		{"path out of reach", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3, 2, 30)}, "reach"},
		{"path out of world", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3, 2, 300)}, "reach"},
	} {
		id, why := ackOf(t, a.place(visitor("u"+string(rune('a'+i)), ""), "", "footprints", c.pose, false, now))
		if why != c.want || (why == "") != (id != "") {
			t.Errorf("%s: got %q (id %q), want %q", c.name, why, id, c.want)
		}
	}
	if _, why := ackOf(t, a.place(visitor("ua", ""), "", "footprints", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3)}, false, now)); why != "rate" {
		t.Errorf("flood: got %q, want rate", why)
	}
	if _, why := ackOf(t, a.place(visitor("uz", ""), "", "footprint", map[string]any{"pos": path(2, 0, -3), "path": path(2, -3)}, false, now)); why != "toy" {
		t.Errorf("wrong toy: got %q, want toy", why)
	}
	m := a.State.Marks[0]
	p := m.Pose["path"].([]any)
	if len(a.State.Marks) != 3 || m.Weight != 1 || len(p) != 6 || p[0] == 2.0 || p[2] == 3.0 {
		t.Fatalf("marks %+v: want 3, weight 1, the 3-point path quantised to cells", a.State.Marks)
	}
	if y := a.State.Marks[1].Pose["pos"].([]any)[1]; y == 999.0 || y != m.Pose["pos"].([]any)[1] {
		t.Errorf("forged y kept: %v", y)
	}
}

// TestFootprintsErode: weather (age) and rain lower the weight in 0.05 steps, re-emitted on the same id; it goes gone below 0.1.
func TestFootprintsErode(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	o, now := &Out{app: a, name: "footprints"}, time.Now()
	pose := map[string]any{"pos": []any{2.0, 0.0, -3.0}, "path": []any{2.0, -3.0, 4.0, -2.0}}
	id, _ := ackOf(t, a.place(visitor("u1", "ana_k"), "", "footprints", pose, true, now))
	drain(c)
	weight := func() float64 {
		for _, m := range a.State.Marks {
			if m.ID == id {
				return m.Weight
			}
		}
		return -1
	}

	tickFootprints(a, o, now.Add(time.Minute))
	if len(drain(c)) != 0 || weight() != 1 {
		t.Fatalf("a fresh footprint re-emitted (weight %v)", weight())
	}
	tickFootprints(a, o, now.Add(24*time.Hour)) // exp(−0.5) = 0.607 → 0.65
	ms := marksOf(drain(c))
	if len(ms) != 1 || ms[0]["id"] != id || weight() != 0.65 || ms[0]["by"] != "ana_k" || len(a.State.Marks) != 1 {
		t.Fatalf("a day of weather: %v, weight %v", ms, weight())
	}
	if s := string(mustJSON(ms[0])); !strings.Contains(s, `"path":[`) {
		t.Errorf("re-emitted pose lost its path: %s", s)
	}

	a.State.Clouds = []Cloud{{ID: ULID(), Pos: [3]float64{30, 40, 30}, State: "rain"}} // far: no effect
	tickFootprints(a, o, now.Add(24*time.Hour))
	if weight() != 0.65 {
		t.Fatalf("rain 30 m away eroded it: %v", weight())
	}
	a.State.Clouds[0].Pos = [3]float64{5, 40, -2}
	tickFootprints(a, o, now.Add(24*time.Hour)) // 0.65 × 0.6 = 0.39 → 0.4
	if weight() != 0.4 {
		t.Fatalf("rain overhead: weight %v, want 0.4", weight())
	}
	drain(c)
	tickFootprints(a, o, now.Add(10*24*time.Hour))
	ms = marksOf(drain(c))
	if len(ms) != 1 || ms[0]["gone"] != true || weight() != -1 {
		t.Fatalf("10 days: %v, weight %v; want gone", ms, weight())
	}
}

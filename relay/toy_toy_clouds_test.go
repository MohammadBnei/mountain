package main

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

func cloudApp(t *testing.T) (*App, *Client) {
	a, c := testApp(t)
	withToys(t, a)
	clouds = nil
	old := toyTick
	toyTick = 15 * time.Second // a fixture test may have left its step here
	t.Cleanup(func() { clouds, toyTick = nil, old })
	return a, c
}

// TestCloudValidate: hostile spawns are rejected or corrected (y recomputed, x/z quantised, extra keys dropped).
func TestCloudValidate(t *testing.T) {
	a, _ := cloudApp(t)
	solid := worldcore.Solid(a.World.Base, a.Land)
	big := make([]any, 1000)
	for i := range big {
		big[i] = 1.0
	}
	for name, tc := range map[string]struct {
		pose map[string]any
		why  string
	}{
		"honest":    {map[string]any{"pos": []any{0.0, 0.0, 4.0}}, ""},
		"forged y":  {map[string]any{"pos": []any{0.3, 999.0, 4.2}, "note": "<script>"}, ""},
		"nan":       {map[string]any{"pos": []any{math.NaN(), 0.0, 4.0}}, "bad"},
		"inf":       {map[string]any{"pos": []any{0.0, 0.0, math.Inf(1)}}, "bad"},
		"text":      {map[string]any{"pos": []any{"0", "0", "4"}}, "bad"},
		"short":     {map[string]any{"pos": []any{0.0, 4.0}}, "bad"},
		"oversized": {map[string]any{"pos": big}, "bad"},
		"missing":   {map[string]any{"at": []any{0.0, 0.0, 4.0}}, "bad"},
		"snowline":  {map[string]any{"pos": []any{60.0, 0.0, 60.0}}, "snowline"},
		"reach":     {map[string]any{"pos": []any{0.0, 50.0, 140.0}}, "reach"},
	} {
		p := &Place{Pose: tc.pose, Solid: solid, Snowline: a.Metrics.Snowline}
		pose, w, why := validateCloud(p)
		if why != tc.why {
			t.Errorf("%s: %q, want %q", name, why, tc.why)
			continue
		}
		if why != "" {
			continue
		}
		pos := pose["pos"].([]float64)
		i, j := gridCell(pos[0]), gridCell(pos[2])
		if cx, cz := worldcore.CellXZ(i, j); len(pose) != 1 || w != 1 || cx != pos[0] || cz != pos[2] || pos[1] != float64(solid[j*worldcore.Grid+i])*worldcore.HeightQ {
			t.Errorf("%s: pose %v weight %v not quantised / y not heightSolid", name, pose, w)
		}
	}
	// the seam's guards, through this toy: a wrong toy name, then a flood from one account
	if _, why := ackOf(t, a.place(visitor("u1", ""), "", "toy-cloud", map[string]any{"pos": []any{0.0, 0.0, 4.0}}, false, time.Now())); why != "toy" {
		t.Errorf("wrong toy: %q", why)
	}
	for i, want := range []string{"", "rate", "rate"} {
		if _, why := ackOf(t, a.place(visitor("u1", ""), "", "toy-clouds", map[string]any{"pos": []any{0.0, 0.0, 4.0}}, false, time.Now().Add(time.Duration(i)*time.Second))); why != want {
			t.Errorf("flood %d: %q, want %q", i, why, want)
		}
	}
}

// TestCloudRain: spawn, drift on the wind, merge on contact, and rain only once 3 distinct accounts joined within the
// hour (W4 verification "Rain needs 3 accounts"), then dissipate.
func TestCloudRain(t *testing.T) {
	a, c := cloudApp(t)
	a.Metrics.Wind = XZ{X: 0.5}
	t0, all := time.Now(), []map[string]any{}
	tick := func(at time.Time) (cl, rain []map[string]any) {
		drain(c)
		toys["toy-clouds"].Tick(a, &Out{app: a, name: "toy-clouds"}, at)
		f := drain(c)
		all = append(all, f...)
		for _, x := range f {
			switch x["kind"] {
			case "cloud":
				cl = append(cl, x)
			case "rain":
				rain = append(rain, x)
			}
		}
		return
	}
	put := func(sub, name string, x, z float64, at time.Time) string {
		t.Helper()
		id, why := ackOf(t, a.place(visitor(sub, name), "", "toy-clouds", map[string]any{"pos": []any{x, 0.0, z}}, name != "", at))
		if why != "" {
			t.Fatal(why)
		}
		return id
	}
	credits := func(f map[string]any) (out []string) {
		for _, h := range f["credits"].([]any) {
			out = append(out, h.(string))
		}
		return
	}

	id := put("u1", "ana_k", 0, 4, t0)
	y0 := a.State.Marks[0].Pose["pos"].([]any)[1].(float64)
	cl, _ := tick(t0)
	if len(cl) != 1 || cl[0]["id"] != id || cl[0]["state"] != "drift" || !slices.Equal(credits(cl[0]), []string{"ana_k"}) {
		t.Fatalf("spawn: %v", cl)
	}
	x0 := cl[0]["pos"].([]any)[0].(float64)
	if y := cl[0]["pos"].([]any)[1].(float64); y != y0+cloudAlt {
		t.Errorf("cloud y %v, want spot %v + %v", y, y0, cloudAlt)
	}
	step := 0.5 * cloudDrift * toyTick.Seconds()
	if cl, _ := tick(t0.Add(toyTick)); len(cl) != 0 {
		t.Errorf("drift under cloudStep re-sent: %v", cl)
	}
	tick(t0.Add(2 * toyTick))
	cl, _ = tick(t0.Add(3 * toyTick))
	if len(cl) != 1 || math.Abs(cl[0]["pos"].([]any)[0].(float64)-(x0+3*step)) > 1e-9 {
		t.Errorf("drift on the wind: %v, want x %v", cl, x0+3*step)
	}

	put("u2", "", 3, 3, t0.Add(time.Minute))
	cl, rain := tick(t0.Add(4 * toyTick))
	if len(cl) != 1 || cl[0]["id"] != id || len(rain) != 0 || !slices.Equal(credits(cl[0]), []string{"ana_k", anonHandle}) {
		t.Fatalf("merge into the older cloud, no rain with 2 accounts: %v %v", cl, rain)
	}
	put("u2", "", 2, -3, t0.Add(2*time.Minute)) // the same account again: still 2
	if cl, rain := tick(t0.Add(5 * toyTick)); len(rain) != 0 || len(cl) != 1 || cl[0]["state"] != "drift" {
		t.Fatalf("2 accounts, 3 marks rained: %v %v", cl, rain)
	}
	if len(a.State.Clouds) != 1 {
		t.Errorf("hello.state.clouds %v", a.State.Clouds)
	}
	put("u3", "dee", 2, -3, t0.Add(3*time.Minute))
	cl, rain = tick(t0.Add(6 * toyTick))
	if len(rain) != 1 || len(cl) != 1 || cl[0]["state"] != "rain" || !slices.Equal(credits(cl[0]), []string{"ana_k", anonHandle, "dee"}) {
		t.Fatalf("3 accounts → rain: %v %v", cl, rain)
	}
	if r := rain[0]; r["r"] != rainR || math.Abs(r["x"].(float64)-cl[0]["pos"].([]any)[0].(float64)) > 1e-9 {
		t.Errorf("rain %v under %v", r, cl[0])
	}
	for k := 1; k < rainTicks; k++ {
		if cl, rain := tick(t0.Add(time.Duration(6+k) * toyTick)); len(rain) != 0 || slices.ContainsFunc(cl, func(f map[string]any) bool { return f["state"] != "rain" }) {
			t.Fatalf("tick %d of rain: %v %v", k, cl, rain)
		}
	}
	cl, _ = tick(t0.Add((6 + rainTicks) * toyTick))
	if len(cl) != 1 || cl[0]["state"] != "gone" || len(a.State.Clouds) != 0 {
		t.Fatalf("dissipates after the rain: %v, state %v", cl, a.State.Clouds)
	}
	if cl, _ := tick(t0.Add((7 + rainTicks) * toyTick)); len(cl) != 0 {
		t.Errorf("respawned: %v", cl)
	}
	validate(t, [][]map[string]any{all})
}

// TestCloudEnds: a cloud goes when its marks are deleted (moderation), after cloudLife, or off the world; a restart
// reseeds from hello.state.clouds without respawning.
func TestCloudEnds(t *testing.T) {
	a, c := cloudApp(t)
	t0 := time.Now()
	tick := func(at time.Time) []map[string]any {
		drain(c)
		toys["toy-clouds"].Tick(a, &Out{app: a, name: "toy-clouds"}, at)
		return drain(c)
	}
	spawn := func(sub string, at time.Time) string {
		id, why := ackOf(t, a.place(visitor(sub, ""), "", "toy-clouds", map[string]any{"pos": []any{0.0, 0.0, 4.0}}, false, at))
		if why != "" {
			t.Fatal(why)
		}
		if f := tick(at); len(f) != 1 || f[0]["id"] != id {
			t.Fatalf("spawn %v", f)
		}
		return id
	}
	admin := &Client{Role: roleAdmin, RoleExp: time.Now().Add(time.Hour), Sub: "adm"}
	id := spawn("u1", t0)
	a.unmark(admin, "", id, t0)
	if f := tick(t0.Add(toyTick)); len(f) != 1 || f[0]["state"] != "gone" {
		t.Errorf("deleted mark: %v", f)
	}

	spawn("u2", t0)
	clouds = nil // a restart: the tick reseeds from hello.state.clouds
	if f := tick(t0.Add(toyTick)); len(f) != 0 || len(clouds) != 1 {
		t.Errorf("restart: frames %v, clouds %d", f, len(clouds))
	}
	if f := tick(t0.Add(cloudLife + time.Minute)); len(f) != 1 || f[0]["state"] != "gone" {
		t.Errorf("past cloudLife: %v", f)
	}

	spawn("u3", t0)
	a.Metrics.Wind = XZ{Z: 1}
	for n := 1; ; n++ { // 0.75 units a tick: off the world (z > 100) within ~130 ticks, the mark still young
		f := tick(t0.Add(time.Second))
		if len(f) > 0 && f[len(f)-1]["state"] == "gone" {
			if z := f[len(f)-1]["pos"].([]any)[2].(float64); z <= 100 || len(clouds) != 0 {
				t.Errorf("gone at z %v, %d clouds left", z, len(clouds))
			}
			break
		}
		if n > 200 {
			t.Fatal("never left the world")
		}
	}
}

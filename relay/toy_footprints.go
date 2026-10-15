package main

// footprints (W4, 03-model-footprints): a click-path on the snow. The pose is {pos: [x, y, z], path: [x, z, …]}: pos goes
// through Spot (quantised, y from heightSolid, above the snowline), path is the walked line, ≤ 8 points within
// fpReach of pos, each quantised to its cell and above the snowline (consecutive repeats collapse). The relay owns the
// weight (06 "footprint weight"): 1 at placement, then Tick erodes it — weather as age (exp(−age/fpLife), age from the
// ULID), rain under a raining toy cloud ×fpRain per tick — and re-emits the mark once it drops a whole 1/fpSteps step, gone
// below fpMin. The client stamps with it; trails where many walk are its additive stamps summing.

import (
	"math"
	"slices"
	"time"
)

const (
	fpReach = 16.0               // m, every path point from pos (a drag on screen, not a hike)
	fpLife  = 2 * 24 * time.Hour // weather: weight = exp(−age/fpLife), so gone (fpMin) after ≈ 4.6 days
	fpRain  = 0.6                // per tick under a raining cloud
	fpRainR = 12.0               // m around a raining cloud. ponytail: world:rain's r is not in state; a fixed radius stands in
	fpSteps = 20                 // weight re-emitted in 0.05 steps: ≤ 20 frames per footprint over its life
	fpMin   = 0.1
)

func init() { RegisterToy("footprints", Toy{Validate: validateFootprints, Tick: tickFootprints}) }

func validateFootprints(p *Place) (map[string]any, float64, string) {
	pos, why := p.Spot("pos")
	if why != "" {
		return nil, 0, why
	}
	l, ok := p.Pose["path"].([]any)
	if !ok || len(l) < 2 || len(l) > 16 || len(l)%2 != 0 {
		return nil, 0, "bad"
	}
	v, _ := p.Vec("path", len(l))
	if v == nil {
		return nil, 0, "bad"
	}
	path := make([]float64, 0, len(v))
	for i := 0; i < len(v); i += 2 {
		x, y, z, ok := p.Ground(v[i], v[i+1])
		switch {
		case !ok || math.Hypot(x-pos[0], z-pos[2]) > fpReach:
			return nil, 0, "reach"
		case y <= p.Snowline:
			return nil, 0, "snowline"
		}
		if n := len(path); n == 0 || path[n-2] != x || path[n-1] != z {
			path = append(path, x, z)
		}
	}
	return map[string]any{"pos": pos, "path": path}, 1, ""
}

func tickFootprints(a *App, o *Out, now time.Time) {
	var rain [][2]float64
	a.mu.Lock()
	for _, c := range a.State.Clouds {
		if c.State == "rain" {
			rain = append(rain, [2]float64{c.Pos[0], c.Pos[2]})
		}
	}
	var fs []Mark
	for _, m := range a.State.Marks {
		if m.Mark == "footprints" {
			fs = append(fs, m)
		}
	}
	a.mu.Unlock()
	for _, m := range fs { // emit with no lock held (validators.go Tick)
		w := min(m.Weight, math.Exp(-float64(now.Sub(ulidTime(m.ID)))/float64(fpLife)))
		if rained(m, rain) {
			w = min(w, m.Weight*fpRain)
		}
		q := math.Ceil(w*fpSteps) / fpSteps
		if w >= fpMin && q >= m.Weight {
			continue
		}
		// an admin delete (or the cap) since the copy: a re-emit would bring the mark back (State.apply appends a new id).
		// ponytail: the window between this check and the emit stays; a deleted-id set in unmark closes it if it ever bites
		a.mu.Lock()
		live := slices.ContainsFunc(a.State.Marks, func(x Mark) bool { return x.ID == m.ID })
		a.mu.Unlock()
		if !live {
			continue
		}
		switch {
		case w < fpMin:
			o.World("mark", map[string]any{"id": m.ID, "mark": m.Mark, "pose": map[string]any{}, "weight": 0.0, "gone": true})
		default: // ponytail: a re-emit moves the mark to the newest end of state.marks (the cap drops it later)
			f := map[string]any{"id": m.ID, "mark": m.Mark, "pose": floats(m.Pose), "weight": q}
			if m.By != "" {
				f["by"] = m.By
			}
			o.World("mark", f)
		}
	}
}

// rained: any path point within fpRainR of a raining cloud.
func rained(m Mark, rain [][2]float64) bool {
	path, _ := floats(m.Pose)["path"].([]float64)
	for _, r := range rain {
		for i := 0; i+1 < len(path); i += 2 {
			if math.Hypot(path[i]-r[0], path[i+1]-r[1]) <= fpRainR {
				return true
			}
		}
	}
	return false
}

// floats: a state pose (decoded JSON: []any) back to the []float64 sanitize's fPose takes.
func floats(pose map[string]any) map[string]any {
	out := map[string]any{}
	for k, x := range pose {
		switch x := x.(type) {
		case []any:
			v := make([]float64, len(x))
			for i, f := range x {
				v[i], _ = f.(float64)
			}
			out[k] = v
		default:
			out[k] = x
		}
	}
	return out
}

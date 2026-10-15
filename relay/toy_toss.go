package main

// toss (03-model-toss): a slingshot throw from `from` to `pos`. pos goes through Place.Spot (quantised, y recomputed
// from heightSolid, above the snowline); from must be in the world (quantised, y recomputed, any height: you can throw
// up onto the snow) and within tossMax of the client's landing point. The client animates from → pos (cosmetic).

import "math"

const tossMax = 24.0 // m, horizontal; the client clamps its slingshot to the same reach (models/toss THROW)

func init() { RegisterToy("toss", Toy{Validate: validateToss}) }

func validateToss(p *Place) (map[string]any, float64, string) {
	pos, why := p.Spot("pos")
	if why != "" {
		return nil, 0, why
	}
	f, ok := p.Vec("from", 3)
	if !ok {
		return nil, 0, "bad"
	}
	to, _ := p.Vec("pos", 3) // Spot accepted it: 3 finite numbers
	fx, fy, fz, ok := p.Ground(f[0], f[2])
	switch {
	case !ok:
		return nil, 0, "reach"
	case math.Hypot(to[0]-f[0], to[2]-f[2]) > tossMax:
		return nil, 0, "throw"
	}
	return map[string]any{"pos": pos, "from": []float64{fx, fy, fz}}, 1, ""
}

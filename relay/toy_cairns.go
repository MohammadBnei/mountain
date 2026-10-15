package main

// cairns (03-model-cairns): one stone per mark, stacked on a flat spot above the snowline. The client settles a
// cannon Box and sends {pos, quat}; the relay keeps only the cell (Place.Spot: x/z quantised, y = heightSolid, reach,
// snowline) and refuses a toppled stone. The stone then settles, like a real one, within cairnNear cells: onto the
// nearest cairn that is not full, else onto the nearest flat cell (slope ≤ cairnSlope) above the snowline; none =
// rejected (slope, or full on a full cairn). Public pose {pos: [x, y, z], quat: [x, y, z, w] (unit)}; the client
// stacks a cell's stones in mark order (models/cairns mirrors the settle).

import (
	"math"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

const (
	cairnSlope = 0.7 // |∇heightSolid| (rise over run): a stone's angle of repose, ≈ 35°. The cap is steep: seed 1 has ≈ 20 such cells above the snowline
	cairnUp    = 0.8 // the stone's up axis · world up (≈ 37° of tilt): below it the stone has toppled
	cairnMax   = 7   // stones per cairn
	cairnNear  = 2   // cells (≈ 3 m) a stone may settle from where it was dropped
)

func init() { RegisterToy("cairns", Toy{Validate: validateCairn}) }

func validateCairn(p *Place) (map[string]any, float64, string) {
	pos, why := p.Spot("pos")
	if why != "" {
		return nil, 0, why
	}
	q, ok := p.Vec("quat", 4)
	if !ok {
		return nil, 0, "bad"
	}
	l := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if !(l > 1e-3) || math.IsInf(l, 0) { // zero, or 1e200² = Inf
		return nil, 0, "bad"
	}
	for i := range q {
		q[i] /= l
	}
	if 1-2*(q[0]*q[0]+q[2]*q[2]) < cairnUp { // R·(0, 1, 0) · (0, 1, 0)
		return nil, 0, "upright"
	}
	stones := map[[2]int]int{} // cell → cairn stones on it
	for _, m := range p.Marks {
		if c, ok := m.Pose["pos"].([]any); m.Mark == "cairns" && ok && len(c) == 3 {
			x, _ := c[0].(float64)
			z, _ := c[2].(float64)
			stones[[2]int{gridCell(x), gridCell(z)}]++
		}
	}
	i, j := gridCell(pos[0]), gridCell(pos[2])
	best, bd := [2]int{-1, -1}, math.Inf(1)
	for dj := -cairnNear; dj <= cairnNear; dj++ {
		for di := -cairnNear; di <= cairnNear; di++ {
			c := [2]int{i + di, j + dj}
			x, z := worldcore.CellXZ(c[0], c[1])
			_, y, _, in := p.Ground(x, z)
			n, d := stones[c], float64(di*di+dj*dj)
			if n > 0 {
				d -= 100 // a cairn first
			}
			if in && n < cairnMax && y > p.Snowline && p.Slope(x, z) <= cairnSlope && d < bd {
				best, bd = c, d
			}
		}
	}
	switch {
	case best[0] < 0 && stones[[2]int{i, j}] >= cairnMax:
		return nil, 0, "full"
	case best[0] < 0:
		return nil, 0, "slope"
	}
	x, y, z, _ := p.Ground(worldcore.CellXZ(best[0], best[1]))
	return map[string]any{"pos": []float64{x, y, z}, "quat": q}, 1, ""
}

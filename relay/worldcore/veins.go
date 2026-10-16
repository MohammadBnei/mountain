package worldcore

import (
	"hash/fnv"
	"math"
)

// Vein is a world:vein payload (01 bus): one gold capsule a→b (world units), radius Thick, in its alert's stratum.
// Ref (W3c src_github: the alert name is in the runbook notes) brightens it; false until then.
type Vein struct {
	FP    string     `json:"fp"`
	A     [3]float64 `json:"a"`
	B     [3]float64 `json:"b"`
	Thick float64    `json:"thick"`
	Layer string     `json:"layer"`
	Ref   bool       `json:"ref"`
}

// VeinMin is the shortest alert that becomes a vein (02: severity ≥ warning and ≥ 10 min).
const VeinMin = 600

// layerY is the altitude band (world y) of each stratum, matching strata's platform bands (proxmox ≤ 4, k8s ≤ 34).
// Bedrock starts at 0, not lower: the rim dives under the sea at y −16 (R6), and a deeper seam would sit in the
// cut face's flooded foot.
var layerY = map[string][2]float64{"bedrock": {0, 4}, "k8s": {6, 32}, "apps": {36, 56}}

// PlaceVein seeds the capsule of alert fp from FNV-1a of fp: a seam on the surface of solid (h.base + h.land) at a
// seeded azimuth and altitude inside its layer's band, running along the contour with a seeded dip, sunk 0.2 m so it
// reads as an inlay. Longer incidents give longer, thicker seams (log of durS / 10 min).
func PlaceVein(fp, layer string, durS int, solid []int16) Vein {
	h := fnv.New32a()
	h.Write([]byte(fp))
	r := mulberry(h.Sum32())
	band, ok := layerY[layer]
	if !ok {
		band = layerY["apps"]
	}
	k := math.Log2(1 + float64(max(durS, 0))/VeinMin)
	th, y := r()*2*math.Pi, band[0]+r()*(band[1]-band[0])
	l, dip := math.Min(24, 8+6*k), (r()-0.5)*0.6
	rad := math.Max(10, surfR(solid, th, y))
	d := l / 2 / rad // half the arc, radians
	end := func(s float64) [3]float64 {
		ye := y + s*dip*l/2
		a := th + s*d
		re := surfR(solid, a, ye)
		x, z := math.Sin(a)*re, math.Cos(a)*re
		return [3]float64{round2(x), round2(heightAt(solid, x, z) - 0.2), round2(z)}
	}
	return Vein{FP: fp, A: end(-1), B: end(1), Thick: round2(math.Min(1, 0.3+0.12*k)), Layer: layer}
}

// surfR walks out from the summit along azimuth th until solid drops below y: the surface radius at altitude y.
func surfR(solid []int16, th, y float64) float64 {
	for r := 0.0; r < 98; r += 0.5 {
		if heightAt(solid, math.Sin(th)*r, math.Cos(th)*r) < y {
			return r
		}
	}
	return 98
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

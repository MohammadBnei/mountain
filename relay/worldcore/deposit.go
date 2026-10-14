package worldcore

import (
	"hash/fnv"
	"math"
)

// Deposit is one rasterised `world:deposit`: a w×h int16 patch (HeightQ steps, added to h.land)
// whose cell (0, 0) is grid cell (X0, Z0). Bus payload: {id, x0, z0, w, h, cells: EncodeI16(Cells)}.
type Deposit struct {
	ID    string  `json:"id"`
	X0    int     `json:"x0"`
	Z0    int     `json:"z0"`
	W     int     `json:"w"`
	H     int     `json:"h"`
	Cells []int16 `json:"-"`
}

// RasterDeposit rasterises the deposit of event id at world (x, z), seeded by FNV-1a of id,
// size = f(heat), on top of solid = h.base + h.land. SIGNATURE IS SEAM; body: W1a lane A.
//
// A seeded ellipse lobe (radius 4–12 m, peak 0.8–3 m with heat ∈ [0, 1]) shifted a third of its
// radius downhill on solid, so the new land sits where the lava would pool.
func RasterDeposit(id string, x, z, heat float64, solid []int16) Deposit {
	h := fnv.New32a()
	h.Write([]byte(id))
	r := mulberry(h.Sum32())
	heat = math.Max(0, math.Min(1, heat))
	rad, peak := 4+8*heat, 0.8+2.2*heat
	a, b := rad*(0.75+0.5*r()), rad*(0.75+0.5*r())
	rot := r() * math.Pi
	x, z = x+(r()-0.5)*2, z+(r()-0.5)*2
	if gx, gz := grad(solid, x, z); gx != 0 || gz != 0 {
		g := math.Hypot(gx, gz)
		x, z = x-gx/g*rad/3, z-gz/g*rad/3
	}
	ext := math.Max(a, b)
	i0, i1 := cell(x-ext), cell(x+ext)
	j0, j1 := cell(z-ext), cell(z+ext)
	d := Deposit{ID: id, X0: i0, Z0: j0, W: i1 - i0 + 1, H: j1 - j0 + 1}
	d.Cells = make([]int16, d.W*d.H)
	cs, sn := math.Cos(rot), math.Sin(rot)
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			cx, cz := CellXZ(i, j)
			u, v := (cx-x)*cs+(cz-z)*sn, -(cx-x)*sn+(cz-z)*cs
			jit := 0.85 + 0.3*r() // drawn for every cell so the stream stays aligned
			if q := (u/a)*(u/a) + (v/b)*(v/b); q < 1 {
				d.Cells[(j-j0)*d.W+i-i0] = ToQ(peak * (1 - q) * (1 - q) * jit)
			}
		}
	}
	return d
}

// cell is the grid index of world coordinate c, clamped to the grid.
func cell(c float64) int { return max(0, min(Grid-1, int(math.Round((c+100)/Cell)))) }

// heightAt is solid (h.base + h.land, HeightQ steps) at the cell nearest world (x, z), in world units.
func heightAt(solid []int16, x, z float64) float64 {
	return float64(solid[cell(z)*Grid+cell(x)]) * HeightQ
}

// grad is the central-difference gradient of solid at (x, z), world units per world unit.
func grad(solid []int16, x, z float64) (float64, float64) {
	if len(solid) != Grid*Grid {
		return 0, 0
	}
	return (heightAt(solid, x+Cell, z) - heightAt(solid, x-Cell, z)) / (2 * Cell),
		(heightAt(solid, x, z+Cell) - heightAt(solid, x, z-Cell)) / (2 * Cell)
}

// Solid returns h.base + h.land (clamped).
func Solid(base, land []int16) []int16 {
	s := make([]int16, len(base))
	for c := range base {
		s[c] = AddQ(base[c], land[c])
	}
	return s
}

// AddQ adds two int16 heights with clamping.
func AddQ(a, b int16) int16 {
	return int16(max(math.MinInt16, min(math.MaxInt16, int32(a)+int32(b))))
}

// Site is a world:site payload: where an allowlisted app's land grows.
type Site struct {
	App, Group string
	X, Z, Y    float64
	Tier       int
}

// PlaceSite places app deterministically: azimuth from FNV-1a of the name, ring by tier
// (0 = platform, upper slopes; 1 = user, mid slopes), y from solid.
// ponytail: hash azimuths can land close together; spread them if two sites ever overlap visibly.
func PlaceSite(app, group string, solid []int16) Site {
	h := fnv.New32a()
	h.Write([]byte(app))
	r := mulberry(h.Sum32())
	tier, rad := 1, 40.0
	if group == "platform" {
		tier, rad = 0, 22
	}
	th, rad := r()*2*math.Pi, rad+(r()-0.5)*8
	x, z := math.Round(math.Sin(th)*rad*100)/100, math.Round(math.Cos(th)*rad*100)/100
	return Site{App: app, X: x, Z: z, Y: math.Round(heightAt(solid, x, z)*100) / 100, Tier: tier, Group: group}
}

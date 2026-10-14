// Package worldcore is the only implementation of the persisted world (06-relay).
//
// base.go is SEAM-OWNED (W1a orchestrator): Base(seed) and the wire encoding.
// Lanes add their own files (deposit.go, sites, veins, ...) and never edit this one.
//
// Grids: Grid×Grid int16, row-major (z outer, x inner); cell (i, j) sits at world
// (-100 + i*Cell, -100 + j*Cell). One int16 step = HeightQ world units (units.json).
package worldcore

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	Grid    = 128
	Extent  = 200.0
	Cell    = Extent / Grid
	HeightQ = 0.01 // world units per int16 step; ponytail: cm, int16 mm (±32 m) cannot hold the ±70 m massif
)

// Cave is a Go-seeded cave mouth on the walkway's inner cut wall: (x, z, y) is the
// walkway bench centre in front of it, yaw the outward (valley-facing) direction.
type Cave struct {
	X    float64 `json:"x"`
	Z    float64 `json:"z"`
	Y    float64 `json:"y"`
	Yaw  float64 `json:"yaw"`
	Seed uint32  `json:"seed"`
}

// World is the terrain base: h.base (int16, HeightQ steps), the walkway bench mask (0..255) and caves.
type World struct {
	Seed  uint32
	Base  []int16
	Bench []uint8
	Caves []Cave
	HMax  float64 // highest h.base cell, world units (snowline default = 0.8 × HMax)
	Grade float64 // walkway grade (rise / run)
}

// CellXZ returns the world position of cell (i, j).
func CellXZ(i, j int) (float64, float64) { return -100 + float64(i)*Cell, -100 + float64(j)*Cell }

// ToQ quantises world units to an int16 height (clamped).
func ToQ(h float64) int16 {
	return int16(math.Max(math.MinInt16, math.Min(math.MaxInt16, math.Round(h/HeightQ))))
}

// EncodeI16 is the wire encoding of an int16 grid: base64 of little-endian int16.
func EncodeI16(v []int16) string {
	b := make([]byte, 2*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(x))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeI16 inverts EncodeI16.
func DecodeI16(s string) ([]int16, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b)%2 != 0 {
		return nil, fmt.Errorf("worldcore: bad int16 grid (%v, %d bytes)", err, len(b))
	}
	v := make([]int16, len(b)/2)
	for i := range v {
		v[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return v, nil
}

func mulberry(a uint32) func() float64 {
	return func() float64 {
		a += 0x6d2b79f5
		t := a
		t = (t ^ t>>15) * (1 | t)
		t = (t + (t^t>>7)*(61|t)) ^ t
		return float64(t^t>>14) / 4294967296
	}
}

func smooth(a, b, v float64) float64 {
	t := math.Max(0, math.Min(1, (v-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// simplex is 2D simplex noise with a seeded permutation (three.js SimplexNoise shape).
type simplex [512]uint8

var grad3 = [12][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}, {1, 0}, {-1, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {0, 1}, {0, -1}}

func newSimplex(r func() float64) *simplex {
	var s simplex
	for i := 0; i < 256; i++ {
		s[i] = uint8(r() * 256)
		s[i+256] = s[i]
	}
	return &s
}

func (p *simplex) noise(x, y float64) float64 {
	const f2, g2 = 0.36602540378443864676, 0.21132486540518711775 // (√3−1)/2, (3−√3)/6
	s := (x + y) * f2
	i, j := math.Floor(x+s), math.Floor(y+s)
	t := (i + j) * g2
	x0, y0 := x-(i-t), y-(j-t)
	i1, j1 := 0.0, 1.0
	if x0 > y0 {
		i1, j1 = 1, 0
	}
	x1, y1 := x0-i1+g2, y0-j1+g2
	x2, y2 := x0-1+2*g2, y0-1+2*g2
	ii, jj := int(i)&255, int(j)&255
	corner := func(x, y float64, g int) float64 {
		t := 0.5 - x*x - y*y
		if t < 0 {
			return 0
		}
		t *= t
		return t * t * (grad3[g%12][0]*x + grad3[g%12][1]*y)
	}
	return 70 * (corner(x0, y0, int(p[ii+int(p[jj])])) +
		corner(x1, y1, int(p[ii+int(i1)+int(p[jj+int(j1)])])) +
		corner(x2, y2, int(p[ii+1+int(p[jj+1])])))
}

// Base generates h.base for a seed (06-relay): broad massif + soft ridges + terraces,
// a continuous spiral walkway carved as a bench, 3–5 caves, and the rim drop.
// Port of plan/style/board.html (the approved styleboard), plus the softer rim (09 Freeze item 2).
func Base(seed uint32) *World {
	nz := newSimplex(mulberry(seed))
	h0 := func(x, z float64) float64 {
		rg, amp, f := 0.0, 1.0, 0.016
		for o := 0; o < 3; o++ {
			n := 1 - math.Abs(nz.noise(x*f, z*f))
			rg += n * n * amp
			amp *= 0.45
			f *= 2
		}
		r := math.Hypot(x*0.8, z) / 96
		fall := math.Pow(math.Max(0, 1-r*r), 1.2)
		h := (54*math.Exp(-r*r*3) + rg*11 + nz.noise(x*0.05, z*0.05)*2) * fall
		const step = 7.0
		b := math.Floor(h/step) * step
		tt := (h - b) / step
		h = h*0.35 + (b+step*tt*tt*tt*(3-2*tt))*0.65 // terraces, 65 % blend
		// Rim drop (09 Freeze item 2): the two ramps overlap (70–104, 96–141) so there is no flat
		// shelf at r = 100 and no shoulder at r = 78; the massif rolls into the valley haze.
		rr := math.Hypot(x, z)
		h -= 34*smooth(70, 104, rr) + 30*smooth(96, 141, rr)
		return h - 3
	}

	w := &World{Seed: seed, Base: make([]int16, Grid*Grid), Bench: make([]uint8, Grid*Grid)}
	hb := make([]float64, Grid*Grid)
	hmax0 := 0.0
	for j := 0; j < Grid; j++ {
		for i := 0; i < Grid; i++ {
			x, z := CellXZ(i, j)
			hb[j*Grid+i] = h0(x, z)
			hmax0 = math.Max(hmax0, hb[j*Grid+i])
		}
	}

	// Spiral walkway: ≈ 2 turns, constant grade, radius follows the altitude contour, smoothed,
	// snapped back where smoothing would cut > 4 m into a ridge (never a tunnel).
	type pt struct{ th, y, r, x, z, wall float64 }
	rng := mulberry(seed ^ 0x9e3779b9)
	const n, turns, yFoot, bw, edge = 700, 2.0, 3.0, 2.6, 1.4
	theta0, yTop := rng()*2*math.Pi, 0.6*hmax0
	rAt := func(th, y float64) float64 {
		for r := 98.0; r > 2; r -= 0.5 {
			if h0(math.Sin(th)*r, math.Cos(th)*r) >= y {
				return r
			}
		}
		return 2
	}
	S := make([]pt, n)
	smoothR := func(win, passes int) {
		r := make([]float64, n)
		for p := 0; p < passes; p++ {
			for k := range S {
				r[k] = S[k].r
			}
			for k := range S {
				a, c := 0.0, 0.0
				for d := -win; d <= win; d++ {
					if k+d >= 0 && k+d < n {
						a += r[k+d]
						c++
					}
				}
				S[k].r = a / c
			}
		}
	}
	for k := range S {
		t := float64(k) / (n - 1)
		S[k].th = theta0 + t*turns*2*math.Pi
		S[k].y = yFoot + t*(yTop-yFoot)
		S[k].r = rAt(S[k].th, S[k].y)
	}
	smoothR(12, 3)
	for it := 0; it < 2; it++ {
		acc := make([]float64, n)
		for k := 1; k < n; k++ {
			a, b := S[k-1], S[k]
			acc[k] = acc[k-1] + math.Hypot(math.Sin(b.th)*b.r-math.Sin(a.th)*a.r, math.Cos(b.th)*b.r-math.Cos(a.th)*a.r)
		}
		L := acc[n-1]
		for k := range S {
			S[k].y = yFoot + acc[k]/L*(yTop-yFoot)
			S[k].r = rAt(S[k].th, S[k].y)
		}
		smoothR(12, 3)
		w.Grade = (yTop - yFoot) / L
	}
	for k := range S {
		if h0(math.Sin(S[k].th)*S[k].r, math.Cos(S[k].th)*S[k].r)-S[k].y > 4 {
			S[k].r = rAt(S[k].th, S[k].y) + 1
		}
	}
	smoothR(4, 1)
	for k := range S {
		s := &S[k]
		s.x, s.z = math.Sin(s.th)*s.r, math.Cos(s.th)*s.r
		s.wall = h0(s.x*0.9, s.z*0.9) - s.y
	}

	// Carve the bench: flat at ±bw, blended to the slope over edge; bench mask for the trail colour.
	for j := 0; j < Grid; j++ {
		for i := 0; i < Grid; i++ {
			x, z := CellXZ(i, j)
			best, bs := math.Inf(1), 0
			for k := range S {
				if d := (x-S[k].x)*(x-S[k].x) + (z-S[k].z)*(z-S[k].z); d < best {
					best, bs = d, k
				}
			}
			d, c := math.Sqrt(best), j*Grid+i
			if d > bw+edge {
				continue
			}
			f := smooth(bw, bw+edge, d)
			hb[c] = S[bs].y + (hb[c]-S[bs].y)*f
			w.Bench[c] = uint8(math.Round(255 * (1 - smooth(bw-0.6, bw, d))))
		}
	}

	// Caves: 3–5, where the inner cut wall is ≥ 7 m tall.
	cr := mulberry(seed ^ 77)
	for k := 40; k < n-40 && len(w.Caves) < 5; k += 70 + int(cr()*50) {
		b := k
		for m := k; m < k+50 && m < n; m++ {
			if S[m].wall > S[b].wall {
				b = m
			}
		}
		if s := S[b]; s.wall > 7 {
			w.Caves = append(w.Caves, Cave{X: s.x, Z: s.z, Y: s.y, Yaw: math.Atan2(s.x, s.z), Seed: seed*1000 + uint32(b)})
		}
	}

	for c, h := range hb {
		w.Base[c] = ToQ(h)
		w.HMax = math.Max(w.HMax, float64(w.Base[c])*HeightQ)
	}
	return w
}

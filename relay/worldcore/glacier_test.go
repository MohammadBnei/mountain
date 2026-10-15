package worldcore

import (
	"fmt"
	"math"
	"testing"
)

func TestGlacier(t *testing.T) {
	g := NewGlacier().Snow(25)
	if g.Mass != 0.6 {
		t.Fatalf("snow: mass %v", g.Mass)
	}
	g = g.Advance("wird").Advance("wird").Advance("mountain")
	if fmt.Sprint(g.Tongues) != "[{wird 3} {mountain 1.5}]" {
		t.Fatalf("advance: %v", g.Tongues)
	}
	g, ok := g.Calve("wird")
	if _, again := g.Calve("wird"); !ok || again || len(g.Tongues) != 1 {
		t.Fatalf("calve: %v %v %v", ok, again, g.Tongues)
	}
	for range 14 {
		g = g.Melt()
	}
	if len(g.Tongues) != 0 || g.Mass >= 0.6 {
		t.Fatalf("melt: %v", g)
	}
	g, _ = g.AddAsh("2026-09-01")
	g, _ = g.AddAsh("2026-10-01")
	if _, dup := g.AddAsh("2026-10-01"); dup || fmt.Sprint(g.Ash) != "[2026-10-01 2026-09-01]" {
		t.Fatalf("ash bands: %v", g.Ash)
	}
	if Snowline(100, 0.5) != 80 || Snowline(100, 1) >= Snowline(100, 0) {
		t.Fatal("snowline: 0.8 hmax at mass .5, lower with more mass")
	}
}

func TestAshAndVeinLists(t *testing.T) {
	var l []Ash
	for d := range 40 {
		l, _ = AddAshLine(l, Ash{Day: fmt.Sprintf("2026-%02d-%02d", 8+d/28, d%28+1)})
	}
	if len(l) != AshMax || l[0].Day != "2026-09-12" {
		t.Fatalf("ash lines: %d, first %v", len(l), l[0])
	}
	if _, ok := AddAshLine(l, Ash{Day: "2026-09-12"}); ok {
		t.Fatal("a repeated day + title is not new")
	}
	var v []Vein
	for i := range 40 {
		v = AddVein(v, Vein{FP: fmt.Sprintf("%016x", i), Thick: 0.3})
	}
	v = AddVein(v, Vein{FP: fmt.Sprintf("%016x", 39), Thick: 0.2, A: [3]float64{1}})
	if len(v) != VeinsMax || v[0].FP != fmt.Sprintf("%016x", 8) || v[31].Thick != 0.3 || v[31].A[0] != 0 {
		t.Fatalf("veins: %d, first %s, refire %+v", len(v), v[0].FP, v[31])
	}
}

func TestPlaceVein(t *testing.T) {
	w := Base(1)
	solid := Solid(w.Base, make([]int16, len(w.Base)))
	a, b := PlaceVein("0123456789abcdef", "k8s", 900, solid), PlaceVein("0123456789abcdef", "k8s", 900, solid)
	if a != b {
		t.Fatal("same fp gave different veins")
	}
	for _, layer := range []string{"bedrock", "k8s", "apps"} {
		for i := range 20 {
			v := PlaceVein(fmt.Sprintf("%016x", i*7919), layer, 600*(1+i), solid)
			mid := (v.A[1] + v.B[1]) / 2
			if mid < layerY[layer][0]-8 || mid > layerY[layer][1]+8 || v.Thick < 0.3 || v.Thick > 1 {
				t.Fatalf("%s vein %d: y %v thick %v", layer, i, mid, v.Thick)
			}
			// both ends sit on the surface (sunk 0.2 m)
			for _, p := range [][3]float64{v.A, v.B} {
				if d := heightAt(solid, p[0], p[2]) - p[1]; math.Abs(d-0.2) > 0.01 {
					t.Fatalf("%s vein %d end %v is %v m off the surface", layer, i, p, d)
				}
			}
		}
	}
	if long := PlaceVein("0123456789abcdef", "k8s", 6*3600, solid); long.Thick <= a.Thick {
		t.Fatal("a longer incident gives a thicker vein")
	}
}

func TestAshTrimmedIsNoChange(t *testing.T) {
	g := Glacier{}
	var l []Ash
	for i := range AshMax {
		g, _ = g.AddAsh(fmt.Sprintf("2026-10-%02d", i+1))
		l, _ = AddAshLine(l, Ash{Day: fmt.Sprintf("2026-10-%02d", i+1)})
	}
	if _, ok := g.AddAsh("2026-01-01"); ok {
		t.Fatal("AddAsh: a day older than a full list reported a change")
	}
	if _, ok := AddAshLine(l, Ash{Day: "2026-01-01"}); ok {
		t.Fatal("AddAshLine: a day older than a full list reported a change")
	}
}

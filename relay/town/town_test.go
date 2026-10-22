package town

import (
	"math"
	"testing"
)

// TestRadii recomputes Radii from the hosts' hardware (infra-bootstrap ARCHITECTURE.md: threads, RAM GB, disk GB).
func TestRadii(t *testing.T) {
	hw := map[string][3]float64{"h1": {12, 32, 2048}, "h2": {6, 32, 476}, "h3": {4, 15, 238}}
	var mx [3]float64
	for _, v := range hw {
		for i := range v {
			mx[i] = math.Max(mx[i], v[i])
		}
	}
	score := map[string]float64{}
	lo := math.Inf(1)
	for h, v := range hw {
		score[h] = 0.45*v[0]/mx[0] + 0.40*v[1]/mx[1] + 0.15*v[2]/mx[2]
		lo = math.Min(lo, score[h])
	}
	for h, s := range score {
		if r := 4 + int(math.Round(3*(s-lo)/(1-lo))); r != Radii[h] {
			t.Errorf("%s: score %.3f → radius %d, Radii says %d", h, s, r, Radii[h])
		}
	}
}

func TestLayout(t *testing.T) {
	l := Get()
	if len(l.Islands) != 3 || len(l.Bridges) != 3 {
		t.Fatalf("islands %d bridges %d", len(l.Islands), len(l.Bridges))
	}
	for _, is := range l.Islands {
		r := is.Radius
		if r != Radii[is.Host] || len(is.Tiles) != 3*r*(r+1)+1 || len(Hexes(is.Host)) != len(is.Tiles) {
			t.Fatalf("%s radius %d tiles %d", is.Host, r, len(is.Tiles))
		}
		n := map[byte]int{}
		for i := range is.Tiles {
			n[is.Tiles[i]]++
		}
		// 2 bridges per island: 2 spokes of r−1 road hexes, 2 heads on the 6r-hex rim
		grass := 3*(r-1)*r - 2*(r-1)
		if n[Square] != 1 || n[Road] != 2*(r-1) || n[Bridge] != 2 || n[Meadow] != 6*r-2 || n[Grass] != grass {
			t.Errorf("%s tiles %v (want grass %d)", is.Host, n, grass)
		}
	}
	// bridge heads face each other across a strait; the widest pair (h1–h2) is `channel` apart
	for _, b := range l.Bridges {
		a := Centre[b.A].Add(Hex{b.AQ, b.AR})
		c := Centre[b.B].Add(Hex{b.BQ, b.BR})
		want := Side - Radii[b.A] - Radii[b.B]
		if d := (Hex{a.Q - c.Q, a.R - c.R}).Ring(); d != want || d < channel {
			t.Errorf("%s-%s heads %v %v are %d apart, want %d", b.A, b.B, a, c, d, want)
		}
		if Tile(b.A, Hex{b.AQ, b.AR}) != Bridge || Tile(b.B, Hex{b.BQ, b.BR}) != Bridge {
			t.Errorf("%s-%s: heads are not bridge tiles", b.A, b.B)
		}
	}
	// islands never overlap: every pair of centres is Side apart, more than the two radii
	for i, a := range Hosts {
		for _, b := range Hosts[i+1:] {
			ca, cb := Centre[a], Centre[b]
			if d := (Hex{ca.Q - cb.Q, ca.R - cb.R}).Ring(); d != Side || d <= Radii[a]+Radii[b] {
				t.Errorf("%s-%s centres %d apart", a, b, d)
			}
		}
	}
}

func TestSlot(t *testing.T) {
	x, ok := Slot("wird", "h1", "inn", nil, nil)
	y, _ := Slot("wird", "h1", "inn", nil, nil)
	if !ok || x != y || Tile("h1", x) != Grass || x.Ring() != 2 || !onSpoke("h1", x) {
		t.Fatalf("a shop takes the street front of ring 2: %v %v %v", x, y, ok)
	}
	if g, _ := Slot("craneyard", "h1", "craneyard", nil, nil); g.Ring() != 1 {
		t.Errorf("public building on ring %d, want 1", g.Ring())
	}
	if g, _ := Slot("gate", "h1", "gate", nil, nil); g.Ring() != Radii["h1"]-2 {
		t.Errorf("defense on ring %d, want the town's edge %d", g.Ring(), Radii["h1"]-2)
	}
	if g, _ := Slot("forge", "h2", "forge", nil, nil); g.Ring() != Radii["h2"]-1 {
		t.Errorf("industry on ring %d, want the outskirts %d", g.Ring(), Radii["h2"]-1)
	}
	// taken hexes are skipped, avoided ones used last, and a full island says so
	taken := map[Hex]bool{x: true}
	if z, _ := Slot("wird", "h1", "inn", taken, nil); z == x {
		t.Error("slot reused a taken hex")
	}
	avoid := map[Hex]bool{}
	for _, h := range Hexes("h3") {
		if Tile("h3", h) == Grass {
			avoid[h] = true
		}
	}
	if z, ok := Slot("wird", "h3", "inn", nil, avoid); !ok || !avoid[z] {
		t.Error("an island with only avoided hexes must still give one")
	}
	if _, ok := Slot("wird", "h3", "inn", avoid, nil); ok {
		t.Error("a full island must refuse")
	}
	r := Radii["h2"]
	if !Plantable("h2", Hex{0, r}) || Plantable("h2", Hex{0, 0}) || Plantable("h2", Hex{-1, 0}) || Plantable("h2", Hex{9, 9}) {
		t.Error("plantable: meadow yes; square, road, off-island no")
	}
	if FamilyOf("keep") != Defense || FamilyOf("nonsense") != Houses {
		t.Error("families")
	}
}

package town

import "testing"

func TestLayout(t *testing.T) {
	l := Get()
	if len(Hexes) != 61 || len(l.Islands) != 3 || len(l.Bridges) != 3 {
		t.Fatalf("hexes %d islands %d bridges %d", len(Hexes), len(l.Islands), len(l.Bridges))
	}
	for _, is := range l.Islands {
		n := map[byte]int{}
		for i := range is.Tiles {
			n[is.Tiles[i]]++
		}
		// 2 bridges per island: 2 spokes of 3 road hexes, 2 heads on the 24-hex rim, 36 − 6 grass
		if n[Square] != 1 || n[Road] != 6 || n[Bridge] != 2 || n[Meadow] != 22 || n[Grass] != 30 {
			t.Errorf("%s tiles %v", is.Host, n)
		}
	}
	// bridge heads face each other across a 3-hex strait (global hex distance 4 along the axis)
	for _, b := range l.Bridges {
		a := Centre[b.A].Add(Hex{b.AQ, b.AR})
		c := Centre[b.B].Add(Hex{b.BQ, b.BR})
		if d := (Hex{a.Q - c.Q, a.R - c.R}).Ring(); d != 4 {
			t.Errorf("%s-%s heads %v %v are %d apart", b.A, b.B, a, c, d)
		}
		if Tile(b.A, Hex{b.AQ, b.AR}) != Bridge || Tile(b.B, Hex{b.BQ, b.BR}) != Bridge {
			t.Errorf("%s-%s: heads are not bridge tiles", b.A, b.B)
		}
	}
	// islands never overlap: every pair of centres is 12 apart (> 2·Radius + 1)
	for i, a := range Hosts {
		for _, b := range Hosts[i+1:] {
			ca, cb := Centre[a], Centre[b]
			if d := (Hex{ca.Q - cb.Q, ca.R - cb.R}).Ring(); d != 12 {
				t.Errorf("%s-%s centres %d apart", a, b, d)
			}
		}
	}
}

func TestSlot(t *testing.T) {
	x, ok := Slot("wird", "h1", false, nil, nil)
	y, _ := Slot("wird", "h1", false, nil, nil)
	if !ok || x != y || Tile("h1", x) != Grass || x.Ring() != 2 {
		t.Fatalf("slot %v %v %v", x, y, ok)
	}
	if g, _ := Slot("gate", "h1", true, nil, nil); g.Ring() != 1 {
		t.Errorf("inner slot on ring %d", g.Ring())
	}
	// taken hexes are skipped, avoided ones used last, and a full island says so
	taken := map[Hex]bool{x: true}
	if z, _ := Slot("wird", "h1", false, taken, nil); z == x {
		t.Error("slot reused a taken hex")
	}
	avoid := map[Hex]bool{}
	for _, h := range Hexes {
		if Tile("h1", h) == Grass {
			avoid[h] = true
		}
	}
	if z, ok := Slot("wird", "h1", false, nil, avoid); !ok || !avoid[z] {
		t.Error("an island with only avoided hexes must still give one")
	}
	if _, ok := Slot("wird", "h1", false, avoid, nil); ok {
		t.Error("a full island must refuse")
	}
	if !Plantable("h2", Hex{0, 4}) || Plantable("h2", Hex{0, 0}) || Plantable("h2", Hex{-1, 0}) || Plantable("h2", Hex{9, 9}) {
		t.Error("plantable: meadow yes; square, road, off-island no")
	}
}

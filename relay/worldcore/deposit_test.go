package worldcore

import "testing"

func TestDepositDeterministic(t *testing.T) {
	w := Base(1)
	solid := Solid(w.Base, make([]int16, len(w.Base)))
	sum := func(d Deposit) (n int, max int16) {
		for _, c := range d.Cells {
			if c < 0 {
				t.Fatalf("negative cell %d", c)
			}
			if c > 0 {
				n++
			}
			if c > max {
				max = c
			}
		}
		return
	}
	a, b := RasterDeposit("01JA0000000000000000000000", 10, -20, 0.5, solid), RasterDeposit("01JA0000000000000000000000", 10, -20, 0.5, solid)
	if EncodeI16(a.Cells) != EncodeI16(b.Cells) || a.X0 != b.X0 || a.Z0 != b.Z0 {
		t.Fatal("same id gave different deposits")
	}
	if c := RasterDeposit("01JA0000000000000000000001", 10, -20, 0.5, solid); EncodeI16(c.Cells) == EncodeI16(a.Cells) {
		t.Fatal("different id gave the same deposit")
	}
	if len(a.Cells) != a.W*a.H || a.W < 1 || a.H < 1 {
		t.Fatalf("shape %dx%d, %d cells", a.W, a.H, len(a.Cells))
	}
	cold, _ := sum(RasterDeposit("x", 10, -20, 0, solid))
	hot, peak := sum(RasterDeposit("x", 10, -20, 1, solid))
	if hot <= cold || peak < ToQ(1) {
		t.Fatalf("heat must grow the deposit: cold %d cells, hot %d cells, peak %d", cold, hot, peak)
	}
	// at the world edge the patch is clipped to the grid
	e := RasterDeposit("edge", 99.9, 99.9, 1, solid)
	if e.X0+e.W > Grid || e.Z0+e.H > Grid {
		t.Fatalf("edge deposit leaves the grid: %+v", e)
	}
}

func TestPlaceSite(t *testing.T) {
	w := Base(1)
	solid := Solid(w.Base, make([]int16, len(w.Base)))
	p, u := PlaceSite("mountain", "platform", solid), PlaceSite("wird", "user", solid)
	if p != PlaceSite("mountain", "platform", solid) {
		t.Fatal("site placement not deterministic")
	}
	if p.Tier != 0 || u.Tier != 1 || p.Y <= u.Y-20 || p.Y < 5 {
		t.Fatalf("tiers: %+v %+v", p, u)
	}
}

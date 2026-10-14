package worldcore

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/MohammadBnei/mountain"
)

func TestBaseDeterministic(t *testing.T) {
	a, b, c := Base(7), Base(7), Base(8)
	if EncodeI16(a.Base) != EncodeI16(b.Base) || !bytes.Equal(a.Bench, b.Bench) {
		t.Fatal("same seed gave different bytes")
	}
	if EncodeI16(a.Base) == EncodeI16(c.Base) {
		t.Fatal("different seeds gave the same terrain")
	}
	for _, s := range []uint32{1, 7, 8, 42} {
		w := Base(s)
		if len(w.Caves) < 3 || len(w.Caves) > 5 {
			t.Errorf("seed %d: %d caves, want 3..5", s, len(w.Caves))
		}
		n := 0
		for _, v := range w.Bench {
			if v > 128 {
				n++
			}
		}
		if n < 200 {
			t.Errorf("seed %d: walkway bench covers only %d cells", s, n)
		}
		if w.HMax < 35 || w.HMax > 80 {
			t.Errorf("seed %d: summit %.1f m outside the massif range", s, w.HMax)
		}
	}
}

func TestEncodingRoundTrip(t *testing.T) {
	in := []int16{0, 1, -1, 32767, -32768, 1234}
	out, err := DecodeI16(EncodeI16(in))
	if err != nil || len(out) != len(in) {
		t.Fatal(err)
	}
	for i := range in {
		if in[i] != out[i] {
			t.Fatalf("cell %d: %d != %d", i, out[i], in[i])
		}
	}
	// little-endian: 1 encodes as 01 00
	if EncodeI16([]int16{1}) != "AQA=" {
		t.Fatal("not little-endian")
	}
	if ToQ(1e9) != 32767 || ToQ(-1e9) != -32768 || ToQ(1.234) != 123 {
		t.Fatal("ToQ clamp/round")
	}
}

func TestUnits(t *testing.T) {
	var u struct {
		GRID     int
		EXTENT   float64
		HEIGHT_Q float64
	}
	if err := json.Unmarshal(mountain.Units, &u); err != nil {
		t.Fatal(err)
	}
	if u.GRID != Grid || u.EXTENT != Extent || u.HEIGHT_Q != HeightQ {
		t.Fatalf("units.json %+v disagrees with worldcore", u)
	}
}

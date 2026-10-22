package main

// The town's visitor toy (R7): plant something on a free hex of an island. What grows depends on the real sun over
// Paris when the relay accepts it (sun.go, server side: the client only previews it): a tree by day, a flag at
// dawn and dusk, a lantern at night.
//
//	client pose {h: 1..3, q, r}   the island (h1..h3 as a number: poses stay numeric) and a local hex on it
//	public pose {h, q, r, item}   item 0 tree | 1 flag | 2 lantern
//
// Rejects: bad (not whole numbers) | reach (no such island or hex) | road (the square, a road or a bridge head)
// | plot (a building stands there) | taken (another plant is there).

import (
	"math"

	"github.com/MohammadBnei/mountain/relay/town"
)

func init() { RegisterToy("plant", Toy{Validate: validatePlant}) }

// Int is the whole number at key k of the client pose (|n| ≤ 64).
func (p *Place) Int(k string) (int, bool) {
	f, ok := p.Num(k)
	if !ok || f != math.Trunc(f) || math.Abs(f) > 64 {
		return 0, false
	}
	return int(f), true
}

func validatePlant(p *Place) (map[string]any, float64, string) {
	h, ok1 := p.Int("h")
	q, ok2 := p.Int("q")
	r, ok3 := p.Int("r")
	if !ok1 || !ok2 || !ok3 {
		return nil, 0, "bad"
	}
	if h < 1 || h > len(town.Hosts) {
		return nil, 0, "reach"
	}
	host, x := town.Hosts[h-1], town.Hex{Q: q, R: r}
	switch town.Tile(host, x) {
	case 0:
		return nil, 0, "reach"
	case town.Square, town.Road, town.Bridge:
		return nil, 0, "road"
	}
	for _, pl := range p.Plots {
		if pl.Host == host && pl.hex() == x {
			return nil, 0, "plot"
		}
	}
	if _, ok := plantAt(p.Marks)[host][x]; ok {
		return nil, 0, "taken"
	}
	return map[string]any{"h": h, "q": q, "r": r, "item": plantItem(sunNow())}, 1, ""
}

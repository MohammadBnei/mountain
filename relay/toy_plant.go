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
//
// Room (R7.1): plants wilt after a week (plots.go wilt). When an island has no free hex left, its oldest plant gives
// way: planting on that plant's hex replaces it (Place.Evict); every other planted hex is still taken.

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
	plants := plantAt(p.Marks)[host]
	if m, ok := plants[x]; ok {
		if !islandFull(host, p.Plots, plants) || oldestPlant(p.Marks, h) != m.ID {
			return nil, 0, "taken"
		}
		p.Evict = m.ID
	}
	return map[string]any{"h": h, "q": q, "r": r, "item": plantItem(sunNow())}, 1, ""
}

// islandFull: no plantable hex of host's island is free of plots and plants.
func islandFull(host string, plots []Plot, plants map[town.Hex]Mark) bool {
	built := map[town.Hex]bool{}
	for _, pl := range plots {
		if pl.Host == host {
			built[pl.hex()] = true
		}
	}
	for _, x := range town.Hexes(host) {
		if _, planted := plants[x]; town.Plantable(host, x) && !built[x] && !planted {
			return false
		}
	}
	return true
}

// oldestPlant is the id of the oldest active plant on island h (marks are oldest first).
func oldestPlant(marks []Mark, h int) string {
	for _, m := range marks {
		if m.Mark == "plant" && poseInt(m.Pose["h"]) == h {
			return m.ID
		}
	}
	return ""
}

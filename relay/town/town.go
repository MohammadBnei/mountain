// Package town is the live town's geometry (R7): three hex islands, one per Proxmox host bucket h1..h3, joined by
// bridges. The relay owns it so Go and the page never disagree about which hex is what: hello.town carries the tile
// map and the page only renders it. Pure and deterministic, no I/O.
//
// Hexes are pointy-top axial (q, r), the pi-lab kit.hex grid: world x = 2(q + r/2), z = √3·r (flat-to-flat 2).
// Each island is a disc of radius Radius around its centre (local q, r); global = Centre + local.
//
//	ring 0          the square (a well), never built on
//	rings 1..3      building ring: plots, except the road spokes
//	ring Radius     meadow: visitor plants only
//	spokes          road from the square to each bridge head (the bridge head is on ring Radius)
package town

import (
	"hash/fnv"
	"slices"
)

// Radius is an island's hex radius (61 hexes).
const Radius = 4

// Hosts are the island keys, in order.
var Hosts = []string{"h1", "h2", "h3"}

// Hex is an axial coordinate.
type Hex struct{ Q, R int }

func (h Hex) Add(o Hex) Hex { return Hex{h.Q + o.Q, h.R + o.R} }

// Ring is the hex distance from the island centre.
func (h Hex) Ring() int { return max(abs(h.Q), abs(h.R), abs(h.Q+h.R)) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Centre is each island's global axial centre: an equilateral triangle (side 12 hexes, 24 world units) around the
// origin. h3 sits at the front (+z, the default camera side).
var Centre = map[string]Hex{"h1": {-4, -4}, "h2": {8, -4}, "h3": {-4, 8}}

// bridges: each pair of islands, with the unit direction from the first to the second (axial).
var bridges = []struct {
	A, B string
	Dir  Hex
}{{"h1", "h2", Hex{1, 0}}, {"h1", "h3", Hex{0, 1}}, {"h2", "h3", Hex{-1, 1}}}

// Tile kinds (one byte each in Island.Tiles).
const (
	Square = 's'
	Grass  = 'g' // building ring, free for a plot or a plant
	Road   = 'r'
	Bridge = 'b' // a bridge head (on the rim)
	Meadow = 'm' // the rim: plants only
)

// Hexes lists an island's local hexes in canonical order: r from −Radius to Radius, then q ascending. Island.Tiles
// and the page index tiles in this order.
var Hexes = func() []Hex {
	var l []Hex
	for r := -Radius; r <= Radius; r++ {
		for q := max(-Radius, -r-Radius); q <= min(Radius, -r+Radius); q++ {
			l = append(l, Hex{q, r})
		}
	}
	return l
}()

// Island is hello.town.islands[i].
type Island struct {
	Host   string `json:"host"`
	Q      int    `json:"q"` // global axial centre
	R      int    `json:"r"`
	Tiles  string `json:"tiles"` // one kind byte per Hexes entry
	tileOf map[Hex]byte
}

// BridgeLink is hello.town.bridges[i]: the two hosts and each end's bridge-head hex (local to its island).
type BridgeLink struct {
	A  string `json:"a"`
	B  string `json:"b"`
	AQ int    `json:"aq"`
	AR int    `json:"ar"`
	BQ int    `json:"bq"`
	BR int    `json:"br"`
}

// Layout is hello.town: static for a build (Radius 4, side 12), sent so the page renders exactly what Go validates.
type Layout struct {
	Radius  int          `json:"radius"`
	Islands []Island     `json:"islands"`
	Bridges []BridgeLink `json:"bridges"`
}

var layout = build()

// Get returns the layout (shared, read-only).
func Get() Layout { return layout }

func build() Layout {
	tiles := map[string]map[Hex]byte{}
	for _, h := range Hosts {
		m := map[Hex]byte{}
		for _, x := range Hexes {
			switch x.Ring() {
			case 0:
				m[x] = Square
			case Radius:
				m[x] = Meadow
			default:
				m[x] = Grass
			}
		}
		tiles[h] = m
	}
	l := Layout{Radius: Radius}
	spoke := func(host string, dir Hex) Hex {
		for i := 1; i < Radius; i++ {
			tiles[host][Hex{dir.Q * i, dir.R * i}] = Road
		}
		head := Hex{dir.Q * Radius, dir.R * Radius}
		tiles[host][head] = Bridge
		return head
	}
	for _, b := range bridges {
		ha := spoke(b.A, b.Dir)
		hb := spoke(b.B, Hex{-b.Dir.Q, -b.Dir.R})
		l.Bridges = append(l.Bridges, BridgeLink{A: b.A, B: b.B, AQ: ha.Q, AR: ha.R, BQ: hb.Q, BR: hb.R})
	}
	for _, h := range Hosts {
		t := make([]byte, len(Hexes))
		for i, x := range Hexes {
			t[i] = tiles[h][x]
		}
		c := Centre[h]
		l.Islands = append(l.Islands, Island{Host: h, Q: c.Q, R: c.R, Tiles: string(t), tileOf: tiles[h]})
	}
	return l
}

// Tile is the kind of local hex x on host's island; 0 off the island or for an unknown host.
func Tile(host string, x Hex) byte {
	for _, is := range layout.Islands {
		if is.Host == host {
			return is.tileOf[x]
		}
	}
	return 0
}

// Plantable: a visitor plant may go on grass (not a plot: the caller checks) or the meadow.
func Plantable(host string, x Hex) bool {
	t := Tile(host, x)
	return t == Grass || t == Meadow
}

// Kinds: the building kinds a plot can carry (world:plot role). Services use their own role; apps get one of the
// house kinds by group unless config.yaml names one.
var Kinds = []string{"home", "house", "market", "tavern", "tower", "church", "castle",
	"gate", "guardhouse", "craneyard", "watchtower", "warehouse", "vault", "granary", "depot"}

// Slot picks a plot for key on host's island: the first free grass hex in a deterministic order. Services and
// platform apps prefer the inner rings (1, 2), user apps the outer ones (2, 3); the start inside each ring set is
// fnv(key), so the same app lands on the same hex every time. taken: hexes already holding a plot. avoid: hexes to
// use only when nothing else is free (visitor plants; the caller evicts the plant). ok = false when the island is full.
func Slot(key, host string, inner bool, taken, avoid map[Hex]bool) (Hex, bool) {
	order := []int{2, 3, 1}
	if inner {
		order = []int{1, 2, 3}
	}
	var cands []Hex
	h := fnv.New32a()
	h.Write([]byte(key))
	seed := int(h.Sum32() & 0x7fffffff)
	for _, ring := range order {
		var rs []Hex
		for _, x := range Hexes {
			if x.Ring() == ring && Tile(host, x) == Grass {
				rs = append(rs, x)
			}
		}
		if len(rs) == 0 {
			continue
		}
		s := seed % len(rs)
		cands = append(cands, append(slices.Clone(rs[s:]), rs[:s]...)...)
	}
	var fallback *Hex
	for _, x := range cands {
		if taken[x] {
			continue
		}
		if avoid[x] {
			if fallback == nil {
				fallback = &x
			}
			continue
		}
		return x, true
	}
	if fallback != nil {
		return *fallback, true
	}
	return Hex{}, false
}

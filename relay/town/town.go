// Package town is the live town's geometry (R7, R8): three hex islands, one per Proxmox host bucket h1..h3, joined by
// bridges. The relay owns it so Go and the page never disagree about which hex is what: hello.town carries the tile
// map and the page only renders it. Pure and deterministic, no I/O.
//
// Hexes are pointy-top axial (q, r), the pi-lab kit.hex grid: world x = 2(q + r/2), z = √3·r (flat-to-flat 2).
// Each island is a disc of its own radius around its centre (local q, r); global = Centre + local.
//
// R8: an island's radius follows its host's power (static hardware, ARCHITECTURE.md): see Radii.
//
//	ring 0              the square (a well), never built on
//	rings 1..r-2        the town: plots, except the road spokes
//	ring r-1            the outskirts: plots too (Industry first)
//	ring r              meadow: visitor plants only
//	spokes              road from the square to each bridge head (the bridge head is on ring r)
package town

import (
	"hash/fnv"
	"slices"
)

// Hosts are the island keys, in order.
var Hosts = []string{"h1", "h2", "h3"}

// Radii: each island's hex radius, from its host's hardware (R8). score = 0.45·threads + 0.40·ram + 0.15·disk, each
// normalised to the strongest host; radius = 4 + round(3·(s − s_min)/(1 − s_min)). h1 bnei 12 threads/32 GB/2 TB = 1.00
// → 7; h2 server1 6/32/476 = 0.66 → 5; h3 ex-laptop 4/15/238 = 0.36 → 4. Hard-coded (this package builds at init,
// before the config); TestRadii recomputes the formula from the specs.
var Radii = map[string]int{"h1": 7, "h2": 5, "h3": 4}

// MaxRadius bounds a local hex coordinate on any island (sanitize).
const MaxRadius = 7

// channel is the hex distance between the two facing bridge heads of the widest pair (h1–h2): 2 hexes of open water.
// The other pairs are smaller islands, so their straits are wider.
const channel = 3

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

// bridges: each pair of islands, with the unit direction from the first to the second (axial). The three directions
// make the islands an equilateral triangle; h3 sits at the front (+z, the default camera side).
var bridges = []struct {
	A, B string
	Dir  Hex
}{{"h1", "h2", Hex{1, 0}}, {"h1", "h3", Hex{0, 1}}, {"h2", "h3", Hex{-1, 1}}}

// Side is the triangle's side in hexes: the widest facing pair plus the channel, so no two islands touch.
var Side = func() int {
	s := 0
	for _, b := range bridges {
		s = max(s, Radii[b.A]+Radii[b.B])
	}
	return s + channel
}()

// Centre is each island's global axial centre: h1 at the back left, h2 = h1 + Side·(1,0), h3 = h1 + Side·(0,1), the
// triangle roughly centred on the origin.
var Centre = func() map[string]Hex {
	o := -Side / 3
	h1 := Hex{o, o}
	return map[string]Hex{"h1": h1, "h2": h1.Add(Hex{Side, 0}), "h3": h1.Add(Hex{0, Side})}
}()

// Tile kinds (one byte each in Island.Tiles).
const (
	Square = 's'
	Grass  = 'g' // town and outskirts, free for a plot or a plant
	Road   = 'r'
	Bridge = 'b' // a bridge head (on the rim)
	Meadow = 'm' // the rim: plants only
)

// HexesOf lists a radius-r island's local hexes in canonical order: r from −radius to radius, then q ascending.
// Island.Tiles and the page index tiles in this order.
func HexesOf(radius int) []Hex {
	var l []Hex
	for r := -radius; r <= radius; r++ {
		for q := max(-radius, -r-radius); q <= min(radius, -r+radius); q++ {
			l = append(l, Hex{q, r})
		}
	}
	return l
}

var hexesOf = map[string][]Hex{}

// Hexes is host's island hexes in canonical order (shared, read-only); nil for an unknown host.
func Hexes(host string) []Hex { return hexesOf[host] }

// Island is hello.town.islands[i].
type Island struct {
	Host   string `json:"host"`
	Q      int    `json:"q"` // global axial centre
	R      int    `json:"r"`
	Radius int    `json:"radius"`
	Tiles  string `json:"tiles"` // one kind byte per Hexes(host) entry
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

// Layout is hello.town: static for a build, sent so the page renders exactly what Go validates.
type Layout struct {
	Islands []Island     `json:"islands"`
	Bridges []BridgeLink `json:"bridges"`
}

var layout = build()

// Get returns the layout (shared, read-only).
func Get() Layout { return layout }

func build() Layout {
	tiles := map[string]map[Hex]byte{}
	for _, h := range Hosts {
		r := Radii[h]
		hexesOf[h] = HexesOf(r)
		m := map[Hex]byte{}
		for _, x := range hexesOf[h] {
			switch x.Ring() {
			case 0:
				m[x] = Square
			case r:
				m[x] = Meadow
			default:
				m[x] = Grass
			}
		}
		tiles[h] = m
	}
	var l Layout
	spoke := func(host string, dir Hex) Hex {
		r := Radii[host]
		for i := 1; i < r; i++ {
			tiles[host][Hex{dir.Q * i, dir.R * i}] = Road
		}
		head := Hex{dir.Q * r, dir.R * r}
		tiles[host][head] = Bridge
		return head
	}
	for _, b := range bridges {
		ha := spoke(b.A, b.Dir)
		hb := spoke(b.B, Hex{-b.Dir.Q, -b.Dir.R})
		l.Bridges = append(l.Bridges, BridgeLink{A: b.A, B: b.B, AQ: ha.Q, AR: ha.R, BQ: hb.Q, BR: hb.R})
	}
	for _, h := range Hosts {
		t := make([]byte, len(hexesOf[h]))
		for i, x := range hexesOf[h] {
			t[i] = tiles[h][x]
		}
		c := Centre[h]
		l.Islands = append(l.Islands, Island{Host: h, Q: c.Q, R: c.R, Radius: Radii[h], Tiles: string(t), tileOf: tiles[h]})
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

// Family is a building's class by what it does (R8 typology): it decides where on the island it stands, the ground
// under it and its label tier.
type Family string

const (
	Defense  Family = "defense"  // security and control: walls, towers, keeps
	Public   Family = "public"   // the shared platform: town hall, market, granary, library, bathhouse
	Industry Family = "industry" // the infra machines: barracks, forge, mills
	Shops    Family = "shops"    // user apps with public traffic
	Houses   Family = "houses"   // small or internal apps
)

// families maps every building kind (world:plot role) to its family. Kinds are role-like words, never product names.
var families = map[string]Family{
	// Defense
	"gate": Defense, "guardhouse": Defense, "watchtower": Defense, "vault": Defense, "keep": Defense,
	"castle": Defense, "tower": Defense, // R7 kinds, still valid in replays of older days
	// Public
	"craneyard": Public, "depot": Public, "warehouse": Public, "granary": Public, "archive": Public, "cellar": Public,
	"market": Public, // R7
	// Industry
	"hall": Industry, "forge": Industry, "witness": Industry, "mill": Industry,
	// Shops
	"church": Shops, "tavern": Shops, "inn": Shops, "alehouse": Shops, "bookshop": Shops, "playhouse": Shops,
	// Houses
	"home": Houses, "house": Houses,
}

// Kinds: every building kind a plot can carry (world:plot role), sorted.
var Kinds = func() []string {
	l := make([]string, 0, len(families))
	for k := range families {
		l = append(l, k)
	}
	slices.Sort(l)
	return l
}()

// FamilyOf is kind's family; Houses for an unknown kind.
func FamilyOf(kind string) Family {
	if f, ok := families[kind]; ok {
		return f
	}
	return Houses
}

// rings is a family's ring preference on a radius-r island (the meadow ring r is never built on).
func rings(f Family, r int) []int {
	span := func(a, b int) []int { // a..b inclusive, either direction
		var l []int
		if a <= b {
			for i := a; i <= b; i++ {
				l = append(l, i)
			}
		} else {
			for i := a; i >= b; i-- {
				l = append(l, i)
			}
		}
		return l
	}
	switch f {
	case Public: // around the square
		return span(1, r-1)
	case Defense: // the town's edge, falling back inwards, then the outskirts
		return append(span(r-2, 1), r-1)
	case Industry: // the outskirts first
		return span(r-1, 1)
	default: // Shops, Houses: the town, then the outskirts, the square's ring last
		return append(append(span(2, r-2), r-1), 1)
	}
}

// onSpoke: x touches a road hex (a spoke), the street front.
func onSpoke(host string, x Hex) bool {
	for _, d := range []Hex{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}} {
		if Tile(host, x.Add(d)) == Road {
			return true
		}
	}
	return false
}

// Slot picks a plot for key (a building of kind) on host's island: the first free grass hex in a deterministic order.
// The order follows the kind's family (rings); Shops take the street front (next to a spoke) of each ring first. The
// start inside each ring is fnv(key), so the same app lands on the same hex every time. taken: hexes already holding a
// plot. avoid: hexes to use only when nothing else is free (visitor plants; the caller evicts the plant). ok = false
// when the island is full.
func Slot(key, host, kind string, taken, avoid map[Hex]bool) (Hex, bool) {
	r := Radii[host]
	f := FamilyOf(kind)
	h := fnv.New32a()
	h.Write([]byte(key))
	seed := int(h.Sum32() & 0x7fffffff)
	var cands []Hex
	add := func(rs []Hex) {
		if len(rs) > 0 {
			s := seed % len(rs)
			cands = append(cands, append(slices.Clone(rs[s:]), rs[:s]...)...)
		}
	}
	for _, ring := range rings(f, r) {
		var front, rest []Hex
		for _, x := range Hexes(host) {
			if x.Ring() != ring || Tile(host, x) != Grass {
				continue
			}
			if f == Shops && onSpoke(host, x) {
				front = append(front, x)
			} else {
				rest = append(rest, x)
			}
		}
		add(front)
		add(rest)
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

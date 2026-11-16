# S2 nav: a layered graph for reachability

Branch `spike/s2-nav` (ukubi-game, commit 759eab4, local only). Machine: Apple M1 Pro, go 1.27.4, `go test`, native (not WASM).

## Question

Bet 2 of `placement.md`: can a layered (hex, floor) nav graph prove reachability on the 2x disc cheaply, and does it give fewer wrong answers than a hex-level check?

## What was built

- `internal/navspike/nav.go` (on the spike branch): the graph, a BFS, and a naive hex BFS.
  - Ground nodes: every dry, non-ramp hex. Edges: `StepHexes` plus a rise check at the edge midpoint, as `physics/coarse.go:63-108`. A mode adds single and double jump climbs.
  - Rider-radius check: a side is open only if some sample on the shared edge keeps `BodyR` clear of every decor circle (taller than a step) and fence box.
  - Tunnel floor: one node per hall and per portal mouth. Edges: bores (hall to hall), ramp chains, mouth, link. Cave floor: one node behind each mouth, joined to its hex.
  - Deck floor: none built. No land decor is a `Deck` box; decks are only player floors.
- `internal/navspike/nav_test.go`: the 2x disc, the comparisons, the sealed-house mutation, the timings.

2x disc: `Disc.R` and zones doubled, and four copies of the 76 buildings (rotated a quarter each, positions doubled). Density stays the same. Villages stay as served, because their hex lists cannot be scaled. Lairs do not exist yet; a wild's centre stands in (`lair*`).

## Numbers

Median of 5 runs, `rtk proxy go test ./internal/navspike -run TestSpike|TestGapCost -v`, jump edges on.

| | 1x disc | 2x disc |
|---|---|---|
| hexes | 11,053 | 43,777 |
| decor colliders | 13,003 | 51,188 |
| nodes | 10,539 | 42,753 |
| edges | 60,614 | 251,594 |
| graph build, ms | 33.6 | 147.4 |
| graph build without the radius check, ms | 11.7 | 62.4 |
| search from spawn (BFS, all targets), ms | 0.2 | 1.4 |
| naive hex BFS, ms | 2.2 | 9.8 |
| `newCoarse` (rider graph, first call), ms | 7.1 | 35.9 |
| land build (`land.Build`), ms | 166 | 1,698 |

Sides closed by the radius check: 200 (1x), 212 (2x), out of about 60k.

Layered with jumps reaches every house, portal, hall, cave, summit and lair from spawn, on both discs (1x: 76 houses, 12 portals, 27 halls). Walk-only on the 2x fixture reaches 89 hexes: a stretched-fixture artefact, not a finding.

Comparison with the rider (fine route, `Router.Route` with the decor grid, 35 targets, 1x): 32 agree. The other 3 are lairs whose target is inside an elder tree. Run alone, each route ends 2.0 to 2.5 units beside it, so I count them as agreeing. Worst route 0.26 s.

Sealed-house mutation (12 houses, a ring of solid circles across all six sides; 1x): said "reachable": layered 0, naive hex 12, `Router.Coarse` 12, fine route 0.

## Findings

- **Today's land has no wrong answers to find.** Every addressable thing is reachable by every method. The hex check is already right for houses, summits, lairs on the real land.
- **The layered graph is right where the hex check is blind.** Sealed houses: the hex check and `Coarse` say reachable 12 of 12 (false reachable); the graph and the rider say 0. `Coarse` and `stepHexes` (`hex.go:1331`) never see decor.
- **Underground is invisible to the hex check.** It answers "reachable" for all 27 halls by looking at the surface hex. `Coarse` refuses ramps (`coarse.go:63`), so it returns "no" for all 12 portals and can never address a hall. The graph reaches them through ramp, mouth and bore edges.
- **Walk-only is stricter than `stepHexes`.** `stepHexes` allows 2 levels (`MaxClimb`); a walk takes `FaceMin`. Both summits and both caves are unreachable on foot without jumps, and naive says reachable. Fine for a proof "with the rider's jump", wrong as a proof "on foot".
- **Radius check changes nothing today** (same reach with and without) and costs 2.9x the build (21.9 ms of 33.6 at 1x). Its value is future decor.

## What changes in the plan

- `placement.md` law 2: keep it. Say the cost: 150 ms and 250k edges on the 2x disc, search 1.4 ms. That fits a server build check. In the WASM it is several times slower; do not ship the graph build in the page. Ship search only, or the CSR arrays.
- Run the graph check on the server, per composer build. The 2x land build (1.7 s) dominates, not the graph.
- State the proof level: "with the rider's jumps". Add a walk-only mode for quests that forbid jumps.
- The roadmap W6 nav item: replace "shared with the sim's coarse nav" with "the sim's coarse nav is built from this graph". `Coarse` cannot enter tunnels today, so the two disagree on every hall.
- Every connector needs the radius check on its ground side (my ramp edges skipped it and a sealed house stayed reachable through a tunnel).
- Add a mutation test to W6: seal a door with decor, expect the failure to name the blocker.

## Open questions

- Is the radius check worth 2.9x build time before decor can actually seal something (player builds, phase 7)?
- The 2x fixture keeps 1x villages. A real 2x disc needs the village layout to scale; re-measure then.
- Should `Coarse` learn tunnels, or be replaced by the graph?

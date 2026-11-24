# Spike S1: placement by rank

> Status: history (2026-11-24). Spike record.

Branch `spike/s1-placement` (ukubi-game `2b30da3`). Line refs are `origin/main` unless marked "spike".

## Question

Does pure ranking by strength hold on real land stages, and at what cost?

## What was built

- `internal/stage/stage.go`: the composer (prims at paths, ranks, hex plus face plus sub-hex, relative positions,
  law 1, a report).
- `internal/stage/toy_test.go`: a pinned village, rooms derived from `yard.Find("photo-album")`, a rival house, a
  sunflower.
- `internal/land/halls.go`: `stageHalls`, the ramps picked on the hex land before `massifs` (flag `spikeLateRamps`).
- `internal/land/spike_test.go`, `spike2_test.go`: the measurements.

## Numbers

Apple M1 Pro, go 1.27.2, `go test -count=1 -v -run 'TestSpike'` on `./internal/land` and `./internal/stage`.

| Measure | Fixture | Dense | Yard |
|---|---|---|---|
| Halls | 9 | 27 | 27 |
| Ramps carved today | 7 | 12 | 21 |
| Ramps, ranked, props above ramps | 7 | 14 | 18 |
| Ramps, ranked, props below ramps | 7 (same hexes) | 14 | 21 |
| Ramps dropped late by `stageTunnels`, any mode | 0 | 0 | 0 |
| Props on a ramp, dropped silently: today / ranked | 0 / 1 | 0 / 0 | 14 / 19 |
| Massifs / trailheads, every mode | 2 / 6 | 2 / 6 | 2 / 6 |
| Ramps changed by reversing the id order | 2 | 6 | 7 |
| Ramp pick, today in `landOf` (mean of 10) | 1.8 ms | 7.3 ms | 10.6 ms |
| Ramp pick, ranked `stageHalls` (mean of 10) | 0.26 ms | 0.69 ms | 0.78 ms |
| Massif stage (mean of 10) | 16 ms | 19 ms | 20 ms |

- Toy compose: 14 to 19 µs (mean of 10,000 runs, 19 prims). Composer: 130 lines of code; `stageHalls`: 92.

## Findings

**What holds**
- Law 1 holds in the toy. Pin against pin fails naming both (spike `stage.go:156`). A derived prim on a pin is
  dropped and reported. Masks are listed.
- The shuffle test passes 50 runs; a mutation to input order makes it fail. Moving the village moves its children.
- The ramp inversion goes. Ranked ramps are never dropped later, and the pick is 7 to 14 times cheaper.
- Massifs do not move in any mode.

**What breaks**
- **Peers read each other.** Each ramp reserves a ring the next hall reads (spike `halls.go:72`). Reversing id order
  changes 2 to 7 ramps. A per-id hash priority fixes input order, but a new hall still moves the halls it outranks.
- **Props are a hidden rank.** They are placed in `landOf`, kept off the ramps (`land.go:805`), and read by massifs
  (`massif.go:203-205`). If props win, the yard loses 3 ramps. If ramps win, it loses 19 props.
- **A silent drop today.** `plotsOf` (`hex.go:881`) drops 14 yard props sitting on ramps; no test reports it.
- **The spec's height estimate is wrong.** `app:ente-web`'s ramp top passes `rampHexOK` but is at level -2 on the
  land. `stageTunnels` checks only `Level <= 1`.
- **What massifs really read.** Removing each reader in turn, only the wild circles (`massif.go:189`) move massifs
  (`[0 -37]` to `[16 -42]`). Props, ramps, passes and plot bands change nothing here. The plot bands
  (`massif.go:184-185`) reserve room for a weaker rank: an inversion with no effect yet.
- The hex hash changes even where ramps are identical, because the props move.

## What changes in the plan

1. **placement.md, ranks.** Replace "never reads ... a peer": inside a generator, peers resolve by per-id hash
   priority; a new prim moves only lower-priority peers it hits, listed in the report.
2. **placement.md, rank list.** Name the props as a rank: pins, derived, halls and ramps, wild centres, props, massifs.
   Add the `plotsOf` prop drop to the silent drops that go.
3. **placement.md, massifs.** The plot-band reservation becomes a stronger rank, or it goes. Decide when massifs are
   ported.
4. **Roadmap W4.** Halls and ramps go first. The golden test cannot gate the port because the props move. Use law
   tests instead: no late ramp drop, no prop on a ramp, massifs and trailheads unchanged.

## Open questions

- Should ramps or props win? This is an owner call on feel.
- Is a ramp moving next to a new app acceptable in review?
- Pits and caves were not ported; no pit hit a ramp here, which proves nothing for the 2× disc.

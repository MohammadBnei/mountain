# Doubt round: placement (2026-11-15)

One fresh adversarial reviewer (read-only, code and ADR only) checked `04-architecture/placement.md` against
ukubi-game `19a3646`.

| # | Severity | Finding | Resolution |
|---|---|---|---|
| F1 | blocker | The land stages are an ordered pipeline with an inversion: tunnel ramps are chosen before any hex stage and masked by massifs, then dropped where massifs land (`hex.go:770-779`, `land.go:724-768`, `massif.go:206-213`, `tunnel.go:357-370`) | Owner: force a pure system. Each generator gets its own rank and reads only stronger ranks; ramps become their own generator above massifs; ported incrementally under a golden test |
| F2 | blocker | A weaker layer masks a stronger one: the wilds' circles and the fun-park props block massif sites (`massif.go:189-193`, `:203-205`) | The wilds' centres become a generator above massifs; only the trails stay decor |
| F3 | blocker | Hex-level reachability is wrong on tunnels and caves (a second floor), bridges and decor-made climbs (`hex.go:1331-1346`, `tunnel.go:73`, `climb_test.go:76`) | Owner: a layered nav graph of (hex, floor) nodes, connector edges every generator emits, decor solids narrowing passages; shared with the sim's coarse nav; cost measured first |
| F4 | major | Silent nudging (`freeNear`, `hex.go:289-306`) and silent drops (`massif.go:268`) are not covered by law 1 | Masked and dropped prims are listed in the build report; `freeNear` goes |
| F5 | major | Law 1 can stall growth: a new app with no room fails the whole build | Owner: keep the last good version and file a proposal; only pin against pin fails |
| F6 | major | `Place` does not hash ids: without pinned districts it places centres by index (`place.go:200-211`) | Claim fixed; per-id hash slots, tested by shuffling the input |
| F7 | major | The realm log addresses planted objects by `uint32` id and float xyz, not `(chunk, index)` (`log.go:98-110`) | Claim fixed; land decor becomes `(chunk, index)`; planted and felled objects keep their ids as player state |
| F8 | major | Sub-hex and float positions exist (pieces, plantings, `BuildWith`, `Building.Hex`, anchors); fixtures re-baseline | Optional sub-hex offset where builds, plants or mounts stand; one deliberate re-baseline |
| F9 | major | The style spec leaves behaviour with no home (`world-hex.ts:84`, `world-build.ts:39`, `world-targets.ts:123`, picking by mesh, `Model.Wild`, `Far`) | A home table in the ADR; picking uses footprint colliders |
| F10 | major | The page has no reason to run the composer; the WASM already ships the land build and `Place` | Owner: server only; the WASM keeps `Step` and nav |
| F11 | major | Queries are unspecified: ties, version pinning, who resolves | The realm resolves at quest start and pins path and space version; a declared tie-break, else fail at quest load |
| F12 | minor | Per-chunk content ids don't hold for whole-space stages | Chunk ids per space version; `sim-world.md` follows |
| F13 | minor | Migration missing; two placers exist (`town/placement.json` + `Place`, logged `LayOut`) | A migration section: seed pins from both, composer under a golden test, port stages, nav graph, re-baseline, WASM cut |

Owner answers (same sitting): pure ranked generators, incremental big-bang rewrite accepted; keep last good on a
derived collision; layered nav graph; composer server-only.

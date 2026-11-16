# S4 test safety (2026-11-15)

## Question

How big is the test migration that makes moving the world safe, and does a stage `Find` API make tests origin-blind?

## What was built

Branch `spike/s4-test-safety` in ukubi-game (worktree under the session scratchpad, one local commit `33aad66`, not pushed).
- `internal/land/find.go`: `HexLand.Find(Query{Kind, ID, Tag})` over today's land (kinds pad, lake, hall, massif, pit, cave, zone; tags `tunnel`, `ramp`). 60 lines.
- `internal/land/find_test.go`: Find on all 5 fixture lands, and a "move" test.
- `cmd/game/websrc/world-find.ts`: the same query on the page, exposed as `__world.find(q)` (`world.ts`, 2 lines).
- Two gate checks migrated: `dev/aim-gate.mjs` (`placeBy`) and `dev/calm-gate.mjs` (the lake spot, "fish").
- `cmd/game/websrc/world-find.test.ts`: runs the old and new closures of both checks on the fixture land.

## Numbers

Classes are by a per-test awk heuristic over the test body (`/tmp/cls.awk`, `/tmp/ts.awk`, one run on this laptop). Treat as plus or minus 15%.

| Suite | Tests | synthetic | query-by-id | literal-position | snapshot |
|---|---|---|---|---|---|
| Go `*_test.go` (204 files) | 890 | 795 | 44 | 36 | 14 |
| TS `*.test.ts` (126 files, top-level tests) | 993 | 947 | 31 | 11 | 4 |
| `dev/*-gate.mjs` (9 gates, 7866 lines) | 7 touch the world | n/a | about 41 lookups | 0 hard-coded coordinates | 1 (hash pin, `world-gate.mjs:618`) |

- Fixture land files: 5 Go goldens (139 to 195 KB each), 2 TS fixtures (`world-land.fixture.json`, `.dense.json`), `town/testdata`, `physics/testdata/traces.json`, `realm/testdata/{land-hash,snapshot,owner-town}.json`.
- One constant carries most literals: `openX, openZ = -84.05, -248.35` (`internal/physics/walk_test.go:12`) has 50 uses.
- Gate scans: about 45 reads of `__world.land().at/find/spec` that look places up by scan, not by number.
- To migrate: 47 literal, 18 snapshot, 75 query (mechanical), about 14 gate sites. Estimate 70 hours (author's guess; the 2 gate checks took about 15 minutes each): literal 24 h, snapshot 20 h, query 11 h, gates 4 h, Find plus law harness 12 h.

Spike checks:

| Check | Result |
|---|---|
| `go test ./internal/land -run TestFind` | pass, 5 lands, 0.55 s |
| `bun test world-find.test.ts` | 3 pass; old and new gate closures give the same spot |
| Move every building 1 hex east, `Disc.R` unchanged | build panics in `stagePasses` (`hex.go:685`, `b.at(c)` is nil) |
| Same move, `Disc.R` +20 | builds; 11 of 11 pads move; 3 of 7 ramp halls vanish (shift 2: 2 vanish). Control (`R` +20, no move): 0 change |

## Findings

- **Holds:** most tests are already origin-blind. 87% of Go and 95% of TS tests build their own grid. No gate hard-codes a coordinate; they derive positions from pads by id or from scans.
- **Holds:** a Find query is small and migrates a gate check in a few lines. `__world.land().find(kind, n)` already exists (`world.ts`, hex kinds); Find is its prim-level sibling.
- **Breaks:** the golden (`land_test.go:46-50`, plus `cliffreach_test.go:15`) fails on any move, and says nothing about why. After a move it cannot tell a real loss from a shifted number.
- **Breaks, and this is the real result:** today's generators are origin-dependent. A 1-hex move loses 3 of 7 tunnel ramps (`tunnel.go:357-370`) or crashes `stagePasses` (`hex.go:685`). A "roles present" law (every hall with a ramp exists) catches this; a golden would only say "differs".
- **Weak:** `zone/capital` is a ring, not a prim; "the capital square" needs a real `Site` prim to be findable by role.
- **Not done:** the two migrated gates were not run end to end. They need atlas, the game and a browser (`dev/gate-stack.sh`). Only their page-side code ran, in Bun. No browser or server was started, nothing to clean up.

## What changes in the plan

- `placement.md`, Migration step 2: add "Find and the law harness land before any generator port". Tests migrate to Find first, so ports run on laws, not goldens.
- Add a law 4 to the laws: "role stability". After a rigid move of a space, every prim of a declared role still exists (tag counts equal). This fails today at `tunnel.go:357-370`.
- Step 5 (re-baseline): do it once for the 5 Go goldens plus `traces.json` and the TS pins. Record 18 snapshot tests as the full list.
- `sim-world-build.md` W2: add "replace `openX/openZ` with an `openSpot(land)` helper" as the first task (1 change, 50 lines).
- W2 size: budget 70 hours for the test migration inside W2, not folded in silently.

## Open questions

- Should Find resolve ties by a declared tie-break in tests too, or fail?
- Should the page's `__world.find` and the Go `Find` share one query spec file, so gates and Go tests cannot drift?
- `Disc.R` is fixed per build. Does the composer grow it per space, or is a prim off the grid a build error (law 1)?

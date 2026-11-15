# sim-world and placement build plan

**Status.** Draft 2026-11-15, for owner approval. It sequences the two ADRs (`../04-architecture/sim-world.md`,
`../04-architecture/placement.md`) and their doubt rounds (`../05-doubt/`). The owner confirmed: no solo mode (the
laptop runs the realm), pure ranked generators rewritten incrementally, keep-last-good on a derived collision, a
layered nav graph, and the composer on the server only.

**Shape of every phase** (`ui-core-build.md`, the agreed workflow shape):
- build fan-out on disjoint files, with fast suites in each lane;
- one integration;
- the safety net once (`../04-architecture/tests.md`);
- one fix agent per round;
- the PR, a fresh review, then the merge on the owner's word.

Builders read `../04-architecture/charter.md`. Decision records go to `decisions.md`.

**Standing proofs.**
- The full Go suite (`go test ./...`, with `codecheck`) runs before any merge.
- The dist stays byte-identical under bun 1.4.x.
- From W1 on, the gc-against-WASM golden runs in CI.
- Each phase names its own proof (laws, determinism, a playthrough). A phase that changes feel is played by hand before the merge.

**Test safety: moving the world must never mean hours of test fixes.** Counted on ukubi-game `main`:
- the 9 browser gates mostly look places up by id (`aim-gate.mjs:136`), with 4 literal-coordinate lines;
- 15 Go test files use literal hexes or x/z, most of them physics tests on synthetic grids;
- the 5 snapshot goldens (`internal/land/testdata/golden-*.json`, read only by `land_test.go:46-50`) exist to prove
  the Go land build matched the old TS copy float for float (`land_test.go:20`). That port is done; they now only
  forbid the world from moving.

The rules:
1. **Tests ask the stage.** `stage.Find(path|query)` in Go and `__world.find(query)` in journeys, never coordinates.
   Exposed tests migrate in W2, before the world moves.
2. **No snapshot goldens of the world.** The 5 land goldens are deleted at W3. What stays: the gc-against-WASM
   determinism golden (synthetic inputs, not world-shaped) and law tests (no collision, roles present, reachable,
   order-free), which hold however the world moves.
3. **A frozen test world.** The fixture worlds (`world-land.fixture.json`, `yard-fixture.json`, `disc-fixture.json`,
   `fiction.json`) are inputs and stay: the journeys run on them. Only the world laws run on the live stage, and
   they check laws, not places.
4. **A count before each phase.** Every test touching the world is classed as synthetic, query-by-id or
   literal-position. The phase's plan states its migration size.

## W0 · Spikes (throwaway branches, in parallel)

| Spike | Partial implementation | Answers |
|---|---|---|
| S1 placement | Composer, ranks and law 1 on a toy space (a pinned village, rooms derived from one app, a sunflower decor rule); then the real halls-and-ramps generator ported against massifs | Does pure ranking hold on real stages, at what cost |
| S2 nav | Layered (hex, floor) graph read from today's disc (tunnel floors, decor solids), a search from the spawn, timed on the 2× disc, compared with the rider route | Bet 2: cost and false answers |
| S3 visual | Style spec for one family (trees and rocks) behind a URL flag: weighted variants, zoom stops; flag off leaves the page as it is | Per-frame cost, spec size |
| S4 test safety | The full test count, plus a `Find` prototype with 2 gates migrated | W2's migration size |

Each spike writes `07-spikes/<name>.md` with its numbers. The plan is adjusted before W1 starts.

## W1 · Proof of determinism (small, main thread)

- Exp and Hypot join `internal/ptrig`.
- The lint bans every `math` function with an assembly path.
- `town/`, `internal/land` and `internal/combat` join the exact set.
- A CI test runs the same inputs through gc and the TinyGo WASM and compares `Float64bits`.
- **The same PR deletes the 9 GPU gates** and `dev/ci-local.sh --gates`, with the one-line ledger per gate check, and
  cuts the HUD fixture to "mounts without error" plus screenshots (`../04-architecture/tests.md`).
- **Behaviour:** none (bit-exact by construction). **Proof:** the golden plus a mutation (put `math.Exp` back, and
  the test fails).

## T1 · One clock (with W2)

- Every clock read in the page (`performance.now`/`Date.now`, 32 in `world.ts`) goes through one injected clock, as
  the HUD's `UiClock` does.
- The realm gets a test-only step endpoint; `advance(ms)` steps its ticks and the page's fixed frames together.
- **Proof:** a stepped boot on the fixture world gives the same state twice.

## T2 · Browser journeys (after W3)

- 6 to 10 scripted journeys, desktop and 390 px, on the real game and realm in `--dev`, the frozen fixture world and
  the shared clock: boot and enter; ride to a house, enter, talk, leave; the map; one fight; a phone tap route; a
  panel open and close. Each checks for console errors.
- The QA helper (screenshots per step, no pass/fail).
- **Budget:** the whole CI net under 60 s. A red run is a real bug.

## W2 · The stage core

- **First, the test migration** (spike S4, about 70 hours, budgeted here):
  - `openSpot(land)` replaces the `openX, openZ` constants (`internal/physics/walk_test.go:12`, 50 uses);
  - `Find` in Go and `__world.find(query)` in the page;
  - the law harness;
  - 47 literal-position and 18 snapshot tests and about 75 id lookups, migrated (the gate sites go with the gates).

- **Composer:**
  - typed prims at paths;
  - ranks by strength;
  - references, payloads and variants;
  - position as hex plus face with an optional sub-hex offset, and world units derived in one function.
- **Registry rows:** schema rows for prim kinds, hex kinds, pieces and models. The `WorldModel` row splits into stage
  and gameplay fields (the look fields wait for W5).
- **Source:** the `Element` projection of atlas, plus the kind registry replacing the switch at `town/place.go:115-128`.
- **Pins:** seeded from both of today's placers, the logged `LayOut` plan and `town/placement.json`.
- **Law 1:**
  - pin against pin fails the build;
  - a derived collision keeps the last good version and files a proposal;
  - a build report lists every masked and dropped prim;
  - `freeNear` goes.
- **Proof.** Law tests and a shuffled-input test (no dependence on id order). Optional: one temporary check that the
  composer, fed the seeded pins, places what today's code does, deleted after the merge.
- **Behaviour:** none visible. The logged layout stops being the source of truth.

## W3 · Server-built land, streamed

- The game server builds each space whole, using today's land build for now.
- It cuts the result into `WriteHex` chunks with ids per space version, plus the strategy overview (hex graph, low-res
  height and biome, zones, placements, portals).
- The page streams chunks and the overview and no longer builds land.
- `Place`, `hex` and the land build leave the WASM, which keeps `Step` and nav.
- The town page, `cmd/townpin` and the old district layout retire.
- The 5 land snapshot goldens and their TS parity tests are deleted.
- **Proof:**
  - a look at the world page before and after, at the same pose;
  - load traces against `01-baseline/load-profile.md`;
  - the WASM size.
- **Behaviour:**
  - The first load needs the server.
  - A land change lands on the next reload, unchanged.
  - The realm's land hash becomes the space version.
- **Owner check:** a reload on desktop and on the phone, timed.

## W4 · Generators, one rank at a time (repeated rounds)

- **Ported in rank order, one round each:**
  1. tunnel entrances inside houses (removes today's inversion);
  2. the wilds' centres;
  3. fun props;
  4. massifs;
  5. pits;
  6. tunnels (halls and bores under the land);
  7. caves, with their mouths on their massifs;
  8. roads and the rest;
  9. decor rules last: the golden trails, sunflowers and grass.
- **Each generator:**
  - reads only stronger ranks;
  - emits its connectors as nav edges;
  - is pure, with a shuffled-input test.
- **First, tunnel entrances move inside their houses** (a stair or trapdoor in the interior). The surface ramps and
  their inversion against massifs go (spike S1, S4). Cave mouths stay on their massifs.
- **Proof per round:** the law tests, and the owner plays it (massif trailhead reach, tunnel mouths, spirals). No
  snapshot golden.
- **Behaviour:** the land may shift where the old pipeline's hidden order differs from the ranks. Each shift is named
  in its PR.

## W5 · The visual axis (in parallel with W4: page files only)

- **The style spec:**
  - rules with filters over kind and tags;
  - expressions for zoom, quality tier, theme variables and attributes;
  - weighted variants seeded by position;
  - themes as stylesheets.
- A keyed patcher onto the three.js scene.
- **Old tables become style rules:** the look halves of `KINDS` (`world-hex.ts:84`, `world-build.ts:39`), `modelOf`,
  `DECOR` and `Model.Far`.
- Picking moves from rendered meshes to footprint colliders. The decor budget (`pickDecor`) reads the chosen
  variant's triangles.
- No sway in v1 unless a BatchedMesh shader patch is costed; zoom stops only for scale and tint (spike S3).
- **The glTF import pipeline:** a schema row with the footprint taken from the mesh, plus a visual asset.
- **Proof:** a second theme renders with no new stage version; the owner looks at both.
- **Owner check:** a look at both themes, and one imported model placed by a pin.

## W6 · The sim package and the layered nav graph

- **The `sim` package:**
  - `Step` with the ship, the lunge and the auto-hop in it;
  - fight and mobs pulled out of `internal/realm`;
  - the realm builds all of it, and the WASM builds `Step` and nav.
- **Nav:** the two-level nav on the layered graph, with (hex, floor) nodes, generator connectors and decor solids
  narrowing passages. It replaces `Coarse`, which cannot enter tunnels and refuses every portal ramp (spike S2).
  Every connector gets the rider-radius check on its ground side.
- **Law 2:** every addressable prim reachable from the spawn, checked in the build. Its cost is measured on the 2× disc
  first.
- **Proof:**
  - law 2 with a seal-a-door-with-decor mutation;
  - the sim golden (frames over seeds);
  - the step bench against `01-baseline/step-bench.md`;
  - law 2 with a mutation (a rock that blocks a door fails the build).
- **Behaviour:** the TS ship, lunge and hop copies are deleted, so their feel must match. **Owner check:** a ship
  flight and a fight.

## W7 · Spaces

- One realm instance per (space, version), each with its own tick, lock, bodies and mobs, and the old one drains.
- Interiors and the underside become shared instances (payloads), with their own colliders.
- Snapshots become binary deltas: interest filtering for bodies, and strategy interest by zoom (units grouped per hex
  or region at map scale).
- **Proof:**
  - two pages in one interior see each other;
  - snapshot bandwidth at 50 bodies against the doubt round's 7 Mbit/s figure;
  - the drain test.
- **Behaviour:** interiors and the underside are no longer private. **Owner check:** two players inside one house.

## W8 · Input replay

- **First, the two-phone race and PvP test,** which settles bet 2 of `sim-world.md`.
- **The rest:**
  - a fixed client step and tick-stamped inputs;
  - rewind and replay;
  - substeps or swept contact for bikes and ships;
  - a lag-compensation rule;
  - `ResolveFall`'s pose check goes.
- **Proof:** a replay golden (the same inputs give the same state on the page and in the realm), and the two-phone
  test.
- **Owner check:** a race and a duel on two phones.

## Later

- Quest queries resolved by the realm, pinned to a path and space version.
- Agent-written quests.
- Endless realms as generators.
- The master control room writing the files.

## Order and overlap

W0 (spikes) → W1 (with the gate deletion) → W2 with T1 → W3 → T2, then W4 and W5 in parallel (server files against page files), then W6 → W7 → W8. W6's nav graph needs
W4's connectors, so it starts after the tunnels and caves rounds of W4.

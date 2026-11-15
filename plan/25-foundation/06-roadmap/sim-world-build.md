# sim-world and placement build plan

**Status.** Draft 2026-11-15, for owner approval. It sequences the two ADRs (`../04-architecture/sim-world.md`,
`../04-architecture/placement.md`) and their doubt rounds (`../05-doubt/`). The owner confirmed: no solo mode (the
laptop runs the realm), pure ranked generators rewritten incrementally, keep-last-good on a derived collision, a
layered nav graph, and the composer on the server only.

**Shape of every phase** (`ui-core-build.md`, the agreed workflow shape):
- build fan-out on disjoint files, with fast suites in each lane;
- one integration;
- gates once, by angle;
- one fix agent per round;
- the PR, a fresh review, then the merge on the owner's word.

Builders read `../04-architecture/charter.md`. Decision records go to `decisions.md`.

**Standing proofs.**
- The full Go suite (`go test ./...`, with `codecheck`) runs before any merge.
- The dist stays byte-identical under bun 1.4.x.
- From W1 on, the gc-against-WASM golden runs in CI.
- Each phase names its own golden test. A phase that changes feel is played by hand before the merge.

## W1 · Proof of determinism (small, main thread)

- Exp and Hypot join `internal/ptrig`.
- The lint bans every `math` function with an assembly path.
- `town/`, `internal/land` and `internal/combat` join the exact set.
- A CI test runs the same inputs through gc and the TinyGo WASM and compares `Float64bits`.
- **Behaviour:** none (bit-exact by construction). **Proof:** the golden plus a mutation (put `math.Exp` back, and
  the test fails).

## W2 · The stage core

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
- **Proof.** A golden of today's disc: the composer, fed the seeded pins, places every village, lot and building where
  today's code does. A shuffled-input test shows no dependence on id order.
- **Behaviour:** none visible. The logged layout stops being the source of truth.

## W3 · Server-built land, streamed

- The game server builds each space whole, using today's land build for now.
- It cuts the result into `WriteHex` chunks with ids per space version, plus the strategy overview (hex graph, low-res
  height and biome, zones, placements, portals).
- The page streams chunks and the overview and no longer builds land.
- `Place`, `hex` and the land build leave the WASM, which keeps `Step` and nav.
- The town page, `cmd/townpin` and the old district layout retire.
- **Proof:**
  - a frame golden of the world page before and after, at the same pose;
  - load traces against `01-baseline/load-profile.md`;
  - the WASM size.
- **Behaviour:**
  - The first load needs the server.
  - A land change lands on the next reload, unchanged.
  - The realm's land hash becomes the space version.
- **Owner check:** a reload on desktop and on the phone, timed.

## W4 · Generators, one rank at a time (repeated rounds)

- **Ported in rank order, one round each:**
  1. halls and ramps (today's inversion);
  2. the wilds' centres;
  3. massifs;
  4. pits;
  5. tunnels;
  6. caves;
  7. roads and the rest;
  8. decor rules last: the golden trails, sunflowers and grass.
- **Each generator:**
  - reads only stronger ranks;
  - emits its connectors as nav edges;
  - is pure, with a shuffled-input test.
- **Proof per round:** the land golden of the untouched ranks is identical. The ported rank's diff is reviewed, and the
  owner plays it (massif trailhead reach, tunnel mouths, spirals).
- **One deliberate re-baseline** of `world-land.fixture.json` and `dense.json`, at the round where positions move to
  the new form.
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
- Picking moves from rendered meshes to footprint colliders.
- **The glTF import pipeline:** a schema row with the footprint taken from the mesh, plus a visual asset.
- **Proof:** a frame golden under the default theme, identical to before. A second theme renders with no new stage
  version.
- **Owner check:** a look at both themes, and one imported model placed by a pin.

## W6 · The sim package and the layered nav graph

- **The `sim` package:**
  - `Step` with the ship, the lunge and the auto-hop in it;
  - fight and mobs pulled out of `internal/realm`;
  - the realm builds all of it, and the WASM builds `Step` and nav.
- **Nav:** the two-level nav on the layered graph, with (hex, floor) nodes, generator connectors and decor solids
  narrowing passages.
- **Law 2:** every addressable prim reachable from the spawn, checked in the build. Its cost is measured on the 2× disc
  first.
- **Proof:**
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

W1 → W2 → W3, then W4 and W5 in parallel (server files against page files), then W6 → W7 → W8. W6's nav graph needs
W4's connectors, so it starts after the tunnels and caves rounds of W4.

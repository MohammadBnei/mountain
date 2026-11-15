# ADR: placement, one composed stage for everything that stands, one style spec for how it looks

Status: proposed 2026-11-15, after an architect round with the owner (conversation, no page) and one doubt round
(`../05-doubt/placement.md`, findings resolved the same sitting). It replaces the placement contract section of
`sim-world.md`.

## Context

The world will mostly be derived or generated, with some hand work: the owner pins places and imports 3D models.
Within 12 months, agents write quests whose storylines send players to places, for example classes on design
patterns or incident response inside an app's house. The quest must not care whether a place was authored,
projected from the graph or generated. Separating the origin from the placement is the purpose of this contract.

What stands in the way today:
- **Two placers with their own rules.** `town/placement.json` pins with `Place` (`town/place.go:170`) for the old
  district layout, and the logged village layout `LayOut` (`town/village.go:792`, `game/layout.go:12-19`), each new
  layout stepped from the last. Without pinned districts, `Place` puts centres by index over the civ count
  (`place.go:200-211`), so one new civ moves every unpinned district.
- **A narrow, hard-coded source view.** The layout reads `DiscNode{ID, Layer, Civ, Audience}`, the roads and the
  anchors (`town/disc.go:106-121`). Which kinds are placed is a switch on `Kind`, `Function` and `Parent`
  (`town/place.go:115-128`).
- **Many position forms:**
  - axial hexes (`Village.At`, `Lot.Hex`; `internal/land/types.go:25-40`, `town/village.go:105`);
  - float Q/R on buildings (`Building.Hex`, `types.go:106-115`);
  - anchors in thousandths of a unit (`town/disc.go:115-121`, `:233`);
  - float xz (`Disc.At`, `BuildWith` buildings, `land.go:985-1003`);
  - pieces on a 0.5 grid with quarter-turn yaw (`world-build.ts:50-58`);
  - planted edits in float xyz (`internal/realm/log.go:98-110`);
  - a global `Scale` and a snap applied afterwards (`land.go:996`).

  Lots are absolute, so a moved village does not carry its lots.
- **Silent nudging and dropping.**
  - `snapDisc` moves colliding buildings by id order (`freeNear`, `hex.go:289-306`).
  - Stages drop what doesn't fit and record it only in `HexLand.Dropped` (`massif.go:268`).
- **The land stages are an ordered pipeline with inversions** (`stages`, `hex.go:770-779`). The tunnel ramps are
  chosen in `landOf` before any hex stage (`land.go:724-768`), and massifs keep off them (`massif.go:206-213`). Yet
  `stageTunnels` later drops ramps where a massif or pit landed (`tunnel.go:357-370`). The wilds' circles (decor)
  block massif sites (`massif.go:189-193`), and so do fun-park props (`:203-205`). Pits read massifs; caves read pits
  and massifs; spirals read plots, massifs and ramps.
- **One registry row mixes three concerns.** The decor model row (`internal/land/registry.go`, generated into
  `world-registry.ts:10`) holds footprint (`R`, `H`, `Box`, `Stepped`), looks (`Far`) and gameplay (`Node`,
  `Plant`, `Wild`).
- **Page tables mix looks with behaviour:**
  - `KINDS` in `world-hex.ts:84` holds walk, swim, wet, ground and rise;
  - `world-build.ts:39` holds sizes, cost, deck and steps;
  - `world-targets.ts:123` is targeting, and building hits come from rendered meshes, so looks decide picking today.
- **Nothing proves reachability**, and the one-height step rule (`stepHexes`, `hex.go:1331`) cannot see tunnels,
  caves, bridges or the decor steps up a massif (`climb_test.go:76`).
- **The WASM ships the whole land build and `Place`** (`cmd/physwasm/main.go:47`, `:68`; 561 KB).

## Decision

Two orthogonal axes. The placement axis says what stands where. The visual axis says how it looks. The model is
borrowed from Pixar's USD (composition) and Mapbox GL's style spec (styling). Neither library is used: a small, pure
Go composer runs on the server only.

### The placement axis: a composed stage

- **Prims.** Everything that stands is a typed prim at a path: `/disc/atlas/house/replica`. Its schema type is one
  of `Space`, `Region`, `Site`, `House`, `Room`, `Portal`, `Anchor`, `Decor`, ... and comes from a registry row
  declaring its fields once. Positions are relative to the parent prim, so moving a village moves its lots.
- **Position.** Integer axial hex plus face on the space's own grid, with an optional sub-hex offset wherever player
  builds, plantings or mounts can stand, and for decor inside a chunk. World units are derived in one Go function and
  never stored; the scale lives in the space file.
- **Footprint is on the stage.** A prim's collision footprint comes only from its schema row, never from its looks.
- **Movers are not on the stage.** Only their anchors are: a lair, a home, a feeding ground. A mover's path and
  life belong to the sim (`sim-world.md`).
- **Ranks by strength, pure.** Every source has a rank, strongest first:
  1. pins (the owner);
  2. derived projections, for example an app's architecture becoming its house's rooms (atlas's graph and
     `internal/yard`, `archetypes.go:66-90`);
  3. each generator on its own declared rank, for example halls and ramps, wild centres, fun props, massifs, pits,
     tunnels, caves (the order is decided when each is ported; spike S1);
  4. decor rules (golden trails, sunflower rings, grass).

  A source reads only the stronger ranks' output, as masks and anchors, never a weaker rank. Inside one generator,
  peers (spacing between ramps, between massifs) resolve by a fixed per-id hash priority: a new prim moves only the
  lower-priority peers it hits, and the build report lists them (spike S1). On
  the same prim the stronger opinion wins. Readers never see which rank wrote a prim. Today's inversions go: the
  tunnel ramps become their own generator above massifs, and the wilds' centres become a generator above massifs,
  with only the trails below them left as decor.
- **Composition arcs borrowed from USD:**
  - *References:* a template instanced many times with overrides. A "data house" is referenced by every app with
    Postgres. A derived layer writes each app's overrides, and the owner authors only template families.
  - *Payloads:* a lazy reference. An interior or a dungeon is a payload, loaded only when someone enters or strategy
    watches it, and it becomes its own space.
  - *Variants:* named switches on a prim (season, damaged or repaired, quest stage). Variants may change looks and
    quest state, never the footprint.
  - Inherits and specializes are not borrowed; the style spec's selectors cover class-wide looks.
- **Generators are pure.** A generator's output is a function of (its bounds, the stronger ranks, the source
  projection, params, seed, version). It has no history and does not depend on id order: per-id hash slots, tested by
  shuffling the input. Its output is points with attributes (path or chunk index, kind, tags, position, face,
  scale), Houdini-style. A changed generator version or params makes a new space version.
- **Generators emit their connectors.** A generator that makes vertical structure declares its ramps, tunnel
  mouths, stairs, bridges and portals as nav edges, so nobody guesses how levels join.
- **Decor is anonymous.** Only its rule, params and seed are stored. Land decor instances become `(chunk, index)`
  (today they are indexes into `w.Nodes`, `resources.go:114`). A quest that needs one particular tree promotes it to
  a named prim in a stronger rank. Planted and felled objects keep the realm log's own `uint32` ids and float
  positions; they are player state, not placement.
- **The source interface.** A derived layer reads one versioned projection of atlas, `Element{ID, Kind, Group,
  Layer, Audience, Parent, Links}`. `Kind` is a registry row that says whether and as what an element is placed,
  which replaces the switch in `place.go`. A new atlas field never moves the map.
- **Files.** Pins, templates, generator params and decor rules are git files, pinned by commit sha and projected by
  atlas (`sim-world.md`). Generators and derived layers write proposals for review. Nothing pinned moves on its own.
- **Server only.** The composer and every generator run in the game server. The page streams chunks; the WASM keeps
  only `Step` and nav, and `Place` and the land build leave it.

### The laws, checked on every build

1. **A pin always wins.**
   - Two pins colliding fail the build and name both prims.
   - A derived or generated prim that collides with a stronger prim never moves it. The space keeps its last good
     version, and a proposal naming the collision is filed for review. The world never goes stale or down because of
     a repo change.
   - A weaker rank masking around a stronger one is normal, and every masked or dropped prim is listed in the build
     report. `freeNear` and silent drops go.
2. **Everything addressable is reachable, on a layered nav graph.**
   - The nodes are (hex, floor): the ground, a tunnel or cave level, or a deck top (a bridge, the steps).
   - The edges are the step rule between floors plus the connectors the generators emit.
   - Decor solids narrow passages: a hex side counts as passable only if a rider's radius fits between the
     colliders.
   - From each space's spawn, every door, every quest-tagged prim, every portal and every mover anchor needs a
     route. A failure names the prim and the blocker.
   - The graph is the sim's coarse nav level, so the proof and the game agree. Spike S2 measured it on the 2× disc:
     about 43k nodes and 250k edges, built in about 150 ms and searched in 1.4 ms, on the server per build.
   - The proof is stated for a rider with jumps, plus a walk-only mode for places that must be reachable on foot.
3. **Roles are stable.** After a rigid move of a space, every prim of a declared role still exists (tag counts
   equal). Today a 1-hex move loses 3 of 7 tunnel ramps (`tunnel.go:357-370`) or crashes `stagePasses`
   (`hex.go:685`) (spike S4).
4. **Looks never change gameplay.** The visual axis carries no footprint field, and the sim never imports it.
   Picking uses footprint colliders, not rendered meshes.

### Reading the stage

- **Paths** for hand-written story beats.
- **Queries** for everything reusable. A quest asks "a room tagged `data.replica` in the house of the app this quest
  teaches", Skyrim-Radiant-alias style, so one agent-written quest runs in every house that has one.
- **Who resolves a query.** The realm resolves it when the quest starts and pins the result to a prim path and a
  space version, so version churn never rebinds a running quest silently. Two matches take the query's declared
  tie-break; a query with none and two matches fails at quest load.
- **When the place leaves.**
  - If the source changed (the replica left the graph), the quest retires or its agent rewrites it.
  - A template that stops providing a declared role fails under law 1.
  - A quest is never silently pointed somewhere else.

### The visual axis: a style spec

- **Rules.** A style is a list of rules. Each rule has a filter over kind and tags (`building.data`, `tree.elder`)
  and properties: asset, material, tint, scale, sway.
- **Expressions.** Properties may be data-driven: zoom stops (one style serves the strategy overview and the street),
  quality tier (phone, desktop), theme variables (season, hour, like Unreal's Material Parameter Collections) and prim
  attributes.
- **Weighted variants.** Variants are seeded by position (Minecraft-blockstate style): a living spread with no
  runtime randomness. They come from a declared asset list (name, triangles, height, footprint class) and swap only
  within one footprint class, so they stay inside the footprint.
- **Selectors read composed data.** Kind and tags come from the stage, never from kit names (today `bigTrees` and the
  critters match names). Far shapes and the minimap read the same spec, so near and far never drift (spike S3). Visual uncertainty is free; footprint uncertainty lives only on
  the stage, under law 2.
- **Themes are stylesheets.** Swapping one restyles the world without a new stage version or chunk id.
- **Under the spec,** a keyed patcher applies the computed result to the three.js scene, keyed by prim path or
  `(chunk, index)`. It runs on chunk stream in and out, on live edits (`internal/realm/log.go:155-175`) and on zoom
  swaps. A version change waits for the next reload. Preact stays on the HUD.
- **Where today's fields go:**

| Field today | Home |
|---|---|
| `KINDS` walk, swim, wet, ground, rise (`world-hex.ts:84`) | Hex kind schema rows (stage) |
| Piece sizes, cost, deck, steps (`world-build.ts:39`) | Piece schema rows (stage) |
| Targeting (`world-targets.ts:123`) | Sim and HUD, over footprint colliders |
| `Model.R`, `H`, `Box`, `Stepped` | Model schema row (stage) |
| `Model.Node`, `Plant`, `Wild` (cover growth) | Model schema row (gameplay) |
| `Model.Far`, the minimap's shapes and colours | Style spec, with zoom stops |
| `modelOf` (`world-yard.ts:151`), `DECOR` (`world-land.ts:140`) | Style rules |

### Imported models

The owner imports glTF models. Import is automated and writes two committed files:
- a schema row, with the footprint (hex footprint and height from the mesh's bounds or a simplified hull) and any
  gameplay (`Node`, `Plant`);
- a visual asset that the style spec references.

Editing the row afterwards is free. A re-import shows a footprint diff in review.

### Chunk ids

Chunk ids are per space version. Wilds, spirals and massifs are whole-space, so a per-chunk hash of only what a
chunk reads does not hold. `sim-world.md` follows.

## Migration

1. Seed the pins file from the current layout snapshot, covering both placers: the logged `LayOut` plan and
   `town/placement.json`.
2. Land `Find` and the law harness, and migrate the exposed tests to them (spike S4: 47 literal-position and 18
   snapshot tests, about 75 mechanical id lookups and 14 gate sites, about 70 hours). Then build the composer, the
   position form, the registry rows and law 1 on the seeded pins.
3. Port the land stages one by one to pure ranked generators, each under the golden test. The halls and ramps come
   first, because they are today's inversion, then massifs.
4. Build the layered nav graph and law 2, and measure its cost.
5. Remove `Place`, the land build and `hex` from the WASM. The logged layout and the town page retire.

The big-bang rewrite is accepted (owner), done incrementally.

## Alternatives considered

| Option | Why not |
|---|---|
| A: keep today's split (pin files per kind, generators with their own outputs, per-kind look tables) | Many position forms stay; readers must know the origin; quests can't be reused across houses |
| B: hand-author everything | Thousands of stones by hand; no generated quests or realms |
| An explicit ordered pipeline inside the generated layer | Matches today's code, but keeps hidden order and inversions; the owner chose a pure system |
| D: the stage in a live database, edits streamed | Breaks atlas's rebuild-from-repos rule and git review; the owner accepts next reload |
| E: adopt OpenUSD itself | No Go or TinyGo library; a C++ runtime. Its model is borrowed instead |
| Composer in the page's WASM | The page streams chunks; it would only cost size and the bit-exact burden |
| Hex-level reachability | Wrong on tunnels, caves, bridges and decor-made climbs |
| Looks on the stage | A theme change would make a new stage version and invalidate chunks; looks could change gameplay |

## Consequences

- An origin can be swapped (a pin replaced by a generator, or the reverse) without any reader changing.
- `town/place.go`'s kind switch, the many position forms, the logged layout, `freeNear`, silent drops and the
  per-kind look tables go.
- Every land stage is rewritten as a pure ranked generator. That is the largest cost.
- A quest becomes a reusable class over queries.
- A new look is a style rule; a new model is an import; neither touches the sim.
- The WASM shrinks to `Step` and nav.

## The bets

1. The land stages can be rewritten as pure ranked generators, each reading only stronger ranks, and keep today's
   feel: massifs keep their trailhead reach, tunnels keep their mouths, wilds keep their spirals. Each port is checked
   by the golden test and played before the next one.
2. The layered nav graph proves reachability on the 2× disc cheaply enough to run on every build.

## Not doing

- USD's file format or runtime.
- The inherits and specializes arcs.
- Movers on the stage.
- Named prims for every decor instance.
- Live stage edits mid-session, beyond the realm's existing plant and fell log.
- The composer in the page.

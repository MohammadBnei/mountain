# ADR: placement, one composed stage for everything that stands, one style spec for how it looks

Status: proposed 2026-11-15, after an architect round with the owner (conversation, no page). A doubt round follows
(`../05-doubt/placement.md`). It replaces the placement contract section of `sim-world.md`.

## Context

The world will mostly be derived or generated, with some hand work: the owner pins places and imports 3D models.
Within 12 months, agents write quests whose storylines send players to places, for example classes on design
patterns or incident response inside an app's house. The quest must not care whether a place was authored,
projected from the graph or generated. Separating the origin from the placement is the purpose of this contract.

What stands in the way today:
- **Placement reads the graph through a narrow, hard-coded view.** The layout reads `DiscNode{ID, Layer, Civ,
  Audience}`, the roads and the anchors (`town/disc.go:106-121`). Which kinds are placed is a switch on `Kind`,
  `Function` and `Parent` (`town/place.go:115-128`).
- **Positions come in four forms:**
  - axial hexes (`Village.At`, `Lot.Hex`; `internal/land/types.go:25-40`, `town/village.go:105`);
  - anchors in thousandths of a unit (`town/disc.go:120`, `:233`);
  - float xz (`Disc.At`);
  - a global `Scale` and a snap applied afterwards (`internal/land/land.go:996`).

  Lots are absolute, so a moved village does not carry its lots.
- **The layout is logged history.** Each new layout is stepped from the last (`town/village.go:792`, `game/layout.go`).
- **Most placement happens out of sight, inside the land build.** Massifs (`internal/land/massif.go:270`), pits
  (`pit.go:64`), tunnels and halls (`tunnel.go:357`, `:218`), caves (`cave.go:75`) and the wilds' golden spirals
  (`spiral.go:109`) each place things by their own rules, on their own cell grid, in no file.
- **One registry row mixes three concerns.** The decor model row (`internal/land/registry.go`, generated into
  `world-registry.ts:10`) holds footprint (`R`, `H`, `Box`, `Stepped`), looks (`Far.Shape`, `Far.Colours`) and gameplay
  (`Node`, `Plant`).
- **The page has several per-kind tables that mix looks with behaviour:** `KINDS` in `world-hex.ts:84`,
  `world-build.ts:39` and `world-targets.ts:123`; `modelOf` in `world-yard.ts:151`; `DECOR` in `world-land.ts:140`.
- **Nothing proves reachability.** A build can block a door, a portal or a quest place, and nothing fails.

## Decision

Two orthogonal axes. The placement axis says what stands where. The visual axis says how it looks. The model is
borrowed from Pixar's USD (composition) and Mapbox GL's style spec (styling). Neither library is used: we write a
small Go composer, pure and in the exact set, so it builds under TinyGo as well.

### The placement axis: a composed stage

- **Prims.** Everything that stands is a typed prim at a path: `/disc/atlas/house/replica`. Its schema type is one
  of `Space`, `Region`, `Site`, `House`, `Room`, `Portal`, `Anchor`, `Decor`, ... and comes from a registry row
  declaring its fields once. Positions are relative to the parent prim, so moving a village moves its lots.
- **One position form.** Integer axial hex plus face, on the space's own grid. World units are derived in one Go
  function and never stored; the scale lives in the space file. Decor instances inside a chunk may carry a sub-hex
  offset.
- **Footprint is on the stage.** A prim's collision footprint comes only from its schema row, never from its looks.
- **Movers are not on the stage.** Only their anchors are: a lair, a home, a feeding ground. A mover's path and
  life belong to the sim (`sim-world.md`).
- **Layers by strength,** strongest first:
  1. pins (the owner);
  2. derived projections, for example an app's architecture becoming its house's rooms, read from atlas's graph and
     `internal/yard` (`archetypes.go:66-90`);
  3. generators (massifs, roads, tunnels, dungeons, later endless regions);
  4. decor rules (golden trails, sunflower rings, grass).

  On the same prim, the stronger opinion wins. A weaker layer treats every stronger layer as a keep-out mask.
  Readers never see which layer wrote a prim.
- **Composition arcs borrowed from USD:**
  - *References:* a template instanced many times with overrides. A "data house" is referenced by every app with
    Postgres. A derived layer writes each app's overrides, and the owner authors only template families.
  - *Payloads:* a lazy reference. An interior or a dungeon is a payload, loaded only when someone enters or strategy
    watches it, and it becomes its own space.
  - *Variants:* named switches on a prim (season, damaged or repaired, quest stage). Variants may change looks and
    quest state, never the footprint.
  - Inherits and specializes are not borrowed; the style spec's selectors cover class-wide looks.
- **Generators are pure.** A generator's output is a function of (its bounds, the stronger layers, the source
  projection, params, seed, version), with no history and no order dependence between ids (id hashes, as `Place`
  does today, `town/place.go:169`). Its output is points with attributes (path or chunk index, kind, tags, position,
  face, scale), Houdini-style. Changing a generator's version or params makes a new space version.
- **Decor is anonymous.** Only its rule, params and seed are stored. Instances are `(chunk, index)` inside chunks, as
  the realm's edit log already addresses planted and felled objects (`internal/realm/log.go:98-110`, `:169`). A quest
  that needs one particular tree promotes it to a named prim in a stronger layer.
- **The source interface.** A derived layer reads one versioned projection of atlas, `Element{ID, Kind, Group,
  Layer, Audience, Parent, Links}`. `Kind` is a registry row that says whether and as what an element is placed,
  which replaces the switch in `place.go`. A new atlas field never moves the map.
- **Files.** Pins, templates, generator params and decor rules are git files, pinned by commit sha and projected by
  atlas (`sim-world.md`). Generators write proposals for review; nothing placed moves on its own. Today's stepper
  becomes the first derived placer once it reads pins instead of the log.

### The laws, checked on every build

1. **A pin always wins.** A collision between prims of the pin, derived or generator layers fails the build and
   names both prims. Nothing is carved or nudged silently.
2. **Everything addressable is reachable.** From each space's spawn, every house door, every prim tagged for quests,
   every portal and every mover anchor has a route on the coarse hex nav graph under the step rule (`StepOK`). A
   failure names the prim and the blocker. The check is hex-level only, so it costs milliseconds.
3. **Looks never change gameplay.** The visual axis carries no footprint field, and the sim never imports it.

### Reading the stage

- **Paths** for hand-written story beats.
- **Queries** for everything reusable. A quest asks "a room tagged `data.replica` in the house of the app this quest
  teaches", Skyrim-Radiant-alias style, so one agent-written quest runs in every house that has one.
- **When a query's place no longer exists:** if the source changed (the replica left the graph), the quest retires
  or its agent rewrites it. A template that stops providing a declared role fails the build (law 1's family). A quest
  is never silently pointed somewhere else.

### The visual axis: a style spec

- **Rules.** A style is a list of rules. Each rule has a filter over kind and tags (`building.data`, `tree.elder`)
  and properties: asset, material, tint, scale, sway.
- **Expressions.** Properties may be data-driven: zoom stops (one style serves the strategy overview and the street),
  quality tier (phone, desktop), theme variables (season, hour, like Unreal's Material Parameter Collections) and prim
  attributes.
- **Weighted variants.** Variants are seeded by position (Minecraft-blockstate style): a living spread with no
  runtime randomness, always inside the footprint. Visual uncertainty is free; footprint uncertainty lives only on
  the stage, under law 2.
- **Themes are stylesheets.** Swapping one restyles the world without a new stage version or chunk id.
- **Under the spec,** a keyed patcher applies the computed result to the three.js scene, keyed by prim path or
  `(chunk, index)`. It runs on chunk stream in and out, on live edits (`internal/realm/log.go:155-175`) and on zoom
  swaps. A version change waits for the next reload. Preact stays on the HUD.

### Imported models

The owner imports glTF models. Import is automated and writes two committed files:
- a schema row, with the footprint (hex footprint and height from the mesh's bounds or a simplified hull) and any
  gameplay (`Node`, `Plant`);
- a visual asset that the style spec references.

Editing the row afterwards is free. A re-import shows a footprint diff in review. Today's `WorldModel` row splits
the same way.

## Alternatives considered

| Option | Why not |
|---|---|
| A: keep today's split (pin files per kind, generators with their own outputs, per-kind look tables) | Four position forms stay; readers must know the origin; quests can't be reused across houses |
| B: hand-author everything | Thousands of stones by hand; no generated quests or realms |
| D: the stage in a live database, edits streamed | Breaks atlas's rebuild-from-repos rule and git review; the owner accepts next reload |
| E: adopt OpenUSD itself | No Go or TinyGo library; a C++ runtime and a large WASM. Its model is borrowed instead |
| Looks on the stage | A theme change would make a new stage version and invalidate chunks; looks could change gameplay |

## Consequences

- An origin can be swapped (a pin replaced by a generator, or the reverse) without any reader changing.
- `town/place.go`'s kind switch, the four position forms, the logged layout and the per-kind look tables go.
- Every land stage becomes a generator behind one contract. That is the largest cost.
- A quest becomes a reusable class over queries.
- A new look is a style rule; a new model is an import; neither touches the sim.

## The bet

Today's land stages can be rewritten as generators that each read only the stronger layers as masks. The doubt
round in `sim-world.md` found that most stages are global and read one another (F3): massifs, pits, roads, tunnels.
If they depend on each other in a cycle, strength order alone can't express it. The generated layer then needs
ordered stages inside it, a pipeline rather than a pure composition. The first slice ports massifs and roads to
find out which holds.

## Not doing

- USD's file format or runtime.
- The inherits and specializes arcs.
- Movers on the stage.
- Named prims for every decor instance.
- Live stage edits mid-session, beyond the realm's existing plant and fell log.
- Fine-grid reachability; the hex-level proof only.

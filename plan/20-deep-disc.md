# 20 · The deep disc: a thick slab, mountains and cavities (draft, 2026-11-02)

> Status: **draft for the owner's review.** Nothing is built. The owner asked on 2026-11-02: "let's make the disc much
> thicker, so we can add mountains and cavities", and chose "plan first". This plan states the facts as built, the
> design choices, the slices, and the questions that only the owner can answer. Once they are answered, it gets a doubt
> cycle like 17, 18 and 19.

## The disc today (ukubi-game main, 06904c7)
- **Size.** The dev world's radius R is about 540 units, with about 11,000 hexes. A hex is 9.9 units flat to flat.
- **Relief.** Ground heights come from `heightAt` (world-land.ts): region forms, rivers, lakes, pads. It is capped at
  `MAX_RELIEF` 170. The hexes quantise it into levels 0 to `MAX_LEVEL` 8 (`LEVEL_H` 8 heightAt units a level). The
  terraces are Fibonacci: 0, 2, 3, 5, 8. Hills sit at level 5 and mountains at 8.
- **Drawn height.** A level is `STEP` = HEX/φ³, about 2.34 units, so the highest ground stands about 19 units over the
  datum. A rider climbs at most 2 STEP onto the next hex (`MAX_CLIMB`).
- **Depth.** The slab's belly is at `-THICK` = -8 (world-rings.ts), just under the lowest carve: a river bed or the sea
  floor at about -5. So the slab is 1.5% of the radius thick. The tall blue side in the overview is the Rimfall's water
  curtain, not the rock.
- **What hangs off the belly:**
  - the underside (world-under.ts): the turtle, one elephant per host, beams per VM and container, and roots from
    buildings down through the land;
  - the control room's window (world-control.ts) at -THICK;
  - the ship's flight under the rim (world-ship.ts);
  - the Rimfall lip and the water ring (world-rings.ts).
- **Caves.** There are none: a heightfield cannot hold an overhang (world-land.ts:126 says so).

## Goal
A disc that reads as a thick, solid world from orbit (a slab with real depth under the land), with mountains worth
climbing or flying round, and cavities to go down into. It must keep the rules the world already has:
- the land is the owner's real systems (17's north star);
- on foot it should feel good to move;
- the budgets hold (220 draws; 350k triangles on desktop and 200k on the phone at orbit).

## Design

### 1. The slab (thickness)
- Raise `THICK` from 8 to a share of R, proposed **R/φ³ (about 128 at R 540)**, so the side reads as rock from orbit.
- Draw the side as strata: banded rock (earth, stone, a darker deep layer) in the theme-neutral palette of the land,
  one draw like the rim today.
- Things that move with the belly:
  - the underside's turtle, elephants and beams (their gap to the belly is measured from -THICK already);
  - the roots (longer);
  - the control-room window;
  - the ship's under-rim path (the clearance grows);
  - the camera's underside view and its clip planes (clipOf).
- The Rimfall pours off the lip as now; with a thick slab it falls past the strata, which is the Discworld picture.

### 2. Mountains (height)
- Extend the terraces past 8 with the next Fibonacci steps: **0, 2, 3, 5, 8, 13, 21**. Mountains rise to level 21,
  about 49 units, and hills stay at 5 to 8.
- Mountains form in a region kind (world-land.ts regions) that the layout places away from the villages: villages and
  roads keep flat or terraced ground (pads win, as now).
- Walking stays the same rule: a rider climbs at most 2 STEP a hex. A mountain is climbed by its passes and switchbacks
  (a carved path of 1 to 2 step terraces). Its faces are cliffs, which the bike and the knight cannot climb, and which
  the ship flies over.
- The tiles: the kit has mountain and rock tiles. A face over a big drop stands on a stack of blocks (blocksOf, as
  terraces do today); a 13-level face is about 5 blocks tall.
- Budget: taller faces mean more side quads on the map mesh and more blocks in the detail. Mountain chunks are counted
  in TRI_CAP like any other (the low tier's skip rule already covers a heavy chunk).

### 3. Cavities (depth)
Three kinds, from cheap to costly. The proposal is to build the first two now and keep the third for later.
- **a. Pits and canyons (heightfield, open to the sky).** The slab's depth lets carves go well under the datum: a
  sinkhole, a gorge, a quarry. They are walkable by the same climb rule, with stairs or ramps carved down their sides.
  This costs one more region form and a lower `LAND_FLOOR`. No overhang.
- **b. Caves as interiors.** A cave mouth (a prop on a cliff face or at a pit's foot) is a door. Walking in loads an
  interior, as a building's does (world-view.ts interiors): a few chambers modelled from kit rock pieces, lit by
  lanterns. The world does not need real overhangs for this, and the existing door, interior and leave code is reused.
- **c. Real tunnels through the slab (later).** Meshes under the heightfield with their own collision. They are costly:
  the ground is no longer one height per point, so rays, the riders' ground, the residents' paths and the shots all
  need a second surface. Not proposed for this plan.

### 4. Meaning (the owner's systems)
Every place on the disc stands for something real (17). A proposal, to confirm:
- **Mountains are the heavy platform:** the cluster's control plane, storage and the observability stack, the hard
  ground everything stands on, and the highest peak for the busiest.
- **Pits and caves are the data:** Postgres and the stores dug into the slab, backups as deep vaults, and a cave per
  data VM.
- **The slab's strata are the stack's layers,** seen from the side: apps on top, platform below, hardware at the belly
  over the turtle.

## Slices
- **T1: the slab.** THICK to R/φ³; the strata side; the underside, roots, window, ship path and clip planes moved;
  gates updated. Small, and visible at once.
- **T2: the mountains.** Terraces to 21; the mountain region; passes and switchbacks; cliff colliders; budgets checked
  on the phone tier.
- **T3: pits and canyons.** A lower floor; the carve forms; ramps.
- **T4: caves as interiors.** Cave mouths as doors; chamber interiors from kit pieces; what each cave holds (4).
- **Later: tunnels (3c).**

## Owner questions
1. **Thickness:** R/φ³ (about 128, the side about an eighth of the disc's width), or thinner or
   thicker?
2. **Mountains:** levels to 21 (about 49 units, 2.5 times today's highest) with walkable passes, or taller still and
   reachable only by ship?
3. **Cavities:** pits and canyons plus caves as interiors now, and tunnels later, as proposed?
4. **Meaning:** mountains for the heavy platform and caves for data (4), or another reading, or scenery only?
5. **Order:** T1 first on its own, so the slab can be seen before the terrain work?

## Owner answers (2026-11-02)
1. **Thickness:** R/φ³, about 128.
2. **Mountains:** taller, summits by ship only. On foot a rider reaches the foothills (levels to 8, as now); the
   mountains rise well past them (about 90 units and more), their faces cliffs.
3. **Cavities:** all three now: pits and canyons, caves as interiors, and real tunnels. And: "for buildings too, let's
   build rich interior worlds so we can use the same third person view": a building's inside becomes a world walked
   in the same chase camera as outside, not a separate view.
4. **Meaning:** mountains for the platform, pits and caves for the data. And: "I don't like the traveling packages.
   Let's build the connection through apps and things through tunnels or something more coherent with the world": the
   connections between apps (today's road traffic and its moving packages) run through tunnels in the slab instead.

### What the answers change
- **Tunnels are the connections.** The roads' traffic (the packages that travel the data and auth roads) goes. A
  connection between two buildings is a tunnel through the slab: its mouths by each building, its bore lit by its
  health and load (a glow that pulses with traffic, red when the edge fails). The roads stay on top as the ways people
  walk. This is 3c built, with its own collision and a second ground for whoever walks or rides in it.
- **Interiors are worlds.** Buildings and caves share one interior kind: rooms modelled from kit pieces, walked in the
  third-person chase camera with the same controls, collision and shooting as outside. A building's rooms show its
  real parts (17's "walk into a service, see its code, pods, history and roads").
- **Mountains** need no walkable passes to their summits: the foothills keep the climb rule, the faces block, the ship
  lands on the tops.

### Slices, revised
- **T1: the slab.** As above.
- **T2: the mountains.** Peaks past the foothills, cliff colliders, ship landings on the tops, budgets on the phone.
- **T3: pits and canyons.**
- **T4: tunnels as connections.** A tunnel per road edge; the packages retired; the bore's ground, collision, light
  and health; mouths by the buildings.
- **T5: interior worlds.** One interior kind for buildings and caves, in the chase camera; rooms from kit pieces; a
  building's real parts inside; cave mouths as doors.

Each slice gets its own short plan section and a doubt cycle before it is built.

## T2 · The mountains (design for review, 2026-11-02)

> The slab (T1) is merged (ukubi-game #137). This section designs T2 to the level a build can follow; it gets a doubt
> cycle before any code.

### The land today (ukubi-game main 8c8a0a3)
- The levels are `TERRACES` 0, 2, 3, 5, 8 (`world-hex.ts`), `STEP` = HEX/φ³ ≈ 2.34 units a level. A grass hex at level
  `HILL_L` 5 or more is a hill (a share of them, `HILL_SHARE`), at `MOUNT_L` 8 a `mountain` hex with a kit mountain
  model on it. Nothing stands higher than level 8, about 19 units.
- Heights come from `heightAt` (`world-land.ts`): each zone's `Region` (capital, cities, countryside, frontier) blends
  base, amplitude, frequency and terraces; `MAX_RELIEF` 170 caps it. The frontier's highlands (amp 92) are the only wild
  high ground.
- A rider climbs at most `MAX_CLIMB` (2 STEP) a hex (`stepOK`); the passes (`lower`, step 8 of `hexLand`) cut every
  road down to that, so every village is reachable on foot.
- A tile over a drop stands on a stack of unstretched blocks (`blocksOf`, a block a STEP), each a few triangles.
- The ship flies to `CEILING` 300 and stands on what is under it (`floorOf`).
- **The land is now pinned twice** (plan/21 phase 1b, ukubi-game #151): `internal/land` is a Go copy of the whole
  `hexLand` chain, held equal to the TypeScript by goldens (`bun dev/land-golden.ts`). Any land change lands in both
  languages in one PR, with the goldens re-recorded, and plan/21 phase 3 later makes the Go copy the only one.

### What T2 adds
- **Massifs, not noise.** A few named massifs, each one of the owner's platform systems (the owner's reading: mountains
  are the platform). The proposal: one massif per platform group the town already knows (the `platform` civ's
  services, grouped as control plane, storage, observability, networking), placed in the frontier behind the Platform
  village, away from every lot, road and pad. A massif's height grows with its group's weight (its services' count;
  later its load), from about 40 to about 90 units.
- **Heights past the terraces.** New levels for the massifs only: 13, 21, 34 (the Fibonacci terraces go on), so a
  massif reads as a few huge steps, then a summit. Everything under level 8 stays as it is today; the villages, roads,
  passes and the land's hash outside the massifs do not move (the goldens' diff shows only the massif hexes).
- **The rule on foot.** The foothills (levels up to 8) keep the climb rule. A face above 8 is a cliff: `stepOK` already
  refuses a climb over `MAX_CLIMB`, so the summits are reachable only by ship (the owner's answer). No pass is cut to a
  summit; a road never crosses a massif (the road router already avoids high ground: verify).
- **The look.**
  - Up close, the faces stand on stretched blocks: one block per terrace step, scaled to its height, instead of one per
    STEP (a 21-level face would be 21 blocks; stretched, it is 3). The map mesh's side quads already draw any drop.
  - The summit hex carries the kit's mountain model, scaled up with the massif. A thin snow tint on the top terraces,
    from the land's own palette.
  - From orbit the massifs throw the land's longest shadows: the sun is already cast from the day cycle.
- **The ship.** A summit is a landing place: the ship's floor is the ground under it, so it already stands on a top. A
  small flat pad on each summit (one hex, level ground) so the landing reads, with the massif's name on a board.
- **The meaning in the world.** A massif's card (a click on its summit, or from the map) names its group and lists its
  services with their health, as a village's card does. Its face colour shifts with the worst health in it (the land's
  palette, never a neon). Visitors see the massif and its name, never its health (owner-only data stays on the owner's
  pod).

### Budgets
- Triangles: a massif of about 60 hexes, stretched blocks, is about the cost of a hill range today. Checked on the phone
  tier at orbit (200k) with the far trees (#136) before merging.
- The map's side quads grow with the taller faces: one quad per edge whatever the height.

### Sequencing with plan/21
- T2 changes `world-hex.ts`, `world-land.ts` and the Go twins in `internal/land` together, with the goldens re-recorded
  in the same PR. The physics session (plan/21) is told before it starts; it is not blocked by phase 2 (collision and
  motion), since cliffs use `stepOK` and the ship's `floorOf` as they are.

### Tests
- Every massif hex at its level, its summit pad flat, its faces refused by `stepOK`, every village still reachable on foot
  (the existing reachability test), the land outside the massifs unchanged (the golden diff limited to their hexes).
- A ship test: from over a summit it settles on the pad.
- Budgets: the triangle count at orbit on both tiers.

### Questions for the owner
1. **Which systems are the massifs?** One per platform group (control plane, storage, observability, networking) as
   proposed, one per platform service (many small peaks), or a single great mountain for the whole platform?
2. **Where?** In the frontier behind the Platform village (proposed), around the capital's edge, or spread across the
   frontier?
3. **Height by weight** (services' count now, load later), or fixed heights?
4. **Health on the face:** the face's colour by the worst health in the massif (owner only), or no health on mountains?

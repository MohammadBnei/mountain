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

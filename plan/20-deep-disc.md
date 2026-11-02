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

## T2 · The mountains (design, after one doubt cycle, 2026-11-02)

> The slab (T1) is merged (ukubi-game #137). T2 is built after plan/21 phase 3 (see Sequencing), as a Go-only land change
> plus the meshing. One doubt cycle (17 findings) is folded in below; the owner's answers decide the last open points.

### The land today (ukubi-game main 8c8a0a3)
- Levels: `TERRACES` 0, 2, 3, 5, 8, `STEP` = HEX/φ³ ≈ 2.34 units, `MAX_LEVEL` 8 (a level is `heightAt` / `LEVEL_H` 8,
  `heightAt` capped at `MAX_RELIEF` 170). A grass hex at 8 is always a `mountain` (a kit model, walk:false, `MOUNT_RISE`
  ≈ 8.9 over it); 35% of the hexes at 5 to 7 are hills (walk:false). So the walkable ground tops out at level 7.
- Mountains of today grow wherever the relief reaches 8: in the frontier (amp 92), and in the capital (amp 70, ridged)
  and the cities (amp 72).
- Roads between villages are straight lines with a hashed swing (`meander`), blind to height; a road hex keeps its
  level. The passes (`passLines`, a spanning tree of straight lines between every pad) are cut to at most level 7 and
  the climb rule (`lower`), so every village is reachable on foot.
- The frontier's high ground is a band from about 270 units to `RIM_FROM` (0.7R); past `RIM_LOW` (0.85R) nothing stands
  above a hill ("no wall at the Rim"). That band also holds six wild zones (plan/21 phase 5's mob zones), the frontier
  landmarks, three hamlets with their lots, the roads and passes, and plan/21's reserved build ring.
- Detail tiles over a drop stand on unstretched blocks, a STEP each, alternately shaded (the owner's minecraft-like
  edge). On the phone tier the map has no side quads, only a floor under it.
- No light casts shadows (world-scene.ts: none at orbit).
- The platform civ's real groups (the public `Building.Group`): `network` 6 (cilium, coredns, kube-proxy, kube-vip,
  metallb, nodelocaldns), `nodes` 5 (k8s-cp-01..03, k8s-worker-01..02), `storage` 3 (csi-nfs, local-path, longhorn),
  `edge` 2 (traefik, redirectors), `metrics` 1, `gpu` 1. Observability is the operations civ's; there is no "control
  plane" group (the control-plane VMs are in `nodes`).

### What T2 adds
- **Massifs from the real groups.** One massif per platform group (the owner's Q1 below may change the grouping),
  derived from the public town's `Building.Group` where `civ == platform`, so the land stays one land for the owner,
  friends and the realm (plan/21: built from the public town; the realm refuses a different hash). Groups of one
  service join the nearest (owner's call).
- **Height from the public count only.** A massif's summit level comes from its group's service count through the
  `LandSpec`: `MASSIF_LEVELS` 13, 21, 34 (≈ 30, 49, 80 units, plus the summit's rise). Load never enters the land (owner-
  only, and a land hash that changes with load would refuse clients); it may show later as a render-only effect.
- **A stage of its own.** A new `hexLand` stage, `massifs`, between `passes` and `features` (in Go only, after plan/21
  phase 3):
  - placement is a pure function of the `LandSpec`: candidate sites in the frontier band, each kept clear of every
    road and river chain, every pass hex and a ring round it, every lot, pad, landmark spot, wild zone and the build
    ring; a massif that does not fit is dropped and reported (a test over seeded lands lists them);
  - it sets levels by an explicit override from `MASSIF_LEVELS` (never by raising `MAX_LEVEL`, `TERRACES` or
    `MAX_RELIEF`: the rest of the disc keeps its levels, its treads and its mountains of today);
  - it runs after the treads, so no tread wears its faces down, and before `features`, which skips massif hexes.
- **New kinds.** `cliff` (a massif's faces: walk:false, no model, rise 0) and `summit` (its top: one flat hex,
  walk:true, a name board). The kit's mountain model stands on the summit's highest neighbour, never over the pad, with
  today's rise. The new kinds join `KINDS`, `TILE_SY`, the hash strings and the map colours in Go.
- **On foot and by ship.** The faces refuse both climbing and descent (`cliff` is walk:false), so the summits are the
  ship's (the owner's answer). Stepping off the ship on a summit is allowed: the rider walks its pad and takes the ship
  back (a one-hex island by design). Heights flow only through `StepOK`, `GroundOf` and the ship's floor, which
  `internal/physics` reads, so cliffs hold for the rider, mobs and the server with no physics change.
- **The look.**
  - Up close the faces keep unstretched blocks (the owner's look), counted honestly against `TRI_CAP` (up to 26 blocks
    a face hex; measured per massif chunk before merging).
  - The phone tier gets the massifs' sides (a small instanced side mesh for massif hexes only), so an 80-unit face
    never floats over the floor; counted against 200k triangles and 60k `TRI_CAP` low.
  - The map's brightness by level is clamped at `MAX_LEVEL`; the top terraces take a deliberate snow tint instead.
  - No shadows are promised; a shadow pass would be its own slice with its draws budgeted.
- **The meaning in the world.** A summit's card names its group and lists its services, as a village's card does.
  Health on the massif is render-only (never in `HexLand`, the `LandSpec` or the hash): owner Q4 decides whether it
  shows, and for visitors whether it follows the existing coarse states (healthy, degraded, maintenance), which
  buildings already show publicly.

### Budgets (measured in the build, before merge)
- Triangles per massif chunk (blocks, cliffs, the model, the side mesh) on both tiers at orbit and close; a chunk the low
  tier skips (`selectChunks`) must still read from orbit through the side mesh.
- Draws: the side mesh (one), the boards (one sprite each, or one batch), against 220.

### Tests (Go, after phase 3)
- Placement: no massif hex on or next to a road, river, pass, lot, pad, landmark, wild zone or the build ring, over
  seeded lands; dropped massifs reported.
- Levels: every massif hex at its override; hexes more than one ring from a massif identical (level, kind, feature,
  drop) before and after; the summit flat and walkable, the faces walk:false.
- Reachability: every village still reachable on foot (the existing test).
- The ship: from over a summit, descending (C held), it rests on the pad; the rider steps off and on.
- The pin and goldens are re-recorded wholesale (one hash for the land).

### Sequencing with plan/21
- Agreed with the physics session (2026-11-02): T2 is **built after plan/21 phase 3 merges**, which makes the Go land
  the only land; T2 is then a Go change in `internal/land` (the new stage and kinds) plus the meshing in
  `world-terrain.ts`. The physics session is told of the new kinds before the build.

### Questions for the owner
1. **Which massifs:** one per platform group (network, nodes, storage, edge; metrics and gpu joined to one of them), all
   six, or a single great mountain for the whole platform?
2. **Where in the frontier:** the band behind the Platform village (it may not hold four), spread round the frontier
   (proposed: more room), or round the capital's edge?
3. **Height:** by service count (network the highest) or one height for all?
4. **Health:** none on the mountains; the owner sees a colour by the worst health inside; or visitors too, in the
   coarse states buildings already show?

### Owner answers (2026-11-02)
1. **Two mountains, one enormous and one medium.** The enormous one is **the cluster** (the platform's `nodes`,
   `network` and `storage` groups: the ground everything runs on); the medium one is **the edge** (`edge`, `metrics`,
   `gpu`). Heights are fixed by size, not by count: the enormous at `MASSIF_LEVELS` 34 (about 80 units), the medium at
   21 (about 49).
2. **Where:** "where currently there is not much": the placement stage picks, among the frontier sites that clear every
   road, river, pass, lot, landmark, wild zone and the build ring, the two emptiest (fewest buildings, decor and ways
   within their footprint and one ring round it), the enormous first.
3. **A compass**, round, by the minimap (built now, in the page; no land change).
4. **"The mountain is alive, with eruption and all following cluster events like the previous mountain".** The old
   mountain (tag `mountain-final`, f1a53b2, plan/03-model-*) drew the cluster's life on itself: an eruption per Argo
   sync (an ash puff when it fails), a summit corona as strong as the traffic, a sag with steam and a red under-glow per
   alert (a flood past 30 minutes critical), scree per pod restart (a boulder per OOM or crash loop), magma dykes per
   build, a heat shimmer by CPU, a glacier fed by commits, strata with ash lines for incidents. The enormous mountain
   (the cluster) takes these back as **T2b, the living mountain**: render-only effects driven by the live frame's
   events, never in `HexLand`, the `LandSpec` or the land's hash. Which effects visitors see follows the public
   snapshot's rule (owner-only detail stays on the owner's pod; coarse states may show); T2b's own plan lists each
   effect with its source in today's live frame and its visitor rule, and gets its doubt cycle.

### Owner answer (2026-11-02): climbing paths
"make sure that both mountain have platform like climbing paths (3, one easy, one hard, one medium. easy is longer, hard
is faster)". This replaces "summits by ship only": each mountain gets **three paths to its summit**, platformer style,
and the ship still lands there.
- **Easy, the longest:** a switchback ledge path round the massif, every step within the walk's climb (`MAX_CLIMB`), no
  jump needed, a few short gaps a single jump clears at most; the slowest way up.
- **Medium:** fewer, steeper switchbacks; ledges a single jump (`JUMP_H`) apart, some gaps that ask a run-up.
- **Hard, the fastest:** nearly straight up a face: ledges a double jump (`DOUBLE_H`) apart, small landings, a slide or
  a roll to cross a gap; a fall drops to a lower ledge, never to the foot (catch ledges), so it is hard, not cruel.
- **How they are built:**
  - the ledges are the land's own: hexes of the massif stage at chosen levels (a path is a chain of hexes, its levels
    stepping by what its grade allows), so they flow through `StepOK`, `GroundOf` and the ship's floor and
    `internal/physics` needs no change; where a hex is too coarse for a platform, a fun-style solid (a crate, a ledge
    stone, world-fun.ts's run pieces: a circle or box of a height) stands on it, as the playgrounds' runs already do;
  - each path starts at a trailhead on the massif's foot by a road (a signpost: easy, medium, hard, with the summit's
    name), and the three meet only at the summit;
  - every gap and step is checked by the jump simulation (as world-fun.test.ts climbs the playground run): the easy path
    walkable end to end, the medium within a single jump, the hard within a double jump; a test per path per massif;
  - the paths' lengths: the easy at least twice the hard's (measured along the path), and a timed walk in the test
    (easy slowest, hard fastest at the rider's run speed).
- **The cliffs stay cliffs** everywhere off the paths (walk:false faces), so the paths are the only ways up on foot.
- **Budgets:** the ledges are land hexes and a few dozen solids a mountain: counted with the massif's chunk.
- **Questions this adds (for the build's plan, not blocking):** whether reaching a summit on foot unlocks something (a
  secret, a badge on the settler's card), and whether the hard path is timed (a best time on the summit board).

## T2b · The living mountain (after one doubt cycle, 2026-11-02)

> The owner: "the mountain is alive, with eruption and all following cluster events like the previous mountain".
> Render-only: nothing here enters `HexLand`, the `LandSpec` or the land's hash, and nothing moves a walkable surface
> (a sag or a crack is a colour, a decal or a plume, never displaced ground the rider, the ship or a climbing path stands
> on). Built after T2. One doubt cycle (16 findings) folded in: the first draft's eruption source and visitor rule were
> wrong, and the owner's first approval was given on them; the questions below are asked again.

### What the data carries (checked against the code)
- **Owner live frame** (`/api/town/live`, polled every 5 s): `looks` (each drawn building's health, `fire` = a critical
  seen alert, `raiders` = seen alerts, `unseen` = alerts its building does not watch), `banners` (civ-level alerts with no
  node: not attributable to one mountain), `centre` (alerts on undrawn nodes), `health` (hosts, VMs and LXC containers),
  `land.hosts` (CPU and memory, use may exceed capacity), `weather` (a CPU bucket), `moons`.
- **Not in the live frame:** `deploys` live only in `/api/cockpit`, read when the Advisors tab opens (no poll). Sync is a
  level (`argocd_app_info` sync_status), not an event: a resync of a Synced app shows nothing, a failed sync often leaves
  health Healthy. Atlas does not query `argocd_app_sync_total` (the old mountain's eruption source). Atlas already sends
  per-app `RPS` (`contract.AppLive`) and the day's critical alerts and degraded apps (`contract.Live.Day`).
- **Public frame** (the visitor page): `live.states{}` (coarse: healthy, degraded, maintenance; Down maps to degraded),
  `weather`, `moons`, `residents`, `keeper`, `at`, `public`. No deploy, no alert, no host. The public pod pulls every 15 s.

### Every model of the old mountain (f1a53b2), with its source here
| Model | What it showed | Source here | When | Visitors |
|---|---|---|---|---|
| eruption | lava per Argo sync, ash per failed one | needs `argocd_app_sync_total{app,phase}` in atlas's contract (a counter: no sync is missed between polls), forwarded as per-app sync counts in the owner live frame | after an atlas change | owner's choice (Q2) |
| cauldron (sag, steam, red glow) | alerts by severity and age | the cluster group's buildings' `raiders` + `unseen`, and `centre` alerts on cluster hosts and VMs (needs `Alert.startsAt` typed); a colour and steam, no displaced ground | now | steam when a cluster building's coarse state is degraded (already public) |
| bedrock cracks (mountain) | a host or VM not ready | `health`, filtered to the cluster's hosts and VMs (no LXC), one face per host; read from the same forced-merged map the underside uses | now | none (host health is owner only) |
| shimmer | CPU heat | `land.hosts` CPU over capacity, clamped to 0..1 (none when capacity is 0) | now | the public `weather` bucket (already public) |
| summit crystals | one crystal per app, glow by health | `looks` health of the cluster's buildings | now | coarse states (already public) |
| lightning corona | traffic (rps) | `contract.AppLive.RPS`, already in the game's frame: forward its sum | now (a game change) | none (rps is owner only) |
| magma dykes | a build in flight | needs builds in atlas (CI runs) | later | none |
| aquifers | one pool per storage pool | needs storage pools in atlas | later | none |
| scree | pod restarts, OOMs | needs restarts in atlas | later | none |
| glacier | snowfall from commits | needs commits in the live frame (atlas reads GitHub) | later | snow only |
| strata ash lines | incidents | `Day.Critical` now (the day's critical alerts), a history later | partly now | bands, no titles |
| river | public traffic bytes | needs bytes in atlas | later | none |
- **Fire** is not separate: `looks.fire` is a critical seen alert, the cauldron's steam; one effect, not two.

### Shape
- One module, `world-mountain-life.ts`, reading each owner live frame (no new poller); the visitor page reads only its
  public frame. Diffs seed silently on the first frame, ignore transitions from "" or Unknown, freeze while not
  connected, and cap how many effects play at once.
- Effects drawn from instanced or `Points` pools with a stated draw count each (not the barrels' 5 meshes a blast),
  measured at orbit and close on both tiers before merge.
- `Building.group` typed in TypeScript (the JSON already carries it); the massif's summit and face hexes exposed by T2.

### Questions for the owner (asked again)
1. **Eruption needs an atlas change** (the Argo sync counter in the contract, forwarded in the live frame). Do it as part
   of T2b, or start without eruptions?
2. **A visitor eruption puff would tell visitors when and how often you deploy** (never shown to them before). Keep it for
   visitors (one new public field, a sync counter, no app named), or eruptions for you only?
3. **The models the first draft left out:** summit crystals (app health) and the corona (traffic) can come now; magma
   (builds), aquifers (storage pools), scree, glacier and river need atlas fields later. Which do you want on the
   mountain?

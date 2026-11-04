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

### T2b owner answers, second round (2026-11-02)
1. **Eruptions: add the Argo sync counter to atlas** (mountain's contract and atlas, the sim mirror with it), forwarded
   per app in the owner live frame; ash on a failed sync.
2. **Visitors see the eruption puffs too:** "open it for visitors too, they don't know what it is (part of the
   mystery)". One new public field, a monotonic sync counter (no app named), pinned in `public-fields.txt`.
3. **Effects wanted:** the corona (now, owner only), magma and aquifers, scree, glacier and river (each when its atlas
   field exists). Summit crystals were not chosen: left out. So the mountain carries, now: eruption (with the atlas
   change), the cauldron's steam and glow, the cracks, the shimmer, the corona; later: magma, aquifers, scree, glacier,
   river, the strata's history.

## T3 · The pits (after one doubt cycle, 2026-11-03)

> Plan/20 §3a: pits and canyons, heightfield carves open to the sky, under the datum, walkable by the climb rule, no
> overhang. The owner (§4): "pits and caves are the data". Built in Go (internal/land), the WASM rebuilt, like T2.
> One doubt cycle (16 findings) folded in: the first draft's geometry did not add up, the phone's floor disc hid the
> pits, the camera called a pit "under the disc", and the exits could end on a hill.

### Owner answers (2026-11-03)
1. **Both cavities:** the vault (a round quarried pit) and the gorge (a canyon).
2. **Depth:** the vault's floor about 30 units down (13 levels), the gorge's about 19 (8 levels).
3. **Bridges over the gorge:** roads keep their routes and cross on bridges.
4. **Meaning:** the vault for the databases (Postgres, Redis), the gorge for the stores (S3, NFS, backups); a name and
   a board for now, their life a later slice.

### What T3 adds
- **Fixed roles** (as the massifs): the spec carries no per-service data, the LandSpec is unchanged. `HexLand.Pits`
  (name, centre, rings, floor level, the ramps' hexes) on the wire beside `Massifs`, with `PitAt(x, z)` like
  `MassifAt`; `Dropped` names a cavity no site held.
- **A `pits` stage** after `massifs`, before `features`. Each cavity's site: blocked as a massif's is (`massifBlocked`:
  pads, plots, landmarks, wild zones, passes and their ring, fun props, quiet circles, the build ring) plus the massifs'
  footprints and their ring, never water or a hex next to water (no sea, lake or river pours in); the emptiest such
  site in the frontier band, apart from the massifs; kept only when `reachOf` from a road reaches its exits. The
  stage is skipped under `Options.NoMassifs` (the trace loaders' bare land).
- **Depth is relative:** a floor is its rim's lowest level minus the cavity's depth (13 or 8), so every wall is at
  least that deep and the rim's own relief is kept.
- **The vault:** radius 5. Its floor is the inner 2 rings; rings 3 to 5 are walls (a cliff kind, walk false, as the
  massifs' faces), but for one spiral ramp: a chain of 13 hexes of the rings, one level a hex, with two step solids a
  rise (the easy path's `massif/step`, so a rider taps up). Its top hex opens onto the rim and is marked a pass, so
  `features` never turns it into a hill.
- **The gorge:** two hexes wide, long enough for its floor and a ramp of 8 hexes at each end (at least 20 hexes; it is
  dropped where no straight-enough run fits). Its walls are the rim's own faces. Each ramp's top is a pass.
- **Bridges:** where a road crosses the gorge, the road's hexes over it stay at the rim's level as `bridge` hexes: a
  deck model on them, no blocks under the deck, the gorge drawn on through. The land is one height a point, so the
  span is a pier a rider in the gorge cannot walk under (T4's second surface is what lets one pass under later); each
  stretch of the gorge between bridges keeps a ramp out, or is not carved (TestClimbs checks every stretch).
- **Negative levels:** `Hex.Level` goes under 0 for the first time.
  - The phone's floor disc (world-terrain.ts, an up-facing disc at WATER_Y − FLOOR_SINK) is holed at each cavity, and
    each cavity gets a cap under its floor; on the phone tier, sides are drawn for every edge where either hex is a
    cavity's (the tallest faces belong to the rim hexes, not the cavity's).
  - `landFloor` stays the relief clamp; the pick ray gets its own floor, the deepest cavity's (Go and TS together).
  - The map colour clamps the level at 0 and tints depth (a negative level must not overshoot `lerp`).
  - `underDisc()` (world.ts: `camera.y < 0` today) becomes "under the belly or in the Underside view", so a camera
    in a pit keeps the buildings, cards and the orbit's chrome.
  - The realm's height check allows a fall into a cavity (a 30-unit drop is not a teleport to other players).
  - The y = 0 planes (the residents' close-up target, the tap fallback) use the ground under the camera instead.
  - The realm's lairs refuse a cavity and its rim (`PitAt` in place.go's `lairAt`), so no mob spawns where it cannot
    get out.
- **Kinds (Go and TS together):** `quarry` (floors and ramps: dark stone, walk true) and the walls on the existing
  `cliff` kind; `bridge` (walk true). Each in `KINDS`, `TILE_SY`, `HexKind` and the map colours.
- **The camera in the gorge:** the gorge is 2 hexes (about 20 units) wide; the chase camera's arm shortens against its
  walls and rises over them before it folds behind the rider (checked hands-on, both tiers).
- **Boards:** a name board at each cavity's rim ("The vault: the databases", "The gorge: the stores"), as a summit's.

### Budgets (measured in the build, before merge)
Desktop and phone at orbit and in each cavity: triangles and draws against T2's (desktop orbit 245k / 69 draws, phone
122k); the rim ring's sides counted in.

### Tests (Go)
Placement keeps every exclusion and the reachability of every village; levels unchanged past each cavity's rim ring;
TestClimbs out of the vault and out of every stretch of the gorge (physics.Step, 3-frame taps) onto a hex `reachOf`
reaches; the ship lands on each floor; a bridge deck is walked across at the road's level; goldens, pins and the WASM
re-recorded.

### Sequencing with plan/21
Land and physics changes are ukubi-2a's area: ping before touching internal/physics or the realm's height check;
whoever merges second rebases and rebuilds the WASM.

### Later, not T3
Cave mouths at the vault's floor (T5), tunnel mouths in the gorge's walls and walking under a bridge (T4), the
cavities' life (a glow per database).

## T4 · Tunnels as connections (after two doubt cycles and the owner's answers, 2026-11-04)

> Plan/20 owner answer 4: "I don't like the traveling packages. Let's build the connection through apps and things
> through tunnels or something more coherent with the world". Built after T3 (ukubi-game d37983b).

### Today (ukubi-game main d37983b)
- The connections are atlas's `lenses.supply`: one road per needed service, `{from, to, via, checked?}`, data
  (postgres, s3) or auth (oidc, forwardauth, infisical). The dev world has 35, most of them ending at a few hubs
  (authentik, infisical, the Postgres proxies). The public page has the same roads, `via` reduced to data|auth, no
  `checked`. The realm already receives them (realm/land.go WorldOfTown passes `Lenses.Supply` to BuildWith).
- They show twice: world-roads.ts draws straight overlay lines read from the orbit; world-carts.ts runs the "packages"
  (carts on the road hexes, boats on the rivers). world-people.ts imports `routeOf`, `wayXZ`, `alongInto`, `fnv` from
  world-carts.ts, so the module stays.
- No live signal is per connection: the frame carries each building's health (`looks`), not an edge's traffic.
- The land is one heightfield. Everything that collides, aims or senses reads it in 2D or at the surface:
  `Ground.OK` and its building footprints (physics/motion.go), step.go's NearAt, the realm's `solids.NearAt`,
  `heightAt`'s swim clamp, `ResolveFall`, the anchor after 0.5 s of silence (realm.go), mobs' aggro and chase
  (mobs/mob.go), build reach (build.go), item drops (resources.go), Burst's Overlap, world-shot.ts's rays, the chase
  camera's floor (world-chase.ts). A shared second layer would touch every one of them (the doubt cycle's main finding).

### Owner answers (2026-11-03)
1. **Underground: private**, like a building's inside. The page sends no pose while its rider is under
   (world-realm.ts:27 already does so inside a building); no other rider sees it there, no mob senses it, nothing is
   built, shot or dropped there. The realm's one new rule: a rider that went silent at a portal comes back only at a
   portal (the anchor after 0.5 s of silence otherwise accepts any spot).
2. **Ways down: open ramps carved into the land**, not stair-heads.
3. **The packages: carts and boats both retired.**
4. **Visitors walk the tunnels too**, with the visitor's coarse health states.

### What the answers change
A ramp is real heightfield, as the pits' ramps are: hexes a level lower each, with the step solids at a third and two
thirds of each rise (pit.go), walked and checked like any surface. Its length is its depth in hexes, so the network is
not at level −17 (a 17-hex ramp per hall): it lies **just under the deepest bed**, and goes round the pits instead of
under them.

### What T4 adds
- **The network.** Every building at one end of a supply road gets a **hall** under it; every road is a **bore** to
  the other end's hall, one per unordered pair and kind (from the supply roads, before landOf's per-pair dedupe). The
  floor of all of it at **level −5** (about 11.7 units under the datum), its ceiling (6 units up) under `landFloor`
  (the deepest bed, about −5.2), so no bore opens into the sea, a lake or a river. A bore follows a hex path between its
  halls (breadth first, never a pit's hex nor its rim ring), so it never opens into the vault or the gorge. The supply
  graph may have several components; each is its own network.
- **Sized for the chase camera and the bike.** Bores 7 wide and 6 high (the bike's turn radius is 3); a hall's radius
  grows with its bores (6, plus 1 per bore past three), so the hubs are wide halls, never a knot of crossing tubes.
- **Ramps down.** Each hall gets one ramp, placed in a `tunnels` stage after `pits`: from a free surface hex at level 0
  or 1 near its building's pad (off the door side, the lanes, the props, the plots; the props' `clearAt` reservation
  in landOf extended to it), down a hex a level to level −5 (five or six hexes), the cut's sides cliffs as the gorge's
  are. Its last hex's far edge is the **portal**: an arch drawn over that cliff face, the side quads behind it left
  out, the bore starting there at the same floor height, so the ramp's floor runs on into the bore. A hall with no room
  for a ramp is recorded in `HexLand.Dropped`; its bores are still walkable from the other halls.
- **The floor plan.** The same stage computes it: the union of halls (discs), bores (capsules along their hex paths)
  and each ramp's portal, flat at level −5, with a ceiling per point. Drawn from that same 2D union (walls extruded from
  its outline, a floor and a ceiling), so a wall drawn is a wall felt, and no tube cuts another's inside. Ramps,
  portals and the floor plan are on the wire (wire.go, as Pits) and in `HexHash`: a change of the supply roads reloads
  the pages, as any land change does.
- **Physics under.** Its own `physics.Land` (as `ShipLand` is): the floor plan's floor, its outline as the wall, its
  ceiling capping jumps (airLand); no surface walls, solids or water. A rider on a ramp's last hex walking through the
  portal goes under (no fade: the floors meet); walking back out of a portal returns it to the surface physics on that
  ramp's last hex. The ship is set down at a ramp's top; it does not go down.
- **Camera under.** The chase keeps its arm inside the floor plan (shortened at a wall, under the ceiling), so the
  view stays third-person. "Under" is the rider's layer, not the camera's y.
- **The bores lit by their ends' health.** Emissive and unlit (no scene light reaches them; day and night alike): a
  strip along each bore's floor in its kind's colour (data orange, auth blue, the overlay's), steady when both ends are
  healthy, amber when either is degraded, red when either is down. On the public page the visitor's coarse states
  (world-public.ts: no down, so never red). Owner page only: a declared, not checked road runs dimmer; a missing
  `checked` counts as proven, as world-roads.ts does. No pulses: nothing per connection would drive them. Each hall
  carries its building's name.
- **Drawn only under.** The network is one group, shown only while the rider is under; the far surface groups are
  hidden then. The ramps are land (their hexes in the land's chunks); the arches one instanced draw.
- **The packages retired.** buildCarts and its frame go; world-carts.ts keeps the ways the residents walk. Gates:
  dev/calm-gate.mjs's carts (day against night) and boats-on-rivers steps go, with `__world.calm.carts` and `cartsAt`;
  the README's carts lines too. The overlay stays.
- **Residents and mobs** never route down a ramp (their walk grids refuse ramp hexes, as they refuse cliffs).

### Coordination
- Agreed with ukubi-2a (2026-11-03): this slice builds all of it; ukubi-2a reviews the draft PR before the WASM merge.
  Their conditions: no building under and nothing hit across layers hold because nothing is placed, shot or dropped
  under and the realm drops silent riders (below); no wall on a ramp is a new refusal in build.go (below).

### The second doubt cycle's changes (2026-11-04; these override the lines above where they differ)
- **Silence, one rule for all.** Today a rider silent over 0.5 s (or reconnecting) may come back anywhere
  (realm.go:451, the anchor branch). ResolveFall's anchor branch now accepts a pose only (a) within top speed × gap of
  the last pose, (b) at a portal of the same network as the portal the rider left by, with the gap at least the
  tunnel path's length over the bike's under speed, or (c) at a door or the spawn. A reconnect is keyed on the
  player's last pose, not on `p.pose == nil`.
- **Silent riders leave the world.** fight.sync and snapshot leave out a player silent over maxDt: no ghost at a
  ramp's foot (nor at a building's door, today's same bug), no mob target, never downed while away. Tested.
- **Ramp sites from the spec.** landOf picks each hall's ramp from spec data (pads are flat at level 0, the paths,
  rivers and lakes), before `funOf`/`clearAt` place the props; plotsOf skips ramp hexes; a test checks every ramp hex
  keeps its step solids (decor `massif/step`, collided by the WASM and the realm alike). Ramps never sit on a road or
  ford hex.
- **The portal opens the cliff.** The ramp's last hex (level −5) borders a higher hex whose column (stackOf) would
  fill the bore's mouth: stackOf stops that hex's column at the bore's ceiling (−5.67) above the portal edge, and the
  arch frames the opening. The portal links to its hall by an explicit capsule in the floor plan.
- **Phone tier.** Ramp hexes count as pit hexes (pitHexes, pitOutlines): a hole in the map floor and a dark cap,
  counted in the budget.
- **Halls and bores keep clear** of every ramp hex and its ring (except their own portal) and of the pits' near ring;
  a hall's radius is capped where it would reach one. The ceiling is then −5.67 everywhere.
- **Physics under is `World.Under *Floor`**, beside `Room` (not a `physics.Land`: Step reads `Ground.Hex`
  directly): its own ground, bound and outline test, both rides allowed, and a new ceiling clamp in stepAir (`Peak`
  raises a floor, it caps nothing). Go and the TS mirror, with a parity test.
- **Bores are 9 wide, string-pulled** (straight runs between hex centres that see each other clear of obstacles), and
  the bike under is capped at 7.5 u/s (its turn radius 3 there). The chase arm and its pitch are kept under the ceiling.
- **The chase and the rays switch with the layer:** the injected `ground` and `landAt` read the floor plan under
  (world-chase's floor and sight, world-shot's rays); a chase test under a level-0 hex.
- **Building.** build.go refuses Place where RampAt or PitAt; a rebuild that carves a ramp under existing pieces
  removes them and returns them to their owners.
- **Mobs** refuse ramp hexes in every direction (walk.go, beside `ontoCliff`). Residents walk the road hexes only, and
  ramps never sit on them.
- **Hashing.** HexHash (Go) and hexHash (world-hex.ts) gain the ramps, portals and floor plan explicitly; a test where
  only the floor plan changes and the hash does too.
- **Health colours.** Public maintenance (the default for unmeasured) is a dim neutral strip.
- **Who gets a hall.** Roads whose both ends have pads, keyed by the sorted pair and roadKind; a test that the owner's
  and the public inputs give the same floor plan.
- **Edge cases.** A page reloaded under starts at the spawn (allowed by the silence rule). The ship never enters a
  portal; its rider dismounts as anywhere. While under, every surface group is hidden, not only the far ones.

### Budgets
Measured before merge on both tiers, at orbit (the ramps' cuts, the arches) and under (a hub's hall and a long bore),
added to dev/world-gate.mjs as T2 and T3 did.

### Tests
Go: the floor plan (halls, bores round pits, ceilings under landFloor, components, portals), ramps (a level a hex, the
step solids, a climb out with physics.Step, dropped halls), determinism; the portal transitions; the ceiling cap; the
realm's portal-only return. Client: the floor plan from the WASM matches Go's; the health colours (owner and public).
Goldens re-recorded.

### Not T4, said plainly
- Walking under a bridge and mouths in the gorge's walls (T3 named both for T4): the bores go round the pits and lie
  under the beds, so neither comes with this shape. Later, with bores at varied depths.
- A shared underground (other riders seen there). Later, if the private one is liked.
- Per-connection traffic (needs a source: Prometheus or the mesh). The halls as T5 interior worlds.

## T5 · Interior worlds (draft with the owner's answers, before its doubt cycle, 2026-11-04)

> Plan/20 owner answer 3: "for buildings too, let's build rich interior worlds so we can use the same third person
> view". Revised slice: one interior kind for buildings and caves, in the chase camera; rooms from kit pieces; a
> building's real parts inside; cave mouths as doors. Built after T4 (ukubi-game 74fd138).

### Today (ukubi-game main 74fd138)
- **What's inside today:** a workshop's inside (world-interior.ts, plan/17 D3). GET /api/town/rooms?id= returns its
  repo's seams (entry, domain, adapters, config) as 8×8 rooms in a row, with bookcases whose books are files. It is
  drawn as its own scene: three draws, at most 40 draws and 60k triangles.
- **Limits today:** first person only, and owner only (/api/town/rooms is an owner route). A building with no repo
  atlas holds gets one empty room. Physics uses `Room`: one box, no colliders.
- **The building card:** it already knows a building's real parts: `parts` (id, kind, health, why, role, lag,
  conns), `needs` and `neededBy` (its supply, checked or declared), signals, attacks and risk, decisions and tech.
  The visitor's card is a coarse cut of it.
- **T4 halls:** every connected building has a hall under it (its tunnels' meeting point) at level −5.
- **The realm:** it sees a rider inside as a declared door leave (T4), so the inside stays private, and nothing
  there needs the realm.

### What T5 adds
- **One interior kind, a floor plan.** An interior is a set of rooms (rectangles joined by door gaps), walked with T4's
  floor-plan physics: `World.Under` generalised to `World.Floor` (ground, outline wall, ceiling). Room and its box go,
  so caves and buildings share one kind. Rooms are built from kit pieces (walls, floors, doorways, props) as
  instanced draws, unlit like the tunnels, with the same fog to dark.
- **Third person inside.** The chase camera runs inside as it does under: its arm is kept in the plan and under the
  ceiling. V still flips to first person. Rooms are 12 by 12 and 5 high, so the arm has room.
- **A building's rooms are its real parts:**
  - **the hall** (entry): the building's name, its health-lit sign, and its signals and alerts as glowing boards;
  - **the parts hall:** one machine or figure per part (pod, VM, replica), lit by its health, with a plaque giving its
    role, lag and conns; E on one opens its Inspect as the card does;
  - **the library:** today's code rooms (seams, bookcases), where atlas holds the repo;
  - **the stair down** to its T4 hall, if it has one: the interior and the tunnels join, so a rider can go building
    → hall → bore → another building's hall → up into that building, never touching the surface (owner only).
- **Caves.** A cave mouth in a pit's wall (the vault's lowest ring: the databases) is a door into a cave interior: the
  same floor-plan kind, its rooms rock (kit rock pieces), one cave per data building (`db:` ids), its parts hall the
  database's replicas (Patroni roles, lag). The vault becomes the place where you walk into a database.
- **Data, owner and visitor.**
  - Owner: GET /api/town/rooms grows the card's parts, signals and supply (atlas already gives them to the card).
  - Visitor: none (owner answer 3): /api/town/rooms stays an owner route; the public page draws every stair up shut.
- **Budgets:** at most 60 draws and 90k triangles inside on both tiers (the surface and tunnels are hidden, as under).

### Later, not T5
- Live per-part metrics on the machines (CPU, memory gauges), and units at work walking the rooms (the residents
  inside).
- Interiors for the capital, the control room (it keeps its own room today) and the carriers.

### Owner answers (2026-11-04)
1. **Rooms:** all four: the entry hall, the parts hall, the code library, the stair down to the tunnels.
2. **Caves:** in the vault (one per database, at its lowest ring) **and in the mountains' faces** (the platform's
   services: a cave per platform building of the massifs' groups, its mouth on a terrace of its massif).
3. **Visitors: owner only**, as today. Visitors walk the tunnels (T4) but no stair up leads them into a building: on
   the public page a hall's stair is closed (drawn shut, no door).
4. **Camera: third person** inside, V flips to first person.

### The doubt cycle's changes (2026-11-04; these override the lines above where they differ)
- **Leaves across the stair.** The join of interiors and tunnels needs one realm rule: a DOOR leave (from the
  building's door) or an UNDER leave may come back at any portal of the network of that building's hall, or at the door
  of any building whose hall is on that network, once `tunnelled`'s path-length time allows (hall to hall; the same
  building's door at once). The client: arriving in a building by its stair sets `back` (the way out by its door) and
  the layer, and `go()`'s under guard lets the stair through. Tests for every pair: door to door, door to portal,
  portal to door, and a refusal across two networks. A building with no hall has no stair.
- **The stair is client-side**, no land change: a door at a fixed point in each hall (its centre, offset clear of the
  bore mouths by a hash of the id), entered by E, gated by `p.doors` (owner). On the public page it is drawn shut and
  does nothing.
- **Caves are second doors of their building**, never second interiors. A db building's cave mouth is in the vault, a
  platform building's in its massif. Inside is the same interior as through its surface door. The realm's
  `hasPad`/`atDoor` accept either door of an id. The client's door map holds a list per id, each with its facing.
- **Cave mouth sites** are a pure function of the public pads, in sorted id order:
  - **Vault:** on the floor's ring-2 hexes, at most 12.
  - **Massifs:** on walkable ledge and trailhead hexes of the building's group's massif, at most 9 a massif.
  - **Overflow:** an overflow building, or one whose massif or vault is Dropped, has no cave (recorded in `Dropped`).
  - **Entry:** mouths are entered by E only, never by walking near, so crossing the vault floor never drops a rider in.
  - **Public page:** mouths are drawn shut.
  - **Test:** a reach test from a path climbs to every massif mouth.
- **Physics keeps `physics.Room`**, extended to a list of boxes joined by door gaps (the union the bound, walls where
  the union ends), plus a ceiling. The exit and `Spent` stay as they are; the WASM room slot grows to carry the boxes.
  No land, hash or realm change for interiors.
- **The control room** moves to the new kind as a one-room plan, keeping its panel hooks and tests.
- **Chase camera:** reachOf scans from the pivot outward and stops at the first point outside, so it never looks
  through a wall into the next room. Walls are thicker than the arm's near clearance. A chase test between two rooms.
- **The library** keeps its shelves, re-laid in 12×12 rooms with ceilings. It shows only when the building's code
  repo is its own (`CodeRepo != GitopsRepo`), never the whole gitops tree.
- **E on a machine** opens the card drawer at that part's row (no part inspector exists on the disc yet).
- **No route changes:** the interior reads the card (`/api/town/card`, owner) beside the rooms. Each room degrades
  on its own: no repo means no library, a card failure means the hall and parts hall say "unknown".
- **Budgets:**
  - Caps: at most 24 machines a parts hall and 6 boards in the entry hall, each with a "+N more".
  - One draw per kit piece type.
  - A gate step inside the most parts-heavy building on the phone tier, at 60 draws and 90k triangles at most.
- **Clicks inside:** in third person a click never shoots (picks a book or a machine, as today); labels hide by their
  distance from the rider, not the camera.
- **Today, corrected:** halls exist only for buildings at one end of a supply road whose both ends have pads.
- **Kit pieces:** the builder lists the model ids it uses from the kit manifest; a piece the kit lacks is made as
  simple merged geometry.

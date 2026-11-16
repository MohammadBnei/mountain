# Play review: the first fix wave (2026-11-16)

The owner's notes and three QA players (owner on desktop, a visitor on a phone, a signed-out guest) found 40+
problems on the live world. They group into nine causes. This wave fixes the groups that land on the plan/25
foundation. The pick path, the camera rig and `world.ts` as a composition root are the second part, designed with the
owner before they are built.

## Owner decisions (interview, 2026-11-16)

| # | Decision |
|---|---|
| PR-1 | Desktop mouse: context click. Ground walks there (the phone's pathfinder), a mob engages, a thing selects; WASD moves and cancels the walk; right-drag turns. *Part 2 (pick).* |
| PR-2 | Phone on foot: no jump button. A tap onto a ledge, roof or crate routes and jumps onto it; out of reach says so. |
| PR-3 | A tap on a plain thing goes onto it: roof, crate or wall top when reachable, else its foot; never the ground behind. Nav side in this wave, pick side in part 2. |
| PR-4 | Camera: close by context (foot about 5-6 m, indoors about 3 m clamped in the room, bike wider, ship wider), walls fade instead of clipping, two-finger pinch zoom. *Part 2 (rig).* |
| PR-5 | Death: downed, respawn at the nearest village with full health; the cost is the carried item, dropped where the rider fell. |
| PR-6 | Spawn: a reload puts the rider back where it was, on foot; inside a building, outside its door; a first visit at the spawn stone. Never on the ship unless it was flying. |
| PR-7 | Uneven land: smooth it. Outside massifs no step is higher than an auto-jump; massifs keep marked trails. |
| PR-8 | Guests: a very small tutorial with visual taps (move, Go to a thing, talk, map and waypoint, a fight), and a better welcome card that says what a guest can do and what signing in adds; Esc closes it. Cards around the world explain some aspects. |
| PR-9 | Go: a visitor can point at a destination or a thing and go there. Buildings stay closed to visitors, with words: "Closed to visitors · inside soon", Look still offered. |
| PR-10 | Disclosure, two tiers. Hidden from non-admins: secrets and credentials, addresses and hosts (IPs, node names, `*.svc`, ports, network layout), exact versions and image tags. Shown to everyone: real app and service names, kinds, health, what they do, how they connect, the architecture (replicas, witnesses); people and group names too. The fictional names stay as in-world flavour beside the real ones. Retires "the public face is a fictional town". |
| PR-11 | Hover re-picks only when the pointer moves. *Part 2 (pick).* |
| PR-12 | Sky: a day-night cycle on the world clock, stars, constellations and the Milky Way; the land's light follows it. |
| PR-13 | Idle: after 20 s with no input, about 20 fps and slower polls; a hidden tab renders nothing and pauses polls; anything moving near the rider wakes it. |
| PR-14 | Everything through a workflow; one PR per lane; the owner looks at the visible ones. |

## Wave 1 lanes (amended after the doubt round, same day)

The doubt round found that the downed state, the respawn, the massif trails (`TestClimbs`), the day-night sky
(`world-day.ts`) and the visitor's Go already exist: their bugs are what this wave fixes. Every lane edits `world.ts` and
the committed `web/dist`, so lanes build in parallel but **merge in series**, each rebased with the dist rebuilt, in
this order. New journeys go to the slow path (T2-a: the per-PR three stay under 60 s).

1. **Quick fixes.** The loader's ring fed from `world.ts`'s `loader.set` again (no contract change). Idle: the draw gate
   at `world.ts`'s `time.shown()` and the live poll in `ui-live.ts` (honour `document.hidden`; slow after 20 s idle;
   the chatter interval too). The yellow screen: `world-targets.ts`'s BackSide hull covers the screen when the camera is
   inside a faded building. The QA hooks the journeys need: `__world.hits` crashing on a `Points` with no position
   (and the game's own raycasts if they can hit it), `view().flying` stale, `__world.place` rubber-banding in QA.
2. **Disclosure, gate and doors (Go side).** PR-10 without moving the world: ids stay HMAC tokens and
   `UKUBI_GAME_PUBLIC_KEY` stays required (the face lays the land out by token); the real app and service names, kinds,
   health and architecture are added as display fields (`town/public.go` Names and cards, health in `PublicLive`), each
   field in `public-fields.txt`; cluster nodes (the turtles) and anything host-like stay hidden; a test per hidden kind.
   The gate: a `--world-url` flag, and a non-admin's refusal links to it. Doors: "Closed to visitors · inside soon" on a
   building's door, and Go aims at the door, not the nearest wall.
3. **Session and death (debug).** Why the downed overlay, the countdown and the full-health respawn did not show live
   (QA desktop #7); respawn at the nearest village (the realm's towns into `sim.Land`); a downed carrier drops its item
   where it fell (a disconnect still stows). Start where you left: the page keeps its pose and door per space version
   and every page starts on foot (no proto change; the realm's `reconnects` already accepts the last pose).
4. **Guests and HUD words.** The welcome card says what a guest can do and what signing in adds, Esc closes it; a Sign
   in chip; "Sign in to …" for a guest instead of "World offline"/"closed" (`ui-people.tsx`, `ui-menu.tsx`); the first
   hint shown before it is marked seen; "Your homelab" only on the owner page; the phone's Settings in touch words; the
   Talk panel's hidden first line, its input autofocus and the Close over it; the Settings tabs' jump and the dead Close
   under it; Esc's double press; "E … (E)". The tutorial: its move, talk and map steps now; its "Go to a thing" and
   fight steps wait for the pick module (part 2).
5. **Sky (debug, then add).** Why the D6 sky renders flat black while the land is lit; the "Material: color undefined"
   warnings traced (the fight's looks are suspects); then constellations and the Milky Way in `world-day.ts`.
6. **Routing on uneven land (root cause).** A route test replays QA visitor #2 (terraces near (-98, 43)) and desktop #8
   (the easy trailhead) on the production snapshot; fix the router or the auto-hop where it fails. The land moves only
   if that test proves a rise above `RideReach2` outside the massifs. Also the resident that shoves the rider and the
   bike trapped in a walled lot.

Moved to part 2: PR-3's roofs, crates and wall tops (they need a standable-top physics rule in Go, TS, WASM and the
realm, and the pick module to aim at them).

Each lane: build, local checks, its tests, PR, a fresh review, one fix agent, my look at the visible ones, merge in
the order above on green CI and a clean review.

## Part 2, to design with the owner

One pick module for tap, click and hover (6 raycasters today), one camera rig with context profiles, and `world.ts`
(3600 lines) as a composition root. PR-1, PR-3 (with a standable-top rule for roofs, crates and wall tops), PR-4,
PR-11 and the tutorial's Go and fight steps wait for it.

## Behaviour changes, named

The public face shows real names beside the in-world ones (lane 2). A reload no longer always starts at the spawn
stone, and the owner's page starts on foot (lane 3). Death costs the carried item (lane 3). The land moves only if lane
6's test proves it must.

## Part 2 design decisions (owner interview, 2026-11-16, with the fleet session's input)

| # | Decision |
|---|---|
| P2-1 | **One pick module** replaces the 6 raycasters: one ray, the nearest solid wins. Grass, flowers and small decor never count as solid; living things (mobs, residents) get a small screen halo (aim assist) so a moving target behind a post stays tappable. Nothing is picked through a solid thing. |
| P2-2 | **Verbs: the radial always.** A tap on a thing opens its radial (Go onto, Light, Look, Talk, Order…); a tap on the ground walks. Built for growth: a thing's verbs are data (a registry row per kind), since play-testing will keep changing them. |
| P2-3 | **Standable tops:** crates, barrels, logs, and house roofs (a roof shape per model; one rule in Go physics, its TS twin, the WASM and the realm). |
| P2-4 | **ECS: bitECS.** Everything interactive is an entity: riders, mobs, residents, critters, buildings, villages, props with verbs or tops, doors, stairs, the camera target, and the fleet's objects (repo, resident, run, need). Dense data stays packed (land hexes, instanced decor: one entity per chunk or batch) for the draw-call budget. Systems run in the one clock's loop. `world.ts` becomes the composition root. |
| P2-5 | **Camera rig:** follows behind, free look on drag, eases back; lock-on aware (frames the rider and the target); close by context (PR-4), room clamp, walls fade, pinch zoom. |
| P2-6 | **Strategy view: the 3D world from above, styled** (the visual layering's zoom levels were chosen for this): a strategic look, banners and Civ-style lenses (health, agents working, needs, cost) over the same scene. |
| P2-7 | **CK interactions for the agent system:** residents (repo, village, world) as characters with portraits, states, traits (their definition) and relations drawn on the map; a CK interaction menu per character (Talk, Order an errand, Ask about, Inspect run, Stop, Warm), greyed with reasons; needs as event windows with choices; a council view (the world resident as ruler, village residents as councillors). |
| P2-8 | **Modes: a toggle** (Tab) between explore and strategy; the selection carries over (I1). |
| P2-9 | **Thread panel, both:** face to face with a resident in explore (bubbles in the scene, full transcript one tap away); a side panel for strategy and runs. |
| P2-10 | **Fleet** (plan/28, the fleet session): owner only for now; residents at three scales (repo, village, world); a game MCP both ways (later round); every fleet decision a game Need tied to its building and village. |

Order to build, proposed: ECS core and the composition root (behaviour-neutral moves first) → pick module and verbs
registry → camera rig → standable tops → strategy toggle and look → CK characters, menu, events, council (with plan/28).

### Part 2, amended after its doubt round (owner, 2026-11-16)

The doubt round found most of part 2 already in the code (`world-targets.ts` KINDS and `nearestHit`, `ui-actions.ts`
FOCUS_ROWS, `world-chase.ts` follow/free/combat/room clamp/fade, pinch, the Tab toggle, `world-lenses.ts`, circle tops).
The QA bugs are gaps in them. Owner rows that replace P2-2, P2-4 and P2-3, and settle PR-1 and PR-11:

| # | Decision |
|---|---|
| P2-2' | **Tap by what the thing offers.** A thing whose only verb is Go onto: the tap goes there. A thing with several verbs, or a building: the tap opens its radial. Mobs, the boss and nodes keep select-then-engage. Ground: walk. `charter.md`'s input rule is updated in the same PR. |
| PR-1' | **Desktop:** click a mob engages; click the ground walks; F and the slot keys attack and cast; hold F charges the Mage. |
| PR-11' | **Hover is sticky:** the hot target is kept while it is still under the pointer; a different hit takes over after about 150 ms. |
| P2-4' | **A thin entity index** (`world-entities.ts`: every interactive thing by its string id, kind, space, position, solid, verbs kind), read by pick, lenses, strategy and CK. No bitECS. Fleet runs and needs live in the ui-core store. `world.ts` becomes a composition root through pure-move PRs after wave 1. |
| P2-3' | **Height first.** Roofs are a symptom: the height model is badly managed (props on a hex act as an invisible second tile). A height-model investigation and redesign comes before any standable-top work. |

Step plan (from the doubt round, amended): pick (occluders, a `prop` kind, one walk path, sticky hover) → verbs (the
tap rule, `ActionDef.why`) → camera profiles (arm by context, interior arm, near limit, rider fade; can start now) →
the height model → the entity index and composition-root moves (after wave 1) → strategy lenses and sheet → CK with
plan/28.

### The height model (owner, 2026-11-16, after the height investigation)

The investigation found eight notions of height and seven climb thresholds that disagree; hills are flat invisible
plateaus 3.96 m up, decor circles are flat-topped cylinders as wide as their radius, the trail's own steps block it,
the realm has no building walls, interiors stand on the outdoor ground (report: session transcript, 2026-11-16).

| # | Decision |
|---|---|
| H-1 | **A physics principle, not rules per object.** One collision world of real 3D shapes (hex prisms at their level, hill domes, steep mountain cones, decor profiles, walls to their real height, roof decks, floors, the bank and groove relief); one character controller (`Step`, Go, bit-identical in the WASM and the realm) decides where a rider stands with two numbers; nav is generated from the same world with the same numbers; a small game layer of walkability tags carries design rules only. |
| H-2 | **Platformer feel:** step height 0.3 m, max slope 35°; jump and double jump as today. |
| H-3 | **Hills** are walkable domes where their slope allows; mountains are steep cones, climbed by their trails. |
| H-4 | **The drawn relief** (banks, grooves) is ported into Go: page, route and realm agree. |
| H-5 | **Lane 6** of wave 1 finishes as built; the height model builds on it. |

Migration, each step proven: the probe facts become red tests (the hill rim floats 3.96 m, the mound 0.9 m, the easy
trail unreached with its steps, the interior ground not 0, step bands in world units) → the collision world and
controller as an exact twin of today (a differential test; step-bits bit-identical) → one behaviour change per PR with
its golden diff reviewed (interior floor, feature profiles, decor profiles, walls and roof decks in the realm too, the
two numbers) → nav generated from the world, law 2 over decks → the pick and the camera read the same world. The land
does not move; WASM about +4-8 KB of its 17 KB headroom.

### Part 2, after its second doubt round (owner, 2026-11-16)

| # | Decision |
|---|---|
| H-1' | Same controller structure (`Step` takes discrete samples; no capsule sweep: WASM headroom, and nothing tunnels at 0.5 m sub-steps). One `TopAt`/blocks reader over baked data, used by `Step`, the realm's checks, nav and the pick. Baked tables ride the land wire, not the WASM. |
| H-2' | **Step 0.35 m**, one number for physics, nav and the route (only bodies taller than it are walls). **A ledge mantle:** at the top of a jump, with the feet within about 0.5 m under a rim pressed toward, the rider pulls up; a standing jump climbs one level; routes drop the run-up rule. |
| H-3' | **Bake the real meshes:** hill tiles get height grids, decor radial profiles and pick hulls, baked from the GLBs like the road grooves; the 35° rule is a baked walkable mask (no runtime trig). **Steep flanks slide.** The **easy massif trail is walkable** without jumping (hidden ramp colliders on its steps); medium and hard trails stay jumps. |
| PR-8' | The card tutorial (lane 4) merges as a first step; the visual-tap tutorial comes with part 2's pick and Go. |
| Go seam | `standOn(id) → point`: the foot or door now, the top once nav reads the world. Lane 2's door aim and P2-2' Go onto share it. |

Step plan (open: the QA terrace failure near (-98, 43) does not reproduce on the fixture land; a live-land repro is still to do): S0a height characterisation tests (now) · S0b camera profiles (now; plus the world-tree trunk and the
waypoint arrow) · S1 pick (colliders plus ground, prop kind, sticky hover, click to walk) · S2 verbs (tap rule,
`standOn`, `ActionDef.why`, charter and D-11) · S3 height (step 0.35; bake; interior floor; realm reads `TopAt`, walls and
roof decks; banks; trail ramps) · S4 mantle · S5 nav reads the world (tops, decks, the nav ⊆ controller law) · S6 entity
index · S7 strategy lenses · S8 CK with plan/28. TS lane S1→S2→S6→S7 beside the Go lane S3→S4→S5.

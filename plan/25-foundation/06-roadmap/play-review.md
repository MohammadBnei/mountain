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

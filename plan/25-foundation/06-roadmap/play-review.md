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

## Wave 1 lanes (disjoint files where possible; each its own PR)

1. **Loader and idle.** The ring painted by `world.ts`'s load steps again (the #264 regression); the idle throttle (PR-13) through the one clock (`clock.ts`). Proof: a journey reads the ring's steps; a law test for the idle states.
2. **Session and death.** `Realm.keep`/`join` and the page's first ride (PR-6); the downed state, respawn at the nearest village, the carried item dropped (PR-5) in `internal/sim` and the realm, with the page's downed words. Proof: realm tests on the step clock, a journey that dies and respawns.
3. **Disclosure and the doors.** The two tiers in `town.PublicOf` and its mask (PR-10), the public-fields golden updated field by field; the visitor's door words and Go (PR-9); `game.bnei.dev` sends a non-admin to `world.bnei.dev` (`internal/gate/auth.go` callback). Proof: the privacy golden, a test per hidden kind, a journey to a closed door.
4. **Guests and the tutorial.** The better welcome card, a Sign in chip, "Sign in to …" on locked things instead of "offline/closed", the first-minute hint shown when it is marked seen, the tutorial steps (PR-8) as one ui-core component family. Proof: HUD fixture states, a guest journey.
5. **Land.** Steps outside massifs capped at the auto-jump height; marked massif trails (PR-7); routes onto roofs, crates and wall tops from the nav graph (PR-3, nav side). Proof: law tests on the four fixtures (no step above the cap outside massifs; every trailhead reaches its summit); the land shift is named in the PR.
6. **Sky.** Day-night on the world clock, stars, constellations and the Milky Way, the land's light following it (PR-12); the "Material: color undefined" warnings traced and fixed. Proof: screenshots at four hours of the day, both themes; draw-call and frame budgets.

Each lane: build, local checks, its own journeys or law tests, PR, a fresh review, one fix agent, my own look at the
visible ones, merge on green CI and a clean review.

## Part 2, to design with the owner

One pick module for tap, click and hover (6 raycasters today), one camera rig with context profiles, and `world.ts`
(3600 lines) as a composition root. PR-1, PR-3 (pick side), PR-4 and PR-11 wait for it.

## Behaviour changes, named

The land moves once (lane 5). The public face shows real names (lane 3). A reload no longer always starts at the spawn
stone (lane 2). Death costs the carried item (lane 2).

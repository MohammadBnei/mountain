# Interview round 4 (2026-11-18): the HUD direction, explore and strategy

After S8 (fleet characters) ran on the owner's phone, the owner asked for careful UI work: "I have to press 3-4
buttons to chat to ops"; the transcript and panels, finding characters, and the HUD overall all hurt. A first HUD v2
draft (cards and sheets over the 3D world) was rejected: "the system of card in a 3D world is weird", too much panel,
a crude look. This round rebuilt the direction one level at a time, with visual options on a design canvas
(<https://claude.ai/artifact/TwUNfBf9mruDbkcr3eBwX3>, private until the owner shares it; pages 1 to 10) and a
prototype of the talk mode (<https://claude.ai/artifact/GBu7bZbhHThhCGo3SifHHc>, the "Try it" board).

The rule this round settled on (owner): base each option on patterns that shipped games and apps already use, and
cite them; invented layouts were rejected, the researched one was taken at once.

## The principle

Two views, clearly apart. **Explore** is the 3D game and stays almost bare. **Strategy** is where the information is
dense, and it is laid out in fixed zones: the left zone picks, the right zone reads and writes. Explore follows its
own rules; the zones hold in strategy only.

The owner's requirement of 2026-11-14 (round-3.md, "Owner requirement on the two modes") stands: one switch, the same
possibilities in both views, every action defined once and only presented by each view, and the test that compares
the two lists. What changes is the presentation: explore presents actions through the prompt at the thing and the
radial (no dock); strategy through the left list, the right zone's faces and the Needs you list (no cards over the
scene).

## Decisions

| # | Area | Decision | Canvas |
|---|---|---|---|
| H-1 | Two views | A switch (Tab on desktop, a button on the phone), and strategy is also a place: the bridge in the Capital hall, walked into | 1A + 1C |
| H-2 | Desktop explore | The RPG pattern: the map in a corner with a **Needs you** tracker under it (a quest tracker: who, what, where, how far; a click points the map; gone when empty); the action shows at the thing ("E Talk", hold E for the radial); updates slide in and fade; Esc menu, Tab strategy, M map, H hide the HUD; no on-screen buttons and no dock | 9D |
| H-3 | Phone explore | The map, a one-line "◆ N need you" under it (a tap opens the list), the movement area; a tap on a thing opens its radial with Talk in the centre (the D-11 tap rule); no dock | 2C, 7A, page 10 |
| H-4 | Needs you, in explore | One light on the switch, and an amber mark over each character or building that needs the owner, with an arrow at the screen edge for one off screen. Amber means "needs you" and nothing else | 4A + 4B |
| H-5 | Desktop strategy | Two fixed sidebars around the map: left = pick, centre = map (lenses), right = read and write | 3A |
| H-6 | The left zone | One list in groups: Needs you, Council, Places | 5A |
| H-7 | The right zone | Faces as tabs: Thread, Logs, Metrics, Spans. The default face follows the subject (a character: Thread; a building: Metrics; an alert: Logs). Threads first for characters: thread chips, decisions answered inline, the composer | 6A + 8A |
| H-8 | Data | The right zone is a data-dense inspector, "well fed": the cluster's Prometheus, Loki and Tempo, joined to the picked subject | 8A |
| H-9 | Phone strategy | A navigation rail (64 px) with full-screen destinations: Needs you (like notifications, a count badge, quick answers), Pick, Map (lens chips), Subject (a summary strip, then the four faces), Explore at the bottom | page 10 |
| H-10 | Talking in explore | One thread in two registers: an in-character short line in explore, the full transcript in strategy. Open decisions show as dialogue choices; the model offers 2-4 reply choices; free text is always there | Dialogue board |
| H-11 | The talk mode | Soft: Talk frames the camera on the owner and the character, the dialogue strip replaces whatever sat at the bottom, walking off ends the talk (no Close) | Try it board |
| H-12 | Diagrams in explore | The character plants a glowing screen on the land and both look at it (the mermaid diagram as a texture on a plane); it fades when the talk ends; "Keep in strategy" keeps a copy in the right zone | Hologram board |
| H-13 | Earlier calls kept | A tap focuses everywhere, buildings included; the Menu opens as usual; a far character picked from a list walks toward the owner; a thread switch takes 2 taps | round of 2026-11-18 |

Not taken: a fixed dock in explore (9A, 2A on desktop), fading corners (9C), a bottom bar (2B), the camera-rise
transition (1B), the map as the only picker (3C), tabs by kind (5B), a tree by place (5C; "tree" only meant the
pick-then-inspect pattern), a subject page (6B), the data in the centre (8C), a half-width sheet on the phone (3B as
drawn).

## The sources

- Show information only when needed: Breath of the Wild (CEDEC 2017, "only essential UI", the Pro HUD) and Elden Ring
  (no minimap, no markers).
- The action at the thing: Breath of the Wild's contextual prompt; one "use" key for most verbs, a separate key for a
  risky one.
- A minimap with the tracked task under it and an update area: The Witcher 3's PC manual; Final Fantasy XIV's Duty
  List (a click shows the objective on the map).
- A hide-HUD key: Genshin Impact 4.8.
- The phone: in a landscape phone (Android's medium width class) a navigation rail replaces the bottom bar, for 3 to 7
  destinations; side panels become full-screen overlays on a phone; Grafana's mobile app is summary first, the
  assistant for investigation.

## What changes from today (ui-surfaces.ts MOUNT, ukubi-game main)

- Gone from explore: `dock:focus` and `dock:line` as a fixed dock (their content moves to the prompt at the thing, the
  radial and the slide-in updates), `panel:card` and `panel:thread` (they move to strategy's right zone), the Menu and
  Strategy buttons on desktop (keys).
- New: the Needs you tracker, the edge arrows, the prompt at the thing, the dialogue strip, the land screen, the
  strategy zones and the phone rail.
- Kept: the map, `radial`, `pad`, `crosshair`, `dock:fight` (the fight face), the system states (loader, sign-in,
  banner, reconnecting, respawn), People and Settings as Menu pages.

## Open, for the architecture interviews (next)

The owner has constraints on the architecture; these get their own interviews before any build:
- The data join: an atlas id (`building:repo:agent-fleet`) to its Prometheus labels, Loki stream and Tempo service,
  likely in mountain's `contract` package; who serves the page (the owner-only game server proxying the backends, the
  browser never talking to them).
- How the right zone's faces are fed and cached (live streams or polls, the volume "a lot of data" means).
- The speech tool's new fields (the short line and the reply choices) on the agent-fleet side.
- How the zones and the rail sit on ui-core's surface table and store.

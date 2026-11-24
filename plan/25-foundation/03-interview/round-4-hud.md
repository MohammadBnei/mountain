# Interview round 4 (2026-11-18): the HUD direction, explore and strategy

> Status: history (2026-11-24). Interview record.

After S8 (fleet characters) ran on the owner's phone, the owner asked for careful UI work: "I have to press 3-4
buttons to chat to ops"; the transcript and panels, finding characters, and the HUD overall all hurt. A first HUD v2
draft (cards and sheets over the 3D world) was rejected: "the system of card in a 3D world is weird", too much panel,
a crude look. This round rebuilt the direction one level at a time, with visual options on a design canvas
(<https://claude.ai/artifact/TwUNfBf9mruDbkcr3eBwX3>, private until the owner shares it; pages 1 to 10) and a
prototype of the talk mode (<https://claude.ai/artifact/GBu7bZbhHThhCGo3SifHHc>, the "Try it" board).

The rule this round settled on (owner): base each option on patterns that shipped games and apps already use, and
cite them; invented layouts were rejected, the researched one was taken at once.

Checked against ukubi-game main db56ee51 and infra-bootstrap main 21fdce54. The gaps the doubt cycle found were put to the owner on 2026-11-19; their answers are marked **(owner, 2026-11-19)**.

## The principle

Two views, clearly apart. **Explore** is the 3D game and stays almost bare. **Strategy** is where the information is
dense, and it is laid out in fixed zones: the left zone picks, the right zone reads and writes. Explore follows its
own rules; the zones hold in strategy only. Strategy stays the owner's (and admins'); guests and friends never reach
it (round-3.md, ukubi-game #242).

The owner's requirement of 2026-11-14 (round-3.md, "Owner requirement on the two modes") stands as I1 states it
(ui-core.md): every `work` action is defined once, both views present it, and the test compares the two lists; `play`
actions stay explore's. The right zone's faces (Thread, Logs, Metrics, Spans) and the full transcript are reads, not
actions: explore reaches them through one `work` action, "Open in strategy", which opens strategy on that subject and
face. That action is the existing `overview` action (ui-actions.ts:275-281), extended with a subject and a face;
today it is refused inside a building, and it becomes allowed there, the place staying the world's (D-07) (owner, 2026-11-19). What changes is the presentation: explore presents actions through the prompt at the thing and the radial;
strategy through the left list, the right zone and the Needs you list.

## Decisions

| # | Area | Decision | Canvas |
|---|---|---|---|
| H-1 | Two views | A switch (Tab on desktop, a button on the phone), and strategy is also a place: the bridge in the Capital hall. Walking through its door switches the mode to strategy; the door is the owner's, shut with its reason for a friend or a guest (I7) | 1A + 1C |
| H-2 | Desktop explore | The RPG pattern: the map in a corner with a **Needs you** tracker under it (a quest tracker: who, what, where, how far; a click points the map; gone when empty); the action shows at the thing ("E Talk", hold E for the radial); updates slide in and fade; no dock at rest (the fight face stays at the bottom in a fight); one small Menu button stays for pointer-only play and guests, and anchors the sign-in chip (owner, 2026-11-19); keys: H the Menu (plan/23 D1, unchanged), Esc closes the top chrome as today, M the map, Tab strategy, and `\` hides the HUD, as in Genshin Impact on PC (owner, 2026-11-19; banner, reconnecting and respawn stay visible when hidden) | 9D |
| H-3 | Phone explore | The map, a one-line "◆ N need you" under it (a tap opens the list); tap to walk, the pad only when riding (as today); a tap on a thing follows D-11 (a building or a thing with several verbs opens its radial, its primary act in the centre: a new radial shape; one verb is focus then primary; mobs, the boss and nodes are select-then-engage; the ground walks). A tap on the map corner still enlarges the map; the one small Menu button stays (as on desktop), and the strategy switch is a Menu row, with the "◆ N need you" line opening strategy on Needs you (owner, 2026-11-19). No dock at rest | 2C, 7A, page 10 |
| H-4 | Needs you, in explore | One light on the switch, and a ◆ mark in amber over each character or building that needs the owner, with an arrow at the screen edge for one off screen. Among explore's marks, amber with ◆ means "needs you" only; the minimap's amber problem beacons and the amber at-risk plots become red alert triangles, and the at-risk plots a dashed outline (owner, 2026-11-19) | 4A + 4B |
| H-5 | Desktop strategy | Two fixed sidebars around the map: left = pick, centre = map (lenses), right = read and write. The owner set this shape ahead of the scene research; its open items (projections, the War table) fold into these zones | 3A |
| H-6 | The left zone | One list in groups: Needs you, Council, Places | 5A |
| H-7 | The right zone | Faces as tabs: Thread, Logs, Metrics, Spans. The default face follows the subject (a character: Thread; a building: Metrics; an alert: Logs). Threads first for characters: thread chips, decisions answered inline, the composer | 6A + 8A |
| H-8 | Data | The right zone is a data-dense inspector fed by the cluster's Prometheus, Loki and Tempo, joined to the picked subject. Limits: Tempo keeps 72 h under a byte cap, and spans come mainly from agent-fleet, so most buildings have no Spans; an empty face shows the last data and its age, else "unknown" (round-3.md, Q73-82) | 8A |
| H-9 | Phone strategy | In landscape, a navigation rail (Material 3's 80 dp) with full-screen destinations: Needs you (like notifications, a count badge, quick answers), Pick, Map (lens chips), Subject (a summary strip, then the four faces), Explore at the bottom. Portrait is played: a bottom bar with the same destinations (owner, 2026-11-19) | page 10 |
| H-10 | Talking in explore | One thread in two registers: an in-character short line in explore, the full transcript in strategy. Open decisions show as dialogue choices; the model offers 2-4 reply choices; free text is always there. Residents use the same dialogue strip (it replaces `panel:talk`), guests included. In strategy, Talk (D-12: K talks to the selected building's resident) opens the right zone's Thread face on that subject | Dialogue board |
| H-11 | The talk mode | Soft: Talk frames the camera on the owner and the character, the dialogue strip replaces whatever sat at the bottom, walking off ends the talk; Esc ends it too on desktop | Try it board |
| H-12 | Diagrams in explore | The character plants a glowing screen on the land and both look at it (the mermaid diagram as a texture on a plane); it fades when the talk ends; "Keep in strategy" keeps a copy in the right zone | Hologram board |
| H-13 | Earlier calls kept | The tap rule is D-11 (above); the Menu opens as usual; a far character picked from a list walks toward the owner; a thread switch takes 2 taps | round of 2026-11-18 |
| H-14 | Keyboard in strategy | Tab is the mode switch, so the zones get their own keys: 1, 2 and 3 (no field focused) jump to the left, centre and right zones, Ctrl+[ and Ctrl+] cycle them (F6 belongs to the browser), arrows move in a list, Enter opens, Esc steps back (owner, 2026-11-19) | — |

Not taken: a fixed dock in explore (9A, 2A on desktop), fading corners (9C), a bottom bar (2B), the camera-rise
transition (1B), the map as the only picker (3C), tabs by kind (5B), a tree by place (5C; "tree" only meant the
pick-then-inspect pattern), a subject page (6B), the data in the centre (8C), a half-width sheet on the phone (3B as
drawn).

## Supersedes

- charter.md, "The HUD": "One bottom dock shows context" becomes the prompt at the thing, the radial, the slide-in
  updates and the Needs you tracker; the fight face stays. "At rest ... a small map corner and one Menu" stays on the
  phone and on desktop (H-2, H-3), plus the tracker when something needs the owner.
- round-3.md, "Owner requirement on the two modes": the presentation lines ("explore through the dock, the radial and
  the panels; strategy through the board, the cards and the inbox") become the ones in "The principle" above.
- ui-core.md, "Not doing": "The strategy mode's look ... waits for the scene research" and "A card stack comes first"
  on the phone give way to H-5 and H-9.

## The sources

- Show information only when needed: Breath of the Wild (CEDEC 2017, "only essential UI", the Pro HUD) and Elden Ring
  (no minimap; markers only by the player's choice).
- The action at the thing: Breath of the Wild's contextual prompt; one "use" key for most verbs, a separate key for a
  risky one.
- A minimap with the tracked task under it and an update area: The Witcher 3's PC manual; Final Fantasy XIV's Duty
  List (a click shows the objective on the map).
- A hide-HUD key: Genshin Impact 4.8 ("\" on PC); hiding it also hides enemy health and skill readiness, the risk of
  using it in a fight.
- The phone: in a landscape phone (Android's medium width class) a navigation rail replaces the bottom bar, for 3 to 7
  destinations; side panels become full-screen overlays on a phone; Grafana's mobile app is summary first (its
  Assistant is a Grafana Cloud preview, taken as a pattern only: the cluster runs Grafana itself).

## Every surface (ui-surfaces.ts MOUNT, ui-contracts.ts PANEL_KINDS)

| Surface | Status |
|---|---|
| `menu`, `map`, `big-map`, `map-pop`, `compass` | kept (one small Menu button on both; a tap on the map corner enlarges the map, as today) |
| `crosshair`, `pad`, `radial`, `tip` | kept (the radial gains the primary act in its centre) |
| `dock:focus`, `dock:line` | gone at rest: the prompt at the thing, the radial and the slide-in updates take their content |
| `dock:fight`, `leave` | kept: the fight face in a fight; Leave goes in the inside radial, and D-01's `leave` beside the fight face stays |
| `people:voice`, `panel:people`, `panel:settings` | kept: panels opened from the Menu |
| `panel:card` | kept in explore as the light Look read for every role; the owner's decisions and data move to strategy's right zone |
| `panel:talk` | replaced by the dialogue strip (residents and characters, every role) |
| `panel:thread` | moves to strategy's right zone (Thread face) |
| `panel:quick`, `panel:yard`, `panel:keeper` | kept: in-world interactions |
| `panel:reader` | kept: in-world, the owner's (OWNER_PANELS) |
| `panel:cockpit`, `panel:control`, `panel:fleet`, `panel:cluster`, `overview`, `strategy` | fold into strategy's zones (owner only) |
| `loader`, `signin`, `signin-chip`, `banner`, `reconnecting`, `respawn` | kept (the chip anchors to the Menu button) |
| new | the Needs you tracker, the edge arrows, the prompt at the thing, the dialogue strip, the land screen, the strategy zones, the phone rail |

## Open, for the architecture interviews (next)

The owner has constraints on the architecture; these get their own interviews before any build:
- What is shown against how and where it is shown: sources, typed data, components and placement as separate layers,
  so the style and the layout change at will (the owner's first constraint).
- The data join: an atlas id (`building:repo:agent-fleet`) to its Prometheus labels, Loki stream and Tempo service,
  likely in mountain's `contract` package; who serves the page (the owner-only game server proxying the backends, the
  browser never talking to them).
- How the right zone's faces are fed and cached (live streams or polls; the volumes).
- The speech tool's new fields (the short line and the reply choices) on the agent-fleet side.
- How the zones and the rail sit on ui-core's surface table and store; the strategy mode at the bridge (the
  `overview` row requires `place !== 'inside'` today).
- Portrait, now played (H-9): explore in portrait (the map corner, the tracker line, the pad) has no board yet; it is drawn
  before the phone build.

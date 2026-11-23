# 18 · Villages: one world for the owner, friends and engineers

> **Superseded in part (2026-11-23) by `25-foundation/00-brief.md`,** row "The old district layout": "**Retire the town page** and its grid (`town.Layout`, `town.Place`, `placement.json`, `cmd/townpin`, and the `Whole` gate on founding). This supersedes plan/18's \"until it retires at parity\"". The rest of this plan stands.

Status: draft 3, 2026-10-31, after two doubt rounds (product and code), the owner's answers and the control room.

This plan supersedes two things in plan/17:
- in §The world, the bullets "rings are stack layers" and "placement inside a ring follows the roads";
- D1's ring wording.

Everything else in 17 stands: the disc, the turtles and elephants, the moons, the Rimfall, the logged-layout
principle and the altitudes.

## Intent (confirmed by the owner, 2026-10-31)

- **Outcome:** one world, laid out as option C on the owner's "Ukubi village maps" page:
  - a capital;
  - the friend-facing apps' bazaar first from the spawn;
  - the infra cities round the capital;
  - the hamlets as outposts in the frontier;
  - countryside and wilds out to the Rim.
  The owner runs the cluster from it, and visitors play in it.
- **Users, in order:**
  1. **Friends.** Fun alone or with the owner (multiplayer comes later), and the public apps made for them.
  2. **Engineers and LinkedIn contacts.** The system and its history, told through NPC dialogue and quests in
     fragments, the way Elden Ring tells its lore.
- **First minute, phone first:**
  1. The visitor spawns on foot in the capital square.
  2. Visual guidance pulls them down the main street to the bazaar.
  3. A quick menu opens on a stall, then the portal takes them to the real app.
  4. Quest 2, which needs sign-in, gives the bike. The ship comes later.
- **Access:**
  - **Guests** get one quest (quest 1), no multiplayer, and progress kept in the browser.
  - **Signing in** through authentik's GitHub and Google sources unlocks everything.
  - Owner-only data (health beyond coarse up or down, commands, hosts) never reaches visitors.
- **Success:** a friend opens a real app within a couple of minutes of landing, and keeps exploring. It is measured
  by a privacy-safe funnel.
- **Constraints:**
  - the disc keeps its size;
  - the layout is deterministic and logged;
  - the LLM writes words only, and visitor calls are capped;
  - a phone shows the capital within about 5 s on 4G.
- **Out of scope here:**
  - plan 19: the quest engine, NPC lore content, mobs, the progress store, the LLM;
  - plan 20: realtime multiplayer;
  - the sign-in setup in infra.
  This plan builds the map those plans need: the spawn square, the quest-giver spots, wild zones, game slots, stalls,
  gateable rides, and a hard-coded quest 1 so success can be tested before plan 19.

## Owner decisions from the doubt rounds

1. **Sign-in:**
   - through authentik's GitHub and Google sources: no passwords, one click;
   - visitors land in a `visitors` group bound to no application except the game;
   - the email comes from the OIDC `email` claim (Google `email` scope, GitHub `user:email`);
   - GDPR basics: a privacy note, a stated purpose, an opt-in for anything beyond the game, account deletion;
   - the setup itself is an infra PR, stacked and never merged by agents.
2. **Lore** is written by the owner with an agent. It is drafted from ADRs, git history, PRs and plans, approved by
   the owner, and stored as data. A live LLM only adds tone.
3. **Stalls (4):** wird, dream-analyst, editable-website, voc on steroid. wedding-2026 is being decommissioned and is
   left out. has-journal is private. There is no agent-fleet demo.
4. **No greeter speaks on its own.**
   - Guidance is visual: the main street frames the bazaar, with awnings, banners and lanterns, and a faint beacon in
     today's beacon language.
   - The quest-giver resident stands silent at the notice board with a marker, and talks only when tapped or E'd.
   - After the first app opens, the beacon moves to the next goal.
5. **Down apps:** a down app's stall closes its shutters and shows a "Closed for now" sign. In the quick menu, Open is
   greyed out, while the pitch and Details stay. This uses only the coarse `states` already in the public live
   frame.
6. **Funnel:**
   - steps: landed, reached the bazaar, opened an app, quest 1, signed in, quest 2;
   - counted server-side, with no cookies, no third parties and no per-guest tracking;
   - owner-only, shown in the cockpit.
7. **Defaults the owner didn't contest:**
   - quest 1 is hard-coded in V3;
   - the load budget above;
   - the owner keeps search, the cockpit and the ship as fast paths, so infra further out is deliberate. Don't "fix"
     it.

## The disc: fixed size, served once

- **`DiscR` = 54.7 server units** is a Go constant (about 540 world units), served as `disc.r`. The client's `landOf`
  takes R from it, and the world gate asserts it.
- **Everything that read the rings reads `DiscR` instead** (finding 2):
  - `MoonRadius` (moons.go:80);
  - the turtle radius (`UnderOf`);
  - the client's `rimOf` (world-rings.ts:68);
  - `landOf`'s R (world-land.ts:308);
  - `regionAt` (world-land.ts:372);
  - the ring labels (world-rings.ts:62);
  - the status line (world.ts:403, 483; world-chrome.ts:149);
  - `PublicDisc.Rings` (public.go:203).
- Go tests pin the moons, the turtles and the wonder anchors at today's positions.
- **Lots stay within `DiscR` − 4.** The client no longer grows R from pads.
- **Frontier relief** fades to under hill height (`HILL_L`·`LEVEL_H`) by about 0.85R. The Rim stays a low coast:
  "no wall at the Rim" holds.

## Zones and placement (option C)

| zone | where | what |
|---|---|---|
| Capital | centre, up to about 0.12R | the town centre (`default` civ, `app:mountain`) at (0,0), the six wonder anchors at `hubRadius` on bearings w·π/3, the spawn square, the info buildings, the twin-city river and bridges kept clear of all of these |
| City ring | centres at **0.25R (135 world units)** | the cities (civs of 10 or more): apps on the spawn square's main street, the others in the solved order |
| Countryside | between and round the cities, and along the long roads | roads, fields, farms, windmills, orchards |
| Frontier | 0.5R to R | the hamlets (civs of 7 or fewer) as outposts at about 0.75R to 0.9R, plus wild forest, ruins, landmarks, secrets, wild zones |

**Travel.** With centres at 0.25R, by bike at 16 u/s:
- capital to a city's centre: about 8.4 s;
- between neighbouring city centres: about 12 s (√2 × 135 = 191 units, with today's four cities 90° apart; a fifth
  city shortens it to about 10 s);
- capital to an outpost (0.75R to 0.9R): about 25 to 30 s.

On foot at 4 u/s, the spawn square to the bazaar's near edge must stay under 160 units (40 s). Every number is
centre to centre unless it says edge. The gate measures them.

**Placement rules (finding 9):**
- A hamlet sits outward from the **city** it shares the most roads with, counting roads to cities only.
- Hamlets that share a parent city fan out ±k° in name order.
- A hamlet with no city link takes a free frontier bearing.

**Growth budget (finding 11):**
- Each village has a maximum radius, from its angular sector and its gap to the next zone.
- Lots past that radius become logged outskirts lots in the village's own sector.
- The gate checks that no lot overlaps across villages.

## Stability: what is logged and what never moves (findings 1, 3)

- **The first v3 lay-out is a fresh one.** `layout()` (internal/play/town.go) re-lays once when the logged Input's
  version is below 3. Today, a changed hash only drifts (up to 0.5).
- **The `layout.stepped` event grows:**
  - the village table: civ, centre, kind (city or hamlet), slot in the order;
  - each building's lot (an integer axial hex) and `face` (a hex edge, 0 to 5).
- **Frozen after the first lay-out:**
  - the order and the centres;
  - a newcomer civ takes the next free slot;
  - a hamlet that becomes a city moves into a reserved city-ring slot, and only that village re-lays, logged once;
  - hysteresis reads the logged kind.
- **Buildings:**
  - a newcomer takes the next free lot on its layer's band;
  - a building that leaves frees its lot, and the lot stays empty;
  - a change of layer or audience is one logged move for that building only.
- **Replay test:** a v2 log becomes v3 once, then stays stable over new roads, a new civ, a newcomer and a removal.

## Meaning: where a building stands tells you something

- **Village = civ.**
- **Distance from the capital = audience:** the apps bazaar first, then infra, then outposts.
- **Inside a village, distance from the square = layer:** core at the square, then workshops, data and services,
  with edge at the gate.
- **Stalls:**
  - friend-facing apps take the bazaar spine's stall lots first;
  - the stall list sits behind one seam in the layout's input. V1 seeds it with the 4 apps, and V7's Showcase editor
    becomes its source;
  - the other apps stand in the bazaar's back streets.
- **Doors face streets.**
  - `face` points at the street toward the partner a building shares the most roads with, or at the square.
  - The world client uses `face`. The town page keeps `rot`.
- **Wonders and landmarks never move.**

## Village shapes

A shape is a **generator in integer axial hex coordinates**, using the client's convention (`hexXZ`,
world-hex.ts:66). It gives ordered lots, nearest the square first, for any count up to its budget.

| village (count today) | kind | shape (topology only) |
|---|---|---|
| capital | city | **twin city** (Ankh-inspired, our own names): a river, bridges, a lane maze, the spawn square and its main street |
| apps (17) | city | **bazaar** (after Tehran): a short covered spine of 4 stalls, a grid of back streets, gardens |
| platform (18) | city | **étoile** (after Paris): boulevards from a circus |
| code (16) | city | **presqu'île** (after Lyon): a peninsula between two rivers, the oldest building on a hill |
| operations (11) | city | **harbour town**: quays on the capital's river mouth |
| guard (6) | hamlet | **watch fort**: a wall, a gatehouse |
| data (4) | hamlet | **lakeside**: houses round the stores' lakes |
| fleet (4) | hamlet | **roots camp**: stables and a smithy |
| `default` | capital | the town centre at (0,0) |

- **Size rule:** a city at 10 or more buildings, a hamlet at 7 or fewer, and in between it keeps its logged kind.
- **`villageShapes`** is a frozen registry in Go. An unknown civ gets a shape chosen by a hash of its name.
- **Determinism (finding 8):**
  - only integers, + − × ÷, Sqrt and `sincos`/`dir` (disc.go);
  - no atan2: `face` is a hex edge, chosen by integer comparison;
  - the order solver is an exhaustive integer search, with ties broken by name;
  - the gate checks that `snapDisc` moves no building.
- **Water is part of the shape (finding 4):**
  - the generators reserve **3-hex water corridors** for the twin city's river, the presqu'île's rivers and the
    harbour's quay basin;
  - they leave **two empty hex rings** round each store's lake (`clearLake` needs about 21 units from every pad).
  - Shape water is a new `LandSpec` kind (`canal`), drawn from the shape and exempt from data-road routing.
  - Gate: lakes equal stores, and every shape's water is drawn.

## Roads and rivers (finding 5)

- **Between villages:**
  - one road per pair of villages that share supply roads, as wide as the link count (a path, a lane, a paved road);
  - it leaves from the gate facing the neighbour;
  - the long outpost roads cross the frontier.
- **A data road becomes a river only inside one village.** Data roads between villages are drawn as roads, with an
  aqueduct look for data. This replaces today's ~15 long rivers into `db:pg-proxmox`.
- **Streets inside villages** replace today's per-building meandering paths.

## The capital

- **The spawn square:** the visitor spawn, the notice board, the silent quest giver, the main street to the bazaar,
  and lanterns and a beacon for guidance.
- **Reserved cells:** the town centre, the six wonder anchors, the spawn square, the bridges and the river corridor.
- **Info buildings:** ids `info/<name>`, anchored. Each opens an existing panel or a story panel.

| building | owner | visitor (needs sign-off) |
|---|---|---|
| Notice board | incidents, banners, waiting on you, recent syncs | the quest board, what's new, the public weather and moons |
| Library | ADRs, techs, file trees | the lore hub: approved fragments, public ADRs, the project timeline |
| Observatory | live signals per app | the sky and the moons |
| Clock tower | age, layout age, last syncs | the town's age and milestones |
| Post office | agent sessions | sign in (later, the visitor's messages) |
| Harbour | ingress, guards, imports | the outside links |
| Bank | capacity per realm | nothing |

- **Models:**
  - library: the church;
  - observatory: tower_A;
  - clock: tower_B;
  - post office: the market;
  - bank: the castle;
  - harbour: coast hexes and crates;
  - notice board: a plaque.

## Quick menu, portal and story cards

- **Clicking or pressing E on a stall** opens the quick menu: the name, a one-line pitch, **Open the app** (greyed
  when the app is down), and **Details**. It is sized for a thumb on the phone, and builds on the aim and targets
  highlight and selection.
- **Open the app** opens the app's public URL in a new tab, and the app's own authentik login takes over.
- **Details** opens the story card: what it is, why it exists, its tech, its public ADRs, and its repo link only when
  its `public` source says so.
- **On any other building, a visitor gets the name and the story card.** The owner keeps today's selection and the
  bottom deck.

## Privacy (finding 7)

- **New public fields,** each listed in `public-fields.txt` with the owner's sign-off:
  - `disc.r`, `disc.villages`, `disc.zones`;
  - per building: `face`, `audience`, the stall pitch and URL;
  - the info buildings' visitor contents;
  - the funnel is never public.
  - Visitors see every village's civ name, its lots and the roads between villages. Building ids, civs and supply
    roads are public today. The owner's sign-off covers this explicitly.
- **The Showcase editor's output (V7):**
  - only published fields with a public mark reach the public town;
  - drafts, unmarked fields, the editor's log and every control-room route and datum stay owner-only;
  - `public-fields.txt` still gates the public output.
- **Per-field public source:**
  - story cards and repo links come from the Showcase editor, marked public per item (seeded empty until V7);
  - with no mark, nothing is shown.
- **Coarse up or down per building stays public,** as today (`live.states`, `cards.state`). The shutters need it.
  Host and VM health, raiders and causes stay owner-only.

## Visitors, guests and rides (the map's part)

- The rides registry gets a `gate`. The owner has every ride, and a visitor starts on foot.
- Quests 1 and 2 are hard-coded in V3.
  - Quest 1: reach the bazaar, open an app. Guests can play it, and its progress is kept in localStorage.
  - Quest 2 needs sign-in: a signed-in visitor who has finished quest 1 collects the bike at the capital's
    stables.
  - Until plan 19's progress store lands, that unlock is a flag derived from the signed-in session plus quest 1.
- Visitors spawn at the square. The owner keeps today's start.

## Life

- Residents get a home village and a home lot.
- A day routine:
  - morning: home to their building;
  - noon: the square or the market;
  - evening: the tavern;
  - night: home.
- They walk the streets and roads. The "always asleep" bug is fixed here.
- Carriers drive the village roads.

## Fun

- **Playground:**
  - city shapes place stairs, ledges and rooftop runs one jump (≤1 UNIT_H) or two jumps (≤2 UNIT_H) high;
  - bridges over the capital's river;
  - ramps on the boulevards.
- **Places:**
  - the market;
  - fountains;
  - a fishing quay;
  - a tavern;
  - gardens.
- **Secrets:** one collectible per city.
- **Game slots:** one empty plaza per city with a "coming soon" sign.

## Funnel (owner only)

The server counts the steps (landed, bazaar, app opened, quest 1, signed in, quest 2) per day. There is no
identity: guests are counted by event, never tracked. The cockpit shows the funnel.

## What breaks and must move

- **Go:**
  - `town/disc.go`: rings, sim and Drift give way to zones, villages, shapes, lots, `face` and `DiscR`;
  - `discVersion` becomes 3;
  - `internal/play` `layout()` re-lays once below v3;
  - `game/layout.go`: the event grows;
  - disc, play and layout tests are rewritten, plus the replay test and the moons, turtles and wonders pins.
- **Public golden:** see §Privacy.
- **Client:**
  - `REGIONS` are keyed by zone;
  - `landOf` takes `disc.r` and the shape water;
  - village boards replace ring labels;
  - status line;
  - civ cards at village centres;
  - minimap villages;
  - doors follow `face`;
  - village roads;
  - the visitor spawn, the ride gates, the quick menu, stalls and shutters, the beacon, quest 1.
- **Gates:**
  - Restate "no wall at the Rim" and the Rimfall count by `disc.r` and zone.
  - The calm gate's Rim fishing spot moves to the frontier.
  - New checks:
    - spawn to the bazaar on foot under 40 s;
    - the travel numbers above;
    - doors face street hexes;
    - buildings stay inside their village and no lots overlap;
    - `snapDisc` moves nothing;
    - lakes equal stores and shape water is drawn;
    - stalls are on the spine;
    - no public field beyond the allowlist;
    - a visitor starts on foot;
    - every control-room route returns 401 or 403 to a non-owner;
    - a visitor's page draws no trapdoor and no control-room data;
    - the phone shows the capital within the load budget.
- **Plan/17** gets a note that 18 supersedes it.
- **The town page** (`/`) keeps its own hex grid (`districts`, `hex`, `RingHex`, `placement.json`) until it retires
  at parity.

## Slices

| slice | what | depends on | files it owns |
|---|---|---|---|
| V0 | this plan in mountain, plus the plan/17 note | none | mountain plan/ |
| V1 | server layout v3: `DiscR`, zones, villages, the frozen order, shape generators with water, lots, `face`, the stall list behind one seam (seeded, swapped by V7), the event, the migration, tests | V0 | town/, internal/play/, game/ |
| V2 | client: zones, `disc.r`, shape water, roads, doors, boards, minimap, cards; restated gates | V1 | world-land.ts, world-hex.ts, world-rings.ts, world.ts, dev gates |
| V3 | capital and first minute: the spawn square, info buildings, quick menu and portal, story cards, shutters, beacon, ride gates, quest 1, funnel | V2, plus aim (world/aim) | new world-capital.ts, world-quick.ts; world.ts is touched last, in sequence |
| V4 | countryside and frontier: fields, farms, landmarks, ruins, wild zones | V2 | world-land.ts and world-hex.ts decor |
| V5 | life: homes, routines, street walking, awake residents, carriers on roads | V2 | world-people.ts, world-carts.ts |
| V6 | fun: playground geometry, places, secrets, game slots | V2, V4 | the shape generators (Go) plus new world-fun.ts |
| V7 | the control room: the interior under the clock tower, owner-only routes, live game data, the Showcase editor with logged events, the lore queue, the LLM budget, the notice-board message, bans, re-lay, sim controls; the slab window | V3 | new world-control.ts, game/showcase.go, internal/play/control.go |
| infra | authentik GitHub and Google sources and the `visitors` group: stacked PR, never merged by agents | none | infra-bootstrap |
| plan 19 | quests, lore pipeline, mobs, progress store, LLM with caps | V3 | |
| plan 20 | multiplayer | 19 | |

- V3 to V7 overlap on world.ts, world-land.ts and world-hex.ts.
- V4 and V6 run in sequence on the land files. V3 and V5 can run in parallel with them.
- One integration pass merges world.ts. V7's trapdoor and slab window touch it last.
- V3's quick menu and story cards read the seeded Showcase data behind V1's seam. V7 swaps the editor in.

## The control room (owner, 2026-10-31)

- **What it is:** the game's own secret control room, an interior (world-interior) under the capital's clock tower.
- **The entrance:** a trapdoor and stairs down into the slab, drawn only for the owner. Visitors never get the
  entrance, its routes or its data.
- **The disc keeps its thickness.** The only outside trace is a small lit window in the slab's underside, a tease
  seen from the ship below.
- **Secrecy is flavour, not security.** Every control-room route and action checks the owner session on the server.
- **Game operations here; cluster operations stay in the cockpit.**
  - **Live data:**
    - the funnel;
    - visitors online, guests vs signed in: a count of live connections, with no identity kept for guests;
    - quest completions;
    - LLM spend against its caps;
    - the layout log;
    - errors.
  - **Actions:**
    - **the Showcase editor:** per app, on a stall or not, URL, pitch, story, repo link and which fields are public;
    - the lore approval queue;
    - the LLM budget;
    - a message on the notice board;
    - banning a signed-in visitor, keyed on the authentik subject. Guests can't be banned one by one; abuse is held by
      rate limits;
    - re-laying a village;
    - sim controls.
- **Showcase edits are logged events in the game's state**, as the layout is. A visitor sees an edit at once, and it
  survives restarts. A stall change is one logged building move.
- **The Showcase editor is the one source** for stall, URL, pitch, story and public marks. It replaces the game-side
  list and mountain's realm files: the owner wants it editable from an admin service. V1 seeds the 4 stalls behind one
  seam.

## Owner answers (2026-10-31)

1. **Stalls:**
   - URLs and pitches are written by the owner in the Showcase editor;
   - Voco'steroid has its own domain;
   - no drafted pitches.
2. The visitor column of the info buildings is signed off as drafted. Village names and the roads between them are
   public.
3. **Clock tower history:** a bounded ring of the last 500 atlas events (deploys, syncs, incidents). Milestones are
   pinned by hand.
4. **Quest 2's reward:** the bike. No horse for now.
5. **Story text and public marks** live in the Showcase editor (§The control room).

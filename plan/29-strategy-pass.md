# 29 · Strategy pass: fixes, upgrades, and where fun meets work

Status: draft, 2026-11-23. Order and personas agreed (§ 5); the turn-based fight is under design.

Sources:
- a hands-on walk of the strategy view on `game.bnei.dev` (desktop, 1400 × 657, with a narrow window tried too) with the
  owner, 2026-11-23, at ukubi-game main `28388035`;
- the owner's own findings during the walk: logs that say "unreadable", the ugly selected row in the left zone, and a
  chip clicked in the world that does not select its row;
- plan/25 ui-core (the strategy zones, hud-build H3, round-4-hud H-5 to H-14), the `game-fleet-integration-intent`
  decisions (P2-4 to P2-9), and plan/19 (the storyboard: "work can be played").

Paths are under `ukubi-game/cmd/game/websrc` unless stated.

## 1. What works today

- Picking a row in the left zone sets the subject, opens its default face, flies the map there and highlights the row.
  A village's buildings open under it.
- Metrics for apps and repos: requests, 5xx, p95 latency and pods, over 15 min, 1 h and 24 h, as a chart or a
  sparkline.
- Logs, with a level filter, a search, Tail and "Open in Grafana". Spans are listed.
- ops-lead's Thread renders Markdown well. Its actions are Talk, Ask about, Inspect run, Start and Stop.

## 2. Fixes

### Selection and the left zone

| # | What | Seen | Where to look |
|---|---|---|---|
| F1 | A chip clicked in the world does not select or reveal its row. The right zone follows, but the left zone stays on the last village, with nothing highlighted. Picking from the list works, so only world → list is missing. | vos-monolith, picked from the map | `ui-strategy.tsx` `slotActs.current`; the scene's pick → store `subject` path |
| F2 | The selected row is drawn as a smaller outlined box inside the row's own outline: a double border. Villages have no expand chevron; their buildings are only indented. | every row | `hud-data.css`, `cockpit.css` |
| F3 | A building label clicked on the map once dropped the view from strategy to explore. It was seen once and is not yet reproduced. | Postgres label | the label's pointer handler against the mode toggle |
| F4 | Search: Enter does not pick a single match. Results do not name their village. A repo and an app with the same name look like duplicates (`ukubi-game`, `dream-analyst`). | `ukubi-game`, `agent-fleet-core` | `ui-strategy.tsx` find |
| F5 | The whole left zone scrolls, so the header and the search scroll away. Pin them and scroll only the list. | Default village open | `hud-data.css` |

### Camera and map

| # | What | Seen |
|---|---|---|
| F6 | Underside does not turn off: with the lens button released, the camera stays under the disc. | Underside → watch |
| F7 | The next pick after Underside put the camera inside the terrain. | Code → agent-fleet |
| F8 | Picking a character (ops-lead) does not move the map. | Council → ops-lead |
| F9 | Labels collide. District names sit under the zones ("Operations"). Chips cover a resident's speech bubble. The lens legend covers chips (Library). "Platform" is clipped at the top edge. | Capital, Apps |

### Faces: data shown wrong

| # | What | Where to look |
|---|---|---|
| F10 | "unreadable" hides the cause. `failed()` maps every Connect code except Unauthenticated and InvalidArgument to one word. Show the code and the message, and log them. The building where the owner saw it is still to be named. | `ui-telemetry.ts:111-118` |
| F11 | An empty window says "No line matches." even with no search typed. Say "no lines in the last 15 min" (the window), and keep "No line matches" for a search. | Logs face |
| F12 | Logs show raw JSON lines. Parse a JSON line: show `msg`, colour the level, and fold the rest of the fields behind a toggle. | Logs face |
| F13 | Spans are mostly the game's own polling (`/api/state`, `/api/town/live`, `/api/town/card`, `GET /`). The telemetry Watch stream shows as a 15,014 ms span. Hide our own polling by default and mark streams as streams. | Spans face |
| F14 | ops-lead's Thread shows about 20 empty "SYSTEM / system" rows, "TOOL / result" rows and raw tool-call JSON. Drop empty system rows, and fold tool calls into one line each ("Grep `spawns` in definitions") that opens to show the call. | `ui-thread.tsx` |
| F15 | Threads are named by their first message cut off mid-word ("…and delete the co"). Give a thread a short title. | thread switcher |
| F16 | mountain shows "–" for requests, 5xx and p95 while it has 1 pod. Say why (no ingress, so no request metrics), not a dash. | Metrics face |

### Noise and stale data

| # | What |
|---|---|
| F17 | "Playing solo: you joined the world from another tab" sits under the zone chips. |
| F18 | The voice-chat mic prompt opened in the middle of the walk, on an unrelated click. Ask once, at a calm moment, not over strategy. |
| F19 | A narrow desktop window overlaps the zones. The phone form only comes with touch. Below a width, use the phone's one-zone layout. |
| F20 | `platform-thot` still has a building. thot is retired. |
| F21 | The village "Default" holds the Capital's buildings. Name it. |

## 3. Upgrades

| # | What |
|---|---|
| U1 | **A village face.** Today every village says "no telemetry here". Show a roll-up: buildings, healthy, degraded, down, unknown, the worst building, open alerts, and the residents at work. The cluster face's four tiles are the model. |
| U2 | **Coverage.** The cluster shows 7 healthy and 44 unknown. Make unknown visible on the map (fog), and list what each unknown building lacks: a probe, a rule, metrics. |
| U3 | **Telemetry for data stores.** pg-proxmox says "no telemetry for a db". Cover Postgres, Redis and Garage (exporters exist upstream). |
| U4 | **Parity with explore's Details.** Explore's building Details has "Attacks and risk", "Parts · N not measured" and Inspect per component. Strategy's right zone has none of it. Bring it in as a face, under the I1 parity rule. |
| U5 | **Lenses.** Let lenses stack (supply over watch). Draw supply for the selected building only, with each line ending visibly at a building. Give walls and land a visible effect at strategy zoom. Make each legend a key of colours, with the prose on hover. |
| U6 | **Needs you and the Council.** Both were empty on the walk (only ops-lead sits on the Council), so neither has been exercised. Seed a sim scenario that fills them and walk it. |

## 4. Personas: where fun meets work

Each persona is followed by what it wants, what stops it today, and the bridge between play and the real system.

### P1. The owner, in the evening

- **Wants:** to know in 30 seconds whether anything is wrong, then to play or to fix.
- **Stopped by:** "nothing needs you" next to 44 unknown, which says neither calm nor alarm. The way in is a menu, not the
  world.
- **Bridge:**
  - **The world shows health directly.** A sick building smokes and its banner droops. Mobs gather at a blind tower.
    The owner sees it while playing.
  - **The Capital's resident gives a three-line evening brief on arrival:** what moved, and what needs you.
  - **A fix is a quest.** An errand ordered from a resident is a worker run, shown as builders walking to the building.
    A merged PR repairs the building.

### P2. The on-call owner, at 2 a.m.

- **Wants:** the shortest road from an alert to its cause.
- **Stopped by:** F12 to F14, and no link from an alert to its building.
- **Bridge:**
  - **An alert is a siege on its building**, and its card waits in Needs you.
  - **Arriving opens the right face:** Logs filtered to errors, and ops-lead's diagnosis already in the Thread.
  - **The siege lasts exactly as long as the incident.** No win in the game while production is still broken.

### P3. A friend who is an engineer (a village guest)

- **Wants:** to see what was built, and to help a little.
- **Bridge:**
  - **Read-only lenses in the public world:** the supply roads show the real architecture.
  - **Adopt a building:** follow one repo's village; its resident tells you about the open issues; your merged PR counts
    as a quest done.
- **Guard:** no internal logs or metrics reach guests. The public pod keeps serving the snapshot only.

### P4. A visitor (public, not technical)

- **Wants:** a pretty world, and the feeling that it is real.
- **Bridge:**
  - **Real events, no details.** A deploy rings the bell and raises a crane. Traffic is walkers on the roads. A quiet
    night looks quiet.
  - **The builder-yard quest (plan/19)** says what each building does.

### P5. The phone player

- **Wants:** a two-minute check-in from the couch.
- **Bridge:**
  - **Strategy on the phone is one zone at a time**, so open on Needs you.
  - **A decision is a card**, swiped to approve or reject, with the resident's face on it.
  - **A daily "kept town":** 24 h with no unknown building. It rewards adding monitoring, not tapping.

### Rules across all five

1. **Rewards follow real outcomes only:** a resolved alert, a merged PR, a building that gains coverage. Never time spent
   in the game. This is the director's rule (it bends pacing, never rewards), applied to the owner's world.
2. **Unknown is not healthy.** Unknown is fog, so making the system observable is exploring.
3. **One action, one path.** An order from explore and an errand from strategy reach the fleet through the same
   command (P2-7, P2-8).

## 5. Owner answers (2026-11-23)

1. **Order: agreed.** One wide PR first: F1, F2, F10 to F14 and U1. The rest of the fixes follow in a second PR, then
   U2 to U6.
2. **Personas: P1 (the owner) and P3 (the engineer friend) are essential.** P2, P4 and P5 wait.
3. **Alerts are mobs you fight, turn by turn.** The LLM does the real work. Each turn the owner answers it, and the
   exchange is the fight's transcript. This needs a deeper design talk of its own: § 6, to come.
4. **F10:** the building that showed "unreadable" is still to be named.

## 6. The fight: an alert as a turn-based mob (design talk, in progress)

Decisions so far, in the order the owner took them:

1. **A turn is one move ops-lead proposes, answered by the owner** (2026-11-23).
   - The mob acts when the real alert changes: it fires, gets worse, or a new signal arrives.
   - ops-lead proposes one move at a time, with its risk: a read, a query, a draft PR.
   - The owner approves, redirects, asks why, or retreats (stops the agent).
   - The real tool's result lands in the transcript as the turn's outcome. The transcript is the fight's log.
2. **Semi-real time, on an active time battle clock** (the pattern of Final Fantasy VI to IX and Grandia; 2026-11-23).
   - Urgency comes only from the real signal. The mob acts on a change in the signal (error rate, restarts, SLO burn),
     never on a made-up timer. A quiet incident is a quiet fight.
   - ops-lead keeps working between the owner's turns. It runs its read-only moves itself (logs, metrics, describe)
     and narrates them as its attacks.
   - A move that writes pauses on its card with a countdown (60 s to start). When the countdown ends, nothing writes.
     ops-lead takes the safest read-only fallback, and the mob gains ground on screen only, never in production.
   - This is agent-fleet's own behaviour: reads run, writes wait for the owner.
3. **No health bar: the mob wears down, as in Monster Hunter** (2026-11-23).
   - The mob's state is the real signal past its threshold (an SLO burn, a 5xx rate), shown only through its body:
     it limps, slows, breaks parts, flees. No number and no bar. A grouped alert is a pack, one mob per firing alert.
   - ops-lead's reads are scouting: they reveal the weak spot (the likely cause) and do no damage. Damage is the real
     signal recovering after a change the owner approved.
   - **Victory:** Alertmanager resolves the alert, and it stays quiet for a hold window (10 min to start). The building
     is repaired.
   - **Retreat:** the owner stops. The mob camps at the building; it stays under siege on the map and in Needs you.
   - **Lasting fix:** the PR that removes the cause is merged. It is the real loot and is counted apart from victory,
     since a restart can win a fight and fix nothing.
   - **Loot:** ops-lead's incident note in its memory, and the transcript as the postmortem draft.
4. **The monster attacks the building from outside, and the party fights it there** (2026-11-23).
   - The monster besieges the building: it claws the walls, sits on the roof, chokes its roads.
   - The fight is on open ground before the building, with a wide camera: the monster, the building, the party and the
     harm in one view. Never inside the building.
   - The building shows the real harm (smoke, cracks, residents fleeing) and mends as the monster wears down.
   - Strategy and explore see the same fight: strategy from above, with the cards and the transcript in the right zone;
     explore on foot.
5. **Friends join the fight itself: the LLM conversation. The agents are the allies** (2026-11-23).
   - The fight's transcript is a shared thread, as in a shared agent workspace. A friend who joins writes into the same
     discussion as the owner, each line marked with its speaker.
   - The party is the humans plus the agents. ops-lead is an ally, and every worker it spawns appears as an ally too:
     it arrives when its run starts and leaves when the run ends.
6. **Only the owner approves a write** (2026-11-23).
   - The approve button on a write card is the owner's alone, as agent-fleet's audience rule (`platform-admins`) has it.
   - A friend's line steers and never decides. ops-lead reads it as input, may run any read a friend asks for, and
     turns any write a friend suggests into a card for the owner.
   - When the owner and a friend disagree, ops-lead follows the owner and says so in the thread.
   - A friend sees the cards and their results, redacted. Raw tool output only when it is cleared for guests.
   - With the owner away, friends and agents scout and talk; every write card waits, and the siege goes on.
7. **A fight starts when the owner engages; ops-lead scouts first** (2026-11-23).
   - Only alerts routed to the owner (warning and critical, on a building in the world) spawn a monster. Info is a
     critter that wanders near the building and is never fought. The monster's size follows severity.
   - ops-lead, already the first responder, scouts from the moment the alert fires, so the transcript holds its first
     reads and a guess at the cause when the owner arrives.
   - The fight starts when the owner walks up to the monster or picks the siege card in Needs you. Until then the
     monster besieges the building, with one gentle notice (a horn at the screen's edge).
   - Alerts on one building are one pack, fought together. Sieges on several buildings are separate; the owner picks.
     ops-lead scouts them all, within its spawn limits.
   - An alert that resolves before the owner arrives: the monster leaves, and the transcript says it left before you
     came. No loot, no fake win.
8. **Hack and slash steers the investigation** (2026-11-23).
   - The monster's parts are the suspects ops-lead's first scouting names: the claws the last deploy, the jaw the DB
     pool, the tail memory pressure, the wings the network or the ingress.
   - A heavy hit on a part asks ops-lead to check that suspect next: one read (logs, metrics, the last deploy's diff).
     Reads only, so a sword never touches production.
   - The real result decides the part: ruled out, it breaks off and the monster shrinks; confirmed, it glows as the
     weak spot.
   - The cause found, the monster sheds its disguise and shows its true form (for example "the pool exhaustion"). The
     fix cards come only then.
   - Rage is the real signal: the error rate climbs and the monster rages; a new alert joins and a second monster comes,
     or a head grows.
   - Real limits: hits queue reads within ops-lead's budget (spawn limits, calls per minute). Faster hits stagger the
     monster with no read behind them, and the game shows it. Button-mashing becomes choosing where to look.
   - While ops-lead thinks (a long session, a workflow), the monster fights back for feel. The owner dodges, places,
     and lines up the next part: the next thing to check.
   - The dock holds the write card and its countdown: approve (a heavy strike lands the change), redirect (a short
     text), ask why, retreat. Typing slows the clock. On the phone the card is a full-width sheet.
9. **A small, fast model narrates: transcript in, world events out** (2026-11-23).
   - ops-lead gets no game tools. A separate small model (the narrator) reads the transcript as it streams and emits
     typed world events: a suspect named, a part ruled out or confirmed, the true form, rage, an ally arriving or
     leaving.
   - The narrator is reusable beyond fights: a resident's talk, a worker run (builders walking), a deploy (the bell and
     the crane) all become world events the same way.
   - It must be very fast: one call per transcript message, a short context (the fight's state and the new lines), its
     output constrained to the event registry's schema.
   - Planned guard (to confirm in Q9): write cards never pass through the narrator. They come from agent-fleet's own
     permission requests, deterministically. A missed or wrong event costs only a picture, never a write.

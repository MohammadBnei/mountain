# 24 · One HUD: the dock, the Menu and the panels

Status: rev 3, owner answers 2026-11-12 and the doubt cycle's fixes.

Sources:
- the owner interview and the mockup review of 2026-11-12 (memory `hud-design-intent`, `hud-coherence`);
- the approved mockups: `.wt/hud-mock/DESIGN.md` and `out/*.png` (P1–P6 phone, D1–D4 desktop);
- the inventory of every current HUD element: `.wt/hud-shots/INVENTORY.md`;
- the owner's answers to Q1–Q18 (rev 2) and one doubt cycle (rev 3).

What it changes in other plans:
- **plan/23.**
  - `#focus` becomes the dock's focus face (§3), and the Menu drops from 8 `GLOBALS` to 4 (§4).
  - The phone gets a dock inside buildings, and the desktop deck leaves explore (§7).
  - Resource nodes become focusable: plan/23 §5 said "a node is never focused (it is shot)".
  - D-pad left changes from Build mode to the self radial (§3).
- **plan/22.** The P2 chat log moves into People › Chat. P3 mail plugs into People › Mail. P5 adds the Party tab. P4's
  mic "near Jump" has no Jump to sit near (§12).

## Context

Every claim was checked on ukubi-game main at `c2e0bfc`. That is plan/23's `132ffc0` plus `e1971f6`, the routes'
terrace double jump, which moved `world-view.ts` lines after `:1449` by 8. Paths are under `cmd/game/web` (`.html`,
`.css`) or `cmd/game/websrc` (`.ts`).

### What is on screen

The inventory counted these floating pieces:
- 11 at rest on the phone, 13 on the desktop;
- 24 with the phone's Menu open;
- 36 in a desktop overview with a selection.

It also found five panel styles and four radii on like things:

| Fact | Where |
|---|---|
| Glass is a hand-kept list in three places; `#focus`, `#toast`, `#deck`, `#menubtn`, `#chat` are on none | `theme.css:7-39,42,48,86` |
| Rem-based glass outside the tokens | `.fight-hud`, `world.css:995` |
| Focus buttons r8 in an r12 card; pad buttons r16; radial r12; chat chips r8 | `world.css:2145,2188,94` |
| One building named three times on a desktop hover: `#hottag`, a `.bcard`, the box | `world-targets.ts:431-439` |
| The phone menu unfolds `#top` into 12 rows, one of them "Menu", which opens a second menu | `world.html:39` |
| Phone hints: `#toast`, 7 s; desktop: the static help line `#line` | `world-modes.ts:129,151-160`; `town-phone.ts:201` |
| Chat: 4 chips, 5 lines fading after 10 s, a form, `#chatshow`; polled every 500 ms | `world.html:78-84`; `world-chat.ts:6-10`; `world.ts:2358` |
| The pad shows exploring outside with no panel; at rest on foot it draws Build, Run, Jump and Skill 2 | `world.html:104`; `world-input.ts:263` |
| The desktop deck `#bottom` opens on a selection, inside, with talk or with `?` | `world.html:89-100`; `world-deck.ts:19-42` |
| The deck's cells: act, enter, talk, leave, up, weapon, pvp, controls, plus Next | `world-deck.ts:19-42` |

### Movement and jumps

| Fact | Where |
|---|---|
| The rider walks unless running is asked for: Shift; the pad's Run toggle; L3; a double tap on the ground; an engage's quick second tap | `world-view.ts:456,1195-1196,1692,1931-1935`; `world-input.ts:235`; `world.ts:1215,1271` |
| Running is read in `drive` and `ridePose`; the ship's boost is `sprint()` | `world-view.ts:1436,1549,1398` |
| Speeds: foot walk 4, run 7; bike 16 and 26 | `world-ride.ts:45-46,59-60` |
| Auto-jump only on a route: `advance` reports a ledge (`m.jump`, now a double jump on two-level terraces) and `routeHold` holds it | `world-view.ts:1474-1477,1552`; `world-route.ts` |
| A plain `walk` (inside, or the engage's `walkTo`) never jumps; a drag turns the camera's yaw and pitch | `world-view.ts:2111-2113,1240` |
| A finger held still on a target for `LONG_PRESS_MS` 450 fires Skill 1 at it | `world-view.ts:209,1245-1253` |
| `skillAt` aims Skill 1 through the screen point | `world-view.ts:1997-2002` |

### Fight, gathering, build

| Fact | Where |
|---|---|
| Each class has two skills and an ultimate | `world-skills.ts:9-20` |
| The Mage's two skills are both `spell`: Skill 1 casts the selected spell, Skill 2 cycles `MAGE_SPELLS` (frost_bolt, nova, ward) | `world-fight.ts:179`; `world.ts:1438-1455` |
| Q fires on keydown, and its repeat passes; X and LB fire on the press | `world-router.ts:320-328`; `world-view.ts:1698-1701` |
| The right button fires Skill 1 on release within `CLICK_SLOP`, however long it was held | `world-router.ts:482-486` |
| The left button held `CHARGE_MS` is the Mage's charged attack, not a skill | `world-router.ts:449-458` |
| A resource node is gathered by attacking it: the aim takes a hot node in reach | `world.ts:1356` |
| On a touch screen, nodes are not tap targets | `fightKind('node', …)` without `touch`, `world-targets.ts:102,228` |
| Nodes are never focused | `world-focus.ts:33`; `town-types.ts:497` |
| `NODES` names each node, and what it gives: `wood` or `iron` | `world-fight.ts:256-264` |
| A PvP hit on a rider names no target on the client: the shot goes along its line, and the realm judges it | `world.ts` shoot, `target` 0 for a rider |
| Build mode: Q cycles `ORDER` (wall, floor, stair, sapling); the pad's Attack places; R turns; X takes down | `world-build.ts:40` |
| Its words show on `.build-hud` | `world-build.ts:210` |
| B, the phone menu's Build row, the pad's Build or D-pad left start it, where `canBuild` holds | `world-modes.ts:186-216`; `world-bindings.ts` PADS |
| Build zones: the ring 30–45 out from Wolf Meadow, or the rider's own yard lot | `world-build.ts:193` |
| No people are drawn inside a building; a building's resident is reached through talk (today's Menu Talk) | `world-people.ts:378`; `world-actions.ts:129-137` |
| `actHint` inside knows the stair and the machines | `world-view.ts:1024-1030` |

### The focus box, the Menu, the overview, the visitor

| Fact | Where |
|---|---|
| The box sits in `#bottom` on the desktop | `world-focus.ts:163-174` |
| On the phone it is fixed over the pad (`--pad-h`), and hidden inside | `world.css:2176-2212,2200` |
| `GLOBALS` holds 8 entries, opened by `#deck`, H or Start | `world-actions.ts:117-139`; `world.ts:2416-2421` |
| The radial is an ellipse, rx 140 by ry 124 | `world-radial.ts:14-21,51-55` |
| Enter opens the chat outside | `world.ts:2690` |
| Every key in `KEYS` and the command letters (C E F I K L M O P Q R U V X Y; an action may take one only as its own default) | `world-bindings.ts:39-57,106-118` |
| The pad's 15 rows take every button but B and Home | `world-bindings.ts:60-76` |
| The visitor page removes `chip`, `island`, `sim`, `account`, `next` and `status` | `world-public.ts:121` |
| It adds `#signin`, `#sound`, `#under` and `#welcome`, and it is explore only | `world-welcome.ts:47`; `world-view.ts:678,803` |
| Parked P3: #250 is the server (open) | ukubi-game |
| `realm/p3-mail-ui` (`2d46f61`) adds `world-mail.ts` (Inbox and Write), the rider radial's Message, and the mail unread on `#deck` | ukubi-game |

## Decisions (owner, 2026-11-12)

| # | Decision | Shape |
|---|---|---|
| 1 | At rest, both tiers: only the map corner (top right) and one Menu button (bottom left) | §1 |
| 2 | One bottom dock, only in context: focus, fight, build, line | §3 |
| 3 | The fight face: a thin HP row, 2 skill slots, and the ultimate with an energy ring | §3 |
| 4 | A tap on a slot fires it. Holding a slot opens a radial of the class's skills, and a slide assigns: it swaps, never fires. The choice is kept per class. A tap on the mob attacks | §3 |
| 5 | The build face: Place, Rotate, Done (rev 3, for 320 px). The piece choice is a long-press on Place | §3 |
| 6 | A line slides over the dock for about 4 s. A tap opens its panel, then the focus returns. A fight is never covered; lines wait | §3 |
| 7 | The Menu radial: Map, People, Overview, Settings | §4–6 |
| 8 | The overview is a side panel: search, Needs you, a Cockpit link (owner only), "Cluster and districts ›" | §7 |
| 9 | The desktop deck lives only in the overview. Explore on the desktop uses the dock as the phone does | §7 |
| 10 | One family: `--radius-lg` containers, `--radius-md` buttons, 44 and 56 px buttons, the theme's glass, Jost, 16 px edges, 8 px gaps, tokens only | §2 |
| 11 | Removed or merged: chat chips and log; toast and tutorial bubble; every always-on pad button; the help line; duplicate labels; crumbs and "PvP off" while exploring; the top bar | fate table |
| 12 | Full speed by default on every ride. On foot there is no Run button and no double-tap run; walk only inside and when arriving at an auto-route target | §8 |
| 13 | The bike's and ship's speed button stays, and their other controls are unchanged | §8 |
| 14 | Auto-jump on every walk, tap or route. There is no Jump button and no manual jump on the phone (rev 3 cut the swipe) | §8 |
| 15 | One label per thing, one dock, nothing floats on its own | §3 |

### Defaults the owner did not object to

| Q | Default, owner did not object |
|---|---|
| Q4 | Shift and L3 do nothing on foot; C while moving slides (`world-moves.ts:1`) |
| Q7 | Wood and Iron: one line in Settings › Wardrobe's foot |
| Q8 | The desktop fight face: 44 px, key caps right button, Q, G |
| Q9 | One-time hints show in the dock line on both tiers, in short words; the full key list is in Settings › Controls |
| Q10 | The phone overview: the same panel, full width less 2 × 16, at the top, capped at 50 vh |
| Q12 | A guest's Sign in sits in Settings' head; `#welcome` is the one panel at rest on a first visit; the realm-down banner becomes a dock line, the Menu's status dot and "World offline" in People |
| Q14 | H stays the Menu key; the cap shows the bound key |
| Q15 | `.fight-boss` is the boss's one label; the world tag skips the boss |
| Q16 | The legend and the account go in the Cluster panel; Underside stays the carrier's focus act (`world-actions.ts:104`); Secrets and Fleet stay in the cockpit's quick row |
| Q17 | The fight face needs `inFight`, a hit within `HURT_MS`, or Ready; the PvP flag alone does not show it |
| Q18 | `ARRIVE_WALK` = 3 units, a knob set by feel |

### Defaults for the owner (N)

The plan builds each default, and each is a one-line swap.

| N | Question | Default |
|---|---|---|
| N1 | What does the bike's and ship's speed button do, now that full speed is the default? | It toggles down to today's cruise speed (`speed`). Shift and L3 hold the same toggle there |
| N2 | Where is the piece choice? | A long-press on Place opens a radial of `ORDER`. Desktop Q still cycles |
| N3 | How is the self radial opened? (G is the ultimate, and every pad button is taken) | Phone: tap your own rider. Desktop: F, allowed as its own default and read only in explore; in the overview F stays the console's Fleet. Pad: D-pad left, which is Build mode today; Build moves into the self radial, and a stored pad `build` migrates through `MOVED_PAD` |
| N4 | Is there a manual jump on the phone? | No. Auto-jump picks the height, and the trade-off is recorded in Risks |
| N5 | What does the skill pool hold? | The class's skills only, not the attack or the ultimate. Knight, Barbarian and Rogue have exactly two, so their radial only swaps; the Mage has three |
| N6 | How do the slot keys work? | The right button fires slot 1 and Q fires slot 2, for every class. Q, X and LB fire on release when held under 500 ms, not on the press |
| N7 | What happens to a line during build? | It waits, as in a fight |
| N8 | What does the inside focus show? | The stair, else a machine, else the building's resident ("Resident · Talk"), else the building ("Hall · Leave · ⋯", ⋯ holding Talk and Look). No people are drawn inside, so "the nearest resident" is the building's own |
| N9 | Where does Take down go? Q6's owner answer listed it, but rev 3's 320 px face has three buttons | Into Place's long-press radial, after the pieces. Desktop X is unchanged |
| N10 | How long does Ready last? | Until Sheathe, a ride, going inside, or the overview |
| N11 | How does slot 1 swap on the desktop? (The right button already fires on release however long it is held) | A 500 ms mouse hold on the dock's slot 1 button. Hold Q for slot 2; on the keyboard, picking slot 1's skill in slot 2's radial swaps them, so every arrangement is reachable |
| N12 | What does Attack on a PvP rider do? | It fires the class attack at the rider's point; the realm judges the hit, as today's PvP clicks do. Attack is the rider's primary only while both have PvP on |
| N13 | What are the gather labels? | Chop for a node that gives `wood`, Mine for one that gives `iron`, else "Gather" |
| N14 | What does the realm-down status dot look like? | A small `--warning` dot at the Menu's other corner, apart from the unread `--danger` dot |

## Shape

### 1. Zones (both tiers)

| Zone | Phone (390×844) | Desktop (1440×900) | Shown |
|---|---|---|---|
| Map corner | top right, 72 px circle, inset 16 + safe area | top right, 160 px circle, inset 16 | always, except in the overview and under the big map |
| Menu | bottom left, 44×44, inset 16, foot on the safe area + 8 | bottom left, 44 px "Menu" + key cap | always, the same spot in every state |
| Dock | bottom, from Menu's right edge + 8 to the right inset | bottom centre, max 560 | §3 states only |
| World tag | over the fight target | the same | a fight target; buildings in the overview |
| Panels | full width less 2 × 16, above the Menu | left, above the Menu: People 380, Settings 640, Overview 360 | on demand |

- **The phone map** moves from the top left at 64 px (`world.css:1560-1567`) to the top right at 72 px.
- **The desktop map** goes from 168 px (`:1739`, `--where`) to 160 px, at `top: var(--hud-inset)`.

### 2. The component family

**New custom properties** go in `tokens.css :root`:

```css
--hud-inset: var(--space-4);   /* screen edge to any HUD element */
--hud-gap: var(--space-2);     /* between items, and the dock's padding */
--hud-row: var(--space-3);     /* between rows in a panel */
--btn: 44px;                   /* tier 1: Menu, dock actions, ⋯, radial items, tabs, panel controls */
--btn-fight: 56px;             /* tier 2: the phone's fight and build dock buttons */
--map-corner: 72px;            /* 160px at min-width 1200 */
--dock-max: 560px;             /* the desktop dock's width cap */
```

| Part | Rule |
|---|---|
| `.glass` | the container; replaces the three hand-kept `:is()` lists (`theme.css:7-39,48,86`) with the same rules, plus `--radius-lg` |
| `.glass-btn` | a free-standing button: glass without the stars, `--radius-md`, `min-height: var(--btn)` |
| ghost / primary / on | 55% `--surface-raised` + 1 px `--edge-strong` / an `--accent` fill / today's `theme.css:131` |
| exceptions | the map: a 50% circle, 2 px ring, one N on the rim (`world.css:925-956`); key caps: `--radius-xs` |
| type | title-3 dock name and panel titles; label buttons and tags; body-sm chat; overline hints, heads, caps; metric cluster numbers |
| colour | `--game-hot` target; `--game-harm` mob HP; `--game-hp` own HP; `--game-energy` the ring; `--danger` unread; `--warning` realm down (N14) |

**`data-hud`** marks every HUD surface with its role:

| Role | Surfaces |
|---|---|
| `rest` | `#worldmap`, `#menu` |
| `dock` | `#dock`, `#said`, `#pad` (bike and ship only) |
| `panel` | `#people`, `#settings`, `#overview`, `#cluster`, `#card`, `#flow`, `#sheet`, `#talk`, `#yard`, `#keeper`, `#reader`, `#cockpit`, `#quick`, `#island`, `#inspector`, `#radial`, `#mappop`, `#welcome`, `#sim`, `#tip`, `#status` |
| `world` | `#hottag`, `.mobbar`, `.fight-boss`, `.bcard`, `#maptip`, `#crosshair`, `#hitmark`, `.dmg`, `.fight-tick` |

### 3. The dock: one element, five states

**The element** replaces `#focus` (`world.html:107`):
`<section id="dock" class="glass" data-hud="dock" aria-label="Dock" hidden>`. It has four faces: focus, fight, build
and line.

**Ownership:**
- `world-dock.ts` (new) holds the state rule, the line queue, and the fight and build faces.
- `makeBox` (`world-focus.ts:140`) still builds the focus face, with `tapHold`.
- Deleted: `place()`, the `--pad-h` observer (`world-focus.ts:163-174`) and the phone rules (`world.css:2176-2212`).

**The state rule (pure, tested):**

```ts
export type DockState = 'rest' | 'focus' | 'fight' | 'build' | 'line';
export interface DockIn {
  view: 'explore' | 'inside' | 'action' | 'flying';
  covered: boolean; // plan/23 covered(): a panel, talk, the sheet
  build: boolean;   // build.on()
  fight: boolean;   // inFight || now - hurtAt < HURT_MS || ready (Q17, N10)
  foot: boolean;    // the fight face is on foot; the bike and the ship keep their pad
  line: boolean;    // a line is up and its pacing allows it
  focus: boolean;
}
export const dockOf = (d: DockIn): DockState =>
  d.covered || d.view === 'action' || d.view === 'flying' ? 'rest'
  : d.build ? 'build'                 // lockToggle refuses while building (world.ts:1155)
  : d.fight && d.foot ? 'fight'       // never covered: lines wait
  : d.line ? 'line'
  : d.focus ? 'focus' : 'rest';
```

**The faces:**

| State | Contents | Phone | Desktop |
|---|---|---|---|
| focus | name over "Hold for more" · primary · ⋯ | the primary is a word (Look, Talk, Chop, Leave) | "Look `E`", and ⋯ "More (hold E)" |
| fight | HP row over slot 1 · slot 2 · Ultimate (a conic `--game-energy` ring to `energy / ENERGY_MAX`) | 3 × 56 px | 44 px, caps: right button, Q, G |
| build | Place (primary) · Rotate · Done, plus a note row for the piece and the refusal (`world-build.ts:210`). 3 × 56 + 2 × 8 = 184 px | 56 px | 44 px, caps: click, R, B |
| line | glyph · source ("Chat · Sami", "Mail", "Hint", "3 new") · one ellipsed line · chevron · a timer hairline | the same | the same |

**The focus face's new sources** (`focusOf`, `INTERACTABLES`):

| Source | Rule |
|---|---|
| resource node | `focusable` drops the node exclusion (`world-focus.ts:33`); `FocusKind` keeps `node` (`town-types.ts:497`); its `fightKind` gains `touch` (`world-targets.ts:228`). Act: Gather, labelled Chop or Mine (N13), run as today's attack on the hot node (`world.ts:1356`). A desktop click still attacks it |
| PvP rider | acts `[Attack (when realm.pvp() && pvpOf(id)), Player…]` (N12) |
| pastime | after carried: a `garden` or `water` focus, already in `INTERACTABLES`; replaces `#calm-hint` |
| inside (N8) | `actHint` (`world-view.ts:1024-1030`) returns the stair, a machine, the building's resident ("Talk"), else the building ("Leave", ⋯ holding Talk and Look). Nothing inside depends on the deck or the overview |

**The skill slots** live in `world-kit.ts` (new, pure plus storage):

| Part | Rule |
|---|---|
| `poolOf(cls)` | `MAGE_SPELLS` for the Mage, else `[skill1, skill2]` (N5) |
| `slotsOf(cls)` | from `ukubi-slots` (JSON `{cls: [a, b]}`, each access in a `try`, as `world-modes.ts:46-73`); with none, the pool's first two |
| `assign(slots, i, id)` | puts `id` in slot `i`, swapping if the other slot holds it |
| `fireSlot(i, target?, point?)` | replaces `skill1`, `skill2` and `cycleSpell` (`world.ts:1438-1455`). Aims at the given target and point, else the engaged target (the locked mob `lockId`, or the focused PvP rider), else `ultAim(false) ?? ahead(r)`; then `run(id, chest, to, pressed)` (`world.ts:1461`) |
| inputs keep their aim | `skillAt` (`world-view.ts:1997`), the right button, pad X and the dock's slot button pass their target and point; a world long-press on a target fires slot 1 at it (`world-view.ts:1245-1253`, kept from plan/23:526) |
| hold to swap | 500 ms opens `radial.open('slot', …)` on the existing ellipse; a release on an item assigns and fires nothing |

| Input | Slot 1 | Slot 2 | What the hold does today |
|---|---|---|---|
| touch | long-press the dock button | the same | nothing (new button) |
| gamepad | hold X | hold LB | nothing: both fire on the press (`world-view.ts:1698-1701`); now they fire on release under 500 ms |
| desktop | a 500 ms mouse hold on the dock's slot 1 button (N11) | hold Q | the right button fires on release however long held (`world-router.ts:482-486`), so slot 1 uses the dock button; Q fires on keydown, repeat passes; now on release under 500 ms |

| Part | Rule |
|---|---|
| build face | `build.place()` (today's pad Shoot in build), `build.turn()`, `build.toggle()`; Place's long-press: a radial of `ORDER` plus Take down (N2, N9); `.build-hud` goes, its words are the face's note |
| self radial (N3) | opened by a tap on your own rider (a new touch-only `self` kind in `world-targets.ts`, hit on the avatar's bounds), F, or D-pad left; on the existing ellipse. Entries: Ready (on foot outside, realm up: the fight face with no hit needed, for ward, rally, leap or a PvP opener), Sheathe (N10), and Build where `canBuild` holds. There is no build-zone focus: the dock rests at the yard and in the meadow. `PINNED_KEY` is deleted (`world-modes.ts:44,115-127`) |
| line queue | `hintQueue` (`world-modes.ts:77-98`) moves here and gains live chat (`world-chat.ts` `hear`) and mail news (P3). A line shows `LINE_MS` 4000; then the focus or rest holds `LINE_GAP_MS` 3000; arrivals meanwhile collapse into one "n new" line that opens People; a hint ranks above chat or mail; fight and build hold lines (N7); a line unseen at its timeout adds to unread. A tap opens People › Chat, People › Mail on the item, or dismisses a hint. The line is one `<button>` |
| motion | rise 8 px and fade over `--dur-panel`; faces cross-fade over `--dur-press`; the box never moves; reduced motion swaps at once. `#said` speaks a focus and a line once; fight and build stay silent |
| one label | `#hottag` only for a fight target (`world-targets.ts:431`); `.bcard`s never draw in explore (`world-cards.ts` `hidden()`); the tagged target's `.mobbar` hides |

### 4. The Menu: 4 entries through `GLOBALS`

**The button.** `#deck` leaves `#top` as `<button id="menu" class="glass-btn" data-hud="rest" aria-haspopup="menu">`.
- It keeps its handler (`world-chrome.ts:172-175`) and `menuOpener` (`world-radial.ts:48`), and opens the Menu under
  itself, as today.
- **Dots:**
  - a `--danger` dot for letters, mail and missed lines, with the count in `aria-label`;
  - a small `--warning` dot while the realm is down (N14), with "world offline" in `aria-label`.
- **While the Menu or a panel is open,** it is pressed and reads "Close (Esc)".

**`GLOBALS`** (`world-actions.ts:117-139`) becomes:

| id | label | when | run |
|---|---|---|---|
| map | Map | always | `minimap.toggle()` |
| people | People | always, visitors and realm down included | `people.open(lastTab)` |
| overview | Overview | owner page | `view.flip()` |
| settings | Settings | always | `settings.open()` |

- **Gone from the Menu:**
  - Cockpit (in the overview);
  - Mail and Chat (People);
  - Controls and Wardrobe (Settings);
  - Talk (a focus act, N8).
- **Layout.** The existing ellipse, with no new layout and no scrim.
- **Keys** stay: H or Start, Esc, M and Tab. `?` (`world-router.ts:244-245`) opens Settings › Controls.

### 5. People: one panel, two tabs

- **The element.** `<aside id="people" class="glass" data-hud="panel" role="dialog" aria-label="People">`. It has a head
  with Close, and 44 px tabs: Chat and Mail · n. Party comes with P5.
  - It is a non-modal chrome in `chromes` (`world.ts:1983-1986`).
  - Its fields are the router's typing context.
- **Chat.** `bindChat` (`world-chat.ts:115-204`) rebinds to the tab:
  - the last `KEEP` lines; the form always shows;
  - **gone:** the chips, Hide and Show, `#chatshow`, `SHOWN_KEY`, `FADE_MS`, `SHOW` and `fading`;
  - unread counts lines that came while the tab was hidden;
  - **desktop Enter** opens it with the input focused (`world.ts:2690`). A submit or Esc closes the panel and gives
    focus back to the canvas.
- **Mail.**
  - Until P3, the owner's tab is the letters inbox (`world-keeper.ts` `inbox`).
  - P3's `world-mail.ts` becomes the tab body, and the rider radial's Message opens Write.
- **Realm down.** The panel reads "World offline" over its tabs. The owner's letters still show under Mail: they come
  from the owner pod.

### 6. Settings: one dialog, three tabs

- **One dialog.** `#controls` and `#wardrobe` (`world.css:1193-1335`) merge into
  `<dialog id="settings" class="glass" data-hud="panel">`, with tabs Controls, Wardrobe and Sound.
  - The modal predicate (`world.ts:1547,2669`) becomes `settings.open()`.
- **Controls tab:**
  - **Ride:** the rides open to this rider (`world-view.ts:674-675`), with `#ride`'s handler;
  - **PvP:** from `#pvp` and the deck cell;
  - then today's camera, keys and pad tables, with the new `self` key and pad rows.
- **Wardrobe tab.** Today's body, with the rider preview (`world.ts:1007-1010`). Wood and Iron go in its foot.
- **Sound tab.** `#sound`, plus the quick row's Sound (`world.ts:2049`). P4's voice row lands here.
- **Head.** A guest's Sign in.
- **Build** has left Settings: it is the self radial's.

### 7. The overview, and the deck's end

**`#top`** (`world.html:38-61`) is retired on both views; the fate table places each piece.

| Part | Holds |
|---|---|
| `#overview` (`panel`, 360 wide, top left) | a head with Explore `Tab` and Close; Find (`/`); Needs you; "Cockpit ›" (owner only, never built on the visitor page); "Cluster and districts ›" |
| `#cluster` | a back row; the 4 metric groups (`world.html:45-50`); the health, the world, the age, Live; the villages (`villages`, `ui.go`); the legend |
| the selection view (the deck's end) | a pick replaces Needs you with a back row, `#portrait`, Next as the primary (Enter), and the `#grid` commands (act, enter, up) as 44 px rows |

- `#bottom` (`world.html:89-100`), `deckOpen`, `body[data-deck]` and its CSS are retired. Talk and leave go to the inside
  focus (N8), weapon to the slots, pvp and controls to Settings.
- **Closing the overview:** its Close, the Menu's Close or Esc flips back to explore (`view.flip()`). Esc first leaves a
  selection view.
- **Gone:** the crumbs (`world.html:63`), the "1 to see" chip and card, the corner map, `#line`. `.bcard`s stay,
  restyled.

### 8. Speed and jump

**One pure gait rule** in `world-ride-step.ts`:

```ts
export const ARRIVE_WALK = 3; // Q18: a knob
export interface GaitIn { ride: RideId; inside: boolean; toEnd: number | null; slow: boolean }
// slow: the bike's or the ship's speed button, Shift, L3 (N1); ignored on foot (Q4)
export const sprints = (g: GaitIn): boolean =>
  g.ride !== 'foot' ? !g.slow
  : !g.inside && (g.toEnd === null || g.toEnd > ARRIVE_WALK);
```

**Code changes:**
- **Reads of the rule:** `world-view.ts:1549`, `drive`'s speed (`:1436`) and the ship's boost (`:1398`). `toEnd` is
  the distance to `walk`, else to the route's end.
- **Deleted:**
  - `walkRun` (`:440,1511,1935,2113`);
  - the double tap (`:1930-1932`);
  - `walkTo`'s and `followTo`'s `run` (`:384,388`);
  - `engageRun` (`world.ts:1176,1215,1271`).
- **The pad's Run** hides on foot. On the bike and the ship it becomes the speed toggle (`pad.slow`).
- **The realm.** The wire's `sprint` (`world-realm-wire.ts:31`) carries the rule's result.
- **Words.** The hints drop "double tap to run" and "hold Sprint" (`world-chrome.ts:90,96-99`). The key is named "Slow
  (bike, ship)" (`world-bindings.ts:46,69`).

**Auto-jump on every walk.** `routeHold` (`world-view.ts:1474-1477,1552`) extends to a plain `walk`:
- `ledgeAhead(pose, dir)` probes the land one step ahead, through `p.land`, as `stand` does;
- it jumps when the rise is over the step-up and within the jump's reach, a double jump for two levels, as the routes
  now do;
- routes keep their own `m.jump`;
- Space stays the desktop jump.

### 9. Phone and desktop specifics

**Phone (`TOUCH_PHONE`, `world-modes.ts:14`):**
- **The pad on foot.** `padShows` (`world-input.ts:263`) gains `&& a.ride !== 'foot'`.
  - Off foot it is as today: Advance, Jump or climb, Attack, and the speed toggle.
  - On foot: no pad, and no centre dot (`world.css:155-165`).
- **Gone:**
  - `#pad .buildpad` and `[data-build]`;
  - `#mapgo` (the big map's pop has Go to it, `world.html:67`);
  - `#rides`;
  - `#menubtn`, the phone menu and `placer` (`world-modes.ts:100-114,162-244`).
- **`controlsOf`** stays for the ride's pad.
- **`#sheet`** stays the Look surface, and the dock hides under it.

**Desktop:**
- The dock sits at the bottom centre, with key caps.
- Hover focus reads `#dock` (`world-focus.ts:99-108`).

### 10. Accessibility

- **Focus order:** Menu, the dock, then an open panel. `#settings` is a modal `<dialog>`.
- **Names:**
  - "Menu (H), 2 unread, world offline";
  - "Chop: Oak";
  - "Frost bolt, slot 1 (hold to change)";
  - "Ultimate, 60% charged";
  - "Place wall (hold for pieces)";
  - "Chat from Sami: …, open People".
- **Live regions:** `#said` speaks a focus and a line once.
- **Targets:** at least `--btn`; the phone's fight and build buttons are `--btn-fight`.
- **Keyboard slot swap:** hold Q (N11).
- **Reduced motion:** no rise, fade, radial scale (`world.css:2253`) or timer animation.
- **Reduced transparency:** `.glass` falls back to `--surface-glass-solid` (`theme.css:86`).

### 11. The visitor page

- **The Menu:** Map, People (always) and Settings. People reads "World offline" while the realm is down.
- **At rest:** the two `rest` roots, plus the `#welcome` panel on a first visit.
- **`OWNER_IDS`** (`world-public.ts:121`) gains `overview` and `cluster`. The Cockpit link is never built.
- **`#under`** goes.

### 12. Plan/22 hooks

- **P3:**
  - People › Mail is `world-mail.ts`;
  - its unread feeds the Menu dot;
  - its news comes as a dock line;
  - the rider radial's Message opens Write.
- **P4:** the mic's place is the owner's call when P4 starts. PTT/OPEN goes in Settings › Sound.
- **P5:**
  - adds the Party tab: members, invite and party chat;
  - Invite and Report join the rider radial, beside PvP's Attack.

## Every current element and its fate

`h` = `world.html`, `c` = `world.css`, `tc` = `town.css`.

| Element | Where | Fate |
|---|---|---|
| `#top` bar, `.mark` | h:38-61; c:1405 | removed |
| `#menubtn` + phone menu | h:39; c:1426; `world-modes.ts:172-244` | removed (Menu) |
| `#health`, `.chip.world`, `.age`, `.live`, `#statsbtn` + `#groups` | h:41-51; c:1729 | merged into Cluster panel |
| `#mode` Explore | h:52; c:6 | merged into overview head + Menu › Overview + Tab |
| `#search`, `#find`, `#names` | h:53-55 | merged into overview Find |
| `#deck` | h:56; c:335 | kept as `#menu` (`rest`) |
| `#helpbtn` | h:57; c:1861 | removed (`?` → Settings › Controls) |
| `#sound` | h:58; c:267 | merged into Settings › Sound |
| `#seecount`, `#stack` + chip + card | h:59,70; c:327,1555,1800 | merged into overview Needs you + Menu dot |
| `#account` | h:60; tc:266 | merged into Cluster panel foot |
| `#signin` | c:1423,1931 | merged into Settings head |
| `#under` | c:268 | removed (carrier focus act) |
| `#sim` | h:62 | kept (`panel`) |
| `#crumbs` | h:63; c:1794 | removed |
| `#rides`, `#ride`, `#pvp` | h:64; c:27,1861 | merged into Settings › Controls |
| `#worldmap`, `#compass` | h:65; c:616,925 | kept (`rest`); top right, 72/160, one N |
| `#mapbtn`, `#lensbtn`, `#lensbox` | h:65,68-69 | removed (already hidden) |
| `#mapgo` | h:65; c:697 | removed |
| `#secrets` | h:65 | kept hidden |
| `#maptip` / `#mappop` | h:66-67; c:725 | kept (`world` / `panel`) |
| `#realm-banner` | `world-realm.ts:695`; c:970 | merged into dock line + Menu status dot + People "World offline" |
| `#card`, `#flow`, `#inspector`, `#island`, `#cockpit`, `#reader`, `#talk`, `#quick`, `#yard`, `#keeper` | h:71-77,85-86 | kept (`panel`, `.glass`); `#cockpit` from the overview |
| `#inbox` | `world-keeper.ts` | merged into People › Mail |
| `#chat` | h:78-83; c:1986-2107 | merged into People › Chat + dock line |
| `#chatshow` | h:84 | removed |
| `#chip` | h:87 | kept hidden |
| `#sheet` | h:88; tc:1155 | kept (`panel`) |
| `#bottom` (`#map`, `#minimap`, `#lenses`, `#cmds`, `#line`) | h:89-98 | removed |
| `#legend` | h:93; tc:816 | merged into Cluster panel |
| `#portrait`, `#commands`/`#grid`, `#next` | h:95-99 | merged into the overview's selection view |
| `#status`, `#tip` | h:101-102 | kept (`panel`) |
| `#loader` | h:21 | kept (before play) |
| `#said` | h:110 | kept (`dock`) |
| `#calm-hint` | h:103; c:396 | merged into dock focus |
| `#pad` on foot (Lock, Attack, Jump, Run, Build) | h:104; c:59-118 | removed |
| `#pad` off foot | h:104 | kept (`dock`); Run becomes the speed toggle |
| `#pad .skills` | c:120-145 | merged into dock fight slots |
| `#pad .buildpad`, `[data-build]` | c:1659 | merged into dock build face |
| `#crosshair`, `#hitmark` | h:105-106 | kept (`world`) |
| `#focus` | h:107; c:2112-2212 | merged into `#dock` |
| `#radial` | h:108; c:2217 | kept (`panel`); + slot, piece and self radials |
| `#hottag` | h:109; c:509 | kept (`world`), fight target only |
| `#toast` | h:111; c:1359 | merged into dock line |
| `.bcard` | c:546 | kept (`world`), overview only |
| `.bl`, `.unit`, `.trunk`, `.room`, `.civ` | tc | kept (scene labels in `#labels`) |
| `.fight-hud .vitals` | `world-fight.ts:415-416,902`; c:995,1733 | merged into dock fight; Wood/Iron to Settings |
| `.fight-hud .skills` | `world-fight.ts:417` | removed |
| `.fight-boss`, `.mobbar`, `.dmg`, `.fight-tick` | `world-fight.ts:428,506`; c:1025,1068 | kept (`world`); the tagged target's `.mobbar` hides |
| `.fight-hurt`, `.fight-downed` | `world-fight.ts` | kept (full-screen effects, `world`) |
| `.build-hud` | c:1162 | merged into dock build note |
| `#welcome` | `world-welcome.ts:47`; c:1949 | kept (`panel`) |
| `#controls`, `#wardrobe` | c:1193-1335 | merged into `#settings` |

## Behaviour changes (only these)

1. **At rest,** only the map and the Menu show, plus `#welcome` on a visitor's first visit.
2. **The top bar is gone,** in the overview too.
3. **The focus box becomes the dock.** On the phone it sits beside the Menu.
4. **Inside, the dock shows the stair, a machine, the resident's Talk, or the building's Leave** (N8).
5. **Resource nodes are focus targets on both tiers,** with Chop or Mine. On the phone a tapped node is gathered
   without an Attack button.
6. **A PvP rider's focus and radial gain Attack,** only in PvP (N12).
7. **The pastime is a dock focus.**
8. **The fight face** has HP, two slots and the ultimate. A hold assigns from the class's skills, kept per class. The
   pad's skills, the vitals and the desktop key strip go.
9. **Slot inputs.** The right button fires slot 1 and Q slot 2, and the Mage's spell cycling goes. Q, X and LB fire on
   release under 500 ms; a hold opens the slot radial (N6, N11).
10. **A slot aims at the engaged target** (the locked mob or the focused PvP rider) before `ultAim` or ahead. A world
    long-press on a target fires slot 1 at it.
11. **The self radial** (your rider, F, D-pad left) holds Ready/Sheathe and, in a build zone, Build. D-pad left no
    longer toggles build directly.
12. **Build is a dock face:** Place, Rotate, Done. The pieces and Take down sit on Place's long-press. The build pad,
    `.build-hud` and the phone menu's Build row go.
13. **The PvP flag alone no longer shows the fight face** (Q17).
14. **Lines.** Hints, chat and mail show in the dock line for 4 s, with 3 s of focus between lines. The lines between
    collapse into "n new", and a hint goes first. Never over a fight or build.
15. **Chat lives in People › Chat.** Show and hide are gone.
16. **The Menu has 4 entries.** People shows for everyone, and reads "World offline" while the realm is down. `?` opens
    Settings › Controls.
17. **The ride, PvP and Sound move into Settings.** 1, 2, 3 and P still work.
18. **The phone's pad is hidden on foot,** and Go (`#mapgo`) is gone.
19. **Full speed is the default on every ride.** On foot the rider walks inside and within `ARRIVE_WALK` of the end,
    with no Run button and no double tap. On the bike and the ship, the speed button, Shift and L3 toggle down to
    cruise (N1).
20. **Auto-jump on every walk.** The phone has no manual jump on foot (N4).
21. **`#hottag` names only a fight target,** and `.bcard`s never draw in explore.
22. **The phone map moves to the top right,** 72 px, with N only.
23. **The overview is the side panel,** holding the Cockpit link, the Cluster panel and the selection view.
    - The desktop deck and the crumbs are gone.
    - Closing the panel returns to explore.
24. **Two Menu dots:** unread (`--danger`) and realm down (`--warning`).

## Phases: review slices inside one PR

Every commit runs `bun run build` (bun 1.4.x for dist identity), typecheck and `bun test`; `go test ./...` runs in full
before merge. Gate rows flip in the commit that changes them.

| # | Slice | Flips |
|---|---|---|
| 1 | The proof on main: `dev/hud-gate.mjs` (new) records today's counts and overlaps; rows that will change carry `changes: N` (the behaviour changes' numbers). Run it twice on main, then break one placement on purpose to see a row fail | none |
| 2 | The family: tokens, `.glass`, `.glass-btn`, `data-hud` on every surface (the "untagged element" row turns green) | none |
| 3 | Speed and jump: `sprints`, the deletions, the speed toggle, `ledgeAhead` | 19, 20 |
| 4 | The dock: `world-dock.ts`, `world-kit.ts`, the faces, the line pacing, the new focus sources (nodes, PvP riders, pastime, inside), `fireSlot` and the holds, the self radial with its key and pad row, the tag rule; `#toast`, `#calm-hint`, the vitals, `.build-hud` and the build pad go | 3–14, 21 |
| 5 | The Menu and the panels: `#menu` with its dots, 4 `GLOBALS`, People, Settings; `#menubtn` and `#helpbtn` go | 15–17, 24 |
| 6 | The pad on foot, `#mapgo`, `#rides`, the map corner | 1, 18, 22 |
| 7 | The overview: `#top` and `#bottom` retired; `#overview` with its selection view and Close-to-explore; `#cluster` | 2, 23 |
| 8 | The visitor page, accessibility, the README controls table, the gate edits | — |

**Gates to update** (labelled edits, each with its reason):

| Gate | Row (`dev/…`) | Becomes |
|---|---|---|
| input | `clearOf`, `placed` (`input-gate.mjs:296-325`) | `#dock` against the `rest` roots |
| input | `openCockpit` (`:365-367,606`) | Menu → Overview → Cockpit |
| input | the Menu rows (`:1751-1766`) | 4 entries |
| input | the visitor Menu (`:1774-1790`) | `["Map","People","Settings"]` |
| input | the phone Menu (`:1997-2013`) | `#menu` |
| input | pad Lock (`:279,1024`) | desktop T on foot |
| input | the box rows (`:1813,1830,1886`) | `#dock .primary`, `.more` |
| world | `#top > *` (`world-gate.mjs:502`); `#mode` (`:533,558`) | `[data-hud]`; Tab |
| onfoot | `#mode` (`onfoot-gate.mjs:271-275,342-343,483`); `#helpbtn` (`:365-374`) | Tab; `?` |
| aim | `#mode` (`aim-gate.mjs:99`); `#hottag` on a building (`:247-249`); `#bottom`, `#rides` (`:20`) | Tab; `#dock .name`; `#dock` |
| calm | the cockpit path (`calm-gate.mjs:163-165`); the quick row's Sound (`:184,300`) | Menu → Overview → Cockpit; Settings › Sound |
| cockpit | `#deck` → Cockpit (`cockpit-gate.mjs:230-241`) | Menu → Overview → Cockpit |
| public | `#hottag` health (`public-gate.mjs:165`); `#ride` (`:184-186`) | on a mob; Settings › Ride |

**Already failing on main** (plan/23 build notes):
- world-gate: Rimfall, Underside;
- onfoot and aim: the phone's `#mode` (that row goes with `#mode`);
- public: `/api/land`.

**Checks (lean-checks):**
- **Every commit:** unit tests and dist identity.
- **Once before review:** a full manual GPU round of world, onfoot, aim, calm, cockpit, public, input and hud.

## Tests

Each new test has a mutation and a positive control.

| Test | Covers |
|---|---|
| `world-dock.test.ts` (new) | `dockOf`: rest when covered or in the overview, then build > fight > line > focus; fight on foot only; Ready counts as a fight. Pacing: 4 s per line; the 3 s gap; arrivals in the gap collapse to "n new"; a hint before chat; fight and build hold lines; a timeout adds to unread |
| `world-kit.test.ts` (new) | `poolOf` per class; defaults; `assign` swaps; storage per class, junk ignored, a throwing store survives; a hold's release assigns and never fires; `fireSlot`'s aim order: given, engaged, then `ultAim`/ahead |
| `world-ride-step.test.ts` (new) | `sprints` on foot (inside, `toEnd` around `ARRIVE_WALK`, Shift ignored) and on a ride (`slow`); `ledgeAhead`'s rise bounds |
| `world-actions.test.ts` | the 4 entries; the visitor's 3 and People with the realm down; node Gather labels; rider Attack only in PvP |
| `world-focus.test.ts`, `world-focus-dom.test.ts` | nodes focusable; the pastime and inside rules; no `place()`, no `--pad-h` |
| `world-bindings.test.ts`, `world-bindings-migrate.test.ts` | `self` key F (allowed as its own default); the pad row `build` → `self` through `MOVED_PAD` |
| `world-router.test.ts` | Q, X and LB: a tap under 500 ms fires on release; a hold opens the slot radial; the right button is unchanged |
| `world-chat.test.ts` | no fade or show; unread while the tab is hidden |
| `world-modes.test.ts` | the menu, the toast and `PINNED_KEY` go; `hintQueue` moves |
| `world-input.test.ts` | `padShows` false on foot on a phone |
| `world-deck.test.ts` | `deckOpen` retired; `deckShows` kept for the overview's commands |

**`dev/hud-gate.mjs` (new)** runs on `dev/stack-realm.sh`.

**Sizes:** 320×568, 390×844 and 430×932 with touch; 1440×900 with a mouse.

**States:**
- rest; near a building; near a tree; inside by a stair; inside a hall with a resident;
- the self radial; build mode;
- a fight (a stub mob's hit, `HURT_MS`); Ready with no mob; a slot's radial;
- lines: one at rest, one over a focus, two in the gap, one during a fight;
- the Menu, People (realm up and down), Settings;
- the overview, with and without a selection, then closed;
- the visitor at rest; the bike.

| Assertion | Value |
|---|---|
| visible `[data-hud="rest"]` | **2** in every explore state (`#worldmap`, `#menu`); 1 in the overview (no map) |
| at rest, every other role | 0 visible (a visitor's first visit: 1 `panel`, `#welcome`) |
| near, inside, build, line | `dock` 1, `panel` 0 |
| fight | `dock` 1; `world` includes `#hottag` and the target's `.mobbar` is hidden; `panel` 0 |
| untagged | fails on any visible `position: fixed` or `absolute` element over `#c` with no `data-hud` |
| overlaps between visible `rest`, `dock` and `panel` boxes | none, except a panel over the dock and the radial over everything |
| on screen | every box on screen; the dock's foot clears the safe area |
| sizes | buttons in `[data-hud]` ≥ 44×44; the phone's fight and build buttons 56×56; the build face ≤ 184 px wide at 320 |
| radii | containers 16, buttons 12 |
| one label | the focused target's name appears in at most one of `#dock .name`, `#hottag`, a visible `.bcard` |
| overview closed | the view is explore; `#overview`, `#cluster` hidden |
| visitor | no Cockpit link in the DOM; People visible with the realm down |

## Hands-on QA (`dev/stack-realm.sh`; phone emulator 390×844 touch, then 320×568; desktop 1440×900)

| # | Phone: do | Expect |
|---|---|---|
| 1 | Rest; tap-walk a long route over ledges and a terrace; tap at a low wall; go inside | Only the map and the Menu. It runs, jumps by itself (double on the terrace), and walks on arrival. It hops the wall. Inside, it walks |
| 2 | Tap a hall; long-press Look and slide to Enter; walk away | The dock shows Look and ⋯; you go in with no stray click; the dock leaves |
| 3 | Inside: stand on a stair; step off; tap a book | The stair's act; then "Resident · Talk" or "Hall · Leave"; the book reads at once |
| 4 | Tap a tree, then tap again | "Oak · Chop"; it is gathered |
| 5 | A wolf hunts you. Tap it twice; tap a slot; long-press a slot and slide; long-press the wolf; reload; change class | HP, 2 slots and the ultimate. You attack. The slot casts at the wolf. They swap and nothing fires. Slot 1 fires at the wolf. The slots are kept, and each class has its own pair |
| 6 | Tap your own rider: Ready; cast a ward; Sheathe | The fight face with no mob; it casts; the face leaves |
| 7 | In the meadow ring, tap your rider: Build. Place; Rotate; long-press Place: floor, then Take down; Done | Build shows only there. The 3-button face fits at 320. A refusal shows in its note |
| 8 | Chat: one line, then three quickly, then one in a fight; a hint pending | About 4 s, then 3 s of focus, then "3 new". The hint shows first. In the fight, the line waits |
| 9 | PvP on with a second rider; tap them | Attack is the primary; it hits. With PvP off, there is no Attack |
| 10 | Menu: People, then send a line. Stop the realm | The Menu shows its status dot, and People reads "World offline" |
| 11 | Menu → Overview, then Close; again, then Esc | Each returns to explore with the map corner back |
| 12 | Settings: bike, then foot; PvP; Sound | The bike runs full speed, and its speed button cruises. On foot the pad goes |
| 13 | 320 px, every state above | Names ellipse; buttons keep their size; nothing overlaps the Menu |
| 14 | A guest visitor, then the realm down | The welcome card; Map, People, Settings; Sign in in Settings; People reads "World offline" |

| # | Desktop: do | Expect |
|---|---|---|
| 1 | Hover the Library; hold E; go inside; Esc | "Look `E`", no tag; the radial; inside the dock shows Leave or Talk; Esc leaves |
| 2 | Enter, then Esc | People › Chat with the input focused; the keys come back |
| 3 | Tab; `/`; Fly there; pick a building; back; Cockpit ›; Cluster ›; Esc twice | The panel, its selection view, the Cockpit, the cluster; back to explore |
| 4 | Fight: right button, a quick Q, G; hold Q; hold the slot 1 button | Slots 1 and 2 at the target, and the ultimate. The slot 2 radial; the slot 1 radial |
| 5 | F: Ready, then Build; B, R, X, Q | The self radial; the build face with its caps |
| 6 | Gamepad: tap and hold X and LB; D-pad left; T on foot | Fire versus radial; the self radial; the lock |
| 7 | `?`; keyboard only with VoiceOver; reduced motion and transparency | Settings › Controls; named items, focus back on Menu, a line spoken once; no rise or fade; solid panels |

## Risks

- **No manual jump on the phone** (N4, a recorded trade-off). A rider stuck in a hole can only walk out where the probe
  sees a ledge within reach.
- **Auto-jump on plain walks** may hop at walls. The probe jumps only for a rise within reach, and QA walks into walls.
- **Release-to-fire on Q, X and LB** adds up to 500 ms of delay to a held press and none to a tap. Fight feel is
  checked at QA.
- **Full speed everywhere** brings collisions faster, and C always slides. Cruise on a ride is a press away.
- **The fight face flickers** if `inFight` toggles at the hunt edge. It reads `hurtAt`; add `FIGHT_HOLD_MS` if needed.
- **A busy chat** shows as "n new", not line by line. The lines stay in People.
- **Slots replace the Mage's cycle.** A Mage's stored slots start as today's first two spells.
- **Nodes as focus targets** can steal a tap meant for a mob behind a tree. `nearestHit` keeps the nearest, as today.
- **A big PR.** One slice per commit, each with its dist.
- **Plan/22 is in flight.** P3's UI rebases onto `#menu` and People › Mail; P4's mic needs a place.
- **Gates already failing on main** may hide a regression. The hud-gate does not depend on them.

## Not doing

- The bike's and ship's controls beyond the default speed and the toggle's meaning.
- The mail server (#250), voice, and the Party tab (P5).
- New themes, the 3D world, in-world signs, a new radial layout, or a scrim.
- Rebinding Esc, M, Enter, `?` or Tab.
- The shooter zone, a pad hover, a focus-cycle key, or automatic facing.
- The town page.

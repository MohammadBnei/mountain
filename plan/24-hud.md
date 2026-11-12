# 24 · One HUD: the dock, the Menu and the panels

Status: rev 2, owner answers 2026-11-12.

Sources:
- the owner interview and the mockup review of 2026-11-12 (memory `hud-design-intent`, `hud-coherence`);
- the approved mockups: `.wt/hud-mock/DESIGN.md` and `out/*.png` (P1–P6 phone, D1–D4 desktop);
- the inventory of every current HUD element: `.wt/hud-shots/INVENTORY.md`;
- the owner's answers to Q1–Q18 (rev 2).

What it changes in other plans:
- **plan/23.**
  - `#focus` becomes the dock's focus face (§3).
  - The Menu radial drops from 8 `GLOBALS` to 4 (§4).
  - The phone gets a dock inside buildings, which plan/23's "Not doing" excluded.
  - The desktop deck leaves explore (§7).
- **plan/22.**
  - The P2 chat log moves into People › Chat.
  - P3 mail and P5 party UI plug into People's tabs.
  - P4's mic "near Jump" has no Jump to sit near (§12).

## Context

Every claim was checked on ukubi-game main at `132ffc0` (plan/23, #248). Paths are under `cmd/game/web` (`.html`,
`.css`) or `cmd/game/websrc` (`.ts`).

### What is on screen

The inventory counted 11 floating pieces at rest on the phone, 13 on the desktop, 24 with the phone's Menu open and 36
in a desktop overview with a selection. It found five panel styles and four radii on like things:

| Fact | Where |
|---|---|
| Glass is a hand-kept list in three places; `#focus`, `#toast`, `#deck`, `#menubtn`, `#chat` are on none | `theme.css:7-39,42,48,86` |
| Rem-based glass outside the tokens | `.fight-hud`, `world.css:995` |
| Focus buttons r8 in an r12 card; pad buttons r16; radial r12; chat chips r8 | `world.css:2145,2188,94` |
| One building named three times on a desktop hover: `#hottag`, a `.bcard`, the box | `world-targets.ts:431-439` |
| The phone menu unfolds `#top` into 12 rows, one of them "Menu", which opens a second menu | `world.html:39` |
| Phone hints: `#toast`, 7 s; desktop: the static help line `#line` | `world-modes.ts:129,151-160`; `town-phone.ts:201` |
| Chat: 4 chips, 5 lines fading after 10 s, a form, `#chatshow`; polled every 500 ms | `world.html:78-84`; `world-chat.ts:6-10`; `world.ts:2358` |
| The pad shows exploring outside with no panel; at rest on foot: Build, Run, Jump, Skill 2 | `world.html:104`; `world-input.ts:263` |
| The desktop deck `#bottom` opens on a selection, inside, talk or `?`; cells act, enter, talk, leave, up, weapon, pvp, controls, plus Next | `world.html:89-100`; `world-deck.ts:19-42` |

### Movement

The rider walks unless asked to run:
- Shift, or the sprint key (`world-view.ts:456,1195-1196`);
- the pad's Run toggle (`world-input.ts:235`, `world-view.ts:523`);
- the gamepad's L3 toggle (`:1684`);
- a double tap on the ground (`:1923-1927`);
- an engage's quick second tap (`world.ts:1215,1271`).

It is read in two places: `drive` (`world-view.ts:1436`) and `ridePose` (`:1541`). The speeds are:
- foot: walk 4, run 7 (`world-ride.ts:45-46`);
- bike: 16 and 26 (`:59-60`);
- ship: `sprint()` is its boost (`world-view.ts:1398`).

Jumps:
- **Auto-jump** happens only on a route: `advance` reports a ledge (`m.jump`) and `routeHold` holds the jump until
  landing (`world-view.ts:1466-1469,1544`).
- **A plain walk** (`walk` with no route: inside, the engage's `walkTo`) never jumps.
- **A drag on the world** turns the camera's yaw and pitch (`:1240`).

### Fight and build

- **Kits.** Each class has two skills and an ultimate (`world-skills.ts:9-13`).
  - The Mage's two are both `spell`. Skill 1 casts the selected spell, and Skill 2 cycles it through `MAGE_SPELLS`
    (frost_bolt, nova, ward; `world-fight.ts:179`, `world.ts:1438-1455`).
  - The attack is the class's own (`world-skills.ts:23-24`), on a click or a tapped mob.
- **Build mode** (`world-build.ts`):
  - Q cycles `ORDER` (wall, floor, stair, sapling; `:40`); the pad's Attack places; R turns; X takes down.
  - The words show on `.build-hud` (`world.css:1162`, `world-build.ts:210`).
  - It starts with B, the phone menu's Build row, or the pad's Build (`world-modes.ts:186-216`), only where B works
    (`canBuild`).
  - Build zones are the ring 30–45 out from Wolf Meadow, or the rider's own yard lot (`world-build.ts:193`).

### The focus box, the Menu, the overview, the visitor

| Fact | Where |
|---|---|
| The box sits in `#bottom` on the desktop, fixed over the pad on the phone (`--pad-h`), hidden inside on the phone | `world-focus.ts:163-174`; `world.css:2176-2212,2200` |
| `GLOBALS` holds 8 entries; opened by `#deck`, H or Start; an ellipse rx 140 / ry 124 | `world-actions.ts:117-139`; `world.ts:2416-2421`; `world-radial.ts:14-21,51-55` |
| Enter opens the chat outside | `world.ts:2690` |
| `#top` holds health, world, age, Cluster and its 4 groups, Live, Explore, search, Menu, ?, Sound, To see, account; crumbs and stack on the right | `world.html:41-63,70` |
| The visitor page removes `chip`, `island`, `sim`, `account`, `next`, `status`; adds `#signin`, `#sound`, `#under`, `#welcome`; explore only | `world-public.ts:121`; `world-welcome.ts:47`; `world-view.ts:678,803` |
| Parked P3: #250 (server, open); `realm/p3-mail-ui` (`2d46f61`) adds `world-mail.ts` (`makeMailModel`, `#mail` Inbox and Write), the rider radial's Message, the mail unread on `#deck` | ukubi-game |

## Decisions (owner, 2026-11-12)

| # | Decision | Shape |
|---|---|---|
| 1 | At rest, both tiers: only the map corner (top right) and one Menu button (bottom left) | §1 |
| 2 | One bottom dock, only in context: **focus** (name, primary, ⋯); **fight** (thin HP row, 2 skill slots, the ultimate with an energy ring; tap a slot fires; long-press opens a radial of the class's skills and a slide assigns, swapping, never firing; kept per class; tap the mob to attack); **build** (Place, Turn, Take down, Done; entered from the yard or Settings); **line** (chat, mail or a one-time hint slides over the dock for about 4 s, a tap opens its panel, then the focus returns; a fight is never covered, lines wait) | §3 |
| 3 | The Menu radial: Map, People (Chat, Mail, Party), Overview, Settings (controls, wardrobe, sound) | §4–6 |
| 4 | The overview is a side panel: search, Needs you, a Cockpit link (owner only), "Cluster and districts ›". The desktop deck lives only there; explore on the desktop uses the dock as the phone does | §7 |
| 5 | One family: `--radius-lg` containers, `--radius-md` buttons, 44 and 56 px buttons, the theme's glass, Jost, 16 px edges, 8 px gaps, tokens only | §2 |
| 6 | Removed or merged: chat chips and log, toast and tutorial bubble, every always-on pad button, the help line, duplicate labels, crumbs and "PvP off" while exploring, the top bar | fate table |
| 7 | Full speed by default on every ride. On foot: no Run button, no double-tap run; walk only inside and when arriving at an auto-route target. The bike's and ship's speed button stays; their other controls are unchanged | §8 |
| 8 | Auto-jump on every walk, tap or route; a swipe up on the world is the manual jump; no Jump button | §8 |
| 9 | One label per thing, one dock, nothing floats on its own | §3 |

### Defaults the owner did not object to

| Q | Default, owner did not object |
|---|---|
| Q4 | Shift and L3 do nothing on foot; C while moving slides (`world-moves.ts:1`) |
| Q7 | Wood and Iron: one line in Settings › Wardrobe's foot |
| Q8 | The desktop fight face: 44 px, key caps right button, Q, G |
| Q9 | One-time hints in the dock line on both tiers, in short words; the full key list in Settings › Controls |
| Q10 | The phone overview: the same panel, full width less 2 × 16, at the top, capped at 50 vh |
| Q11 | Inside, the dock shows parts (stairs, machines); a book tap still reads at once (`world-view.ts:1901-1903`) |
| Q12 | A guest's Sign in in Settings' head; `#welcome` the one third piece on a first visit; the realm-down banner becomes a dock line plus a People note |
| Q14 | H stays the Menu key; the cap shows the bound key |
| Q15 | `.fight-boss` is the boss's one label; the world tag skips the boss |
| Q16 | The legend and the account in the Cluster panel; Underside the carrier's focus act (`world-actions.ts:104`); Secrets and Fleet in the cockpit's quick row |
| Q17 | The fight face needs `inFight` or a hit within `HURT_MS`; the PvP flag alone does not show it |
| Q18 | `ARRIVE_WALK` = 3 units, a knob set by feel |

### New open questions (rev 2)

The plan builds each default; each is a one-line swap.

| N | Question | Default |
|---|---|---|
| N1 | What the bike's and ship's speed button does now that full speed is the default | it toggles down to today's cruise (`speed`); Shift and L3 hold it there |
| N2 | Build's four buttons hold no "Next piece" (wall, floor, stair, sapling) | a long-press on Place opens a radial of `ORDER`; desktop Q still cycles |
| N3 | "From the yard": `world-yard.ts` is the visitor's builder's-yard quest; building happens in build zones | where `canBuild` holds (the meadow ring, your yard lot), the dock offers "Build zone · Build" |
| N4 | A swipe up against the camera's pitch drag (`world-view.ts:1240`) | an upward flick under `SWIPE_MS` 220 ms, over `SWIPE_PX` 48, mostly vertical, jumps; slower drags look |
| N5 | Knight, Barbarian and Rogue have exactly two skills, so their radial only swaps; the Mage has three spells | the pool is the class's skills only, not the attack or the ultimate |
| N6 | Desktop Q cycles the Mage's spell today | the right button fires slot 1, Q slot 2, every class |
| N7 | A line during build | it waits, as in a fight |
| N8 | Inside on the desktop, the deck's Talk and Leave go | Talk is the resident's focus act; Leave is Esc or the door |

## Shape

### 1. Zones (both tiers)

| Zone | Phone (390×844) | Desktop (1440×900) | Shown |
|---|---|---|---|
| Map corner | top right, 72 px circle, inset 16 + safe area | top right, 160 px circle, inset 16 | always, except in the overview and under the big map |
| Menu | bottom left, 44×44, inset 16, foot on the safe area + 8 | bottom left, 44 px "Menu" + key cap | always, the same spot in every state |
| Dock | bottom, from Menu's right edge + 8 to the right inset | bottom centre, max 560 | §3 states only |
| World tag | over the fight target | the same | a fight target; buildings in the overview |
| Panels | full width less 2 × 16, above the Menu | left, above the Menu: People 380, Settings 640, Overview 360 | on demand |

The phone map moves from the top left at 64 px (`world.css:1560-1567`) to the top right at 72 px. The desktop map goes
from 168 px (`:1739`, `--where`) to 160 px, at `top: var(--hud-inset)`.

### 2. The component family

**New custom properties** go in `tokens.css :root`, once:

```css
--hud-inset: var(--space-4);   /* screen edge to any HUD element */
--hud-gap: var(--space-2);     /* between items, and the dock's padding */
--hud-row: var(--space-3);     /* between rows in a panel */
--btn: 44px;                   /* tier 1: Menu, dock actions, ⋯, radial items, tabs, panel controls */
--btn-fight: 56px;             /* tier 2: the phone's fight and build dock buttons */
--map-corner: 72px;            /* 160px at min-width 1200 */
--dock-max: 560px;             /* the desktop dock's width cap */
```

**Classes:**
- **`.glass`** replaces the three hand-kept `:is()` lists (`theme.css:7-39`, `:48`, `:86`), with the same rules. It is
  the container: glass, starfield, `--glass-blur`, a 1 px `--edge-glow`, `--shadow-glass` and `--radius-lg`.
- **`.glass-btn`** is a free-standing button: glass without the stars, `--radius-md`, `min-height: var(--btn)`.
- **Ghost** is a button in glass: 55% `--surface-raised` plus 1 px `--edge-strong`.
- **Primary** is an `--accent` fill, an `--accent-strong` edge and `--on-accent` text.
- **On or pressed** is today's `theme.css:131` rule.

**Exceptions:**
- the map: a 50% circle with a 2 px `--edge-strong` ring, and one N on the rim instead of N/E/S/W (`world.css:925-956`);
- key caps: `--radius-xs`, overline type.

**Type and colour:**
- title-3: the dock name and panel titles;
- label: buttons and tags;
- body-sm: chat and alerts;
- overline: hints, heads and caps;
- metric: cluster numbers;
- `--game-hot` for the target, `--game-harm` for mob HP, `--game-hp` for own HP, `--game-energy` for the ring;
- `--danger` for the one unread dot, replacing two dot styles (`world.css:349`).

**`data-hud`** marks every HUD root. The layout gate counts the visible ones.

### 3. The dock: one element, five states

**The element** replaces `#focus` (`world.html:107`):

```html
<section id="dock" class="glass" data-hud aria-label="Dock" hidden>
  <div class="face focus"></div><div class="face fight"></div><div class="face build"></div><div class="face line"></div>
</section>
```

**Ownership.**
- `world-dock.ts` (new) holds the state rule, the line queue and the fight and build faces.
- `makeBox` (`world-focus.ts:140`) still builds the focus face, with `tapHold` (name, primary, ⋯, note).
- **Deleted:** its `place()` (`:163-170`), the `--pad-h` observer (`:172-174`) and the phone rules
  (`world.css:2176-2212`).

**The state rule (pure, tested):**

```ts
export type DockState = 'rest' | 'focus' | 'fight' | 'build' | 'line';
export interface DockIn {
  view: 'explore' | 'inside' | 'action' | 'flying';
  covered: boolean; // plan/23 covered(): a panel, talk, the sheet
  build: boolean;   // build.on()
  fight: boolean;   // inFight || now - hurtAt < HURT_MS (Q17)
  foot: boolean;    // fight face on foot; the bike and the ship keep their pad
  line: boolean;    // a line is up
  focus: boolean;
}
export const dockOf = (d: DockIn): DockState =>
  d.covered || d.view === 'action' || d.view === 'flying' ? 'rest'
  : d.build ? 'build'                 // lockToggle refuses while building (world.ts:1155)
  : d.fight && d.foot ? 'fight'       // never covered: lines wait
  : d.line ? 'line'                   // over focus or rest, LINE_MS
  : d.focus ? 'focus' : 'rest';
```

**The faces:**

| State | Contents | Phone | Desktop |
|---|---|---|---|
| focus | name over "Hold for more" · primary (accent) · ⋯ | primary is a word (Look, Read, Talk, Build) | "Look `E`" and ⋯ "More (hold E)" (`world-focus.ts:256-258`) |
| fight | HP row ("HP", 8 px `--game-hp` bar, number) over slot 1 · slot 2 · Ultimate (a conic ring of `--game-energy` to `energy / ENERGY_MAX`) | 56 px | 44 px, caps: right button, Q, G |
| build | Place (primary) · Turn · Take down · Done, plus a note row for the piece and the refusal (`world-build.ts:210`) | 56 px | 44 px, caps: click, R, X, B |
| line | glyph · source ("Chat · Sami", "Mail", "Hint") · one ellipsed line · chevron · timer hairline | the same | the same |

**The skill slots** live in `world-kit.ts` (new, pure plus storage):
- **The pool.** `poolOf(cls)` is `MAGE_SPELLS` for the Mage, else `[skill1, skill2]` (`world-skills.ts:9-20`). N5:
  never the attack nor the ultimate.
- **Defaults.** `slotsOf(cls)` is the stored pair from `ukubi-slots` (JSON `{cls: [a, b]}`, every access in a `try` as
  `world-modes.ts:46-73`). Without one, it is the pool's first two.
- **`assign(slots, i, id)`** puts `id` in slot `i`. If the other slot held it, the two swap.
- **A tap** fires: `run(id, chest, ultAim(false) ?? ahead(r))` (`world.ts:1461`). This replaces `skill1`, `skill2` and
  the Mage's `cycleSpell` path (`:1438-1455`).
- **A long-press** (`tapHold`, `ACT_HOLD_MS`) opens `radial.open('slot', pool…)` above the slot. Release on an item
  assigns, and nothing fires. The keyboard opens it with Shift+F10 or the context-menu key on a focused slot.
- **Desktop** (N6): the right button fires slot 1 and Q fires slot 2 (`world-bindings.ts`, labels updated).

**The build face** runs `build.place()` (today's pad Shoot in build mode), `build.turn()`, `build.remove()` and
`build.toggle()`.
- Place's long-press opens a radial of `ORDER` (N2).
- `.build-hud` goes, and its words are the face's note.

**The line queue.**
- `hintQueue` (`world-modes.ts:77-98`) moves here and gains live chat lines (`world-chat.ts` `hear`, live only) and mail
  news (P3).
- One line at a time; a newer one replaces it; a hint waits behind chat or mail.
- `LINE_MS` = 4000. While a fight or build holds it back it waits. Unseen at timeout, it adds to the unread count.
- A tap opens People › Chat, People › Mail on that item, or dismisses a hint. The line is one `<button>`.

**Motion.**
- The dock rises 8 px and fades over `--dur-panel`. Faces cross-fade over `--dur-press`, and the box never moves.
- Reduced motion: an instant swap.
- The polite region speaks a focus and a line (`world-focus.ts:271-277`). The fight face is silent.

The fate table lists what the dock replaces.

**One label per thing.**
- `#hottag` shows only for a fight target, with the name, an HP bar and a `--game-hot` caret
  (`world-targets.ts:431`).
- `.bcard`s do not draw in explore (`world-cards.ts` `hidden()`).
- The tagged target's `.mobbar` hides while its tag shows.

### 4. The Menu: 4 entries through `GLOBALS`

**The button.**
- `#deck` leaves `#top` as `<button id="menu" class="glass-btn" data-hud aria-haspopup="menu">`, bottom left.
- It keeps its handler (`world-chrome.ts:172-175`) and `menuOpener` (`world-radial.ts:48`).
- One `--danger` dot shows for letters, mail and missed lines. The count is in `aria-label`.
- While the Menu or a panel is open, it is pressed and reads "Close (Esc)".

**`GLOBALS`** (`world-actions.ts:117-139`) becomes:

| id | label | when | run |
|---|---|---|---|
| map | Map | always | `minimap.toggle()` |
| people | People | owner page, or realm up | `people.open(lastTab)` |
| overview | Overview | owner page | `view.flip()` |
| settings | Settings | always | `settings.open()` |

- **Gone from the Menu:** Cockpit (now in the overview), Mail and Chat, Controls and Wardrobe, and Talk (a focus act).
- **`ActionDeps`** changes to match.
- **The visitor's Menu:** Map, People (realm up) and Settings.

**The layout.** The Menu radial uses a new `arc(n, r = 130)`, a half circle above the bottom centre, beside `layout`
(`world-radial.ts:51`).
- Items are 44 px icon buttons with labels under them (P5).
- A 40% `--surface-sunken` scrim covers the world.
- The focus, slot and piece radials keep the ellipse.

**Keys** stay: H or Start, Esc, M and Tab. `?` (`world-router.ts:244-245`) opens Settings › Controls.

### 5. People: one panel, three tabs

- **The element.** `<aside id="people" class="glass" data-hud role="dialog" aria-label="People">`. It has a head with
  Close, and three 44 px tabs: Chat, Mail · n and Party.
  - It is non-modal, a chrome in `chromes` (`world.ts:1983-1986`).
  - Its fields are the router's typing context.
- **Chat.** `bindChat` (`world-chat.ts:115-204`) rebinds to the tab.
  - The log is the last `KEEP` lines, and the form always shows.
  - **Gone:** the chips, Hide and Show, `#chatshow`, `SHOWN_KEY`, `FADE_MS`, `SHOW` and `fading`.
  - Unread counts lines that came while the tab was not shown.
  - **Desktop Enter** opens it with the input focused (`world.ts:2690`). A submit or Esc closes the panel, and focus
    goes back to the canvas.
- **Mail (P3 hook).**
  - Until P3, the owner's tab is the letters inbox (`world-keeper.ts` `inbox`), moved from `#inbox`. The visitor's tab
    is hidden.
  - P3's `world-mail.ts` model and its Inbox and Write views become the tab body. The rider radial's Message opens
    Write.
- **Party (P5 hook).** An empty state, "Parties come later".
- **The realm down.** Chat and Party say "The shared world is out of reach".

### 6. Settings: one dialog, three tabs

- **One dialog.** `#controls` and `#wardrobe` (`world.css:1193-1335`) merge into `<dialog id="settings" class="glass"
  data-hud>`, with tabs Controls, Wardrobe and Sound.
  - The modal context's predicate (`world.ts:1547,2669`) becomes `settings.open()`.
- **Controls tab:**
  - **Ride:** a segmented choice of the rides open to this rider (`world-view.ts:674-675`), with `#ride`'s handler.
    1, 2 and 3 stay keys.
  - **PvP:** from `#pvp` (`world.html:64`) and the deck cell.
  - **Build:** a toggle where `canBuild` holds, which opens the build dock. It replaces the phone menu's row
    (`world-modes.ts:186-212`).
  - Then today's camera, keys and pad tables.
- **Wardrobe tab.** Today's body. The rider preview still turns (`world.ts:1007-1010`). Wood and Iron go in its foot
  (Q7).
- **Sound tab.** The sound toggle (`#sound`; the cockpit quick row's Sound, `world.ts:2049`). P4's voice row lands here.
- **Head.** A guest's Sign in (Q12).
- **Gone:** "Buttons: always / in fights" (`world-modes.ts:187-199`, `PINNED_KEY`).

### 7. The overview, and the deck's end

**`#top`** (`world.html:38-61`) is retired on both views; the fate table says where each piece goes.

**`#overview`** is a new aside, 360 wide, top left, sized to its content. It holds:
- a head;
- Find;
- Needs you;
- "Cockpit ›" (owner only; absent from the visitor's DOM);
- "Cluster and districts ›".

**`#cluster`** has a back row, then:
- the 4 metric groups (`world.html:45-50`);
- the health, the world, the age and Live;
- the villages list (`villages` in `world.ts`), each row flying there (`ui.go`);
- the legend (Q16).

**The deck becomes the overview's selection view** (decision 4). A pick in the overview replaces Needs you with:
- a back row;
- the portrait (`#portrait`);
- Next as the primary (Enter);
- the commands as 44 px rows (`#grid` cells: act, enter, up).

`#bottom` (`world.html:89-100`) is retired, along with `deckOpen`, `body[data-deck]` and its CSS. The deck's other cells
go:
- talk: the focus act;
- leave: Esc or the door (N8);
- weapon: the slots;
- pvp, controls: Settings.

**Gone:** the crumbs (`world.html:63`), the "1 to see" chip, the floating alert card, the corner map, and `#line`.
`.bcard`s stay in the overview, restyled.

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

**The code changes:**
- `world-view.ts:1541` reads `sprints(...)`, and so does `drive`'s speed (`:1436`). The ship's boost (`:1398`) reads it
  too.
  - `toEnd` is the distance to `walk`, else to the route's end.
- **Deleted:**
  - `walkRun` (`:440,1503,1927,2105`);
  - the double tap (`:1922-1924`);
  - the `run` parameter of `walkTo` and `followTo` (`:384,388`);
  - `engageRun` (`world.ts:1176,1215,1271`).
- **The pad's Run button** hides on foot. On the bike and the ship it becomes the speed toggle: `pad.run` turns into
  `pad.slow`, which is pressed while cruising (N1).
- **The realm.** The wire's `sprint` (`world-realm-wire.ts:31`) carries the rule's result.
- **Hint words** drop "double tap to run" and "hold Sprint" (`world-chrome.ts:90,96-99`). Settings names the key "Slow
  (bike, ship)" (`world-bindings.ts:46,69`).

**Auto-jump on every walk.**
- `routeHold`'s jump (`world-view.ts:1466-1469`) extends to a plain `walk`: a `ledgeAhead(pose, dir)` probe, the land
  height one step ahead (`p.land`, as `stand`).
- It jumps when the rise is over the step-up and under the jump's reach.
- Routes keep their own `m.jump`.

**The swipe-up jump.**
- The router's gesture classifier (`world-router.ts` around `:411-449`) gets a `swipe-up` gesture (N4): shorter than
  `SWIPE_MS`, past `SWIPE_PX`, with |dy| > 2|dx| going up.
- It goes to the view's `jump()`. Nothing else swipes.
- Space stays the desktop jump.

### 9. Phone and desktop specifics

**Phone (`TOUCH_PHONE`, `world-modes.ts:14`):**
- **The pad is hidden on foot.** `padShows` (`world-input.ts:263`) gains `&& a.ride !== 'foot'`.
  - Off foot it shows as today: Advance, Jump or climb, Attack, and the speed toggle.
  - Lock, Attack, Jump, Run, Build and the skills are gone on foot, and so is the centre dot (`world.css:155-165`).
- **`#pad .buildpad`** and the `[data-build]` captions go (the build dock).
- **`#mapgo`** goes (`world.html:65`, `world.css:697`). The big map's pop already has Go to it (`world.html:67`).
- **`#rides`** goes (§6).
- **`#menubtn`, the phone menu and `placer`** go (`world-modes.ts:100-114,162-244`). `controlsOf` stays for the bike's
  and the ship's pad.
- **`#sheet`** (`town-phone.ts:121`) stays the Look and details surface, `.glass`. The dock hides under it.

**Desktop:**
- The dock sits at the bottom centre, with key caps.
- Hover focus, `HOVER_KEEP_MS` and the freeze read `#dock` (`world-focus.ts:99-108`).
- Inside a building there is no deck (N8).

### 10. Accessibility

- **Focus order:** Menu, the dock, then an open panel. `#settings` is a modal `<dialog>`; People is not.
- **Names:**
  - "Menu (H), 2 unread";
  - "Look: Library";
  - "More actions for Library";
  - "Frost bolt, slot 1 (hold to change)";
  - "Ultimate, 60% charged";
  - "Place wall (hold for pieces)";
  - "Chat from Sami: …, open People".
- **Live regions:** a focus and a line speak once; fight and build stay silent.
- **Targets:** at least `--btn`; the phone's fight and build buttons are `--btn-fight`.
- **Reduced motion:** no rise, fade, radial scale (`world.css:2253`) or timer animation.
- **Reduced transparency:** `.glass` falls back to `--surface-glass-solid`, through one class (`theme.css:86`).
- **The swipe-up jump** has no keyboard need: Space jumps.

### 11. The visitor page

- **The Menu:** Map, People (realm up) and Settings.
- **At rest:** 2 pieces, or 3 with `#welcome` on a first visit.
- **`OWNER_IDS`** (`world-public.ts:121`) gains `overview` and `cluster`. The Cockpit link is never built.
- **`PHONE_IDS`** (`:125`) drops `seecount`.
- **`#under`** goes.

### 12. Plan/22 hooks

- **P3:** People › Mail is `world-mail.ts`. Its unread feeds the Menu dot, its news a dock line, and the rider radial's
  Message opens Write.
- **P4:** the mic loses its "near Jump" spot. Its place is the owner's call when P4 starts. PTT/OPEN goes in Settings ›
  Sound.
- **P5:** People › Party holds members, invite and party chat. Invite and Report join the rider radial.

## Every current element and its fate

`h` = `world.html`, `c` = `world.css`, `t` = `theme.css`, `tc` = `town.css`.

| Element | Where | Fate |
|---|---|---|
| `#top` bar, `.mark` | h:38-61; c:1405 | removed; pieces in §7 |
| `#menubtn` + phone menu | h:39; c:1426; `world-modes.ts:172-244` | removed (Menu) |
| `#health`, `.chip.world`, `.age`, `.live` | h:41-43,51 | merged into Cluster panel |
| `#statsbtn` + `#groups` | h:44-50; c:1729 | merged into Cluster panel |
| `#mode` Explore | h:52; c:6 | merged into overview head + Menu › Overview + Tab |
| `#search`, `#find`, `#names` | h:53-55 | merged into overview Find |
| `#deck` | h:56; c:335 | kept as `#menu`, bottom left |
| `#helpbtn` | h:57; c:1861 | removed (`?` → Settings › Controls) |
| `#sound` | h:58; c:267 | merged into Settings › Sound |
| `#seecount`, `#stack` + chip + card | h:59,70; c:327,1555,1800 | merged into overview Needs you + Menu dot |
| `#account` | h:60; tc:266 | merged into Cluster panel foot |
| `#signin` (guest) | c:1423,1931 | merged into Settings head |
| `#under` (visitor) | c:268 | removed (carrier focus act) |
| `#sim` | h:62 | kept (sim world), `.glass` |
| `#crumbs` | h:63; c:1794 | removed |
| `#rides`, `#ride`, `#pvp` | h:64; c:27,1861 | merged into Settings › Controls |
| `#worldmap` | h:65; c:616,1560,1774 | kept; top right, 72/160 |
| `#compass` | h:65; c:925 | kept as one N |
| `#mapbtn`, `#lensbtn`, `#lensbox` | h:65,68-69 | removed (already hidden) |
| `#mapgo` | h:65; c:697 | removed (big map's Go to it) |
| `#secrets` | h:65; c:324 | kept hidden (cockpit row) |
| `#maptip`, `#mappop` | h:66-67; c:725 | kept, family |
| `#realm-banner` | `world-realm.ts:695`; c:970 | merged into a dock line + People note |
| `#card`, `#flow`, `#inspector`, `#island`, `#reader`, `#talk`, `#quick`, `#yard`, `#keeper` | h:71-77,85-86 | kept, `.glass` |
| `#cockpit` | h:75 | kept; opened from the overview's Cockpit link |
| `#inbox` (letters) | `world-keeper.ts` | merged into People › Mail |
| `#chat` (chips, log, form, note) | h:78-83; c:1986-2107 | merged into People › Chat + dock line |
| `#chatshow` | h:84; c:2002 | removed |
| `#chip` (fleet) | h:87 | kept hidden |
| `#sheet` | h:88; tc:1155 | kept, `.glass` |
| `#bottom` deck (`#map`, `#minimap`, `#lenses`) | h:89-94 | removed (`#minimap` stays hidden) |
| `#legend` | h:93; tc:816 | merged into Cluster panel |
| `#portrait`, `#commands`/`#grid`, `#next` | h:95-99 | merged into the overview's selection view |
| `#cmds` | h:98 | removed |
| `#line` | h:97; `town-phone.ts:201` | removed |
| `#status`, `#tip`, `#loader`, `#said` | h:101-102,21,110 | kept |
| `#calm-hint` | h:103; c:396 | merged into dock focus |
| `#pad` foot buttons (Lock, Attack, Jump, Run, Build) | h:104; c:59-118 | removed on foot |
| `#pad` off foot (Advance, Jump/climb, Attack, Run) | h:104 | kept; Run becomes the speed toggle |
| `#pad .skills` | h:104; c:120-145 | merged into dock fight slots |
| `#pad .buildpad`, `[data-build]` | h:104; c:1659 | merged into dock build face |
| `#pad::after` dot | c:155 | kept with the pad only |
| `#crosshair`, `#hitmark` | h:105-106 | kept |
| `#focus` | h:107; c:2112-2212 | merged into `#dock` focus face |
| `#radial` | h:108; c:2217 | kept; + `arc`, slot and piece radials |
| `#hottag` | h:109; c:509 | kept for a fight target only |
| `#toast` | h:111; c:1359 | merged into dock line |
| `.bcard` | c:546 | kept in the overview only |
| `.bl`, `.unit`, `.trunk`, `.room`, `.civ` | tc | kept |
| `.fight-hud .vitals` | `world-fight.ts:415-416,902`; c:995,1733 | merged into dock fight (HP, ring); Wood/Iron to Settings |
| `.fight-hud .skills` | `world-fight.ts:417` | removed |
| `.fight-boss` | `world-fight.ts:428`; c:1068 | kept (the boss's label) |
| `.mobbar` | `world-fight.ts:506`; c:1025 | kept; hidden on the tagged target |
| `.dmg`, `.fight-tick`, `.fight-hurt`, `.fight-downed` | `world-fight.ts` | kept |
| `.build-hud` | c:1162 | merged into dock build note |
| `#welcome` | `world-welcome.ts:47`; c:1949 | kept, restyled |
| `#controls`, `#wardrobe` | c:1193-1335 | merged into `#settings` |

## Behaviour changes (only these)

1. **At rest,** only the map and the Menu show (plus `#welcome` on a visitor's first visit).
2. **The top bar is gone,** in the overview too.
3. **The focus box becomes the dock.** On the phone it sits beside the Menu, and it shows inside for parts.
4. **The pastime is a dock focus.** A build zone offers Build (N3).
5. **The fight face** has HP, two assignable slots and the ultimate. A long-press assigns from the class's skills,
   remembered per class. The pad's skills, the vitals and the desktop key strip go.
6. **Desktop Q fires slot 2.** The Mage's spell cycling is gone (N6).
7. **Build is a dock face:** Place, Turn, Take down and Done. A long-press on Place picks the piece. It is entered from
   a build zone's focus or Settings, and the build pad and `.build-hud` go.
8. **The PvP flag alone no longer shows the fight controls** (Q17).
9. **One-time hints show in the dock line on both tiers** for 4 s.
10. **Chat lines show one at a time in the dock line,** never over a fight or build. The log lives in People › Chat,
    and show/hide is gone.
11. **The Menu has 4 entries.** `?` opens Settings › Controls.
12. **The ride, PvP and Sound move into Settings.** 1, 2, 3 and P still work.
13. **The phone's pad is hidden on foot.** A mob is attacked by tapping it.
14. **Go (`#mapgo`) is gone.**
15. **Full speed is the default on every ride.** On foot the rider walks inside and within `ARRIVE_WALK` of the end, and
    the Run button and the double tap go. On the bike and the ship the speed button toggles down to cruise, as do
    Shift and L3 (N1).
16. **Auto-jump on every walk.** A swipe up jumps. The phone has no Jump button on foot.
17. **`#hottag` names only a fight target,** and `.bcard`s never draw in explore.
18. **The phone map moves to the top right,** 72 px, with N only.
19. **The overview is the side panel.** It holds the Cockpit link and the Cluster panel. A selection shows there, and
    the desktop deck and the crumbs are gone.
20. **Inside on the desktop,** the deck is gone. Talk is a focus act, and Leave is Esc or the door (N8).
21. **One unread dot,** `--danger`, on the Menu.

## Phases: review slices inside one PR

Every commit runs `bun run build` (bun 1.4.x for dist identity), typecheck and `bun test`. Before merge,
`go test ./...` runs in full. Each gate row flips in the commit that changes it.

1. **The proof on main.** `dev/hud-gate.mjs` (new) records today's counts and overlaps.
   - Rows that will change carry `changes: N`, with N from the list above.
   - Run it twice on main, then break one placement on purpose to see a row fail.
2. **The family:** tokens, `.glass`, `.glass-btn`, `data-hud`. Flips nothing.
3. **Speed and jump:** `sprints`, the deletions, the speed toggle, `ledgeAhead` and the swipe. Flips 15 and 16.
4. **The dock:** `world-dock.ts`, `world-kit.ts`, the five faces and the line queue; `#toast`, `#calm-hint`, the
   vitals, `.build-hud` and the build pad go; the tag rule. Flips 3–10 and 17.
5. **The Menu and the panels:** `#menu`, 4 `GLOBALS`, `arc`, People, Settings; `#menubtn` and `#helpbtn` go. Flips 11,
   12 and 21.
6. **The pad on foot, `#mapgo`, `#rides` and the map corner.** Flips 1, 13, 14 and 18.
7. **The overview:** `#top` and `#bottom` retired, `#overview` with its selection view, `#cluster`. Flips 2, 19 and
   20.
8. **The visitor page, accessibility, the README controls table and the gate edits.**

**Gates to update** (labelled edits, each with its reason):
- **input-gate:**
  - `clearOf` and `placed` (`dev/input-gate.mjs:296-325`): `#dock` against `#menu` and `#worldmap`;
  - `openCockpit` (`:365-367,606`): Menu → Overview → Cockpit;
  - the Menu rows (`:1751-1766`): 4 entries;
  - the visitor Menu (`:1774-1790`): `["Map","People","Settings"]`;
  - the phone Menu (`:1997-2013`): `#menu`;
  - pad Lock (`:279,1024`): moves to a bike row or desktop T;
  - the box rows (`:1813,1830,1886`): `#dock .primary` and `.more`.
- **world-gate:** `#top > *` (`dev/world-gate.mjs:502`) becomes `[data-hud]`; `#mode` (`:533,558`) becomes Tab.
- **onfoot-gate:** `#mode` (`:271-275,342-343,483`) becomes Tab; `#helpbtn` (`:365-374`) becomes `?`.
- **aim-gate:** `#mode` (`:99`); `#hottag` on a building (`:247-249`) becomes `#dock .name`; `#bottom` and `#rides`
  (`:20`) become `#dock`.
- **calm-gate:** the cockpit path (`:163-165`) and the quick row's Sound (`:184,300`) become Settings › Sound.
- **cockpit-gate:** `#deck` → Cockpit (`:230-241`) becomes Menu → Overview → Cockpit.
- **public-gate:** `#hottag` health (`:165`) on a mob; `#ride` (`:184-186`) becomes Settings › Ride.
- **Already failing on main** (plan/23 build notes): world-gate's Rimfall and Underside rows; onfoot and aim on the
  phone's `#mode` (that row goes with `#mode`); public's `/api/land`.

**Checks (lean-checks):**
- Every commit: unit tests and dist identity.
- Once before review: a full manual GPU round of world, onfoot, aim, calm, cockpit, public, input and hud.
- `ci-local --gates` covers world and public only.

## Tests

Each new test has a mutation and a positive control.

| Test | Covers |
|---|---|
| `world-dock.test.ts` (new) | `dockOf`: covered and the overview give rest, then build > fight > line > focus; fight on foot only. The queue: one at a time, newer replaces, a hint waits behind chat, fight and build hold lines, a timeout adds to unread, a hint is seen only once shown |
| `world-kit.test.ts` (new) | `poolOf` per class (the Mage's three spells); defaults are the pool's first two; `assign` swaps; storage per class, junk ignored, a throwing store survives; a long-press release assigns and never fires |
| `world-ride-step.test.ts` (new) | `sprints` on foot (inside, `toEnd` around `ARRIVE_WALK`, Shift ignored) and on a ride (`slow`) |
| `world-hud-tokens.test.ts` (new, the guard) | in the rules for `#dock`, `#menu`, `#people`, `#settings`, `#overview`, `#cluster`, `.glass`, `.glass-btn`: radii only through `var(--radius-*)`, no `rem`, no raw colours, sizes through `--btn` or `--btn-fight` |
| `world-actions.test.ts` | the 4 entries; the visitor's 3 |
| `world-chat.test.ts` | no fade or show; unread while the tab is hidden |
| `world-modes.test.ts` | the menu and toast go; `hintQueue` moves |
| `world-focus.test.ts`, `world-focus-dom.test.ts` | the pastime and build-zone rules; no `place()`, no `--pad-h` |
| `world-radial.test.ts` | `arc(4)` above the centre, no overlap |
| `world-input.test.ts` | `padShows` false on foot on a phone |
| `world-router.test.ts` | `swipe-up` against a slow drag, a sideways flick and a pinch |
| `world-deck.test.ts` | `deckOpen` retired; `deckShows` kept for the overview's commands |

**`dev/hud-gate.mjs` (new)** runs on `dev/stack-realm.sh`.

Sizes: 320×568, 390×844 and 430×932 with touch, and 1440×900 with a mouse.

States:
- rest; near a building; inside by a stair;
- a build zone, then build mode;
- a fight (a stub mob's hit, `HURT_MS`) and a slot's radial;
- a chat line at rest, over a focus, and during a fight, where it must wait;
- the Menu, People and Settings;
- the overview, with and without a selection;
- the visitor at rest, and the bike.

| Assertion | Value |
|---|---|
| visible `[data-hud]` at rest | **2** (`#worldmap`, `#menu`); 3 on a visitor's first visit |
| near, inside, build, line | 3; fight: 3 + the target's `#hottag` |
| overlaps between visible `[data-hud]` boxes | none, except the scrim and a panel over the dock |
| on screen; the dock's foot clears the safe area | yes |
| buttons in `[data-hud]` | ≥ 44×44; the phone's fight and build buttons 56×56 |
| computed radii | containers 16, buttons 12 |
| one label | the focused target's name in at most one of `#dock .name`, `#hottag`, a visible `.bcard` |
| explore | `#overview`, `#cluster` and the selection view hidden |
| visitor DOM | no Cockpit link |
| pixels | one screenshot per state and size, diffed against the PR's own first run; `--update` accepts |

## Hands-on QA (`dev/stack-realm.sh`; phone emulator 390×844 touch, then 320×568; desktop 1440×900)

| # | Phone: do | Expect |
|---|---|---|
| 1 | Rest; tap-walk a long route over ledges; tap at a low wall; swipe up; drag slowly up; go inside | only map and Menu; it runs, jumps by itself, walks on arrival; hops the wall; a swipe jumps, a slow drag looks; inside it walks |
| 2 | Tap a hall; long-press Look and slide to Enter; walk away | the dock shows Look and ⋯; you go in with no stray click; the dock leaves |
| 3 | Inside, stand on a stair; tap a book | the dock shows the stair; the book reads at once |
| 4 | A wolf hunts you; tap it twice; tap a slot; long-press a slot and slide to the other skill; reload; change class | HP, 2 slots and the ultimate under the right thumb; you attack; the slot casts; they swap and nothing fires; kept; each class has its own pair |
| 5 | Chat from a second page: at rest, over a focus, in a fight; tap the line | about 4 s, then the focus returns; it waits out the fight; People › Chat opens |
| 6 | Walk into the meadow ring; Build; Place, Turn, Take down; long-press Place; Done | the dock offers Build; the build face works; a floor picked; a refusal shows in its note; Done ends it |
| 7 | Menu; People: send a line; Settings: bike, then foot; PvP and Sound | 4 items on an arc; the bike runs full speed with its pad and its speed button cruises; on foot the pad goes |
| 8 | 320 px, every state above | the name ellipses, buttons keep their size, nothing overlaps the Menu |
| 9 | Guest visitor | welcome card; Menu: Map, People, Settings; Sign in in Settings |

| # | Desktop: do | Expect |
|---|---|---|
| 1 | Hover the Library; hold E; go inside; Esc | "Look `E`" in the dock, no tag; the radial; no deck inside; Esc leaves |
| 2 | Enter, then Esc | People › Chat with the input focused; the keys come back |
| 3 | Tab; `/`; Fly there; pick a building; back; Cockpit ›; Cluster and districts › | the overview panel, its selection view with Next and the commands, the Cockpit, the cluster numbers and villages |
| 4 | A fight: right button, Q, G; then B and R, X, Q | slots 1 and 2 and the ultimate; the build face with its caps |
| 5 | `?` | Settings › Controls |
| 6 | Keyboard only with VoiceOver: Menu, its items, Esc; a slot's Shift+F10; a chat line | named items, focus back on Menu; the slot radial; the line spoken once |
| 7 | Reduced motion; reduced transparency | no rise or fade; solid panels |

## Risks

- **Auto-jump on plain walks** may hop at walls the rider should stop at. The probe jumps only for a rise within the
  jump's reach, and QA walks into walls on purpose.
- **The swipe-up jump fights the camera's pitch drag.** The thresholds are knobs, and the router test pins a slow drag
  as a look.
- **Full speed everywhere changes feel.** Collisions come faster, and C always slides. The bike's cruise is now a
  press away instead of the default.
- **The fight face flickers** if `inFight` toggles at the hunt edge. It reads `hurtAt` too; add `FIGHT_HOLD_MS` if
  needed.
- **Chat in 4 s lines** hides a busy chat from view, not from People: those lines count as unread.
- **Slots replace the Mage's cycle.** A Mage used to Q-cycling must relearn. Its stored slots start from today's first
  two spells.
- **A big PR.** One slice per commit, each with its dist.
- **Plan/22 is in flight.** The P3 UI branch rebases onto `#menu` and People › Mail. P4's mic needs a place.
- **The gates already failing on main** may hide a regression. The hud-gate does not depend on them.

## Not doing

- The bike's and the ship's controls beyond the default speed and the toggle's meaning.
- The mail server (#250), and voice and party beyond their hooks.
- New themes, the 3D world, or in-world signs.
- Rebinding Esc, M, Enter, `?` or Tab.
- The shooter zone, a pad hover, a focus-cycle key, or automatic facing.
- The town page.

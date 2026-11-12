# 24 · One HUD: the dock, the Menu and the panels

Status: draft, 2026-11-12. For the owner's review.

Sources:
- the owner interview and the mockup review of 2026-11-12 (memory `hud-design-intent`, `hud-coherence`);
- the approved mockups: `.wt/hud-mock/DESIGN.md` and `out/*.png` (P1–P6 phone, D1–D4 desktop);
- the inventory of every current HUD element: `.wt/hud-shots/INVENTORY.md`.

What it changes in other plans:
- **plan/23.** `#focus` becomes the dock's focus face (§3). The Menu radial drops from 8 `GLOBALS` to 4 (§4). The
  phone gets a dock inside buildings, which plan/23's "Not doing" excluded.
- **plan/22.** The P2 chat log moves into People › Chat. P3 mail and P5 party UI plug into People's tabs. P4's mic
  "near Jump" has no Jump to sit near on foot; its place is set when P4 starts (§12).

## Context

Every claim was checked on ukubi-game main at `132ffc0` (plan/23, #248). Paths are under `cmd/game/web` (`.html`,
`.css`) or `cmd/game/websrc` (`.ts`).

### What is on screen

- **Counts.** The inventory counted 11 floating pieces at rest on the phone and 13 on the desktop, 24 with the phone's
  Menu open and 36 in a desktop overview with a selection.
- **Five panel styles:**
  - glass (`theme.css:7-39`, `:42`);
  - flat raised + `--shadow-float`;
  - rem-based glass outside the tokens (`.fight-hud`, `world.css:995`);
  - bare stroked text;
  - accent fill.
- **Four radii on like things:**
  - `#focus` buttons are r8 inside an r12 card (`world.css:2145`, `:2188`);
  - pad buttons are r16 (`:94`);
  - radial buttons are r12;
  - chat chips are r8.
- **The glass list is hand-kept in three places** (`theme.css:7`, `:48`, `:86`). `#focus`, `#toast`, `#deck`, `#menubtn`
  and `#chat` are on none of them.
- **One thing, three names.** On a desktop hover, `#hottag` (`world-targets.ts:431-439`), a `.bcard` and the focus box
  all name the same building (inventory, desktop-3).
- **The phone menu** (`#menubtn`, `world.html:39`) unfolds `#top` into 12 rows. One of those rows is "Menu" (`#deck`),
  which opens a second menu, the radial.

### Hints, chat and the pad

- **Hints.**
  - On the phone, one-time hints show in `#toast`, top centre, for `TOAST_MS` 7 s (`world-modes.ts:129,151-160`).
  - On the desktop, the same words are the static help line `#line` (`town-phone.ts:201`, `world-chrome.ts:244-246`).
- **Chat** is a log, bottom left: 4 chips, the lines, a form, plus `#chatshow` (`world.html:78-84`).
  - It shows the last 5 lines, each fading after 10 s (`world-chat.ts:6-10`).
  - Its visibility is polled every 500 ms (`world.ts:2358-2360`).
  - Party is a disabled chip.
- **The pad** (`world.html:104`, `world.css:59-166`) shows exploring outside with no panel open (`world-input.ts:263`).
  On foot at rest the phone draws Build, Run, Jump and Skill 2 (inventory phone-1).
- **The ride button** (`#rides`, `world.html:64`) moves into the phone menu at rest and floats otherwise
  (`world-modes.ts:169`).

### Running

The rider walks unless something asks for a run:
- **Shift held**, or the rebound sprint key (`world-view.ts:456,1195-1196`).
- **The pad's Run toggle.** It is kept through resets (`world-input.ts:235`, `world-view.ts:523`).
- **The gamepad's L3 toggle** (`world-view.ts:1684`).
- **A double tap on the ground** (`world-view.ts:1923-1927`).
- **An engage's second tap within `DOUBLE_MS`** (`world.ts:1215,1271`).

The read happens in two places:
- `drive` (`world-view.ts:1436`) for a route;
- `ridePose` (`:1541`) for the step.

Speeds (`world-ride.ts:45-46`, the same `run` field on every ride):
- foot: walk 4, run 7 heights a second;
- bike: 16 and 26 (`:59-60`);
- ship: `sprint()` is its boost (`world-view.ts:1398`).

### The focus box today (plan/23)

- **Placement.** One element: in `#bottom` on the desktop, fixed in the body on a touch phone
  (`world-focus.ts:163-170`). It sits over the pad through `--pad-h` (`:172-174`, `world.css:2176-2212`).
- **No box inside on the phone.** `body:has(#pad[hidden]) #focus` hides it (`world.css:2200`).
- **The Menu.** `GLOBALS` holds 8 entries (`world-actions.ts:117-139`). It opens from `#deck`, H or Start
  (`world.ts:2416-2421`, `world-router.ts:237-260`).
- **The radial** lays out up to 8 items on an ellipse, rx 140 by ry 124 (`world-radial.ts:14-21,51-55`).
- **Enter opens the chat** while exploring outside (`world.ts:2690`).

### The overview

- **`#top`** holds:
  - the health, the world chip, the age and Cluster with its 4 groups (`world.html:41-50`);
  - Live, Explore (`#mode`), search, Menu, ?, Sound, To see and the account (`:51-60`).
- **On the right:** the crumbs (`:63`) and the To-see stack (`:70`).
- **At the bottom left:** the road legend (`#legend`, `town.css:816`).
- **At the bottom:** the desktop deck `#bottom` (`world.html:89-100`) opens on a selection
  (`world-deck.ts:41-42`).

### The visitor page

- **Removed:** `chip`, `island`, `sim`, `account`, `next` and `status` (`world-public.ts:121`).
- **Hidden but kept:** the phone-read nodes (`:125`).
- **Added:** `#signin` (`world.css:1423`), `#sound`, `#under` and `#welcome` (`world-welcome.ts:47`).
- **No overview.** The world is explore only (`exploreOnly`, `world-view.ts:678,803`).

### The parked P3 work

- **Server: #250 (`realm/p3-mail`, open).** It adds `realm_mail`, the wire, caps, forget and `--import-letters`.
- **UI: `realm/p3-mail-ui` (wip, `2d46f61`).** It adds:
  - `world-mail.ts`: a pure `makeMailModel` plus a `#mail` chrome with Inbox and Write views;
  - the rider radial's Message;
  - the Menu's Mail for everyone while the realm is up;
  - mail unread on `#deck`'s dot.

## Decisions (owner, 2026-11-12, confirmed)

1. **At rest,** exploring on either tier, only the map corner (top right) and one Menu button (bottom left) show.
2. **One bottom dock,** shown only in context:
   - **near a thing:** the name, the primary and ⋯ (hold for more);
   - **in a fight:** a thin own-HP row, then one skill and the ultimate with an energy ring, under the right thumb.
     There is no Attack button: you tap the mob;
   - **a chat line, a mail or a one-time hint** slides into the dock line for a few seconds. A tap opens its panel.
3. **The Menu radial has 4 entries:** Map, People (Chat, Mail, Party tabs), Overview, Settings (controls, wardrobe,
   sound).
4. **The overview** gets its own layout. One side panel holds search and "Needs you", plus a "Cluster and districts ›"
   link to a second panel.
5. **One component family:**
   - containers `--radius-lg`; buttons `--radius-md`;
   - 44 px command buttons; 56 px phone fight buttons;
   - the theme's glass; Jost;
   - 16 px edge spacing and 8 px gaps;
   - tokens only.
6. **Removed or merged:**
   - the chat chips and log;
   - the toast and the tutorial bubble;
   - the always-on pad buttons (Jump, the bolt, home/Go, ride-swap);
   - the help line;
   - duplicate labels;
   - the crumbs and "PvP off" while exploring;
   - the top bar while exploring.
7. **Running is the default on foot.**
   - The rider walks only inside buildings and when arriving at an auto-route target.
   - The Run button and the double-tap run go.
   - Bike and ship controls stay as today.
8. **One label per thing, one dock,** and nothing floats on its own.

### Open questions for the owner

The plan builds the **default** named in each one. Each is a one-line swap if the owner picks otherwise.

- **Q1, a line over a focus.**
  - The brief says a line overlays briefly. DESIGN.md says "near beats line".
  - Default: the line overlays the focus for `LINE_MS`, then the focus face returns. A fight still holds the line back.
- **Q2, the Cockpit's way in.** It is not one of the 4 entries.
  - Default: C in the overview, as today, plus one "Cockpit ›" row in the overview panel under Needs you.
  - Without that row, the phone has no way in.
- **Q3, the fight dock's skill.**
  - Default: Skill 1 (`world.ts:1449`, the Mage's selected spell).
  - The Mage's spell cycling (Skill 2, `:1438`) then has no phone button. Q stays on the desktop.
- **Q4, Shift, L3 and C on foot** now that running is the default.
  - Default: Shift (or the sprint key) and L3 do nothing on foot; they still sprint on the bike and the ship.
  - C while moving always slides (`world-moves.ts:1`). A crouch-walk needs a stop first.
- **Q5, the Run button on the bike and the ship.**
  - Default: kept there, as "controls stay as today". It goes on foot only.
- **Q6, Build on the phone.** DESIGN.md moves it "to the radial of the thing (yard)", but no yard focus kind exists.
  - Default: Build becomes a Settings › Controls toggle where B works.
  - The build pad (Place, Next piece, Turn, Take down) shows while building, as the bike's pad does.
- **Q7, Wood and Iron** (`world-fight.ts:902`).
  - Default: one line in Settings › Wardrobe's foot, beside the kills and ults it already counts.
- **Q8, the desktop fight dock** (not mocked).
  - Default: the same face with 44 px buttons and key caps (Skill 1's binding, G), bottom centre.
- **Q9, desktop hints.** The help line goes.
  - Default: the same one-time hint ids show in the dock line on both tiers.
  - The desktop's words are short: "E acts · H menu · Enter chat". The full key list lives in Settings › Controls.
- **Q10, the phone overview** (not mocked).
  - Default: the same panel, full width less 2 × 16, at the top. It is capped at 50 vh and scrolls.
- **Q11, the P6 "Answers board · Read".** Inside, books are not focusable: a tap reads at once
  (`world-view.ts:1901-1903`).
  - Default: inside, the dock shows parts only (stairs, machines). A book tap still reads at once.
- **Q12, a guest's Sign in, `#welcome` and the realm-down banner** at rest.
  - Default: Sign in moves into Settings' head.
  - `#welcome` stays a first-visit card in the family, as the one allowed third element.
  - The banner becomes one dock line, plus a note in People.
- **Q13, the desktop deck `#bottom`:** its portrait, the commands grid, Next, Up and `#line`.
  - Default:
    - explore and inside use the dock and its radial;
    - the overview keeps the deck for a selection only, restyled;
    - `#line` goes.
- **Q14, the Menu's key cap.**
  - The mockups show `Esc` on the Menu. Plan/23 D1 bound H.
  - Default: H stays and the cap shows the bound key. Esc keeps its closing chain.
- **Q15, the boss.**
  - Default: the boss's top bar (`.fight-boss`) is its one label, and the world tag skips the boss.
- **Q16, homes for the leftovers.** Default:
  - the legend and the account go to the Cluster panel;
  - Underside stays on the carrier's focus act (`world-actions.ts:104`);
  - Secrets and Fleet stay in the cockpit's quick row.
- **Q17, PvP on with no mob near.** Today it counts as a fight (`world-modes.ts:30`).
  - Default: the dock's fight face needs `inFight` or a hit in the last `HURT_MS`. The PvP flag alone stays at rest.
- **Q18, the arrival walk.**
  - Default: `ARRIVE_WALK` = 3 units from the route's end. It is a tuning knob, set by feel at QA.

## Shape

### 1. Zones (both tiers)

| Zone | Phone (390×844) | Desktop (1440×900) | Shown |
|---|---|---|---|
| Map corner | top right, 72 px circle, inset 16 + safe area | top right, 160 px circle, inset 16 | always, except in the overview (the world is the map) and under the enlarged map |
| Menu | bottom left, 44×44, inset 16, foot on `env(safe-area-inset-bottom)` + 8 | bottom left, 44 px "Menu" + key cap | always, the same spot in every state |
| Dock | bottom, from Menu's right edge + 8 to the right inset | bottom centre, max 560 | §3 states only |
| World tag | over the fight target | the same | a fight target; buildings in the overview |
| Panels | from the Menu: full width less 2 × 16, above the Menu | left, above the Menu: People 380 wide, Settings 640 | on demand |

Changes from today:
- **The phone map** moves from the top left, 64 px (`world.css:1560-1567`), to the top right at 72 px.
- **The desktop map** goes from 168 px (`:1739`, `--where`) to 160 px, at `top: var(--hud-inset)`, since there is
  no top bar.

### 2. The component family

**New custom properties** go in `tokens.css :root`, once, with no per-theme values:

```css
--hud-inset: var(--space-4);   /* screen edge to any HUD element */
--hud-gap: var(--space-2);     /* between items, and the dock's padding */
--hud-row: var(--space-3);     /* between rows in a panel */
--btn: 44px;                   /* tier 1: Menu, dock actions, ⋯, radial items, tabs, panel controls */
--btn-fight: 56px;             /* tier 2: the phone fight dock's two buttons only */
--map-corner: 72px;            /* 160px at min-width 1200 */
--dock-max: 560px;             /* the desktop dock's width cap */
--line-ms: 5s;                 /* the dock line's stay (read by world-dock.ts) */
```

**One glass class.** `.glass` replaces the three hand-kept `:is()` lists (`theme.css:7-39`, `:48`, `:86`). The rules do
not change; only the selector does:
- `.glass` is the container: glass, starfield, `--glass-blur`, `--edge-glow` 1 px, `--shadow-glass`, `--radius-lg`.
- `.glass-btn` is a free-standing button: glass without the stars, `--radius-md`, `min-height: var(--btn)`.
- **Ghost** is a button inside glass: `color-mix(in srgb, var(--surface-raised) 55%, transparent)` plus 1 px
  `--edge-strong`.
- **Primary** is an `--accent` fill, an `--accent-strong` edge and `--on-accent` text.
- **On or pressed** is 2 px `--edge-on` plus `--shadow-glow-soft`. This is today's `theme.css:131` rule, kept.

**Exceptions:**
- the map: a 50% circle with a 2 px `--edge-strong` ring and one N on the rim, replacing N/E/S/W
  (`world.html:65`, `world.css:925-956`);
- key caps: `--radius-xs` 4 px, overline type.

**Type:**
- `--type-title-3` for the dock name and panel titles;
- `--type-label` for buttons and tags;
- `--type-body-sm` for chat and alerts;
- `--type-overline` for hints, section heads and key caps;
- `--type-metric` for the cluster numbers.

**State colours:**
- `--game-hot`: the target's edge and caret;
- `--game-harm`: mob HP;
- `--game-hp`: own HP;
- `--game-energy`: the ultimate's ring;
- `--danger`: the unread dot. One dot style replaces the red-vs-warning pair (`world.css:349`).

**Tagging.** Every HUD root carries `data-hud`. The layout gate (§11) counts `[data-hud]` that are visible.

### 3. The dock: one element, four states

**The element.** `world.html` replaces `#focus` (`:107`) with:

```html
<section id="dock" class="glass" data-hud aria-label="Dock" hidden>
  <div class="face focus"></div><div class="face fight"></div><div class="face line"></div>
</section>
```

**Ownership.**
- **`world-dock.ts` (new)** holds the pure state rule and the line queue, and shows one face.
- **`makeBox` (`world-focus.ts:140`)** still builds the focus face: name, primary, ⋯, note, `tapHold`.
- **Its placement code goes:** `place()` (`:163-170`), the `--pad-h` observer (`:172-174`), and the phone rules
  (`world.css:2176-2212`). The dock has one place per tier, and nothing else is ever under it.

**The state rule (pure, tested):**

```ts
export type DockState = 'rest' | 'focus' | 'fight' | 'line';
export interface DockIn {
  view: 'explore' | 'inside' | 'action' | 'flying';
  covered: boolean; // plan/23 covered(): a panel, talk, the sheet
  fight: boolean;   // inFight || now - hurtAt < HURT_MS (Q17: the PvP flag alone is not)
  foot: boolean;    // the fight face is on foot only; the bike and the ship keep their pad
  focus: boolean;   // focus.current() !== null
  line: boolean;    // a line is up (its LINE_MS not run out)
}
export const dockOf = (d: DockIn): DockState =>
  d.covered || d.view === 'action' || d.view === 'flying' ? 'rest'
  : d.fight && d.foot ? 'fight'
  : d.line ? 'line'          // Q1 default: a line overlays the focus briefly
  : d.focus ? 'focus' : 'rest';
```

**The faces:**

| State | Contents | Phone | Desktop |
|---|---|---|---|
| rest | none: `hidden` | | |
| focus | name (title-3) over "Hold for more" (overline) · primary (accent) · ⋯ (ghost) | primary is a word (Look, Read, Talk) | "Look `E`", ⋯ titled "More (hold E)": today's `world-focus.ts:256-258` |
| fight | HP row: "HP" overline, an 8 px bar in `--game-hp`, the number · Skill 1 · Ultimate, its border a conic ring of `--game-energy` filled to `energy / ULT_COST` | 56 px buttons, flush right | 44 px with key caps (Q8) |
| line | glyph · source overline ("Chat · Sami", "Mail · Sami", "Hint") · one ellipsed line · chevron · a hairline timer | the same | the same |

**The line queue.**
- `world-modes.ts`'s `hintQueue` (`:77-98`) moves to `world-dock.ts` unchanged and gains two more sources:
  - a live chat line (`world-chat.ts` `hear`, `live` only);
  - a mail item (P3's `hear`, news only).
- **One line at a time.** A newer line replaces the one showing. A hint waits behind a chat or mail line.
- **Each shows for `LINE_MS`** (5000). While a fight holds it back, it waits. If it times out unseen, it adds to the
  Menu's and People's unread count. A hint is marked seen only once shown, as today (`:89`).
- **A tap on a line:**
  - chat: People › Chat;
  - mail: People › Mail on that item;
  - hint: dismissed.
- **The line is one `<button>`.**

**Transitions.**
- The dock rises 8 px and fades in over `--dur-panel`. Its faces cross-fade over `--dur-press`, and its box never
  moves.
- **Reduced motion:** no rise and no fade, an instant swap (`@media (prefers-reduced-motion: reduce)`).
- **The polite live region** speaks a new line and a new focus (`world-focus.ts:271-277`). The fight face is silent,
  with `aria-live` off: the numbers move too fast.

**What it replaces:**
- `#focus` and its two placements;
- `#toast` (`world.html:111`, `world-modes.ts:147-160,241-244`);
- `#calm-hint` (`world.html:103`, `world-pastime.ts:202`). The pastime becomes a focus rule: after carried, before tap,
  `pastime.hint()` gives a `garden` or `water` focus that is already in `INTERACTABLES` (`world-actions.ts:81-97`);
- on foot, `#pad .skills` (`world.html:104`, `world.css:120-145`);
- `.fight-hud .vitals` (`world-fight.ts:415-416`): HP goes into the fight face, and energy is the ultimate's ring;
- `.fight-hud .skills`, the desktop key strip (`world-fight.ts:417`);
- the desktop `#line` (`town-phone.ts:201`).

**Labels: one per thing.**
- **`#hottag`** shows only for a fight target (mob, rider in PvP). It holds the name, a `.mobbar`-style HP bar and a
  `--game-hot` caret. `targets.mark` gets `tagged: h.kind === 'mob' || ...` (`world-targets.ts:431`).
- **A focused building** has no tag. Its name is in the dock, and the scene's outline marks it.
- **`.bcard`s** do not draw in explore (`world-cards.ts` `hidden()` gains `mode === 'explore'`). They stay in the
  overview.
- **The target's own `.mobbar`** hides while its tag shows. Other mobs keep theirs.

### 4. The Menu: 4 entries through `GLOBALS`

**The button.**
- `#deck` leaves `#top` and becomes `<button id="menu" class="glass-btn" data-hud aria-haspopup="menu">` at the bottom
  left. It keeps its handler (`world-chrome.ts:172-175`) and `menuOpener` (`world-radial.ts:48`), renamed to `#menu`.
- **Label:** a grid icon. The desktop adds the word "Menu" and the bound key's cap (Q14).
- **Unread** is one `--danger` dot for letters, mail and missed lines. Its count is in `aria-label`, as the P3 branch
  writes it.
- **While the Menu or a panel is open,** it shows the pressed look and becomes Close (✕, "Close (Esc)").

**`GLOBALS`** (`world-actions.ts:117-139`) becomes:

| id | label | when | run |
|---|---|---|---|
| map | Map | always | `minimap.toggle()` |
| people | People | owner page, or realm up | `people.open(lastTab)` |
| overview | Overview | owner page | `view.flip()` |
| settings | Settings | always | `settings.open()` |

- **Gone from the Menu:**
  - Cockpit (Q2), Mail and Chat: People's tabs;
  - Controls and Wardrobe: Settings' tabs;
  - Talk: the resident's or building's focus act already holds it (`:64-72`); inside, the dock shows it.
- `ActionDeps` loses `cockpit`, `mail`, `chat`, `controls` and `wardrobe`, and gains `people` and `settings`.
- **The visitor's Menu:** Map, People (realm up) and Settings.

**The radial's Menu layout.** It is a half circle above the screen's bottom centre. A new `arc(n, r = 130)` sits beside
`layout` (`world-radial.ts:51`), and `open(..., { arc: true })` picks it.
- Items are 44 px icon buttons with the label under them (P5).
- A scrim of `--surface-sunken` at 40% covers the world.
- The focus radial keeps its ellipse.

**Keys stay:** H (or Start) opens the Menu, Esc closes it, M is the map and Tab is the overview.

**`?` (`world-router.ts:244-245`)** opens Settings › Controls, the key list. It was the deck plus `#line`.

### 5. People: one panel, three tabs

- **The element.** `<aside id="people" class="glass" data-hud role="dialog" aria-label="People">`. Inside:
  - a head with the title and Close (Esc);
  - a `role="tablist"` of three 44 px tabs: Chat, Mail · n, Party.
- **Non-modal.** It is a chrome in `chromes` (`world.ts:1983-1986`), so Esc closes it in order.
- **The router.** Its fields are the typing context's.
- **Chat tab.** `bindChat` (`world-chat.ts:115-204`) is kept and rebinds to the tab's elements:
  - the log is the last `KEEP` (50) lines, not 5 that fade;
  - the form is always shown, with the placeholder "Say something" and an `Enter` cap;
  - **gone:** the 4 chips, the Hide and Show buttons, `#chatshow`, `SHOWN_KEY` and its storage, `FADE_MS`, `SHOW` and
    `fading`;
  - the unread count is now lines that came while the tab was not shown. It feeds the Menu dot and the tab's count;
  - the 500 ms poll (`world.ts:2358-2360`) stays, for `avail`.
- **Enter (desktop)** opens People › Chat with the input focused, through the same globals path (`world.ts:2690`). A
  submit or Esc closes the panel and gives focus back to the canvas, as `close()` does today.
- **Mail tab, the plan/22 P3 hook.**
  - Until P3 lands, the owner's tab is today's letters inbox (`inbox`, `world-keeper.ts`), moved from `#inbox`, and the
    visitor's tab is hidden.
  - P3 drops `world-mail.ts`'s model and its Inbox and Write views in as the tab body: its `#mail` chrome becomes this
    tab. The rider radial's Message opens People › Mail › Write.
- **Party tab, the plan/22 P5 hook.** An empty state, "Parties come later", with no controls. P5 fills it, and party
  chat lives there.
- **The realm down.** Chat and Party say "The shared world is out of reach". Mail keeps the owner's letters.
- **Placement:**
  - desktop: left, above the Menu, 380 wide, up to `100vh - 2 × inset - btn - gap`;
  - phone: full width less 2 × 16, from the top safe area to above the Menu.

### 6. Settings: one dialog, three tabs

- **One dialog.** `#controls` and `#wardrobe` are two `<dialog>`s today (`world.css:1193-1335`, `world-controls.ts`,
  `world-wardrobe.ts`). They merge into `<dialog id="settings" class="glass" data-hud>` with tabs Controls, Wardrobe and
  Sound.
- **The router.** The modal context's predicate (`world.ts:1547,2669`) becomes `settings.open()`. Its pad polls are
  kept.
- **Controls tab:**
  - **Ride.** The current ride with a 44 px segmented choice of the rides open to this rider (`rideNext` and `mayRide`,
    `world-view.ts:674-675`). The click is today's `#ride` handler. 1, 2 and 3 stay keys.
  - **PvP**, a toggle, from `#pvp` (`world.html:64`) and the deck cell (`world-deck.ts:28`).
  - **Build** (phone, Q6), from the phone menu's row (`world-modes.ts:186-212`).
  - Then today's camera, keys and pad-button tables, unchanged.
- **Wardrobe tab.** Today's body. Its rider preview (`world.ts:1007-1010`) still turns the rider while the tab shows.
- **Sound tab.** The sound toggle (`#sound`, `world.html:58`, plus the cockpit quick row's Sound, `world.ts:2049`).
  Plan/22 P4's voice row lands here.
- **Head:** the title, Close, and a guest's Sign in (Q12).
- **Gone:**
  - "Buttons: always / in fights" (`world-modes.ts:187-199`, `PINNED_KEY`): there is no pad on foot to pin;
  - the phone menu's Controls row.
- **Placement:**
  - desktop: docked left, above the Menu, 640 wide;
  - phone: full width less 2 × 16, top sheet, 60 vh.

### 7. The overview layout

**`#top`** (`world.html:38-61`) is retired on both views. Its pieces go:

| Piece | Goes to |
|---|---|
| `.mark` | gone |
| `#mode` Explore | the overview panel's head: "Explore `Tab`" |
| `#search` + `#find` | the panel's "Find a building `/`" (the input moves; `/` unchanged) |
| `#deck` | the Menu button (§4) |
| `#helpbtn` | gone (`?` opens Settings › Controls) |
| `#sound` | Settings › Sound |
| `#seecount`, `#stack` (`:59`, `:70`) | the panel's "Needs you · n": the stack's slots, each with "Fly there `Enter`" |
| `#health`, `.chip.world`, `.age`, `#statsbtn` + `#groups`, `.live` | the Cluster panel |
| `#account`, `#signin` | the Cluster panel's foot (owner) / Settings' head (guest) |

**`#overview`** is a new `<aside class="glass" data-hud>`, 360 wide, top left at the insets, sized to its content. It
holds:
- a head: "Overview" and Explore `Tab`;
- Find;
- Needs you;
- one row link, "Cluster and districts ›";
- "Cockpit ›" (Q2, owner only).

**`#cluster`** is a second panel in the same place, with a back row. It holds:
- the four metric groups, unchanged (`world.html:45-50`);
- the health, the world, the age and Live;
- the villages list (the disc's districts, `villages` in `world.ts`), each a 44 px row that flies there (`ui.go`);
- the road legend (`#legend`, Q16).

**Gone from the overview:**
- the crumbs (`world.html:63`, `world.css:1794`);
- the "1 to see" chip;
- the floating alert card;
- the corner map, hidden in the overview.

**The deck.** `#bottom` shows in the overview only for a selection (Q13), restyled with `.glass`. Its `#line` goes.

**`.bcard` building tags** stay in the overview, restyled to `--radius-lg`, label type and glass.

### 8. Running by default

**One pure gait rule in `world-ride-step.ts`, tested:**

```ts
export const ARRIVE_WALK = 3; // units from a route's or walk's end where the rider slows to a walk (Q18: a knob)
export interface GaitIn { ride: RideId; inside: boolean; toEnd: number | null; held: boolean }
// held: Shift / the sprint key / L3 / the pad's Run (the bike's and the ship's only, Q4/Q5)
export const sprints = (g: GaitIn): boolean =>
  g.ride !== 'foot' ? g.held
  : !g.inside && (g.toEnd === null || g.toEnd > ARRIVE_WALK);
```

**The code changes:**
- `world-view.ts:1541` becomes `sprint: sprints({ ride, inside: !!inside, toEnd, held: sprint() })`.
  - `toEnd` is the distance to `walk`, else to the route's last point, else `null`.
- `:1436` (`drive`'s speed) reads the same rule.
- **Deleted:**
  - `walkRun` (`:440,1503,1927,2105`);
  - the double tap's `isDoubleTap` use and `lastTap` (`:1922-1924`);
  - `walkTo`'s and `followTo`'s `run` parameter (`:384,388`);
  - `engageRun` (`world.ts:1176,1215,1271`).
- **The pad's Run:**
  - `data-pad="run"` hides on foot: `body[data-ride="foot"] #pad [data-pad="run"] { display: none }`;
  - `pad.run` is read only off foot (`world-view.ts:493`);
  - on the bike and the ship it stays (Q5).
- **The realm.** The wire's `sprint` (`world-realm-wire.ts:31`) carries the rule's result, so the realm's physics sees
  the same gait.
- **Hint words** drop "double tap to run" and "hold Sprint" (`world-chrome.ts:96-99`). The keys line drops "Shift
  sprint" on foot (`:90`). Settings › Controls names sprint "Sprint (bike, ship)" (`world-bindings.ts:46,69`).

The avatar's run clip already follows speed (`runAt`, `world-ride.ts:32`), so nothing changes there.

### 9. Phone and desktop specifics

**Phone (`TOUCH_PHONE`, `world-modes.ts:14`):**
- **On foot outside, the pad is hidden.**
  - `padShows` (`world-input.ts:263`) gains `&& a.ride !== 'foot'` for the touch phone.
  - Off foot it shows as today: Advance, Jump or climb, Attack, and Run.
  - **Removed on foot:** Lock, Attack, Jump, Run, Build and the skills.
- **`#mapgo` goes** (`world.html:65`, `world.css:697`). The big map's pop already has "Go to it" and "Guide me here"
  (`world.html:67`). A set waypoint shows one dock line hint once: "Go from the map".
- **`#rides` goes** (§6).
- **`#menubtn`, the phone menu, and `placer`/`put`** (`world-modes.ts:100-114,162-170,172-244`) go. `makeModes`
  keeps `controlsOf` for the bike and the ship pad and for `data-controls`.
- **`#pad::after`, the centre dot** (`world.css:155-165`), shows only with the pad, so never on foot.
- **The sheet** (`#sheet`, `town-phone.ts:121`) stays the Look and details surface, restyled to `.glass`. The dock hides
  under it (`covered`).

**Desktop:**
- The dock is bottom centre, 560 max.
- Key caps sit in its buttons.
- The fight dock is Q8.
- Hover focus, `HOVER_KEEP_MS` and the freeze over the dock are unchanged (`world-focus.ts:99-108`). `frozen` reads
  `#dock` instead of `#focus`.

**Narrow non-touch (under 1200, fine pointer)** gets the desktop rules at the phone's sizes. Its dock is full width
less the Menu.

### 10. Accessibility

- **Focus order:** Menu, then the dock (primary, ⋯, or the fight buttons, or the line), then any open panel.
  - The map canvas keeps `tabindex="0"`.
  - Panels trap nothing. `#settings` is a modal `<dialog>`, and the browser traps there.
- **Names:**
  - Menu: "Menu (H), 2 unread";
  - dock primary: "Look: Library";
  - ⋯: "More actions for Library";
  - fight buttons: "Frost bolt", "Ultimate, 60% charged";
  - the line: "Chat from Sami: …, open People".
  - Every icon-only button has an `aria-label`.
- **Live regions:** the focus box's polite region is kept. The line speaks once. The fight face is silent.
- **Targets:** every control is at least `--btn` (44 px); the phone fight buttons are `--btn-fight` (56 px). The gate
  checks every control with a box.
- **Reduced motion:** no dock rise, no face fade, no radial scale (today's `world.css:2253`), and no line timer
  animation. The timer still runs.
- **Reduced transparency:** `.glass` falls back to `--surface-glass-solid`, as `theme.css:86` does today, now through
  one class.
- **Contrast:** text on glass is `--text` on at least 85% tint; white on `--accent` is 5.2:1 (DESIGN.md).

### 11. The visitor page

- **The Menu:** Map, People (realm up) and Settings.
- **At rest:** the map and the Menu. On a first visit, `#welcome` adds a third piece (Q12), restyled into the family
  (`--radius-lg`, glass, 44 px buttons).
- **`OWNER_IDS`** (`world-public.ts:121`) gains `overview` and `cluster`.
- **`PHONE_IDS`** (`:125`) drops `seecount` and keeps the others. `#seecount` is gone with `#top`.
- **`#under`** goes. Underside stays the carrier's focus act (Q16).

## Every current element and its fate

`h` = `world.html`, `c` = `world.css`, `t` = `theme.css`, `tc` = `town.css`. TS paths are under `websrc`.

| Element | Where | Fate |
|---|---|---|
| `#top` bar | h:38-61; tc:153; c:1405 | removed; pieces in §7 |
| `#menubtn` + phone menu | h:39; c:1426; `world-modes.ts:172-244` | removed (Menu) |
| `.mark` | h:40 | removed |
| `#health` | h:41 | merged into Cluster panel |
| `.chip.world`, `.age`, `.live` | h:42-43,51 | merged into Cluster panel |
| `#statsbtn` + `#groups` | h:44-50; c:1729 | merged into Cluster panel |
| `#mode` Explore | h:52; c:6 | merged into overview head + Menu › Overview (+ Tab) |
| `#search`, `#find`, `#names` | h:53-55 | merged into overview panel's Find |
| `#deck` | h:56; c:335; `world-chrome.ts:172` | kept as `#menu`, bottom left |
| `#helpbtn` | h:57; c:1861 | removed (`?` → Settings › Controls) |
| `#sound` | h:58; c:267 | merged into Settings › Sound |
| `#seecount` | h:59 | merged into overview "Needs you · n" + Menu dot |
| `#account` | h:60; tc:266 | merged into Cluster panel foot (Q16) |
| `#signin` (guest) | c:1423,1931 | merged into Settings head (Q12) |
| `#under` (visitor) | c:268 | removed (carrier focus act) |
| `#sim` | h:62 | kept (sim world only), `.glass` |
| `#crumbs` | h:63; c:1794; t:11 | removed |
| `#rides`, `#ride` | h:64; c:27 | merged into Settings › Controls › Ride |
| `#pvp` | h:64; c:1861 | merged into Settings › Controls |
| `#worldmap` canvas | h:65; c:616,1560,1774 | kept; top right both tiers, 72/160 |
| `#mapbtn` | h:65; c:322 | removed (already hidden) |
| `#compass` N/E/S/W | h:65; c:925 | kept as one N on the rim |
| `#mapgo` | h:65; c:697 | removed (big map pop's Go to it) |
| `#secrets` | h:65; c:324 | kept hidden (cockpit quick row) |
| `#maptip`, `#mappop` | h:66-67; c:725 | kept, `.glass` + family radii |
| `#lensbtn`, `#lensbox` | h:68-69; c:320 | removed (already hidden) |
| `#stack` + slot card + "1 to see" chip | h:70; c:327,1555,1800 | merged into overview Needs you |
| `#realm-banner` | `world-realm.ts:695`; c:970 | merged into a dock line + People note (Q12) |
| `#card`, `#flow` drawers | h:71-72 | kept, `.glass` |
| `#inspector`, `#island`, `#reader`, `#talk`, `#quick`, `#yard` | h:73-77,85-86 | kept, `.glass` |
| `#cockpit` | h:75 | kept; way in Q2 |
| `#keeper` | `world-keeper.ts` | kept, `.glass` |
| `#inbox` (letters) | `world-keeper.ts` | merged into People › Mail |
| `#chat` + 4 chips + log + form + note | h:78-83; c:1986-2107 | merged into People › Chat (+ dock line) |
| `#chatshow`, `#chatcount` | h:84; c:2002 | removed |
| `#chip` (fleet) | h:87 | kept removed/hidden |
| `#sheet` | h:88; tc:1155 | kept, `.glass` |
| `#bottom` deck: `#map`, `#minimap`, `#lenses`, `#legend` | h:90-94 | `#minimap` kept hidden; `#legend` merged into Cluster (Q16) |
| `#portrait`, `#commands`/`#grid`, `#cmds`, `#next` | h:95-99 | kept in the overview for a selection only (Q13) |
| `#line` help line | h:97; `town-phone.ts:201` | removed |
| `#status` debug | h:101 | kept (`?debug=1`) |
| `#tip` | h:102 | kept |
| `#calm-hint` | h:103; c:396; `world-pastime.ts:202` | merged into dock focus face |
| `#pad` Lock, Attack, Jump, Run, Build (foot) | h:104; c:59-118 | removed on foot; kept off foot (bike, ship) |
| `#pad` Advance | h:104 | kept off foot |
| `#pad .skills` (Skill 1, 2, Ult) | h:104; c:120-145 | merged into dock fight face (Skill 1 + Ult, Q3) |
| `#pad .buildpad`, `[data-build]` captions | h:104; c:1659 | kept while building (Q6) |
| `#pad::after` dot | c:155 | kept with the pad only |
| `#crosshair`, `#hitmark` | h:105-106; c:420,440 | kept |
| `#focus` box | h:107; c:2112-2212 | merged into `#dock` focus face |
| `#radial` | h:108; c:2217 | kept; + `arc` layout for the Menu |
| `#hottag` | h:109; c:509 | kept for a fight target only |
| `#said`, `.focus-say` | h:110 | kept |
| `#toast` | h:111; c:1359; `world-modes.ts:147-160` | merged into dock line |
| `.bcard` name cards | c:546 | kept in the overview only |
| `.bl`, `.unit`, `.trunk`, `.room`, `.civ` | tc | kept (scene labels) |
| `.fight-hud .vitals` (HP, energy, Wood/Iron) | `world-fight.ts:415-416,902`; c:995,1733,1818 | HP and energy into dock fight face; Wood/Iron Q7 |
| `.fight-hud .skills` strip | `world-fight.ts:417` | removed (dock fight face) |
| `.fight-boss` | `world-fight.ts:428`; c:1068 | kept as the boss's one label (Q15) |
| `.mobbar` | `world-fight.ts:506`; c:1025 | kept; hidden on the tagged target |
| `.dmg`, `.fight-tick`, `.fight-hurt`, `.fight-downed` | `world-fight.ts` | kept (in-world feedback) |
| `.build-hud` | c:1162 | kept while building, `.glass` + tokens (Q6) |
| `#welcome` | `world-welcome.ts:47`; c:1949 | kept, restyled (Q12) |
| `#controls`, `#wardrobe` dialogs | c:1193-1335 | merged into `#settings` |
| `#loader` | h:21-34 | kept |

## Behaviour changes (only these)

1. **At rest** only the map and the Menu show, on both tiers and both pages. The visitor's first-visit card is the
   exception.
2. **The top bar is gone,** in the overview too. Its pieces move per §7.
3. **The focus box becomes the dock.** On the phone it sits at the bottom, beside the Menu, not over the pad. On the
   phone it also shows inside a building, for parts.
4. **The pastime** (plant, fish) is a dock focus, not `#calm-hint`.
5. **A fight shows the fight face:** HP, Skill 1 and the ultimate. The pad's skills, the vitals card and the desktop
   key strip go.
6. **The PvP flag alone no longer shows the fight controls** on the phone (Q17).
7. **One-time hints show in the dock line on both tiers,** for `LINE_MS`. They were the phone's toast for 7 s and the
   desktop's static line.
8. **Chat lines show one at a time in the dock line.** The log lives in People › Chat. The show/hide setting is gone.
9. **The Menu has 4 entries.** Mail and Chat are People's tabs; Controls and Wardrobe are Settings' tabs. Talk is the
   focus act only.
10. **`?` opens Settings › Controls.**
11. **The ride, PvP and Sound move into Settings.** 1, 2, 3 and P still work.
12. **The phone's pad is hidden on foot.** Jump, Lock and Attack go. A tapped mob is attacked by its second tap, as
    plan/23 change 1 already does.
13. **Go (`#mapgo`) is gone.** The big map's pop holds Go to it.
14. **Running is the default on foot,** walking inside and within `ARRIVE_WALK` of a walk's or route's end. The Run
    button and the double tap go on foot. The ground tap and the engage tap no longer differ in gait.
15. **Shift, the sprint key and L3** sprint only on the bike and the ship (Q4).
16. **`#hottag` names only a fight target.** `.bcard`s never draw in explore. The target's mob bar folds into its tag.
17. **The phone map** moves from the top left to the top right, 72 px. The compass shows N only.
18. **The overview** is the left panel plus the Cluster panel. The crumbs, the To-see chip and card, and the corner map
    go there.
19. **One unread dot,** `--danger`, on the Menu: letters, mail and missed lines.

## Phases: review slices inside one PR

Every commit runs `bun run build` (bun 1.4.x for dist identity), typecheck and `bun test`. Before merge, `go test ./...`
runs in full. Each gate row flips in the commit that changes it, with its reason.

1. **The proof on main.**
   - `dev/hud-gate.mjs` (new) records today's counts and overlaps at 320, 390, 430 and 1440, for each state in §Tests.
   - Rows that will change are marked `changes: N` (this plan's numbers above).
   - Run it twice on main, then break one placement on purpose to see a row fail.
2. **The family.** The tokens, `.glass`, `.glass-btn` and `data-hud`. No layout moves. Flips nothing.
3. **Running by default.** `sprints`, the deletions in §8, and the hint words. Flips 14 and 15.
4. **The dock.**
   - `world-dock.ts`, `#dock` and the line queue;
   - `#toast`, `#calm-hint` and the vitals go;
   - the tag rule.
   - Flips 3, 4, 5, 6, 7 and 16.
5. **The Menu and the panels.**
   - `#menu`, the 4 `GLOBALS` and `arc`;
   - People, with chat moved in;
   - Settings, with the dialogs merged, the ride, PvP and sound;
   - `#menubtn`, the phone menu and `#helpbtn` go.
   - Flips 8, 9, 10, 11 and 19.
6. **The pad on foot, `#mapgo`, `#rides` and the map corner.** Flips 1, 12, 13 and 17.
7. **The overview.** `#top` retired, `#overview`, `#cluster`. Flips 2 and 18.
8. **The visitor page, accessibility, the README controls table and the gate edits.**

**Gates to update** (labelled edits, each with its reason):
- **input-gate:**
  - `clearOf` and `placed` (`dev/input-gate.mjs:296-325`): the box's neighbours become `#dock` versus `#menu` and
    `#worldmap`. `#pad button`, `#mapgo` and `#chat` go;
  - `openCockpit` (`:365-367,606`): through Q2's path;
  - the Menu rows (`:1751-1766`): 4 entries;
  - the visitor Menu (`:1774-1790`): `["Map","People","Settings"]`;
  - the phone Menu (`:1997-2013`): `#menu`, not `#menubtn` → `#deck`;
  - the pad Lock rows (`:279,1024`): the phone foot has no Lock, so they move to a bike or desktop T row;
  - the box rows (`:1813,1830,1886`): `#dock .primary` and `#dock .more`.
- **world-gate:** `#top > *` (`dev/world-gate.mjs:502`) becomes `[data-hud]`; `#mode` (`:533,558`) becomes Tab.
- **onfoot-gate:** `#mode` (`:271-275,342-343,483`) becomes Tab or the panel's Explore; `#helpbtn` (`:365-374`) becomes
  `?`.
- **aim-gate:** `#mode` (`:99`); `#hottag` on a hovered building (`:247-249`) becomes `#dock .name`, and `#hottag` only
  on a mob; `#bottom` and `#rides` (`:20`) become `#dock`.
- **calm-gate:** `#deck` → Cockpit (`:163-165`); the quick row's Sound (`:184,300`) becomes Settings › Sound.
- **cockpit-gate:** `#deck` → Cockpit (`:230-241`), via Q2's path.
- **public-gate:** `#hottag` health (`:165`) holds, on a mob; `#ride` (`:184-186`) becomes Settings › Ride.
- **Already failing on main** (plan/23 build notes), not this PR's:
  - world-gate: Rimfall, Underside, phone underside;
  - onfoot and aim: the phone's hidden `#mode` (the row goes with `#mode` here);
  - public: `/api/land`.

**Checks, under the lean-checks rule:**
- In every commit: unit tests and dist identity.
- Once, before review: one full manual GPU gate round of world, onfoot, aim, calm, cockpit, public, input and hud.
- `ci-local --gates` covers world and public only, so input and hud run by hand.

## Tests

**New tests,** each with a mutation and a positive control:
- **`world-dock.test.ts`:**
  - `dockOf`'s order: covered and the overview give rest; fight beats line beats focus; fight on foot only;
  - the line queue:
    - one at a time;
    - a newer line replaces the one showing;
    - a hint waits behind chat;
    - a fight holds the line;
    - a timeout unseen adds to unread;
    - a hint is marked seen only once shown.
- **`world-ride-step.test.ts` (new):** `sprints` over foot and ride, inside, `toEnd` around `ARRIVE_WALK`, `held` off
  foot.
- **`world-actions.test.ts` (edit):** `GLOBALS` is exactly Map, People, Overview and Settings on the owner page, and Map,
  People (realm up) and Settings for a visitor. Ids are unique.
- **`world-chat.test.ts` (edit):** no fade or show state; unread counts while the tab is not shown; `KEEP` trims.
- **`world-modes.test.ts` (edit):** the menu and the toast go. `hintQueue` moves with its tests to `world-dock.test.ts`.
- **`world-focus.test.ts` (edit):** the pastime rule sits between carried and tap.
- **`world-focus-dom.test.ts` (edit):** no `place()`, and no `--pad-h`.
- **`world-radial.test.ts` (edit):** `arc(n)` puts every item above the centre, with none overlapping at n = 4.
- **`world-input.test.ts` (edit):** `padShows` is false on foot for a touch phone.
- **`world-hud-tokens.test.ts` (new, the guard).** It reads `world.css` and `theme.css` and checks the rules for
  `#dock`, `#menu`, `#people`, `#settings`, `#overview`, `#cluster`, `.glass` and `.glass-btn`:
  - `border-radius` only through `var(--radius-*)`, or 50% for the map;
  - no `rem`;
  - no raw colour;
  - sizes through `--btn` or `--btn-fight`.
  - Each banned form gets a mutation.

**`dev/hud-gate.mjs` (new),** on `dev/stack-realm.sh`:
- **Sizes:** 320×568, 390×844 and 430×932 with touch, and 1440×900 with a mouse.
- **States:**
  - rest;
  - near a building;
  - inside, by a stair;
  - a fight, forced by a stub mob hit (`HURT_MS`, as the inventory notes);
  - a chat line;
  - the Menu open;
  - People open;
  - Settings open;
  - the overview;
  - the visitor page at rest.
- **Each row asserts:**
  - **at rest, `visible('[data-hud]').length === 2`** (`#worldmap`, `#menu`). The visitor's first visit is 3, with
    `#welcome`;
  - near: 3; fight: 3 plus the target's `#hottag`; line: 3; inside: 3;
  - no two visible `[data-hud]` boxes overlap, except the radial's scrim over everything and a panel over the dock;
  - every box is on screen, and the dock's foot is at or above `env(safe-area-inset-bottom)`;
  - every `button` in a `[data-hud]` is at least 44×44, and the phone fight buttons are 56×56;
  - computed `border-radius`: containers 16, buttons 12;
  - one label: the focused target's name appears in at most one of `#dock .name`, `#hottag` and a visible `.bcard`.
- **A pixel check:** a screenshot per state and size, diffed against the PR's own first run (no baseline on main). It
  runs with `--update` to accept.

## Hands-on QA (`dev/stack-realm.sh`, phone emulator 390×844 touch, then 320×568; desktop 1440×900)

**Phone:**
1. At rest on foot: only the map (top right) and the Menu (bottom left). Walk a long route by taps. The rider runs and
   slows to a walk on arrival. Inside, it walks.
2. Tap a hall: the dock rises with "Look" and ⋯.
   - A long-press on Look, slid to Enter, goes in, and no stray click fires.
   - Walk away: the dock leaves.
3. Inside, stand on a stair: the dock shows it. Tap a book: it reads at once (Q11).
4. A wolf hunts you:
   - the fight face shows HP, Skill 1 and the ultimate under the right thumb;
   - tap the wolf: the first tap focuses, the second attacks;
   - the ring fills as energy comes;
   - the fight ends: the dock leaves.
5. A second page says something in chat:
   - it slides into the dock for about 5 s, and a tap opens People › Chat;
   - during a fight it waits, then shows after.
6. The Menu: 4 items on an arc, the world dimmed. Then:
   - People: the tabs, a line typed and sent;
   - Settings: change the ride to the bike. The pad appears as today; ride back to foot and it goes;
   - toggle PvP and Sound.
7. A first-visit hint shows once in the dock, and never again after a reload.
8. At 320 px: the dock's name ellipses, the primary and ⋯ stay 44 px, and nothing overlaps the Menu.
9. A guest on the visitor page: the welcome card; the Menu shows Map, People and Settings; Sign in sits in Settings.

**Desktop:**
1. At rest: the map and the Menu only. Hover the Library: the dock shows "Look `E`" and the building has no tag. Hold E:
   the radial opens.
2. Enter opens People › Chat with the input focused. Esc closes it, and the keys go back to the canvas.
3. Tab: the overview panel appears.
   - Find with `/`, and Needs you → Fly there `Enter`.
   - Cluster and districts › shows the numbers and the villages.
   - Tab back.
4. `?` opens Settings › Controls with the key list.
5. Shift on foot changes nothing; on the bike it sprints (Q4).
6. In a fight: the HP row and two buttons with key caps (Q8).
7. Keyboard only, with VoiceOver:
   - Menu, then its items read aloud, Esc, and focus returns to Menu;
   - the dock's primary and ⋯ are reachable and named;
   - a new chat line is spoken once.
8. Reduced motion: the dock appears and leaves with no rise or fade. Reduced transparency: solid panels.

## Risks

- **The phone loses manual Jump on foot.** A tap walk over a ledge relies on the route's auto-jump (`routeHold`,
  `world-view.ts:1544`). A free tap walk into a ledge stalls. QA step 1 walks long routes on purpose.
- **Running everywhere changes feel.**
  - Collisions come faster, and C always slides (Q4).
  - `ARRIVE_WALK` is a knob, set by feel.
  - The realm's physics sees `sprint: true` far more often. That is the same input range as today's Run toggle.
- **The fight face flickers** if `inFight` toggles at the hunt radius. The face keeps `HURT_MS` hysteresis by reading
  `hurtAt` too. If it still flickers, add `FIGHT_HOLD_MS`.
- **Chat lines show 5 s, one at a time.** A busy chat drops lines from view, not from People. They count as unread.
- **The desktop deck** (Q13) is the least specified part. Keeping it for the overview selection is the safe default.
- **A big PR.** Each slice is a commit with its own dist and green tests, as plan/23 did.
- **Plan/22 P3 and P4 are in flight.**
  - The P3 UI branch writes `#deck`'s dot and a `#mail` chrome. It rebases onto `#menu` and People › Mail.
  - P4's mic needs a new place (§12).
- **The gates already failing on main** may hide a regression. The hud-gate's own rows do not depend on them.

## 12. Plan/22 hooks

- **P3 mail:** People › Mail is `world-mail.ts`'s body. Unread feeds the Menu dot. The rider radial's Message opens
  Write. A news item enters the dock line.
- **P4 voice:** the mic is not "near Jump" on foot. Its place (the dock's fight face, a Settings toggle, or a held Menu
  gesture) is the owner's call when P4 starts. The PTT/OPEN choice goes in Settings › Sound.
- **P5 party:** People › Party holds the members, invite and party chat. Invite and Report join the rider radial.

## Not doing

- Bike and ship controls (a separate discussion): their pad, Advance, Run and climb stay.
- The mail server (#250), voice and party features beyond their hooks.
- New themes, the 3D world, or the in-world signs.
- Rebinding Esc, M, Enter, `?` or Tab.
- The shooter zone, or a pad hover.
- A focus-cycle key, or automatic facing focus.
- The town page.

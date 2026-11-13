# 23 · One input system: the router, focus and the radial

Status: approved, 2026-11-11 (rev 4, after doubt cycles 1–3 and the owner's answers (a)–(f); D1–D5 approved as written;
changes 8, 10 and 12 folded into the one PR by the owner's word).

Sources:
- the owner interview (memory `interaction-system-intent`);
- the owner's answers (a)–(f).

What it changes in other plans:
- It amends plan/22 §7: the player menu, the mic key and the inbox become registry entries, not new listeners.
- It supersedes the "never for a mob" line in `hud-placement-intent`.

## Context

The world page's input grew one listener per feature, and whichever listener registered first wins. Every claim was
checked on main at `bff987b`; paths are under `cmd/game/websrc` unless given.

### Keys

- **11 window key listeners, plus blur:**
  - `world.ts`: `:2221` (?), `:2227` (minimap), `:2234` (B, R, X, Q, G, T, P, then `view.key`), `:2289` (`ui.command`).
  - `page.ts:92` (`keys()`).
  - `world-view.ts`: `:1161` (keyup) and `:1172` (keydown, sprint), both in the bubble phase.
  - Capture phase: `world-view.ts:1657` (handBack), `world-chat.ts:193` (Enter), `world-input.ts:109,118` (bindE).
  - Blur and visibility: `world-view.ts:1185`, `world-input.ts:128`, and `visibilitychange` at `world-view.ts:1186`.
- **They win with `stopImmediatePropagation`**: 10 calls, 9 in `world.ts:2230-2296` and 1 in `world-chat.ts:200`.
- **Modifiers.** Every handler steps aside for Ctrl, Meta and Alt: `page.ts:94`, `world.ts:2222,2235,2290`,
  `world-minimap.ts:952`, `world-chat.ts:196`, `world-input.ts:104`, `world-view.ts:1137`.
  - Shift has rules of its own: it is the sprint (`:1176`); Shift+Tab moves focus (`:1104`); Shift+1 is still read as a
    digit through its code (`:1150`).
- **Time.** `e.timeStamp` drives the roll's double tap (`:1127,1141`) and E's hold (`world-input.ts:114,124`).

### Keys with two meanings

- **Exploring outside,** `view.key` swallows every character and Enter (`world-view.ts:1159`, `world.ts:2285`). So the
  deck letters K (Talk) and I (Letters) are dead on foot.
- **Inside,** letters run the commands (`:1148`). The exception is C, which is the crouch (`:1137-1146`).
- **E** is both the act (`bindE`) and the deck's Enter (`world-chrome.ts:347`).
- **C, E, P, Q, R and X** are both `KEYS` defaults and console or deck letters.
- **In the overview:**
  - Q runs `skill2`, so a Mage's spell cycles (`world.ts:1422-1425`).
  - P flips PvP.
  - G is swallowed and does nothing (`:862`).
- **On a focused button,** the game keys still act, because they skip only fields (`world.ts:2237`).

### Esc, in today's order

1. **The minimap:** its pop, then the big map. One Esc also blurs a focused field (`world-minimap.ts:952-963`).
2. **`view.esc`** (`world-view.ts:1074-1092`):
   - handBack;
   - the pointer-lock release, or its grace window;
   - if a control is focused, focus moves to the canvas and the Esc goes on;
   - in a field, it stops;
   - inside a building, when `busy()` is false, the rider leaves. `busy` is `overlaysOpen()` (`world.ts:1587,1731-1741`),
     and talk and the drawer are not part of it.
3. **`page.ts:97-108`:** blur the field; else the tip; else the first open chrome in `chromes` order
   (`world.ts:1957-1961`); else up a level.

The rider menu is not in `chromes`.

### "Busy", the field test and the pointer

- **"Busy" is worked out three ways:**
  - `padSync` (`world-view.ts:640`): busy, talk or the sheet;
  - `skillMay` (`:1198`): busy or talk;
  - `lookMay` (`:1214`): busy, talk or the drawer, plus the ship.
- **The field test** is written out 7 times, plus the helper at `world-input.ts:169`.
- **The canvas has 23 listeners:** 6 in `world-view.ts`, 10 in `world.ts`, 1 in `world-input.ts`, 3 in `page.ts`, 2 in
  `world-cards.ts` (with a `setTimeout` cursor) and 1 in `world-look.ts`. The middle-button lock uses mouse events,
  because a chorded button sends no `pointerdown`.
- **Outside-press closers: 4.** They are `world-minimap.ts:632`, `world-modes.ts:234`, `town-phone.ts:152` and
  `world-rider-menu.ts:68`. The minimap's `stopPropagation` (`:639`) starves the others, so a press outside the big map
  closes only the map.

### Clicks and taps

- **The click chain** is `own(clickWay)`, then `dragged || cards.pick || targetClick || stone/guild || groundPick`
  (`world.ts:2487-2505`).
- **`targetClick`** runs on every ride, the ship included. It dispatches `ACTS` through `ON`/`clickOf`.
- **On a touch screen,** only buildings, residents, riders, mobs and the boss are targets (`kindsFor`,
  `world-targets.ts:234`).
- **`ACTS.select`** opens the drawer (`world-select.ts:85`), and on a phone the sheet (`town-phone.ts:91-100`). That
  hides `#pad`.
- **A tap on a mob** calls `engageAct(h, true)` (`world.ts:1192-1221`):
  - `:1196` refuses off foot, while building, or past `LOCK_FAR` 60; the tap then walks.
  - `:1199-1205`: `was`, `engageEnd()`, `quick`, `tapMob`/`tapMobAt`, `lockId`/`lockedAt`.
  - `:1206-1209`: in reach, it fires.
  - `:1210-1219`: when `was` holds, it engages.

### E (`world.ts:2535-2561`)

E runs in this order:
1. `view.act()`: the inside stair or machine, the under-disc stair up, or the cave mouth.
2. Inside a building: stop.
3. Carrying something: the hot item, else drop, or throw when held.
4. `actAt(pointer, …)`: the hovered target.
5. `pickNear() || guildNear() || pastime.act()`.

### The pad and the visitor page

- **The pad** replays buttons as synthetic KeyboardEvents and fakes the pointer lock.
- **Y** is the ultimate on a tap and the act on a hold, on foot. Off foot, a press is the act at once (`ultHold`).
- **All 16 pad buttons are taken.**
- **The visitor page** removes `#deck` (`world-public.ts:121`) and enters no doors.

The goal: **one place that reads input, fewer handlers, better code**, with the focus, the box and the radial built on
top.

## Decisions (owner-confirmed)

1. **One deep PR,** measured by handler counts and by "every input path lands in exactly one place" (§9).
2. **One context stack,** top first: modal > typing > overlay > **globals** (below overlay) > focus > build >
   combat/ride > view > tail. One window `keydown` and one `keyup`, one canvas pointer pipeline, and no
   `stopImmediatePropagation`.
3. **Focus comes only from** a tap (touch), a hover (the desktop mouse), a part where the rider stands, or the carried
   item. There is no automatic facing (answer e).
   - The bottom box shows the name, the primary and "hold for more".
   - Tap E, the box, or pad Y for the primary; hold any of them for the radial.
   - Walking away clears it.
4. **Every first tap only focuses,** on every ride, the ship included. The second tap, or the box, runs the primary
   (answers c and f).
5. **E with no focus is today's fallback, unchanged.** Pad Y is E.
6. **The globals** work everywhere except in a field or on a focused control: Esc, M, ? and Menu. Enter opens the chat
   only while exploring outside; inside and in the overview, Enter is Next (answer a).
7. **Tab** flips the view only from the canvas, per WCAG 2.1.2. View mode is also a Menu entry (answer b).
8. **Console and deck letters run only in the overview.** Inside, the Menu's Mail and Talk replace them (answer d).
   1, 2 and 3 still switch the ride.
9. **Two registries:** `INTERACTABLES` and `GLOBALS`. Plan/22's voice, chat, mail and party plug in as entries.

### Open choices for the owner

- **D1:** the Menu key is `KeyH`. It can be rebound.
- **D2:** the pad's ultimate moves to D-pad up, on foot. The row reads "Ultimate (building: turn; ship: climb)".
- **D3:** `#deck` becomes the Menu button on both pages (§7).
- **D4:** Start opens the Menu, which holds View mode. Back stays the map.
- **D5:** a building's primary is Look at any distance, as today's tap and E do. Enter goes in its radial, along with the
  Enter deck cell and walking into the door. A rider's primary opens its radial, as today's tap opens its popup. Neither
  changes from today.

## Shape

### 1. Types (`town-types.ts`)

```ts
export type Outcome = 'handled' | 'pass' | 'swallow'; // handled: preventDefault + stop; swallow: stop, no preventDefault
export type Target = 'canvas' | 'page' | 'control' | 'field';
export interface KeyIn {
  key: string; code: string; phase: 'down' | 'up'; repeat: boolean; timeStamp: number;
  shift: boolean; ctrl: boolean; meta: boolean; alt: boolean; at: Target;
}
export type Gesture = 'tap' | 'long' | 'drag' | 'pinch' | 'wheel' | 'right' | 'middle' | 'hover';
export interface PointerIn { g: Gesture; x: number; y: number; touch: boolean; heldMs?: number; dx?: number; dy?: number }
export type PadIn = { press: PadAction | 'back' } | { hold: 'act'; phase: 'tap' | 'hold' | 'release' } | { stick: [number, number] };
export interface Context {
  id: string; active(): boolean;
  key?(k: KeyIn): Outcome; pointer?(p: PointerIn): Outcome; pad?(p: PadIn): Outcome;
}
export interface Focus { kind: FocusKind; id: string; at: XZ3; how: 'tap' | 'hover' | 'part' | 'carried'; d0: number }
export interface Act { id: string; label: string; when?(f: Focus): boolean; run(f: Focus): void }
export interface Interactable { kind: FocusKind; acts: readonly Act[] } // first available = primary
export interface GlobalCmd { id: string; label: string; when(): boolean; run(): void }
```

- **`FocusKind`** is `KindId` minus `node`, plus `stone`, `guild`, `part` and `carried`.
- **The DOM event** stays in the router.
- **Every context passes on Ctrl, Meta and Alt.** Shift keeps its three rules.
- **Each context reads `isKey` itself.** The overview context takes letters for the console before any bound action.

### 2. `world-router.ts` (new): one reader

- **Listeners:**
  - window `keydown`, bubble phase, so element handlers act first;
  - window `keyup`, capture phase. A keyup is always `pass`: it releases held intents and fires E's tap, and it never
    calls `preventDefault` or stops propagation;
  - `blur` and `visibilitychange`, which release everything;
  - one window capture `pointerdown`, for the dismiss list (§9, in today's order, starving kept) and handBack;
  - on the canvas: `pointerdown/move/up/cancel/leave`, `wheel` and `contextmenu`, plus `mousedown`/`mouseup` for the
    middle button only.
- **The target** comes from `e.target`, never from `activeElement`.
- **Held intents:** steer, jump, crouch and sprint. A keyup, a pad release, a blur or `releaseAll()` always releases
  them. `onBindings` calls `releaseAll()`.
- **Gestures:** one classifier. It keeps today's constants, the capture try, and the `aim` lock path.
- **One cursor writer.** The three others are deleted.
- **`lastSrc`** is `'mouse' | 'pad'`. Only real mouse movement sets `mouse`, and keys never change it.
- **Digits** are read by `e.key` or by the `e.code` `DigitN`.
- **The pad:** `router.pad(...)`, with no synthetic KeyboardEvents.

### 3. The contexts (one frozen array, registered in `world.ts`)

1. **modal:** active when Controls or Wardrobe is open. Swallows every key and pointer event.
2. **typing:** active when `k.at === 'field'`.
   - Esc blurs the field, focuses the canvas and closes the big map (`handled`).
   - Every other key goes down to the field (`swallow`).
3. **overlay:** active with the radial, the minimap pop or big map, the pointer lock or its grace, or `busy()`.
   - Esc, in order: radial, pop, big map, lock, tip, then the first open chrome in `chromes` order.
   - M while the big map is open.
   - While the radial is open: its navigation.
4. **globals:** active when not typing and `k.at !== 'control'`.
   - M, ? and Menu.
   - Enter while exploring outside: the chat, or `swallow` when the realm is down.
5. **focus:** active in explore or inside, when not flying.
   - `act` down and up: the E rule.
   - A touch tap on a focusable target.
   - The pad's act hold.
   - Esc clears a tap or hover focus.
6. **build:** `turn`, `down`, `skill2`, and the pad's `ult`.
7. **combat/ride:** in explore outside only. Claims `skill2`, `ult`, `lock`, `pvp`, `build`, the mouse attack, `right`,
   `middle`, `long`, and 1/2/3.
8. **view:** always active.
   - Tab from the canvas.
   - WASD and the arrows, as today.
   - **Crouch and the double-tap roll, in explore and inside.**
   - Wheel, drag and pinch.
   - A tap: walk, clear the focus and call `engageEnd()`; or the overview's pick chain.
   - In the overview: letters first, then Enter as Next and `/` as search.
   - Inside: Enter is Next; Esc leaves the building while `!busy()`.
9. **tail:** Esc closes the tip, then the first open chrome, else goes up a level.

**Before every Esc,** handBack runs. If a control has focus, the canvas takes it and the Esc goes on.

**On a focused control,** only Enter, Space, E and the globals step aside. Every bound game key (B, R, X, Q, G, T, P,
1/2/3, WASD, C) still acts there, as today, and so do Esc and the overview letters.

**The Esc order is the stack order and nothing else.** Inside a building, leaving still beats talk and the drawer.

### 4. The predicates: named once, nothing merged

- **`busy()`** is today's `overlaysOpen()` plus the radial. It gates Esc, `skillMay` and `lookMay`, and it releases the
  ship's pointer lock when the Menu radial opens.
- **`covered()`** is `busy()`, minus the tip and minus the radial, plus talk. It hides the box and blocks a new focus.
  The radial never clears the focus it acts on and never hides its own box. The box and its ⋯ opener stay mounted while
  their radial is open, so the touch capture survives.
- **`padFree()`** is padSync's rule without the radial.

### 5. `world-focus.ts` (new)

**`focusOf`** takes the first rule that matches:
1. **None:** in the overview, during a door flight, or when `covered()`.
2. **Part:** what `view.act()` would take where the rider stands, judged by `actHint`'s tests. Its words become the name
   and the primary, and its toast is dropped while the box shows.
3. **Carried:** the carried item, unless another item is hovered.
4. **Tap:** the last tapped target. It clears on another tap, a ground tap, Esc, or the rider moving more than
   `LEAVE_MARGIN` (2) farther from it than at the tap (`d0`).
5. **Hover:** when `lastSrc === 'mouse'`, `targets.hot()` minus `node`.
   - It holds for `HOVER_KEEP_MS` (800) after the pointer leaves the target, and indefinitely while the pointer is over
     `#focus` or `#radial`.
   - It clears on another hover, on Esc, or under the `LEAVE_MARGIN` rule.
6. **None.**

**Mobs and the boss are focusable only where `engageAct` acts:** on foot, not building, within `LOCK_FAR`. Anywhere else
a tap on one walks or rides there, as today.

**The E rule.** It also applies to Y and the box.
1. `view.act()`.
2. Inside a building: done.
3. Carrying something: today's logic. A tap drops; a hold opens the radial, which holds Throw.
4. A tap or hover focus: its primary.
5. Otherwise `pickNear() || guildNear() || pastime.act()`, with no box.

**Taps.** The focus context claims a tap when `targets.at(x, y)` is a focusable touch kind.
- The first tap focuses.
- A second tap on the same target runs its primary.
- A tap on another target moves the focus there.
- Any other tap goes to the view.

**Mobs.**
- **First tap:** focus, plus `engageAct` with a new `select` flag that returns after `:1205`. The state is the same as
  today, with no shot, and `lockBy` is `'tap'`.
- **The primary** (second tap, E, Y or the box) is `engageAct(h, true)`. In reach it fires. Out of reach, with `was`, it
  engages, at a run when the second tap comes within `DOUBLE_MS`. A tap on a mob that is already locked counts as the
  second tap.
- **Clearing a tapped mob's focus** sets `lockId = null`, unless it was locked by key (T, LT, the middle button,
  `lockToggle`) or the engage has begun.

**The mark.** `targets.mark(focus)` draws the existing ring, hull and `#hottag`.

**The box (`#focus`)** shows exactly when there is a focus, `!covered()`, and the view is explore outside or inside. On
a phone it also hides while the sheet is open.
- **Content:** the name, a primary `<button>` ("Look (E)"), and a ⋯ `<button aria-haspopup="menu">`.
- **Placement:** one element, moved by `append` into `#bottom` on desktop or into `document.body` on a phone, at the
  `TOUCH_PHONE` media change. It never sits inside `#pad`, so the pad's at-rest rules and button rules never reach it.
- **Desktop:**
  - the first row of `#bottom`;
  - `deckOpen` loses `hotResident` and gains `|| focus`;
  - `SHOWS.talk` becomes `talking || (building && resident)`;
  - `body[data-focus] #commands` hides in explore outside unless help is on.
- **Phone:** a `ResizeObserver` on `#pad` sets `--pad-h` on `body`.

  ```css
  @media (pointer: coarse) and (max-width: 1199px) {
    #focus { position: fixed; z-index: 3; width: 148px; right: max(12px, env(safe-area-inset-right, 0px));
      bottom: calc(max(16px, env(safe-area-inset-bottom, 0px)) + var(--pad-h, 0px) + var(--space-2)); }
    body[data-ride="foot"] #focus {
      bottom: calc(max(16px, env(safe-area-inset-bottom, 0px)) + var(--pad-h, 0px) + var(--space-2) + 44px + var(--space-2)); }
    body:is([data-sheet="half"], [data-sheet="full"]) #focus, body:has(#pad[hidden]) #focus { display: none; }
    body[data-focus] #chat { width: min(320px, calc(100vw - 180px)); }
    #focus button { touch-action: none; user-select: none; -webkit-user-select: none; -webkit-touch-callout: none; }
  }
  ```

  It stays clear of the corner map, Go and the left half. Inside a building there is no pad, so no box.
- **Accessibility:**
  - When the box vanishes while it has keyboard focus, the canvas takes focus.
  - A polite live region speaks on a tap, or on a hover held still for 600 ms.
  - Block's "Try again in a moment" shows in the box for `NOTE_MS`.

### 6. `world-radial.ts` (new): one component for focus and the Menu

- **Pure functions:** `sectorOf` and `layout`, n ≤ 8.
- **DOM:** `#radial role="menu"`, holding `menuitem` buttons with full names.
- **Placement:** at the cursor, the thumb, the box, or the screen centre, clamped to the viewport. Sectors are measured
  from the drawn centre. Nothing highlights until the input has moved past `dead`.
- **`tapHold`** is one timer for E, Y and the box button, on every ride. It replaces `ultHold`.
  - A tap released before 400 ms runs the primary, or the E fallback.
  - At 400 ms it opens the radial. Nothing fires at the press.
  - Releasing after a hold picks the highlighted item. Released in the dead zone without moving, the radial stays open as
    a menu: arrows or Tab move; Enter, Space, A or a click picks; Esc, B or a click outside closes.
- **Mouse:** the direction you move highlights an item.
- **Touch:** the box button keeps its `pointerId` with `setPointerCapture`. The click after a hold is suppressed. Slide,
  then lift.
- **Pad:** the right stick highlights, and the camera holds still.
- **Keyboard:** the ⋯ button, or E held and then released. DOM focus follows the highlight, then returns to the opener.
- **Ship:** the Menu radial releases the pointer lock first. While locked, the crosshair marks the hovered target.
- **Reduced motion:** no scale and no fade.

### 7. `world-actions.ts` (new): the registries. Each `run` is today's handler, moved

| Kind | Acts (first available = primary) | Today's handler |
|---|---|---|
| building | Look · Enter · Go · Guide me here · Talk (when it has a resident) | `selection.select(id,'aim')`, `view.enter`, `view.followTo`, the guide, `talkTo` |
| resident | Talk · Look | `talkTo` |
| garden | Plant · Look | `world.ts:2431` |
| water | Fish | `:2432` |
| item | Pick up | `:2433` |
| carried | Drop · Throw | `:2551-2552` |
| mob, boss | Engage · Lock · Inspect | `engageAct(h, true)`, `lockToggle(id)` |
| rider | Player… (opens its radial: Mute · Block; P5 adds Invite, Message, Report) | `riderActions` |
| carrier | Underside | `:2435` |
| stone, guild | their clicks | `:1920,1929` |
| part | stair, machine, cave mouth | `view.act()` |

**`GLOBALS`,** each shown while its `when()` holds:
- Cockpit (owner)
- Mail (owner; P3 swaps in the realm mail)
- Chat (realm up, exploring outside)
- Map
- Controls
- Wardrobe (realm up)
- View mode (owner page only)
- Talk (inside a building with a resident)

The visitor's Menu shows Chat, Map, Controls and Wardrobe. Its Talk never shows, because visitors enter no doors today.

Not in `GLOBALS`: Fleet and Secrets. The cockpit quick row is unchanged.

**`#deck` becomes the Menu on both pages:**
- Its click (`world-chrome.ts:169-172`) opens the Menu radial instead of `cockpit.toggle()`.
- `world.html:56` gets `aria-label="Menu (H)"`, `aria-haspopup="menu"` and a Menu title, and loses `aria-controls`.
- Its phone label becomes `['deck', 'Menu']` (`world-modes.ts:221`).
- `world-public.ts:121` drops `'deck'` from `OWNER_IDS`.

**Deleted:** `ON`, `clickOf`, `takes` and `ACTS`. **Kept:** `KINDS`.

### 8. Bindings and migration

- **`KEYS`:** add `menu` (KeyH). `act` is relabelled "Interact (hold: more)".
- **`PADS`:** add `act` (Y) and `menu` (Start). `ult` defaults to D-pad up. The `turn` and `mode` rows go.
- **`parse`:** `MOVED_PAD` drops `ult: 'act'` and gains `ult: 'turn'`, and a stored `ult` wins. A stored `mode` is
  dropped. Where two rows land on one button, the pad half is dropped.
- **Deleted:** `tapOf` and `keyInit`.
- **Unchanged:** the `COMMAND_LETTERS` refusals.

### 9. Every input path lands in exactly one place

| Today | After |
|---|---|
| `world.ts:2221` (?) | globals |
| `world.ts:2227`, `world-minimap.ts:950-971` (M, Esc) | overlay (Esc), globals (M) |
| `world.ts:2234-2286` (B, R, X, Q, G, T, P) | build, combat/ride |
| `world.ts:2289-2298` (command letters) | view (overview) |
| `page.ts:92-131` (Esc, /, Enter, letters) | overlay, tail, view |
| `world-view.ts:1099-1160` (`key`, `esc`) | view, combat/ride, overlay |
| `world-view.ts:1161-1186` (keyup, sprint, blur, visibility) | the router's held intents |
| `world-view.ts:1647,1650,1657` (handBack) | the router's handBack step |
| `world-chat.ts:193` (Enter) | globals |
| `world-input.ts:99-137` (bindE) | focus (the E rule) |
| `world-view.ts:1231-1314` (canvas gestures) | router gestures, then view and combat |
| `world.ts:1025-1067` (middle button, charge) | router gestures, then combat |
| `world.ts:2342-2347,2455-2462` (pointer state, cursor) | the router |
| `page.ts:136-166`, `world-input.ts:64-68` (click, cursor) | router gestures, then the contexts |
| `world.ts:2487-2528` (`clickAt`, clickWay) | combat (mouse attack), focus (touch tap), view (walk, pick) |
| `world.ts:2535-2561` (E) | focus (the E rule) |
| `world-cards.ts:198-205` (hover) | the router's `hover`, then `cards.hover` |
| the four outside-press closers | the dismiss list, in that order; the big map still starves the rest |
| `world-view.ts:1594-1610,1696-1726` (pad taps) | `router.pad` |
| `world-view.ts:1320-1334` (refocus after a click) | moved unchanged into the router |

| Count | Today | After |
|---|---|---|
| Window key listeners | 11 | 2 |
| Canvas listeners | 23 | 10 |
| Outside-press listeners | 4 | 1 |
| `stopImmediatePropagation` | 10 | 0 |
| Field-test copies | 8 | 1 |

These stay: the minimap's canvas listeners, MapControls, the `gamepadconnected` listeners, the dialogs' pad polls, and
one `ResizeObserver`.

## Phases: review slices inside one PR

Every commit runs `bun run build`, typecheck and `bun test`. There is no temporary compatibility layer. Each input-gate
row flips in the commit that changes it, with its reason.

1. **The proof on main.** `dev/input-gate.mjs` holds today's rows:
   - every Esc step, including talk and the drawer open inside a building;
   - a field plus the big map;
   - the find's Esc;
   - R, X and C in the overview; C in explore and inside;
   - Ctrl+R in the overview;
   - B, Q and 1 on a focused button;
   - a press outside the big map;
   - E to plant, E at a cave mouth, E carrying.

   Run it twice on main, then break one handler on purpose to see a row fail. Rows that change are marked `changes:`.
2. **Types and registries,** not wired yet. No rows flip.
3. **The key router and the contexts.** `bindE` moves with today's behaviour: a tap on keyup, a hold to throw,
   `held('act')` for E+click. Flips changes 3, 8 and 12.
4. **The pointer pipeline and the dismiss list.** E+click ports through `held('act')`. No rows flip.
5. **The pad as intents.** No rows flip.
6. **Focus, the box, the radial, `tapHold`, the Menu, the pad layout and the migration.** Flips changes 1, 2, 4, 5, 6,
   7, 9, 10, 11, 13, 14 and 15. The help line and the README controls table are updated.

**Checks, under the lean-checks rule:**
- In every commit: unit tests and dist identity.
- Once, before review: one full manual GPU gate round of world, onfoot, aim, calm, cockpit, public and input.
- `ci-local --gates` covers only world and public, so input-gate runs by hand.

## Behaviour changes (only these)

1. **Every first tap only focuses,** on every ride, the ship included. That covers buildings, residents, in-reach mobs
   (locked, not fired), riders, and targets tapped from the ship. The second tap, or the box, does what the first tap
   did. A mob tap that `engageAct` refuses still walks.
2. **E on a hovered or tapped mob** calls `engageAct(h, true)`.
3. **Letters run only in the overview.** Inside, the console letters, I and K are lost. The Menu's Mail and Talk replace
   them, and C stays the crouch.
4. **E+click is retired.** Hover plus E covers it.
5. **Holding E opens the radial.** Throw becomes the carried item's entry; a tap still drops.
6. **The pad:**
   - Y is E on every ride (tap for the primary, hold for the radial).
   - D-pad up is the ultimate.
   - Start opens the Menu, which holds View mode.
7. **A tap or hover focus clears** on a ground tap, Esc, another tap, or walking away. A tapped mob's lock goes with it.
8. **In the overview, Q and P stop working,** and G is no longer swallowed.
9. **In explore, a focus folds the deck cells into the box.** `hotResident` leaves `deckOpen` and `SHOWS.talk`.
10. **The rider popup becomes the rider's radial,** and Esc closes it.
11. **`#deck` is the Menu on both pages.** The visitor page gets it back.
12. **On a focused control, M, ? and Menu step aside.** The find's Esc inside a building no longer leaves the building.
13. **A part shows in the box.** Its toast is dropped while the box shows.
14. **The carried item shows in the box.**
15. **Block's "try again" moves into the box.**

Changes 8, 10 and 12 fix code that this PR deletes. The owner folded them into this PR (2026-11-11).

## Tests

**New tests,** each with a mutation and a positive control:
- **`world-router.test.ts`:**
  - the stack order and the three outcomes;
  - every keyup passes;
  - the target is read from `e.target`;
  - the control guard: Enter, Space, E and the globals step aside, while B, Q and 1 act;
  - every modifier passes (Ctrl+R);
  - Shift+Tab, `timeStamp`, and releases;
  - R, X and C by context.
- **`world-focus.test.ts`:**
  - `focusOf`'s order;
  - `node` excluded;
  - mobs focusable only where `engageAct` acts;
  - `d0 + LEAVE_MARGIN`, `HOVER_KEEP_MS` and the freeze;
  - `lastSrc`;
  - the radial never clears the focus or hides the box, and the tip never hides the box;
  - the E fallback order;
  - the mob select state equals `engageAct` up to `:1205`;
  - a `was` tap engages;
  - a `'tap'` lock against a `'key'` lock.
- **`world-radial.test.ts`:** `sectorOf`, the dead zone, a clamped centre, and the radial staying open after a dead-zone
  release.
- **`world-actions.test.ts`:**
  - acts per kind, and the primary is the first available;
  - Look for buildings, and the radial for riders;
  - `GLOBALS.when` on the owner and visitor pages, at most 8 entries, unique ids.
- **`world-gamepad.test.ts`:** `tapHold`.
- **The guard,** an AST check: exactly one window `keydown` and one `keyup`; none on `document`; no
  `stopImmediatePropagation`; no field selector outside `world-input.ts`. Each banned form gets a mutation.

**Edited tests,** with the reason given:
- `world-input.test.ts`
- `world-gamepad.test.ts`
- `world-bindings-migrate.test.ts`
- `world-bindings.test.ts`
- `world-chat.test.ts`
- `world-deck.test.ts`
- `world-rider-menu.test.ts`

**Gates:**
- world and public: kept.
- onfoot and aim: E+click becomes hover, then E.
- calm: Menu, then Cockpit; its E plants are kept.
- cockpit: the phone path goes through the radial.
- input-gate covers desktop, phone with touch, a gamepad stub and reduced motion, plus roles, names and focus return.

## Hands-on QA (`dev/stack-realm.sh`)

**Phone, 390×844, touch:**
1. At rest on foot, tap a hall. The box sits above the skills row, clear of Jump, Run, Go and the chat log, which
   narrows.
2. On the bike, the box follows the shorter pad. With the realm down, it sits lower still.
3. Tap the box: the sheet opens and the box hides. Close the sheet: the box comes back.
4. Long-press the box and slide to Enter: you go in, and no stray click fires.
5. Tap an in-reach wolf: the ring shows and nothing fires. Tap again: you attack.
6. Tap a far wolf twice quickly: you engage at a run.
7. A ground tap walks, clears the focus, and drops the tap's lock.
8. On the bike, tap a wolf: you ride there.
9. Tap a rider: it is focused. Tap again: its radial opens. Block while the realm is closed shows the box's "try again".
10. On the ship, a first tap focuses and a second tap acts. Drag, pinch, the skill-1 long press, the double-tap run, and
    `#deck` opening the Menu all still work.

**Desktop:**
1. Hover a resident, then move across the ground onto the box: the focus holds. Click Talk. Hold E: the radial opens.
   Release without moving: it stays open, and the arrows then Enter pick.
2. With no hover: E by a garden plants, E at a cave mouth goes in, E carrying a log drops it, and holding E gives Throw.
3. A left click on a mob attacks it. Hover a mob, then press E: you engage.
4. Enter is the chat outside, and Next inside and in the overview.
5. In the overview, R, X and C run the console, and Ctrl+R reloads the page. With a deck button focused, B and 1 still
   act.
6. Esc goes one step per press: radial, big map, lock, panels, focus, out of the building (talk open too), drawer, up a
   level. With a field focused and the big map open, one Esc does both.
7. Inside, C crouches and the Menu's Talk works. Keyboard only with VoiceOver: ⋯, then the items are read aloud, then
   Esc returns focus.

**Pad:**
1. A Y tap is today's E. A Y hold opens the radial, and the stick picks while the camera holds still.
2. D-pad up fires the ultimate, turns a piece in build mode, and climbs on the ship.
3. Start opens the Menu, and B closes it. The Controls panel shows the new rows, and an old stored layout migrates.

**Also:**
- The visitor page's Menu shows Chat, Map, Controls and Wardrobe.
- With reduced motion, the radial does not animate.

## Risks

- **The Esc order:** pinned row by row on main before any code moves.
- **Element handlers in the bubble phase:** they stop the event on purpose; a field that does not stop it reaches
  typing, which swallows the key.
- **A pad-only player focuses only parts and the carried item.** Mobs stay reachable by RT and LT, buildings by the
  overview and the deck. The owner may want pad hover later.
- **The first mob tap sets the lock,** which starts the fight framing, as an out-of-reach first tap already does today.
- **D-pad up for the ultimate** is a stretch mid-fight. It can be rebound; the owner judges it by feel.
- **The PR is large.** Each slice is a commit with its own dist and green tests.

## Not doing

- Automatic facing focus, and a focus-cycle key (answer e).
- The dialogs' pad polls.
- Rebinding Esc, M, Enter or ?.
- `COMMAND_LETTERS`, the overview's picking, the cards, the console, MapControls, or `world-look.ts`'s lock.
- A cockpit row from `GLOBALS`, a Fleet entry, or a phone "Top bar".
- Two-ring radials.
- A phone box inside buildings.
- The shooter zone.
- Plan/22's features beyond their registry slots.

## Build notes (2026-11-12, after ukubi-game #248)

Shipped as ukubi-game #248 (`132ffc0`). The build followed the code where this plan was wrong or silent:

- **Counts.** Before → after:
  - window key listeners: 11 → 2;
  - canvas listeners: 22 → 10 (the plan said 23);
  - outside-press listeners: 4 → 1;
  - `stopImmediatePropagation`: 10 → 0;
  - field-test copies: 8 → 1 (a ninth, in `world-control.ts`, also moved).
- **Names.** The plan's `Target` type is named `InputAt`; `Target` already exists in `town-types.ts`.
- **Contexts.** `key(k, e)` still gets the DOM event, because today's handlers read it.
- **Overview letters.**
  - R and X are write actions that are always off. In the overview they take the key and run nothing; C opens the Cockpit.
  - The overview letters skip the bound fight keys, so the deck's Q and P cells no longer run from the keyboard there.
- **Esc and fields.**
  - The chat input stops its own Esc, so "one Esc closes the big map and blurs the field" holds only for the find box.
  - Inside a building the door opens the drawer, so Esc closes the drawer before the Cockpit, then leaves.
- **Combat and rides.**
  - The fight context is active wherever today's checks pass, so Q and P still act inside buildings.
  - 1, 2 and 3 stay in `view.key`.
- **Release and blur.**
  - `visibilitychange` still releases only the view.
  - A rebind still calls `view.release()`.
  - handBack now runs on the router's bubble keydown, so an Esc that a field or dialog stops no longer hands the pad's look back.
- **Pointer pipeline.**
  - Gestures go to named host stages, not to the contexts' `pointer()`.
  - The long press and the refocus-after-click stay in the view.
  - The rider-menu and phone-menu closers moved to the one capture pointerdown.
- **E and carrying.**
  - With no hover, E's fallback still tries the spawn stone and the guild under the pointer.
  - The throw threshold moved from 350 ms to 400 ms (`ACT_HOLD_MS`).
  - Throwing can't happen in the live realm, because nothing puts an item in the hands. The gate stubs a carried item.
- **Desktop box (the owner's fix).** A focus shows only the box. `deckOpen` does not gain `|| focus`.
- **Radial placement.** On the phone the focus radial is drawn above the box, and on touch only the item under the thumb (within 16 px) is picked. H and Start open the Menu at the screen centre; `#deck` opens it under the button.
- **Mobs.**
  - A tapped mob's focus clears only when `engageAct` would refuse, not by distance.
  - The mob's Inspect act was dropped (no handler).
  - A tap on a mob after T engages it.
- **Gamepad.** The pad's Start passes no opener. H takes only `#deck` or ⋯ as the opener.
- **Pre-existing, not fixed.**
  - The middle click's target is read after the hover blanks it, so it locks the mob ahead (`world.ts`, `middlePress`).
  - At 320 px the chat's Hide chip sits under the ride-swap button (P2).
- **Gates already failing on main.**
  - world-gate: Rimfall opacity, the Underside toggle, the phone underside.
  - onfoot-gate and aim-gate: the phone tier's `#mode` is hidden.
  - public-gate: the visitor page asks for `/api/land`.

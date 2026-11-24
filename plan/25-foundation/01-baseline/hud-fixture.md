# The HUD fixture (ukubi-game #255)

> Status: history (2026-11-24). Record of 2026-11-14 measurements.

The HUD fixture checks the HUD in CI with no realm, no WebGL and no `world.ts`. It is the proof tool for every later
UI step.

## How it works

- **The page.** `cmd/game/websrc/hud-fixture.ts` mounts the real `makeDock`, `makeBox`, `makeRadial`, `makePeople`,
  settings and `hideOwnerChrome` on `world.html`'s own markup.
- **The runner.** `dev/hud-fixture.mjs`:
  - builds the bundle into a temp dir and serves the page on loopback, with the game's CSP;
  - runs 9 states at 4 sizes, 36 rows in about 5 s;
  - asserts that every shown surface is on screen;
  - asserts that the dock, the Menu and the map corner never overlap;
  - asserts that touch targets are at least 44 px;
  - asserts there is no console error and no CSP violation;
  - asserts each state's own shown and away lists.
- **CI.** The `hud-fixture` job runs in about 46 s and keeps the 36 screenshots as an artifact. `dev/ci-local.sh`
  runs it too.
- **Served nowhere.** The page is not in the embedded web tree, so no production route serves it, and the dist is
  unchanged.

**The states:**
- rest;
- focus;
- fight;
- menu;
- panel (People);
- overview;
- inside;
- public;
- settings.

## The couplings it found (inputs for `../04-architecture/ui-core.md`)

1. **Modules find their own elements** by id from the document: `world-dock.ts:185,214` and `world-focus.ts:216,226`.
   So any fixture must reuse `world.html`. A better shape passes the elements in.
2. **`globals()` took the whole 52-callback `ActionDeps`** (`world-actions.ts:163`). #255 narrows it to a `Pick`, a
   type-only seam.
3. **`world.ts` injects the Menu, panel-close and pad icons** (`world.ts:1695-1699`), so the fixture shows those
   buttons with no icon.
4. **The form comes from `matchMedia`, read in several places:**
   - `TOUCH_PHONE` at `world-modes.ts:10`, read at `:47-50`;
   - `world-focus.ts:228`;
   - `world-radial.ts` (`innerWidth`, `rxFor`).

   The fixture gets the right form only through Playwright's emulation.
5. **Two surfaces are not mountable alone:**
   - The overview and the cluster need `makeChrome` (`world-chrome.ts:105`), with the shell, the town and the live
     state.
   - The minimap needs `buildMinimap` (`world-minimap.ts:386`), with a `HexLand` and a live feed (`:364`).

   So the fixture shows only their static frames.
6. **The dock's rise and fade animation** makes the sizes measured mid-way unstable unless reduced motion is on
   (`world.css`, near `:1935`).
7. **The HUD state is spread over five carriers:**
   - `body[data-view]`;
   - `data-controls`;
   - `data-public`;
   - the dock's `data-face`;
   - each element's own `hidden`.

   So the fixture has to call five modules in `world.ts`'s order. One render function from state to DOM would let a
   fixture feed one object.

`hideOwnerChrome` (`world-public.ts:122-130`) is already a clean DOM-only function and can be reused as it is.

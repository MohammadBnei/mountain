# Audit: the UI, the sim and the world model

> Status: history (2026-11-24). Record of 2026-11-14 measurements.

Checked on ukubi-game main `67bd436`. Three read-only explorers did the first pass and one adversarial reviewer
checked it; this file carries the reviewer's corrections. Paths: `websrc/` is `cmd/game/websrc/`; other paths are from
the ukubi-game root.

## The UI

**There is no single UI state.**
- **Form and input.** Two axes are each decided in several places. Form (phone or desktop) is checked in
  `websrc/town-phone.ts:9` (`max-width: 1199px`) and `websrc/world-modes.ts:10` (the same width plus
  `pointer: coarse`). Input (touch or mouse) is checked in `websrc/world-chrome.ts:96` (`pointer: coarse`),
  `websrc/world.ts:2353` (`hover: none`, then each event's `pointerType`) and `websrc/world-minimap.ts:454,574`.
  The CSS has 43 `@media` rules in six breakpoint variants, plus stray 900, 599 and 374 px.
  So a touch laptop under 1200 px gets the phone sheet without the phone controls.
- **The mode.** `Mode = 'explore' | 'action'` (`websrc/world-types.ts:165`) is a closure `let` inside
  `makeWorldView` (`websrc/world-view.ts:427`). It is compared 54 times. `view.info()` is called 58 times in
  `world.ts`, and each call builds a fresh object of about 25 fields (`world-view.ts:2154-2177`).
- **Hidden axes.** Public or owner, inside or not, and flying multiply the four views. `websrc/world-dock.ts:18`
  invents its own four-value enum, and `world.ts:2687` derives it again.
- **State on the page.** Body data attributes hold state and are written from 6 files.

**The view writes the HUD.**
- `makeWorldView` is one closure of about 1750 lines. It writes `#ride`, `#pad`, `#labels`, `#crosshair` and
  `#hitmark` (`world-view.ts:594-597,1612,2023`).
- It also adds a document click handler, in the capture phase, that steals focus (`world-view.ts:1304-1320`).

**The HUD is fed by polling.**
- `lockFrame` (`world.ts:1257-1288`) pushes `view.info()` into four frames on every frame.
- Each consumer diffs on its own: `world-modes.ts:52-75`, the `keyed` signature strings (`town-util.ts:17`) and
  `modeWas` (`world.ts:1806`).

**`world.ts` is the composition root.**
- It is a 3782-line top-level script with 100 imports and about 42 top-level `let`s.
- `ViewPage` and `WorldView` have 46 and 51 members.
- `ContextDeps` holds 52 callbacks (`websrc/world-router.ts:84`).

**The DOM moves with the form.** `applyMoves` re-parents `#inspector`, `#card` and `#flow` into the phone sheet
(`websrc/town-phone.ts:15-46`), so selectors and gates must know the form.

**The component family stops at paint.**
- `.glass:not(#_)` borrows ID specificity (`web/theme.css:6-9`), and radius opt-outs follow at `:20`.
- `world.css` is 2025 lines with 223 `#id` rules.
- Panel heads and close buttons are rebuilt in each module.
- The colour tokens are clean: all 197 raw colours sit in `tokens.css`.

**Input is mostly routed.**
- The router (`websrc/world-router.ts`) owns the keyboard, and an AST guard checks that.
- Pointer and gesture listeners are still added ad hoc:
  - document `mousemove` twice (`world-view.ts:1640`, `world-look.ts:88`);
  - the radial's window `pointermove` (`world-radial.ts:208`);
  - hold handlers on the shoot button and the focus box (`world-input.ts:315-348`, `world-focus.ts:168-193`);
  - the minimap's own cursor (`world-minimap.ts:470,645`).

**Checks.**
- There are 105 bun test files, mostly pure logic. Eleven of them use a hand-written fake `El`; nothing checks
  layout.
- The UI is checked only by 9 GPU browser gates that need a live realm stack. CI runs no gate.

## The sim and the agents

**The rider is written once.** Its `physics.Step` (`internal/physics/step.go:240`) lives in Go and runs in the
browser as TinyGo WASM (`cmd/physwasm`), with no TS fallback.

**The server checks a move with a different rule.**
- `World.ResolveFall` (`internal/realm/realm.go:850-905`) clamps speed, sweeps against hard colliders and caps a
  step-up at `base + physics.StepUp` (`:916`).
- It never applies the land's hex step rule `land.StepOK`, which only the WASM and the land tests call.
- The server never runs `Step`.

**The clocks differ.**
- The client steps at a variable dt, capped at 0.1 s (`websrc/world.ts:3033`).
- The realm ticks at a fixed 20 Hz (`internal/realm/realm.go:47`, `fight.go:46`).

**Some movement lives outside `Step`.**
- The lunge, the slow and the push (`websrc/world-view.ts:1569-1590`).
- The auto-hop (`world-view.ts:1426`, with `ledgeAhead` in `world-ride-step.ts:126`).
- The ship's own integrator (`websrc/world-ship.ts:222`).
- The overview's TS motion copy (`websrc/world-motion.ts:104`).
- A TS `hitOf` (`websrc/world-shot.ts:47`) next to Go's `physics/shot.go`.

**Five pathfinders and four kinds of nav data.**
- In Go:
  - the rider's A* on a 0.5-unit cell grid with jumps (`internal/physics/route.go:228`), chained along a hex A*
    (`coarse.go`);
  - the mobs' A* with no jumps (`physics.Walk.Path`, `walk.go:318`), also used as the knock-back test
    (`internal/realm/fight.go:748`).
- In TS:
  - the residents' BFS over the road graph (`websrc/world-carts.ts:108`, still named after the retired carts);
  - the `walkLen` BFS in `world-hex.ts`.
- Only the rider's grid knows ledges.
- A mob with no path, or past its path's end, heads straight at its goal (`internal/mobs/mob.go:302-304`). It still
  collides through `SweepMotion` (`:335-337`), but its plan ignores whether the hexes are walkable.

**Collision stores.**
- The server keeps three stores in sync by hand: `arena.Grid`, `w.Solid` and each lair's `Walk`
  (`internal/realm/build.go:247-254`).
- Planted trees and nodes skip `Walk.Mark` (`edit.go:264-265`, `resources.go:182-183`).

**Constants are copied by hand across the WASM boundary:**
- `websrc/world-ride-step.ts:113-116` (reach and face);
- `websrc/world-physics.ts:945-947` (jump leads);
- `websrc/world-hex.ts:17-20` (step and climb).

**There is no shared agent model.**
- Each kind has its own position source:
  - mobs: snapshots from a server state machine (`mob.go:240-250`);
  - residents: a pure function of id, building and hour (`world-people.ts:279`), drawing only the nearest 8;
  - critters: a shader;
  - carriers: a formula (`world-yard.ts:158`).
- Facing conventions differ: `atan2(dx,dz)` (`world-people.ts:943`) against `atan2(-x,-z)` (`physics.go:111`).
- Each kind has its own render pool.

**Determinism trap.**
- `cmd/physwasm/main.go:6` imports `sin`, `cos` and `atan2` from JS Math. They have 26 call sites across
  `internal/physics` and `internal/mobs`.
- So a client step and a server step cannot be bit-identical today. Re-running `Step` on the server is a tolerance
  check, unless trig is ported to pure Go.
- `physics.wasm` is 555.6 KB. It is built with no scheduler, using `-gc=precise`, only in a pinned amd64 Docker
  image.

**Tests.**
- There are about 98 Go physics tests.
- The rider traces were recorded from the deleted TS rider (`trace_test.go:18-20`, `traces.json` 930 KB), so they
  freeze old behaviour rather than specify it.
- No test covers:
  - mobs at cliffs or water;
  - `ResolveFall` against what `Step` makes;
  - different frame rates.

## The world model

**There is one land builder, but its input is put together twice.**
- Go builds the land, and the browser runs it as WASM. That retirement of the TS builders, in plan/21 phase 3,
  held.
- The building set the land is built from is assembled in TS (`websrc/world.ts:349-366`, with
  `world-capital.ts:138` and `world-yard.ts:215`). It is assembled again in Go (`internal/realm/land.go:131-170`),
  with the same `_(blue|green|red|yellow)$` regex. Parity rests on a dev golden script.

**The pipeline:**
1. The atlas graph (`internal/play/town.go`).
2. The old district layout (`town/town.go:141-153`, still run on each `/api/town`).
3. The disc layout: zones, villages on the golden-angle arm, lots and the yard (`town/village.go`, `town/disc.go`,
   `town/yard.go`).
4. Scale and snap (`land/wire.go:517 Place`).
5. The extra buildings (assembled twice, above).
6. `landOf` (`land/land.go:582`).
7. The hex land (`land/hex.go:1086`, `massif.go`, `pit.go`, `tunnel.go`, `cave.go`, `spiral.go`).
8. The realm's rebuild and its land-hash handshake (`internal/realm/land.go`, `realm.go:500`).
9. Rendering, the minimap and physics.

**The old layout gates founding.** `town.Whole` runs `town.Place` (`town/build.go:23-40`), called from
`internal/play/town.go:285,349`.

**The TS height copy.**
- `world-land.ts:314 heightAt` and `:292 regionAt` copy Go's `land.go:939 heightOn` and `:904 regionAt`.
- Go is checked float for float against goldens (`internal/land/land_test.go:17-20,431`) recorded from the old
  client copy. Nothing checks the TS copy any more.
- It is live only for the cover tint and the cover (`world.ts:452,456,2256`) and one gate probe (`:3353`).

**Seven coordinate spaces**, with hex↔xz math copied about 10 times:
- Go: `land/hex.go:202,207,226`, `land/land.go:252,256`, `town/village.go:139,168,173` (a second grid, twice as
  wide), `town/place.go:157`;
- TS: `world-hex.ts:53,58`, `world-land.ts:219`, `world-fun.ts:47`, inline in `world-tunnels.ts:42`.
- A building's world xz is stored in `hex: {Q: x, R: z}` (`websrc/world-face.ts:30`, `world.ts:343`), and the
  minimap reads it as xz.

**Copies and literals.**
- The arm road is two curves: `town/village.go:444-457` and `land/land.go:389-490`, with a hard-coded
  `0.26*54.6556` at `land.go:56`.
- The spawn `{5,0}` appears three times: `village.go:250`, `world-capital.ts:129`, `realm/place.go:43`.
- Hex kinds are copied by hand into `land/hex.go:51-95`, `world-hex.ts:84`, `world-terrain.ts:123` and
  `world-types.ts:268`. The decor registry, by contrast, is generated and checked (`land/registry_ts_test.go`).
- Special places are string checks: `'town-centre'` appears in 11 non-test files, and the `'info/'` and `'yard:'`
  prefixes and zone names are literals in the land stages.

**Load and scale.**
- The whole disc is sent and built at load (`world.ts:433-435`, timed as `hexMs`). The realm builds it whole too.
- The realm hashes it once for the handshake (`realm.go:500`).
- Chunks of 8×8 hexes only pick the render detail (`world-terrain.ts:210`, which scans every chunk).
- The disc is fixed: `DiscR` 54.66 (`town/disc.go:35`).

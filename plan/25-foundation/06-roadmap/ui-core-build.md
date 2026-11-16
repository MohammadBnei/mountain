# ui-core build plan

**Status.** The owner approved the ADR (`../04-architecture/ui-core.md`) on 2026-11-14 ("let's implement now"). This
plan carries the doubt round's amendments (`../05-doubt/ui-core.md`). The build starts once ukubi-game #250 (plan/22
P3, mail) has merged.

**Inputs.**
- Builders read `../04-architecture/charter.md` first.
- Decision records go to `decisions.md`, in this folder.

## Shape

- **One workflow, one pipeline per lane:** build, then review, then fix, then the lane's gate. There is no barrier
  after the build.
- **Models:**
  - S0 and the review: opus;
  - the builders: sonnet, at high effort for S2 and S8;
  - running the gates: haiku.
- **Fast suites inside each lane:**
  - `go test` with its cache, and `-race` only on the touched packages;
  - bun typecheck and test;
  - the lane's own fixture rows.
- **Integration:** the full suite and the GPU gates run once, in two waves on distinct ports. Main's known gate
  failures are passed in.

## S0: contracts and mechanical prep (opus, before the fan-out)

1. **Dependencies.** Add `preact` 11.2.0 and `@preact/signals` 2.11.4 as devDependencies, since they are bundled. In
   tsconfig, set `"jsx": "react-jsx"` and `"jsxImportSource": "preact"`, and add `*.tsx` to the include list.
2. **`ui-contracts.ts`**, frozen before any lane starts.
   - **Statechart regions:**
     - Mode: explore, strategy;
     - Place: outside, inside, door-flight, swim, under;
     - Ride: foot, bike, ship;
     - Fight: calm, fighting, down;
     - Panel: compound, `open:<kind>`;
     - Overlay: radial, minimap pop, big map, tip;
     - Link: the realm's 7 states plus the live client;
     - Typing and modal;
     - Loading and sign-in.
   - **Ctx:** form and role (owner, signed or guest). Input is a value the store updates from events.
   - **Facts:** focus `{kind, id, how}`, line, subject, selection.
   - **The store's fields.** A thread is `{id, cursor}` only (I4).
   - **`ActionDef`:** `{id, domain, label, roles, when, run}`.
   - **`SurfaceRow`:** keyed by the regions plus the facts; it marks mounted against shown.
   - **`SceneApi`:** facts on change, commands, and fast bindings, as listed below.
   - **`LiveClient`:** an event interface. Version 1 polls inside it.
3. **Mechanical prep, with no behaviour change:**
   - split `world.css` by surface into `hud-*.css`, linked from both `world.html` and `index.html`;
   - typed skeletons for every new `ui-*` file;
   - `ui-fake-scene.ts`;
   - `mountHud(sceneApi)`, which `world.ts` calls. `world.ts` stays the scene root.
4. **Check:** typecheck, bun test, dist, hud-fixture, and one world-gate run.

## The scene API (from today's reads)

**Facts** (scene to store, on change only):
- `view {ride, inside, flying, under, swim, locked, padLook}`;
- focus;
- `fight {on, me, ready, downed, pvp}`;
- realm link;
- selection and walked;
- loading;
- `unread {chat, mail}`;
- `rides {may}`;
- `pad.connected`.

**Commands** (store to scene):
- the scene members of today's `ActionDeps`;
- `setMode`;
- the fight's slots, ultimate and lock;
- `setRide` and `setPvp`;
- `select` and `go`;
- `release`;
- `sound`;
- `setContext`, which replaces the `ViewPage` hooks.

**Fast bindings** (per frame, never in the store):
- HP and max;
- slot cooldowns and the ultimate;
- the Mage charge;
- the focus mark point;
- the compass;
- the minimap pose;
- cards and labels;
- damage numbers.

## Lanes (disjoint ownership)

| Lane | Owns | Checks |
|---|---|---|
| S1 Store | `ui-store.ts`, `ui-chart.ts`, `ui-form.ts` (the only reader of the form, pointer and hover queries), `ui-surfaces.ts`, `ui-live.ts` (the seam), `ui-clock.ts`, `world-modes.ts`, `page.ts` | I2, I3, I6, I8, I9 |
| S2 Actions and input | `ui-actions.ts` (grown from `world-actions.ts`), `world-router.ts`, `world-bindings.ts`, `world-gamepad.ts`, `world-input.ts`, `ui-pad.tsx` | I1, I7, the key and tap catalogue |
| S3 Dock family | `ui-dock.tsx`, `ui-focus-box.tsx`, `ui-radial.tsx`, `world-dock.ts`, `world-focus.ts` (DOM half), `world-radial.ts`, `world-kit.ts`, `hud-dock.css` | fixture rows: rest, focus, fight, menu, down |
| S4 Menu and panels | `ui-menu.tsx`; `ui-panel.tsx` (the host plus the `Chrome` adapter); people, settings, account (D-02), loader, welcome, public, chat, controls, wardrobe; `world.html` and `index.html`; `hud-menu.css` | fixture rows: panel, settings, public; I5 |
| S5 Overview shell | `ui-overview.tsx` (the strategy shell placeholder and the thread panel's frame); `world-chrome.ts` (the host only; the `makeChrome` body stays); `town-shell.ts`, `town-phone.ts`, `world-select.ts`, `world-deck.ts`; `world-fleet.ts` (behind `LiveClient`); `hud-overview.css` | fixture rows: overview, strategy, thread |
| S6 Anchored and fast | `ui-fast.ts`; the minimap's frame and buttons; `world-cards.ts`; the compass; `hud-anchored.css` | I6 |
| S7 View | `world-view.ts` (strip the HUD writes; add `setMode`; emit view facts; take the pointer kind from the router); `world-look.ts`; `world-pastime.ts` | view tests |
| S8 Scene feed | `world.ts` (remove the HUD wiring, the 500 ms interval and the poll; feed the `SceneApi`); `world-scene-api.ts`. Every `__world` key is kept | bun test; a `__world` key test |
| Checks lane | `*.contract.test.ts`; `dev/hud-fixture.mjs` and `hud-fixture.ts` (13 states × 4 sizes, fed one store object); the behaviour catalogue; new rows for downed, banners, loader, sign-in, rebind, modals, swim, ship, Enter to chat, tunnels | proved red on the skeletons |

**Also in the bang: the P4 voice and P5 party UI.** That is:
- the mic in the pad and the ptt key;
- the Settings voice tab;
- the Party tab;
- the radial entries.

They are written from ukubi-73's unmerged branches, `realm/p4-voice-ui` and `realm/p5-party-ui`. `world-voice.ts`
ships in P4 as an event-based transport, and the bang wraps it in a store slice.

## Integration (one integrator)

- **Merge order:** S1 and S2, then S7 and S8, then S3 to S6, then the checks lane.
- **Build** with bun 1.4.x. `world.js` changes. `app.js` and `boot.js` must not.
- **The full suite runs once** (`go test ./...`), plus hud-fixture.
- **The GPU gates run in two waves of 4**, on distinct ports:
  - wave A: hud, input, world, public;
  - wave B: onfoot, aim, calm, cockpit.
- **`input-gate` also runs at each merge point.**
- **The decision records** are gathered for one sitting with the owner.

## Hands-on QA

On the final commit, at 390×844 touch, then 320×568, then 1440×900:
- plan/24 QA 1–14;
- plan/23 phone 1–10, desktop 1–7 and pad 1–3;
- Tab and the Menu to the overview and back: the selection and the thread are kept;
- kill the realm: "reconnecting" shows, then it recovers;
- a visitor sees no owner action in either mode.

## Estimate

| Step | Time |
|---|---|
| S0 | about 60 min |
| Lanes | 60–80 min |
| Integration and suite | about 30 min |
| Gates | about 30 min |
| Fixes | about 20 min |
| Hands-on QA | about 45 min |

That is about 4–4.5 h in total, plus the PR review and the owner's look.

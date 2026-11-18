# ADR: ui-core, one state and two modes

Status: approved by the owner on 2026-11-14 ("I agree with this plan"). The reviewed page, with its diagrams and the
owner's comments, is the claude.ai artifact `3XXtouZb9izpGJ5u8pzayM`. This file is the record.

## Context

Changing the UI is slow because there is no single UI state.
- Form and input are read from `matchMedia` and `pointerType` in about 15 files, and `world.css` alone has about 9
  `@media` shapes.
- `Mode = 'explore' | 'action'` (`world-types.ts`) lives in a closure in `makeWorldView` and is compared about 54
  times.
- `view.info()` returns a fresh 21-field object on every call, and `world.ts` calls it from 77 places.
- `world.ts` is about 3,800 lines with 100 imports, and `ContextDeps` holds 52 callbacks.

What already helps:
- `world-actions.ts` (plan/23) defines the actions once.
- The HUD fixture (#255) checks 9 states at 4 sizes in CI with no realm.
- The town page's `agentPanel` (`app.ts`) shows a session's status, its todos and a say box.

Coming next: a strategy mode (CK first, Civ lenses behind, cluster → town → app) and an immersive thread panel.

## Decision

Preact with signals, over one store and one action registry. Layout is a surface table keyed by a statechart. The 3D
scene stays outside, behind a narrow scene API.

- **The state machine.** The store's discrete state is a statechart with four parallel regions, each with guarded
  transitions. An event that makes no sense in the current state is refused. Form, input and role are fixed context
  and are not regions.
  - **Mode:** explore and strategy.
  - **Place:** outside, inside and flying.
  - **Fight:** calm, fighting and down.
  - **Panel:** none and open.
- **The store** holds:
  - the mode, form, input and role;
  - the subject (cluster → town → app → component);
  - the selection and the focus;
  - the open panel;
  - the open thread and operation;
  - the fight facts: in a fight, the target, the party, a skill ready or spent, death.

  Elsewhere:
  - Fast values (HP fill, cooldown sweep, cast bar, damage numbers) bind per frame to the realm-authoritative combat
    model.
  - Camera, physics and animation stay in the scene.
  - Conversation entries stay in agent-fleet core. The game keeps only a thread id and a cursor.
- **Sync is by events, never polling.**
  - The scene emits facts when they change.
  - The game pod gets one push stream to the page (town, live state, the fleet relay), which replaces `page.ts`
    `POLL_MS` and the `setInterval` loops. The realm WebSocket stays.
  - A lost connection is a "reconnecting" state.
- **The surface table** maps states to surfaces. The components are dumb, and the invariants and the fixture read the
  table.
- **The actions** are defined once, grown from `world-actions.ts`, and each carries a domain:
  - `work`: reachable in both modes;
  - `play`: explore only;
  - `social`: not yet bound by the parity rule.

## Invariants

| # | Never false | Enforced by |
|---|---|---|
| I1 | Every `work` action is reachable in explore and in strategy | a parity test over the registry |
| I2 | Switching mode keeps the subject, selection, thread and operation | a store test |
| I3 | Form and input are decided in one module | an AST guard (no `matchMedia`, `pointerType` or `innerWidth` elsewhere) |
| I4 | The game never holds the conversation | a type rule |
| I5 | One DOM for all forms, with no re-parenting | the HUD fixture (parents equal across the 4 sizes) |
| I6 | The store never changes per frame | a test hook (0 notifications over 300 idle frames) |
| I7 | A public visitor never sees an owner action | the parity test, filtered by role |
| I8 | Nothing in the UI polls | an AST guard (no `setInterval` or poll loop outside the stream clients) |
| I9 | Every reachable state has a surface-table row and a fixture row | a test that enumerates the reachable states |

## Scope: a big-bang rewrite with the checks written first

**In the rewrite:**
- all HUD and DOM surfaces (dock, Menu, radial, panels, cards, focus box, minimap frame, loader, settings);
- input (the router's contexts and gestures);
- the store, the state machine and the action layer;
- the composition root that replaces `world.ts`;
- the HUD writes inside `makeWorldView`;
- the game pod's push stream.

**Out, behind the scene API:** every 3D renderer, the camera, `world-physics.ts` and the realm socket.

**Checks:**
1. Written first on main: the fixture rows, I1–I9, and a catalogue of today's actions, keys and taps.
2. Run by every builder on every slice.
3. The GPU gates and the owner's hands-on QA, once at the end.

## The builders' charter

Builders read the charter (`charter.md`) before touching code. A close call is decided by asking what makes the most
sense for the persona who owns the surface:
- **Mo**, the architect and admin, owns the owner and `work` surfaces.
- **Sam**, a recruiter and a guest, owns the public surfaces.
- **Nico**, the player, owns the `play` surfaces.

A decision record is written only when that persona can't settle the question, or when the door is one-way.

**The record's fields:**
- id, surface, question, options;
- the option taken and why;
- whether the door is one-way or two-way;
- its fixture rows.

**How a record is handled:**
- A two-way door doesn't stop the build. The default sits behind one named choice point.
- A one-way door pauses only its own slice.
- The integration step gathers every record into one review sitting with the owner.

## Alternatives considered

| Shape | Why not |
|---|---|
| Plain TS with hand-built DOM | Rich surfaces bring back hand-diffing |
| A diegetic UI drawn only in three.js | Threads and accessibility suffer. Kept for decoration only |
| Server-driven UI from Go | Latency, two languages for one screen, no solo play |
| Strangler migration | The owner chose the big bang, with the checks first |

## The bet

1. Preact with signals builds a byte-identical dist under Bun 1.4 and runs clean under `style-src 'self'`.
2. The surface table expresses today's 9 fixture states plus strategy and the thread panel, with no escape hatches.

A spike checks both before the build. If 1 fails, use signals with no JSX, or Lit. If 2 fails, the table covers only
the mode and form layout.

## Consequences and not doing

**Consequences:**
- A HUD change becomes a table row plus a component, and the fixture checks it in CI.
- A new dependency: Preact with signals, estimated at 5–10 KB gzipped (the spike measures it).
- plan/22's UI lands first, and the rewrite migrates it.
- agent-fleet narrows to core and the provisioner.

**Not doing:**
- The strategy mode's look. It waits for the scene research. (Settled by round 4, 03-interview/round-4-hud.md H-5 and H-9.)
- The 3D renderers. sim-core and world-model own them.
- Full phone parity for `work`. A card stack comes first. (Round 4 H-9: the phone's strategy is a navigation rail.)
- Fleet cards for the decision records.

# Doubt round on the ui-core ADR (2026-11-14)

> Status: history (2026-11-24). Doubt record.

One fresh adversarial reviewer (read-only, on ukubi-game `7b5f98e`) and one spike (`../01-baseline/spike-ui.md`).

**The spike:**
- **Bet 1 passes:**
  - The dist is byte-identical under bun 1.4.4 and 1.4.2, with preact 11.2.0 and @preact/signals 2.11.4.
  - Bun handles TSX natively.
  - The page is clean under the game's exact CSP.
  - world.js grows by 10.1 KB gzipped.
  - A Preact change costs about 10 µs, 0.06% of a frame.
- **Bet 2 passes only as amended.** The table is keyed by the regions plus declared facts. Panel is a compound state. It distinguishes mounted from shown.

## Owner decisions on the contested findings

| Finding | Owner's choice |
|---|---|
| The push stream is multi-service server work: live state computed from 7 sources, the public pod's 30 s WriteTimeout, the online count and the keeper lantern counted from polls, the fleet relay | **A seam now, the stream later.** One `ui-live` client module behind an event interface, still polling inside, and I8's one allowed exception. Real server streams get their own PR and server ADR |
| Replacing `world.ts` rewires the scene code that is out of scope | **A `ui/` root mounted by `world.ts`.** `world.ts` keeps the scene and the frame loop and feeds the scene API. It slims later, in sim-core and world-model |
| P4 voice waits on the LiveKit infrastructure | **P3 (#250) merges first.** P4 and P5 ship their server and wire only. Their UI is written once inside the bang |

## Findings taken into the ADR (amendments)

1. **The statechart gains regions:**
   - Ride: foot, bike, ship.
   - Link: the realm's 7 states, plus the live client.
   - Overlay: radial, minimap pop, big map, tip.

   Also, Place gains `swim` and `under`, and `flying` becomes `door-flight`. Role has 3 values: owner, signed, guest. Typing, modal and the loader or sign-in are states too.
2. **The router stays the input layer.** Its `active()` predicates read the store. It is not a region.
3. **Scene widgets.** The minimap canvas, the labels, the crosshair, the hitmark and the focus mark are drawn per frame. Only their frames and buttons join the rewrite.
4. **Focus is emitted as `{kind, id, how}` only when the id changes.** The UI derives the name and the primary action.
5. **I3 is narrowed.** It bans only the form media queries and the `(pointer|hover)` queries outside `ui-form`. Per-event `pointerType`, `prefers-*` and viewport clamping stay allowed. Input is a store value updated from events.
6. **I6 uses a fake scene clock.** The 1 s age ticker is a clock signal outside the store.
7. **I8 has an allow-list, with a reason per entry:** the age clock, chatter, and the `ui-live` seam. Its guard also catches a `setTimeout` that re-arms itself.
8. **I9 enumerates the guard-filtered product,** with a CI time budget.
9. **`input-gate` runs at every integration point.** New checks come first:
   - downed and respawn;
   - the banned, replaced and stale banners;
   - the loader and the sign-in card;
   - rebinding;
   - the Wardrobe and Controls modals;
   - swim;
   - the ship;
   - Enter to chat;
   - tunnels.
10. **The spike's guard:** no `setAttribute('style')`, `dangerouslySetInnerHTML` or `innerHTML` with styles.
11. **Cut from the bang, as follow-ups:**
    - the push stream;
    - the minimap internals;
    - the overview's `makeChrome` body;
    - replacing `world.ts`.

## Decision record seeded

- **D-01.** Phone, fighting inside a building: the fight face replaces the focus face that holds Leave. The default is a `leave` surface on that row (two-way). The other option is refusing `engage` inside.

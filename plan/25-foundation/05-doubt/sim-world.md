# Doubt round: sim-world (2026-11-15)

One fresh adversarial reviewer (read-only, code and proposal only) attacked decision C, "chunked everything".

| # | Severity | Finding | Resolution |
|---|---|---|---|
| F1 | blocker | The layout is logged, history-dependent state (`game/layout.go:12-19`, `town/village.go:792`), not a pure function | Owner: change the layout at will, rarely. Placements become authored git files; logged `LayOut`/`ReLay` retire |
| F2 | blocker | Bit-exact unproven: `math.Exp`, `math.Hypot` arch paths; `town/` and lair placement outside the lint; no gc vs WASM golden | Phase 1 of the order: port, widen the lint, add the golden |
| F3 | blocker | Most land stages are whole-world passes; bet (a) fails for endless realms | Decision changed to A′: whole-space build on the server, chunks for transport; endless realms later with their own generator |
| F4 | major | Bandwidth: full protojson every tick, ~7 Mbit/s per client at 250 movers; one realm, one lock | Binary delta snapshots, interest for bodies, one instance per space; ambient movers stay scripted (owner) |
| F5 | major | Input replay needs a fixed client step, tick-stamped inputs, rewind/replay, and the ship, lunge and auto-hop in `Step` | Listed as prerequisites in the order |
| F6 | major | 20 Hz is coarse for bikes (1.3 u/tick) and ships (3.75 u/tick) against 0.6 u contact; no lag compensation | Substeps or swept contact, a lag-compensation rule, a two-phone test before the switch |
| F7 | major | Interiors and the underside are private today | Shared instances with their own colliders |
| F8 | major | Solo cannot run fight and mobs (they live in `internal/realm`) | A pure `sim` package built by realm and WASM |
| F9 | major | Atlas is the wrong owner for authored lore (its projections rebuild from repos; dev is memory-only) | Owner: git files, atlas projects them |
| F10 | minor | One land hash; the realm swaps worlds live | One instance per (space, version), old drains |
| F11 | minor | A cheaper first step for load | Caching and gzip already shipped (#253); the server-built `WriteHex` land is phase 2 |

Owner answers (same sitting): layout changed at will, rarely; lore as git files projected by atlas; bounded spaces now,
endless later, ambient movers scripted. Assumed and accepted: inventory and progression stay realm events.

# Doubt round: tests (2026-11-15, quick)

> Status: history (2026-11-24). Doubt record.

One fresh adversarial reviewer (read-only, code and ADR only) checked `04-architecture/tests.md`.

| # | Severity | Finding | Resolution |
|---|---|---|---|
| F1 | blocker | 60 s assumes CI shrinks: `test` is about 13 min (`go test -race` 556 s, the atlas integration 159 s), `web` about 3 min (WASM rebuild 52 s) | Owner: split fast and slow. Per PR: no `-race` except changed packages, bun, laws, journeys, WASM only when its Go changes. Nightly and before main: atlas integration, full `-race` |
| F2 | blocker | The realm can't be fully stepped: its 20 Hz ticker runs beside the step channel (`realm.go:137`, `:372`); more tickers at `bans.go:42`, `chat.go:160`, `voice.go:137`, `realm.go:293`, `voice.go:178`; 31 timer sites in the page | Step mode stops the ticker and routes every ticker through one clock; real-time parts (LiveKit, audio, sockets, three.js) are named and kept out of journeys |
| F3 | major | A step route behind `--dev` (a runtime flag, `cmd/realm/config.go:60`) ships in production | Build tag `teststep`; a test that the release build answers 404 |
| F4 | major | The HUD fixture's red runs were real: 2 a broken `bun install` step, 1 a missing panel (`panel-card @ desk`) | Keep the surface-table "is shown" rows; fix the install step |
| F5 | major | Deleting the gates in W1 leaves a gap until T2; public-gate alone checks visitor privacy, CSP and `#hottag` | Owner: delete all 9 now; privacy is not a concern in this phase |
| F6 | major | Dead wiring after the deletion: about 25 `__world` hooks in `world.ts`, gate comments, `CLAUDE.md` lines, `gate-stack.sh`, `found-gate.mjs` | Removed in the same PR; hooks the journeys need are kept and named in the ledger |
| F7 | major | Performance has no owner (only public-gate's 220-draw check) | Byte budgets for the bundle and `physics.wasm`; a draw-call journey; frame time in the QA helper, no pass/fail |
| F8 | minor | 96 clock reads in the page, not 32; `sim/clock.go` is `cmd/sim`'s clock, not the realm's; CI has no GPU | Facts fixed; the realm's `Realm.Now` and step channel cited; one journey measured on CI before the budget is fixed |

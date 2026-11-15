# ADR: tests, a safety net under a minute, in virtual time

Status: accepted 2026-11-15, after an architect round with the owner (conversation) and a quick doubt round
(`../05-doubt/tests.md`). The gate deletion ships inside the next wider PR, not on its own.

## Context

ukubi is testing its concept. Bugs, UI discrepancies and errors are many, and the tests don't catch anything deep:
they prevent crashes and massive failures. What "right" looks like keeps moving, so tests that pin details break on
every change without catching the regressions that matter. The owner's playthrough is the real check for feel and
UI.

Facts (ukubi-game, 2026-11-15):
- **CI is often red.** 10 of the last 40 CI runs failed (25%), with a median of 11 minutes and a maximum of 28.
  Failing jobs: `hud-fixture` 5, `web` 5, `lint` 3, `test` 3.
- **The gates are slow and outside CI.** There are 9 GPU gates (`dev/*-gate.mjs`, about 375 KB; `input-gate.mjs`
  alone is 102 KB). They need atlas, the game, the realm and a GPU browser, and they are not in CI. They hold about
  185 s of literal sleeps (`onfoot-gate.mjs` 57 s, `input-gate.mjs` 45 s, `world-gate.mjs` 27 s). The ui-core
  integration ran them for about 2 hours.
- **Unit tests are mostly synthetic already.** 87% of Go tests and 95% of TS tests build their own grid (spike S4).
- **CI is slow.** The `test` job takes about 13 minutes: `go test -race ./...` 556 s, the real atlas against the
  sim 159 s, vet 32 s. `web` takes about 3 minutes: `bun test` 62 s, the WASM rebuild 52 s, typecheck 23 s.
- **Wall time everywhere.** The page reads it directly: 96 `performance.now`/`Date.now` reads across about 10 files,
  plus 31 `setTimeout`/`setInterval`/`requestAnimationFrame` sites. The HUD already takes an injected clock (`UiClock`,
  `ui-contracts.ts:441`). The realm has an injectable `Realm.Now` (`realm.go:98`) and a test-only step channel beside
  its 20 Hz ticker (`realm.go:137`, `:372`), but its other tickers (`bans.go:42`, `chat.go:160`, `voice.go:137`, the
  land poll `realm.go:293`, `time.After` at `voice.go:178`) run on wall time.

## Decision

**Tests are a safety net against disasters: no crash, the page boots, the world builds valid, the sim stays
deterministic. The per-PR net runs in under 60 s, in virtual time, and a red run always means a real bug.**

- **Fast and slow paths.**
  - Per PR, under about 60 s: `go test` without `-race`, plus `-race` only on changed packages (as `dev/ci-local.sh`
    already does); `bun test` and typecheck; the laws; the journeys.
  - The WASM rebuild runs only when Go code it builds changes.
  - Nightly and before a merge to main: the atlas integration and a full `-race` run.
  - One journey is measured on CI (no GPU: SwiftShader, about 3x slower) before the 60 s budget is fixed.

- **Layers:**
  1. Go and TS unit tests (synthetic, as today).
  2. World laws on every build (`placement.md`): no pin collision, everything addressable reachable, looks never
     change gameplay, no dependence on id order.
  3. Determinism: gc against WASM, `Float64bits` (`sim-world.md`).
  4. Headless sim scenarios: rides, fights and routes through the pure `sim` package, minutes of game time in under a
     second (from W6).
  5. 6 to 10 scripted browser journeys, desktop and 390 px. For example: boot and enter; ride to a house, enter, talk,
     leave; open the map; fight one mob; a phone tap route; open and close a panel. Every journey checks that there
     are no console errors.
- **One clock, owned by the test.**
  - The real game and realm start once, in `--dev` with the in-memory store, on the frozen fixture world, with no
    atlas.
  - Every clock read in the page goes through one injected clock.
  - The realm's step channel grows into a step endpoint behind a build tag (`//go:build teststep`). A test checks that
    the release build answers 404. Stepping the real realm is not a fake.
  - In step mode the 20 Hz ticker stops, and every other ticker and `time.After` goes through the same clock.
  - `advance(ms)` steps the realm's ticks and the page's fixed frames together.
  - What stays real-time is named and kept out of the journeys: LiveKit, audio, socket latency, three.js internals.
  - No test sleeps.
- **No snapshot goldens of the world** (`06-roadmap/sim-world-build.md`). The fixture worlds stay as inputs.
- **Tests ask the stage, never coordinates:** `Find` in Go, `__world.find(query)` in the page.
- **Deleted now, all 9 GPU gates** (owner: public-gate's privacy and CSP checks go too, with no stand-in until the
  journeys). The same PR removes their wiring: `dev/ci-local.sh --gates`, `dev/gate-stack.sh`, `dev/found-gate.mjs`,
  the gate comments (`world-cards.ts:160`, `world-cockpit-tabs.ts:55`, `:81`, `world-hex.ts:176`,
  `world-terrain.ts:303`), `CLAUDE.md`'s gate lines, and the `__world` hooks in `world.ts` that no journey will need. A
  one-line ledger per gate check records it as covered by a journey to come, covered by a law or unit test, kept as a
  hook, or dropped.
- **The HUD fixture keeps its surface-table rows** ("is shown", from `SURFACE_IDS`), which caught a real missing
  panel, and drops the rest. Its CI job gets a working `bun install`; 2 of its red runs were that step.
- **Performance budgets.** Byte budgets for the bundle and `physics.wasm` in CI; one journey checks a draw-call
  budget; the QA helper records frame time with no pass/fail.
- **QA helper.** A script plays a route at desktop and phone sizes and saves a screenshot per step. It has no
  pass/fail. It serves the owner's and the agent's eyes before a handoff, and it closes its browser and stack.

## Alternatives considered

| Option | Why not |
|---|---|
| Keep and fix the gates | 375 KB of wall-time scripts pinning a UI in flux; 2-hour fix rounds |
| Owner playthrough only | A crash or a broken bundle reaches the owner first |
| Gates on demand, kept | Kept tests rot and pin moving details; their cost stays |
| Speed up wall time | Still flaky: two processes and network timing; virtual time removes the class |
| Every job on every PR | About 13 minutes per change |
| Keep public-gate until the journeys | The owner does not need privacy checked in this phase |

## Consequences

- A change is green or red in under a minute, and red means broken.
- Feel and UI detail rest on the owner's playthrough and the QA helper's screenshots until a surface settles.
- A settled surface may earn a journey; detail assertions come back only for settled surfaces.
- The clock seam and the realm step endpoint come first (T1); the journeys follow (T2).

## The bet

Between the deletion and the journeys, the unit tests, CI's dist identity and the owner's playthrough catch every
disaster. The gates never ran in CI, so only pre-handoff runs are lost.

## Not doing

- Pixel or snapshot comparisons of the world or the HUD.
- GPU gates in CI.
- Tests on wall time or with sleeps.
- Fakes of what we own (the realm runs for real, stepped).

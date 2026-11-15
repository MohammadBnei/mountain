# ADR: tests, a safety net under a minute, in virtual time

Status: accepted 2026-11-15, after an architect round with the owner (conversation). The gate deletion ships inside
the next wider PR, not on its own.

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
- **The page reads wall time directly** (`performance.now`/`Date.now`, 32 reads in `world.ts`). The HUD already takes
  an injected clock (`UiClock`, `ui-contracts.ts:441`), and the sim's clock can pause and step (`sim/clock.go:21`).

## Decision

**Tests are a safety net against disasters: no crash, the page boots, the world builds valid, the sim stays
deterministic. The whole net runs in under 60 s, in virtual time, and a red run always means a real bug.**

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
  - The realm gets a test-only step endpoint.
  - `advance(ms)` steps the realm's ticks and the page's fixed frames together.
  - Nothing moves on wall time and no test sleeps.
- **No snapshot goldens of the world** (`06-roadmap/sim-world-build.md`). The fixture worlds stay as inputs.
- **Tests ask the stage, never coordinates:** `Find` in Go, `__world.find(query)` in the page.
- **Deleted now:** the 9 GPU gates and `dev/ci-local.sh --gates`. A one-line ledger per gate check, written in the
  same PR, records it as covered by a journey to come, covered by a law or unit test, or dropped.
- **The HUD fixture becomes a picture tool.** It renders the HUD from canned states at four sizes and saves
  screenshots. Its assertions go, except "mounts without error".
- **QA helper.** A script plays a route at desktop and phone sizes and saves a screenshot per step. It has no
  pass/fail. It serves the owner's and the agent's eyes before a handoff, and it closes its browser and stack.

## Alternatives considered

| Option | Why not |
|---|---|
| Keep and fix the gates | 375 KB of wall-time scripts pinning a UI in flux; 2-hour fix rounds |
| Owner playthrough only | A crash or a broken bundle reaches the owner first |
| Gates on demand, kept | Kept tests rot and pin moving details; their cost stays |
| Speed up wall time | Still flaky: two processes and network timing; virtual time removes the class |

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

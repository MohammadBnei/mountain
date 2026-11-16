# 25 · Foundation: the UI, the sim and the world model

Status: research, 2026-11-14. Nothing here is decided beyond the owner answers below.

## Why

The owner said it is hard to change the UI, the interface and the visual interactions. Part of the reason is the four
views (phone and desktop, times explore and strategy). Part is the way the code is built. The owner sees the same
problem in the physics, in the automatisms (pathfinding, the residents, the mobs) and in how the world is mapped.
The goal is a foundation that is decided once, through research, interviews and architect rounds, and is then built
with workflows.

## Owner answers (interview rounds 1 and 2, then the doubt round, 2026-11-14)

| Question | Answer |
|---|---|
| What must the foundation buy first? | Cheap change, fewer regressions and room to grow, **all equally** |
| How bold? | **New cores**, not only seams added to today's code |
| Which order? | **Research all three, then the owner picks** |
| Where? | This folder, one sub-folder per step |
| UI library | **A small reactive library is allowed.** Preact with signals is the default candidate. Any library must pass a spike: the dist stays byte-identical under Bun, and the page passes the CSP (`style-src 'self'`) |
| The four views | **Rethink the strategy view.** This supersedes plan/24 rows 8–9 (the overview as a side panel, shipped in ukubi-game #252). An amendment row in `00-decisions.md` is drafted in step 05 |
| Authority | **One shared fixed-step Go core** for every mover, running in the client's WASM and in the realm. The server checks moves with it, and the client predicts and reconciles. What "solo" means when the realm is down (plan/21) changes, and that change will be listed |
| World loading | The owner's pain: **the world takes a few seconds to load, on every reload.** Streaming is one way to fix that. Step 01 measures where the time goes before any way is chosen |
| The old district layout | **Retire the town page** and its grid (`town.Layout`, `town.Place`, `placement.json`, `cmd/townpin`, and the `Whole` gate on founding). This supersedes plan/18's "until it retires at parity" |

## How each goal is measured

- **Cheap change.**
  - A HUD change touches at most 2 files.
  - The change is checked in CI with no realm running (the HUD fixture, step 01).
- **Room to grow.** A new mob kind, hex kind, zone or building role is one registry row.
- **Fewer regressions.**
  - Go and TS keep no hand-copied constants or algorithms.
  - The server checks a move with the same rule the client used to make it.
- **Load.** The cold-load and reload targets are set from the step 01 baseline.
- **Budgets.**
  - The WASM size and the time per frame stay inside a budget.
  - The server's CPU per player at 20 Hz stays inside a budget.

## Steps

| Folder | Step | Who |
|---|---|---|
| `01-baseline/` | Measure before designing: the audit, the load profile, the `Step` benchmark and trig divergence, the HUD fixture (a real PR) | main thread and agents |
| `02-quick-fixes/` | Fixes that ship first, apart: delete TS `heightAt`/`regionAt`; assemble the building set once, server side; make `ResolveFall` apply `StepOK` | main thread |
| `03-interview/` | Round 3, per area, once the baseline numbers are in | owner |
| `04-architecture/` | Architect rounds and one ADR each: `ui-core.md`, `sim-core.md`, `world-model.md` | main thread |
| `05-doubt/` | One fresh adversarial reviewer per ADR; the findings, the owner's resolutions and the amendment rows | agents, owner |
| `06-roadmap/` | Phases in the order the owner picks, each sized for one workflow | main thread, owner |

Implementation workflows start only after `06-roadmap/` is approved.

## Cut in the doubt round

- A general Go↔TS parity generator. Deleting the TS copies is enough.
- A survey of flow fields and navmeshes. About 100 agents run A* over a hex grid.
- A separate composition document. It becomes a section of `ui-core.md`.

# mountain: the master plans

Status: current, 2026-11-23. Revision 14: the index is rebuilt as one status row per file. The R7-R9 town sections,
their verification and rollback commands are gone: the town code (relay, `relay/rollback/`, `www/dev/schema/`,
`www/dev/townshots.mjs`) was deleted in A2 and exists only in git history.

mountain holds atlas (the cluster understanding surface, `cmd/atlas`) and these plans. Each plan is a file in this
folder; this file is the index.
The master plan that orders this work is owner-only and is not in this repo. How a change is made is `16-dev-rules.md`.

**Status words.** *approved*: the owner signed it off. *agreed*: decided in an interview. *draft*: not yet signed off.
*history*: describes code that is gone or work that is done. *superseded*: replaced, with a banner in the file.

## Index

| File | Content | Status |
|---|---|---|
| `00-decisions.md` | Decisions and review log (the attribution table is superseded) (the R rows) | current, appended per revision |
| `01-architecture.md` | The retired town's repo layout, world/bus, kit, budgets, CI | history (town deleted in A2); CI rows updated |
| `02-data.md` | The retired town's sources, PromQL, placement, persistence | history |
| `03-objects.md` | The retired town's objects: what each signal looks like | history |
| `04-sandbox.md` | The retired town's dev loop, gate, fixtures | history |
| `06-relay.md` | The retired Go relay: endpoints, protocol, security | history (absorbed by atlas) |
| `07-infra.md` | infra-bootstrap PRs and ADRs of the town, each with its owner | history |
| `08-auth.md` | The town's detail mode and visitor hardening | history |
| `09-play.md` | Playing the town with fleet agents (research) | history (research parked 2026-10-24, R10) |
| `10-understanding.md` | atlas: the system understanding surface (R10 design) | A0, A1, A2 merged, A3 built |
| `11-cluster-return.md` | The owner's checklist for when the cluster came back (2026-10-24 outage) | history |
| `12-world-and-game.md` | ukubi-game: an offline world mirroring the contracts, and the game on top (R11) | approved 2026-10-25 |
| `13-game-town.md` | The game's town: buildings, civs, ages, kits (R12) | superseded by `17-world-disc.md` |
| `14-console.md` | Control of the cluster from the game: providers, bounded writes, audit (R13) | history: aim stands; write path superseded |
| `15-architecture.md` | The ukubi code standard: seams, errors, config, front end, Bun (R13) | superseded (owner-only plan) |
| `16-dev-rules.md` | How a change is made: the loop, checks, merge | current, 2026-11-23 |
| `17-world-disc.md` | The world as a disc, from orbit to a building's rooms (R16) | draft 2026-10-29; layout superseded in part by 18 |
| `18-villages.md` | Villages by civ round a capital: one world for owner, friends, engineers (R16) | draft 3, 2026-10-31; superseded in part by `25-foundation/00-brief.md` |
| `19-storyboard.md` | The world's story and its one core quest: "work can be played" (R16) | draft 2, 2026-11-01 |
| `20-deep-disc.md` | Stub: the deep disc plan lives on branch `origin/plan/20-deep-disc` | stub |
| `21-the-shared-world.md` | Go physics core, realm service, MMO-lite world (R17) | approved 2026-11-02 |
| `22-voice-and-chat.md` | Proximity voice, World and Party chat, realm mail (R18) | approved 2026-11-11 |
| `23-interaction-system.md` | One input system: router, focus, hold radial (R19) | approved 2026-11-11, rev 4 |
| `24-hud.md` | One HUD: the dock, the Menu, the panels (R20) | approved 2026-11-12, rev 4; superseded in part by `25-foundation/00-brief.md` |
| `25-foundation/` | New cores for the UI, the sim and the world model: baseline, interviews, architecture ADRs, doubt rounds, roadmap (see below) | decided, built in part; see `06-roadmap/` |
| `26-meetings.md` | Stub: meetings live in infra-bootstrap ADR-0087 | stub |
| `27-play-review.md` | Stub: the play review lives on branch `origin/plan/27-play-review` | stub |
| `28-fleet-integration.md` | Stub: there is no plan/28 file | stub |
| `29-strategy-pass.md` | A hands-on walk of the strategy view: fixes, upgrades, personas (R21) | draft 2026-11-23; open questions for the owner |
| `adr-0056-amendment.md` | The R7 draft for infra-bootstrap, landed as ib:ADR-0064 | history |

## plan/25-foundation

| Folder | Content | Status |
|---|---|---|
| `00-brief.md` | Why, the owner's answers, how each goal is measured, the steps | decided, built in part; see `06-roadmap/` |
| `01-baseline/` | Measurements: audit, load profile, `Step` benchmark, HUD fixture, UI spike | history, 2026-11-14 |
| `02-quick-fixes/` | Fixes that ship first, apart | see its README |
| `03-interview/` | Owner interview rounds 1-4 and scenarios | history |
| `04-architecture/` | One ADR each: `ui-core.md` (approved 2026-11-14), `sim-world.md`, `placement.md`, `tests.md`, `ui-data.md`, `charter.md` | see each file's Status line |
| `05-doubt/` | One fresh doubt round per architecture file | history |
| `06-roadmap/` | The build phases and their decision records (`ui-core-build.md`, `sim-world-build.md`, `hud-build.md`, `s8-characters.md`, `decisions.md`) | per-file; merged work noted in each; `sim-world-build.md` is draft, not approved |
| `07-spikes/` | Spikes (placement, navigation, visual, test safety) | history |

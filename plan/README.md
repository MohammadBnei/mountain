# mountain.bnei.dev: ukubi as a live town

> This is the master plan. Each section lives in its own file in this folder, and this file is the index.
> **Revision 7, 2026-10-22.** The Aretē mountain is retired: its metaphor did not carry over to 3D. The mountain's last
> state is the local tag `mountain-final` (`f1a53b2`), which holds its plan files (03-model-*, 05-story, 09-style,
> style/) and code.

## Context
mountain.bnei.dev draws the ukubi cluster live. Since R7 it is a **town** built from pi-lab's clay kit
(`pi-lab/compose/www`: KayKit hex tiles and buildings). It is driven by the same Go relay: PromQL, Alertmanager and
GitHub feed sanitised `/ws` frames, with Postgres history, scrub, OIDC detail mode and visitor marks.

- **Three islands = the Proxmox hosts h1..h3**, joined by bridges.
- **Every allowlisted app and platform service is a building** on the island of the host its pods really run on. Go
  places the plots and persists them.
- Signals are shown as town life:
  - syncs drop a building in;
  - restarts make it hop;
  - traffic is walkers at the door;
  - alerts light beacon fires.
- The sky follows the real Paris sun. Signed-in visitors plant a tree by day, a flag at dawn or dusk, and a lantern at
  night.
- The look follows the **Ukubi design system**: Office tokens by day, Underground·Glow at night, and a wire colour per
  system.

The binding rule is unchanged: **every image is true and tied to something real.**

## Status (2026-10-22)
- **R7 phase 1 is live** (PR #20). The relay is additive: `relay/town`, the `pods` placement, services, `world:plot`,
  alerts with `host`/`app`, `sun.go` and the plant toy.
- **Phases 2–4 are on `claude/modest-mayer-zbwumy`.**
  - The owner approved the styleboard and the mapping.
  - The town replaced `www/`. The mountain code, STT and `/api/topology` are deleted.
  - Wire schemas are v2. Migration 004 archives the mountain history.
- **Deploy 2** happens when that branch merges.
- **Live checks are not done from the agent container.** Its proxy denies `mountain.bnei.dev`, so the owner runs them
  until the host is added to the environment's allowed domains.
- **Human-gated:**
  - Land `adr-0056-amendment.md` in infra-bootstrap.
  - Remove `STT_TOKEN_MOUNTAIN` from Infisical.
  - The W4 items in 07/08 still apply: enrollment, the binding gate and Grafana strict.

## Section index
| File | Content |
|---|---|
| `00-decisions.md` | Decisions, attribution rules, review log |
| `01-architecture.md` | Repo layout, world/bus, object contract, kit, budgets, CI |
| `02-data.md` | Sources, PromQL, placement, static maps, event enum, metrics frame, allowlist, persistence |
| `03-objects.md` | The town objects: what each signal looks like, and where it is drawn |
| `04-sandbox.md` | Dev loop, gate, fixtures, CSP |
| `06-relay.md` | Go relay: endpoints, town geometry, WS protocol, security, self-observability |
| `07-infra.md` | infra-bootstrap PRs and ADRs, each with its owner |
| `08-auth.md` | Detail mode, visitor enrollment hardening, moderation |
| `adr-0056-amendment.md` | Draft for infra-bootstrap: platform services become public under role names |

## R7 phases
| Phase | Content | Exit |
|---|---|---|
| 1 ✅ | Relay, additive: town geometry, pods placement, services, `world:plot`, alert host/app, sun, plant toy, fixtures | Merged and deployed (PR #20). The mountain page still rendered |
| 2 ✅ | Town skeleton in `www/next/` plus the styleboard (`www/dev/townshots.mjs`) | **Owner approved the look and the mapping** |
| 3 ✅ | Live v1: reactions, walkers, night windows, beacons, stale, cards, plant toolbar and sign-in, admin eraser | Gate on both tiers within budget |
| 4 ✅ | Swap `www/next` → `www`. Delete the mountain Go and www and STT. Schemas v2, migration 004, `FROZEN.sha256`, CI | All checks green |
| 5 | These docs. Deploy 2 (PR → CI green → merge) | Live verification below |

**Later waves (not v1):**
- replica annexes and the relocation flight (v1 drops out and drops in);
- packets on bridges (`river`), a storage yard (pools), rx clouds, a cpu/mem chimney;
- gitops carts and ADR notices;
- the scrub UI (the relay and `world.scrub` already work).

## Verification
**Local, every change:**
- `go vet ./... && go test ./...`, with `MOUNTAIN_TEST_DSN` against a Postgres and `promtool` on `PATH` for the DB and
  rule tests.
- `node www/dev/schema/check.mjs` and `node www/dev/core.test.mjs`.
- `CHROME=… node www/dev/gate.mjs page` and `selftest`. These run against the real relay binary and check the CSP,
  the `/v/<build>/` prefix, the console, the budget on desktop and on phone-low, and resync.

**Deploy 2 (live, after the merge):**
- The page loads with a clean console, the hello arrives, and the overview, fly-in and phone views render.
- Every allowlisted app and service has a plot on a plausible host, and h3 gets something.
- A real ArgoCD sync drops a building.
- No product name, pod or node name appears in any frame.
- `/metrics` is still unreachable publicly.
- The relay log shows migration 004 and the epoch start.

**Rollback:**
1. Revert the merge on main. CI pins the previous image.
2. Run `relay/rollback/004_town_down.sql` by hand. It moves the mountain rows back and parks the town's rows in
   `town_archive_*`.

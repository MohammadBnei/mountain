# mountain.bnei.dev: ukubi as a live town

> This is the master plan. Each section lives in its own file in this folder, and this file is the index.
> **Revision 7, 2026-10-22.** The Aretē mountain is retired: its metaphor did not carry over to 3D. The mountain's last
> state is the local tag `mountain-final` (`f1a53b2`), which holds its plan files (03-model-*, 05-story, 09-style,
> style/) and code.
> **Revision 8, 2026-10-22.** The town grows up: islands sized by their hosts, a building typology, then the machines
> and the annexes (00 R8 rows).
> **Revision 10, 2026-10-24.** atlas: admins only, the town rebuilt as realm islands, the relay absorbed into a
> CQRS projector (00 R10 rows, `10-understanding.md`). A1 and A2 (`cmd/atlas`, L0/L1, the town deleted) merged; A3
> (L2 cities, L3 cards, the L4 reader) built.
> **Revision 11, 2026-10-25.** `ukubi-game`: an offline world mirroring the contracts, and the game on top
> (`12-world-and-game.md`). mountain exports what atlas reads as `contract/`, and `realms/` goes public.
> **Revision 12, 2026-10-25 (draft).** The game's town: everything but the hardware is a building, civs are the
> realms renamed, one held age paid by goals, research, users, latency and stability, AoE-style supply lines and
> defence, kits repainted into palettes (`13-game-town.md`).
> From A2 on, the town sections below (Context, R7–R9, `01`–`08`) are history: the code they describe is gone, and
> `10-understanding.md` is the live design.

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
- **R7 is live:**
  - Phase 1 (relay, additive) shipped in PR #20.
  - Phases 2–5 shipped in PR #21 (image `2573bce34991`).
  - The owner approved the styleboard and the mapping.
  - The town replaced `www/`. The mountain code, STT and `/api/topology` are deleted.
  - Migration 004 archived the mountain history.
- **R7.1** is a hardening wave from a post-deploy doubt cycle: three read-only reviews of the merged code.
  - **Plants:** they wilt after 7 days, and a full island's oldest plant gives way.
  - **Data:** the storage class says `replicated`, not the product. An empty Ready or quorum answer changes nothing.
    A host is unknown, not down, until its first answer.
  - **Plots:** a freed ruin waits for ArgoCD before it is rebuilt. A pending move restarts after a gap in the answers.
  - **Ops:**
    - a first boot still seeds the rate limits;
    - the `MountainTownEmpty` alarm;
    - the rollback can run twice;
    - quorum rides the hello.
  - **Front end:**
    - moved buildings stay tappable, and so do ruins;
    - a failed model no longer freezes the town;
    - no instance overflow during a storm of alerts;
    - a pinch is never a tap;
    - a "can't reach" message;
    - the session is re-read when the page returns, and the quota is known before Plant enables;
    - the kill switch hides the plants;
    - label widths are right after the fonts load.
- **Live checks are not done from the agent container.** Its proxy denies `mountain.bnei.dev`, so the owner runs them
  until the host is added to the environment's allowed domains.
- **Human-gated:**
  - Land `adr-0056-amendment.md` in infra-bootstrap.
  - Remove `STT_TOKEN_MOUNTAIN` from Infisical.
  - The W4 items in 07/08 still apply: enrollment, the binding gate and Grafana strict.

## R8 (2026-10-22)
The live check of R7.1 found h3 empty (no app runs most of its pods on `k8s-cp-03`) and Garage with traffic but no
building (it is an LXC). The owner's answers, one question at a time, are the R8 rows of 00.

| Step | Content | State |
|---|---|---|
| Phase 1 | infra-bootstrap PR #366: ADR-0064 (amends ADR-0056 Decisions 2–3) and seven `guest-<role>` blackbox probes | Merged (`2ded417`), probes live |
| Deploy A | Islands 7/5/4, families and ring order, eleven more KayKit models, ground caps, streets, bridge-head walls, fitted camera, island chips, label shapes, build guard, migration 005 + rollback | **Live** (PR #23, `aa59d30`, 2026-10-22) |
| Deploy B | The machines: keeps, halls, guests from `guest-*` probes, debounced state, admin names via `/auth/me` | Live 2026-10-22 (`e4ca1b1`): 12 machines, the 7 guests published up after their 10-min hold |
| Deploy C0 | The gate, in infra-bootstrap (ADR-0065): `mountain-gate` forwardAuth, invitation-only enrollment | PR open; the owner merges, then reviews `mountain-visitors` |
| Deploy C1 | The gate ref, automatic relay sign-in, the expiry reload, private assets, coarse replicas + migration 006 | Merged (PR #25, `9d82f82`, 2026-10-23), image `9d82f8224ecc` deployed. The gate holds only after the authentik worker restart; live checks are the owner's |
| Deploy C2 | Annexes: a copy of the app's building + banner, zoom levels | Merged (PR #26, `bea87d4`, 2026-10-23) during a power outage; deploys when the cluster and its runner are back |
| Civ (R9) | The herald, technologies, golden age bunting, turns, a guide (00 Civ row); levels taken out until goals exist (09-play) | Styleboard **signed off by the owner** (2026-10-23); PR open after C2 merged |
| Engine | Measure (shadow shapes, walker shadows, simplification test, gate breakdown), then BatchedMesh | After Civ (its worst case fits as is) |
| Play (research) | Make the town playable with fleet agents: owner interview, architecture, doubt cycle (`09-play.md`) | **Parked** 2026-10-24 (admins only, R10) |
| Atlas (R10) | The system understanding surface: realm islands, a 5-step ladder, a CQRS projector over the repos and live data, admins only (`10-understanding.md`). Joint with agent-kit | A0 + A1 merged (#28, `8d0e3ad`). A2 merged (#29, `7c8623a`): `cmd/atlas` replaces the relay, L0 map + land lens and L1 trade routes, the repo poll, the town deleted; not live until the cluster is back (`11-cluster-return.md`). A3 built: L2 cities, L3 CSS3D cards, the L4 reader with redaction; merge on the owner's word |
| World and game (R11) | `ukubi-game` (private): an offline sim mirroring Prometheus, Alertmanager, GitHub and authentik on the wire, so atlas iterates with no cluster; a goals-per-building game on top, with agent-fleet definition sessions doing the work (`12-world-and-game.md`) | Approved 2026-10-25. mountain's slice 1a merged (#31). In `ukubi-game`: slices 1, 2, 4a, 5a and 3 merged (sim, forge, goals and Merge, the director, fleet agents on steps); the full agent loop waits on the owner's `/kind-local` run |
| Game town (R12) | The game's look and AoE mechanics: buildings and civs from atlas's realms (renamed), supply lines, defence, research, capacity, one held age, units as live sessions, the kit system (`13-game-town.md`) | **Draft** 2026-10-25 (four owner rounds, two doubt cycles). Next: slice A7 (atlas data), with its own plan |
| Later | **The sea** (owner, 2026-10-23): water between the islands, with waves | After Deploy C; its own plan |

**Known limit of deploy A:** a tab opened before it runs R7.1 code, which predates `hello.build`. On reconnect it reads
the bigger islands with its old code and draws buildings off its old tiles until it is reloaded. Cosmetic, no data;
every later deploy reloads such tabs by itself.

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
| `09-play.md` | Playing the town (research, parked in R10) |
| `10-understanding.md` | atlas: the system understanding surface (R10 design) |
| `11-cluster-return.md` | The owner's checklist for when the cluster is back (2026-10-24 outage) |
| `adr-0056-amendment.md` | The R7 draft for infra-bootstrap, landed as ADR-0064 (R8) |

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

- `MountainTownEmpty` is inactive and `mountain_query_failed_total{query="pods"}` stays flat (R7.1).

**Rollback of R8 deploy B** (to the deploy A image): revert the merge; no migration. The deploy A relay drops the
machine plots at restore (they are not in its config), except granary, which it keeps as a service building. Replays
of deploy B days still show the machines.

**Rollback of R8 deploy A** (to the R7.1 image): the same steps as below, but step 3 runs
`relay/rollback/005_r8_down.sql` and step 4 reverts the R8 merge. To go back to the mountain, run 005's script first,
then 004's.

**Rollback** (the order matters: a mountain image that starts before step 3 writes a fresh mountain snapshot over
the history):
1. Turn ArgoCD auto-sync and self-heal off for the `mountain` app, so it does not undo step 2 or roll an image early.
2. Scale the relay to 0. For example, run `kubectl -n mountain scale deploy/mountain --replicas=0`, after checking the names
   with `kubectl -n mountain get deploy`.
3. Run `relay/rollback/004_town_down.sql` (`psql -1 -f …`). It moves the mountain rows back and keeps the town's in
   `town_archive_*`. It can run twice, so going forward and back again works.
4. Revert the R7 merges on main. CI builds a new image of the reverted code and pins it in `helm/values.yaml`. That
   is a new tag, not the old one.
5. Turn auto-sync back on. ArgoCD rolls the reverted image at 1 replica.

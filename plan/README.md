# mountain.bnei.dev — ukubi as a live Aretē mountain

> This is the master plan. Each section lives in its own file in this folder; this file is the index.
> **Revision 3, 2026-10-14.** It folds in 2 adversarial cycles (6 reviews) and 4 technique research passes. **(R2)** and **(R3)** mark the user's decisions from after each review. **(R4)** marks the W0.5 amendment (parallel lanes, style, deploy shape).

## Context
pi-lab has a scroll-driven three.js landing page that M BN loves (`pi-lab/compose/www/`). This is a sibling page for ukubi. It tells a scroll story of the cluster as a mountain, then ends in a **live finale** where real cluster events reshape the mountain in real time. The metaphor comes from hq's Aretē: strata = Proxmox → K8s → apps, lightning = usage, an eruption makes new land, the ice cauldron is observability, gold veins are incidents that became knowledge, and clouds are people. The binding rule: **"one physics only, and every image true and tied to something real."**

## Status (2026-10-15)

**W3a + W3b are done** (PR #10, image `1e099e2586f7`; PR #11, image `0453f71d5713`). Live prod restored from a pre-W3b snapshot cleanly ("0 veins, 0 ash lines"). **W3c seam (branch `feat/w3c`):** `src_github` (builds, gitops snow, ADR ash lines, vein `ref`, token-expiry metric; off without `MOUNTAIN_GITHUB_TOKEN`, not set in prod yet), the fixture's fake GitHub API, the cauldron test no longer assumes no firing alerts, and what the sky-full / river / shimmer lanes need (`js/sun.js`, kit `uHaze` + `uMask` with post.js projecting the mask, sky listens `world:cloud`, both models already in `page.js`). Next: the W3c lanes.

**W0, W0.5, W1a, W1b and W2 are done (2026-10-14).** **W3b seam (2026-10-15, branch `feat/w3b`):** relay worldcore veins/glacier/ash/snowline (`relay/derive.go`, `worldcore/{veins,glacier}.go`), `hello.state` += veins, glacier, ash, health, replicas, builds (persisted in `snapshots.state`, mirrored by world.js; frozen `world.js` + `hello.json` changed), strata reseeds from `world.state.ash` (ash survives a reload), hello fixture from `dump-hello --events fixtures/hello-events.json`. mountain.bnei.dev is live (image `4a0d0a6a4f90`, infra-bootstrap #300 + #302). Next: W3.

**W2 (PR #6, infra-bootstrap #302).** Done: Postgres persistence (`relay/store.go`, 02 "Persistence"), restart survival, `hello` = snapshot + deltas + `state` + metrics, scrub → replay (06), sandbox scrub controls, the freeze (`FROZEN.sha256` + `freeze_test.go`: contract.js, world.js, layout.js with a profile for every model, units.json, kit.js + tokens, the wire schema), the two carried style items, a GitHub-hosted `db-test` CI job, helm `envFrom` `mountain-config` (optional). Local checks: `go test ./...` with `MOUNTAIN_TEST_DSN` against `postgres:17`, core test, schema check, gate selftest + every model (dusk and day) + page + styleboard, pose gate on a hello restored from the DB. Not in W2: `marks`/`limits` tables (W4), snapshot veins/glacier/ash/clouds columns (W3, with their models), `events.detail` (detail mode). Live exit gate (2026-10-14):
- Restart survival on the pod: a wird sync landed a deposit (hello: 36 snapshot cells + 2 deltas); after `kubectl delete pod`, hello showed 102 snapshot cells and 0 deltas (shutdown snapshot folded them), log `db: restored snapshot + 0 frames`. Firing-alert restore was proven locally (SIGTERM and kill -9); production had no alert to carry.
- `scrub{day}` through Cloudflare: today replays (110 KB); a second scrub inside 2 s gets `rate`, a future day `bad-day`. Yesterday has no rows yet (the DB started today); the yesterday replay was proven locally against backdated rows.
- `mountain_source_last_success{source="db"}` is fresh in Prometheus.

Also live: adaptive draw rate (PR #5: 30 fps on battery, 60 when charging, pixel-ratio ladder) and orbit control on the finale (PR #7: drag/zoom, glides back to the sway after 45 s idle). Carried into W3: ~~cauldron `test.js` assumes no firing alerts~~ (fixed in the W3c seam); the scroll story must share input with the orbit.

**W1a exit gate (live, 2026-10-14):** a `wird` ArgoCD resync finished at 17:21:00Z and `ev:argo.sync` + `world:deposit` reached a client through Cloudflare at 17:21:26Z (26 s, target 45 s). A WebSocket held 380 s through Cloudflare. `/metrics` returns 404 publicly and `:9090` is unreachable. A foreign Origin gets 403. The CSP has no dev origin. Live `/ws` showed no IP, node, internal namespace or version. Dev loop: `scripts/devpod.sh` (infra-bootstrap #301 makes it a skill).

**W1b exit gate (live, 2026-10-14):** PR #1 merged. Checks:
- OOM test pod in `mountain-test`: `pod.churn{oom}` 38 s after start (dev pod, test config).
- `amtool` alert, severity info (warning and critical route to thot, so they are tested locally only): `alert.fire` reached production through Cloudflare 7 s after firing, and the cauldron sag ring showed. Watchdog causes no sag.
- `--prom-url=http://invalid`: "stale since" label, dimmed sky, `mountain_source_last_success` frozen.
- `Mountain*` rules (severity `notice` -> Discord; `info` would be inhibited) loaded and healthy.
- ArgoCD controller restart at 20:19Z: no `app.gone` over 11.6 min of watching production /ws; `argocd_app_info` stayed at 39 series.
- `MOUNTAIN_FP_SALT` synced from Infisical `mountain-8c-cz`.

Post-deploy fixes:
- Spike storm after relay start: no traffic baseline from the first poll.
- Cloudflare's 4 h browser TTL mixed module versions after a deploy: assets are now served under `/v/<build>/` as immutable, and `index.html` uses `no-cache` (PRs #2, #3). `boot.js` now tells a load error from a missing WebGL2.

Carried into W2 (both fixed in W2):
- The ember pool is not visible in the day key. → own rig key `pool` (`uPoolK`, day 2.4), no longer tied to `uEmber` (09).
- Lightning bolts at the day key are thin and have little contrast. → the bolt is sheathed by the elmo halo quads along its ribbon (03-model-lightning).


**Carried into the W1b seam (done):** the ember pool sits under the fog from the default camera (Freeze item 3); the cave close-up clip check misses by 1–2 %; the 'other' world deltas still re-emit on reconnect; it is unconfirmed whether the build runner has Chrome for the CI gate.

| Item | State | Where |
|---|---|---|
| Plan split + private repo `MohammadBnei/mountain` | done | this repo |
| ADR-0056 (= ADR-A: public telemetry, PromQL only, no RBAC) + DECISION.md entry | **merged** | infra-bootstrap #298 |
| Infisical project for app secrets | **exists**: `mountain-8c-cz`, id `8746c02c-24a0-4b4c-950f-1ee9b36c2625` (empty so far) | Infisical |
| `ACCESS_TOKEN` PAT covers the repo | **verified** (GET runners → 200) | — |
| Build runner `ukubi-build-mountain` (labels `self-hosted, ukubi-build`) | **online** | build-runner LXC, `build_runner_repos` |
| CI OIDC identity `19807471-…` bound claim includes `MohammadBnei/mountain` | **done** (`event_name` = `push, workflow_dispatch` only) | Infisical |
| Postgres `dbuser_mountain` / `mountaindb` | **live**, verified: connlimit 10, owner, PUBLIC connect revoked, login + DDL via `.232` | Pigsty `pg-proxmox`, leader `.205` |
| DB password | `DBUSER_MOUNTAIN_PASSWORD` in the **infra-bootstrap** Infisical project (`8a3fa54f-…`, env `dev`), not in `mountain-8c-cz`: one copy only, per infra-bootstrap convention | Infisical |
| Docs marking the above live | open | infra-bootstrap #299 |
| `/add-app` `mountain.bnei.dev` (points at this repo's `helm/`) | **done**, infra-bootstrap #300 merged | W1a lane D |
| Traefik scrape interval 15 s | **done**, #300 | W1a lane D |

**Next: W0.5** (stack verification + style, amendment R4), then W1a. W1a and W1b now run as **parallel lanes after an orchestrator seam commit** (00 R4, 01 ownership).

What the W1a lanes need to know:
- Build workflow: copy hq's `.github/workflows/app.yml` pattern. `runs-on: [self-hosted, ukubi-build]`. Registry creds come from `platform-commons-76pb` over GitHub OIDC (ADR-0049). Only `push` and `workflow_dispatch` can read them, so guard that step with an `if` for PRs. Image: `registry.bnei.lan:5000/mountain`.
- DB (W2, wired): `DATABASE_URL=postgres://dbuser_mountain:<pw>@postgres.bnei.lan:5432/mountaindb` (5432, not 6432, because pgbouncer is off). It is assembled in infra-bootstrap by a `gitops/bootstrap/mountain-secret.yaml` modelled on `hq-secret.yaml`. Keep the password urlsafe. **pgxpool `MaxConns` must be set ≤ 8** (connlimit 10).
- `/add-app` lands with the first image (**W1a lane D**, 07): the ArgoCD app points at **this repo's `helm/`**, CI pins the `:sha` tag in `helm/values.yaml`, replicas 1, `Recreate`, baseline middleware on, metrics on a separate `service.extraPorts` port that is never routed, **ServiceMonitor in `extraManifests`**.

## Section index
| File | Content |
|---|---|
| `00-decisions.md` | Decisions, attribution rules, review log |
| `01-architecture.md` | Repo layout, units, model contract, world/bus/fields, shaders, post, budgets, file ownership |
| `02-data.md` | Sources, PromQL + counter handling, static maps, event enum, metrics frame, allowlist, persistence |
| `03-model-<name>.md` × 18 | Rules, technique, budget, clipping, **behaviour fixture + assertion** |
| `04-sandbox.md` | Sandbox, gate, perf, CSP |
| `05-story.md` | Scroll mechanism. **Copy is human-gated** |
| `06-relay.md` | Go relay: endpoints, worldcore, WS protocol, security, self-observability |
| `07-infra.md` | infra-bootstrap PRs and ADRs, each with its owner |
| `08-auth.md` | Detail mode, visitor enrollment hardening, moderation |
| `09-style.md` | Aretē tokens, time keys + light rig, kit factories, facet bands, bans (R4) |

## Waves
| Wave | Who | Content | Exit gate |
|---|---|---|---|
| **W0** ✅ (except `/add-app`) | M BN + 1 agent | Create the private repo `MohammadBnei/mountain` and split the plan. **ADR-A** PR (public telemetry, PromQL-only). Human steps: create the Infisical project `mountain`, add the repo to the `ACCESS_TOKEN` PAT repo list, run `build-runner-configure.yml`. `/add-app` (this repo's `helm/`, `:sha` pinned by CI, ServiceMonitor in `extraManifests`) **moved to W1a lane D** | All human steps are done, so W1 can deploy |
| **W0.5** (R4) | Orchestrator + ~4 agents | Stack verification: facts (hard gate) + fit (advisory), live read-only PromQL probes. `09-style.md` + R4 plan edits. Swatches + lit styleboard for the 4 time keys | Facts table green, probes return series, **user approves the swatches + styleboard** |
| **W1a** ✅ | Orchestrator seam commit, then parallel lanes: (A) relay, (B) www, (C) models, (D) infra PR | Relay: `src_prom` (sync, metrics), worldcore (`h.base`, sites, deposits), `hello`, `/ws`, `/api/topology`, `/metrics` on its own port. www: kit, world, socket, sandbox, gate. Models: **mountain** (body + plain slabs), **eruption**. Deployed | A real ArgoCD sync makes land appear on the live site within 45 s |
| **W1b** ✅ | Orchestrator seam commit (incl. `post.js`), then parallel lanes: (A) `src_alerts`, (B) cauldron, (C) sky-min, (D) lightning, (E) infra PR | `src_alerts`. Models: **cauldron** (own sag-ring mesh), **sky-min** (dome + cloud floor; owns `world.clouds`), **lightning**. post.js on desktop. Stale label | W1 verification passes |
| **W2** (done) | 1 agent | **Freeze** `contract.js`, `world.js`, `units.json`, `layout.js` (every profile), the bus registry, the event enum and the metrics frame, as revised by what W1 taught. Pigsty DB, persistence, scrub, `hello.state` | W2 verification passes |
| **W3** | Parallel | Wave start, serial: **`cut.js` + strata** as the reference field provider (1 agent). Then parallel: aquifers, magma, veins (providers); glacier, summit, scree, sky (full), river, shimmer; story mechanism; `src_github`; detail-mode OIDC | Every model passes `gate.mjs <name>` + its fixture assertion; the page stays within budget |
| **W4** | M BN + agents | **ADR-B** (visitor enrollment). Hardening PRs (08). Then, in parallel: toy-clouds, cairns, footprints, toss + the relay `toy_<name>.go` validators | W4 verification passes |


## Verification
**W1a:**
- The sandbox gate + `test.js` pass for mountain and eruption.
- A real ArgoCD sync of a test app → `argo.sync` + `world:deposit` → land on the live site **within 45 s**.
- `curl https://mountain.bnei.dev:…/metrics` is unreachable; `:9090` answers only in-cluster.
- `websocat /ws` shows no namespace, IP, hostname, version or non-allowlisted app. The Go `sanitize` unit test covers the fixture corpus.
- A cross-origin upgrade gets 403.
- A WS session survives more than 5 min through Cloudflare.

**W1b:**
- **OOM test pod** (a memory hog with a low limit), or `kill 1` in a test container → `pod.churn{oom}`.
- `amtool` alert → cauldron ring + steam. **Watchdog causes no sag.**
- Traffic burst → glow, then a bolt (fallback cloud height).
- Relay started with `--prom-url=http://invalid` → `MountainSourceStale` (notice → Discord) + "stale since" + the sky dims.
- Restarting the Prometheus or ArgoCD controller causes **no mass `app.gone`**.

**W2:**
- After a relay restart, land, veins and glacier state survive. `hello.state` restores firing alerts.
- Scrub (via websocat `scrub{day}` + the sandbox) replays yesterday, and release returns to live.
- The `pose` determinism gate passes.

**W3:**
- Every model passes gate + `test.js`.
- The full page stays within 220 draws / 200k tris at peak.
- `perf.mjs` (Android profile) holds frame time.
- Non-admins never receive detail frames.
- A detail-mode admin can delete a mark.

**W4:**
- Enrollment without the captcha fails.
- The binding gate fails on a fixture app with no binding.
- **A `mountain-visitors` user is denied at ArgoCD, Grafana, hq and hermes.**
- A forged cairn pose has its y recomputed, or is rejected.
- Rain needs 3 accounts.
- `VISITORS_ENABLED=false` hides the surface layer live.

## On approval (done 2026-10-14)
Repo created, plan split and pushed. W0 infra is done; see Status above.

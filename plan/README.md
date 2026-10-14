# mountain.bnei.dev — ukubi as a live Aretē mountain

> This is the master plan. Each section lives in its own file in this folder; this file is the index.
> **Revision 3, 2026-10-14.** It folds in 2 adversarial cycles (6 reviews) and 4 technique research passes. **(R2)** and **(R3)** mark the user's decisions from after each review.

## Context
pi-lab has a scroll-driven three.js landing page that M BN loves (`pi-lab/compose/www/`). This is a sibling page for ukubi. It tells a scroll story of the cluster as a mountain, then ends in a **live finale** where real cluster events reshape the mountain in real time. The metaphor comes from hq's Aretē: strata = Proxmox → K8s → apps, lightning = usage, an eruption makes new land, the ice cauldron is observability, gold veins are incidents that became knowledge, and clouds are people. The binding rule: **"one physics only, and every image true and tied to something real."**

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

## Waves
| Wave | Who | Content | Exit gate |
|---|---|---|---|
| **W0** | M BN + 1 agent | Create the private repo `MohammadBnei/mountain` and split the plan. **ADR-A** PR (public telemetry, PromQL-only). Human steps: create the Infisical project `mountain`, add the repo to the `ACCESS_TOKEN` PAT repo list, run `build-runner-configure.yml`, merge `/add-app` | All human steps are done, so W1 can deploy |
| **W1a** | 1 agent, serial | Relay: `src_prom` (sync, metrics), worldcore (`h.base`, sites, deposits), `hello`, `/ws`, `/api/topology`, `/metrics` on its own port. www: kit, world, socket, sandbox, gate. Models: **mountain** (body + plain slabs), **eruption**. Deployed | A real ArgoCD sync makes land appear on the live site within 45 s |
| **W1b** | 1 agent, serial | `src_alerts`. Models: **cauldron** (own sag-ring mesh), **sky-min** (dome + cloud floor; owns `world.clouds`), **lightning**. post.js on desktop. Stale label | W1 verification passes |
| **W2** | 1 agent | **Freeze** `contract.js`, `world.js`, `units.json`, `layout.js` (every profile), the bus registry, the event enum and the metrics frame, as revised by what W1 taught. Pigsty DB, persistence, scrub, `hello.state` | W2 verification passes |
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
- Relay started with `--prom-url=http://invalid` → `MountainSourceStale` (info → Discord) + "stale since" + the sky dims.
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

## On approval
1. Create `~/Code/mountain` (`git init`; private GitHub repo `MohammadBnei/mountain`, created by M BN or through `gh`).
2. Split this file into `plan/README.md` + `plan/00…08-*.md` + 18 `plan/03-model-*.md`.
3. Commit with no attribution.
4. Stop. W0's human steps and the workflow launch are M BN's call.

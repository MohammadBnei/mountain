# 00-decisions — Decisions

| Topic | Decision |
|---|---|
| Audience | Public story + sanitized live layer; **detail mode** for `platform-admins` |
| Repo **(R3)** | **Private** `MohammadBnei/mountain`, with the plan inside. No fork-PR risk to the self-hosted build-runner |
| Metaphor | Mountain = ukubi as Aretē strata. M BN signs off the copy |
| Story | Hero → Summit → Water → Strata → Bedrock → Live (P ≥ 5). The camera orbits while the cutaway deepens |
| Cluster access **(R2)** | PromQL only, no RBAC, no ServiceAccount |
| Transport | WebSocket, single `/ws`. The session cookie (read at upgrade) adds detail frames |
| Actions | v2 only |
| Relay **(R2)** | 1 replica, `Recreate` |
| World authority **(R2, R3)** | **Go computes everything persisted, including the terrain base `h.base`.** The client applies rasterised int16 cells and never recomputes them |
| Physics | cannon-es 0.20, one world, **collision groups `real` and `surface` that never collide**. Cosmetic only, never authoritative |
| Coupling | Full ecosystem via bus + fields. One writer per field and per shared state |
| Truth | A real metric **sets** the level; the simulation only animates the change. No source means it is drawn **unmeasured** (dim, static): Garage, the Pigsty spring |
| River **(R3)** | **All public traffic**: the sum of Traefik response bytes/s across allowlisted apps |
| Clouds **(R3)** | Real cloud floor = net rx **+ visitor clouds (one-way surface→sky)**. Lightning is driven by real traffic only |
| Memory | Land, veins, glacier state, ash lines and toy clouds/marks persist. Daily snapshots + deltas + 5-minute metric rollups |
| Visitors **(R2)** | Shared and persisted, after hardening + moderation |
| Email **(R3)** | **Shared SMTP2GO sender; keep Wird's email-verification stage.** Accepted risk recorded in ADR-B, with a quota watch |
| Sequencing **(R2)** | Vertical slice (W1a/W1b), then freeze, then fan out |
| Post FX **(R2)** | Desktop only, one composer owned by the orchestrator |
| Cut face **(R2)** | One cut shader with field providers, frozen at the start of W3 with strata as the reference |
| Style | Procedural low-poly, flatShading, Aretē palette (dusk canonical, time keys R4), gradient sky dome, **no shadows** |
| Lanes **(R4)** | W1a and W1b run **parallel lanes** inside the wave. Safe only because the orchestrator first writes every orchestrator-owned and soon-frozen file as a **seam commit**; lanes own only their own files (01 ownership) and stop and ask for any seam change |
| Palette **(R4)** | From hq Aretē (`app/arete.html` + `app/web/src/arete.css`), frozen in `kit.js`, gate-enforced. **Time keys** night/dawn/day/dusk (dusk canonical; the others shift lightness/temperature only). The real Paris sun drives `kit.setPhase` via `world.rig` (09) |
| Assets **(R4)** | **All procedural.** No KayKit GLBs; kit helpers `rock`/`puff`/`shard` (09) |
| Story input **(R5)** | **Buttons, not scroll.** Previous/Next buttons (plus ←/→ keys) step P one section and animate it along the keyframes; the page never hijacks scroll. **The mouse (and touch) always orbits** (OrbitControls, PR #7), in every section; a step glides the camera back onto the section's keyframe |
| Readability **(R6, W5)** | User test: "quite nice, but totally not ready". **Island on a sea** (the dropped rim dives under a kit sea at y −16, iceDeep fading to the horizon colour by r 400; the terrain's fog fade below y −4 is gone, the last 1.5 m are wet rock). **Real sun and moon light** (js/sun.js: SunCalc sun + moon + lit fraction; kit.setSky: the directional light comes from the real sun, a second one from the moon, each with its disc on the same vector; the hemisphere is the ambient floor; south = −z so the day arc crosses the finale view; backlit when the sun is in view, accepted). **The slice reads as one** (cap chain starts at stone2, glows at half its colour, an ink rim where it meets the surface, a "Cross-section" label). **The page opens on Live**, never replays the story by itself ("Story ↺"; the hash is a deep link, never written). **Cards + legend** (cards.js: tap/hover → models' pick(ray) → {kind, d, name?, ...raw}, nearest unhidden; legend.<kind> lines; story lines name the mapping). **Toy feedback** (hint, aiming ring from tool.ok(hit), success line, quota countdown from the mark ack {next, left}) |
| Deploy **(R4)** | hq/wedding style: helm values in this repo (`helm/values.yaml`), CI pins the `:sha` tag in-repo, the ArgoCD app from `/add-app` points at this repo's `helm/` |
| Storage / aquifers **(R4)** | **No NFS.** One pool per PVC, mapped to an app via the ns→app map; non-allowlisted PVCs (fleet, monitoring, …) merge into one `other` pool per storage class (no names leak), ~8–12 pools. Pool size = capacity bucket. **longhorn** = clear, deep, triple-layered shimmer (3 replicas), level **real** (`kubelet_volume_stats_used_bytes` ⋈ `kube_persistentvolumeclaim_info`, 6 PVCs live). **local-path** (node-local, fleet, 31 PVCs) = murky, shallow, pinned to its host's bedrock slab, level **unmeasured** (hostPath volumes emit no kubelet stats). No direct PVC access (R2). Optional W3 infra PR: node-exporter textfile `du` per PV dir makes local-path measured |
| Day key **(R4)** | Day relaxes the ≤ 10 % tint for the sky group only (sky0/sky1/ridge → water .5, horizon → water .6, cloud → ink .5), exposure 1.25, hemisphere ground = `stone`. Day must read as day, not dawn (09) |
| Terrain shape **(R4)** | Broad massif + terraces + **a continuous spiral walkway carved by Go in `h.base`** (≈ 2 turns, constant gentle grade, wraps around ridges on the surface, **no tunnels**) + 3–5 natural **caves** off the walkway (Go-seeded niches, kit `cave()`), shown on the cut face by the `caves` provider. Cave interiors not walkable in v1 (06, 03-mountain) |
| Push **(R4)** | Pushing to main is outward-facing (it deploys): **once per wave, after the user confirms** |

**Attribution, per repo** (agents must follow it):

| Repo | Rule |
|---|---|
| `mountain` | No AI trailer, no session link, no co-author (user's global rule) |
| `infra-bootstrap` | `Co-Authored-By: ukubi-claude-macbook <noreply@bnei.dev>` only, per its CLAUDE.md |
| `hq` | None |

**Review log, fixed in R2 and R3:**
- RBAC in app values
- 4 writers on `height`
- missing enum, sites and clip
- cross-site WebSocket hijacking (CSWSH)
- unhardened enrollment
- Go/JS float math, now with **`h.base` and the deposit raster also from Go**
- pgbouncer
- Traefik 60 s scrape
- `increase()` over a 15 s window, now with **counters diffed in the relay**
- namespace ≠ app
- OOM gauge semantics
- Watchdog keeps the cauldron sagging
- mass `app.gone`
- semver ≠ SHA build matching
- `/metrics` exposed publicly
- Grafana strict being a no-op
- the thot wake-up from warning alerts
- `rollout restart` not producing churn
- W1 depending on W3
- relay file ownership
- moderation
- PAT expiry
- static maps rotting
- the CF-Connecting-IP trust boundary
- clipped open meshes
- the coordinate contract

**R4 doubt cycle (W0.5):** 16 findings: 14 folded in (the R4 rows above, the R4 edits in 01/02/03/04/06/07, and `09-style.md`), 1 trade-off (scree rock variety comes from non-uniform scale + rotation, since its 2-draw budget rules out k meshes), 1 merged into another (CHROME/perf thresholds folded into the QA tooling finding). Cross-model review skipped by the user.

**R4 style review (W0.5):** 20 findings, all folded into 09/01/02/03/06/README. The lit styleboard confirmed the first-draft rig rendered the mountain black (dusk terrain p50 0.0022 linear); rig, fog and tone mapping retuned (09).


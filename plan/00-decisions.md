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
| Style | Procedural low-poly, flatShading, Aretē dusk palette, gradient sky dome, **no shadows** |

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


# 00-decisions — Decisions

| Topic | Decision |
|---|---|
| Audience | A public, sanitised live layer. **Detail mode** for `platform-admins` |
| Repo **(R3)** | **Private** `MohammadBnei/mountain`, plan inside. No fork-PR risk to the self-hosted build runner |
| Metaphor **(R7)** | **A live town** (pi-lab's kit), replacing the Aretē mountain, whose metaphor did not carry over to 3D. Live only: no scroll story |
| Islands **(R7)** | **3 islands = the Proxmox host buckets h1..h3**, joined by bridges. A bridge to a host out of quorum is raised |
| Placement **(R7)** | **Go places and persists the plots** (`world:plot`). Rules: the majority host of the app's Running pods wins; a move needs the new majority for ≥ 5 min; a building never moves off a NotReady host; an empty or failed answer keeps every plot; `app.gone` ruins the building and frees the plot after 24 h |
| Services **(R7)** | **Platform services are shown in full under generic role names**: gate, guardhouse, craneyard, watchtower, warehouse, vault, granary, depot (`relay/config.yaml`, group `service`). Only the role key reaches the wire. This amends ADR-0056 (`adr-0056-amendment.md`, to land in infra-bootstrap) |
| Geometry **(R7)** | **Go owns the town geometry** (`relay/town`, sent in `hello.town`): pointy-top axial hexes, radius 4, tile kinds square/grass/road/bridge head/meadow. JS only renders it |
| Style **(R7)** | **The Ukubi design system** (claude.ai artifact "Ukubi"): Office tokens by day, Underground·Glow at night, one wire colour per system (app/agent/data/network/storage/compute) tinting roofs, which glow at night. Readex Pro and JetBrains Mono, self-hosted. Status is always said in words |
| Assets **(R7)** | **KayKit Medieval Hexagon GLBs** (CC0), repainted with pi-lab's clay atlas (`www/models/`). One InstancedMesh per building kind |
| Camera **(R7)** | Opens on the whole town. Tap an island or a building to fly there (and read its card); tap the sea to fly back. Orbit always works |
| Toy **(R7)** | **Plant**: the relay decides the item from the real Paris sun at acceptance: a tree when the sun is above +6°, a flag between −6° and +6°, a lantern below. Only on free grass or meadow hexes. The four mountain toys are gone |
| History **(R7)** | **Fresh start.** Migration 004 moves the mountain history into `mountain_archive_*` (no dump could be taken), soft-deletes old-toy marks and records `town_epoch`. Scrub before the epoch answers `unavailable`. `relay/rollback/004_town_down.sql` reverses it by hand |
| Dropped **(R7)** | The scroll story, the STT card, `/api/topology`, terrain, veins, glacier, ash, wind, snowline, deposits, post FX, cannon-es |
| Cluster access **(R2)** | PromQL only, no RBAC, no ServiceAccount |
| Transport | WebSocket, a single `/ws`. The session cookie, read at upgrade, adds detail frames |
| Relay **(R2)** | 1 replica, `Recreate` |
| Truth | A real metric **sets** the state; the animation only shows the change. Exact counts (pods, requests) never leave the relay, only buckets |
| Visitors **(R2)** | Shared and persisted, after hardening and moderation (08) |
| Email **(R3)** | **Shared SMTP2GO sender. Keep Wird's email-verification stage.** The accepted risk is recorded in ADR-B, with a quota watch |
| Deploy **(R4)** | hq/wedding style. The helm values live in this repo (`helm/values.yaml`); CI pins the `:sha` tag in-repo; the ArgoCD app points at this repo's `helm/` |
| Push **(R4)** | Pushing to main deploys, so it is outward-facing: **merge only after the user confirms**. For R7 the owner asked for deploys after phase 1 and phase 4 |
| Plant room **(R7.1)** | The owner chose both: **plants wilt after 7 days**, and **on a full island the oldest plant gives way** (planting on its hex replaces it; every other planted hex stays taken). Otherwise the ~140 plantable hexes fill within days and every plant is refused forever |
| Names **(R7.1)** | The owner chose to **scrub the data and keep the plumbing**. Storage pools say `replicated`/`local`, never the product, and the null `garage`/`pigsty` keys are gone (metrics v3). The `argo.*` event kinds and the authentik sign-up link stay public; they name the mechanism, and the amendment says so |
| Unknown ≠ down **(R7.1)** | A host with no Ready answer yet is `ready: null`, drawn up. An empty Ready, quorum or pods answer is no answer and changes nothing. An empty town raises `MountainTownEmpty` |
| Islands sized **(R8)** | The owner chose **static hardware**: an island's radius follows its host's threads, RAM and disk (7/5/4, 06 Town geometry). Not live load: a box does not shrink because it is idle |
| Typology **(R8)** | The owner chose **families by function**: Defense, Public, Industry, Shops, Houses (03 Building kinds), plus streets. The family sets where a building stands, its ground and its label tier. Shape = class, roof = wire, ground = family, words = status |
| Machines **(R8)** | The owner chose to draw the **Terraform-level topology**: the k8s VMs (keeps, halls) and the guests outside Kubernetes, **probes first** (drawn only once blackbox measures them; infra-bootstrap ADR-0064). Their state is **debounced 10 min** both ways; host state stays live. **k9s-dashboard is never drawn.** Real hostnames reach only a signed-in `platform-admins` session (`/auth/me`), never the socket, a replay or the DB. Deploy B doubt cycle, owner 2026-10-22: **keeps and halls follow their island** (no per-node state: it would publish etcd quorum live); **granary keeps its live S3 traffic** (ADR-0064 records the early signal); infra-bootstrap's visibility is accepted as is; the **admin-only detail channel keeps its alert labels** (carve-out) |
| Annexes **(R8)** | Minority pods: a small tent beside the app on each other host it runs on. Deploy C |
| Camera **(R8)** | The overview is **fitted** (every island's rim above the dock); the projection centre sits above the dock so no view hides behind it; portrait picks its closest angle; the phone's overview shows island chips. The owner turned Cloudflare Web Analytics off (its beacon broke the CSP) |
| Re-placement **(R8)** | No new epoch: local hexes keep their meaning, so history and scrub stay. Migration 005 marks R8's start, and the first boot re-places every plot by family once. `hello.build` makes a page from another build reload once |
| Open **(R7)** | Host rename (`mountain` is still the hostname, CSP, OIDC redirect, `mountain_*` metrics). Drop the `mountain_archive_*` tables once the town has held for a while |

**Attribution, per repo** (agents must follow it):

| Repo | Rule |
|---|---|
| `mountain` | No AI trailer, no session link, no co-author (user's global rule) |
| `infra-bootstrap` | `Co-Authored-By: ukubi-claude-macbook <noreply@bnei.dev>` only, per its CLAUDE.md |
| `hq` | None |

**Review log.** R2–R6 (the mountain) is kept at the `mountain-final` tag.

These fixes still hold in the town:
- cross-site WebSocket hijacking (own `originOK`);
- unhardened enrollment;
- pgbouncer;
- `increase()` over a 15 s window (the relay diffs counters itself);
- namespace ≠ app;
- OOM gauge semantics;
- Watchdog filtered;
- mass `app.gone`;
- `/metrics` exposed publicly;
- Grafana strict being a no-op;
- the thot wake-up from warning alerts;
- PAT expiry;
- static maps rotting;
- the CF-Connecting-IP trust boundary.

**R7 doubt cycle (2026-10-22).** Three adversarial reviews (relay/data, front end, scope/process) plus a design pass
gave 22 findings, all folded into the plan.

What they changed:
- **Geometry:** Go owns the geometry.
- **Placement:**
  - the 5-minute move hysteresis;
  - no move off a NotReady host;
  - an empty `pods` answer keeps the plots;
  - ties go to the current host, then to sorted order;
  - 24 h ruins.
- **Wire:**
  - the field is `role`, not `kind` (`Out.frame` owns `kind`);
  - integer plant poses;
  - the toolbar reports the item the relay chose (from its `world:mark`), not its own preview;
  - only role keys reach the wire, never product names.
- **Front end:**
  - labels on every tier;
  - the visitor plant style differs from scenery;
  - the night is lifted;
  - the build goes beside the old page until the swap;
  - one InstancedMesh per kind;
  - a worst-case fixture gated on both tiers.
- **Process:** an archive, not a delete, because no dump could be taken.

**R7.1 doubt cycle (2026-10-22, after Deploy 2).** Three read-only reviews of the merged code (relay + migration,
front end, scope + deploy) found no blockers and about 30 findings. They are fixed in R7.1 (README status), apart
from these:
- A restored plot whose app never comes back to ArgoCD is not ruined after a restart. Services may have no Argo
  Application at all, so seeding the gone clock from plots would ruin them.
- The R7 commits' author field is `Claude <noreply@anthropic.com>`. It is left as is unless the owner asks, since
  rewriting main's history is out of scope.

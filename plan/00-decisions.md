# 00-decisions — Decisions

| Topic | Decision |
|---|---|
| Audience | A sanitised live layer, **gated since R8 deploy C** (below). **Detail mode** for `platform-admins` |
| Gate **(R8 C)** | The owner chose to **put the whole site behind authentik forwardAuth** rather than trim what the annexes show (infra-bootstrap ADR-0065): `Middleware/mountain-forwardauth` (`helm/values.yaml`) on the `mountain-gate` provider, for `platform-admins` and **invited** `mountain-visitors` (enrollment is invitation-only, no captcha). Behind it the page signs in to the relay by itself (no button), reloads once onto the login wall when the gate session lapses, and `/v/` assets are `private`. The sanitising, role keys and buckets stay, as defence in depth against an invited visitor. Agents can no longer check the live site |
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
| Truth | A real metric **sets** the state; the animation only shows the change. Exact counts (pods, requests) never leave the relay, only buckets. **R8 C:** replica buckets are coarse (0, 1, 2–4, 5–7, 8–15, 16+), so with the annexes they give no small exact counts; migration 006 rewrote the history to them |
| Visitors **(R2)** | Shared and persisted, after hardening and moderation (08). **R8 C:** invited only (an invitation stage replaced the Turnstile captcha) |
| Email **(R3)** | **Shared SMTP2GO sender. Keep Wird's email-verification stage.** The accepted risk is recorded in ADR-B, with a quota watch |
| Deploy **(R4)** | hq/wedding style. The helm values live in this repo (`helm/values.yaml`); CI pins the `:sha` tag in-repo; the ArgoCD app points at this repo's `helm/` |
| Push **(R4)** | Pushing to main deploys, so it is outward-facing: **merge only after the user confirms**. For R7 the owner asked for deploys after phase 1 and phase 4 |
| Plant room **(R7.1)** | The owner chose both: **plants wilt after 7 days**, and **on a full island the oldest plant gives way** (planting on its hex replaces it; every other planted hex stays taken). Otherwise the ~140 plantable hexes fill within days and every plant is refused forever |
| Names **(R7.1)** | The owner chose to **scrub the data and keep the plumbing**. Storage pools say `replicated`/`local`, never the product, and the null `garage`/`pigsty` keys are gone (metrics v3). The `argo.*` event kinds and the authentik sign-up link stay public; they name the mechanism, and the amendment says so |
| Unknown ≠ down **(R7.1)** | A host with no Ready answer yet is `ready: null`, drawn up. An empty Ready, quorum or pods answer is no answer and changes nothing. An empty town raises `MountainTownEmpty` |
| Islands sized **(R8)** | The owner chose **static hardware**: an island's radius follows its host's threads, RAM and disk (7/5/4, 06 Town geometry). Not live load: a box does not shrink because it is idle |
| Typology **(R8)** | The owner chose **families by function**: Defense, Public, Industry, Shops, Houses (03 Building kinds), plus streets. The family sets where a building stands, its ground and its label tier. Shape = class, roof = wire, ground = family, words = status |
| Machines **(R8)** | The owner chose to draw the **Terraform-level topology**: the k8s VMs (keeps, halls) and the guests outside Kubernetes, **probes first** (drawn only once blackbox measures them; infra-bootstrap ADR-0064). Their state is **debounced 10 min** both ways; host state stays live. **k9s-dashboard is never drawn.** Real hostnames reach only a signed-in `platform-admins` session (`/auth/me`), never the socket, a replay or the DB. Deploy B doubt cycle, owner 2026-10-22: **keeps and halls follow their island** (no per-node state: it would publish etcd quorum live); **granary keeps its live S3 traffic** (ADR-0064 records the early signal); infra-bootstrap's visibility is accepted as is; the **admin-only detail channel keeps its alert labels** (carve-out) |
| Annexes **(R8)** | Minority pods: on each other host an app runs on (02 Annexes). The owner chose (2026-10-23): **every app**, the site gated first (Gate row), **a 5-min hold** both ways like a move, and, after three tent shapes, **a full copy of the app's own building** (every kind fits a tile) marked by a banner in its wire colour, named with the machines; a tap opens the app's card. **Zoom levels**: banners in the overview, whole copies on the island in focus (it keeps the budget). Defaults: DaemonSet services (watchtower, warehouse) keep their permanent copies; Job pods never make one |
| Budget **(R8)** | 220 draws on both tiers; **350k tris on the high tier (desktop), 200k on phone-low** (owner, 2026-10-23) |
| Engine **(R8)** | After a doubt cycle on vendoring InstancedMesh2 (it culls nothing here: ~100 instances, the overview shows all; it adds draws per LOD level and breaks the night roof tint), the owner chose **measure first, then three.js's own BatchedMesh**: cheap shadow shapes, no walker shadows, a simplification test on the heaviest models, a per-mesh breakdown and a Civ worst case in the gate; then buildings on BatchedMesh (bodies one draw, roofs one), nothing new to vendor. Civ (R9) went first: its worst case fits the budget as is (gate) |
| Civ **(R9)** | The owner chose (2026-10-23) to read the town as a civilisation game **before the engine step**, every mechanic set by a real signal (Truth row). **Levels**: built from 30-day sync activity and signed off, then **taken out before shipping** (owner, 2026-10-23): levels will come from goals reached (09-play), since sync counts are cheap to farm; the look (taller, props, a stone wall and a weathervane at 3) waits for that. **Technologies**: the ADRs of infra-bootstrap, as a count and the highest number only (titles never leave the relay). **The herald** calls a decree at an island's square for a live sync, new laws (gitops commits) or a new ADR, never in a replay. **Golden age**: 7 days with no warning or critical alert and no Degraded or Missing app (`state.calm`, kept by the fold): **bunting** on the bridges and squares and a panel chip. **Turns**: days since the town's first day (`hello.snapshot.started`). A guide card names each mechanic and its signal. The riot (crashloop) is deferred |
| Audience **(R10)** | **Admins only, real names** (owner, 2026-10-24): mountain is internal, and a client-facing islands project comes later as its own thing. atlas enforces it: a deny-by-default mux (only `/auth/*` and `/healthz` open), `/ws` refused to non-admins, a test calling every route as a visitor, and `mountain-gate` narrowed to `platform-admins`. This supersedes the Audience, Gate, Visitors and Toy rows once A2 ships. **Amended by R12 (slice 4e1, 2026-10-26):** one machine principal, the authentik service account `atlas-game`, reads `GET /api/graph` and `GET /ws` (the town's reads, nothing else) with a 10-minute `client_credentials` token checked offline against a pinned certificate (infra-bootstrap ADR-0068); the route test calls every route as it too |
| atlas **(R10)** | **The town is rebuilt as realm islands** (Infra, Platform, Data, Identity, Events, Fleet, Apps) and the relay becomes **atlas**: one service for the projector, live overlay, history, WS, OIDC and page. The site is a work in progress, so frozen files may change and history is dropped. Hardware is the land (a CPU/RAM/disk lens), not a second map. 5 steps: map, empire (trade routes), 3D city, building with CSS3D cards, reader. Repo, namespace and image stay `mountain`. Design: `10-understanding.md`. This supersedes the Metaphor, Islands, Placement, Services, Geometry and Annexes rows once A2 ships **Amended by R13 (2026-10-27, `15-architecture.md` §8):** the game's client becomes the one 3D surface; L0–L3 retire as soon as the game's one-screen client (slice 4f) lands, before the game deploys (owner), and atlas keeps its reader (L4) |
| Play **(R10)** | **Parked** (owner, 2026-10-24): watchers and players contradict admins only. agent-kit (the other session) brings the process stages as a city's build queue instead (10, joint section) |
| Game town **(R12, draft)** | The owner chose (2026-10-25, `13-game-town.md`): **everything but the hardware is a building** (hosts are the land lens; hostnames fold as doors, single-app repos as workshop annexes, ADRs are technologies); **civs are the realms, reshaped** (Platform, Data, Operations, Guard, Fleet, Apps, Code), with a separate `function` field, owned by atlas (2026-10-26: models are derived from atlas's facts, never pinned in a `model:` field; walls sit on the doors: forwardAuth a wall, OIDC or a path's forwardAuth a gate, `public` and `own` marks, the rest open; mesh peers become walls at ADR-0055's cutover); the town centre is atlas and the game as one, rendered in the game client; supply lines, watchtowers by real coverage, walls from Guard, real research, capacity as real cost; **one age for the town, paid by goals, research, unique daily users, latency and stability, held or lost**, built only once every input is measured and mirrored by the sim, and director-independent in the sim; units are live sessions of the game's client only; kits are CC0 free tiers, kitbashing and procedural, repainted into palettes, in a public `ukubi-kit`. **Amends**: atlas R10 (the realm list reshaped; repos leave the rim for code; hostnames stay at the Cloudflare rim in atlas and are doors in the game), Audience R10 (one read-only machine principal `atlas-game` for the town centre; the game client gets its own admins-only gate before it leaves loopback), Civ R9 (levels and score give way to the age; golden age renamed the calm streak; technologies from every repo's ADRs), 09-play (ages can fall, ages only; traffic counts as unique users; the tech tree unparked without unlocks; named players stay parked). Open questions are listed in 13 |
| Console **(R13, draft)** | The owner chose (2026-10-27, `14-console.md`): **complete control of the cluster from the game**. Each app type declares its actions through one Go `Provider` interface in the game's server (kubernetes, argocd, patroni, postgres, logs, fleet, links, and the game's own verbs), never in atlas, which stays read-only; the page holds no credential and draws only what the server lists for the selection, with drawers chosen by transport, never by provider. **Direct writes, bounded**: a ValidatingAdmissionPolicy admits only what each action needs, exec skips the platform namespaces, and limits widen one action at a time by ADR amendment; every write is confirmed with a single-use token bound to the session and what it touches, audited with the API server's `Audit-Id`, and the strongest ask a fresh sign-in after 10 minutes. Pods in full (exec, logs, describe), the repo in vscode.dev, the VMs' databases (users, connections, pgweb, Patroni); Postgres roles read live from Patroni, never from `pg_role`; **Secret values never read** (owner: only through Infisical); ArgoCD's diff through a scoped ArgoCD account. The sim mirrors every source (Kubernetes, ArgoCD with its controller and self-heal, Patroni, a Postgres wire mirror, Loki and Pigsty's stores, authentik's re-auth). **Amends**: R11 (goals, steps and wonders deferred in the client, the fleet island in their place; the engine stays); plan/10's read-only rule holds for atlas only. Its credentials land as an infra-bootstrap ADR in four parts, one per slice. Open questions are listed in 14 |
| Architecture **(R13)** | The owner chose (2026-10-27, `15-architecture.md`, after an architect interview and a doubt cycle): **one standard for every ukubi app**. Narrow interfaces per source, owned by the consumer; no fakes of what we own; one contract suite per foreign source over a recorded real leg, the sim's mirror and a map fake; client-go for Kubernetes. Ordered `{Name, Version, Run}` stages for one-shot transforms; state machines only for lifecycles we own (wonders, write tokens, sim incidents), the event-log fold staying total. A thin main, `internal/` packages, no mutable globals; shared code extracted on second use into the nested module `mountain/pkg`. Connect's 16 codes plus a key, the page's words, the source and its own words, with a closed key registry per app. One validated Config per binary. `Log[E]`, file first, Postgres when an app has its own database. OpenTelemetry: traces pushed over OTLP (to Tempo on Garage S3; amended 2026-10-28: the backend is open, and every app's tracing stays inert until its endpoint is set), metrics still pulled (names pinned), logs JSON on stdout with trace ids (2026-10-28: atlas, the game and the bridge move to it). `pkg/` tags may be pushed by the agent (2026-10-28), never an app's. Bun everywhere (the kit rebuilt and reviewed); the game's web in TypeScript with a committed dist and no inline script |
| Dev rules **(R13)** | The owner asked (2026-10-27) for dev rules from the standard's rollout, so future agents work the same way: **every non-trivial change goes plan (facts checked against the code, every test edit and behaviour change listed), doubt cycle (a fresh adversarial agent, every finding taken or answered), the proof built first from the old code, build, local checks, an independent review (every finding fixed or answered), PR, merge.** A refactor proves it changed nothing (goldens run twice on main first and shown to reach the changed path, mutation harnesses, dist byte checks per engine and platform). A step is skipped, and a PR merged, only on the owner's word. Amends plan/15's "existing suites pass unedited": no assertion is edited, and setup, package clauses, imports and renames may change, each listed in the PR. Canonical in `16-dev-rules.md`; ukubi-game's and ukubi-kit's CLAUDE.md and the fleet's `change-loop` skill condense it |
| Config defaults **(R13)** | The owner (2026-10-28): "put sane defaults where they fit, and only fail on required keys for the life of the app." **An optional setting that is set and invalid is a WARN line and its default; only a missing or unusable required setting refuses the start**; security settings fail closed, by a refusal or a closed gate (plan/15 §5). Amends plan/15 §5's "fails closed". Applied to the optional settings of atlas and the game (the traces endpoint), the bridge, and the fleet's core and provisioner |
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

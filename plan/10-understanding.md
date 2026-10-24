# 10 · atlas: understanding the system (design, 2026-10-24)

> Status: **design**. The owner went through five interview rounds, two doubt cycles (five read-only reviews) and an
> architect round. Nothing here is built. It is written down as step A0; the code follows in slices A1–A6 (end of
> this file). A section is shared with the agent-kit design (the other session's work) and is signed there before
> anything assumes its schema.

## Why
ukubi is too big to hold in one head: hosts, VMs, the cluster, the platform, apps, the fleet, the bridge, and about
230 ADRs across repos. Reading docs or code gives all of it at once, flat. The owner wants one surface that sets
three dials:
- **altitude**: how abstract the view is;
- **load**: how many items are shown at once;
- **overview**: never losing your place.

The first lenses are **structure** (what exists, what contains what) and **flows** (who talks to whom). The model is
Civilization's zoom ladder (map, empire, city, building), with two Age of Empires helpers for real-time work.

The binding rule still holds: every image is true and tied to something real.

## What changes for mountain (R10 rows of 00)
- **Admins only, real names.** mountain is now internal; a client-facing islands project comes later as its own
  thing. The sanitiser, role keys and buckets go with the town; real names appear everywhere.
- **The town is rebuilt as realm islands.** The site is a work in progress with no real use and no data worth
  keeping. Frozen files may change, history is dropped, and the visitor features (plants, enrollment) are deleted with
  the town. `09-play.md` is parked: watchers and players contradict admins only.
- **atlas absorbs the relay.** One Go service: projector, live overlay, history, WebSocket, OIDC and the page.
  "atlas" is the code name only. The repo, namespace and image stay `mountain`, because a rename breaks the CI
  identity's `boundClaims.repository` (`infra-bootstrap/docs/secrets.md:238-244`) and the build-runner registration
  (`infra-bootstrap/ansible/playbooks/build-runner-configure.yml:98-99`).

## The ladder
| Step | What you see | Drawn as |
|---|---|---|
| **L0 Map** | Realm islands: Infra, Platform, Data, Identity, Events, Fleet, Apps. The outside (GitHub, Cloudflare, Discord, laptops on the mesh, the model API) sits at the rim. Troops are live agent sessions, and alerts ping where they happen | 3D |
| **L1 Empire** | Trade routes between systems, colour-coded per wire and weighted by observed traffic | 3D, over L0 |
| **L2 City** | One system's district: its parts, routes in and out, secret names, the ADRs that govern it, its **build queue** (work in flight at its process stage) and its **worked tiles** (its share of each host) | 3D |
| **L3 Building** | One part (a pod, an RPC, a definition, a table). Its walls carry live HTML cards: facts, routes, last events | 3D + CSS3D |
| **L4 Read** | An ADR, a file, a diff or a session transcript in a full reader. The 3D scene shrinks to a corner minimap | 2D |

**Hardware is the land, not a second map.** The owner first asked for a switchable physical/logical lens, then chose
this after pushback: a toggle that re-lays out the map breaks "never lose your place", and the two trees are
lopsided (physical is shallow, logical is deep). Like Civ's tile yields:
- a **land lens** recolours and annotates the map without moving anything;
- CPU, RAM and disk count as a **per-pod share of cluster capacity**, plus an explicit "unattributed" remainder.
  Summing hosts per realm would count DaemonSets and replicated disks up to three times;
- the Infra city is where hardware is the subject (hosts, VMs, LXCs, disks).

**The load rule.**
- At most about 7 **groups** per view, each a typed list, folded by a real sub-key (RPC service, wire, ADR status),
  never by overflow.
- Search is the way out of a fold.
- A breadcrumb and minimap are always on screen.
- Hash routes (`#/L2/fleet/core?lens=land`): back zooms out one step, and every view is a link.

Real counts it has to hold: 15 registry apps, about 22 platform services, 6 fleet components, about 100 RPCs over 5
services, and 67 + 79 + 21 ADRs.

**Age of Empires helpers.**
- **Waiting on you**: a counter of sessions blocked on a person and open proposals, from agent-fleet ADR-0078's
  `session.waiting`. A key jumps through them.
- **Last event**: one key flies to it, at the deepest level where its node stands alone. If it is folded, its
  parent opens with the group expanded.

**Rendering rules (from the doubt cycles).**
- One scene per level, unloaded on exit, so each level fits the 220-draw budget alone. `dev/gate.mjs` gets a case
  per level.
- **CSS3D cards:**
  - shown only at L3, at most 7, at native pixel size;
  - the CSS3D root keeps `pointer-events: none`, and one card takes the pointer only while it is focused, so cards
    never fight orbit, pinch and wheel;
  - below about 600 px they become flat dock cards (iOS blurs and crashes on many `preserve-3d` layers);
  - CSS3D draws in its own layer, so geometry can't hide a card. Accepted at L3.
- HTML-in-Canvas is a later upgrade: it is a Chrome-only origin trial, with names still changing.
- **The reader:**
  - atlas renders markdown with goldmark (raw HTML off) to token JSON, and the page builds it with `createElement` and
    `textContent`;
  - the page still has no `innerHTML`;
  - highlighting is by class names only, because the CSP is `style-src 'self'` (`relay/main.go:400`);
  - `javascript:` links are refused;
  - output passes a secret-pattern redaction pass;
  - while reading, the 3D is frozen to a still image.

## CQRS
**Write side (atlas never writes it).**
- The repos at the branch ArgoCD tracks for each app (`targetRevision`, not always main: `vos-monolith` has `dev`,
  `has-journal` has `master`).
- The live stores: Prometheus, Alertmanager, the fleet DB through its events, and the bridge's log.

**How atlas hears about changes.**
- The bridge's `pull_request.closed` with `merged: true` (it carries `merge_commit_sha`) is only a **hint** to look
  sooner.
- Truth is a poll of each tracked head, because CI and release-it push straight to main
  (`.github/workflows/app.yml:215-235`, agent-fleet `docker.yml:146-209`), and the bridge drops `push` on purpose
  (`ukubi-bridge/internal/github/github.go:49`, ADR-0060 §5).
- History comes from `git log`, never from the bridge, whose retention is 30 days.

**Projection.**
- **One projector goroutine** with a coalescing queue. A hint only triggers a fetch, and the vector is the set of heads
  after that fetch.
- **A two-phase vector** `{repo@branch → sha}` with an atlas sequence number. The infra-bootstrap sha comes first; its
  registry and ApplicationSets give the keys, then the app shas follow.
- **Declared vs deployed.** The vector holds a declared sha (the tracked head) and a deployed sha (ArgoCD's sync
  revision). Their difference is drift. The source of the deployed revision is confirmed in A4.
- **Moves.** A move is accepted only if it descends from the current sha (`is-ancestor`); a force-push starts a new
  epoch.
- **Extractors.** Each is a versioned Go package with an **allowlist of fields**, never a whole-file parse. Its output
  is cached by (version, the shas it reads), so only extractors whose inputs changed re-run.
  - Helm keys matching `(?i)secret|password|token|key` are dropped.
  - Vendored trees (`pigsty/conf`, `pigsty/roles`) and submodule paths are skipped; a submodule pointer is a `git`
    edge only.
  - Any extractor reads app values at the same vector's `repo@targetRevision`.
- **Rebuilds.** A snapshot is a pure function of (extractor versions, vector). A fixed extractor re-projects into a
  new epoch rather than patching rows.

**Read side (mountain's Pigsty DB, a new migration series).**
- `vector(seq, repo, branch, declared_sha, deployed_sha, at)`.
- `node_ver` and `edge_ver`, keyed by content hash, with `valid_from_seq` and `valid_to_seq`. "History of a node" is
  a range scan. One jsonb blob per seq would grow about 70 MB a day, since image-pin bumps move the vector many times
  a day.
- One aggregate per level per seq, for the views.
- The live `events` log.
- A test fails if any stored string looks like a secret (high entropy).

**Nodes and edges.**
- A node is `{id, kind, level, parent, realm, source path@sha}`.
- An edge is `{from, to, kind: http|grpc|event|sql|git|secret|auth|governs, declared|observed}`.
- **Ids are `kind:qualified-name`**, stable across shas: `host:h1`, `vm:k8s-cp-01`, `app:mountain`,
  `svc:agent-fleet/core`, `rpc:agentfleet.v1.CoreService/PublishEvent`, `adr:infra-bootstrap/0064`,
  `secret:<project>/<name>`.
  - Renames get an alias edge.
  - `wird` and `wird-adminweb` share a namespace, and agent-fleet's core has three names. So `realms.yaml` holds
    alias rows: app → services → namespace/deployment.
- **"Governs" edges come from ADR citations in files**, because no ADR names the components it governs.
  - Numbers collide across repos (infra-bootstrap 0064 and agent-fleet 0064 both exist).
  - So a prefixed citation (`infra-bootstrap ADR-0064`) resolves to that repo, and a bare one to the citing repo's
    own ADRs.
  - mountain has none, so its bare citations map through `realms.yaml`.
  - Anything still ambiguous becomes a drift node, not a guess.

**Realms are curated, and that is said plainly.** Nothing in the repos carries a realm. `realms.yaml` (in this repo)
maps component → realm and group. A component atlas finds that isn't in it shows as an **unassigned** drift node,
and CI fails on a new one. It moves into the service registry when the planned move to medium, islands-oriented
services brings one.

**First realm draft** (for the styleboard; `realms.yaml` decides):

| Realm | Groups |
|---|---|
| Infra | hosts h1–h3; k8s nodes (5); network (Cilium, MetalLB); storage (Longhorn, local-path, csi-nfs, nfs VM); build (build-runner, actions-runner); GPU (nvidia plugin). k9s-dashboard is never drawn |
| Platform | delivery (ArgoCD, zot); edge (Traefik, redirectors); observability (Prometheus, Grafana, Loki, Alloy, blackbox, Alertmanager); tools (searxng, pgweb, silverbullet, ente) |
| Data | Postgres (pg01, pg02, the etcd witness); Redis; Garage S3; backups |
| Identity | authentik (blueprints, outposts); secrets (Infisical, its operator, agent-vault); mesh (NetBird); GitHub Apps |
| Events | ukubi-bridge; alert routing (Alertmanager to Discord and core); webhooks in |
| Fleet | control (core, provisioner); pod (worker, sidecar); executor and thot; definitions; hermes |
| Apps | the other registry apps, folded by a real sub-key in `realms.yaml` |

## Flows
**Declared** extractors are the "should be" side, the routes the repos say exist:
- Helm `env` URL hosts;
- authentik blueprints (auth edges);
- bridge subscriptions and the client table;
- proto clients;
- `InfisicalSecret` refs (names only);
- the 4 network policies.

**Observed** traffic weights them. A declared route with no traffic is drawn dashed; traffic with no declaration is
drawn red. That mismatch is drift, and often the most useful thing to see.

- **Hubble (now).** It is enabled (`infra-bootstrap/inventory/ukubi/group_vars/k8s_cluster/k8s-net-cilium.yml:11-23`),
  but its `flow` metric has no source or destination context and nothing scrapes it.
  - The setting becomes
    `flow:sourceContext=workload-name|reserved-identity;destinationContext=workload-name|reserved-identity;labelsContext=source_namespace,destination_namespace,traffic_direction`.
    kubespray v2.31.0 passes the list through unchanged.
  - A gitops ServiceMonitor picks up the chart's `hubble-metrics` Service on :9965. It does not go through
    `cilium_extra_values`, because the CRD doesn't exist on a fresh `cluster.yml`.
  - The agent-fleet namespace is collapsed in `metricRelabelings`, since each Job is a new workload.
  - This gives pod-pair routes with no ports and no payloads.
- **OTEL (with atlas).** Services we write carry otelconnect or otelgrpc interceptors (connect v1.21.0; otel is
  already an indirect dependency).
  - Spans go to a **1-replica Alloy traces Deployment**. The node DaemonSet can't pair client and server spans across
    nodes; the servicegraph connector keeps unpaired spans for 2 s.
  - The servicegraph connector, with an `rpc.method` dimension, writes
    `traces_service_graph_request_total{client,server}` into Prometheus by remote write.
  - That needs `enableRemoteWriteReceiver`, with ingress to `/api/v1/write` limited to that Alloy.
  - No Tempo for now.
  - Both ends must be instrumented, so atlas and the bridge go together, then the fleet.
- **Later:** the service registry, as one more extractor.

## Security
- **Admins only, enforced by atlas.** Today `/ws` and the static files are served to anyone the gate admits
  (`relay/hub.go:133-163`, `relay/main.go:358-360`), and the gate admits invited visitors.
  - A deny-by-default mux opens only `/auth/*` and `/healthz`.
  - `/ws` refuses any role but admin.
  - A test registers every route and calls each one as a visitor.
  - In infra-bootstrap, `mountain-gate` narrows to `platform-admins`.
- **Git access.** A new read-only App `ukubi-atlas`: contents, metadata, pull_requests, checks and actions read.
  `actions:read` replaces the PAT's build reads.
  - It is installed on an explicit repo list, never "all".
  - The key is a mounted file, and only the 1 h installation token is kept in memory.
  - `ukubi-fleet` is never used (agent-fleet ADR-0075 reserves it for agents).
  - The relay's PAT stays as a fallback until a release proves the App path, then it and `MountainTokenExpiring` go.
- **Runtime.**
  - go-git with on-disk storage (the image is distroless, with no git binary);
  - an `emptyDir` cache with a `sizeLimit`, since the clones are a cache;
  - gitlinks only, never submodule contents;
  - explicit limits of 1–1.5 Gi, because the chart's LimitRange default is 512 Mi.
- **Secret values never stored or shown.** The field allowlist, the entropy test, and redaction on reader output.
  Redaction matters because the clones hold full history, including the deliberate ArgoCD webhook literal
  (`infra-bootstrap/gitops/bootstrap/argocd-application.yaml:159-169`, a documented low-stakes exception).

## Joint section: agent-kit (signed by the "Fleet and infra assessment" session)
> Signed by that session on 2026-10-24 (with four corrections, folded in). The shape is owned by agent-kit; when that repo exists, its `schema/` becomes the source and
> this section pins its sha. Nothing in atlas assumes it before it is signed.

- **The surface is one model with two halves.** atlas shows what the system *is* and how it flows. agent-kit shows
  how work *moves through* it.
- **Units and abilities.**
  - Agent definitions and sessions are **units**: troops at L0, workers at L2.
  - Skills and packs are their **abilities**, shown at L3.
  - A unit card reads two sources: the persona at agent-kit's sha and the authority at agent-fleet's sha.
  - Each runtime's `.claude/rules/99-runtime.md` (it sorts last, so the runtime facts win over the `20-*` packs; on laptops, `~/.claude/rules/99-runtime.md`) names the agent-kit sha it runs, so skill drift per runtime shows.
- **Process stages are a city's build queue.** The fixed enum: ideation, design, interview, doubt-design, architect,
  doubt-architect, plan, doubt-plan, sync, implement, quality, live-qa, pr, review, merge, deploy-watch,
  post-deploy-qa.
- **`process.yaml` maps onto atlas nodes:**
  - nodes: `stage:<id>`, `skill:agent-kit/<id>`, `pack:agent-fleet/<id>`, `def:agent-fleet/<name>` (the one `kind:qualified-name` form);
  - edges: stage→skill (uses), definition→skill (has), definition→stage (runs), skill→pack (shipped-in).
- **Evidence** `{work_id, stage, result: pass|fail|skipped|waiting, sha, runtime, at}`.
  - **Before a PR:** bridge events under `dev.bnei.process.`, published by core and read only by atlas's client.
    Never under `dev.bnei.agent.`, which the dispatcher reads (`agent-fleet/core/internal/dispatch/dispatch.go`).
  - **On a PR:** check runs `process/<stage>`, posted only by a verifier App `ukubi-gate` whose key core holds
    (never minted into pods). Verdicts follow agent-fleet ADR-0079's ledger, with the rule that a session that wrote a commit on
    the PR can't review it. The bridge ingests `check_run.completed` filtered by `app.id`, under
    `dev.bnei.process.check`, with an ADR-0060 amendment.
  - **Per-repo table:** each repo is recorded as a **gate** (a ruleset is possible) or a **signal** (a private repo on
    GitHub Free). agent-kit is **public**, so it is a gate (owner, 2026-10-24). It is the laptops' supply chain, so it
    joins the infra repos under the `process/*` rules, with gitleaks in its CI. Until `ukubi-gate` exists, only the
    rulesets that need no checks apply.
- **Commit trailers label commits per unit.** atlas reads them with `git interpret-trailers --parse`. They are claims,
  never evidence: evidence stays with the agent-fleet ADR-0079 ledger and `ukubi-gate`.
  - `Co-Authored-By: <agent> <noreply@bnei.dev>`, where the agent is one of:
    - `ukubi-claude-<host>` for laptops;
    - `ukubi-claude-cloud` for claude.ai/code;
    - `ukubi-agent` for fleet repo pods;
    - nothing on definition pods, whose author is already the definition's `git.identity`.
  - `Agent-Session: <opaque id>` on fleet and cloud sessions.
  - `Agent-Definition: <name>@<sha>` on definition pods.
  - `Agent-Kit: <sha>` once vendored.
  - No model ids and no URLs. mountain's own rule (no AI trailer) is unchanged until the owner says otherwise.
- **Drift atlas shows for the process:** a PR merged with neither `process/review` nor an approving human review (laptop PRs keep human review: a laptop's `gh` user can't post `process/*` checks, and laptop work isn't in the ledger), and skill drift per runtime.
- **atlas's bridge client:** `bridge-atlas` (ADR-0066 service account) subscribes to `dev.bnei.github.pull_request.`
  on `github/`, and to `dev.bnei.process.` once that lands.

## Delivery
Each slice gets a code review and a merge on the owner's word.

| # | Repo | What |
|---|---|---|
| A0 | mountain | This file, the R10 rows of 00, and a 5-step styleboard for the owner's sign-off |
| A1 | mountain | pure `internal/graph` and `internal/extract`: registry and ApplicationSets, terraform and inventory, ADR indexes and citations, `realms.yaml`. An **additive** migration and an admin-only `GET /api/graph` on the relay's mux. Tests run over local checkouts |
| A2 | mountain + infra | `cmd/atlas` (OIDC, the generic hub and the Prometheus/Alertmanager parsing copied over, keyed by node id), the www rebuild L0/L1 and the land lens. Deletes `relay/`, `FROZEN.sha256`, `freeze_test.go`, the rollback scripts, the old tables and the town alert rules (`helm/values.yaml:152-191`), **in one deploy** so the live site never runs half of it. The gate narrows to admins |
| A3 | mountain | L2 cities, L3 CSS3D cards, the L4 reader with redaction |
| A4 | infra + mountain + bridge | Hubble metrics and the ServiceMonitor, the Alloy traces Deployment, the remote-write receiver, OTEL in atlas and the bridge, the live overlay |
| A5 | infra + bridge | the `ukubi-atlas` App and its Infisical project (its own InfisicalSecret and identity), the `bridge-atlas` blueprint and client entry, the poll and hints, retiring the PAT |
| A6 | mountain (+ fleet, bridge via agent-kit's session) | the remaining extractors (proto, definitions, blueprints, env hosts, network policies, bridge clients), the drift views, OTEL in the fleet, the agent-kit extractor |

**Human-gated** (queued; the cluster and build runner are down):
- create the App, its key and its installs;
- create the Infisical project and identity;
- the kubespray run for Hubble (it rolls the cilium agents);
- the authentik worker restart for the new blueprints (ADR-0066 is still proposed);
- `workflow_dispatch` re-runs of builds queued while the runner was down. Deploy C2 and Civ are merged but not live.

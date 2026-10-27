# 14 · R13: the console, complete control of the cluster from the game (draft, 2026-10-27)

> Status: **draft**. It comes from the owner's review of the one-screen wireframes (2026-10-27), one doubt cycle (15
> findings, all taken) and the owner's answers after it. Nothing is built, and each slice below gets its own plan
> and doubt cycle when it starts.
>
> It extends `13-game-town.md` (the town is where the console lives) and follows `15-architecture.md` (the code
> standard it is built to). It amends rows of `00-decisions.md`, listed under "What changes elsewhere".

## Context
The game (R11, R12) draws the cluster as a town and reads it through atlas. Today it can act on only three things:
goals, merges and fleet sessions. The owner wants it to be where the cluster is run from:

> "The goal is to have complete control from the UI over our cluster. We aim toward that. That is why we can go as
> low as opening vscode remote and seeing the files themselves."
> "The goal here is to create a technical system where adding functionalities like that should be flexible enough
> to fit in a golang interface."

The layout is the one-screen wireframe, agreed in Claude Design (canvas "Ukubi game · one-screen wireframes", 13
boards):
- **Desktop:** the map, a district, a building, a part, the fleet island, an app with its rollback confirmation, the
  same app after the rollback, the town centre, and the sim.
- **Phone:** the map, a building, the fleet island, and a restart confirmation.

The brief behind it is the doc "ukubi game: one-screen wireframe brief".

## Owner decisions (2026-10-27)
**Scope**
- **Complete control is the aim.** Each app type defines what can be done on it. Logs and ArgoCD sync are first
  examples, not the limit.
- **One Go interface.** A new capability is one more provider, or one more action on a provider.
- **Goals, steps and wonders are deferred.** The fleet island takes their place: one agent session per selection
  (building, district or town). The goals engine and its log stay; only the client stops drawing them.

**Writes**
- **Direct writes, bounded.** The game calls ArgoCD, the Kubernetes API, Patroni and Postgres itself. An admission
  policy limits each write to exactly what its action needs, and the limits widen one action at a time, each by an
  ADR amendment.
- **Every write is confirmed and audited.** Its confirmation names exactly what it touches.
- **A fresh sign-in for the strongest actions** (an exec shell, cordon, a sync across a district, a Patroni
  switchover) when the session is older than 10 minutes.

**What the game can reach**
- **Pods in full:** exec, logs and describe.
- **VS Code opens the repo in the browser** (vscode.dev on the repo that builds the building).
- **Databases on the VMs too:** users and open connections, a database console (pgweb, which already runs), and
  Patroni's controls.
- **Full admin detail.** Postgres shows its live primary and replicas, lag, connections, databases and sizes, read
  from Patroni and Pigsty's metrics. Never from `pigsty.yml`'s `pg_role`, which has been wrong twice (its comments of
  2026-10-03 and 2026-10-18).
- **Secrets: the game never reads a value.** Values are reached only through Infisical: a link to the project and
  environment, or Infisical's secret sharing.
- **ArgoCD's diff to git: yes,** through a scoped ArgoCD account of the game's own.

## 1. Where it lives
- **In the game's server (`ukubi-game`), never in atlas.** atlas stays read-only (plan/10) and keeps feeding the map;
  the console reads and acts on the sources directly.
- **The page holds no credential.** It asks the server what can be done on a target, shows it, and asks the server
  to run it. Every token, password and kubeconfig stays on the server, in the game's own Infisical project.
- **Only a signed-in platform admin reaches it.** Slice 4e2's gate already guards every route: the session cookie,
  the Host check, and an Origin check on every unsafe method.
  - The gate grows two things here: an Origin check on websocket upgrades (§2), and a session id that can be
    revoked (§5).
- **One replica in v1.** Single-use write tokens live in its memory.

## 2. The interface
```go
// A Provider brings one family of capabilities. The registry holds them all; the page knows none by name.
type Provider interface {
	Name() string                                    // "kubernetes", "argocd", "patroni", "postgres", "logs", "fleet", "links", "game", "world"
	Actions(ctx context.Context, t Target) []Action  // what applies to t, from its facts; cheap, no writes
	Run(ctx context.Context, r Request) (Result, error)
}

// Target is what the player selected, resolved server-side: never a name the page sends.
type Target struct {
	ID, Kind, Function string // atlas's node: "app:dreamer", "app", "products"
	Facts              Facts  // namespace, ArgoCD app, workloads, pods, host IP, Patroni member, databases, Infisical project
}

type Action struct {
	ID, Label, Key string    // "kubernetes.restart", "Restart", "X"
	Effect         Effect    // Read or Write: what it can change
	Transport      Transport // Once (an answer), Stream (logs that follow, a shell) or Link (a URL)
	Args           []Arg     // what the player chooses: a revision, a pod, a replica count
	Touches        string    // for a Write: what it acts on, in full ("deployment dreamer in namespace dreamer, 2 pods")
	Fresh          bool      // needs a sign-in younger than 10 minutes
	Off            string    // greyed with this reason ("Postgres is not an ArgoCD app"); empty when available
}
```
**How the page uses it**
- **The page draws `Actions`.** Each action becomes a command with its key, greyed with its reason when it's off.
- **The drawer is chosen by transport, never by provider.** An answer, a stream and a link each have one generic
  drawer, so a new provider never changes the page.

**How the server runs it**
- **The server never trusts the page's list.** On `Run` it resolves the target again, recomputes the actions, and
  refuses one that no longer applies.
- **Writes are aimed from the source's own facts.** The namespace an ArgoCD write lands in is the Application's
  `destination`, not what atlas says.
- **A Write runs only with a confirmation token.** Effect, not transport, decides this, so an exec shell (a
  Write-effect Stream) needs one too.
  - The confirmation shows `Touches`, the args, and what follows.
  - Confirming returns a single-use token bound to the session id, the action, the target, the args and `Touches`.
    It is valid for 2 minutes.
  - On `Run`, the server recomputes `Touches` and refuses the write if it changed since the confirmation.
- **A Stream runs over a websocket on the same origin.**
  - The gate requires an exact Origin and `Sec-Fetch-Site: same-origin` on every `Upgrade` request, with a test.
    Today `gate.go` checks Origin only on unsafe methods, and an upgrade is a GET.
  - The stream ends when the session does.
- **Errors use the standard model** (`15-architecture.md`). A refused write shows its `Key`, the page's wording, and
  the source's own words, such as what Kubernetes, the admission policy or ArgoCD answered. Never just "failed".

**The game's own commands are providers too.** Today `cmd/game/main.go` hand-writes 11 handlers, each repeating
decode, wrap, fail and reply, plus nil and world checks. They become providers:
- **game:** goal and wonder verbs, kept but not drawn while goals are deferred;
- **fleet:** answer, say, open a session (the island). It is registered only when a fleet token exists, so the nil
  checks go;
- **world:** merge and incident answer, through the sim `Control` seam.

The existing paths stay as thin aliases, so `gate_test.go`'s route table and the integration tests hold.

## 3. The providers (v1)
| Provider | Reads | Writes | Streams and links | Through |
|---|---|---|---|---|
| kubernetes | pods, describe, events, the object as YAML (redacted); which Secrets a workload references | restart a workload, delete a pod, cordon or uncordon a node; scale (greyed unless auto-sync is off) | logs that follow; an exec shell (Write effect) | client-go, with the game's ServiceAccount |
| argocd | the app, its history, its diff to git | sync, rollback (auto-sync off first), auto-sync back on | none | `Application` objects through Kubernetes; the diff through ArgoCD's API, with the game's ArgoCD account |
| patroni | members, roles, lag, timeline, paused | switchover, restart a member, reinit a replica | none | Patroni's REST API on each member (port 8008) |
| postgres | users, connections, databases and sizes, locks, slow queries (redacted) | cancel or end a connection | open pgweb; a SQL console later | `dbuser_game` on the primary |
| logs | the last N lines | none | follow | Loki for pods; Pigsty's VictoriaLogs for the VMs |
| fleet | sessions on this target | open a session, stop one, answer, say | the fleet island | core's AgentService (R11) |
| links | none | none | the repo in vscode.dev, the app's door, its Grafana dashboard, its Infisical project | nothing: a URL |
| game, world | goals, wonders, incidents | the game's verbs; merge; answer an incident | none | the game's log; the sim's `Control` |

**Reads never show a secret.**
- Secret objects are never read. The game lists core resources one by one, and `secrets` is not among them.
- Which Secrets a workload references comes from its `envFrom` and volume refs. Each links to its Infisical project
  and environment (named by the app's `InfisicalSecret`), or to Infisical's secret sharing.
- plan/10's redaction pass runs on every YAML, describe and `pg_stat_activity` text the game shows. That includes
  the literal `env:` values the chart renders with `toYaml`.

**Which actions a building gets** comes from its facts, never from its name:

| Facts | Gets |
|---|---|
| an ArgoCD app | argocd, kubernetes (its workloads), logs, links |
| a k8s node (houses) | kubernetes node actions: describe, cordon, uncordon. Drain is an open question |
| add-ons | kubernetes (their DaemonSets and Deployments), logs |
| Postgres | patroni, postgres, logs (VictoriaLogs), links (pgweb) |
| other VMs (guests) | read only (probes) until SSH is decided |
| any building | fleet, links (its repo) |

- **Off, with the reason shown:**
  - writes on `bootstrap` and `argocd` themselves;
  - exec in an excluded namespace (§5);
  - ending a superuser backend, which `pg_signal_backend` cannot do;
  - ending the replication walsender, which would cut replication.
- **A part** (a pod or a member) gets the same actions, scoped to itself.
- **A district or the town** gets only what makes sense across many targets: the fleet island, and sync for every app
  in a district. That write lists every app it touches and needs a fresh sign-in.

**What each write does, exactly**
- **Restart:** the `kubectl.kubernetes.io/restartedAt` pod-template annotation.
- **Cordon:** `spec.unschedulable` on the node.
- **Scale:** through the `scale` subresource. It is greyed while auto-sync is on, because self-heal puts the chart's
  `replicas` back.
- **Rollback:**
  - Writes `operation.sync` on the Application, with `revisions` and `sources` for every source of a multi-source
    app, taken from its history, and `initiatedBy` the signed-in human.
  - Before that, it turns `spec.syncPolicy.automated` off.
  - The confirmation says three things: PreSync hooks run again, prune deletes objects newer than the target
    revision, and the app then stays off git until auto-sync is turned back on.
- **Switchover:**
  - Through Patroni's REST API.
  - The confirmation and the audit say that `pigsty.yml`'s `pg_role` labels are then wrong again: the trap where a
    Pigsty run aims writes at the standby.

## 4. The sim
The binding rule holds: the sim mirrors every source the game reads or writes, and none of these sources is ours.
The endpoints each mirror serves are fixed in a contract package, the way atlas's queries are.

| Source | How the sim mirrors it |
|---|---|
| Kubernetes | A narrow API mirror serving only the verbs the providers use, so client-go talks to it unchanged: discovery, pods, events, logs, delete, the restart patch, scale, cordon, and exec over the websocket protocol. Exec answers from a scripted shell that says it's the sim. It also mirrors the admission policy's denials, so the refusals are tested. |
| ArgoCD | Its `Application` objects through the same mirror, plus its diff API. Sync, rollback and the sync-policy patch become sim events, the way `app.sync` already is. The ApplicationSet controller and self-heal are mirrored too, so the rule that makes a rollback hold is tested. |
| Patroni | Its REST endpoints. A switchover moves the sim's leader, and atlas sees it through the metrics mirror. |
| Postgres | A wire mirror for the read queries. Not a container: a real Postgres brings its own pids and clocks, and breaks determinism. |
| Loki, VictoriaMetrics, VictoriaLogs | Their query APIs, answered from the sim's events, deterministic per seed. |
| authentik | Its re-authentication (`prompt=login`, `auth_time`), beside the token endpoint it already mirrors. |
| Fleet | Runs for real (kind-local), as in R11. Exec on a kind-local fleet pod reaches it for real; everything else gets the scripted shell. |

## 5. Security
The console makes a signed-in admin's session powerful, so its power is bounded at the API server, not only in the
game's code.
- **Bounded writes.**
  - A **ValidatingAdmissionPolicy** bound to the game's ServiceAccount admits only these changes:
    - the `restartedAt` annotation on a pod template;
    - `/spec/unschedulable` on a node;
    - `/spec/syncPolicy/automated` and `/operation` on an Application.
  - Anything else the account sends is refused, and the game shows the refusal's reason.
  - **Exec** goes through per-namespace Roles. kube-system, cilium, longhorn-system, argocd, infisical, authentik
    and agent-fleet are excluded, because a shell in their privileged pods is root on a node or holds a platform
    secret.
  - **Applications** go through a Role in `argocd`, not a ClusterRole.
  - **The verbs:** `get` and `create` on `pods/exec` (the websocket upgrade checks both since 1.31), and `get` on
    `pods/log`. `describe` also reads ReplicaSets, Endpoints, PVCs and `events.k8s.io`.
  - **Never:** Secret objects, RBAC objects, CRD definitions.
- **Without the policy it would be cluster-admin.** Every app is in the `default` AppProject, and its controller is
  cluster-admin. So editing an Application's source deploys anything, and a free patch on a Deployment can mount any
  Secret. The policy is what stops that.
- **ArgoCD:** a local account `ukubi-game` with get, sync and diff on applications, its token in the game's Infisical
  project.
- **Patroni:**
  - The credential is `PATRONI_PASSWORD` for the user `postgres`. That user can make every unsafe call (including
    `PATCH /config` and failover), over plain HTTP on the LAN.
  - So the password moves to the game's own Infisical project and identity, like `fleet-github-app`'s. It is not
    shared with the infra-bootstrap project or platform-commons (ADR-0049).
  - The game's code limits it to §3's actions. That limit lives in code only, and the ADR says so.
- **Postgres:**
  - `dbuser_game`, a member of `pg_monitor` (reads) and `pg_signal_backend` (cancel and end non-superuser
    backends). Never a superuser.
  - `revokeconn` databases show limited stats.
  - The role is created the repo's way: a committed SCRAM verifier, the leader checked first, and a human Pigsty
    run.
  - pgweb is a write-capable console (no `--readonly`). Its gate is "merged, not live", and is verified before the
    game links to it.
- **Sessions:**
  - The session cookie gains an id and a server-side revocation set, so a sign-out really ends it.
  - A fresh sign-in sends authentik `prompt=login` and `max_age`, and checks `auth_time` in the ID token. Without
    that, authentik's SSO re-issues silently.
- **Audit:**
  - Every Run except a Link writes `action.run` to the game's event log and to stdout (Loki): `Sub` (never
    `--actor`), action, target, args, outcome and duration.
  - Each Kubernetes write also logs the API server's `Audit-Id`, which ties it to the API server's own audit log
    (ADR-0040 D2). The game's own log is never the only copy.
  - An exec records each command it runs; other streams record their start and end only.
  - The town centre gets a journal room listing every action run, newest first.
- **Network:** a `CiliumNetworkPolicy`, following ADR-0040's practice: plain NetworkPolicy can't express the API
  server or DNS under Cilium. Its egress:
  - `toEntities: [kube-apiserver]`, and DNS;
  - GitHub, Alertmanager, core, atlas and authentik;
  - Loki, and Pigsty's VictoriaMetrics and VictoriaLogs;
  - Patroni (port 8008), Postgres, and ArgoCD's API.
- **Drift.** Direct writes create states that atlas's git poll can't see. The marks on buildings show them
  (auto-sync off, cordoned), read from the cluster's own facts.

## 6. On the page
The wireframe is the target.
- **The command grid draws `Actions`.** Off actions are greyed with their reason.
- **The drawers, one per transport:**
  - answers: describe, YAML, the tables;
  - streams: logs (follow, pause, filter) and the shell;
  - links.
- **The confirmation dialog** shows `Touches`, the args and what follows, and asks for a fresh sign-in when the
  action needs one.
- **A mark on a building** shows a state the console left behind (auto-sync off, cordoned) at every zoom level, and
  disappears when the state does.
- **Phone:** the same actions in the bottom sheet, with the confirmation as a sheet.
- **Changes the wireframe needs:** pg02's logs drawer shows its logs from VictoriaLogs. The "No logs reach the game"
  empty state stays for parts whose logs reach no store.

## 7. Slices (each with its own plan and doubt cycle)
1. **C1, read-only, built to `15-architecture.md` inside the game's `internal/`:**
   - the interface, the registry, target resolution, and the audit;
   - the game's commands as providers;
   - the links provider and the Kubernetes reads;
   - the gate's upgrade check and session id;
   - the contract package and the Kubernetes mirror's read verbs;
   - a real leg replaying responses recorded from the live cluster.
   
   It needs the read ServiceAccount (ADR part 1).
2. **C2, Kubernetes writes:** restart, delete a pod, cordon, scale, and the exec shell. Also the confirmation tokens,
   the fresh sign-in, the admission policy (ADR part 2) and their mirrors.
3. **C3, ArgoCD:** history, the diff, sync, rollback, auto-sync back on. Also the ApplicationSet and `bootstrap`
   changes and the ArgoCD account (ADR part 3), and the mirror, controller and self-heal included.
4. **C4, the databases:** Patroni and Postgres (ADR part 4) and their mirrors. atlas gains Patroni's roles for the
   map's labels in the same slice, as a contract query.
5. **Later:** a SQL console, SSH to the guests, a fleet pod's files, Redis, Garage, and other providers. Each one is
   decided and designed when it starts.

Slice 4f (the one-screen client) carries C1's grid and drawers.

## 8. The infra-bootstrap ADR (draft)
It opens once infra-bootstrap #377 has merged. #377 is ADR-0068's PR, which waits for its Infisical key, and the
working branch carries it. The ADR has one part per slice, each landing with its slice.

1. **Read (C1):**
   - the ServiceAccount `ukubi-game` with get, list and watch on the resources §3 reads, Secrets excluded;
   - the game's Infisical project and identity;
   - the `CiliumNetworkPolicy`.
2. **Kubernetes writes (C2):**
   - the ValidatingAdmissionPolicy and its binding;
   - the write verbs;
   - the per-namespace exec Roles with their exclusions.
3. **ArgoCD (C3):**
   - the Role in `argocd`;
   - the `ukubi-game` local account and its token;
   - `ignoreApplicationDifferences` on `/spec/syncPolicy/automated`, narrowed so `syncOptions` and `retry` still
     propagate, in all three ApplicationSets (`apps`, `platform`, `platform-common-apps`);
   - `ignoreDifferences` with `RespectIgnoreDifferences=true` for the plain Applications that `bootstrap`
     self-heals;
   - `bootstrap` and `argocd` themselves stay Off.
4. **Databases (C4):**
   - `dbuser_game` (a committed SCRAM verifier, a human Pigsty run);
   - `PATRONI_PASSWORD` in the game's project;
   - egress to ports 8008 and 5432.

**Rejected:** cluster-admin, a kubeconfig in Infisical, a superuser Postgres role, reading Secret values, and SSH
keys (deferred).

**Consequence:** a stolen admin session can do what the policy admits. The 1 h session, the revocation set and the
fresh sign-in are its limits.

**ADR-0068 stays:** the game still reads atlas as `atlas-game`.

## What changes elsewhere
- **00-decisions:**
  - the R13 row;
  - plan/10's "atlas reads only" stays true, because the game acts, not atlas;
  - R11's goals and steps are deferred in the client; the engine stays.
- **ukubi-game:** the providers, the registry, the audit, the mirrors, the contract package, and the gate's upgrade
  check and session id.
- **infra-bootstrap:** the ADR above, in four parts, and its human steps (the Postgres role, the Infisical project).
- **agent-fleet:** nothing until "later". The browser VS Code (code-server) and sshd were on the e2e pod, which
  ADR-0048 §6 removed; exec into a session pod already reaches the same files.

## Open questions (owner)
1. SSH to the guests (Pigsty's members, Garage, NFS, the build runner): yes or no, and with which user.
2. The SQL console: read-only first (the default), or with write power.
3. Node drain: in v1, or later (the default: later, because a drain moves many pods at once).

**Answered (2026-10-27):**
- Secrets: values only through Infisical.
- The fresh sign-in: yes, after 10 minutes.
- ArgoCD's diff: yes, with its own account.
- The sim's Postgres: a wire mirror.
- Logs from the VMs: Pigsty v4.3, as vendored, enables VictoriaLogs on its infra node (`vlogs_enabled: true`, port
  9428, `/vlogs` on `pigsty.bnei.lan`) and a vector collector on every Postgres node, and `pigsty.yml` turns neither
  off. The live cluster confirms this before C1 relies on it.

## Verification
- **This draft:** two doubt cycles, plus the owner's answers.
- **Each slice:** its own checks, set in its plan. At least these, each refused:
  - every action called as a non-admin;
  - a write with no token, a replayed token, another session's token, or a `Touches` that changed;
  - a websocket upgrade from a foreign Origin;
  - a write the admission policy rejects, with its reason shown;
  - a strong action from a stale session.
- **Also:** integration against the sim, and contract suites over the recorded real leg, the mirror and the fake
  (`15-architecture.md`).
- **To confirm on the live cluster:** that `connect-src 'self'` covers `wss:` on iOS Safari.

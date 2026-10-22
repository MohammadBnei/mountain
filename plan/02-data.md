# 02-data — Data

## Endpoints (verified)
- Prometheus: `platform-prometheus-kube-p-prometheus.monitoring:9090`. Retention 7d.
- Alertmanager: `platform-prometheus-kube-p-alertmanager.monitoring:9093`.
- ArgoCD controller + server metrics are scraped (`argocd-application.yaml:174-186`) at the default 30 s.
- Traefik: lower the scrape to 15 s (`values/traefik/values.yaml:97`, PR in 07).

## Live probe gotchas (W0.5, 2026-10-14)
Probes ran with `KUBECONFIG=~/.kube/ukubi-admin.conf`.

| Signal | What the live cluster actually has | Rule |
|---|---|---|
| Traefik per-service label | `exported_service`, format `<ns>-<name>-<hash>@kubernetescrd`, **not** `service` | map `exported_service` → app server-side |
| Traefik `code` | includes `0` and `2` (not only HTTP codes) | 5xx = `code=~"5.."` only; ignore others |
| Traefik scrape | currently **1 m** | 07 PR to 15 s still needed |
| `argocd_app_sync_total` | **no `dest_namespace`** label | map by `name` (or `exported_namespace`) |
| `argocd_app_sync_total{phase}` | only `Succeeded` seen so far | `ok:false` path tested with the fixture |
| OOM / `…_waiting_reason` series | **absent until an event** | absent = 0 |
| `kubelet_volume_stats_*` | only **6/37 PVCs**: longhorn reported, local-path (31) not | local pools drawn unmeasured |
| storage classes | `longhorn` + `local-path` (no NFS) | see storage row |
| `node_network_receive_bytes_total` | no `node` label | join on `instance` |

## Counters and polls
- The relay polls Prometheus every 15 s.
- It **keeps the raw counter value per series and diffs it itself**, handling counter resets. It never uses `increase()` on short windows. Applies to `argocd_app_sync_total`, `kube_pod_container_status_restarts_total` and Traefik counters.
- Events land within one scrape plus one poll: **at most 45 s**.

| Signal | Source and rule |
|---|---|
| deploy | `argocd_app_sync_total{phase}` counter diff → `argo.sync{app, ok}` |
| health | `argocd_app_info{health_status, sync_status}` |
| app gone | absent from `argocd_app_info` **for 10 min while `up{job=~".*application-controller.*"} == 1`** |
| restarts / OOM / CrashLoop | restart counter diff. OOM only when restarts went up **and** `…_last_terminated_reason{reason="OOMKilled"} == 1`. CrashLoop from `…_waiting_reason{reason="CrashLoopBackOff"}` going 0 → 1 |
| replicas | `kube_deployment_status_replicas`, bucketed |
| **pods (R7)** | `max by (namespace, pod, node) (kube_pod_info{node!=""}) * on (namespace, pod) group_left() max by (namespace, pod) (kube_pod_status_phase{phase="Running"} == 1) unless on (namespace, pod) kube_pod_deletion_timestamp` (promtool-tested, `relay/testdata/pods.test.yaml`). **Optional**: a failure is logged once and never marks the poll stale. Feeds placement only |
| node Ready / capacity | `kube_node_status_condition`, `kube_node_status_allocatable` |
| PVE quorum | `probe_success{target=~"pve-.*"}` (verified label). The first answer is a baseline (`hello.state.quorum`, R7.1); after that, a change is `ev quorum` |
| req/s, 5xx, bytes out | `traefik_service_requests_total{code}`, `traefik_service_responses_bytes_total`. Service label mapped server-side |
| storage | One pool per PVC (ns→app map; non-allowlisted PVCs merge into one `other` pool per class). Size = `kube_persistentvolumeclaim_resource_requests_storage_bytes`, bucketed. **longhorn** level = `kubelet_volume_stats_used_bytes` ⋈ `kube_persistentvolumeclaim_info{storageclass}` (real). **local-path** level = unmeasured (no kubelet stats for hostPath; optional W3 textfile `du`, 07). No NFS (00). **Garage and Pigsty: unmeasured** (Pigsty metrics live in its own VictoriaMetrics) |
| cpu, mem, net rx | node-exporter. `rx` is in the metrics frame (no object draws it yet) |
| river | Σ Traefik bytes out over allowlisted apps (metrics frame; a later wave draws it on the bridges) |
| alerts | `/api/v2/alerts?active=true&silenced=false&inhibited=false`, dropping `Watchdog` and `InfoInhibitor`. Uses Alertmanager `fingerprint` and `startsAt` (gives `durS`). **R7:** optional `host` (the `node` label through the node → host map) and `app` (the `namespace` label through the ns → app map), so a beacon has a place |
| builds | GitHub Actions runs (`src_github`), mapped by **workflow repo + time window** (2 h). Not SHA |
| gitops commits, ADR merges | GitHub commits on infra-bootstrap main touching `gitops/` and `docs/adr/`. A title is public **only if it is on the relay's title allowlist**; otherwise date only. No town object draws them yet |

## Placement (R7, `relay/plots.go`, run from each prom poll)
- Every allowlisted app and service with Running pods gets one plot: `world:plot {app, host, q, r, role, ruinAt?, gone?}`.
- **Host:**
  - the majority host of its Running pods wins;
  - a tie keeps the current host, then goes to sorted host order (DaemonSets tie);
  - a move needs the new majority for **≥ 5 min**;
  - a building **never moves off a NotReady host**: it stays there, dark;
  - an empty or failed `pods` answer, or an app scaled to 0, **keeps every plot**.
- **Hex:**
  - `town.Slot(app, host, …)` picks a free grass hex in rings 2, 3, 1 (rings 1, 2, 3 for services and platform apps, which sit nearer the square);
  - the start inside each ring is the FNV of the app, and apps are placed in sorted order, so the choice is
    deterministic;
  - hexes with a visitor plant are skipped. When the island is full, the plant on the chosen hex gives way
    (`world:mark gone`).
- `app.gone` sets `ruinAt`. A ruin older than 24 h frees its plot (`gone`).
  - It is not rebuilt from its pods until the app is back in ArgoCD (R7.1).
  - A pending move restarts whenever an answer lacks the app, or there is no answer.
  - At restore, a plot whose app left `config.yaml` is dropped.
- Plot placement and planting share one lock (`placeMu`), so a plant never lands on a hex a plot is taking.
  The lock order is visits → placeMu → h.mu → a.mu.
- The cap is 64 plots. Pod names, node names, IPs and pod counts never leave the relay: only the host bucket does.

**Static relay config** (`relay/config.yaml`):
- `apps`: the **public allowlist**, each with `group` (`user` | `platform` | `service`), `display`, `building` (kind).
  Services use role keys (gate, guardhouse, craneyard, watchtower, warehouse, vault, granary, depot), never product
  names (ADR-0056 amendment).
- `namespace(+deployment) → app` (e.g. ns `vos` → `vos-monolith`, ns `traefik` → `gate`; agent-fleet previews
  excluded).
- `node → host (h1..h3)`.
- `alert label → layer`.
- `repo → app`.
- The title allowlist.

Rot guard:
- `mountain_unmapped_total{kind=node|app|repo|ns}` plus a rule.
- A CI check diffs the app allowlist and the ns map against `infra-bootstrap/gitops/apps/registry.yaml`.

## Event enum (FROZEN)
`Ev{id (ULID), ts, kind, …}`:

| Kind | Fields |
|---|---|
| `argo.sync` | `app`, `ok` |
| `argo.health` | `app`, `status` |
| `pod.churn` | `app`, `restarts`, `oom`, `crashloop`, over the **15 s** window |
| `node.ready` | `host`, `ready` |
| `quorum` | `up[]` |
| `alert.fire` | `fp`, `sev`, `layer`, `startsAt` (unix ms, Alertmanager's: a re-fire after a relay restart keeps it), `host?`, `app?` (R7) |
| `alert.resolve` | `fp`, `sev`, `layer`, `durS`, `host?`, `app?` (R7) |
| `build` | `app`, `state` |
| `gitops.commit` | `n` |
| `adr.merge` | `day`, `title?` |
| `replicas` | `app`, `bucket` |
| `app.gone` | `app` |

`fp` = a salted hash. **The salt is never rotated**: restored alerts and history link to it.

## Metrics frame (every 15 s; equals `world.metrics`; FROZEN v2)
```
{ apps:{[app]:{rpsB, errB, spike, bytesB}}, storage:{pools:[{id, app|'other', class:'replicated'|'local', host?, capB, usedB|null}]},
  cpu, mem, buildsInFlight, rx, river, hosts:{h1..h3:{alloc, ready: bool|null, load}}, stale:{[source]:ts}, visitors }
```
- `spike` means the relay sees more than 3× the 30-minute baseline.
- v2 dropped `heat`, `wind` and `snowline` (the mountain's).
- v3 (R7.1) changed three things:
  - pool `class` is `replicated` for the longhorn storage class, so no product name;
  - the always-null `garage`/`pigsty` keys are gone;
  - `ready: null` means not measured yet, which the page draws as up.
- An empty `ready` or `quorum` answer is no answer: every host keeps its last state, with no `node.ready` and no
  `quorum` event.
- The walkers read `rpsB`, `errB` and `spike` (03).

## Wire encoding
- Schema: `www/dev/schema/{hello,ev,metrics,world,replay,scrub,mark}.json` (JSON Schema v2, FROZEN). The relay and
  the www tests both validate frames against it.
- `hello.town` carries the geometry as one tile letter per hex in canonical order (r ascending, then q), not as a
  raster (06).

## Public allowlist (built by allowlist, never by deletion; also used for replays)
- Apps and services from the allowlist only; others are dropped (or merged into an "other" pool). Hosts appear as
  `h1..h3`. Group = `user | platform | service`.
- **Bucketed values only.** The plot's host is a bucket too.
- Alerts: hashed `fp`, severity, layer.
- No versions. Story versions are static i18n copy.
- Visitors: an opt-in handle passed through the moderation filter (08), otherwise "a visitor".

## Persistence (`relay/store.go`; Pigsty side live)
**Already done (2026-10-14):** role `dbuser_mountain` + database **`mountaindb`** on cluster `pg-proxmox`, in hq's shape: committed SCRAM verifier, `pgbouncer: false`, `revokeconn`, **connlimit 10**. The password is `DBUSER_MOUNTAIN_PASSWORD` in the infra-bootstrap Infisical project.
- DSN: `postgres://dbuser_mountain:<pw>@postgres.bnei.lan:5432/mountaindb`. It reaches the pod through an infra-bootstrap `gitops/bootstrap/mountain-secret.yaml` modelled on `hq-secret.yaml` (W2 PR).
- pgx: set `MaxConns` ≤ 8 explicitly. The default `max(4, NumCPU)` can exceed connlimit 10.
- Direct connection, so prepared statements and advisory-lock migrations are fine.

- `DATABASE_URL` from the Secret `mountain-config` (helm `envFrom`, **optional**): unset = memory-only (logged once; the world is lost on restart). A DB unusable at boot also runs memory-only and never writes, so it cannot overwrite a stored world; restart once it is back.
- Migrations: embedded `relay/migrations/*.sql`, applied in order under `pg_advisory_lock`, recorded in `schema_migrations`.
- **Writes never block the hub:** every write is a closure on a buffered queue (1024) drained by one writer goroutine, so the DB sees frames and snapshots in broadcast order. A full queue drops the write (log + `mountain_db_dropped_total{reason=full}`); a failed write counts `reason=error`.
- **Health:** `mountain_source_last_success{source="db"}` on every good write and a 30 s ping → `MountainSourceStale` covers a dead DB. Memory-only seeds it once and lets it age (the alarm for a missing Secret). The page is **not** marked stale for the DB: the live view does not depend on it.

**Tables (migration `001_init.sql`):** one sequence, `frame_seq`, orders every frame and snapshot.
- `events(seq, id, ts, day, kind, public jsonb)`: every broadcast `ev` frame as sent (sanitised). `detail` (002,
  kept 30 days) carries the detail-mode payload (08).
- `world_deltas(seq, ts, day, kind, frame jsonb)`: every broadcast `world` frame as sent (`plot`, `mark`).
- `snapshots(day PK, seq, ts, seed, land bytea, state jsonb, metrics jsonb)`:
  - upserted every 10 min, at UTC rollover and on SIGTERM;
  - **R7:** `seed` holds the epoch (`townEpoch = 2`) and `land` is an empty bytea;
  - `state` = `hello.state` (`alerts`, `health`, `replicas`, `builds`, `marks`, `plots`, `quorum`).
- `metrics_5m(ts, day, frame jsonb)`: the metrics frame every 5 min (scrub).
- `marks` (migration `003_marks.sql`): `id, sub, handle, toy, ts, day, pose jsonb, weight, deleted_at`.
  - It is the private record of each accepted mark.
  - The public side is the `world:mark` frame.
  - The per-account rate limits are seeded from it at boot.
- **R7, migration `004_town.sql`:**
  - It moves every row of `events`, `world_deltas`, `snapshots` and `metrics_5m` into
    `mountain_archive_{events,world_deltas,snapshots,metrics_5m}`.
  - It soft-deletes the old-toy marks; their ids are kept in `mountain_archive_marks_hidden`.
  - It records `town_epoch(started)`.
  - It is reversed by hand with `relay/rollback/004_town_down.sql`, for a revert to the mountain image.

**Boot:**
1. Load the latest snapshot of the town epoch. A pre-town row is ignored.
2. Re-apply every frame after it through `State.apply`, the same rules as live and world.js.
3. Restore `metrics`, with the stale map cleared: sources re-mark what is down.

Edge cases:
- No snapshot yet: write one at once.
- Restored alerts seed `src_alerts`:
  - still active → no second `alert.fire` (`startsAt` is kept);
  - gone from Alertmanager → resolved on the first good poll.
- Restored marks of a removed toy are filtered out.

**`hello`** = `snapshot{day, epoch}` + the `world` and `ev` frames since it (`deltas`) + `state` + the current metrics + `town`. `snapshot.day` is the UTC day of the latest snapshot and advances at rollover.


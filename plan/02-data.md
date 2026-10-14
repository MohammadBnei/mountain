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
| node Ready / capacity | `kube_node_status_condition`, `kube_node_status_allocatable` |
| PVE quorum | `probe_success{target=~"pve-.*"}` (verified label) |
| req/s, 5xx, bytes out | `traefik_service_requests_total{code}`, `traefik_service_responses_bytes_total`. Service label mapped server-side |
| storage | One pool per PVC (ns→app map; non-allowlisted PVCs merge into one `other` pool per class). Size = `kube_persistentvolumeclaim_resource_requests_storage_bytes`, bucketed. **longhorn** level = `kubelet_volume_stats_used_bytes` ⋈ `kube_persistentvolumeclaim_info{storageclass}` (real). **local-path** level = unmeasured (no kubelet stats for hostPath; optional W3 textfile `du`, 07). No NFS (00). **Garage and Pigsty: unmeasured** (Pigsty metrics live in its own VictoriaMetrics) |
| cpu, mem, net rx | node-exporter. `net rx` feeds wind + the cloud floor. Copy says "inbound traffic"; it includes inter-node traffic |
| river | Σ Traefik bytes out over allowlisted apps |
| alerts | `/api/v2/alerts?active=true&silenced=false&inhibited=false`, dropping `Watchdog` and `InfoInhibitor`. Uses Alertmanager `fingerprint` and `startsAt` (gives `durS`). **A vein needs severity ≥ warning and a duration ≥ 10 min** |
| builds | GitHub Actions runs (W3), mapped by **workflow repo + time window**, then the next `argo.sync` of the mapped app. Not SHA |
| gitops commits, ADR merges | GitHub commits on infra-bootstrap main touching `gitops/` and `docs/adr/`. A title is public **only if it is on the relay's title allowlist**; otherwise date only |
| vein `ref` flag | daily GitHub contents scan of infra-bootstrap `docs/bootstrap-test-notes.md` and `.claude/skills/**` for the alert name. **No hq data** |

**Static relay config** (`relay/config.yaml`):
- `namespace(+deployment) → app` (e.g. ns `vos` → `vos-monolith`; agent-fleet previews excluded)
- `node → host (h1..h3)`
- `alert label → layer`
- `repo → app`
- **public app allowlist**
- title allowlist
- display names

Rot guard:
- `mountain_unmapped_total{kind=node|app|repo|ns}` plus a rule.
- A CI check diffs the app allowlist and the ns map against `infra-bootstrap/gitops/apps/registry.yaml`.

## Event enum (frozen W2)
`Ev{id (ULID), ts, kind, …}`:

| Kind | Fields |
|---|---|
| `argo.sync` | `app`, `ok` |
| `argo.health` | `app`, `status` |
| `pod.churn` | `app`, `restarts`, `oom`, `crashloop`, over the **15 s** window |
| `node.ready` | `host`, `ready` |
| `quorum` | `up[]` |
| `alert.fire` | `fp`, `sev`, `layer` |
| `alert.resolve` | `fp`, `sev`, `layer`, `durS` |
| `build` | `app`, `state` |
| `gitops.commit` | `n` |
| `adr.merge` | `day`, `title?` |
| `replicas` | `app`, `bucket` |
| `app.gone` | `app` |

`fp` = salted hash. **The salt is never rotated** (veins link to it).

## Metrics frame (every 15 s; equals `world.metrics`)
```
{ apps:{[app]:{rpsB, errB, spike, bytesB}}, storage:{pools:[{id, app|'other', class:'longhorn'|'local', host?, capB, usedB|null}], garage:null, pigsty:null},
  cpu, mem, heat, buildsInFlight, wind:{x,z}, snowline, rx, river,
  hosts:{h1..h3:{alloc, ready, load}}, stale:{[source]:ts} }
```
- `spike` = the relay sees more than 3× the 30-minute baseline.
- `heat` = f(builds in flight, syncs in flight). It is the same value Go uses for deposit size.

## Wire encoding (R4)
- Schema: `www/dev/schema/{hello,ev,metrics,world}.json` (JSON Schema). The relay and the www tests both validate frames against it.
- int16 grids (`h.base`, `h.land`, deposit `cells`): **base64 of little-endian int16, row-major, z outer, x inner**.
- Cell 0 sits at world **(−100, −100)**. Cell size = 200 / N.
- Heights are millimetres: world units = **mm / 1000**.

## Public allowlist (built by allowlist, never by deletion; also used for `/api/topology` and replays)
- Apps from the allowlist only; others go into an "other" aggregate with no site. Hosts appear as `h1..h3`. Group = `platform|user`.
- **Bucketed values only** (the 85 % seep threshold sits on a bucket edge; it applies per measured pool).
- Alerts: hashed `fp`, severity, layer.
- No versions. Story versions are static i18n copy.
- Visitors: an opt-in handle passed through the moderation filter (08), otherwise "a visitor".

## Persistence (W2; Pigsty side already live)
**Already done (2026-10-14):** role `dbuser_mountain` + database **`mountaindb`** on cluster `pg-proxmox`, in hq's shape: committed SCRAM verifier, `pgbouncer: false`, `revokeconn`, **connlimit 10**. The password is `DBUSER_MOUNTAIN_PASSWORD` in the infra-bootstrap Infisical project.
- DSN: `postgres://dbuser_mountain:<pw>@postgres.bnei.lan:5432/mountaindb`. It reaches the pod through an infra-bootstrap `gitops/bootstrap/mountain-secret.yaml` modelled on `hq-secret.yaml` (W2 PR).
- pgx: set `MaxConns` ≤ 8 explicitly. The default `max(4, NumCPU)` can exceed connlimit 10.
- Direct connection, so prepared statements and advisory-lock migrations are fine.

**Tables:**
- `events` (`public`, `detail`; detail kept 30 days)
- `world_deltas`
- `snapshots(day, base bytea, land bytea, veins, glacier, ash, clouds)`
- `metrics_5m`
- `marks`
- `limits`

**`hello`** = latest snapshot + the deltas since it + `state` + the current metrics.


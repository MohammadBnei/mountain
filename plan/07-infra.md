# 07-infra — Infra (infra-bootstrap)

| Wave | Step | Owner | Status |
|---|---|---|---|
| W0 | **ADR-A**: public telemetry allowlist, PromQL only, no RBAC, unmeasured Garage/Pigsty, river = public traffic, CF-IP trust note | agent PR, M BN merges | ✅ ADR-0056, #298 merged |
| W0 | Infisical project `mountain` (cookie key; later DB creds, OIDC, PAT, Turnstile) | M BN, via local CLI with the agent | ✅ `mountain-8c-cz` (`8746c02c-24a0-4b4c-950f-1ee9b36c2625`); DB creds live in the infra-bootstrap project instead |
| W0 | Add the mountain repo to the `ACCESS_TOKEN` PAT, add it to `build_runner_repos`, run `build-runner-configure.yml` | M BN | ✅ PAT already covered it; runner `ukubi-build-mountain` online; CI identity claim added |
| W0 | `/add-app` `mountain.bnei.dev`, **pointing at this repo's `helm/` (R4)**, replicas 1, `Recreate`, baseline middleware on, `service.extraPorts` metrics, **ServiceMonitor in `extraManifests`** | agent PR (W1a lane D), M BN merges | ✅ infra-bootstrap #300 |
| W1a | Traefik serviceMonitor interval 15 s | agent PR (W1a lane D) | ✅ #300 |
| W1b | PrometheusRule `MountainSourceStale`/`MountainTokenExpiring`/`MountainUnmapped` (severity notice, 06) in the mountain app's `extraManifests` | agent PR (W1b lane E), M BN merges | ✅ |
| W1b | Alert fp salt: random `MOUNTAIN_FP_SALT` (32+ bytes, **never rotated**, 02) in Infisical `mountain-8c-cz`, synced to the Secret `mountain-infisical` (grant the shared machine identity, `infisical.enabled` in `helm/values.yaml`). The chart loads it via `envFrom` (common-app-chart `infisical`); without the key, alerts are dropped | M BN set the key + access; wired in PR #1 | ✅ |
| W2 | Pigsty `dbuser_mountain` + db `mountain` (hq shape) in `pigsty/pigsty.yml` | agent PR. **M BN runs** `pgsql-user.yml` + `pgsql-db.yml` | ✅ **done early** (2026-10-14): `dbuser_mountain` / **`mountaindb`** (not `mountain`), connlimit 10, verified live |
| W3 | Fine-grained read-only GitHub PAT → Infisical | M BN | ⏳ |
| later (optional) | node-exporter textfile collector: per-PV-dir `du` on each node's local-path root → local-path pools become measured (the storage yard) | agent PR, M BN merges | ⏳ optional |
| W4 | **ADR-B**: visitor enrollment, shared SMTP2GO accepted risk + quota watch, Turnstile, moderation, kill switch | agent PR, M BN merges | ⏳ |
| R7 | **ADR-0056 amendment**: platform services public under role names; the plot's host bucket public; alerts with `host`/`app` (draft: `plan/adr-0056-amendment.md`) | M BN lands it in infra-bootstrap | ⏳ |
| R7 | Remove `STT_TOKEN_MOUNTAIN` from Infisical `mountain-8c-cz` (the STT card is gone) | M BN | ⏳ |
| R7.1 (optional) | Chrome on the build runner, so `gate.mjs page`/`selftest` run in CI. Either add `chromium` to `build-runner-configure.yml`, or run the gate in a podman container on the runner (04) | agent PR, M BN merges | ⏳ ask |

- No ServiceAccount, RBAC, chart change or NetworkPolicy.
- No Garage scrape job unless M BN asks: the unexposed port 3903 is deliberate.

- Commands that were run, for reference: `build-runner-configure.yml` from the Mac; `pgsql-user.yml -l pg-proxmox -e username=dbuser_mountain` and `pgsql-db.yml -l pg-proxmox -e dbname=mountaindb` from the k9s hub (`docs/runbook-k9s-ops-hub.md`), after checking that the Patroni leader matches `pg_role`.

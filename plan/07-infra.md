# 07-infra — Infra (infra-bootstrap)

| Wave | Step | Owner | Status |
|---|---|---|---|
| W0 | **ADR-A**: public telemetry allowlist, PromQL only, no RBAC, unmeasured Garage/Pigsty, river = public traffic, CF-IP trust note | agent PR, M BN merges | ✅ ADR-0056, #298 merged |
| W0 | Infisical project `mountain` (cookie key; later DB creds, OIDC, PAT, Turnstile) | M BN, via local CLI with the agent | ✅ `mountain-8c-cz` (`8746c02c-24a0-4b4c-950f-1ee9b36c2625`); DB creds live in the infra-bootstrap project instead |
| W0 | Add the mountain repo to the `ACCESS_TOKEN` PAT, add it to `build_runner_repos`, run `build-runner-configure.yml` | M BN | ✅ PAT already covered it; runner `ukubi-build-mountain` online; CI identity claim added |
| W0 | `/add-app` `mountain.bnei.dev`, replicas 1, baseline middleware on, `service.extraPorts` metrics | agent PR, M BN merges | ⏳ deferred to W1a (needs the first image) |
| W1 | Traefik serviceMonitor interval 15 s | agent PR | ⏳ |
| W2 | Pigsty `dbuser_mountain` + db `mountain` (hq shape) in `pigsty/pigsty.yml` | agent PR. **M BN runs** `pgsql-user.yml` + `pgsql-db.yml` | ✅ **done early** (2026-10-14): `dbuser_mountain` / **`mountaindb`** (not `mountain`), connlimit 10, verified live |
| W3 | Fine-grained read-only GitHub PAT → Infisical | M BN | ⏳ |
| W4 | **ADR-B**: visitor enrollment, shared SMTP2GO accepted risk + quota watch, Turnstile, moderation, kill switch | agent PR, M BN merges | ⏳ |

- No ServiceAccount, RBAC, chart change or NetworkPolicy.
- No Garage scrape job unless M BN asks: the unexposed port 3903 is deliberate.

- Commands that were run, for reference: `build-runner-configure.yml` from the Mac; `pgsql-user.yml -l pg-proxmox -e username=dbuser_mountain` and `pgsql-db.yml -l pg-proxmox -e dbname=mountaindb` from the k9s hub (`docs/runbook-k9s-ops-hub.md`), after checking that the Patroni leader matches `pg_role`.

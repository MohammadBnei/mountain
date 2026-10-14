# 07-infra — Infra (infra-bootstrap)

| Wave | Step | Owner |
|---|---|---|
| W0 | **ADR-A**: public telemetry allowlist, PromQL only, no RBAC, unmeasured Garage/Pigsty, river = public traffic, CF-IP trust note | agent PR, M BN merges |
| W0 | Infisical project `mountain` (cookie key; later DB creds, OIDC, PAT, Turnstile) | M BN, via local CLI with the agent |
| W0 | Add the mountain repo to the `ACCESS_TOKEN` PAT, add it to `build_runner_repos`, run `build-runner-configure.yml` | M BN |
| W0 | `/add-app` `mountain.bnei.dev`, replicas 1, baseline middleware on, `service.extraPorts` metrics | agent PR, M BN merges |
| W1 | Traefik serviceMonitor interval 15 s | agent PR |
| W2 | Pigsty `dbuser_mountain` + db `mountain` (hq shape) in `pigsty/pigsty.yml` | agent PR. **M BN runs** `pgsql-user.yml` + `pgsql-db.yml` |
| W3 | Fine-grained read-only GitHub PAT → Infisical | M BN |
| W4 | **ADR-B**: visitor enrollment, shared SMTP2GO accepted risk + quota watch, Turnstile, moderation, kill switch | agent PR, M BN merges |

- No ServiceAccount, RBAC, chart change or NetworkPolicy.
- No Garage scrape job unless M BN asks: the unexposed port 3903 is deliberate.


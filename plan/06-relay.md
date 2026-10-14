# 06-relay — Relay

**Process:** 1 replica, `Recreate`. Go binary embeds `www/`. Ownership per [01].

**Ports:**
- `:8080` public: `/`, `/api/topology`, `/ws`, `/auth/*`, `/healthz`.
- **`:9090` metrics, a separate port via `service.extraPorts`**, never routed by the IngressRoute.

**worldcore.go** is the only implementation of:
- `h.base` generation (seeded, int16 mm)
- site placement
- deposit rasterisation (int16 cells, seeded by FNV-1a of the event id, size = f(heat))
- vein capsules
- glacier state
- ash lines
- snowline
- wind
- toy-cloud drift, merge and rain
- footprint weight

**WS protocol:**
- Server → client:
  - `hello{snapshot, deltas, state, metrics}`
  - `ev`
  - `metrics`
  - `world{deposit|vein|glacier|ash|site|mark|cloud|rain}`
- Client → server:
  - `subscribe{filter}`
  - `scrub{day}` → `replay{snapshot, frames}`
  - `mark{kind, pose}` (session in `mountain-visitors` or `platform-admins`)
- `action`: v2.

**Keepalive:** a server ping every 30 s. Clients reconnect with jittered backoff and resync through `hello`. Logging in requires a reconnect, because the cookie is read at upgrade.

**Security:**
- `Origin` must equal `https://mountain.bnei.dev` at upgrade.
- Cookie: `Secure; HttpOnly; SameSite=Lax`, signed, **TTL ≤ 1 h** with a groups re-check on refresh.
- OIDC `state` is random. The return view is in a short-lived signed cookie.
- **`CF-Connecting-IP` is trusted only when the TCP peer is in a Cloudflare range.** Otherwise the peer IP is used (LAN/pod CIDR spoofing is accepted and noted).

Limits:

| Limit | Value |
|---|---|
| WS connections per IP | 4 |
| Client message rate | 10/s |
| Client frame size | 4 KB |
| Scrub | 1 per 2 s, per connection |
| Marks per account | 1/min, 20/day |
| Active marks, global cap | 2000 |
| Day replays | cached in memory |

**Kill switch:** `VISITORS_ENABLED=false` hides the surface layer and rejects marks, with no deploy (Infisical-synced env + restart-free reload of the flag).

**Self-observability:**
- `mountain_source_last_success{source}`
- `mountain_unmapped_total`
- `mountain_github_token_days_left` (from the `github-authentication-token-expiration` header)
- `mountain_ws_connections`

The ServiceMonitor and PrometheusRule live in **the mountain app's `extraManifests`**, since common-app-chart has no template for them.

Rules (all `severity: info` → Discord; **not warning, which would wake thot**):
- `MountainSourceStale`
- `MountainTokenExpiring` (< 14 d)
- `MountainUnmapped`


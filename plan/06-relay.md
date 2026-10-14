# 06-relay — Relay

**Process:** 1 replica, `Recreate`. Go binary embeds `www/` via the root package `mountain` (`embed.go`, `//go:embed www`), imported by relay. Go 1.27 toolchain. Ownership per [01].

**Ports:**
- `:8080` public: `/`, `/api/topology`, `/ws`, `/auth/*`, `/healthz`.
- **`:9090` metrics, a separate port via `service.extraPorts`**, never routed by the IngressRoute.

**worldcore.go** is the only implementation of:
- `h.base` generation (seeded, int16 (HEIGHT_Q steps)): broad massif + soft ridged noise + terraces (step ≈ 7 m, 65 % blend). **Spiral walkway** (seeded: start azimuth, turns ≈ 2, foot height, top = 0.6 × summit so it stays below the snow cap, bench width ≈ 5 m): per azimuth the radius follows the altitude contour, smoothed; altitude is linear in arc length (constant grade, ≈ 6 % on the board); where smoothing would cut > 4 m into a ridge it snaps back to the contour, so the ledge always wraps around ridges and never passes through. Carved as a flat bench (centre ± 2.6 m) blended to the slope over 1.4 m: a clean outer lip and an inner cut wall. Emits the **`bench` mask** (u8, 128²). **Caves**: 3–5 seeded `caves[{x,z,y,yaw,seed}]` on the walkway where the inner cut wall is ≥ 7 m tall (Go keeps that wall steep; no niche carve: at 1.56 m cells it only makes a slot). **Edge**: beyond r = 78 m the height drops 34 m (to r = 100) and 30 m more to the corners, so the massif falls into the valley with no flat apron. All sent in `hello.snapshot`. No tunnels
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
  - `scrub{day}` → `replay{day, snapshot, state, frames, metrics}` to that client only (W2, `relay/scrub.go`): `snapshot` + `state` = the world at the start of the UTC day (the latest snapshot before it + the frames between, or the bare base); `frames` = the day's `ev` + `world` frames in broadcast order; `metrics` = its `metrics_5m` frames. Errors: `replay{day?, error: bad-day|rate|unavailable}` (bad format, future, > 30 days; 1 per 2 s; no DB or a DB error), so a paused client always hears back. Client side: `world.scrub(day)` holds live frames off (dropped) until `world.release()`, which resyncs through a fresh hello; the replay frames play through the bus in ≤ 20 s
  - `mark{mark, pose}` (session in `mountain-visitors` or `platform-admins`)
- `action`: v2.

**Dev and fixture modes (R4):**
- `relay --dev` allows an `http://127.0.0.1:*` Origin and adds `connect-src ws://127.0.0.1:*` to the CSP. A Go test proves the dev flags are off by default and that production still rejects foreign origins with 403.
- `relay --source=fixture:<file>` serves a scripted Prometheus/Alertmanager timeline through an in-process fake HTTP server. The fixture is seeded with hostile labels (04).
- `relay dump-hello` prints a `hello` frame from real `worldcore` (W1a seam: minimal `worldcore.Base(seed)`). It is the only source of the hello fixture.

**Keepalive:** a server ping every 30 s. Clients reconnect with jittered backoff and resync through `hello`. Logging in requires a reconnect, because the cookie is read at upgrade.

**Security:**
- `Origin` must equal `https://mountain.bnei.dev` at upgrade.
- **coder/websocket v1.8.15:** its default origin check ignores the scheme and passes an empty `Origin`. So the relay runs its own `originOK` check (exact scheme + host, empty = reject; dev flag adds `http://127.0.0.1:*`) and then calls `Accept` with `InsecureSkipVerify: true`. `Ping` needs a concurrent reader goroutine, or it never returns.
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
| Day replays | cached in memory (last 7 built): a day ended > 1 min ago until evicted, today 10 s; at most 2 builds at once relay-wide (else `rate`) |

**Kill switch:** `VISITORS_ENABLED=false` hides the surface layer and rejects marks, with no deploy (Infisical-synced env + restart-free reload of the flag).

**Self-observability:**
- `mountain_source_last_success{source}`
- `mountain_unmapped_total`
- `mountain_github_token_days_left` (from the `github-authentication-token-expiration` header)
- `mountain_ws_connections`
- `mountain_db_dropped_total{reason=full|error}` (W2)
- `mountain_scrub_dropped_total`: scrub replies lost to a full send buffer (the connection is dropped) (W2)
- `source="db"` in `mountain_source_last_success` (W2: Postgres writes + 30 s ping; memory-only ages)

The ServiceMonitor and PrometheusRule live in **the mountain app's `extraManifests`**, since common-app-chart has no template for them.

Rules (all `severity: notice` → Discord; **not warning, which would wake thot**). **Not `info` either** (decided at the W1b seam): the live Alertmanager keeps kube-prometheus-stack's `InfoInhibitor` inhibit rule (`target severity="info", equal [namespace]`), which swallows every info alert unless a warning or critical fires in the same namespace. `notice` matches neither the thot route (`severity=~"critical|warning"`) nor that inhibit rule, so it falls through to the default `discord` route. No infra-bootstrap change; the relay's alert source drops these (unknown `sev`), which is fine: stale already shows on the page.
- `MountainSourceStale`
- `MountainTokenExpiring` (< 14 d)
- `MountainUnmapped`


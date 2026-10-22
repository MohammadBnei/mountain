# 06-relay — Relay

**Process:** 1 replica, `Recreate`. The Go binary embeds `www/` through the root package `mountain` (`embed.go`,
`//go:embed www`), imported by relay. Go 1.27 toolchain.

**Ports:**
- `:8080` public: `/`, `/v/<build>/…`, `/ws`, `/auth/*`, `/healthz`. `/api/topology` is gone (R7): plots ride the
  hello.
- **`:9090` metrics, a separate port via `service.extraPorts`**, never routed by the IngressRoute.

**Static files:**
- Cloudflare overrides Cache-Control, so every asset `index.html` references lives under `/v/<build>/`, immutable:
  a new deploy means a new URL.
- `/` is `no-cache`, and the CSP hashes the importmap actually served.
- Another build's `/v/<id>/` is a `no-store` 404. `main.js` answers it with one reload onto the current index.

**`relay/town` is the only implementation of the town geometry (R7):**
- **Hexes** are pointy-top axial, as in pi-lab `kit.hex`: x = 2(q + r/2), z = √3·r.
- **Islands** have radius 4, so 61 hexes each. The centres are h1 (−4, −4), h2 (8, −4) and h3 (−4, 8).
- **Tiles**, per local hex:
  - `s`: the square, at the centre;
  - `r`: roads, the spokes from the square to each bridge head;
  - `b`: the bridge heads, which face h1–h2 (1, 0), h1–h3 (0, 1) and h2–h3 (−1, 1);
  - `g`: grass, rings 1–3, the building ring;
  - `m`: meadow, ring 4, for marks only.
- **`town.Get()`** is sent as `hello.town {radius, islands[{host, q, r, tiles}], bridges[{a, b, aq, ar, bq, br}]}`.
  `tiles` is one letter per hex in canonical order: r ascending, then q.
- **`town.Slot`** places plots (02 Placement) and **`town.Plantable`** says where a plant may go.
- `town_test.go` pins the vectors that `core.test.mjs` checks on the JS side.

**`plots.go`** runs from each prom poll, after the nodes section. It turns the `pods` answer into `world:plot`
frames (02).

**`toy_plant.go` + `sun.go`:**
- `sun.go` ports `js/sun.js`'s solar elevation for Paris; `sun_test.go` pins the JS vectors. The clock is `sunNow`,
  which a fixture may pin. The rate-limit clock is never pinned.
- The client pose is `{h, q, r}`, all integers.
- A pose is rejected as:
  - `bad`: a malformed pose;
  - `reach`: off the island;
  - `road`: on the square, a road or a bridge head;
  - `plot`: on a building's hex;
  - `taken`: on another active mark.
- The public pose is `{h, q, r, item}`. `item` is 0 (a tree) above +6°, 1 (a flag) from −6° to +6°, and 2 (a lantern)
  below −6°.

**src_github (`relay/src_github.go`):**
- **Token:** env `MOUNTAIN_GITHUB_TOKEN`, a fine-grained read-only PAT (Actions + Contents + Metadata on the
  `config.yaml` repos and infra-bootstrap; Infisical `mountain-8c-cz`).
- **Every minute:**
  - Actions runs per mapped repo → `ev:build`. In flight means a run created in the last 2 h, `queued` or `running`;
    after that, the latest completed run's conclusion is `success`, else `failure`.
  - `metrics.buildsInFlight`.
  - infra-bootstrap `main` commits under `gitops/` → `ev:gitops.commit{n}`, and under `docs/adr/` →
    `ev:adr.merge{day, title}`. The title is kept only if it is on the allowlist.
  - The first answer is a baseline, so nothing replays after a restart.
- **Requests:** conditional (ETag, so 304s are free), and paused until the reset when fewer than 100 requests are
  left. `mountain_github_token_days_left` is read from the expiry header.
- **No token = off:** one log line and no `source="github"` series.
- **Failures:**
  - A failing GitHub never marks the page stale.
  - A repo the token cannot see (404) is skipped and logged once.
- **Fixture mode** serves a fake GitHub API from the same timeline (`"gh"`, hostile payloads).

**WS protocol:**
- Server → client:
  - `hello{snapshot{day, epoch}, deltas, state, metrics, town}`
  - `ev`
  - `metrics`
  - `world{plot|mark}`
- Client → server:
  - `subscribe{filter}`
  - `scrub{day}` → `replay{day, snapshot, state, frames, metrics}`, to that client only (`relay/scrub.go`).
    - `snapshot` + `state` are the town at the start of the UTC day: the latest snapshot before it plus the frames in
      between.
    - `frames` are the day's `ev` and `world` frames in broadcast order. `metrics` are its `metrics_5m` frames.
    - Errors are `replay{day?, error: bad-day|rate|unavailable}`:
      - `bad-day`: a bad format, a future day, or more than 30 days back;
      - `rate`: more than 1 scrub per 2 s;
      - `unavailable`: no DB, a DB error, or **a day before `town_epoch.started`**.

      A paused client therefore always hears back.
    - Client side: `world.scrub(day)` holds live frames off until `world.release()`, which resyncs through a fresh
      hello.
  - `mark{mark, pose, named?}`, for a session in `mountain-visitors` or `platform-admins`. The sender alone hears back
    `{t:"mark", ref?, id}` or `{…, error}`. The broadcast `world:mark` is queued before the ack on the same
    connection, so the sender's state already holds the mark when the ack lands.
  - `mark.delete{id}`, admins only (08).
- `action`: v2.

**Dev and fixture modes:**
- `relay --dev` allows an `http://127.0.0.1:*` Origin and adds `connect-src ws://127.0.0.1:*` to the CSP. A Go test
  proves the dev flags are off by default and that production still rejects foreign origins with 403.
- `relay --source=fixture:<file>` serves a scripted Prometheus/Alertmanager/GitHub timeline through an in-process fake
  HTTP server, seeded with hostile labels (04).
- `relay dump-hello --events <file> --out <file>` prints a `hello` built by the real relay state fold. It is the only
  source of the hello fixture.

**Keepalive:**
- A server ping every 30 s.
- Clients reconnect with jittered backoff and resync through `hello`.
- Logging in requires a reconnect, because the cookie is read at upgrade.

**Security:**
- `Origin` must equal `https://mountain.bnei.dev` at upgrade.
- **coder/websocket v1.8.15:**
  - Its default origin check ignores the scheme and passes an empty `Origin`.
  - So the relay runs its own `originOK` check (exact scheme + host; empty = reject; the dev flag adds
    `http://127.0.0.1:*`), then calls `Accept` with `InsecureSkipVerify: true`.
  - `Ping` needs a concurrent reader goroutine, or it never returns.
- Cookie: `Secure; HttpOnly; SameSite=Lax`, signed, **TTL ≤ 1 h** with a groups re-check on refresh.
- OIDC `state` is random. The return view is in a short-lived signed cookie.
- **`CF-Connecting-IP` is trusted only when the TCP peer is in a Cloudflare range.** Otherwise the peer IP is used
  (LAN/pod CIDR spoofing is accepted and noted).

Limits:

| Limit | Value |
|---|---|
| WS connections per IP | 4 |
| Client message rate | 10/s |
| Client frame size | 4 KB |
| Scrub | 1 per 2 s, per connection |
| Marks per account | 1/min, 20 per 24 h |
| Active marks, global cap | 2000 |
| Plots | 64 |
| Day replays | cached in memory (the last 7 built): a day that ended > 1 min ago stays until evicted, today for 10 s. At most 2 builds at once relay-wide (else `rate`) |

**Kill switch:**
- `VISITORS_ENABLED=false` hides the visitor plants and rejects marks, with no deploy. The flag is a mounted Secret
  file, re-read every 10 s, and is reported as `metrics.visitors`.
- `hello.state.marks` still carries the `by` handles; clients only hide them.
- So the switch is not the tool for an abusive handle. Admin `mark.delete` is, along with deactivating the user in
  authentik.

**Self-observability:**
- `mountain_source_last_success{source}`, including `source="db"` (Postgres writes + a 30 s ping; memory-only ages)
- `mountain_unmapped_total`
- `mountain_github_token_days_left`
- `mountain_ws_connections`
- `mountain_db_dropped_total{reason=full|error}`
- `mountain_scrub_dropped_total`: scrub replies lost to a full send buffer (the connection is dropped)

The optional `pods` query failing is logged once and never marks prom stale.

The ServiceMonitor and PrometheusRule live in **the mountain app's `extraManifests`**, since common-app-chart has no
template for them.

Rules (all `severity: notice` → Discord):
- `MountainSourceStale`
- `MountainTokenExpiring` (< 14 d)
- `MountainUnmapped`

Why `notice`:
- **Not `warning`**, which would wake thot.
- **Not `info`** either: the live Alertmanager keeps kube-prometheus-stack's `InfoInhibitor` inhibit rule
  (`target severity="info", equal [namespace]`). It swallows every info alert unless a warning or critical fires in
  the same namespace.
- `notice` matches neither the thot route (`severity=~"critical|warning"`) nor that inhibit rule, so it falls through
  to the default `discord` route.
- The relay's alert source drops these (unknown `sev`), which is fine: stale already shows on the page.

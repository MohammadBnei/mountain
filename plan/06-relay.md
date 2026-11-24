# 06-relay — Relay

> Status: history (2026-11-24). Already history (absorbed by atlas).

**Process:** 1 replica, `Recreate`. The Go binary embeds `www/` through the root package `mountain` (`embed.go`,
`//go:embed www`), imported by relay. Go 1.27 toolchain.

**Ports:**
- `:8080` routed: `/`, `/v/<build>/…`, `/ws`, `/auth/*`, `/healthz`. `/api/topology` is gone (R7): plots ride the
  hello. Since R8 deploy C every routed path sits behind the authentik forwardAuth gate (infra-bootstrap ADR-0065;
  `helm/values.yaml` `ingress.middlewares`); `/healthz` probes and `:9090` never go through the ingress.
- **`:9090` metrics, a separate port via `service.extraPorts`**, never routed by the IngressRoute.

**Static files:**
- Cloudflare overrides Cache-Control, so every asset `index.html` references lives under `/v/<build>/`, immutable:
  a new deploy means a new URL.
- `/` is `no-cache`, and the CSP hashes the importmap actually served.
- Another build's `/v/<id>/` is a `no-store` 404. `main.js` answers it with one reload onto the current index.

**`relay/town` is the only implementation of the town geometry (R7):**
- **Hexes** are pointy-top axial, as in pi-lab `kit.hex`: x = 2(q + r/2), z = √3·r.
- **Islands** (R8) are sized by their host's hardware: score = 0.45·threads + 0.4·RAM + 0.15·disk, each normalised to
  the strongest host, radius = 4 + round(3·(s − s_min)/(1 − s_min)). h1 (12 threads, 32 GB, 2 TB) → 7 (169 hexes);
  h2 (6, 32 GB, 476 GB) → 5 (91); h3 (4, 15 GB, 238 GB) → 4 (61). `town.Radii` is hard-coded (it builds before the
  config loads); `TestRadii` recomputes it from the specs.
- The islands stay an equilateral triangle of side max(rᵢ + rⱼ) + 3 = 15. Centres: h1 (−5, −5), h2 (10, −5),
  h3 (−5, 10). A bridge head is dir·r on its island, so the widest strait (h1–h2) is 3 hexes.
- **Tiles**, per local hex:
  - `s`: the square, at the centre;
  - `r`: roads, the spokes from the square to each bridge head;
  - `b`: the bridge heads, which face h1–h2 (1, 0), h1–h3 (0, 1) and h2–h3 (−1, 1);
  - `g`: grass, rings 1..r−1: the town (1..r−2) and the outskirts (r−1);
  - `m`: meadow, ring r, for marks only.
- **`town.Get()`** is sent as `hello.town {islands[{host, q, r, radius, tiles}], bridges[{a, b, aq, ar, bq, br}]}`.
  `tiles` is one letter per hex in canonical order: r ascending, then q.
- **`town.Slot`** places plots by the kind's family (02 Placement, 03 Building kinds) and **`town.Plantable`** says
  where a plant may go.
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
- **Room (R7.1):**
  - **Wilting:** the prom poll removes plants older than 7 days (`world:mark gone`, row `deleted_at`). The ULID id is
    the planting time.
  - **Full island:** when an island has no free hex, its oldest plant's hex accepts a new plant, which replaces it.
    The old plant's `gone` is sent before the new mark.
  - The toolbar's aiming ring mirrors both rules.

**src_github (`relay/src_github.go`):**
> 2026-11-21 (P4.5): the relay is gone; atlas, which took over its repo poll, reads Forgejo now
> (`--forge-url`, `MOUNTAIN_FORGE_TOKEN`, source label `forge`), with no conditional request and no token-expiry
> header, so `mountain_github_token_days_left` and `MountainTokenExpiring` below are gone. See plan/10 "A2 as built".
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
  - `hello{snapshot{day, epoch}, deltas, state, metrics, town, build?}`. `build` (R8) is the served www build id; a
    page whose own `/v/<build>/` differs reloads once.
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
  - `mark{mark, pose, named?}`, for a session in `mountain-visitors` or `platform-admins`. `/auth/me` also returns
    the quota `{next, left}` and `signedIn` (a session without a planting group), since R7.1. R8: for role
    `admin` only, `names {machine key: real hostname}`; the reply is `private, no-store`, `Vary: Cookie`, no CORS. The sender alone hears back
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
- **The gate (R8 C):** the relay trusts no forwarded identity; the page signs in to it by itself behind the gate
  (`toys.js autoSignIn`, once per 10 min) and reloads once onto the login wall when the gate session lapses
  (`socket.js gateLapsed`: `/auth/me` with `redirect: 'manual'`, an opaque redirect or a 401). `/v/` is
  `Cache-Control: private` so Cloudflare keeps no copy.
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
- `mountain_github_token_days_left` (gone, P4.5)
- `mountain_ws_connections`
- `mountain_db_dropped_total{reason=full|error}`
- `mountain_scrub_dropped_total`: scrub replies lost to a full send buffer (the connection is dropped)
- `mountain_plots` (R7.1): the standing buildings
- `mountain_query_failed_total{query}` (R7.1): failed answers of the optional `pods` query, which is logged once and
  never marks prom stale

The ServiceMonitor and PrometheusRule live in **the mountain app's `extraManifests`**, since common-app-chart has no
template for them.

Rules (all `severity: notice` → Discord):
- `MountainSourceStale`
- `MountainTokenExpiring` (< 14 d) (gone, P4.5)
- `MountainUnmapped`
- `MountainTownEmpty` (R7.1): `mountain_plots == 0` for 15 min, the alarm a broken `pods` query would otherwise never
  raise

Why `notice`:
- **Not `warning`**, which would wake thot.
- **Not `info`** either: the live Alertmanager keeps kube-prometheus-stack's `InfoInhibitor` inhibit rule
  (`target severity="info", equal [namespace]`). It swallows every info alert unless a warning or critical fires in
  the same namespace.
- `notice` matches neither the thot route (`severity=~"critical|warning"`) nor that inhibit rule, so it falls through
  to the default `discord` route.
- The relay's alert source drops these (unknown `sev`), which is fine: stale already shows on the page.

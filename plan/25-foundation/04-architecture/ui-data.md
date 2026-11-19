# ADR: ui-data, what is shown apart from how and where

Status: agreed with the owner in the architecture interview of 2026-11-18 and 2026-11-19 (after round 4,
03-interview/round-4-hud.md), then put through a doubt cycle (fresh adversarial read of this record and the code; its
findings are folded in below). The owner's words: "build the base, we'll get it perfect later". It sits on ui-core
(ui-core.md): the store, the surface table and the actions registry stay as they are; this record adds the data and
placement layers beside them.

## Context

The owner's constraint: change the style and move the UI around at will, which needs what is shown kept apart from how
and where it is shown. Round 4 also makes the right zone of strategy a data-dense inspector (Thread, Logs, Metrics,
Spans, H-7 and H-8), and other services (fleet first) are to reuse the same data through a solid contract.

What exists today (ukubi-game main db56ee51, mountain main 5fd2cc7, infra-bootstrap main 21fdce54, agent-fleet main
666dbdcc):
- The surface table decides whether a surface shows for a state (`ui-surfaces.ts` TABLE), not where: placement lives
  in the `hud-*.css` files and each component's markup.
- mountain's `contract` package is what atlas reads from the systems it does not own (PromQL, labels, Alertmanager,
  GitHub), standard library only, and the sim mirrors it (`contract/contract.go:1-6`). atlas reads Prometheus and
  Alertmanager; its network policy lets it reach :9090 and :9093 only, not Loki or Tempo
  (infra-bootstrap `gitops/bootstrap/monitoring-ciliumnetworkpolicy.yaml:58-68`). Its routes are plain HTTP and JSON
  plus a websocket (`cmd/atlas/main.go:250-259`); mountain has no protobuf.
- The browser does not reach atlas. The game's server reads it with a machine token that never reaches the page
  (ukubi-game `cmd/game/main.go:151`), and the machine principal may read only `GET /api/graph` and `GET /ws`
  (mountain `cmd/atlas/machine.go:46`).
- agent-fleet core already answers some telemetry for fleet (`QueryLogs`, `QueryMetrics` in its dashboard proto) and
  may reach Prometheus and Loki (the same policy, :80-90). That is fleet's backend, for fleet's own views; it is not
  the joined, per-subject source this record needs.
- The game reaches agent-fleet through a hand-rolled Connect JSON client (`cmd/game/websrc/fleet-client.ts`), whose
  `follow` reconnects a stream that core ends at 90 s, under Cloudflare's 100 s (`fleet-client.ts:7,162`).
- The page's CSP is `default-src 'self'` with `connect-src 'self'` plus the configured origins, and no `frame-src`
  (`internal/gate/gate.go:244-256`); Grafana has no `allow_embedding` (infra-bootstrap `values/grafana/values.yaml`).
- The page bundle sits near its budget: world.js 2,476,870 of 2,510,000 bytes (`dev/budgets.sh:20`) after ukubi-game #318, about 33 KB left; the startup work (S2) takes about 1 KB more.

## Decision

Four layers, joined only through typed interfaces.

### 1. Source

Services we own, called through generated Connect clients.

- **atlas serves telemetry** (owner, 2026-11-19: "it makes sense for atlas to do it, specially since we talked about
  the contract"). atlas gains `atlas.telemetry.v1.TelemetryService`: `Read(subject, face, window)` and
  `Watch(subject, face)`, answering typed frames (series, log lines, spans), each with its source's age and a Grafana
  Explore URL for the same query and window, built by atlas. atlas joins a subject (an atlas id such as
  `building:repo:agent-fleet`) to its Prometheus labels, Loki stream and Tempo service through `contract`; nothing
  outside atlas writes PromQL, LogQL or TraceQL.
- **The page reaches it through the game's server, same origin.** The game's server mounts the telemetry service's
  Connect paths behind its owner-only gate and forwards them to atlas with its machine token, as it already reads
  `/api/graph`. No CORS, no change to `connect-src`, and the token stays server-side. atlas adds the two RPC paths to
  `machineRoutes`; they write nothing, so the principal stays read-only by construction.
- **`Watch` ends before 90 s** and the client resumes it, the same rule as core's streams (Cloudflare's 100 s).
- **Fleet is the second source**: agent-fleet's own Connect services (characters, threads, decisions). Fleet, or any
  other service, consumes telemetry from atlas's API, not from its own backends, when it needs the joined view.
- **Protobuf and Connect** (owner's pick: "use the right tool, specially for fleet").
  - Each repo owns its protos. atlas's live in a module of their own in mountain (for example `api/telemetry/v1`), so
    `contract` stays standard library only.
  - A consumer runs `buf generate` against the owner's git repo at a pinned commit (a main SHA only) and commits the
    output: the game's generated TypeScript is checked like `web/dist`; Go consumers import the owner's generated
    package as a module, as ukubi-game already imports agent-fleet's (`go.mod:6-7`).
  - mountain is private: the game's CI reads it with the token it already uses for the mountain module (`ci.yml:42-50`,
    `MOUNTAIN_READ_TOKEN`). `buf`, `protoc-gen-es` and connect-es are pinned to exact versions; the game's
    `buf.gen.yaml` sets `include_imports`.
  - No Buf Schema Registry. `buf breaking` runs in each owner's CI against its main.
- **The game uses generated clients only**, fleet's included. `fleet-client.ts` keeps `follow` (its reconnect and
  back-off rules, named in I8's allow-list, `ui-invariants.contract.test.ts:509`) and swaps its hand-rolled transport
  for the generated client. The generated code, the adapters and the owner-only components load in an owner chunk by `import()`, from a
  code-splitting build of world.ts so the chunk shares Preact, the signals and the store with world.js (hud-build.md
  HB-2; today every dist file is its own entry, and `ui-live.ts`, `world-characters.ts` and `world-fleet.ts` import
  the fleet client statically into world.js). The spike measured the clients at 144 KB minified (40.6 KB gzip), the
  runtime 110 KB of it, so a static import would break world.js's budget. The public page does not grow.

### 2. Type

A small fixed set of UI types in a new file, `ui-data.ts`, independent of any source: `Series`, `Stat`, `LogLines`,
`SpanTree`, `Thread`, `Decision`, `Line`, `Character`, `EntityRef`, `Unknown` (with the age of the last data, or a
reason).

- They are not in `ui-contracts.ts`: that file holds store state, and its I4 test bans fields such as `lines`, `text`
  and `entries` there (`ui-invariants.contract.test.ts:293-300`). UI types are component inputs, not store state: I4
  stands (a thread's text never enters the store), and the store keeps only ids, cursors and which view is chosen.
- One adapter per source maps its messages into them. A source may offer several types for the same data (a bundle:
  atlas latency as `Series` and `Stat`; a fleet run as `Thread` and `Line`). The adapter is where "unknown" and a
  stale age are decided, once (round-3.md, Q73-82).

### 3. Component

Components are registered per UI type (a `Series` as a sparkline or a chart with axes; `LogLines` as a list with a
level filter). They never import generated code or know a source. Style comes from the theme tokens.

### 4. Placement

Layout data, per surface and per form (phone, desktop), edited in the repo and shipped with a deploy (owner: "us, by
editing layout data"; no live editor, no per-player layouts).

- **grid**: a named slot, one CSS grid area (the map corner, the zones, the rail, the strip, the updates).
- **anchor**: attached like a popover to one of three targets:
  - an entity in the world, through its `Source.anchor(id)`: a top point and a radius, defaulting to `at()` plus a
    height and radius per kind;
  - another surface (the tracker under the map);
  - the screen edge (the arrow for a thing off screen).
  Entity anchors are projected per frame through the fast bindings (`ui-fast.ts`), never the store (I6).
- Placement sets grid areas and position variables on surfaces that stay where they are mounted. It never moves a
  node to another parent (I5).
- Each slot declares the UI types it accepts, in order, and a component per type, with a default. The first type in
  the source's bundle that the slot accepts is shown; if none matches, the slot shows `Unknown` with its reason.
- A view chip on the slot lets the viewer switch between the matching types and components. The choice is kept per
  slot in the viewer's browser storage, never in the layout.

### The faces' limits

Defaults, tweakable in the layout:
- Metrics: at most 4 series, windows 15 min, 1 h and 24 h, no query editor.
- Logs: the last 200 lines, a level filter, a text search and a live tail.
- Spans: the last 20 traces and one waterfall (Tempo keeps 72 h).
- Each face has "Open in Grafana ›", the URL atlas sent with the frame. No Grafana in a frame.

### New invariants

ui-core's I1-I9 stand.

| # | Invariant | Enforced by |
|---|---|---|
| I10 | Every slot names only UI types that exist, and each has a registered component | a test over the layout registry |
| I11 | No component imports generated code or a source client | an AST guard |
| I12 | At most 10 anchored surfaces show at once, nearest first; the ◆ marks give way before the prompt and the radial | a test over the anchor layer |
| I13 | The page reaches telemetry only through the game's same-origin forward to atlas's Connect API | a test that no client names a backend host, and `connect-src` unchanged |

## Alternatives considered

- **Fleet core serves the faces** (its `QueryLogs` and `QueryMetrics` exist and the page reaches it): rejected by the
  owner; core is fleet's backend, and the joins of a subject to its telemetry belong with atlas and `contract`.
- **The page calls atlas directly**: CORS on atlas, a preflight that forwardAuth refuses, and an owner token in the
  page. The same-origin forward costs one handler.
- **The game's server queries Prometheus, Loki and Tempo itself**: a second owner of the joins, and the queries in
  the game.
- **No data faces**, only Grafana links and the character's summary: not the inspector the owner asked for.
- **A telemetry gateway service of its own, with a cache**: right at ten times the load, early with one consumer. It
  is the upgrade path if the bet fails; the API stays `atlas.telemetry.v1`.
- **Grafana panels in iframes**: a CSP change, `allow_embedding`, the authentik session in a third-party frame,
  Grafana's look in the right zone's narrow column, and a click-jacking surface on the page that holds merge and
  approval buttons.
- **Hand-rolled Connect JSON in the page** (today's `fleet-client.ts` transport): refused by the owner; the generated
  clients are the tool.
- **Proto messages straight into components**: one layer fewer, but no single place for "unknown", no fixture without
  a backend, and generated code in the components.

## Consequences

- mountain gets a buf toolchain and a protos module. atlas grows a Connect API beside its routes, mounted as an
  `http.Handler` behind its gate, with the two paths added to `machineRoutes`. atlas reads Loki and Tempo for the first time, and exports a read-duration metric per source (it has none today), so the bet is checked in production. Log filters prefer a text match: the spike found `detected_level` 2-3 times slower.
- infra-bootstrap: the monitoring network policy lets mountain's pods reach Loki (:3100) and Tempo (:3200). A
  draft infra PR, applied by the owner.
- ukubi-game: a forward handler for the telemetry paths behind the owner gate; committed generated TypeScript,
  re-pinned on each schema change (as it re-pins the mountain module); `fleet-client.ts` moves to the generated
  transport; a new chunk entry and budget line.
- The sim answers telemetry with canned frames per subject (series, lines, spans) for dev and the gates; it does not
  evaluate LogQL or TraceQL.
- Restyling is a token change; moving a surface is a layout-data change; a new view of existing data is an adapter or
  a component, never both.
- The HUD fixture feeds components canned UI types, with no backend.

## The bet

Spike, 2026-11-19 (ui-data-spike.md): holds on cost.


atlas can serve on-demand, per-subject queries (Loki and Tempo as well as Prometheus) beside its live frame without
slowing the frame's poll. If not, a gateway takes the same `atlas.telemetry.v1` contract and atlas keeps the frame.

## Not doing

- A live layout editor or per-player layouts.
- Grafana in a frame; a query editor in the faces.
- A Buf Schema Registry.
- Telemetry through fleet core.

## Open (not blocking the base)

- A second consumer of atlas's API (fleet): its own machine principal (today there is one, `MOUNTAIN_MACHINE_SUB`),
  and how public agent-fleet's CI reads private mountain's protos.
- The speech tool's new fields on agent-fleet (the short in-character line and the reply choices, round 4 H-10).
- `Watch`'s caching and its fan-in when several pages watch one subject.
- The mode at the bridge, inside a building (the `overview` row requires `place !== 'inside'`).

# 15 · The ukubi architecture standard (2026-10-27)

> Status: **agreed** with the owner in an architect interview (2026-10-27), then put through one doubt cycle (14
> findings). The five findings that touched an owner decision went back to the owner and were answered. It holds
> for every ukubi app written from here on: atlas (mountain), the game (ukubi-game), the kit (ukubi-kit), and, by
> convention in their own repos, the bridge and the fleet. Existing code moves to it step by step (the end of this
> file), one behaviour-preserving PR at a time.

## Why
The owner's words, on the console's `Provider` interface: "You see the usefulness of the go interface here, how it
cleans things up and simplifies everything? I want this level of code design throughout our apps!"

Three surveys of the code found the same smell in every repo:
- a list that grows by hand: a switch per kind, a block per signal, a handler per command;
- a source hard-wired into its consumer, so tests need an HTTP server where a map would do.

This standard names the few shapes that remove both, and where each one applies. It also names where each one
doesn't.

## 1. Seams: one narrow interface per source
- **Every external source sits behind an interface its consumer owns,** with 1–3 methods that say what the source
  does for this consumer (Go's idiom: the interface lives with its user). That covers Prometheus, Alertmanager,
  GitHub, the sim's control API, the fleet, Kubernetes, ArgoCD, Patroni and Postgres.
  ```go
  // game/ owns this; github.Client, the sim's forge and a map satisfy it
  type Forge interface {
      Head(ctx context.Context, repo, branch string) (string, error)
      Compare(ctx context.Context, repo, base, head string) (string, error)
  }
  ```
- **A seam reads or commands, never both.** A source whose read also writes stays out of a registry of reads. The
  fleet's tick dispatches steps, so it is not one of the game's need sources.
- **Sources we own** (atlas, fleet core, the bridge) **get no fakes.** Tests run the real service, as the game's
  integration tests already run the real atlas. This keeps the game's rule: "never a fake of something we own".
- **Sources we don't own** get **one contract suite per seam**, run against three implementations:
  1. **The real client, replaying golden responses recorded from the live cluster.** They are recorded with the
     dev-pod skill and refreshed by a nightly live job whenever the self-hosted runner is up. A public service
     needs no cluster: GitHub's leg is recorded from api.github.com, and its nightly live check runs on a
     GitHub-hosted runner (ukubi-game `live.yml`).
  2. **The sim's mirror,** so the sim can never drift from what the real client expects.
  3. **A map fake,** for fast unit tests.
- **Kubernetes:**
  - It uses **client-go** behind the narrow interfaces, never raw REST of our own.
  - It is tested with envtest or kind on GitHub-hosted runners.
  - The sim's mirror serves only the verbs the providers use (discovery included). They are listed in a contract
    package, so client-go talks to the mirror unchanged.

## 2. Two idioms, chosen by shape
**Ordered stages for one-shot transforms**
- A transform is a plain ordered slice of `{Name, Version, Run}`. Its order is part of its meaning, so it is never a
  map. Its versions are recorded wherever the output is stored.
  ```go
  var Passes = []Pass{
      {"declare", 1, declare},
      {"resolve", 1, resolve},
      {"watch", 3, watch}, // bump when its logic changes: the projection records it
  }
  ```
- **Used by:** atlas's extraction passes and live-signal joiners, the game's card sections, and the kit's build
  checks (the same shape in JS).
- **No generic runner** until a second user really shares one.
- **Not used for:** the sim's frame. Its steps share state, and their order is what the mirrors serve, so it stays a
  function.

**State machines, only where the lifecycle is ours and lives over time**
- A transition table plus guards, with a test that every state can be reached.
  ```go
  var Wonder = fsm.Table[WonderState, WonderEvent]{
      Opened: {GoalDone: Rising, Drop: Dropped},
      Rising: {GoalDone: Rising, Finish: Standing, Drop: Dropped},
  }
  ```
- **Used by:** wonders, the console's write tokens, and the sim's incidents.
- **The event-log fold stays total.** Guards run when a command is accepted, never while a log is replayed, so an old
  log always loads after a guard tightens.
- **Projections are not machines.** Construction sites (a function of live health), units (core owns its session
  states) and the console's marks (cluster facts) are recomputed from their sources, never driven by a table.

## 3. Layout
- **`cmd/<bin>/main.go` holds only config and wiring,** about 100–150 lines. The logic lives in `internal/`
  packages, each testable alone.
- **No mutable package-level state.** Every dependency is built in `main` and passed to a constructor. Tests never
  swap globals.
- **Shared code is extracted on second use.** A package starts in the app that needs it. It moves to the nested
  module **`mountain/pkg`** (its own `go.mod`, tagged `pkg/vX`) only once a second app uses it, with mountain's
  Dockerfile and CI paths updated in the same PR.
- **The bridge and the fleet** follow these conventions in their own repos. They don't import mountain.

## 4. Errors: one model for every app
- **The codes are Connect's 16** (`invalid_argument`, `failed_precondition`, `not_found`, `permission_denied`,
  `unavailable`, `internal`, …), which the fleet and the bridge already use. The code strings and Connect's
  code-to-HTTP-status table are copied in (about 40 lines). There is no Connect dependency in atlas or the game.
- **An error carries:**
  - its **code**;
  - a stable **key** (`argocd.not_an_app`);
  - **say**, the words a person reads;
  - the **source** whose words the detail holds;
  - **detail**, the source's own message, kept as it was;
  - the **cause**, which can be unwrapped.
  ```go
  return errs.Off("argocd.not_an_app", "Postgres is not an ArgoCD app")      // failed_precondition
  return errs.Unavailable("argocd", err)                                       // keeps ArgoCD's words
  // on the wire: {"code":"failed_precondition","key":"argocd.not_an_app","message":"Postgres is not an ArgoCD app","source":"","detail":""}
  ```
- **Each app keeps a closed registry of its keys.** A test fails on an unregistered key. Pages translate keys and
  never parse messages.
- **Every error is recorded on its span** (§7).
- **Moving an app to this body changes what users see:** today the game shows plain-text errors as they come. Its
  clients are updated in the same PR, and the PR says so.

## 5. Config
- **One `Config` struct per binary,** built from flags and the environment, and validated once at start. It fails
  closed and reports every error at once, for example a sign-in only partly set.
- **Nothing else reads `os.Getenv`.** A test enforces it.
- **Every field appears in the README's table,** and a test checks that.
- **An optional part is a nil pointer.** No fleet token means a nil `Fleet` config, so the fleet provider is simply
  not registered. No scattered nil checks.

## 6. Event log
- **`Log[E]`** has two methods: `Append` and `Replay`. State is a fold over it.
- **The File backend comes first.** It keeps today's guarantees: torn lines tolerated, fsync, atomic multi-event
  appends, and a file from another world refused. In the cluster it lives on a PVC, with one replica.
- **A Postgres backend comes once an app has a database of its own.** That needs:
  - a Pigsty user and database, which is a human run;
  - an owner role;
  - migrations owned by the consuming app;
  - one transaction per record;
  - a `world` key.
  
  An audit table there grants only INSERT and SELECT.

## 7. Observability: OpenTelemetry
- **Every app uses the OpenTelemetry API.**
- **Traces are pushed:** OTLP to Alloy, and on to **Tempo, stored on Garage S3**.
  - Connect's `otelconnect` instruments the fleet and the bridge.
  - An HTTP middleware instruments atlas and the game.
  - Spans cross from the game to atlas, the fleet and ArgoCD.
- **Metrics are pulled.**
  - OTel's Prometheus exporter serves today's `/metrics` port, with views that pin today's names, units and
    buckets.
  - Scrapes, their `namespace`, `pod` and `job` labels, `TargetDown`, mountain's promtool-tested rules and fleet
    core's read-back of its own metrics all stay as they are.
- **Logs stay JSON on stdout** and gain `trace_id` and `span_id`. Alloy keeps tailing them, because the bridge's
  and the fleet's Loki alerts, and fleet core's log reads, select on the labels the file tailing gives.
- **Tests:** each app's metric names, and its log `level` and messages, are unchanged.
- **The sim:** OTel is emitted, never read, so the sim needs no mirror for it. It exports to stdout or nowhere.
- **The infra-bootstrap ADR** adds Tempo, Alloy's OTLP receiver and its Service, and Grafana's datasources and its
  links between traces, logs and metrics.

## 8. Front end
- **There is one 3D client: the game's.**
  - atlas's L0–L3 pages are **retired as soon as slice 4f lands** (owner). Until the game deploys, the 3D view
    exists only locally.
  - atlas keeps its reader (L4).
- **Registries, each a typed shape:**
  - lenses: `{id, label, build, legend, apply?}`. A lens's labels are made by its `build` from atlas's data, so they
    hang on its group (`g.userData.labels`), not on the lens;
  - live looks: `{id, init, apply, frame?, summary}`. Each look keeps its own state, made by `init`. The status line
    stays the page's own projection, because its words mix the looks with what isn't one (construction, staleness);
  - card sections, as a table of rows `{k, title, items, empty, when}`;
  - levels: `{mount, panel, pick}`, from mountain's level interface;
  - the console: the command grid, the drawers (one per transport) and the confirmation.
  
  The page wires them together and names none of them.
- **The build: TypeScript, with Bun.**
  - The output is committed as `web/dist`. CI, with Bun pinned to an exact version, fails when it's stale and runs
    `bun test`.
  - `go build`, `go test` and the integration tests need no Bun.
  - The Playwright gates stay on Node, which Playwright supports.
  - npm `three` is pinned to the version that is vendored today.
- **No inline script.** Bun bundles `three`, so the importmap goes and the CSP needs no hashes.
- **The order:** the registry split first, in plain JS. The conversion to TypeScript follows in its own PR, so each
  can be checked against the gates.

## 9. Toolchain
- **Bun is the JS toolchain everywhere,** pinned to an exact version: the game's web, and ukubi-kit (owner: now,
  accepting a rebuild).
- **The kit's dist may change.** It is hashed byte for byte, and `Math.sin`/`cos` can differ between V8 and
  JavaScriptCore. If it does, the new models are reviewed once in that PR, the kit is bumped in the game, and the
  game's budget gate runs again.

## 10. Kept as found
These already follow the standard:
- atlas's `extract.Extractor`/`Ctx`;
- the game's `game.Agents`;
- `mirror.Listener`;
- the game's router with its route table;
- mountain's `router.js` and its level interface (which become the game's levels);
- `contract`'s data tables;
- the kit's generator contract.

## Moving existing code to it (each step one PR, behaviour unchanged, through the usual loop)
1. **mountain CI:** the `test` job moves to `ubuntu-latest`, and `build-push` stays self-hosted. Mountain's PRs are
   then tested while the runner is down.
2. **atlas, small and urgent:**
   - its globals go: `auth`, `machine` and `devOpen` into a `gate` the app holds; `app` itself; the read cache, the
     render slots and `unread` onto the app; `liveEvery` becomes a const and `promQ` gives way to `contract.Queries`.
     The metric vecs stay global until the OTel step replaces them: moving them twice is churn;
   - the post-extraction passes become a versioned ordered list.
   
   **The gap was metadata only** (corrected in step 3). The passes carried no version, so a change to the watch,
   needs or walls logic didn't show in a projection's recorded versions. The deployed service never served a stale
   graph: a restart clears the round cache, and the store compares the graph's content. Now each pass records
   `pass:<name>`, `realms` records the digest of the realms.yaml the binary embeds, and new versions over the same
   graph store a new projection, so a projection is a pure function of what it records (plan/10 "Rebuilds"). One
   case stays: an `atlas project` run from another binary at the same vector is not replaced by the service until a
   head moves (its round cache keys on the vector).
3. **The game:**
   - `internal/`;
   - the error model and config;
   - the gate's upgrade check and session id;
   - the sim `Control` seam, which replaces two partial decoders and the scattered world checks;
   - the Forge and NeedSource seams;
   - then the console's slices (`14-console.md`).

   **Done up to the console** (ukubi-game #21–#27, 2026-10-27):
   - `internal/` (gate, play, control), the error model and the one Config per binary;
   - the upgrade check and the session id;
   - `simctl`, the game's one client of the sim, behind a `Control` each consumer owns;
   - `game.Forge`, its contract on the recorded GitHub leg, the sim's mirror and a map, with a nightly live check;
   - the need sources as a registry, and `game.AlertSource`. Its contract runs on the mirror and a map, and its
     recorded leg waits for a reachable cluster to record from.

   The console's slices are next. C1's Kubernetes reads need the same recorded leg, so C1 waits for the cluster too.
4. **The front end:**
   - `town.js` into registries (done, ukubi-game #28–#30: the card, the lenses, the live looks; each proven
     identical to main's, in its cards, its page and one frame's WebGL draw sequence);
   - Bun and TypeScript (done, ukubi-game #31–#32: bundled into a committed dist with no importmap, so the CSP
     carries no hash; then TypeScript with typed registries, each entry and the page it reads checked; the bundle
     differs from the JS one only in Bun's path comments);
   - slice 4f;
   - atlas's L0–L3 retired.
5. **Extraction:** once atlas adopts the gate, the error model and OTel, they move to `mountain/pkg`. The rest of
   atlas follows:
   - live joiners with a `promAPI` seam;
   - evidence finders for `needs:` rows;
   - a graph `store` seam (Postgres and memory);
   - `Classify` as a map of classifiers;
   - `realms` checks as a list;
   - access as a `Gate`.
6. **The sim:**
   - ladders as a registry;
   - a failure `series` type shared by app health, probes and node readiness;
   - one list of the sources that can go down.

   **Done** (ukubi-game #33–#34): the three, and no mutable package-level state in the whole module (the check
   now covers every package). The sim's frames for 38 worlds, seeds and intensities are byte-identical; the one
   difference the doubt cycle found, an empty supplier counted down, was fixed first, apart (#33).
7. **ukubi-kit:**
   - Bun (done, ukubi-kit #6, with dist unchanged: the one engine-dependent step, gltf-transform's sort of tied skin
     weights, is done by the kit in V8's order; CI checks dist under Bun on Linux and macOS, and under Node);
   - sources keyed by kind;
   - the world kit as data (no kit names in `build.mjs`);
   - checks as a list;
   - the atlas grid constants imported from procgen.
8. **The bridge and the fleet:** the error model and OTel, in their own repos.

## Verification, for every step
- **Unchanged behaviour:** the existing suites pass with no assertion edited (Go tests, integration tests, the kit's
  check, the town gate, mountain's gate and promtool rules; a test's setup, package clauses, imports and renames
  may change, each listed in the PR, `16-dev-rules.md` §2), `go vet` is clean, and a diff review confirms no behaviour change. The
  error-body PR is labelled as a behaviour change.
- **Determinism:** where a step touches it, a golden comparison before and after. atlas's projection of the fixture
  graph and the sim's frames for a seed must be identical, except for the new version entries.
- **New tests the standard brings:**
  - contract suites over all three implementations;
  - fsm reachability;
  - the key registry;
  - config documentation;
  - metric names and log format unchanged.

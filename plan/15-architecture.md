# 15 · The ukubi architecture standard (2026-10-27)

> Status: **agreed** with the owner in an architect interview (2026-10-27), then put through one doubt cycle (14
> findings). The five findings that touched an owner decision went back to the owner and were answered. It holds
> for every ukubi app written from here on: atlas (mountain), the game (ukubi-game), the kit (ukubi-kit), and, by
> convention in their own repos, the bridge and the fleet. Existing code moves to it step by step (the end of this
> file), one behaviour-preserving PR at a time.
> Amended 2026-10-28 (owner): the standard has three tiers (cluster baseline for all apps, service, static; each
> CLAUDE.md declares its tier), and shared Go lives in the public module `github.com/MohammadBnei/ukubi-go` (§3).

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
  Dockerfile and CI paths updated by the time the first app requires it. The move may be staged (amended for A6,
  2026-10-28): the module first, then each app's use, since an app can require only a version that is already on
  main. The shape: no `replace` and no committed `go.work` (a dependency's are ignored, so mountain's require must
  name a real, fetchable version: ukubi-game resolves it), the root requiring pkg by version, and CI and the image
  build reading the private module with the job's own token. Built so far (A6a): the module, its own workflow, the
  token in the test jobs. With atlas's use: the require, and the image build's buildah secret mount (in no layer).
  Tags (owner, 2026-10-28): the agent may push `pkg/vX.Y.Z` tags on main after a merge that changes `pkg/`, never
  an app's tags; until a tag exists, the version is a pseudo-version of a main commit. Each shared concern is its own
  package (`pkg/errs`, `pkg/telemetry`), so an app that imports only the error model links no OTel package. Its
  module graph still gains the OTel modules, and MVS may raise ones it shares (golang.org/x/text and x/sync for atlas):
  the PR that bumps pkg lists every module that rises.
- **The bridge and the fleet** follow these conventions in their own repos. They don't import mountain.
  - **Amended 2026-10-28 (owner):** shared Go moves to the public module `github.com/MohammadBnei/ukubi-go`
    (`errs`, `errs/connectkeys`, `telemetry` with the §7 error handler, the `ukubicheck` analyzer, the
    `ukubi.errors.v1.ErrorKey` proto). mountain, the game, the bridge, the fleet, wird and hq import it. This
    supersedes the nested `mountain/pkg` and the line above (the bridge and the fleet copy the shape). The text
    above stays as the history of what was built. Order: ukubi-go v0.1.0, then the game, then mountain, then a
    mountain PR deletes `pkg/`.

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
- **Every error is recorded on its span** (§7). `errs.Write(w, err)` will keep its signature for that: it will
  call `RecordError(*errs.Error)` on the first writer in `w`'s `Unwrap() http.ResponseWriter` chain that has it
  (the OTel middleware's will), internal errors included, so a wrapping middleware without it doesn't drop the
  record. No breaking bump in either app; a behaviour change, labelled in the OTel step.
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
- **Traces are pushed** over OTLP. The backend is open (owner, 2026-10-28: explored later; Tempo on Garage S3 was
  the first idea), so every app's tracing is opt-in and inert until `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (the one
  variable for every app) is set. The wiring for atlas and the game is `pkg/telemetry`.
  - Connect's `otelconnect` instruments the fleet and the bridge.
  - An HTTP middleware instruments atlas and the game.
  - Spans cross from the game to atlas, the fleet and ArgoCD. Until propagation lands, every server span starts a
    trace of its own, linked to a caller's traceparent, never its child (a public endpoint: the choice runs before
    the gate); trusting a machine principal's context is an option added with propagation, decided after the gate.
- **Metrics are pulled.**
  - OTel's Prometheus exporter serves today's `/metrics` port, with views that pin today's names, units and
    buckets.
  - Scrapes, their `namespace`, `pod` and `job` labels, `TargetDown`, mountain's promtool-tested rules and fleet
    core's read-back of its own metrics all stay as they are.
- **Logs are JSON on stdout** and gain `trace_id` and `span_id` (owner, 2026-10-28: atlas, the game and the bridge,
  which log text today, move to it, each site with its level). Alloy keeps tailing them, because the bridge's
  and the fleet's Loki alerts, and fleet core's log reads, select on the labels the file tailing gives.
- **Tests:** each app's metric names, and its log messages, are unchanged; a level changes only where it is labelled
  (a site given its own, or Write's internal line at ERROR).
- **The sim:** OTel is emitted, never read, so the sim needs no mirror for it. It exports to stdout or nowhere.
- **The infra-bootstrap ADR**, once the backend is chosen, adds it, Alloy's OTLP receiver and its Service, and
  Grafana's datasources and its links between traces, logs and metrics.

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
*Adoption order (owner, 2026-10-28), for every app: Config, then error keys, then logs, then traces.*
1. **mountain CI:** the `test` job moves to `ubuntu-latest`, and `build-push` stays self-hosted. Mountain's PRs are
   then tested while the runner is down.
2. **atlas, small and urgent:**
   - its globals go: `auth`, `machine` and `devOpen` into a `gate` the app holds; `app` itself; the read cache, the
     render slots and `unread` onto the app; `liveEvery` becomes a const and `promQ` gives way to `contract.Queries`.
     The metric vecs stay global until the OTel step replaces them (done, below): moving them twice is churn;
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

   The evidence finders and the realms checks are done (mountain #58: the finders a registry keyed by
   `realms.Suppliers`' kinds but postgres, since their order is not observable, each kind keeping its own evidence;
   `realms.Parse`'s checks an ordered list, since the first error wins; no version, since only the file's digest is
   stored).

   These refactors don't wait for the extraction: they change no exported API (`contract.Classify`'s signature and
   results are unchanged, and ukubi-game's suite and integration test pass against it), and the seams give the later
   error model one place to go (an unavailable source is one client's error). **Started** (2026-10-27):
   - `Classify` is a registry of classifiers (`contract`), one per measurable kind, so the list of measurable kinds
     is no longer kept twice;
   - atlas's two live sources sit behind `promAPI` and `alertsAPI`, built in `main`, and the Prometheus join is an
     ordered list of joiners. The joiners carry no version: the live frame is never stored, so there is nowhere to
     record one (§2's "wherever the output is stored").
   
   Each seam's contract suite runs the real client against a local server answering in the API's wire format
   (hand-written bodies, not recordings), and a map fake. §1's other two legs are missing: the recorded leg waits for
   a reachable cluster, as the game's `AlertSource` does, and the sim's mirror lives in ukubi-game, which imports
   this module, so its leg is ukubi-game's integration test (the real atlas against the mirror), not the same cases.

   Then atlas's graph store sat behind two seams, `projections` (Graphs.put) and `history` (serveGraph's older
   projections), served by a Postgres store and a memory one. Those are both atlas's own (the real Postgres in the
   db-test job, and memory, a production mode), so §1's legs don't apply: they run shared cases (new, same, changed,
   lost, dropped, another writer) and differ by design where the tests say (memory keeps no history, tells a change
   by content, and keeps the latest's seq on a lost repo, where Postgres answers 0).

   Access as a `Gate`, first part: atlas's sign-out ends the session (a session id and a revocation set, as the
   game's since its #22, plus the socket of a signed-out session closing at its next ping, with a code the page stops
   reconnecting on, and a signed-out mark that stops the page reloading into the sign-in). The two gates still differ
   (the game checks Host and Origin in its router, atlas Origin per handler; 401 against 403; the return pattern; the
   game's 10 s discovery timeout; /auth/me's shape; cookie names; atlas's machine principal and signed-out mark), so
   the extraction into `mountain/pkg` is its own plan.

   Then atlas's one Config (§5): `cmd/atlas/config.go` reads the flags and the environment once, and a test at the
   module root refuses a listed reader of the environment in any other non-test file (os's and syscall's own, the
   directory and proxy helpers that read one, os/exec and os/user whole, cgo and go:linkname; under any import name,
   dot imports refused). Proven by the old and new binaries over 51 cases of flags and environment, identical after
   masking timestamps and ports (exit codes, output, and for serving runs the sign-in redirect against a stub
   discovery server, a refused machine bearer and the metric names); the exiting runs also compared raw, stdout and
   stderr apart. Not proven there: a valid machine token, and the IPv6 loopback under --dev (the sandbox has no
   IPv6). Two of §5's rules are not met yet, each on purpose. "Validated once, every error at once": a sign-in partly
   set stays off silently (refusing it is a behaviour change, its own PR), the machine principal is checked in serve
   after realms.Load as before (so its refusal keeps its exit path), and errors still come one at a time (a fixtures
   error hides a machine one). "An optional part is a nil pointer": the machine's configuration is raw strings, for
   the same reason. Mountain has no README: the table is main.go's doc comment, which a test checks against each
   FlagSet's usage line and every variable the loaders read.

   Then the error model's extraction, first part (A6a): `mountain/pkg`, a nested module, holds `errs`, the game's
   model with its registry left per app (each app's keys, its sources, its test; only `KeyInternal`, Write's own, is
   shared). Nothing in mountain uses it yet; its own workflow tests it (on main too), and the test jobs read the
   private module with the job's token, ready for atlas. The image build's half (a buildah secret mount for `go mod
   download`) ships with atlas's use, after the build runner has deployed main again, so an unverified Dockerfile
   never sits in front of another change's deploy. Next: the game moves onto it behind a facade that keeps its call
   sites and its key test; then atlas speaks the model (its routes' errors; the gate's refusals stay plain text).

   Then atlas speaks the model (A6b): /api/graph's and /api/read's errors are pkg/errs bodies with keys from
   cmd/atlas/errkeys.go, today's words as each message. TestErrKeys checks it syntactically, form by form (every
   constructor called with a registered constant, named directly; no constructor as a value; errs.Key and errs.Error
   never named outside errkeys.go; no .Key write; no registry name declared again; no From or Down: atlas has no
   sources); a type-checked guard like the game's TestKeys waits for its codecheck's extraction. The gate's refusals,
   the hub's before the upgrade, the static files' 404s and the sign-in pages stay plain text, as the game's do. Two
   statuses change, since Connect has no code for them: not text 415 → 400 and too large 413 → 400
   (failed_precondition); the page's reader decides on the key (reader.js readError), falling back to the status for
   a body that is not the model's. The image build reads the private module through a secret mount: checked under
   BuildKit locally (the build passes, a rewrite to a bogus host fails the download, git's error carries no token),
   and under buildah with the job's own flags (the build passes, the token is in no layer or history entry, a missing
   or empty secret fails); it merged ahead of the runner on the owner's word.

   Then the traces' extraction (T1): the game's internal/telemetry moves, unchanged, to `pkg/telemetry`, for atlas's
   use; the game moves onto it next.

   Then the logs' shared half (L1): `pkg/telemetry.NewLogger`, JSON lines with trace ids from a span in the
   context, which an app installs as slog's default (the log package then comes through it, at INFO); `errs.Write`
   logs an error outside the model at ERROR through slog. Each app then gives every site its level, log.Fatal's
   included, and moves to stdout. Write's line has no span yet (its signature holds no context): it gains one with
   the RecordError hook, found through w's Unwrap chain.

   Then atlas's logs (L2): JSON lines, every one of its 24 sites with a level (WARN for a source, projector, GitHub
   or sign-in hiccup and for a missing optional setting; ERROR for the projector off, the latest projection or a
   read failing; the rest INFO), log.Fatal's two as ERROR before exit 1, net/http's own lines (a panic, an Accept
   error) ERROR, as Loki's detector counted them when they were text. The service logs on stdout; `atlas project`
   on stderr, its stdout being its result line; serve's flag errors stay text on stderr. The same words, exit codes
   and answers over A5's 51 cases. TestLogLevels refuses the log package in atlas (its Logger type aside),
   TestServersLog the servers' ErrorLog, TestFatal fatal's line and exit. Bumping pkg raises golang.org/x/text,
   x/sync, x/mod and x/tools, and links the OTel SDK and grpc (pkg/telemetry is one package): 25.7 → 36.5 MB,
   which atlas's traces need next anyway.

   Then atlas's OTel (O): the six series go through OTel's Prometheus exporter onto the same `/metrics`, held by a
   `Metrics` on the app (the last package-level state, the vecs, is gone; `atlas project` and the tests pass none and
   record nothing; the table of the six is locals of `newMetrics`, so no package variable is left). The exposition is the
   same series: names, types, HELP texts and label keys, compared on the built binary before and after (lines sorted,
   the values of `mountain_source_last_success` and `mountain_github_token_days_left` masked, being clocks), in three
   scenarios (a token and a stored projection with one signal failing, local checkouts with one required query failing,
   and nothing configured), with `target_info`, the `otel_scope_*` labels, suffixes and the default registry's
   `go_*`/`process_*` series off; the number format is pinned by `TestMetricsExposition`. The differences: a family
   nothing has written yet no longer prints its HELP and TYPE lines (it had no sample either way); families print in
   name order; the Content-Type gains `charset` and `escaping` and a scraper that asks for protobuf gets it (the
   platform Prometheus asks for text only). Traces are `pkg/telemetry` on `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, one
   server span per route outside the gate (`/ws` included: a socket's span lasts as long as the socket and is dropped
   at shutdown), except `/healthz`, which the kubelet's probes hit every few seconds (owner, 2026-10-28), and nothing
   at all when it is unset; a malformed value refuses
   to start (JSON ERROR line, exit 1), together with a partial machine principal's refusal when both are wrong. The
   writers take the `Metrics` in their constructors, and `TestMetricsWritersReach` and `TestServeMetrics` drive each
   writer's real path and serve's own wiring (a writer left without it records nothing, silently). Not in this step:
   `errs.Write`'s RecordError hook (§4) and `trace_id` on atlas's log lines (its log calls take no context), which
   wait for the owner's word.

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
8. **The bridge and the fleet:** the error model and OTel, in their own repos (amended 2026-10-28: through ukubi-go, §3).

## Verification, for every step
- **Unchanged behaviour:** the existing suites pass with no assertion edited (Go tests, integration tests, the kit's
  check, the town gate, mountain's gate and promtool rules; a test's setup, package clauses, imports and renames
  may change, each listed in the PR, `16-dev-rules.md` §2), `go vet` is clean, and a diff review confirms no
  behaviour change. The error-body PR is labelled as a behaviour change.
- **Determinism:** where a step touches it, a golden comparison before and after. atlas's projection of the fixture
  graph and the sim's frames for a seed must be identical, except for the new version entries.
- **New tests the standard brings:**
  - contract suites over all three implementations;
  - fsm reachability;
  - the key registry;
  - config documentation;
  - metric names and log format unchanged.

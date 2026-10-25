# 12 · R11: ukubi-game, an offline world that mirrors the contracts, and the game on top (design, 2026-10-25)

> Status: **approved 2026-10-25; slice 1a in progress** (this repo's half: `contract/` exported, `realms/` public). The
> code lives in the private repo `ukubi-game` once the owner creates it. Slices 3–5 wait on the owner's go for
> agent-fleet items (c) and (d).

## Context
The cluster is down until about 2026-10-26. atlas can only be iterated on against static fixtures, which bypass its
own clients (`--live-fixture` unmarshals straight into `LiveState`, and nothing in them ever happens). The owner
wants an all-offline world that speaks the real wire contracts, so atlas iterates fast. The same world hosts a
Civ-like game in which real agent-fleet agents do real work toward goals set per building.

Suite run:
- ideation (four code maps);
- 7 owner rounds;
- an architect round;
- doubt cycle 1 (14 findings) and doubt cycle 2 (8 findings), all folded in;
- a sync with the other session (its 4 corrections adopted).

**What this approval starts:** slices 1a, 1 and 2, which need no fleet work. Slices 3–5 are designed here but wait
on the owner's go for the agent-fleet items listed under them.

## Decisions (owner 2026-10-24/25; the other session's corrections adopted)
- **Purpose:** iterate atlas offline; prototype the game.
- **Drivers:** an adaptive director, scripted scenarios, real fleet agents.
- **Realism:** statistical replay. Synthetic models now; series recorded **forward only** from the cluster's return.
- **Time:** real time plus pause/step. Rewind/branch are deferred, but not designed out.
- **Homes:**
  - a new **private repo `ukubi-game`** for the sim, the game backend and the game client;
  - **atlas stays the catalog**;
  - the offline fleet lives in agent-fleet, as **`/kind-local` extended** (the real Jobs and provisioner; no
    process-spawning provisioner).
- **One game, two worlds:** sim or real, same contracts, one per process. The director runs in the sim only.
- **The first loop is goals per building.** The fun knob bends incident pacing, seeded opportunities and stakes;
  never work time or rewards. **No visual guard** for sim mode.
- **The query contract** is an exported Go package in mountain.
- **Orders:** AgentService (core :9091) as a registered client `game`. Work runs as definition sessions.
- **"Needs you":** core's announcer (`dev.bnei.fleet.session.waiting|resumed`), plus the game's own incidents and
  opportunities.
- **Progress** is the agent's latest `TodoWrite`. **Attribution:** evidence is the ledger (online) or the sim
  forge's push record (offline); the `Agent-Session` trailer is a label only.
- **Any session, headless or not, can ask a question. This is essential (owner, 2026-10-25).** Today `Open` allows
  `ask` only for resident threads, and ADR-0070's `Invoke` is `deny` + single-shot. So ADR-0070's amendment (item c)
  must allow `ask` in single-shot runs: a question parks the run as a pending decision and announces it, an answer
  resumes it, and the run then ends in its typed result. The other session has been told; it starts nothing
  without the owner's go.
- **A step closes on its checkable signal** (work merged, below), and **also on its typed result once (c) exists**.
- **The other session's items:**

  | Item | Owner's go |
  |---|---|
  | (a) the pack lines in `packs/fleet/PACK.md` | **go** |
  | (b) `Agent-Session`/`Agent-Definition` stamping in the commit hook | **go** |
  | (c) ADR-0070 `Invoke`/`WaitForResult`, with ask allowed | **not yet** |
  | (d) the offline-fleet ADR and kind-local work | **not yet** |

- **Deferred deep-dive:** the offline forge (PRs, checks).

## Slice 1a (start now, before the cluster returns 2026-10-26)
**mountain: a PR exporting the contract.**
- **New package `github.com/MohammadBnei/mountain/contract`** (no imports) holding:
  - the 14 PromQL queries, by name (today `promQ` in `cmd/atlas/live.go:253-269`);
  - the Alertmanager request (path and params, live.go:525) and the skipped names `Watchdog`/`InfoInhibitor`
    (live.go:547);
  - the label keys atlas reads: `name`, `health_status`, `sync_status`, `node`, `namespace`, `target`,
    `exported_service`, `alertname`, `severity`, `summary`;
  - the GitHub owner and its three routes with their Accept headers (`fetch.go:39,72,122,135`).

  atlas imports these instead of its own copies, with no behaviour change.
- **Move `internal/realms` to a public `github.com/MohammadBnei/mountain/realms`**, so the sim can map probes
  (`ProbeNode`, realms.go:39) and hosts the same way. The imports in `internal/extract` and `cmd/atlas` change, and
  nothing else.
- **Checks:** mountain's full suite and the render gate, unchanged.

**ukubi-game: scaffold and recorder** (the owner creates the private repo first):
- `go.mod` requiring mountain; `GOPRIVATE=github.com/MohammadBnei/*`; CI reads mountain with a token through
  `insteadOf`; `go.work` locally.
- **`cmd/record`** (read-only):
  - polls the 14 queries as range queries over the last minute at a 15 s step, and AM's `/api/v2/alerts`, every
    15 s;
  - appends to `recordings/<id>/{prom/<name>/<day>.json.zst, am/<day>.jsonl.zst}`, with a `meta.json` (window,
    query-table hash, graph seq);
  - `record capture` saves verbatim responses with headers into `contract/captures/<date>/`.
- **Proves (offline):** the recorder against a fake Prometheus/AM; the query table covers `contract` exactly.
- **Where the recorder runs** when the cluster is back: the k9s hub or a dev pod. To be added to
  `mountain/plan/11-cluster-return.md` as an owner step.

## Slice 1: atlas against a living world
**`cmd/sim`** in ukubi-game:
- **Listeners on 127.0.0.1:** 7401 control, 7490 Prometheus, 7493 Alertmanager. One listener per mirror, so
  `source.down` closes the socket for real.
- **Seed:** atlas's `/api/graph` plus `mountain/realms`. The sim starts with no graph and re-seeds whenever `seq`
  moves. Labels follow atlas's joins:
  - `node` lower-case → `vm:<node>`;
  - `exported_service` as `<ns>-<svc>-<port>@kubernetes`;
  - probe targets through `realms.ProbeNode`;
  - app namespaces from node attrs.
- **Frame:** `Frame{Prom map[QueryName][]Sample; Alerts []Alert}`.
  - Primitives: cpu/mem per (ns, node), disk per ns, rps per service, ArgoCD health/sync, Ready, probes.
  - These are projected into the 14 answers. Invariants: `cpu = Σ cpunode`, `mem = Σ memnode`,
    `cpucap = Σ cpucapnode`, `memcap = Σ memcapnode`.
  - Values are strings. An unknown query answers `bad_data`.
- **Model (slice 1 only):** one synthetic shape per numeric series (level × diurnal + value noise), plus two-state
  Markov chains for ArgoCD health and sync, Ready and probes.
- **Determinism:** a hash per 15 s bucket of `(seed, seriesKey, bucket)`; chains fold from hourly checkpoints. The
  world is `base(t) ⊕ fold(eventLog ≤ t)`.
- **Pause/step:** `simT = wall − pausedTotal`, and `step(d)` subtracts `d` from `pausedTotal`.
  - While paused the mirrors repeat the Frame, so atlas's values freeze.
  - atlas's own clocks (event `ts`, `at`, `stale`) keep running; that is stated, not hidden.
  - Alert `startsAt` is emitted in wall time.
- **Alerts:** rules over the Frame, under real names: KubeNodeNotReady, ProbeFailed, KubePodCrashLooping,
  KubeDeploymentReplicasMismatch, TargetDown, plus `Watchdog`. Fingerprints use AM's FNV-64a.
- **Scenarios:** `scenarios/*.yaml` timelines (`node.notready`, `probe.fail`, `app.degrade`, `source.down`,
  `alert.fire`).
- **Control API:** `/control/{pause,resume,step,scenario,inject,answer}`.
- **Dev wiring:** `dev/Procfile` runs the sim plus atlas:
  `--dev --listen 127.0.0.1:7088 --metrics 127.0.0.1:7089 --prom-url http://127.0.0.1:7490
  --am-url http://127.0.0.1:7493 --repos <checkouts>` (loopback is required for devOpen, main.go:98).
- **Proves:**
  - the real atlas binary runs against the sim with `mountain_query_failed_total` = 0 and fresh `prom`/`alerts`
    (`db` excluded, memory-only);
  - a scenario produces `ready`, `probe`, `health` and `alert.fire`/`alert.resolve` events in atlas's hub;
  - pause freezes values;
  - `source.down` sets `stale`;
  - atlas's render gate passes against the sim.

  This runs in ukubi-game CI (atlas built from the pinned mountain module).
- **The capture shape test** skips while `contract/captures/` is empty. Once the cluster is back and captures
  exist, it joins slice 1's exit check.

## Slice 2: the forge mirror and stakes
- **GitHub mirror** on 127.0.0.1:7480:
  - `/repos/MohammadBnei/<r>/commits/<branch>` (raw sha for `vnd.github.sha`, ETag/304);
  - `/compare`;
  - `/tarball/<sha>` (`git archive --format=tar.gz --prefix=MohammadBnei-<r>-<sha>/`);
  - git smart-HTTP through `git http-backend` behind `net/http/cgi`, with `http.receivepack=true`, over bare
    mirrors filled by `sim forge sync` while online;
  - receive-pack records who pushed what, as the offline evidence later.
- **atlas** drops `--repos` and takes `--github-url http://127.0.0.1:7480`, `MOUNTAIN_GITHUB_TOKEN=any`
  (project.go:131-147) and a writable `ATLAS_CACHE`.
- **Causal hook:** a push to a tracked branch starts an ArgoCD sync cycle in the sim.
- **Richer models:** log-normal levels, spikes, a restart sawtooth, disk drift.
- **Escalation ladders** (e.g. `mem-leak`: warning → CrashLooping → Degraded and a probe failure) climb while
  unanswered. In slice 2 an answer is `POST /control/answer`; from slice 3 it is the player's action in the game.
- **Scenario library.**
- **Proves:**
  - atlas's poll and tarball path works offline (ETag, 304, unpack, a new projection after a local push);
  - a push starts a sync cycle;
  - a ladder climbs, and an answer halts it.

## Later slices (designed; each waits on the owner's go for agent-fleet work)
**Slice 3: a real agent in the offline world.** Waits on (d), with (a) and (b) landed.
- **Runs** without the cluster, **but not without internet**: api.anthropic.com is the agents' one outbound host.
- **kind-local** is extended per the agent-fleet ADR (d):
  - a local git server, i.e. the sim forge reachable on the docker bridge address (kind pods can't reach the
    host's 127.0.0.1, so the sim, bridge and IdP bind that address too);
  - an explicit Agent Vault dev mode;
  - definitions and genome read from a local mirror;
  - the core manifest gains the 9091 Service port, `FLEET_CLIENT_TOKEN_GAME` and the `BRIDGE_*` env
    (`local/kind/30-core.yaml:90-95`);
  - sim-only definitions `game-<repo>` (`git.mode: push`, `branch: game/<repo>`, `invokers: [game]`) and
    `clients/game.yaml` (audience, `actors.allow: [MohammadBnei]`, `announceSessions: true`).
- **The bridge runs locally:**
  - env: `BRIDGE_GITHUB_WEBHOOK_SECRET`, `BRIDGE_ALERT_TOKEN`, `BRIDGE_DATABASE_URL` (direct) and
    `BRIDGE_OIDC_{ISSUER,CLIENT_ID,CERT}` pointing at the sim IdP (7470);
  - the IdP accepts `client_credentials` with username and password, a Host rewritten to `BRIDGE_AUTH_HOST` with
    `X-Forwarded-Proto: https`, and signs RS256 with exact `iss`/`aud`/`azp` and `sub` ∈ `BRIDGE_API_CLIENTS`;
  - the sim POSTs alerts to `/webhook/alertmanager`.
- **Game backend v0:**
  - **opening a step:** `Open(definition=game-<repo>, thread_key=goal/<g>/step/<n>, wake=true, actor=MohammadBnei,
    idempotency_key, message = the step template + goal context)`; a follow-up is another `Open` on the same
    `thread_key`;
  - **answers** go through `RespondToPermission`/`AnswerQuestion`/`ApprovePlan` with the `game` bearer;
  - **progress** comes from StreamTranscript's latest TodoWrite (a Go port of agent-fleet's `latestTodos`,
    `dashboard/src/transcript.ts:481-494`);
  - **needs-you** comes from `session.waiting` on `bridge-game`;
  - `CloseThread` runs when the goal closes. A client is capped at 500 open threads.

**Slice 4: goals per building, the first loop.**
- The game client sets goals and steps per building and shows TodoWrite progress and needs-you.
- **A step closes on its checkable signal:** the step's commits on `game/<repo>` are merged into the base, by the
  owner's **Merge** button offline (`POST /control/forge/merge`, evidence from the sim forge's push record) or by
  the PR in the ledger online.
- With (c) built, a headless step also returns its typed result and can still ask, per the owner's rule.
- An incident ends when a fix lands on the building's branch. Events: `dev.bnei.game.goal.{opened,step,closed}`
  and `dev.bnei.game.needs_you.{decision,incident,opportunity}`, with `world` in every payload.

**Slice 5:**
- **The adaptive director:**
  - observes open goals, needs-you age, answer latency and idle time;
  - controls λ, the severity mix, the opportunity rate and the ladder timers;
  - bounds: λ between 0.2× and 3×, at most 2 incidents, 10 min of calm after a resolution, nothing new while a
    needs-you is older than 15 min, never work time or rewards.
- **SLO closure.**
- **Fitted replay** (`Bootstrap`, `fit` from recordings).
- **The `real` world profile.**
- **The forge deep-dive** (PRs, checks).

## Verification
- **Slice 1a:**
  - mountain's suite and render gate, unchanged after the export;
  - the recorder against a fake Prometheus/AM;
  - query-table coverage.
- **Slice 1:**
  - unit tests: determinism (same seed and t → same Frame, across pause/step); the four projection invariants;
    alert rules; the AM fingerprint;
  - integration in CI: the real atlas against the sim, running a scenario, with the asserts listed under slice 1;
  - atlas's render gate pointed at the sim.
- **Slice 2:**
  - integration: a local push → atlas's new projection (ETag/304 seen) → a sync cycle;
  - a ladder climbs, then halts on `/control/answer`.
- **Slices 3–4:** an order in kind-local gives TodoWrite progress and a needs-you, the answer resumes it, and the
  Merge button closes the goal offline.

## Commits and process
- **ukubi-game:** owner's private repo, feature branch plus PR, merged on the owner's word. Trailer convention to
  confirm when the repo is created (mountain's rule is no AI trailer).
- **mountain:** PR on `claude/modest-mayer-zbwumy`, no AI trailer, merged on the owner's word.
- **Before each PR:** a short plan, then an adversarial review.

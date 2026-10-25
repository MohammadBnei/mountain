# 13 · R12: the game's town: buildings, civs, ages, kits (design draft, 2026-10-25)

> Status: **draft**. It comes from four owner interview rounds and three doubt cycles (19, 20 and 16 findings). The
> owner's answers below are decisions; the open questions at the end are not. Nothing is built, and each slice gets
> its own plan and doubt cycle when it starts. It extends `12-world-and-game.md` and amends rows of `00-decisions.md`
> and `09-play.md` (listed under "What changes elsewhere").

## Context
The game (ukubi-game, R11) has goals, steps, Merge, the director, and fleet agents on steps, proven against a real
core. The full agent loop still waits on the owner's `/kind-local` run. Its client is a flat panel page, and the clay
town was deleted in R10.

The owner wants the visual pass, rebuilt around **Age of Empires** dynamics: a town whose buildings change with its
age, villagers, research, utilities and defences. The binding rule holds: every mechanic is tied to a real signal,
and the sim mirrors (or, for what we own, runs) every signal the game uses.

## 1. What is a building
**Everything is a building except the hardware (owner).**
- **The land** is the 3 physical hosts. It shows as a **land lens** that recolours the ground by which host carries
  what (plan/10's lens; nothing moves). The land is not the map's layout.
- **Buildings** are every other element: apps, fleet services, VMs, containers, repos and hostnames. New elements
  join them:
  - kubespray's add-ons: Cilium, CoreDNS, nodelocaldns, kube-proxy, MetalLB, kube-vip;
  - Pigsty's Postgres cluster and its Redis;
  - the game itself.
- **How elements fold onto buildings**, so the count stays readable:
  - a hostname is a **door** on the app it routes to;
  - **ADRs are technologies**, not buildings (§3 Research);
  - Pigsty's VMs are parts of the Postgres building (pg01 and pg02 from the `pg-proxmox` group, the witness from the
    `etcd` group);
  - the fleet services are parts of their app.
- **Repos fold by what they feed** (the fixture's 14):
  - 10 feed exactly one app and fold as a **workshop annex** on it;
  - agent-fleet, vos-monolith and wird feed two apps each (open question 5);
  - infra-bootstrap feeds all 44 and stays a workshop in Code.
- **Hosts and the town centre carry no civ by design.** What is never drawn carries over: R8's list (k9s) and the
  Audience rows.

**One taxonomy, owned by atlas (owner: civs = realms).**
- atlas's realms are **reshaped** into the civs (§2): merged, split, and one added. atlas's islands and the game's
  districts then group things the same way, and repos and hostnames leave atlas's rim.
- **`function` is its own field** in `realms.yaml`, set per group and overridable per element. It says what a building
  does in play, which is separate from its civ: blackbox is in the Operations civ and works as a watchtower.
- **The component → kit-model mapping** (`model:`) lives in `realms.yaml` too. ukubi-kit holds no component names.
- An element with no civ or function is drift, except hosts and the town centre.

**Functions** (AoE-like, not the same):

| Function | Meaning | Examples |
|---|---|---|
| Town centre | the information hub: atlas and the game as one (below) | one building at the centre |
| Houses | the schedulable nodes, which hold the population (§3 Resources) | all 5 k8s nodes |
| Workshops | repos, where a unit (`game-<repo>`) is trained and works | infra-bootstrap, the game, annexes |
| Economy | data stores | Postgres, Redis, Garage, NFS |
| Utilities | network, ingress, storage | Cilium, CoreDNS, MetalLB, Traefik, Longhorn |
| Defence | identity, secrets, mesh, probes | authentik, Infisical, agent-vault, NetBird, blackbox |
| Trade | build, ship, observe | Argo CD, zot, runners, Prometheus, Grafana, Loki |
| Products | what people use | the apps and sites |
| Gates | where outside traffic enters | hostnames, as doors |

**The town centre is the information hub: atlas and the game as one (owner).**
- **On the map** it is one central building. Its interior holds the catalog, goals, needs-you, the age and research.
- **Rendering:** the town renders in the game client, under a CSP matching atlas's.
- **Reading atlas:** the client reads atlas through a **server-side machine credential**, an authentik service account
  `atlas-game` declared as a blueprint in infra-bootstrap. It is read-only, never sent to the page.
- **Admins only:** the game client is admins-only too. It gets the same `platform-admins` check and a
  call-every-route-as-visitor test, and **until it has them it stays loopback-only**. This amends Audience R10: atlas
  admits one machine principal, read-only, for the town centre.
- **Commands** stay in the game backend, and atlas keeps reading only (plan/10). `app:mountain` is the town centre,
  not an Operations building.

## 2. Civs
**A civ is a group of elements of the same category (owner), one per reshaped realm.**
- Each civ has its own kits, palette, biome and **style**. "Style" is the art, and the age is progress (§3).
- **Mixed styles are intended:** a fortress can stand next to a space base and a modern city.
- Where today's realms go:
  - Infra splits into Platform, Operations, Data and the land;
  - Platform's delivery and observability move to Operations, and its tools to Apps;
  - Events merges into Operations;
  - Identity becomes Guard;
  - Code is new.

| Civ | Holds (fixture, after the folds) | Style (draft) |
|---|---|---|
| Platform | nodes, network, ingress, storage, GPU: 15, plus the new add-ons | fortress |
| Data | Postgres (with its VMs), Garage, NFS: 3, plus Redis | miners |
| Operations | delivery, build, observability, the bridge: 12 | market town |
| Guard | identity, secrets, mesh: 6 | walled monastery |
| Fleet | core, provisioner, thot, hermes (services as parts): 4 | space base |
| Apps | products, sites, AI, tools: 17 | modern city |
| Code | infra-bootstrap and the game (A7 adds ukubi-game to `sources`), plus the multi-app repos per open question 5 | workshops |

- **Layout:** the map is laid out by **civ district**, and a district folds by group into 7 or fewer (plan/10's load
  rule).
- **Placement:** a hash of (civ, id), with overrides in a committed file shared by both worlds, so the sim and the
  real world agree. A building never moves, and a reclassification is an explicit move.

## 3. Mechanics
**The loop: what the player does.**
- **Actions:** set goals and steps, answer what needs you, Merge, send research. All of these exist today except
  research.
- **Lenses** (looked at, never acted on): civs, walls, watchtowers, supply lines, population, capacity, roads.
- **Every age input** has an action that moves it, or is only a gate.

**Each mechanic, its signal, and its sim mirror.** "New" means a slice below builds it; until then the look reads
**unknown**, never "fine".

| Mechanic | Real signal | Sim |
|---|---|---|
| Building health | ArgoCD health/sync, Ready, probes (contract today); add-ons: DaemonSet/Deployment ready (new) | mirrors today; add-on query new |
| Supply lines | curated `needs:` rows (new) | correlated failures along them (new) |
| Watchtowers | coverage (new extractor, below) | ladders respect coverage, real rule names (new) |
| Walls | forwardAuth middlewares, authentik blueprints, mesh (new extractor); `public: true` mark | static facts, from the same graph |
| Occupancy, population | per-namespace usage (contract today); per-workload and pod queries (new) | mirrors today; new queries mirrored |
| Research | ADR status in the graph (today) | the forge's ADRs (today) |
| Daily users | daily requests (new query); unique users (new counter, ADR) | the counter runs for real on a mirrored access-log stream (new mirror) |
| Latency | a duration histogram query (new) | seed base curves (new) |
| Stability | open question 3 | the player's response only (below) |

**Supply lines (owner: supply lines, goals lift dependents, outages spread).**
- **Dependencies** start as curated, tested `needs:` rows in `realms.yaml` (ingress, database, identity). Secrets
  carry the database URLs, so extraction comes later.
- **Roads:** git edges are supply roads from a workshop to what it feeds. Streets join a district's doors to its
  roads (layout only, no edge).
- **Goals lift dependents:** a goal closed on a utility puts a **credit mark** on its dependents (a look only). It
  counts **once** toward the age, so Traefik's 21 dependents don't pay it 21 times.
- **Outages spread honestly:** a building whose declared supplier is down is **at risk**, a look separate from
  **down**, which only its own signal shows.

**Defence (owner: watchtowers, walls, weapons).**
- **Coverage:** a building is covered by a rule whose selector or labels name its namespace or target, or by a probe
  that targets it. Sources:
  - gitops `PrometheusRule`s;
  - Traefik's `prometheusRule`;
  - `common-app-chart` `logAlerts:`;
  - blackbox targets.
- **kube-prometheus-stack's default rules** are live and generic (KubePodCrashLooping and the like). How they count
  is open question 6.
- **Watchtowers:** a covered building sees raiders at the warning rung. An uncovered one first sees them when the
  damage shows (Degraded, NotReady).
- **Walls come from Guard:** buildings behind authentik or the mesh get walls and gates. A `public: true` mark in
  `realms.yaml` keeps the blog and the wedding site from reading as unguarded.
- **Weapons fight back:** answering an incident sends a unit out, on a live session only (Units).

**Research is real research (owner).**
- An agent investigates a new way, architecture, technology or repo idea, sent out as a **scout** from the building
  where the question was born.
- **Sharing:** its result can be shared with similar buildings, and the reverse. Sharing is a PR that cites the
  result, since governs edges come from citations (plan/10).
- **Technologies** are the ADRs of **every** repo, folded by repo and status, not only infra-bootstrap's count (amends
  Civ R9). Their amends and supersedes links form the tree; "related" links are not prerequisites. This unparks 09's
  tech tree, without unlocks.
- **Done:** research is done when an ADR the research opened reaches Accepted, at merge or by a later flip.
  Rejected and Proposed don't count.
- **Scouts** need single-shot sessions that can ask (ADR-0070 item c, "not yet"). Until then, research runs as a
  builder's role.

**Resources: capacity, a real cost, nothing spent (owner).**
- **Capacity:** CPU, RAM and disk are the cluster's allocatable (A2's land lens).
  - A building occupies what it really uses, once the per-workload query exists.
  - Until then, a shared namespace (kube-system, monitoring, ente, wird, agent-fleet) shows its total on the
    namespace, not per building.
- **Houses:** all 5 schedulable nodes, the control planes included.
  - Population is bounded by CPU and RAM, which do bind; the pod cap never does.
  - Population is drawn only once the pod query and its mirror exist.
- **Data's occupancy is unknown, never zero:** Pigsty's metrics live in its own VictoriaMetrics.

**One age for the whole town, held (owner).**
- **It replaces 09's score and Civ R9's per-building levels.** The age's look takes over theirs, and Civ R9's
  "golden age" is renamed the **calm streak**.
- **Paid by:**
  - goals;
  - research done;
  - **daily users**;
  - **latency**;
  - **stability**.
- **What a goal pays:** 09's checkable signal is "a PR merged plus a check, or an SLO held for N days". A goal pays
  the age only when **all its steps merged and its named check passed**. Today ukubi-game closes a goal `done` when its
  steps are closed and one merged, so `goal.closed(done)` must tighten to this. Each goal counts once.
- **No staged age (owner):** the age waits until every input is measured, in `contract`, and mirrored by the sim.
  Users pay through unique users. Daily requests are shown, labelled, but never pay the age (open question 4 can
  change this).
- **It must be held:** losing its inputs for a whole hold window demotes the town.
  - **Ages only:** closed goals, technologies and wonders stay (amends 09's "levels never drop").
  - **No answer is no answer:** a missing or stale input pauses the hold clock and never demotes (R7.1).
- **Unique users (owner: measured both ways, labelled):**
  - **Counting:** a HyperLogLog of a daily-salted hash. The counter stores no IP. Traefik's access logs in Loki already
    hold client IPs for 7 days, and the infra-bootstrap ADR decides whether that changes.
  - **Exclusions:** probe, agent and owner traffic.
  - **Where it runs** is the ADR's call. If we own it, the sim runs it for real on a mirrored access-log stream.
- **Latency:** needs a duration histogram query in `contract`. Its action is a performance goal.
- **Stability:** what it counts is open question 3. Its action is fix goals.
- **One age per world:** the game log is per world.
  - **In the sim, every age input is director-independent.** The director paces incidents and never bends rewards
    (12):
    - stability is the per-incident response score, normalised by severity;
    - latency and users come from the seed's base curves, excluding incident effects.
  - **The test:** a scripted incident trace, replayed under each director setting's timers, gives the same age.
- **Pressure** keeps 09's look. Its score drain has no target now that the score is gone (open question 7).
- **Look:**
  - each civ restyles in its own way per age (materials, model chains, size tiers within its style);
  - a demotion shows at the town centre and on the banners, and buildings restyle only when a hold window ends;
  - ages, construction and wonders never use the health looks.

**Units: live sessions only (owner).**
- **One unit per live session of the game's client:** `MAX_EXTERNAL_SESSIONS`, which is 1 in production.
  - kind-local needs `MAX_DEFINITION_SESSIONS` ≥ 3 for 2: core requires the external cap below the definition pool.
  - Console, thot and other sessions are not units.
- **Unit types follow agent definitions:** a builder for `game-<repo>`, and later a scout for research and a guard
  for incidents. Until those exist, scout and guard are roles on a builder session.
- **Training:** a workshop shows "can train" only when its `game-<repo>` definition exists. Today that is only
  `game-ukubi-game`, in kind-local only.
- **The hero** is you, carrying your own steps (worker `me`), and the only named unit.
- **Named signed-in players** wait for a decision row that unparks 09's player groups (Play is parked, R10).
- **A crowd** only ever comes from a bucketed count of measured unique users, never per person. There is no crowd
  padding.

**Wonders:** a big multi-goal project (a new product, a platform migration), rising in stages and visible from the
whole map.

**Construction:** a new building rises through stages. A goal's work shows as scaffolding beside the standing
building, never by rebuilding a live app from dirt.

**Three visual channels that never mix:**
- **at risk:** a supplier is down;
- **pressure:** 09's own look, never smoke, fading, grey or rubble, and never spreading;
- **demotion:** the town centre and the banners.

## 4. Zoom with information levels
- **Map:** civ districts as banners and icons, the town centre, attacks, supply lines on demand.
- **District:** buildings in their age look, roads, units.
- **Building:**
  - its parts (fleet services, Postgres's VMs);
  - its technologies and research;
  - its goals and units at work;
  - its supply and defence.
- **LODs for performance** come on top, measured before they are built (R8 Engine).

## 5. Kits and the asset system
**Sources (owner):** CC0 kits, free tiers only, plus kitbashing and procedural generation. AI generation comes
later, as one more source feeding the same catalog and review.

**Free kits checked on GitHub:**
- KayKit's free tiers are CC0, each textured with one 1024² atlas of about 8×4 gradient swatches:
  - Medieval Hexagon: tiles, buildings in 4 colours, construction stages, scaffolding, walls, terrain;
  - Adventurers and Skeletons: rigged characters, with 76 and 95 embedded clips (their pages say "75" and "90+");
  - City Builder Bits, Space Base Bits, Halloween Bits, Dungeon Remastered.
- Kenney and Quaternius are CC0 according to search results, but unreachable from the build container. They come in
  as archives with their licence checked in.

**Repaint (owner: every kit into palettes).**
- Clay becomes one palette among several.
- A palette is a set of **gradient-ramp slots**: wall, roof, wood, stone, metal, foliage, water, banner, emissive,
  skin.
- Each kit's swatches map to the slots, with per-model overrides. The Hexagon, City and Space atlases share their
  swatch columns.

**Characters:**
- The KayKit characters share one 41-joint rig (checked).
- There is no "work" clip; `Interact` or `Use_Item` stands in.
- One shared file of about 5 clips replaces the clips embedded per model (3.6 and 4.8 MB).

**Render:**
- BatchedMesh per civ material (R8 Engine), never InstancedMesh per kind.
- 220 draws, 350k tris on desktop and 200k on phone-low.
- Buildings alone are about 200k tris on the fixture, so the map draws banners only, and dressed buildings show from
  district zoom.
- A gate case covers the heaviest civ, at its top age, on phone-low.
- Skinned characters are one draw each, and the small unit counts keep that cheap.

**Grid:**
- Hex terrain comes from the Hexagon tiles, repainted per biome.
- Square kits are scaled to fit a hex: City and Space are 2×2, and Halloween is about 3× the scale. There is a scale
  per kit and a footprint validator.
- Only `KHR_mesh_quantization` is used, since the CSP blocks WASM decoders.

**Home:**
- A **public `ukubi-kit` repo** (CC0, GitHub-hosted runners only), holding:
  - the catalog `{id, source, licence, civ, role, age, state, footprint, variants}`;
  - palettes, kitbash recipes and generators;
  - a Go package that embeds `dist/`.
- The game gains no second private dependency.
- Builds are pinned: Node, a lockfile, glTF-Transform 4.5.1, and a sha256 per fetched file.

## 6. What changes elsewhere
- **00-decisions, amended:**
  - **atlas R10's realm list** is reshaped into the civs, and repos and hostnames leave the rim;
  - **Audience R10** admits one read-only machine principal for the town centre;
  - **Civ R9:** its levels and score give way to the age, its golden age is renamed the calm streak, and its
    technologies cover every repo's ADRs.
- **09-play (parked), unparked and reversed:**
  - ages can fall (ages only), where 09 said levels never drop;
  - traffic counts, as unique users, where 09 said the score doesn't count traffic;
  - the tech tree is unparked, without unlocks;
  - goals per building, the checkable signal (quoted exactly) and the pressure look are taken up;
  - named players and watchers stay parked.
- **mountain (atlas):**
  - the realms reshaped into civs;
  - the `function` and `model:` fields;
  - the folds and `needs:` rows;
  - the extractors (slice 1);
  - the town centre's machine-credential route.
- **infra-bootstrap:** the unique-users ADR and the `atlas-game` service-account blueprint.
- **agent-fleet:**
  - production `game-<repo>` definitions and `clients/game.yaml`;
  - scout and guard definitions;
  - ADR-0070 item (c);
  - pool sizes.
- **ukubi-game:**
  - the sim mirrors every new signal;
  - correlated failures;
  - coverage-aware ladders with real rule names;
  - director-independent age inputs;
  - placement;
  - `goal.closed(done)` tightened;
  - its own admins-only gate before it leaves loopback.

## 7. Slices (each with its own plan and doubt cycle)
1. **A7, atlas data (mountain):**
   - the realms reshaped into civs, `function`, `model:`, the folds, `needs:` rows, `public: true`;
   - ukubi-game added to `sources`;
   - extractors, reading allowlisted fields only and keeping the never-draw list:
     - **kubespray** (`addons.yml`, `k8s-cluster.yml`, `k8s-net-cilium.yml`): Cilium, CoreDNS, nodelocaldns,
       kube-proxy, MetalLB, kube-vip;
     - **Pigsty** (`pigsty.yml`): `pg-proxmox` and `etcd` membership, joined to VMs by IP through
       `ansible/playbooks/vars/bnei-lan-records.yml`, plus `redis-main`. Allowlist: `pg_cluster`, host keys,
       `pg_seq`. Never `pg_role`: its labels are unreconciled. Never users or passwords;
     - **coverage** (rules and probes, as in §3 Defence);
     - **walls** (forwardAuth middlewares, authentik blueprints).
   - The `realms.yaml` namespaces include `metallb-system`.
2. **Signals:**
   - the infra-bootstrap ADR for unique users;
   - contract queries for daily requests, latency, add-on health, per-workload usage and pods, each with its sim
     mirror (the access-log stream for the counter);
   - correlated failures and real rule names in the sim;
   - **stability's query waits for open question 3.**
3. **ukubi-kit plus a real-data diorama:**
   - the fixture graph with 3 contrasting civs (fortress, space base, modern city) dressed at district zoom, the rest
     in clay, and the map with banners;
   - 2 ages and one construction state;
   - measured on both tiers with the existing gate harness.

   This is the owner's sign-off artifact.
4. **The live town:**
   - placement, districts, zoom levels;
   - supply lines, defence;
   - units, construction, wonders;
   - the game client's admins-only gate before it leaves loopback.
5. **The age**, once every signal from slice 2 is live and `goal.closed(done)` is tightened.

## Open questions (owner)
1. The age thresholds, and how long each must be held.
2. Each civ's style and look.
3. What "stability" counts exactly.
4. Users: unique users only (the draft's default), or daily requests as a stated exception to "no staged age".
5. Repos that feed two apps (agent-fleet, vos-monolith, wird): an annex on one of them, or a workshop in Code
   (default: a workshop in Code).
6. kube-prometheus-stack's generic default rules: do they cover every k8s building, or nothing? (default: they make
   the warning rung visible on every k8s building, but don't count as coverage of a building's own risks)
7. Pressure's drain, now that the score is gone: does waiting slow the hold clock, or have no effect? (default: no
   effect, look only)
8. How a research result is shared with similar buildings, beyond a citing PR.

## Verification
- **This draft:** three doubt cycles. The third checked that every mechanic names its signal and its sim mirror (the
  table in §3), and that every 00 row it amends is listed in §6.
- **Each slice:** its own checks, set in its plan.

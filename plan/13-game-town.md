# 13 · R12: the game's town: buildings, civs, ages, kits (design draft, 2026-10-25)

> Status: **draft**. It comes from four owner interview rounds and three doubt cycles (19, 20 and 16 findings). The
> owner's answers below are decisions; the open questions at the end are not: the owner settles them by in-game
> tests (2026-10-25), so each default below ships tunable. Nothing is built, and each slice gets
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
- **Hosts and the town centre sit in a civ for atlas** (hosts in Platform with the function `land`, atlas in
  Operations with `town-centre`), and the game treats both by function: land is a lens, never a building, and the
  town centre stands at the map's centre, outside every district. What is never drawn carries over: R8's list (k9s)
  and the Audience rows.

**One taxonomy, owned by atlas (owner: civs = realms).**
- atlas's realms are **reshaped** into the civs (§2): merged, split, and one added. atlas's islands and the game's
  districts then group things the same way, and repos leave atlas's rim for code. Hostnames stay at its Cloudflare
  rim, where outside traffic enters; the game draws them as doors.
- **`function` is its own field** in `realms.yaml`, set per group and overridable per element. It says what a building
  does in play, which is separate from its civ: blackbox is in the Operations civ and works as a watchtower.
- **A building's model is derived, never pinned** (owner, 2026-10-26: "a system, so we can derive instead of
  pinning"; this replaces a `model:` field in `realms.yaml`). The game reads a building's static facts from atlas's graph
  (its doors, its parts, what depends on it) and the kit tags its variants with generic traits; the facts choose among
  the eligible variants. ukubi-kit holds no component names. Its design (A7d2) has its own plan and doubt cycle.
- An element with no civ or function is drift. Hostnames and ADRs are never placed.

**Functions** (AoE-like, not the same):

| Function | Meaning | Examples |
|---|---|---|
| Land | the hardware: a lens, never a building | the 3 hosts |
| Town centre | the information hub: atlas and the game as one (below) | one building at the centre |
| Houses | the schedulable nodes, which hold the population (§3 Resources) | all 5 k8s nodes |
| Workshops | repos, where a unit (`game-<repo>`) works on its code (set by atlas, never in realms.yaml) | infra-bootstrap, the game, annexes |
| Barracks | the fleet, which runs the units (agent sessions) | core, provisioner, thot, hermes |
| Economy | data stores | Postgres, Redis, Garage, NFS |
| Utilities | network, ingress, storage | Cilium, CoreDNS, MetalLB, Traefik, Longhorn |
| Defence | identity, secrets, mesh, probes | authentik, Infisical, agent-vault, NetBird, blackbox |
| Trade | build, ship, observe | Argo CD, zot, runners, Prometheus, Grafana, Loki |
| Products | what people use | the apps and sites |
| Gates | where outside traffic enters; the game's, from route edges (atlas leaves hostnames unplaced) | hostnames, as doors |

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
| Platform | the hosts (land), nodes, ingress, storage, GPU, metrics-server: 15, plus the new add-ons | fortress |
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
- **Walls come from Guard** (owner, 2026-10-26), on the **doors** (a hostname routing to an app), summed on the app:
  - a **wall** is forwardAuth on the whole host: nothing reaches the app before authentik;
  - a **gate** is an OIDC login through authentik, or forwardAuth on part of the host (`forwardauth-path`);
  - two marks in `realms.yaml` `walls:` rows: **public** (meant for everyone: the blog, the wedding site, ADR-0039's
    Public tier) and **own** (the app authenticates its own callers: Ente, the bridge's signed webhook);
  - a door with none of them reads **open**: a finding, shown, not drift;
  - a machine enrolled in NetBird is a **mesh peer**; it becomes a mesh wall at ADR-0055's cutover, when its sshd
    stops answering on the LAN.
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
- **One unit per live session of the game's client:** `MAX_EXTERNAL_SESSIONS`, which is 1 in production. Live means a
  turn in flight (core's loading, working, waiting): core's `idle` is both "no pod" and "its turn ended", so an idle
  session is asleep, no unit, and the building's card says so (4d2's reading, 2026-10-26).
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
whole map. Built (4d3): on one of the town centre's six ring hexes, its slot recorded; a stage per goal closed done,
never going down; standing as the kit's monument once the player finishes it (two goals done at least); its label is
what makes it visible from the whole map.

**Construction:** a new building rises through stages. A goal's work shows as scaffolding beside the standing
building, never by rebuilding a live app from dirt. Built (4d3): new means after the town's founding (the game's first
whole graph, in its log); a newcomer rises until atlas first calls it ok (recorded built). The stage is its own channel,
the model: health stays atlas's (down lies in grey foundations, degraded rises smoking), so atlas's page and the town
agree, and the town alone adds "not built yet".

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
- ~~Buildings alone are about 200k tris on the fixture, so the map draws banners only.~~ Measured in slice 3: the map
  draws dressed buildings within budget (the age-2 overview: 178k tris on desktop, 135k on phone-low, 23 draws), so
  banners are not needed yet. Shadows are on at district zoom only (all on, the overview was 374k); the heaviest
  district view with shadows is 244k.
- A gate case covers the heaviest civ, at its top age, on phone-low.
- Skinned characters are one draw each, and the small unit counts keep that cheap.

**Grid:**
- Hex terrain comes from the Hexagon tiles, repainted per biome; every district keeps them (owner, 2026-10-26).
- **One kit per civ** (owner, 2026-10-26): a civ's buildings, plot and dressing come from its own kit, and the kit's
  build refuses anything else. A civ's biome is its tiles' ground colour, a floor of its kit under each building (the
  square kits' floors cannot tile a hex) and its kit's props on the free land. The construction scaffold is the one
  piece still shared by every civ, painted in each civ's style.
- Square kits are scaled to fit a hex (City 0.7, Space 0.66, Halloween, Dungeon and Prototype 0.32 on their 4-unit
  grid). There is a scale and a turn count per kit, and a footprint validator: per-vertex hex reach, ≤ 0.9 for a
  building on a plot, ≤ 1.0 for a plot, a 4500-triangle cap per building.
- Only `KHR_mesh_quantization` is used, since the CSP blocks WASM decoders.

**Home:**
- A **public `ukubi-kit` repo** (CC0, GitHub-hosted runners only), holding:
  - the catalog `{id, source, licence, civ, role, age, state, footprint, variants}`;
  - palettes, kitbash recipes and generators: **procgen**, a reusable library for kits we make (primitives,
    composition, palette and atlas, seeded randomness, tests), with a how-to in the kit's `docs/procedural.md`;
  - a Go package that embeds `dist/`.
- The game gains no second private dependency.
- Builds are pinned: Node, a lockfile, glTF-Transform 4.5.1, and a sha256 per fetched file.

## 6. What changes elsewhere
- **00-decisions, amended:**
  - **atlas R10's realm list** is reshaped into the civs, repos leave the rim for code, and hostnames stay at its
    Cloudflare rim (doors in the game);
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
  - the `function` field (models are derived, §1);
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
1. **A7, atlas data (mountain)**, in three PRs:
   - **A7a (built):** the realms reshaped into civs (`realms.yaml`, the page, the fixtures); `function` per group with
     per-component overrides; the repos placed in code by atlas, grouped by the realm of the apps they feed
     (`shared` when several, `own` when none). metrics-server moved to Platform (it shares kube-system with the
     add-ons). Hostnames stay at atlas's Cloudflare rim, where outside traffic enters; the game draws them as doors
     from the route edges;
   - **A7b (built):** two new node kinds, `addon` and `db`. The kubespray extractor reads named group_vars keys only
     (no address, pool or extra values; literals only: a template is drift, never a failed projection; kubespray's own
     defaults where a key is absent) and draws Cilium, kube-proxy (unless Cilium replaces it), CoreDNS,
     nodelocaldns, MetalLB and kube-vip in Platform's network group. The Pigsty extractor draws `db:pg-proxmox` and
     `db:redis-main` in Data, with runs-on edges to their VMs (joined by IP through the LAN records; the etcd members,
     the witness among them, as Postgres's DCS); it reads host keys and cluster names only, never `pg_role`, users or
     passwords. pg01, pg02 and the witness are parts of `db:pg-proxmox`;
   - **A7c:** coverage and `needs:` rows moved into slice 2 (owner, 2026-10-26: "the same topic": they are the
     static half of its real rule names and correlated failures);
   - **A7d (before slice 4):**
     - **A7d1 (built):** walls on the doors, read from the sources (a forwardAuth middleware on a values file's host or
       on an IngressRoute route, a `forward_single` provider's host, an OIDC redirect host), `walls:` rows for the
       marks and the gates atlas can't read, `mesh: peer` from the NetBird inventory's keys; the fleet's session-pod
       containers are the provisioner's parts and the executor thot's (Fleet draws 4 buildings). infra-bootstrap #375
       moved pgweb and Alertmanager behind their own forwardAuth gates (owner: "move all old forward Auth to the new
       one"). Gaps: doors come only from ApplicationSet hostnames, so bootstrap and redirector routes (Alertmanager,
       ArgoCD, Grafana, hermes, Proxmox) have no door yet; ukubi-stt and dev.api.voconsteroid.com read open;
     - **A7d2 (owner, 2026-10-26: shapes, mapped privately):** ukubi-kit tags variants with **shapes** (what a model
       looks like: tower, gate, yard, vault, mill, bay, rig, pit, wide) and draws a building among the variants with a
       shape it asks for, else the untagged ones, else all (`PickFor`). atlas maps each group to the shapes its
       buildings ask for (`realms.yaml` `shapes:`, ours, never the public kit's) as the `shapes` attribute, and marks a
       **landmark** (5 reviewed dependents, or 3 parts: Postgres, authentik, Infisical, the provisioner), which the
       world draws larger, never a model of its own. A model changes only when a building's group or size class does.

   Its full scope:
   - the realms reshaped into civs, `function`, the folds, `needs:` rows, `walls:` rows;
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
2. **Signals** (owner, 2026-10-26: requests only for now; stability is both calm days and per-app availability;
   `needs:` curated and checked; coverage only from rules that name a building), in four PRs:
   - **S2a1 (mountain, built: #36):** `contract.Signals`, optional queries on their own cadence: an unknown answer is absent,
     never 0, and never fails the poll; a healthy state is an explicit 0 (from the request counter, from
     `argocd_app_info`, and from the always-firing Watchdog, without which the day is unknown, not calm). Per app:
     5xx/s, p95 latency, the last 24 h of requests and the share answered without error; per probed node its
     availability; pods per k8s node; the town's day (critical alerts that fired, apps that were Degraded, calm when
     neither). The day windows are rolling 24 h, asked every 5 min. Shown on the map panel and the building cards;
   - **S2a2 (mountain, built):** curated `watch:` rows (node → rule names, Prometheus or Grafana), checked against the rule
     names the sources declare, stored as node attributes (no rule nodes); curated `needs:` rows with the evidence
     that proves each (forwardAuth refs, OIDC redirect hosts, the Infisical block, a Pigsty database by name only,
     Garage S3), drift both ways, `declared` for evidence outside the sources; add-ons' workload lists and their
     readiness (kube-vip, a static pod, stays unknown);
   - **S2i (infra-bootstrap, built: #374):** Traefik's request-duration buckets (the defaults 0.1/0.3/1.2/5 s make p95 an
     interpolation);
   - **S2b (ukubi-game, with small mountain PRs for the default rules at the chart's pinned version):**
     - **S2b1 (built: ukubi-game #7):** the mirrors of every signal. A day window integrates the full 24 h of the pure
       world (a restart answers the same day), and a Prometheus gap is skipped where the real `*_over_time` would.
     - **S2b2 (built: mountain #39, ukubi-game #8, #9):** real rule names with their real severities, `for:` and label shapes only; correlated failures along
       `needs:` (a supplier down raises its dependents' errors and latency, never marks them Degraded), derived purely,
       never logged. mountain carries each default rule's severity and `for:` (`contract.DefaultRules`) and each
       watched rule's (`watch-rules`), and places a watched rule's alert on its one watchtower.
     - **Needs-you (owner, 2026-10-26):** every critical alert, plus a warning from a watched rule; in the sim, its own
       incidents stay needs too (the director's stakes, answered by the player), and the alerts an incident causes are
       linked to it through the sim's API (no invented label), so nothing counts twice.
   - Limits recorded: Grafana-managed rules never reach Alertmanager or `ALERTS`, so a calm day counts the
     Prometheus-evaluated critical alerts only (a Grafana rule still makes a watchtower: it notifies); with real
     rules, most incidents reach warning only, so the game's needs-you (critical only) was revisited in S2b2 (above); the sim's
     latency moves with incidents, so the age (slice 5) must not read it as stability there.
   - **Unique users and their infra-bootstrap ADR are deferred**; the age waits on them (no staged age).
3. **ukubi-kit plus a real-data diorama (built, 2026-10-26):** ukubi-kit #1 and ukubi-game #6.
   - Seven civs in two ages, one kit each:

     | Civ | Kit |
     |---|---|
     | Platform | Medieval Hexagon |
     | Data | Dungeon |
     | Operations | Prototype |
     | Guard | Halloween |
     | Fleet | Space Base |
     | Apps | City |
     | Code | `ukubi`, made by procgen |

     The town centre is the Hexagon keep, the town's own.
   - The diorama draws atlas's fixture graph (78 buildings, 7 districts) at `/town` in ukubi-game, with:
     - each civ's plots and dressing;
     - the scaffold frame for a goal's work in progress;
     - the age as a preview.
   - The gate measures the map and every district on both tiers.
   - Open:
     - Fleet drew 7 buildings, not 4, while atlas left the sidecar, worker and executor unparented (fixed in A7d1);
     - the town page needs a CSP before slice 4 takes it off loopback.
4. **The live town**, in five PRs:
   - **4a, the town lives:** the live frame's shape moves into `contract` (`contract.Live`), and atlas classifies each
     node's health server-side (`contract.Classify`, the rules its page had: ArgoCD Missing is down, a failed probe or
     NotReady is down, a host degraded when some of its k8s nodes aren't Ready), sending `health` in the frame, which
     the page and the game's town both read. New in the classifier, so atlas shows them too: Postgres by a three-state
     majority of its members (`contract.Majority`, which the sim's supplier rule also uses) and add-ons degraded only
     after 2 minutes below all ready (a rollout is not damage). The frame gains `alertsAt`: before Alertmanager's first
     answer, no alerts is no answer. The town draws the frame's states with the world's looks (down grey, degraded
     smoke, a critical alert firing on it fire), unknown as is (R7.1) with a count, at risk (a `needs` supplier down,
     one hop; a supplier unknown is risk unknown) as a ring on the plot, and raiders by the Watchtowers rule exactly
     (the watched rule at the warning rung, others once the damage shows, its own or a part's). A Grafana-only watch
     (Postgres's) never reaches Alertmanager: that watchtower is blind until a Prometheus rule watches it. Built:
     mountain #42, ukubi-game #11. Open (owner, 2026-10-26: decided once seen): fire follows the Watchtowers rule (a
     critical alert on an unwatched, undamaged building lights nothing and is counted unseen), or fire is a health look
     lit by any critical alert placed on the building;
   - **4b, placement pinned:** §2's rule as written: `town/placement.json` in ukubi-game (embedded, shared by both
     worlds, never written by the game) pins each district's centre and each building's hex; a building it doesn't pin
     stands where its (civ, id) hashes among the free hexes of its district and moves nobody, a reclassified one is
     placed in its new district (its old hex kept for it), a departed one keeps its hex, a new civ lies outside every
     district; the town says what isn't pinned, and `cmd/townpin` pins it for review in a PR (`--prune` drops the
     departed; a partial graph pins nothing);
   - **4c, lenses (built: ukubi-game #13):** one at a time, in colours of their own (at risk, damage and pressure keep
     their channels): supply lines by what they are needed for (dashed where only declared), roads from each workshop
     (a workshop feeding 8 or more is one trunk per district), walls and doors (a post per door by its kind, mesh
     peers apart), watchtowers (blind where only Grafana watches, a post where a probe has no rule), and atlas's own
     land lens. Gaps: doors are read from ApplicationSet hostnames only, and Postgres's tower stays blind;
   - **4d, zoom, units, construction,** in three PRs:
     - **4d1, zoom levels and the building card (built: ukubi-game #14):** the level is read from how much ground the
       camera sees (map, district, building); the map's banners carry each district's live marks; shadows follow the
       level. A building is picked by a click or `?b=`; its card (`/api/town/card`) shows its live state with the
       alerts it doesn't see listed apart, atlas's signals, parts, supply both ways, roads, doors, watch and
       technologies (the ADRs its files cite; a workshop's, what its code cites and the ADRs written in it), and the
       game's half: goals and units at work (each session as core reports it), the needs-you on it (an alert placed
       by its fingerprint), and "can train". The town centre's card adds its interior: the catalog, goals and needs by
       building, the game's own workshop (`repo:ukubi-game`, not in atlas's graph yet), the age and research (none
       yet). Read only: commands stay in the panels;
     - **4d2, units (built: ukubi-kit #3, ukubi-game #15):** KayKit's Adventurers in ukubi-kit, one skinned draw each
       (hat and cape baked into the skin, nothing held), one clip file on their shared rig. A builder per agent step's
       session with a turn in flight (arriving, at work, needing you), unknown when its status can't be read; the hero
       at your most recently started open step's building, else at the town centre saying so; units walk when they
       move, are labelled in words, and hide on the map;
     - **4d3, construction and wonders (built: ukubi-kit #4, ukubi-game #PR4D3):** the founding and what is built
       since in the game log (recorded by the game's loop, never a GET; `--refound` for a bad founding); newcomers in
       foundations or rising with atlas's health on them; the sim mirrors a new app's arrival (`app.new`: Missing,
       then Progressing, then Healthy); wonders on the centre's ring, opened, finished and dropped in the panels, the
       kit's monument when standing; `goal.closed(done)` records its rule, for slice 5's tightening;
   - **4e, off loopback:** the town page's CSP, and the game client's admins-only gate with its machine credential for
     atlas (`atlas-game`); until then the real world runs a local `atlas --dev` against port-forwarded sources.
5. **The age**, once every signal from slice 2 is live and `goal.closed(done)` is tightened.

## Open questions (owner)
1. The age thresholds, and how long each must be held.
2. Each civ's style and look.
3. ~~What "stability" counts exactly.~~ Answered 2026-10-26: both, the town's calm days (no critical alert fired, no
   app Degraded, over the rolling 24 h) and each app's availability (requests answered without a 5xx, probes up).
4. Users: unique users only (the draft's default), or daily requests as a stated exception to "no staged age".
   2026-10-26: requests only for now; unique users and their ADR come later, and the age waits for them.
5. Repos that feed two apps (agent-fleet, vos-monolith, wird): an annex on one of them, or a workshop in Code
   (default: a workshop in Code).
6. ~~kube-prometheus-stack's generic default rules: do they cover every k8s building, or nothing?~~ Answered
   2026-10-26: only a rule that names a building is its watchtower; the defaults are the town's general watch. A
   probe alone (the guest probes have no rule) is not coverage either.
7. Pressure's drain, now that the score is gone: does waiting slow the hold clock, or have no effect? (default: no
   effect, look only)
8. How a research result is shared with similar buildings, beyond a citing PR.

## Verification
- **This draft:** three doubt cycles. The third checked that every mechanic names its signal and its sim mirror (the
  table in §3), and that every 00 row it amends is listed in §6.
- **Each slice:** its own checks, set in its plan.

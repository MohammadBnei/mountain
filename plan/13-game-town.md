# 13 · R12: the game's town: buildings, civs, ages, kits (design draft, 2026-10-25)

> Status: **draft**. It comes from four owner interview rounds and two doubt cycles (19 + 20 findings), and the
> owner's answers below are decisions. Nothing is built, and each slice gets its own plan and doubt cycle when it
> starts. It extends `12-world-and-game.md` (the game) and amends rows of `09-play.md` (listed under "What changes
> elsewhere").

## Context
The game (ukubi-game, R11) works end to end: goals, steps, agents, Merge, the director. Its client, however, is a
flat panel page, and the clay town was deleted in R10. The owner wants the visual pass, rebuilt around **Age of
Empires** dynamics: a town whose buildings change with its age, villagers, research, utilities and defences. The
binding rule still holds: every mechanic is tied to a real signal, and the sim mirrors every signal the game uses.

## 1. What is a building
**Everything is a building except the hardware (owner).**
- **The land** is the 3 physical hosts. It shows as a **land lens** that recolours the ground by which host carries
  what (plan/10's lens; nothing moves). The land is not the map's layout.
- **Buildings** are every other element: apps, fleet services, VMs, containers, repos and hostnames. Two kinds of
  element atlas doesn't extract yet become buildings too: kubespray's add-ons (Cilium, CoreDNS, MetalLB, kube-proxy)
  and Pigsty's Postgres cluster.
- **How elements fold onto buildings** (doubt 2), so the count stays readable:
  - a hostname is a **door** on the app it routes to;
  - a repo that feeds one app is a **workshop annex** on that app. 13 of the 14 repos do;
  - Pigsty's VMs are parts of the Postgres building, and the fleet services are parts of their app;
  - ADRs are **technologies**, not buildings (§3 Research).
- **What is never drawn** carries over: R8's list (k9s is never drawn) and the Audience rows.

**One taxonomy, owned by atlas (owner: civs = realms, renamed).**
- atlas's realms are reshaped into the civs (§2), so atlas's islands and the game's districts group things the same
  way.
- **`function` is its own field** in `realms.yaml`, set per group and overridable per element. It says what a building
  does in play (table below), and is separate from which civ it belongs to: blackbox is in the Operations civ and works
  as a watchtower.
- An element with no civ or function is drift, as an element with no realm is today.

**Functions** (AoE-like, not the same):

| Function | Meaning | Examples |
|---|---|---|
| Town centre | the information hub: atlas and the game as one (§1 below) | one building at the centre |
| Houses | the schedulable nodes; they hold the population (§3 Resources) | all 5 k8s nodes |
| Workshops | repos, where a unit (`game-<repo>`) is trained and works | infra-bootstrap, agent-fleet, annexes |
| Economy | data stores | Postgres, Garage, NFS |
| Utilities | network, ingress, storage | Cilium, CoreDNS, MetalLB, Traefik, Longhorn |
| Defence | identity, secrets, mesh, probes, alert rules | authentik, Infisical, agent-vault, NetBird, blackbox |
| Trade | build, ship, observe | Argo CD, zot, runners, Prometheus, Grafana, Loki |
| Products | what people use | the apps and sites |
| Gates | where outside traffic enters | hostnames, as doors |

**The town centre is the information hub: atlas and the game as one (owner).**
- On the map it is one central building. Its interior holds the catalog, goals, needs-you, the age and research.
- **Built as** a view in the game client that reads atlas through a **server-side machine credential**, never through
  the page. That is a new, tested route through atlas's admin-only gate (Audience R10).
- Commands stay in the game backend, and atlas keeps reading only (plan/10). `app:mountain` counts as the town centre,
  not as an Operations building.

## 2. Civs
**A civ is a group of elements of the same category (owner), one per realm once the realms are renamed.** Each civ
has its own kits, palette, biome and **style**. "Era" keeps 09's meaning of progress. **Mixed styles are intended**: a
fortress can stand next to a space base and a modern city.

| Civ | Holds (fixture counts) | Style (draft) |
|---|---|---|
| Platform | control plane and nodes, network, ingress, storage, GPU: about 15 | fortress |
| Data | Postgres (with its 3 VMs), Garage, NFS: about 4 | miners |
| Operations | delivery, build, observability, the bridge: about 12 | market town |
| Guard | identity, secrets, mesh: about 6 | walled monastery |
| Fleet | core, provisioner, thot, hermes (services as parts): about 5 | space base |
| Apps | products, sites, AI, tools: about 16 | modern city |
| Code | infra-bootstrap, repos with no app, ukubi-game | workshops |

- **Layout:** the map is laid out by **civ district**, and a district folds by group into 7 or fewer (plan/10's load
  rule).
- **Placement:** a hash of (civ, id), with overrides in a committed file shared by both worlds, so the sim and the
  real world agree. A building never moves, and a reclassification is an explicit move.

## 3. Mechanics
**The loop: what the player does.**
- **Actions:** set goals and steps, answer what needs you, Merge, send research. All of these exist today except
  research.
- **Lenses** (looked at, never acted on): civs, walls, watchtowers, supply lines, population, capacity, roads.
- **Every age input** has an action that moves it, or is only a gate (§3 Age).

**Supply lines (owner: supply lines, goals lift dependents, outages spread).**
- **Dependencies** start as curated, tested `needs:` rows in `realms.yaml` (ingress, database, identity). The graph
  has no such edges today, and secrets carry database URLs, so extraction comes later.
- **Roads:** git edges are supply roads from a workshop to what it feeds, plus streets between doors.
- **Goals lift dependents:** a goal closed on a utility marks its dependents as credited. It counts **once** toward the
  age, so Traefik's 21 dependents don't pay it 21 times.
- **Outages spread honestly:** a building whose declared supplier is down is **at risk**, a look separate from
  **down**, which only its own signal shows. The sim correlates failures along the same edges.

**Defence (owner: watchtowers, walls, weapons).**
- **Watchtowers:** a building is covered when a probe or alert rule names it, read from what gitops declares.
  - A covered building sees raiders at the warning rung.
  - An uncovered one first sees them when the damage shows (Degraded, NotReady): with no rule, nothing fires.
  - The sim's ladders respect coverage and use the real rule names.
- **Walls come from Guard:** buildings behind authentik or the mesh get walls and gates. A **public by design** mark
  keeps the blog and the wedding site from reading as unguarded.
- **Weapons fight back:** answering an incident sends a unit out, on a live session only (§3 Units).

**Research is real research (owner).**
- An agent investigates a new way, architecture, technology or repo idea, sent out as a **scout** from the building
  where the question was born.
- Its result can be **shared** with similar buildings, and the reverse. Sharing is a PR that cites the result, since
  governs edges come from citations (plan/10).
- **Technologies** are the ADRs, folded by repo and status. Their amends and supersedes links form the tree; "related"
  links are not prerequisites.
- Research is **done** when its ADR merges as Accepted.
- Scouts need single-shot sessions that can ask (ADR-0070 item c, "not yet"). Until then, research runs as a
  builder's role.

**Resources: capacity, a real cost, nothing spent (owner).**
- CPU, RAM and disk are the cluster's allocatable (A2's land lens). A building occupies what it really uses.
- **The houses** are all 5 schedulable nodes, the control planes included.
  - Population is bounded by CPU and RAM, which do bind; the pod cap never does.
  - Pods are drawn only once a pod query and its sim mirror exist.
- **Data's occupancy is unknown, never zero:** Pigsty's metrics live in its own VictoriaMetrics, not in the cluster's
  Prometheus.

**One age for the whole town, held (owner).**
- **Paid by:**
  - goals that meet 09's checkable signal (all steps merged plus a check), each counted once;
  - research done;
  - **daily users** on apps;
  - **latency**;
  - **stability**.
- **No staged age (owner):** the age waits until every input is measured, in `contract`, and mirrored by the sim.
- **It must be held:** losing its inputs for a whole hold window demotes the town.
  - **Ages only:** closed goals, technologies and wonders stay.
  - **No answer is no answer:** a missing or stale input pauses the hold clock and never demotes (R7.1).
- **Users are measured both ways, labelled (owner):**
  - Traefik requests per day, as a proxy;
  - **unique users**, counted through a HyperLogLog of a daily-salted hash. No IP is ever stored, and this needs an
    infra-bootstrap ADR. Probe, agent and owner traffic are excluded.

  Each measure has its own threshold, so switching from one to the other never demotes the town. Users are a gate
  only: no action moves them.
- **Latency** needs a duration histogram query in `contract`. Its action is a performance goal.
- **Stability:** what it counts is open (below). Its action is fix goals.
- **One age per world:** the game log is per world.
  - **In the sim, stability counts only your response** (time to answer and time to fix an incident), never whether
    one happened. The director paces incidents and never bends rewards (12).
  - A test checks that the same player actions give the same age at every director setting.
- **Look:**
  - each civ restyles in its own way per age (materials, model chains, size tiers within its style);
  - a demotion shows at the town centre and on the banners, and buildings restyle only when a hold window ends;
  - ages, construction and wonders never use the health looks.

**Units: live sessions only (owner).**
- One unit per real agent session: 1 in production (`MAX_EXTERNAL_SESSIONS=1`) and 2 in kind-local.
- **Unit types** follow agent definitions: a builder for `game-<repo>`, and later a scout for research and a guard for
  incidents. Until those definitions exist, scout and guard are roles on a builder session.
- A workshop shows "can train" only when its `game-<repo>` definition exists (today: `game-ukubi-game`).
- **The hero** is you, carrying your own steps (worker `me`), and the only named unit.
- **Named signed-in players** wait for a decision row that unparks 09's player groups (Play is parked, R10).
- **A crowd** only ever comes from a bucketed count of measured unique users, never per person. There is no crowd
  padding.

**Wonders:** a big multi-goal project (a new product, a platform migration), rising in stages and visible from the
whole map.

**Construction:** a new building rises through stages. A goal's work shows as scaffolding beside the standing
building, never by rebuilding a live app from dirt.

**Three visual channels that never mix:** at risk (a supplier is down), pressure (09's own look: never smoke, fading,
grey or rubble, and never spreading), and demotion (the town centre and banners).

## 4. Zoom with information levels
- **Map:** civ districts as banners and icons, the town centre, attacks, supply lines on demand.
- **District:** buildings in their age look, roads, units.
- **Building:** its parts (fleet services, Postgres's VMs), its technologies and research, its goals and units at
  work, its supply and defence.
- **LODs for performance** come on top, measured before they are built (R8 Engine).

## 5. Kits and the asset system
**Sources (owner):** CC0 kits, free tiers only, plus kitbashing and procedural generation. AI generation comes
later, as one more source feeding the same catalog and review.

**Free kits checked on GitHub:**
- KayKit's free tiers, CC0, each textured with one 1024² atlas of about 8×4 gradient swatches:
  - Medieval Hexagon: tiles, buildings in 4 colours, construction stages, scaffolding, walls, terrain;
  - Adventurers and Skeletons: rigged characters, 75 and 90+ animations;
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
- There is no "work" clip, so `Interact` or `Use_Item` stands in.
- One shared file of about 5 clips, instead of the 76–95 embedded per model (3.6 and 4.8 MB).

**Render:**
- BatchedMesh per civ material (R8 Engine), never InstancedMesh per kind.
- 220 draws, and 350k tris on desktop / 200k on phone-low.
- Buildings alone are about 200k tris on the fixture, so the map draws banners only, and a gate case covers the
  heaviest civ at its top age on phone-low.
- Skinned characters are one draw each, which the small unit counts keep cheap.

**Grid:**
- Hex terrain from the Hexagon tiles, repainted per biome.
- Square kits are scaled to fit a hex (City and Space are 2×2; Halloween is about 3× the scale), with a scale per kit
  and a footprint validator.
- Only `KHR_mesh_quantization` is used: atlas's CSP blocks WASM decoders.

**Home:**
- A **public `ukubi-kit` repo** (CC0, GitHub-hosted runners only), holding:
  - the catalog `{id, source, licence, civ, role, age, state, footprint, variants}`;
  - palettes, kitbash recipes and generators;
  - a Go package that embeds `dist/`.
- The game gains no second private dependency.
- Builds are pinned: Node, a lockfile, glTF-Transform 4.5.1, a sha256 per fetched file.
- It holds no component names or mappings; those stay in atlas.

## 6. What changes elsewhere
- **09-play (parked), unparked and reversed:**
  - **Ages can fall:** 09 said levels never drop. This applies to ages only.
  - **Traffic counts toward the age:** 09 said the score doesn't count traffic. It counts as labelled users.
  - Goals per building, 09's checkable signal, and its pressure look are taken up as written.
  - Named players and watchers stay parked.
- **mountain (atlas):** the realms renamed into civs, the `function` field, the folds (doors, annexes), `needs:` rows,
  extractors (below), and the town centre's machine-credential route.
- **infra-bootstrap:** the unique-users ADR.
- **ukubi-game:** the sim mirrors every new signal, correlated failures, coverage-aware ladders with real rule names,
  and placement.

## 7. Slices (each with its own plan and doubt cycle)
1. **A7, atlas data (mountain):**
   - the realms renamed into civs, `function`, the folds, `needs:` rows;
   - extractors for the kubespray add-ons (`addons.yml`, `k8s-cluster.yml`) and Pigsty's cluster (`pg-proxmox` in
     `pigsty.yml`).

   The leader is never drawn from pigsty.yml: its primary and replica labels are known to be wrong. The never-draw
   list holds, and the `realms.yaml` namespaces include `metallb-system`.
2. **Signals:**
   - the infra-bootstrap ADR for unique users;
   - contract queries for daily requests, latency and stability, and their sim mirrors;
   - correlated failures and real rule names in the sim.
3. **ukubi-kit plus a real-data diorama:**
   - the fixture graph at map zoom: 3 contrasting civs (fortress, space base, modern city) dressed, the rest in clay;
   - 2 ages and one construction state;
   - measured on both tiers with the existing gate harness.

   It is the owner's sign-off artifact.
4. **The live town:** placement, districts, zoom levels, supply lines, defence, units, construction, wonders.
5. **The age**, once every signal from slice 2 is live.

## Open questions (owner)
- The age thresholds, and how long each must be held.
- Each civ's style and look.
- What "stability" counts exactly.
- How a research result is shared with similar buildings.

## Verification
- **This draft:** a third doubt cycle checks two things:
  - every mechanic names its signal and its sim mirror;
  - there is no contradiction with a 00 row the draft doesn't explicitly amend.
- **Each slice:** its own checks, set in its plan.

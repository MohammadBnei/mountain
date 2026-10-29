# 17 · R16: the world as a disc: one world from orbit to a building's rooms (design, 2026-10-29)

> Status: **draft**. Three owner rounds (ideation, interview, architecture) and one doubt cycle (12 findings, all
> folded in). The living copy, with the map of the disc, is the owner's manifest doc "The Mountain — World Manifest".
> It replaces the floating islands of `13-game-town.md` and the realm-as-civ grouping there; it keeps 12's sim, goals
> and fleet wiring as they are. It amends 00's Audience row (R10: admins only) with a public, read-only audience and a
> `public` route role, which 14's every-route test gains. 14's island targets map onto buildings (top) and turtles and
> elephants (underside); 13's kits re-key from civ to building kind and type. Nothing is built; each slice gets its own plan and doubt cycle when it starts.

## North star
The ukubi platform is a world you live in while you work: a disc you tend from orbit and understand on foot, where
every place, road and change is real.
- **Work is the game.** Only real dev work counts (repos, infra, PRs, incidents). Progress is earned by merged,
  checkable work, never granted. Life projects stay out.
- **Two paces, one world.** Real events bring the rushes (incidents, agent questions, PRs to review); when nothing
  needs you, the world goes calm.
- **Understanding and acting meet.** Walk into a service, see its code, pods, history and roads, and start an agent
  from where you stand.
- **Showcase and cockpit.** It runs in the browser so others can see the work, and it is deep enough to run the infra.

Influences: Civilization V (the map read at a glance, research and wonders), Age of Empires III (living settlements),
Stardew Valley (calm between rushes), Discworld (the voice, and a world flat on purpose). Names are our own from day
one: the game may go public, so none of Pratchett's names.

## The world
- **Top of the disc is logical, the underside physical.** On top: every service, data VMs included, and every road.
  Underneath: one turtle per Proxmox host, one elephant per VM or LXC container. A building's root runs down to its
  elephant only where atlas has the edge (db → VM, VM/CT → host); app roots wait until atlas reads pod placement. A host
  that fails sinks its turtle, buckles its elephants and sags the disc above.
- **Rings are stack layers**, Hub to Rim: core (cluster control, fleet core), workshops (shared modules), data,
  services, edge (ingress). No sectors.
- **Placement inside a ring follows the roads** and breathes slowly, over days, never live: a road is a spring,
  stronger with traffic averaged over days; kinship (shared repo or realm) is a weaker pull; buildings repel; a radial
  force holds the ring.
- **Findable** through anchors (wonders and chosen landmarks never move), jump search, and breadcrumbs (recent
  buildings keep a trail and a pin).
- **Moons** are everything off the disc: external services (GitHub, the Anthropic API, Cloudflare, DNS) and our own
  separate systems (pi-lab). A road to a moon is a beam across the Rim; an outage dims the moon.
- **A building** is an app and its repo together. A repo that feeds several apps, or a shared module, is a workshop
  with supply roads to the apps that import it. Its look comes from kind, type (web, mobile, public, internal,
  utility), size (code, replicas), state (health, sync, alerts) and age and activity. Type comes from the `walls` data
  and the login gates atlas finds, overridden in `realms.yaml`; an ingress alone does not make a building public.
- **Roads** are the real runtime connections: calls, data and auth. Data and auth are atlas's checked `needs` edges
  today. Calls come from OTel traces (collector servicegraph connector → Prometheus → atlas PromQL); until Alloy's
  OTLP receiver and connector are wired, no call roads are drawn.

## Altitudes and lenses
One world, three altitudes; the vehicle picks the altitude.

| Altitude | Vehicle | You see | You do |
|---|---|---|---|
| Orbit | Starship | The whole disc, its underside, the moons | Strategy, from the cockpit |
| District | Bicycle | A stretch of the rings with its buildings and roads | Follow connections, spot trouble |
| Ground | On foot | Inside one building: cluster state, repo (files and code as rooms, history, PRs), roads out | Understand; start an agent with a prompt |

- **The starship cockpit** is the Civ-style overview and reaches everything: the needs-you queue (ranked by age and
  severity, click to fly there), the tech tree, jump search, advisor dashboards (health, spend, agent activity,
  deploys). On foot or on a small vehicle, a tablet you carry mimics it.
- **The interior is its own scene** behind a camera flight through the door; it feels continuous. Each altitude has
  its own draw budget.
- **Two input modes, flipped by Tab.** Exploration: the keyboard steers you or your vehicle, no command keys. Action:
  command keys, click-to-pick, cockpit panels.

## Play
- **The player is the keeper**, a gardener rather than a ruler. Actions keep their technical names (deploy, fix,
  release, PR); the keeper is a role and a tone.
- **The core loop.** Pick a tech → its goal opens and characters go to work → their questions and incidents pull you
  in → it lands when merged and checkable → the building grows, a milestone raises a wonder or starts an age → calm.
- **The tech tree** holds plan items with unlocks between them, technologies researched before adoption (Garage →
  SeaweedFS: research, then migrate), and ADRs as the discoveries research produces.
- **Failure.** A failed session, closed PR or red CI leaves the goal open: the building shows stalled scaffolding and
  the character returns to the needs-you queue. Only the player drops a goal.
- **Characters.** Each building has a resident (its `game-<repo>` definition); a few travelling specialists (the
  researcher, the night watch) cross buildings. A character that needs you marks where it stands, like a mission
  marker, and the call reaches the tablet too.
- **Every line is live.** Talking to a resident is a real session on its definition. Ambient chatter uses a small
  model, only near you, a few lines a minute, under a daily cap.
- **Personality** is generated deterministically into the personality prompt from the building's language and stack,
  age and activity, and role and exposure. No memory for now: a session stays close to a normal Claude Code session.
- **The calm.** Real day and night, ambient life (traffic as carts, idle characters, weather from load), sound, and
  pastimes (planting, fishing, discovering, talking). Some surface real things, such as a log line or an
  undocumented corner of a repo; others are pure rest. None is scored or forced.
- **Sim runs** (acting from a building before the owner's go on agent-fleet items (c) and (d) of plan 12) are labelled
  and never count toward growth, ages or goals.

## Architecture
- **Client:** stays on the web and on three.js, moving to its WebGPU renderer with a WebGL fallback for phones.
  Immersion comes from fullscreen, pointer lock, sound and a PWA install. A heavier engine was weighed: Babylon.js
  would be a port for features we can add piece by piece; Godot's web build is second-class, and the web is the
  showcase. The Go backend and `/api/town` stay.
- **Scene:** the disc is a new scene beside `town.ts`, reusing the card, looks, lenses and util modules; `town.ts`
  retires at parity. Instancing by kind keeps the town gate's budgets (220 draws, 350k triangles desktop, 200k
  phone-low).
- **Two faces, one binary, two Deployments.** The owner pod runs as today behind the game's own OIDC sign-in (never
  forward-auth on its host) and holds every write path and credential. The public pod runs the same binary in
  `--public` mode with no credentials and no ServiceAccount token, and serves a redacted snapshot the owner pod
  publishes. One replica each, stated; per-role concurrency limits.
- **Routes declare their role.** The gate's `Open` becomes `public` / `owner` / `signin`; a route with none is refused
  at start-up, and a test lists every route with its role.
- **Public types, not stripped owner types.** `PublicTown` and `PublicCard` are built from an allowlist, with a golden
  test that fails on any new field.
- **A visitor** sees coarse states only (healthy, degraded, maintenance per building; no walls, no probe details, no
  underside beyond hosts), walks the code rooms of repos public on GitHub (private ones are closed doors), and talks to
  characters only through the direct-API voice path: persona prompt and public facts, no fleet, no tools, a
  per-visitor rate limit and its own budget, visitor text treated as untrusted. The public pod holds one credential only: the capped voice key, in its own
  workspace; nothing else. Ambient chatter is generated for the owner only.
- **Secrets and failure.** Direct API calls use a real API key in its own workspace with a spend cap (fleet keeps its
  subscription token). Budget gone: canned lines. Atlas down: the last snapshot. Fleet down: residents asleep.
- **The layout runs on the server** and serves positions with `/api/town`; clients never simulate. Each layout step
  (positions plus an input hash) is logged as an event, and the layout is fed from the log, so restarts and the sim
  replay it.
- **LLM calls split by weight.** Ambient chatter and visitor talk: a direct API call from the backend (small model,
  persona prompt, no tools). The owner talking to a resident: a fleet session on its `game-<repo>` definition. Work:
  fleet sessions, per plan 12.
- **State:** the game's append-only log moves into a Pigsty Postgres database (`gamedb`) in the cluster, per the
  standard's event log; the JSONL file stays for local and sim runs. An import job carries the JSONL log over keeping
  order, `At` and the game id (live fleet threads key on it); one record = one Append. Egress to Postgres needs
  the console ADR's part 4 (14).
- **Interiors:** 3 to 4 rooms, one per architecture seam, each flat (files on shelves, code in a reader panel). They
  need atlas work first: machine access to `/api/read` (an ADR-0068 amendment), the deployed sha rather than the
  branch head, a cache sized for every app repo, and a public-repo flag.
  The file list a room is built from is atlas's `GET /api/tree?repo=` (cmd/atlas/tree.go, admin-only for now).
- **Art:** ukubi-kit's procgen re-keyed by building kind and type (age, state, size as parameters); hand-made glTF
  heroes for wonders, characters, vehicles, turtles and elephants.
- **Sim parity holds:** the sim mirrors every new signal (service-graph metrics, pod placement, moon probes) on the
  wire; character voices are stubbed with canned persona lines in sim and CI, the real API opt-in.
- **Atlas work before the promises that need it:** every app repo in `sources` and an import extractor (code rooms,
  size, workshops), pod placement (app roots), an `ext:` node kind from declared needs with blackbox probes (moons),
  and a layer per component (rings).

## Slices
0. **D0 · go public.** The infra-bootstrap app entry, chart and hosts, DNS and Cloudflare with the origin lock, the
   OIDC blueprint, the `--public` mode and snapshot, the public types, and a rate limit on a trusted client IP.
1. **D1 · the orbit disc.** The rings, buildings placed by the breathing layout, data and auth roads from `needs`,
   the underside (turtles, elephants, roots where edges exist). Replaces the islands. Its plan settles where each
   component's layer comes from.
   The layer lives in realms.yaml `layers:` (by realm/group, a component may override it with `layer:`; the repos'
   `workshops` is atlas's, the hosts have none) and reaches the game as the node attr `layer`, written by atlas's
   watch pass (modelFacts).
2. **D2 · moons and call roads.** The `ext:` kind and probes; Alloy's OTLP receiver and servicegraph connector; atlas
   reads the service graph.
3. **D3 · on foot.** The interior scene, the camera flight, the two input modes, the tablet (a stub until D5's
   cockpit). Needs the repo and import work in atlas.
4. **D4 · characters.** Residents and specialists, markers, live talk, the voice key and the chatter and visitor
   budgets, personality prompts.
5. **D5 · the cockpit and the tech tree.**
6. **D6 · the calm.** Day and night, ambient life, sound, pastimes.

## Open questions (owner)
1. Which landmarks are anchors besides the wonders.
2. How exploration mode steers on a phone (touch).
3. The model and daily cap for ambient chatter.

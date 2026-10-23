# 03-objects — The town

One meaning per channel, readable by day. Every object follows the contract in 01:
- its steady look comes from `world.state`, so a reload, a resync and a replay agree;
- transient acts come only from live frames.

## What each signal looks like
| Signal | Town | Object |
|---|---|---|
| host bucket h1..h3 | a floating hex island (`hello.town`) sized by its host's hardware (R8: radius 7/5/4): a square with a well, roads to the bridge heads, the town, the outskirts, an outer meadow, and a short stone wall either side of each bridge head | `islands.js` |
| host `ready` false | the island greys and sinks a little; its buildings grey with it. Unknown (no answer yet) is drawn up | `islands.js`, `buildings.js` |
| an app's pods on another host (R8 C, `world:annex`) | a full copy of the app's building on that island's outskirts, marked by a banner in its wire colour (the main building keeps its ground cap and lamp). Zoom levels: in the overview a copy stands as its banner alone; the island in focus shows its copies whole. Greyed with its island; named with the machines, last; a tap opens the app's card ("also on h1 and h3", the tapped island first) and flies to the copy. Never a count | `buildings.js`, `labels.js` |
| `quorum{up[]}` | the drawbridge to a host out of quorum is raised (both halves swing up) | `bridges.js` |
| an app or service | a KayKit building on its plot (`world:plot`), facing the square, roof tinted by its Ukubi wire, name label. Its kind's family (R8) sets where it stands and the ground under it | `buildings.js`, `labels.js` |
| the town's ways (R8) | streets: tan hex caps along the shortest walk over free grass from each door to a road. They reserve nothing, and walkers keep to them | `buildings.js`, `people.js` |
| a new or moved plot | the building drops in (a crane drop and squash) | `buildings.js` |
| `argo.sync` ok / failed | drop and squash / a red flag beside it for 10 min | `buildings.js` |
| health Progressing, or a build in flight | a scaffold | `buildings.js` |
| health Degraded | chimney smoke | `buildings.js` |
| health Missing / Suspended | the building fades | `buildings.js` |
| `app.gone` | ruins (rubble and a broken wall). The plot frees after 24 h | `buildings.js` |
| a machine (R8) | its building on its own host, from the probe or the node (02 Machines). A probed guest not answering (10 min held) is greyed with its lamp off, not sunk; the card says `up` / `not answering` / `checking`, and an admin also reads its real name | `buildings.js`, `main.js` |
| a live sync, gitops commits, a new ADR (R9) | the herald: a figure at the island's square (the crane yard's for laws and technologies) and its decree as a label for 6 s, first among the labels; on the phone's overview it takes the island's chip. One per island, never in a replay; a polite live region repeats it | `herald.js`, `labels.js` |
| the golden age (R9, 7 days calm) | bunting along every bridge and over each square in the wire colours, lit at night; the panel chip `golden age · day N` | `bunting.js`, `main.js` |
| turns, technologies (R9) | the panel: `Turn N ·` before the summary, a `N technologies` card (count, latest ADR number), a `guide` card naming every mechanic and its signal | `main.js`, `civ.js` |
| `pod.churn` | a hop. `crashloop` keeps hopping until the next healthy sync | `buildings.js` |
| `rpsB` / `errB` / `spike` | walkers cross a bridge, take the road to the square, then the roads and streets to the door (across the grass only where no street reaches it) / some turn red and walk away / twice the crowd | `people.js` |
| `alert.fire` / `alert.resolve` | a beacon fire at the alert's app, else its host's square, else the watchtower. Several on one spot spread on a spiral, and at most 64 are drawn. The flame's size and colour follow `sev`. One LED in the panel, lit while alerts fire, counts them all | `buildings.js`, `main.js` |
| `metrics.stale` / socket drop | a "Stale since hh:mm" label | `main.js` |
| the real sun (`sun.js`, Paris) | day and night sky, light from the sun's vector, door lamps and roof glow at night (the night is lifted so the town still reads) | `main.js`, `buildings.js` |
| `world:mark` (plant) | a visitor's tree, flag or lantern on its hex. The town has no scenery trees, so every tree is a visitor's. A plant wilts after 7 days; on a full island the oldest gives way to a new one (R7.1). `metrics.visitors` false hides them all | `marks.js` |
| a tap | an island or a building: fly there and open its card (name, description, link, host, state in words). The sea: back to the overview | `main.js` |

## Building kinds (R8 typology)
Shape says the class, the roof the system's wire, the ground the family, and words the status.

- `layout.js BUILDINGS` maps each kind to its KayKit GLB, scale and props; `layout.js FAMILY` mirrors
  `relay/town families`.
- `config.yaml` `building` chooses the kind per app. Otherwise the group decides: user → home, platform → house,
  service → warehouse.

| Family | Stands (`town.Slot` ring order, island radius r) | Ground | Kinds |
|---|---|---|---|
| Defense | the town's edge, ring r−2, then inwards, then the outskirts | stone cap | gate (wall gate), guardhouse (catapult tower), watchtower, vault (mine + fence), keep (castle) |
| Public | around the square, ring 1 outwards | paving cap | craneyard (belfry tower), depot (market), warehouse (house + barrels), granary (grain field + sacks), archive (tower base + crates), cellar (watermill) |
| Industry | the outskirts, ring r−1, then inwards | earth cap | hall (barracks), forge (blacksmith), witness (windmill), mill (lumbermill) |
| Shops | the street front (next to a spoke) of rings 2..r−2 first, then the outskirts, ring 1 last | grass | church, and a tavern with its own props for inn, alehouse, bookshop, playhouse |
| Houses | rings 2..r−2, then the outskirts, ring 1 last | grass | home, house |

- R7's `castle`, `tower` and `market` stay valid kinds, so a replay of an older day still draws.
- Industry and Houses cast no shadow, and neither do the tiles.
- The labels' dot: round for an app, a diamond for a platform service, a square for a machine (R8 deploy B). On the
  phone's overview, one chip per island ("h1 · 12 buildings") replaces the names.

Services by role:

| Role | Product |
|---|---|
| gate | traefik |
| guardhouse | authentik |
| craneyard | argocd |
| watchtower | monitoring |
| warehouse | longhorn |
| vault | infisical |
| granary | garage |
| depot | zot |

Only the role key reaches the wire.

## Budgets and tiers
- Each object instances per kind: building bodies and roofs, scaffolds, smoke, rubble, braziers, flames, lamps,
  flags, walkers and marks.
- The page gate's fixture (`relay-town.json`: all 27 apps, services and machines placed, builds, firing alerts, visitor
  marks, traffic) stays within 220 draws on both tiers, and 350k tris on the high tier (desktop) / 200k on phone-low
  (`gate.mjs page`; owner, 2026-10-23, room for the Civ release).
- `relay-town-states.json` drives the styleboard (`townshots.mjs`): every building state side by side.
- Phone-low caps walkers at 24 (72 on desktop) and has no shadow pass.
- R8 C: the gate adds every app's annex copies (30 on `relay-town.json`) under the 70 alerts, on both tiers, in the
  overview (banners) and with each island in focus (whole copies, on their own meshes casting no shadow).
- R9: under the copies and alerts, a Civ worst case: the golden age's bunting and a herald on each island. Measured:
  123 draws / 168k tris on desktop, 70 / 104k on phone-low.
- Labels show on every tier, decluttered: nearest first, apps before services, never behind the dock.
- A model that fails to load is drawn as a home. A 404, meaning this page's build was retired, also reloads the page
  once (R7.1).

## Not in v1
- OOM crates;
- grey stale fog;
- the relocation flight (v1 drops out and drops in);
- packets on bridges (`river`);
- a storage yard (pools);
- rx clouds;
- a cpu/mem chimney;
- gitops carts (the herald calls laws and technologies, R9);
- the riot: walkers circling a crashlooping door (R9, deferred);
- the scrub UI.

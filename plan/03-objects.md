# 03-objects — The town

One meaning per channel, readable by day. Every object follows the contract in 01:
- its steady look comes from `world.state`, so a reload, a resync and a replay agree;
- transient acts come only from live frames.

## What each signal looks like
| Signal | Town | Object |
|---|---|---|
| host bucket h1..h3 | a floating hex island (`hello.town`): a square with a well, roads to the bridge heads, a building ring, an outer meadow | `islands.js` |
| host `ready` false | the island greys and sinks a little; its buildings grey with it. Unknown (no answer yet) is drawn up | `islands.js`, `buildings.js` |
| `quorum{up[]}` | the drawbridge to a host out of quorum is raised (both halves swing up) | `bridges.js` |
| an app or service | a KayKit building on its plot (`world:plot`), facing the square, roof tinted by its Ukubi wire, name label | `buildings.js`, `labels.js` |
| a new or moved plot | the building drops in (a crane drop and squash) | `buildings.js` |
| `argo.sync` ok / failed | drop and squash / a red flag beside it for 10 min | `buildings.js` |
| health Progressing, or a build in flight | a scaffold | `buildings.js` |
| health Degraded | chimney smoke | `buildings.js` |
| health Missing / Suspended | the building fades | `buildings.js` |
| `app.gone` | ruins (rubble and a broken wall). The plot frees after 24 h | `buildings.js` |
| `pod.churn` | a hop. `crashloop` keeps hopping until the next healthy sync | `buildings.js` |
| `rpsB` / `errB` / `spike` | walkers cross a bridge, take the road to the square, then a grass path to the door / some turn red and walk away / twice the crowd | `people.js` |
| `alert.fire` / `alert.resolve` | a beacon fire at the alert's app, else its host's square, else the watchtower. Several on one spot spread on a spiral, and at most 64 are drawn. The flame's size and colour follow `sev`. One LED in the panel, lit while alerts fire, counts them all | `buildings.js`, `main.js` |
| `metrics.stale` / socket drop | a "Stale since hh:mm" label | `main.js` |
| the real sun (`sun.js`, Paris) | day and night sky, light from the sun's vector, door lamps and roof glow at night (the night is lifted so the town still reads) | `main.js`, `buildings.js` |
| `world:mark` (plant) | a visitor's tree, flag or lantern on its hex. The town has no scenery trees, so every tree is a visitor's. A plant wilts after 7 days; on a full island the oldest gives way to a new one (R7.1). `metrics.visitors` false hides them all | `marks.js` |
| a tap | an island or a building: fly there and open its card (name, description, link, host, state in words). The sea: back to the overview | `main.js` |

## Building kinds
- `layout.js BUILDINGS` maps each kind to its KayKit GLB.
- `config.yaml` `building` chooses the kind per app. Otherwise the group decides: user → home, platform → tower,
  service → warehouse.

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
- The page gate's fixture (`relay-town.json`: all 16 apps and services placed, builds, firing alerts, visitor
  marks, traffic) stays within 220 draws and 200k tris on desktop and on phone-low (`gate.mjs page`).
- `relay-town-states.json` drives the styleboard (`townshots.mjs`): every building state side by side.
- Phone-low caps walkers at 24 (72 on desktop) and has no shadow pass.
- Labels show on every tier, decluttered: nearest first, apps before services.
- A model that fails to load is drawn as a home. A 404, meaning this page's build was retired, also reloads the page
  once (R7.1).

## Not in v1
- OOM crates;
- grey stale fog;
- replica annexes;
- the relocation flight (v1 drops out and drops in);
- packets on bridges (`river`);
- a storage yard (pools);
- rx clouds;
- a cpu/mem chimney;
- gitops carts and ADR notices;
- the scrub UI.

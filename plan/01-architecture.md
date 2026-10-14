# 01-architecture — Architecture

## Repo and ownership
```
mountain/
  go.mod embed.go       repo root (R4): package `mountain`, `//go:embed www`, imported by relay
  units.json            FROZEN W2 — shared Go/JS constants
  Dockerfile helm/values.yaml .github/workflows/   orchestrator (seam); CI pins :sha in helm/values.yaml
  relay/
    main.go hub.go config.yaml   orchestrator-only
    sanitize.go store.go worldcore.go   owner per wave (W1a, W2, W1a)
    src_prom.go src_alerts.go src_github.go   one owner each; registered in sources.go (frozen W2)
    toy_<name>.go       one per toy (W4), registered in validators.go (frozen W2)
    sources.go metrics.go   orchestrator (W1a seam)
    oidc.go
  www/
    index.html vendor/ (three r186 from pi-lab + SimplexNoise, RoundedBoxGeometry,
                        postprocessing chain + shaders, cannon-es 0.20.0; manifest.json regenerated)
    js/boot.js main.js story.js kit.js shaders.js post.js socket.js   orchestrator (socket.js body: W1a lane B)
    js/contract.js world.js layout.js                              FROZEN W2
    js/cut.js                                                      W3 serial (frozen after)
    models/<name>/{index.js, field.glsl, field.js, fixture.json, test.js}   owned by one agent
    dev/sandbox.html gate.mjs perf.mjs fixtures/relay-*.json schema/*.json
  plan/
```

**Seam commits (R4).** Each wave starts with one orchestrator commit that writes every orchestrator-owned and soon-frozen file. Lanes then own only their own files. A lane that needs a seam change stops and asks the orchestrator.

| Seam | Orchestrator writes | Lanes own |
|---|---|---|
| W1a | `units.json`; root `go.mod` + `embed.go`; `relay/{main,hub,sources,metrics}.go`, `relay/config.yaml`; minimal `worldcore.Base(seed)` + `relay dump-hello`; `www/index.html`; `js/{boot,main,kit,shaders,socket,contract,world,layout}.js` (interfaces; `layout.js` profiles for every model of the wave); `vendor/` (re-vendored three@0.186.1 + RoundedBoxGeometry, SimplexNoise, EffectComposer/RenderPass/UnrealBloomPass/OutputPass/ShaderPass, cannon-es 0.20.0, regenerated `manifest.json`); `www/dev/schema/*.json`; Dockerfile, `helm/values.yaml`, CI; minimal `dev/gate.mjs` + `dev/sandbox.html`; kit `lava()` + interface stubs `crystal()`/`flow()`/`stamp()`/`uFootprint` (09) | (A) relay: `src_prom.go`, `worldcore.go` body, `sanitize.go`, `/ws`, `/api/topology`, `:9090`, fixture source + **relay fixtures `www/dev/fixtures/relay-*.json`**. (B) www: `world.js`/`socket.js` bodies, sandbox controls, full `gate.mjs`, **`dev/perf.mjs`**, styleboard. (C) `models/mountain/`, `models/eruption/` (each model's `fixture.json` belongs to its model owner). (D) infra-bootstrap PRs (`/add-app`, Traefik 15 s) |
| W1b | `post.js`; `layout.js` profiles for cauldron, sky, lightning; `world.rig` slot | (A) `src_alerts.go` + stale handling. (B) `models/cauldron/`. (C) `models/sky/` (sky-min). (D) `models/lightning/`. (E) infra-bootstrap PR |

## Units (`units.json`, frozen W2)
- World extent: 200 × 200 units, origin at the centre, y up.
- Grids, all with cell size = extent / N:
  - `GRID = 128` (heightSolid, collider, render)
  - `GRID_LOW = 96` (render only; sampled from 128)
  - `ICE = 64`
- Heights are int16 millimetres. Mark x/z are quantised to `GRID` cells, and y is recomputed by the relay.

## Model contract (`contract.js`)
```js
// (ctx) => Promise<Model>;  ctx = { THREE, kit, world, plot:{x,z,r,yRange}, params, rng }
// Model = { root, params, pose(P) /* pure given world snapshot+state */, step(dt,t) /* 1/30, no alloc */,
//           on?:{[busType]:fn}, field?:{glsl,uniforms}, pick?(ray)->card|null, busy(), dispose() }
```
Every model folder ships `fixture.json` (a scripted event and world-delta timeline) plus `test.js`, which checks behaviour. Example: "after 3× `argo.sync`, `ice` at the site grows by at least X". The gate runs both.

## World (`world.js`)
**Fields** (128² unless noted; writers are enforced in dev mode):

| Field | Writer | Use |
|---|---|---|
| `h.base` | **world.js**, from `hello.snapshot` | Go-generated terrain base, int16 mm, spiral walkway + cave niches already carved (06); `bench` mask (u8) and `caves[]` alongside |
| `h.land` | **world.js**, from `world:deposit` raster cells | ramps over 3 s so lava arrives first |
| `heightSolid` | — (`h.base + h.land`) | physics, picking, snowline, relay validation |
| `h.talus` | scree | visual only |
| `ice` (64²) | glacier, from `world:glacier` state | visual |
| `temp` | magma | eruption emits `lava.heat` |
| `moisture` | sky | from `steam` + `melt` |

**Shared state:**

| State | Writer | Source |
|---|---|---|
| `world.metrics` | socket.js | metrics frame (02) |
| `world.state` | socket.js | `hello.state` + events (alerts firing, health, replicas, nodes, quorum, builds in flight) |
| `world.sites` | world.js | `/api/topology` + `world:site` deltas; `app.gone` removes |
| `world.wind`, `world.snowline` | socket.js | metrics frame (relay-computed) |
| `world.clouds` | sky (W1b sky-min) | real floor from net rx + visitor clouds + steam |
| `world.sun` | sky | SunCalc port, Paris |
| `world.rig` **(R4)** | sky | `{elev, rising}` → `kit.setPhase` (09). Fixed at dusk until sky-full (W3) |
| `world.clip` | story.js (`cut(P)`) | plane + depth; closed = `constant 1e4` |
| `world.stale` | socket.js | since-timestamp |

**Bus registry:**
- Bus types may have several **emitters**, but each world/shared-state slot has exactly one writer.
- Every `layout.js` profile declares `emits[]` and `listens[]`, and the gate checks them.

| Type | Payload | Emitters | Listeners |
|---|---|---|---|
| `ev:<kind>` | 02 enum | socket.js | per model |
| `world:deposit` | `{id=ev.id, x0, z0, w, h, cells:int16[]}` | world.js | eruption |
| `world:vein` | `{fp, a, b, thick, layer, ref}` | world.js | veins |
| `world:glacier` | `{mass, tongues:[{app, len}], ash:[day]}` | world.js | glacier |
| `world:ash` | `{day, title?}` | world.js | strata |
| `world:site` | `{app, x, z, y, tier, group}` | world.js | summit, eruption, magma |
| `world:mark` | `{id, kind, pose, weight}` | world.js | cairns, footprints, toss |
| `world:cloud` | `{id, pos, state, credits[]}` | world.js | toy-clouds, sky |
| `world:rain` | `{x, z, r}` | world.js | footprints, cairns |
| `magma.dyke` | `{app, x, z, heat}` | magma | eruption (size boost only) |
| `lava.heat` | `{x, z, q}` | eruption | magma |
| `cauldron.sag` | `{x, z, depth, sev}` | cauldron | glacier |
| `jokulhlaup` | `{x, z, vol}` | cauldron | river |
| `melt` | `{x, z, vol}` | glacier | river, sky |
| `steam` | `{x, z, vol}` | glacier, cauldron | sky |
| `ice.calve` | `{x, z, size}` | glacier | scree |
| `aquifer.seep` | `{pool, x, z}` | aquifers | river |

## Wire encoding (R4)
- JSON Schema for the `hello`, `ev`, `metrics` and `world` frames lives in `www/dev/schema/*.json`. Go and JS tests both validate against it.
- int16 grids: **base64, little-endian, row-major (z outer, x inner)**. Cell 0 sits at world (−100, −100).
- Heights: int16 mm; units = mm / 1000.
- The hello fixture comes from `relay dump-hello` (real Go), never by hand.

## Kit (`kit.js`, orchestrator-owned; see `09-style.md`)
- Factories, each tagging `userData.kit='<factory>'`: `lit()`, `glow()`, `line()`, `points()`, `dome()`, `terrain()`, `stamp()`, `cave()`, `crystal()`, `lava()`, `flow()`, `ramp(name)`. Procedural helpers: `rock(seed)`, `puff(seed)`, `shard(seed)`.
- Kit pins tone mapping (`LinearToneMapping`), exposure (1), the light rig (hemisphere + directional) and `FogExp2`, with colours from the current time key. `kit.setPhase(elev, rising)` lerps them all.
- No GLBs, no shadows, no textures outside `glow`/`points`/fields.

## Shaders (`kit.js` + `shaders.js`, orchestrator-owned)
- The orchestrator owns the **terrain and crystal shaders**. They expose uniform slots: `uClip`, `uSnowline`, `uWobble`, `uHeat`, `uStale`, `uSnowLift`, `uEmber`, **`uVeins`** (kit-owned vein capsule texture), `bench` attribute, **`uFootprint`** (terrain; sampler bound only to the kit-owned 512² footprint RT, drawn by an offscreen stamp scene built from kit `stamp()`). Sky and shimmer **only write uniforms** and never call `onBeforeCompile`.
- Patches work on both kit materials (Standard on high, Lambert on low), with `customProgramCacheKey` set.
- Tone mapping and output colour space are pinned the same for the composer path and the direct path.

## CI (R4, `.github/workflows/`, orchestrator)
| Item | Rule |
|---|---|
| Triggers | `push` to main on paths `relay/**`, `www/**`, `embed.go`, `go.mod`, `go.sum`, `Dockerfile`; plus `workflow_dispatch` |
| Excluded paths | `helm/**`, `plan/**` (so the tag-bump commit cannot retrigger CI) |
| Tag bump | CI writes the image `:sha` into `helm/values.yaml` and pushes with `GITHUB_TOKEN` (which also cannot retrigger workflows) |
| Orchestrator push | `git pull --rebase` before each wave push (CI's bump commits land on main in between) |

## Cut (`cut.js`, W3 serial)
- The mountain is a closed mesh (skirt + bottom), DoubleSide.
- The back-face cap computes the view-ray ∩ plane point `p` and writes `gl_FragDepth`. perf.mjs measures early-z loss; the fallback is a separate cap mesh.
- Field providers (`field.glsl`: `vec4 <name>Field(vec3 p, vec4 base)`) are composed in `layout.js` order: strata → aquifers → magma → veins → caves (mountain-owned: a short irregular `deep` blob hollow per `caves[]` entry, not a tube). Missing providers are skipped, so nobody edits cut.js after W3 start.
- **Clipping list:**

  | Clipped | Not clipped |
  |---|---|
  | mountain body, bedrock slabs (closed, non-overlapping), glacier shell (closed), eruption lava | sky, clouds, crystals, river, toys, particles |

- `cut.pick(ray)` = ray ∩ plane, then check `y < heightSolid`.

## Post (`post.js`)
Desktop: RenderPass → UnrealBloom (threshold 1, HalfFloat) → haze ShaderPass (enabled while `uHeat > 0`) → OutputPass. `samples: 4`. Phones skip it. Models only write uniforms; emissive above 1 means glow.

## Budgets (`layout.js`)
- 220 draws / 200k tris at peak.
- **No shadows.**
- Each profile: `tris`, `draws`, `peak`, `stepMs`, `writes`, `emits`, `listens`, `clipped`, `visibleFrom`, `goneFrom`, `footprint`, `yRange`.
- Research estimate: about 30 draws and 100k tris total.


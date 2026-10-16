# 01-architecture — Architecture

## Repo and ownership
```
mountain/
  go.mod embed.go       repo root (R4): package `mountain`, `//go:embed www`, imported by relay
  units.json            FROZEN W2 (done 2026-10-14) — shared Go/JS constants
  FROZEN.sha256 freeze_test.go   the W2 freeze, enforced: sha256 of every frozen file; CI fails on an unannounced change
  Dockerfile helm/values.yaml .github/workflows/   orchestrator (seam); CI pins :sha in helm/values.yaml
  relay/
    main.go hub.go config.yaml   orchestrator-only
    sanitize.go store.go scrub.go migrations/   owner per wave (W1a, W2)
    worldcore/          package (W1a seam): base.go = Base(seed) + wire encoding (orchestrator, frozen); other files lane A
    src_prom.go src_alerts.go src_github.go   one owner each; registered in sources.go (frozen W2)
    toy_<name>.go       one per toy (W4), registered in validators.go (W4 seam: RegisterToy, Place, marks, kill switch, tick)
    sources.go metrics.go   orchestrator (W1a seam)
    oidc.go
  www/
    index.html vendor/ (three r186 from pi-lab + SimplexNoise, RoundedBoxGeometry,
                        postprocessing chain + shaders, cannon-es 0.20.0; manifest.json regenerated)
    js/boot.js main.js story.js kit.js shaders.js post.js socket.js   orchestrator (socket.js body: W1a lane B)
    js/contract.js world.js layout.js kit.js tokens.json           FROZEN W2 (done; FROZEN.sha256)
    js/page.js                                                     orchestrator: landed models, load order (not frozen)
    js/toys.js                                                     orchestrator (W4 seam): toolbar, input routing, pickTerrain, surfacePhysics
    js/cut.js                                                      W3 serial (frozen after)
    models/<name>/{index.js, field.glsl, field.js, fixture.json, test.js}   owned by one agent
    dev/sandbox.html gate.mjs perf.mjs fixtures/relay-*.json schema/*.json (schema FROZEN W2)
  plan/
```

**Seam commits (R4).** Each wave starts with one orchestrator commit that writes every orchestrator-owned and soon-frozen file. Lanes then own only their own files. A lane that needs a seam change stops and asks the orchestrator.

| Seam | Orchestrator writes | Lanes own |
|---|---|---|
| W1a | `units.json`; root `go.mod` + `embed.go`; `relay/{main,hub,sources,metrics}.go`, `relay/config.yaml`; minimal `worldcore.Base(seed)` + `relay dump-hello`; `www/index.html`; `js/{boot,main,kit,shaders,socket,contract,world,layout}.js` (interfaces; `layout.js` profiles for every model of the wave); `vendor/` (re-vendored three@0.186.1 + RoundedBoxGeometry, SimplexNoise, EffectComposer/RenderPass/UnrealBloomPass/OutputPass/ShaderPass, cannon-es 0.20.0, regenerated `manifest.json`); `www/dev/schema/*.json`; Dockerfile, `helm/values.yaml`, CI; minimal `dev/gate.mjs` + `dev/sandbox.html`; kit `lava()` + interface stubs `crystal()`/`flow()`/`stamp()`/`uFootprint` (09) | (A) relay: `src_prom.go`, `worldcore.go` body, `sanitize.go`, `/ws`, `/api/topology`, `:9090`, fixture source + **relay fixtures `www/dev/fixtures/relay-*.json`**. (B) www: `world.js`/`socket.js` bodies, sandbox controls, full `gate.mjs`, **`dev/perf.mjs`**, styleboard. (C) `models/mountain/`, `models/eruption/` (each model's `fixture.json` belongs to its model owner). (D) infra-bootstrap PRs (`/add-app`, Traefik 15 s) |
| W1b | `post.js`; `layout.js` profiles for cauldron, sky, lightning; `world.rig` slot | (A) `src_alerts.go` + stale handling. (B) `models/cauldron/`. (C) `models/sky/` (sky-min). (D) `models/lightning/`. (E) infra-bootstrap PR |

## Units (`units.json`, FROZEN W2)
- World extent: 200 × 200 units, origin at the centre, y up.
- Grids, all with cell size = extent / N:
  - `GRID = 128` (heightSolid, collider, render)
  - `GRID_LOW = 96` (render only; sampled from 128)
  - `ICE = 64`
- Heights are int16 in steps of `HEIGHT_Q` = 0.01 units (cm; **W1a seam**: int16 mm caps at ±32.8 m, the approved massif spans −67…+68 m). Mark x/z are quantised to `GRID` cells, and y is recomputed by the relay.

## Model contract (`contract.js`)
```js
// (ctx) => Promise<Model>;  ctx = { THREE, kit, world, plot:{x,z,r,yRange}, params, rng }
// Model = { root, params, pose(P) /* pure given world snapshot+state */, step(dt,t) /* 1/30, no alloc */,
//           on?:{[busType]:fn}, field?:{glsl,uniforms}, pick?(ray)->{kind, d, name?, ...raw}|null (R6: d = distance along the ray, kind = an i18n legend.<kind>; raw sanitised state only, cards.js formats it), busy(), dispose() }
```
Every model folder ships `fixture.json` (a scripted event and world-delta timeline) plus `test.js`, which checks behaviour. Example: "after 3× `argo.sync`, `ice` at the site grows by at least X". The gate runs both.

## World (`world.js`)
**Fields** (128² unless noted; writers are enforced in dev mode):

| Field | Writer | Use |
|---|---|---|
| `h.base` | **world.js**, from `hello.snapshot` | Go-generated terrain base, int16 (HEIGHT_Q steps), spiral walkway + cave niches already carved (06); `bench` mask (u8) and `caves[]` alongside |
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
| `world.state` | socket.js | `hello.state`, then kept current by the frames before their emit (W3b): `alerts`, `veins` (≤ 32), `glacier`, `ash` (≤ 32), `health`, `replicas`, `builds` (in flight). A new object after every hello/replay: a model that caches it resyncs on identity change (strata). Nodes, quorum: not yet |
| `world.sites` | world.js | `/api/topology` + `world:site` deltas; `app.gone` removes |
| `world.wind`, `world.snowline` | socket.js | metrics frame (relay-computed) |
| `world.clouds` | sky (W1b sky-min) | real floor from net rx + visitor clouds + steam |
| `world.sun` | sky | SunCalc port, Paris |
| `world.rig` **(R4)** | sky | `{elev, rising}` → `kit.setPhase` (09). Fixed at dusk until sky-full (W3) |
| `world.replay` (W2) | world.js | `null` or `{day, frames, metrics, left}` while a scrub replay is held (`world.scrub(day)` / `world.release()`, 06) |
| `world.clip` | story.js (`cut(P)`) | plane + depth; closed = `constant 1e4` |
| `world.stale` | socket.js | since-timestamp |

**Bus registry (FROZEN W2):**
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
| `world:mark` | `{id, mark, pose, weight}` | world.js | cairns, footprints, toss |
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
- Heights: int16 steps of `units.json` `HEIGHT_Q` (0.01, cm); units = value × HEIGHT_Q (W1a seam, was mm / 1000).
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
- ~~The back-face cap computes the view-ray ∩ plane point `p` and writes `gl_FragDepth`.~~ **W3a: the cap is a separate mesh** (the fallback). perf.mjs measured the back-face cap: the body lost early-z and the Android profile went from 16.7 to 33.3 ms p50 with the cut closed. The cap mesh (`cut.cap()`, in the mountain's root, 1 draw / 2 tris, hidden while closed) is a kit `terrain()` quad laid on the `uClip` plane in its vertex shader; it keeps the fragments inside the solid (heightSolid texture, interpolated as the body mesh) and shades them from the providers. Android stays at 16.7 ms p50 closed, 16.7 p50 / 33.4 p95 open (cut 60).
- Providers are built with kit `field(glsl, uniforms)` (09): token-named uniforms follow the time key. The cut is driven by `world.clip` (writer: story; until story.js, the dev knob `?cut=<depth>` on the page and the sandbox `cut` slider), `cut.at(depth, az)` builds the vertical plane facing the camera azimuth.
- Field providers (`field.glsl`: `vec4 <name>Field(vec3 p, vec4 base)`) are composed in `layout.js` order: strata → aquifers → magma → veins → caves (mountain-owned: a short irregular `deep` blob hollow per `caves[]` entry, not a tube). Missing providers are skipped, so nobody edits cut.js after W3 start.
- **Clipping list:**

  | Clipped | Not clipped |
  |---|---|
  | mountain body, bedrock slabs (closed, non-overlapping), glacier shell (closed), eruption lava | sky, clouds, crystals, river, toys, particles |

- `cut.pick(ray)` = ray ∩ plane, then check `y < heightSolid`.

## Post (`post.js`)
Desktop: RenderPass → UnrealBloom (threshold 1, HalfFloat) → haze ShaderPass (enabled while `uHeat > 0`) → OutputPass. `samples: 4`. Phones skip it. Models only write uniforms; emissive above 1 means glow.

## Budgets (`layout.js`, FROZEN W2: a profile for every model of W1–W4)
- 220 draws / 200k tris at peak.
- **No shadows.**
- Each profile: `tris`, `draws`, `peak`, `stepMs`, `writes`, `emits`, `listens`, `clipped`, `visibleFrom`, `goneFrom`, `footprint`, `yRange`.
- Research estimate: about 30 draws and 100k tris total.


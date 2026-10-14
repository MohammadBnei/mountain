# 09-style — Style (W0.5, amendment R4)

**Source of truth:** hq `app/arete.html` (hero SVG) and `app/web/src/arete.css` (tokens). The palette is **frozen in `kit.js`** and enforced by `gate.mjs`. Models never declare a colour: they take a token, a semantic colour or a `ramp(name)` from kit.

**Generated, not hand-typed.** Every derived hex below (time keys, semantic mixes, `unmeasured`, rig) is output of the generator: `w05/style/gen.mjs` now, `kit.js` (same code) from W1a. The table must equal the generator output.

| Derivation rule | Value |
|---|---|
| Mixing space | **sRGB** (gamma-encoded channels, 0–1), not linear |
| Rounding | each channel `Math.round(v × 255)` after clamp to [0, 1] |
| Gate comparison | sRGB hex (`Color.getHexString()`), **±1/255 per channel** |

## Tokens (dusk, canonical)

| Token | Hex | Aretē name | Use |
|---|---|---|---|
| `sky0` | `#191b38` | `--sky` | dome top |
| `sky1` | `#2a2347` | hero gradient 45 % | dome middle, hemisphere sky |
| `horizon` | `#5c3254` | `--dusk` | dome bottom, **fog colour** |
| `cloud` | `#232742` | `--cloud` | cloud shadow side / bases (`ramp('cloud')` stop 0) |
| `ridge` | `#3b2a48` | hero far ridge | distant ridge, backdrop |
| `rock0` | `#2a2733` | `rockG` stop 0 | rock ramp top |
| `rock1` | `#17141b` | `rockG` stop .8 | rock ramp body, scree |
| `emberRoot` | `#3a1610` | `--ember` / `rockG` stop 1 | rock root, hemisphere ground |
| `stone` | `#1b1e25` | `--stone` | bedrock slabs, strata |
| `stone2` | `#232730` | `--stone-2` | slab idle, alternate strata |
| `deep` | `#120b0a` | `--deep` | mountain bottom, cut-face deep |
| `snow` | `#cfe2ea` | hero snow cap | snow above `uSnowline` |
| `ice` | `#b6d3e0` | `--ice` | glacier surface, crystals |
| `iceDeep` | `#1b2a35` | `--ice-deep` | glacier underside, cauldron water |
| `cloudLit` | `#bab6b6` | mix(ink, cloud, .25) | cumulus lit albedo (the rig tints it: white by day, sun-tinted dawn/dusk, blue-grey night). `cloud` stays the shadow/underside |
| `trail` | `#423638` | mix(rock0, sunMid, .12) | worn walkway bench (warm dust; never snow-white by day) |
| `gold` | `#ddb153` | `--gold` | veins |
| `magma` | `#e5562f` | `--magma` | lava, magma glow |
| `lavaHot` | `#ff8a5c` | `.l-magma` | lava core, eruption front |
| `sunCore` | `#ffcf8f` | hero sun disk | sun disk |
| `sunMid` | `#f2a65e` | `--sun` | sun halo, dusk directional |
| `sunHalo` | `#ffd59a` | `sunG` stop 0 | sun glow, dawn directional |
| `elmo` | `#d9c8ff` | `elmoG` | St. Elmo summit glow, lightning halo |
| `above` | `#c9b6ff` | `.z-above` | group `user` |
| `slopes` | `#9fd0b9` | `.z-slopes` | health `ok` |
| `water` | `#8fc3df` | `.l-ocean` | river, aquifers, group `platform` |
| `ink` | `#ece6dc` | `--ink` | lightning core, day directional, UI text |

**Fixed tokens:** `gold`…`ink` (emissive, accent, semantic) never change with the time key. **Env tokens:** `sky0`…`iceDeep`, `cloudLit`, `trail` shift per key. A token written `name@dusk` is the dusk value **pinned** (never shifted).

## Semantic colours (kit exports only)

| Name | Value | Derived from |
|---|---|---|
| health `ok` / `degraded` / `down` | `#9fd0b9` / `#ddb153` / `#e5562f` | slopes / gold / magma |
| group `platform` / `user` | `#8fc3df` / `#c9b6ff` | water / above |
| slab not ready | `#120b0a` | deep |
| `errB` red | `#e5562f` | magma |
| longhorn water (clear, deep, 3 shimmer layers) | `#8fc3df`, opacity .55 | water |
| local water (murky, shallow; local-path, unmeasured until W3 textfile) | `#697582`, opacity .8; when unmeasured `#444649` | mix(water, emberRoot@dusk, .45) |
| `unmeasured` | HSL saturation ×0.3, lightness ×0.6, static. Water → `#5c7380`, gold → `#6e6249`, local → `#444649` | any token |
| vein inlay / `ref` boost | albedo `#ddb153` × .55, metallic; emissive .12 + core .7 / core `#ffd59a` × 1.4 | gold / sunHalo |
| crystal body | `#8397a2` | mix(ice@dusk, stone2@dusk, .35) |
| lightning core / halo | `#ece6dc` × 3 / `#d9c8ff` | ink / elmo |
| ember pool (magma vent, cut closed) | **terrain-shader emissive term**, so it hugs the ground at the mountain foot: `magma × .6 × max(heat, .45) × uEmber × (1 − smoothstep(.25, 1, ‖(xz − c)/r‖))`, centre `c` at the foot facing the story camera (board: (0, 86)), radii `r` ≈ (30, 9). Kit uniforms `uPool` (centre, radii) + `uPoolCol`, written only by magma via kit. Never a sprite | magma |

Semantic colours encode data, so they do **not** shift with the time key.

## Ramps (kit exports only; discrete stops)

A ramp is a list of stops `[t, colour]`. An instance colour must **equal a stop or lie on the segment between two adjacent stops within 1/255** (sRGB). Models cannot declare new ramps.

| Ramp | Kind | Stops |
|---|---|---|
| `slabLoad` | semantic | 0 `#232730` (stone2@dusk) → .5 `#3a1610` (emberRoot@dusk) → 1 `#e5562f` (magma) |
| `health` | semantic | 0 `#9fd0b9` (ok) → .5 `#ddb153` (degraded) → 1 `#e5562f` (down) |
| `group` | semantic | 0 `#8fc3df` (platform) → 1 `#c9b6ff` (user) |
| `lava` | semantic | 0 `#ff8a5c` (lavaHot) → .5 `#e5562f` (magma) → 1 `#3a1610` (emberRoot@dusk) |
| `rock` | env | 0 mix(`rock1`, `emberRoot`, `uEmber`) → .06 `rock1` → 1 `rock0` (current key; ember only at the root, faded by day) |
| `snow` | env | 0 `ice` → 1 `snow` (current key) |
| `cloud` | env | 0 `cloud` (base) → 1 `cloudLit` (top), per lump by height in its cluster |

- **Semantic ramps** never shift.
- **Env ramps** follow the key. Chosen mechanism (simplest that stays exact): the mesh is registered with kit and colours are set with `kit.setRampAt(mesh, i, t)`, which stores `t`; `setPhase` re-evaluates the ramp of the blended key and rewrites `instanceColor` (only when the blend weight moved ≥ 0.01). The terrain does not use instance colours: `terrain()` evaluates `rock`/`snow` in its shader from height, with stop uniforms written by `setPhase`.

## Time keys

**Rule** (generator output; the gate re-derives and compares):
- dusk = the tokens above.
- Env tokens with L < 0.5 scale HSL lightness by `1 + a` (cap 0.9). Lightening uses HSL L; darkening uses an RGB multiply, which keeps saturation. Tokens with L ≥ 0.5 keep their L when lifted.
- Then mix toward a palette tint by `t` (in sRGB).
- Hues come from dusk plus a ≤ 10 % palette tint, so the identity holds.
- **Exception, day sky group (R4 style review, decision):** after the shift, `sky0`, `sky1`, `ridge` mix toward `water` by .5, `horizon` toward `water` by .6, `cloud` toward `ink` by .5. Without it day read as dawn (mauve haze). Ground tokens keep the ≤ 10 % rule, so rock stays grey-violet stone.

| Key | `a` | Tint (`t`) |
|---|---|---|
| `night` | −0.45 | water `#8fc3df` (.08) |
| `dawn` | +0.8 | above `#c9b6ff` (.08) |
| `day` | +1.6 | water `#8fc3df` (.10); sky group per the exception |
| `dusk` | 0 | — |

| Token | night | dawn | day | dusk |
|---|---|---|---|---|
| `sky0` | `#181d2e` | `#393c71` | `#6c8bbd` | `#191b38` |
| `sky1` | `#212136` | `#56498a` | `#8298cb` | `#2a2347` |
| `horizon` | `#3a293c` | `#a8619f` | `#a5b8d6` | `#5c3254` |
| `cloud` | `#1d2333` | `#4a4f82` | `#a7abc5` | `#232742` |
| `ridge` | `#292537` | `#72548c` | `#93a2ca` | `#3b2a48` |
| `rock0` | `#21232c` | `#564f69` | `#706e8e` | `#2a2733` |
| `rock1` | `#171a20` | `#363041` | `#444255` | `#17141b` |
| `emberRoot` | `#291b1a` | `#70332f` | `#96473c` | `#3a1610` |
| `stone` | `#191f24` | `#3d4052` | `#4d5a6d` | `#1b1e25` |
| `stone2` | `#1d232a` | `#4a4f64` | `#606e87` | `#232730` |
| `deep` | `#151517` | `#2e2125` | `#392e2e` | `#120b0a` |
| `snow` | `#748289` | `#cfdeec` | `#c9dfe9` | `#cfe2ea` |
| `ice` | `#677a83` | `#b8d1e2` | `#b2d1e0` | `#b6d3e0` |
| `iceDeep` | `#19252d` | `#3d546c` | `#4d7693` | `#1b2a35` |
| `cloudLit` | `#696c6e` | `#bbb6bc` | `#b6b7ba` | `#bab6b6` |
| `trail` | `#2d2b2e` | `#7e6871` | `#a4979c` | `#423638` |

### Light rig per key (kit-owned; tuned on the lit styleboard, `w05/style/board.png`)

`HemisphereLight` + one `DirectionalLight`, `FogExp2`. **Tone mapping: `LinearToneMapping`** (exposure 1, day 1.25) (the tokens are display colours; ACES's toe crushed the dusk palette ~4× and the dome away from the hero gradient. Lit surfaces stay < 1 by the luminance rule; only emissives exceed 1 and are carried by bloom).

three r186 lights are physically based (diffuse = albedo/π × irradiance), and the env tokens used as light colours are dark (`sky1` ≈ 0.02 linear luminance). So **kit applies every rig colour at unit max channel** (`c / max(r, g, b)`): the token gives the hue, the intensity gives the energy.

| Value | night | dawn | day | dusk | Shifts with key? |
|---|---|---|---|---|---|
| hemisphere sky (= `sky1` of the key) | `#212136` | `#56498a` | `#8298cb` | `#2a2347` | **yes** |
| hemisphere ground (= `emberRoot` of the key; **day: `stone`**, so daylight rock is not warmed) | `#291b1a` | `#70332f` | `#4d5a6d` | `#3a1610` | **yes** |
| hemisphere intensity | 0.5 | 1.4 | 2.0 | 2.0 | — |
| directional colour | `ice@dusk` `#b6d3e0`, **pinned** (moon) | `sunHalo` `#ffd59a` | `ink` `#ece6dc` | `sunMid` `#f2a65e` | **no** (fixed / pinned) |
| directional intensity | 0.8 | 2.4 | 3.2 | 1.6 | — |
| fog colour (= `horizon` of the key) | `#3a293c` | `#a8619f` | `#a5b8d6` (blue-white haze) | `#5c3254` | **yes** |
| fog density (`FogExp2`, world 200, camera ≈ 200 out) | 0.0045 | 0.0035 | 0.0022 | 0.0035 | — |
| exposure | 1 | 1 | 1.25 | 1 | — |
| `uSnowLift` (terrain snow term) | 0.12 | 0.30 | 0.05 | 0.45 | — |
| `uStars` (dome star field) | 1 | 0.15 | 0 | 0.3 | — |
| `uEmber` (ember pool × rock-ramp root) | 1 | 0.5 | 0.12 | 1 | — |
| styleboard terrain p50 / max linear luminance (HDR, before exposure) | 0.0055 / 0.37 | 0.019 / 0.71 | 0.091 / 0.94 | 0.0055 / 0.66 | (measured, round 3) |

The R4 first-draft rig (hemi 0.35–1.1, dir 0.25–1.4, fog 0.004–0.010, ACES) rendered dusk terrain at p50 0.0022 linear: fully fog-coloured, no form. Rejected.

**Phase:**
- `kit.setPhase(elev, rising)` lerps every kit material colour, the env ramps, the terrain stop uniforms, the fog and the rig between keys. `elev` is the sun elevation in degrees.
  - `elev ≤ −12`: night.
  - `−12 … −4`: night → dawn (rising) or dusk (setting).
  - `−4 … 6`: dawn or dusk.
  - `6 … 15`: dawn/dusk → day.
  - `≥ 15`: day.
- The writer is **sky**, via `world.rig = {elev, rising}` (01). Until sky-full (W3) it is fixed at **dusk**.
- **The user approves the 4 keys from the swatch + styleboard screenshots** at the end of W0.5.

## World edge (no visible rim)

| Layer | Rule |
|---|---|
| Go `h.base` | beyond r = 78 m the height falls by 34 m (smoothstep 78 → 100) and by 30 m more out to the corners (100 → 141): the massif drops into the valley, no flat apron, no plane corners |
| Skirt | the mountain's closed skirt (cut) continues down from the dropped rim |
| Valley haze floor | kit-owned horizontal disc (r 900) at y = −16 in the key's `horizon` colour, fogged: the dropped rim dissolves into haze and meets the dome's horizon band. 1 draw, 48 tris |

## Kit factories

Each factory tags `userData.kit = '<factory>'`, plus `userData.kitToken` / `userData.kitIntensity` where it takes them. The gate rules are **per factory**.

| Factory | Builds | Gate rule |
|---|---|---|
| `lit(token\|ramp)` | `MeshStandardMaterial` (high) / `MeshLambertMaterial` (low), `flatShading: true`, fog on | flatShading true; no `map`; colour or `instanceColor` ∈ tokens/ramps; facet band |
| `glow(token, intensity)` | additive `MeshBasicMaterial`/`SpriteMaterial`, `depthWrite:false`, `fog:false`. Replaces pi-lab `halo.js`'s CanvasTexture | `color == kitToken × kitIntensity` (±1/255 per channel after dividing out the intensity); token ∈ fixed/semantic; `map` only the kit sprite; excluded from the luminance check |
| `line(token, intensity)` | additive `LineBasicMaterial`, `depthWrite:false`, `fog:false`, for `Line`/`LineSegments` (lightning, rain, ash lines, **St. Elmo corona**: ~11 short arcs from the summit, midpoint displacement depth 3, ~60 % fork once, re-seeded every 80–150 ms, `elmo` × 1.6 so it blooms, + faint core `glow(elmo, .3)`; never a sphere sprite) | same colour rule as `glow`; facet exempt |
| `points(token, size)` | `PointsMaterial` with a round sprite generated in kit | `map` only the kit sprite; colour ∈ tokens |
| `dome()` | sky `ShaderMaterial`, BackSide, gradient `sky0 → sky1 → horizon` of the current phase, **plus the star field** in the same shader (0 extra draws): hashed cells on the view sphere, fixed seed, `ink` × per-star 0.5–3, faded above the horizon, weight `uStars` per key (full night, faint dawn/dusk, 0 day). Sky owns it; decorative but true (stars are there); fixed orientation, sidereal rotation for Paris optional | uniforms written only by `setPhase`; one per scene |
| `terrain()` | `lit` + orchestrator shader patch: `uClip`, `uSnowline`, `uWobble`, `uHeat`, `uStale`, **`uFootprint`** (sampler), **`uVeins`** (capsule texture), **`uSnowLift`**, **`uEmber`** + `rock`/`snow`/`trail` stop uniforms, and a per-vertex **`bench`** attribute. Snow: `h > uSnowline` on faces with normal.y > .2, or any face 10 % above it → one continuous cap; adds unlit `snow × uSnowLift`. **Walkway:** colour = mix(rock, `trail`, smoothstep(.35, .75, bench)), never on snow; `bench` is Go's bench mask (06), so the edge is smooth, not face-stepped. **Veins (ore inlay):** ≤ 64 local-space capsule segments (a, b, radius tapering a→b; kit expands each relay capsule into a seeded jittered, forking polyline) in a float DataTexture; the fragment takes the 3D distance to them (object space), so veins are flush and unstretched on any slope (exact, so no triplanar needed); albedo → `gold` × .55, metalness .85, roughness .4 (dark metallic seam that glints under the directional), emissive `gold` × .12 + a **thin bright core** (inner 8–22 % of the radius) × .7; trunk radius ≈ .38 tapering to ≈ .1 on forks (`ref`: core `sunHalo` × 1.4). Low tier: surface veins off (cut face only) | as `lit`; patch only via kit (`customProgramCacheKey`); **`uFootprint` only ever holds the kit-owned footprint RT** |
| `stamp()` | `MeshBasicMaterial` for the offscreen footprint stamp scene (writes into the kit-owned 512² RGBA8 RT) | only in the stamp scene, never in the main scene; no `map` |
| `crystal()` | `lit` + crystal patch. `instanceColor` = `ramp('group')` (body tint). Per-instance `aHealth` (vec3) ∈ `ramp('health')` × per-instance `aGlow` scalar drives emissive | `instanceColor` rows ∈ `group`; `aHealth` rows ∈ `health`; `aGlow` ∈ [0, 1.5] |
| `lava()` | `lit` + lava patch: **instance colour also drives emissive** (`emissive = instanceColor × uLavaHeat`, **`uLavaHeat` capped at 1.6**), so hot blobs just cross bloom threshold 1 and the halo stays small up close | `instanceColor` ∈ `ramp('lava')`; excluded from the luminance check |
| `flow(token)` | `ShaderMaterial` for river ribbons: scrolled foam from **`uTime` only**, no `map`, fog on | colour ∈ water semantic (`water`, longhorn, local); only uniform written by the model: `uTime` (+ width/speed attributes) |
| `cave()` | natural cave mouth at Go-placed `caves[]` (3–5, on the walkway's inner cut wall where it is ≥ 7 m tall): an irregular jittered 13-gon (≈ 7 × 4 m, flat floor edge) seated on the wall face (polygon offset), unlit vertex gradient `deep` (centre) → `rock1` (rim, pow 4); one flattened `rock()` **brow** slab overhanging the top, 2–3 uneven `rock()` jambs low on the sides, 2–3 loose rocks on the threshold. No arch, no ring of stones, no hood above the path. A heightfield cannot hold an overhang, and a carved niche at 1.56 m cells only made a slot, so the mouth is a seated opening, not a hollow | `lit` rules for the rocks (band `rock`); mouth is unlit, facet-exempt |
| `puff(seed)` cluster | lump = `IcosahedronGeometry(1, 1)` with the base clamped flat (y ≥ −.25), instanced with non-uniform scale; colour per instance on `ramp('cloud')`. **Layer** clusters: 30–44 lumps of 1.8–5, y ×.8. **Mass** clusters (towering, over the summit): 60–120 lumps of 4.5–12, y ×.55, tapering upward | `lit` rules; facet band `cloud`; instance colours ∈ `ramp('cloud')` |
| `ramp(name)` | stop list + `kit.setRampAt(mesh, i, t)` (not a material) | every `instanceColor` ∈ ramp (stop or segment, 1/255) |
| `field(…)` | cut-face providers (W3, `cut.js`) | `map` allowed (data textures) |

**All factories:** `castShadow`/`receiveShadow` false and `renderer.shadowMap.enabled` false.

`lava()`, `crystal()`, `flow()`, `stamp()` and the `uFootprint` slot are orchestrator-owned and land in the **W1a seam** (`lava()` is needed by eruption in W1a; the others are interface stubs until W3/W4).

**Sky budget (styleboard):** 11 layer clusters + 2 mass clusters = 584 lumps × 80 tris ≈ 47k tris in 2 draws. Low tier: lumps at detail 0 (20 tris) ≈ 12k.

## Procedural helpers (kit)

| Helper | Geometry | Tris |
|---|---|---|
| `rock(seed)` | `IcosahedronGeometry(1, 1)`, seeded vertex jitter ±25 %, `toNonIndexed` | 80 |
| `puff(seed)` | 3–5 merged `IcosahedronGeometry(r, 0)` lumps, bottom flattened | ≤ 100 |
| `shard(seed)` | hex prism (`CylinderGeometry(r, .7r, h, 6)`) + seeded off-centre tip | ≤ 36 |

Variety comes from per-instance non-uniform scale (0.6–1.4 per axis) + rotation. One geometry per helper per model; scree's 2-draw budget rules out k meshes.

## Facet bands (median edge length, world units; world 200, `GRID` cell 1.5625, `ICE` cell 3.125)

Median is computed **per mesh, in world space, including instance matrices** (each instance's edges are measured after its matrix). A mesh may override its class with `userData.facet = [min, max]` (orchestrator-reviewed).

| Class | `facet:[min,max]` | Models |
|---|---|---|
| terrain | [1.2, 3.2] | mountain body (128² and 96²), eruption lava |
| ice | [2.4, 4.6] | glacier shell (64²) |
| slab | [10, 200] | bedrock slabs |
| rock | [0.15, 1.2] | scree, cairns |
| chunk | [0.1, 2.0] | glacier calving chunks (RoundedBox) |
| pebble | [0.05, 0.6] | toss pebble / snowball |
| ring | [0.5, 4.0] | cauldron sag ring |
| crystal | [0.3, 2.5] | summit |
| cloud | [0.8, 8.0] | sky layer + mass clusters, toy-clouds |
| cave | [0.15, 2.5] | cave hoods (rim rocks: band `rock`) |
| exempt | — | `glow`, `line`, `points`, `dome`, `field` (incl. strata), `flow`, `stamp` |

The bands are set in each `layout.js` profile. Only the orchestrator tunes them.

## Gate checks (style)

| Check | Rule |
|---|---|
| Colour membership | gate calls `setPhase` to an **exact key** (dusk, then day), then compares sRGB `getHexString()` per channel ±1/255 against that key's tokens/ramps |
| Rendered luminance (ceiling) | `uHeat = 0`, haze pass off, every emissive zeroed, `glow`/`line`/`points`/`lava` hidden. Render to the gate's **own `HalfFloatType` RT** and read it with `readRenderTargetPixels`. **Max Rec.709 luminance < 1.0** at dusk and day. If float readback is unsupported the gate **fails loudly** (never skips) |
| Rendered luminance (floor) | same render: **p50 luminance of terrain pixels at dusk > 0.004** (the one floor, here, in 04 and in `gate.mjs`; styleboard measured 0.0055–0.006, so ≈ 30 % headroom), so the scene is never black |
| Static source scan | grep `models/**/*.js` for `/#[0-9a-f]{6}\|0x[0-9a-f]{6}\|new (THREE\.)?Color\(\|TextureLoader\|GLTFLoader\|CanvasTexture\|onBeforeCompile/i` → fail on any hit |
| Rig/dome untouched | snapshot dome uniforms, light colours/intensities, fog and exposure before and after each model `step()` → fail on any change |
| `uFootprint` | holds only the kit RT |

## Bans
- **No GLBs** (no `GLTFLoader`, no KayKit). All geometry is procedural, from kit helpers or the model's own code.
- **No shadows** anywhere.
- **No textures** except in `glow`, `points`, `field`, the kit footprint RT and the kit vein capsule texture. No `clay.png`, and no `TextureLoader` outside kit.
- **No foreign lights.** The sandbox and the styleboard use the kit rig. Never use pi-lab `object.html`'s white lights.

## Freeze (2026-10-14)
The style was approved by M BN after four styleboard rounds and is now frozen. The frozen values are the tokens, the rig, the ramps, the factories, the shapes and the gate rules in this file.

| Reference | File |
|---|---|
| Approved board (4 keys × 4 orbit angles) | `plan/style/board.png` |
| Cave close-up | `plan/style/cave.png` |
| Aretē hero | `plan/style/board-ref.png` |
| Token generator and output | `plan/style/gen.mjs`, `plan/style/tokens.json` (kit.js imports these values) |
| Spike scene | `plan/style/board.html` (reference only; not shipped) |

Three items carry into the W1a seam. Each one is checked on the real kit render, not on the spike:
1. **Dusk caves:** a cave mouth must be legible at dusk. Use a rim catch-light from the existing rig or a `snowLift`-style lift on the cave rim, with no new light.
2. **Rim falloff:** the edge drop must not read as a mesa from any story or orbit camera. Use a softer falloff that blends into the valley haze.
3. **Framing:** the live and finale cameras must show the ember pool at the foot.

# 03-model-sky

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/sky/{index.js, fixture.json, test.js}` and pass `gate.mjs sky`.

## Wave
W1b min, W3 full

## Rules
**min:** gradient dome **with the star field** (dome shader, `uStars` per key; decorative but true, fixed orientation, Paris sidereal rotation optional), `world.clouds` floor from rx as a **cloud layer around and below the summit** (big cumulus clusters, not a few toys), stale dimming; writes `world.rig` at the dusk default. **full:** the real SunCalc sun (Paris) writes `world.rig = {elev, rising}`, which drives `kit.setPhase` (palette per time, rig, fog; 09). Visitor + steam clouds, moisture, `uSnowline`

## Technique
kit `dome()`; kit `puff(seed)` clusters (09), colours on `ramp('cloud')` (`cloud` base → `cloudLit` top), lit by the rig (white day, sun-tinted dawn/dusk, blue-grey night). Two sets: the **layer** around/below the summit (≈ 11 clusters × 30–44 small lumps) and the **mass**: 1–3 towering clusters over the summit (footprint ≥ massif width, flat base above the St Elmo corona so the summit stays visible). **The mass is the real cloud mass: its size and height come from net rx (02 cloud floor)**, and its base is the lightning bolt source. Budget: 2 InstancedMesh draws, ≈ 584 lumps × 80 tris ≈ 47k (low tier detail 0 ≈ 12k); big lumps keep the mass instance count low. Never creates lights: the rig is kit-owned

## Clip / cost
not clipped / 3–4 draws

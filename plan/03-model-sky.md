# 03-model-sky

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/sky/{index.js, fixture.json, test.js}` and pass `gate.mjs sky`.

## Wave
W1b min, W3 full

## Rules
**min:** gradient dome, `world.clouds` floor from rx, KayKit cloud instances, stale dimming. **full:** SunCalc sun + Hemisphere/Directional, visitor + steam clouds, moisture, `uSnowline`

## Technique
Dome ShaderMaterial, `cloud_big/small` InstancedMesh

## Clip / cost
not clipped / 3–4 draws

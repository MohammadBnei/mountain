# 03-model-magma

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/magma/{index.js, fixture.json, test.js}` and pass `gate.mjs magma`.

## Wave
W3

## Rules
Glow = `metrics.heat`. Dyke per `build` in flight toward `world.sites[app]`. Writes `temp`, emits `magma.dyke`

## Technique
Field provider: chamber + capsule dykes. **Cut closed: the ember pool** = a terrain-shader emissive term at the mountain foot (kit `uPool` centre/radii + `uPoolCol`, 09), conforming to the ground, intensity `.6 × max(metrics.heat, .45) × uEmber`, so it shows at idle (hero ellipse) and fades by day. 0 draws. W3; the W0.5 styleboard renders it

## Clip / cost
0–1 draw

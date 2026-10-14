# 03-model-glacier

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/glacier/{index.js, fixture.json, test.js}` and pass `gate.mjs glacier`.

## Wave
W3

## Rules
Renders `world:glacier` (Go: snowfall from commits, tongue advance per sync, calve on `app.gone`, **annual ash bands in the ice**). Applies `cauldron.sag`. `temp` → cosmetic `melt` + `steam` (does not change persisted mass)

## Technique
64² closed shell = heightSolid + `ice`. Prefab RoundedBox calving chunks → `ice.calve`

## Clip / cost
clipped / 2 draws, 9k

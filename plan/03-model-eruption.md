# 03-model-eruption

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/eruption/{index.js, fixture.json, test.js}` and pass `gate.mjs eruption`.

## Wave
W1a

## Rules
`ev:argo.sync{ok}` → lava flows toward the paired `world:deposit` (same id) and cools magma → stone. `ok:false` → ash puff, no land. Emits `lava.heat`

## Technique
≤200 icosa blobs InstancedMesh, steepest descent on `heightSolid`, `setColorAt` ramp

## Clip / cost
clipped / 3 draws, 5k

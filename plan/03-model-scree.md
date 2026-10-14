# 03-model-scree

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/scree/{index.js, fixture.json, test.js}` and pass `gate.mjs scree`.

## Wave
W3

## Rules
`pod.churn`: pebble per restart, boulder per OOM/CrashLoop, talus erodes when stable. Consumes `ice.calve`. Group `real`

## Technique
cannon Sphere/Box on the 128² heightSolid collider, sleep, ≤80 bodies, oldest → `h.talus`, SAP broadphase; KayKit rocks InstancedMesh; snap resting bodies to render height

## Clip / cost
not clipped / 2 draws

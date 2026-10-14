# 03-model-veins

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/veins/{index.js, fixture.json, test.js}` and pass `gate.mjs veins`.

## Wave
W3

## Rules
Only from `world:vein`. Thickness from the relay. `ref` → brighter

## Technique
**Cut face:** field provider, ≤32 capsules in a DataTexture, then a 64³ `Data3DTexture` bake. **Surface (flush, no stretch):** kit expands each capsule into a seeded jittered, tapering, forking polyline (≤ 64 segments total) in the kit capsule texture bound to terrain `uVeins`; the terrain fragment takes the exact 3D distance in object space (09), so seams stay thin and inlaid on steep faces. 0 draws. No tubes or meshes above the surface. Low tier: cut face only

## Clip / cost
0 draws

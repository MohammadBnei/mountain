# 03-model-veins

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/veins/{index.js, fixture.json, test.js}` and pass `gate.mjs veins`.

## Wave
W3

## Rules
Only from `world:vein`. Thickness from the relay. `ref` → brighter

## Technique
Field provider: ≤32 capsules in a DataTexture, then a 64³ `Data3DTexture` bake

## Clip / cost
0 draws

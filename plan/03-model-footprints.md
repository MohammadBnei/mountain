# 03-model-footprints

Layer: **surface**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/footprints/{index.js, fixture.json, test.js}` and pass `gate.mjs footprints`.

## Rules
Click-path on the snow; trails where many walk; rain and weather erode them

## Technique
512² RGBA8 stamp render target, **kit-owned**, sampled by the terrain `uFootprint` slot; the offscreen stamp scene uses kit `stamp()` materials. **The relay computes `weight` (erosion)**; the client stamps with it

## Surface-layer rules (shared)
(W4)

- Group `surface`. Placement only above `world.snowline`.
- The relay validates through `toy_<name>.go`, recomputes y from `heightSolid`, and quantises x/z.
- Visitor clouds are the only surface element that reaches the real layer: they feed the real cloud floor. Lightning stays driven by traffic only.

## Input
- A tool mode exists only in the finale with explore on.
- A surface-start drag goes to the tool. Two fingers always orbit.
- Terrain picking = analytic heightSolid ray-march.
- Built on pi-lab `main.js:217-307`.

---

# 03-model-cairns

Layer: **surface**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/cairns/{index.js, fixture.json, test.js}` and pass `gate.mjs cairns`.

## Rules
Stack a stone on a flat spot

## Technique
cannon Box collider under the rock mesh, solver 20, friction 0.8. Settle, then send `{pos, quat}`. Relay checks slope, snowline, reach, upright, rate. **Settle (W4):** the relay keeps the dropped cell, then the stone settles within 2 cells (≈ 3 m): onto the nearest cairn with room (7 stones max), else the nearest cell with |∇heightSolid| ≤ 0.7 (≈ 35°, a stone's angle of repose) above the snowline; none → `full` or `slope`. "Flat" is 0.7, not less, because the snow cap is steep (seed 1: ≈ 20 cells qualify at 0.7, 6 at 0.35). The client mirrors the settle (models/cairns `target`)

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

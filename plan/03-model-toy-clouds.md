# 03-model-toy-clouds

Layer: **surface**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/toy-clouds/{index.js, fixture.json, test.js}` and pass `gate.mjs toy-clouds`.

## Rules
Spawn, drift on `world.wind`, merge on contact. **3 distinct accounts within 1 h → rain** (surface only), then dissipate. Contributors credited by moderated handle

## Technique
Relay owns drift (`pos = f(spawn, ∫wind)`), merge and rain → `world:cloud` / `world:rain`. Client: puff InstancedMesh, rain `LineSegments`

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

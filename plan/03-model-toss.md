# 03-model-toss

Layer: **surface**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/toss/{index.js, fixture.json, test.js}` and pass `gate.mjs toss`.

## Rules
Slingshot drag; the pebble rests (W4: pebbles only. One draw call cannot hold the rock and snow ramps; snowballs would need `draws: 2` in the frozen layout.js)

## Technique
cannon Sphere; rest on sleep or after 6 s. Relay validation + max throw distance (`tossMax` 24 m, reject `throw`)

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

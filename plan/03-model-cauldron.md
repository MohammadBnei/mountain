# 03-model-cauldron

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/cauldron/{index.js, fixture.json, test.js}` and pass `gate.mjs cauldron`.

## Wave
W1b

## Rules
Sag depth by severity (from `world.state.alerts`, so it survives reload), steam column on critical, red under-glow. Over `breakMin` (30 min critical) → `jokulhlaup`. Emits `cauldron.sag` for glacier (W3) **and draws its own sag ring in W1**

## Technique
Ring mesh + `Points` steam (150–300, vertex-animated)

## Clip / cost
not clipped / 2 draws

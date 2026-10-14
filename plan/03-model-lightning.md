# 03-model-lightning

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/lightning/{index.js, fixture.json, test.js}` and pass `gate.mjs lightning`.

## Wave
W1b

## Rules
Glow = `rpsB`. Bolt on `spike` from the nearest cloud; **fallback: a fixed cloud height above the site** if `world.clouds` is empty. Red when `errB > 0`

## Technique
Midpoint-displacement ribbon about 150 ms; instanced additive glow quads (pi-lab `halo.js`)

## Clip / cost
not clipped / 3 draws

# 03-model-river

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/river/{index.js, fixture.json, test.js}` and pass `gate.mjs river`.

## Wave
W3

## Rules
Width/speed = `metrics.river` (public traffic bytes). Visual feeds: `melt`, `aquifer.seep`, `jokulhlaup`

## Technique
CatmullRom ribbon on heightSolid, scrolled UV + foam

## Clip / cost
not clipped / 1 draw

# 03-model-mountain

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/mountain/{index.js, fixture.json, test.js}` and pass `gate.mjs mountain`.

## Wave
W1a

## Rules
Renders `h.base` + `h.land` from Go. Bedrock = 3 slabs `h1..h3`: thickness = alloc, colour = ready/load. **W3 adds:** Voronoi cracks, GPU seam, quorum fault lines, subsidence (slab-only)

## Technique
`PlaneGeometry` 128² (96² on low) + skirt + bottom, `toNonIndexed`, flatShading. Partial `addUpdateRange` on deltas. Slabs InstancedMesh

## Clip / cost
clipped / 2 draws, 35k (21k) tris

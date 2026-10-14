# 03-model-mountain

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/mountain/{index.js, fixture.json, test.js}` and pass `gate.mjs mountain`.

## Wave
W1a

## Rules
Renders `h.base` + `h.land` from Go. Bedrock = 3 slabs `h1..h3`: thickness = alloc, colour = ready/load. **W3 adds:** Voronoi cracks, GPU seam, quorum fault lines, subsidence (slab-only)

## Technique
Hand-built non-indexed geometry (equivalent to `PlaneGeometry` 128² (96² on low) + `toNonIndexed`) + skirt + bottom at y −70 (3 m under the −67 worldcore rim), flatShading, kit `terrain({closed: true})` (DoubleSide; never mutate kit materials). Slabs hang 0.5 m below the body, 6–20 m thick, so `layout.js` yRange is [−90.5, 75]. A resync hello rebuilds caves: `kit.dispose(old)` then `kit.cave()`. Shape comes from Go `h.base` (06): broad massif, soft ridges, terraces, a **spiral walkway** carved as a ledge (≈ 2 turns, constant grade, fun to orbit; colour from the Go `bench` mask with the `trail` token, never jagged). **Caves:** kit `cave()` at Go `caves[]` (3–5 natural mouths seated on the walkway's tall inner cut wall: dark irregular opening, brow slab, jamb and threshold rocks; 1 InstancedMesh per part). **Edge:** the Go rim drop + kit valley haze floor hide the world edge from every story/orbit camera (09). On the cut face, the mountain-owned `caves` field provider draws each as a short irregular `deep` hollow (blob, not a tube). Collider stays the heightfield: **cave interiors not walkable in v1**.

## Clip / cost
clipped / 6 draws (body, slabs, 4 cave InstancedMesh parts: mouth, brow, jambs, threshold rocks), ≈ 36k (22k) tris

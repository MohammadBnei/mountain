# S3 visual: is a style spec cheap and expressive enough?

> Status: history (2026-11-24). Spike record.

## Question
Can a Mapbox-style spec (rules, expressions, seeded weighted variants) drive the look of trees and rocks, at a cost
the page does not notice, with the land and chunk data untouched?

## What was built
Branch `spike/s3-visual` in `ukubi-game` (worktree `scratchpad/spikes/s3-visual`, one local commit, not pushed).
- `cmd/game/websrc/world-style-spike.ts` (109 lines): the engine. Filter on kind and tags (kind from the kit name,
  tags from the registry: pack, `Far.Shape`, `wild`, `node`). Properties: asset, tint, scale, sway. Expressions:
  `zoom`, `tier`, `var` (season), `rand`, `case`, `match`, `+`, `*`, `interpolate` (zoom stops). Variants are weighted
  and picked by a hash of the position.
- `cmd/game/websrc/world-style-spike.json` (1039 bytes): two rules, trees and rocks.
- `world-terrain.ts` buildDecor: the hook, `STYLE_SPIKE` at :1000, applied in `draw` at :1121. `?style=spike`
  turns it on, `?season=autumn` sets the theme variable. Also `window.__styleSpike` for the bench.

## Numbers
This laptop, Chrome with `--use-angle=metal --enable-gpu --ignore-gpu-blocklist` plus vsync and frame cap off (on,
every row reads 16.7 ms). One grass spot, 6 s of rAF deltas, atlas on fixtures, real world. Script: `spikes/measure.mjs`.

| Measure | Flag off | Flag on (summer) | Flag on (autumn) |
|---|---|---|---|
| Desktop 1440x900, frame ms p50 / p95 | 2.1 / 3.2 | 1.9 / 3.2 | 2.0 / 3.2 |
| Phone 390x844 `q=low`, frame ms p50 / p95 | 1.7 / 2.9 | 1.8 / 2.9 | 1.7-2.3 / 2.7-3.0 |
| Decor drawn / triangles (desktop; low) | 275 / 149,962 (88 / 59,904) | same | same |
| Spec cost per tree, in place (275 items) | n/a | 3.3 ms total (one cold run), 0.3-0.7 ms typical, 1-12 us each | same |
| Spec cost per item, microbench (13,982 items x 20 runs) | n/a | 0.17-0.46 us | 0.18-0.46 us |
| Items the spec swaps to another asset | 0 | 117 (rocks only) | 3,740 |
| Land hash (`hashOf(spec)`) | 83d7645b | 83d7645b | 83d7645b |

Other numbers:
- Spec file: 1039 bytes JSON (423 gzipped). Engine source 4.3 KB.
- Dist: `world.js` 2,277,641 to 2,283,448 bytes (+5.8 KB, +1.8 KB gzipped) with the flag off, because the code ships
  always. Bun 1.4.4; a clean `bun run build` on origin/main left the dist identical before my edit.
- Frame differences are run-to-run noise (flag-off repeats vary 1.7 to 1.9 ms p50; one 2.3 ms low autumn run did not repeat).
- Screenshots: `scratchpad/spikes/s3-visual-shots/`. Autumn shows another tree variant and tint on the same spot.

## Findings
- **Cost holds.** Evaluation is sub-microsecond per item, run once per instance at `draw`, never per frame.
- **Data unchanged.** Land hash is equal on and off, summer and autumn. The spec only runs after the land exists.
- **Variants must already be in the registry.** `gid` (world-terrain.ts, `buildDecor`) only holds models named by
  `WORLD.Models`. The spec can only choose among those. A new look means a new registry row, which is append-only and
  Go-generated. The ADR's "spec references a visual asset" needs an asset list outside the registry.
- **Sway not built.** It needs a BatchedMesh material patch. Unmeasured.
- **Zoom is not live.** It is the item's distance from the camera target at draw time; items never re-evaluate.
- **Looks that drive behaviour today** (the hand-off list):
  - Trees are wood nodes (`registry.go:106`, `Node: wood(...)`). Harvest and hit are by item index
    (`world-fight.ts:255-261`), not mesh, so a variant swap is safe. The spec must never be a way to turn a node
    into a non-node: `kind` must come from data, not from a name.
  - Picking by mesh (`world-targets.ts:123`) is for buildings only. Decor budgets use the model's `tris`
    (`pickDecor`, world-terrain.ts:975), so a heavier variant (halloween pine, 1.88 high against 1.2) escapes the cap.
  - `bigTrees` (world-terrain.ts:962) and critters (`world-critters.ts:84`) test the model name for `tree`.
  - Minimap and far shapes read `Far` (`world-fardecor.ts:35`); the spike left them alone, so a tinted tree has an
    untinted far shape.
  - Footprint `R` and `H` are registry facts; a swapped variant has its own mesh size (rock A 0.069 high, B 0.22).

## What changes in the plan
- placement.md, visual axis: say variants are drawn from a declared asset list (name, tris, height, footprint
  class), not from the registry. The spec may swap only within one footprint class.
- Spec `kind` and `tags` come from composed data, never kit names (bigTrees, critters do that today).
- Roadmap W5: add a task for the budget: `pickDecor` must read the chosen variant's tris.
- Roadmap W5: far shapes and the minimap must read the same spec, or tint drifts between near and far.
- Drop `sway` from v1 unless a shader patch is costed; keep zoom stops for scale and tint at the far/near switch.

## Open questions
- Does one spec evaluation at `draw` stay cheap when 1,500 items churn each second on foot (high-tier cap)?
- Sway: shader patch cost on a BatchedMesh, and on the phone tier.

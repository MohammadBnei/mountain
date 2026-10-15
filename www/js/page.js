// Page load order (main.js, styleboard, gate page): the models that have landed, in draw order. Orchestrator-owned
// and NOT frozen: a lane's model joins here when it lands (gate page fails on a built model missing from it).
// Every model's profile is already in the frozen layout.js.
// strata (W3a): 0 draws, colours the cut face; the story opens the cut (P 3), the finale keeps it closed.
// W3b: the cut-face providers (aquifers, magma, veins; 0 draws, chained in layout.js `fields` order), then the glacier
// shell over the body, the summit crystals and the scree rocks.
// W3c: the river ribbon over the glacier/scree surface (1 draw), sky-full, shimmer last (0 draws: uniforms after the
// models it hazes).
// W4: the surface toys (toys.js SURFACE, layout.js order), before shimmer so its haze covers them.
export const page = ['mountain', 'strata', 'aquifers', 'magma', 'veins', 'glacier', 'summit', 'scree', 'river', 'eruption', 'sky', 'cauldron', 'lightning',
  'toy-clouds', 'cairns', 'footprints', 'toss', 'shimmer'];

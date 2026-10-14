// Layout: one profile per model (01 "Budgets"), every model of W1–W4. FROZEN W2 (FROZEN.sha256): a change is an
// announced orchestrator change. The page load order (which models have landed) lives in page.js, not here.
// Profile keys: tris, draws, peak{tris,draws}, stepMs, writes[], emits[], listens[], clipped,
// visibleFrom, goneFrom, footprint{x,z,r}, yRange[min,max], facet (class in FACET; a mesh may set
// userData.facet to another class or 'exempt'). Gate: budgets, writes/emits/listens, facet bands.

// Median world-space edge length bands (09 "Facet bands").
export const FACET = {
  terrain: [1.2, 3.2], ice: [2.4, 4.6], slab: [10, 200], rock: [0.15, 1.2], chunk: [0.1, 2.0], pebble: [0.05, 0.6],
  ring: [0.5, 4.0], crystal: [0.3, 2.5], cloud: [0.8, 8.0], cave: [0.15, 2.5], exempt: null,
};

// Facet bands that apply to the whole terrain-sized world (footprint = the world).
const WORLD = { x: 0, z: 0, r: 141.5 };
// Lowest mountain geometry: worldcore rim min −67 (h − 34 − 30 − 3 at the corners), body bottom 3 m under it (−70),
// bedrock slabs 0.5 m below that, up to 20 m thick (03-model-mountain) → −90.5.
const BEDROCK_Y = -70 - 0.5 - 20;

export const profiles = {
  mountain: {
    tris: 40000, draws: 8, peak: { tris: 40000, draws: 8 }, stepMs: 1,
    writes: [], emits: [], listens: ['world:deposit'],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [BEDROCK_Y, 75], facet: 'terrain',
  },
  eruption: {
    tris: 5000, draws: 3, peak: { tris: 5000, draws: 3 }, stepMs: 1,
    writes: [], emits: ['lava.heat'], listens: ['ev:argo.sync', 'world:deposit', 'world:site', 'magma.dyke'],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 80], facet: 'terrain', minCoverage: 0,
  },
  // W1b (03-model-cauldron/sky/lightning). The summit of the live seed sits at (0, 0), y ≈ 68.
  cauldron: { // own sag ring (ring band) + Points steam 150–300; reads world.state.alerts (survives reload)
    tris: 3000, draws: 2, peak: { tris: 3000, draws: 2 }, stepMs: 1,
    writes: [], emits: ['cauldron.sag', 'jokulhlaup', 'steam'], listens: ['ev:alert.fire', 'ev:alert.resolve'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: { x: 0, z: 0, r: 18 }, yRange: [30, 140], facet: 'ring', minCoverage: 0,
  },
  sky: { // sky-min: layer + mass cumulus (2 InstancedMesh draws, 584 lumps × 80 tris); the dome is the kit rig's.
    // sky-full (W3): SunCalc sun → rig, moisture from steam + melt, visitor + steam clouds (within the same budget)
    tris: 50000, draws: 4, peak: { tris: 50000, draws: 4 }, stepMs: 2,
    writes: ['clouds', 'rig', 'sun', 'moisture'], emits: [], listens: ['steam', 'melt'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-10, 200], facet: 'cloud',
  },
  lightning: { // glow from rpsB, St. Elmo corona (line + glow), bolt on spike from the mass base (world.clouds)
    tris: 2000, draws: 3, peak: { tris: 2000, draws: 3 }, stepMs: 1,
    writes: [], emits: [], listens: [],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 200], facet: 'exempt', minCoverage: 0,
  },
  // W3 (03-model-*). Field providers (strata, aquifers, magma, veins) draw through cut.js: 0 draws of their own.
  strata: { // platform bands + ash lines (cut face)
    tris: 0, draws: 0, peak: { tris: 0, draws: 0 }, stepMs: 0.5,
    writes: [], emits: [], listens: ['world:ash'],
    clipped: true, visibleFrom: 3, goneFrom: null, footprint: WORLD, yRange: [BEDROCK_Y, 75], facet: 'exempt', minCoverage: 0,
  },
  aquifers: { // one pool per metrics.storage pool (cut face); ≥ 85 % on a measured pool seeps
    tris: 0, draws: 0, peak: { tris: 0, draws: 0 }, stepMs: 0.5,
    writes: [], emits: ['aquifer.seep'], listens: [],
    clipped: true, visibleFrom: 2, goneFrom: null, footprint: WORLD, yRange: [BEDROCK_Y, 75], facet: 'exempt', minCoverage: 0,
  },
  magma: { // chamber + dykes (cut face), temp; the ember pool is the kit terrain term (uPool, 09)
    tris: 1000, draws: 1, peak: { tris: 1000, draws: 1 }, stepMs: 1,
    writes: ['temp'], emits: ['magma.dyke'], listens: ['ev:build', 'lava.heat'],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [BEDROCK_Y, 75], facet: 'exempt', minCoverage: 0,
  },
  veins: { // cut-face capsules + the kit terrain uVeins inlay
    tris: 0, draws: 0, peak: { tris: 0, draws: 0 }, stepMs: 0.5,
    writes: [], emits: [], listens: ['world:vein'],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [BEDROCK_Y, 75], facet: 'exempt', minCoverage: 0,
  },
  glacier: { // 64² closed shell + RoundedBox calving chunks (userData.facet 'chunk')
    tris: 9000, draws: 2, peak: { tris: 9000, draws: 2 }, stepMs: 1,
    writes: ['ice'], emits: ['melt', 'steam', 'ice.calve'], listens: ['world:glacier', 'cauldron.sag'],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: { x: 0, z: 0, r: 60 }, yRange: [0, 75], facet: 'ice', minCoverage: 0,
  },
  summit: { // one shard crystal per allowlisted site (rings at r 22 and 40 ± 4), y from world.sites
    tris: 7000, draws: 2, peak: { tris: 7000, draws: 2 }, stepMs: 1,
    writes: [], emits: [], listens: ['world:site', 'ev:argo.health', 'ev:replicas', 'ev:app.gone'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: { x: 0, z: 0, r: 50 }, yRange: [-10, 80], facet: 'crystal', minCoverage: 0,
  },
  scree: { // ≤ 80 cannon bodies, kit rock() in one InstancedMesh; oldest → h.talus
    tris: 8000, draws: 2, peak: { tris: 8000, draws: 2 }, stepMs: 2,
    writes: ['h.talus'], emits: [], listens: ['ev:pod.churn', 'ice.calve'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 75], facet: 'rock', minCoverage: 0,
  },
  river: { // CatmullRom ribbon on heightSolid, kit flow(water)
    tris: 4000, draws: 1, peak: { tris: 4000, draws: 1 }, stepMs: 1,
    writes: [], emits: [], listens: ['melt', 'aquifer.seep', 'jokulhlaup'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 75], facet: 'exempt', minCoverage: 0,
  },
  shimmer: { // uniforms only (kit uWobble / uHeat)
    tris: 0, draws: 0, peak: { tris: 0, draws: 0 }, stepMs: 0.5,
    writes: [], emits: [], listens: [],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 75], facet: 'exempt', minCoverage: 0,
  },
  // W4 surface layer (03 shared rules: finale only, above world.snowline; the relay owns poses)
  'toy-clouds': { // kit puff() InstancedMesh + rain LineSegments
    tris: 10000, draws: 2, peak: { tris: 10000, draws: 2 }, stepMs: 1,
    writes: [], emits: [], listens: ['world:cloud', 'world:rain'],
    clipped: false, visibleFrom: 5, goneFrom: null, footprint: WORLD, yRange: [0, 200], facet: 'cloud', minCoverage: 0,
  },
  cairns: { // stacked kit rock() stones, one InstancedMesh
    tris: 16000, draws: 1, peak: { tris: 16000, draws: 1 }, stepMs: 2,
    writes: [], emits: [], listens: ['world:mark', 'world:rain'],
    clipped: false, visibleFrom: 5, goneFrom: null, footprint: WORLD, yRange: [0, 80], facet: 'rock', minCoverage: 0,
  },
  footprints: { // kit stamp() into the kit footprint RT (offscreen): no main-scene draws
    tris: 0, draws: 0, peak: { tris: 0, draws: 0 }, stepMs: 1,
    writes: [], emits: [], listens: ['world:mark', 'world:rain'],
    clipped: false, visibleFrom: 5, goneFrom: null, footprint: WORLD, yRange: [0, 80], facet: 'exempt', minCoverage: 0,
  },
  toss: { // resting pebbles / snowballs, one InstancedMesh
    tris: 8000, draws: 1, peak: { tris: 8000, draws: 1 }, stepMs: 2,
    writes: [], emits: [], listens: ['world:mark'],
    clipped: false, visibleFrom: 5, goneFrom: null, footprint: WORLD, yRange: [0, 80], facet: 'pebble', minCoverage: 0,
  },
  // gate self-test fixtures (www/dev/fixtures/*.js)
  _test: {
    tris: 2000, draws: 4, peak: { tris: 2000, draws: 4 }, stepMs: 2,
    writes: [], emits: [], listens: [],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: { x: 0, z: 0, r: 4 }, yRange: [0, 4], facet: 'rock',
  },
  _terrain: { // fixture:terrain, the Go h.base rendered with kit terrain() + cave() (style carry items)
    tris: 40000, draws: 8, peak: { tris: 40000, draws: 8 }, stepMs: 2,
    writes: [], emits: [], listens: [],
    clipped: true, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 75], facet: 'terrain',
  },
};

// Cut-face field provider order (cut.js, W3). Page load order: page.js.
export const fields = ['strata', 'aquifers', 'magma', 'veins', 'caves'];

// Live finale camera (09 Freeze item 3): sways ±0.45 rad × min(1, aspect) around +z so the ember pool at the foot
// (kit U.uPool, (0, 66)) and the summit both stay in frame on landscape and portrait screens.
export const finale = { radius: 200, height: 10, look: [0, 35, 0], sway: 0.45, period: 160, fov: 50 };

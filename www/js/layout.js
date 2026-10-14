// Layout: one profile per model (01 "Budgets"). FROZEN W2; only the orchestrator edits it.
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
  sky: { // sky-min: layer + mass cumulus (2 InstancedMesh draws, 584 lumps × 80 tris); the dome is the kit rig's
    tris: 50000, draws: 4, peak: { tris: 50000, draws: 4 }, stepMs: 2,
    writes: ['clouds', 'rig'], emits: [], listens: ['steam'],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-10, 200], facet: 'cloud',
  },
  lightning: { // glow from rpsB, St. Elmo corona (line + glow), bolt on spike from the mass base (world.clouds)
    tris: 2000, draws: 3, peak: { tris: 2000, draws: 3 }, stepMs: 1,
    writes: [], emits: [], listens: [],
    clipped: false, visibleFrom: 0, goneFrom: null, footprint: WORLD, yRange: [-70, 200], facet: 'exempt', minCoverage: 0,
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

// Page load order (main.js) and cut-face field provider order (cut.js, W3).
export const page = ['mountain', 'eruption', 'sky', 'cauldron', 'lightning'];
export const fields = ['strata', 'aquifers', 'magma', 'veins', 'caves'];

// Live finale camera (09 Freeze item 3): sways ±0.45 rad × min(1, aspect) around +z so the ember pool at the foot
// (kit U.uPool, (0, 66)) and the summit both stay in frame on landscape and portrait screens.
export const finale = { radius: 200, height: 10, look: [0, 35, 0], sway: 0.45, period: 160, fov: 50 };

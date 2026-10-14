// Model contract (01). FROZEN W2 (FROZEN.sha256).
//
//   export default async function build(ctx) → Model
//   ctx   = { THREE, kit, world, plot: {x, z, r, yRange}, params, rng, name }
//           world is world.as(name): reads are shared, write()/emit() are bound to this model.
//   Model = { root,                      THREE.Object3D, every material from kit
//             params,                    the tunables (sandbox sliders)
//             pose(P),                   pure given the world snapshot + model state
//             step(dt, t),               fixed 1/30 s, allocation-free
//             on?: {[busType]: fn},      subscribed by the host (world.on), listed in the profile's listens[]
//             field?: {glsl, uniforms},  cut-face provider (W3)
//             pick?(ray) → card | null,
//             busy() → bool,             true while animating (the host keeps stepping)
//             dispose() }
//
// models/<name>/test.js:
//   export default async function test({ model, world, inject, step, fixture, assert }) — throws on failure.
//   inject(type, payload): 'ev:<kind>' and 'world:<kind>' go through world.applyFrame, others through world.emit.
//   step(n, dt = 1/30): world.step + model.step n times.  fixture = models/<name>/fixture.json.

export function mulberry32(a) {
  return () => { a |= 0; a = (a + 0x6d2b79f5) | 0; let t = Math.imul(a ^ (a >>> 15), 1 | a); t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t; return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
}

export function makeCtx({ THREE, kit, world, name, profile, params = {}, seed = 1 }) {
  return { THREE, kit, world: world.as(name), name, params, rng: mulberry32(seed), plot: { ...profile.footprint, yRange: profile.yRange } };
}

/** Shape check of a built model; returns a list of problems (empty = ok). */
export function checkModel(m, profile) {
  const p = [];
  if (!m || typeof m !== 'object') return ['build() returned no model'];
  if (!m.root?.isObject3D) p.push('root is not an Object3D');
  for (const f of ['pose', 'step', 'busy', 'dispose']) if (typeof m[f] !== 'function') p.push(`missing ${f}()`);
  if (m.params === undefined) p.push('missing params');
  for (const t of Object.keys(m.on || {})) if (profile && !profile.listens.includes(t)) p.push(`on.${t} not in profile listens[]`);
  return p;
}

/** Subscribe a model's on{} handlers; returns the unsubscribe. */
export function wire(model, world) {
  const offs = Object.entries(model.on || {}).map(([t, fn]) => world.on(t, fn));
  return () => offs.forEach((off) => off());
}

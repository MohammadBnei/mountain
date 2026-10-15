// shimmer behaviour (03-model-shimmer): strength follows metrics.cpu (eased), 0 at idle so the post haze pass is off;
// desktop: uHaze + uMask, a sphere centred over the GPU slab (h1) that swells with h1's load; low tier: uWobble only.
// Never uHeat (magma's), no draws.
import { U, QUALITY } from '../../js/kit.js';

export default async function test({ model, world, step, fixture, assert }) {
  const M = U.uMask.value, heat = U.uHeat.value, m0 = world.metrics;
  const set = (o) => world.write('metrics', 'socket', { ...m0, ...o });
  let wob = 0;
  const run = (n) => { wob = 0; for (let i = 0; i < n; i++) { step(1); wob = Math.max(wob, Math.abs(U.uWobble.value)); } };
  try {
    assert(model.root.children.length === 0, 'no draws');
    set(fixture.idle);
    run(300);
    assert(U.uHaze.value === 0 && U.uWobble.value === 0, `idle → haze ${U.uHaze.value}, wobble ${U.uWobble.value} (exactly 0: post haze off)`);

    set(fixture.busy);
    run(300);
    if (QUALITY === 'low') {
      assert(wob > 0.2 && U.uHaze.value === 0, `low: cpu .8 → wobble peak ${wob.toFixed(3)}, no haze (${U.uHaze.value})`);
    } else {
      const r = M.w;
      assert(U.uHaze.value > 1 && U.uWobble.value === 0, `desktop: cpu .8 → haze ${U.uHaze.value.toFixed(3)}, wobble ${U.uWobble.value}`);
      assert(Math.abs(M.x - (-100 + 127 * 200 / 128 / 6)) < 1e-6 && Math.abs(M.z - (-100 + 127 * 200 / 128 / 2)) < 1e-6 && r > 0,
        `mask over the h1 slab (${M.toArray().map((v) => v.toFixed(2))})`);
      set(fixture.cool);
      run(300);
      assert(M.w < r * 0.8 && U.uHaze.value > 1, `h1 cooler, same cpu → smaller mask ${r.toFixed(1)} → ${M.w.toFixed(1)}, haze kept`);
    }
    assert(model.busy() === false, 'settled → not busy');

    set(fixture.idle);
    run(450);
    assert(U.uHaze.value === 0 && U.uWobble.value === 0, `back to idle → haze ${U.uHaze.value}, wobble ${U.uWobble.value}`);
    assert(U.uHeat.value === heat, `uHeat untouched (${heat} → ${U.uHeat.value})`);
  } finally { world.write('metrics', 'socket', m0); }
}

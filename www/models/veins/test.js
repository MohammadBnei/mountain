// veins behaviour (03-model-veins): the hello's veins draw at build (a reload); world:vein adds one, a re-fired fp keeps
// its seam and the thicker radius, ref brightens; ≤ 32 capsules on the cut face, ≤ 64 surface segments, the expansion
// is seeded (same fp → same seam) and its new joints sit on the surface; a new world.state resyncs; the cut chain has veins.
import { U } from '../../js/kit.js';
import { CUT } from '../../js/shaders.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const u = model.field.uniforms, cap = u.uVeinCap.value.image.data, kv = U.uVeins.value.image.data, s0 = world.state, n0 = s0.veins.length;
  const thick = (i) => cap[i * 4 + 3], ref = (i) => cap[(32 + i) * 4 + 3];
  assert(n0 === 3 && u.uVeinCapN.value === n0, `hello.state.veins → ${n0} cut capsules at build (${u.uVeinCapN.value})`);
  assert(model.caps.length === n0 * 6 && U.uVeinN.value === n0 * 6, `3 veins → 6 surface segments each (trunk 4 + 2 forks): ${U.uVeinN.value}`);
  const az = (p, q) => Math.cos(Math.atan2(p[0], p[2]) - Math.atan2(q[0], q[2])) > 1 - 1e-9, v0 = s0.veins[0]; // same azimuth
  assert(az(model.caps[0].a, v0.a) && az(model.caps[3].b, v0.b), 'the trunk runs from the relay a to the relay b (azimuths kept)');
  for (const c of model.caps) for (const p of [c.a, c.b]) {
    const d = world.heightAt(p[0], p[2]) - p[1];
    assert(d >= 0 && d < 1, `every joint is inlaid just under the surface (sunk ${d.toFixed(2)} m at ${p.map((x) => x.toFixed(1))})`);
  }
  assert(model.caps.every((c) => c.ra > c.rb || c.rb === 0.1) && model.caps.every((c) => c.ra <= 0.5 && c.rb >= 0.1), 'radii taper a→b, trunk ≤ .5, forks → .1');
  const seam0 = JSON.stringify(model.caps);

  // a replay / hello: a new state with the same veins gives the same seams; one without veins clears both
  world.write('state', 'socket', { ...s0, veins: [] });
  step(1);
  assert(u.uVeinCapN.value === 0 && U.uVeinN.value === 0, 'a resync to a state without veins clears the cut face and the inlay');
  world.write('state', 'socket', { ...s0, veins: s0.veins.slice() });
  step(1);
  assert(JSON.stringify(model.caps) === seam0, 'the fork expansion is seeded by fp: same veins → same seams');

  // a new vein, ref: brighter core on both the cut face and the inlay
  inject('world:vein', fixture.vein);
  assert(u.uVeinCapN.value === 4 && ref(3) === 1 && ref(0) === 0, `world:vein → 4 capsules, the new one ref (${u.uVeinCapN.value}, ${ref(3)})`);
  assert(U.uVeinN.value === 24 && kv[(128 + 23) * 4] === 1 && kv[128 * 4] === 0, 'its surface segments carry ref into uVeins');
  const r1 = model.caps[18].ra;
  // a re-fire: the seam stays, the thicker radius wins
  inject('world:vein', fixture.refire);
  assert(u.uVeinCapN.value === 4 && cap[3 * 4] === Math.fround(model.caps[18].a[0]) && thick(3) === Math.fround(fixture.refire.thick), `re-fire keeps the seam, thick ${thick(3)}`);
  assert(az(model.caps[18].a, fixture.vein.a) && model.caps[18].ra > r1, 'the inlay keeps the seam and thickens');
  inject('world:vein', { ...fixture.refire, thick: 0.3 });
  assert(thick(3) === Math.fround(fixture.refire.thick), 'a thinner re-fire does not shrink it');

  // overflow: ≤ 32 veins (oldest dropped), ≤ 64 segments (2 each: no forks)
  for (let i = 0; i < fixture.overflow; i++) inject('world:vein', { ...fixture.vein, fp: `of${i}`, ref: false });
  assert(u.uVeinCapN.value === 32 && U.uVeinN.value === 64 && model.caps.length === 64, `≤ 32 capsules, ≤ 64 segments (${u.uVeinCapN.value}, ${model.caps.length})`);
  assert(world.state.veins[0].fp === `of${fixture.overflow - 32}`, 'the oldest veins went first');

  world.write('state', 'socket', s0); // leave the hello's veins for the after-test render
  step(1);
  assert(JSON.stringify(model.caps) === seam0, 'back to the hello seams');
  assert(CUT.key === 'cap:veins,caves' && CUT.pars.includes('c = veinsField(p, c);'), `cut chain veins → caves (${CUT.key})`);
}

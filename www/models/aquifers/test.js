// aquifers behaviour (03-model-aquifers): one pool per metrics.storage pool + the garage and pigsty springs; size follows
// capB; longhorn level = usedB / 20 and shimmers, local and springs are unmeasured (full, static) and local sits low over
// its host's slab; a measured pool at ≥ 85 % seeps and emits aquifer.seep once per onset (not for a re-sent frame, again
// after it drains and refills); the cut chain runs aquifers between strata and the caves.
import { CUT } from '../../js/shaders.js';

export default async function test({ model, world, step, fixture, assert }) {
  const u = model.field.uniforms, A = u.uAqA.value, B = u.uAqB.value, seeps = [];
  const off = world.on('aquifer.seep', (p) => seeps.push(p));
  const metrics = (pools) => world.write('metrics', 'socket', { ...world.metrics, storage: { ...fixture.storage, pools } }); // the test plays socket
  const ps = fixture.storage.pools, at = (id) => ps.findIndex((p) => p.id === id);

  assert(u.uAqN.value === (world.metrics?.storage?.pools.length ?? -2) + 2, `hello pools + 2 springs at build (${u.uAqN.value})`);
  metrics(ps);
  step(1);
  assert(u.uAqN.value === ps.length + 2, `${ps.length} pools + garage + pigsty (${u.uAqN.value})`);
  ps.forEach((p, i) => {
    const m = p.usedB !== null;
    assert(B[i].z === (m ? 1 : 0) && Math.abs(B[i].x - (m ? p.usedB / 20 : 1)) < 1e-6, `${p.id}: level ${m ? p.usedB / 20 : 'unmeasured (full)'} (${B[i].x}, measured ${B[i].z})`);
    assert(B[i].y === (p.class === 'longhorn' ? 0 : 1), `${p.id}: kind ${p.class} (${B[i].y})`);
  });
  for (const i of [ps.length, ps.length + 1]) assert(B[i].y === 2 && B[i].z === 0, `spring ${i}: unmeasured`);
  assert(A[at('other-local-h3')].z > A[at('wird-2')].z && A[at('mountain-1')].z > A[at('editable-blog-1')].z, 'bigger capB → bigger pool');
  assert(A[at('other-longhorn')].w > A[at('other-local-h1')].w, 'longhorn deep, local shallow');
  const h1 = [at('wird-2'), at('other-local-h1')].map((i) => A[i]), h3 = A[at('other-local-h3')];
  assert(h1.every((a) => a.x < -30 && a.y < 4) && h3.x > 30 && h3.y < 4 && Math.abs(h1[0].x - h1[1].x) > 4, `local pools low (proxmox band) over their host slab, side by side (${h1.map((a) => a.x)} | ${h3.x})`);

  const want = ps.filter((p) => p.usedB >= 17).map((p) => p.id).sort();
  assert(JSON.stringify(seeps.map((s) => s.pool).sort()) === JSON.stringify(want), `≥ 85 % measured pools seep once each (${JSON.stringify(seeps)})`);
  assert(seeps.every((s) => Number.isFinite(s.x) && Number.isFinite(s.z)), 'seep carries x, z');
  ps.forEach((p, i) => assert(B[i].w === (want.includes(p.id) ? 1 : 0), `${p.id}: seep streak ${B[i].w}`));
  const t0 = u.uAqT.value;
  metrics(ps); // the next poll, same buckets
  step(30);
  assert(seeps.length === want.length, `a re-sent frame does not re-emit (${seeps.length})`);
  assert(u.uAqT.value > t0, 'measured water ripples on the model clock');

  const drained = ps.map((p) => (p.id === 'mountain-1' ? { ...p, usedB: fixture.drained } : p));
  metrics(drained);
  step(1);
  assert(seeps.length === want.length && B[at('mountain-1')].w === 0, 'drained below 85 %: no seep, no emit');
  metrics(ps);
  step(1);
  assert(seeps.length === want.length + 1 && seeps.at(-1).pool === 'mountain-1', `refilled: seeps again (${JSON.stringify(seeps.at(-1))})`);
  off();

  assert(/aquifers/.test(CUT.key) && CUT.pars.indexOf('c = aquifersField(p, c);') < CUT.pars.indexOf('c = cavesField(p, c);'), `cut chain … aquifers → caves (${CUT.key})`);
}

// scree behaviour (03-model-scree): a pebble per restart and a boulder per OOM / CrashLoop fall at the app's site,
// tumble downhill and come to rest on the terrain (snapped, asleep); ice.calve drops a pale boulder where it calved;
// past 80 bodies the oldest go to talus (heaps + h.talus); talus holds while churn goes on and erodes once stable;
// pick → the churn card.
import * as THREE from 'three';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const live = model.root.getObjectByName('scree'), talus = model.root.getObjectByName('talus');
  const M = new THREE.Matrix4(), P = new THREE.Vector3(), Q = new THREE.Quaternion(), S = new THREE.Vector3();
  // s 0 = a retired slot (scale 0: decompose reads a singular matrix as scale 1)
  const at = (i) => { live.getMatrixAt(i, M); M.decompose(P, Q, S); return { x: P.x, y: P.y, z: P.z, s: M.determinant() ? Math.max(S.x, S.y, S.z) : 0 }; };
  const sum = () => world.fields['h.talus'].reduce((a, b) => a + b, 0);
  const settle = () => { for (let k = 0; k < 60 && model.busy(); k++) step(30); }; // ≤ 60 s (bodies sleep by 25 s)
  const { site } = fixture;
  assert(live.count === 0 && talus.count === 0 && !model.busy(), 'starts empty (live only, nothing persisted)');

  inject('world:site', site);
  inject('ev:pod.churn', fixture.restarts);
  step(1);
  assert(live.count === 3, `3 restarts → 3 rocks (${live.count})`);
  for (let i = 0; i < 3; i++) {
    const r = at(i);
    assert(r.s < 0.8, `restart → pebble (scale ${r.s.toFixed(2)})`);
    assert(Math.hypot(r.x - site.x, r.z - site.z) < 3, `falls at the app's site (${r.x.toFixed(1)}, ${r.z.toFixed(1)})`);
  }
  const y0 = at(0).y;
  step(15);
  assert(at(0).y < y0, `falls (${y0.toFixed(2)} → ${at(0).y.toFixed(2)})`);
  settle();
  assert(!model.busy(), 'every body sleeps');
  for (let i = 0; i < 3; i++) {
    const r = at(i);
    if (!r.s) continue; // rolled off the world
    assert(Math.abs(r.y - world.heightAt(r.x, r.z)) < 1, `rests snapped on the terrain (${(r.y - world.heightAt(r.x, r.z)).toFixed(2)} m)`);
    assert(world.heightAt(r.x, r.z) <= world.heightAt(site.x, site.z) + 0.5, 'came to rest at or below the site');
  }
  const kept = [0, 1, 2].map(at).filter((x) => x.s), r = kept[0], o = new THREE.Vector3(r.x, r.y + 50, r.z), card = model.pick(new THREE.Ray(o, new THREE.Vector3(0, -1, 0)));
  assert(card?.kind === 'scree' && Number.isFinite(card.d) && card.d < 50 && card.churn === kept.length, `pick a pebble → the churn card, rocks still on the world (${JSON.stringify(card)})`);
  assert(model.pick(new THREE.Ray(o, new THREE.Vector3(0, 1, 0))) === null, 'pick into the sky → null');

  inject('ev:pod.churn', fixture.oom);
  step(1);
  assert(live.count === 5 && at(3).s > 0.8 && at(4).s > 0.8, `OOM + CrashLoop → 2 boulders (${live.count}, ${at(3).s.toFixed(2)}, ${at(4).s.toFixed(2)})`);
  inject('ice.calve', fixture.calve);
  step(1);
  const c = at(5);
  assert(live.count === 6 && c.s > 0.8 && Math.hypot(c.x - fixture.calve.x, c.z - fixture.calve.z) < 3, 'ice.calve → a boulder where it calved');
  assert(live.userData.kitRampT[5] === 1, 'the calved boulder is the palest rock (ramp t 1)');
  settle();

  for (let k = 0; k < fixture.flood.n; k++) { inject('ev:pod.churn', fixture.flood); step(10); }
  assert(live.count === 80, `≤ 80 bodies (${live.count})`);
  assert(talus.count > 0 && sum() > 0, `past the cap the oldest go to talus (${talus.count} heaps, h.talus Σ ${sum().toFixed(2)})`);
  step(90);
  const s0 = sum();
  step(25, 10); // 250 s < STABLE: no erosion yet
  assert(Math.abs(sum() - s0) < 1e-6, `talus holds while churn is recent (${s0.toFixed(3)} → ${sum().toFixed(3)})`);
  step(60, 60); // an hour of quiet
  const s1 = sum();
  assert(s1 < s0, `talus erodes when stable (${s0.toFixed(3)} → ${s1.toFixed(3)})`);
  inject('ev:pod.churn', fixture.restarts);
  step(25, 10);
  assert(sum() >= s1 - 1e-6, 'new churn stops the erosion');
  step(200, 120);
  assert(talus.count === 0 && sum() === 0, `talus erodes away (${talus.count} heaps, Σ ${sum()})`);
}

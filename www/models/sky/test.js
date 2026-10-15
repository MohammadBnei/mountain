// sky behaviour (03-model-sky): the real Paris sun → world.sun + world.rig (night / dawn / day / dusk keys, a new rig
// object only when the sun moved); rx up → the cloud mass grows (and a second tower joins); visitor clouds feed the
// floor and are listed; steam rises as a puff and wets world.fields.moisture (melt too), both fade; world.clouds holds
// the mass base above the summit first (lightning's bolt source); stale → kit uStale eases to 1, back on fresh.
import * as THREE from 'three';
import { U, keyMix } from '../../js/kit.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const mass = model.root.getObjectByName('mass'), layer = model.root.getObjectByName('layer'), steam = model.root.getObjectByName('steam');
  const frame = (o) => world.applyFrame({ ...fixture.base, ...o });
  const box = new THREE.Box3(), M = new THREE.Matrix4(), V = new THREE.Vector3();
  const size = () => { // world-space extent of the mass lump centres (instance matrices)
    box.makeEmpty();
    for (let i = 0; i < mass.count; i++) box.expandByPoint(V.setFromMatrixPosition(mass.getMatrixAt(i, M)));
    return box.getSize(V).x * box.getSize(V).y;
  };
  const [low, high, stale, fresh] = fixture.timeline;

  // sun: the wall clock, stubbed per key instant
  const real = Date.now;
  let now = 0;
  Date.now = () => now;
  try {
    for (const [key, iso] of Object.entries(fixture.clock)) {
      now = Date.parse(iso);
      step(31);
      const { a, b, w } = keyMix(world.rig.elev, world.rig.rising), got = w > 0.5 ? b : a;
      assert(got === key && world.sun.elev === world.rig.elev, `${iso}: rig ${JSON.stringify(world.rig)} → key ${got}, want ${key}`);
    }
    assert(Math.abs(world.sun.az - 300) < 10, `dusk sun in the west-north-west (az ${world.sun.az})`);
    const rig = world.rig, sun0 = world.sun;
    now += 20e3; // the sun moves ≈ 0.07° in 20 s: no new rig (setPhase re-lerps the whole kit)
    step(31);
    assert(world.rig === rig && world.sun !== sun0, 'small sun move: world.sun updates, world.rig object kept');
  } finally { Date.now = real; }

  assert(world.clouds.length >= 12, `world.clouds listed (${world.clouds.length})`);
  frame(low);
  step(750);
  assert(!model.busy(), 'settled at low rx');
  const c0 = world.clouds[0], a0 = size(), n0 = mass.count, y0 = c0.y, r0 = c0.r;
  assert(world.clouds.length >= 12 && c0.mass && !world.clouds.at(-1).mass, `world.clouds: mass first, then the layer (${world.clouds.length})`);
  assert(c0.y > world.hmax + 8, `mass base ${c0.y} above the summit ${world.hmax} + corona`);
  assert(layer.count > 300, `cloud layer is a layer, not a few toys (${layer.count} lumps)`);

  // visitor clouds feed the real floor (R3) and are listed in world.clouds
  const nc = world.clouds.length;
  for (const v of fixture.visitors) inject('world:cloud', v);
  assert(world.clouds.length === nc + 3 && world.clouds.at(-1).y === fixture.visitors[2].pos[1], 'visitor clouds listed in world.clouds');
  step(750);
  assert(size() > a0 * 1.2 && mass.count > n0, `3 visitor clouds grow the mass (${a0.toFixed(0)} → ${size().toFixed(0)} m², ${mass.count} lumps)`);
  for (const v of fixture.visitors) inject('world:cloud', { ...v, state: 'gone' });
  step(750);
  assert(world.clouds.length === nc && mass.count === n0, 'gone visitor clouds leave the floor');

  frame(high);
  step(1);
  assert(model.busy(), 'busy while the mass grows');
  step(750);
  const a1 = size();
  assert(a1 > a0 * 1.5 && mass.count > n0, `rx 0.2 → 0.9: mass grows (${a0.toFixed(0)} → ${a1.toFixed(0)} m², ${n0} → ${mass.count} lumps)`);
  assert(world.clouds[0].r > r0 && world.clouds.filter((c) => c.mass).length === 2, 'world.clouds: second mass tower listed');
  assert(Math.abs(world.clouds[0].y - y0) < 1e-6, 'mass base stays flat at summit + corona clearance');

  // steam: one puff rises over the source, moisture splat; a second steam within 2 s only wets; both fade
  const { x, z } = fixture.steam, k = world.cell(x, z), moist = world.fields.moisture, nc2 = world.clouds.length;
  inject('steam', fixture.steam);
  assert(steam.count === 8 && moist[k] > 0.2 && world.clouds.length === nc2 + 1, `steam → one puff (${steam.count} lumps), moisture ${moist[k]}`);
  const ya = V.setFromMatrixPosition(steam.getMatrixAt(0, M)).y;
  step(150);
  const yb = V.setFromMatrixPosition(steam.getMatrixAt(0, M)).y;
  assert(yb > ya + 4 && model.busy(), `steam puff rises (${ya.toFixed(1)} → ${yb.toFixed(1)})`);
  inject('steam', fixture.steam);
  inject('steam', fixture.steam);
  assert(steam.count === 16, `steam throttled to one puff per 2 s (${steam.count / 8} puffs)`);
  inject('melt', { x: -40, z: 0, vol: 1 });
  assert(moist[world.cell(-40, 0)] > 0 && steam.count === 16, 'melt wets the air, no puff');
  const m0 = moist[k];
  step(30 * 60);
  assert(steam.count === 0 && world.clouds.length === nc2 && !model.busy(), `steam puffs fade (${steam.count} lumps left)`);
  assert(moist[k] < m0 * 0.7 && moist[k] > m0 * 0.4, `moisture decays (${m0.toFixed(3)} → ${moist[k].toFixed(3)} in 60 s)`);

  frame(stale);
  step(60);
  assert(world.stale != null && U.uStale.value > 0.85, `stale → dims (uStale ${U.uStale.value})`);
  frame(fresh);
  step(150);
  assert(world.stale == null && U.uStale.value < 0.05, `fresh → undims (uStale ${U.uStale.value})`);
}

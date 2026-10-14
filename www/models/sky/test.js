// sky behaviour (03-model-sky): rx up → the cloud mass grows (and a second tower joins); world.clouds holds the mass
// base above the summit first (lightning's bolt source); stale → kit uStale eases to 1 (dome/stars/terrain dim), back on fresh.
import * as THREE from 'three';
import { U } from '../../js/kit.js';

export default async function test({ model, world, step, fixture, assert }) {
  const mass = model.root.getObjectByName('mass'), layer = model.root.getObjectByName('layer');
  const frame = (o) => world.applyFrame({ ...fixture.base, ...o });
  const box = new THREE.Box3(), M = new THREE.Matrix4(), V = new THREE.Vector3();
  const size = () => { // world-space extent of the mass lump centres (instance matrices)
    box.makeEmpty();
    for (let i = 0; i < mass.count; i++) box.expandByPoint(V.setFromMatrixPosition(mass.getMatrixAt(i, M)));
    return box.getSize(V).x * box.getSize(V).y;
  };
  const [low, high, stale, fresh] = fixture.timeline;

  assert(world.rig.elev === 0 && world.rig.rising === false, `world.rig at the dusk default (${JSON.stringify(world.rig)})`);
  frame(low);
  step(750);
  assert(!model.busy(), 'settled at low rx');
  const c0 = world.clouds[0], a0 = size(), n0 = mass.count, y0 = c0.y, r0 = c0.r;
  assert(world.clouds.length >= 12 && c0.mass && !world.clouds.at(-1).mass, `world.clouds: mass first, then the layer (${world.clouds.length})`);
  assert(c0.y > world.hmax + 8, `mass base ${c0.y} above the summit ${world.hmax} + corona`);
  assert(layer.count > 300, `cloud layer is a layer, not a few toys (${layer.count} lumps)`);
  frame(high);
  step(1);
  assert(model.busy(), 'busy while the mass grows');
  step(750);
  const a1 = size();
  assert(a1 > a0 * 1.5 && mass.count > n0, `rx 0.2 → 0.9: mass grows (${a0.toFixed(0)} → ${a1.toFixed(0)} m², ${n0} → ${mass.count} lumps)`);
  assert(world.clouds[0].r > r0 && world.clouds.filter((c) => c.mass).length === 2, 'world.clouds: second mass tower listed');
  assert(Math.abs(world.clouds[0].y - y0) < 1e-6, 'mass base stays flat at summit + corona clearance');

  frame(stale);
  step(60);
  assert(world.stale != null && U.uStale.value > 0.85, `stale → dims (uStale ${U.uStale.value})`);
  frame(fresh);
  step(150);
  assert(world.stale == null && U.uStale.value < 0.05, `fresh → undims (uStale ${U.uStale.value})`);
}

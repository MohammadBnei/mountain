// mountain behaviour (03-model-mountain): a deposit raises heightSolid AND the rendered body at the site;
// slabs follow hosts (thickness = alloc, colour = ready/load ramp).
import * as THREE from 'three';
import { rampColor, color, QUALITY } from '../../js/kit.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const body = model.root.getObjectByName('body'), slabs = model.root.getObjectByName('slabs');
  const d = fixture.deposit, cell = (d.z0 + 1) * 128 + d.x0 + 1, x = -100 + ((d.x0 + 1) * 200) / 128, z = -100 + ((d.z0 + 1) * 200) / 128;
  const ray = new THREE.Raycaster(new THREE.Vector3(x, 500, z), new THREE.Vector3(0, -1, 0));
  const meshY = () => { body.updateMatrixWorld(); return ray.intersectObject(body)[0].point.y; };

  const tol = QUALITY === 'low' ? 0.6 : 0.05; // 96² low tier interpolates between cells
  const h0 = world.fields.heightSolid[cell], y0 = meshY();
  assert(Math.abs(y0 - h0) < tol, `body matches heightSolid before (${y0} vs ${h0})`);
  inject('world:deposit', d);
  step(1);
  assert(model.busy(), 'busy while the deposit ramps');
  step(100);
  const h1 = world.fields.heightSolid[cell], y1 = meshY();
  assert(Math.abs(h1 - h0 - 3) < 0.02, `heightSolid at the site rose 3 m (${h0} -> ${h1})`);
  assert(Math.abs(y1 - y0 - 3) < tol, `body at the site rose 3 m (${y0} -> ${y1})`);
  assert(!model.busy(), 'idle after the ramp');

  world.applyFrame(fixture.metrics);
  step(1);
  const m = new THREE.Matrix4(), s = new THREE.Vector3(), th = (i) => (slabs.getMatrixAt(i, m), s.setFromMatrixScale(m).y);
  assert(th(0) > th(2) && th(2) > th(1), `slab thickness follows alloc (${th(0)}, ${th(1)}, ${th(2)})`);
  const c = color('deep'), hex = (i) => (slabs.getColorAt(i, c), c.getHexString());
  assert(hex(0) === rampColor('slabLoad', 1).getHexString(), `h1 ready, load 1 -> ${hex(0)}`);
  assert(hex(1) === color('slabNotReady').getHexString(), `h2 not ready -> ${hex(1)}`);
  model.pose(3);
  assert(meshY() === y1, 'pose leaves the body alone');
}

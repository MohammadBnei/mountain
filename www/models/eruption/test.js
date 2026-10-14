// eruption behaviour (03-model-eruption): ok sync + paired deposit → land rises at the site, lava blobs appear,
// reach the deposit, cool (ramp lava → darker), emit lava.heat once, then clear. ok:false → ash puff, no land, no lava.
import { color, rampColor } from '../../js/kit.js';
import { cellXZ } from '../../js/world.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const blobs = model.root.getObjectByName('lava'), ash = model.root.getObjectByName('ash');
  const [dep, ev] = fixture.timeline, d = dep.payload, c = color('deep');
  const [sx, sz] = cellXZ(d.x0 + 1.5, d.z0 + 1.5), site = world.cell(...cellXZ(d.x0 + 1, d.z0 + 1));
  const lum = () => { let s = 0; for (let i = 0; i < blobs.count; i++) { blobs.getColorAt(i, c); s += c.r + c.g + c.b; } return s / blobs.count; };
  const heat = [];
  const off = world.on('lava.heat', (p) => heat.push(p));

  const h0 = world.fields.heightSolid[site];
  inject(dep.type, d);
  step(5);
  assert(blobs.count === 0, 'a deposit alone (no paired sync yet) starts no lava');
  inject(ev.type, ev.payload);
  step(30);
  assert(blobs.count > 10, `lava blobs exist after the ok sync (${blobs.count})`);
  const hot = lum();
  step(150);
  assert(world.fields.heightSolid[site] - h0 > 3.9, `heightSolid at the site rose (${h0} -> ${world.fields.heightSolid[site]})`);
  let near = 0;
  for (let i = 0; i < blobs.count; i++) if (Math.hypot(blobs.instanceMatrix.array[i * 16 + 12] - sx, blobs.instanceMatrix.array[i * 16 + 14] - sz) < 6) near++;
  assert(blobs.count > 0 && near / blobs.count > 0.8, `lava reached the deposit (${near}/${blobs.count} within 6 m)`);
  assert(lum() < hot * 0.85, `lava cooled (${hot.toFixed(3)} -> ${lum().toFixed(3)})`);
  // swap-remove: the blob moved into a freed slot is drawn there the same step (no frame of the dead blob)
  const A = blobs.instanceMatrix.array, end = rampColor('lava', 1).getHexString();
  let ghosts = 0;
  for (let s = 0; s < 600; s++) {
    const prev = A.slice(0, blobs.count * 16);
    step(1);
    for (let i = 0; i < blobs.count; i++) if (A.subarray(i * 16, i * 16 + 16).every((v, k) => v === prev[i * 16 + k]) && (blobs.getColorAt(i, c), c.getHexString() === end)) ghosts++;
  }
  assert(ghosts === 0, `no dead blob left in a live slot after swap-remove (${ghosts})`);
  assert(blobs.count === 0 && !model.busy(), `lava cleared (${blobs.count} blobs, busy ${model.busy()})`);
  assert(heat.length === 1 && Math.hypot(heat[0].x - sx, heat[0].z - sz) < 2, `one lava.heat at the site (${JSON.stringify(heat)})`);

  const land = world.fields['h.land'].reduce((a, b) => a + b, 0);
  inject(fixture.failed.type, fixture.failed.payload);
  step(30);
  assert(ash.visible && blobs.count === 0, 'ok:false: ash puff, no lava');
  step(120);
  assert(!ash.visible && !model.busy(), 'ash puff gone');
  assert(world.fields['h.land'].reduce((a, b) => a + b, 0) === land, 'ok:false creates no land');
  model.pose(4);
  off();
}

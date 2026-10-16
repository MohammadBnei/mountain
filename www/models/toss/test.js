// toss behaviour (03-model-toss): a world:mark toss flies from `from` to `pos` and rests (asleep or after 6 s) on the
// terrain near pos; other toys' marks are ignored; gone removes it; a hello / replay state redraws the persisted
// pebbles at rest; ≤ 100 pebbles (the newest); the slingshot sends {pos, from} = from + 3 × the pull, ≤ 24 m, and a tap
// sends nothing; tool.ok() mid-drag = the landing is a relay Spot; a refused place() is caught.
import * as THREE from 'three';
import { GRID, CELL } from '../../js/world.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const mesh = model.root.getObjectByName('toss');
  const M = new THREE.Matrix4(), P = new THREE.Vector3(), Q = new THREE.Quaternion(), S = new THREE.Vector3();
  const at = (i) => { mesh.getMatrixAt(i, M); M.decompose(P, Q, S); return { x: P.x, y: P.y, z: P.z }; };
  const xyz = ([x, z]) => [x, world.heightAt(x, z), z];
  const mark = (id, pos, from, mark = 'toss') => inject('world:mark', { id, mark, pose: { pos: xyz(pos), ...(from ? { from: xyz(from) } : {}) }, weight: 1 });
  const n0 = (world.state.marks || []).filter((m) => m.mark === 'toss').length;
  assert(mesh.count === n0 && !model.busy(), `starts with the persisted pebbles at rest (${mesh.count} / ${n0})`);

  const { throw: th, steep } = fixture;
  mark(th.id, th.pos, th.from);
  step(1);
  const k = mesh.count - 1, a = at(k);
  assert(mesh.count === n0 + 1 && model.busy(), 'a toss mark → one flying pebble');
  assert(Math.hypot(a.x - th.from[0], a.z - th.from[1]) < 1 && a.y > world.heightAt(a.x, a.z), `it leaves from \`from\`, above the ground (${a.x.toFixed(1)}, ${a.y.toFixed(1)}, ${a.z.toFixed(1)})`);
  step(15);
  const b = at(k);
  assert(Math.hypot(b.x - th.pos[0], b.z - th.pos[1]) < Math.hypot(a.x - th.pos[0], a.z - th.pos[1]), 'it flies toward pos');
  step(6 * 30 + 2);
  assert(!model.busy(), 'it rests by 6 s');
  const r = at(k);
  assert(Math.hypot(r.x - th.pos[0], r.z - th.pos[1]) < 4, `it rests near pos (${r.x.toFixed(1)}, ${r.z.toFixed(1)})`);
  assert(Math.abs(r.y - world.heightAt(r.x, r.z)) < 0.5, `rests snapped on the terrain (${(r.y - world.heightAt(r.x, r.z)).toFixed(2)} m)`);
  step(30);
  const r2 = at(k);
  assert(r2.x === r.x && r2.y === r.y && r2.z === r.z, 'a resting pebble stays put');

  mark(steep.id, steep.pos, steep.from);
  step(6 * 30 + 2);
  assert(!model.busy() && mesh.count === n0 + 2, 'a throw onto a steep face rests by 6 s too');

  mark(fixture.other.id, fixture.other.pos, null, 'cairns');
  step(1);
  assert(mesh.count === n0 + 2, 'another toy\'s mark is not a pebble');
  inject('world:mark', { id: steep.id, mark: 'toss', pose: {}, weight: 0, gone: true });
  step(1);
  assert(mesh.count === n0 + 1, 'gone removes the pebble (admin delete, cap)');

  // a hello / replay replaces world.state: the persisted pebbles come back at rest, at their pos
  const hello = fixture.hello.map((p, i) => ({ id: `01JHELLO${String(i).padStart(18, '0')}`, mark: 'toss', pose: { pos: xyz(p), from: xyz(p) }, weight: 1 }));
  world.write('state', 'socket', { ...world.state, marks: [...hello, { id: 'x', mark: 'cairns', pose: { pos: xyz([0, 0]) }, weight: 1 }] });
  step(1);
  assert(mesh.count === hello.length && !model.busy(), `a new state → its ${hello.length} pebbles at rest (${mesh.count})`);
  for (let i = 0; i < hello.length; i++) {
    const h = at(i), [x, , z] = hello[i].pose.pos;
    assert(Math.abs(h.x - x) < 1e-3 && Math.abs(h.z - z) < 1e-3 && Math.abs(h.y - world.heightAt(x, z)) < 0.5, `persisted pebble ${i} rests at its pos`);
  }

  const { n, x0, z0 } = fixture.flood;
  for (let i = 0; i < n; i++) mark(`01JFLOOD${String(i).padStart(18, '0')}`, [x0 + (i % 10) * 2, z0 + Math.floor(i / 10) * 2]);
  step(1);
  assert(mesh.count === 100, `≤ 100 pebbles (${mesh.count})`);
  const last = at(99);
  assert(Math.abs(last.x - (x0 + ((n - 1) % 10) * 2)) < 1e-3 && Math.abs(last.z - (z0 + Math.floor((n - 1) / 10) * 2)) < 1e-3, 'the newest are kept');

  // the slingshot
  const sent = [], place0 = world.place, V = (x, z) => new THREE.Vector3(x, world.heightAt(x, z), z);
  world.place = (m, pose) => { sent.push([m, pose]); return Promise.resolve('01JID'); };
  try {
    assert(model.tool?.label === 'tool.toss' && typeof model.tool.drag === 'function', 'joins the toolbar (tool.toss)');
    const drag = ({ start, end }) => { model.tool.drag('start', V(...start)); model.tool.drag('move', V(...end)); model.tool.drag('end', V(...end)); };
    drag(fixture.pull);
    const [m, pose] = sent[0] || [];
    const [sx, sz] = fixture.pull.start, [ex, ez] = fixture.pull.end;
    assert(m === 'toss' && Math.abs(pose.from[0] - sx) < 1e-6 && Math.abs(pose.from[2] - sz) < 1e-6, 'sends from = the drag start');
    assert(Math.abs(pose.pos[0] - (sx + 3 * (sx - ex))) < 1e-6 && Math.abs(pose.pos[2] - (sz + 3 * (sz - ez))) < 1e-6, `flies the other way, 3 × the pull (${pose.pos})`);
    assert(Object.values(pose).every((v) => Array.isArray(v) && v.every(Number.isFinite)), 'the pose holds numbers only');
    drag(fixture.long);
    const p2 = sent[1][1];
    assert(Math.abs(Math.hypot(p2.pos[0] - p2.from[0], p2.pos[2] - p2.from[2]) - 24) < 1e-6, 'a long pull is clamped to 24 m (the relay tossMax)');
    drag(fixture.tap);
    model.tool.drag('start', V(0, 0)); model.tool.drag('cancel', null);
    model.tool.drag('start', V(0, 0)); model.tool.drag('end', null);
    assert(sent.length === 2, 'a tap, a cancel or a release off the terrain sends nothing');
    // ok(): mid-drag, a release there lands on a relay Spot (the aiming ring); the peak throws onto snow, the lowest cell under it
    const hs = world.fields.heightSolid, at = (k) => V(-100 + (k % GRID) * CELL, -100 + Math.floor(k / GRID) * CELL);
    const peak = at(hs.reduce((b, v, k) => (v > hs[b] ? k : b), 0)), low = at(hs.reduce((b, v, k) => (v < hs[b] ? k : b), 0));
    model.tool.drag('start', peak);
    assert(model.tool.ok(V(peak.x + 1, peak.z)) && !model.tool.ok(peak), 'ok() for a throw from the peak, not for a tap');
    model.tool.drag('cancel', null);
    model.tool.drag('start', low);
    assert(!model.tool.ok(V(low.x + 1, low.z)), 'not ok() for a throw that lands under the snowline');
    model.tool.drag('cancel', null);
    assert(sent.length === 2, 'ok() sends nothing');
    world.place = () => Promise.reject(new Error('snowline'));
    drag(fixture.pull);
    await new Promise((r) => setTimeout(r, 0));
  } finally {
    world.place = place0;
  }
  step(1);
  assert(!model.busy(), 'a refused place() changes nothing');
}

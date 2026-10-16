// cairns behaviour (03-model-cairns): relay marks draw as stacked stones (one per mark, levelled per cell in mark
// order, other toys and malformed poses ignored); a gone mark lets the stones above drop; a hello/replay reseeds from
// state.marks; the tool settles a cannon Box on a flat cell above the snowline (onto a cairn within 2 cells first,
// as the relay does) and sends {pos, quat} upright; tool.ok() mirrors that settle; a refusal or a drop with nowhere to stand leaves nothing; rain
// darkens the stones under it until they dry; at most 200 stones (the newest) are drawn.
import * as THREE from 'three';
import { GRID, CELL } from '../../js/world.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const mesh = model.root.getObjectByName('cairns');
  const M = new THREE.Matrix4(), P = new THREE.Vector3(), Q = new THREE.Quaternion(), S = new THREE.Vector3(), Y = new THREE.Vector3();
  const at = (i) => { mesh.getMatrixAt(i, M); M.decompose(P, Q, S); return { x: P.x, y: P.y, z: P.z, up: Y.set(0, 1, 0).applyQuaternion(Q).y }; };
  const ij = (x, z) => [Math.round((x + 100) / CELL), Math.round((z + 100) / CELL)];
  const hs = world.fields.heightSolid, h = (i, j) => hs[j * GRID + i];
  const flat = (i, j) => h(i, j) > world.snowline && Math.hypot(h(i + 1, j) - h(i - 1, j), h(i, j + 1) - h(i, j - 1)) / (2 * CELL) <= 0.7;
  const [ax, az] = fixture.a, tick = async () => { for (let k = 0; k < 3; k++) await null; step(1); };
  const hit = ([x, z]) => new THREE.Vector3(x, world.heightAt(x, z), z);

  step(1);
  assert(mesh.count === 0, `the fixture hello has no cairns (${mesh.count})`);
  for (const m of fixture.marks) inject('world:mark', m);
  step(1);
  assert(mesh.count === 4, `3 stones on A + 1 on B; the toss pebble and the malformed pose are not stones (${mesh.count})`);
  const stack = [0, 1, 3].map(at), g = world.heightAt(ax, az);
  for (const s of stack) assert(Math.abs(s.x - ax) < 1e-4 && Math.abs(s.z - az) < 1e-4, `A's stones stand on A's cell (${s.x.toFixed(2)}, ${s.z.toFixed(2)})`);
  assert(stack[0].y > g && stack[0].y - g < 0.5, `the bottom stone sits on the ground (${(stack[0].y - g).toFixed(2)} m)`);
  assert(stack[1].y - stack[0].y > 0.3 && stack[2].y - stack[1].y > 0.3, `stacked in mark order (${stack.map((s) => s.y.toFixed(2)).join(' < ')})`);
  assert(at(2).y - world.heightAt(at(2).x, at(2).z) < 0.5, 'B is a cairn of one');

  inject('world:mark', { id: fixture.marks[1].id, mark: 'cairns', pose: {}, weight: 0, gone: true });
  step(1);
  assert(mesh.count === 3 && Math.abs(at(2).y - stack[1].y) < 1e-4, `a gone stone: the one above drops a level (${mesh.count}, ${at(2).y.toFixed(2)})`);
  const marks = world.state.marks;
  world.write('state', 'socket', { ...world.state, marks: [] });
  step(1);
  assert(mesh.count === 0, 'a hello without marks clears them');
  world.write('state', 'socket', { ...world.state, marks });
  step(1);
  assert(mesh.count === 3, 'a hello (or replay) reseeds from state.marks');

  // the tool: the relay echo is stubbed (place → the mark id, then its world:mark)
  const { tool } = model, place = world.place, sent = [];
  assert(tool?.label === 'tool.cairns' && typeof tool.drag === 'function', 'joins the toolbar (tool.cairns)');
  assert(tool.ok(hit(fixture.steep)) && !tool.ok(hit(fixture.low)), 'ok() where a stone finds a cell, not under the snowline (the aiming ring)');
  let reply = () => Promise.resolve(`01JA00000000000000000S${String(sent.length).padStart(4, '0')}`);
  world.place = (mark, pose) => { sent.push({ mark, pose }); return reply(); };
  const throwAt = async (xz) => {
    const n = sent.length;
    tool.drag('start', hit(xz));
    tool.drag('move', hit(xz));
    tool.drag('end', hit(xz));
    for (let k = 0; k < 200 && sent.length === n; k++) step(1); // ≤ 6.7 s: settles or times out at 4 s
    await tick();
    return sent[n];
  };
  try {
    tool.drag('start', hit(fixture.steep));
    step(1);
    const ghost = at(mesh.count - 1);
    assert(mesh.count === 4 && ghost.y > world.heightAt(ghost.x, ghost.z) + 0.5, `aiming shows the stone over its cell (${mesh.count})`);
    tool.drag('cancel', null);
    step(1);
    assert(mesh.count === 3 && !model.busy(), 'a cancelled drag leaves nothing');

    const s1 = await throwAt(fixture.steep);
    assert(s1?.mark === 'cairns' && Object.keys(s1.pose).sort().join() === 'pos,quat', `sends {pos, quat} (${JSON.stringify(s1)})`);
    const [i1, j1] = ij(s1.pose.pos[0], s1.pose.pos[2]), [i0, j0] = ij(...fixture.steep);
    assert(!flat(i0, j0) && flat(i1, j1) && Math.max(Math.abs(i1 - i0), Math.abs(j1 - j0)) <= 2, `a steep drop settles onto a flat cell within 2 cells (${i0},${j0} → ${i1},${j1})`);
    const q = new THREE.Quaternion(...s1.pose.quat);
    assert(Y.set(0, 1, 0).applyQuaternion(q).y > 0.8, 'settled upright (the relay checks it)');
    assert(Math.abs(s1.pose.pos[1] - world.heightAt(s1.pose.pos[0], s1.pose.pos[2])) < 0.6, `settled on the ground (${s1.pose.pos[1].toFixed(2)} vs ${world.heightAt(s1.pose.pos[0], s1.pose.pos[2]).toFixed(2)})`);
    assert(mesh.count === 3 && !model.busy(), `the stone in hand goes once the relay answers (${mesh.count})`);
    const x1 = -100 + i1 * CELL, z1 = -100 + j1 * CELL;
    inject('world:mark', { id: '01JA0000000000000000000S01', mark: 'cairns', pose: { pos: [x1, h(i1, j1), z1], quat: s1.pose.quat }, weight: 1 });
    step(1);
    assert(mesh.count === 4, 'drawn when its world:mark arrives');

    const s2 = await throwAt(fixture.nearA);
    assert(Math.abs(s2.pose.pos[0] - ax) < 0.5 && Math.abs(s2.pose.pos[2] - az) < 0.5, `dropped beside a cairn: onto it (${s2.pose.pos[0].toFixed(2)}, ${s2.pose.pos[2].toFixed(2)})`);
    assert(s2.pose.pos[1] > g + 0.6, `lands on top of the stack, not the ground (${(s2.pose.pos[1] - g).toFixed(2)} m up)`);

    reply = () => Promise.reject(new Error('rate'));
    const s3 = await throwAt(fixture.steep);
    assert(s3 && mesh.count === 4 && !model.busy(), `a refusal leaves nothing (${mesh.count})`);
    const n = sent.length;
    tool.drag('end', hit(fixture.low));
    step(30);
    assert(sent.length === n && mesh.count === 4, 'under the snowline there is nowhere to stand: nothing is sent');
  } finally {
    world.place = place;
  }

  const t0 = mesh.userData.kitRampT.slice(0, 4);
  inject('world:rain', fixture.rain);
  step(1);
  const t1 = mesh.userData.kitRampT.slice(0, 4);
  assert(t1[0] < t0[0] && t1[2] < t0[2] && t1[1] === t0[1], `rain darkens the stones under it only (${t0.map((x) => x.toFixed(2))} → ${t1.map((x) => x.toFixed(2))})`);
  assert(model.busy(), 'drying is animation');
  step(130, 1);
  assert(mesh.userData.kitRampT[0] === t0[0] && !model.busy(), 'they dry');

  for (let k = 0; k < 250; k++) inject('world:mark', { id: `01JA00000000000000000F${String(k).padStart(4, '0')}`, mark: 'cairns', pose: { pos: [-100 + (38 + (k % 50)) * CELL, 0, -100 + (90 + Math.floor(k / 50)) * CELL], quat: [0, 0, 0, 1] }, weight: 1 });
  step(1);
  assert(mesh.count === 199, `≤ 199 stones drawn + the one in hand: 200 × 80 tris (${mesh.count})`);
  assert(Math.abs(at(198).x - (-100 + 87 * CELL)) < 1e-4 && Math.abs(at(0).x - (-100 + 39 * CELL)) < 1e-4, 'the newest are drawn');
}

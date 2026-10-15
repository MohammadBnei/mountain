// footprints behaviour (03-model-footprints): every footprints world:mark is stamped into the kit footprint RT's scene
// (one quad per path segment along the path, red = K × the relay's weight, nothing in the main scene); a second walker
// on the same line adds on top (trails), an eroded re-emit dims its stamps in place, gone removes them, other toys'
// marks are ignored, a replaced world.state restamps; the tool turns a drag into a {pos, path} place() within reach.
import * as kit from '../../js/kit.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const mesh = kit.stampScene.getObjectByName('footprints'), col = mesh.instanceColor.array, K = 0.4;
  const ud = kit.stampScene.userData;
  let flagged = 0, v = ud.dirty; // count dirty sets (the sandbox render may clear it before we look)
  Object.defineProperty(ud, 'dirty', { configurable: true, get: () => v, set: (x) => { if (x) flagged++; v = x; } });
  const mark = (p) => { const f = flagged; inject('world:mark', p); step(1); return flagged > f; };
  const reds = () => Array.from({ length: mesh.count }, (_, i) => +col[i * 3].toFixed(3));
  const M = mesh.matrix.clone(), at = (i) => { mesh.getMatrixAt(i, M); return [M.elements[12], M.elements[14]]; };

  assert(model.root.children.length === 0 && mesh.parent === kit.stampScene && mesh.material.userData.kit === 'stamp', 'stamps live in kit.stampScene only, kit stamp() material');
  const n0 = mesh.count;
  assert(mark(fixture.walk) && mesh.count === n0 + 2, `a 3-point walk stamps 2 segments and flags the RT dirty (${mesh.count - n0})`);
  const p = fixture.walk.pose.path, [mx, mz] = at(n0);
  assert(Math.abs(mx - (p[0] + p[2]) / 2) < 1e-4 && Math.abs(mz - (p[1] + p[3]) / 2) < 1e-4, `segment centred between its path points (${mx}, ${mz})`);
  assert(reds().slice(n0).every((r) => r === K), `red = K × weight 1 (${reds()})`);

  assert(mark(fixture.again) && mesh.count === n0 + 4 && at(n0 + 2).join() === at(n0).join(), 'a second walker on the same line stamps on top (additive: a trail)');
  const n2 = mesh.count;
  assert(!mark(fixture.cairn) && mesh.count === n2, "another toy's mark is not stamped");
  assert(mark(fixture.tap) && mesh.count === n2 + 1, 'a lone tap stamps one square');

  assert(mark({ ...fixture.walk, weight: fixture.eroded }) && mesh.count === n2 + 1, 'an eroded re-emit restamps in place (no new quads)');
  const eroded = reds().filter((r) => r === +(K * fixture.eroded).toFixed(3)).length;
  assert(eroded === 2, `the eroded walk dims to K × ${fixture.eroded} (${reds()})`);
  assert(mark({ ...fixture.again, pose: {}, weight: 0, gone: true }) && mesh.count === n2 - 1, 'gone removes its stamps');

  world.write('state', 'socket', { ...world.state, marks: [fixture.walk, { ...fixture.tap, pose: {} }] }); // a hello / replay; a pose without path draws nothing
  step(1);
  assert(mesh.count === 2, `a replaced world.state restamps from its marks (${mesh.count})`);

  // the tool: a drag from the first hit, points ≥ a cell apart, within reach, ≤ 8 → world.place('footprints', {pos, path})
  const place = world.place, sent = [], d = fixture.drag;
  world.place = (m, pose) => (sent.push([m, pose]), Promise.reject(new Error('rate'))); // a refusal is caught
  const hit = (k) => { const x = d.from[0] + k * d.step, z = d.from[1]; return { x, y: world.heightAt(x, z), z }; };
  try {
    model.tool.drag('start', hit(0));
    for (let k = 1; k < d.n; k++) model.tool.drag('move', k % 7 ? hit(k) : null);
    model.tool.drag('end', hit(d.n));
    model.tool.drag('start', hit(0));
    model.tool.drag('cancel', null);
    await Promise.resolve();
  } finally { world.place = place; }
  assert(sent.length === 1 && sent[0][0] === 'footprints' && model.tool.label === 'tool.footprints', `one place() per finished drag, none on cancel (${sent.length})`);
  const { pos, path } = sent[0][1], far = Math.max(...path.filter((_, i) => i % 2 === 0).map((x, i) => Math.hypot(x - pos[0], path[i * 2 + 1] - pos[2])));
  assert(pos[0] === d.from[0] && pos[2] === d.from[1] && path[0] === pos[0] && path[1] === pos[2], 'pos = the first hit, the path starts there');
  assert(path.length >= 4 && path.length <= 16 && path.length % 2 === 0 && far <= 15, `≤ 8 [x, z] points within reach (${path.length / 2}, ${far.toFixed(1)} m)`);

  delete ud.dirty; ud.dirty = true;
}

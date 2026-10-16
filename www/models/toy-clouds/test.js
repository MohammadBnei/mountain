// toy-clouds behaviour (03-model-toy-clouds): a world:cloud draws K lumps at its pos, bigger with more contributors;
// between relay frames it drifts on world.wind (the relay's ∫wind), a new frame moves it there smoothly; state rain
// drops streaks under it, gone removes it; pick → {kind: 'marks', d, credits}; tool.ok() is the relay's Spot (above the
// snowline); the tool sends {pos} through world.place
// and shows a provisional cloud (expires unless the relay owns it, a refusal shows nothing); a replaced world.state
// (hello, replay) reseeds from state.clouds.
import * as THREE from 'three';
import { GRID, CELL } from '../../js/world.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const lumps = model.root.getObjectByName('clouds'), rain = model.root.getObjectByName('rain');
  const K = 4, M = new THREE.Matrix4(), V = new THREE.Vector3(); // K: lumps a cloud (index.js)
  const centre = (c) => { // mean lump position of the c-th cloud
    const s = new THREE.Vector3();
    for (let j = 0; j < K; j++) { lumps.getMatrixAt(c * K + j, M); s.add(V.setFromMatrixPosition(M)); }
    return s.divideScalar(K);
  };
  const size = (c) => { lumps.getMatrixAt(c * K, M); return V.setFromMatrixScale(M).z; };
  const [c1, c2] = fixture.clouds, s0 = (v) => `(${v.x.toFixed(1)}, ${v.y.toFixed(1)}, ${v.z.toFixed(1)})`;
  assert(lumps.count === 0 && !model.busy(), `starts empty (hello.json has no clouds; ${lumps.count} lumps)`);

  inject('world:cloud', c1);
  inject('world:cloud', c2);
  step(1);
  assert(lumps.count === 2 * K, `2 clouds → ${2 * K} lumps (${lumps.count})`);
  let p = centre(0);
  assert(Math.hypot(p.x - c1.pos[0], p.z - c1.pos[2]) < 3 && Math.abs(p.y - c1.pos[1]) < 4, `drawn at its pos ${s0(p)}`);
  assert(size(1) > size(0), `3 contributors → a bigger cloud (${size(1).toFixed(2)} > ${size(0).toFixed(2)})`);

  world.write('wind', 'socket', fixture.wind);
  step(30 * 60); // a minute without a relay frame
  p = centre(0);
  const want = c1.pos[0] + fixture.wind.x * model.params.drift * 60;
  assert(Math.abs(p.x - want) < 1.5, `drifts on the wind between frames (x ${p.x.toFixed(2)}, want ≈ ${want.toFixed(2)})`);

  inject('world:cloud', fixture.move);
  step(1);
  const jump = Math.abs(centre(0).x - p.x);
  step(30 * 10);
  p = centre(0);
  assert(jump < 1 && Math.abs(p.x - (fixture.move.pos[0] + fixture.wind.x * model.params.drift * 10)) < 1.5, `eases to the relay's new pos (first frame ${jump.toFixed(2)}, then x ${p.x.toFixed(2)})`);

  assert(rain.geometry.drawRange.count === 0, 'no rain while drifting');
  inject('world:cloud', fixture.rain);
  step(30);
  const a = rain.geometry.attributes.position, n = rain.geometry.drawRange.count, c = centre(1);
  assert(n > 0, `rain → streaks (${n / 2})`);
  let under = true;
  for (let i = 0; i < n; i++) under &&= Math.hypot(a.getX(i) - c.x, a.getZ(i) - c.z) < 12 && a.getY(i) <= fixture.rain.pos[1] + 0.01 && a.getY(i) >= world.heightAt(c.x, c.z) - 0.01;
  assert(under, 'the streaks fall under the raining cloud, above the ground');
  const y0 = a.getY(0);
  step(3);
  assert(a.getY(0) !== y0, 'the rain falls');

  const card = model.pick(new THREE.Ray(new THREE.Vector3(c.x, 150, c.z), new THREE.Vector3(0, -1, 0)));
  assert(card?.credits?.join() === fixture.rain.credits.join(), `pick credits the contributors (${JSON.stringify(card)})`);
  assert(card.kind === 'marks' && card.d > 150 - fixture.rain.pos[1] - 8 && card.d < 150 - fixture.rain.pos[1] + 8, `the card is a mark at its distance along the ray (R6; d ${card.d?.toFixed(1)})`);
  assert(model.pick(new THREE.Ray(new THREE.Vector3(90, 150, 90), new THREE.Vector3(0, -1, 0))) === null, 'pick misses the open sky');

  inject('world:cloud', { ...fixture.rain, state: 'gone', credits: [] });
  step(1);
  assert(lumps.count === K && rain.geometry.drawRange.count === 0, `gone removes it and its rain (${lumps.count} lumps)`);

  // the tool: world.place → {t:'mark', mark:'toy-clouds', pose:{pos}}; a provisional cloud until the relay's frame
  const sent = [], { x, z } = { x: fixture.spawn.pos[0], z: fixture.spawn.pos[2] }, hit = new THREE.Vector3(x, world.heightAt(x, z), z);
  world.send = (f) => sent.push(f) > 0;
  const hs = world.fields.heightSolid, low = hs.reduce((b, v, k) => (v < hs[b] ? k : b), 0), lowHit = new THREE.Vector3(-100 + (low % GRID) * CELL, 0, -100 + Math.floor(low / GRID) * CELL);
  assert(hit.y > world.snowline && model.tool.ok(hit), 'ok() on a spot above the snowline (the aiming ring is healthOk)');
  assert(!model.tool.ok(lowHit), `not ok() under the snowline (the relay would refuse: snowline; ${hs[low].toFixed(1)} m)`);
  try {
    model.tool.drag('start', hit);
    model.tool.drag('end', hit);
    assert(sent.length === 1 && sent[0].mark === 'toy-clouds' && sent[0].pose.pos.length === 3 && Object.keys(sent[0].pose).join() === 'pos', `the tool sends {pos} (${JSON.stringify(sent[0])})`);
    world.applyFrame({ t: 'mark', ref: sent[0].ref, id: '01JA0000000000000000000C05' });
    await Promise.resolve(); await Promise.resolve();
    step(1);
    assert(lumps.count === 2 * K, `accepted → a provisional cloud at once (${lumps.count} lumps)`);
    p = centre(1);
    assert(Math.hypot(p.x - x, p.z - z) < 3 && p.y > hit.y + 8, `over the spot ${s0(p)}`);
    step(30 * 45);
    assert(lumps.count === K, 'no relay frame (merged into a neighbour) → the provisional cloud expires');
    model.tool.drag('end', hit);
    world.applyFrame({ t: 'mark', ref: sent[1].ref, error: 'rate' });
    await Promise.resolve(); await Promise.resolve();
    assert(lumps.count === K, 'a refusal draws nothing');
  } finally { delete world.send; }

  world.write('state', 'socket', { ...world.state, clouds: fixture.hello }); // a hello / replay: a new world.state
  step(1);
  assert(lumps.count === fixture.hello.length * K && rain.geometry.drawRange.count > 0, `reseeds from world.state.clouds (${lumps.count} lumps, rain ${rain.geometry.drawRange.count / 2})`);
  assert(model.busy(), 'busy while clouds drift');
}

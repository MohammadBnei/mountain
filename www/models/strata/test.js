// strata behaviour (03-model-strata): ash days land as lines, newest on top, older ones buried by age, ≤ 32 kept,
// duplicates and malformed days ignored; a click on the open cut face at a line gives its day + title; the cut.js
// chain composes providers in layout.js order (strata before the mountain's caves).
import * as THREE from 'three';
import * as cut from '../../js/cut.js';
import { CUT } from '../../js/shaders.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const u = model.field.uniforms, ys = u.uStrataAsh.value.image.data, clip0 = world.clip;
  // a reload: the hello's world.state.ash is drawn at build; a new state object (hello, replay) resyncs the lines
  assert(u.uStrataN.value === world.state.ash.length, `hello.state.ash → ${world.state.ash.length} lines at build (${u.uStrataN.value})`);
  world.write('state', 'socket', { ...world.state, ash: [] }); // the test plays socket: a hello with no ash
  step(1);
  assert(u.uStrataN.value === 0, 'a resync to a state without ash clears the lines');
  for (const a of fixture.ash) inject('world:ash', a);
  step(1);
  assert(u.uStrataN.value === 3, `3 ash lines (${u.uStrataN.value})`);
  assert(ys[0] === 30 && Math.abs(ys[1] - (30 - 13 * 0.25)) < 1e-4 && Math.abs(ys[2] - (30 - 43 * 0.25)) < 1e-4, `newest on top, older buried by age (${ys.slice(0, 3).join(', ')})`);
  inject('world:ash', fixture.ash[0]);
  inject('world:ash', { day: '14/10/2026' });
  assert(u.uStrataN.value === 3, 'a duplicate or malformed day adds no line');
  const d0 = Date.parse(`${fixture.overflow.from}T00:00:00Z`);
  for (let i = 0; i < fixture.overflow.n; i++) inject('world:ash', { day: new Date(d0 + i * 864e5).toISOString().slice(0, 10) });
  assert(u.uStrataN.value === 32 && ys[0] === 30, `≤ 32 lines, the newest still on top (${u.uStrataN.value}, ${ys[0]})`);

  // pick: an eye on the removed side, level with the newest line, looking straight at the cut face
  world.write('clip', 'story', cut.at(fixture.cut));
  cut.apply(world);
  const zc = 100 - fixture.cut;
  let x = -60;
  while (x < 60 && world.heightAt(x, zc) < 33) x++;
  assert(x < 60, `the cut face at z ${zc} rises above the newest line somewhere`);
  const ray = (y) => new THREE.Ray(new THREE.Vector3(x, y, 200), new THREE.Vector3(0, 0, -1));
  const card = model.pick(ray(30.1));
  assert(card?.day === '2026-10-14' && card.title === fixture.ash[1].title, `newest line → its day + title (${JSON.stringify(card)})`);
  assert(model.pick(ray(23)) === null, 'between lines → no card');
  assert(cut.pick(ray(world.heightAt(x, zc) + 2), world) === null, 'above the terrain → no cut face');
  world.write('clip', 'story', cut.at(0));
  assert(model.pick(ray(30.1)) === null, 'cut closed → no card');
  world.write('clip', 'story', clip0);
  cut.apply(world);

  assert(CUT.key === 'cap:strata,caves' && CUT.pars.indexOf('c = strataField(p, c);') < CUT.pars.indexOf('c = cavesField(p, c);'), `cut chain strata → caves (${CUT.key})`);
}

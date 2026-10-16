// summit behaviour (03-model-summit): one crystal (K shards) per site, at the site's y; height follows the replicas
// bucket, glow the health (aHealth on ramp('health') × aGlow), hue the group; live ev:replicas / ev:argo.health /
// ev:app.gone and a new world.state (reload) all rebuild; pick(ray) → the app's card.
import * as THREE from 'three';
import { rampColor } from '../../js/kit.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const mesh = model.root.getObjectByName('crystals'), K = 5, hello = world.state;
  assert(mesh.count === world.sites.size * K, `one crystal per site at build (${mesh.count} instances, ${world.sites.size} sites)`);
  for (const s of fixture.sites) inject('world:site', s);
  step(1);
  assert(mesh.count === 5 * K, `5 sites → 5 crystals (${mesh.count / K})`);

  const M = new THREE.Matrix4(), P = new THREE.Vector3(), Q = new THREE.Quaternion(), S = new THREE.Vector3(), C = rampColor('health', 0); // scratch
  const at = (app) => [...world.sites.keys()].sort().indexOf(app) * K; // main shard of app
  const main = (app) => (mesh.getMatrixAt(at(app), M), M.decompose(P, Q, S), { y: P.y, h: S.y, x: P.x, z: P.z });
  const hex = (i) => (mesh.getColorAt(i, C), C.getHexString());
  const health = (i) => C.setRGB(mesh.geometry.attributes.aHealth.getX(i), mesh.geometry.attributes.aHealth.getY(i), mesh.geometry.attributes.aHealth.getZ(i)).getHexString();
  const glow = (i) => mesh.geometry.attributes.aGlow.getX(i);

  for (const s of fixture.sites) {
    const m = main(s.app);
    assert(Math.abs(m.x - s.x) < 1e-3 && Math.abs(m.z - s.z) < 1e-3 && Math.abs(m.y - (s.y - 0.6)) < 1e-3, `${s.app} sits at its site, y from the site (${m.y})`);
    assert(hex(at(s.app)) === rampColor('group', s.group === 'platform' ? 0 : 1).getHexString(), `${s.app} hue = group ${s.group}`);
  }
  // hello.state: replicas editable-blog 3, wird 2, mountain 1, dream-analyst none; health mountain Healthy, wird Degraded
  const h = (a) => main(a).h;
  assert(h('editable-blog') > h('wird') && h('wird') > h('mountain') && h('mountain') > h('dream-analyst'), 'height follows the replicas bucket');
  assert(health(at('wird')) === rampColor('health', 0.5).getHexString() && glow(at('wird')) > glow(at('mountain')), 'Degraded glows gold, brighter than Healthy');
  assert(health(at('mountain')) === rampColor('health', 0).getHexString() && glow(at('mountain')) > 0, 'Healthy glows ok');
  assert(glow(at('dream-analyst')) === 0, 'no health yet → no glow');

  const h0 = h('wird');
  inject('ev:replicas', fixture.replicas);
  inject('ev:argo.health', fixture.health);
  step(1);
  assert(h('wird') > h0 + 4, `ev:replicas bucket 6 grows wird (${h0} -> ${h('wird')})`);
  assert(health(at('mountain')) === rampColor('health', 1).getHexString(), 'ev:argo.health Missing → down');

  const s = fixture.sites.find((x) => x.app === 'wird'), ray = (x) => new THREE.Ray(new THREE.Vector3(x.x, 200, x.z), new THREE.Vector3(0, -1, 0));
  const card = model.pick(ray(s));
  assert(card?.kind === 'summit' && card.name === 'wird' && card.group === 'user' && card.health === 'Degraded' && card.replicas === 6 && Number.isFinite(card.d), `pick → wird's card (${JSON.stringify(card)})`);
  assert(Math.abs(card.d - (200 - (s.y - 0.6 + 1.5 + 1.1 * 6))) < 1e-3, `d = distance to the crystal top (${card.d})`);
  assert(model.pick(new THREE.Ray(new THREE.Vector3(0, 200, 90), new THREE.Vector3(0, -1, 0))) === null, 'pick off the crystals → null');
  assert(model.pick(new THREE.Ray(new THREE.Vector3(s.x, s.y + 20, s.z), new THREE.Vector3(0, 1, 0))) === null, 'pick into the sky → null');

  const gone = fixture.sites.find((x) => x.app === fixture.gone.app);
  inject('ev:app.gone', fixture.gone);
  step(1);
  assert(mesh.count === 4 * K && model.pick(ray(gone)) === null, `ev:app.gone drops its crystal (${mesh.count / K})`);

  // a reload/replay: a new world.state object resyncs (here: no health known yet)
  world.write('state', 'socket', { ...world.state, health: {} });
  step(1);
  assert([...Array(mesh.count).keys()].every((i) => glow(i) === 0), 'a new state without health → no glow');
  world.write('state', 'socket', { ...hello, health: { ...hello.health, mountain: 'Missing' } }); // leave glowing crystals for the after-test checks
  step(1);
  assert(glow(at('mountain')) > glow(at('wird')) && glow(at('wird')) > 0, 'resync restores the glow (Missing > Degraded)');
}

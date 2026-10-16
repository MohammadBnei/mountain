// toy-clouds (03-model-toy-clouds, W4 surface layer): visitor clouds. The relay owns them (relay/toy_toy_clouds.go:
// spawn over the spot, drift on the wind, merge on contact, rain once 3 accounts joined within the hour, then gone).
// This draws each world:cloud {id, pos, state, credits} as K kit puff() lumps on lit('cloud') (one InstancedMesh,
// colour per lump on ramp('cloud') by its height, bigger with more contributors), extrapolates the relay's drift
// (pos + wind · DRIFT · t since its frame: the relay re-sends only every unit of drift), and rains under a cloud in
// state rain: falling streaks, one LineSegments on kit line('water'). Reseeds from world.state.clouds when world.state
// is replaced (hello, replay). tool: a tap on the terrain sends {pos} (world.place); a provisional cloud shows over the
// spot until the relay's frame (or for PROV s: the relay may merge it into a neighbour); tool.ok(hit) = the spot is a
// relay Spot. pick(ray) → {kind: 'marks', d, credits} (R6 card).
import { spot } from '../../js/toys.js';

const DRIFT = 0.05, ALT = 14; // relay cloudDrift, cloudAlt
const K = 4, MAX = 24; // lumps a cloud, clouds drawn: K · MAX · ≤ 100 tris ≤ the 10k budget
const SIZE = 2.6, TAU = 2, PROV = 40; // lump scale, easing toward the relay's pos (s), provisional cloud life (s)
const CALM = { x: 0, z: 0 };
const DROPS = 40, RAINS = 4, FALL = 16, LEN = 1.4, R = 5; // rain: streaks a cloud, raining clouds drawn, fall speed, streak, radius

export default async function build({ THREE, kit, world }) {
  const lumps = new THREE.InstancedMesh(kit.puff(7), kit.lit('cloud'), K * MAX);
  lumps.name = 'clouds';
  lumps.count = 0;
  for (let i = 0; i < K * MAX; i++) kit.setRampAt(lumps, i, 0.5); // allocates instanceColor
  const pos = new Float32Array(RAINS * DROPS * 6), rainGeo = new THREE.BufferGeometry();
  rainGeo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  rainGeo.setDrawRange(0, 0);
  const rain = new THREE.LineSegments(rainGeo, kit.line('water', 0.5));
  rain.name = 'rain';
  lumps.frustumCulled = rain.frustumCulled = false; // instances and streaks move every frame
  const root = new THREE.Group();
  root.add(lumps, rain);
  const drop = Float32Array.from({ length: DROPS * 3 }, kit.mulberry(11)); // per streak: x, z offset (unit disc-ish), phase

  const list = []; // drawn order = instance order: {id, x, y, z (relay), t (s since), dx, dy, dz (shown), state, credits, s, off, prov}
  const O = new THREE.Object3D();
  const seedOf = (id) => { let h = 7; for (let i = 0; i < id.length; i++) h = Math.imul(h, 31) + id.charCodeAt(i); return h; };
  function relayout() { // instance colours by lump height, after the list changed
    lumps.count = Math.min(list.length, MAX) * K;
    for (let i = 0; i < lumps.count; i++) kit.setRampAt(lumps, i, 0.35 + 0.65 * list[(i / K) | 0].off[(i % K) * 4 + 1]);
  }
  function put({ id, pos: [x, y, z], state, credits }, prov = false) {
    let c = list.find((e) => e.id === id);
    if (state === 'gone') { if (c) list.splice(list.indexOf(c), 1); return relayout(); }
    if (!c) {
      const r = kit.mulberry(seedOf(id)), off = new Float32Array(K * 4);
      for (let k = 0; k < K; k++) off.set([(r() - 0.5) * 2.2, r(), (r() - 0.5) * 1.4, 0.7 + r() * 0.5], k * 4);
      list.push(c = { id, dx: x, dy: y, dz: z, off });
      relayout();
    }
    Object.assign(c, { x, y, z, t: 0, state, credits, prov, s: SIZE * (1 + 0.15 * Math.min(6, credits.length)) });
  }
  let state = null;
  function seed() {
    state = world.state;
    list.length = 0;
    for (const c of state.clouds || []) put(c);
    relayout();
  }
  seed();

  let clock = 0;
  function draw(dt) {
    const k = Math.min(1, dt / TAU), w = world.wind || CALM;
    let n = 0;
    for (let i = 0; i < list.length && i < MAX; i++) {
      const c = list[i];
      c.t += dt;
      c.dx += (c.x + w.x * DRIFT * c.t - c.dx) * k;
      c.dy += (c.y - c.dy) * k;
      c.dz += (c.z + w.z * DRIFT * c.t - c.dz) * k;
      for (let j = 0; j < K; j++) {
        const o = c.off, s = c.s * o[j * 4 + 3];
        O.position.set(c.dx + o[j * 4] * c.s, c.dy + o[j * 4 + 1] * c.s * 0.5, c.dz + o[j * 4 + 2] * c.s);
        O.scale.set(s * 1.2, s * 0.7, s);
        O.updateMatrix();
        lumps.setMatrixAt(i * K + j, O.matrix);
      }
      if (c.state !== 'rain' || n >= RAINS) continue;
      const g = world.heightAt(c.dx, c.dz), h = Math.max(1, c.dy - g);
      for (let d = 0; d < DROPS; d++, n++) { // ponytail: one ground height a cloud; streaks may end inside a slope
        const x = c.dx + (drop[d * 3] - 0.5) * 2 * R, z = c.dz + (drop[d * 3 + 1] - 0.5) * 2 * R;
        const y = c.dy - ((clock * FALL + drop[d * 3 + 2] * h) % h);
        const q = n * 6;
        pos[q] = pos[q + 3] = x; pos[q + 2] = pos[q + 5] = z; pos[q + 1] = y; pos[q + 4] = Math.max(g, y - LEN);
      }
    }
    lumps.instanceMatrix.needsUpdate = true;
    rainGeo.attributes.position.needsUpdate = n > 0;
    rainGeo.setDrawRange(0, n * 2);
  }
  draw(0);

  const caster = new THREE.Raycaster();
  return {
    root,
    params: { drift: DRIFT, size: SIZE },
    pose() {}, // not on the scroll story: the relay's clouds and the wind
    step(dt) {
      if (world.state !== state) seed(); // a hello or replay replaced world.state
      clock += dt;
      for (let i = list.length - 1; i >= 0; i--) if (list[i].prov && list[i].t > PROV) { list.splice(i, 1); relayout(); }
      draw(dt);
    },
    on: { 'world:cloud': (p) => put(p) },
    tool: {
      icon: '☁', label: 'tool.toy-clouds',
      drag(phase, hit) {
        if (phase !== 'end' || !hit) return;
        const { x, y, z } = hit;
        world.place('toy-clouds', { pos: [x, y, z] }).then((id) => { if (!list.some((c) => c.id === id)) put({ id, pos: [x, y + ALT, z], state: 'drift', credits: [] }, true); }, () => {});
      },
      ok: (hit) => spot(world, hit.x, hit.z), // the relay's rule (toy_toy_clouds.go: Place.Spot)
    },
    pick(ray) {
      caster.ray.copy(ray);
      lumps.boundingSphere = null; // instances moved
      const hit = caster.intersectObject(lumps)[0], c = hit && list[Math.floor(hit.instanceId / K)];
      return c ? { kind: 'marks', d: hit.distance, credits: c.credits } : null;
    },
    busy: () => list.length > 0,
    dispose() { kit.dispose(root); },
  };
}

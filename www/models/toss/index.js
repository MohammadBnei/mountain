// toss (03-model-toss): a slingshot drag throws a pebble. The drag starts on the terrain where the pebble sits (from)
// and pulls back; on release it flies the other way, K × the pull, ≤ THROW m. The relay (toy_toss.go) quantises both
// ends, recomputes y and checks the snowline and the reach; the accepted world:mark {pose: {pos, from}} reaches everyone,
// and each client flies a cannon Sphere (surface group) from `from` to `pos` on a ballistic arc. It rests on sleep or
// after REST s and is drawn snapped to the render height. hello / replay marks are drawn at rest at their pos.
// ≤ MAX pebbles (the newest), one InstancedMesh of kit rock() on lit('rock').
import * as CANNON from 'cannon-es';
import { surfacePhysics } from '../../js/toys.js';

const MAX = 100, THROW = 24, K = 3, REST = 6, G = 9.82; // MAX × 80 tris = 8000 (profile); THROW = relay tossMax

export default async function build({ THREE, kit, world }) {
  const mesh = new THREE.InstancedMesh(kit.rock(7), kit.lit('rock'), MAX);
  mesh.name = 'toss';
  kit.setRampAt(mesh, 0, 0.5); // allocate instanceColor
  mesh.count = 0;
  mesh.frustumCulled = false;
  const root = new THREE.Group();
  root.add(mesh);
  const S = surfacePhysics({ max: 16 });

  // pebbles, oldest first: {id, x, z, s, t, yaw, body, age}; the look is hashed from the id (same on every client)
  const list = [];
  let dirty = false, seen = null;
  const O = new THREE.Object3D(), F = new THREE.Vector3();

  function hash(id) {
    let h = 2166136261;
    for (let i = 0; i < id.length; i++) h = Math.imul(h ^ id.charCodeAt(i), 16777619);
    return h >>> 0;
  }
  function drop(i) {
    const p = list[i];
    if (p.body) S.phys.removeBody(p.body);
    list.splice(i, 1);
    dirty = true;
  }
  function add(m, fly) {
    const pos = m.pose?.pos;
    if (!pos) return;
    const k = list.findIndex((p) => p.id === m.id);
    if (k >= 0) drop(k);
    if (list.length >= MAX) drop(0);
    const h = hash(m.id), p = { id: m.id, x: pos[0], z: pos[2], s: 0.2 + (h % 100) / 1000, t: 0.1 + ((h >>> 8) % 80) / 100, yaw: (h >>> 16) % 628 / 100, body: null, age: 0 };
    const f = m.pose.from;
    if (fly && f && list.filter((q) => q.body).length < S.cap) { // ponytail: past the cap a throw just appears at rest
      if (!list.some((q) => q.body)) S.collider(world); // deposits move the land: re-read before the first flier
      const b = S.body({ mass: 1, shape: new CANNON.Sphere(p.s * 0.85), linearDamping: 0, angularDamping: 0, sleepSpeedLimit: 0.3, sleepTimeLimit: 0.5 });
      const y0 = f[1] + 1.2, y1 = pos[1] + p.s;
      let T2 = (Math.hypot(pos[0] - f[0], pos[2] - f[2]) / 14) ** 2 + 0.5; // a lob, then high enough to clear any ridge between
      for (let u = 0.125; u < 1; u += 0.125) {
        const h = world.heightAt(f[0] + (pos[0] - f[0]) * u, f[2] + (pos[2] - f[2]) * u) + 0.6 - (y0 + (y1 - y0) * u);
        T2 = Math.max(T2, (2 * h) / (G * u * (1 - u)));
      }
      const T = Math.sqrt(T2);
      b.position.set(f[0], y0, f[2]);
      b.velocity.set((pos[0] - f[0]) / T, (y1 - y0) / T + 0.5 * G * T, (pos[2] - f[2]) / T); // lands on pos after T s
      b.angularVelocity.set(6, 0, 3);
      // no air drag on the arc; it bites on landing so the pebble stays near pos (spheres in cannon never stop rolling)
      b.addEventListener('collide', () => { b.linearDamping = b.angularDamping = 0.95; });
      S.phys.addBody(b);
      p.body = b;
    }
    list.push(p);
    dirty = true;
  }
  function seed() {
    for (const p of list) if (p.body) S.phys.removeBody(p.body);
    list.length = 0;
    for (const m of world.state.marks || []) if (m.mark === 'toss') add(m, false);
    dirty = true;
  }
  seen = world.state;
  seed();

  function draw() {
    mesh.count = list.length;
    for (let i = 0; i < list.length; i++) {
      const p = list[i], b = p.body;
      if (b) {
        O.position.set(b.position.x, b.position.y, b.position.z);
        O.quaternion.set(b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w);
      } else {
        O.position.set(p.x, world.heightAt(p.x, p.z) + p.s * 0.4, p.z); // rests sunk a little into the slope
        O.rotation.set(0, p.yaw, 0);
      }
      O.scale.set(p.s, p.s * 0.7, p.s);
      O.updateMatrix();
      mesh.setMatrixAt(i, O.matrix);
      kit.setRampAt(mesh, i, p.t);
    }
    mesh.instanceMatrix.needsUpdate = true;
  }

  // the slingshot: start = where the pebble sits, release = the pull; a tap (pull < 0.5 m) throws nothing
  let from = null;
  const tool = {
    icon: '•',
    label: 'tool.toss',
    drag(phase, hit) {
      if (phase === 'start') { from = hit ? F.copy(hit) : null; return; }
      if (phase !== 'end' || !from || !hit) { if (phase !== 'move') from = null; return; }
      const dx = from.x - hit.x, dz = from.z - hit.z, d = Math.hypot(dx, dz), k = Math.min(K, THROW / Math.max(d, 1e-6));
      const f = [from.x, from.y, from.z];
      from = null;
      if (d < 0.5) return;
      world.place('toss', { pos: [f[0] + dx * k, 0, f[2] + dz * k], from: f }).catch(() => {}); // the toolbar shows the reason
    },
  };

  return {
    root,
    tool,
    params: { max: MAX, throw: THROW, k: K, rest: REST },
    pose() {}, // the marks are live state, not story
    step(dt) {
      if (world.state !== seen) { seen = world.state; seed(); } // a hello or replay replaced the state
      let flying = false;
      for (const p of list) if (p.body) { flying = true; break; }
      if (flying) {
        S.phys.step(1 / 30, dt, 3);
        for (const p of list) {
          const b = p.body;
          if (!b || ((p.age += dt) < REST && b.sleepState !== CANNON.Body.SLEEPING)) continue;
          // ponytail: rests where it settled (cosmetic, close to pos); a reload shows the relay's pos
          if (Math.abs(b.position.x) < 100 && Math.abs(b.position.z) < 100 && b.position.y > -75) { p.x = b.position.x; p.z = b.position.z; }
          S.phys.removeBody(b);
          p.body = null;
        }
        dirty = true;
      }
      if (!dirty) return;
      dirty = false;
      draw();
    },
    on: {
      'world:mark': (m) => {
        if (m.mark !== 'toss') return;
        if (m.gone) { const k = list.findIndex((p) => p.id === m.id); if (k >= 0) drop(k); return; }
        add(m, true);
      },
    },
    busy: () => list.some((p) => p.body),
    dispose() { for (const p of list) if (p.body) S.phys.removeBody(p.body); kit.dispose(root); },
  };
}

// cairns (03-model-cairns): visitors stack stones on flat spots above the snowline, one stone per mark. The tool
// drops a cannon Box (surfacePhysics: group surface, solver 20, friction 0.8) onto the cell the relay will keep, lets
// it settle, then sends {pos, quat}; the relay (toy_cairns.go) quantises, re-heights and settles it, and the stone is
// drawn when its world:mark arrives; tool.ok(hit) = target() finds a cell (the aiming ring). Each stone's level is its rank among the cairn marks on its cell (oldest at the
// bottom), so a deleted or capped-out stone lets the ones above it drop. world:rain wets the stones it falls on (darker
// rock ramp) and they dry over WET s. One InstancedMesh of kit rock() + lit('rock'): the newest MAX − 1 stones + the
// one in hand.
import * as CANNON from 'cannon-es';
import { GRID, CELL } from '../../js/world.js';
import { surfacePhysics } from '../../js/toys.js';

const MAX = 200; // 16000 tris / 80 per kit rock()
const SLOPE = 0.7, NEAR = 2, FULL = 7; // relay toy_cairns.go: cairnSlope, cairnNear, cairnMax
const PITCH = 0.36, LIFT = 0.18, SETTLE = 4, WET = 120; // m per stone up a cairn; m a base stone sits above the cell; s

export default async function build({ THREE, kit, world, rng }) {
  const mesh = new THREE.InstancedMesh(kit.rock(5), kit.lit('rock'), MAX);
  mesh.name = 'cairns';
  kit.setRampAt(mesh, 0, 0); // allocate instanceColor
  mesh.count = 0;
  mesh.frustumCulled = false;
  const root = new THREE.Group();
  root.add(mesh);
  const sp = surfacePhysics({ max: 2 });
  const O = new THREE.Object3D();

  // a stone's look, hashed from its mark id (09: variety = scale + rotation + ramp t)
  const hash = (s) => { let h = 2166136261; for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619); return h >>> 0; };
  const look = (id, level) => {
    const h = hash(id), k = 1 - 0.07 * level; // a cairn tapers
    return { sx: (0.55 + ((h & 255) / 255) * 0.2) * k, sy: 0.2 + (((h >>> 8) & 255) / 255) * 0.06, sz: (0.45 + (((h >>> 16) & 255) / 255) * 0.2) * k, t: 0.25 + ((h >>> 24) / 255) * 0.5 };
  };
  const ij = (x, z) => [Math.min(GRID - 1, Math.max(0, Math.round((x + 100) / CELL))), Math.min(GRID - 1, Math.max(0, Math.round((z + 100) / CELL)))];
  const levelY = (x, z, level) => world.heightAt(x, z) + LIFT + level * PITCH;

  /** The cairn stones of world.state, oldest first, each with its level on its cell and its look; cached until a
   *  world:mark or a new world.state (drying only re-colours: step stays allocation-free). */
  let cache = null;
  function stones() {
    if (cache) return cache;
    const out = [], on = new Map();
    for (const m of world.state.marks || []) {
      const p = m.pose?.pos;
      if (m.mark !== 'cairns' || !Array.isArray(p) || p.length !== 3 || !p.every(Number.isFinite)) continue;
      const [i, j] = ij(p[0], p[2]), key = j * GRID + i, level = on.get(key) || 0;
      on.set(key, level + 1);
      out.push({ m, i, j, level, s: look(m.id, level) });
    }
    return (cache = { out, on });
  }

  /** The relay's settle (toy_cairns.go), mirrored: the cell a stone dropped at (x, z) ends on, or null. */
  function target(x, z) {
    const hs = world.fields.heightSolid, { on } = stones(), [i0, j0] = ij(x, z);
    const h = (i, j) => hs[Math.min(GRID - 1, Math.max(0, j)) * GRID + Math.min(GRID - 1, Math.max(0, i))];
    let best = null, bd = Infinity;
    for (let dj = -NEAR; dj <= NEAR; dj++) for (let di = -NEAR; di <= NEAR; di++) {
      const i = i0 + di, j = j0 + dj;
      if (i < 0 || j < 0 || i >= GRID || j >= GRID) continue;
      const n = on.get(j * GRID + i) || 0, d = di * di + dj * dj - (n ? 100 : 0); // a cairn first
      const slope = Math.hypot(h(i + 1, j) - h(i - 1, j), h(i, j + 1) - h(i, j - 1)) / (2 * CELL);
      if (n < FULL && h(i, j) > world.snowline && slope <= SLOPE && d < bd) { bd = d; best = { x: -100 + i * CELL, z: -100 + j * CELL, level: n }; }
    }
    return best;
  }

  // the stone in hand: aiming (body null), falling (body), or sent (waiting for the relay)
  let held = null, seen = null, dirty = true, dryT = 0;
  const wet = new Map(); // mark id → s left wet

  function draw() {
    const { out } = stones(), from = Math.max(0, out.length - (MAX - 1));
    let n = 0;
    for (let k = from; k < out.length; k++) {
      const { m, i, j, level, s } = out[k], x = -100 + i * CELL, z = -100 + j * CELL, q = m.pose.quat;
      O.position.set(x, levelY(x, z, level), z);
      if (Array.isArray(q) && q.length === 4 && q.every(Number.isFinite)) O.quaternion.set(q[0], q[1], q[2], q[3]).normalize();
      else O.quaternion.identity();
      O.scale.set(s.sx, s.sy, s.sz);
      O.updateMatrix();
      mesh.setMatrixAt(n, O.matrix);
      kit.setRampAt(mesh, n++, s.t * (1 - 0.65 * Math.min(1, (wet.get(m.id) || 0) / WET * 2))); // wet = darker, dries over the last half
    }
    mesh.count = n + (held ? 1 : 0);
    if (held) kit.setRampAt(mesh, n, held.s.t);
    mesh.instanceMatrix.needsUpdate = true;
  }
  function drawHeld() {
    const b = held.body;
    if (b) { O.position.set(b.position.x, b.position.y, b.position.z); O.quaternion.set(b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w); }
    else { O.position.set(held.x, levelY(held.x, held.z, held.level) + 1, held.z); O.quaternion.identity(); } // aiming: hovers over its cell
    O.scale.set(held.s.sx, held.s.sy, held.s.sz);
    O.updateMatrix();
    mesh.setMatrixAt(mesh.count - 1, O.matrix);
    mesh.instanceMatrix.needsUpdate = true;
  }

  function aim(hit) {
    if (held?.body || held?.sent) return; // one stone at a time
    const t = hit && target(hit.x, hit.z);
    if (!t) { if (held) { held = null; dirty = true; } return; }
    if (!held) { held = { s: look(`h${rng()}`, t.level) }; dirty = true; }
    Object.assign(held, t);
  }
  function drop() {
    sp.collider(world);
    const { x, z, level, s } = held, y0 = levelY(x, z, 0) - LIFT;
    // the stack below as one Box column, the stone as a Box under its rock mesh
    held.stack = level ? sp.body({ mass: 0, shape: new CANNON.Box(new CANNON.Vec3(0.45, (level * PITCH) / 2, 0.45)), position: new CANNON.Vec3(x, y0 + (level * PITCH) / 2, z) }) : null;
    held.body = sp.body({ mass: 1, shape: new CANNON.Box(new CANNON.Vec3(s.sx * 0.8, s.sy * 0.85, s.sz * 0.8)), position: new CANNON.Vec3(x, y0 + level * PITCH + 1, z),
      angularDamping: 0.5, linearDamping: 0.1, sleepSpeedLimit: 0.2, sleepTimeLimit: 0.4 });
    held.body.quaternion.setFromEuler((rng() - 0.5) * 0.3, rng() * 6.28, (rng() - 0.5) * 0.3);
    if (held.stack) sp.phys.addBody(held.stack);
    sp.phys.addBody(held.body);
    held.age = 0;
  }
  function send() {
    const h = held, b = h.body, q = b.quaternion;
    h.sent = true;
    const done = () => { if (held === h) { held = null; dirty = true; } };
    world.place('cairns', { pos: [b.position.x, b.position.y, b.position.z], quat: [q.x, q.y, q.z, q.w] }).then(done, done); // a refusal: the toolbar says why
    for (const x of [h.stack, b]) if (x) sp.phys.removeBody(x);
  }

  return {
    root,
    params: { slope: SLOPE, near: NEAR, full: FULL, wet: WET },
    pose(P) { root.visible = P > 4.5; }, // the finale (layout visibleFrom 5)
    step(dt) {
      if (world.state !== seen) { seen = world.state; wet.clear(); cache = null; dirty = true; } // hello or replay: reseed from state.marks
      if (wet.size && (dryT += dt) >= 1) {
        for (const [id, s] of wet) if (s - dryT > 0) wet.set(id, s - dryT); else wet.delete(id);
        dryT = 0; dirty = true;
      }
      if (held?.body && !held.sent) {
        sp.phys.step(1 / 30, dt, 3);
        if (held.body.sleepState === CANNON.Body.SLEEPING || (held.age += dt) > SETTLE) send();
      }
      if (dirty) { dirty = false; draw(); }
      if (held) drawHeld();
    },
    tool: {
      icon: '▲', label: 'tool.cairns',
      drag(phase, hit) {
        if (phase === 'cancel') { if (held && !held.body) { held = null; dirty = true; } return; }
        aim(hit);
        if (phase === 'end' && held && !held.body) drop();
      },
      ok: (hit) => !!target(hit.x, hit.z), // a stone dropped here has somewhere to stand
    },
    on: {
      'world:mark': (p) => { cache = null; if (p.mark === 'cairns' || p.gone) dirty = true; }, // state.marks already holds it (or dropped it)
      'world:rain': ({ x, z, r }) => {
        for (const { m, i, j } of stones().out) if (Math.hypot(-100 + i * CELL - x, -100 + j * CELL - z) <= r) wet.set(m.id, WET);
        dirty = true;
      },
    },
    busy: () => !!held || wet.size > 0,
    dispose() { if (held?.body) sp.phys.removeBody(held.body); if (held?.stack) sp.phys.removeBody(held.stack); kit.dispose(root); },
  };
}

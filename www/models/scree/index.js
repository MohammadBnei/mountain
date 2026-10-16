// scree (03-model-scree): ev:pod.churn drops rocks at the app's site: a pebble per restart, a boulder for an OOM and
// one for a CrashLoop; ice.calve drops a pale boulder where the glacier calved. cannon Sphere (pebble) / Box (boulder)
// bodies tumble down the 128² heightSolid collider and sleep; a resting body is drawn snapped to the render height.
// ≤ 80 bodies: the oldest retires into a talus heap (≤ 20, one flattened rock each, mirrored into h.talus); talus
// erodes once churn has been quiet for STABLE s. Live only (nothing persisted). Both meshes share kit rock() + lit('rock').
// pick(ray) → {kind 'scree', d, churn: live rocks} on the nearest rock or heap, each as its bounding sphere.
import * as CANNON from 'cannon-es';
import { GRID, CELL } from '../../js/world.js';

const MAX = 80, HEAPS = 20, PER_EV = 16, MERGE = 3, STABLE = 300, TAU = 1800, SLEEP_AFTER = 25;
const REAL = 1; // 00 "Physics": collision group real (bit 1), never meets the surface group

export default async function build({ THREE, kit, world, rng }) {
  const geo = kit.rock(11), mat = kit.lit('rock');
  const live = new THREE.InstancedMesh(geo, mat, MAX), talus = new THREE.InstancedMesh(geo, mat, HEAPS);
  live.name = 'scree'; talus.name = 'talus';
  kit.setRampAt(live, 0, 0); kit.setRampAt(talus, 0, 0); // allocate instanceColor
  live.count = talus.count = 0;
  live.frustumCulled = talus.frustumCulled = false;
  const root = new THREE.Group();
  root.add(live, talus);

  const phys = new CANNON.World({ gravity: new CANNON.Vec3(0, -9.82, 0), allowSleep: true });
  phys.broadphase = new CANNON.SAPBroadphase(phys);
  phys.defaultContactMaterial.friction = 0.6;
  phys.defaultContactMaterial.restitution = 0.15;
  // heightSolid as a cannon Heightfield: data[i][yi] (local z up) rotated −90° about x, so world z = z0 − yi·CELL
  const hs = world.fields.heightSolid, data = Array.from({ length: GRID }, () => new Array(GRID).fill(0));
  const hf = new CANNON.Heightfield(data, { elementSize: CELL });
  const ground = new CANNON.Body({ mass: 0, shape: hf, collisionFilterGroup: REAL, collisionFilterMask: REAL });
  ground.position.set(-100, 0, -100 + (GRID - 1) * CELL);
  ground.quaternion.setFromEuler(-Math.PI / 2, 0, 0);
  phys.addBody(ground);
  // ponytail: re-read on every spawn (deposits and resyncs move the land); a full 128² copy, ≤ one per churn event
  function collider() {
    for (let i = 0; i < GRID; i++) for (let yi = 0; yi < GRID; yi++) data[i][yi] = hs[(GRID - 1 - yi) * GRID + i];
    hf.updateMinValue(); hf.updateMaxValue(); hf.updateBoundingSphereRadius(); hf.update();
  }

  // live slots: a ring, `head` = the oldest once full
  const body = new Array(MAX).fill(null), sx = new Float32Array(MAX), sy = new Float32Array(MAX), sz = new Float32Array(MAX), age = new Float32Array(MAX);
  let head = 0, quiet = STABLE, erodeT = 0;
  // talus heaps: centre, volume (Σ r³ of the retired rocks), yaw
  const hx = new Float32Array(HEAPS), hz = new Float32Array(HEAPS), hv = new Float32Array(HEAPS), hy = new Float32Array(HEAPS);
  let dirty = false;
  const O = new THREE.Object3D(), H = world.heightAt;
  const heapS = (k) => Math.min(1.8, 0.5 + Math.cbrt(hv[k])); // ≤ 1.8: facet band rock (median edge ≤ 1.2)
  const ball = new THREE.Sphere(), hit = new THREE.Vector3(); // pick scratch
  /** Distance along the ray to the ball, else Infinity. */
  const along = (ray) => (ray.intersectSphere(ball, hit) ? hit.distanceTo(ray.origin) : Infinity);

  function retire(i) {
    const b = body[i];
    if (!b) return;
    phys.removeBody(b);
    body[i] = null;
    if (b.position.y < -75) return; // fell off the world: nothing to heap
    const x = b.position.x, z = b.position.z, v = (sx[i] * sy[i] * sz[i]) / 1.5;
    let best = -1, bd = MERGE;
    for (let k = 0; k < talus.count; k++) { const d = Math.hypot(hx[k] - x, hz[k] - z); if (d < bd) { bd = d; best = k; } }
    if (best < 0 && talus.count < HEAPS) {
      best = talus.count++;
      hx[best] = x; hz[best] = z; hv[best] = 0; hy[best] = rng() * 6.28;
      kit.setRampAt(talus, best, 0.15 + rng() * 0.25);
    }
    if (best < 0) for (let k = 0, m = 1e9; k < talus.count; k++) { const d = Math.hypot(hx[k] - x, hz[k] - z); if (d < m) { m = d; best = k; } } // all 20 taken: nearest
    hv[best] += v;
    dirty = true;
  }

  function spawn(x0, z0, n, big, t0) {
    collider();
    quiet = 0;
    for (let k = 0; k < n; k++) {
      let i = head;
      if (live.count < MAX) i = live.count++;
      else { retire(i); head = (head + 1) % MAX; } // full: the oldest goes to the talus
      const boulder = k < big, r = boulder ? 1 + rng() * 0.4 : 0.4 + rng() * 0.15;
      sx[i] = r * (0.6 + rng() * 0.8); sy[i] = r * (0.6 + rng() * 0.8); sz[i] = r * (0.6 + rng() * 0.8); // 09: variety = scale + rotation
      const shape = boulder ? new CANNON.Box(new CANNON.Vec3(sx[i] * 0.75, sy[i] * 0.75, sz[i] * 0.75)) : new CANNON.Sphere(r * 0.85);
      const b = new CANNON.Body({ mass: boulder ? 20 : 1, shape, linearDamping: 0.1, angularDamping: 0.4, sleepSpeedLimit: 0.3, sleepTimeLimit: 0.6, collisionFilterGroup: REAL, collisionFilterMask: REAL });
      const x = x0 + (rng() - 0.5) * 3, z = z0 + (rng() - 0.5) * 3;
      b.position.set(x, H(x, z) + 1.5 + k * 0.9, z); // a short column: they land one after another
      b.quaternion.setFromEuler(rng() * 6, rng() * 6, rng() * 6);
      phys.addBody(b);
      body[i] = b; age[i] = 0;
      kit.setRampAt(live, i, t0 ?? (boulder ? 0.1 + rng() * 0.3 : 0.3 + rng() * 0.6));
    }
  }
  /** Where an app's rocks fall: its site, else a stable spot on the upper slopes hashed from the name. */
  function siteOf(app) {
    const s = world.sites.get(app);
    if (s) return [s.x, s.z];
    let h = 2166136261;
    for (let i = 0; i < app.length; i++) h = Math.imul(h ^ app.charCodeAt(i), 16777619);
    const a = ((h >>> 0) % 3600) / 3600 * Math.PI * 2, r = 20 + ((h >>> 12) % 20);
    return [Math.sin(a) * r, Math.cos(a) * r];
  }

  function stampTalus(f) {
    f.fill(0);
    for (let k = 0; k < talus.count; k++) {
      const h = heapS(k) * 0.4, c = world.cell(hx[k], hz[k]), i = c % GRID, j = (c - i) / GRID;
      for (let dj = -1; dj <= 1; dj++) for (let di = -1; di <= 1; di++) {
        if (i + di < 0 || j + dj < 0 || i + di >= GRID || j + dj >= GRID) continue;
        const g = (j + dj) * GRID + i + di;
        f[g] = Math.max(f[g], di || dj ? h * 0.5 : h);
      }
    }
  }

  return {
    root,
    params: { stable: STABLE, tau: TAU },
    pose() {},
    step(dt) {
      phys.step(1 / 30, dt, 3);
      quiet += dt;
      for (let i = 0; i < live.count; i++) {
        const b = body[i];
        if (!b) { O.scale.setScalar(0); O.updateMatrix(); live.setMatrixAt(i, O.matrix); continue; }
        if (b.position.y < -75) { retire(i); continue; } // left the collider
        if ((age[i] += dt) > SLEEP_AFTER && b.sleepState !== CANNON.Body.SLEEPING) b.sleep(); // ponytail: a slider on a steep face stops
        const p = b.position, rest = b.sleepState === CANNON.Body.SLEEPING;
        // resting: snap to the render height (bilinear heightSolid), sunk a third so it sits in the slope
        O.position.set(p.x, rest ? H(p.x, p.z) + sy[i] * 0.4 : p.y, p.z);
        O.quaternion.set(b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w);
        O.scale.set(sx[i], sy[i], sz[i]);
        O.updateMatrix();
        live.setMatrixAt(i, O.matrix);
      }
      live.instanceMatrix.needsUpdate = true;

      if (quiet > STABLE && talus.count && (erodeT += dt) >= 1) { // erosion: stable for STABLE s, a 1 s tick (h.talus restamps)
        const k0 = Math.exp(-erodeT / TAU);
        erodeT = 0;
        for (let k = talus.count - 1; k >= 0; k--) {
          if ((hv[k] *= k0) > 0.01) continue;
          const l = --talus.count; // ponytail: the last 0.7 m mound pops out; sink it if it shows
          hx[k] = hx[l]; hz[k] = hz[l]; hv[k] = hv[l]; hy[k] = hy[l];
          kit.setRampAt(talus, k, talus.userData.kitRampT[l]);
        }
        dirty = true;
      }
      if (!dirty) return;
      dirty = false;
      for (let k = 0; k < talus.count; k++) {
        const s = heapS(k);
        O.position.set(hx[k], H(hx[k], hz[k]) + s * 0.1, hz[k]);
        O.rotation.set(0, hy[k], 0);
        O.scale.set(s, s * 0.4, s);
        O.updateMatrix();
        talus.setMatrixAt(k, O.matrix);
      }
      talus.instanceMatrix.needsUpdate = true;
      world.write('h.talus', stampTalus);
    },
    on: {
      'ev:pod.churn': (ev) => {
        const big = (ev.oom ? 1 : 0) + (ev.crashloop ? 1 : 0), n = Math.min(PER_EV, (ev.restarts | 0) + big); // ponytail: ≤ 16 rocks per event
        if (n > 0) spawn(...siteOf(ev.app), n, big);
      },
      'ice.calve': (c) => spawn(c.x, c.z, Math.min(3, Math.max(1, Math.round(c.size))), 3, 1), // calved chunks: boulders, the palest rock
    },
    pick(ray) {
      let d = Infinity, churn = 0;
      for (let i = 0; i < live.count; i++) {
        const b = body[i];
        if (!b) continue;
        churn++;
        const p = b.position, rest = b.sleepState === CANNON.Body.SLEEPING; // where step() draws it
        ball.center.set(p.x, rest ? H(p.x, p.z) + sy[i] * 0.4 : p.y, p.z);
        ball.radius = Math.max(sx[i], sy[i], sz[i]);
        d = Math.min(d, along(ray));
      }
      for (let k = 0; k < talus.count; k++) {
        const s = heapS(k);
        ball.center.set(hx[k], H(hx[k], hz[k]) + s * 0.1, hz[k]);
        ball.radius = s;
        d = Math.min(d, along(ray));
      }
      return d < Infinity ? { kind: 'scree', d, churn } : null;
    },
    busy: () => body.some((b) => b && b.sleepState !== CANNON.Body.SLEEPING), // talus erosion is hours of slow drift, not animation
    dispose() { for (let i = 0; i < MAX; i++) if (body[i]) phys.removeBody(body[i]); kit.dispose(root); }, // kit drops them from the env-ramp set
  };
}

// lightning (03-model-lightning): a halo per site sized by rpsB; on a metrics frame with spike, one bolt per spiking
// app from the base of the nearest world.clouds mass cluster (none yet: params.above m over the site) down to the
// site, a midpoint-displacement ribbon for ~150 ms, magma when errB > 0; an idle St. Elmo corona at the summit
// (~11 short forking arcs re-seeded every 80–150 ms + a faint core halo), strength = total rpsB.
// Three draws: halos (instanced kit glow quads, billboarded), corona (kit line), bolt (kit line, ink or magma).
// While a bolt is out, the halos also run along its main ribbon (W2: a 1 px line alone was thin and lost on the day sky).
import { CELL, GRID } from '../../js/world.js';

const SITES = 64, ARCS = 11, LIFE = 0.15, QUEUE = 8, SHEATH = 33, SHEATH_M = 7; // bolt sheath: one quad per ribbon point, 7 m (cores overlap: no beads)
const DB = 5, DA = 3; // ribbon depths: bolt 32 segments (+ an 8-segment fork), arc 8 (+ a 2-segment fork)

export default async function build({ THREE, kit, world, rng }) {
  const params = { above: 40 }; // fallback bolt source height over the site (m) while world.clouds is empty
  const halo = kit.glow('elmo', 0.3, { mesh: true }), spr = kit.glow('elmo');
  halo.map = spr.map; // the kit sprite: soft round quads (09 glow map rule)
  spr.dispose();
  const glows = new THREE.InstancedMesh(new THREE.PlaneGeometry(1, 1), halo, 3 * SITES + 1 + SHEATH);
  glows.name = 'halos';
  glows.count = 0;
  const camQ = new THREE.Quaternion();
  glows.onBeforeRender = (r, s, cam) => camQ.copy(cam.quaternion); // ponytail: billboards lag the camera one reseed (≤ 150 ms)
  const seg = (n) => {
    const g = new THREE.BufferGeometry();
    g.setAttribute('position', new THREE.BufferAttribute(new Float32Array(n * 6), 3).setUsage(THREE.DynamicDrawUsage));
    return g;
  };
  const white = kit.line('ink', 3), red = kit.line('magma', 3);
  const bolt = new THREE.LineSegments(seg((1 << DB) + (1 << (DB - 2))), white);
  const arcs = new THREE.LineSegments(seg(ARCS * ((1 << DA) + (1 << (DA - 2)))), kit.line('elmo', 1.6));
  bolt.name = 'bolt';
  arcs.name = 'corona';
  for (const o of [glows, bolt, arcs]) { o.frustumCulled = false; o.visible = false; } // buffers move every reseed
  const root = new THREE.Group();
  root.add(glows, arcs, bolt);

  const P = new Float32Array(((1 << DB) + 1) * 3);
  /** Midpoint displacement a→b into P (2^depth + 1 points); each midpoint moves up to amp × its span, kept ≥ 0.5 m over the ground. */
  function ribbon(ax, ay, az, bx, by, bz, depth, amp) {
    const n = 1 << depth;
    P[0] = ax; P[1] = ay; P[2] = az; P[n * 3] = bx; P[n * 3 + 1] = by; P[n * 3 + 2] = bz;
    for (let s = n >> 1; s >= 1; s >>= 1) for (let i = s; i < n; i += 2 * s) {
      const a = (i - s) * 3, b = (i + s) * 3, d = Math.hypot(P[b] - P[a], P[b + 1] - P[a + 1], P[b + 2] - P[a + 2]) * amp;
      for (let k = 0; k < 3; k++) P[i * 3 + k] = (P[a + k] + P[b + k]) / 2 + (rng() - 0.5) * d;
      P[i * 3 + 1] = Math.max(P[i * 3 + 1], world.heightAt(P[i * 3], P[i * 3 + 2]) + 0.5); // never under the slope (QA bolt-dusk)
    }
    return n;
  }
  // segment i = points i, i+1 = P[i*3 .. i*3+6]
  const put = (A, at, n) => { for (let i = 0; i < n; i++) for (let k = 0; k < 6; k++) A[(at + i) * 6 + k] = P[i * 3 + k]; return at + n; };
  /** A ribbon a→b as line segments from segment `at`, forking once (depth − 2) with probability `fork`; → next free segment. */
  function zap(A, at, ax, ay, az, bx, by, bz, depth, amp, fork) {
    const n = ribbon(ax, ay, az, bx, by, bz, depth, amp);
    at = put(A, at, n);
    if (rng() >= fork) return at;
    const o = ((n * (0.3 + 0.4 * rng())) | 0) * 3, fx = P[o], fy = P[o + 1], fz = P[o + 2], L = Math.hypot(bx - ax, by - ay, bz - az) * 0.4;
    const ex = (fx + bx) / 2 + (rng() - 0.5) * L, ez = (fz + bz) / 2 + (rng() - 0.5) * L;
    return put(A, at, ribbon(fx, fy, fz, ex, Math.max((fy + by) / 2, world.heightAt(ex, ez) + 0.5), ez, depth - 2, amp));
  }

  const O = new THREE.Object3D();
  let h = 0;
  const halo1 = (x, y, z, s) => { O.position.set(x, y, z); O.quaternion.copy(camQ); O.scale.setScalar(s); O.updateMatrix(); glows.setMatrixAt(h++, O.matrix); };
  /** Corona arcs + core and the site halos, from the current metrics. */
  function reseed() {
    const apps = world.metrics?.apps;
    let total = 0;
    for (const app in apps) total += apps[app].rpsB;
    const k = Math.min(1, total / 20), i = world.fields.heightSolid.indexOf(world.hmax), x = (i % GRID - GRID / 2) * CELL, z = (((i / GRID) | 0) - GRID / 2) * CELL, y = world.hmax;
    const A = arcs.geometry.attributes.position;
    let at = 0;
    for (let a = 0, m = Math.ceil(ARCS * k); a < m; a++) {
      const th = rng() * 2 * Math.PI, up = 0.3 + 0.7 * rng(), r = Math.sqrt(1 - up * up), len = (1 + 3 * k) * (0.5 + rng());
      at = zap(A.array, at, x, y, z, x + Math.cos(th) * r * len, y + up * len, z + Math.sin(th) * r * len, DA, 0.5, 0.6);
    }
    arcs.geometry.setDrawRange(0, at * 2);
    A.needsUpdate = true;
    arcs.visible = at > 0;
    h = 0;
    if (k > 0) halo1(x, y + 0.5, z, (3 + 5 * k) * (0.8 + 0.4 * rng())); // flickering core
    for (const app in apps) {
      const s = world.sites.get(app), b = apps[app].rpsB;
      // rpsB 1–10 → 1–3 stacked quads (additive: brighter core) of 3–14 m
      if (s && b > 0 && h <= 3 * SITES - 2) for (let j = 0, y = world.heightAt(s.x, s.z) + 1; j <= b >> 2; j++) halo1(s.x, y, s.z, (4 + b) * (1 - j * 0.25));
    }
    if (bolt.visible) { // sheath: the main ribbon's points (segment i starts at array offset i * 6)
      const B = bolt.geometry.attributes.position.array, n = 1 << DB;
      for (let i = 0; i <= n; i++) { const o = i < n ? i * 6 : (n - 1) * 6 + 3; halo1(B[o], B[o + 1], B[o + 2], SHEATH_M); }
    }
    glows.count = h;
    glows.instanceMatrix.needsUpdate = true;
    glows.visible = h > 0;
  }

  /** One bolt from the nearest mass cluster's base (or params.above over the site) to the site's ground. */
  function strike(app) {
    const s = world.sites.get(app);
    if (!s) return;
    const gy = world.heightAt(s.x, s.z);
    let c = null, best = Infinity;
    for (const q of world.clouds) { const d = Math.hypot(q.x - s.x, q.z - s.z); if (q.mass && d < best) { best = d; c = q; } }
    let ax = s.x, ay = gy + params.above, az = s.z;
    if (c) { // the point of the base disc nearest the site
      const f = best > 0 ? Math.min(1, (c.r * 0.5) / best) : 0;
      ax = c.x + (s.x - c.x) * f; ay = c.y; az = c.z + (s.z - c.z) * f;
    }
    const at = zap(bolt.geometry.attributes.position.array, 0, ax, ay, az, s.x, gy, s.z, DB, 0.35, 1);
    bolt.geometry.setDrawRange(0, at * 2);
    bolt.geometry.attributes.position.needsUpdate = true;
    bolt.material = world.metrics?.apps[app]?.errB > 0 ? red : white;
    bolt.visible = true;
    life = LIFE;
    reseed();
  }

  const queue = []; // ponytail: one bolt at a time (one draw, one colour); a burst of spikes strikes in turn
  let last = null, life = 0, tick = 0;
  return {
    root,
    params,
    pose() {},
    step(dt) {
      const mt = world.metrics;
      if (mt !== last) { // a new metrics frame: one bolt per spiking app
        last = mt;
        for (const app in mt?.apps) if (mt.apps[app].spike && queue.length < QUEUE) queue.push(app);
      }
      if (life > 0 && (life -= dt) <= 0) { bolt.visible = false; reseed(); }
      if (life <= 0 && queue.length) strike(queue.shift());
      if ((tick -= dt) <= 0) { tick = 0.08 + 0.07 * rng(); reseed(); }
    },
    busy: () => life > 0 || queue.length > 0,
    dispose() { for (const o of [glows, bolt, arcs]) o.geometry.dispose(); for (const m of [halo, white, red, arcs.material]) m.dispose(); },
  };
}

// magma (03-model-magma): a cut-face field provider (cut.js), 0 draws of its own. A chamber under the summit and a
// feeder dyke up to the ember pool at the foot; one capsule dyke per build in flight (world.state.builds, kept current
// by world.js before ev:build; a reload or replay brings it back) rising from the chamber toward world.sites[app]:
// 'queued' stalls at QUEUED of the way, 'running' reaches the site and emits magma.dyke once; the build leaving the
// state retracts it. Glow = metrics.heat (eased): the cut-face glow weight and the kit ember pool (kit.setPool, 09).
// Writes temp (GRID², 0..1): ground warmth over the chamber and the dykes, plus lava.heat spots that cool over ~1 min.
import { CELL, GRID } from '../../js/world.js';

const MAXD = 8, QUEUED = 0.35, SPOTS = 8, COOL = 60;
const C = [0, -4, 10], R = [30, 9, 34]; // chamber centre and radii (m): reaches the default cut face (z 40, cut 60)
const POOL = [0, 66, 30, 9]; // the kit pool (09: centre (0, 66), radii (30, 9)); the feeder dyke ends under it
const GLSL = `uniform vec4 uMagmaA[${MAXD}], uMagmaB[${MAXD}]; uniform int uMagmaN; uniform float uMagmaGlow;
uniform vec3 uMagmaC, uMagmaR, uMagmaCol, uMagmaHot, uMagmaRim;
vec4 magmaField(vec3 p, vec4 base) {
  float d = (length((p - uMagmaC) / uMagmaR) - 1.) * uMagmaR.y; // ponytail: ellipsoid distance, exact on y only
  for (int i = 0; i < ${MAXD}; i++) {
    if (i >= uMagmaN) break;
    vec3 a = uMagmaA[i].xyz, ab = uMagmaB[i].xyz - a; float t = clamp(dot(p - a, ab) / max(dot(ab, ab), 1e-6), 0., 1.);
    d = min(d, length(p - a - ab * t) - mix(uMagmaA[i].w, uMagmaB[i].w, t));
  }
  float fw = fwidth(d), k = 1. - smoothstep(-fw, fw, d), crust = 1. - smoothstep(0., 1.2 + fw, d);
  vec3 c = mix(uMagmaCol, uMagmaHot, (1. - smoothstep(-5., -.5, d)) * uMagmaGlow / 1.6); // hotter inward
  return vec4(mix(mix(base.rgb, uMagmaRim, crust * .8), c, k), mix(base.a, uMagmaGlow, k));
}`;

export default async function build({ THREE, kit, world }) {
  const params = { rise: 4 }; // seconds for a dyke to climb from the chamber to its site
  const V = () => Array.from({ length: MAXD }, () => new THREE.Vector4());
  const field = kit.field(GLSL, {
    uMagmaA: V(), uMagmaB: V(), uMagmaN: 1, uMagmaGlow: 0.6, uMagmaC: new THREE.Vector3(...C), uMagmaR: new THREE.Vector3(...R),
    uMagmaCol: 'magma', uMagmaHot: 'lavaHot', uMagmaRim: 'emberRoot',
  });
  const u = field.uniforms, A = u.uMagmaA.value, B = u.uMagmaB.value;
  // slot 0: the feeder, from the chamber roof to just under the pool centre
  A[0].set(C[0], C[1] + R[1] * 0.5, C[2] + R[2] * 0.6, 2.2);
  B[0].set(POOL[0], world.heightAt(POOL[0], POOL[1]) - 1.5, POOL[1], 1.2);
  // build dykes: fixed slots (allocation-free step), app null = free
  const D = Array.from({ length: MAXD - 1 }, () => ({ app: null, a: [0, 0, 0], b: [0, 0, 0], g: 0, target: 0, emitted: false }));
  const spots = Array.from({ length: SPOTS }, () => ({ x: 0, z: 0, q: 0, k: 0, age: COOL * 9 }));
  let heat = 0, tick = 0, nextSpot = 0;

  // history (a hello, replay or reload: a new world.state object): its builds' dykes snap to their stage, no magma.dyke
  let seen = null, hist = null;
  function reconcile() {
    if (world.state !== seen) { seen = world.state; hist = new Set(Object.keys(seen.builds || {})); }
    const builds = world.state.builds || {};
    for (const d of D) if (d.app && !(d.app in builds)) d.target = 0;
    for (const app in builds) {
      let d = null;
      for (const x of D) if (x.app === app) d = x;
      if (!d) {
        const s = world.sites.get(app);
        if (!s) continue; // not a site (yet): picked up on a later tick once world:site lands
        for (const x of D) if (!d && !x.app) d = x;
        if (!d) continue; // ponytail: ≤ 7 build dykes, the rest wait for a free slot
        Object.assign(d, { app, g: 0, emitted: false });
        d.a[0] = C[0] + (s.x - C[0]) * 0.25; d.a[1] = C[1] + R[1] * 0.4; d.a[2] = C[2] + (s.z - C[2]) * 0.25;
        d.b[0] = s.x; d.b[1] = s.y - 1.5; d.b[2] = s.z;
      }
      const t = builds[app] === 'running' ? 1 : QUEUED;
      if (t === 1 && d.target !== 1) d.emitted = false; // a new running build emits, even on a slot still retracting
      d.target = t;
      if (hist.delete(app)) { d.g = t; d.emitted = true; }
    }
  }

  // temp: max of chamber warmth (0 at idle: heat is builds + syncs in flight), dyke paths (xz distance to the grown
  // part), lava.heat spots. Squared distances gate the sqrt; the writer is built once (step allocates nothing)
  const warm = (d, r0, r1) => 1 - Math.min(1, Math.max(0, (d - r0) / (r1 - r0)));
  const paint = (T) => {
    for (const s of spots) s.k = s.age < COOL * 5 ? s.q * Math.exp(-s.age / COOL) : 0;
    for (let j = 0; j < GRID; j++) for (let i = 0; i < GRID; i++) {
      const x = -100 + i * CELL, z = -100 + j * CELL; // cellXZ without the allocation
      let v = heat > 0.005 ? heat * warm(Math.hypot((x - C[0]) / R[0], (z - C[2]) / R[2]), 0.6, 1) : 0;
      for (const d of D) if (d.app && d.g > 0) {
        const ax = d.a[0], az = d.a[2], bx = ax + (d.b[0] - ax) * d.g, bz = az + (d.b[2] - az) * d.g, ex = bx - ax, ez = bz - az;
        const t = Math.min(1, Math.max(0, ((x - ax) * ex + (z - az) * ez) / Math.max(ex * ex + ez * ez, 1e-6)));
        const dx = x - ax - ex * t, dz = z - az - ez * t, q = dx * dx + dz * dz;
        if (q < 36) v = Math.max(v, d.g * warm(Math.sqrt(q), 2, 6));
      }
      for (const s of spots) if (s.k > 0) {
        const q = (x - s.x) ** 2 + (z - s.z) ** 2;
        if (q < 100) v = Math.max(v, s.k * warm(Math.sqrt(q), 0, 10));
      }
      T[j * GRID + i] = Math.min(1, v);
    }
  };
  const temp = () => world.write('temp', paint);

  function step(dt) {
    heat += ((world.metrics?.heat ?? 0) - heat) * Math.min(1, dt / 2);
    kit.setPool(POOL[0], POOL[1], POOL[2], POOL[3], heat);
    u.uMagmaGlow.value = 0.6 + heat;
    let n = 1;
    for (const d of D) {
      if (!d.app) continue;
      const k = dt / params.rise;
      d.g = d.g < d.target ? Math.min(d.target, d.g + k) : Math.max(d.target, d.g - k);
      if (d.g >= 1 && !d.emitted) { d.emitted = true; world.emit('magma.dyke', { app: d.app, x: d.b[0], z: d.b[2], heat }); }
      if (d.target === 0 && d.g === 0) { d.app = null; continue; }
      A[n].set(d.a[0], d.a[1], d.a[2], 1.6);
      B[n].set(d.a[0] + (d.b[0] - d.a[0]) * d.g, d.a[1] + (d.b[1] - d.a[1]) * d.g, d.a[2] + (d.b[2] - d.a[2]) * d.g, 1.6 - 0.8 * d.g);
      n++;
    }
    u.uMagmaN.value = n;
    for (const s of spots) s.age += dt;
    if (++tick % 10 === 0) { reconcile(); temp(); } // 3 Hz: a new site for a waiting build, a resynced world.state
  }

  reconcile();
  temp();
  return {
    root: new THREE.Group(),
    params,
    field,
    pose() {},
    step,
    on: {
      'ev:build': (p) => { hist.delete(p.app); reconcile(); }, // world.js has applied it to world.state.builds: live
      'lava.heat': ({ x, z, q }) => { Object.assign(spots[nextSpot], { x, z, q: Math.min(1, 0.5 + q), age: 0 }); nextSpot = (nextSpot + 1) % SPOTS; },
    },
    busy: () => D.some((d) => d.app && d.g !== d.target),
    dispose() {},
  };
}

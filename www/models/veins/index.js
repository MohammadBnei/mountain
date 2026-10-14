// veins (03-model-veins): gold seams for incidents that became knowledge. 0 draws: the relay's world:vein capsules
// (world.state.veins, ≤ 32, kept current by world.js for hello, replay and frames) colour two things:
//  - the surface inlay: each capsule expands into a seeded (FNV-1a of fp) jittered trunk on the surface plus forks,
//    ≤ 64 segments in all, into kit.setVeins (terrain uVeins; the low tier's terrain skips it, so cut face only there);
//  - the cut face: the raw capsules in a 32×2 DataTexture, looped per cap fragment like strata's ash lines
//    (ponytail: no 64³ Data3DTexture bake; the cap is one quad and 32 capsules is strata's loop size; bake if perf.mjs says so).
// Thickness and placement are the relay's; ref brightens the core (sunHalo).
const MAX = 32, SEGS = 64, NONE = [];
const GLSL = `uniform sampler2D uVeinCap; uniform int uVeinCapN; uniform vec3 uVeinGold, uVeinCore;
vec4 veinsField(vec3 p, vec4 base) {
  float v = 0., c = 0., rf = 0.;
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= uVeinCapN) break;
    vec4 A = texelFetch(uVeinCap, ivec2(i, 0), 0), B = texelFetch(uVeinCap, ivec2(i, 1), 0);
    vec3 ab = B.xyz - A.xyz; float t = clamp(dot(p - A.xyz, ab) / max(dot(ab, ab), 1e-6), 0., 1.), d = length(p - A.xyz - ab * t);
    v = max(v, 1. - smoothstep(A.w * .45, A.w, d));
    float k = 1. - smoothstep(A.w * .08, A.w * .22, d); if (k > c) { c = k; rf = B.w; }
  }
  vec3 g = mix(base.rgb, uVeinGold * .55, v);
  return vec4(mix(g, mix(uVeinGold, uVeinCore, rf), c), max(mix(base.a, .12, v), c * .7));
}`;

const fnv = (s) => { let h = 2166136261; for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619); return h >>> 0; };

export default async function build({ THREE, kit, world }) {
  const params = { jitter: 0.6, fork: 3 }; // trunk jitter (m, ±half) and fork drop (m)
  const cap = new Float32Array(MAX * 2 * 4), tex = new THREE.DataTexture(cap, MAX, 2, THREE.RGBAFormat, THREE.FloatType);
  const field = kit.field(GLSL, { uVeinCap: tex, uVeinCapN: 0, uVeinGold: 'gold', uVeinCore: 'sunHalo' });
  let caps = []; // the last kit.setVeins input (tests read it)

  // the surface point at azimuth th (x = sin, z = cos, as the relay) and altitude y: walk out until the solid drops
  // below y (worldcore surfR), then sink it `sink` m so the capsule is an inlay, not a tube
  const surf = (th, y, sink) => {
    let r = 0;
    while (r < 98 && world.heightAt(Math.sin(th) * r, Math.cos(th) * r) >= y) r += 0.5;
    const x = Math.sin(th) * r, z = Math.cos(th) * r;
    return [x, world.heightAt(x, z) - sink, z];
  };
  // one relay capsule → per segments: a T-segment jittered trunk a→b along the contour (radius 0.2 + 0.3 thick, tapering
  // to 70 %) plus per − T forks dropping `fork` m from interior joints and tapering to 0.1
  const expand = (v, per) => {
    const rnd = kit.mulberry(fnv(v.fp)), r0 = 0.2 + 0.3 * v.thick, T = Math.min(4, per), out = [];
    const ta = Math.atan2(v.a[0], v.a[2]), R = Math.max(10, Math.hypot(v.a[0], v.a[2]));
    const dth = ((Math.atan2(v.b[0], v.b[2]) - ta + 3 * Math.PI) % (2 * Math.PI)) - Math.PI; // shortest way round
    const rad = (t) => r0 * (1 - 0.3 * t), j = (k) => (rnd() - 0.5) * params.jitter * k;
    // every joint, the relay's ends too, is re-seated on this mesh's surface at its azimuth and altitude: the relay
    // samples solid by nearest cell (worldcore heightAt), up to metres off the bilinear body on a steep rim
    const pts = [];
    for (let i = 0; i <= T; i++) { const t = i / T, k = i > 0 && i < T ? 1 : 0; pts.push(surf(ta + dth * t + j(k) / R, v.a[1] + (v.b[1] - v.a[1]) * t + j(k), 0.4 * rad(t))); }
    for (let i = 0; i < T; i++) out.push({ a: pts[i], b: pts[i + 1], ra: rad(i / T), rb: rad((i + 1) / T), ref: v.ref });
    for (let f = 0; f < per - T; f++) {
      const i = 1 + (f % (T - 1)), t = i / T, p = pts[i]; // forks only when per > 4, so T = 4
      out.push({ a: p, b: surf(Math.atan2(p[0], p[2]) + j(4) / R, p[1] - params.fork * (0.6 + 0.8 * rnd()), 0.04), ra: rad(t) * 0.7, rb: 0.1, ref: v.ref });
    }
    return out;
  };

  let seen;
  const sync = () => {
    const vs = world.state.veins || NONE; // a shared empty array: a missing veins key must not resync every frame
    if (vs === seen) return;
    seen = vs;
    const live = vs.slice(-MAX);
    cap.fill(0);
    const per = Math.min(6, Math.floor(SEGS / Math.max(1, live.length)));
    caps = live.flatMap((v) => expand(v, per));
    // the cut face gets the trunk ends (re-seated) at the relay's radius
    live.forEach((v, i) => { const t = caps[i * per]; cap.set([...t.a, v.thick], i * 4); cap.set([...caps[i * per + Math.min(4, per) - 1].b, v.ref ? 1 : 0], (MAX + i) * 4); });
    field.uniforms.uVeinCapN.value = live.length;
    tex.needsUpdate = true;
    kit.setVeins(caps);
  };
  sync();

  return {
    root: new THREE.Group(),
    params,
    field,
    get caps() { return caps; },
    pose() {},
    step: sync,
    on: { 'world:vein': sync }, // world.js has already folded the frame into world.state.veins
    busy: () => false,
    dispose() { kit.setVeins([]); tex.dispose(); },
  };
}

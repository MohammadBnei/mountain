// aquifers (03-model-aquifers): a cut-face field provider (cut.js), 0 draws. One pool per metrics.storage.pools entry
// (+ the garage and pigsty springs, always unmeasured until they report): an ellipse (the slice of an ellipsoid) with
// lobed rims, sized by the capB bucket. longhorn: deep, clear, three shimmer layers (3 replicas), its level = usedB
// (5 % steps) with a rippled surface; local: murky, shallow, low in the proxmox band over its host's bedrock slab,
// unmeasured (dim, full, static) until the W3 textfile collector. Fake refraction: the rock behind is darkened and
// tinted, nothing is transmitted. A measured pool at ≥ 85 % (usedB ≥ 17) seeps: a wet streak down the face from its
// rim and one aquifer.seep {pool, x, z} when it starts seeping (again on a reload, once the river is listening).
// ponytail: pools are laid out along the cut face (u = to the camera's right), not at fixed world points, so every
// cut depth and azimuth slices the water table; seep x, z = the pool on the az 0 face line (z 0). Fixed world
// ellipsoids if the story ever needs pools that stay put under an orbiting cut.
const MAX = 18, SEEP = 17, HOST = { h1: -54, h2: 0, h3: 54 }; // 16 pools (metrics.json maxItems) + 2 springs
const GLSL = `uniform vec4 uAqA[${MAX}], uAqB[${MAX}]; uniform int uAqN; uniform float uAqT;
uniform vec3 uAqLong, uAqLocal, uAqUnW, uAqUnL, uAqDry;
vec4 aquifersField(vec3 p, vec4 base) {
  float u = dot(p.xz, vec2(-uClip.z, uClip.x)), fy = fwidth(p.y);
  for (int i = 0; i < ${MAX}; i++) {
    if (i >= uAqN) break;
    vec4 A = uAqA[i], B = uAqB[i]; // A (u, y, rx, ry); B (level 0..1, kind 0 longhorn | 1 local | 2 spring, measured, seep)
    bool meas = B.z > .5, lh = B.y < .5;
    if (B.w > .5) { // seep: a wet streak from the rim down the face, fading over 8 m
      float s = (1. - smoothstep(.2, .45, abs(u - A.x - .6 * A.z))) * step(p.y, A.y) * (1. - smoothstep(0., 8., A.y - .8 * A.w - p.y));
      base.rgb = mix(base.rgb, uAqLong * .5, s * .7);
    }
    vec2 q = vec2(u - A.x, p.y - A.y) / A.zw;
    float d = length(q) - 1. - .07 * sin(3. * atan(q.y, q.x) + A.x), w = fwidth(d), in_ = 1. - smoothstep(-w, w, d);
    if (in_ <= 0.) continue;
    vec3 wc = lh ? (meas ? uAqLong : uAqUnW) : B.y < 1.5 ? (meas ? uAqLocal : uAqUnL) : uAqUnW;
    float top = A.y + A.w * (2. * B.x - 1.) + (meas ? .12 * sin(u * 1.7 + uAqT * 1.3) + .05 * sin(u * 4.3 - uAqT * 2.1) : 0.);
    float wet = 1. - smoothstep(-fy, fy, p.y - top), sh = 0.;
    if (meas && lh) for (int k = 0; k < 3; k++) { float f = float(k); sh += pow(.5 + .5 * sin(u * (.9 + .4 * f) + p.y * (2.3 - .5 * f) + uAqT * (.6 + .35 * f) + f * 2.), 8.); }
    vec3 c = mix(base.rgb * .55, wc, lh ? .55 : .8) + wc * sh * .15;
    base = mix(base, vec4(mix(uAqDry, c, wet), sh * .06 * wet), in_);
  }
  return base;
}`;

/** pools → [{id, u, y, rx, ry, level, kind, measured, seep}] in face coordinates. */
function layout(storage) {
  const ps = storage?.pools || [], nL = ps.filter((p) => p.class === 'longhorn').length, perHost = {};
  let iL = 0;
  const out = ps.slice(0, 16).map((p) => {
    const rx = 2.5 + 0.6 * p.capB, measured = p.usedB !== null && p.usedB !== undefined, o = { id: p.id, rx, level: measured ? Math.min(1, p.usedB / 20) : 1, measured, seep: measured && p.usedB >= SEEP };
    if (p.class === 'longhorn') return Object.assign(o, { kind: 0, u: -45 + (90 * (iL + 0.5)) / nL, y: 17 - 4 * (iL++ % 2), ry: rx * 0.55 });
    const k = (perHost[p.host] = (perHost[p.host] ?? -1) + 1); // local: shallow, low, side by side over its host's slab
    return Object.assign(o, { kind: 1, u: (HOST[p.host] ?? 0) + (k % 2 ? 1 : -1) * Math.ceil(k / 2) * 9, y: 1.5, ry: Math.min(2, rx * 0.25) });
  });
  if (storage) for (const [id, u] of [['garage', -30], ['pigsty', 30]]) out.push({ id, u, y: 6, rx: 2.5, ry: 1.4, level: 1, kind: 2, measured: false, seep: false });
  return out;
}

export default async function build({ THREE, kit, world }) {
  const A = Array.from({ length: MAX }, () => new THREE.Vector4()), B = Array.from({ length: MAX }, () => new THREE.Vector4());
  const field = kit.field(GLSL, { uAqA: A, uAqB: B, uAqN: 0, uAqT: 0, uAqLong: 'longhorn', uAqLocal: 'local', uAqUnW: 'unmeasuredWater', uAqUnL: 'unmeasuredLocal', uAqDry: 'deep' });
  const u = field.uniforms, seeping = new Set();
  let pools = [], seen;
  const sync = () => {
    seen = world.metrics;
    pools = layout(seen?.storage);
    pools.forEach((p, i) => { A[i].set(p.u, p.y, p.rx, p.ry); B[i].set(p.level, p.kind, p.measured ? 1 : 0, p.seep ? 1 : 0); });
    u.uAqN.value = pools.length;
    for (const id of seeping) if (!pools.some((p) => p.seep && p.id === id)) seeping.delete(id); // drained: may seep again
  };
  sync();

  return {
    root: new THREE.Group(),
    params: {},
    field,
    pose() {},
    step(dt) {
      if (world.metrics !== seen) sync();
      u.uAqT.value = (u.uAqT.value + dt) % 1e3;
      for (const p of pools) if (p.seep && !seeping.has(p.id)) { seeping.add(p.id); world.emit('aquifer.seep', { pool: p.id, x: p.u, z: 0 }); }
    },
    busy: () => false,
    dispose() {},
  };
}

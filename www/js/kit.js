// Kit: the only source of materials, colours, the light rig and procedural helpers (09-style).
// ORCHESTRATOR-OWNED, style FROZEN. Colours come from tokens.json (byte copy of plan/style/tokens.json,
// a Go test keeps them equal); nothing here invents a colour. Every factory tags userData.kit.
//
// Exported interface (models use only these):
//   token(name) → '#rrggbb' in the current phase ('name@dusk' = pinned dusk, semantic names allowed)
//   color(name) → THREE.Color;  rampColor(ramp, t, out?) → THREE.Color;  RAMPS, FACET re-exported from layout
//   lit(tokenOrRamp, opts?)  glow(token, intensity, {mesh}?)  line(token, intensity)  points(token, size)
//   dome()  terrain({veins}?)  crystal()  lava()  flow(token)  stamp()  cave(caves, heightAt)
//   rock(seed)  puff(seed)  shard(seed)  setRampAt(mesh, i, t)  lavaHeat(material, v)
//   rig(renderer, scene) → {hemi, dir, dome, floor};  setPhase(elev, rising);  setKey(key)
//   U (terrain uniforms; writers: setPhase → stops/uSnowLift/uEmber, setPool → uPool/uPoolCol/uHeat,
//      setVeins → uVeins/uVeinN, main/socket → uSnowline, story → uClip);  setPool(...)  setVeins(caps)
//   footprintRT, stampScene (W4)
import * as THREE from 'three';
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import T from './tokens.json' with { type: 'json' };
import { patchTerrain, patchCrystal, patchLava, domeVert, domeFrag, flowVert, flowFrag } from './shaders.js';

export const QUALITY = globalThis.__quality || 'high';
const LOW = QUALITY === 'low';
export const KEYS = ['night', 'dawn', 'day', 'dusk'];
export const TOKENS = T;
export const RAMPS = { ...T.ramps.semantic, ...T.ramps.env };
const ENV_RAMP = new Set(Object.keys(T.ramps.env));

// ---------- colour math (sRGB space, 8-bit rounding: the generator's rule) ----------
const h2r = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
const r2h = (c) => '#' + c.map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0')).join('');
export const mixHex = (a, b, t) => (t <= 0 ? a : t >= 1 ? b : r2h(h2r(a).map((v, i) => v + (h2r(b)[i] - v) * t)));

// ---------- phase ----------
let ph = { a: 'dusk', b: 'dusk', w: 0 };
const rigAt = (k) => (ph.w <= 0 ? T[ph.a].rig[k] : T[ph.a].rig[k] + (T[ph.b].rig[k] - T[ph.a].rig[k]) * ph.w);

export function token(name) {
  const [n, pin] = name.split('@');
  if (n in T.semantic) return T.semantic[n];
  if (!(n in T.dusk)) throw new Error(`kit: unknown token ${name}`);
  if (pin) return T[pin][n];
  return mixHex(T[ph.a][n], T[ph.b][n], ph.w);
}
export const color = (name, out = new THREE.Color()) => out.set(token(name));
const rigHex = (k) => mixHex(T[ph.a].rig[k], T[ph.b].rig[k], ph.w);
const unit = (c) => c.multiplyScalar(1 / Math.max(c.r, c.g, c.b, 1e-6)); // rig colours at unit max channel

export function rampColor(name, t, out = new THREE.Color()) {
  const stops = RAMPS[name];
  if (!stops) throw new Error(`kit: unknown ramp ${name}`);
  const hex = ([s, v]) => (!ENV_RAMP.has(name) ? v : name === 'rock' && s === 0 ? mixHex(token('rock1'), token('emberRoot'), rigAt('ember')) : token(v));
  t = Math.min(1, Math.max(0, t));
  let i = 0;
  while (i < stops.length - 2 && t > stops[i + 1][0]) i++;
  const [a, b] = [stops[i], stops[i + 1]];
  return out.set(mixHex(hex(a), hex(b), (t - a[0]) / (b[0] - a[0])));
}

// ---------- registry for setPhase ----------
const tracked = new Set(); // materials with a kitToken (colour) or kitLift (emissive lift)
const rampMeshes = new Set();
const hooks = new Set(); // (internal) per-phase callbacks: cave mouths, dome, rig
function tag(m, kind, extra = {}) {
  m.userData.kit = kind;
  Object.assign(m.userData, extra);
  if (m.userData.kitToken || m.userData.kitLift) {
    tracked.add(m);
    m.addEventListener('dispose', () => tracked.delete(m));
    paint(m);
  }
  return m;
}
function paint(m) {
  const { kitToken: n, kitIntensity: k = 1, kitLift } = m.userData;
  if (n) {
    color(n, m.color);
    if (k !== 1) m.color.multiplyScalar(k);
  }
  if (kitLift) m.emissive.copy(m.color).multiplyScalar(rigAt('snowLift') * kitLift);
}

// ---------- materials ----------
function base(opts) {
  if (LOW) {
    const { roughness, metalness, ...rest } = opts;
    return new THREE.MeshLambertMaterial({ flatShading: true, fog: true, ...rest });
  }
  return new THREE.MeshStandardMaterial({ flatShading: true, fog: true, roughness: 1, metalness: 0, ...opts });
}
const assertToken = (n, allowed) => {
  token(n);
  if (allowed && !allowed.includes(n.split('@')[0])) throw new Error(`kit: ${n} not allowed here (${allowed.join('|')})`);
};
const FIXED = ['gold', 'magma', 'lavaHot', 'sunCore', 'sunMid', 'sunHalo', 'elmo', 'above', 'slopes', 'water', 'ink', ...Object.keys(T.semantic)];

/** Lit surface: a token, or a ramp name (colours then come per instance via setRampAt). */
export function lit(name, opts = {}) {
  if (RAMPS[name]) return tag(base({ ...opts, color: 0xffffff }), 'lit', { kitRamp: name });
  assertToken(name);
  return tag(base(opts), 'lit', { kitToken: name });
}

let SPRITE;
function sprite() { // round soft sprite, generated (no canvas, no loader)
  if (SPRITE) return SPRITE;
  const n = 64, d = new Uint8Array(n * n * 4);
  for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) {
    const r = Math.hypot(x - n / 2 + 0.5, y - n / 2 + 0.5) / (n / 2), a = r < 0.4 ? 1 - (r / 0.4) * 0.65 : Math.max(0, 0.35 * (1 - (r - 0.4) / 0.6));
    d.set([255, 255, 255, Math.round(a * 255)], (y * n + x) * 4);
  }
  SPRITE = new THREE.DataTexture(d, n, n);
  SPRITE.needsUpdate = true;
  return SPRITE;
}

/** Additive glow, fog off. SpriteMaterial (kit sprite) by default, MeshBasicMaterial with {mesh:true}. */
export function glow(name, intensity = 1, { mesh = false } = {}) {
  assertToken(name, FIXED);
  const o = { blending: THREE.AdditiveBlending, transparent: true, depthWrite: false, fog: false };
  const m = mesh ? new THREE.MeshBasicMaterial(o) : new THREE.SpriteMaterial({ ...o, map: sprite() });
  return tag(m, 'glow', { kitToken: name, kitIntensity: intensity });
}
export function line(name, intensity = 1) {
  assertToken(name, FIXED);
  return tag(new THREE.LineBasicMaterial({ blending: THREE.AdditiveBlending, transparent: true, depthWrite: false, fog: false }), 'line', { kitToken: name, kitIntensity: intensity });
}
export function points(name, size = 1) {
  assertToken(name);
  return tag(new THREE.PointsMaterial({ size, map: sprite(), transparent: true, depthWrite: false, fog: true }), 'points', { kitToken: name });
}

// Terrain uniforms: one shared set for every terrain material.
const VEINS_MAX = 64;
const veinTex = new THREE.DataTexture(new Float32Array(VEINS_MAX * 3 * 4), VEINS_MAX, 3, THREE.RGBAFormat, THREE.FloatType);
veinTex.needsUpdate = true;
export const footprintRT = new THREE.WebGLRenderTarget(512, 512);
export const stampScene = new THREE.Scene();
export const U = {
  uClip: { value: new THREE.Vector4(0, 1, 0, 1e4) }, uSnowline: { value: 1e4 }, uHMax: { value: 60 },
  uWobble: { value: 0 }, uHeat: { value: 0 }, uStale: { value: 0 }, uSnowLift: { value: 0 }, uEmber: { value: 1 },
  uPool: { value: new THREE.Vector4(0, 76, 30, 9) }, uPoolCol: { value: new THREE.Color() },
  uVeins: { value: veinTex }, uVeinN: { value: 0 }, uFootprint: { value: footprintRT.texture },
  uRock0: { value: new THREE.Color() }, uRock1: { value: new THREE.Color() }, uEmberRoot: { value: new THREE.Color() },
  uSnow: { value: new THREE.Color() }, uIce: { value: new THREE.Color() }, uTrail: { value: new THREE.Color() },
  uGold: { value: color('gold') }, uVeinRef: { value: color('sunHalo') },
};
color('magma', U.uPoolCol.value).multiplyScalar(0.6);

/** Ember pool (09): centre (x, z), radii (rx, rz), heat. Writer: magma. Default (0, 76, 30, 9): the board's (0, 86)
 *  sits below the haze floor once the rim is softened (Freeze item 2), so the foot moved 10 m in. */
export function setPool(x, z, rx, rz, heat) {
  U.uPool.value.set(x, z, rx, rz);
  if (heat !== undefined) U.uHeat.value = heat;
}
/** Veins: [{a:[x,y,z], b:[x,y,z], ra, rb, ref?}] in terrain object space (≤ 64). W3 veins adds the jittered fork expansion. */
export function setVeins(caps) {
  const d = veinTex.image.data;
  d.fill(0);
  caps.slice(0, VEINS_MAX).forEach((c, i) => {
    d.set([...c.a, c.ra], i * 4);
    d.set([...c.b, c.rb], (VEINS_MAX + i) * 4);
    d[(2 * VEINS_MAX + i) * 4] = c.ref ? 1 : 0;
  });
  U.uVeinN.value = Math.min(caps.length, VEINS_MAX);
  veinTex.needsUpdate = true;
}

/** The mountain surface: lit + terrain patch. Geometry supplies `bench` (0..1). */
export function terrain({ veins = !LOW } = {}) {
  const m = base({ color: 0xffffff });
  m.onBeforeCompile = (sh) => patchTerrain(sh, U, { veins });
  m.customProgramCacheKey = () => `kit-terrain-${LOW}-${veins}`;
  return tag(m, 'terrain');
}
export function crystal() {
  const m = base({ color: 0xffffff });
  m.onBeforeCompile = patchCrystal;
  m.customProgramCacheKey = () => `kit-crystal-${LOW}`;
  return tag(m, 'crystal', { kitRamp: 'group' });
}
export function lava() {
  const m = base({ color: 0xffffff });
  m.userData.uniforms = { uLavaHeat: { value: 1 } };
  m.onBeforeCompile = (sh) => patchLava(sh, m.userData.uniforms);
  m.customProgramCacheKey = () => `kit-lava-${LOW}`;
  return tag(m, 'lava', { kitRamp: 'lava' });
}
/** Lava emissive gain, capped at 1.6 (09). */
export const lavaHeat = (m, v) => (m.userData.uniforms.uLavaHeat.value = Math.min(1.6, Math.max(0, v)));

export function flow(name = 'water') {
  assertToken(name, ['water', 'longhorn', 'local', 'unmeasuredWater', 'unmeasuredLocal']);
  const m = new THREE.ShaderMaterial({
    fog: true, transparent: true, depthWrite: false, vertexShader: flowVert, fragmentShader: flowFrag,
    uniforms: THREE.UniformsUtils.merge([THREE.UniformsLib.fog, { uColor: { value: color(name) }, uTime: { value: 0 }, uOpacity: { value: name === 'local' ? 0.8 : 0.55 } }]),
  });
  return tag(m, 'flow', { kitToken: name });
}
export const stamp = () => tag(new THREE.MeshBasicMaterial({ color: 0xffffff }), 'stamp');

/** Sky dome mesh + star field (one per scene). Uniforms written only by setPhase. */
export function dome() {
  const u = { uSky0: { value: new THREE.Color() }, uSky1: { value: new THREE.Color() }, uHorizon: { value: new THREE.Color() }, uInk: { value: color('ink') }, uStars: { value: 0 } };
  const m = tag(new THREE.ShaderMaterial({ side: THREE.BackSide, depthWrite: false, fog: false, uniforms: u, vertexShader: domeVert, fragmentShader: domeFrag }), 'dome');
  const paintDome = () => { color('sky0', u.uSky0.value); color('sky1', u.uSky1.value); color('horizon', u.uHorizon.value); u.uStars.value = rigAt('stars'); };
  hooks.add(paintDome);
  paintDome();
  const mesh = new THREE.Mesh(new THREE.SphereGeometry(900, 32, 16), m);
  mesh.renderOrder = -1;
  return mesh;
}

// ---------- procedural helpers ----------
export function mulberry(a) {
  return () => { a |= 0; a = (a + 0x6d2b79f5) | 0; let t = Math.imul(a ^ (a >>> 15), 1 | a); t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t; return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
}
/** Icosahedron(1,1), seeded ±25 % vertex jitter, non-indexed (80 tris). */
export function rock(seed = 1) {
  const r = mulberry(seed), g = new THREE.IcosahedronGeometry(1, 1).toNonIndexed(), p = g.attributes.position, s = new Map();
  for (let i = 0; i < p.count; i++) {
    const k = `${p.getX(i).toFixed(3)},${p.getY(i).toFixed(3)},${p.getZ(i).toFixed(3)}`;
    if (!s.has(k)) s.set(k, 0.75 + r() * 0.5);
    p.setXYZ(i, p.getX(i) * s.get(k), p.getY(i) * s.get(k), p.getZ(i) * s.get(k));
  }
  g.computeVertexNormals();
  return g;
}
/** 3–5 merged Icosahedron(r, 0) lumps, bottom flattened (≤ 100 tris). */
export function puff(seed = 1) {
  const r = mulberry(seed), n = 3 + Math.floor(r() * 3), parts = [];
  for (let i = 0; i < n; i++) {
    const s = 0.5 + r() * 0.6;
    parts.push(new THREE.IcosahedronGeometry(s, 0).translate((r() - 0.5) * 1.6, r() * 0.4, (r() - 0.5) * 1.0));
  }
  const g = mergeGeometries(parts.map((p) => p.toNonIndexed()));
  const p = g.attributes.position;
  for (let i = 0; i < p.count; i++) if (p.getY(i) < 0) p.setY(i, 0);
  g.computeVertexNormals();
  return g;
}
/** Hex prism with a seeded off-centre tip (≤ 36 tris). */
export function shard(seed = 1) {
  const r = mulberry(seed), g = new THREE.CylinderGeometry(0.3, 0.21, 1, 6).toNonIndexed(), p = g.attributes.position;
  const tx = (r() - 0.5) * 0.25, tz = (r() - 0.5) * 0.25, th = 0.3 + r() * 0.4;
  for (let i = 0; i < p.count; i++) if (p.getY(i) > 0.49 && Math.abs(p.getX(i)) < 1e-6 && Math.abs(p.getZ(i)) < 1e-6) p.setXYZ(i, tx, 0.5 + th, tz);
  g.computeVertexNormals();
  return g;
}

/** Instance colour from a ramp; env ramps are re-evaluated by setPhase. */
export function setRampAt(mesh, i, t) {
  const name = mesh.material.userData.kitRamp;
  if (!name) throw new Error('kit.setRampAt: material has no ramp (use lit(ramp), crystal() or lava())');
  (mesh.userData.kitRampT ||= [])[i] = t;
  mesh.setColorAt(i, rampColor(name, t));
  mesh.instanceColor.needsUpdate = true;
  if (ENV_RAMP.has(name) && !rampMeshes.has(mesh)) rampMeshes.add(mesh);
}

/**
 * Cave mouths at Go `caves[]` (09 cave()): 4 InstancedMeshes (mouth, brow, jambs, threshold rocks).
 * heightAt(x, z) → world y of heightSolid. Dusk legibility (09 Freeze item 1): the brow/jamb rocks get a
 * snowLift-style emissive lift (colour × rig.snowLift), so the dark mouth reads by contrast; no new light.
 */
export function cave(caves, heightAt) {
  const g = new THREE.Group(), n = caves.length, W = 2.6, e = 0.5;
  const mouthG = new THREE.CircleGeometry(1, 13), mp = mouthG.attributes.position, rad = [];
  for (let i = 0; i < mp.count; i++) {
    const a = Math.atan2(mp.getY(i), mp.getX(i)), rr = Math.hypot(mp.getX(i), mp.getY(i)), j = rr > 0 ? 0.78 + ((Math.sin(a * 5.3) + 1) / 2) * 0.44 : 1;
    mp.setXYZ(i, mp.getX(i) * j * 3.6, Math.max(-0.55, mp.getY(i) * j) * 2.6, 0);
    rad.push(Math.pow(rr, 4) * 0.8);
  }
  mouthG.setAttribute('color', new THREE.Float32BufferAttribute(new Float32Array(mp.count * 3), 3));
  const paintMouth = () => {
    const c = mouthG.attributes.color, d = color('deep'), r1 = color('rock1'), t = new THREE.Color();
    rad.forEach((w, i) => c.setXYZ(i, ...t.lerpColors(d, r1, w).toArray()));
    c.needsUpdate = true;
  };
  hooks.add(paintMouth);
  paintMouth();
  const mouthM = tag(new THREE.MeshBasicMaterial({ vertexColors: true, fog: true, polygonOffset: true, polygonOffsetFactor: -6, polygonOffsetUnits: -6 }), 'cave');
  const rockG = rock(3), browM = lit('rock0'), jambM = lit('stone'), looseM = lit('rock0');
  browM.userData.kitLift = jambM.userData.kitLift = 1;
  tracked.add(browM); tracked.add(jambM); paint(browM); paint(jambM);
  const mouth = new THREE.InstancedMesh(mouthG, mouthM, n), brow = new THREE.InstancedMesh(rockG, browM, n);
  const jamb = new THREE.InstancedMesh(rockG, jambM, n * 3), loose = new THREE.InstancedMesh(rockG, looseM, n * 3);
  const F = new THREE.Object3D(), L = new THREE.Object3D(), M = new THREE.Matrix4();
  const put = (im, k, pos, scale, rot) => { L.position.set(...pos); L.scale.set(...scale); L.rotation.set(...rot); L.updateMatrix(); im.setMatrixAt(k, M.multiplyMatrices(F.matrix, L.matrix)); };
  caves.forEach((c, i) => {
    const r = mulberry(c.seed), out = new THREE.Vector3(Math.sin(c.yaw), 0, Math.cos(c.yaw)), tan = new THREE.Vector3(out.z, 0, -out.x), ym = c.y + 2.4;
    let dd = W;
    while (dd < W + 10 && heightAt(c.x - out.x * dd, c.z - out.z * dd) < ym) dd += 0.1;
    const P = new THREE.Vector3(c.x - out.x * dd, ym, c.z - out.z * dd);
    const nrm = new THREE.Vector3(heightAt(P.x - e, P.z) - heightAt(P.x + e, P.z), 2 * e, heightAt(P.x, P.z - e) - heightAt(P.x, P.z + e)).normalize().lerp(out, 0.5).normalize();
    F.position.copy(P).addScaledVector(nrm, 0.35);
    F.lookAt(F.position.clone().add(nrm));
    F.updateMatrix();
    put(mouth, i, [0, 0, 0], [1, 1, 1], [0, 0, 0]);
    put(brow, i, [(r() - 0.5) * 0.8, 2.5, 0.5], [3.6, 0.9, 1.3], [0.2, 0, (r() - 0.5) * 0.25]);
    [[-3.5, 0.2, 0.9 + r() * 0.4], [3.3 + r() * 0.4, -0.1, 0.6 + r() * 0.3], [-2.8, 1.6, 0.5 + r() * 0.3]].forEach(([x, y, k], j) => put(jamb, i * 3 + j, [x, y, 0.2], [k, k * 1.3, k * 0.8], [r() * 3, r() * 3, r() * 3]));
    for (let j = 0; j < 3; j++) {
      const k = 0.3 + r() * 0.4, q = P.clone().addScaledVector(out, 1.2 + r() * 1.5).addScaledVector(tan, (r() - 0.5) * 6);
      L.position.set(q.x, heightAt(q.x, q.z) + k * 0.3, q.z); L.scale.set(k, k * 0.7, k); L.rotation.set(r() * 3, r() * 3, r() * 3); L.updateMatrix();
      loose.setMatrixAt(i * 3 + j, L.matrix);
    }
  });
  mouth.userData.facet = 'exempt';
  for (const m of [brow, jamb, loose]) m.userData.facet = 'cave';
  g.add(mouth, brow, jamb, loose);
  return g;
}

// ---------- rig ----------
// Directional light direction per key (board.html SUN table: [azimuth°, elevation°], camera on +z).
const SUN = { night: [-35, 35], dawn: [-60, 8], day: [20, 50], dusk: [55, 6] };
const sunDir = (k) => { const [az, el] = SUN[k].map((d) => (d * Math.PI) / 180); return new THREE.Vector3(Math.sin(az) * Math.cos(el), Math.sin(el), -Math.cos(az) * Math.cos(el) * 0.4 + 0.6).normalize(); };
let RIG = null;

/** Pins tone mapping, exposure, fog, hemisphere + directional, dome, valley haze floor. No shadows. */
export function rig(renderer, scene) {
  renderer.toneMapping = THREE.LinearToneMapping;
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.shadowMap.enabled = false;
  scene.fog = new THREE.FogExp2(0x000000, 0.0035);
  const hemi = new THREE.HemisphereLight(0xffffff, 0xffffff, 1), dir = new THREE.DirectionalLight(0xffffff, 1);
  const floor = new THREE.Mesh(new THREE.CircleGeometry(900, 48).rotateX(-Math.PI / 2).translate(0, -16, 0), tag(new THREE.MeshBasicMaterial({ fog: true }), 'floor', { kitToken: 'horizon' }));
  const sky = dome();
  scene.add(hemi, dir, dir.target, floor, sky);
  const prev = renderer.getRenderTarget();
  renderer.setRenderTarget(footprintRT); renderer.setClearColor(0x000000, 0); renderer.clear(); renderer.setRenderTarget(prev);
  RIG = { renderer, scene, hemi, dir, floor, dome: sky };
  apply();
  return RIG;
}

let lastW = { a: null, b: null, w: -1 };
function apply() {
  for (const m of tracked) paint(m);
  for (const f of hooks) f();
  color('rock0', U.uRock0.value); color('rock1', U.uRock1.value); color('emberRoot', U.uEmberRoot.value);
  color('snow', U.uSnow.value); color('ice', U.uIce.value); color('trail', U.uTrail.value);
  U.uSnowLift.value = rigAt('snowLift');
  U.uEmber.value = rigAt('ember');
  if (lastW.a !== ph.a || lastW.b !== ph.b || Math.abs(lastW.w - ph.w) >= 0.01) {
    lastW = { ...ph };
    for (const m of rampMeshes) m.userData.kitRampT.forEach((t, i) => t !== undefined && m.setColorAt(i, rampColor(m.material.userData.kitRamp, t)));
    for (const m of rampMeshes) m.instanceColor.needsUpdate = true;
  }
  if (!RIG) return;
  const { renderer, scene, hemi, dir } = RIG;
  renderer.toneMappingExposure = rigAt('exposure');
  scene.fog.color.set(rigHex('fog'));
  scene.fog.density = rigAt('fogD');
  unit(hemi.color.set(rigHex('hemiSky')));
  unit(hemi.groundColor.set(rigHex('hemiGround')));
  hemi.intensity = rigAt('hemiI');
  unit(dir.color.set(rigHex('dir')));
  dir.intensity = rigAt('dirI');
  dir.position.copy(sunDir(ph.a).lerp(sunDir(ph.b), ph.w).normalize().multiplyScalar(100));
}

/** Key blend for a sun elevation (09 "Phase"). */
export function keyMix(elev, rising) {
  const tw = rising ? 'dawn' : 'dusk';
  if (elev <= -12) return { a: 'night', b: 'night', w: 0 };
  if (elev < -4) return { a: 'night', b: tw, w: (elev + 12) / 8 };
  if (elev <= 6) return { a: tw, b: tw, w: 0 };
  if (elev < 15) return { a: tw, b: 'day', w: (elev - 6) / 9 };
  return { a: 'day', b: 'day', w: 0 };
}
/** Lerp every kit colour, ramp, terrain stop, fog and the rig to the sun elevation. Writer: sky via world.rig. */
export function setPhase(elev, rising) { ph = keyMix(elev, rising); apply(); }
/** Exact key (gate / styleboard). */
export function setKey(k) { if (!KEYS.includes(k)) throw new Error(`kit: unknown key ${k}`); ph = { a: k, b: k, w: 0 }; apply(); }
export const phase = () => ({ ...ph });
apply();

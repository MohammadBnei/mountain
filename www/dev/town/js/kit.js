// Clay kit for the town (R7), from pi-lab compose/www/js/kit.js: every material comes from here, tagged
// userData.kit (the gate rejects anything else). Clay = the KayKit atlas repainted (models/clay.png, 8×4 cells).
import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';

const base = new URL('../', import.meta.url); // the page root (models/ lives there)

// 'low' (weak phones): Lambert instead of PBR, no shadows. Set by main.js before anything is built.
export const QUALITY = globalThis.__quality || 'high';
const LOW = QUALITY === 'low';

function tag(m) { m.userData.kit = true; return m; }
function clayMat(color, extra = {}) {
  if (LOW) {
    const { roughness, metalness, ...rest } = extra;
    return tag(new THREE.MeshLambertMaterial({ color, ...rest }));
  }
  return tag(new THREE.MeshStandardMaterial({ color, roughness: 0.9, metalness: 0, ...extra }));
}

export const colors = {
  bg: 0xf5f3ef, night: 0x0b1020,
  grass: 0x9fb487, earth: 0x8f7663, stone: 0xb9b6b2, wood: 0x9c7f62,
  roof: 0x8b97a6, offwhite: 0xefe9e1, raspberry: 0xc51a4a, slate: 0x6f767b, ink: 0x2b2f33,
  window: 0xffc27a, windowGlow: 0xd9862f, smoke: 0x8d8a86, fire: 0xff8a2a, ember: 0xffd27a,
  ok: 0x7fa36a, warn: 0xe0a33a, crit: 0xd24a3a, grey: 0x9a9a9a,
};

// Roof colours: one per app, muted clay hues that stay apart from each other and from the grass.
export const roofs = [0xc51a4a, 0x3f7fb0, 0xd9862f, 0x6a5aa8, 0x2f8f83, 0xb0603a, 0x8a9a3a, 0xa05a8a];
export const serviceRoof = 0x6f767b; // services: slate

export const palette = {
  grass: clayMat(colors.grass), earth: clayMat(colors.earth), stone: clayMat(colors.stone), wood: clayMat(colors.wood),
  offwhite: clayMat(colors.offwhite), raspberry: clayMat(colors.raspberry), slate: clayMat(colors.slate),
  ink: clayMat(colors.ink),
  smoke: clayMat(0xdedad4, { emissive: 0x5a5753, emissiveIntensity: 1, transparent: true, opacity: 0.85 }),
  fire: clayMat(colors.fire, { emissive: colors.fire, emissiveIntensity: 1.6 }),
  window: clayMat(colors.window, { emissive: colors.windowGlow, emissiveIntensity: 0 }),
  white: clayMat(0xffffff), // per-instance colour (InstancedMesh.instanceColor) carries the hue
};

const manager = new THREE.LoadingManager();
manager.onProgress = (url, loaded, total) => globalThis.__loader?.models(loaded, total);

const clayTex = new THREE.TextureLoader(manager).load(new URL('models/clay.png', base).href);
clayTex.colorSpace = THREE.SRGBColorSpace;
clayTex.flipY = false; // glTF UV convention
const sharedOpts = { map: clayTex };
export const shared = LOW ? tag(new THREE.MeshLambertMaterial(sharedOpts)) : tag(new THREE.MeshStandardMaterial({ ...sharedOpts, roughness: 0.9, metalness: 0 }));

/** A per-object copy of a palette material (or of `shared`) that may be animated without touching the others. */
export function variant(name, extra = {}) {
  const src = name === 'shared' ? shared : palette[name];
  if (!src) throw new Error(`kit.variant: unknown material ${name}`);
  const m = src.clone();
  Object.assign(m, extra);
  return tag(m);
}

export function mesh(geometry, material, shadows = !LOW) {
  const m = new THREE.Mesh(geometry, material);
  m.castShadow = m.receiveShadow = shadows;
  return m;
}

export function instanced(geometry, material, count, shadows = !LOW) {
  const m = new THREE.InstancedMesh(geometry, material, Math.max(1, count));
  m.castShadow = m.receiveShadow = shadows;
  m.count = 0;
  return m;
}

const loader = new GLTFLoader(manager);
const cache = new Map();
/** models/<name>.glb as one plain non-indexed float geometry in model space (node transforms baked, int16 de-quantized). */
export async function geometry(name) {
  if (!cache.has(name)) cache.set(name, loader.loadAsync(new URL(`models/${name}.glb`, base).href).then((gltf) => {
    const parts = [];
    gltf.scene.updateMatrixWorld(true);
    gltf.scene.traverse((o) => {
      if (!o.isMesh) return;
      const src = o.geometry.index ? o.geometry.toNonIndexed() : o.geometry, g = new THREE.BufferGeometry();
      for (const a of ['position', 'normal', 'uv']) {
        const at = src.attributes[a], arr = new Float32Array(at.count * at.itemSize);
        for (let i = 0; i < at.count; i++) for (let c = 0; c < at.itemSize; c++) arr[i * at.itemSize + c] = at.getComponent(i, c);
        g.setAttribute(a, new THREE.BufferAttribute(arr, at.itemSize));
      }
      parts.push(g.applyMatrix4(o.matrixWorld));
    });
    return parts;
  }));
  const parts = await cache.get(name);
  return parts.map((g) => g.clone());
}

// clay.png is an 8×4 grid of colour cells: [col, row] (row 0 at the top, glTF v).
export const CELL = { sage: [4, 1], tan: [3, 1], olive: [0, 2], teal: [1, 2], cream: [5, 2], stone: [2, 0], dark: [4, 0] };
const cellOf = (u, v) => [Math.floor(u * 8), Math.floor(v * 4)];

/** Move each triangle's UVs from one atlas cell to another (pick(normalY) → target cell or null to keep). */
export function recell(g, from, pick, vScale = 1, vShift = 0) {
  const uv = g.attributes.uv, n = g.attributes.normal;
  for (let i = 0; i < uv.count; i += 3) {
    const [cu, cv] = cellOf((uv.getX(i) + uv.getX(i + 1) + uv.getX(i + 2)) / 3, (uv.getY(i) + uv.getY(i + 1) + uv.getY(i + 2)) / 3);
    if (cu !== from[0] || cv !== from[1]) continue;
    const to = pick((n.getY(i) + n.getY(i + 1) + n.getY(i + 2)) / 3);
    if (!to) continue;
    for (let j = i; j < i + 3; j++) {
      const fv = uv.getY(j) * 4 - cv;
      uv.setXY(j, uv.getX(j) - from[0] / 8 + to[0] / 8, (to[1] + vShift + fv * vScale) / 4);
    }
  }
  return g;
}

/** Split a non-indexed geometry into [rest, picked] by a triangle test on its mean UV (u, v). */
export function splitByUV(g, test) {
  const uv = g.attributes.uv, keep = [], pick = [];
  for (let i = 0; i < uv.count; i += 3) {
    const u = (uv.getX(i) + uv.getX(i + 1) + uv.getX(i + 2)) / 3, v = (uv.getY(i) + uv.getY(i + 1) + uv.getY(i + 2)) / 3;
    (test(u, v) ? pick : keep).push(i);
  }
  const take = (tris) => {
    const out = new THREE.BufferGeometry();
    for (const a of ['position', 'normal', 'uv']) {
      const src = g.attributes[a], s = src.itemSize, arr = new Float32Array(tris.length * 3 * s);
      tris.forEach((i, n) => arr.set(src.array.subarray(i * s, (i + 3) * s), n * 3 * s));
      out.setAttribute(a, new THREE.BufferAttribute(arr, s));
    }
    return out;
  };
  return [take(keep), take(pick)];
}

// The KayKit roof cells (pi-lab office.js muteRoof): the team-colour blue, repainted slate in clay.png.
export const inRoof = (u, v) => (u > 0.125 && u < 0.25 && v > 0.25 && v < 0.5) || (u < 0.125 && v > 0.75);

// --- small helpers ---
export const clamp01 = (x) => Math.min(1, Math.max(0, x));
export const easeInOut = (x) => x * x * (3 - 2 * x);
export function easeOutBack(x, s = 1.70158) { const c3 = s + 1; return 1 + c3 * Math.pow(x - 1, 3) + s * Math.pow(x - 1, 2); }

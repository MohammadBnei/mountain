// Cut (01 "Cut"). Orchestrator-owned, W3 serial, FROZEN after W3a: the story writes world.clip, providers come in
// through their model's kit.field(), nobody edits this file.
//
// The mountain body is a closed DoubleSide kit terrain(). Its cut face is a separate cap mesh (cap(), added by the mountain
// to its root; 1 draw, 2 tris, hidden while the cut is closed): a kit terrain() quad that the vertex shader lays on the
// uClip plane, keeping only the fragments inside the solid (y ≤ heightSolid as the body mesh interpolates it, above the
// body bottom, on the grid) and shading them from the field providers, lit with the plane's normal. W3a measured the
// plan's first option, a back-face cap writing gl_FragDepth: the body lost early-z and the Android profile went from
// 16.7 to 33.3 ms p50 (perf.mjs), so the cap mesh it is (01 "Cut" fallback).
//
//   compose(models)    once, after the models are built and before the first render. Each model `field` (kit.field) that
//                      defines `vec4 <name>Field(vec3 p, vec4 base)` for a layout.js `fields` name is chained in that order
//                      (strata → aquifers → magma → veins → caves); missing ones are skipped; a field that matches no name, or
//                      two fields matching one, throw. The chain starts at
//                      (deep, glow 0); a provider returns linear rgb + glow weight. Returns the names used.
//   cap()              → the cap Mesh (the mountain's)
//   at(depth, az = 0)  → a world.clip value: the vertical plane facing the camera azimuth az (camera at (sin az, ·, cos az))
//                      that removes what lies nearer than `depth` from the world edge (r 100). ≤ 0 = closed
//   apply(world)       per frame (host): uClip ← world.clip.plane, cap shown while open, its height texture ← heightSolid
//   pick(ray, world)   → the cut-face point (Vector3) the ray hits, else null (01: ray ∩ plane, then y < heightSolid)
//
// Clipping list (01): clipped = mountain body + caves, bedrock slabs, glacier shell, eruption lava (kit terrain(),
// clipped(), lava()); not clipped = sky, clouds, crystals, river, toys, particles. Only the body has a cap (ponytail: the
// slabs sit under the y −16 haze floor and the glacier shell is metres thin; give them caps if the story shows them cut).
import * as THREE from 'three';
import { QUALITY, U, field, terrain } from './kit.js';
import { CUT } from './shaders.js';
import { fields } from './layout.js';
import N from './units.json' with { type: 'json' };

// the body's render grid (03-model-mountain: G² vertices over the heightSolid span, 96² on low), its spacing S, bottom
const G = QUALITY === 'low' ? N.GRID_LOW : N.GRID, CELL = N.EXTENT / N.GRID, X0 = -N.EXTENT / 2, S = (CELL * (N.GRID - 1)) / (G - 1), X1 = X0 + S * (G - 1), YB = -70;
const f = (v) => v.toFixed(5);
const hs = new Float32Array(G * G), hTex = new THREE.DataTexture(hs, G, G, THREE.RedFormat, THREE.FloatType);
const base = field('', { uCutDeep: 'deep' });
// cap quad (fill cost): centred at y 28, ±48 up a vertical plane (the solid above the haze floor, −20…76), ±142 across (the chord)
const QUAD = { value: new THREE.Vector3(28, 48, 142) };

const PARS = `uniform sampler2D uCutH; uniform vec3 uCutDeep; uniform float uWobble;
float kCutH(vec2 xz) { // heightSolid as the G² body mesh interpolates it: quad (i, j) = tris (a, b, c), (c, b, d)
  vec2 g = (xz - ${f(X0)}) / ${f(S)}; ivec2 i = ivec2(clamp(floor(g), vec2(0.), vec2(${G - 2}.))); g -= vec2(i);
  float a = texelFetch(uCutH, i, 0).r, b = texelFetch(uCutH, i + ivec2(0, 1), 0).r, c = texelFetch(uCutH, i + ivec2(1, 0), 0).r, d = texelFetch(uCutH, i + ivec2(1, 1), 0).r;
  // + the body's vertex wobble (shaders.js), per fragment: off from the per-vertex lerp by a sub-cell curvature only
  return (g.x + g.y <= 1. ? a + g.x * (c - a) + g.y * (b - a) : d + (1. - g.x) * (b - d) + (1. - g.y) * (c - d)) + uWobble * sin(xz.x * .7 + xz.y * .5);
}
bool kCutIn(vec3 p) { return p.y >= ${f(YB)} && all(greaterThanEqual(p.xz, vec2(${f(X0)}))) && all(lessThanEqual(p.xz, vec2(${f(X1)}))) && p.y <= kCutH(p.xz); }
`;

export function compose(models) {
  const own = models.map((m) => m.field).filter(Boolean), used = [];
  for (const x of own) if (x.kit !== 'field') throw new Error('cut: a model field must come from kit.field()');
  for (const n of fields) {
    const xs = own.filter((o) => new RegExp(`vec4\\s+${n}Field\\s*\\(`).test(o.glsl));
    if (xs.length > 1) throw new Error(`cut: ${xs.length} model fields define ${n}Field`);
    if (xs.length) used.push([n, xs[0]]);
  }
  // a misnamed provider (or one whose name is not in layout.js `fields`) would silently draw nothing
  if (own.some((o) => !used.some(([, x]) => x === o))) throw new Error(`cut: a model field defines no vec4 <name>Field( for a layout.js fields name (${fields.join(', ')})`);
  const uniforms = { ...base.uniforms, uCutH: { value: hTex }, uCutQuad: QUAD };
  for (const [n, x] of used) for (const k of Object.keys(x.uniforms)) {
    if (k in uniforms || k in U) throw new Error(`cut: ${n} uniform ${k} is already taken`);
    uniforms[k] = x.uniforms[k];
  }
  const names = used.map(([n]) => n);
  Object.assign(CUT, {
    pars: PARS + used.map(([, x]) => x.glsl).join('\n') + `\nvec4 kCutField(vec3 p) { vec4 c = vec4(uCutDeep, 0.); ${names.map((n) => `c = ${n}Field(p, c);`).join(' ')} return c; }\n`,
    uniforms, key: `cap:${names.join(',')}`,
  });
  return names;
}
compose([]); // a host that never composes still gets the plain (deep) cap

let capMesh = null;
export function cap() {
  const m = terrain({ closed: true });
  m.defines = { K_CUT_CAP: '' };
  capMesh = new THREE.Mesh(new THREE.PlaneGeometry(2, 2), m);
  // placed by its vertex shader; drawn before the body so the body's hidden fragments fail early-z
  Object.assign(capMesh, { name: 'cap', frustumCulled: false, visible: false, renderOrder: -0.5 });
  capMesh.userData.facet = 'exempt';
  return capMesh;
}

export const at = (depth, az = 0) => (depth > 0 ? { plane: [-Math.sin(az), 0, -Math.cos(az), N.EXTENT / 2 - depth], depth } : { plane: [0, 1, 0, 1e4], depth: 0 });

let sum = NaN;
export function apply(world) {
  const p = world.clip.plane, h = world.fields.heightSolid;
  U.uClip.value.set(p[0], p[1], p[2], p[3]);
  if (capMesh) capMesh.visible = p[3] < 1e4; // closed: (0, 1, 0, 1e4)
  let s = 0;
  for (let i = 0; i < h.length; i++) s += h[i] * ((i & 7) + 1); // ponytail: 16k-cell checksum per frame, cheaper than a re-upload
  if (s === sum) return;
  sum = s; // the body's vertex heights, sampled exactly as the mountain samples them (heightAt at its grid points)
  for (let j = 0; j < G; j++) for (let i = 0; i < G; i++) hs[j * G + i] = world.heightAt(X0 + i * S, X0 + j * S);
  hTex.needsUpdate = true;
}

export function pick(ray, world) {
  const [nx, ny, nz, w] = world.clip.plane, o = ray.origin, d = ray.direction;
  const so = nx * o.x + ny * o.y + nz * o.z + w, dn = nx * d.x + ny * d.y + nz * d.z;
  if (!(so < 0) || Math.abs(dn) < 1e-9) return null; // the eye is on the kept side: no cut face faces it
  const t = -so / dn;
  if (t <= 0) return null;
  const p = ray.at(t, new THREE.Vector3());
  return p.y >= YB && p.x >= X0 && p.x <= X1 && p.z >= X0 && p.z <= X1 && p.y < world.heightAt(p.x, p.z) ? p : null;
}

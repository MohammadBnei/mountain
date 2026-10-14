// Orchestrator-owned shader patches (01 "Shaders", 09 "terrain()/crystal()/lava()").
// Only kit.js calls these, from onBeforeCompile. Models never patch shaders.
// Each patch works on both kit materials (Standard on high, Lambert on low).

const need = (src, anchor) => {
  if (!src.includes(anchor)) throw new Error(`shaders: anchor ${anchor} missing (three upgrade?)`);
  return src;
};
const after = (src, anchor, code) => need(src, anchor).replace(anchor, `${anchor}\n${code}`);
const afterIf = (src, anchor, code) => (src.includes(anchor) ? src.replace(anchor, `${anchor}\n${code}`) : src);

// Terrain. Uniform slots (kit-owned, shared by every terrain material):
//   uClip vec4 (plane n.xyz, constant w; fragment discarded where n·p + w < 0; closed = (0,1,0,1e4))
//   uSnowline (world y), uHMax (summit y), uWobble, uHeat, uStale, uSnowLift, uEmber,
//   uPool vec4 (centre xz, radii xz), uPoolCol, uVeins (64×3 float capsule texture), uVeinN,
//   uFootprint (kit 512² RT only), uCaves vec4[5] (cave() mouth centres, w = 1 live), stop colours uRock0 uRock1 uEmberRoot uSnow uIce uTrail uGold uVeinRef.
// Attribute: bench (0..1, Go bench mask / 255).
export function patchTerrain(sh, U, { veins = true } = {}) {
  Object.assign(sh.uniforms, U);
  sh.vertexShader = after(
    'attribute float bench;\nvarying float vKBench;\nvarying vec3 vKWorld;\nvarying vec3 vKObj;\nflat varying float vKFaceH;\nuniform float uWobble;\n' + sh.vertexShader,
    '#include <begin_vertex>',
    'transformed.y += uWobble * sin(position.x * .7 + position.z * .5);\nvKBench = bench; vKObj = transformed;\nvKWorld = (modelMatrix * vec4(transformed, 1.)).xyz; vKFaceH = vKWorld.y;',
  );
  let f = `uniform vec4 uClip, uPool; uniform float uSnowline, uHMax, uHeat, uStale, uSnowLift, uEmber; uniform int uVeinN;
uniform vec3 uRock0, uRock1, uEmberRoot, uSnow, uIce, uTrail, uGold, uVeinRef, uPoolCol;
uniform sampler2D uVeins, uFootprint; uniform vec4 uCaves[5];
varying float vKBench; varying vec3 vKWorld; varying vec3 vKObj; flat varying float vKFaceH;
float kVein = 0., kCore = 0., kRef = 0.;
void kVeins(vec3 p) {
  for (int i = 0; i < 64; i++) {
    if (i >= uVeinN) break;
    vec4 A = texelFetch(uVeins, ivec2(i, 0), 0), B = texelFetch(uVeins, ivec2(i, 1), 0);
    vec3 ab = B.xyz - A.xyz; float t = clamp(dot(p - A.xyz, ab) / max(dot(ab, ab), 1e-6), 0., 1.);
    float th = mix(A.w, B.w, t), d = length(p - A.xyz - ab * t);
    float v = 1. - smoothstep(th * .45, th, d), c = 1. - smoothstep(th * .08, th * .22, d);
    kVein = max(kVein, v); if (c > kCore) { kCore = c; kRef = texelFetch(uVeins, ivec2(i, 2), 0).x; }
  }
}
` + sh.fragmentShader;
  f = after(f, '#include <clipping_planes_fragment>', 'if (dot(uClip.xyz, vKWorld) + uClip.w < 0.) discard;');
  f = after(f, '#include <color_fragment>', `
  vec3 kN = normalize(cross(dFdx(vKWorld), dFdy(vKWorld)));
  float kH = vKFaceH / max(uHMax, 1.);
  float kSnow = ((vKFaceH > uSnowline && abs(kN.y) > .2) || vKFaceH > uSnowline + .1 * uHMax) ? 1. : 0.;
  kSnow *= 1. - .5 * texture2D(uFootprint, (vKWorld.xz + 100.) / 200.).r;
  vec3 kEr = mix(uRock1, uEmberRoot, uEmber);
  vec3 kRock = kH < .06 ? mix(kEr, uRock1, clamp(kH / .06, 0., 1.)) : mix(uRock1, uRock0, clamp((kH - .06) / .94, 0., 1.));
  vec3 kSnowC = mix(uIce, uSnow, clamp((vKFaceH - uSnowline) / (.15 * uHMax), 0., 1.));
  diffuseColor.rgb = mix(kRock, kSnowC, kSnow);
  diffuseColor.rgb = mix(diffuseColor.rgb, uTrail, smoothstep(.35, .75, vKBench) * (1. - kSnow));
  ${veins ? 'kVeins(vKObj);' : ''}
  diffuseColor.rgb = mix(diffuseColor.rgb, uGold * .55, kVein);
  diffuseColor.rgb = mix(diffuseColor.rgb, vec3(dot(diffuseColor.rgb, vec3(.2126, .7152, .0722))) * .6, uStale * .5);`);
  f = afterIf(f, '#include <metalnessmap_fragment>', 'metalnessFactor = mix(metalnessFactor, .85, kVein);');
  f = afterIf(f, '#include <roughnessmap_fragment>', 'roughnessFactor = mix(roughnessFactor, .4, kVein);');
  f = after(f, '#include <emissivemap_fragment>', `
  float kCave = 0.; // cave rim lift (09 Freeze item 1): snowLift-style, rock within ~24 m of a mouth, no new light
  for (int i = 0; i < 5; i++) kCave = max(kCave, uCaves[i].w * (1. - smoothstep(8., 24., distance(vKWorld, uCaves[i].xyz))));
  totalEmissiveRadiance += max(kSnow, kCave) * uSnowLift * diffuseColor.rgb
    + uGold * kVein * .12 + mix(uGold * .7, uVeinRef * 1.4, kRef) * kCore
    + uPoolCol * max(uHeat, .45) * uEmber * (1. - smoothstep(.25, 1., length((vKWorld.xz - uPool.xy) / uPool.zw)))
      * smoothstep(.45, .75, abs(kN.y)) * (1. - smoothstep(.1, .16, kH)); // the foot band (h < ~10 % of the summit), gentle faces only, never up the face`);
  // Rim (09 Freeze item 2): below y = -4 the terrain dissolves into the fog/haze colour, fully at the
  // valley haze floor (y = -16), so the dropped rim never shows a hard mesa edge against the floor.
  f = after(f, '#include <fog_fragment>', '#ifdef USE_FOG\ngl_FragColor.rgb = mix(gl_FragColor.rgb, fogColor, 1. - smoothstep(-16., -4., vKWorld.y));\n#endif');
  sh.fragmentShader = f;
}

// Clip only (lit/basic parts of a clipped model, e.g. cave mouths and rocks): the terrain's uClip discard.
export function patchClip(sh, U) {
  sh.uniforms.uClip = U.uClip;
  sh.vertexShader = after('varying vec3 vKClipW;\n' + sh.vertexShader, '#include <project_vertex>',
    'vec4 kCW = vec4(transformed, 1.);\n#ifdef USE_INSTANCING\nkCW = instanceMatrix * kCW;\n#endif\nvKClipW = (modelMatrix * kCW).xyz;');
  sh.fragmentShader = after('uniform vec4 uClip; varying vec3 vKClipW;\n' + sh.fragmentShader, '#include <clipping_planes_fragment>',
    'if (dot(uClip.xyz, vKClipW) + uClip.w < 0.) discard;');
}

// Crystal: instanceColor = ramp('group') body; per-instance aHealth (vec3 on ramp('health')) × aGlow drives emissive.
export function patchCrystal(sh) {
  sh.vertexShader = after('attribute vec3 aHealth;\nattribute float aGlow;\nvarying vec3 vKGlow;\n' + sh.vertexShader,
    '#include <begin_vertex>', 'vKGlow = aHealth * aGlow;');
  sh.fragmentShader = after('varying vec3 vKGlow;\n' + sh.fragmentShader, '#include <emissivemap_fragment>', 'totalEmissiveRadiance += vKGlow;');
}

// Lava: instance colour (ramp('lava')) also drives emissive × uLavaHeat (kit caps it at 1.6).
export function patchLava(sh, U) {
  Object.assign(sh.uniforms, U);
  sh.fragmentShader = after('uniform float uLavaHeat;\n' + sh.fragmentShader, '#include <emissivemap_fragment>',
    '#if defined( USE_COLOR ) || defined( USE_INSTANCING_COLOR )\ntotalEmissiveRadiance += vColor.rgb * uLavaHeat;\n#endif');
}

// Sky dome: gradient sky0 → sky1 → horizon + hashed star field (0 extra draws). Uniforms written only by kit.setPhase.
export const domeVert = 'varying vec3 vD;\nvoid main() { vD = normalize(position); gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.); }';
export const domeFrag = `uniform vec3 uSky0, uSky1, uHorizon, uInk; uniform float uStars, uStale; varying vec3 vD;
float hash(vec3 p) { return fract(sin(dot(p, vec3(127.1, 311.7, 74.7))) * 43758.5453); }
void main() {
  vec3 n = normalize(vD); float y = clamp(n.y, 0., 1.);
  vec3 col = y < .18 ? mix(uHorizon, uSky1, y / .18) : mix(uSky1, uSky0, clamp((y - .18) / .4, 0., 1.));
  vec3 q = n * 260.; float h = hash(floor(q));
  float star = step(.992, h) * smoothstep(.45, .1, length(fract(q) - .5)) * (.5 + 2.5 * fract(h * 97.)) * smoothstep(.04, .2, n.y);
  gl_FragColor = vec4((col + uInk * star * uStars) * (1. - .45 * uStale * smoothstep(0., .18, y)), 1.); // stale: the sky dims (kit.setStale), fading to 0 at the horizon to meet the undimmed fog/floor
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}`;

// River ribbons: scrolled foam from uTime only, fog on.
export const flowVert = `varying vec2 vUv;\n#include <fog_pars_vertex>
void main() { vUv = uv; vec4 mvPosition = modelViewMatrix * vec4(position, 1.); gl_Position = projectionMatrix * mvPosition;\n#include <fog_vertex>\n}`;
export const flowFrag = `uniform vec3 uColor; uniform float uTime, uOpacity; varying vec2 vUv;\n#include <fog_pars_fragment>
void main() { float foam = step(.9, fract(vUv.x * 8. - uTime * .5 + sin(vUv.y * 6.) * .1));
  gl_FragColor = vec4(uColor * (1. + .4 * foam), uOpacity);\n#include <tonemapping_fragment>\n#include <colorspace_fragment>\n#include <fog_fragment>\n}`;

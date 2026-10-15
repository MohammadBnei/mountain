// Post (01 "Post", 00 R2): desktop only, one composer, orchestrator-owned. Models only write uniforms.
//   RenderPass → UnrealBloom (threshold 1, HalfFloat) → haze ShaderPass (enabled while kit.U.uHeat or uHaze > 0) → OutputPass,
//   MSAA samples 4. Emissive above 1 = glow; lit surfaces stay < 1 (09 luminance rule), so only emissives bloom.
// Tone mapping: OutputPass reads renderer.toneMapping / toneMappingExposure / outputColorSpace, which kit.rig pins,
// so both paths share one operator and exposure. The RenderPass target is linear: fog mixes before tone mapping, so
// kit pre-exposes the fog colour only on the direct path (kit.setComposed, called here).
//
//   createPost(renderer, scene, camera) → { render(t), setSize(w, h), composer, bloom, haze } | null (phones / low)
import * as THREE from 'three';
import * as kit from './kit.js';
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js';
import { ShaderPass } from 'three/addons/postprocessing/ShaderPass.js';
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js';

// Heat haze: a vertical shimmer. uHeat (magma, metrics.heat) at the bottom of the frame (the hot foot); uHaze (shimmer,
// cpu) inside uMask, a world sphere (x, y, z, r) projected here to a screen ellipse each frame (r 0 = the foot band too).
const HAZE = {
  uniforms: { tDiffuse: { value: null }, uHeat: kit.U.uHeat, uHaze: kit.U.uHaze, uMaskUV: { value: new THREE.Vector4() }, uTime: { value: 0 } },
  vertexShader: 'varying vec2 vUv;\nvoid main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.); }',
  fragmentShader: `uniform sampler2D tDiffuse; uniform float uHeat, uHaze, uTime; uniform vec4 uMaskUV; varying vec2 vUv;
void main() {
  float foot = 1. - smoothstep(.15, .6, vUv.y);
  float a = .0025 * (uHeat * foot + uHaze * (uMaskUV.z > 0. ? 1. - smoothstep(.25, 1., length((vUv - uMaskUV.xy) / uMaskUV.zw)) : foot));
  vec2 o = vec2(sin(vUv.y * 90. + uTime * 4.) + .5 * sin(vUv.y * 211. - uTime * 7.), 0.) * a;
  gl_FragColor = texture2D(tDiffuse, vUv + o);
}`,
};

export function createPost(renderer, scene, camera) {
  if (kit.QUALITY === 'low') return null;
  const composer = new EffectComposer(renderer, new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 }));
  const bloom = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.6, 0.4, 1);
  const haze = new ShaderPass(HAZE);
  haze.uniforms.uHeat = kit.U.uHeat; // ShaderPass clones uniforms; keep the kit's live objects
  haze.uniforms.uHaze = kit.U.uHaze;
  const c = new THREE.Vector3(), m = kit.U.uMask.value, uv = haze.uniforms.uMaskUV.value;
  for (const p of [new RenderPass(scene, camera), bloom, haze, new OutputPass()]) composer.addPass(p);
  kit.setComposed(true);
  return {
    composer, bloom, haze,
    setSize: (w, h) => composer.setSize(w, h), // composer keeps the renderer's pixel ratio
    render(t) {
      haze.enabled = kit.U.uHeat.value > 0 || kit.U.uHaze.value > 0;
      if (m.w > 0) { // the mask sphere's screen centre and radii (uv); behind the camera = nowhere
        const d = c.set(m.x, m.y, m.z).distanceTo(camera.position), ry = m.w / (2 * Math.max(d, 1e-3) * Math.tan((camera.fov * Math.PI) / 360));
        c.project(camera);
        if (c.z > 1) uv.set(9, 9, 0.01, 0.01);
        else uv.set(c.x * 0.5 + 0.5, c.y * 0.5 + 0.5, ry / camera.aspect, ry);
      } else uv.set(0, 0, 0, 0);
      haze.uniforms.uTime.value = t;
      composer.render();
    },
  };
}

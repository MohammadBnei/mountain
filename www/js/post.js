// Post (01 "Post", 00 R2): desktop only, one composer, orchestrator-owned. Models only write uniforms.
//   RenderPass → UnrealBloom (threshold 1, HalfFloat) → haze ShaderPass (enabled while kit.U.uHeat > 0) → OutputPass,
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

// Heat haze: a vertical shimmer, strongest at the bottom of the frame (the hot foot), amplitude uHeat.
const HAZE = {
  uniforms: { tDiffuse: { value: null }, uHeat: kit.U.uHeat, uTime: { value: 0 } },
  vertexShader: 'varying vec2 vUv;\nvoid main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.); }',
  fragmentShader: `uniform sampler2D tDiffuse; uniform float uHeat, uTime; varying vec2 vUv;
void main() {
  float a = .0025 * uHeat * (1. - smoothstep(.15, .6, vUv.y));
  vec2 o = vec2(sin(vUv.y * 90. + uTime * 4.) + .5 * sin(vUv.y * 211. - uTime * 7.), 0.) * a;
  gl_FragColor = texture2D(tDiffuse, vUv + o);
}`,
};

export function createPost(renderer, scene, camera) {
  if (kit.QUALITY === 'low') return null;
  const composer = new EffectComposer(renderer, new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 }));
  const bloom = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.6, 0.4, 1);
  const haze = new ShaderPass(HAZE);
  haze.uniforms.uHeat = kit.U.uHeat; // ShaderPass clones uniforms; keep the kit's live object
  for (const p of [new RenderPass(scene, camera), bloom, haze, new OutputPass()]) composer.addPass(p);
  kit.setComposed(true);
  return {
    composer, bloom, haze,
    setSize: (w, h) => composer.setSize(w, h), // composer keeps the renderer's pixel ratio
    render(t) {
      haze.enabled = kit.U.uHeat.value > 0;
      haze.uniforms.uTime.value = t;
      composer.render();
    },
  };
}

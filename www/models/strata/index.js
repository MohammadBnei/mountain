// strata (03-model-strata): the reference cut-face field provider (cut.js). 0 draws: it colours the mountain's cut face.
// Platform bands by height (proxmox → k8s → apps, the Aretē strata; k8s in sediment beds, deep seams between them) and
// ≤ 32 ash lines from world:ash (one per ADR day, 02): the newest at y TOP, each older one RATE m lower per day of age,
// so time buries them under the haze floor. pick(ray) → {day, title?} of the ash line under the ray on the cut face.
import { pick as cutPick } from '../../js/cut.js';

const MAX = 32, TOP = 30, RATE = 0.25, DAY = /^\d{4}-\d{2}-\d{2}$/;
const GLSL = `uniform sampler2D uStrataAsh; uniform int uStrataN; uniform vec3 uStrataEdge, uStrataStone, uStrataStone2, uStrataSeam, uStrataAshC;
float strataLine(float y, float at, float hw, float fw) { return 1. - smoothstep(hw - fw, hw + fw, abs(y - at)); }
vec4 strataField(vec3 p, vec4 base) {
  float y = p.y, fw = fwidth(y), s = sin(y * 3.14159 / uStrataEdge.z), fs = fwidth(s);
  vec3 c = y < uStrataEdge.x ? uStrataStone : y < uStrataEdge.y ? mix(uStrataStone, uStrataStone2, smoothstep(-fs, fs, s)) : uStrataStone2;
  c = mix(c, uStrataSeam, max(strataLine(y, uStrataEdge.x, .35, fw), strataLine(y, uStrataEdge.y, .35, fw)));
  float a = 0.;
  for (int i = 0; i < ${MAX}; i++) { if (i >= uStrataN) break; a = max(a, strataLine(y, texelFetch(uStrataAsh, ivec2(i, 0), 0).r, .22, fw)); }
  return vec4(mix(c, uStrataAshC, a), base.a);
}`;

export default async function build({ THREE, kit, world }) {
  const params = { proxmox: 4, k8s: 34, bed: 3 }; // platform band tops (y, m) and the k8s sediment bed (m)
  const ys = new Float32Array(MAX), tex = new THREE.DataTexture(ys, MAX, 1, THREE.RedFormat, THREE.FloatType);
  const field = kit.field(GLSL, { uStrataAsh: tex, uStrataN: 0, uStrataEdge: new THREE.Vector3(), uStrataStone: 'stone', uStrataStone2: 'stone2', uStrataSeam: 'deep', uStrataAshC: 'cloudLit' });
  const ash = []; // [{day, title?, y}] newest first
  const days = (d) => Date.parse(`${d}T00:00:00Z`) / 864e5;
  const pose = () => field.uniforms.uStrataEdge.value.set(params.proxmox, params.k8s, params.bed);
  pose();

  return {
    root: new THREE.Group(),
    params,
    field,
    pose,
    step() {},
    on: {
      'world:ash'({ day, title }) {
        if (!DAY.test(day) || Number.isNaN(days(day)) || ash.some((a) => a.day === day && a.title === title)) return;
        ash.push({ day, ...(typeof title === 'string' ? { title: title.slice(0, 120) } : {}) }); // the relay allowlists titles; cap anyway
        ash.sort((a, b) => (a.day < b.day ? 1 : a.day > b.day ? -1 : 0));
        ash.length = Math.min(ash.length, MAX);
        ash.forEach((a, i) => (ys[i] = a.y = TOP - (days(ash[0].day) - days(a.day)) * RATE));
        field.uniforms.uStrataN.value = ash.length;
        tex.needsUpdate = true;
      },
    },
    pick(ray) {
      const p = cutPick(ray, world), a = p && ash.find((x) => Math.abs(p.y - x.y) < 0.6);
      return a ? { day: a.day, ...(a.title ? { title: a.title } : {}) } : null;
    },
    busy: () => false,
    dispose() { tex.dispose(); },
  };
}

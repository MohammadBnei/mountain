// R9 Civ: the golden age (civ.js goldenAge: no warning or critical alert and no Degraded or Missing app for
// GOLDEN_DAYS). The town puts out bunting once, not on each building: pennants strung along every bridge and in a ring
// over each island's square, in the Ukubi wire colours, lit at night like the roofs. One instanced mesh.
import { plotXZ, islandsOf } from '../layout.js';
import { goldenAge } from '../civ.js';

export default async function build({ THREE, kit, world }) {
  const root = new THREE.Group();
  const flag = new THREE.BufferGeometry();
  flag.setAttribute('position', new THREE.Float32BufferAttribute([-0.09, 0, 0, 0.09, 0, 0, 0, -0.2, 0, 0.09, 0, 0, -0.09, 0, 0, 0, -0.2, 0], 3)); // both faces
  flag.computeVertexNormals();
  flag.setAttribute('uv', new THREE.Float32BufferAttribute(new Array(12).fill(0.5), 2));
  const mat = kit.glowing();
  const spots = [];
  const line = (a, b, h, sag) => { // pennants every ~0.45 along a → b, hanging in a curve
    const n = Math.max(2, Math.round(Math.hypot(b[0] - a[0], b[1] - a[1]) / 0.45));
    for (let i = 1; i < n; i++) {
      const f = i / n;
      spots.push({ x: a[0] + (b[0] - a[0]) * f, y: h - sag * 4 * f * (1 - f), z: a[1] + (b[1] - a[1]) * f, yaw: Math.atan2(b[0] - a[0], b[1] - a[1]) + Math.PI / 2 });
    }
  };
  for (const br of world.town.bridges) line(plotXZ(world.town, br.a, br.aq, br.ar), plotXZ(world.town, br.b, br.bq, br.br), 1.7, 0.35);
  for (const is of islandsOf(world.town)) { // a ring over the square, hung between six posts' heights
    const [cx, cz] = plotXZ(world.town, is.host, 0, 0), R = 1.55, pts = Array.from({ length: 7 }, (_, i) => [cx + R * Math.cos((i * Math.PI) / 3), cz + R * Math.sin((i * Math.PI) / 3)]);
    for (let i = 0; i < 6; i++) line(pts[i], pts[i + 1], 1.9, 0.25);
  }
  const mesh = kit.instanced(flag, mat, spots.length, false);
  mesh.frustumCulled = false;
  root.add(mesh);
  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3(1, 1, 1), C = new THREE.Color();
  const wires = Object.keys(kit.ukubi.wires.office);
  let shown = null;
  return {
    root,
    update(dt, t, night = 0) {
      const on = goldenAge(world).on;
      mat.emissiveIntensity = 0.6 * night;
      if (on !== shown) {
        shown = on;
        mesh.count = on ? spots.length : 0;
      }
      if (!on) return false;
      const set = kit.ukubi.wires[night > 0.5 ? 'glow' : 'office'];
      spots.forEach((s, i) => {
        M.compose(P.set(s.x, s.y, s.z), Q.setFromEuler(E.set(0.15 * Math.sin(t * 2 + i), s.yaw, 0)), S);
        mesh.setMatrixAt(i, M);
        mesh.setColorAt(i, C.set(set[wires[i % wires.length]]));
      });
      mesh.instanceMatrix.needsUpdate = true;
      if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;
      return true;
    },
  };
}

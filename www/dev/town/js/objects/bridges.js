// Bridges (R7): a wooden drawbridge between each pair of islands, head to head (hello.town.bridges). A bridge to a
// Proxmox host out of quorum (ev quorum) is raised: both halves swing up at their island's end.
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { hexXZ } from '../layout.js';

const RAISE = 0.95; // radians, a raised half
const DUR = 1.6;    // seconds to raise or lower

export default async function build({ THREE, kit, world }) {
  const root = new THREE.Group();
  const halves = [];
  for (const b of world.town.bridges) {
    const ia = world.town.islands.find((i) => i.host === b.a), ib = world.town.islands.find((i) => i.host === b.b);
    const [ax, az] = hexXZ(ia.q + b.aq, ia.r + b.ar), [bx, bz] = hexXZ(ib.q + b.bq, ib.r + b.br);
    const dx = bx - ax, dz = bz - az, len = Math.hypot(dx, dz), ux = dx / len, uz = dz / len;
    const startA = 0.9, span = len - 2 * startA, half = span / 2; // from each head tile's edge
    const yaw = Math.atan2(ux, uz);
    for (const [end, sign] of [[[ax + ux * startA, az + uz * startA], 1], [[bx - ux * startA, bz - uz * startA], -1]]) {
      // one half: deck planks + two rails, built along +z from its pivot at the island end
      const parts = [];
      const n = Math.max(4, Math.round(half / 0.32));
      for (let i = 0; i < n; i++) parts.push(new THREE.BoxGeometry(1.1, 0.08, half / n - 0.04).translate(0, -0.04, (i + 0.5) * (half / n)));
      for (const s of [-1, 1]) {
        parts.push(new THREE.BoxGeometry(0.06, 0.06, half).translate(s * 0.55, 0.32, half / 2));
        for (let i = 0; i <= 2; i++) parts.push(new THREE.BoxGeometry(0.06, 0.34, 0.06).translate(s * 0.55, 0.15, (i / 2) * (half - 0.05) + 0.025));
      }
      const pivot = new THREE.Group();
      pivot.position.set(end[0], 0.02, end[1]);
      pivot.rotation.y = sign > 0 ? yaw : yaw + Math.PI;
      const tilt = new THREE.Group(); // rotates about the pivot's x axis to raise the half
      tilt.add(kit.mesh(mergeGeometries(parts), kit.palette.wood));
      pivot.add(tilt);
      root.add(pivot);
      halves.push({ tilt, a: b.a, b: b.b, k: 0, want: 0 });
    }
  }
  return {
    root,
    /** → true while moving (render). */
    update(dt) {
      let busy = false;
      const up = world.quorum;
      for (const h of halves) {
        h.want = up && (!up.includes(h.a) || !up.includes(h.b)) ? 1 : 0;
        if (h.k !== h.want) {
          h.k = h.want > h.k ? Math.min(h.want, h.k + dt / DUR) : Math.max(h.want, h.k - dt / DUR);
          busy = true;
        }
        h.tilt.rotation.x = -RAISE * kit.easeInOut(h.k);
      }
      return busy;
    },
  };
}

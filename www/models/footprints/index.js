// footprints (03-model-footprints): a click-path on the snow. The tool turns a drag on the terrain into {pos, path}
// (≤ 8 points ≥ one cell apart, within REACH of the first: toy_footprints.go checks and quantises it); every footprints
// mark in world.state.marks is stamped into the kit footprint RT as one kit stamp() quad per path segment (a lone point
// = one square), instance colour red = K × weight. The relay erodes the weight (weather, rain) and re-emits the mark;
// the stamps are additive, so trails where many walk saturate the snow to trodden. Nothing in the main scene (root is
// empty): the quads live in kit.stampScene, redrawn only when a footprints mark arrives or world.state is replaced.
import { CELL } from '../../js/world.js';

const CAP = 16000, WIDE = 1.2, K = 0.4, REACH = 15, PTS = 8; // 2000 marks × ≤ 8 quads; m wide; red per walker; m; points

export default async function build({ THREE, kit, world }) {
  const mesh = new THREE.InstancedMesh(new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2), kit.stamp(), CAP);
  mesh.name = 'footprints';
  mesh.instanceColor = new THREE.InstancedBufferAttribute(new Float32Array(CAP * 3), 3);
  mesh.count = 0;
  mesh.frustumCulled = false;
  kit.stampScene.add(mesh);
  const root = new THREE.Group(), M = new THREE.Matrix4(), q = new THREE.Quaternion(), Y = new THREE.Vector3(0, 1, 0), P = new THREE.Vector3(), S = new THREE.Vector3();

  let seen = null, dirty = true;
  function stamp() {
    seen = world.state; dirty = false;
    const col = mesh.instanceColor.array;
    let n = 0;
    const quad = (x0, z0, x1, z1, r) => {
      const dx = x1 - x0, dz = z1 - z0;
      q.setFromAxisAngle(Y, Math.atan2(dx, dz)); // local +z along the segment
      mesh.setMatrixAt(n, M.compose(P.set((x0 + x1) / 2, 0, (z0 + z1) / 2), q, S.set(WIDE, 1, Math.hypot(dx, dz) || WIDE)));
      col[n * 3] = r; col[n * 3 + 1] = col[n * 3 + 2] = 0;
      n++;
    };
    for (const m of world.state.marks || []) {
      if (m.mark !== 'footprints') continue;
      const p = m.pose?.path || [], r = K * m.weight;
      if (p.length === 2 && n < CAP) quad(p[0], p[1], p[0], p[1], r);
      // ponytail: segments meet without a joint, so a sharp turn leaves a sub-texel wedge (0.39 m texels)
      for (let i = 0; i + 3 < p.length && n < CAP; i += 2) quad(p[i], p[i + 1], p[i + 2], p[i + 3], r);
    }
    mesh.count = n;
    mesh.instanceMatrix.needsUpdate = mesh.instanceColor.needsUpdate = true;
    kit.stampScene.userData.dirty = true;
  }
  stamp();

  // the tool: a walk from the drag's first terrain hit
  let walk = null;
  const tool = {
    icon: '∴', label: 'tool.footprints',
    drag(phase, hit) {
      if (phase === 'start') walk = { pos: [hit.x, hit.y, hit.z], pts: [[hit.x, hit.z]] };
      if (!walk) return;
      const last = walk.pts.at(-1);
      if (hit && Math.hypot(hit.x - last[0], hit.z - last[1]) >= CELL && Math.hypot(hit.x - walk.pos[0], hit.z - walk.pos[2]) <= REACH) walk.pts.push([hit.x, hit.z]);
      if (phase === 'cancel') walk = null;
      if (phase !== 'end') return;
      const { pos, pts } = walk, k = pts.length;
      walk = null;
      const path = (k > PTS ? Array.from({ length: PTS }, (_, i) => pts[Math.round((i * (k - 1)) / (PTS - 1))]) : pts).flat();
      world.place('footprints', { pos, path }).catch(() => {}); // the toolbar shows the reason; the stamp comes with world:mark
    },
  };

  return {
    root,
    params: {},
    tool,
    pose() {},
    step() { if (dirty || world.state !== seen) stamp(); }, // a hello or replay replaces world.state
    on: { 'world:mark'(p) { if (p.mark === 'footprints') dirty = true; } },
    busy: () => dirty,
    dispose() { kit.stampScene.remove(mesh); kit.dispose(mesh); kit.stampScene.userData.dirty = true; },
  };
}

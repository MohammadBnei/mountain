// sky-min (03-model-sky, 09 puff clusters): the cumulus LAYER around and below the summit + the real cloud MASS
// over it, both from net rx (metrics.rx, 0..1, log-normalised by the relay). Two InstancedMeshes of flat-based
// lumps on kit lit('cloud'), colour per lump on ramp('cloud') by its height in the cluster (board.html shapes).
// Writes world.clouds [{x, z, y (base), r, mass}] (mass first: lightning's bolt source) and world.rig at the dusk
// default (SunCalc is W3). The dome + stars are the kit rig's (dome(), uStars per key, uniforms by setPhase); sky
// only eases kit.setStale toward world.stale. Never creates lights.
import { cellXZ, GRID } from '../../js/world.js';

const TAU_RX = 4, TAU_STALE = 1, BASE = 13; // mass base 13 m over the summit: above the St Elmo corona
const DUSK = { elev: 0, rising: false };

/** n lumps as flat [x, y, z, size, t] (t: 0 base .. 1 top → ramp('cloud')). Same shape as plan/style/board.html. */
function cluster(r, n, [sx, sz], height, s0, s1, taper = 0.5, pow = 2) {
  const out = new Float32Array(n * 5);
  for (let i = 0; i < n; i++) {
    const s = s0 + r() * (s1 - s0), u = Math.pow(r(), pow), y = u * height;
    out.set([(r() - 0.5) * sx * (1 - taper * u), y + s * 0.14, (r() - 0.5) * sz * (1 - taper * u), s, Math.min(1, y / Math.max(1, height * 0.6))], i * 5);
  }
  return out;
}

export default async function build({ THREE, kit, world }) {
  const lump = new THREE.IcosahedronGeometry(1, kit.QUALITY === 'low' ? 0 : 1), p = lump.attributes.position;
  for (let i = 0; i < p.count; i++) if (p.getY(i) < -0.25) p.setY(i, -0.25);
  lump.computeVertexNormals();
  const mat = kit.lit('cloud');

  const LAYER = Array.from({ length: 11 }, (_, i) => {
    const r = kit.mulberry(300 + i), a = -2.4 + i * 0.44 + (r() - 0.5) * 0.2, rad = 72 + r() * 45; // open towards the camera (+z)
    return { x: Math.sin(a) * rad * 1.3, z: -Math.cos(a) * rad * 0.7 - 10, y: 26 + r() * 12, r: 22, lumps: cluster(kit.mulberry(400 + i), 30 + Math.floor(r() * 14), [44, 20], 7, 1.8, 5) };
  });
  // offsets from the summit; the second tower only joins when inbound traffic is high
  const MASS = [
    { dx: -16, dz: -20, r: 50, lumps: cluster(kit.mulberry(901), 120, [100, 50], 44, 5, 12, 0.75, 0.8) },
    { dx: 40, dz: -40, r: 32, lumps: cluster(kit.mulberry(902), 60, [64, 36], 28, 4.5, 10, 0.7, 0.9) },
  ];
  const mesh = (sets, name) => {
    const n = sets.reduce((a, c) => a + c.lumps.length / 5, 0), im = new THREE.InstancedMesh(lump, mat, n);
    im.name = name;
    let i = 0;
    for (const c of sets) for (let j = 0; j < c.lumps.length; j += 5) kit.setRampAt(im, i++, 0.35 + 0.65 * c.lumps[j + 4]);
    return im;
  };
  const layer = mesh(LAYER, 'layer'), mass = mesh(MASS, 'mass'), n0 = MASS[0].lumps.length / 5;
  const root = new THREE.Group();
  layer.frustumCulled = mass.frustumCulled = false; // instances move with rx; the sky is always in frame anyway
  root.add(layer, mass);

  // world.clouds entries, mutated in place (step is allocation-free)
  const C = [...MASS, ...LAYER].map((c) => ({ x: c.x ?? 0, z: c.z ?? 0, y: c.y ?? 0, r: c.r, mass: !c.y }));
  const clouds = [];
  const fill = (out) => { out.length = 0; for (let i = 0; i < C.length; i++) if (i !== 1 || mass.count > n0) out.push(C[i]); };

  const O = new THREE.Object3D();
  let m = world.metrics?.rx ?? 0, drawn = -1, hDrawn = NaN, sx = 0, sz = 0, st = world.stale != null ? 1 : 0;
  const put = (im, i, x, y, z, s, sy) => { O.position.set(x, y, z); O.scale.set(s * 1.15, s * sy, s); O.updateMatrix(); im.setMatrixAt(i, O.matrix); };
  function place() {
    if (world.hmax !== hDrawn) { // summit cell (deposits and resyncs move it)
      const k = world.fields.heightSolid.indexOf(world.hmax);
      [sx, sz] = cellXZ(k % GRID, Math.floor(k / GRID));
      hDrawn = world.hmax;
    }
    const kl = 0.7 + 0.3 * m, km = 0.55 + 0.45 * m, ks = 0.75 + 0.25 * m, base = world.hmax + BASE; // footprint, height, lump size
    let i = 0;
    for (let q = 0; q < LAYER.length; q++) for (let j = 0, c = LAYER[q], L = c.lumps; j < L.length; j += 5) put(layer, i++, c.x + L[j], c.y + L[j + 1], c.z + L[j + 2], L[j + 3] * kl, 0.55);
    i = 0;
    for (let q = 0; q < MASS.length; q++) {
      const c = MASS[q], L = c.lumps, x = sx + c.dx * km, z = sz + c.dz * km;
      for (let j = 0; j < L.length; j += 5) put(mass, i++, x + L[j] * km, base + L[j + 1] * km, z + L[j + 2] * km, L[j + 3] * ks, 0.8);
      C[q].x = x; C[q].z = z; C[q].y = base; C[q].r = c.r * km;
    }
    mass.count = m >= 0.5 ? i : n0;
    layer.instanceMatrix.needsUpdate = mass.instanceMatrix.needsUpdate = true;
    world.write('clouds', fill);
    drawn = m;
  }
  world.write('clouds', clouds);
  world.write('rig', DUSK);
  kit.setStale(st);
  place();

  return {
    root,
    params: { tauRx: TAU_RX, tauStale: TAU_STALE, base: BASE },
    pose() {}, // sky is not on the scroll story: its state is the live metrics
    step(dt) {
      const rx = world.metrics?.rx ?? 0;
      m = Math.abs(rx - m) < 0.002 ? rx : m + (rx - m) * Math.min(1, dt / TAU_RX);
      st += ((world.stale != null ? 1 : 0) - st) * Math.min(1, dt / TAU_STALE);
      kit.setStale(st);
      if (Math.abs(m - drawn) > 0.001 || world.hmax !== hDrawn) place();
    },
    busy: () => Math.abs((world.metrics?.rx ?? 0) - m) > 0.002 || Math.abs((world.stale != null ? 1 : 0) - st) > 0.01,
    dispose() { kit.dispose(root); },
  };
}

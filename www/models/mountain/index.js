// mountain (03-model-mountain): the Go terrain h.base + h.land as one closed flat-shaded body,
// 3 bedrock slabs h1..h3 (thickness = alloc, colour = ready/load), kit cave mouths at Go caves[].
import U from '../../js/units.json' with { type: 'json' };

export default async function build({ THREE, kit, world }) {
  const G = kit.QUALITY === 'low' ? U.GRID_LOW : U.GRID, Q = G - 1;
  const CELL = U.EXTENT / U.GRID, SPAN = CELL * (U.GRID - 1), S = SPAN / Q, X0 = -U.EXTENT / 2, YB = -70;
  const vx = (i) => X0 + i * S;

  // vertex heights on the G² render grid, sampled from heightSolid (exact cells at 128²)
  const vh = new Float32Array(G * G);
  const sample = (j0, j1) => { for (let j = j0; j <= j1; j++) for (let i = 0; i < G; i++) vh[j * G + i] = world.heightAt(vx(i), vx(j)); };
  sample(0, Q);

  const TOP = Q * Q * 6, SK = 4 * Q * 6, n = TOP + SK + 6;
  const pos = new Float32Array(n * 3), bench = new Float32Array(n);
  let k = 0;
  const put = (x, y, z, b = 0) => { pos[k * 3] = x; pos[k * 3 + 1] = y; pos[k * 3 + 2] = z; bench[k++] = b; };
  const v = (p, q) => put(vx(p), vh[q * G + p], vx(q), world.bench[world.cell(vx(p), vx(q))] / 255);
  // quad (i, j) → 6 vertices at ((j*Q + i) * 6): tris (a, b, c), (c, b, d), +y facing; allocation-free
  const quad = (i, j) => { k = (j * Q + i) * 6; v(i, j); v(i, j + 1); v(i + 1, j); v(i + 1, j); v(i, j + 1); v(i + 1, j + 1); };
  for (let j = 0; j < Q; j++) for (let i = 0; i < Q; i++) quad(i, j);
  // skirt down to YB along the 4 edges, then the bottom: a closed body (cut face, W3)
  for (const e of [(t) => [t, 0], (t) => [t, Q], (t) => [0, t], (t) => [Q, t]]) for (let t = 0; t < Q; t++) {
    const [pi, pj] = e(t), [qi, qj] = e(t + 1), py = vh[pj * G + pi], qy = vh[qj * G + qi];
    put(vx(pi), py, vx(pj)); put(vx(pi), YB, vx(pj)); put(vx(qi), qy, vx(qj));
    put(vx(qi), qy, vx(qj)); put(vx(pi), YB, vx(pj)); put(vx(qi), YB, vx(qj));
  }
  const x1 = vx(Q);
  put(X0, YB, X0); put(x1, YB, X0); put(X0, YB, x1); put(x1, YB, X0); put(x1, YB, x1); put(X0, YB, x1);

  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  geo.setAttribute('bench', new THREE.BufferAttribute(bench, 1));
  const mat = kit.terrain();
  mat.side = THREE.DoubleSide; // closed body, back-face cap in cut.js (W3)
  const body = new THREE.Mesh(geo, mat);
  body.name = 'body';

  // bedrock: 3 slabs under the body, side by side along x, non-overlapping
  const slabs = new THREE.InstancedMesh(new THREE.BoxGeometry(1, 1, 1), kit.lit('slabLoad'), 3);
  slabs.name = 'slabs';
  slabs.userData.facet = 'slab';
  const O = new THREE.Object3D(), SW = SPAN / 3;
  let seen;
  const slabsFrom = (m) => {
    seen = m;
    ['h1', 'h2', 'h3'].forEach((h, i) => {
      const s = m?.hosts?.[h] || { alloc: 0, ready: false, load: 0 };
      // ponytail: alloc read as a 0..1 share (bucketed by the relay); saturates if it is ever absolute
      const th = 6 + 14 * Math.min(1, Math.max(0, s.alloc));
      O.position.set(X0 + SW * (i + 0.5), YB - 0.5 - th / 2, X0 + SPAN / 2);
      O.scale.set(SW - 1, th, SPAN);
      O.updateMatrix();
      slabs.setMatrixAt(i, O.matrix);
      if (s.ready) kit.setRampAt(slabs, i, Math.min(1, Math.max(0, s.load)));
      else slabs.setColorAt(i, kit.color('slabNotReady'));
    });
    slabs.instanceMatrix.needsUpdate = true;
    slabs.instanceColor.needsUpdate = true;
  };
  slabsFrom(world.metrics);

  const root = new THREE.Group();
  root.add(body, slabs, kit.cave(world.caves, world.heightAt));

  // deposits: re-sample + rewrite the affected quad rows while world.js ramps h.land (3 s)
  const regions = [];
  const row = (cz) => (cz * CELL) / S;
  function refresh(r) {
    const j0 = Math.max(0, Math.floor(row(r.z0 - 1))), j1 = Math.min(Q, Math.ceil(row(r.z0 + r.h)));
    sample(j0, j1);
    const q0 = Math.max(0, j0 - 1), q1 = Math.min(Q - 1, j1);
    for (let j = q0; j <= q1; j++) for (let i = 0; i < Q; i++) quad(i, j);
    // ponytail: skirt not re-sampled; Go sites deposits on the massif, never on the rim
    const a = geo.attributes.position;
    a.addUpdateRange(q0 * Q * 18, (q1 - q0 + 1) * Q * 18);
    a.needsUpdate = true;
  }

  return {
    root,
    params: {},
    pose() {},
    step(dt) {
      if (world.metrics !== seen) slabsFrom(world.metrics);
      for (let i = regions.length - 1; i >= 0; i--) {
        const r = regions[i];
        refresh(r);
        if ((r.age += dt) > 3.1) regions.splice(i, 1);
      }
    },
    on: { 'world:deposit': (d) => regions.push({ z0: d.z0, h: d.h, age: 0 }) },
    busy: () => regions.length > 0,
    dispose() { root.traverse((o) => { o.geometry?.dispose(); o.material?.dispose?.(); }); },
  };
}

// glacier (03-model-glacier): the ice cap and its tongues as one closed 64² shell (heightSolid + the `ice` field this
// model writes), from world.state.glacier (hello, replay) and world:glacier. Cap thickness = mass above world.snowline
// (the relay lowers it as the mass grows); one tongue per app runs down-slope from the cap toward its site; each ash day
// is a thinned band in the ice (newest near the top, RATE m lower per day of age, as strata); cauldron.sag dishes the
// cap; magma's `temp` thins it cosmetically and emits melt/steam (the persisted mass never changes here). A tongue that
// vanishes from world:glacier (app.gone; a melted-out one is < 1 m) calves RoundedBox chunks + one ice.calve.
// Corners with no ice put top = bottom, so the shell closes without walls.
import { RoundedBoxGeometry } from 'three/addons/geometries/RoundedBoxGeometry.js';
import { cellXZ, GRID, ICE } from '../../js/world.js';

const S = 200 / ICE, X0 = -100, Q = ICE - 1;
const CAP = 6, TONGUE_K = 2, TONGUE_W = 4.5, TONGUE_T = 3.5, RATE = 0.25, BAND = 1.6, SINK = 0.4;
const MELT_S = 5, CHUNKS = 12, LIFE = 5, MAXC = 1900; // MAXC × 4 tris + CHUNKS × 108 ≤ the 9k budget
const day = (d) => Date.parse(`${d}T00:00:00Z`) / 864e5;
const smooth = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
const hash = (s) => { let h = 7; for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0; return h; };

export default async function build({ THREE, kit, world, rng }) {
  const NN = ICE * ICE, hv = new Float32Array(NN), tgt = new Float32Array(NN), cur = new Float32Array(NN);
  const nx = (k) => X0 + (k % ICE) * S, nz = (k) => X0 + Math.floor(k / ICE) * S;

  const pos = new Float32Array(MAXC * 12 * 3), geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  const shell = new THREE.Mesh(geo, kit.clipped(kit.lit('ice', { side: THREE.DoubleSide })));
  shell.name = 'shell';
  shell.frustumCulled = false;

  const chunkMat = kit.clipped(kit.lit('ice'));
  const chunks = new THREE.InstancedMesh(new RoundedBoxGeometry(1, 1, 1, 1, 0.15), chunkMat, CHUNKS);
  chunks.name = 'chunks';
  chunks.userData.facet = 'chunk';
  chunks.count = 0;
  chunks.frustumCulled = false;
  const root = new THREE.Group();
  root.add(shell, chunks);

  // chunk state (struct of arrays; step is allocation-free)
  const cp = new Float32Array(CHUNKS * 3), cv = new Float32Array(CHUNKS * 3), cs = new Float32Array(CHUNKS), ca = new Float32Array(CHUNKS).fill(LIFE);
  const O = new THREE.Object3D();

  let g, seen, cavesSeen, sl, sag = null, sx = 0, sz = 0, meltT = 0, ramp = false;
  const heights = () => {
    cavesSeen = world.caves; // a hello replaces the land (world.caves identity marks it, as the mountain)
    for (let k = 0; k < NN; k++) hv[k] = world.heightAt(nx(k), nz(k));
    // ponytail: summit and node heights refresh on a hello only; deposits land on the flanks, not on the cap
    const i = world.fields.heightSolid.indexOf(world.hmax);
    [sx, sz] = cellXZ(i % GRID, Math.floor(i / GRID));
  };
  // tongue axis: toward the app's site, else a stable azimuth from its name; r0 = where the cap ends along it
  const axis = (app) => {
    const s = world.sites.get(app), a = s ? Math.atan2(s.x - sx, s.z - sz) : (hash(app) % 628) / 100;
    const ux = Math.sin(a), uz = Math.cos(a);
    let r0 = 0;
    while (r0 < 100 && world.heightAt(sx + ux * r0, sz + uz * r0) > sl) r0++;
    return { ux, uz, r0 };
  };

  function target() {
    sl = world.snowline;
    const tongues = (g.tongues || []).map((t) => ({ ...axis(t.app), reach: 0, len: t.len }));
    for (const t of tongues) t.reach = t.r0 + t.len * TONGUE_K; // ponytail: ×2 so a 1.5 m step reads at 3 m cells
    const ash = g.ash || [], d0 = ash.length ? day(ash[0]) : 0, bands = ash.map((d) => world.hmax - 1 - (d0 - day(d)) * RATE);
    const temp = world.fields.temp;
    for (let k = 0; k < NN; k++) {
      const x = nx(k), z = nz(k), h = hv[k];
      let t = g.mass * CAP * smooth(sl - 2, sl + 4, h);
      for (const u of tongues) {
        const rx = x - sx, rz = z - sz, al = rx * u.ux + rz * u.uz, w = 1 - Math.abs(rx * u.uz - rz * u.ux) / TONGUE_W;
        if (w > 0 && al > u.r0 - 4 && al < u.reach) t = Math.max(t, TONGUE_T * w * Math.min(1, (u.reach - al) / 3));
      }
      for (const y of bands) t *= 1 - 0.5 * Math.max(0, 1 - Math.abs(h - y) / BAND);
      if (sag?.depth > 0) {
        const r = 6 + 2 * sag.depth, d = Math.hypot(x - sag.x, z - sag.z) / r; // the cauldron ring's outer radius
        if (d < 1) t *= 1 - Math.min(1, sag.depth / 2) * (1 - d * d);
      }
      t *= 1 - 0.6 * Math.min(1, Math.max(0, temp[world.cell(x, z)])); // ponytail: temp read as a 0..1 heat share
      tgt[k] = t < 0.05 ? 0 : t;
    }
    ramp = true;
  }

  let n = 0; // mesh()'s write cursor; put and the ice writer are built once (step allocates nothing per frame)
  const put = (k, bottom) => {
    const c = cur[k];
    pos[n++] = nx(k); pos[n++] = bottom ? hv[k] - SINK * Math.min(1, c) : hv[k] + c; pos[n++] = nz(k);
  };
  const iceW = (f) => f.set(cur);
  function mesh() {
    n = 0;
    for (let j = 0; j < Q && n < pos.length; j++) for (let i = 0; i < Q && n < pos.length; i++) {
      const a = j * ICE + i, b = a + ICE, c = a + 1, d = b + 1;
      if (!(cur[a] || cur[b] || cur[c] || cur[d])) continue;
      put(a); put(b); put(c); put(c); put(b); put(d); // top, +y facing (the mountain's quad order)
      put(a, 1); put(c, 1); put(b, 1); put(c, 1); put(d, 1); put(b, 1); // bottom, reversed
    }
    // ponytail: count, not setDrawRange: the facet gate measures position.count, and a fixed buffer keeps step allocation-free
    geo.attributes.position.count = n / 3;
    geo.attributes.position.needsUpdate = true;
    world.write('ice', iceW);
  }

  // a new state object (hello, replay) or the first build: snap to it, no calving (history)
  const sync = () => {
    seen = world.state;
    g = seen.glacier || { mass: 0, tongues: [], ash: [] };
    if (world.caves !== cavesSeen) heights();
    target();
    cur.set(tgt);
    ramp = false;
    mesh();
  };
  sync();

  function calve(t, size) {
    const tip = t.reach;
    let n = Math.min(CHUNKS, 2 + Math.ceil(size / 2));
    for (let i = 0; i < CHUNKS && n; i++) if (ca[i] >= LIFE) {
      n--;
      const x = sx + t.ux * tip + (rng() - 0.5) * 3, z = sz + t.uz * tip + (rng() - 0.5) * 3;
      cp[i * 3] = x; cp[i * 3 + 1] = world.heightAt(x, z) + 1 + rng(); cp[i * 3 + 2] = z;
      cv[i * 3] = t.ux * (2 + rng() * 2); cv[i * 3 + 1] = 1 + rng() * 2; cv[i * 3 + 2] = t.uz * (2 + rng() * 2);
      cs[i] = 0.8 + rng(); ca[i] = 0;
    }
    world.emit('ice.calve', { x: sx + t.ux * tip, z: sz + t.uz * tip, size });
  }

  return {
    root,
    params: {},
    pose() {},
    step(dt) {
      if (world.state !== seen) sync();
      else if (world.snowline !== sl) target();
      if ((meltT += dt) >= MELT_S) { // cosmetic melt: re-read temp, emit where it is hot under the ice
        meltT = 0;
        let vol = 0, hot = 0, hk = 0;
        const temp = world.fields.temp;
        for (let k = 0; k < NN; k++) if (cur[k] > 0) {
          const h = Math.min(1, Math.max(0, temp[world.cell(nx(k), nz(k))]));
          if (h > 0.2) vol += h * cur[k];
          if (h > hot) { hot = h; hk = k; }
        }
        if (vol > 0) {
          vol = Math.round(vol * 100) / 100;
          world.emit('melt', { x: nx(hk), z: nz(hk), vol });
          if (hot > 0.6) world.emit('steam', { x: nx(hk), z: nz(hk), vol });
        }
        target();
      }
      if (ramp) {
        let left = false;
        const k1 = Math.min(1, dt / 1.5);
        for (let k = 0; k < NN; k++) {
          const d = tgt[k] - cur[k];
          if (Math.abs(d) < 0.01) cur[k] = tgt[k];
          else { cur[k] += d * k1; left = true; }
        }
        ramp = left;
        mesh();
      }
      let m = 0;
      for (let i = 0; i < CHUNKS; i++) {
        if (ca[i] >= LIFE) continue;
        const a = (ca[i] += dt), p = i * 3, ground = world.heightAt(cp[p], cp[p + 2]) + cs[i] * 0.4;
        cv[p + 1] -= 9.8 * dt;
        cp[p] += cv[p] * dt; cp[p + 1] += cv[p + 1] * dt; cp[p + 2] += cv[p + 2] * dt;
        if (cp[p + 1] < ground) { cp[p + 1] = ground; cv[p + 1] = 0; cv[p] *= 0.9; cv[p + 2] *= 0.9; } // slides to a stop
        if (a >= LIFE) continue;
        O.position.set(cp[p], cp[p + 1], cp[p + 2]);
        O.rotation.set(i + a * 0.7, i * 2.1, a * 0.5);
        O.scale.set(cs[i], cs[i] * 0.7, cs[i] * 1.2).multiplyScalar(Math.min(1, (LIFE - a) / 1.5));
        O.updateMatrix();
        chunks.setMatrixAt(m++, O.matrix);
      }
      if (m || chunks.count) { chunks.count = m; chunks.instanceMatrix.needsUpdate = true; }
    },
    on: {
      'world:glacier': (p) => {
        if (world.state !== seen) return sync(); // first frame after a new state object: history, no calving
        const old = g;
        g = p;
        target();
        for (const t of old.tongues || []) if (t.len >= 1 && !(p.tongues || []).some((x) => x.app === t.app)) {
          const a = axis(t.app);
          calve({ ...a, reach: a.r0 + t.len * TONGUE_K }, t.len);
        }
      },
      'cauldron.sag': (p) => { sag = p; target(); },
    },
    busy: () => ramp || chunks.count > 0,
    dispose() { kit.dispose(root); },
  };
}

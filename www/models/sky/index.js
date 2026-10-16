// sky-full (03-model-sky, 09 puff clusters): the cumulus LAYER around and below the summit + the real cloud MASS
// over it, both from net rx (metrics.rx, 0..1, log-normalised by the relay) plus visitor clouds (world:cloud, R3:
// they feed the real floor). Steam (glacier, cauldron) rises as short-lived puffs. Three InstancedMeshes of flat-based
// lumps on kit lit('cloud'), colour per lump on ramp('cloud') by its height in the cluster (board.html shapes).
// Writes world.clouds [{x, z, y (base), r, mass}] (mass first: lightning's bolt source; then layer, steam, visitors),
// world.sun + world.moon (js/sun.js, Paris, real clock), world.rig = {elev, rising} (a new object only when the sun moved enough:
// the host re-lerps the whole kit on each), world.fields.moisture (steam + melt splats, decaying). The dome + stars are
// the kit rig's (uStars per key via setPhase); sky only eases kit.setStale toward world.stale. Never creates lights.
import { cellXZ, GRID, CELL } from '../../js/world.js';
import { sun, moon, now } from '../../js/sun.js';

const TAU_RX = 4, TAU_STALE = 1, BASE = 13; // mass base 13 m over the summit: above the St Elmo corona
const SUN_S = 1, RIG_DEG = 0.2; // sun recomputed every second; a new rig only past 0.2° (keyMix blends over 8–9°)
const VISIT = 0.1; // each live visitor cloud adds this much to the floor's rx
const SLOTS = 4, PUFF = 8, LIFE = 24, RISE = 1.2, GAP = 2; // steam: ≤ 4 puffs × 8 lumps (budget: 50k tris), one per 2 s
const TAU_MOIST = 120, SPLAT = 3; // moisture: e-folding 2 min, splat radius in cells

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
  const STEAM = cluster(kit.mulberry(950), PUFF, [7, 7], 5, 2.2, 4, 0.6, 1); // one shape, mirrored per slot
  const mesh = (sets, name) => {
    const n = sets.reduce((a, c) => a + c.lumps.length / 5, 0), im = new THREE.InstancedMesh(lump, mat, n);
    im.name = name;
    let i = 0;
    for (const c of sets) for (let j = 0; j < c.lumps.length; j += 5) kit.setRampAt(im, i++, 0.35 + 0.65 * c.lumps[j + 4]);
    return im;
  };
  const layer = mesh(LAYER, 'layer'), mass = mesh(MASS, 'mass'), n0 = MASS[0].lumps.length / 5;
  const steam = mesh(Array.from({ length: SLOTS }, () => ({ lumps: STEAM })), 'steam');
  const root = new THREE.Group();
  layer.frustumCulled = mass.frustumCulled = steam.frustumCulled = false; // instances move; the sky is always in frame anyway
  steam.count = 0;
  root.add(layer, mass, steam);

  // world.clouds entries, mutated in place (step is allocation-free)
  const C = [...MASS, ...LAYER].map((c) => ({ x: c.x ?? 0, z: c.z ?? 0, y: c.y ?? 0, r: c.r, mass: !c.y }));
  const S = Array.from({ length: SLOTS }, (_, i) => ({ age: LIFE, x: 0, z: 0, y0: 0, k: 1, flip: i & 1 ? -1 : 1, c: { x: 0, z: 0, y: 0, r: 6, mass: false } }));
  const visitors = new Map(); // id → world.clouds entry; a hello resync reseeds them from world.state.clouds (world.caves identity, as the glacier)
  const seed = () => { visitors.clear(); for (const c of world.state.clouds || []) visitors.set(c.id, { r: 8, mass: false, x: c.pos[0], y: c.pos[1], z: c.pos[2] }); };
  const vis = () => world.metrics?.visitors !== false; // the kill switch hides visitor clouds from the real floor too (06)
  let cavesSeen = world.caves, visSeen = vis();
  seed();
  const clouds = [];
  const fill = (out) => {
    out.length = 0;
    for (let i = 0; i < C.length; i++) if (i !== 1 || mass.count > n0) out.push(C[i]);
    for (const s of S) if (s.age < LIFE) out.push(s.c);
    if (visSeen) for (const v of visitors.values()) out.push(v);
  };

  const O = new THREE.Object3D();
  const target = () => Math.min(1, (world.metrics?.rx ?? 0) + (visSeen ? VISIT * visitors.size : 0));
  let m = target(), drawn = -1, hDrawn = NaN, sx = 0, sz = 0, st = world.stale != null ? 1 : 0, sunT = 0, since = GAP, live = 0;
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
  // steam puffs, packed into the first instances (a dead slot draws nothing): rise, drift on the wind, swell then shrink
  function puffs(dt) {
    let i = 0, n = 0;
    for (const s of S) {
      if (s.age >= LIFE) continue;
      s.age += dt;
      if (s.age >= LIFE) continue;
      n++;
      s.x += world.wind.x * 4 * dt; s.z += world.wind.z * 4 * dt;
      const y = s.y0 + RISE * s.age, g = s.k * (0.6 + 0.4 * Math.min(1, s.age / 4, (LIFE - s.age) / 4));
      for (let j = 0; j < STEAM.length; j += 5) put(steam, i++, s.x + STEAM[j] * s.flip * g, y + STEAM[j + 1] * g, s.z + STEAM[j + 2] * g, STEAM[j + 3] * g, 0.7);
      s.c.x = s.x; s.c.z = s.z; s.c.y = y; s.c.r = 6 * g;
    }
    steam.count = i;
    steam.instanceMatrix.needsUpdate = true;
    if (n !== live) { live = n; world.write('clouds', fill); }
  }
  function splat(x, z, vol) {
    const M = world.fields.moisture, ci = Math.round((x + 100) / CELL), cj = Math.round((z + 100) / CELL);
    for (let j = Math.max(0, cj - SPLAT); j <= Math.min(GRID - 1, cj + SPLAT); j++) for (let i = Math.max(0, ci - SPLAT); i <= Math.min(GRID - 1, ci + SPLAT); i++) {
      const d2 = (i - ci) ** 2 + (j - cj) ** 2, k = j * GRID + i;
      if (d2 <= SPLAT * SPLAT) M[k] = Math.min(1, M[k] + 0.25 * vol * Math.exp(-d2 / SPLAT));
    }
  }
  function sky() { // the real sun + moon (Paris, wall clock): world.sun/moon every call, world.rig only when it matters
    const t = now(), s = sun(t), r = world.rig; // ponytail: sun()/moon() allocate once a second, not per frame
    world.write('sun', s);
    world.write('moon', moon(t));
    if (Math.abs(s.elev - r.elev) >= RIG_DEG || s.rising !== r.rising) world.write('rig', { elev: s.elev, rising: s.rising });
  }
  world.write('clouds', clouds);
  kit.setStale(st);
  sky();
  place();

  return {
    root,
    params: { tauRx: TAU_RX, tauStale: TAU_STALE, base: BASE, visit: VISIT },
    pose() {}, // sky is not on the scroll story: its state is the live metrics and the clock
    step(dt) {
      if (world.caves !== cavesSeen || vis() !== visSeen) { // a hello or replay (state.clouds: the toy clouds, W4) or the kill switch
        if (world.caves !== cavesSeen) seed();
        cavesSeen = world.caves; visSeen = vis();
        world.write('clouds', fill);
      }
      const rx = target();
      m = Math.abs(rx - m) < 0.002 ? rx : m + (rx - m) * Math.min(1, dt / TAU_RX);
      st += ((world.stale != null ? 1 : 0) - st) * Math.min(1, dt / TAU_STALE);
      kit.setStale(st);
      if (Math.abs(m - drawn) > 0.001 || world.hmax !== hDrawn) place();
      since += dt;
      if (live) puffs(dt);
      if ((sunT += dt) >= SUN_S) { // once a second: the sun and the moisture decay (16k cells)
        const f = Math.exp(-sunT / TAU_MOIST);
        world.write('moisture', (M) => { for (let i = 0; i < M.length; i++) M[i] *= f; });
        sunT = 0;
        sky();
      }
    },
    on: {
      steam({ x, z, vol = 1 }) {
        world.write('moisture', () => splat(x, z, vol));
        if (since < GAP) return; // ponytail: one puff per 2 s, the rest only wets the air
        since = 0;
        const s = S.reduce((a, b) => (b.age > a.age ? b : a)); // oldest (or dead) slot
        Object.assign(s, { age: 0, x, z, y0: world.heightAt(x, z) + 8, k: Math.min(1.3, 0.7 + 0.3 * vol) });
        puffs(0);
      },
      melt({ x, z, vol = 1 }) { world.write('moisture', () => splat(x, z, 0.3 * vol)); },
      'world:cloud'({ id, pos, state }) { // ponytail: 'gone' ends a visitor cloud; W4 toy-clouds names the real states
        if (state === 'gone') visitors.delete(id);
        else visitors.set(id, Object.assign(visitors.get(id) || { r: 8, mass: false }, { x: pos[0], y: pos[1], z: pos[2] }));
        world.write('clouds', fill);
      },
    },
    busy: () => Math.abs(target() - m) > 0.002 || Math.abs((world.stale != null ? 1 : 0) - st) > 0.01 || live > 0,
    dispose() { kit.dispose(root); },
  };
}

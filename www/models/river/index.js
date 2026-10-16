// river (03-model-river): one CatmullRom ribbon on heightSolid, kit flow('water'). Its course starts under the snowline
// on the camera side of the summit and runs downhill (with momentum and an outward push, so a hollow does not trap it)
// to the world edge; it is laid out again on a hello (world.caves identity, as the glacier). Width and foam
// speed = metrics.river (Σ public traffic bytes, 0..1); the visual feeds melt, aquifer.seep and jokulhlaup add a surge
// that widens and quickens it, then decays. The model writes only uTime (speed = how fast it advances); width is the
// ribbon geometry, draped on the mountain's own triangles so it never sinks into the faceted body. pick(ray) → {kind
// 'river', d, traffic: metrics.river in %} where the ray passes within the half width of the draped centre line.
import U from '../../js/units.json' with { type: 'json' };
import { cellXZ, GRID } from '../../js/world.js';

// course: source azimuth AZ (left of the camera's +z, clear of the ember pool at (0, 66)), 1 m walk steps, ≤ MAXW;
// LIFT clears shimmer's low-tier terrain wobble (≤ 0.35 m)
const SEG = 160, ACROSS = 4, LIFT = 0.4, AZ = -0.55, EDGE = 97, MAXW = 260;
// half-width m, foam speed, surge decay s, metric ease s; SURGE caps stacked feeds (3 jökulhlaups ≠ a 36 m river)
const W0 = 0.7, WK = 2.6, V0 = 0.4, VK = 2.6, TAU = 10, EASE = 2, SURGE = 6;
const FEED = { melt: (p) => Math.min(0.8, 0.2 + (p.vol || 0) / 20), 'aquifer.seep': () => 0.35, jokulhlaup: (p) => p.vol || 5 };

export default async function build({ THREE, kit, world }) {
  // the mountain's render grid (models/mountain): its triangles (a, b, c), (c, b, d) per quad, so the drape matches
  const G = kit.QUALITY === 'low' ? U.GRID_LOW : U.GRID, CELL = U.EXTENT / U.GRID, S = (CELL * (U.GRID - 1)) / (G - 1), X0 = -U.EXTENT / 2;
  const ground = (x, z) => {
    const fx = Math.min(G - 1.001, Math.max(0, (x - X0) / S)), fz = Math.min(G - 1.001, Math.max(0, (z - X0) / S));
    const i = Math.floor(fx), j = Math.floor(fz), u = fx - i, v = fz - j, h = (p, q) => world.heightAt(X0 + p * S, X0 + q * S);
    const a = h(i, j), b = h(i, j + 1), c = h(i + 1, j);
    if (u + v <= 1) return a + (c - a) * u + (b - a) * v;
    const d = h(i + 1, j + 1);
    return d + (b - d) * (1 - u) + (c - d) * (1 - v);
  };

  const NV = (SEG + 1) * (ACROSS + 1);
  const pos = new Float32Array(NV * 3), uv = new Float32Array(NV * 2), idx = [];
  for (let r = 0; r < SEG; r++) for (let c = 0; c < ACROSS; c++) {
    const a = r * (ACROSS + 1) + c, b = a + ACROSS + 1;
    idx.push(a, a + 1, b, a + 1, b + 1, b); // +y facing
  }
  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  geo.setAttribute('uv', new THREE.BufferAttribute(uv, 2));
  geo.setIndex(idx);
  const mat = kit.flow('water'), ribbon = new THREE.Mesh(geo, mat);
  ribbon.name = 'ribbon';
  ribbon.frustumCulled = false;
  const root = new THREE.Group();
  root.add(ribbon);

  // centre line (x, z), side unit vector (sx, sz) and taper per row; drape() writes the vertices from them
  const cx = new Float32Array(SEG + 1), cy = new Float32Array(SEG + 1), cz = new Float32Array(SEG + 1), sx = new Float32Array(SEG + 1), sz = new Float32Array(SEG + 1), taper = new Float32Array(SEG + 1);
  let cavesSeen, hw = W0, surge = 0, drapeT = 0, drawn = -1;
  function course() {
    cavesSeen = world.caves;
    const k = world.fields.heightSolid.indexOf(world.hmax), [ox, oz] = cellXZ(k % GRID, Math.floor(k / GRID));
    let dx = Math.sin(AZ), dz = Math.cos(AZ), x = ox, z = oz, r = 0;
    while (r < 100 && world.heightAt(x, z) > world.snowline - 2) { r++; x = ox + dx * r; z = oz + dz * r; } // source: under the snowline
    const pts = [new THREE.Vector3(x, 0, z)];
    for (let n = 0; n < MAXW && Math.abs(x) < EDGE && Math.abs(z) < EDGE; n++) {
      // downhill over a ±3 m stencil, pushed outward from the summit: the seeded bumps never trap it in a hollow
      const gx = world.heightAt(x + 3, z) - world.heightAt(x - 3, z), gz = world.heightAt(x, z + 3) - world.heightAt(x, z - 3), g = Math.hypot(gx, gz) || 1;
      const rx = x - ox, rz = z - oz, rr = Math.hypot(rx, rz) || 1, ex = -gx / g + 0.5 * rx / rr, ez = -gz / g + 0.5 * rz / rr, e = Math.hypot(ex, ez) || 1;
      dx = 0.6 * dx + 0.4 * ex / e; dz = 0.6 * dz + 0.4 * ez / e; // momentum carries it over small rises
      const l = Math.hypot(dx, dz) || 1;
      dx /= l; dz /= l; x += dx; z += dz;
      if (n % 3 === 2) pts.push(new THREE.Vector3(x, 0, z));
    }
    if (pts.length < 2) pts.push(new THREE.Vector3(x + dx, 0, z + dz)); // a source on the rim: a stub, never a crash
    const curve = new THREE.CatmullRomCurve3(pts), p = curve.getSpacedPoints(SEG), len = curve.getLength();
    for (let i = 0; i <= SEG; i++) {
      const t = i / SEG, d = curve.getTangentAt(t);
      cx[i] = p[i].x; cz[i] = p[i].z; sx[i] = -d.z; sz[i] = d.x;
      taper[i] = (0.35 + 0.65 * Math.min(1, t * 6)) * Math.min(1, (1 - t) * 10); // a spring at the source; thins out at the world edge
      for (let c = 0; c <= ACROSS; c++) uv.set([(t * len) / 32, c / ACROSS], (i * (ACROSS + 1) + c) * 2); // foam bands every 4 m
    }
    geo.attributes.uv.needsUpdate = true;
    drawn = -1;
  }
  function drape() {
    let n = 0;
    for (let i = 0; i <= SEG; i++) for (let c = 0; c <= ACROSS; c++) {
      const o = (2 * c / ACROSS - 1) * hw * taper[i], x = cx[i] + sx[i] * o, z = cz[i] + sz[i] * o;
      pos[n++] = x; pos[n++] = ground(x, z) + LIFT; pos[n++] = z;
      if (c * 2 === ACROSS) cy[i] = pos[n - 2]; // the centre line's height, for pick
    }
    geo.attributes.position.needsUpdate = true;
    drawn = hw; drapeT = 0;
  }
  course();
  const river = () => Math.min(1, Math.max(0, world.metrics?.river || 0));
  hw = W0 + WK * river();
  drape();

  const want = () => W0 + WK * river() + surge;
  const a = new THREE.Vector3(), b = new THREE.Vector3(), on = new THREE.Vector3(); // pick scratch
  return {
    root,
    params: {},
    pose() {},
    step(dt) {
      if (world.caves !== cavesSeen) course();
      // ponytail: not clipped (01), so while the cut is open the ribbon would hang in front of the face: hidden whole
      ribbon.visible = !(world.clip.depth > 0);
      surge = surge < 0.01 ? 0 : surge * Math.exp(-dt / TAU);
      const w = want();
      hw = Math.abs(w - hw) < 0.005 ? w : hw + (w - hw) * Math.min(1, dt / EASE);
      mat.uniforms.uTime.value = (mat.uniforms.uTime.value + dt * (V0 + VK * river() + 0.5 * surge)) % 200; // foam period 2: exact wrap
      // ponytail: the whole ribbon swells at once (no travelling flood front); re-drape every 1 s for deposits landing on it
      if (hw !== drawn || (drapeT += dt) >= 1) drape();
    },
    on: Object.fromEntries(Object.entries(FEED).map(([t, f]) => [t, (p) => { surge = Math.min(SURGE, surge + f(p)); }])),
    pick(ray) {
      if (!ribbon.visible) return null;
      let d = Infinity;
      for (let i = 0; i < SEG; i++) { // the drawn band as capsules: centre-line segment ± its half width
        const w = drawn * Math.max(taper[i], taper[i + 1]);
        if (ray.distanceSqToSegment(a.set(cx[i], cy[i], cz[i]), b.set(cx[i + 1], cy[i + 1], cz[i + 1]), on) < w * w) d = Math.min(d, on.distanceTo(ray.origin));
      }
      return d < Infinity ? { kind: 'river', d, traffic: Math.round(river() * 100) } : null;
    },
    busy: () => surge > 0 || hw !== want(),
    dispose() { kit.dispose(root); },
  };
}

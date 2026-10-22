// Walkers (R7): traffic made visible. Each app's walkers cross a bridge onto its island, follow the road to the
// square, then the shortest grass path (hex BFS around the other buildings) to its door, and go in.
// metrics.apps[app]: rpsB (log2 bucket 0..10) → how many walk at once; errB → how many of them are turned back at the
// door (they turn red and walk away); spike → twice the crowd. Pure in t: offsets spread them along the loop.
// R8: from the square they keep to the roads and the streets (buildings.js streets()), and cut across the grass only
// where no street reaches the door.
import { hexXZ, islandsOf, ring } from '../layout.js';
import { streets } from './buildings.js';

const SPEED = 1.1, PAUSE = 0.6; // world units / s, seconds at the door
const NEIGH = [[1, 0], [1, -1], [0, -1], [-1, 0], [-1, 1], [0, 1]];

export default async function build({ THREE, kit, world }) {
  const root = new THREE.Group();
  const low = kit.QUALITY === 'low', MAX = low ? 24 : 72;
  const body = kit.instanced(new THREE.CapsuleGeometry(0.1, 0.17, 1, low ? 5 : 7).translate(0, 0.19, 0), kit.palette.white, MAX);
  const head = kit.instanced(new THREE.IcosahedronGeometry(0.095, 1).translate(0, 0.43, 0), kit.palette.white, MAX);
  for (const m of [body, head]) { m.frustumCulled = false; root.add(m); }
  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3(1, 1, 1), C = new THREE.Color();
  const clothes = [0xefe9e1, 0x6f767b, 0x9c7f62, 0xc4904a, 0x4f6a8c, 0x6f8a55].map((c) => new THREE.Color(c));
  const skin = [0xe8c9a8, 0xc69c78, 0x8d6748].map((c) => new THREE.Color(c));
  const red = new THREE.Color(0xa5402a);

  let routes = [], dirty = true;
  world.on('state', () => { dirty = true; });
  world.on('metrics', () => { dirty = true; });

  // BFS over walkable local hexes (not a plot, on the island) from the square to a neighbour of the target plot
  function path(is, plots, to, paved) {
    const tiles = new Map();
    for (let i = 0; i < is.tilesList.length; i++) {
      const k = `${is.tilesList[i][0]},${is.tilesList[i][1]}`;
      if (!paved || 'rsb'.includes(is.tiles[i]) || paved.has(k)) tiles.set(k, is.tiles[i]);
    }
    const blocked = new Set(plots.filter((p) => p.host === is.host).map((p) => `${p.q},${p.r}`));
    const goal = `${to.q},${to.r}`, prev = new Map([['0,0', null]]), queue = [[0, 0]];
    while (queue.length) {
      const [q, r] = queue.shift();
      for (const [dq, dr] of NEIGH) {
        const k = `${q + dq},${r + dr}`;
        if (k === goal) { const out = [[q, r]]; for (let c = `${q},${r}`; prev.get(c); c = prev.get(c)) out.unshift(prev.get(c).split(',').map(Number)); return out; }
        if (prev.has(k) || !tiles.has(k) || blocked.has(k)) continue;
        prev.set(k, `${q},${r}`);
        queue.push([q + dq, r + dr]);
      }
    }
    return paved ? path(is, plots, to) : [[0, 0]]; // no paved way to that door: across the grass
  }

  function relayout() {
    dirty = false;
    routes = [];
    const t = world.town, s = world.state, apps = world.metrics?.apps || {};
    if (!t) return;
    const list = [];
    for (const p of s.plots) {
      const m = apps[p.app];
      if (!m || p.ruinAt || !m.rpsB || world.ready[p.host] === false) continue;
      list.push({ p, n: Math.min(6, m.rpsB) * (m.spike ? 2 : 1), bad: m.errB ? Math.max(1, Math.round(Math.min(6, m.rpsB) * m.errB / Math.max(1, m.rpsB))) : 0 });
    }
    const total = list.reduce((a, x) => a + x.n, 0), k = total > MAX ? MAX / total : 1;
    let id = 0;
    const paved = new Map(islandsOf(t).map((is) => [is.host, new Set(streets(is, s.plots.filter((x) => x.host === is.host)).map(([q, r]) => `${q},${r}`))]));
    for (const { p, n, bad } of list) {
      const is = t.islands.find((i) => i.host === p.host);
      is.tilesList ||= [];
      const R = is.radius;
      if (!is.tilesList.length) for (let r = -R; r <= R; r++) for (let q = Math.max(-R, -r - R); q <= Math.min(R, -r + R); q++) is.tilesList.push([q, r]);
      const heads = t.bridges.flatMap((b) => (b.a === p.host ? [[b.aq, b.ar]] : b.b === p.host ? [[b.bq, b.br]] : []));
      const inner = path(is, s.plots, p, paved.get(p.host));
      const count = Math.max(1, Math.round(n * k));
      for (let i = 0; i < count; i++) {
        const [hq, hr] = heads[i % heads.length], len = ring(hq, hr);
        const spoke = []; // bridge head → square, along the road
        for (let j = len; j >= 1; j--) spoke.push([Math.round(hq * j / len), Math.round(hr * j / len)]);
        const hexes = [...spoke, ...inner];
        const pts = hexes.map(([q, r]) => hexXZ(is.q + q, is.r + r));
        const [dx, dz] = hexXZ(is.q + p.q, is.r + p.r), [lx, lz] = pts[pts.length - 1];
        pts.push([lx + (dx - lx) * 0.55, lz + (dz - lz) * 0.55]); // the door, at the building's edge
        // keep right: a small sideways offset per walker
        const side = ((i % 3) - 1) * 0.16;
        let L = 0; const seg = [];
        for (let j = 1; j < pts.length; j++) { const l = Math.hypot(pts[j][0] - pts[j - 1][0], pts[j][1] - pts[j - 1][1]); seg.push(l); L += l; }
        routes.push({ pts, seg, L, side, off: (i / count + (id * 0.37) % 1) % 1, turned: i < bad, cloth: clothes[(id * 7) % clothes.length], skin: skin[(id * 5) % skin.length] });
        id++;
      }
    }
  }

  function at(r, d) { // position + heading at distance d along route r
    for (let j = 0; j < r.seg.length; j++) {
      if (d <= r.seg[j] || j === r.seg.length - 1) {
        const f = Math.min(1, d / r.seg[j]), a = r.pts[j], b = r.pts[j + 1];
        const dx = b[0] - a[0], dz = b[1] - a[1], l = Math.hypot(dx, dz) || 1;
        return [a[0] + dx * f - (dz / l) * r.side, a[1] + dz * f + (dx / l) * r.side, Math.atan2(dx, dz)];
      }
      d -= r.seg[j];
    }
    return [0, 0, 0];
  }

  return {
    root,
    update(t) {
      if (dirty) relayout();
      let n = 0;
      for (const r of routes) {
        if (n >= MAX) break;
        const cycle = r.L / SPEED * (r.turned ? 2 : 1) + PAUSE, u = ((t / cycle + r.off) % 1) * cycle;
        let d, back = false;
        const go = r.L / SPEED;
        if (u < go) d = u * SPEED;
        else if (!r.turned) continue; // inside: the next loop starts on the bridge
        else if (u < go + PAUSE) d = r.L; // refused at the door
        else { d = r.L - (u - go - PAUSE) * SPEED; back = true; }
        const [x, z, yaw] = at(r, Math.max(0, d));
        const bob = Math.abs(Math.sin(t * 9 + r.off * 20)) * 0.04;
        M.compose(P.set(x, bob, z), Q.setFromEuler(E.set(0, yaw + (back ? Math.PI : 0), 0)), S);
        body.setMatrixAt(n, M); head.setMatrixAt(n, M);
        body.setColorAt(n, r.turned && (back || u >= go) ? red : C.copy(r.cloth));
        head.setColorAt(n, r.skin);
        n++;
      }
      body.count = head.count = n;
      for (const m of [body, head]) { m.instanceMatrix.needsUpdate = true; if (m.instanceColor) m.instanceColor.needsUpdate = true; }
      return n > 0;
    },
  };
}

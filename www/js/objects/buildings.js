// Buildings (R7): one per world.state.plots entry, on its host's island, facing the square. One InstancedMesh pair
// (body in clay, roof in the app's colour) per building kind keeps the draw count flat however many apps there are.
// The steady look comes from world.state alone, so a reload and a replay show the same town:
//   health Progressing or a build in flight → scaffold;  Degraded → chimney smoke;  Missing / Suspended → faded
//   plot.ruinAt (app.gone) → ruins;  the host's nodes not Ready → greyed with its island;  alerts → beacon fires
// Transient acts (from live frames, never replayed state): a new or moved plot and a successful sync drop the
// building in (crane drop + squash, pi-lab office.js); pod.churn makes it hop (pi-lab main.js hopPose), and a
// crashloop keeps it hopping until the next healthy sync.
// R8 typology: the family of a building's kind (layout.js FAMILY) sets its ground: a flat hex cap of stone (Defense),
// paving (Public) or packed earth (Industry); Shops and Houses stand on the grass. Streets: from each door that does
// not already open on a road, the shortest walk over free grass to a spoke or the square (a BFS, recomputed with the
// plots), drawn as tan caps. They reserve nothing: a plot or a plant may take a street hex, and the street reroutes.
// Industry and Houses cast no shadow (the low, busy kinds; it keeps the shadow pass small).
// R8 C annexes (world.state.annexes): an app's pods on another island show as a full copy of its own building there
// (every kind fits a tile), marked by a banner in the app's wire colour; the main building keeps its ground cap and
// lamp. A copy has no lamp, scaffold, smoke, flag or ground, greys with its island, and drops in when it goes up live,
// never with its app's syncs. Tapping one opens the app's card. A copy beside no standing building of its app, or on
// its app's own island, is not drawn (the relay removes it on its next poll). Copies block streets and walkers.
// Zoom levels: in the overview a copy stands as its banner alone ("this app also lives here"); once the camera comes
// in, the island it looks at (the one nearest the orbit target) shows its copies whole. It keeps every view in budget.
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { BUILDINGS, familyOf, islandsOf, plotXZ, wireOf } from '../layout.js';

const GROUND = { defense: 0xaaa69f, public: 0xece2cf, industry: 0xb49272 }, STREET = 0xdcc8a2;
const DIRS = [[1, 0], [1, -1], [0, -1], [-1, 0], [-1, 1], [0, 1]];

/** The streets of an island: local hexes from each door to the road (see above). plots: [{q, r}] on that island;
 *  blocked: more taken hexes with no door of their own (R8 C tents). */
export function streets(is, plots, blocked = []) {
  const key = (q, r) => `${q},${r}`, taken = new Set(plots.concat(blocked).map((p) => key(p.q, p.r))), out = new Map();
  const free = (q, r) => is.tileAt(q, r)?.kind === 'g' && !taken.has(key(q, r));
  const onRoad = (q, r) => DIRS.some(([dq, dr]) => 'rsb'.includes(is.tileAt(q + dq, r + dr)?.kind || 'x'));
  for (const p of [...plots].sort((a, b) => a.q - b.q || a.r - b.r)) {
    if (onRoad(p.q, p.r)) continue;
    const from = new Map([[key(p.q, p.r), null]]), queue = [[p.q, p.r]];
    let end = null;
    while (queue.length && !end) {
      const [q, r] = queue.shift();
      for (const [dq, dr] of DIRS) {
        const n = [q + dq, r + dr], k = key(...n);
        if (from.has(k) || !free(...n)) continue;
        from.set(k, [q, r]);
        if (onRoad(...n)) { end = n; break; }
        queue.push(n);
      }
    }
    for (let h = end; h && from.get(key(...h)); h = from.get(key(...h))) out.set(key(...h), h);
  }
  return [...out.values()];
}

const DROP = 0.9, HOP = 0.75;

export default async function build({ THREE, kit, world, apps }) {
  const root = new THREE.Group();
  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3();
  const C = new THREE.Color();

  // ---- geometry per kind: body (clay atlas, roof cells cut out, props merged in) + roof (white, tinted per instance
  // in its Ukubi wire colour; at night the roofs light up in the Glow theme's wire colours)
  const kinds = {}, roofMat = kit.glowing(), CAP = 24; // instances per kind, annex copies included (≤ 64 plots + 48 copies, never 24 of one kind)
  async function kindOf(role) {
    if (kinds[role]) return kinds[role];
    try { return await loadKind(role); } catch (e) {
      // a model that failed to load (a network blip; a 404 = this page's build was retired) stands in as a home,
      // so one GLB never freezes the town; the 404 also asks main.js for the one reload onto the current build
      if (/\b404\b/.test(String(e))) globalThis.dispatchEvent?.(new Event('town:retired'));
      if (role === 'home') throw e;
      console.warn(`buildings: ${role} failed to load, drawn as a home`, e);
      return (kinds[role] = await kindOf('home'));
    }
  }
  async function loadKind(role) {
    const spec = BUILDINGS[role] || BUILDINGS.home;
    const [body, roof] = kit.splitByUV(mergeGeometries(await kit.geometry(spec.glb)), kit.inRoof);
    const bodies = [body.scale(spec.s, spec.s, spec.s)];
    for (const [name, x, z, ry, s] of spec.props || []) {
      const g = mergeGeometries(await kit.geometry(name));
      bodies.push(g.applyMatrix4(new THREE.Matrix4().compose(new THREE.Vector3(x * spec.s / 1.4, 0, z * spec.s / 1.4),
        Q.setFromEuler(E.set(0, ry, 0)), S.set(s, s, s))));
    }
    const bg = mergeGeometries(bodies);
    bg.computeBoundingBox();
    const k = {
      body: kit.instanced(bg, kit.shared, CAP),
      roof: kit.instanced(roof.scale(spec.s, spec.s, spec.s), roofMat, CAP),
      family: familyOf(role),
      height: bg.boundingBox.max.y, width: Math.max(bg.boundingBox.max.x - bg.boundingBox.min.x, bg.boundingBox.max.z - bg.boundingBox.min.z),
      apps: [],
    };
    // R8 C: the annex copies of this kind, on their own pair of meshes that cast no shadow (a copy repeats a building,
    // the shadow pass would pay for it twice); apps and items per copy instance
    k.copy = { body: kit.instanced(k.body.geometry, kit.shared, CAP), roof: kit.instanced(k.roof.geometry, roofMat, CAP), apps: [], items: [] };
    for (const m of [k.copy.body, k.copy.roof]) { m.userData.pick = { kind: 'building', of: k.copy }; m.frustumCulled = false; m.castShadow = false; root.add(m); }
    for (const m of [k.body, k.roof]) {
      m.userData.pick = { kind: 'building', of: k }; m.frustumCulled = false; root.add(m);
      if (k.family === 'industry' || k.family === 'houses') m.castShadow = false;
    }
    kinds[role] = k;
    return k;
  }

  // ---- overlays, all instanced
  const box = (w, h, d, x, y, z) => new THREE.BoxGeometry(w, h, d).translate(x, y, z);
  const scaffoldGeo = (() => {
    const p = [], W = 0.85, H = 1.5;
    for (const sx of [-1, 1]) for (const sz of [-1, 1]) p.push(box(0.06, H, 0.06, sx * W, H / 2, sz * W));
    for (const y of [0.5, 1.0, 1.45]) {
      p.push(box(2 * W + 0.06, 0.05, 0.12, 0, y, W), box(2 * W + 0.06, 0.05, 0.12, 0, y, -W));
      p.push(box(0.12, 0.05, 2 * W, W, y, 0), box(0.12, 0.05, 2 * W, -W, y, 0));
    }
    p.push(box(0.05, 1.3, 0.05, W, 0.75, 0).rotateZ(0.0)); // a ladder rail
    return mergeGeometries(p);
  })();
  function atlasCell(g, [c, r]) { // a procedural piece painted with one clay cell
    const uv = g.attributes.uv;
    for (let i = 0; i < uv.count; i++) uv.setXY(i, (c + 0.5) / 8, (r + 0.5) / 4);
    return g;
  }
  const rubbleGeo = mergeGeometries([
    ...(await kit.geometry('rock_single_C')).map((g) => g.scale(2.4, 1.6, 2.4).translate(-0.35, 0, 0.2)),
    ...(await kit.geometry('rock_single_A')).map((g) => g.scale(3, 2.4, 3).translate(0.4, 0, -0.25)),
    ...(await kit.geometry('rock_single_C')).map((g) => g.scale(1.8, 1.2, 1.8).translate(0.25, 0, 0.45)),
    ...(await kit.geometry('crate_A_small')).map((g) => g.scale(2.4, 2.4, 2.4).rotateY(0.6).translate(-0.5, 0, -0.45)),
    atlasCell(box(0.9, 0.35, 0.12, -0.1, 0.17, -0.6).rotateY(0.3).toNonIndexed(), kit.CELL.stone), // a broken wall stub
  ]);
  const N = 64;
  const scaffold = kit.instanced(scaffoldGeo, kit.palette.wood, N);
  const rubble = kit.instanced(rubbleGeo, kit.shared, N);
  const ruins = { apps: [] }; // ruins are tappable too: their card says the app left the cluster
  rubble.userData.pick = { kind: 'building', of: ruins };
  const smoke = kit.instanced(new THREE.IcosahedronGeometry(0.3, 1), kit.palette.smoke, N * 4, false);
  // a beacon: a stone post with a fire basket on top, tall enough to read from the overview
  const brazier = kit.instanced(mergeGeometries([new THREE.CylinderGeometry(0.1, 0.16, 1.5, 6).translate(0, 0.75, 0), new THREE.CylinderGeometry(0.34, 0.2, 0.22, 6).translate(0, 1.6, 0)]), kit.palette.stone, N);
  const flameMat = { info: kit.variant('fire', { color: new THREE.Color(0xffe08a), emissive: new THREE.Color(0xffd060) }), warning: kit.variant('fire'), critical: kit.variant('fire', { color: new THREE.Color(0xff4a2a), emissive: new THREE.Color(0xff2a10), emissiveIntensity: 2.2 }) };
  const flameGeo = new THREE.ConeGeometry(0.3, 1.0, 7).translate(0, 0.5, 0); // scaled at its base, lifted onto the basket
  const flames = Object.fromEntries(Object.entries(flameMat).map(([sev, m]) => [sev, kit.instanced(flameGeo, m, N, false)]));
  // a door lamp per building, lit at night (the town's windows); a red flag after a failed sync (10 min)
  const lampMat = kit.variant('window');
  const lampPost = kit.instanced(mergeGeometries([new THREE.BoxGeometry(0.07, 0.75, 0.07).translate(0, 0.375, 0), new THREE.BoxGeometry(0.26, 0.05, 0.05).translate(0.1, 0.72, 0)]), kit.palette.wood, N);
  const lamp = kit.instanced(new THREE.BoxGeometry(0.16, 0.2, 0.16).translate(0.2, 0.58, 0), lampMat, N, false);
  const [fpole, fcloth] = kit.splitByUV(mergeGeometries(await kit.geometry('flag_blue')), kit.inRoof);
  const failPole = kit.instanced(fpole.scale(3, 3, 3), kit.shared, N);
  const failCloth = kit.instanced(fcloth.scale(3, 3, 3), kit.variant('raspberry', { color: new THREE.Color(0xa5402a) }), N); // Ukubi danger
  const failed = new Map(); // app → until (t, s)
  // R8 C: an annex copy's banner, the same flag in the app's wire colour (the roofs' glowing material, tinted per instance)
  const bannerPole = kit.instanced(fpole.scale(3, 3, 3), kit.shared, N);
  const bannerCloth = kit.instanced(fcloth.scale(3, 3, 3), roofMat, N);
  // ground caps and streets: one instanced flat hex (pointy-top like the tiles), tinted per instance
  const cap = kit.instanced(new THREE.CylinderGeometry(0.98, 0.98, 0.05, 6).translate(0, 0.025, 0), kit.palette.white, 256);
  cap.castShadow = false;
  const isles = islandsOf(world.town);
  let ground = [];
  for (const m of [scaffold, rubble, smoke, brazier, lampPost, lamp, failPole, failCloth, bannerPole, bannerCloth, cap, ...Object.values(flames)]) { m.frustumCulled = false; root.add(m); }

  // ---- state → layout
  let plots = [], tents = [], beacons = [], dirty = true, relaying = null, retryAt = 0;
  const acts = new Map(); // app → {type, t0, until?}
  world.on('state', () => { dirty = true; });
  world.on('metrics', () => { dirty = true; }); // hosts' Ready rides the metrics frame
  world.on('world:plot', (p) => { if (!p.gone && !p.ruinAt) acts.set(p.app, { type: 'drop', t0: now }); });
  world.on('world:annex', (x) => { if (!x.gone) acts.set(`${x.app}@${x.host}`, { type: 'drop', t0: now }); }); // a copy's own act
  world.on('ev:argo.sync', (f) => {
    if (f.ok) { acts.set(f.app, { type: 'drop', t0: now }); failed.delete(f.app); } else failed.set(f.app, now + 600);
  });
  world.on('ev:pod.churn', (f) => acts.set(f.app, { type: 'hop', t0: now, loop: f.crashloop }));
  world.on('ev:argo.health', (f) => { if (f.status === 'Healthy' && acts.get(f.app)?.loop) acts.delete(f.app); });
  let now = 0;

  async function relayout() {
    dirty = false;
    const s = world.state, list = [];
    for (const p of s.plots) {
      const k = await kindOf(p.role);
      const [x, z] = plotXZ(world.town, p.host, p.q, p.r);
      const is = world.town.islands.find((i) => i.host === p.host), [cx, cz] = plotXZ(world.town, p.host, 0, 0);
      const yaw = Math.atan2(cx - x, cz - z);
      const a = apps[p.app] || {}, health = s.health[p.app], down = world.ready[p.host] === false;
      // R8: off = a probed machine not answering (its own state, 10 min held): greyed and dark, but not sunk: its island is up
      list.push({ ...p, k, x, z, yaw, is, wire: wireOf(apps, p.app), down, health, off: s.machines?.[p.app] === 'down',
        building: !!s.builds[p.app] || health === 'Progressing', smoke: health === 'Degraded', faded: health === 'Missing' || health === 'Suspended' });
    }
    plots = list;
    // R8 C annexes: a copy of the app's building (its kind), only beside a standing one of it on another island
    const standing = new Map(list.filter((p) => !p.ruinAt).map((p) => [p.app, p]));
    tents = [];
    for (const x of s.annexes || []) {
      const main = standing.get(x.app);
      if (!main || main.host === x.host) continue;
      const [tx, tz] = plotXZ(world.town, x.host, x.q, x.r), [cx, cz] = plotXZ(world.town, x.host, 0, 0);
      tents.push({ ...x, key: `${x.app}@${x.host}`, k: main.k, x: tx, z: tz, yaw: Math.atan2(cx - tx, cz - tz), wire: main.wire, down: world.ready[x.host] === false });
    }
    ground = [];
    for (const is of isles) {
      const here = list.filter((p) => p.host === is.host);
      for (const p of here) if (!p.ruinAt && GROUND[p.k.family]) ground.push({ host: p.host, q: p.q, r: p.r, color: GROUND[p.k.family] });
      for (const [q, r] of streets(is, here, tents.filter((x) => x.host === is.host))) ground.push({ host: is.host, q, r, color: STREET });
    }
    // beacons: at the alert's app, else its host's square, else the watchtower, else h1's square
    const byApp = new Map(list.map((p) => [p.app, p])), tower = list.find((p) => p.role === 'watchtower');
    const at = new Map();
    beacons = s.alerts.map((al) => {
      let x, z, y = 0;
      const p = al.app && byApp.get(al.app);
      if (p) { x = p.x + Math.cos(p.yaw) * 1.05; z = p.z - Math.sin(p.yaw) * 1.05; }
      else if (al.host || !tower) { [x, z] = plotXZ(world.town, al.host || 'h1', 0, 0); x += 1.25; z += 0.4; }
      else { x = tower.x + Math.cos(tower.yaw) * 0.95; z = tower.z - Math.sin(tower.yaw) * 0.95; }
      const key = `${x.toFixed(2)},${z.toFixed(2)}`, n = at.get(key) || 0;
      at.set(key, n + 1);
      const r = 0.45 * Math.sqrt(n), th = n * 2.39996; // several on one spot spread on a sunflower spiral, not a line
      return { sev: al.sev, x: x + r * Math.cos(th), y, z: z + r * Math.sin(th), seed: parseInt(al.fp.slice(0, 4), 16) };
    }).slice(0, N); // ≤ N fires drawn (the LED counts every alert)
  }

  function pose(p, t) {
    // → [y offset, squash (y scale), spin] from the plot's live act (an annex copy's: keyed app@host)
    const id = p.key || p.app, a = acts.get(id);
    if (!a) return [0, 1, 0];
    const s = t - a.t0;
    if (a.type === 'drop') {
      if (s >= DROP + 0.5) { acts.delete(id); return [0, 1, 0]; }
      if (s < DROP * 0.6) { const f = s / (DROP * 0.6); return [3.2 * (1 - f * f), 1 + 0.25 * (1 - f), 0]; }
      const v = (s - DROP * 0.6) / (DROP * 0.4 + 0.5), d = Math.exp(-6 * v);
      return [0, 1 - 0.28 * d * Math.cos(v * 22), 0];
    }
    const cycle = a.loop ? (s % 1.6) : s;
    if (!a.loop && s >= HOP + 0.6) { acts.delete(id); return [0, 1, 0]; }
    if (cycle < HOP) { const f = cycle / HOP; return [1.1 * 4 * f * (1 - f), 1 + 0.15 * Math.abs(Math.cos(Math.PI * f)), Math.PI * 2 * kit.easeInOut(f)]; }
    const v = cycle - HOP, d = Math.exp(-6 * v);
    return [0, 1 - 0.25 * d * Math.cos(v * 24), 0];
  }

  const grey = new THREE.Color(0.42, 0.44, 0.5), SINK = 0.5; // islands.js
  return {
    root,
    /** → true while something moves (the host keeps rendering). */
    /** focus: the orbit target, whose nearest island shows its annex copies whole (null = the overview: banners only). */
    update(dt, t, night = 0, focus = null) {
      now = t;
      roofMat.emissiveIntensity = 0.75 * night;
      lampMat.emissiveIntensity = 0.1 + 2.4 * night;
      const wires = kit.ukubi.wires[night > 0.5 ? 'glow' : 'office'];
      if (dirty && !relaying && t >= retryAt) relaying = relayout() // a new kind's GLB loads async
        .catch((e) => { console.error('buildings:', e); dirty = true; retryAt = t + 5; }) // keep the last town, retry
        .finally(() => { relaying = null; });
      for (const k of new Set(Object.values(kinds))) { k.body.count = k.roof.count = k.copy.body.count = k.copy.roof.count = 0; k.apps = []; k.copy.apps = []; k.copy.items = []; }
      ruins.apps = [];
      let nS = 0, nR = 0, nSm = 0, nL = 0, nF = 0, busy = false;
      const counts = { info: 0, warning: 0, critical: 0 };
      for (const p of plots) {
        if (p.ruinAt) {
          if (nR >= N) continue;
          ruins.apps[nR] = p.app;
          M.compose(P.set(p.x, p.down ? -SINK : 0, p.z), Q.setFromEuler(E.set(0, p.yaw, 0)), S.set(1, 1, 1));
          rubble.setMatrixAt(nR, M);
          rubble.setColorAt(nR++, p.down ? grey : C.set(0xd8d2c8));
          continue;
        }
        const [dy, sq, spin] = pose(p, t);
        busy ||= acts.has(p.app);
        const k = p.k;
        if (k.body.count >= CAP) continue; // more than CAP of one kind: never with the allowlist (≤ 64 plots)
        const i = k.body.count++;
        k.roof.count++;
        M.compose(P.set(p.x, dy - (p.down ? SINK : 0), p.z), Q.setFromEuler(E.set(0, p.yaw + spin, 0)), S.set(1 / Math.sqrt(sq), sq, 1 / Math.sqrt(sq)));
        k.body.setMatrixAt(i, M);
        k.roof.setMatrixAt(i, M);
        k.body.setColorAt(i, p.down || p.off ? grey : C.setScalar(p.faded ? 0.72 : 1));
        k.roof.setColorAt(i, p.down || p.off ? grey : C.set(wires[p.wire]).multiplyScalar(p.faded ? 0.6 : 1));
        k.apps[i] = p.app;
        const fx = (lx, lz) => [p.x + Math.cos(p.yaw) * lx + Math.sin(p.yaw) * lz, p.z - Math.sin(p.yaw) * lx + Math.cos(p.yaw) * lz];
        if (nL < N && !p.off) { const [x, z] = fx(0.5 * k.width, 0.5 * k.width); M.compose(P.set(x, p.down ? -SINK : 0, z), Q.setFromEuler(E.set(0, p.yaw, 0)), S.set(1, 1, 1)); lampPost.setMatrixAt(nL, M); lamp.setMatrixAt(nL++, M); }
        if ((failed.get(p.app) ?? -1) > t && nF < N) { const [x, z] = fx(-0.5 * k.width, 0.45 * k.width); M.compose(P.set(x, 0, z), Q.setFromEuler(E.set(0, p.yaw, 0)), S.set(1, 1, 1)); failPole.setMatrixAt(nF, M); failCloth.setMatrixAt(nF++, M); }
        if (p.building && nS < N) {
          M.compose(P.set(p.x, 0, p.z), Q.setFromEuler(E.set(0, p.yaw, 0)), S.set(k.width / 1.6, Math.max(0.8, k.height / 1.5), k.width / 1.6));
          scaffold.setMatrixAt(nS++, M);
        }
        if (p.smoke) {
          busy = true;
          for (let j = 0; j < 4 && nSm < smoke.instanceMatrix.count; j++) {
            const f = (t * 0.35 + j / 4) % 1, s = 0.6 + 1.6 * f;
            M.compose(P.set(p.x + 0.25 * Math.sin(j * 2.1 + t * 0.6) + 0.3 * f, k.height * 0.85 + 0.3 + f * 1.6, p.z + 0.2 * Math.cos(j * 1.7)), Q.identity(), S.set(s, s, s));
            smoke.setMatrixAt(nSm++, M);
          }
        }
      }
      let nB = 0, focusHost = null;
      if (focus) {
        let best = Infinity;
        for (const is of world.town.islands) {
          const [cx, cz] = plotXZ(world.town, is.host, 0, 0), d = Math.hypot(cx - focus.x, cz - focus.z);
          if (d < best) { best = d; focusHost = is.host; }
        }
      }
      for (const x of tents) { // R8 C: their own drop, their host's grey, a banner; nothing else
        const k = x.k.copy, width = x.k.width;
        const [dy, sq] = pose(x, t);
        busy ||= acts.has(x.key);
        const near = x.host === focusHost;
        if (near && k.body.count < CAP) {
          const i = k.body.count++;
          k.roof.count++;
          M.compose(P.set(x.x, dy - (x.down ? SINK : 0), x.z), Q.setFromEuler(E.set(0, x.yaw, 0)), S.set(1 / Math.sqrt(sq), sq, 1 / Math.sqrt(sq)));
          k.body.setMatrixAt(i, M);
          k.roof.setMatrixAt(i, M);
          k.body.setColorAt(i, x.down ? grey : C.setScalar(1));
          k.roof.setColorAt(i, x.down ? grey : C.set(wires[x.wire]));
          k.apps[i] = x.app;
          k.items[i] = x;
        }
        if (nB < N) { // the banner where a main building has its lamp: front right
          const bx = x.x + Math.cos(x.yaw) * 0.5 * width + Math.sin(x.yaw) * 0.5 * width, bz = x.z - Math.sin(x.yaw) * 0.5 * width + Math.cos(x.yaw) * 0.5 * width;
          M.compose(P.set(bx, dy - (x.down ? SINK : 0), bz), Q.setFromEuler(E.set(0, x.yaw, 0)), S.set(1, 1, 1));
          bannerPole.setMatrixAt(nB, M);
          bannerCloth.setMatrixAt(nB, M);
          bannerPole.setColorAt(nB, x.down ? grey : C.setScalar(1));
          bannerCloth.setColorAt(nB++, x.down ? grey : C.set(wires[x.wire]));
        }
      }
      bannerPole.count = bannerCloth.count = nB;
      for (const b of beacons) {
        busy = true;
        const n = counts[b.sev]++, fl = 1 + 0.18 * Math.sin(t * 9 + b.seed) + 0.1 * Math.sin(t * 23 + b.seed * 3);
        const big = b.sev === 'critical' ? 1.35 : b.sev === 'info' ? 0.75 : 1;
        M.compose(P.set(b.x, 1.68, b.z), Q.setFromEuler(E.set(0, t * 2 + b.seed, 0)), S.set(big, big * fl, big));
        flames[b.sev].setMatrixAt(n, M);
      }
      let nc = 0;
      for (const g of ground.slice(0, cap.instanceMatrix.count)) {
        const [x, z] = plotXZ(world.town, g.host, g.q, g.r), down = world.ready[g.host] === false;
        M.compose(P.set(x, down ? -SINK : 0, z), Q.identity(), S.set(1, 1, 1));
        cap.setMatrixAt(nc, M);
        cap.setColorAt(nc++, down ? grey : C.set(g.color));
      }
      cap.count = nc;
      let nb = 0;
      for (const b of beacons) { M.compose(P.set(b.x, 0, b.z), Q.identity(), S.set(1, 1, 1)); brazier.setMatrixAt(nb++, M); }
      brazier.count = nb;
      for (const [sev, m] of Object.entries(flames)) m.count = counts[sev];
      scaffold.count = nS; rubble.count = nR; smoke.count = nSm;
      lampPost.count = lamp.count = night > 0.05 ? nL : 0; failPole.count = failCloth.count = nF;
      for (const m of [scaffold, rubble, smoke, brazier, lampPost, lamp, failPole, failCloth, bannerPole, bannerCloth, cap, ...Object.values(flames)]) { m.instanceMatrix.needsUpdate = true; if (m.instanceColor) m.instanceColor.needsUpdate = true; }
      for (const k of new Set(Object.values(kinds))) for (const m of [k.body, k.roof, k.copy.body, k.copy.roof]) {
        m.instanceMatrix.needsUpdate = true; if (m.instanceColor) m.instanceColor.needsUpdate = true;
        m.boundingSphere = null; // three computes an InstancedMesh's sphere once, on the first raycast: moved plots must stay tappable
      }
      rubble.boundingSphere = null;
      return busy;
    },
    /** Building under a raycast hit → its plot (cards, fly-to); an annex copy → its app's plot (the card is the app's). */
    plotOf(hit) {
      const k = hit.object.userData.pick?.of;
      const app = k?.apps[hit.instanceId];
      return app ? plots.find((p) => p.app === app) : null;
    },
    /** An annex copy under a raycast hit → {app, host, q, r, x, z} (R8 C: fly to it, open its app's card), else null. */
    annexOf(hit) {
      return hit.object.userData.pick?.of?.items?.[hit.instanceId] || null;
    },
    plots: () => plots,
    annexes: () => tents,
    /** R8 C: annex copies drawn whole this frame (the rest stand as their banner). */
    copies: () => [...new Set(Object.values(kinds))].reduce((n, k) => n + k.copy.body.count, 0),
    pickables: () => [...new Set(Object.values(kinds))].flatMap((k) => [k.body, k.roof, k.copy.body, k.copy.roof]).concat(rubble),
  };
}

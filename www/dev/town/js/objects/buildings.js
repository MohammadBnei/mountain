// Buildings (R7): one per world.state.plots entry, on its host's island, facing the square. One InstancedMesh pair
// (body in clay, roof in the app's colour) per building kind keeps the draw count flat however many apps there are.
// The steady look comes from world.state alone, so a reload and a replay show the same town:
//   health Progressing or a build in flight → scaffold;  Degraded → chimney smoke;  Missing / Suspended → faded
//   plot.ruinAt (app.gone) → ruins;  the host's nodes not Ready → greyed with its island;  alerts → beacon fires
// Transient acts (from live frames, never replayed state): a new or moved plot and a successful sync drop the
// building in (crane drop + squash, pi-lab office.js); pod.churn makes it hop (pi-lab main.js hopPose), and a
// crashloop keeps it hopping until the next healthy sync.
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { BUILDINGS, plotXZ, wireOf } from '../layout.js';

const DROP = 0.9, HOP = 0.75;

export default async function build({ THREE, kit, world, apps }) {
  const root = new THREE.Group();
  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3();
  const C = new THREE.Color();

  // ---- geometry per kind: body (clay atlas, roof cells cut out, props merged in) + roof (white, tinted per instance
  // in its Ukubi wire colour; at night the roofs light up in the Glow theme's wire colours)
  const kinds = {}, roofMat = kit.glowing();
  async function kindOf(role) {
    if (kinds[role]) return kinds[role];
    const spec = BUILDINGS[role] || BUILDINGS.home;
    const src = mergeGeometries(await kit.geometry(spec.glb));
    const [body, roof] = kit.splitByUV(src, kit.inRoof);
    const bodies = [body.scale(spec.s, spec.s, spec.s)];
    for (const [name, x, z, ry, s] of spec.props || []) {
      const g = mergeGeometries(await kit.geometry(name));
      bodies.push(g.applyMatrix4(new THREE.Matrix4().compose(new THREE.Vector3(x * spec.s / 1.4, 0, z * spec.s / 1.4),
        Q.setFromEuler(E.set(0, ry, 0)), S.set(s, s, s))));
    }
    const bg = mergeGeometries(bodies);
    bg.computeBoundingBox();
    const k = {
      body: kit.instanced(bg, kit.shared, 16),
      roof: kit.instanced(roof.scale(spec.s, spec.s, spec.s), roofMat, 16),
      height: bg.boundingBox.max.y, width: Math.max(bg.boundingBox.max.x - bg.boundingBox.min.x, bg.boundingBox.max.z - bg.boundingBox.min.z),
      apps: [],
    };
    for (const m of [k.body, k.roof]) { m.userData.pick = { kind: 'building', of: k }; m.frustumCulled = false; root.add(m); }
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
  const smoke = kit.instanced(new THREE.IcosahedronGeometry(0.3, 1), kit.palette.smoke, N * 4, false);
  // a beacon: a stone post with a fire basket on top, tall enough to read from the overview
  const brazier = kit.instanced(mergeGeometries([new THREE.CylinderGeometry(0.1, 0.16, 1.5, 6).translate(0, 0.75, 0), new THREE.CylinderGeometry(0.34, 0.2, 0.22, 6).translate(0, 1.6, 0)]), kit.palette.stone, N);
  const flameMat = { info: kit.variant('fire', { color: new THREE.Color(0xffe08a), emissive: new THREE.Color(0xffd060) }), warning: kit.variant('fire'), critical: kit.variant('fire', { color: new THREE.Color(0xff4a2a), emissive: new THREE.Color(0xff2a10), emissiveIntensity: 2.2 }) };
  const flameGeo = new THREE.ConeGeometry(0.3, 1.0, 7).translate(0, 0.5, 0); // scaled at its base, lifted onto the basket
  const flames = Object.fromEntries(Object.entries(flameMat).map(([sev, m]) => [sev, kit.instanced(flameGeo, m, N, false)]));
  for (const m of [scaffold, rubble, smoke, brazier, ...Object.values(flames)]) { m.frustumCulled = false; root.add(m); }

  // ---- state → layout
  let plots = [], beacons = [], dirty = true, relaying = null;
  const acts = new Map(); // app → {type, t0, until?}
  world.on('state', () => { dirty = true; });
  world.on('metrics', () => { dirty = true; }); // hosts' Ready rides the metrics frame
  world.on('world:plot', (p) => { if (!p.gone && !p.ruinAt) acts.set(p.app, { type: 'drop', t0: now }); });
  world.on('ev:argo.sync', (f) => { if (f.ok) acts.set(f.app, { type: 'drop', t0: now }); });
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
      list.push({ ...p, k, x, z, yaw, is, wire: wireOf(apps, p.app), down, health,
        building: !!s.builds[p.app] || health === 'Progressing', smoke: health === 'Degraded', faded: health === 'Missing' || health === 'Suspended' });
    }
    plots = list;
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
      return { sev: al.sev, x: x + n * 0.42, y, z: z + n * 0.18, seed: parseInt(al.fp.slice(0, 4), 16) };
    });
  }

  function pose(p, t) {
    // → [y offset, squash (y scale), spin] from the plot's live act
    const a = acts.get(p.app);
    if (!a) return [0, 1, 0];
    const s = t - a.t0;
    if (a.type === 'drop') {
      if (s >= DROP + 0.5) { acts.delete(p.app); return [0, 1, 0]; }
      if (s < DROP * 0.6) { const f = s / (DROP * 0.6); return [3.2 * (1 - f * f), 1 + 0.25 * (1 - f), 0]; }
      const v = (s - DROP * 0.6) / (DROP * 0.4 + 0.5), d = Math.exp(-6 * v);
      return [0, 1 - 0.28 * d * Math.cos(v * 22), 0];
    }
    const cycle = a.loop ? (s % 1.6) : s;
    if (!a.loop && s >= HOP + 0.6) { acts.delete(p.app); return [0, 1, 0]; }
    if (cycle < HOP) { const f = cycle / HOP; return [1.1 * 4 * f * (1 - f), 1 + 0.15 * Math.abs(Math.cos(Math.PI * f)), Math.PI * 2 * kit.easeInOut(f)]; }
    const v = cycle - HOP, d = Math.exp(-6 * v);
    return [0, 1 - 0.25 * d * Math.cos(v * 24), 0];
  }

  const grey = new THREE.Color(0.42, 0.44, 0.5), SINK = 0.5; // islands.js
  return {
    root,
    /** → true while something moves (the host keeps rendering). */
    update(dt, t, night = 0) {
      now = t;
      roofMat.emissiveIntensity = 0.9 * night;
      const wires = kit.ukubi.wires[night > 0.5 ? 'glow' : 'office'];
      if (dirty && !relaying) relaying = relayout().finally(() => { relaying = null; }); // a new kind's GLB loads async
      for (const k of Object.values(kinds)) { k.body.count = k.roof.count = 0; k.apps = []; }
      let nS = 0, nR = 0, nSm = 0, busy = false;
      const counts = { info: 0, warning: 0, critical: 0 };
      for (const p of plots) {
        if (p.ruinAt) {
          M.compose(P.set(p.x, p.down ? -SINK : 0, p.z), Q.setFromEuler(E.set(0, p.yaw, 0)), S.set(1, 1, 1));
          rubble.setMatrixAt(nR, M);
          rubble.setColorAt(nR++, p.down ? grey : C.set(0xd8d2c8));
          continue;
        }
        const [dy, sq, spin] = pose(p, t);
        busy ||= acts.has(p.app);
        const k = p.k, i = k.body.count++;
        k.roof.count++;
        if (i >= k.body.instanceMatrix.count) continue; // more than 16 of one kind: never with the allowlist (≤ 64 plots)
        M.compose(P.set(p.x, dy - (p.down ? SINK : 0), p.z), Q.setFromEuler(E.set(0, p.yaw + spin, 0)), S.set(1 / Math.sqrt(sq), sq, 1 / Math.sqrt(sq)));
        k.body.setMatrixAt(i, M);
        k.roof.setMatrixAt(i, M);
        k.body.setColorAt(i, p.down ? grey : C.setScalar(p.faded ? 0.72 : 1));
        k.roof.setColorAt(i, p.down ? grey : C.set(wires[p.wire]).multiplyScalar(p.faded ? 0.6 : 1));
        k.apps[i] = p.app;
        if (p.building) {
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
      for (const b of beacons) {
        busy = true;
        const n = counts[b.sev]++, fl = 1 + 0.18 * Math.sin(t * 9 + b.seed) + 0.1 * Math.sin(t * 23 + b.seed * 3);
        const big = b.sev === 'critical' ? 1.35 : b.sev === 'info' ? 0.75 : 1;
        M.compose(P.set(b.x, 1.68, b.z), Q.setFromEuler(E.set(0, t * 2 + b.seed, 0)), S.set(big, big * fl, big));
        flames[b.sev].setMatrixAt(n, M);
      }
      let nb = 0;
      for (const b of beacons) { M.compose(P.set(b.x, 0, b.z), Q.identity(), S.set(1, 1, 1)); brazier.setMatrixAt(nb++, M); }
      brazier.count = nb;
      for (const [sev, m] of Object.entries(flames)) m.count = counts[sev];
      scaffold.count = nS; rubble.count = nR; smoke.count = nSm;
      for (const m of [scaffold, rubble, smoke, brazier, ...Object.values(flames)]) { m.instanceMatrix.needsUpdate = true; if (m.instanceColor) m.instanceColor.needsUpdate = true; }
      for (const k of Object.values(kinds)) for (const m of [k.body, k.roof]) { m.instanceMatrix.needsUpdate = true; if (m.instanceColor) m.instanceColor.needsUpdate = true; }
      return busy;
    },
    /** Building under a raycast hit → its plot (cards, fly-to). */
    plotOf(hit) {
      const k = hit.object.userData.pick?.of;
      const app = k?.apps[hit.instanceId];
      return app ? plots.find((p) => p.app === app) : null;
    },
    plots: () => plots,
    pickables: () => Object.values(kinds).flatMap((k) => [k.body, k.roof]),
  };
}

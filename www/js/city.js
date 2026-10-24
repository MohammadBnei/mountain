// L2, the city (plan/10 "The ladder"): one realm, its groups as districts and their parts as buildings. A building's
// height is how many ADR citations govern it (the knowledge weight, from A1's governs edges); its colour is its live
// health, or in the land lens its namespace's share of the cluster. Alerts stand as beacons on the building they
// name. One scene, built on entry and disposed on exit; three instanced draws plus the ground.
import * as THREE from 'three';
import { REALM, governs, health, use, namespaces, label } from './model.js';
import { layout, heightOf } from './geometry.js';

const SURFACE = new THREE.Color('#25201b'), SEA = new THREE.Color('#1c1814'), PLOT = new THREE.Color('#2e2822');
const STATE = { ok: new THREE.Color('#7cc4bf'), degraded: new THREE.Color('#ff8a1f'), down: new THREE.Color('#ec8a72'), unknown: new THREE.Color('#7a6a57') };
const SEV = { critical: new THREE.Color('#ec8a72'), warning: new THREE.Color('#ff8a1f') };

export function createCity({ model, realm }) {
  const scene = new THREE.Scene();
  scene.background = SEA;
  scene.add(new THREE.HemisphereLight(0xfff2dd, 0x14110e, 1.4));
  const sun = new THREE.DirectionalLight(0xfff4e6, 1.7);
  sun.position.set(-6, 14, 8);
  scene.add(sun);
  const r = model.realms[realm], wire = new THREE.Color(REALM[realm].wire);
  const { plots, width, depth } = layout([...r.groups.values()]);
  const disposables = [];
  const keep = (x) => (disposables.push(x), x);

  const ground = new THREE.Mesh(keep(new THREE.BoxGeometry(width + 3, 0.3, depth + 3)), keep(new THREE.MeshStandardMaterial({ color: SURFACE, roughness: 0.95 })));
  ground.position.y = -0.15;
  const rim = new THREE.Mesh(keep(new THREE.BoxGeometry(width + 3.2, 0.12, depth + 3.2)), keep(new THREE.MeshStandardMaterial({ color: wire, emissive: wire, emissiveIntensity: 0.5 })));
  rim.position.y = -0.27;
  scene.add(rim, ground);

  const plotMesh = new THREE.InstancedMesh(keep(new THREE.BoxGeometry(1, 0.06, 1)), keep(new THREE.MeshStandardMaterial({ roughness: 0.9 })), Math.max(1, plots.length));
  const all = plots.flatMap((p) => p.buildings.map((b) => ({ ...b, group: p.g.id })));
  const blds = new THREE.InstancedMesh(keep(new THREE.BoxGeometry(0.8, 1, 0.8)), keep(new THREE.MeshStandardMaterial({ roughness: 0.75, emissive: 0xffffff, emissiveIntensity: 0.04 })), Math.max(1, all.length));
  const beacons = new THREE.InstancedMesh(keep(new THREE.ConeGeometry(0.14, 0.45, 6)), keep(new THREE.MeshBasicMaterial({ color: 0xffffff })), 48);
  const m = new THREE.Matrix4(), q = new THREE.Quaternion(), s = new THREE.Vector3(), p = new THREE.Vector3();
  plots.forEach((pl, i) => {
    plotMesh.setMatrixAt(i, m.compose(p.set(pl.x + pl.w / 2, 0.03, pl.z + pl.d / 2), q, s.set(pl.w, 1, pl.d)));
    plotMesh.setColorAt(i, PLOT);
  });
  plotMesh.count = plots.length;
  for (const b of all) b.h = heightOf(governs(model, b.id).reduce((n, x) => n + x.count, 0));
  all.forEach((b, i) => blds.setMatrixAt(i, m.compose(p.set(b.x, 0.06 + b.h / 2, b.z), q, s.set(1, b.h, 1))));
  blds.count = all.length;
  beacons.count = 0;
  scene.add(plotMesh, blds, beacons);

  function paint(live, view) {
    const c = new THREE.Color(), land = view.lens === 'land';
    const shares = all.map((b) => { const u = use(live, namespaces(model, [b.id])).share; return Math.max(u.cpu || 0, u.mem || 0); });
    const top = Math.max(0.0001, ...shares);
    all.forEach((b, i) => {
      if (land) c.copy(SURFACE).lerp(wire, shares[i] ? 0.15 + 0.85 * (shares[i] / top) : 0.05);
      else c.copy(STATE[health(model, live, b.id).state]).lerp(wire, 0.25);
      if (view.group && view.group !== b.group) c.lerp(SEA, 0.6);
      if (view.sel && view.sel !== b.id) c.lerp(SEA, 0.3);
      blds.setColorAt(i, c);
    });
    if (blds.instanceColor) blds.instanceColor.needsUpdate = true;
    plots.forEach((pl, i) => plotMesh.setColorAt(i, view.group === pl.g.id ? PLOT.clone().lerp(wire, 0.3) : PLOT));
    if (plotMesh.instanceColor) plotMesh.instanceColor.needsUpdate = true;
    beacons.count = 0;
    const at = new Map(all.map((b) => [b.id, b]));
    for (const a of live?.alerts || []) {
      const b = at.get(a.node);
      if (!b || !SEV[a.severity] || beacons.count >= 48) continue;
      beacons.setMatrixAt(beacons.count, m.compose(p.set(b.x, 0.06 + b.h + 0.45, b.z), q.identity(), s.set(1, 1, 1)));
      beacons.setColorAt(beacons.count++, SEV[a.severity]);
    }
    beacons.instanceMatrix.needsUpdate = true;
    if (beacons.instanceColor) beacons.instanceColor.needsUpdate = true;
  }

  return {
    scene,
    /** where the camera goes: the whole city, a district, or a building */
    aim(view) {
      const b = view.sel && all.find((x) => x.id === view.sel), pl = view.group && plots.find((x) => x.g.id === view.group);
      if (b) return { target: new THREE.Vector3(b.x, b.h / 2, b.z), dist: 7 };
      if (pl) return { target: new THREE.Vector3(pl.x + pl.w / 2, 0, pl.z + pl.d / 2), dist: 5 + 1.4 * Math.max(pl.w, pl.d) };
      return { target: new THREE.Vector3(0, 0, 0), fit: [(width + 3) * 1.15, (depth + 3) * 1.25] }; // the towers stand above the plan
    },
    pick(ray) {
      const hit = ray.intersectObjects([blds, plotMesh, ground], false)[0];
      if (!hit) return null;
      if (hit.object === blds) return { sel: all[hit.instanceId].id, group: all[hit.instanceId].group };
      if (hit.object === plotMesh) return { group: plots[hit.instanceId].g.id };
      return { city: true };
    },
    paint,
    labels() {
      return [
        ...plots.map((pl) => ({ key: `d:${pl.g.id}`, text: pl.g.id, sub: `${pl.g.members.length} parts`, at: new THREE.Vector3(pl.x + pl.w / 2, 0.3, pl.z), cls: 'district', group: pl.g.id })),
        ...all.map((b) => ({ key: `b:${b.id}`, text: label(model.nodes.get(b.id)), at: new THREE.Vector3(b.x, 0.06 + b.h + 0.2, b.z), hit: new THREE.Vector3(b.x, 0.06 + b.h * 0.6, b.z), cls: 'bld', group: b.group, id: b.id })),
      ];
    },
    frame(now) { beacons.position.y = 0.1 * Math.sin(now / 300); },
    dispose() {
      for (const x of disposables) x.dispose();
      for (const x of [plotMesh, blds, beacons]) x.dispose();
    },
  };
}

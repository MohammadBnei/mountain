// The 3D map (plan/10 "The ladder", L0 and L1): the seven realms as hex islands, each realm's groups as tiles on it,
// the outside at the rim, and at L1 the trade routes between groups. The L1 route layer is built on entry and
// disposed on exit, so each level holds the draw budget alone (www/dev/gate.mjs).
//
// It is also the stage the deeper levels mount on (mount()): one renderer, camera, controls and loop for every level.
// A mounted level (city.js, building.js) brings its own scene, and a CSS3D scene for L3's cards; only one level's
// scene is ever drawn.
//
// Everything drawn comes from the model and the live frame: an island's size from its component count, a tile's
// height from its group's, a route's width from observed traffic (dashed when nothing measured it), a beacon from a
// firing alert. The land lens recolours tops by their share of cluster capacity; nothing moves.
import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { CSS3DRenderer } from 'three/addons/renderers/CSS3DRenderer.js';
import { health, namespaces, OUTSIDE, REALM, REALMS, routes as tradeRoutes, use } from './model.js';

const SURFACE = new THREE.Color('#25201b'),
  SEA = new THREE.Color('#1c1814'),
  SUNK = new THREE.Color('#14110e');
const SEV = { critical: new THREE.Color('#ec8a72'), warning: new THREE.Color('#ff8a1f') };
const HEALTH = { down: new THREE.Color('#ec8a72'), degraded: new THREE.Color('#ff8a1f') };
const Y = new THREE.Vector3(0, 1, 0);

export const islandRadius = count => 1.15 + 0.32 * Math.sqrt(count);

/** tileSlots places n ≤ 7 group tiles on an island: one at the centre, the rest on a ring. */
export function tileSlots(n, R) {
  if (n <= 0) return [];
  if (n === 1) return [[0, 0]];
  const ring = Math.min(n - (n >= 4 ? 1 : 0), 6),
    slots = n >= 4 ? [[0, 0]] : [],
    r = R * 0.55;
  for (let i = 0; i < ring; i++) {
    const a = (i / ring) * Math.PI * 2 + Math.PI / 6;
    slots.push([r * Math.cos(a), r * Math.sin(a)]);
  }
  return slots;
}

export function createMap({ canvas, low = false, cover = () => 0, onPick = () => {} }) {
  const renderer = new THREE.WebGLRenderer({
    canvas,
    antialias: !low,
    powerPreference: 'high-performance'
  });
  renderer.setPixelRatio(Math.min(devicePixelRatio, low ? 1.25 : 2));
  const scene = new THREE.Scene();
  scene.background = SEA;
  scene.add(new THREE.HemisphereLight(0xfff2dd, 0x14110e, 1.4));
  const sun = new THREE.DirectionalLight(0xfff4e6, 1.6);
  sun.position.set(-6, 14, 8);
  scene.add(sun);
  const cssRenderer = new CSS3DRenderer(); // L3's cards: its own DOM layer over the canvas, pointer-events none
  cssRenderer.domElement.id = 'css3d';
  cssRenderer.domElement.style.pointerEvents = 'none';
  canvas.after(cssRenderer.domElement);
  const emptyCSS = new THREE.Scene();
  let active = null,
    frozen = false; // a mounted level (L2, L3); frozen: the reader (L4) is open, nothing renders
  const camera = new THREE.PerspectiveCamera(38, 1, 0.5, 300);
  const controls = new OrbitControls(camera, canvas);
  Object.assign(controls, {
    enableDamping: true,
    dampingFactor: 0.08,
    minDistance: 5,
    maxDistance: 80,
    maxPolarAngle: 1.25,
    screenSpacePanning: false
  });

  const world = new THREE.Group(),
    routeLayer = new THREE.Group();
  scene.add(world, routeLayer);
  const hexGeo = new THREE.CylinderGeometry(1, 1, 1, 6, 1),
    rimGeo = new THREE.CylinderGeometry(1, 1, 1, 6, 1, true);
  const tileGeo = new THREE.CylinderGeometry(0.42, 0.46, 1, 6, 1),
    buoyGeo = new THREE.CylinderGeometry(0.7, 0.8, 0.25, 6, 1);
  const beaconGeo = new THREE.ConeGeometry(0.16, 0.5, 6),
    dotGeo = new THREE.SphereGeometry(0.05, 6, 4);

  let model = null,
    live = null,
    view = { level: -1 },
    islands = {},
    tiles = null,
    tileInfo = [],
    buoys = null,
    beacons = null,
    beaconInfo = [];
  const pos = new Map(); // endpoint key ("realm/group", "outside/cloudflare", "realm") -> Vector3 on the tops

  // clear frees what a layer built: its own geometries and materials, and each InstancedMesh's instance buffers
  // (three frees those only on the mesh's own dispose()). The shared geometries above stay.
  const shared = new Set([hexGeo, rimGeo, tileGeo, buoyGeo, beaconGeo, dotGeo]);
  function clear(g) {
    for (const o of [...g.children]) {
      g.remove(o);
      o.traverse(x => {
        if (!x.isMesh) return;
        if (!shared.has(x.geometry)) x.geometry.dispose();
        for (const mm of Array.isArray(x.material) ? x.material : [x.material]) mm.dispose();
        if (x.isInstancedMesh) x.dispose();
      });
    }
  }

  /** build lays out the islands, tiles and buoys for a model (a new projection rebuilds them). */
  function build(m) {
    clear(world);
    model = m;
    islands = {};
    pos.clear();
    tileInfo = [];
    let nTiles = 0;
    for (const r of REALMS) nTiles += m.realms[r].groups.size;
    tiles = new THREE.InstancedMesh(
      tileGeo,
      new THREE.MeshStandardMaterial({
        roughness: 0.85,
        metalness: 0,
        emissive: 0xffffff,
        emissiveIntensity: 0.06
      }),
      Math.max(1, nTiles)
    );
    tiles.count = 0;
    const mat = new THREE.Matrix4(),
      q = new THREE.Quaternion(),
      s = new THREE.Vector3(),
      p = new THREE.Vector3();
    for (const r of REALMS) {
      const realm = m.realms[r],
        [x, z] = REALM[r].at,
        R = islandRadius(realm.count),
        h = 0.6;
      const wire = new THREE.Color(REALM[r].wire);
      const top = new THREE.Mesh(
        hexGeo,
        new THREE.MeshStandardMaterial({ color: SURFACE.clone(), roughness: 0.9, metalness: 0 })
      );
      top.scale.set(R, h, R);
      top.position.set(x, -h / 2, z);
      top.rotation.y = Math.PI / 6;
      top.userData = { pick: { realm: r } };
      const rim = new THREE.Mesh(
        rimGeo,
        new THREE.MeshStandardMaterial({
          color: wire,
          emissive: wire,
          emissiveIntensity: 0.55,
          side: THREE.DoubleSide,
          roughness: 0.6
        })
      );
      rim.scale.set(R * 1.03, 0.16, R * 1.03);
      rim.position.set(x, -0.02, z);
      rim.rotation.y = Math.PI / 6;
      rim.userData = { pick: { realm: r } };
      world.add(top, rim);
      islands[r] = { top, rim, R, wire, centre: new THREE.Vector3(x, 0, z) };
      pos.set(r, new THREE.Vector3(x, 0.2, z));
      const groups = [...realm.groups.values()],
        slots = tileSlots(groups.length, R);
      groups.forEach((g, i) => {
        const [dx, dz] = slots[i] || [0, 0],
          th = 0.25 + 0.28 * Math.log2(1 + g.members.length);
        p.set(x + dx, th / 2, z + dz);
        s.set(1, th, 1);
        q.setFromAxisAngle(Y, Math.PI / 6);
        tiles.setMatrixAt(tiles.count, mat.compose(p, q, s));
        tiles.setColorAt(tiles.count, wire.clone().lerp(SURFACE, 0.55));
        tileInfo.push({ realm: r, group: g.id, top: new THREE.Vector3(x + dx, th, z + dz) });
        pos.set(`${r}/${g.id}`, new THREE.Vector3(x + dx, th, z + dz));
        tiles.count++;
      });
    }
    tiles.instanceMatrix.needsUpdate = true;
    if (tiles.instanceColor) tiles.instanceColor.needsUpdate = true;
    tiles.userData = { tiles: true };
    world.add(tiles);
    buoys = new THREE.InstancedMesh(
      buoyGeo,
      new THREE.MeshStandardMaterial({
        color: SUNK,
        roughness: 0.9,
        emissive: 0x7a6a57,
        emissiveIntensity: 0.25
      }),
      Object.keys(OUTSIDE).length
    );
    buoys.count = 0;
    for (const [k, o] of Object.entries(OUTSIDE)) {
      p.set(o.at[0], 0, o.at[1]);
      buoys.setMatrixAt(buoys.count++, mat.compose(p, q.identity(), s.set(1, 1, 1)));
      pos.set(`outside/${k}`, new THREE.Vector3(o.at[0], 0.15, o.at[1]));
    }
    buoys.userData = { buoys: Object.keys(OUTSIDE) };
    world.add(buoys);
    beacons = new THREE.InstancedMesh(
      beaconGeo,
      new THREE.MeshBasicMaterial({ color: 0xffffff }),
      64
    );
    beacons.count = 0;
    world.add(beacons);
    paint();
    if (view.level === 1) buildRoutes();
  }

  /** paint recolours from the live frame and the lens: tops, tile tint, beacons. Nothing moves. */
  function paint() {
    if (!model) return;
    const land = view.lens === 'land';
    const shares = REALMS.map(r => {
      const s = live?.land?.realms?.[r];
      return s ? Math.max(s.cpu, s.mem, s.disk) : 0;
    });
    const top = Math.max(0.0001, ...shares);
    for (const [i, r] of REALMS.entries()) {
      const is = islands[r],
        focus = !view.realm || view.realm === r;
      is.top.material.color.copy(
        land ? SURFACE.clone().lerp(is.wire, 0.12 + 0.6 * (shares[i] / top)) : SURFACE
      );
      is.rim.material.emissiveIntensity = focus ? 0.6 : 0.12;
      is.rim.material.color.copy(focus ? is.wire : is.wire.clone().lerp(SEA, 0.6));
    }
    const c = new THREE.Color();
    tileInfo.forEach((t, i) => {
      const realm = model.realms[t.realm],
        g = realm.groups.get(t.group),
        wire = islands[t.realm].wire;
      if (land) {
        const u = use(live, namespaces(model, g.members)).share;
        const v = Math.max(u.cpu || 0, u.mem || 0, u.disk || 0);
        c.copy(SURFACE).lerp(wire, Math.min(1, 0.15 + 4 * v));
      } else {
        const states = g.members.map(id => health(model, live, id).state);
        const worst = states.includes('down')
          ? 'down'
          : states.includes('degraded')
            ? 'degraded'
            : null;
        c.copy(wire).lerp(SURFACE, 0.55);
        if (worst) c.lerp(HEALTH[worst], 0.6);
      }
      if (view.realm && view.realm !== t.realm) c.lerp(SEA, 0.65);
      if (view.group && view.realm === t.realm && view.group !== t.group) c.lerp(SEA, 0.35);
      tiles.setColorAt(i, c);
    });
    if (tiles.instanceColor) tiles.instanceColor.needsUpdate = true;
    // beacons: one per firing alert (warning, critical), over its group's tile, else its realm's centre
    beaconInfo = [];
    const mat = new THREE.Matrix4(),
      q = new THREE.Quaternion(),
      s = new THREE.Vector3(1, 1, 1);
    beacons.count = 0;
    const seen = new Map();
    for (const a of live?.alerts || []) {
      if (!SEV[a.severity] || beacons.count >= 64) continue;
      const node = a.node && model.nodes.get(a.node);
      const key = node?.realm ? `${node.realm}/${node.group}` : a.realm;
      const at = key && pos.get(key);
      if (!at) continue;
      const n = seen.get(key) || 0;
      seen.set(key, n + 1);
      const p = at.clone().add(new THREE.Vector3(0.22 * n, 0.55, 0));
      beacons.setMatrixAt(beacons.count, mat.compose(p, q, s));
      beacons.setColorAt(beacons.count, SEV[a.severity]);
      beaconInfo.push(p);
      beacons.count++;
    }
    beacons.instanceMatrix.needsUpdate = true;
    if (beacons.instanceColor) beacons.instanceColor.needsUpdate = true;
    if (view.level === 1) paintRoutes();
  }

  // ---- L1: trade routes
  let routeList = [],
    routeShape = '';
  function buildRoutes() {
    clear(routeLayer);
    routeList = [];
    if (!model) return;
    routeShape = tradeRoutes(model, live)
      .map(r => `${r.key}:${r.observed ? Math.round(8 * Math.log2(1 + r.rps)) : '-'}`)
      .join();
    const dots = [];
    for (const r of tradeRoutes(model, live)) {
      const a = pos.get(r.from),
        b = pos.get(r.to);
      if (!a || !b) continue;
      const mid = a.clone().lerp(b, 0.5),
        lift = 0.6 + 0.12 * a.distanceTo(b);
      mid.y += lift;
      const curve = new THREE.QuadraticBezierCurve3(a.clone(), mid, b.clone());
      const realmEnd = r.from.startsWith('outside/') ? r.to : r.from;
      const wire = new THREE.Color(REALM[realmEnd.split('/')[0]]?.wire || '#9a8a72');
      const item = { route: r, curve, wire, mesh: null };
      if (r.observed) {
        const radius = 0.035 + 0.035 * Math.log2(1 + r.rps);
        item.mesh = new THREE.Mesh(
          new THREE.TubeGeometry(curve, 24, radius, 5, false),
          new THREE.MeshStandardMaterial({ color: wire, emissive: wire, emissiveIntensity: 0.5 })
        );
        routeLayer.add(item.mesh);
      } else {
        const n = Math.max(6, Math.round(curve.getLength() / 0.35));
        for (let i = 1; i < n; i++) dots.push({ p: curve.getPoint(i / n), item });
      }
      routeList.push(item);
    }
    const inst = new THREE.InstancedMesh(
      dotGeo,
      new THREE.MeshBasicMaterial({ color: 0xffffff }),
      Math.max(1, dots.length)
    );
    inst.count = dots.length;
    const mat = new THREE.Matrix4(),
      q = new THREE.Quaternion(),
      s = new THREE.Vector3(1, 1, 1);
    dots.forEach((d, i) => {
      inst.setMatrixAt(i, mat.compose(d.p, q, s));
    });
    inst.userData = { dots };
    routeLayer.add(inst);
    paintRoutes();
  }
  function paintRoutes() {
    const inst = routeLayer.children.find(o => o.userData.dots);
    const on = r =>
      !view.realm || r.from.startsWith(view.realm + '/') || r.to.startsWith(view.realm + '/');
    for (const it of routeList)
      if (it.mesh) it.mesh.material.emissiveIntensity = on(it.route) ? 0.6 : 0.05;
    if (!inst) return;
    const c = new THREE.Color();
    inst.userData.dots.forEach((d, i) => {
      inst.setColorAt(i, c.copy(d.item.wire).lerp(SEA, on(d.item.route) ? 0.25 : 0.85));
    });
    if (inst.instanceColor) inst.instanceColor.needsUpdate = true;
  }

  // ---- camera
  const goal = { target: new THREE.Vector3(), dist: 30, t: 1, dir: null },
    from = { target: new THREE.Vector3(), pos: new THREE.Vector3() };
  const DIR = new THREE.Vector3(0.12, 0.92, 0.75).normalize();
  function aim(instant) {
    if (!model) return;
    let target,
      dist,
      dir = DIR;
    if (active) {
      const a = active.aim(view);
      target = a.target;
      dist = a.fit ? fitDistance(...a.fit) : a.dist;
      dir = a.dir || DIR;
    } else if (view.realm && islands[view.realm]) {
      target = islands[view.realm].centre.clone();
      dist = 6 + islands[view.realm].R * 3.2;
    } else {
      target = new THREE.Vector3(-1.5, 0, 0);
      dist = fitDistance();
    }
    goal.target.copy(target);
    goal.dist = dist;
    goal.dir = dir;
    from.target.copy(controls.target);
    from.pos.copy(camera.position);
    goal.t = instant ? 1 : 0;
    if (instant) {
      controls.target.copy(target);
      camera.position.copy(target).addScaledVector(dir, dist);
    }
  }
  function fitDistance(w = 24, h = 17) {
    // the whole extent on screen: the widest extent over the narrower half-FOV
    const vfov = (camera.fov * Math.PI) / 360,
      hfov = Math.atan(Math.tan(vfov) * camera.aspect * free);
    return Math.max(h / 2 / Math.tan(vfov), w / 2 / Math.tan(hfov)) * 1.05;
  }

  // The panel covers the right of the canvas on wide screens: the projection centre sits in the free area left of it
  // (setViewOffset), so every view and fly-in centres where it can be seen, never behind the panel.
  let free = 1;
  function resize() {
    const w = canvas.clientWidth,
      h = canvas.clientHeight;
    if (!w || !h) return;
    renderer.setSize(w, h, false);
    cssRenderer.setSize(w, h);
    const p = Math.min(cover(), w * 0.6);
    free = (w - p) / w;
    camera.aspect = w / h;
    if (p > 0) camera.setViewOffset(w + p, h, p, 0, w, h);
    else camera.clearViewOffset();
    camera.updateProjectionMatrix();
  }

  // ---- picking: an island, a tile (group) or a buoy
  const ray = new THREE.Raycaster(),
    ndc = new THREE.Vector2();
  function pick(x, y) {
    const r = canvas.getBoundingClientRect();
    ndc.set(((x - r.left) / r.width) * 2 - 1, -((y - r.top) / r.height) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    if (active) return active.pick(ray);
    const hit = ray.intersectObjects(
      [tiles, buoys, ...Object.values(islands).flatMap(i => [i.top, i.rim])],
      false
    )[0];
    if (!hit) return null;
    if (hit.object === tiles)
      return { realm: tileInfo[hit.instanceId].realm, group: tileInfo[hit.instanceId].group };
    if (hit.object === buoys) return { outside: buoys.userData.buoys[hit.instanceId] };
    return hit.object.userData.pick || null;
  }
  let down = null;
  canvas.addEventListener('pointerdown', e => {
    down = [e.clientX, e.clientY];
  });
  canvas.addEventListener('pointerup', e => {
    if (!down || Math.hypot(e.clientX - down[0], e.clientY - down[1]) > 6) return;
    onPick(pick(e.clientX, e.clientY));
  });

  // ---- loop
  const listeners = [];
  let last = performance.now(),
    stats = { calls: 0, triangles: 0 };
  renderer.setAnimationLoop(now => {
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    if (goal.t < 1) {
      goal.t = Math.min(1, goal.t + dt / 0.7);
      const k = goal.t * goal.t * (3 - 2 * goal.t);
      controls.target.lerpVectors(from.target, goal.target, k);
      const end = goal.target.clone().addScaledVector(goal.dir || DIR, goal.dist);
      camera.position.lerpVectors(from.pos, end, k);
    }
    if (frozen) return;
    controls.update();
    if (beacons) beacons.position.y = 0.12 * Math.sin(now / 300); // the beacons bob: something is firing
    active?.frame(now);
    renderer.render(active ? active.scene : scene, camera);
    cssRenderer.render(active?.css || emptyCSS, camera);
    stats = { calls: renderer.info.render.calls, triangles: renderer.info.render.triangles };
    for (const f of listeners) f();
  });
  new ResizeObserver(resize).observe(canvas);
  resize();

  return {
    camera,
    get stats() {
      return stats;
    },
    onFrame(f) {
      listeners.push(f);
    },
    /** where an endpoint ("realm", "realm/group", "outside/cloudflare") sits, for the labels. */
    where(key) {
      return pos.get(key) || null;
    },
    /** the back edge of a realm's island (its label's place at L1, clear of the group labels) */
    edge(r) {
      const is = islands[r];
      return is ? is.centre.clone().add(new THREE.Vector3(0, 0.3, -is.R * 0.95)) : null;
    },
    setModel(m) {
      build(m);
      aim(true);
    },
    /** mount a deeper level (L2 city, L3 building) on the stage, or null for the map. The previous one is disposed. */
    mount(mod, keepCamera = false) {
      if (active && active !== mod) active.dispose();
      active = mod;
      if (mod) mod.paint(live, view);
      if (!keepCamera) aim(false);
    },
    get active() {
      return active;
    },
    /** re-aim the camera for the current view (a focused card, a selected building) */
    reaim() {
      aim(false);
    },
    /** freeze (the reader is open): stop drawing, and hand back one still of the last frame for the minimap */
    freeze(on) {
      if (on && !frozen) {
        renderer.render(active ? active.scene : scene, camera);
        const still = canvas.toDataURL('image/png');
        frozen = true;
        return still;
      }
      frozen = !!on;
      return null;
    },
    setLive(l) {
      live = l;
      active?.paint(live, view);
      // the routes are rebuilt only when their shape changed (which are measured, how wide): a live frame that
      // moves a number by a hair repaints, never reallocates
      const shape = model
        ? tradeRoutes(model, live)
            .map(r => `${r.key}:${r.observed ? Math.round(8 * Math.log2(1 + r.rps)) : '-'}`)
            .join()
        : '';
      const rebuild = view.level === 1 && shape !== routeShape;
      routeShape = shape;
      if (rebuild) buildRoutes();
      else paint();
    },
    setView(v) {
      const levelChanged = v.level !== view.level,
        moved =
          v.realm !== view.realm ||
          levelChanged ||
          v.group !== view.group ||
          v.sel !== view.sel ||
          v.id !== view.id;
      view = v;
      if (levelChanged) {
        if (v.level === 1) buildRoutes();
        else {
          clear(routeLayer);
          routeList = [];
        }
      }
      paint();
      active?.paint(live, view);
      if (moved && v.level <= 3) aim(false);
    },
    resize
  };
}

/** project gives screen coordinates (CSS px) of a world point, or null when it is behind the camera. */
export function project(camera, canvas, v) {
  const p = v.clone().project(camera);
  if (p.z > 1 || p.z < -1) return null;
  return [((p.x + 1) / 2) * canvas.clientWidth, ((1 - p.y) / 2) * canvas.clientHeight];
}

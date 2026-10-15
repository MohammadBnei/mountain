// Visitor layer host (W4; 03 "Surface-layer rules" + "Input", 08). Orchestrator-owned. Lanes (toy-clouds, cairns,
// footprints, toss) only write models/<name>/ and relay/toy_<name>.go; this file is what they plug into.
//
//   SURFACE                       the surface-layer models: main.js parents their roots under one group it hides when
//                                 metrics.visitors is false (the kill switch), with kit.stampScene (footprints)
//   pickTerrain(world, ray, out?) → out (THREE.Vector3) | null: analytic ray-march of world.heightAt (heightSolid), no mesh
//   surfacePhysics({max}?) → {phys, collider(), body(opts), cap}: a cannon-es world for one surface model (00 "Physics":
//                                 group surface = 2, mask 2, never meets scree's real group 1): gravity, SAP, solver 20,
//                                 friction 0.8, heightSolid as a Heightfield (collider() re-reads it: call it before a
//                                 drop, deposits move the land), body(opts) = a CANNON.Body in the surface group,
//                                 cap = max bodies (low tier: half). The model steps phys itself in step(dt).
//   createToys({world, camera, canvas, orbit, tale, models, surface}) → {tick()}: the toolbar + input routing; tick() per frame
//
// A model joins the toolbar by exposing `tool` (optional, beside the contract's members):
//   model.tool = { icon: '▲', label: 'tool.<name>' (i18n key), drag(phase, hit, e) }
//     phase 'start' | 'move' | 'end' | 'cancel'; hit = THREE.Vector3 on the terrain (reused: copy it) or null off it.
//     The model then sends its mark with world.place('<name>', pose) → Promise<id> (world.js), e.g. pose {pos: [x, y, z]}
//     (the relay recomputes y, quantises x/z, checks the snowline: Place.Spot), and draws it when world:mark arrives
//     (to everyone, the sender too). A refused place() rejects with the reason (catch it); the toolbar shows it too.
// Tool mode exists only in the Live section with the story at rest (explore), for a signed-in visitor or admin while
// visitors are enabled. A drag that starts on the terrain goes to the tool; one that starts on the sky, or any second
// finger, orbits (pi-lab compose/www/js/main.js:217-307 routing). Admins also get an eraser: a tap on a mark deletes it.
import * as THREE from 'three';
import * as CANNON from 'cannon-es';
import { GRID, CELL } from './world.js';
import { QUALITY } from './kit.js';
import S from './i18n.json' with { type: 'json' };

export const SURFACE = new Set(['toy-clouds', 'cairns', 'footprints', 'toss']);
const ENROLL = 'https://authentik.bnei.dev/if/flow/mountain-enrollment/'; // 08 item 4: the brand enrollment_flow is unset on purpose
const T = S[navigator.language?.toLowerCase().startsWith('fr') ? 'fr' : 'en'];

export function pickTerrain(world, ray, out = new THREE.Vector3(), far = 600) {
  const o = ray.origin, d = ray.direction, below = (t) => {
    const x = o.x + d.x * t, z = o.z + d.z * t;
    return Math.abs(x) <= 100 && Math.abs(z) <= 100 && o.y + d.y * t < world.heightAt(x, z);
  };
  let a = 0;
  for (let t = 0.5; t <= far; a = t, t += 0.5) {
    if (!below(t)) continue;
    for (let i = 0, b = t; i < 10; i++) { const m = (a + b) / 2; if (below(m)) b = m; else a = m; } // bisect onto the surface
    return out.copy(d).multiplyScalar(a).add(o);
  }
  return null;
}

export function surfacePhysics({ max = 40 } = {}) {
  const phys = new CANNON.World({ gravity: new CANNON.Vec3(0, -9.82, 0), allowSleep: true });
  phys.broadphase = new CANNON.SAPBroadphase(phys);
  phys.solver.iterations = 20;
  phys.defaultContactMaterial.friction = 0.8;
  const SURF = 2, body = (opts) => new CANNON.Body({ ...opts, collisionFilterGroup: SURF, collisionFilterMask: SURF });
  // heightSolid as a Heightfield, the scree convention: data[i][yi] (local z up) rotated −90° about x, world z = z0 − yi·CELL
  const data = Array.from({ length: GRID }, () => new Array(GRID).fill(0)), hf = new CANNON.Heightfield(data, { elementSize: CELL });
  const ground = body({ mass: 0, shape: hf });
  ground.position.set(-100, 0, -100 + (GRID - 1) * CELL);
  ground.quaternion.setFromEuler(-Math.PI / 2, 0, 0);
  phys.addBody(ground);
  return {
    phys, body, cap: QUALITY === 'low' ? max >> 1 : max,
    collider(world) {
      const hs = world.fields.heightSolid;
      for (let i = 0; i < GRID; i++) for (let yi = 0; yi < GRID; yi++) data[i][yi] = hs[(GRID - 1 - yi) * GRID + i];
      hf.updateMinValue(); hf.updateMaxValue(); hf.updateBoundingSphereRadius(); hf.update();
    },
  };
}

export function createToys({ world, camera, canvas, orbit, tale, models, surface }) {
  const $ = (id) => document.getElementById(id), bar = $('toys'), play = $('play'), status = $('toystatus');
  const ray = new THREE.Raycaster(), ndc = new THREE.Vector2(), hit = new THREE.Vector3(), down = new Set();
  let role = null, tool = null, drag = null, shown = null;

  const button = (label, icon, onclick) => {
    const b = Object.assign(document.createElement('button'), { type: 'button', textContent: icon, title: T[label] || label, onclick });
    b.setAttribute('aria-label', T[label] || label);
    b.setAttribute('aria-pressed', 'false');
    bar.append(b);
    return b;
  };
  const say = (k) => { status.textContent = T[`toys.err.${k}`] || T['toys.err.bad']; status.hidden = false; setTimeout(() => (status.hidden = true), 4000); };
  const select = (t, b) => {
    tool = tool === t ? null : t;
    for (const x of bar.querySelectorAll('button[data-tool]')) x.setAttribute('aria-pressed', String(x === b && !!tool));
  };
  bar.setAttribute('aria-label', T['toys.label']);
  for (const m of models) if (m.tool) {
    const b = button(m.tool.label, m.tool.icon, () => select(m.tool, b));
    b.dataset.tool = '';
  }
  // admin eraser (08 moderation, detail mode): the nearest active mark within 3 m of the tap
  const eraser = { drag(phase, h) {
    if (phase !== 'end' || !h) return;
    let best = null, bd = 9;
    for (const m of world.state.marks || []) {
      const p = m.pose?.pos;
      const d = p ? (p[0] - h.x) ** 2 + (p[2] - h.z) ** 2 : 1e9;
      if (d < bd) { bd = d; best = m; }
    }
    if (best) world.unmark(best.id).catch((e) => say(e.message));
  } };
  const erase = button('toys.erase', '✕', () => select(eraser, erase));
  erase.dataset.tool = '';
  erase.hidden = true;
  const named = button('toys.named', '✎', () => { world.named = !world.named; named.setAttribute('aria-pressed', String(world.named)); });
  $('enroll').href = ENROLL;
  $('enroll').textContent = T['toys.play'];
  $('signin2').textContent = T['toys.signin'];
  $('signin2').href = '/auth/login?return=%2F%23live';
  fetch('/auth/me').then((r) => (r.ok ? r.json() : null)).then((a) => {
    role = a?.enabled ? a.role : null;
    erase.hidden = role !== 'admin';
    if (a?.handle) named.title = `${T['toys.named']} (${a.handle})`;
  }, () => {});
  // every refused place() shows its reason on the status line; the model still gets the rejection (models see world
  // through world.as views, which inherit this)
  const place = world.place;
  world.place = (mark, pose) => place(mark, pose).catch((e) => { say(e.message); throw e; });
  const tools = models.filter((m) => m.tool).length;

  const allowed = () => shown === 'tools';
  const pick = (e) => {
    const r = canvas.getBoundingClientRect();
    ndc.set(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    return pickTerrain(world, ray.ray, hit);
  };
  const stop = (phase, e) => {
    const t = drag.tool;
    drag = null;
    orbit.enabled = true;
    t.drag(phase, phase === 'end' ? pick(e) : null, e);
  };
  // capture phase: runs before OrbitControls' own pointerdown on the canvas, so a tool drag never reaches the orbit
  canvas.addEventListener('pointerdown', (e) => {
    down.add(e.pointerId);
    if (drag) { if (down.size > 1) stop('cancel', e); return; } // a second finger: the gesture is the orbit's
    // ponytail: OrbitControls missed the first finger, so after a cancel it orbits on the second alone until both lift
    if (!tool || !allowed() || down.size > 1 || e.button !== 0 || !pick(e)) return; // the sky orbits
    drag = { tool, id: e.pointerId };
    orbit.enabled = false;
    canvas.setPointerCapture(e.pointerId);
    tool.drag('start', hit, e);
  }, { capture: true });
  canvas.addEventListener('pointermove', (e) => { if (drag?.id === e.pointerId) drag.tool.drag('move', pick(e), e); });
  for (const type of ['pointerup', 'pointercancel']) canvas.addEventListener(type, (e) => {
    down.delete(e.pointerId);
    if (drag?.id === e.pointerId) stop(type === 'pointerup' ? 'end' : 'cancel', e);
  });

  return {
    /** Per frame: the surface layer follows the kill switch; the toolbar or the sign-in links follow section + role. */
    tick() {
      const on = world.metrics?.visitors !== false, live = tale.section === 'live' && tale.P === 5;
      const want = !on || !live || !role ? 'none' : role === 'view' ? 'play' : tools || role === 'admin' ? 'tools' : 'none';
      if (surface.visible !== on) surface.visible = on; // main.js redraws the stamp RT on this change
      if (want === shown) return;
      shown = want;
      bar.hidden = want !== 'tools';
      play.hidden = want !== 'play';
      if (want !== 'tools' && drag) stop('cancel', null);
    },
  };
}

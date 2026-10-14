// Page: kit rig + world + socket + the models of page.js, live finale camera (story is W3), post on desktop.
// Orchestrator-owned. ?fixture=1 loads dev/fixtures/hello.json instead of /ws (static serving).
// Host duties: world.rig → kit.setPhase on change; world.stale → the "stale since" label (sky dims via kit.setStale).
// Dev (127.0.0.1): globalThis.__page = {renderer, scene, camera, post, world, kit, quality(), orbit()} for the gate.
import * as THREE from 'three';
import * as kit from './kit.js';
import { createWorld } from './world.js';
import { connect } from './socket.js';
import { profiles, finale } from './layout.js';
import { page } from './page.js';
import { makeCtx, checkModel, wire } from './contract.js';
import { createPost } from './post.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

const q = new URLSearchParams(location.search);
const renderer = new THREE.WebGLRenderer({ canvas: document.getElementById('c'), antialias: kit.QUALITY !== 'low' });
const ratioMax = Math.min(devicePixelRatio, 1.5); // bloom at DPR 2 on a retina laptop costs heat for detail fog hides
renderer.setPixelRatio(ratioMax);
const scene = new THREE.Scene();
const camera = new THREE.PerspectiveCamera(finale.fov, 1, 1, 2000);
kit.rig(renderer, scene);
const post = createPost(renderer, scene, camera); // null on phones (low tier)

const dev = location.hostname === '127.0.0.1';
const world = createWorld({ dev });
if (dev) globalThis.__page = { renderer, scene, camera, post, world, kit, quality: () => ({ rung, fps: ladder[rung][0], ratio: ladder[rung][1], charging }), orbit: () => orbit };
if (q.has('fixture')) fetch('./dev/fixtures/hello.json').then((r) => r.json()).then(world.applyHello);
else connect({ world });
await world.ready;

const models = [];
for (const name of page) {
  let build;
  try {
    ({ default: build } = await import(`../models/${name}/index.js`));
  } catch (e) {
    console.warn(`main: model ${name} not available yet`, e.message);
    continue;
  }
  const m = await build(makeCtx({ THREE, kit, world, name, profile: profiles[name] }));
  const bad = checkModel(m, profiles[name]);
  if (bad.length) { console.error(`main: ${name}: ${bad.join('; ')}`); continue; }
  wire(m, world);
  m.pose(6); // live finale; story.js drives P from W3
  scene.add(m.root);
  models.push(m);
}

function resize() {
  const w = innerWidth, h = innerHeight;
  renderer.setSize(w, h, false);
  post?.setSize(w, h);
  camera.aspect = w / h;
  camera.updateProjectionMatrix();
}
addEventListener('resize', resize);
resize();

const staleEl = document.getElementById('stale'), hhmm = new Intl.DateTimeFormat([], { hour: '2-digit', minute: '2-digit' });
let staleShown, rig;
function host() {
  if (world.rig !== rig) { rig = world.rig; kit.setPhase(rig.elev, rig.rising); } // sky writes a new object per change
  if (world.stale !== staleShown) {
    staleShown = world.stale;
    staleEl.hidden = staleShown == null;
    if (staleShown != null) staleEl.textContent = `stale since ${hhmm.format(staleShown)}`;
  }
}

// Orbit (drag, wheel/pinch; no pan) around the finale look point. The first touch takes the camera from
// the sway; 45 s after the last input it glides back. Limits keep the camera off the massif and above the valley floor.
const look = new THREE.Vector3(...finale.look), swayPos = new THREE.Vector3();
const orbit = new OrbitControls(camera, renderer.domElement);
Object.assign(orbit, { enablePan: false, enableDamping: true, dampingFactor: 0.08, minDistance: 110, maxDistance: 260,
  minPolarAngle: 0.9, maxPolarAngle: 1.7, rotateSpeed: 0.6, zoomSpeed: 0.8 });
orbit.target.copy(look);
let orbiting = false, idleAt = 0;
orbit.addEventListener('start', () => { orbiting = true; idleAt = 0; });
orbit.addEventListener('end', () => { idleAt = performance.now() + 45000; });

const DT = 1 / 30;
// Adaptive draw rate (heat/battery): a ladder of [fps, pixel ratio] from cheapest to richest. 60 fps is
// allowed only while charging on desktop (Battery API; none = never). Each ~1 s window of draws: mean
// frame > 1.2× its budget steps one rung down and doubles that rung's retry wait; otherwise, once the
// wait is over, it probes one rung up. The sim steps at 1/30 s either way; at 60 only the camera moves in between.
const ladder = [[30, Math.min(1, ratioMax)], [30, Math.min(1.25, ratioMax)], [30, ratioMax], [60, ratioMax]]
  .filter((r, i, l) => !i || r[0] !== l[i - 1][0] || r[1] !== l[i - 1][1]);
let charging = false, rung = ladder.findLastIndex((r) => r[0] === 30), gSum = 0, gN = 0;
const wait = ladder.map(() => 3000), upAt = ladder.map(() => 0);
const top = () => (charging && kit.QUALITY !== 'low' ? ladder.length - 1 : ladder.findLastIndex((r) => r[0] === 30));
const setRung = (i) => { rung = i; renderer.setPixelRatio(ladder[i][1]); post?.composer.setPixelRatio(ladder[i][1]); }; // the composer keeps its own copy
navigator.getBattery?.().then((b) => {
  const set = () => { charging = b.charging; if (rung > top()) setRung(top()); };
  set();
  b.addEventListener('chargingchange', set);
}).catch(() => {});
function govern(ms, now) {
  gSum += Math.min(ms, 200); gN++;
  if (gSum < 1000) return;
  const late = gSum / gN > 1.2 * (1000 / ladder[rung][0]);
  gSum = gN = 0;
  if (late && rung > 0) { wait[rung] = Math.min(wait[rung] * 2, 120000); upAt[rung] = now + wait[rung]; setRung(rung - 1); }
  else if (!late && rung < top() && now >= upAt[rung + 1]) setRung(rung + 1);
}
let acc = 0, t = 0, last = performance.now(), drawn = 0;
addEventListener('visibilitychange', () => { drawn = gSum = gN = 0; }); // the gap of a hidden tab is not a slow frame
renderer.setAnimationLoop((now) => {
  if (now - drawn < 1000 / ladder[rung][0] - 2) return;
  if (drawn) govern(now - drawn, now);
  drawn = now;
  acc = Math.min(acc + (now - last) / 1000, 0.25);
  last = now;
  while (acc >= DT) {
    world.step(DT);
    for (const m of models) m.step(DT, t);
    acc -= DT;
    t += DT;
  }
  host();
  kit.U.uSnowline.value = world.snowline;
  kit.U.uHMax.value = world.hmax; // deposits and resyncs move the summit
  const tc = t + acc; // camera time between sim steps: smooth at 60 fps
  const az = finale.sway * Math.min(1, camera.aspect) * Math.sin((2 * Math.PI * tc) / finale.period); // narrower on portrait: the pool stays in frame
  swayPos.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
  if (!orbiting) { camera.position.copy(swayPos); camera.lookAt(look); }
  else if (idleAt && now > idleAt) { // idle: glide back to the sway, then hand the camera back to it
    camera.position.lerp(swayPos, 0.05);
    orbit.target.lerp(look, 0.05);
    camera.lookAt(orbit.target);
    if (camera.position.distanceTo(swayPos) < 0.5) { orbiting = false; idleAt = 0; orbit.target.copy(look); }
  } else orbit.update();
  if (post) post.render(t);
  else renderer.render(scene, camera);
});

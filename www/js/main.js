// Page: kit rig + world + socket + the models of layout.page, live finale camera (story is W3), post on desktop.
// Orchestrator-owned. ?fixture=1 loads dev/fixtures/hello.json instead of /ws (static serving).
// Host duties: world.rig → kit.setPhase on change; world.stale → the "stale since" label (sky dims via kit.setStale).
// Dev (127.0.0.1): globalThis.__page = {renderer, scene, camera, post, world, kit} for the gate.
import * as THREE from 'three';
import * as kit from './kit.js';
import { createWorld } from './world.js';
import { connect } from './socket.js';
import { profiles, page, finale } from './layout.js';
import { makeCtx, checkModel, wire } from './contract.js';
import { createPost } from './post.js';

const q = new URLSearchParams(location.search);
const renderer = new THREE.WebGLRenderer({ canvas: document.getElementById('c'), antialias: kit.QUALITY !== 'low' });
renderer.setPixelRatio(Math.min(devicePixelRatio, kit.QUALITY === 'low' ? 1.5 : 2));
const scene = new THREE.Scene();
const camera = new THREE.PerspectiveCamera(finale.fov, 1, 1, 2000);
kit.rig(renderer, scene);
const post = createPost(renderer, scene, camera); // null on phones (low tier)

const dev = location.hostname === '127.0.0.1';
const world = createWorld({ dev });
if (dev) globalThis.__page = { renderer, scene, camera, post, world, kit };
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

const DT = 1 / 30;
let acc = 0, t = 0, last = performance.now();
renderer.setAnimationLoop((now) => {
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
  const az = finale.sway * Math.min(1, camera.aspect) * Math.sin((2 * Math.PI * t) / finale.period); // narrower on portrait: the pool stays in frame
  camera.position.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
  camera.lookAt(...finale.look);
  if (post) post.render(t);
  else renderer.render(scene, camera);
});

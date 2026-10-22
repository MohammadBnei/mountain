// The live town (R7): ukubi as three islands, one per Proxmox host, every app and service a building on the host
// its pods run on. The relay (/ws) is the only source: hello, then ev / world / metrics frames (js/world.js).
// Opens on the whole town; tap an island or a building to fly in (and read its card), tap the sea to fly back.
// The sky follows the real sun over Paris (js/sun.js). Dev (127.0.0.1): window.__town hooks for the gate.
import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { createWorld } from './world.js';
import { connect } from './socket.js';
import { sun as sunAt, now as clock } from './sun.js';
import { createLabels } from './labels.js';

const canvas = document.getElementById('c');
const q = new URLSearchParams(location.search);
const dev = location.hostname === '127.0.0.1';
const lang = () => (q.get('lang') || navigator.language || 'en').slice(0, 2) === 'fr' ? 'fr' : 'en';
const coarse = matchMedia('(pointer: coarse)').matches;
globalThis.__quality = q.get('q') || (coarse && ((navigator.deviceMemory || 8) <= 4 || (navigator.hardwareConcurrency || 8) <= 4) ? 'low' : 'high');
if (dev) globalThis.__town = { ready: false };

if (!document.createElement('canvas').getContext('webgl2')) {
  document.getElementById('fallback').hidden = false;
  document.getElementById('loader').hidden = true;
  canvas.remove();
} else start().catch((e) => {
  console.error(e);
  document.getElementById('loader').textContent = 'The town could not be built. Reload to try again.';
  if (dev) globalThis.__town = { ready: true, error: String(e?.stack || e) };
});

async function start() {
  const kit = await import('./kit.js');
  const low = kit.QUALITY === 'low';
  const apps = await (await fetch(new URL('../apps.json', import.meta.url))).json();
  const world = createWorld();
  if (q.has('hello')) fetch(q.get('hello')).then((r) => r.json()).then(world.applyHello); // dev: a hello file, no relay
  else connect({ world, ...(dev && q.get('ws') ? { url: q.get('ws') } : {}) }); // dev: a relay on another port
  await world.loaded;

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: !low, powerPreference: 'high-performance' });
  renderer.setPixelRatio(Math.min(devicePixelRatio, low ? 1.25 : 2));
  renderer.shadowMap.enabled = !low;
  const scene = new THREE.Scene();
  const bg = new THREE.Color(kit.colors.bg);
  scene.background = bg;
  scene.fog = new THREE.Fog(bg, 70, 160);
  const hemi = new THREE.HemisphereLight(0xffffff, 0xd9cfc4, 1.6);
  const sun = new THREE.DirectionalLight(0xfff4e6, 2.4);
  sun.castShadow = !low;
  sun.shadow.mapSize.set(4096, 4096);
  Object.assign(sun.shadow.camera, { left: -30, right: 30, top: 30, bottom: -30, near: 1, far: 140 });
  sun.shadow.bias = -0.0004; sun.shadow.normalBias = 0.03;
  const moon = new THREE.DirectionalLight(0xa9bbff, 0);
  scene.add(hemi, sun, sun.target, moon);

  const camera = new THREE.PerspectiveCamera(38, 1, 0.5, 400);
  const controls = new OrbitControls(camera, canvas);
  Object.assign(controls, { enableDamping: true, dampingFactor: 0.08, minDistance: 6, maxDistance: 120, maxPolarAngle: 1.35, screenSpacePanning: false });

  // ---- the town
  const ctx = { THREE, kit, world, apps };
  const [islands, bridges, buildings, marks] = await Promise.all(['islands', 'bridges', 'buildings', 'marks']
    .map(async (n) => (await import(`./objects/${n}.js`)).default(ctx)));
  for (const o of [islands, bridges, buildings, marks]) scene.add(o.root);
  const labels = createLabels({ THREE, layer: document.getElementById('labels'), apps, lang });

  // ---- camera: overview of the three islands, or one island / building up close
  const centres = islands.islands.map((i) => i.centre);
  const mid = centres.reduce((a, c) => [a[0] + c[0] / 3, a[1] + c[1] / 3], [0, 0]);
  const span = 0.85 * (Math.max(...centres.map((c) => Math.hypot(c[0] - mid[0], c[1] - mid[1]))) + 2 * world.town.radius);
  let fly = null;
  function view(target, radius) {
    // distance so a sphere of `radius` fits the narrower screen axis, seen from the front-high angle
    const fov = THREE.MathUtils.degToRad(camera.fov), aspect = camera.aspect;
    const half = Math.min(fov / 2, Math.atan(Math.tan(fov / 2) * aspect));
    const d = radius / Math.sin(half);
    const dir = new THREE.Vector3(0.15, 0.85, 0.85).normalize(); // front-high: the islands read as a map, buildings still stand up
    return { pos: target.clone().addScaledVector(dir, d), target };
  }
  const overview = () => view(new THREE.Vector3(mid[0], 0, mid[1] + 3), span); // a little forward: the dock covers the bottom
  function flyTo(v, dur = 1.2) {
    fly = { from: camera.position.clone(), fromT: controls.target.clone(), to: v.pos, toT: v.target, t0: performance.now(), dur: dur * 1000 };
    overviewBtn.hidden = v.home === true;
  }
  const overviewBtn = document.getElementById('overview');
  overviewBtn.onclick = () => { closeCard(); flyTo({ ...overview(), home: true }); };

  // ---- picking: a tap (not a drag) picks a building, else an island, else flies home
  const ray = new THREE.Raycaster(), ndc = new THREE.Vector2();
  let down = null;
  canvas.addEventListener('pointerdown', (e) => { down = [e.clientX, e.clientY]; });
  canvas.addEventListener('pointerup', (e) => {
    if (!down || Math.hypot(e.clientX - down[0], e.clientY - down[1]) > 8) return;
    down = null;
    const r = canvas.getBoundingClientRect();
    ndc.set(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    pick(ray);
  });
  function pick(r) {
    const hb = r.intersectObjects(buildings.pickables(), false)[0];
    const p = hb && buildings.plotOf(hb);
    if (p) { openCard(p); return flyTo(view(new THREE.Vector3(p.x, 0.6, p.z), 4.5)); }
    const hi = r.intersectObjects(islands.pickables, false)[0];
    if (hi) {
      closeCard();
      const c = islands.islands.find((i) => i.host === hi.object.userData.pick.host).centre;
      return flyTo(view(new THREE.Vector3(c[0], 0, c[1]), 2 * world.town.radius + 1.5));
    }
    closeCard();
    flyTo({ ...overview(), home: true });
  }

  // ---- card + panel
  const card = document.getElementById('card'), fr = () => lang() === 'fr';
  let carded = null;
  function openCard(p) { carded = p.app; card.hidden = false; fillCard(); }
  function closeCard() { carded = null; card.hidden = true; }
  document.getElementById('cardclose').onclick = closeCard;
  const healthText = { Healthy: ['running', 'en marche'], Progressing: ['deploying', 'en déploiement'], Degraded: ['degraded', 'dégradé'],
    Missing: ['missing', 'absent'], Suspended: ['suspended', 'suspendu'], Unknown: ['unknown', 'inconnu'] };
  function fillCard() {
    const p = buildings.plots().find((x) => x.app === carded);
    if (!p) return closeCard();
    const a = apps[p.app] || {}, s = world.state, F = fr();
    card.style.setProperty('--roof', `#${new THREE.Color(p.roof).getHexString()}`);
    card.querySelector('.kicker').textContent = `${a.group === 'service' ? (F ? 'service' : 'service') : (F ? 'application' : 'app')} · ${F ? 'hôte' : 'host'} ${p.host}`;
    card.querySelector('h2').textContent = a.name || p.app;
    card.querySelector('.desc').textContent = a[lang()] || a.en || '';
    const st = [];
    if (p.ruinAt) st.push(F ? 'Retirée du cluster.' : 'Removed from the cluster.');
    else {
      const h = s.health[p.app];
      if (h) st.push(`${healthText[h]?.[F ? 1 : 0] || h}`);
      if (s.builds[p.app]) st.push(F ? 'une construction en cours' : 'a build in progress');
      const n = s.alerts.filter((x) => x.app === p.app).length;
      if (n) st.push(F ? `${n} alerte${n > 1 ? 's' : ''}` : `${n} alert${n > 1 ? 's' : ''}`);
      if (p.down) st.push(F ? `l’hôte ${p.host} ne répond plus` : `host ${p.host} is down`);
    }
    card.querySelector('.state').textContent = st.join(' · ');
    const go = card.querySelector('.go');
    go.hidden = !a.url || !!p.ruinAt;
    if (a.url) { go.href = a.url; go.textContent = `${F ? 'Ouvrir' : 'Open'} ${a.name} ↗`; }
  }
  const line = document.getElementById('line'), facts = document.getElementById('facts'), staleEl = document.getElementById('stale');
  function fillPanel() {
    const F = fr(), s = world.state, n = s.plots.filter((p) => !p.ruinAt).length;
    line.textContent = F ? `Le cluster ukubi en direct : ${n} bâtiments sur 3 îles, une par hôte.` : `The ukubi cluster, live: ${n} buildings on 3 islands, one per host.`;
    const hosts = world.town.islands.map((i) => `${i.host} ${world.ready[i.host] === false ? '✗' : '✓'}`).join('  ');
    const al = s.alerts.length;
    facts.textContent = `${hosts}${al ? (F ? ` · ${al} alerte${al > 1 ? 's' : ''}` : ` · ${al} alert${al > 1 ? 's' : ''}`) : ''}`;
    staleEl.hidden = world.stale == null;
    if (world.stale != null) staleEl.textContent = `${F ? 'figé depuis' : 'stale since'} ${new Date(world.stale).toLocaleTimeString(F ? 'fr' : 'en', { hour: '2-digit', minute: '2-digit' })}`;
    if (carded) fillCard();
  }
  world.on('state', fillPanel); world.on('metrics', fillPanel); world.on('hello', fillPanel);
  fillPanel();

  // ---- sky: the real sun over Paris → day, dusk, night
  const day = { bg: new THREE.Color(kit.colors.bg), hemi: low ? 1.1 : 1.6, sun: low ? 3.2 : 2.4 };
  const dark = { bg: new THREE.Color(0x1a2240), hemi: 0.85, sun: 0 };
  const dusk = new THREE.Color(0xf2c9a0);
  let night = 0, skyAt = 0, pinned = null;
  function sky(ms) {
    const s = sunAt(ms), el = s.elev;
    night = kit.clamp01((4 - el) / 12); // +4° → day, −8° → night
    const glow = kit.clamp01(1 - Math.abs(el - 1) / 7) * (1 - night * 0.6); // a warm band around sunset/sunrise
    bg.copy(day.bg).lerp(dusk, glow * 0.55).lerp(dark.bg, night);
    scene.fog.color.copy(bg);
    hemi.intensity = THREE.MathUtils.lerp(day.hemi, dark.hemi, night);
    sun.intensity = THREE.MathUtils.lerp(day.sun, 0.25, night);
    sun.color.set(0xfff4e6).lerp(new THREE.Color(0xffb27a), glow);
    moon.intensity = 0.9 * night;
    // light from the sun's real direction (south = −z, so the day arc crosses the default view), never from below
    const az = THREE.MathUtils.degToRad(s.az), e = THREE.MathUtils.degToRad(Math.max(el, 12));
    sun.position.set(mid[0] - 60 * Math.cos(e) * Math.sin(az), 60 * Math.sin(e), mid[1] + 60 * Math.cos(e) * Math.cos(az));
    sun.target.position.set(mid[0], 0, mid[1]);
    moon.position.set(mid[0] + 30, 50, mid[1] + 40);
    document.body.classList.toggle('night', night > 0.5);
  }

  // ---- loop
  let w = 0, h = 0, last = performance.now(), t = 0, drawn = 0;
  const fps = low ? 30 : 60;
  function resize() {
    if (canvas.clientWidth === w && canvas.clientHeight === h) return;
    w = canvas.clientWidth; h = canvas.clientHeight;
    renderer.setSize(w, h, false);
    camera.aspect = w / h; camera.updateProjectionMatrix();
  }
  resize();
  { const v = overview(); camera.position.copy(v.pos); controls.target.copy(v.target); controls.update(); }
  sky(pinned ?? clock());
  document.getElementById('loader').hidden = true;
  renderer.setAnimationLoop((nowMs) => {
    if (nowMs - drawn < 1000 / fps - 2) return;
    drawn = nowMs;
    const dt = Math.min(0.1, (nowMs - last) / 1000); last = nowMs; t += dt;
    resize();
    if (nowMs - skyAt > 30000) { skyAt = nowMs; sky(pinned ?? clock()); }
    if (fly) {
      const f = Math.min(1, (nowMs - fly.t0) / fly.dur), k = kit.easeInOut(f);
      camera.position.lerpVectors(fly.from, fly.to, k);
      controls.target.lerpVectors(fly.fromT, fly.toT, k);
      if (f >= 1) fly = null;
    }
    controls.update();
    { const d = camera.position.distanceTo(controls.target); scene.fog.near = d + 25 + 200 * night; scene.fog.far = d + 110 + 400 * night; }
    islands.update(); bridges.update(dt); marks.update(night);
    buildings.update(dt, t);
    labels.update(buildings.plots(), camera, w, h);
    renderer.render(scene, camera);
  });

  if (dev) globalThis.__town = {
    ready: true, world, scene, renderer, camera, controls, buildings,
    inject: (f) => world.applyFrame(f),
    sunAt: (iso) => { pinned = Date.parse(iso); skyAt = 0; sky(pinned); },
    fly: (what) => {
      const p = buildings.plots().find((x) => x.app === what);
      if (p) { openCard(p); flyTo(view(new THREE.Vector3(p.x, 0.6, p.z), 4.5), 0.01); return; }
      const is = islands.islands.find((i) => i.host === what);
      if (is) return flyTo(view(new THREE.Vector3(is.centre[0], 0, is.centre[1]), 2 * world.town.radius + 1.5), 0.01);
      closeCard(); flyTo({ ...overview(), home: true }, 0.01);
    },
    info: () => ({ draws: renderer.info.render.calls, tris: renderer.info.render.triangles, plots: buildings.plots().length }),
  };
}

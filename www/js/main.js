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
import { createToys } from './toys.js';

const canvas = document.getElementById('c');
const q = new URLSearchParams(location.search);
const dev = location.hostname === '127.0.0.1';
const lang = () => (q.get('lang') || navigator.language || 'en').slice(0, 2) === 'fr' ? 'fr' : 'en';
const coarse = matchMedia('(pointer: coarse)').matches;
globalThis.__quality = q.get('q') || (coarse && ((navigator.deviceMemory || 8) <= 4 || (navigator.hardwareConcurrency || 8) <= 4) ? 'low' : 'high');
if (dev) globalThis.__town = { ready: false };
const RETRY = 'town:reloaded';
const store = (f) => { try { return f(sessionStorage); } catch { return null; } };

if (!document.createElement('canvas').getContext('webgl2')) {
  document.getElementById('fallback').hidden = false;
  document.getElementById('loader').hidden = true;
  canvas.remove();
} else {
  start().then(() => store((s) => s.removeItem(RETRY)), failed);
  // a model fetched after the start 404s (buildings.js): a deploy retired this page's /v/<build>/ while it was open
  globalThis.addEventListener('town:retired', () => { if (!dev) failed(new Error('this build was retired')); }, { once: true });
}

function failed(e) {
  console.error(e);
  if (dev) globalThis.__town = { ready: true, error: String(e?.stack || e) };
  // A failed load (a deploy retired this page's /v/<build>/ modules, a flaky fetch) reloads once onto the current
  // index; the sessionStorage flag stops a loop and is cleared once a load succeeds.
  const loader = document.getElementById('loader');
  loader.hidden = false;
  if (dev || store((s) => s.getItem(RETRY)) || store((s) => (s.setItem(RETRY, '1'), true)) !== true) {
    loader.textContent = 'The town could not be built. Reload to try again.';
    return;
  }
  loader.textContent = 'Could not load, retrying…';
  setTimeout(() => location.reload(), 1000);
}

async function start() {
  const kit = await import('./kit.js');
  const low = kit.QUALITY === 'low';
  const apps = await (await fetch(new URL('../apps.json', import.meta.url))).json();
  const world = createWorld();
  if (q.has('hello')) fetch(q.get('hello')).then((r) => r.json()).then(world.applyHello); // dev: a hello file, no relay
  else connect({ world, ...(dev && q.get('ws') ? { url: q.get('ws') } : {}) }); // dev: a relay on another port
  const slow = setTimeout(() => { // no hello yet: say so in words (the socket keeps retrying)
    document.getElementById('loader').textContent = lang() === 'fr'
      ? 'Impossible de joindre la ville pour l’instant, nouvel essai… (trop d’onglets ouverts sur ce réseau ?)'
      : 'Can’t reach the town yet, retrying… (too many tabs open on this network?)';
  }, 8000);
  await world.loaded;
  // R8 build guard: a hello from another build means a deploy happened while this page was open (a reconnect after
  // the relay restarted). Its town may not be the one this code draws: reload once onto the current index.
  const mine = import.meta.url.match(/\/v\/([0-9a-f]{12})\//)?.[1];
  const guard = (h) => { if (mine && h.build && h.build !== mine) failed(new Error(`build ${mine} → ${h.build}`)); };
  guard({ build: world.build });
  world.on('hello', guard);
  clearTimeout(slow);

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: !low, powerPreference: 'high-performance' });
  renderer.setPixelRatio(Math.min(devicePixelRatio, low ? 1.25 : 2));
  renderer.shadowMap.enabled = !low;
  const scene = new THREE.Scene();
  const bg = new THREE.Color(kit.ukubi.office.surface);
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
  const [islands, bridges, buildings, marks, people] = await Promise.all(['islands', 'bridges', 'buildings', 'marks', 'people']
    .map(async (n) => (await import(`./objects/${n}.js`)).default(ctx)));
  for (const o of [islands, bridges, buildings, marks, people]) scene.add(o.root);
  const labels = createLabels({ THREE, layer: document.getElementById('labels'), apps, lang });
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---- camera: overview of the three islands, or one island / building up close
  const centres = islands.islands.map((i) => i.centre);
  const mid = centres.reduce((a, c) => [a[0] + c[0] / 3, a[1] + c[1] / 3], [0, 0]);
  // R8: the shadow camera fits the town (the islands' rims around mid, plus a margin for the tallest buildings)
  const townR = Math.max(...islands.islands.map((i) => Math.hypot(i.centre[0] - mid[0], i.centre[1] - mid[1]) + 2 * i.radius + 2));
  Object.assign(sun.shadow.camera, { left: -townR, right: townR, top: townR, bottom: -townR, far: 60 + 2 * townR });
  sun.shadow.camera.updateProjectionMatrix();
  // R8: the dock (panel or card) covers the bottom of the canvas. The projection centre sits in the middle of the free
  // area above it (setViewOffset), so every view, the fly-ins too, centres above the dock, never behind it; and the
  // overview is fitted: the distance at which every island's rim (tiles, building tops, the cone's tip) is on screen.
  const dock = document.getElementById('dock'), Y = new THREE.Vector3(0, 1, 0), V = new THREE.Vector3();
  const FRONT = new THREE.Vector3(0.15, 0.85, 0.85).normalize(); // front-high: the islands read as a map, buildings still stand up
  const rim = islands.islands.flatMap((is) => {
    const [cx, cz] = is.centre, R = 2 * is.radius + 0.4, pts = [new THREE.Vector3(cx, -7.5 * is.radius / 4, cz)];
    for (let i = 0; i < 12; i++) for (const y of [0, 2.6]) pts.push(new THREE.Vector3(cx + R * Math.cos(i * Math.PI / 6), y, cz + R * Math.sin(i * Math.PI / 6)));
    return pts;
  });
  let free = 1, dockBox = null; // the fraction of the canvas height above the dock; the dock in canvas px (labels)
  function lens() {
    const dr = dock.getBoundingClientRect(), cr = canvas.getBoundingClientRect(), top = dr.top - cr.top;
    dockBox = [dr.left - cr.left, top, dr.right - cr.left, dr.bottom - cr.top];
    free = Math.min(1, Math.max(0.45, (top - 8) / h));
    const u = free * h, H = 2 * h - u; // a virtual image H tall whose centre is the free area's: the canvas is its bottom h
    camera.aspect = w / H;
    camera.setViewOffset(w, H, 0, h - u, w, h);
    camera.updateProjectionMatrix();
  }
  let fly = null, home = true, moved = false;
  function view(target, radius) {
    // distance so a sphere of `radius` fits the free area's narrower axis
    const t = Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2), H = 2 * h - free * h;
    const half = Math.atan(t * Math.min(free * h, w) / H);
    return { pos: target.clone().addScaledVector(FRONT, radius / Math.sin(half)), target };
  }
  // portrait: a steeper look (the depth axis foreshortens least) from whichever side lets the town come closest
  const STEEP = new THREE.Vector3(0.1, 0.92, 0.5).normalize();
  const PORTRAIT = [FRONT, STEEP].flatMap((d) => [0, Math.PI / 2, -Math.PI / 2].map((a) => d.clone().applyAxisAngle(Y, a)));
  function overview() {
    const target = new THREE.Vector3(mid[0], 0, mid[1]), cam = camera.clone();
    const fit = (dir) => { // the closest distance at which the whole town is on screen above the dock (binary search)
      const fits = (d) => {
        cam.position.copy(target).addScaledVector(dir, d); cam.lookAt(target); cam.updateMatrixWorld();
        return rim.every((p) => (V.copy(p).project(cam), V.z < 1 && Math.abs(V.x) <= 0.95 && V.y <= 0.95 && V.y >= 1.03 - 2 * free));
      };
      let lo = 5, hi = 400;
      for (let i = 0; i < 24; i++) { const m = (lo + hi) / 2; if (fits(m)) hi = m; else lo = m; }
      return hi;
    };
    let best = { dir: FRONT, d: fit(FRONT) };
    if (w < 0.8 * h) for (const dir of PORTRAIT) { const d = fit(dir); if (d < best.d - 0.5) best = { dir, d }; }
    controls.maxDistance = Math.max(120, 1.3 * best.d);
    return { pos: target.clone().addScaledVector(best.dir, best.d), target, home: true };
  }
  function flyTo(v, dur = reduced ? 0.01 : 1.2) {
    fly = { from: camera.position.clone(), fromT: controls.target.clone(), to: v.pos, toT: v.target, t0: performance.now(), dur: dur * 1000 };
    home = v.home === true;
    if (home) moved = false;
    overviewBtn.hidden = home;
  }
  controls.addEventListener('start', () => { moved = true; overviewBtn.hidden = false; });
  // the dock grows or shrinks (a card opens, the panel's text wraps): re-centre; at home and untouched, refit
  function refit() {
    lens();
    if (home && !moved && !fly) { const v = overview(); camera.position.copy(v.pos); controls.target.copy(v.target); }
  }
  new ResizeObserver(() => { if (w) refit(); }).observe(dock);
  const overviewBtn = document.getElementById('overview');
  overviewBtn.onclick = () => { closeCard(); flyTo(overview()); };

  // ---- picking: a tap (not a drag) picks a building, else an island, else flies home
  const ray = new THREE.Raycaster(), ndc = new THREE.Vector2();
  // a tap = one pointer that went down and up in place; a gesture that ever had two (a pinch) is never a tap
  const pointers = new Map();
  let multi = false;
  canvas.addEventListener('pointerdown', (e) => {
    pointers.set(e.pointerId, [e.clientX, e.clientY]);
    if (pointers.size > 1) multi = true;
  });
  const lift = (e) => { pointers.delete(e.pointerId); if (!pointers.size) setTimeout(() => { if (!pointers.size) multi = false; }); };
  canvas.addEventListener('pointercancel', lift);
  canvas.addEventListener('pointerup', (e) => {
    const down = pointers.get(e.pointerId), wasMulti = multi;
    lift(e);
    if (!down || wasMulti || Math.hypot(e.clientX - down[0], e.clientY - down[1]) > 8) return;
    if (toys.tap(e)) return; // plant / remove mode takes the tap
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
      const is = islands.islands.find((i) => i.host === hi.object.userData.pick.host);
      return flyTo(view(new THREE.Vector3(is.centre[0], 0, is.centre[1]), 2 * is.radius + 1.5));
    }
    closeCard();
    flyTo(overview());
  }

  // ---- card + panel
  const card = document.getElementById('card'), fr = () => lang() === 'fr';
  let carded = null;
  function openCard(p) { carded = p.app; card.hidden = false; fillCard(); }
  function closeCard() { carded = null; card.hidden = true; }
  document.getElementById('cardclose').onclick = closeCard;
  // Status always comes with a word (Ukubi): the colour only repeats it.
  const healthText = { Healthy: ['running', 'en marche', 'ok'], Progressing: ['deploying', 'en déploiement', 'warn'], Degraded: ['degraded', 'dégradé', 'bad'],
    Missing: ['missing', 'absent', 'bad'], Suspended: ['suspended', 'suspendu', 'info'], Unknown: ['unknown', 'inconnu', 'info'] };
  const word = (cls, text) => Object.assign(document.createElement('span'), { className: cls, textContent: text });
  function fillCard() {
    const p = buildings.plots().find((x) => x.app === carded);
    if (!p) return closeCard();
    const a = apps[p.app] || {}, s = world.state, F = fr();
    card.style.setProperty('--wire', `var(--wire-${p.wire})`);
    card.querySelector('.kicker').textContent = `${a.group === 'service' ? 'service' : (F ? 'application' : 'app')} · ${F ? 'hôte' : 'host'} ${p.host}`;
    card.querySelector('h2').textContent = a.name || p.app;
    card.querySelector('.desc').textContent = a[lang()] || a.en || '';
    const st = [];
    if (p.ruinAt) st.push(word('bad', F ? 'retirée du cluster' : 'removed from the cluster'));
    else {
      const h = healthText[s.health[p.app]];
      if (h) st.push(word(h[2], h[F ? 1 : 0]));
      if (s.builds[p.app]) st.push(word('warn', F ? 'construction en cours' : 'build in progress'));
      const n = s.alerts.filter((x) => x.app === p.app).length;
      if (n) st.push(word('bad', F ? `${n} alerte${n > 1 ? 's' : ''}` : `${n} alert${n > 1 ? 's' : ''}`));
      if (p.down) st.push(word('bad', F ? `hôte ${p.host} hors ligne` : `host ${p.host} down`));
    }
    card.querySelector('.state').replaceChildren(...st);
    const go = card.querySelector('.go');
    go.hidden = !a.url || !!p.ruinAt;
    if (a.url) { go.href = a.url; go.textContent = `${F ? 'Ouvrir' : 'Open'} ${a.name}`; }
  }
  const line = document.getElementById('line'), facts = document.getElementById('facts'), staleEl = document.getElementById('stale');
  const led = Object.assign(document.createElement('button'), { type: 'button' }), ledText = document.createElement('span');
  led.append(Object.assign(document.createElement('span'), { className: 'led' }), ledText);
  led.onclick = () => world.state.alerts[0] && goAlert(world.state.alerts[0]);
  function fillPanel() {
    const F = fr(), s = world.state, n = s.plots.filter((p) => !p.ruinAt).length;
    line.textContent = F ? `Le cluster ukubi en direct : ${n} bâtiments sur 3 îles.` : `The ukubi cluster, live: ${n} buildings on 3 islands.`;
    const parts = world.town.islands.map((i) => world.ready[i.host] === false ? word('bad', `${i.host} ${F ? 'hors ligne' : 'down'}`) : word('ok', `${i.host} ${F ? 'en ligne' : 'up'}`));
    const al = s.alerts.length;
    if (al) { // the LED: lit only while something fires; it takes you there (one button, updated in place: taps land)
      ledText.textContent = F ? `${al} alerte${al > 1 ? 's' : ''}` : `${al} alert${al > 1 ? 's' : ''}`;
      parts.push(led);
    }
    facts.replaceChildren(...parts);
    staleEl.hidden = world.stale == null;
    if (world.stale != null) staleEl.textContent = `${F ? 'Figé depuis' : 'Stale since'} ${new Date(world.stale).toLocaleTimeString(F ? 'fr' : 'en', { hour: '2-digit', minute: '2-digit' })}`;
    if (carded) fillCard();
  }
  function goAlert(al) {
    const p = al.app && buildings.plots().find((x) => x.app === al.app);
    if (p) { openCard(p); return flyTo(view(new THREE.Vector3(p.x, 0.6, p.z), 4.5)); }
    const host = al.host || buildings.plots().find((x) => x.role === 'watchtower')?.host || 'h1';
    const is = islands.islands.find((i) => i.host === host);
    flyTo(view(new THREE.Vector3(is.centre[0], 0, is.centre[1]), 2 * is.radius + 1.5));
  }
  world.on('state', fillPanel); world.on('metrics', fillPanel); world.on('hello', fillPanel);
  const toys = createToys({ THREE, kit, world, canvas, camera, lang });
  scene.add(toys.aim);
  fillPanel();

  // ---- sky: the real sun over Paris → day, dusk, night
  // Ukubi: the Office surface by day, the Underground surface at night (with the Glow theme: the wires light up)
  const day = { bg: new THREE.Color(kit.ukubi.office.surface), hemi: low ? 1.1 : 1.6, sun: low ? 3.2 : 2.4 };
  const dark = { bg: new THREE.Color(kit.ukubi.glow.surface), hemi: 0.85, sun: 0 };
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
    document.body.classList.toggle('glow', night > 0.5);
  }

  // ---- loop
  let w = 0, h = 0, last = performance.now(), t = 0, drawn = 0;
  const fps = low ? 30 : 60;
  function resize() {
    if (canvas.clientWidth === w && canvas.clientHeight === h) return;
    w = canvas.clientWidth; h = canvas.clientHeight;
    renderer.setSize(w, h, false);
    refit();
  }
  resize();
  controls.update();
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
    islands.update(); bridges.update(dt, night); marks.update(night);
    buildings.update(dt, t, night);
    people.update(t);
    toys.tick();
    world.step(dt);
    labels.update(buildings.plots(), camera, w, h, {
      obstacle: dockBox,
      chips: w < 600 && home && !moved && !fly ? islands.islands : null, // the phone's overview: one chip per island
    });
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
      if (is) return flyTo(view(new THREE.Vector3(is.centre[0], 0, is.centre[1]), 2 * is.radius + 1.5), 0.01);
      closeCard(); flyTo(overview(), 0.01);
    },
    info: () => ({ draws: renderer.info.render.calls, tris: renderer.info.render.triangles, plots: buildings.plots().length }),
  };
}

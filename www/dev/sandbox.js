// Sandbox body (see sandbox.html). Minimal W1a seam version; lane B adds the controls
// (P slider, play/step, params, injectors, heatmaps, bus log, tier toggle) in this folder.
const q = new URLSearchParams(location.search);
if (q.get('q')) globalThis.__quality = q.get('q');
window.__gate = { ready: false };

try {
  const THREE = await import('three');
  const kit = await import('../js/kit.js');
  const { createWorld, encodeI16, BUS } = await import('../js/world.js');
  const { profiles, finale } = await import('../js/layout.js');
  const { makeCtx, checkModel, wire } = await import('../js/contract.js');

  const names = (q.get('m') || 'fixture:cube').split(',');
  const renderer = new THREE.WebGLRenderer({ canvas: document.getElementById('c'), antialias: true });
  renderer.setPixelRatio(1);
  renderer.setSize(innerWidth, innerHeight, false);
  renderer.info.autoReset = false;
  const scene = new THREE.Scene();
  kit.rig(renderer, scene);
  kit.setKey(q.get('key') || 'dusk');

  const world = createWorld({ dev: true });
  world.applyHello(await (await fetch(q.get('snapshot') || './fixtures/hello.json')).json());
  kit.U.uHMax.value = world.hmax;
  kit.U.uSnowline.value = world.snowline;

  const loaded = [];
  for (const name of names) {
    const fx = name.startsWith('fixture:') ? name.slice(8) : null;
    const profile = fx ? profiles[`_${fx}`] || profiles._test : profiles[name];
    if (!profile) throw new Error(`no layout profile for ${name}`);
    const mod = await import(fx ? `./fixtures/${fx}.js` : `../models/${name}/index.js`);
    const model = await mod.default(makeCtx({ THREE, kit, world, name: fx ? '_test' : name, profile }));
    const problems = checkModel(model, profile);
    if (problems.length) throw new Error(`${name}: ${problems.join('; ')}`);
    wire(model, world);
    scene.add(model.root);
    loaded.push({ name, fx, mod, model, profile });
  }

  // Framing comes from the profile only, so a missing or misplaced model shows up blank or cut off.
  const { profile } = loaded[0], { x, z, r } = profile.footprint, [y0, y1] = profile.yRange;
  const camera = new THREE.PerspectiveCamera(finale.fov, innerWidth / innerHeight, 0.1, 2000);
  const cam = q.get('cam') || '';
  if (cam.startsWith('cave')) { // cave close-up (09 Freeze item 1): cam=cave:<i>[:side]
    const [, i = 0, side = 1] = cam.split(':').map(Number), c = world.caves[i % world.caves.length], o = new THREE.Vector3(Math.sin(c.yaw), 0, Math.cos(c.yaw));
    const P = new THREE.Vector3(c.x - o.x * 4, c.y + 2.4, c.z - o.z * 4);
    camera.position.copy(P).addScaledVector(o, 22).add(new THREE.Vector3(0, 5, 0)).addScaledVector(new THREE.Vector3(-o.z, 0, o.x), 10 * side);
    camera.lookAt(P);
  } else if (r > 50) { // world-sized: the live finale camera at its sway angle (?az=)
    const az = Number(q.get('az') || 0);
    camera.position.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
    camera.lookAt(...finale.look);
  } else {
    const yMid = (y0 + y1) / 2, d = Math.max(r, (y1 - y0) / 2) * 3.4 + 2;
    camera.position.set(x + d * 0.45, yMid + d * 0.55, z + d * 0.8);
    camera.lookAt(x, yMid, z);
  }

  const gl = renderer.getContext(), W = gl.drawingBufferWidth, H = gl.drawingBufferHeight;
  const render = (buf) => {
    renderer.info.reset();
    renderer.render(scene, camera);
    if (buf) gl.readPixels(0, 0, W, H, gl.RGBA, gl.UNSIGNED_BYTE, buf);
    return { tris: renderer.info.render.triangles, draws: renderer.info.render.calls };
  };
  // baseline: rig only (dome + haze floor), so stats count the models alone
  const base = new Uint8Array(W * H * 4), buf = new Uint8Array(W * H * 4);
  loaded.forEach((l) => (l.model.root.visible = false));
  const b0 = render(base);
  loaded.forEach((l) => (l.model.root.visible = true));

  function stats(P) {
    for (const l of loaded) l.model.pose(P);
    const s = render(buf);
    let hit = 0;
    for (let i = 0; i < buf.length; i += 4) if (Math.abs(buf[i] - base[i]) + Math.abs(buf[i + 1] - base[i + 1]) + Math.abs(buf[i + 2] - base[i + 2]) > 18) hit++;
    const foreign = new Set(), shadows = new Set();
    for (const l of loaded) l.model.root.traverse((o) => {
      if (!o.isMesh && !o.isPoints && !o.isLine && !o.isSprite) return;
      for (const m of [].concat(o.material)) if (!m?.userData?.kit) foreign.add(`${o.name || o.type}:${m?.type}`);
      if (o.castShadow || o.receiveShadow) shadows.add(o.name || o.type);
    });
    return { ready: true, models: names, P, tris: s.tris - b0.tris, draws: s.draws - b0.draws, coverage: hit / (W * H), foreignMaterials: [...foreign], shadows: [...shadows], profile };
  }

  let n = 0;
  const inject = (type, p = {}) => {
    if (type.startsWith('ev:')) return world.applyFrame({ t: 'ev', id: p.id || `inject-${++n}`, ts: Date.now(), kind: type.slice(3), ...p });
    if (type.startsWith('world:')) return world.applyFrame({ t: 'world', kind: type.slice(6), ...p, ...(Array.isArray(p.cells) ? { cells: encodeI16(p.cells) } : {}) });
    world.emit(type, p, BUS[type]?.[0]);
  };
  let t = 0;
  const step = (k = 1, dt = 1 / 30) => { for (let i = 0; i < k; i++) { world.step(dt); for (const l of loaded) l.model.step(dt, (t += dt)); } };

  window.__inject = inject;
  window.__step = step;
  window.__setP = (P) => (window.__gate = { ...stats(P), hasTest: loaded.some((l) => !l.fx || l.mod.test) });
  window.__test = async () => {
    try {
      for (const l of loaded) {
        const test = l.fx ? l.mod.test : (await import(`../models/${l.name}/test.js`)).default;
        if (!test) continue;
        const fixture = l.fx ? null : await (await fetch(`../models/${l.name}/fixture.json`)).json();
        await test({ model: l.model, world, inject, step, fixture, assert: (c, m) => { if (!c) throw new Error(`assert: ${m}`); } });
      }
      return { ok: true };
    } catch (e) {
      return { ok: false, error: String(e?.stack || e) };
    }
  };
  window.__setP(Number(q.get('P') ?? 6));
} catch (e) {
  console.error(e);
  window.__gate = { ready: true, error: String(e?.stack || e) };
}

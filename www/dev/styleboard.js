// Styleboard (09): the models of layout.page on the kit rig, 4 time keys (rows) × 4 orbit angles (columns),
// one canvas, scissored viewports. Models not built yet are listed as missing; with none, the Go h.base
// terrain fixture stands in so the rig is still judged on the real kit render. ?q=low  Hook: __board.
const q = new URLSearchParams(location.search);
if (q.get('q')) globalThis.__quality = q.get('q');
window.__board = { ready: false };
try {
  const THREE = await import('three');
  const kit = await import('../js/kit.js');
  const { createWorld } = await import('../js/world.js');
  const { profiles, page, finale } = await import('../js/layout.js');
  const { makeCtx, checkModel, wire } = await import('../js/contract.js');

  const canvas = document.getElementById('c');
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(1);
  renderer.setSize(innerWidth, innerHeight, false);
  const scene = new THREE.Scene();
  kit.rig(renderer, scene);
  const world = createWorld({ dev: true });
  world.applyHello(await (await fetch('./fixtures/hello.json')).json());
  kit.U.uHMax.value = world.hmax;
  kit.U.uSnowline.value = world.snowline;

  const models = [], missing = [];
  for (const name of page) {
    let mod;
    try { mod = await import(`../models/${name}/index.js`); } catch { missing.push(name); continue; }
    const m = await mod.default(makeCtx({ THREE, kit, world, name, profile: profiles[name] }));
    const bad = checkModel(m, profiles[name]);
    if (bad.length) throw new Error(`${name}: ${bad.join('; ')}`);
    wire(m, world);
    m.pose(6);
    scene.add(m.root);
    models.push(name);
  }
  if (!models.length) {
    const m = await (await import('./fixtures/terrain.js')).default(makeCtx({ THREE, kit, world, name: '_test', profile: profiles._terrain }));
    scene.add(m.root);
    models.push('fixture:terrain');
  }

  const W = canvas.width, H = canvas.height, cw = W / 4, ch = H / 4;
  const camera = new THREE.PerspectiveCamera(finale.fov, cw / ch, 1, 2000);
  const labels = document.getElementById('labels');
  renderer.setScissorTest(true);
  kit.KEYS.forEach((key, row) => {
    kit.setKey(key);
    for (let col = 0; col < 4; col++) {
      const az = (col * Math.PI) / 2;
      camera.position.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
      camera.lookAt(...finale.look);
      const y = H - (row + 1) * ch; // GL origin bottom-left; row 0 at the top
      renderer.setViewport(col * cw, y, cw, ch);
      renderer.setScissor(col * cw, y, cw, ch);
      renderer.render(scene, camera);
      labels.append(Object.assign(document.createElement('span'), { textContent: `${key} · ${col * 90}°` }));
    }
  });
  kit.setKey('dusk');
  document.getElementById('title').textContent = `${models.join(' + ')}${missing.length ? ` (missing: ${missing.join(', ')})` : ''}`;
  window.__board = { ready: true, models, missing };
} catch (e) {
  console.error(e);
  window.__board = { ready: true, error: String(e?.stack || e) };
}

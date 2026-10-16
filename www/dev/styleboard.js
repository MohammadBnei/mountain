// Styleboard (09): the models of page.js on the kit rig, 4 time keys (rows) × 4 orbit angles (columns),
// one canvas, scissored viewports. Models not built yet are listed as missing; with none, the Go h.base
// terrain fixture stands in so the rig is still judged on the real kit render. ?q=low  Hook: __board.
const q = new URLSearchParams(location.search);
if (q.get('q')) globalThis.__quality = q.get('q');
window.__board = { ready: false };
try {
  const THREE = await import('three');
  const kit = await import('../js/kit.js');
  const { createWorld } = await import('../js/world.js');
  const { profiles, finale } = await import('../js/layout.js');
  const { page } = await import('../js/page.js');
  const { makeCtx, checkModel, wire } = await import('../js/contract.js');
  const cut = await import('../js/cut.js');

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

  const models = [], missing = [], built = [];
  for (const name of page) {
    const url = `../models/${name}/index.js`; // only a 404 means not built yet; a broken model throws and fails the board
    if (!(await fetch(url, { method: 'HEAD' })).ok) { missing.push(name); continue; }
    const mod = await import(url);
    const m = await mod.default(makeCtx({ THREE, kit, world, name, profile: profiles[name] }));
    const bad = checkModel(m, profiles[name]);
    if (bad.length) throw new Error(`${name}: ${bad.join('; ')}`);
    wire(m, world);
    m.pose(6);
    scene.add(m.root);
    models.push(name);
    built.push(m);
  }
  cut.compose(built);
  cut.apply(world);
  if (!models.length) {
    const m = await (await import('./fixtures/terrain.js')).default(makeCtx({ THREE, kit, world, name: '_test', profile: profiles._terrain }));
    scene.add(m.root);
    models.push('fixture:terrain');
  }

  const W = canvas.width, H = canvas.height, cw = W / 4, ch = H / 4;
  const camera = new THREE.PerspectiveCamera(finale.fov, cw / ch, 1, 2000);
  const labels = document.getElementById('labels');
  renderer.setScissorTest(true);
  // Haze seam (09, R6 sea): the fogged sea must meet the dome with no hard horizon line. Down the left edge of each cell
  // (sky → sea): the largest step between neighbouring pixels beyond the steps 2 px either side, 0..255. A line is a lone
  // jump; the sea fading into the horizon is a steady ramp (5–6 /px in a 200 px cell) and scores ~0.
  const gl = renderer.getContext(), px = new Uint8Array(4 * ch), haze = {};
  const cell = (key, row, col) => {
    const az = (col * Math.PI) / 2;
    camera.position.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
    camera.lookAt(...finale.look);
    const y = H - (row + 1) * ch; // GL origin bottom-left; row 0 at the top
    renderer.setViewport(col * cw, y, cw, ch);
    renderer.setScissor(col * cw, y, cw, ch);
    renderer.render(scene, camera);
    gl.readPixels(col * cw + 2, y, 1, ch, gl.RGBA, gl.UNSIGNED_BYTE, px);
    const step = (i) => (i < 1 ? 0 : Math.max(...[0, 1, 2].map((c) => Math.abs(px[4 * i + c] - px[4 * i - 4 + c]))));
    for (let i = 1; i < Math.round(ch * 0.45); i++) haze[key] = Math.max(haze[key] || 0, step(i) - Math.max(step(i - 2), step(i + 2)));
  };
  // stale (kit.setStale(1)) dims the sky: it must still meet the fogged floor with no seam. Measured in cell 0,
  // which the key loop then repaints.
  kit.setKey('dusk'); kit.setStale(1); cell('dusk+stale', 0, 0); kit.setStale(0);
  kit.KEYS.forEach((key, row) => {
    kit.setKey(key);
    for (let col = 0; col < 4; col++) {
      cell(key, row, col);
      labels.append(Object.assign(document.createElement('span'), { textContent: `${key} · ${col * 90}°` }));
    }
  });
  kit.setKey('dusk');
  document.getElementById('title').textContent = `${models.join(' + ')}${missing.length ? ` (missing: ${missing.join(', ')})` : ''}`;
  window.__board = { ready: true, models, missing, haze };
} catch (e) {
  console.error(e);
  window.__board = { ready: true, error: String(e?.stack || e) };
}

// Sandbox body (see sandbox.html). Lane B: controls + every in-page gate measurement (04, 09 "Gate checks").
//
// Hooks (gate.mjs drives them; ?ui=0 hides the panel and stops the render loop):
//   __setP(P) → stats {tris, draws, coverage, foreignMaterials, shadows}      __step(n, dt)   __inject(type, payload)
//   __checks() → {pose, style:{dusk, day}, facet, lum:{dusk, day}, clip, physics}   (pure measurements, state restored)
//   __caveLum() → [lum dusk] per world.caves close-up      __world (the sandbox world)
//   __test() → {ok, error, peak, stepMs, writes, emits, listens, rig, fields}       (test.js under audit)
// Physics convention checked here (00 "Physics"): cannon-es collisionFilterGroup bit 1 = real, bit 2 = surface;
// two bodies of different layers in one world must not be able to collide (group & mask both ways).
const q = new URLSearchParams(location.search);
if (q.get('q')) globalThis.__quality = q.get('q');
window.__gate = { ready: false };

try {
  const THREE = await import('three');
  const CANNON = await import('cannon-es');
  const kit = await import('../js/kit.js');
  const { createWorld, encodeI16, BUS, EV_KINDS, WRITERS } = await import('../js/world.js');
  const { profiles, finale, FACET } = await import('../js/layout.js');
  const { makeCtx, checkModel, wire } = await import('../js/contract.js');

  let audit = null; // set by __test(); handlers can fire as soon as models are wired
  const bodies = []; // {world, body}
  const addBody = CANNON.World.prototype.addBody;
  CANNON.World.prototype.addBody = function (b) { bodies.push({ world: this, body: b }); return addBody.call(this, b); };

  const names = (q.get('m') || 'fixture:cube').split(',');
  const KEY = q.get('key') || 'dusk';
  const renderer = new THREE.WebGLRenderer({ canvas: document.getElementById('c'), antialias: true });
  renderer.setPixelRatio(1);
  renderer.setSize(innerWidth, innerHeight, false);
  renderer.info.autoReset = false;
  const scene = new THREE.Scene();
  const RIG = kit.rig(renderer, scene);
  kit.setKey(KEY);

  const world = createWorld({ dev: true });
  world.applyHello(await (await fetch(q.get('snapshot') || './fixtures/hello.json')).json());
  kit.U.uHMax.value = world.hmax;
  kit.U.uSnowline.value = world.snowline;

  const loaded = [];
  for (const name of names) {
    const fx = name.startsWith('fixture:') ? name.slice(8) : null;
    const mod = await import(fx ? `./fixtures/${fx}.js` : `../models/${name}/index.js`);
    // a fixture may export `profile` overrides on top of _test (dev-only, keeps fixture profiles out of layout.js)
    const profile = fx ? profiles[`_${fx}`] || { ...profiles._test, ...mod.profile } : profiles[name];
    if (!profile) throw new Error(`no layout profile for ${name}`);
    const rec = { writes: new Set(), emits: new Set(), listens: new Set() }, l = { name, fx, mod, profile, rec };
    const ctx = makeCtx({ THREE, kit, world, name: fx ? '_test' : name, profile });
    const v = ctx.world; // record what the model actually writes, emits and listens to; handlers run under the audit
    ctx.world = Object.create(v, {
      write: { value: (s, x) => (rec.writes.add(s), v.write(s, x)) },
      emit: { value: (t, p) => (rec.emits.add(t), v.emit(t, p)) },
      on: { value: (t, fn) => (rec.listens.add(t), v.on(t, (...a) => audited(l, `on ${t}`, () => fn(...a)))) },
    });
    const model = (l.model = await mod.default(ctx));
    const problems = checkModel(model, profile);
    if (problems.length) throw new Error(`${name}: ${problems.join('; ')}`);
    if (model.on) model.on = Object.fromEntries(Object.entries(model.on).map(([t, fn]) => [t, (...a) => audited(l, `on ${t}`, () => fn(...a))]));
    wire(model, world);
    scene.add(model.root);
    loaded.push(l);
  }

  // Framing comes from the profile only, so a missing or misplaced model shows up blank or cut off.
  const { profile } = loaded[0], { x, z, r } = profile.footprint, [y0, y1] = profile.yRange;
  const camera = new THREE.PerspectiveCamera(finale.fov, innerWidth / innerHeight, 0.1, 2000);
  const look = new THREE.Vector3();
  const place = (cam) => {
    if (cam.startsWith('cave')) { // cave close-up (09 Freeze item 1): cam=cave:<i>[:side]
      const [, i = 0, side = 1] = cam.split(':').map(Number), c = world.caves[i % world.caves.length], o = new THREE.Vector3(Math.sin(c.yaw), 0, Math.cos(c.yaw));
      look.set(c.x - o.x * 4, c.y + 2.4, c.z - o.z * 4);
      camera.position.copy(look).addScaledVector(o, 22).add(new THREE.Vector3(0, 5, 0)).addScaledVector(new THREE.Vector3(-o.z, 0, o.x), 10 * side);
    } else if (r > 50) { // world-sized: the live finale camera at its sway angle (?az=)
      const az = Number(q.get('az') || 0);
      camera.position.set(finale.radius * Math.sin(az), finale.height, finale.radius * Math.cos(az));
      look.set(...finale.look);
    } else {
      const yMid = (y0 + y1) / 2, d = Math.max(r, (y1 - y0) / 2) * 3.4 + 2;
      camera.position.set(x + d * 0.45, yMid + d * 0.55, z + d * 0.8);
      look.set(x, yMid, z);
    }
    camera.lookAt(look);
  };
  place(q.get('cam') || '');

  const gl = renderer.getContext(), W = gl.drawingBufferWidth, H = gl.drawingBufferHeight;
  const render = (buf) => {
    renderer.info.reset();
    renderer.render(scene, camera);
    if (buf) gl.readPixels(0, 0, W, H, gl.RGBA, gl.UNSIGNED_BYTE, buf);
    return { tris: renderer.info.render.triangles, draws: renderer.info.render.calls };
  };
  const roots = () => loaded.map((l) => l.model.root);
  const objects = (pred = () => true) => { const out = []; for (const rt of roots()) rt.traverse((o) => (o.isMesh || o.isPoints || o.isLine || o.isSprite) && pred(o) && out.push(o)); return out; };
  const mats = (o) => [].concat(o.material).filter(Boolean);
  const hidden = (list, fn) => { const was = list.map((o) => o.visible); list.forEach((o) => (o.visible = false)); try { return fn(); } finally { list.forEach((o, i) => (o.visible = was[i])); } };
  const diffPx = (a, b) => { let n = 0; for (let i = 0; i < a.length; i += 4) if (Math.abs(a[i] - b[i]) + Math.abs(a[i + 1] - b[i + 1]) + Math.abs(a[i + 2] - b[i + 2]) > 18) n++; return n; };

  // baseline: rig only (dome + haze floor), so stats count the models alone
  const base = new Uint8Array(W * H * 4), buf = new Uint8Array(W * H * 4);
  const rigOnly = () => { roots().forEach((o) => (o.visible = false)); const b = render(base); roots().forEach((o) => (o.visible = true)); return b; };
  let b0 = rigOnly();

  function stats(P) {
    if (P !== undefined) for (const l of loaded) l.model.pose(P);
    const s = render(buf), hit = diffPx(buf, base);
    const foreign = new Set(), shadows = new Set();
    for (const o of objects()) {
      for (const m of mats(o)) if (!m.userData?.kit) foreign.add(`${o.name || o.type}:${m.type}`);
      if (o.castShadow || o.receiveShadow) shadows.add(o.name || o.type);
    }
    if (renderer.shadowMap.enabled) shadows.add('renderer.shadowMap');
    return { ready: true, models: names, P, tris: s.tris - b0.tris, draws: s.draws - b0.draws, coverage: hit / (W * H), foreignMaterials: [...foreign], shadows: [...shadows], profile };
  }

  // ---------- colour membership (09: exact key, sRGB ±1/255 per channel) ----------
  const C = new THREE.Color(), C2 = new THREE.Color(), V = { r: 0, g: 0, b: 0 };
  const rgb255 = (c) => { c.getRGB(V, THREE.SRGBColorSpace); return [V.r * 255, V.g * 255, V.b * 255]; };
  const hex255 = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
  const near = (a, b) => a.every((v, i) => Math.abs(v - b[i]) <= 1.01);
  const isToken = (c, name) => near(rgb255(c), hex255(kit.token(name)));
  const onRamp = (name, p) => {
    const st = kit.RAMPS[name].map(([t]) => rgb255(kit.rampColor(name, t, C2)));
    for (let i = 0; i < st.length - 1; i++) {
      const a = st[i], d = st[i + 1].map((v, k) => v - a[k]), dd = d.reduce((s, v) => s + v * v, 0);
      const t = dd ? Math.min(1, Math.max(0, d.reduce((s, v, k) => s + v * (p[k] - a[k]), 0) / dd)) : 0;
      if (near(p, a.map((v, k) => v + d[k] * t))) return true;
    }
    return false;
  };
  const ALL = () => [...Object.keys(kit.TOKENS.dusk).filter((k) => k !== 'rig'), ...Object.keys(kit.TOKENS.semantic)];
  const unitHex = (c) => C2.copy(c).multiplyScalar(1 / Math.max(c.r, c.g, c.b, 1e-9)).getHexString();
  const sameHue = (a, b) => near(hex255('#' + unitHex(a)), hex255('#' + unitHex(b)));
  const LIT = new Set(['lit', 'terrain', 'crystal', 'lava']);
  const RAMP_ALSO = { slabLoad: ['slabNotReady'] }; // 09 semantic colours: "slab not ready" is the off-ramp slab state
  const SPRITE = kit.points('ink').map;

  function style(key) {
    kit.setKey(key); // repaints every kit colour; re-pose so a colour a model sets in pose() is what gets checked
    for (const l of loaded) l.model.pose(window.__gate.P ?? 6);
    const err = new Set(), add = (o, m) => err.add(`${o.name || o.type}/${o.material.userData?.kit || o.material.type}: ${m}`);
    for (const o of objects()) for (const m of mats(o)) {
      const k = m.userData?.kit, u = m.userData || {};
      if (!k) continue; // foreign: reported by stats()
      for (const [p, v] of Object.entries(m)) if (v?.isTexture && !(p === 'map' && (k === 'glow' || k === 'points') && v === SPRITE)) add(o, `texture on .${p}`);
      if (k === 'stamp') add(o, 'stamp() material in the main scene');
      if (LIT.has(k) && !m.flatShading) add(o, 'flatShading off');
      if (LIT.has(k) && m.vertexColors) add(o, 'vertex colours on a lit material');
      if (o.isInstancedMesh && o.instanceColor && !u.kitRamp) add(o, 'instanceColor without a kit ramp');
      if (u.kitRamp) {
        if (o.isInstancedMesh && o.instanceColor) {
          const a = o.instanceColor.array;
          for (let i = 0; i < o.count; i++) if (!onRamp(u.kitRamp, rgb255(C.setRGB(a[i * 3], a[i * 3 + 1], a[i * 3 + 2]))) && !(RAMP_ALSO[u.kitRamp] || []).some((n) => isToken(C, n))) { add(o, `instance ${i} #${C.getHexString()} not on ramp ${u.kitRamp}`); break; }
        } else if (k !== 'crystal' && k !== 'lava') add(o, `ramp ${u.kitRamp} material without instance colours`);
        if (k === 'crystal') {
          const hA = o.geometry.attributes.aHealth, gA = o.geometry.attributes.aGlow;
          for (let i = 0; hA && i < hA.count; i++) if (!onRamp('health', rgb255(C.setRGB(hA.getX(i), hA.getY(i), hA.getZ(i))))) { add(o, `aHealth ${i} not on ramp health`); break; }
          for (let i = 0; gA && i < gA.count; i++) if (!(gA.getX(i) >= 0 && gA.getX(i) <= 1.5)) { add(o, `aGlow ${i} = ${gA.getX(i)} outside [0, 1.5]`); break; }
        }
      }
      if ((LIT.has(k) && !u.kitToken) && m.color && m.color.getHex() !== 0xffffff) add(o, `base colour #${m.color.getHexString()} (ramp/patched materials stay white)`);
      if (u.kitToken && m.color && (k === 'lit' || k === 'points') && !isToken(m.color, u.kitToken)) add(o, `colour #${m.color.getHexString()} != ${u.kitToken} ${kit.token(u.kitToken)}`);
      if (u.kitToken && (k === 'glow' || k === 'line') && !isToken(C.copy(m.color).multiplyScalar(1 / (u.kitIntensity || 1)), u.kitToken)) add(o, `colour / intensity != ${u.kitToken}`);
      if (k === 'flow' && !isToken(m.uniforms.uColor.value, u.kitToken)) add(o, `uColor != ${u.kitToken}`);
      if (m.emissive && (m.emissive.r || m.emissive.g || m.emissive.b) && !(m.color && sameHue(m.emissive, m.color)) && !ALL().some((n) => sameHue(m.emissive, kit.color(n, C)))) add(o, `emissive #${m.emissive.getHexString()} not a kit colour`);
    }
    kit.setKey(KEY);
    return [...err];
  }

  // ---------- facet bands: median world-space edge length, instance matrices included ----------
  const EXEMPT = new Set(['glow', 'line', 'points', 'dome', 'flow', 'stamp', 'field', 'cave']);
  function facet() {
    const out = [], A = new THREE.Vector3(), B = new THREE.Vector3(), Cc = new THREE.Vector3(), M = new THREE.Matrix4(), IM = new THREE.Matrix4();
    roots().forEach((rt) => rt.updateMatrixWorld(true));
    for (const o of objects((o) => o.isMesh)) {
      const kind = mats(o)[0]?.userData?.kit, cls = o.userData.facet || (EXEMPT.has(kind) ? 'exempt' : profileOf(o).facet);
      const band = FACET[cls];
      if (band === null) continue;
      if (!band) { out.push({ mesh: o.name || o.type, cls, error: `unknown facet class ${cls}` }); continue; }
      const p = o.geometry.attributes.position, idx = o.geometry.index, nTri = (idx ? idx.count : p.count) / 3, n = o.isInstancedMesh ? o.count : 1;
      if (!nTri || !n) continue;
      const stride = Math.max(1, Math.floor((nTri * n) / 100000)), lens = [];
      for (let ins = 0; ins < n; ins++) {
        M.copy(o.matrixWorld);
        if (o.isInstancedMesh) { o.getMatrixAt(ins, IM); M.multiply(IM); }
        for (let t = (ins * nTri) % stride; t < nTri; t += stride) {
          const v = (j) => (idx ? idx.getX(t * 3 + j) : t * 3 + j);
          A.fromBufferAttribute(p, v(0)).applyMatrix4(M); B.fromBufferAttribute(p, v(1)).applyMatrix4(M); Cc.fromBufferAttribute(p, v(2)).applyMatrix4(M);
          lens.push(A.distanceTo(B), B.distanceTo(Cc), Cc.distanceTo(A));
        }
      }
      lens.sort((a, b) => a - b);
      const med = lens[lens.length >> 1];
      out.push({ mesh: o.name || o.type, cls, band, median: +med.toFixed(3), ok: med >= band[0] && med <= band[1] });
    }
    return out;
  }
  const profileOf = (o) => { for (const l of loaded) { let p = o; while (p && p !== l.model.root) p = p.parent; if (p) return l.profile; } return profile; };

  // ---------- luminance (09): own HalfFloat RT, emissives zeroed, glow/line/points/lava hidden ----------
  const rt = new THREE.WebGLRenderTarget(W, H, { type: THREE.HalfFloatType }), rt8 = new THREE.WebGLRenderTarget(W, H);
  const white = new THREE.MeshBasicMaterial({ color: 0xffffff, fog: false }), black = new THREE.MeshBasicMaterial({ color: 0x000000, fog: false });
  function lum(key) {
    kit.setKey(key);
    const U = kit.U, saved = [U.uHeat.value, U.uPoolCol.value.clone(), U.uVeinN.value], em = [];
    U.uHeat.value = 0; U.uPoolCol.value.setRGB(0, 0, 0); U.uVeinN.value = 0;
    for (const o of objects()) for (const m of mats(o)) if (m.emissive) { em.push([m, m.emissive.clone()]); m.emissive.setRGB(0, 0, 0); }
    const hide = objects((o) => ['glow', 'line', 'points', 'lava'].includes(mats(o)[0]?.userData?.kit));
    try {
      return hidden(hide, () => {
        renderer.setRenderTarget(rt);
        renderer.render(scene, camera);
        const h = new Uint16Array(W * H * 4);
        renderer.readRenderTargetPixels(rt, 0, 0, W, H, h);
        // terrain mask: model meshes drawn white (terrain) / black (others), rig hidden, black clear
        const swap = objects((o) => o.isMesh).map((o) => [o, o.material]);
        swap.forEach(([o, m]) => (o.material = [].concat(m)[0]?.userData?.kit === 'terrain' ? white : black));
        const mask = new Uint8Array(W * H * 4), bg = scene.background;
        hidden([RIG.dome, RIG.floor], () => { scene.background = new THREE.Color(0); renderer.setRenderTarget(rt8); renderer.render(scene, camera); renderer.readRenderTargetPixels(rt8, 0, 0, W, H, mask); });
        scene.background = bg;
        swap.forEach(([o, m]) => (o.material = m));
        renderer.setRenderTarget(null);
        const f = THREE.DataUtils.fromHalfFloat, terr = [];
        let max = 0, any = 0;
        for (let i = 0; i < W * H * 4; i += 4) {
          const L = 0.2126 * f(h[i]) + 0.7152 * f(h[i + 1]) + 0.0722 * f(h[i + 2]);
          if (L > max) max = L;
          if (h[i] || h[i + 1] || h[i + 2]) any++;
          if (mask[i] > 127) terr.push(L);
        }
        if (!any) throw new Error('HalfFloat readback returned an all-zero frame (float readback unsupported?)');
        terr.sort((a, b) => a - b);
        return { max: +max.toFixed(4), terrainPx: terr.length, p50: terr.length ? +terr[terr.length >> 1].toFixed(5) : null };
      });
    } finally {
      [U.uHeat.value, , U.uVeinN.value] = saved; U.uPoolCol.value.copy(saved[1]);
      em.forEach(([m, e]) => m.emissive.copy(e));
      renderer.setRenderTarget(null);
      kit.setKey(KEY);
    }
  }

  // ---------- clipped flag: closing uClip over everything must remove (clipped) or keep (not) every solid ----------
  function clip() {
    const solid = (o) => o.isMesh && ['lit', 'terrain', 'lava'].includes(mats(o)[0]?.userData?.kit);
    const others = objects((o) => !solid(o)), U = kit.U.uClip.value, was = U.clone();
    return hidden(others, () => {
      render(buf); const open = diffPx(buf, base);
      U.set(0, 1, 0, -1e6); render(buf); const cut = diffPx(buf, base);
      U.copy(was);
      return { open, cut };
    });
  }

  // ---------- pose determinism: pose(P) twice, other P in between, same state ----------
  function poseHash() {
    let h = 2166136261;
    const mix = (v) => { h = Math.imul(h ^ Math.round(v * 1e4), 16777619); };
    for (const rt of roots()) {
      rt.updateMatrixWorld(true);
      rt.traverse((o) => {
        mix(o.visible ? 1 : 0); o.matrixWorld.elements.forEach(mix);
        if (o.isInstancedMesh) { mix(o.count); for (let i = 0; i < o.count * 16; i++) mix(o.instanceMatrix.array[i]); if (o.instanceColor) for (let i = 0; i < o.count * 3; i++) mix(o.instanceColor.array[i]); }
        for (const m of o.material ? mats(o) : []) { mix(m.opacity); m.emissive && mix(m.emissive.r + m.emissive.g + m.emissive.b); for (const u of Object.values(m.userData.uniforms || {})) typeof u.value === 'number' && mix(u.value); }
      });
    }
    return h >>> 0;
  }
  function pose(Ps) {
    const bad = [];
    for (const P of Ps) {
      for (const l of loaded) l.model.pose(P);
      const a = poseHash();
      for (const Q of Ps) if (Q !== P) for (const l of loaded) l.model.pose(Q);
      for (const l of loaded) l.model.pose(P);
      if (poseHash() !== a) bad.push(P);
    }
    return bad;
  }

  function physics() {
    const bad = [];
    for (let i = 0; i < bodies.length; i++) for (let j = i + 1; j < bodies.length; j++) {
      const a = bodies[i], b = bodies[j], ga = a.body.collisionFilterGroup, gb = b.body.collisionFilterGroup;
      if (a.world !== b.world || !(((ga & 1) && (gb & 2)) || ((ga & 2) && (gb & 1)))) continue;
      if ((ga & b.body.collisionFilterMask) && (gb & a.body.collisionFilterMask)) bad.push(`bodies ${a.body.id} (group ${ga}) and ${b.body.id} (group ${gb}) can collide (real↔surface)`);
    }
    return bad;
  }

  // dusk terrain luminance from each cave close-up (09 Freeze item 1), camera restored
  window.__caveLum = () => { try { return world.caves.map((_, i) => (place(`cave:${i}`), lum('dusk'))); } finally { place(q.get('cam') || ''); } };

  window.__checks = (Ps = [0, 3, 6]) => {
    const P0 = window.__gate.P ?? 6;
    const res = { pose: pose(Ps), style: { dusk: style('dusk'), day: style('day') }, facet: facet(), lum: { dusk: lum('dusk'), day: lum('day') }, clip: clip(), physics: physics() };
    window.__setP(P0);
    return res;
  };

  // ---------- step (audited during __test): peak budget, stepMs, rig/dome untouched, foreign field writes ----------
  // uStale is sky's (kit.setStale), not the rig's: excluded
  const rigSnap = () => JSON.stringify([RIG.hemi.color, RIG.hemi.groundColor, RIG.hemi.intensity, RIG.dir.color, RIG.dir.intensity, RIG.dir.position, scene.fog.color, scene.fog.density,
    renderer.toneMapping, renderer.toneMappingExposure, Object.entries(RIG.dome.material.uniforms).filter(([k]) => k !== 'uStale').map(([, u]) => u.value), kit.phase(), kit.U.uFootprint.value === kit.footprintRT.texture]);
  // Every WRITERS slot the model does not own (fields by checksum, shared state by JSON) must be unchanged after its
  // step() and after each of its bus/ev handlers. A nested call (a handler run by an emit inside step) may write its
  // own slots: those are re-baselined in every enclosing snapshot, so the outer caller is not blamed.
  const fieldSum = (a) => { let s = 0; for (let i = 0; i < a.length; i++) s += a[i] * ((i % 7) + 1); return s; };
  const val = (s) => (s in world.fields ? fieldSum(world.fields[s]) : JSON.stringify(world[s], (k, x) => (x instanceof Map ? [...x] : x)));
  const open = [];
  function audited(l, what, fn) {
    if (!audit) return fn();
    const r0 = rigSnap(), own = new Set(l.profile.writes), f0 = new Map(Object.keys(WRITERS).filter((s) => !own.has(s)).map((s) => [s, val(s)]));
    open.push(f0);
    try { return fn(); } finally {
      open.pop();
      if (rigSnap() !== r0) audit.rig.add(`${l.name} ${what} changed the rig/dome/fog/exposure/uFootprint`);
      for (const [s, x] of f0) if (val(s) !== x) audit.fields.add(`${l.name} ${what} changed ${s in world.fields ? 'field' : 'slot'} ${s} (not in its writes[])`);
      for (const outer of open) for (const s of own) if (outer.has(s)) outer.set(s, val(s));
    }
  }
  let t = 0;
  const step = (k = 1, dt = 1 / 30) => {
    for (let i = 0; i < k; i++) {
      world.step(dt);
      kit.U.uHMax.value = world.hmax; // deposits move the summit (terrain shading keys off it)
      t += dt;
      for (const l of loaded) audited(l, 'step()', () => {
        const a = performance.now();
        l.model.step(dt, t);
        audit?.ms.push(performance.now() - a);
      });
      if (audit && ++audit.n % 10 === 0) { const s = render(); audit.peak.tris = Math.max(audit.peak.tris, s.tris - b0.tris); audit.peak.draws = Math.max(audit.peak.draws, s.draws - b0.draws); }
    }
  };

  let n = 0;
  const inject = (type, p = {}) => {
    if (type.startsWith('ev:')) return world.applyFrame({ t: 'ev', id: p.id || `01JA${String(++n).padStart(22, '0')}`, ts: Date.now(), kind: type.slice(3), ...p });
    if (type.startsWith('world:')) return world.applyFrame({ t: 'world', kind: type.slice(6), ...p, ...(Array.isArray(p.cells) ? { cells: encodeI16(p.cells) } : {}) });
    world.emit(type, p, BUS[type]?.[0]);
  };

  window.__inject = inject;
  window.__step = step;
  window.__world = world; // dev scripts (frames the injectors do not cover, e.g. a metrics spike)
  window.__setP = (P) => (window.__gate = { ...stats(P), hasTest: loaded.some((l) => !l.fx || l.mod.test) });
  window.__test = async () => {
    audit = { n: 0, ms: [], peak: { tris: 0, draws: 0 }, rig: new Set(), fields: new Set() };
    const out = { ok: true };
    try {
      for (const l of loaded) {
        const test = l.fx ? l.mod.test : (await import(`../models/${l.name}/test.js`)).default;
        if (!test) continue;
        const fixture = l.fx ? null : await (await fetch(`../models/${l.name}/fixture.json`)).json();
        await test({ model: l.model, world, inject, step, fixture, assert: (c, m) => { if (!c) throw new Error(`assert: ${m}`); } });
      }
      step(30); // idle steps: stepMs and the rig audit also cover models without a test
    } catch (e) {
      Object.assign(out, { ok: false, error: String(e?.stack || e) });
    }
    const ms = audit.ms.sort((a, b) => a - b);
    Object.assign(out, {
      steps: ms.length, peak: audit.peak, rig: [...audit.rig], fields: [...audit.fields],
      stepMs: ms.length ? { mean: +(ms.reduce((a, b) => a + b, 0) / ms.length).toFixed(3), p95: +ms[Math.floor(ms.length * 0.95)].toFixed(3) } : null,
      rec: loaded.map((l) => ({ name: l.name, profile: { writes: l.profile.writes, emits: l.profile.emits, listens: l.profile.listens }, writes: [...l.rec.writes], emits: [...l.rec.emits], listens: [...l.rec.listens] })),
    });
    audit = null;
    return out;
  };
  window.__setP(Number(q.get('P') ?? 6));

  if (q.get('ui') !== '0') (await import('./sandbox-ui.js')).default({ THREE, kit, world, loaded, renderer, scene, camera, look, render, step, inject, EV_KINDS, BUS, WRITERS, setP: window.__setP, rebase: () => (b0 = rigOnly()) });
} catch (e) {
  console.error(e);
  window.__gate = { ready: true, error: String(e?.stack || e) };
}

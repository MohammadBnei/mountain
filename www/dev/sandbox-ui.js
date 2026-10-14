// Sandbox controls (04): P slider, play/step, dt multiplier, key, tier toggle, params sliders, injectors for every
// ev / world:* / bus type, scrub (live socket + day replay + release), field heatmaps, bus log, drag-orbit + wheel zoom.
// Loaded by sandbox.js unless ?ui=0.
// No inline styles in markup (relay CSP: style-src 'self'); the look lives in sandbox.css.
const TPL = {
  'ev:argo.sync': { app: 'wird', ok: true }, 'ev:argo.health': { app: 'wird', status: 'Degraded' },
  'ev:pod.churn': { app: 'wird', restarts: 2, oom: true, crashloop: false }, 'ev:node.ready': { host: 'h2', ready: false },
  'ev:quorum': { up: ['h1', 'h2'] }, 'ev:alert.fire': { fp: '0123456789abcdef', sev: 'warning', layer: 'k8s', startsAt: 1760450000000 },
  'ev:alert.resolve': { fp: '0123456789abcdef', sev: 'warning', layer: 'k8s', durS: 900 }, 'ev:build': { app: 'wird', state: 'running' },
  'ev:gitops.commit': { n: 1 }, 'ev:adr.merge': { day: '2026-10-14' }, 'ev:replicas': { app: 'wird', bucket: 2 }, 'ev:app.gone': { app: 'wird' },
  'world:deposit': { id: '01JA0000000000000000000099', x0: 60, z0: 60, w: 8, h: 8, cells: Array(64).fill(400) },
  'world:site': { app: 'wird', x: 12.5, z: -30, y: 18.2, tier: 1, group: 'user' },
  'world:mark': { id: '01JA0000000000000000000098', mark: 'cairn', pose: { x: 1, z: 2 }, weight: 1 },
  'world:vein': { fp: '0123456789abcdef', a: [0, 10, 0], b: [8, 2, 4], thick: 0.4, layer: 'k8s', ref: false },
  'world:glacier': { mass: 0.5, tongues: [{ app: 'wird', len: 10 }], ash: [] }, 'world:ash': { day: '2026-10-14' },
  'world:cloud': { id: 'c1', pos: { x: 0, y: 60, z: 0 }, state: 'drift', credits: [] }, 'world:rain': { x: 0, z: 0, r: 6 },
};
const busTpl = { 'magma.dyke': { app: 'wird', x: 0, z: 0, heat: 0.5 }, 'lava.heat': { x: 0, z: 0, q: 1 }, 'cauldron.sag': { x: 0, z: 0, depth: 1, sev: 'warning' }, 'aquifer.seep': { pool: 'wird-pg', x: 0, z: 0 } };

export default function ui({ THREE, kit, world, loaded, renderer, scene, camera, look, render, step, inject, EV_KINDS, BUS, setP, rebase }) {
  const el = (tag, attrs = {}, ...kids) => { const e = Object.assign(document.createElement(tag), attrs); e.append(...kids); return e; };
  const row = (label, ...kids) => el('label', { className: 'row' }, el('span', { textContent: label }), ...kids);
  const q = new URLSearchParams(location.search);
  let P = Number(q.get('P') ?? 6), playing = false, mult = 1;

  // camera: drag to orbit around `look`, wheel to zoom
  const off = camera.position.clone().sub(look), sph = new THREE.Spherical().setFromVector3(off);
  const place = () => { camera.position.setFromSpherical(sph).add(look); camera.lookAt(look); };
  let drag = null;
  renderer.domElement.addEventListener('pointerdown', (e) => (drag = [e.clientX, e.clientY]));
  addEventListener('pointerup', () => (drag = null));
  addEventListener('pointermove', (e) => {
    if (!drag) return;
    sph.theta -= (e.clientX - drag[0]) * 0.005; sph.phi = Math.min(3.1, Math.max(0.05, sph.phi - (e.clientY - drag[1]) * 0.005));
    drag = [e.clientX, e.clientY]; place();
  });
  renderer.domElement.addEventListener('wheel', (e) => { sph.radius *= Math.exp(e.deltaY * 0.001); place(); e.preventDefault(); }, { passive: false });
  addEventListener('resize', () => { renderer.setSize(innerWidth, innerHeight, false); camera.aspect = innerWidth / innerHeight; camera.updateProjectionMatrix(); rebase(); });

  // P, play/step, dt, key, tier
  const pIn = el('input', { type: 'range', min: 0, max: 6, step: 0.01, value: P });
  const pOut = el('output', { textContent: P.toFixed(2) });
  pIn.oninput = () => { P = Number(pIn.value); pOut.textContent = P.toFixed(2); setP(P); };
  const play = el('button', { textContent: 'play' });
  play.onclick = () => { playing = !playing; play.textContent = playing ? 'pause' : 'play'; };
  const stepB = el('button', { textContent: 'step' });
  stepB.onclick = () => step(1, mult / 30);
  const dt = el('select', {}, ...[0.25, 0.5, 1, 2, 4].map((v) => el('option', { value: v, textContent: `dt ×${v}`, selected: v === 1 })));
  dt.onchange = () => (mult = Number(dt.value));
  const key = el('select', {}, ...kit.KEYS.map((k) => el('option', { value: k, textContent: k, selected: k === (q.get('key') || 'dusk') })));
  key.onchange = () => kit.setKey(key.value);
  const tier = el('button', { textContent: `tier: ${kit.QUALITY} → ${kit.QUALITY === 'low' ? 'high' : 'low'}` });
  tier.onclick = () => { q.set('q', kit.QUALITY === 'low' ? 'high' : 'low'); location.search = q; };

  // params sliders (numbers → range, booleans → checkbox); a change re-poses at the current P
  const params = loaded.flatMap((l) => Object.entries(l.model.params || {}).filter(([, v]) => typeof v === 'number' || typeof v === 'boolean').map(([k, v]) => {
    if (typeof v === 'boolean') { const c = el('input', { type: 'checkbox', checked: v }); c.onchange = () => { l.model.params[k] = c.checked; l.model.pose(P); }; return row(`${l.name}.${k}`, c); }
    const hi = Math.max(1, Math.abs(v) * 4), lo = v < 0 ? -hi : 0, o = el('output', { textContent: v });
    const s = el('input', { type: 'range', min: lo, max: hi, step: (hi - lo) / 200, value: v });
    s.oninput = () => { l.model.params[k] = Number(s.value); o.textContent = Number(s.value).toFixed(3); l.model.pose(P); };
    return row(`${l.name}.${k}`, s, o);
  }));

  // injectors
  const types = [...EV_KINDS.map((k) => `ev:${k}`), ...Object.keys(BUS)];
  const sel = el('select', {}, ...types.map((t) => el('option', { value: t, textContent: t })));
  const body = el('textarea', { rows: 5, spellcheck: false });
  const fill = () => (body.value = JSON.stringify(TPL[sel.value] || busTpl[sel.value] || { x: 0, z: 0, vol: 1 }));
  sel.onchange = fill; fill();
  const go = el('button', { textContent: 'inject' }), injErr = el('div', { className: 'err' });
  go.onclick = () => { try { inject(sel.value, JSON.parse(body.value)); injErr.textContent = ''; } catch (e) { injErr.textContent = e.message; } };

  // field heatmaps
  const fields = [...Object.keys(world.fields), 'bench'];
  const fSel = el('select', {}, el('option', { value: '', textContent: 'heatmap: off' }), ...fields.map((f) => el('option', { value: f, textContent: f })));
  const cv = el('canvas', { width: 128, height: 128, className: 'heat', hidden: true }), range = el('div', { className: 'dim' });
  fSel.onchange = () => (cv.hidden = !fSel.value);
  const heat = () => {
    if (!fSel.value) return;
    const a = fSel.value === 'bench' ? world.bench : world.fields[fSel.value], N = Math.round(Math.sqrt(a.length));
    cv.width = cv.height = N;
    let lo = Infinity, hi = -Infinity;
    for (const v of a) { if (v < lo) lo = v; if (v > hi) hi = v; }
    const img = new ImageData(N, N), c = new THREE.Color();
    for (let i = 0; i < a.length; i++) { // row-major z outer: canvas row = z (north up = −z)
      kit.rampColor('lava', 1 - (hi > lo ? (a[i] - lo) / (hi - lo) : 0), c);
      img.data.set([c.r * 255, c.g * 255, c.b * 255, 255].map(Math.round), i * 4);
    }
    cv.getContext('2d').putImageData(img, 0, 0);
    range.textContent = `${fSel.value} ${N}²  min ${lo.toFixed(2)}  max ${hi.toFixed(2)}`;
  };
  setInterval(heat, 500);

  // scrub (06, W2): live = socket.js to this page's relay (sandbox served by `relay --dev`); scrub asks for one UTC day's
  // replay (frames play while stepping); release resyncs to live. The finale timeline bar is W3 (05).
  const dayIn = el('input', { type: 'date', value: new Date(Date.now() - 864e5).toISOString().slice(0, 10) });
  const scrubOut = el('div', { className: 'dim', textContent: 'not connected' });
  const liveB = el('button', { textContent: 'live' }), scrubB = el('button', { textContent: 'scrub' }), relB = el('button', { textContent: 'release' });
  liveB.onclick = async () => {
    const { connect } = await import('../js/socket.js');
    liveB.disabled = true;
    connect({ world, onStatus: (st) => (scrubOut.textContent = `socket ${st}`) });
  };
  scrubB.onclick = () => world.scrub(dayIn.value)
    .then((r) => (scrubOut.textContent = `replay ${r.day}: ${r.frames.length} frames, ${r.metrics.length} metrics (play to run them)`))
    .catch((e) => (scrubOut.textContent = e.message));
  relB.onclick = () => { world.release(); scrubOut.textContent = 'released: resyncing'; };

  // bus log: every registered bus type + every ev kind
  const log = el('pre', { className: 'log' }), lines = [];
  for (const t of [...Object.keys(BUS), ...EV_KINDS.map((k) => `ev:${k}`)]) world.on(t, (p) => {
    const s = JSON.stringify(p, (k, v) => (ArrayBuffer.isView(v) ? `[${v.length} cells]` : v));
    lines.unshift(`${new Date().toISOString().slice(11, 19)} ${t} ${s.slice(0, 140)}`);
    lines.length = Math.min(lines.length, 40);
    log.textContent = lines.join('\n');
  });

  document.body.append(el('aside', { className: 'panel' },
    el('h1', { textContent: loaded.map((l) => l.name).join(' + ') }),
    row('P', pIn, pOut), el('div', { className: 'btns' }, play, stepB, dt, key, tier),
    ...(params.length ? [el('h2', { textContent: 'params' }), ...params] : []),
    el('h2', { textContent: 'inject' }), sel, body, go, injErr,
    el('h2', { textContent: 'scrub' }), el('div', { className: 'btns' }, dayIn, liveB, scrubB, relB), scrubOut,
    el('h2', { textContent: 'fields' }), fSel, cv, range,
    el('h2', { textContent: 'bus' }), log));

  // render loop: one step per 1/30 s of real time while playing, each step dt = mult/30 (same as the step button)
  // Draws at 30 fps at most (also while paused: sliders and orbit still change the view).
  let acc = 0, last = performance.now(), drawn = 0;
  renderer.setAnimationLoop((now) => {
    if (now - drawn < 1000 / 30 - 1) return;
    drawn = now;
    acc = Math.min(acc + (now - last) / 1000, 0.25);
    last = now;
    while (playing && acc >= 1 / 30) { step(1, mult / 30); acc -= 1 / 30; }
    if (!playing) acc = 0;
    kit.U.uSnowline.value = world.snowline;
    render();
  });
}

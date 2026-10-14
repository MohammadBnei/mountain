#!/usr/bin/env node
// Render gate (04, 09 "Gate checks"). Node >= 22, no dependencies. Exit 0 = PASS, 1 = FAIL.
//
//   node www/dev/gate.mjs <model>          models/<model>/ in the sandbox, profile layout.profiles[<model>], test.js
//   node www/dev/gate.mjs fixture:<name>   dev/fixtures/<name>.js (profile _<name> or _test)
//   node www/dev/gate.mjs page             the real relay binary (go build ./relay; --dev --source=fixture:<file>):
//                                          CSP header, console, foreign requests, /ws hello, relay restart → resync
//   node www/dev/gate.mjs styleboard       dev/styleboard.html renders (4 keys × 4 angles) with a clean console
//   node www/dev/gate.mjs selftest         every fixture against its expected verdict and reason
// Options: --P 0,3,6  --w 1280  --h 800  --key dusk  --q low  --out <dir>  --qs 'cam=cave:0&az=0.4'
//          --fixture <relay fixture json> (page; default: www/dev/fixtures/relay-hostile.json, else the first relay-*.json,
//          else fixture:none with a WARN)   --strict (page: fail without a relay fixture)   (env CHROME, GATE_OUT)
//
// Model checks: build error; contract shape; draws/tris vs profile at each P and peak during test.js; coverage;
// stepMs (mean over audited steps); writes/emits/listens ⊆ profile; no foreign field writes in step(); clipped flag
// (rendered: closing uClip removes every solid iff clipped); pose(P) determinism; test.js; kit factories only;
// shadows; style per factory at the exact dusk and day keys (colour ∈ tokens/ramps ±1/255, flatShading, no maps,
// emissive hue); facet bands; luminance ceiling < 1 (dusk, day) and dusk terrain p50 > 0.004 on a HalfFloat RT
// (default camera and, without --qs, every cave:<i> close-up);
// static source scan; rig/dome/fog/exposure/uFootprint unchanged around step(); no real↔surface bodies;
// console errors, exceptions, failed or foreign requests. Checks run before and after test.js.
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createServer } from 'node:net';
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ROOT, launch, serveStatic, sleep, waitFor } from './cdp.mjs';

const argv = process.argv.slice(2);
const OPTS = ['--P', '--w', '--h', '--out', '--key', '--q', '--qs', '--fixture'];
const target = argv.find((a, i) => !a.startsWith('--') && !(i > 0 && OPTS.includes(argv[i - 1])));
const opt = (n, d) => { const i = argv.indexOf(`--${n}`); return i >= 0 ? argv[i + 1] : d; };
if (!target) { console.error('usage: gate.mjs <model|fixture:name|page|styleboard|selftest> [--P list] [--w px] [--h px] [--key k] [--q low] [--out dir] [--fixture file] [--strict]'); process.exit(2); }

if (target === 'selftest') {
  const want = {
    cube: ['PASS'], terrain: ['PASS'], blank: ['FAIL', /nearly blank/], broken: ['FAIL', /build error/], foreign: ['FAIL', /not from kit/],
    failtest: ['FAIL', /test\.js failed/], offpalette: ['FAIL', /colour #ff0000 != rock0/], scan: ['FAIL', /static scan/], facet: ['FAIL', /facet .* median 3/],
    bright: ['FAIL', /luminance day max/], impure: ['FAIL', /pose\(P\) not deterministic/], fieldwrite: ['FAIL', /changed field temp/],
    handler: ['FAIL', /on ev:argo\.sync changed field temp[^]*on ev:argo\.sync changed slot wind/],
    rig: ['FAIL', /changed the rig/], unclipped: ['FAIL', /clipped:false but/], contact: ['FAIL', /real↔surface/],
  };
  const run = ([f, [v, why]]) => new Promise((done) => {
    const p = spawn(process.execPath, [fileURLToPath(import.meta.url), `fixture:${f}`, ...argv.slice(1)]);
    let out = '';
    p.stdout.on('data', (d) => (out += d)); p.stderr.on('data', (d) => (out += d));
    p.on('close', (code) => {
      const got = code === 0 ? 'PASS' : 'FAIL', ok = got === v && (!why || why.test(out));
      console.log(`${ok ? 'ok  ' : 'BAD '} fixture:${f} want ${v}${why ? ` ${why}` : ''} got ${got}`);
      if (!ok) console.log(out);
      done(ok);
    });
  });
  const jobs = Object.entries(want), res = [];
  await Promise.all(Array.from({ length: 4 }, async () => { while (jobs.length) res.push(await run(jobs.shift())); }));
  const rf = (fs) => relayFixture(fs).split('/').pop();
  const pick = rf(['relay-a.json', 'relay-hostile.json']) === 'relay-hostile.json' && rf(['hello.json', 'relay-b.json', 'relay-a.json']) === 'relay-a.json' && rf(['hello.json']) === 'none';
  console.log(`${pick ? 'ok  ' : 'BAD '} page fixture pick: relay-hostile.json > first relay-*.json > none`);
  res.push(pick);
  const bad = res.filter((x) => !x).length;
  console.log(bad ? `SELFTEST FAIL (${bad})` : 'SELFTEST PASS');
  process.exit(bad ? 1 : 0);
}

const W = Number(opt('w', 1280)), H = Number(opt('h', 800));
const outDir = resolve(opt('out', process.env.GATE_OUT || join(tmpdir(), 'mountain-gate')), target.replace(/[:/]/g, '_'));
await mkdir(outDir, { recursive: true });
const failures = [];
const fail = (m) => failures.push(m);
const finish = async (cleanup) => {
  await sleep(100);
  await cleanup?.();
  const uniq = [...new Set(failures)];
  if (uniq.length) { console.log(`GATE FAIL ${target}`); uniq.forEach((f) => console.log('  - ' + f)); process.exit(1); }
  console.log(`GATE PASS ${target}`);
  process.exit(0);
};

// Static source scan (09): no colour literals, Color constructors, loaders, canvas textures or shader patches in models.
const BANNED = /#[0-9a-f]{6}|0x[0-9a-f]{6}|new (THREE\.)?Color\(|TextureLoader|GLTFLoader|CanvasTexture|onBeforeCompile/i;
async function scan(files) {
  for (const f of files) (await readFile(f, 'utf8')).split('\n').forEach((l, i) => { const m = l.match(BANNED); if (m) fail(`static scan: ${f.slice(ROOT.length + 1)}:${i + 1} "${m[0]}"`); });
}
const jsUnder = async (dir) => (existsSync(dir) ? (await readdir(dir, { recursive: true })).filter((f) => f.endsWith('.js')).map((f) => join(dir, f)) : []);

// A layout.page model whose folder does not exist yet (another lane's) 404s on import: noted, not failed.
const unbuilt = (s) => [...s.matchAll(/\/models\/([^/]+)\/index\.js/g)].some((m) => !existsSync(join(ROOT, 'models', m[1])));
/** Console errors and warnings, exceptions, HTTP ≥ 400, failed and foreign requests → failures (unless allow(text)). */
function watch(page, origin, { allow = () => false } = {}) {
  const ok = (...xs) => { const s = xs.join(' '); return allow(s) || unbuilt(s); };
  page.on(({ method, params }) => {
    if (method === 'Runtime.consoleAPICalled' && (params.type === 'error' || params.type === 'assert' || params.type === 'warning')) fail(`console.${params.type}: ${params.args.map((a) => a.value ?? a.description ?? '').join(' ').slice(0, 400)}`);
    if (method === 'Runtime.exceptionThrown') fail(`exception: ${(params.exceptionDetails.exception?.description || params.exceptionDetails.text).slice(0, 400)}`);
    if (method === 'Log.entryAdded' && params.entry.level === 'error' && !ok(params.entry.text, params.entry.url)) fail(`log error: ${params.entry.text.slice(0, 300)} ${params.entry.url || ''}`);
    if (method === 'Network.requestWillBeSent') { const u = params.request.url; if (!u.startsWith(origin) && !u.startsWith('data:') && !u.startsWith('blob:')) fail(`foreign request: ${u}`); }
    if (method === 'Network.responseReceived' && params.response.status >= 400 && !ok(params.response.url)) fail(`HTTP ${params.response.status}: ${params.response.url}`);
    if (method === 'Network.loadingFailed' && !params.canceled && !ok(params.errorText, params.type)) fail(`request failed: ${params.errorText}`);
  });
  return Promise.all(['Runtime.enable', 'Log.enable', 'Network.enable', 'Page.enable'].map((m) => page.s(m)));
}
const shot = async (page, name) => {
  const png = join(outDir, name);
  await writeFile(png, Buffer.from((await page.s('Page.captureScreenshot', { format: 'png' })).data, 'base64'));
  return png;
};

if (target === 'page') await pageMode();
else if (target === 'styleboard') await styleboardMode();
else await modelMode();

// ---------------------------------------------------------------- model / fixture
async function modelMode() {
  const isFixture = target.startsWith('fixture:');
  const { profiles } = await import(join(ROOT, 'js/layout.js'));
  let profile = isFixture ? profiles[`_${target.slice(8)}`] || profiles._test : profiles[target];
  if (!profile) { fail('no layout profile'); return finish(); }
  const Ps = opt('P') ? opt('P').split(',').map(Number) : [0, 3, 6];
  await scan(isFixture ? [join(ROOT, 'dev/fixtures', `${target.slice(8)}.js`)] : await jsUnder(join(ROOT, 'models', target)));

  const srv = await serveStatic();
  let page;
  try { page = await launch({ w: W, h: H, mobile: W < 600 }); } catch (e) { fail(e.message); return finish(srv.close); }
  const cleanup = async () => { await page.close(); srv.close(); };
  const timer = setTimeout(async () => { fail('global timeout 240 s'); await finish(cleanup); }, 240000);
  await watch(page, srv.origin);

  const qs = new URLSearchParams({ m: target, P: String(Ps[0]), key: opt('key', 'dusk'), ui: '0', ...(opt('q') ? { q: opt('q') } : {}), ...Object.fromEntries(new URLSearchParams(opt('qs', ''))) });
  await page.s('Page.navigate', { url: `${srv.origin}/dev/sandbox.html?${qs}` });
  const g = await waitFor(page, 'window.__gate?.ready && window.__gate');
  if (!g) fail('sandbox not ready within 30 s');
  if (g?.profile) profile = g.profile; // includes a fixture's own `profile` overrides
  await sleep(300); // let late console errors and requests land
  const pct = (n) => (n * 100).toFixed(2) + '%';

  if (g?.error) fail(`build error: ${g.error.split('\n')[0]}`);
  else if (g) {
    for (const P of Ps) {
      const st = await page.evalJSON(`window.__setP(${P})`);
      const png = await shot(page, `P${P}-${W}${opt('key') ? '-' + opt('key') : ''}${opt('qs') ? '-' + opt('qs').replace(/[^a-z0-9.]+/gi, '_') : ''}.png`);
      console.log(`P=${P}  coverage=${pct(st.coverage)}  tris=${st.tris}  draws=${st.draws}  ${png}`);
      const minCov = profile.minCoverage ?? 0.01;
      if (P >= profile.visibleFrom && st.coverage < minCov) fail(`P=${P}: nearly blank (coverage ${pct(st.coverage)} < ${pct(minCov)})`);
      if (st.tris > profile.tris) fail(`P=${P}: triangles ${st.tris} > ${profile.tris}`);
      if (st.draws > profile.draws) fail(`P=${P}: draw calls ${st.draws} > ${profile.draws}`);
      if (st.foreignMaterials.length) fail(`P=${P}: materials not from kit: ${st.foreignMaterials.join(', ')}`);
      if (st.shadows.length) fail(`P=${P}: shadows on: ${st.shadows.join(', ')}`);
    }
    await checks('', Ps);
    const r = await page.evalJSON('window.__test()');
    console.log(`test.js: ${g.hasTest ? (r.ok ? 'ok' : 'FAILED') : 'none'}  steps=${r.steps}  stepMs mean=${r.stepMs?.mean} p95=${r.stepMs?.p95}  peak tris=${r.peak.tris} draws=${r.peak.draws}`);
    if (!r.ok) fail(`test.js failed: ${r.error.split('\n')[0]}`);
    if (r.peak.tris > profile.peak.tris) fail(`peak triangles ${r.peak.tris} > ${profile.peak.tris}`);
    if (r.peak.draws > profile.peak.draws) fail(`peak draw calls ${r.peak.draws} > ${profile.peak.draws}`);
    if (r.stepMs && r.stepMs.mean > profile.stepMs) fail(`stepMs mean ${r.stepMs.mean} > ${profile.stepMs}`);
    r.rig.forEach(fail); r.fields.forEach(fail);
    for (const m of r.rec) for (const k of ['writes', 'emits', 'listens']) {
      const extra = m[k].filter((x) => !m.profile[k].includes(x));
      if (extra.length) fail(`${m.name} ${k} ${extra.join(', ')} not declared in its profile ${k}[]`);
    }
    await checks('after test.js: ', Ps);
  }
  clearTimeout(timer);

  async function checks(pre, Ps) {
    let c;
    try { c = await page.evalJSON(`window.__checks(${JSON.stringify(Ps)})`); } catch (e) { return fail(`${pre}checks threw: ${e.message.split('\n')[0]}`); }
    if (c.pose.length) fail(`${pre}pose(P) not deterministic at P=${c.pose.join(',')}`);
    for (const k of ['dusk', 'day']) c.style[k].forEach((e) => fail(`${pre}style ${k}: ${e}`));
    for (const f of c.facet) if (f.error || !f.ok) fail(`${pre}facet ${f.mesh}: ${f.error || `class ${f.cls} median ${f.median} outside [${f.band}]`}`);
    for (const k of ['dusk', 'day']) if (!(c.lum[k].max < 1)) fail(`${pre}luminance ${k} max ${c.lum[k].max} ≥ 1 (emissives off)`);
    if (c.lum.dusk.terrainPx && !(c.lum.dusk.p50 > 0.004)) fail(`${pre}dusk terrain p50 luminance ${c.lum.dusk.p50} ≤ 0.004 (too dark)`);
    if (!opt('qs')) (await page.evalJSON('window.__caveLum()')).forEach((l, i) => l.terrainPx && !(l.p50 > 0.004) && fail(`${pre}cave:${i} dusk terrain p50 luminance ${l.p50} ≤ 0.004 (too dark)`));
    if (c.clip.open > 0) {
      const kept = c.clip.cut / c.clip.open;
      if (profile.clipped && kept > 0.02) fail(`${pre}clipped:true but ${pct(kept)} of solid pixels ignore uClip`);
      if (!profile.clipped && kept < 0.98) fail(`${pre}clipped:false but uClip removes ${pct(1 - kept)} of solid pixels`);
    }
    c.physics.forEach((e) => fail(`${pre}physics: ${e}`));
    if (!pre) console.log(`style ok=${!c.style.dusk.length && !c.style.day.length}  lum dusk max=${c.lum.dusk.max} p50=${c.lum.dusk.p50} (${c.lum.dusk.terrainPx} px)  day max=${c.lum.day.max}  clip ${c.clip.open}→${c.clip.cut} px  facet ${c.facet.map((f) => `${f.cls}:${f.median}`).join(' ')}`);
  }
  await finish(cleanup);
}

// ---------------------------------------------------------------- page (real relay)
/** relay-hostile.json if present, else the first relay-*.json, else 'none'. */
function relayFixture(files) {
  const rs = files.filter((f) => /^relay-.*\.json$/.test(f)).sort();
  const f = rs.includes('relay-hostile.json') ? 'relay-hostile.json' : rs[0];
  return f ? join(ROOT, 'dev/fixtures', f) : 'none';
}
async function freePort() { const s = createServer(); await new Promise((r) => s.listen(0, '127.0.0.1', r)); const p = s.address().port; s.close(); return p; }
async function pageMode() {
  const repo = resolve(ROOT, '..'), bin = join(outDir, 'relay');
  const b = spawnSync('go', ['build', '-o', bin, './relay'], { cwd: repo, encoding: 'utf8' });
  if (b.status !== 0) { fail(`go build ./relay: ${b.stderr.trim().split('\n')[0]}`); return finish(); }
  const fx = opt('fixture') || relayFixture(await readdir(join(ROOT, 'dev/fixtures')));
  if (fx === 'none') {
    console.warn('WARN '.repeat(3) + 'page: no www/dev/fixtures/relay-*.json, running the relay with --source=fixture:none (empty world)');
    if (argv.includes('--strict')) { fail('page --strict: no relay fixture (www/dev/fixtures/relay-*.json)'); return finish(); }
  }
  const port = await freePort(), origin = `http://127.0.0.1:${port}`;
  let relay, relayLog = '';
  const start = async () => {
    relay = spawn(bin, ['--dev', `--listen=127.0.0.1:${port}`, '--metrics=127.0.0.1:0', `--source=fixture:${fx}`], { cwd: repo });
    relay.stdout.on('data', (d) => (relayLog += d)); relay.stderr.on('data', (d) => (relayLog += d));
    for (let i = 0; i < 100; i++) { if (await fetch(`${origin}/healthz`).then((r) => r.ok, () => false)) return true; await sleep(100); }
    return false;
  };
  const stop = () => new Promise((r) => { relay.once('exit', r); relay.kill('SIGTERM'); });
  if (!(await start())) { fail(`relay did not answer /healthz: ${relayLog.slice(-300)}`); relay.kill('SIGKILL'); return finish(); }
  console.log(`relay ${bin} --dev --source=fixture:${fx} on ${origin}`);
  // Every built model is on the page: a landed lane left out of layout.page would ship invisible.
  const { page: onPage } = await import(join(ROOT, 'js/layout.js'));
  for (const m of await readdir(join(ROOT, 'models'))) if (!onPage.includes(m)) fail(`page: models/${m}/ is not in layout.page`);

  // CSP: exactly one source (the Go relay); importmap hash, connect-src, no third-party origins
  const res = await fetch(`${origin}/`), csp = res.headers.get('content-security-policy') || '', html = await res.text();
  const dir = Object.fromEntries(csp.split(';').map((d) => d.trim().split(/\s+/)).filter((d) => d[0]).map(([k, ...v]) => [k, v]));
  if (!csp) fail('CSP: no Content-Security-Policy header on /');
  for (const [, body] of html.matchAll(/<script(?: type="importmap")?>([\s\S]*?)<\/script>/g)) {
    const h = `'sha256-${createHash('sha256').update(body).digest('base64')}'`;
    if (!dir['script-src']?.includes(h)) fail(`CSP: script-src lacks the inline script hash ${h}`);
  }
  if (!dir['connect-src']?.includes('wss://mountain.bnei.dev')) fail('CSP: connect-src lacks wss://mountain.bnei.dev');
  if (dir['default-src']?.join(' ') !== "'self'") fail(`CSP: default-src ${dir['default-src']}`);
  if (dir['object-src']?.join(' ') !== "'none'") fail(`CSP: object-src ${dir['object-src']}`);
  const third = [...csp.matchAll(/[a-z][a-z0-9+.-]*:\/\/[^\s;]+/gi)].map((m) => m[0]).filter((u) => u !== 'wss://mountain.bnei.dev' && u !== 'ws://127.0.0.1:*');
  if (third.length) fail(`CSP: third-party origins ${third.join(' ')}`);
  console.log(`CSP ${csp}`);
  await scan(await jsUnder(join(ROOT, 'models')));

  let page;
  try { page = await launch({ w: W, h: H, mobile: W < 600 }); } catch (e) { fail(e.message); relay.kill('SIGKILL'); return finish(); }
  const cleanup = async () => { await page.close(); relay.kill('SIGKILL'); };
  const timer = setTimeout(async () => { fail('global timeout 120 s'); await finish(cleanup); }, 120000);
  let restarting = false;
  // while we restart the relay on purpose, the dropped socket and refused reconnects are expected
  await watch(page, origin, { allow: (s) => restarting && /websocket|ERR_CONNECTION_REFUSED|\/ws/i.test(s) });
  const ws = { created: 0, hello: 0, frames: 0, closed: 0 };
  page.on(({ method, params }) => {
    if (method === 'Network.webSocketCreated') ws.created++;
    if (method === 'Network.webSocketClosed') ws.closed++;
    if (method === 'Network.webSocketFrameReceived') { ws.frames++; if (params.response.payloadData.startsWith('{') && JSON.parse(params.response.payloadData).t === 'hello') ws.hello++; }
  });
  await page.s('Page.navigate', { url: `${origin}/${opt('q') ? `?q=${opt('q')}` : ''}` });
  for (let i = 0; i < 150 && !ws.hello; i++) await sleep(100);
  if (!ws.hello) fail('no hello frame on /ws within 15 s');
  await sleep(3000); // models build, first frames render
  console.log(`page: ${await shot(page, `page-${W}.png`)}  ws created=${ws.created} hello=${ws.hello} frames=${ws.frames}`);

  // post (01 "Post"): desktop composer passes + MSAA 4 + HalfFloat; phones none. Same tone mapping as the direct path:
  // the composer with bloom off (haze is off at uHeat 0) must match a direct render of the same frame, exactly without
  // fog (tone mapping, exposure, colour space) and within 3/255 with it (the composer mixes fog in linear, ≈ 1.6/255
  // measured); at day (exposure 1.25) the composer's fog colour is the raw key colour (kit pre-exposes it on the direct path only).
  const pp = await page.evalJSON(`(() => {
    const p = globalThis.__page; if (!p) return { err: 'no __page (dev hook)' };
    const { renderer: r, scene, camera, post } = p;
    if (!post) return { post: false };
    const gl = r.getContext(), w = gl.drawingBufferWidth, h = gl.drawingBufferHeight, a = new Uint8Array(w * h * 4), b = new Uint8Array(w * h * 4);
    const diff = () => {
      r.render(scene, camera); gl.readPixels(0, 0, w, h, gl.RGBA, gl.UNSIGNED_BYTE, a);
      post.bloom.enabled = false; post.composer.render(); gl.readPixels(0, 0, w, h, gl.RGBA, gl.UNSIGNED_BYTE, b); post.bloom.enabled = true;
      let d = 0; for (let i = 0; i < a.length; i += 4) for (let c = 0; c < 3; c++) d += Math.abs(a[i + c] - b[i + c]);
      return +(d / (w * h * 3)).toFixed(3);
    };
    const fog = scene.fog, withFog = diff(); p.kit.setKey('day'); const dayFog = '#' + fog.color.getHexString() + ' want ' + p.kit.TOKENS.day.rig.fog; p.kit.setKey('dusk'); scene.fog = null; const noFog = diff(); scene.fog = fog;
    const rt = post.composer.renderTarget1;
    return { post: true, passes: post.composer.passes.map((x) => x.constructor.name).join(','), samples: rt.samples, half: rt.texture.type === 1016, threshold: post.bloom.threshold, noFog, withFog, dayFog };
  })()`).catch((e) => ({ err: e.message }));
  if (pp.err) fail(`post: ${pp.err}`);
  else if (opt('q') === 'low') { if (pp.post) fail('post: the low tier must render direct (no composer)'); }
  else if (!pp.post) fail('post: no composer on desktop');
  else {
    if (pp.passes !== 'RenderPass,UnrealBloomPass,ShaderPass,OutputPass' || pp.samples !== 4 || !pp.half || pp.threshold !== 1) fail(`post: ${JSON.stringify(pp)}`);
    if (!(pp.noFog < 0.25)) fail(`post: composer (bloom off, no fog) differs from the direct path by ${pp.noFog}/255 (tone mapping not matched)`);
    if (!(pp.withFog < 3)) fail(`post: composer (bloom off) differs from the direct path by ${pp.withFog}/255 (fog not matched)`);
    const [got, , want] = pp.dayFog.split(' ');
    if (got !== want) fail(`post: day fog ${got} != ${want}: the composer's fog must not be pre-exposed (OutputPass exposes it)`);
  }
  console.log(`post: ${JSON.stringify(pp)}`);

  // relay restart: the socket must reconnect with backoff and resync through a fresh hello
  restarting = true;
  await stop();
  for (let i = 0; i < 50 && !ws.closed; i++) await sleep(100);
  if (!ws.closed) fail('socket not closed after the relay stopped');
  await sleep(1500);
  const staleEl = `(e => e && !e.hidden && e.textContent)(document.getElementById('stale'))`;
  const label = await page.evalJSON(staleEl);
  if (!/^stale since \d\d:\d\d/.test(label || '')) fail(`stale label not shown while the relay is down (got ${JSON.stringify(label)})`);
  if (!(await start())) fail('relay restart failed');
  for (let i = 0; i < 400 && ws.hello < 2; i++) await sleep(100); // ≤ 1 s + 2 s backoff × 1.5 jitter, plus slack
  restarting = false;
  if (ws.hello < 2) fail(`no resync hello after the relay restart (created=${ws.created})`);
  else console.log(`resync: reconnect ok (sockets created=${ws.created}, hello=${ws.hello})  label while down: ${JSON.stringify(label)}`);
  await sleep(300);
  if (await page.evalJSON(staleEl)) fail('stale label still shown after the resync hello');
  await sleep(1000);
  clearTimeout(timer);
  await finish(cleanup);
}

// ---------------------------------------------------------------- styleboard
async function styleboardMode() {
  const srv = await serveStatic();
  let page;
  try { page = await launch({ w: W, h: H }); } catch (e) { fail(e.message); return finish(srv.close); }
  const cleanup = async () => { await page.close(); srv.close(); };
  await watch(page, srv.origin);
  await page.s('Page.navigate', { url: `${srv.origin}/dev/styleboard.html${opt('q') ? `?q=${opt('q')}` : ''}` });
  const b = await waitFor(page, 'window.__board?.ready && window.__board', 60000);
  if (!b) fail('styleboard not ready within 60 s');
  else if (b.error) fail(`styleboard error: ${b.error.split('\n')[0]}`);
  else {
    console.log(`styleboard haze seam (max step /255): ${JSON.stringify(b.haze)}`);
    for (const [k, d] of Object.entries(b.haze)) if (d > 4) fail(`styleboard ${k}: hard horizon line (step ${d}/255 > 4): fog colour ≠ exposed horizon`);
    console.log(`styleboard models=${b.models.join(',')} missing=${b.missing.join(',') || '-'}  ${await shot(page, `styleboard-${W}.png`)}`);
  }
  await sleep(300);
  await finish(cleanup);
}

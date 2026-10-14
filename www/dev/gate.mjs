#!/usr/bin/env node
// Render gate (04). Node >= 22, no dependencies. Port of pi-lab compose/www/dev/gate.mjs, minimal W1a seam
// version: lane B extends it (style checks, writes/emits, pose determinism, page/relay mode, CSP, perf).
//
//   node www/dev/gate.mjs <model>             models/<model>/, profile layout.profiles[<model>], runs test.js
//   node www/dev/gate.mjs fixture:<name>      dev/fixtures/<name>.js (profile _<name> or _test)
//   node www/dev/gate.mjs selftest            every fixture against its expected verdict
// Options: --P 0,3,6  --w 1280  --h 800  --key dusk  --q low  --out <dir>  --qs 'cam=cave:0&az=0.4' (extra sandbox params)
//          (env CHROME, GATE_OUT)
// Checks: build error, contract shape, draws/tris vs profile, coverage, foreign (non-kit) materials,
// shadows, console errors / exceptions / failed or foreign requests, test.js. Exit 0 = PASS, 1 = FAIL.
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, extname, join, normalize, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..'); // www/
const CHROME = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const argv = process.argv.slice(2);
const OPTS = ['--P', '--w', '--h', '--out', '--key', '--q', '--qs'];
const target = argv.find((a, i) => !a.startsWith('--') && !(i > 0 && OPTS.includes(argv[i - 1])));
const opt = (n, d) => { const i = argv.indexOf(`--${n}`); return i >= 0 ? argv[i + 1] : d; };
if (!target) { console.error('usage: gate.mjs <model|fixture:name|selftest> [--P list] [--w px] [--h px] [--key k] [--q low] [--out dir]'); process.exit(2); }

if (target === 'selftest') {
  const want = { cube: 'PASS', terrain: 'PASS', blank: 'FAIL', broken: 'FAIL', foreign: 'FAIL', failtest: 'FAIL' };
  const why = { blank: /nearly blank/, broken: /build error/, foreign: /not from kit/, failtest: /test\.js failed/ };
  let bad = 0;
  for (const [f, v] of Object.entries(want)) {
    const r = spawnSync(process.execPath, [fileURLToPath(import.meta.url), `fixture:${f}`, ...argv.slice(1)], { encoding: 'utf8' });
    const got = r.status === 0 ? 'PASS' : 'FAIL', ok = got === v && (!why[f] || why[f].test(r.stdout));
    console.log(`${ok ? 'ok  ' : 'BAD '} fixture:${f} want ${v} got ${got}`);
    if (!ok) { bad++; console.log(r.stdout + r.stderr); }
  }
  console.log(bad ? `SELFTEST FAIL (${bad})` : 'SELFTEST PASS');
  process.exit(bad ? 1 : 0);
}

const W = Number(opt('w', 1280)), H = Number(opt('h', 800));
const outDir = resolve(opt('out', process.env.GATE_OUT || join(tmpdir(), 'mountain-gate')), target.replace(/[:/]/g, '_'));
const isFixture = target.startsWith('fixture:');
const { profiles } = await import(join(ROOT, 'js/layout.js'));
const profile = isFixture ? profiles[`_${target.slice(8)}`] || profiles._test : profiles[target];
if (!profile) { console.log(`GATE FAIL ${target}: no layout profile`); process.exit(1); }
const Ps = opt('P') ? opt('P').split(',').map(Number) : [0, 3, 6];

// ---------- static server (www/) ----------
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png' };
const server = createServer(async (req, res) => {
  const path = normalize(decodeURIComponent(new URL(req.url, 'http://x').pathname));
  const file = join(ROOT, path.endsWith('/') ? path + 'index.html' : path);
  if (!file.startsWith(ROOT + sep) || !existsSync(file)) { res.writeHead(404).end(); return; }
  res.writeHead(200, { 'content-type': MIME[extname(file)] || 'application/octet-stream', 'cache-control': 'no-store' }).end(await readFile(file));
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const origin = `http://127.0.0.1:${server.address().port}`;

// ---------- chrome (own profile + ports per run, so parallel runs never collide) ----------
const profileDir = await mkdtemp(join(tmpdir(), 'mountain-gate-chrome-'));
const chrome = spawn(CHROME, ['--headless=new', '--remote-debugging-port=0', `--user-data-dir=${profileDir}`, '--no-first-run', '--no-default-browser-check',
  '--hide-scrollbars', '--mute-audio', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', `--window-size=${W},${H}`, 'about:blank'], { stdio: 'ignore' });
const failures = [];
const fail = (m) => failures.push(m);
let ws;
const cleanup = async () => { try { ws?.close(); } catch {} chrome.kill('SIGKILL'); server.close(); await rm(profileDir, { recursive: true, force: true }).catch(() => {}); };
const timeout = setTimeout(async () => { console.log(`GATE FAIL ${target}: global timeout 120 s`); await cleanup(); process.exit(1); }, 120000);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let port;
for (let i = 0; i < 100 && !port; i++) {
  const f = join(profileDir, 'DevToolsActivePort');
  if (existsSync(f)) port = (await readFile(f, 'utf8')).split('\n')[0].trim();
  if (!port) await sleep(100);
}
if (!port) { console.log(`GATE FAIL ${target}: Chrome did not start (${CHROME})`); await cleanup(); process.exit(1); }
const { webSocketDebuggerUrl } = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();

// ---------- minimal CDP client ----------
ws = new WebSocket(webSocketDebuggerUrl);
await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
let nextId = 1;
const pending = new Map(), listeners = [];
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) { const { res, rej } = pending.get(msg.id); pending.delete(msg.id); msg.error ? rej(new Error(msg.error.message)) : res(msg.result); }
  else if (msg.method) listeners.forEach((l) => l(msg));
};
const send = (method, params = {}, sessionId) => new Promise((res, rej) => { const id = nextId++; pending.set(id, { res, rej }); ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) })); });
const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
const s = (m, p) => send(m, p, sessionId);
listeners.push(({ method, params, sessionId: sid }) => {
  if (sid !== sessionId) return;
  if (method === 'Runtime.consoleAPICalled' && (params.type === 'error' || params.type === 'assert')) fail(`console.${params.type}: ${params.args.map((a) => a.value ?? a.description ?? '').join(' ').slice(0, 400)}`);
  if (method === 'Runtime.exceptionThrown') fail(`exception: ${(params.exceptionDetails.exception?.description || params.exceptionDetails.text).slice(0, 400)}`);
  if (method === 'Log.entryAdded' && params.entry.level === 'error') fail(`log error: ${params.entry.text.slice(0, 300)} ${params.entry.url || ''}`);
  if (method === 'Network.requestWillBeSent') { const u = params.request.url; if (!u.startsWith(origin) && !u.startsWith('data:') && !u.startsWith('blob:')) fail(`foreign request: ${u}`); }
  if (method === 'Network.responseReceived' && params.response.status >= 400) fail(`HTTP ${params.response.status}: ${params.response.url}`);
  if (method === 'Network.loadingFailed' && !params.canceled) fail(`request failed: ${params.errorText}`);
});
await s('Runtime.enable'); await s('Log.enable'); await s('Network.enable'); await s('Page.enable');
await s('Emulation.setDeviceMetricsOverride', { width: W, height: H, deviceScaleFactor: 1, mobile: W < 600 });
const evalJSON = async (expr) => {
  const r = await s('Runtime.evaluate', { expression: `(async () => JSON.stringify(await (${expr})))()`, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
  return r.result.value ? JSON.parse(r.result.value) : null;
};

const qs = new URLSearchParams({ m: target, P: String(Ps[0]), key: opt('key', 'dusk'), ...(opt('q') ? { q: opt('q') } : {}), ...Object.fromEntries(new URLSearchParams(opt('qs', ''))) });
await s('Page.navigate', { url: `${origin}/dev/sandbox.html?${qs}` });
let g = null;
for (const t0 = Date.now(); Date.now() - t0 < 30000; await sleep(100)) {
  g = await evalJSON('window.__gate || null').catch(() => null);
  if (g?.ready) break;
}
if (!g?.ready) fail(`not ready within 30 s (${JSON.stringify(g)})`);
await sleep(300); // let late console errors and requests land
await mkdir(outDir, { recursive: true });
const pct = (n) => (n * 100).toFixed(2) + '%';

if (g?.ready && g.error) fail(`build error: ${g.error.split('\n')[0]}`);
else if (g?.ready) {
  for (const P of Ps) {
    const st = await evalJSON(`window.__setP(${P})`);
    const shot = await s('Page.captureScreenshot', { format: 'png' });
    const png = join(outDir, `P${P}-${W}${opt('key') ? '-' + opt('key') : ''}${opt('qs') ? '-' + opt('qs').replace(/[^a-z0-9.]+/gi, '_') : ''}.png`);
    await writeFile(png, Buffer.from(shot.data, 'base64'));
    console.log(`P=${P}  coverage=${pct(st.coverage)}  tris=${st.tris}  draws=${st.draws}  ${png}`);
    const minCov = profile.minCoverage ?? 0.01;
    if (P >= profile.visibleFrom && st.coverage < minCov) fail(`P=${P}: nearly blank (coverage ${pct(st.coverage)} < ${pct(minCov)})`);
    if (st.tris > profile.tris) fail(`P=${P}: triangles ${st.tris} > ${profile.tris}`);
    if (st.draws > profile.draws) fail(`P=${P}: draw calls ${st.draws} > ${profile.draws}`);
    if (st.foreignMaterials.length) fail(`P=${P}: materials not from kit: ${st.foreignMaterials.join(', ')}`);
    if (st.shadows.length) fail(`P=${P}: shadows on: ${st.shadows.join(', ')}`);
  }
  if (g.hasTest) {
    const r = await evalJSON('window.__test()');
    console.log(`test.js: ${r.ok ? 'ok' : 'FAILED'}`);
    if (!r.ok) fail(`test.js failed: ${r.error.split('\n')[0]}`);
  }
}
await sleep(100);
clearTimeout(timeout);
await cleanup();
const uniq = [...new Set(failures)];
if (uniq.length) { console.log(`GATE FAIL ${target}`); uniq.forEach((f) => console.log('  - ' + f)); process.exit(1); }
console.log(`GATE PASS ${target}`);

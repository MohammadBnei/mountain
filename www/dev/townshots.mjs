#!/usr/bin/env node
// Town screenshots (R7 styleboard + visual QA). Node >= 22, no deps. Starts the real relay on a fixture
// (go build ./relay; --dev --source=fixture:<file>) and serves www/ statically, so the page under test is the
// working tree (no rebuild), then drives headless Chrome through scripted views.
//   node www/dev/townshots.mjs [--fixture www/dev/fixtures/relay-town.json] [--out dir] [--only name,name]
// env CHROME (as gate.mjs). Exit 1 on a console error, an exception or a failed request.
import { spawn, spawnSync } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { ROOT, launch, serveStatic, sleep, waitFor } from './cdp.mjs';

const arg = (k, d) => { const i = process.argv.indexOf(`--${k}`); return i > 0 ? process.argv[i + 1] : d; };
const OUT = resolve(arg('out', '/tmp/townshots'));
const FIXTURE = resolve(arg('fixture', join(ROOT, 'dev/fixtures/relay-town.json')));
const ONLY = arg('only', '')?.split(',').filter(Boolean);
await mkdir(OUT, { recursive: true });

const bin = join(OUT, 'relay');
const b = spawnSync('go', ['build', '-o', bin, './relay'], { cwd: resolve(ROOT, '..'), stdio: 'inherit' });
if (b.status) process.exit(1);
const relays = new Map();
let relayLog = '';
async function relayFor(fixture) {
  if (relays.has(fixture)) return relays.get(fixture).port;
  const port = await new Promise((r) => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
  const proc = spawn(bin, ['--dev', `--listen=127.0.0.1:${port}`, '--metrics=127.0.0.1:0', `--source=fixture:${fixture}`], { stdio: ['ignore', 'ignore', 'pipe'] });
  proc.stderr.on('data', (d) => { relayLog += d; });
  relays.set(fixture, { port, proc });
  await sleep(500);
  return port;
}
const stat = await serveStatic();
const bad = [];
// Styleboard states (fixtures/relay-town-states.json): h1's nodes NotReady (its island greys), h2 out of PVE quorum
// (its bridges raised), dream-analyst crashlooping (keeps hopping), a critical alert at vos-monolith; plus, injected
// here, editable-blog in ruins (app.gone needs 10 min of absence, too long for a fixture).
const STATES = `__town.inject({ t: 'world', kind: 'plot', ...__town.world.state.plots.find((p) => p.app === 'editable-blog'), ruinAt: 1792000000000 })`;
const SF = join(ROOT, 'dev/fixtures/relay-town-states.json');
// R8 typology board: h1 cleared, then every current kind on it, one family per row (Industry at the back, Houses at
// the front), the label = the kind. Rows avoid the spokes (r = 0 for q > 0, q = 0 for r > 0).
const KINDS = `(() => {
  const w = __town.world;
  for (const p of w.state.plots.filter((p) => p.host === 'h1')) __town.inject({ t: 'world', kind: 'plot', ...p, gone: true });
  const rows = [[-5, [-1, 1, 3, 5], ['hall', 'forge', 'witness', 'mill']],
    [-3, [-3, -1, 1, 3, 5], ['keep', 'gate', 'guardhouse', 'watchtower', 'vault']],
    [-1, [-5, -3, -1, 1, 3, 5], ['craneyard', 'depot', 'warehouse', 'granary', 'archive', 'cellar']],
    [1, [-5, -3, -1, 1, 3, 5], ['church', 'tavern', 'inn', 'alehouse', 'bookshop', 'playhouse']],
    [3, [-5, -3], ['home', 'house']]];
  for (const [r, qs, kinds] of rows) kinds.forEach((k, i) => __town.inject({ t: 'world', kind: 'plot', app: k, host: 'h1', q: qs[i], r, role: k }));
})()`;
// R8 C annexes (copies of the app's building), as the relay would put them up live: the DaemonSet services (watchtower, warehouse) on both other
// islands, a few apps on one more; outskirts first (the relay's ring order), never on a plot, a plant or a tent.
const TENTS = `(() => {
  const w = __town.world, H = ['h1', 'h2', 'h3'], taken = new Set(w.state.plots.map((p) => p.host + ',' + p.q + ',' + p.r)
    .concat(w.state.marks.map((m) => 'h' + m.pose.h + ',' + m.pose.q + ',' + m.pose.r)));
  const free = (host, key) => { // ring R-1 down to 1 (the relay's tent order), rotated by the key like town.Slot's fnv seed
    const is = w.town.islands.find((i) => i.host === host), R = is.radius, seed = [...key].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7);
    for (let ring = R - 1; ring >= 1; ring--) {
      const out = [];
      let idx = 0;
      for (let r = -R; r <= R; r++) for (let q = Math.max(-R, -r - R); q <= Math.min(R, -r + R); q++, idx++)
        if (is.tiles[idx] === 'g' && Math.max(Math.abs(q), Math.abs(r), Math.abs(q + r)) === ring && !taken.has(host + ',' + q + ',' + r)) out.push({ q, r });
      if (out.length) return out[seed % out.length];
    }
    return null; };
  for (const [app, n] of [['watchtower', 2], ['warehouse', 2], ['gate', 1], ['guardhouse', 1], ['wird', 1], ['vos-monolith', 1], ['dream-analyst', 1]]) {
    const p = w.state.plots.find((x) => x.app === app && !x.ruinAt);
    if (!p) continue;
    for (const host of H.filter((h) => h !== p.host).slice(0, n)) {
      const x = free(host, app + '@' + host);
      if (!x) continue;
      taken.add(host + ',' + x.q + ',' + x.r);
      __town.inject({ t: 'world', kind: 'annex', app, host, q: x.q, r: x.r });
    }
  }
})()`;
// R9 Civ: levels from activity (injected ev level), the technologies (ev tech), a golden age (calm set 9 days back
// on the page, dev only), the herald (a live sync)
const CIV = `(() => {
  const w = __town.world, L = { wird: 3, 'vos-monolith': 2, craneyard: 3, gate: 2, watchtower: 3, 'dream-analyst': 2, 'wedding-2026': 3, depot: 2, mountain: 2, guardhouse: 3 };
  for (const [app, level] of Object.entries(L)) __town.inject({ t: 'ev', id: 'civ' + app, ts: Date.now(), kind: 'level', app, level });
  __town.inject({ t: 'ev', id: 'civtech', ts: Date.now(), kind: 'tech', count: 65, top: 65 });
  w.state.alerts = []; w.state.calm = new Date(Date.now() - 9 * 864e5).toISOString().slice(0, 10); w.emit('state', w.state);
})()`;
const CIV_STEADY = `(() => { // the same town, levels set quietly (no level-up moment): the steady look
  const w = __town.world;
  w.state.levels = { wird: 3, 'vos-monolith': 2, craneyard: 3, gate: 2, watchtower: 3, 'dream-analyst': 2, 'wedding-2026': 3, depot: 2, mountain: 2, guardhouse: 3 };
  w.state.tech = { count: 65, top: 65 }; w.state.alerts = []; w.state.calm = new Date(Date.now() - 9 * 864e5).toISOString().slice(0, 10); w.emit('state', w.state);
})()`;
const HERALD = `__town.inject({ t: 'ev', id: 'civsync', ts: Date.now(), kind: 'argo.sync', app: 'wedding-2026', ok: true })`;
const views = [
  // [name, width, height, setup (in the page), wait seconds]
  ['overview-noon', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('home')`, 2],
  ['overview-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('home')`, 2],
  ['overview-dusk', 1280, 800, `__town.sunAt('2026-10-22T16:20:00Z'); __town.fly('home')`, 2],
  ['overview-night', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); __town.fly('home')`, 2],
  ['island-h3', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('h3')`, 2],
  ['night-h1-lamps', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); __town.fly('h1')`, 3],
  ['walkers-h2', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('vos-monolith')`, 3],
  ['plant-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); document.getElementById('plant').click(); __town.fly('h2')`, 3, undefined, '&role=admin'],
  ['card-wird-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('wird')`, 2],
  ['kinds-h1', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${KINDS}; __town.fly('h1')`, 3],
  ['kinds-h1-dusk', 1280, 800, `__town.sunAt('2026-10-22T16:20:00Z'); ${KINDS}; __town.fly('h1')`, 3],
  // R8 machines: every machine on its island; a guest not answering (the states fixture's witness, held 3 steps);
  // the admin card with a (placeholder) real name; machine labels up close but not in the overview
  ['machines-overview', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('home')`, 2],
  ['machines-h3', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('h3')`, 2, SF],
  ['machines-h2', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('h2')`, 2],
  ['machines-h1-night', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); __town.fly('h1')`, 3],
  ['card-witness-admin', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('witness')`, 2, SF, '&role=admin&names=1'],
  ['card-keep-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); __town.fly('keep-h2')`, 2],
  // R8 C tents: the overview, an island close up (tents named only near the camera), a tent's card (its app's, "also
  // on"), the softer glow at night, a down island's tents greyed with it, the phone
  ['tents-overview', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; __town.fly('home')`, 3],
  ['tents-h3', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; __town.fly('h3')`, 3],
  ['tents-card', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; setTimeout(() => __town.fly(__town.buildings.annexes().find((x) => x.app === 'watchtower').key), 1500)`, 4],
  ['tents-h2-night', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); ${TENTS}; __town.fly('h2')`, 3],
  ['tents-down-h1', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; __town.fly('h1')`, 3, SF],
  ['tents-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; __town.fly('home')`, 3],
  ['tents-card-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); ${TENTS}; setTimeout(() => __town.fly(__town.buildings.annexes().find((x) => x.app === 'wird')?.key || ''), 1500)`, 4],
  // R9 Civ
  ['civ-overview', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; __town.fly('home')`, 7],
  ['civ-h1', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV}; __town.fly('h1')`, 2], // the level-up moment: scaffolds, the herald's decree,
  ['civ-h3-herald', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; __town.fly('h3'); setTimeout(() => { ${HERALD} }, 2500)`, 7],
  ['civ-night', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); ${CIV_STEADY}; __town.fly('h1')`, 7],
  ['civ-tech-card', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; __town.fly('home'); setTimeout(() => [...document.querySelectorAll('#facts button')].find((b) => /technolog/.test(b.textContent))?.click(), 1500)`, 3],
  ['civ-guide', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; __town.fly('home'); setTimeout(() => [...document.querySelectorAll('#facts button')].find((b) => /guide/.test(b.textContent))?.click(), 1500)`, 3],
  ['civ-phone', 390, 844, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; __town.fly('home'); setTimeout(() => { ${HERALD} }, 2500)`, 7],
  ['civ-tents-h3', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; ${TENTS}; __town.fly('h3')`, 7], // the copies follow their app's level
  ['civ-card-wird', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${CIV_STEADY}; setTimeout(() => __town.fly('wird'), 1500)`, 7],
  // the styleboard: every state at once (injected frames on top of the fixture's live ones)
  ['states-overview', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${STATES}; __town.fly('home')`, 3, SF],
  ['states-h2', 1280, 800, `__town.sunAt('2026-10-22T11:00:00Z'); ${STATES}; __town.fly('h2')`, 3, SF],
  ['states-h1-night', 1280, 800, `__town.sunAt('2026-10-22T21:00:00Z'); ${STATES}; __town.fly('h1')`, 3, SF],
  ['states-phone', 390, 844, `__town.sunAt('2026-10-22T16:20:00Z'); ${STATES}; __town.fly('h2')`, 3, SF],
];
try {
  for (const [name, w, h, setup, wait, fixture = FIXTURE, qs = ''] of views) {
    if (ONLY.length && !ONLY.includes(name)) continue;
    const port = await relayFor(fixture);
    const page = await launch({ w, h, mobile: w < 600 });
    page.on(({ method, params }) => {
      if (method === 'Runtime.consoleAPICalled' && (params.type === 'error' || params.type === 'assert')) bad.push(`${name}: console ${params.args.map((a) => a.value ?? a.description).join(' ')}`);
      if (method === 'Runtime.exceptionThrown') bad.push(`${name}: ${params.exceptionDetails.exception?.description || params.exceptionDetails.text}`);
      if (method === 'Network.loadingFailed' && !params.canceled) bad.push(`${name}: request failed ${params.errorText}`);
    });
    await page.s('Runtime.enable'); await page.s('Network.enable'); await page.s('Page.enable');
    await page.s('Page.navigate', { url: `${stat.origin}/index.html?ws=ws://127.0.0.1:${port}/ws${qs}` });
    const ready = await waitFor(page, `globalThis.__town?.ready && (__town.error || __town.info().plots > 0) && true`, 60000);
    if (!ready) { bad.push(`${name}: never ready`); await page.close(); continue; }
    const err = await page.evalJSON('__town.error || null');
    if (err) { bad.push(`${name}: ${err}`); await page.close(); continue; }
    await sleep(6000); // a few fixture steps: sync, alerts, marks
    await page.evalJSON(`(() => { ${setup}; return true; })()`);
    await sleep(wait * 1000);
    const info = await page.evalJSON('__town.info()');
    await writeFile(join(OUT, `${name}.png`), Buffer.from((await page.s('Page.captureScreenshot', { format: 'png' })).data, 'base64'));
    console.log(`${name}: ${JSON.stringify(info)}`);
    await page.close();
  }
} finally {
  for (const r of relays.values()) r.proc.kill();
  stat.close();
}
if (bad.length) { console.log(bad.join('\n')); console.log(relayLog.split('\n').slice(-20).join('\n')); process.exit(1); }
console.log(`SHOTS OK → ${OUT}`);

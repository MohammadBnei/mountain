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
    await page.s('Page.navigate', { url: `${stat.origin}/dev/town/index.html?ws=ws://127.0.0.1:${port}/ws${qs}` });
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

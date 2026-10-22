#!/usr/bin/env node
// Render gate for the live town (R7). Node >= 22, no dependencies. Exit 0 = PASS, 1 = FAIL.
//
//   node www/dev/gate.mjs page [--fixture www/dev/fixtures/relay-town.json] [--out dir]
//   node www/dev/gate.mjs selftest      page on the hostile fixture (relay-hostile.json), plus a leak scan of /ws
//   (env CHROME: a Chrome / Chromium binary)
//
// Runs the real relay binary (go build ./relay; --dev --source=fixture:<file>) and checks what it serves:
//   - / : the CSP (script-src self + the importmap hash only, no third-party origin), nosniff, every asset under
//     the immutable /v/<build>/ prefix;
//   - the page, desktop (1280×800) and phone low tier (390×844, ?q=low): built without a console error, an exception,
//     a failed request or a request to another origin; the town drawn (plots > 0, labels shown) within the page
//     budget (≤ 220 draw calls, ≤ 200k triangles, shadow pass included);
//   - resync: the relay restarts on the same port, the page says "stale since" while it is down and resyncs through
//     the next hello (stale cleared).
import { spawn, spawnSync } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { ROOT, launch, sleep, waitFor } from './cdp.mjs';

const argv = process.argv.slice(2), mode = argv[0];
const opt = (k, d) => { const i = argv.indexOf(`--${k}`); return i > 0 ? argv[i + 1] : d; };
if (mode !== 'page' && mode !== 'selftest') { console.error('usage: gate.mjs page|selftest [--fixture file] [--out dir]'); process.exit(2); }
const OUT = resolve(opt('out', process.env.GATE_OUT || '/tmp/mountain-gate/town'));
const FIXTURE = resolve(opt('fixture', join(ROOT, `dev/fixtures/${mode === 'selftest' ? 'relay-hostile' : 'relay-town'}.json`)));
const fails = [];
const fail = (m) => { fails.push(m); console.log(`FAIL ${m}`); };
const note = (m) => console.log(m);
await mkdir(OUT, { recursive: true });

const repo = resolve(ROOT, '..'), bin = join(OUT, 'relay');
const b = spawnSync('go', ['build', '-o', bin, './relay'], { cwd: repo, encoding: 'utf8' });
if (b.status !== 0) { console.log(`FAIL go build ./relay: ${b.stderr.trim().split('\n')[0]}`); process.exit(1); }
const port = await new Promise((r) => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
const origin = `http://127.0.0.1:${port}`;
let relay, relayLog = '';
async function startRelay() {
  relay = spawn(bin, ['--dev', `--listen=127.0.0.1:${port}`, '--metrics=127.0.0.1:0', `--source=fixture:${FIXTURE}`], { cwd: repo, env: { ...process.env, VISITORS_ENABLED: '' } });
  relay.stderr.on('data', (d) => { relayLog += d; });
  for (let i = 0; i < 50; i++) { try { if ((await fetch(`${origin}/healthz`)).ok) return; } catch {} await sleep(100); }
  fail('relay did not answer /healthz');
}
const stopRelay = () => new Promise((r) => { relay.once('exit', r); relay.kill(); });

try {
  await startRelay();
  note(`relay ${bin} --dev --source=fixture:${FIXTURE} on ${origin}`);

  // ---- / : headers, versioned assets
  const res = await fetch(`${origin}/`), html = await res.text(), csp = res.headers.get('content-security-policy') || '';
  note(`CSP ${csp}`);
  if (!/script-src 'self' 'sha256-[A-Za-z0-9+/=]+'(;|$)/.test(csp)) fail('CSP: script-src must be self + the importmap hash only');
  if (/https?:\/\/(?!127\.0\.0\.1)/.test(csp.replace('wss://mountain.bnei.dev', ''))) fail('CSP: a third-party origin');
  if (res.headers.get('x-content-type-options') !== 'nosniff') fail('nosniff missing on /');
  const prefix = html.match(/"\/v\/([0-9a-f]{12})\//)?.[1];
  if (!prefix || /"\.\//.test(html)) fail('index: every asset must be under /v/<build>/');

  // ---- the page, desktop then phone low tier
  for (const [name, w, h, qs] of [['desktop', 1280, 800, ''], ['phone-low', 390, 844, '?q=low']]) {
    const page = await launch({ w, h, mobile: w < 600 });
    const urls = {};
    page.on(({ method, params }) => {
      if (method === 'Network.requestWillBeSent') {
        urls[params.requestId] = params.request.url;
        const u = new URL(params.request.url);
        if (!['http:', 'ws:', 'data:', 'blob:'].includes(u.protocol) || (u.protocol !== 'data:' && u.protocol !== 'blob:' && u.host !== `127.0.0.1:${port}`)) fail(`${name}: request to another origin ${params.request.url}`);
      }
      if (method === 'Runtime.consoleAPICalled' && (params.type === 'error' || params.type === 'assert')) fail(`${name}: console.${params.type} ${params.args.map((a) => a.value ?? a.description).join(' ').slice(0, 300)}`);
      if (method === 'Runtime.exceptionThrown') fail(`${name}: ${(params.exceptionDetails.exception?.description || params.exceptionDetails.text).slice(0, 300)}`);
      if (method === 'Network.loadingFailed' && !params.canceled && params.errorText !== 'net::ERR_ABORTED') fail(`${name}: request failed ${params.errorText} ${urls[params.requestId]}`);
      if (method === 'Network.responseReceived' && params.response.status >= 400 && !/\/auth\/me$/.test(params.response.url)) fail(`${name}: HTTP ${params.response.status} ${params.response.url}`);
    });
    for (const m of ['Runtime.enable', 'Network.enable', 'Page.enable']) await page.s(m);
    await page.s('Page.navigate', { url: `${origin}/${qs}` });
    const ready = await waitFor(page, 'globalThis.__town?.ready && (globalThis.__town.error || globalThis.__town.info().plots > 0) && true', 60000);
    const err = ready && (await page.evalJSON('globalThis.__town.error || null'));
    if (!ready || err) { fail(`${name}: town never built ${err || ''}`); await page.close(); continue; }
    await sleep(3000);
    const info = await page.evalJSON(`({ ...__town.info(), labels: [...document.querySelectorAll('.label')].filter((l) => !l.hidden).length, quality: globalThis.__quality })`);
    note(`${name}: ${JSON.stringify(info)}`);
    if (info.draws > 220 || info.tris > 200000) fail(`${name}: over the page budget (${info.draws} draws / 220, ${info.tris} tris / 200k)`);
    if (!info.labels) fail(`${name}: no building label shown`);
    if (qs && info.quality !== 'low') fail(`${name}: ?q=low did not select the low tier`);
    await writeFile(join(OUT, `${name}.png`), Buffer.from((await page.s('Page.captureScreenshot', { format: 'png' })).data, 'base64'));

    if (name === 'desktop') { // resync through a relay restart
      await stopRelay();
      const down = await waitFor(page, `!document.getElementById('stale').hidden && document.getElementById('stale').textContent`, 10000);
      if (!down || !/stale since/i.test(down)) fail(`resync: no "stale since" while the relay is down (${down})`);
      await startRelay();
      const back = await waitFor(page, 'globalThis.__town.world.stale === null && document.getElementById("stale").hidden', 40000);
      note(`resync: ${down} → ${back ? 'resynced' : 'NOT resynced'}`);
      if (!back) fail('resync: the page did not resync after the relay came back');
    }
    await page.close();
  }

  // ---- selftest: nothing hostile reaches /ws (the hostile fixture's IPs, nodes, namespaces, product names)
  if (mode === 'selftest') {
    const hello = await new Promise((r, j) => {
      const ws = new WebSocket(`ws://127.0.0.1:${port}/ws`, { headers: { origin } });
      ws.onmessage = (e) => { ws.close(); r(String(e.data)); };
      ws.onerror = () => j(new Error('ws error'));
    }).catch((e) => { fail(`selftest: /ws ${e.message}`); return ''; });
    // (a storage pool's class, longhorn | local, is public since ADR-0056: not a leak)
    for (const leak of ['10.0.3', '10.233', 'k8s-', 'bnei.lan', 'traefik', 'authentik', 'argocd', 'grafana', 'prometheus', 'infisical', 'has-journal', 'adminweb', '<script>'])
      if (hello.includes(leak)) fail(`selftest: "${leak}" in the hello`);
    note(`selftest: hello ${hello.length} bytes scanned`);
  }
} finally {
  relay?.kill();
}
if (fails.length) { console.log(relayLog.split('\n').slice(-15).join('\n')); console.log(`GATE FAIL (${fails.length})`); process.exit(1); }
console.log(`GATE PASS → ${OUT}`);

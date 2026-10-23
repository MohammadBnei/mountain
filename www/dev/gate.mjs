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
//   - R7.1, desktop: 70 firing alerts stay within budget with no GL error; a building moved after a first tap is
//     still tappable at its new place (its card opens); a model that fails to load leaves the town drawn.
//   - selftest: every /ws frame of a fixture run (not only the hello) is scanned for leaks.
//   - R8: the alert storm runs on both tiers; the moved building is never a machine (they never move); the leak scan
//     adds every machine's real name, probe and node, read from relay/config.yaml at run time (never written in www/),
//     the machine words and ports, and any IPv4 address, case-insensitively.
import { spawn, spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
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
// R8: the machines, from relay/config.yaml (key + the identity that must never reach the wire)
const MACHINES = [...readFileSync(resolve(ROOT, '../relay/config.yaml'), 'utf8').matchAll(/^\s+([a-z0-9-]+):\s*\{[^}]*group: machine[^}]*\}/gm)]
  .map(([line, key]) => ({ key, words: [...line.matchAll(/\b(?:name|probe|node): ([^,}\s]+)/g)].map((m) => m[1].toLowerCase()) }));
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
  const BLOCK = '*building_tavern_blue.glb'; // the GLB-failure run: the tavern's model never loads
  for (const [name, w, h, qs] of [['desktop', 1280, 800, ''], ['phone-low', 390, 844, '?q=low'], ...(mode === 'page' ? [['glb-blocked', 1280, 800, '']] : [])]) {
    const page = await launch({ w, h, mobile: w < 600 });
    const urls = {}, blocked = name === 'glb-blocked';
    page.on(({ method, params }) => {
      if (method === 'Network.requestWillBeSent') {
        urls[params.requestId] = params.request.url;
        const u = new URL(params.request.url);
        if (!['http:', 'ws:', 'data:', 'blob:'].includes(u.protocol) || (u.protocol !== 'data:' && u.protocol !== 'blob:' && u.host !== `127.0.0.1:${port}`)) fail(`${name}: request to another origin ${params.request.url}`);
      }
      if (method === 'Runtime.consoleAPICalled' && (params.type === 'error' || params.type === 'assert')) fail(`${name}: console.${params.type} ${params.args.map((a) => a.value ?? a.description).join(' ').slice(0, 300)}`);
      if (method === 'Runtime.exceptionThrown') fail(`${name}: ${(params.exceptionDetails.exception?.description || params.exceptionDetails.text).slice(0, 300)}`);
      if (method === 'Network.loadingFailed' && !params.canceled && params.errorText !== 'net::ERR_ABORTED' && !(blocked && params.blockedReason)) fail(`${name}: request failed ${params.errorText} ${urls[params.requestId]}`);
      if (method === 'Network.responseReceived' && params.response.status >= 400 && !/\/auth\/me$/.test(params.response.url)) fail(`${name}: HTTP ${params.response.status} ${params.response.url}`);
    });
    for (const m of ['Runtime.enable', 'Network.enable', 'Page.enable']) await page.s(m);
    if (blocked) await page.s('Network.setBlockedURLs', { urls: [BLOCK] });
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
    if (blocked) { // the tavern still stands (drawn as a home), every other building too
      const n = await page.evalJSON(`__town.world.state.plots.filter((p) => !p.ruinAt).length`);
      if (info.plots !== n || !n) fail(`glb-blocked: ${info.plots} of ${n} buildings drawn`);
      await page.close();
      continue;
    }
    await writeFile(join(OUT, `${name}.png`), Buffer.from((await page.s('Page.captureScreenshot', { format: 'png' })).data, 'base64'));

    if (name === 'desktop') { // R7.1: a moved building stays tappable; 70 alerts stay within budget
      const tapAt = async (app) => { // the building's screen point, then a real click there
        const pt = await page.evalJSON(`(() => { const p = __town.buildings.plots().find((x) => x.app === ${JSON.stringify(app)}); if (!p) return null;
          const v = new (__town.camera.position.constructor)(p.x, p.k.height * 0.5, p.z).project(__town.camera), r = __town.renderer.domElement.getBoundingClientRect();
          return { x: r.left + (v.x + 1) / 2 * r.width, y: r.top + (1 - v.y) / 2 * r.height }; })()`);
        if (!pt) return null;
        for (const type of ['mousePressed', 'mouseReleased']) await page.s('Input.dispatchMouseEvent', { type, x: pt.x, y: pt.y, button: 'left', clickCount: 1 });
        return waitFor(page, `!document.getElementById('card').hidden && document.querySelector('#card h2').textContent`, 3000);
      };
      const mover = await page.evalJSON(`(() => { const m = ${JSON.stringify(MACHINES.map((x) => x.key))}; // a machine never moves
        const ps = __town.world.state.plots.filter((p) => !p.ruinAt && p.host !== 'h3' && !m.includes(p.app)); return ps[0]?.app; })()`);
      const name0 = mover && await tapAt(mover);
      if (!name0) fail(`tap: the card of ${mover} did not open`);
      await page.evalJSON(`(() => { // move it to a free grass hex on h3, as a world:plot from the relay would
        const w = __town.world, is = w.town.islands.find((i) => i.host === 'h3'), R = is.radius, p = w.state.plots.find((x) => x.app === ${JSON.stringify(mover)});
        let idx = 0, hex = null;
        for (let r = -R; r <= R; r++) for (let q = Math.max(-R, -r - R); q <= Math.min(R, -r + R); q++, idx++)
          if (!hex && is.tiles[idx] === 'g' && !w.state.plots.some((x) => x.host === 'h3' && x.q === q && x.r === r) && !w.state.marks.some((m) => m.pose.h === 3 && m.pose.q === q && m.pose.r === r)) hex = { q, r };
        __town.inject({ t: 'world', kind: 'plot', ...p, host: 'h3', q: hex.q, r: hex.r });
        __town.fly(''); return true; })()`);
      await sleep(2500); // the drop + the fly back to the overview
      const name1 = await tapAt(mover);
      note(`tap: ${mover} → "${name0}", moved to h3 → ${name1 ? `"${name1}"` : 'NO CARD'}`);
      if (!name1) fail(`tap: ${mover} moved to h3 can no longer be tapped`);
    }
    if (name === 'desktop' || name === 'phone-low') { // R8 C: every app's tents on both tiers (≤ 32), all drawn, and the alerts below on top
      const n = await page.evalJSON(`(() => { const w = __town.world, m = ${JSON.stringify(MACHINES.map((x) => x.key))}, taken = new Set(
          w.state.plots.map((p) => p.host + ',' + p.q + ',' + p.r).concat(w.state.marks.map((k) => 'h' + k.pose.h + ',' + k.pose.q + ',' + k.pose.r)));
        let n = 0;
        for (const p of w.state.plots) {
          if (p.ruinAt || m.includes(p.app)) continue;
          for (const is of w.town.islands) {
            if (is.host === p.host) continue;
            const R = is.radius; let idx = 0, hex = null;
            for (let r = -R; r <= R; r++) for (let q = Math.max(-R, -r - R); q <= Math.min(R, -r + R); q++, idx++)
              if (!hex && is.tiles[idx] === 'g' && !taken.has(is.host + ',' + q + ',' + r)) hex = { q, r };
            if (!hex) continue;
            taken.add(is.host + ',' + hex.q + ',' + hex.r);
            __town.inject({ t: 'world', kind: 'annex', app: p.app, host: is.host, q: hex.q, r: hex.r }); n++;
          }
        }
        __town.fly(''); return n; })()`);
      await sleep(1500);
      const ti = await page.evalJSON('__town.info()');
      note(`tents ×${n}: ${JSON.stringify(ti)}`);
      if (!n || ti.annexes !== n || (mode !== 'selftest' && n < 20)) fail(`tents: ${ti.annexes} drawn of ${n} injected`);
      if (ti.draws > 220 || ti.tris > 200000) fail(`tents: over the page budget (${ti.draws} draws, ${ti.tris} tris)`);
      if (name === 'desktop') { // a tap on a tent opens its app's card ("also on …") and the camera stays on the tent
        let t0 = null, pt = null; // the first tent in clear view once the camera is there (a big building may hide one)
        for (let i = 0; i < 8 && !pt; i++) {
          t0 = await page.evalJSON(`(() => { const x = __town.buildings.annexes()[${i}]; __town.fly(x.key); return { key: x.key, host: x.host, x: x.x, z: x.z }; })()`);
          await sleep(400);
          await page.evalJSON(`document.getElementById('cardclose').click()`);
          await sleep(500); // the dock shrinks with the card and the camera refits: project after that
          pt = await page.evalJSON(`(async () => { const THREE = await import('three'), T = __town, x = T.buildings.annexes().find((y) => y.key === ${JSON.stringify(t0.key)});
            const v = new THREE.Vector3(x.x, x.k.height * 0.4, x.z).project(T.camera), r = T.renderer.domElement.getBoundingClientRect();
            const rc = new THREE.Raycaster(); rc.setFromCamera(new THREE.Vector2(v.x, v.y), T.camera);
            const hit = rc.intersectObjects(T.buildings.pickables(), false)[0];
            return hit && T.buildings.annexOf(hit)?.key === x.key ? { x: r.left + (v.x + 1) / 2 * r.width, y: r.top + (1 - v.y) / 2 * r.height } : null; })()`);
        }
        if (!pt) fail('tent tap: no tent in clear view');
        else for (const type of ['mousePressed', 'mouseReleased']) await page.s('Input.dispatchMouseEvent', { type, x: pt.x, y: pt.y, button: 'left', clickCount: 1 });
        const card = await waitFor(page, `!document.getElementById('card').hidden && document.querySelector('#card .state').textContent`, 3000);
        await sleep(1200);
        const at = await page.evalJSON(`(() => { const c = __town.controls.target; return Math.hypot(c.x - ${t0.x}, c.z - ${t0.z}); })()`);
        note(`tent tap: ${t0.key} → "${card}", camera ${at.toFixed(2)} from the tent`);
        if (!card || !card.includes(`also on ${t0.host}`)) fail(`tent tap: the card of ${t0.key} does not say "also on ${t0.host}" first (${card})`);
        if (at > 0.5) fail(`tent tap: the camera is ${at.toFixed(2)} from the tent`);
        await page.evalJSON(`(document.getElementById('cardclose').click(), __town.fly(''))`);
        await sleep(800);
      }
    }
    if (name === 'desktop' || name === 'phone-low') { // 70 alerts stay within budget, on both tiers (R8)
      await page.evalJSON(`(__town.fly(''), Array.from({ length: 70 }, (_, i) => __town.inject({ t: 'ev', kind: 'alert.fire', id: 'gate' + i, ts: Date.now(),
        fp: (0x1000000000000000n + BigInt(i)).toString(16), sev: ['info', 'warning', 'critical'][i % 3], layer: 'apps', startsAt: Date.now() })).length)`);
      await sleep(1500);
      const storm = await page.evalJSON('__town.info()');
      const glErr = await page.evalJSON(`(() => { const gl = __town.renderer.getContext(); return gl.getError(); })()`);
      note(`alerts ×70: ${JSON.stringify(storm)} gl ${glErr}`);
      if (storm.draws > 220 || storm.tris > 200000) fail(`alerts ×70: over the page budget (${storm.draws} draws, ${storm.tris} tris)`);
      if (glErr) fail(`alerts ×70: WebGL error ${glErr}`);
      const led = await page.evalJSON(`[__town.world.state.alerts.length, document.getElementById('facts').textContent]`);
      if (led[0] < 70 || !led[1].includes(`${led[0]} alerts`)) fail(`alerts ×70: the LED says "${led[1]}" for ${led[0]} alerts`);
    }
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
  if (mode === 'selftest') { // every frame for 20 s of the fixture run (its steps are 2 s apart), from a fresh relay
    await stopRelay(); await startRelay();
    const frames = await new Promise((r, j) => {
      const ws = new WebSocket(`ws://127.0.0.1:${port}/ws`, { headers: { origin } }), got = [];
      ws.onmessage = (e) => got.push(String(e.data));
      ws.onerror = () => j(new Error('ws error'));
      setTimeout(() => { ws.close(); r(got); }, 20000);
    }).catch((e) => { fail(`selftest: /ws ${e.message}`); return []; });
    const all = frames.join('\n').toLowerCase();
    // R7.1: no product name of a platform service either (the wire carries role keys: gate, guardhouse, warehouse, …)
    // R8: no machine identity (names, probes and nodes from config.yaml), no machine word or port, no IPv4 at all
    for (const leak of ['10.0.3', '10.233', 'k8s-', 'bnei.lan', 'traefik', 'authentik', 'argocd', 'grafana', 'prometheus', 'infisical',
      'longhorn', 'garage', 'zot', 'pigsty', 'has-journal', 'adminweb', '<script>',
      'guest-', 'k9s', 'pg-', 'pg0', 'patroni', 'etcd', 'postgres', 'blackbox', 'probe_success', 'instance', 'hermes', 'runner', 'lxc',
      '.mesh', 'laptop', 'server1', ':8008', ':2379', ':3900', ':2049', ':9119', ...MACHINES.flatMap((m) => m.words)])
      if (all.includes(leak)) fail(`selftest: "${leak}" on /ws: ${all.slice(Math.max(0, all.indexOf(leak) - 120), all.indexOf(leak) + 40)}`);
    const ip = all.match(/\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/);
    if (ip) fail(`selftest: an IPv4 address on /ws: ${ip[0]}`);
    if (MACHINES.length !== 12) fail(`selftest: read ${MACHINES.length} machines from config.yaml, want 12`);
    if (frames.length < 5 || !frames.some((f) => f.includes('"t":"metrics"'))) fail(`selftest: only ${frames.length} frames in 20 s`);
    note(`selftest: ${frames.length} frames, ${all.length} bytes scanned`);
  }
} finally {
  relay?.kill();
}
if (fails.length) { console.log(relayLog.split('\n').slice(-15).join('\n')); console.log(`GATE FAIL (${fails.length})`); process.exit(1); }
console.log(`GATE PASS → ${OUT}`);

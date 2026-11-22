#!/usr/bin/env node
// Render gate for atlas (R10 A2). Node >= 22, no dependencies. Exit 0 = PASS, 1 = FAIL.
//
//   node www/dev/gate.mjs page       the page on the fixtures (dev/fixtures/graph.json + live.json)
//   node www/dev/gate.mjs selftest   the page, plus a hostile graph: nothing from the graph is ever parsed as HTML
//   (env CHROME: a Chrome / Chromium binary)
//
// Runs the real binary (go build ./cmd/atlas; --dev on loopback with sign-in unset, so every route is open) and checks:
//   - / : the CSP (script-src self + the importmap hash only, no third-party origin), nosniff, every asset under the
//     immutable /v/<build>/ prefix;
//   - each level, desktop (1280×800) and phone low tier (390×844, ?q=low): drawn without a console error, an exception,
//     a failed request or a request to another origin, within the per-level budget (plan/10: ≤ 220 draw calls;
//     ≤ 350k triangles high, ≤ 200k phone-low), with its labels, panel and minimap;
//   - both tiers: the reader on a hostile markdown file builds no element from its markup, links only in-atlas or out
//     with no referrer, and redacts the secrets in it;
//   - desktop: a click on an island opens its realm (L1), search finds a node and Enter opens its card, Escape zooms
//     out one step; in a city a click picks a building and a second enters it (L3), a click on a wall focuses its card
//     (the only one taking the pointer), Escape unfocuses it, a governs link opens the ADR (L4) and Escape climbs
//     back to the building and the district; 100 firing alerts stay within budget; the page says the connection is lost while atlas is down
//     and recovers through the next hello.
import { spawn, spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { launch, ROOT, sleep, waitFor } from './cdp.mjs';

const mode = process.argv[2];
if (mode !== 'page' && mode !== 'selftest') {
  console.error('usage: gate.mjs page|selftest');
  process.exit(2);
}
const OUT = resolve(process.env.GATE_OUT || '/tmp/mountain-gate/atlas');
const fails = [];
const fail = m => {
  fails.push(m);
  console.log(`FAIL ${m}`);
};
const note = m => console.log(m);
await mkdir(OUT, { recursive: true });

// the hostile graph: markup and script in every string a node can carry (selftest)
const HOSTILE = '<img src=x onerror="globalThis.__pwned=1"><script>globalThis.__pwned=1</script>';
let graphFile = join(ROOT, 'dev/fixtures/graph.json');
if (mode === 'selftest') {
  const g = JSON.parse(readFileSync(graphFile, 'utf8'));
  const adr = g.nodes.find(n => n.kind === 'adr');
  adr.attrs.title = HOSTILE;
  const app = g.nodes.find(n => n.id === 'app:wird');
  app.attrs = { ...app.attrs, note: HOSTILE, link: 'javascript:globalThis.__pwned=1' };
  app.source.path = '../../x"><img src=x onerror=1>';
  g.drift.push({
    kind: 'unresolved',
    subject: 'app:wird',
    detail: HOSTILE,
    source: { repo: 'x', path: '' }
  });
  graphFile = join(OUT, 'hostile-graph.json');
  await writeFile(graphFile, JSON.stringify(g));
}

const repo = resolve(ROOT, '..'),
  bin = join(OUT, 'atlas');
const b = spawnSync('go', ['build', '-o', bin, './cmd/atlas'], { cwd: repo, encoding: 'utf8' });
if (b.status !== 0) {
  console.log(`FAIL go build ./cmd/atlas: ${b.stderr.trim().split('\n')[0]}`);
  process.exit(1);
}
const port = await new Promise(r => {
  const s = createServer().listen(0, '127.0.0.1', () => {
    const p = s.address().port;
    s.close(() => r(p));
  });
});
const origin = `http://127.0.0.1:${port}`;
let srv,
  log = '';
async function start() {
  srv = spawn(
    bin,
    [
      '--dev',
      `--listen=127.0.0.1:${port}`,
      '--metrics=127.0.0.1:0',
      `--graph-fixture=${graphFile}`,
      `--live-fixture=${join(ROOT, 'dev/fixtures/live.json')}`,
      `--repos=${join(ROOT, 'dev/fixtures/repos')}`
    ],
    {
      cwd: repo,
      env: { ...process.env, MOUNTAIN_OIDC_ISSUER: '', MOUNTAIN_FORGE_TOKEN: '', DATABASE_URL: '' }
    }
  );
  srv.stderr.on('data', d => {
    log += d;
  });
  srv.stdout.on('data', d => {
    log += d;
  }); // atlas's JSON log lines
  for (let i = 0; i < 50; i++) {
    try {
      if ((await fetch(`${origin}/healthz`)).ok) return;
    } catch {}
    await sleep(100);
  }
  fail('atlas did not answer /healthz');
}
const stop = () =>
  new Promise(r => {
    srv.once('exit', r);
    srv.kill();
  });

// [name, hash, the labels it must show at least]
const VIEWS = [
  ['map', '#/', 7],
  ['land', '#/?lens=land', 7],
  ['routes', '#/L1', 7],
  ['realm', '#/L1/fleet', 7],
  ['card', '#/L1/platform?g=nodes&sel=vm:k8s-worker-02', 7],
  ['city', '#/L2/operations', 4],
  ['district', '#/L2/operations?g=observability', 6],
  ['citylens', '#/L2/apps?lens=land', 3],
  ['building', '#/L3/svc:agent-fleet/core', 1],
  ['reader', '#/L4/infra-bootstrap/docs/hostile.md', 0]
];

try {
  await start();
  note(`atlas ${bin} --dev on ${origin}, graph ${graphFile}`);

  const res = await fetch(`${origin}/`),
    html = await res.text(),
    csp = res.headers.get('content-security-policy') || '';
  note(`CSP ${csp}`);
  if (!/script-src 'self' 'sha256-[A-Za-z0-9+/=]+'(;|$)/.test(csp))
    fail('CSP: script-src must be self + the importmap hash only');
  if (!/style-src 'self';/.test(csp)) fail('CSP: style-src must be self only (no inline style)');
  if (/https?:\/\/(?!127\.0\.0\.1)/.test(csp.replace('wss://mountain.bnei.dev', '')))
    fail('CSP: a third-party origin');
  if (res.headers.get('x-content-type-options') !== 'nosniff') fail('nosniff missing on /');
  if (!html.match(/"\/v\/([0-9a-f]{12})\//) || /"\.\//.test(html))
    fail('index: every asset must be under /v/<build>/');

  for (const [tier, w, h, qs] of [
    ['desktop', 1280, 800, ''],
    ['phone-low', 390, 844, '?q=low']
  ]) {
    const TRIS = qs ? 200000 : 350000;
    const page = await launch({ w, h, mobile: w < 600 });
    const urls = {};
    page.on(({ method, params }) => {
      if (method === 'Network.requestWillBeSent') {
        urls[params.requestId] = params.request.url;
        const u = new URL(params.request.url);
        if (
          !['http:', 'ws:', 'data:', 'blob:'].includes(u.protocol) ||
          (!['data:', 'blob:'].includes(u.protocol) && u.host !== `127.0.0.1:${port}`)
        )
          fail(`${tier}: request to another origin ${params.request.url}`);
      }
      if (
        method === 'Runtime.consoleAPICalled' &&
        (params.type === 'error' || params.type === 'assert') &&
        !/WebSocket|connection/i.test(JSON.stringify(params.args))
      )
        fail(
          `${tier}: console.${params.type} ${params.args
            .map(a => a.value ?? a.description)
            .join(' ')
            .slice(0, 300)}`
        );
      if (method === 'Runtime.exceptionThrown')
        fail(
          `${tier}: ${(params.exceptionDetails.exception?.description || params.exceptionDetails.text).slice(0, 300)}`
        );
      if (
        method === 'Network.loadingFailed' &&
        !params.canceled &&
        params.errorText !== 'net::ERR_ABORTED' &&
        !/\/ws$/.test(urls[params.requestId] || '')
      )
        fail(`${tier}: request failed ${params.errorText} ${urls[params.requestId]}`);
      if (method === 'Network.responseReceived' && params.response.status >= 400)
        fail(`${tier}: HTTP ${params.response.status} ${params.response.url}`);
    });
    for (const m of ['Runtime.enable', 'Network.enable', 'Page.enable']) await page.s(m);
    await page.s('Page.navigate', { url: `${origin}/${qs}${VIEWS[0][1]}` });
    const ready = await waitFor(
      page,
      'globalThis.__atlas?.ready && (globalThis.__atlas.error || globalThis.__atlas.stats) && true',
      60000
    );
    const err = ready && (await page.evalJSON('globalThis.__atlas.error || null'));
    if (!ready || err) {
      fail(`${tier}: the map was never drawn ${err || ''}`);
      await page.close();
      continue;
    }
    for (const [name, hash, minLabels] of VIEWS) {
      await page.evalJSON(`(location.hash = ${JSON.stringify(hash)}, true)`);
      await sleep(1800); // the fly-in, then a few frames
      const info =
        await page.evalJSON(`({ ...__atlas.stats(), labels: [...document.querySelectorAll('.lbl')].filter((l) => !l.hidden && l.offsetParent).length,
        panel: document.getElementById('panel').textContent.length, reader: document.getElementById('reader').hidden ? 0 : document.getElementById('reader').textContent.length,
        minimap: document.querySelectorAll('#minimap polygon').length, still: !!document.querySelector('#minimap img'), cards: document.querySelectorAll('#css3d .card3d').length,
        level: document.getElementById('level').textContent, pwned: globalThis.__pwned || 0 })`);
      note(`${tier} ${name}: ${JSON.stringify(info)}`);
      if (info.calls > 220 || info.triangles > TRIS)
        fail(
          `${tier} ${name}: over the budget (${info.calls} draws / 220, ${info.triangles} tris / ${TRIS / 1000}k)`
        );
      if (info.labels < minLabels)
        fail(`${tier} ${name}: ${info.labels} labels shown, want ≥ ${minLabels}`);
      if (name === 'reader' ? !info.reader || !info.still : !info.panel || info.minimap !== 7)
        fail(
          `${tier} ${name}: panel ${info.panel}, reader ${info.reader} chars, minimap ${info.minimap} realms, still ${info.still}`
        );
      if (name === 'building' && (qs ? info.cards !== 0 : info.cards < 2 || info.cards > 7))
        fail(`${tier} building: ${info.cards} wall cards (flat on phones, 2..7 on desktop)`);
      if (info.pwned) fail(`${tier} ${name}: graph text ran as script`);
      if (tier === 'desktop')
        await writeFile(
          join(OUT, `${name}.png`),
          Buffer.from((await page.s('Page.captureScreenshot', { format: 'png' })).data, 'base64')
        );
    }
    {
      // the reader on a hostile document, both tiers: markup is text, links are hash routes or external, secrets redacted
      await page.evalJSON(`(location.hash = '#/L4/infra-bootstrap/docs/hostile.md', true)`);
      const r = await waitFor(
        page,
        `document.getElementById('reader').textContent.includes('hostile') && (() => { const b = document.getElementById('reader');
        return { bad: b.querySelectorAll('img:not(#readstill img), script, div[onclick], b').length, js: [...b.querySelectorAll('a')].filter((a) => !/^(#\\/|https:\\/\\/)/.test(a.getAttribute('href') || '')).length,
          ext: [...b.querySelectorAll('a[target]')].every((a) => a.rel.includes('noopener') && a.rel.includes('noreferrer')), redacted: (b.textContent.match(/\\[redacted\\]/g) || []).length,
          secret: /ghp_|hunter2|Ab3dEf7h/.test(b.textContent), sibling: !!b.querySelector('a[href^="#/L4/infra-bootstrap/docs/other.md"]'), out: /etc\\/passwd/.test([...b.querySelectorAll('a')].map((a) => a.href).join()),
          pwned: globalThis.__pwned || 0 }; })()`,
        8000
      );
      note(`${tier} hostile reader: ${JSON.stringify(r)}`);
      if (
        !r ||
        r.bad ||
        r.js ||
        !r.ext ||
        r.redacted < 3 ||
        r.secret ||
        !r.sibling ||
        r.out ||
        r.pwned
      )
        fail(`${tier} hostile reader: ${JSON.stringify(r)}`);
    }
    if (mode === 'selftest') {
      // every hostile string shows as text: no element built from it, no javascript: link
      await page.evalJSON(`(location.hash = '#/L1/apps?g=products&sel=app:wird', true)`);
      await sleep(800);
      const h =
        await page.evalJSON(`(() => { const p = document.getElementById('panel'); return { imgs: p.querySelectorAll('img, script').length,
        text: p.textContent.includes(${JSON.stringify(HOSTILE)}), js: [...document.querySelectorAll('a')].filter((a) => /^\\s*javascript:/i.test(a.getAttribute('href') || '')).length,
        hrefs: [...p.querySelectorAll('a[target]')].map((a) => a.href) }; })()`);
      note(`${tier} hostile card: ${JSON.stringify(h)}`);
      if (h.imgs || !h.text || h.js) fail(`${tier} hostile: ${JSON.stringify(h)}`);
      if (h.hrefs.some(u => !u.startsWith('https://github.com/MohammadBnei/')))
        fail(`${tier} hostile: a source link off GitHub ${h.hrefs}`);
      await page.evalJSON(
        `(location.hash = '#/?sel=' + encodeURIComponent(${JSON.stringify(JSON.parse(readFileSync(graphFile, 'utf8')).nodes.find(n => n.kind === 'adr').id)}), true)`
      );
      await sleep(600);
      if (
        await page.evalJSON(
          `document.querySelectorAll('#panel img, #panel script, #crumbs img').length || globalThis.__pwned || 0`
        )
      )
        fail(`${tier} hostile ADR title built an element`);
    }

    if (tier === 'desktop') {
      // a click on an island opens its realm (Guard: 3 groups, so its centre is island, not a group tile)
      await page.evalJSON(`(location.hash = '#/', true)`);
      await sleep(1500);
      const pt = await page.evalJSON(`__atlas.point('guard')`);
      for (const type of ['mousePressed', 'mouseReleased'])
        await page.s('Input.dispatchMouseEvent', {
          type,
          x: pt.x,
          y: pt.y,
          button: 'left',
          clickCount: 1
        });
      const opened = await waitFor(
        page,
        `location.hash.startsWith('#/L1/guard') && location.hash`,
        3000
      );
      note(`click on Guard → ${opened}`);
      if (!opened) fail('click: the Guard island did not open its realm');
      // search, Enter, Escape
      await page.evalJSON(
        `(() => { const b = document.getElementById('search'); b.focus(); b.value = 'k8s-worker-02'; b.dispatchEvent(new Event('input')); return true; })()`
      );
      await sleep(200);
      const key = async (k, code, vk, text) => {
        // a real key press, as a person types it
        await page.s('Input.dispatchKeyEvent', {
          type: text ? 'keyDown' : 'rawKeyDown',
          key: k,
          code,
          windowsVirtualKeyCode: vk,
          ...(text ? { text } : {})
        });
        await page.s('Input.dispatchKeyEvent', {
          type: 'keyUp',
          key: k,
          code,
          windowsVirtualKeyCode: vk
        });
      };
      await key('Enter', 'Enter', 13, '\r');
      const sel = await waitFor(
        page,
        `location.hash.includes('sel=vm%3Ak8s-worker-02') && location.hash`,
        3000
      );
      note(`search → ${sel}`);
      if (!sel) fail('search: Enter did not open k8s-worker-02');
      await page.evalJSON(`(document.activeElement.blur(), true)`);
      await sleep(300);
      await key('Escape', 'Escape', 27);
      const upped = await waitFor(
        page,
        `!location.hash.includes('sel=') && location.hash.includes('g=nodes') && location.hash`,
        3000
      );
      if (!upped)
        fail(`Escape: did not zoom out to the group (${await page.evalJSON('location.hash')})`);
      // the Last event key flies to the newest event's node
      await key('l', 'KeyL', 76, 'l');
      const last = await waitFor(
        page,
        `location.hash.includes('sel=app%3Awird-adminweb') && location.hash`,
        3000
      );
      if (!last)
        fail(`Last event: did not open wird-adminweb (${await page.evalJSON('location.hash')})`);
      // L2 → L3: a building is picked, then entered; a wall card takes the pointer only once focused; Escape unfocuses, then
      // goes back to the city's district; L4 opens from a card's ADR and Escape returns to the building
      await page.evalJSON(`(location.hash = '#/L2/fleet', true)`);
      await sleep(1600);
      const clickAt = async pt => {
        for (const type of ['mousePressed', 'mouseReleased'])
          await page.s('Input.dispatchMouseEvent', {
            type,
            x: pt.x,
            y: pt.y,
            button: 'left',
            clickCount: 1
          });
      };
      let bp = await page.evalJSON(`__atlas.pointAt('b:svc:agent-fleet/core')`);
      if (bp) await clickAt(bp);
      const selected = await waitFor(
        page,
        `location.hash.includes('sel=svc%3Aagent-fleet%2Fcore') && location.hash`,
        3000
      );
      await sleep(2000);
      bp = await page.evalJSON(`__atlas.pointAt('b:svc:agent-fleet/core')`);
      if (bp) await clickAt(bp);
      const entered = await waitFor(
        page,
        `location.hash.startsWith('#/L3/svc:agent-fleet/core') && location.hash`,
        3000
      );
      note(`city: pick → ${selected}, enter → ${entered}`);
      if (!selected || !entered)
        fail(`city: picking and entering core failed (${selected} / ${entered})`);
      await sleep(1600);
      const cp = await page.evalJSON(`__atlas.pointCard('governs')`);
      if (cp) await clickAt(cp);
      await sleep(1200);
      const focus = await page.evalJSON(
        `({ f: __atlas.focused(), auto: [...document.querySelectorAll('#css3d .card3d')].filter((c) => c.style.pointerEvents === 'auto').map((c) => c.dataset.card) })`
      );
      note(`building: card focus → ${JSON.stringify(focus)}`);
      if (focus.f !== 'governs' || focus.auto.join() !== 'governs')
        fail(`building: focusing the governs card: ${JSON.stringify(focus)}`);
      // a live frame refreshes the cards on the same walls: the focused card's element and the focus survive it
      await page.evalJSON(`(globalThis.__card = document.querySelector('#css3d .card3d[data-card="governs"]'),
        fetch('/dev/fixtures/live.json').then((r) => r.json()).then((l) => { l.t = 'live'; __atlas.inject(l); return true; }))`);
      await sleep(300);
      const kept = await page.evalJSON(
        `!!globalThis.__card && globalThis.__card === document.querySelector('#css3d .card3d[data-card="governs"]') && globalThis.__card.isConnected && __atlas.focused() === 'governs' && globalThis.__card.textContent.length > 0`
      );
      note(`building: live frame → walls kept ${kept}`);
      if (!kept)
        fail(
          'building: a live frame rebuilt the walls (or lost the focus) instead of refreshing its cards in place'
        );
      await key('Escape', 'Escape', 27);
      await sleep(400);
      if (await page.evalJSON(`__atlas.focused()`))
        fail('building: Escape did not unfocus the card first');
      const adr = await page.evalJSON(
        `document.querySelector('.bcard[data-card="governs"] a[href^="#/L4/"]')?.getAttribute('href')`
      );
      if (adr) await page.evalJSON(`(location.hash = ${JSON.stringify(adr)}.slice(1), true)`);
      const read = await waitFor(
        page,
        `globalThis.__atlas.read?.doc && !document.getElementById('reader').hidden && document.querySelector('#reader article') && location.hash`,
        6000
      );
      note(
        `building → reader: ${read} ${JSON.stringify(await page.evalJSON('globalThis.__atlas.read'))}`
      );
      await key('Escape', 'Escape', 27);
      const backL3 = await waitFor(
        page,
        `location.hash.startsWith('#/L3/svc:agent-fleet/core') && location.hash`,
        3000
      );
      await key('Escape', 'Escape', 27);
      const city = await waitFor(
        page,
        `location.hash.startsWith('#/L2/fleet') && location.hash.includes('g=control') && location.hash`,
        3000
      );
      note(`reader → back ${backL3} → city ${city}`);
      if (!adr || !read || !backL3 || !city)
        fail(`ladder: adr ${adr}, read ${read}, back ${backL3}, city ${city}`);
      // 100 alerts: the beacons are instanced and capped, the budget holds
      await page.evalJSON(`(location.hash = '#/', true)`);
      await sleep(1200);
      await page.evalJSON(`(fetch('/dev/fixtures/live.json').then((r) => r.json()).then((l) => { const ids = ['app:wird', 'vm:k8s-cp-01', 'app:platform-traefik', 'ct:hermesagent', 'vm:pg01'];
        l.t = 'live'; l.alerts = Array.from({ length: 100 }, (_, i) => ({ fp: 'g' + i, name: 'Gate' + i, severity: i % 2 ? 'warning' : 'critical', startsAt: Date.now(), node: ids[i % 5], labels: {} }));
        __atlas.inject(l); return true; }))`);
      await sleep(1500);
      const storm = await page.evalJSON('__atlas.stats()');
      note(`alerts ×100: ${JSON.stringify(storm)}`);
      if (storm.calls > 220 || storm.triangles > TRIS)
        fail(`alerts ×100: over the budget (${storm.calls} draws, ${storm.triangles} tris)`);
      // resync through a restart
      await stop();
      const down = await waitFor(
        page,
        `/connection lost/i.test(document.getElementById('status').textContent) && document.getElementById('status').textContent`,
        10000
      );
      if (!down) fail('resync: the status line did not say the connection was lost');
      await start();
      const back = await waitFor(
        page,
        `!/connection lost/i.test(document.getElementById('status').textContent)`,
        40000
      );
      note(`resync: ${down} → ${back ? 'recovered' : 'NOT recovered'}`);
      if (!back) fail('resync: the page did not recover after atlas came back');
    }
    await page.close();
  }
} finally {
  srv?.kill();
}
if (fails.length) {
  console.log(log.split('\n').slice(-15).join('\n'));
  console.log(`GATE FAIL (${fails.length})`);
  process.exit(1);
}
console.log(`GATE PASS → ${OUT}`);

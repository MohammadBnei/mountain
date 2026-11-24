#!/usr/bin/env node
// Gate for the feed's page (cmd/atlas/feed_page.go, www/js/feed.js). Node >= 22, no dependencies. Exit 0 = PASS.
//
//   node www/dev/feed-gate.mjs      (env CHROME: a Chrome / Chromium binary; GATE_OUT: where the screenshots go)
//
// Runs the real binary twice (--dev on loopback). Off: no entry on the map, /feed is a 404, no CSP names core. On, over
// dev/fixtures/feed.json, at 390×844, 1440×900 and 360×740, dark and light: every scene drawn with no console error, no
// request to another origin, no horizontal scroll and no hostile word made into an element or a link. Some scenes
// answer /api/feed from here instead (Fetch.fulfillRequest): words the server would have cut, the other speakers, and
// the empty, loading, stale and failed states.
import { spawn, spawnSync } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { launch, ROOT, sleep, waitFor } from './cdp.mjs';

const OUT = resolve(process.env.GATE_OUT || '/tmp/mountain-gate/feed');
await mkdir(OUT, { recursive: true });
const fails = [];
const fail = m => {
  fails.push(m);
  console.log(`FAIL ${m}`);
};
const repo = resolve(ROOT, '..'),
  bin = join(OUT, 'atlas');
const b = spawnSync('go', ['build', '-o', bin, './cmd/atlas'], { cwd: repo, encoding: 'utf8' });
if (b.status !== 0) {
  console.log(`FAIL go build ./cmd/atlas: ${b.stderr.trim().split('\n')[0]}`);
  process.exit(1);
}
let log = '';
async function start(...extra) {
  const port = await new Promise(r => {
    const s = createServer().listen(0, '127.0.0.1', () => {
      const p = s.address().port;
      s.close(() => r(p));
    });
  });
  const srv = spawn(
    bin,
    [
      '--dev',
      `--listen=127.0.0.1:${port}`,
      '--metrics=127.0.0.1:0',
      `--graph-fixture=${join(ROOT, 'dev/fixtures/graph.json')}`,
      `--live-fixture=${join(ROOT, 'dev/fixtures/live.json')}`,
      ...extra
    ],
    {
      cwd: repo,
      env: {
        ...process.env,
        MOUNTAIN_OIDC_ISSUER: '',
        MOUNTAIN_FORGE_TOKEN: '',
        DATABASE_URL: '',
        FEED_ENABLED: ''
      }
    }
  );
  for (const s of [srv.stdout, srv.stderr])
    s.on('data', d => {
      log += d;
    });
  const origin = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 50; i++) {
    try {
      if ((await fetch(`${origin}/healthz`)).ok) return { srv, origin };
    } catch {}
    await sleep(100);
  }
  srv.kill();
  throw new Error('atlas did not answer /healthz');
}
const CORE = 'https://fleet.bnei.dev';
const cspOf = r => r.headers.get('content-security-policy') || '';

// ---- off
{
  const { srv, origin } = await start();
  try {
    const r = await fetch(`${origin}/`);
    if ((await r.text()).includes('/feed')) fail('off: the map links to the feed');
    if (cspOf(r).includes(CORE)) fail('off: the CSP names core');
    for (const p of ['/feed', '/feed/pr-0011223344556677', '/feed.html', '/api/feed'])
      if ((await fetch(origin + p)).status !== 404) fail(`off: ${p} is not a 404`);
  } finally {
    srv.kill();
  }
}

// ---- on
const H =
  '<script>globalThis.__pwned=1</script><img src=x onerror="globalThis.__pwned=1">\u202Egnp.exe\u202C javascript:globalThis.__pwned=1 <a href="https://evil.example">x</a>';
const now = Date.now();
const iso = min => new Date(now - min * 60e3).toISOString();
const age = { last_event_age: 60, newest_activity_at: iso(1) };
const ID = 'pr-00000000000000aa';
const hostile = {
  id: ID,
  kind: 'pr',
  title: `PR ${H}${'W'.repeat(10000)}`,
  title_quoted: true,
  state: 'open',
  opened_at: iso(9),
  last_activity_at: iso(1),
  needs_owner: true,
  entries: [
    { time: iso(9), actor: 'system', text: H, quoted: true },
    { time: iso(8), actor: H, text: `${H}\n${'x '.repeat(5000)}`, quoted: true },
    { time: iso(7), actor: 'system', text: 'A'.repeat(10000), quoted: true },
    { time: 'never', actor: 'system', text: H, quoted: false }
  ],
  links: [
    { kind: 'run', ref: 'javascript:globalThis.__pwned=1', quoted: true },
    { kind: 'run', ref: 'http://evil.example/', quoted: true },
    { kind: 'run', ref: 'https://git.bnei.dev@evil.example/', quoted: true },
    { kind: 'run', ref: '//evil.example', quoted: true },
    { kind: 'generator', ref: 'data:text/html,<script>1</script>', quoted: true },
    { kind: 'deploy', ref: '../../auth/logout', quoted: false },
    { kind: 'previous', ref: 'alert-0011223344556677', quoted: true },
    { kind: H, ref: `https://forge.example.org/${'deep/'.repeat(60)}`, quoted: true }
  ],
  age
};
const speakers = {
  id: 'session-00000000000000bb',
  kind: 'session',
  title: 'Session: Morning look at the cluster',
  title_quoted: true,
  state: 'open',
  opened_at: iso(30),
  last_activity_at: iso(2),
  needs_owner: true,
  entries: [
    { time: iso(30), actor: 'system', text: 'opened (character)', quoted: false },
    {
      time: iso(26),
      actor: 'system',
      text: 'example-01 has 4 hours of disk left.\nUsed 91 percent.',
      quoted: true
    },
    {
      time: iso(20),
      actor: 'demo-lead',
      text: 'The disk fills since the last deploy: the demo worker writes its scratch files there. I opened an issue with the sizes. Restarting it would free 60 GB; that needs your yes.',
      quoted: false
    },
    { time: iso(12), actor: 'owner', text: 'Yes, restart it.', quoted: false },
    {
      time: iso(2),
      actor: 'demo-scribe',
      text: 'Noted for the evening summary.',
      quoted: false
    }
  ],
  links: [
    { kind: 'previous', ref: 'session-00000000000000cc', quoted: false },
    { kind: 'run', ref: 'https://forge.example.org/octo/atlas-demo/actions/runs/41', quoted: true }
  ],
  age
};
const list = (threads, a = age) => ({ threads, next_cursor: '', dropped: 0, age: a });
const slim = t => ({ ...t, entries: t.entries.slice(-1), links: null });
const byNeeds = ts => u =>
  list(ts.filter(t => String(Number(t.needs_owner)) === u.searchParams.get('needs')).map(slim));
const manyNeeds = Array.from({ length: 8 }, (_, i) => ({
  ...speakers,
  id: `alert-000000000000000${i}`,
  kind: 'alert',
  title: `Alert: DemoAlert${i}`
}));

// [name, path ("@pr" = the fixture's thread of that kind), stub (u => body | status), step, full page]
const SCENES = [
  ['list', '/feed'],
  ['list-scrolled', '/feed', null, `scrollTo(0, 900)`],
  ['kind-pr', '/feed?kind=pr'],
  ['needs', '/feed', byNeeds(manyNeeds)],
  ['thread-alert', '@alert'],
  ['thread-deploy', '@deploy'],
  ['thread-pr', '@pr'],
  ['thread-session', '@session'],
  ['thread-card', '@card'],
  ['speakers', `/feed/${speakers.id}`, () => speakers],
  [
    'hostile-list',
    '/feed',
    byNeeds([hostile, { ...hostile, id: 'pr-00000000000000ad', needs_owner: false }])
  ],
  ['hostile-thread', `/feed/${ID}`, () => hostile],
  [
    'hostile-open',
    `/feed/${ID}`,
    () => hostile,
    `(document.querySelector('.more').click(), document.querySelector('.more').scrollIntoView({ block: 'end' }))`
  ],
  ['empty', '/feed', () => list([], { last_event_age: null, newest_activity_at: null })],
  ['error', '/feed', () => 502],
  ['gone', '/feed/pr-00000000000000ff'],
  [
    'stale',
    '/feed',
    () => list([slim(speakers)], { last_event_age: 5 * 3600, newest_activity_at: iso(300) })
  ],
  ['loading', '/feed', () => 'wait']
];
const SIZES = [
  [390, 844],
  [1440, 900],
  [360, 740]
];

const { srv, origin } = await start(`--feed-fixture=${join(ROOT, 'dev/fixtures/feed.json')}`);
try {
  const map = await fetch(`${origin}/`),
    feed = await fetch(`${origin}/feed`);
  if (!(await map.text()).includes('href="/feed"')) fail('on: the map has no entry to the feed');
  if (cspOf(map).includes(CORE)) fail("on: the map's CSP names core");
  const csp = cspOf(feed);
  console.log(`feed CSP ${csp}`);
  if (!csp.includes(`connect-src 'self' wss://mountain.bnei.dev ${CORE}`))
    fail("on: the feed's CSP does not let it reach core");
  if (!/script-src 'self';/.test(csp) || !/style-src 'self';/.test(csp) || /unsafe/.test(csp))
    fail('on: scripts and styles must be self only');
  if ((await fetch(`${origin}/feed.html`)).status !== 404)
    fail('on: /feed.html is served as a file');
  if ((await fetch(`${origin}/feed/x`)).status !== 404) fail('on: /feed/x is not a 404');

  const first = {};
  for (const k of ['alert', 'deploy', 'pr', 'session', 'card']) {
    const ts = (await (await fetch(`${origin}/api/feed?kind=${k}&limit=50`)).json()).threads;
    const pick = { pr: t => t.title.includes('#7') }[k] || (t => t.needs_owner);
    first[k] = `/feed/${(ts.find(pick) || ts[0]).id}`;
  }

  for (const [w, h] of SIZES) {
    const page = await launch({ w, h, mobile: w < 600 });
    let stub = null,
      where = '';
    page.on(({ method, params }) => {
      if (method === 'Fetch.requestPaused') {
        const u = new URL(params.request.url),
          body = stub && u.pathname.startsWith('/api/feed') ? stub(u) : null;
        if (body === 'wait') return; // never answered
        if (body === null) return page.s('Fetch.continueRequest', { requestId: params.requestId });
        return page.s('Fetch.fulfillRequest', {
          requestId: params.requestId,
          responseCode: typeof body === 'number' ? body : 200,
          responseHeaders: [{ name: 'Content-Type', value: 'application/json' }],
          body: Buffer.from(JSON.stringify(body)).toString('base64')
        });
      }
      if (method === 'Network.requestWillBeSent') {
        const u = new URL(params.request.url);
        if (u.origin !== origin && u.protocol !== 'data:') fail(`${where}: request to ${u}`);
      }
      if (method === 'Runtime.exceptionThrown')
        fail(`${where}: ${params.exceptionDetails.exception?.description?.slice(0, 300)}`);
      if (method === 'Runtime.consoleAPICalled' && ['error', 'assert'].includes(params.type))
        fail(`${where}: console.${params.type} ${JSON.stringify(params.args).slice(0, 300)}`);
      if (method === 'Log.entryAdded' && /Content Security|Refused/i.test(params.entry.text))
        fail(`${where}: ${params.entry.text}`);
    });
    for (const m of ['Runtime.enable', 'Network.enable', 'Page.enable', 'Log.enable'])
      await page.s(m);
    await page.s('Fetch.enable', { patterns: [{ urlPattern: `${origin}/api/*` }] });
    for (const scheme of ['dark', 'light']) {
      await page.s('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-color-scheme', value: scheme }]
      });
      for (const [name, at, st, step] of SCENES) {
        where = `${name} ${w} ${scheme}`;
        stub = st || null;
        await page.s('Page.navigate', { url: origin + (at[0] === '@' ? first[at.slice(1)] : at) });
        const ready =
          name === 'loading'
            ? await sleep(700).then(() => true)
            : await waitFor(
                page,
                `document.querySelector('#view .card, #view .thead, #view .state:has(b)') && document.fonts.status === 'loaded'`,
                8000
              );
        if (!ready) fail(`${where}: never drawn`);
        if (step) await page.evalJSON(`(${step}, true)`);
        await sleep(250);
        const i = await page.evalJSON(`({
          wide: document.documentElement.scrollWidth - innerWidth, pwned: globalThis.__pwned || 0,
          made: document.querySelectorAll('#view img, #view script, #view iframe, #view [onerror], #view [style]').length,
          links: [...document.querySelectorAll('#view a')].map(a => a.getAttribute('href')).filter(h => !/^(\\/feed(\\/[a-z]+-[0-9a-f]{16})?|https:\\/\\/(fleet\\.bnei\\.dev|forge\\.example\\.org|prometheus\\.example\\.org)\\/.*)$/.test(h)),
          rel: [...document.querySelectorAll('#view a[href^="https:"]')].filter(a => a.rel !== 'noopener noreferrer' || !a.target).length,
          bidi: /[\\u202A-\\u202E\\u2066-\\u2069]/.test(document.body.textContent + document.title) })`);
        if (i.wide > 0) fail(`${where}: ${i.wide}px of horizontal scroll`);
        if (i.pwned || i.made || i.links.length || i.rel || i.bidi)
          fail(`${where}: ${JSON.stringify(i)}`);
        if (w !== 360) {
          const shot = await page.s('Page.captureScreenshot', { format: 'png' });
          await writeFile(
            join(OUT, `${name}-${w}x${h}-${scheme}.png`),
            Buffer.from(shot.data, 'base64')
          );
        }
      }
    }

    // new threads wait behind the pill, the scroll stays, and the pill shows them; j walks the cards, . shows the new
    where = `pill ${w}`;
    let ts = Array.from({ length: 12 }, (_, i) => ({
      ...slim(speakers),
      id: `deploy-00000000000000${10 + i}`,
      kind: 'deploy',
      title: `Deploy: demo-${i}`,
      title_quoted: false,
      needs_owner: false,
      last_activity_at: iso(10 + i)
    }));
    stub = u => byNeeds(ts)(u);
    await page.s('Page.navigate', { url: `${origin}/feed` });
    await waitFor(page, `document.querySelectorAll('.card').length === 12`, 8000);
    await page.evalJSON(`(scrollTo(0, 300), document.activeElement.blur(), true)`);
    await sleep(100);
    const top = await page.evalJSON('scrollY');
    ts = [
      {
        ...ts[0],
        id: 'deploy-00000000000000ee',
        title: 'Deploy: the-new-one',
        last_activity_at: iso(0)
      },
      ...ts
    ];
    // a hidden and shown page reads at once
    await page.evalJSON(`(document.dispatchEvent(new Event('visibilitychange')), true)`);
    const pill = await waitFor(
      page,
      `!document.getElementById('pill').hidden && document.getElementById('pill').textContent`,
      5000
    );
    const after = await page.evalJSON(
      `({ y: scrollY, n: document.querySelectorAll('.card').length })`
    );
    if (pill !== '1 new' || after.y !== top || after.n !== 12)
      fail(`${where}: pill ${pill}, scroll ${top} → ${after.y}, ${after.n} cards`);
    const shot = await page.s('Page.captureScreenshot', { format: 'png' });
    if (w !== 360)
      await writeFile(join(OUT, `pill-${w}x${h}-light.png`), Buffer.from(shot.data, 'base64'));
    const press = async (k, code) => {
      await page.s('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, text: k });
      await page.s('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code });
    };
    await press('.', 'Period');
    const shown = await waitFor(
      page,
      `document.querySelectorAll('.card').length === 13 && document.activeElement.textContent`,
      3000
    );
    if (!shown?.includes('the-new-one') || (await page.evalJSON('scrollY')) !== 0)
      fail(`${where}: "." did not show the new thread on top (${shown})`);
    await press('j', 'KeyJ');
    if (!(await page.evalJSON('document.activeElement.textContent')).includes('demo-0'))
      fail(`${where}: j did not move to the next card`);
    // a failed read says so and keeps what is on screen
    stub = () => 502;
    await page.evalJSON(`(document.dispatchEvent(new Event('visibilitychange')), true)`);
    if (
      !(await waitFor(
        page,
        `document.getElementById('banner').textContent.startsWith('Not updating')`,
        5000
      ))
    )
      fail(`${where}: a failed read is not said`);
    if ((await page.evalJSON(`document.querySelectorAll('.card').length`)) !== 13)
      fail(`${where}: a failed read took the list away`);
    if (w !== 360) {
      const s2 = await page.s('Page.captureScreenshot', { format: 'png' });
      await writeFile(join(OUT, `notupdating-${w}x${h}-light.png`), Buffer.from(s2.data, 'base64'));
    }
    // paging: the fixture's second page follows the first, no thread twice
    where = `paging ${w}`;
    stub = null;
    await page.s('Page.navigate', { url: `${origin}/feed?kind=deploy` });
    await waitFor(page, `document.querySelector('#foot button')`, 8000);
    await page.evalJSON(`(document.querySelector('#foot button').click(), true)`);
    const ids = await waitFor(
      page,
      `document.querySelector('#foot .end') && [...document.querySelectorAll('#view .card')].map(c => c.dataset.id)`,
      5000
    );
    if (!ids || ids.length < 60 || new Set(ids).size !== ids.length)
      fail(`${where}: ${ids?.length} cards, ${new Set(ids).size} different`);
    await page.close();
  }
} finally {
  srv.kill();
}
if (fails.length) {
  console.log(log.split('\n').slice(-10).join('\n'));
  console.log(`FEED GATE FAIL (${fails.length})`);
  process.exit(1);
}
console.log(`FEED GATE PASS → ${OUT}`);

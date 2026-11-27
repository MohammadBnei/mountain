#!/usr/bin/env node
// Gate for the feed's page (cmd/atlas/feed_page.go, www/js/feed.js). Node >= 22, no dependencies. Exit 0 = PASS.
//
//   node www/dev/feed-gate.mjs      (env CHROME: a Chrome / Chromium binary; GATE_OUT: where the screenshots go)
//
// Runs the real binary twice (--dev on loopback). Off: no entry on the map, /feed is a 404, no CSP names core. On, over
// dev/fixtures/feed.json, at 390×844, 1440×900, 360×740 and 430×932, dark and light: every scene drawn with no console
// error, no request to another origin, no horizontal scroll, no hostile word made into an element or a link, every
// touch target 44 px on the phones, and every text at WCAG AA on its own background (glass over the worst behind it).
// Then the accessibility tree, the pill, the keys, the tabs, back, a failed read, paging, and the two reduced settings.
// Some scenes answer /api/feed from here instead (Fetch.fulfillRequest): words the server would have cut, the other
// speakers, a session's posts (dev/fixtures/feed-posts.json), and the empty, loading, stale and failed states.
import { spawn, spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
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
// served answers /api/feed and /api/feed/<id> from threads, as the server would: filters, previews, newest first.
const list = (threads, a = age) => ({ threads, next_cursor: '', dropped: 0, age: a });
// a list's thread: its preview alone, cut to 400 characters as the server does
const slim = t => ({
  ...t,
  entries: t.entries
    .slice(-1)
    .map(e => (e.text.length > 400 ? { ...e, text: e.text.slice(0, 400), has_more: true } : e))
    .map(e =>
      e.needs_list?.length > 3 || e.needs_list?.some(n => n.length > 120)
        ? {
            ...e,
            needs_list: e.needs_list.slice(0, 3).map(n => n.slice(0, 120)),
            needs_count: e.needs_list.length
          }
        : e
    ),
  links: null
});
const served =
  (threads, a = age) =>
  u => {
    const id = /^\/api\/feed\/(.+)$/.exec(u.pathname)?.[1];
    if (id) {
      const t = threads.find(x => x.id === id);
      return t ? { ...t, age: a } : 404;
    }
    const kind = u.searchParams.get('kind'),
      needs = u.searchParams.get('needs');
    return list(
      threads
        .filter(t => (!kind || t.kind === kind) && (!needs || t.needs_owner === (needs === '1')))
        .sort((x, y) => Date.parse(y.last_activity_at) - Date.parse(x.last_activity_at))
        .map(slim),
      a
    );
  };
// a session's posts, in the shape the API will give them (dev/fixtures/feed-posts.json: made up)
const told = JSON.parse(readFileSync(join(ROOT, 'dev/fixtures/feed-posts.json'), 'utf8')).map(
  t => ({
    ...t,
    opened_at: iso(t.entries[0].ago_min),
    last_activity_at: iso(t.entries[t.entries.length - 1].ago_min),
    entries: t.entries.map(({ ago_min, ...e }) => ({ ...e, time: iso(ago_min) }))
  })
);
const asking = told[0];
const evil = told.find(t => t.title.includes('hostile'));
const manyNeeds = Array.from({ length: 8 }, (_, i) => ({
  ...speakers,
  id: `alert-000000000000000${i}`,
  kind: 'alert',
  title: `Alert: DemoAlert${i}`,
  entries: [{ time: iso(2 + i), actor: 'system', text: 'firing', quoted: false }],
  last_activity_at: iso(2 + i)
}));
const all = [...told, speakers, ...manyNeeds.slice(0, 2)];

// [name, path ("@pr" = the fixture's thread of that kind), stub (u => body | status), step]
const SCENES = [
  ['list', '/feed', served(all)],
  ['list-real', '/feed'],
  ['list-scrolled', '/feed', null, `scrollTo(0, 900)`],
  ['kind-pr', '/feed?kind=pr'],
  ['kind-laptop', '/feed?kind=laptop', served(all)],
  ['needs', '/feed?tab=needs', served([asking, ...manyNeeds])],
  ['thread-alert', '@alert'],
  ['thread-deploy', '@deploy'],
  ['thread-pr', '@pr'],
  ['thread-session', '@session'],
  ['thread-card', '@card'],
  ['thread-posts', `/feed/${asking.id}`, served(all)],
  [
    'thread-posts-end',
    `/feed/${asking.id}`,
    served(all),
    `document.querySelector('.thread .post:last-child').scrollIntoView({ block: 'end' })`
  ],
  ['thread-long', `/feed/${told[1].id}`, served(all)],
  ['thread-claim', `/feed/${told.find(t => t.kind === 'pr').id}`, served(all)],
  [
    'more-open',
    '/feed?kind=laptop',
    served(all),
    `[...document.querySelectorAll('.more')].slice(0, 1).map(b => b.click())`
  ],
  ['speakers', `/feed/${speakers.id}`, served(all)],
  [
    'hostile-list',
    '/feed',
    served([hostile, { ...hostile, id: 'pr-00000000000000ad', needs_owner: false }, evil])
  ],
  ['hostile-thread', `/feed/${ID}`, served([hostile])],
  ['hostile-posts', `/feed/${evil.id}`, served(all)],
  [
    'hostile-open',
    `/feed/${ID}`,
    served([hostile]),
    `(b => (b.click(), b.scrollIntoView({ block: 'end' })))([...document.querySelectorAll('.thread .more')][0])`
  ],
  ['empty', '/feed', () => list([], { last_event_age: null, newest_activity_at: null })],
  ['empty-needs', '/feed?tab=needs', () => list([])],
  ['error', '/feed', () => 502],
  ['gone', '/feed/pr-00000000000000ff'],
  [
    'stale',
    '/feed',
    served(
      all.map(t => ({ ...t, last_activity_at: iso(440) })),
      {
        last_event_age: 7 * 3600,
        newest_activity_at: iso(420)
      }
    )
  ],
  ['loading', '/feed', () => 'wait']
];
const SIZES = [
  [390, 844],
  [1440, 900],
  [360, 740],
  [430, 932]
];

// ---- what is measured in the page, per scene (a string: it runs there)

// AUDIT: the width, what hostile words became, the touch targets and the contrast of every text on its own background,
// composed down to the page's; a bar over scrolling posts is taken over the worst of black and white behind its glass,
// and a starfield's tint is added under everything it lies under.
const AUDIT = `(() => {
  const parse = c => { const m = c.match(/[\\d.]+/g)?.map(Number) ?? [0, 0, 0, 0]; return c.startsWith('color(srgb') ? [m[0] * 255, m[1] * 255, m[2] * 255, m[3] ?? 1] : [m[0], m[1], m[2], m[3] ?? 1]; };
  const over = (f, b) => [0, 1, 2].map(i => f[i] * f[3] + b[i] * (1 - f[3])).concat(1);
  const lum = c => { const [r, g, b] = c.map(v => { v /= 255; return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
  const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
  const root = getComputedStyle(document.documentElement), v = n => root.getPropertyValue(n).trim();
  const probe = document.body.appendChild(document.createElement('i')); const css = c => { probe.style.color = c; return parse(getComputedStyle(probe).color); };
  const surface = css(v('--surface')), glow = css(v('--glow'));
  const grounds = e => { // every ground e may stand on
    let layers = [], bars = false;
    for (let n = e; n; n = n.parentElement) {
      const s = getComputedStyle(n), bg = parse(s.backgroundColor);
      if (bg[3] > 0) layers.push(bg);
      if (n !== document.body && s.backgroundImage.includes('gradient')) layers.push([...glow.slice(0, 3), 0.1]);
      if (bg[3] === 1) break;
      if ((s.position === 'sticky' || s.position === 'fixed') && bg[3] > 0 && s.backdropFilter !== 'none') { bars = true; break; }
    }
    const behind = bars ? [[0, 0, 0, 1], [255, 255, 255, 1]] : [surface, over([...glow.slice(0, 3), 0.1], surface)];
    return behind.map(b => layers.reduceRight((b2, l) => over(l, b2), b));
  };
  const shown = e => e.checkVisibility({ visibilityProperty: true, opacityProperty: true }) && !e.closest('.sr, .skip') && e.getClientRects().length;
  const text = {};
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (w.nextNode()) {
    const e = w.currentNode.parentElement;
    if (!w.currentNode.data.trim() || !shown(e) || e === probe) continue;
    const s = getComputedStyle(e), px = parseFloat(s.fontSize), big = px >= 24 || (px >= 18.66 && s.fontWeight >= 700);
    const r = Math.min(...grounds(e).map(g => ratio(over(parse(s.color), g), g)));
    const k = big ? 'large' : 'text';
    if (!text[k] || r < text[k].r) text[k] = { r, at: e.tagName + '.' + e.className, words: w.currentNode.data.trim().slice(0, 20) };
    if (r < (big ? 3 : 4.5)) (text.bad ??= []).push([+r.toFixed(2), e.tagName + '.' + e.className, w.currentNode.data.trim().slice(0, 20)]);
  }
  // the edges that make a control or a mark: 3:1 on what is beside them
  const edges = {};
  for (const e of document.querySelectorAll('.chip, #tabs a, .ref, .quoted, .tag, .avatar, a[aria-current]')) {
    if (!shown(e) || e.id === 'back') continue;
    const s = getComputedStyle(e), c = parse(s.borderTopColor);
    if (parseFloat(s.borderTopWidth) === 0 || c[3] === 0) continue;
    // an edge shows when it stands out from what is around it or from what it holds
    const r = Math.max(Math.min(...grounds(e.parentElement).map(g => ratio(over(c, g), g))), Math.min(...grounds(e).map(g => ratio(over(c, g), g))));
    const k = e.className.split(' ')[0] || e.tagName;
    if (!edges[k] || r < edges[k]) edges[k] = +r.toFixed(2);
  }
  const ring = css(v('--focus'));
  const rings = [...document.querySelectorAll('.post, .top, #dest, .ref, .chip, .quoted, .own')].filter(shown).flatMap(grounds).concat([surface, css(v('--accent-soft'))]);
  const focus = Math.min(...rings.map(g => ratio(ring, g)));
  probe.remove();
  const small = [];
  if (matchMedia('(pointer: coarse)').matches || innerWidth < 720)
    for (const e of document.querySelectorAll('a, button, summary, [tabindex="0"]')) {
      if (!shown(e) || e.closest('.md p, .md li, .md td, .md th, .claim .text, .side') || e.matches('.open, .mdtable, pre, .skip')) continue; // a link in a sentence is exempt (WCAG 2.5.8)
      let { width, height } = e.getBoundingClientRect();
      for (const part of ['::before', '::after']) { // a finger's worth, drawn around it
        const p = getComputedStyle(e, part);
        if (p.content !== 'none' && p.position === 'absolute') { width = Math.max(width, parseFloat(p.width)); height = Math.max(height, parseFloat(p.height)); }
      }
      const b = { width, height };
      if (b.width < 43.5 || b.height < 43.5) small.push([e.tagName + '#' + e.id + '.' + e.className, Math.round(b.width), Math.round(b.height)]);
    }
  return {
    wide: document.documentElement.scrollWidth - innerWidth, pwned: globalThis.__pwned || 0,
    made: document.querySelectorAll('#view img, #view script, #view iframe, #view [onerror], #view [style]').length,
    links: [...document.querySelectorAll('#view a')].map(a => a.getAttribute('href')).filter(h => !/^(\\/feed(\\/[a-z]+-[0-9a-f]{16}(#[\\w-]{1,64})?)?|https:\\/\\/(fleet\\.bnei\\.dev|(forge|prometheus|docs)\\.example\\.org)\\/.*)$/.test(h)),
    rel: [...document.querySelectorAll('#view a[href^="https:"]')].filter(a => a.rel !== 'noopener noreferrer' || !a.target).length,
    bidi: /[\\u202A-\\u202E\\u2066-\\u2069]/.test(document.body.textContent + document.title),
    inputs: [...document.querySelectorAll('input, textarea, select')].filter(e => parseFloat(getComputedStyle(e).fontSize) < 16).length,
    text, edges, focus, small
  };
})()`;
const worst = {}; // scheme -> the lowest of each measure over every scene and size
function measured(where, scheme, i) {
  worst[scheme] ??= { text: { r: 99 }, large: { r: 99 }, focus: 99, edges: {} };
  const w = worst[scheme];
  for (const k of ['text', 'large'])
    if (i.text[k] && i.text[k].r < w[k].r) w[k] = { ...i.text[k], where };
  w.focus = Math.min(w.focus, i.focus);
  for (const [k, r] of Object.entries(i.edges)) w.edges[k] = Math.min(w.edges[k] ?? 99, r);
  if (i.text.bad) fail(`${where}: contrast under AA ${JSON.stringify(i.text.bad.slice(0, 3))}`);
  if (i.focus < 3) fail(`${where}: focus ring ${i.focus.toFixed(2)}`);
  for (const [k, r] of Object.entries(i.edges))
    if (r < 3) fail(`${where}: the edge of ${k} is ${r}`);
}

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
  if (
    !/default-src 'self';/.test(csp) ||
    !/script-src 'self';/.test(csp) ||
    !/style-src 'self';/.test(csp) ||
    /unsafe|font-src/.test(csp)
  )
    fail('on: scripts, styles and fonts must be self only');
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
    if (w < 600) await page.s('Emulation.setTouchEmulationEnabled', { enabled: true });
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
    const shoot = async name => {
      const s = await page.s('Page.captureScreenshot', { format: 'png' });
      await writeFile(join(OUT, `${name}.png`), Buffer.from(s.data, 'base64'));
    };
    const drawn = () =>
      waitFor(
        page,
        `document.querySelector('#view .post, #view .main, #view .state b') && document.fonts.status === 'loaded'`,
        8000
      );
    for (const scheme of ['dark', 'light']) {
      await page.s('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-color-scheme', value: scheme }]
      });
      for (const [name, at, st, step] of SCENES) {
        where = `${name} ${w} ${scheme}`;
        stub = st || null;
        await page.s('Page.navigate', { url: origin + (at[0] === '@' ? first[at.slice(1)] : at) });
        const ready = name === 'loading' ? await sleep(700).then(() => true) : await drawn();
        if (!ready) fail(`${where}: never drawn`);
        if (step) await page.evalJSON(`(${step}, true)`);
        await sleep(300);
        const i = await page.evalJSON(AUDIT);
        if (i.wide > 0) fail(`${where}: ${i.wide}px of horizontal scroll`);
        if (i.pwned || i.made || i.links.length || i.rel || i.bidi || i.inputs)
          fail(`${where}: ${JSON.stringify({ ...i, text: 0, edges: 0 })}`);
        if (i.small.length)
          fail(`${where}: targets under 44 px ${JSON.stringify(i.small.slice(0, 4))}`);
        measured(where, scheme, i);
        await shoot(`${name}-${w}x${h}-${scheme}`);
      }
    }

    // reduced transparency: the glass is solid and nothing blurs
    where = `solid ${w}`;
    stub = served(all);
    await page.s('Emulation.setEmulatedMedia', {
      features: [
        { name: 'prefers-color-scheme', value: 'dark' },
        { name: 'prefers-reduced-transparency', value: 'reduce' },
        { name: 'prefers-reduced-motion', value: 'reduce' }
      ]
    });
    await page.s('Page.navigate', { url: `${origin}/feed` });
    await drawn();
    const solid = await page.evalJSON(
      `(() => { const s = getComputedStyle(document.querySelector('.top')), p = getComputedStyle(document.querySelector('.post')); return { bg: s.backgroundColor, blur: s.backdropFilter, move: p.transitionDuration }; })()`
    );
    if (/rgba/.test(solid.bg) || solid.blur !== 'none' || solid.move !== '0s')
      fail(`${where}: ${JSON.stringify(solid)}`);
    await page.s('Emulation.setEmulatedMedia', {
      features: [{ name: 'prefers-color-scheme', value: 'dark' }]
    });

    // what a screen reader is given: headings, lists, buttons, quotes, the tabs' current one
    if (w === 390) {
      const tree = async () => {
        const { nodes } = await page.s('Accessibility.getFullAXTree');
        return nodes
          .filter(n => !n.ignored)
          .map(n => ({
            role: n.role?.value,
            name: n.name?.value ?? '',
            props: Object.fromEntries((n.properties ?? []).map(p => [p.name, p.value?.value]))
          }));
      };
      const has = (ns, role, name) =>
        ns.some(
          n => n.role === role && (name === undefined || (name.test?.(n.name) ?? n.name === name))
        );
      where = 'a11y';
      stub = served(all);
      await page.s('Page.navigate', { url: `${origin}/feed` });
      await drawn();
      let ns = await tree();
      for (const [role, name] of [
        ['heading', 'Feed'],
        ['heading', 'make the demo search fast'],
        ['heading', 'Alerts'],
        ['navigation', 'Main'],
        ['navigation', 'Timelines'],
        ['list', 'Posts: All'],
        ['listitem', undefined],
        ['blockquote', undefined],
        ['button', 'Show more'],
        ['link', /^Open Laptop session make the demo search fast/]
      ])
        if (!has(ns, role, name)) fail(`${where}: the timeline has no ${role} ${name ?? ''}`);
      const roles = {};
      for (const n of ns) roles[n.role] = (roles[n.role] ?? 0) + 1;
      console.log(`a11y timeline ${JSON.stringify(roles)}`);
      await page.s('Page.navigate', {
        url: `${origin}/feed/${told.find(t => t.kind === 'pr').id}`
      });
      await drawn();
      ns = await tree();
      for (const [role, name] of [
        ['heading', 'Post'],
        ['heading', 'Forge'],
        ['article', 'PR thread'],
        ['link', 'Back to the feed'],
        ['list', 'Posts in this thread'],
        ['list', 'Links'],
        ['group', 'Reply'],
        ['blockquote', undefined],
        ['link', /make the demo search fast/]
      ])
        if (!has(ns, role, name)) fail(`${where}: a thread has no ${role} ${name ?? ''}`);
      await page.s('Page.navigate', { url: `${origin}/feed/${asking.id}` });
      await drawn();
      const posts = await tree();
      for (const role of ['table', 'columnheader', 'blockquote', 'listitem'])
        if (!has(posts, role)) fail(`${where}: a session's thread has no ${role}`);
      const un = ns.filter(n => ['link', 'button'].includes(n.role) && !n.name.trim());
      if (un.length) fail(`${where}: ${un.length} links or buttons without a name`);
    }

    // new posts wait behind the pill, the scroll stays, and the pill shows them; j walks the posts, . shows the new;
    // the active tab goes to the top; a tapped post opens its thread and back finds the timeline where it was
    where = `pill ${w}`;
    let ts = Array.from({ length: 24 }, (_, i) => ({
      ...slim(speakers),
      id: `deploy-00000000000000${10 + i}`,
      kind: 'deploy',
      title: `Deploy: demo-${i}`,
      title_quoted: false,
      needs_owner: false,
      last_activity_at: iso(10 + i),
      entries: [
        { time: iso(10 + i), actor: 'system', text: 'synced to 0a1b2c3d4e5f', quoted: false }
      ]
    }));
    stub = u => served(ts)(u);
    await page.s('Page.navigate', { url: `${origin}/feed` });
    await waitFor(page, `document.querySelectorAll('.post').length === 24`, 8000);
    await page.evalJSON(`(scrollTo(0, 300), document.activeElement.blur(), true)`);
    await sleep(100);
    const top = await page.evalJSON('scrollY');
    const add = (id, title) => {
      ts = [{ ...ts[0], id, title, last_activity_at: new Date().toISOString() }, ...ts];
    };
    add('deploy-00000000000000ee', 'Deploy: the-new-one');
    const reread = () =>
      page.evalJSON(`(document.dispatchEvent(new Event('visibilitychange')), true)`);
    await reread();
    const pillOn = () =>
      waitFor(
        page,
        `!document.getElementById('pill').hidden && document.getElementById('pill').textContent`,
        5000
      );
    const pill = await pillOn();
    const after = await page.evalJSON(
      `({ y: scrollY, n: document.querySelectorAll('.post').length })`
    );
    if (pill !== '1 new post' || after.y !== top || after.n !== 24)
      fail(`${where}: pill ${pill}, scroll ${top} → ${after.y}, ${after.n} posts`);
    await shoot(`pill-${w}x${h}-dark`);
    const press = async (k, code) => {
      await page.s('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, text: k });
      await page.s('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code });
    };
    await press('.', 'Period');
    const shown = await waitFor(
      page,
      `document.querySelectorAll('.post').length === 25 && document.activeElement.textContent`,
      3000
    );
    if (!shown?.includes('the-new-one') || (await page.evalJSON('scrollY')) !== 0)
      fail(`${where}: "." did not show the new post on top (${shown})`);
    await press('j', 'KeyJ');
    if (!(await page.evalJSON('document.activeElement.textContent')).includes('demo-0'))
      fail(`${where}: j did not move to the next post`);
    add('deploy-00000000000000ef', 'Deploy: newer-still');
    await page.evalJSON(`(scrollTo(0, 400), true)`);
    await reread();
    await pillOn();
    await page.evalJSON(`(document.querySelector('#tabs [aria-current]').click(), true)`);
    const atTop = await waitFor(
      page,
      `scrollY === 0 && document.querySelectorAll('.post').length === 26 && document.getElementById('pill').hidden`,
      3000
    );
    if (!atTop) fail(`${where}: the active tab did not go to the top with what is new`);
    await page.evalJSON(`(scrollTo(0, 500), true)`);
    await sleep(100);
    const y0 = await page.evalJSON('scrollY');
    await page.evalJSON(`(document.querySelectorAll('.post .open')[9].click(), true)`);
    const inThread = await waitFor(
      page,
      `location.pathname.startsWith('/feed/deploy-') && document.querySelector('.main') && document.activeElement.id === 'title' && scrollY === 0`,
      5000
    );
    if (!inThread) fail(`${where}: a tapped post did not open its thread`);
    await page.evalJSON(`(document.getElementById('back').click(), true)`);
    const backAt = await waitFor(
      page,
      `location.pathname === '/feed' && scrollY === ${y0} && document.activeElement.classList.contains('open')`,
      3000
    );
    if (!backAt)
      fail(
        `${where}: back did not find the timeline where it was (${await page.evalJSON('scrollY')} for ${y0})`
      );
    // a correction's chip goes to the post it corrects, and back again
    stub = served(all);
    await page.s('Page.navigate', { url: `${origin}/feed/${asking.id}` });
    await drawn();
    for (const id of [told[0].entries[1].post_id, told[0].entries[2].post_id]) {
      await page.evalJSON(`(document.querySelector('a.tag[href$="#${id}"]').click(), true)`);
      if (!(await waitFor(page, `document.activeElement.dataset.post === '${id}'`, 2000)))
        fail(`${where}: the chip to ${id} did not go there`);
    }
    // Show more reads the whole of a post the list cut, where it is
    await page.s('Page.navigate', { url: `${origin}/feed?kind=laptop` });
    await drawn();
    await page.evalJSON(`(document.querySelector('.post .more').click(), true)`);
    if (
      !(await waitFor(
        page,
        `document.querySelector('.post').textContent.includes('I am writing the tests meanwhile.') && document.querySelector('.post .more').getAttribute('aria-expanded') === 'true' && location.pathname === '/feed'`,
        3000
      ))
    )
      fail(`${where}: Show more did not show the whole post`);
    stub = u => served(ts)(u);
    await page.s('Page.navigate', { url: `${origin}/feed` });
    await drawn();
    // a failed read says so and keeps what is on screen
    stub = () => 502;
    await reread();
    if (
      !(await waitFor(
        page,
        `document.getElementById('banner').textContent.startsWith('Not updating')`,
        5000
      ))
    )
      fail(`${where}: a failed read is not said`);
    if ((await page.evalJSON(`document.querySelectorAll('.post').length`)) !== 26)
      fail(`${where}: a failed read took the list away`);
    await page.evalJSON(`(scrollTo(0, 0), true)`);
    await shoot(`notupdating-${w}x${h}-dark`);
    // paging: the fixture's second page follows the first, no thread twice
    where = `paging ${w}`;
    stub = null;
    await page.s('Page.navigate', { url: `${origin}/feed?kind=deploy` });
    await waitFor(page, `document.querySelector('.foot button')`, 8000);
    await page.evalJSON(`(document.querySelector('.foot button').click(), true)`);
    const ids = await waitFor(
      page,
      `document.querySelector('.foot p') && [...document.querySelectorAll('#view .post')].map(c => c.dataset.id)`,
      5000
    );
    if (!ids || ids.length < 60 || new Set(ids).size !== ids.length)
      fail(`${where}: ${ids?.length} posts, ${new Set(ids).size} different`);
    await page.close();
  }
} finally {
  srv.kill();
}
for (const [scheme, w] of Object.entries(worst))
  console.log(
    `contrast ${scheme}: text ${w.text.r.toFixed(2)} (${w.text.at} "${w.text.words}" ${w.text.where}), large ${w.large.r.toFixed(2)}, focus ring ${w.focus.toFixed(2)}, edges ${JSON.stringify(w.edges)}`
  );
if (fails.length) {
  console.log(log.split('\n').slice(-10).join('\n'));
  console.log(`FEED GATE FAIL (${fails.length})`);
  process.exit(1);
}
console.log(`FEED GATE PASS → ${OUT}`);

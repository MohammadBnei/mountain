#!/usr/bin/env node
// Unit tests for the page's pure modules (model.js, router.js, geometry.js, reader.js's highlighter) over the real
// fixture graph. Node >= 22, no deps.
import { readFileSync } from 'node:fs';
import {
  asks,
  clean,
  fresh,
  isLong,
  lastEvent,
  linkOf,
  posts,
  rel,
  route,
  sig,
  speaker,
  titleOf
} from '../js/feed.js';
import { blocks, httpsURL, inline } from '../js/feed-md.js';
import { cardSlots, heightOf, layout, MAX_CARDS } from '../js/geometry.js';
import {
  adrOfDoc,
  alertsOn,
  buildModel,
  docOf,
  fold,
  governs,
  groupOf,
  health,
  label,
  MAX_GROUPS,
  namespaces,
  REALMS,
  realmGoverns,
  routes,
  search,
  sourceURL,
  use,
  workedTiles
} from '../js/model.js';
import { readError, tokens } from '../js/reader.js';
import { format, parse, up } from '../js/router.js';
import { connect, gateLapsed, SIGNED_OUT, signedOut } from '../js/socket.js';
import { status } from '../js/ui.js';

let failed = 0;
const ok = (c, m) => {
  if (!c) {
    failed++;
    console.log(`FAIL ${m}`);
  } else console.log(`ok   ${m}`);
};
const g = JSON.parse(readFileSync(new URL('./fixtures/graph.json', import.meta.url)));
const live = JSON.parse(readFileSync(new URL('./fixtures/live.json', import.meta.url)));
const m = buildModel(g);

// ---- model: realms and groups (the load rule)
ok(
  REALMS.every(r => m.realms[r].groups.size >= 1 && m.realms[r].groups.size <= MAX_GROUPS),
  `every realm has 1..${MAX_GROUPS} groups (the load rule)`
);
const placed = g.nodes.filter(n => n.realm).length;
ok(
  REALMS.reduce((s, r) => s + m.realms[r].count, 0) === placed,
  'every placed node is in exactly one realm'
);
ok(
  m.outside.cloudflare.length === g.nodes.filter(n => n.kind === 'dns').length &&
    !('github' in m.outside),
  'hostnames sit at the rim'
);
ok(
  g.nodes.filter(n => n.kind === 'repo').every(n => groupOf(m, n.id)?.realm === 'code') &&
    m.realms.code.count === g.nodes.filter(n => n.kind === 'repo').length,
  'repos are the code realm (R12)'
);
ok(
  m.adrs.length === g.nodes.filter(n => n.kind === 'adr').length,
  'ADRs are indexed, never placed on an island'
);
const bad = buildModel({
  nodes: [{ id: 'app:a', kind: 'app', realm: 'apps', group: 'x' }],
  edges: [{ from: 'app:a', to: 'app:gone', kind: 'git' }]
});
ok(bad.edges.length === 0, 'an edge to a node not in the projection is dropped, never drawn');
ok(
  fold(m, m.realms.platform.groups.get('nodes').members).every(f => f.kind === 'vm'),
  'a group folds by node kind'
);

// ---- routes: declared, aggregated to groups, weighted by observed traffic
const rs = routes(m, live);
ok(
  rs.every(r => ['route', 'git', 'runs-on'].includes(r.kind)),
  'only trade edges are routes (no governs, no ADR relations)'
);
ok(
  rs.every(r => r.from !== r.to),
  'no route to itself'
);
const https = rs.filter(r => r.kind === 'route');
ok(
  https.every(r => r.from === 'outside/cloudflare'),
  'https routes come in from Cloudflare'
);
ok(
  https.some(r => r.observed && r.rps > 0) && https.every(r => r.observed === r.rps > 0),
  'observed = Traefik measured traffic on it'
);
const noTraffic = routes(m, null);
ok(
  noTraffic.every(r => !r.observed),
  'without a live frame every route is declared only'
);
ok(
  rs.reduce((s, r) => s + r.count, 0) ===
    g.edges.filter(
      e =>
        ['route', 'git', 'runs-on'].includes(e.kind) &&
        (() => {
          const a = m.nodes.get(e.from),
            b = m.nodes.get(e.to);
          return (
            a &&
            b &&
            (a.realm ? `${a.realm}/${a.group}` : a.kind) !==
              (b.realm ? `${b.realm}/${b.group}` : b.kind)
          );
        })()
    ).length,
  'route counts add up to the edges they carry'
);

// ---- live joins
ok(health(m, live, 'app:wird-adminweb').state === 'degraded', 'a Degraded app is degraded');
ok(
  health(m, live, 'vm:k8s-worker-02').state === 'down' &&
    health(m, live, 'vm:k8s-cp-01').state === 'ok',
  'a k8s node is down when not Ready'
);
ok(health(m, live, 'host:server1').state === 'ok', 'a host answering its probe is ok');
ok(health(m, live, 'ct:hermesagent').state === 'down', 'a guest whose probe fails is down');
ok(health(m, null, 'app:wird').state === 'unknown', 'no live frame: unknown, never ok');
ok(
  alertsOn(m, live, { realm: 'platform' }).length === 1 &&
    alertsOn(m, live, { id: 'vm:k8s-worker-02' }).length === 1,
  'alerts land on their realm and node'
);
ok(alertsOn(m, live, { realm: 'platform', group: 'nodes' }).length === 1, 'and on their group');
const u = use(live, namespaces(m, ['app:wird', 'app:wird-adminweb']));
ok(
  u.cpu === live.land.ns.wird.cpu &&
    Math.abs(u.share.cpu - live.land.ns.wird.cpu / live.land.capacity.cpu) < 1e-9,
  'a shared namespace counts once'
);
ok(use({}, ['x']).share.cpu === null, 'no capacity: no share, not 0%');

// ---- search, governs, labels, links
ok(search(m, 'k8s-worker-02')[0] === 'vm:k8s-worker-02', 'search: an exact name first');
ok(
  search(m, 'adr-0065').some(id => id.endsWith('/0065')),
  'search: ADR-NNNN finds the ADRs'
);
ok(
  search(m, '').length === 0 && search(m, 'zzzz-nothing').length === 0,
  'search: nothing for nothing'
);
const core = governs(m, 'svc:agent-fleet/core');
ok(
  core.length > 0 && core.every((x, i) => i === 0 || core[i - 1].count >= x.count),
  'governs: the ADRs citing a component, most cited first'
);
ok(
  groupOf(m, 'app:wird')?.realm === 'apps' &&
    groupOf(m, 'repo:mountain')?.realm === 'code' &&
    groupOf(m, 'dns:wird.bnei.dev') === null,
  'groupOf: placed nodes only'
);
ok(
  /^infra-bootstrap ADR-0065: /.test(label(m.nodes.get('adr:infra-bootstrap/0065'))),
  'an ADR reads as its repo, number and title'
);
const url = sourceURL({ source: { repo: 'infra-bootstrap', path: 'gitops/a b.yaml', sha: 'abc' } });
ok(
  url === 'https://github.com/MohammadBnei/infra-bootstrap/blob/abc/gitops/a%20b.yaml',
  'a source link is GitHub at the sha, path-encoded'
);
ok(sourceURL({ source: { repo: 'x', path: '' } }) === null, 'no path: no link');

// ---- router
for (const h of [
  '#/',
  '#/L1',
  '#/L1/fleet',
  '#/L1/fleet?g=control',
  '#/L1/platform?g=nodes&sel=vm%3Ak8s-cp-01&lens=land',
  '#/?sel=adr%3Ainfra-bootstrap%2F0065'
])
  ok(format(parse(h)) === h, `router: ${h} round-trips`);
ok(
  parse('#/L1/nowhere').realm === null && parse('#/L9').level === 0,
  'router: an unknown realm or level falls back'
);
ok(
  parse('#/?sel=javascript:alert(1)').sel === null && parse('#/L1/fleet?g=<b>').group === null,
  'router: a sel or group that is not an id is dropped'
);
ok(parse('#/?g=control').group === null, 'router: a group needs its realm');
let v = parse('#/L1/platform?g=nodes&sel=vm%3Ak8s-cp-01');
const steps = [];
for (let i = 0; i < 5; i++) {
  v = up(v);
  steps.push(format(v));
}
ok(
  steps.join(' ') === '#/L1/platform?g=nodes #/L1/platform #/L1 #/ #/',
  'router: up zooms out one step at a time, to the map'
);

// ---- router, L2..L4
for (const h of [
  '#/L2/fleet',
  '#/L2/fleet?g=control&sel=svc%3Aagent-fleet%2Fcore',
  '#/L2/apps?lens=land',
  '#/L3/svc:agent-fleet/core',
  '#/L3/vm:k8s-cp-01?lens=land',
  '#/L4/agent-fleet/docs/adr/0048-x.md?from=svc%3Aagent-fleet%2Fcore',
  '#/L4/infra-bootstrap/gitops/a%20b.yaml'
])
  ok(format(parse(h)) === h, `router: ${h} round-trips`);
ok(parse('#/L2').level === 0 && parse('#/L2/nowhere').level === 0, 'router: a city needs a realm');
ok(
  parse('#/L3/<b>').level === 0 && parse('#/L3/svc:').level === 0,
  'router: a building needs a node id'
);
for (const bad of [
  '#/L4/infra-bootstrap/../etc/passwd',
  '#/L4/infra-bootstrap/./x.md',
  '#/L4/infra-bootstrap/%2Fetc%2Fpasswd',
  '#/L4/a%2Fb/x.md',
  '#/L4/x'
])
  ok(parse(bad).level === 0, `router: ${bad} is not a document`);
ok(
  parse('#/L4/infra-bootstrap//etc/passwd').doc.path === 'etc/passwd',
  'router: an empty segment collapses, the path stays in the repo'
);
ok(
  parse('#/L4/a/b.md?from=%3Cb%3E').from === null &&
    format(parse('#/L4/a/b.md?from=javascript:x')).startsWith('#/L4/'),
  'router: from is a node id, and only ever a hash route'
);
const where = id => groupOf(m, id);
v = parse('#/L4/agent-fleet/docs/adr/0048-x.md?from=svc%3Aagent-fleet%2Fcore');
const ladder = [];
for (let i = 0; i < 6; i++) {
  v = up(v, where);
  ladder.push(format(v));
}
ok(
  ladder.join(' ') ===
    '#/L3/svc:agent-fleet/core #/L2/fleet?g=control #/L2/fleet #/L1/fleet #/L1 #/',
  `router: up climbs L4 → L3 → district → city → realm → L1 → map (${ladder.join(' ')})`
);
ok(
  format(up(parse('#/L4/a/b.md'), where)) === '#/' &&
    format(up(parse('#/L3/svc:nowhere/x'), where)) === '#/',
  'router: no from, or no city: back to the map'
);
ok(
  format(up(parse('#/L4/a/b.md?lens=land'), where)) === '#/?lens=land',
  'router: up keeps the lens'
);

// ---- geometry: L2's city plan, L3's wall cards
const groups = [...m.realms.fleet.groups.values()],
  plan = layout(groups);
const blds = plan.plots.flatMap(p => p.buildings);
ok(
  blds.length === groups.reduce((s, x) => s + x.members.length, 0),
  'city: every part is one building'
);
ok(
  plan.plots.every(p =>
    p.buildings.every(b => b.x > p.x && b.x < p.x + p.w && b.z > p.z && b.z < p.z + p.d)
  ),
  'city: a building stands inside its district'
);
ok(
  plan.plots.every((p, i) =>
    plan.plots.every(
      (q, j) =>
        i === j || p.x + p.w <= q.x || q.x + q.w <= p.x || p.z + p.d <= q.z || q.z + q.d <= p.z
    )
  ),
  'city: districts never overlap'
);
ok(
  new Set(blds.map(b => `${b.x},${b.z}`)).size === blds.length,
  'city: no two buildings on one spot'
);
const cx = plan.plots.reduce((s, p) => Math.min(s, p.x), Infinity) + plan.width / 2,
  cz = plan.plots.reduce((s, p) => Math.min(s, p.z), Infinity) + plan.depth / 2;
ok(Math.abs(cx) < 1e-9 && Math.abs(cz) < 1e-9, 'city: centred on the origin');
ok(layout([]).plots.length === 0, 'city: no groups, no districts');
ok(
  heightOf(0) === 0.5 && heightOf(3) > heightOf(1) && heightOf(1023) === 6,
  'city: height grows with citations, by log'
);
for (let n = 1; n <= MAX_CARDS; n++) {
  const s = cardSlots(n),
    ang = s.map(x => Math.atan2(x.z, x.x));
  const gap = Math.min(
    ...ang.flatMap((a, i) =>
      ang.slice(i + 1).map(b => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b))))
    )
  );
  ok(
    s.length === n &&
      (n === 1 || gap >= Math.PI / 4.5 - 1e-9) &&
      s.every(x => Math.abs(Math.hypot(x.x, x.z) - 3.4) < 1e-9),
    `building: ${n} cards on the ring, ≥ 40° apart`
  );
}
ok(
  Math.abs(cardSlots(1)[0].x) < 1e-9 && cardSlots(1)[0].z > 0,
  'building: one card faces the default camera'
);

// ---- L2's panel joins: worked tiles (where a realm's work runs), what governs a realm, a node's document
const tiles = workedTiles(m, live, 'fleet');
ok(
  tiles.length > 0 &&
    tiles.every(t => t.host.startsWith('host:') && t.capCpu > 0 && t.cpu <= t.capCpu),
  "worked tiles: a realm's use, per PVE host, within its capacity"
);
ok(
  workedTiles(m, null, 'fleet').length === 0 && workedTiles(m, { land: {} }, 'fleet').length === 0,
  'worked tiles: no live frame, no tiles'
);
const rg = realmGoverns(m, 'fleet');
ok(
  rg.length > 0 &&
    rg.every((x, i) => i === 0 || rg[i - 1].count >= x.count) &&
    rg.every(x => m.nodes.get(x.id)?.kind === 'adr'),
  'realm governs: its ADRs, most cited first'
);
const adr = m.nodes.get('adr:agent-fleet/0048');
ok(
  adrOfDoc(m, docOf(adr)) === adr &&
    adrOfDoc(m, { repo: 'agent-fleet', path: 'README.md' }) === null &&
    docOf({ source: { repo: 'x', path: '' } }) === null,
  "docOf / adrOfDoc: a node's file and back"
);

// ---- the reader's highlighter: classes only, and the text survives exactly
const lines = [
  'func main() { return 42 } // done',
  'name: "a # not a comment" # but this is',
  "const x = 'y'; # hash in js",
  'apiVersion: v1',
  ''
];
for (const [line, lang] of [
  [lines[0], 'go'],
  [lines[1], 'yaml'],
  [lines[2], 'js'],
  [lines[3], 'yaml'],
  [lines[4], 'go']
])
  ok(
    tokens(line, lang)
      .map(([, t]) => t)
      .join('') === line,
    `reader: ${JSON.stringify(line)} survives tokenising`
  );
const go = tokens(lines[0], 'go');
ok(
  go.some(([c, t]) => c === 'k' && t === 'func') &&
    go.some(([c, t]) => c === 'n' && t === '42') &&
    go.some(([c, t]) => c === 'c' && t === '// done'),
  'reader: keywords, numbers, comments'
);
const yaml = tokens(lines[1], 'yaml');
ok(
  yaml.some(([c, t]) => c === 's' && t === '"a # not a comment"') &&
    yaml.at(-1)[0] === 'c' &&
    yaml.at(-1)[1] === '# but this is',
  'reader: a # inside a string is not a comment'
);
ok(!tokens(lines[2], 'js').some(([c]) => c === 'c'), 'reader: # is no comment in js');
ok(tokens(lines[3], 'yaml')[0][0] === 'k', 'reader: a manifest key reads as a keyword');
const long = '"' + '\\"'.repeat(160000),
  t0 = Date.now(),
  lt = tokens(long, 'js');
ok(
  Date.now() - t0 < 200 && lt.length === 1 && lt[0][1] === long,
  'reader: a very long line is shown plain, at once (no quadratic backtracking)'
);

// ---- socket: a lapsed sign-in reloads the page once (the gate's redirect, or atlas's own session ending)
const store = () => {
  const m = new Map();
  return { getItem: k => m.get(k) ?? null, setItem: (k, v) => m.set(k, v) };
};
const probe = async res => {
  let n = 0;
  await gateLapsed({ fetchFn: async () => res, reload: () => n++, store: store(), now: 1e12 });
  return n;
};
const json = (b, status = 200) => ({
  type: 'basic',
  status,
  ok: status < 300,
  json: async () => b
});
ok(
  (await probe({ type: 'opaqueredirect', status: 0, ok: false })) === 1,
  "socket: the gate's redirect reloads"
);
ok(
  (await probe(json({ enabled: true, admin: false }))) === 1,
  "socket: atlas's session ended (expired, gate still open) reloads"
);
ok(
  (await probe(json({ enabled: true, admin: true }))) === 0,
  'socket: still an admin: no reload (atlas is down, not the session)'
);
ok(
  (await probe(json({ enabled: false, admin: true }))) === 0 && (await probe(json({}, 503))) === 0,
  'socket: dev, or a 5xx: no reload'
);
const st = store();
let reloads = 0;
for (const now of [1e12, 1e12 + 60e3])
  await gateLapsed({
    fetchFn: async () => json({ enabled: true, admin: false }),
    reload: () => reloads++,
    store: st,
    now
  });
ok(reloads === 1, 'socket: at most one reload per 5 min');

// ---- socket: a sign-out stops the page for good (no reload into the sign-in); any other close reconnects
ok(
  (await probe(json({ enabled: true, admin: false, signedOut: true }))) === 0,
  'socket: signed out (/auth/me says so): no reload'
);
ok(
  (await gateLapsed({
    fetchFn: async () => json({ enabled: true, admin: false, signedOut: true }),
    reload: () => {},
    store: store(),
    now: 1e12
  })) === 'signed-out',
  'socket: signed out is answered as such'
);
ok(
  (await gateLapsed({
    fetchFn: async () => json({ enabled: true, admin: true, signedOut: true }),
    reload: () => {},
    store: store(),
    now: 1e12
  })) === false,
  'socket: an admin is never signed out'
);
const so = res =>
  signedOut({
    fetchFn: async () => {
      if (res instanceof Error) throw res;
      return res;
    }
  });
ok(
  (await so(json({ enabled: true, admin: false, signedOut: true }))) === true,
  'page: /auth/me says signed out'
);
ok(
  (await so(json({ enabled: true, admin: true, signedOut: true }))) === false &&
    (await so(json({ enabled: true, admin: false }))) === false,
  'page: an admin, or an expiry, is not signed out'
);
ok(
  (await so(json({ signedOut: true }, 503))) === false &&
    (await so(new Error('down'))) === false &&
    (await so({
      type: 'basic',
      status: 200,
      ok: true,
      json: async () => {
        throw new SyntaxError('x');
      }
    })) === false,
  'page: a 5xx, no answer or no JSON says nothing'
);
ok(
  SIGNED_OUT === 4001,
  "socket: SIGNED_OUT is the server's close code (cmd/atlas/hub.go closeSignedOut)"
);
{
  const real = {
    WebSocket: globalThis.WebSocket,
    setTimeout: globalThis.setTimeout,
    clearTimeout: globalThis.clearTimeout,
    addEventListener: globalThis.addEventListener
  };
  let socks,
    seen,
    on,
    timers,
    ids = 0;
  const start = probe => {
    socks = [];
    seen = [];
    on = {};
    timers = new Map();
    return connect({ url: 'ws://x/ws', onFrame: () => {}, onStatus: s => seen.push(s), probe });
  };
  const tick = () => {
    const due = [...timers.values()];
    timers.clear();
    due.forEach(f => {
      f();
    });
  }; // the reconnect timers fire
  const settle = () => new Promise(r => real.setTimeout(r, 0)); // the probe answers
  globalThis.WebSocket = class {
    constructor() {
      this.closes = 0;
      socks.push(this);
    }
    close() {
      this.closes++;
    }
  };
  globalThis.setTimeout = f => {
    timers.set(++ids, f);
    return ids;
  };
  globalThis.clearTimeout = id => timers.delete(id);
  globalThis.addEventListener = (type, f) => {
    on[type] = f;
  };
  try {
    let probes = 0;
    start(() => probes++);
    socks[0].onclose({ code: 1006 });
    tick();
    ok(socks.length === 2 && seen.at(-1) === 'closed', 'socket: a dropped connection reconnects');
    socks[1].onclose({ code: SIGNED_OUT });
    tick();
    ok(
      socks.length === 2 && seen.at(-1) === 'signed-out' && probes === 0,
      'socket: signed out: no reconnect, no probe (it would sign straight back in)'
    );
    on.pageshow({ persisted: true });
    ok(
      socks.length === 2,
      'socket: signed out: a page restored from the back/forward cache does not reconnect'
    );
    start(() => 0);
    on.pagehide({ persisted: true });
    on.pageshow({ persisted: true });
    ok(socks.length === 2, 'socket: a page restored from the back/forward cache reconnects'); // the positive control

    let answer;
    start(
      () =>
        new Promise(r => {
          answer = r;
        })
    ); // the socket was down at the sign-out: its probe finds it
    socks[0].onclose({ code: 1006 });
    tick();
    socks[1].onclose({ code: 1006 }); // the 2nd failure probes; its reconnect timer is pending
    answer('signed-out');
    await settle();
    ok(
      seen.at(-1) === 'signed-out' && socks[1].closes === 1,
      'socket: a probe answering signed out stops'
    );
    tick();
    ok(socks.length === 2, 'socket: ... and the pending reconnect never runs');
    socks[1].onclose({ code: 1006 });
    ok(
      seen.at(-1) === 'signed-out' && timers.size === 0,
      'socket: ... nor does a late close revive it'
    );
    const c = start(
      () =>
        new Promise(r => {
          answer = r;
        })
    );
    socks[0].onclose({ code: 1006 });
    tick();
    socks[1].onclose({ code: 1006 });
    c.close();
    answer('signed-out');
    await settle();
    ok(seen.at(-1) === 'closed', 'socket: a probe answering after close() says nothing');
    start(async () => false);
    socks[0].onclose({ code: 1006 });
    tick();
    socks[1].onclose({ code: 1006 });
    await settle();
    tick();
    ok(
      seen.at(-1) === 'closed' && socks.length === 3,
      'socket: a probe saying nothing keeps reconnecting'
    );
  } finally {
    Object.assign(globalThis, real);
  }
  ok(
    status(null, null, 'signed-out') === 'Signed out: reload to sign in again' &&
      status(null, null, 'closed') === 'Live connection lost, reconnecting…',
    'status: signed out is said as such'
  );
}

// ---- reader: a failed /api/read is said by its key (atlas's error model), else by its status
const held =
  'atlas does not hold this file at the sha it read (moved, renamed, or a repo it does not read).';
ok(
  readError(404, { key: 'read.repo' }) === held && readError(404, { key: 'read.file' }) === held,
  'reader: a file or repo atlas does not hold'
);
ok(
  readError(400, { key: 'read.too_large' }) === 'The file is larger than the reader reads.' &&
    readError(400, { key: 'read.not_text' }) === 'Not a text file.',
  'reader: too large, not text (400 since the model)'
);
ok(
  readError(400, { key: 'read.path' }) === 'atlas answered 400.' &&
    readError(403, null) === 'atlas answered 403.' &&
    readError(502, null) === 'atlas answered 502.',
  "reader: anything else says the status (the gate's plain 403, a proxy's page)"
);
ok(readError(404, null) === held, "reader: a 404 without the model's body still says not held");

// ---- the feed's page (feed.js): every word and link from an event is hostile until shown otherwise
ok(
  clean('a\u202Eb\u2066c\u200Bd\u0000e\u2028f\n\tg') === 'abcdef\n\tg' && clean(null) === '',
  'feed: bidi, zero-width and control characters are dropped, line breaks stay'
);
{
  const bad = [
    'javascript:alert(1)',
    'JaVaScRiPt:alert(1)',
    'java\nscript:alert(1)',
    'data:text/html,x',
    'http://evil.example/',
    '//evil.example',
    'https:evil.example',
    'https://u:p#evil.example/'.replace('#', String.fromCharCode(64)),
    'https://git.bnei.dev#evil.example/'.replace('#', String.fromCharCode(64)),
    'https://',
    '/auth/logout',
    '../x',
    'blob:https://x/y'
  ];
  let n = 0;
  for (const kind of ['run', 'generator', 'deploy', 'previous', 'revision', '__proto__', 'x'])
    for (const ref of bad)
      for (const quoted of [true, false]) if (linkOf({ kind, ref, quoted }).href) n++;
  ok(n === 0, 'feed: no hostile ref becomes a link, whatever its kind');
  const run = linkOf({ kind: 'run', ref: 'https://forge.example.org/a/b?c#d', quoted: true });
  ok(
    run.href === 'https://forge.example.org/a/b?c#d' && run.host === 'forge.example.org' && run.out,
    'feed: an https link keeps its host in sight'
  );
  ok(
    linkOf({ kind: 'run', ref: 'https://b\u0430nk.example/', quoted: true }).host.startsWith(
      'xn--'
    ),
    'feed: a lookalike host shows as punycode'
  );
  ok(
    linkOf({ kind: 'deploy', ref: 'deploy-0011223344556677', quoted: false }).href ===
      '/feed/deploy-0011223344556677' &&
      !linkOf({ kind: 'deploy', ref: 'deploy-0011223344556677', quoted: true }).href &&
      !linkOf({ kind: 'deploy', ref: 'deploy-0011223344556677/../../auth', quoted: false }).href,
    "feed: a thread link is the system's and nothing but an id"
  );
}
{
  const of = (e, kind = 'session') => speaker(e, { kind, title: 'Session: tidy' });
  ok(
    of({ actor: 'system' }, 'alert').name === 'Alerts' &&
      of({ actor: 'system' }, 'pr').handle === 'forge' &&
      of({}, 'toString').name === 'System' &&
      of({ actor: 'owner' }).who === 'owner' &&
      of({ actor: 'ops-lead', quoted: true }).who === 'agent' &&
      of({ actor: '\u202E', quoted: true }).who === 'system' &&
      of({ actor: 'system', quoted: true, markdown: true }).name === 'tidy' &&
      of({ actor: 'system', markdown: true, by: 'b' }).who === 'session',
    "feed: who posts (a session's words are the session's, never the system's)"
  );
  const ps = posts({
    entries: [
      { time: '1', actor: 'system', text: 'comment', by: 'octo' },
      { time: '1', actor: 'system', text: 'words', quoted: true, by: 'octo' },
      { time: '2', actor: 'system', text: 'alone', quoted: true },
      { time: '2', actor: 'ops-lead', text: 'hi' },
      { time: '2', actor: 'system', text: 'outside', quoted: true },
      { time: '3', actor: 'system', text: '**a**', quoted: true, markdown: true },
      { time: '3', actor: 'system', quoted: true, text: 'q' },
      { time: '3', actor: 'system', quoted: true, claim: true, by: 's' }
    ]
  });
  ok(
    ps.length === 7 &&
      ps[0].quotes.length === 1 &&
      ps[0].e.by === '' &&
      ps[1].quotes[0].text === 'alone' &&
      ps[1].e.text === '' &&
      ps[2].quotes.length === 0 &&
      ps[3].quotes[0].text === 'outside' &&
      ps[4].e.markdown &&
      ps[4].quotes.length === 0 &&
      ps[5].quotes.length === 1 &&
      ps[6].e.claim &&
      ps.every((p, k) => k === 0 || p.i > ps[k - 1].i),
    "feed: a quote is a card in the system's post it came with, never in an agent's or a session's; a login shows once"
  );
  ok(
    asks({ asks_owner: true }) &&
      asks({ needs_list: ['x'] }) &&
      !asks({ needs_list: [] }) &&
      !asks({}),
    'feed: what asks the owner'
  );
}
ok(
  titleOf({ kind: 'alert', title: 'Alert: Alert: x' }) === 'Alert: x' &&
    titleOf({ kind: 'pr', title: 'PR a/b#1: t' }) === 'a/b#1: t' &&
    titleOf({ kind: 'toString', title: 'x' }) === 'x',
  "feed: a title drops its kind's prefix once"
);
ok(
  route('/feed').tab === 'all' &&
    route('/feed', '?kind=pr').tab === 'pr' &&
    route('/feed', '?tab=needs').tab === 'needs' &&
    route('/feed', '?kind=constructor').tab === 'all' &&
    route('/feed/pr-0011223344556677').id === 'pr-0011223344556677' &&
    !route('/feed/pr-0011223344556677/x').id,
  'feed: the route'
);
{
  const n = Date.parse('2026-11-24T12:00:00Z'),
    at = m => new Date(n - m * 60e3).toISOString();
  ok(
    rel(n - 30e3, n) === 'now' &&
      rel(n - 5 * 60e3, n) === '5m' &&
      rel(n - 119 * 60e3, n) === '1h' &&
      rel(n - 49 * 3600e3, n) === '2d' &&
      rel(n + 5e3, n) === 'now' &&
      rel(Number.NaN, n) === '?',
    'feed: relative times'
  );
  const th = (id, m, more) => ({
    id,
    last_activity_at: at(m),
    needs_owner: false,
    state: 'open',
    ...more
  });
  const rows = [th('a', 1), th('b', 5)],
    shown = new Map(rows.map(t => [t.id, sig(t)]));
  ok(
    fresh(shown, rows, at(5)) === 0 &&
      fresh(shown, [th('c', 0), ...rows], at(5)) === 1 &&
      fresh(shown, [th('a', 0), th('b', 5)], at(5)) === 1 &&
      fresh(shown, [th('a', 1, { needs_owner: true })], at(5)) === 1 &&
      fresh(shown, [th('old', 9)], at(5)) === 0 &&
      fresh(shown, [th('old', 9)]) === 1,
    'feed: what counts as new (a thread that only slid up into the page does not)'
  );
  ok(
    lastEvent({ last_event_age: null }, n) === null &&
      lastEvent(undefined, n) === null &&
      lastEvent({ last_event_age: 10, newest_activity_at: at(300) }, n) === n - 300 * 60e3 &&
      lastEvent({ last_event_age: 600, newest_activity_at: at(-60) }, n) === n - 600e3,
    'feed: the last event is the older of the two ages'
  );
}
ok(
  isLong('x'.repeat(10000)) && isLong('a\n'.repeat(9)) && !isLong('short'),
  'feed: what is folded'
);

// ---- a session's Markdown (feed-md.js): a small subset, hostile until shown otherwise, linear in its length
{
  const flat = ns =>
    ns.map(n => (typeof n === 'string' ? n : `<${n.tag}>${flat(n.kids)}</${n.tag}>`)).join('');
  const is = (src, want) =>
    ok(flat(inline(src)) === want, `feed md: ${JSON.stringify(src)} is ${want}`);
  is('a **b *c* d** e', 'a <strong>b <em>c</em> d</strong> e');
  is('snake_case_name and __b__', 'snake_case_name and <strong>b</strong>');
  is('`x*y*` ``a`b``', '<code>x*y*</code> <code>a`b</code>');
  is('open **bold and `code', 'open **bold and `code');
  is('\\*not\\* a \\`code\\`', '*not* a `code`');
  is('2 * 3 * 4', '2 * 3 * 4');
  is('<b onclick=x>&amp;</b>', '<b onclick=x>&amp;</b>');
  is('![i](https://a.example/x.png)', '![i](https://a.example/x.png)');
  is('https://a.example/bare', 'https://a.example/bare');
  for (const u of [
    'javascript:alert(1)',
    'http://a.example',
    '//a.example',
    'data:text/html,x',
    'https://u:p#a.example/',
    '/auth/logout',
    'https:a.example'
  ])
    is(
      `[t](${u.replace('#', String.fromCharCode(64))})`,
      `[t](${u.replace('#', String.fromCharCode(64))})`
    );
  const a = inline('see [t](https://b\u0430nk.example/p)')[1];
  ok(
    a.tag === 'a' && a.host.startsWith('xn--') && a.kids[0] === 't',
    'feed md: a link keeps its host in sight, as punycode'
  );
  ok(
    httpsURL('https://a.example/')?.host === 'a.example' && !httpsURL('https://'),
    'feed md: httpsURL'
  );
  const t = src =>
    blocks(src)
      .map(b => b.t)
      .join();
  ok(
    t(
      '# h\n\np\np\n\n- a\n  - b\n- c\n\n1. x\n\n|a|b|\n|-|-|\n|1|2|\n\n> q\n\n---\n```\n**no**\n```\nend'
    ) === 'h,p,list,list,table,quote,hr,code,p',
    'feed md: the blocks'
  );
  const l = blocks('- a\n  - b\n    - c\n- d')[0];
  ok(
    l.items.length === 2 && l.items[0].sub.items.length === 2 && !l.items[0].sub.items[0].sub,
    'feed md: one nested level'
  );
  ok(
    t('```\nnever closed \n# x') === 'code' && t('a | b\nc') === 'p' && t('|') === 'p',
    'feed md: an unclosed fence, a bar alone'
  );
  const deep = b => (b.t === 'quote' ? 1 + Math.max(0, ...b.kids.map(deep)) : 0);
  ok(
    Math.max(...blocks('>'.repeat(40) + 'x').map(deep)) === 2,
    'feed md: quotes nest twice at most'
  );
  const wide = blocks(`|${'a|'.repeat(500)}\n|-|-|\n|${'b|'.repeat(500)}`)[0];
  ok(
    wide.head.length === 12 && wide.rows[0].length === 12,
    'feed md: a row holds 12 cells, the last one the rest'
  );
  // 16 KiB of each: the whole of it in well under a second, however it repeats
  const K = 16384,
    of = x => x.repeat(Math.ceil(K / x.length)).slice(0, K);
  const run = src => {
    const t0 = performance.now();
    const walk = b => {
      if (b.text) inline(b.text);
      for (const k of b.kids ?? []) walk(k);
      for (const it of [...(b.items ?? []), ...(b.items ?? []).flatMap(x => x.sub?.items ?? [])])
        inline(it.text.join('\n'));
      for (const r of [b.head ?? [], ...(b.rows ?? [])]) r.forEach(inline);
    };
    blocks(src).forEach(walk);
    return performance.now() - t0;
  };
  let slow = [0, ''];
  for (const x of [
    '**',
    '*a',
    '_a ',
    '*a **b ',
    '***',
    '**a*_',
    '[',
    '[a](',
    '[a](b',
    '![',
    '`',
    '` ``',
    '`a``b```c',
    '> ',
    '>',
    '- ',
    '  - ',
    '- - ',
    '* * ',
    '1. ',
    '|',
    '|a',
    '|-',
    'a|b\n-|-\n',
    '```\n',
    '~~~',
    '\\',
    '# ',
    '-',
    '<b>',
    '\n',
    ' '
  ])
    for (const src of [of(x), of(`${x}\n`), `a|b\n-|-\n${of(x)}`]) {
      const ms = run(src);
      if (ms > slow[0]) slow = [ms, x];
    }
  ok(
    slow[0] < 500,
    `feed md: 16 KiB of hostile text, the slowest ${slow[0].toFixed(0)} ms (${JSON.stringify(slow[1])})`
  );
}

console.log(failed ? `${failed} FAILED` : 'all passed');
process.exit(failed ? 1 : 0);

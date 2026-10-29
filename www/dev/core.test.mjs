#!/usr/bin/env node
// Unit tests for the page's pure modules (model.js, router.js, geometry.js, reader.js's highlighter) over the real
// fixture graph. Node >= 22, no deps.
import { readFileSync } from 'node:fs';
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
    due.forEach(f => f());
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

console.log(failed ? `${failed} FAILED` : 'all passed');
process.exit(failed ? 1 : 0);

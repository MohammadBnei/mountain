#!/usr/bin/env node
// Unit tests for the page's pure modules (model.js, router.js) over the real fixture graph. Node >= 22, no deps.
import { readFileSync } from 'node:fs';
import { buildModel, routes, search, fold, use, namespaces, health, alertsOn, governs, groupOf, sourceURL, label, REALMS, MAX_GROUPS } from '../js/model.js';
import { parse, format, up } from '../js/router.js';

let failed = 0;
const ok = (c, m) => { if (!c) { failed++; console.log(`FAIL ${m}`); } else console.log(`ok   ${m}`); };
const g = JSON.parse(readFileSync(new URL('./fixtures/graph.json', import.meta.url)));
const live = JSON.parse(readFileSync(new URL('./fixtures/live.json', import.meta.url)));
const m = buildModel(g);

// ---- model: realms and groups (the load rule)
ok(REALMS.every((r) => m.realms[r].groups.size >= 1 && m.realms[r].groups.size <= MAX_GROUPS), `every realm has 1..${MAX_GROUPS} groups (the load rule)`);
const placed = g.nodes.filter((n) => n.realm).length;
ok(REALMS.reduce((s, r) => s + m.realms[r].count, 0) === placed, 'every placed node is in exactly one realm');
ok(m.outside.github.length === g.nodes.filter((n) => n.kind === 'repo').length && m.outside.cloudflare.length === g.nodes.filter((n) => n.kind === 'dns').length, 'repos and hostnames sit at the rim');
ok(m.adrs.length === g.nodes.filter((n) => n.kind === 'adr').length, 'ADRs are indexed, never placed on an island');
const bad = buildModel({ nodes: [{ id: 'app:a', kind: 'app', realm: 'apps', group: 'x' }], edges: [{ from: 'app:a', to: 'app:gone', kind: 'git' }] });
ok(bad.edges.length === 0, 'an edge to a node not in the projection is dropped, never drawn');
ok(fold(m, m.realms.infra.groups.get('nodes').members).every((f) => f.kind === 'vm'), 'a group folds by node kind');

// ---- routes: declared, aggregated to groups, weighted by observed traffic
const rs = routes(m, live);
ok(rs.every((r) => ['route', 'git', 'runs-on'].includes(r.kind)), 'only trade edges are routes (no governs, no ADR relations)');
ok(rs.every((r) => r.from !== r.to), 'no route to itself');
const https = rs.filter((r) => r.kind === 'route');
ok(https.every((r) => r.from === 'outside/cloudflare'), 'https routes come in from Cloudflare');
ok(https.some((r) => r.observed && r.rps > 0) && https.every((r) => r.observed === r.rps > 0), 'observed = Traefik measured traffic on it');
const noTraffic = routes(m, null);
ok(noTraffic.every((r) => !r.observed), 'without a live frame every route is declared only');
ok(rs.reduce((s, r) => s + r.count, 0) === g.edges.filter((e) => ['route', 'git', 'runs-on'].includes(e.kind) && (() => { const a = m.nodes.get(e.from), b = m.nodes.get(e.to); return a && b && (a.realm ? `${a.realm}/${a.group}` : a.kind) !== (b.realm ? `${b.realm}/${b.group}` : b.kind); })()).length, 'route counts add up to the edges they carry');

// ---- live joins
ok(health(m, live, 'app:wird-adminweb').state === 'degraded', 'a Degraded app is degraded');
ok(health(m, live, 'vm:k8s-worker-02').state === 'down' && health(m, live, 'vm:k8s-cp-01').state === 'ok', 'a k8s node is down when not Ready');
ok(health(m, live, 'host:server1').state === 'ok', 'a host answering its probe is ok');
ok(health(m, live, 'ct:hermesagent').state === 'down', 'a guest whose probe fails is down');
ok(health(m, null, 'app:wird').state === 'unknown', 'no live frame: unknown, never ok');
ok(alertsOn(m, live, { realm: 'infra' }).length === 1 && alertsOn(m, live, { id: 'vm:k8s-worker-02' }).length === 1, 'alerts land on their realm and node');
ok(alertsOn(m, live, { realm: 'infra', group: 'nodes' }).length === 1, 'and on their group');
const u = use(live, namespaces(m, ['app:wird', 'app:wird-adminweb']));
ok(u.cpu === live.land.ns.wird.cpu && Math.abs(u.share.cpu - live.land.ns.wird.cpu / live.land.capacity.cpu) < 1e-9, 'a shared namespace counts once');
ok(use({}, ['x']).share.cpu === null, 'no capacity: no share, not 0%');

// ---- search, governs, labels, links
ok(search(m, 'k8s-worker-02')[0] === 'vm:k8s-worker-02', 'search: an exact name first');
ok(search(m, 'adr-0065').some((id) => id.endsWith('/0065')), 'search: ADR-NNNN finds the ADRs');
ok(search(m, '').length === 0 && search(m, 'zzzz-nothing').length === 0, 'search: nothing for nothing');
const core = governs(m, 'svc:agent-fleet/core');
ok(core.length > 0 && core.every((x, i) => i === 0 || core[i - 1].count >= x.count), 'governs: the ADRs citing a component, most cited first');
ok(groupOf(m, 'app:wird')?.realm === 'apps' && groupOf(m, 'repo:mountain') === null, 'groupOf: placed nodes only');
ok(/^infra-bootstrap ADR-0065: /.test(label(m.nodes.get('adr:infra-bootstrap/0065'))), 'an ADR reads as its repo, number and title');
const url = sourceURL({ source: { repo: 'infra-bootstrap', path: 'gitops/a b.yaml', sha: 'abc' } });
ok(url === 'https://github.com/MohammadBnei/infra-bootstrap/blob/abc/gitops/a%20b.yaml', 'a source link is GitHub at the sha, path-encoded');
ok(sourceURL({ source: { repo: 'x', path: '' } }) === null, 'no path: no link');

// ---- router
for (const h of ['#/', '#/L1', '#/L1/fleet', '#/L1/fleet?g=control', '#/L1/infra?g=nodes&sel=vm%3Ak8s-cp-01&lens=land', '#/?sel=adr%3Ainfra-bootstrap%2F0065'])
  ok(format(parse(h)) === h, `router: ${h} round-trips`);
ok(parse('#/L1/nowhere').realm === null && parse('#/L9').level === 0, 'router: an unknown realm or level falls back');
ok(parse('#/?sel=javascript:alert(1)').sel === null && parse('#/L1/fleet?g=<b>').group === null, 'router: a sel or group that is not an id is dropped');
ok(parse('#/?g=control').group === null, 'router: a group needs its realm');
let v = parse('#/L1/infra?g=nodes&sel=vm%3Ak8s-cp-01');
const steps = [];
for (let i = 0; i < 5; i++) { v = up(v); steps.push(format(v)); }
ok(steps.join(' ') === '#/L1/infra?g=nodes #/L1/infra #/L1 #/ #/', 'router: up zooms out one step at a time, to the map');

console.log(failed ? `${failed} FAILED` : 'all passed');
process.exit(failed ? 1 : 0);

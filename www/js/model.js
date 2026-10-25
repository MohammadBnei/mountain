// The graph as the page reads it (plan/10 "The ladder", "The load rule"): realms, their groups, the components in
// each, the outside rim, and the trade routes between them. Pure: no DOM, no three.js (www/dev/core.test.mjs).
//
// Input: an /api/graph answer {seq, nodes, edges, drift, vector} and a live frame (cmd/atlas live.go). Every count and
// every route comes from those two; nothing here is invented.

// The seven realms (since R12 the game's civs too, plan/13), in L0 order, where they sit on the map (in map units)
// and their wire colour. Data, the smallest, holds the centre; code holds the repos (atlas places them).
export const REALMS = ['platform', 'data', 'operations', 'guard', 'fleet', 'apps', 'code'];
export const REALM = {
  apps: { label: 'Apps', at: [0, -4.6], wire: '#ffd83a' },
  guard: { label: 'Guard', at: [-5.2, -2.3], wire: '#9a8a72' },
  fleet: { label: 'Fleet', at: [5.2, -2.3], wire: '#ff7a2e' },
  data: { label: 'Data', at: [0, 0], wire: '#1ff2dc' },
  operations: { label: 'Operations', at: [-5.2, 2.3], wire: '#3fa2ff' },
  code: { label: 'Code', at: [5.2, 2.3], wire: '#ff5cc8' },
  platform: { label: 'Platform', at: [0, 4.6], wire: '#b4ff3c' },
};
// The outside (L0 rim): only what the graph names. Cloudflare holds the public hostnames (the repos are in code).
export const OUTSIDE = {
  cloudflare: { label: 'Cloudflare', kind: 'dns', at: [-10.5, 0.2] },
};
export const KIND = { host: 'host', vm: 'VM', ct: 'container', app: 'Application', svc: 'component', repo: 'repo', dns: 'hostname', adr: 'ADR', addon: 'add-on', db: 'database' };
export const MAX_GROUPS = 7;

const nameOf = (id) => id.slice(id.indexOf(':') + 1);
export { nameOf };

/** A node's display name: an ADR's number and title, else the name part of its id. */
export function label(n) {
  if (!n) return '';
  if (n.kind === 'adr') return `${n.attrs?.repo || ''} ADR-${n.attrs?.number || nameOf(n.id).split('/')[1]}${n.attrs?.title ? `: ${n.attrs.title}` : ''}`;
  return nameOf(n.id);
}

/** buildModel indexes one projection. */
export function buildModel(view) {
  const nodes = new Map(), out = new Map(), inn = new Map();
  for (const n of view.nodes || []) nodes.set(n.id, n);
  const edges = (view.edges || []).filter((e) => nodes.has(e.from) && nodes.has(e.to));
  for (const e of edges) {
    if (!out.has(e.from)) out.set(e.from, []);
    if (!inn.has(e.to)) inn.set(e.to, []);
    out.get(e.from).push(e);
    inn.get(e.to).push(e);
  }
  const realms = {};
  for (const r of REALMS) realms[r] = { id: r, ...REALM[r], groups: new Map(), count: 0 };
  const outside = { cloudflare: [] }, adrs = [];
  for (const n of nodes.values()) {
    if (n.realm && realms[n.realm]) {
      const g = realms[n.realm].groups;
      if (!g.has(n.group)) g.set(n.group, { realm: n.realm, id: n.group, members: [] });
      g.get(n.group).members.push(n.id);
      realms[n.realm].count++;
    } else if (n.kind === 'dns') outside.cloudflare.push(n.id);
    else if (n.kind === 'adr') adrs.push(n.id);
  }
  for (const r of Object.values(realms)) {
    r.groups = new Map([...r.groups.entries()].sort(([a, x], [b, y]) => y.members.length - x.members.length || a.localeCompare(b)));
    for (const g of r.groups.values()) g.members.sort();
  }
  for (const l of Object.values(outside)) l.sort();
  adrs.sort();
  const drift = Array.isArray(view.drift) ? view.drift : [];
  return { seq: view.seq, at: view.at, vector: view.vector || {}, nodes, edges, out, inn, realms, outside, adrs, drift };
}

/** groupOf is the group a component sits in: {realm, group} or null (hostnames, ADRs). */
export function groupOf(m, id) {
  const n = m.nodes.get(id);
  return n?.realm && m.realms[n.realm] ? { realm: n.realm, group: n.group } : null;
}

/** The ADRs that govern a node (governs edges from it), with citation counts, most cited first. */
export function governs(m, id) {
  return (m.out.get(id) || []).filter((e) => e.kind === 'governs')
    .map((e) => ({ id: e.to, count: Number(e.attrs?.count || 1) }))
    .sort((a, b) => b.count - a.count || a.id.localeCompare(b.id));
}

/** The kinds in a group, each with its members: the typed lists a group folds into (by node kind, a real sub-key). */
export function fold(m, members) {
  const by = new Map();
  for (const id of members) {
    const k = m.nodes.get(id)?.kind || '?';
    if (!by.has(k)) by.set(k, []);
    by.get(k).push(id);
  }
  return [...by.entries()].map(([kind, ids]) => ({ kind, ids }));
}

const endpoint = (m, id) => {
  const n = m.nodes.get(id);
  if (!n) return null;
  if (n.realm && m.realms[n.realm]) return `${n.realm}/${n.group}`;
  if (n.kind === 'dns') return 'outside/cloudflare';
  return null;
};

// The edge kinds that are trade (things moving between systems), and how each reads. governs and the ADR relations
// are knowledge, not trade: they show in a component's card, never as a route.
export const TRADE = { route: 'https', git: 'git', 'runs-on': 'runs on' };

/**
 * routes are the L1 trade routes: declared edges aggregated to group → group (or the rim), weighted by what the live
 * frame observed (Traefik's requests per second for an https route). observed = some traffic was measured on it.
 * → [{key, from, to, kind, count, rps, observed, edges}]
 */
export function routes(m, live) {
  const by = new Map();
  for (const e of m.edges) {
    if (!TRADE[e.kind]) continue;
    const from = endpoint(m, e.from), to = endpoint(m, e.to);
    if (!from || !to || from === to) continue;
    const key = `${from}>${to}>${e.kind}`;
    if (!by.has(key)) by.set(key, { key, from, to, kind: e.kind, count: 0, rps: 0, observed: false, edges: [] });
    const r = by.get(key);
    r.count++;
    r.edges.push(e);
    if (e.kind === 'route') {
      const rps = live?.apps?.[e.to]?.rps || 0;
      r.rps += rps;
      r.observed ||= rps > 0;
    }
  }
  return [...by.values()].sort((a, b) => a.key.localeCompare(b.key));
}

/** The namespaces a set of components deploy into (app nodes' namespace attr), each once. */
export function namespaces(m, ids) {
  const s = new Set();
  for (const id of ids) {
    const ns = m.nodes.get(id)?.attrs?.namespace;
    if (ns) s.add(ns);
  }
  return [...s].sort();
}

/** use sums the live land lens over namespaces: absolute CPU cores, RAM and disk bytes, and their share of capacity. */
export function use(live, nss) {
  const cap = live?.land?.capacity || {}, ns = live?.land?.ns || {}, u = { cpu: 0, mem: 0, disk: 0 };
  for (const n of nss) for (const k of ['cpu', 'mem', 'disk']) u[k] += ns[n]?.[k] || 0;
  const share = (k) => (cap[k] > 0 ? u[k] / cap[k] : null);
  return { ...u, share: { cpu: share('cpu'), mem: share('mem'), disk: share('disk') } };
}

/** Health of a component from the live frame: 'down' | 'degraded' | 'ok' | 'unknown', and why, in words. */
export function health(m, live, id) {
  const n = m.nodes.get(id);
  if (!n || !live) return { state: 'unknown', why: 'no live data yet' };
  if (n.kind === 'app') {
    const a = live.apps?.[id];
    if (!a?.health) return { state: 'unknown', why: 'not in ArgoCD\'s answer' };
    const bad = { Degraded: 'degraded', Missing: 'down', Unknown: 'unknown', Progressing: 'ok', Suspended: 'ok', Healthy: 'ok' }[a.health] || 'unknown';
    return { state: bad, why: `${a.health}${a.sync ? `, ${a.sync}` : ''}` };
  }
  if (id in (live.probes || {})) return live.probes[id] ? { state: 'ok', why: 'probe answers' } : { state: 'down', why: 'probe not answering' };
  if (id in (live.ready || {})) return live.ready[id] ? { state: 'ok', why: 'node Ready' } : { state: 'down', why: 'node not Ready' };
  if (n.kind === 'host') {
    const vms = (m.inn.get(id) || []).filter((e) => e.kind === 'runs-on').map((e) => e.from).filter((v) => v in (live.ready || {}));
    if (!vms.length) return { state: 'unknown', why: 'no k8s node on it' };
    const down = vms.filter((v) => !live.ready[v]);
    return down.length ? { state: 'degraded', why: `${down.length} of ${vms.length} k8s nodes not Ready` } : { state: 'ok', why: `${vms.length} k8s nodes Ready` };
  }
  return { state: 'unknown', why: 'nothing live measures it' };
}

/** Alerts firing on a node, a group or a realm. */
export function alertsOn(m, live, where) {
  const list = live?.alerts || [];
  if (where.id) return list.filter((a) => a.node === where.id);
  if (where.group) return list.filter((a) => { const g = a.node && groupOf(m, a.node); return g && g.realm === where.realm && g.group === where.group; });
  if (where.realm) return list.filter((a) => a.realm === where.realm);
  return list;
}

/** search matches node ids, names and ADR titles; best first, at most limit. */
export function search(m, q, limit = 20) {
  const s = q.trim().toLowerCase();
  if (!s) return [];
  const hits = [];
  for (const n of m.nodes.values()) {
    const name = nameOf(n.id).toLowerCase(), title = (n.attrs?.title || '').toLowerCase();
    const adr = n.kind === 'adr' ? `adr-${n.attrs?.number || ''}` : '';
    let score = 0;
    if (name === s || n.id === s || adr === s) score = 4;
    else if (name.startsWith(s) || adr.startsWith(s)) score = 3;
    else if (name.includes(s) || n.id.includes(s)) score = 2;
    else if (title.includes(s)) score = 1;
    if (score) hits.push({ id: n.id, score });
  }
  hits.sort((a, b) => b.score - a.score || a.id.length - b.id.length || a.id.localeCompare(b.id));
  return hits.slice(0, limit).map((h) => h.id);
}

/** A GitHub link to where a fact was read, at the sha it was read at (no sha: the default branch). */
export function sourceURL(n) {
  const s = n?.source;
  if (!s?.repo || !s.path) return null;
  return `https://github.com/MohammadBnei/${encodeURIComponent(s.repo)}/blob/${s.sha || 'HEAD'}/${s.path.split('/').map(encodeURIComponent).join('/')}`;
}

/**
 * workedTiles is a realm's share of each host (Civ's worked tiles): for each host, the CPU and RAM its k8s nodes'
 * pods of this realm use, against those nodes' allocatable. Hosts with no k8s node (or no live frame) are left out.
 * → [{host, cpu, mem, capCpu, capMem}] sorted by host id
 */
export function workedTiles(m, live, realm) {
  const nodes = live?.land?.nodes || {}, nsRealm = live?.land?.nsRealm || {}, by = new Map();
  for (const [vm, nl] of Object.entries(nodes)) {
    const host = (m.out.get(vm) || []).find((e) => e.kind === 'runs-on')?.to;
    if (!host) continue;
    const t = by.get(host) || { host, cpu: 0, mem: 0, capCpu: 0, capMem: 0 };
    t.capCpu += nl.capacity?.cpu || 0;
    t.capMem += nl.capacity?.mem || 0;
    for (const [ns, u] of Object.entries(nl.ns || {})) if (nsRealm[ns] === realm) { t.cpu += u.cpu || 0; t.mem += u.mem || 0; }
    by.set(host, t);
  }
  return [...by.values()].sort((a, b) => a.host.localeCompare(b.host));
}

/** The ADRs governing a whole realm: each ADR's citations summed over the realm's components, most cited first. */
export function realmGoverns(m, realm) {
  const sum = new Map();
  for (const g of m.realms[realm].groups.values()) for (const id of g.members) for (const x of governs(m, id)) sum.set(x.id, (sum.get(x.id) || 0) + x.count);
  return [...sum].map(([id, count]) => ({ id, count })).sort((a, b) => b.count - a.count || a.id.localeCompare(b.id));
}

/** The file a node was read from, as a reader document {repo, path}, or null. */
export function docOf(n) {
  return n?.source?.repo && n.source.path ? { repo: n.source.repo, path: n.source.path } : null;
}

/** The ADR node a reader document is, or null. */
export function adrOfDoc(m, doc) {
  for (const id of m.adrs) {
    const n = m.nodes.get(id);
    if (n?.source?.repo === doc.repo && n.source.path === doc.path) return n;
  }
  return null;
}

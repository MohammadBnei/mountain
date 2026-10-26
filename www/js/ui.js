// The page around the map: the breadcrumb, the panel (a list per view, a card per node), search results, the minimap
// and the status line. Built with createElement and textContent only: nothing from the graph is ever parsed as HTML.
import { REALMS, REALM, OUTSIDE, KIND, MAX_GROUPS, label, nameOf, groupOf, governs, fold, routes, use, namespaces, health, alertsOn, sourceURL, TRADE, workedTiles, realmGoverns, docOf, adrOfDoc } from './model.js';
import { format } from './router.js';
import { MAX_CARDS } from './geometry.js';

/** el builds an element: el('a', {href, class}, 'text', child…). Attributes are set as properties or attributes. */
export function el(tag, attrs = {}, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k === 'on') for (const [ev, f] of Object.entries(v)) e.addEventListener(ev, f);
    else e.setAttribute(k, v === true ? '' : String(v));
  }
  for (const k of kids.flat(Infinity)) if (k != null && k !== false) e.append(k instanceof Node ? k : document.createTextNode(String(k)));
  return e;
}

const pct = (x) => (x == null ? '—' : x < 0.001 && x > 0 ? '<0.1%' : `${(x * 100).toFixed(x < 0.1 ? 1 : 0)}%`);
const bytes = (b) => { if (!b) return '0'; const u = ['B', 'kB', 'MB', 'GB', 'TB']; let i = 0; while (b >= 1000 && i < 4) { b /= 1000; i++; } return `${b.toFixed(b < 10 ? 1 : 0)} ${u[i]}`; };
const cores = (c) => (c < 0.1 ? `${Math.round(c * 1000)}m` : c.toFixed(2));
const rps = (r) => (r >= 10 ? r.toFixed(0) : r.toFixed(2));
const link = (v, ...kids) => el('a', { href: format(v), class: 'nav' }, ...kids);
const dot = (state) => el('span', { class: `dot ${state}`, 'aria-hidden': 'true' });
const ago = (ms) => { const s = Math.max(0, Math.round((Date.now() - ms) / 1000)); return s < 90 ? `${s} s ago` : s < 5400 ? `${Math.round(s / 60)} min ago` : `${Math.round(s / 3600)} h ago`; };

/** crumbs is the breadcrumb for a view: ukubi › realm › group › node (› file). */
export function crumbs(m, v) {
  const out = [link({ level: 0, lens: v.lens }, 'ukubi')];
  if (v.level === 1 && !v.realm) out.push(link({ level: 1, lens: v.lens }, 'trade routes'));
  const id = v.level === 3 ? v.id : v.level === 4 ? v.from : null, g = id && groupOf(m, id);
  const realm = v.realm || g?.realm, group = v.group || g?.group;
  if (realm) out.push(link({ level: 2, realm, lens: v.lens }, REALM[realm].label));
  if (group) out.push(link({ level: 2, realm, group, lens: v.lens }, group));
  if (id) out.push(v.level === 3 ? el('span', { class: 'here' }, label(m.nodes.get(id)) || id) : link({ level: 3, id, lens: v.lens }, label(m.nodes.get(id)) || id));
  if (v.level === 4) out.push(el('span', { class: 'here' }, v.doc.path.split('/').pop()));
  else if (v.sel) out.push(el('span', { class: 'here' }, label(m.nodes.get(v.sel)) || v.sel));
  const nav = el('ol', { class: 'crumbs' });
  out.forEach((c, i) => nav.append(el('li', {}, i ? el('span', { class: 'sep', 'aria-hidden': 'true' }, '›') : null, c)));
  return nav;
}

function liveFacts(m, live, ids) {
  const nss = namespaces(m, ids), u = use(live, nss), facts = [];
  if (!live?.land?.capacity?.cpu || !nss.length) return facts; // nothing deploys into a namespace: nothing to measure
  facts.push(`cpu ${cores(u.cpu)} (${pct(u.share.cpu)})`, `ram ${bytes(u.mem)} (${pct(u.share.mem)})`);
  if (u.disk) facts.push(`disk ${bytes(u.disk)} (${pct(u.share.disk)})`);
  return facts;
}

/** signalFacts: a node's optional signals (slice 2), the known ones only: an unknown signal is left out, never a 0. */
function signalFacts(live, id) {
  const a = live?.apps?.[id], f = [];
  if (a?.err != null) f.push(`${rps(a.err)} errors/s`);
  if (a?.lat != null) f.push(`p95 ${a.lat < 1 ? `${Math.round(a.lat * 1000)} ms` : `${a.lat.toFixed(2)} s`}`);
  if (a?.reqDay != null) f.push(`${a.reqDay >= 1000 ? `${(a.reqDay / 1000).toFixed(1)}k` : a.reqDay} requests in 24 h`);
  if (a?.avail != null) f.push(`${(a.avail * 100).toFixed(a.avail < 1 ? 2 : 0)}% answered without error`);
  const pd = live?.probeDay?.[id];
  if (pd != null) f.push(`probe up ${(pd * 100).toFixed(pd < 1 ? 1 : 0)}% of 24 h`);
  const ready = live?.addons?.[id];
  if (ready != null) f.push(`${(ready * 100).toFixed(0)}% of its pods ready (its least ready workload)`);
  const pods = live?.pods?.[id];
  if (pods != null) f.push(`${pods} pods`);
  return f;
}

/** dayLine: the town's last 24 h: calm, or what broke it; unknown when its signals are. */
function dayLine(m, live) {
  const d = live?.day;
  if (!d) return el('p', { class: 'meta' }, 'Last 24 h: unknown (no answer yet, or the alert rules did not run).');
  if (d.calm) return el('p', { class: 'meta' }, 'Last 24 h: calm. No critical alert fired, no app was Degraded.');
  const deg = d.degraded.map((id) => label(m.nodes.get(id)) || id);
  return el('p', { class: 'meta' }, 'Last 24 h: not calm. ', d.critical.length ? `Critical: ${d.critical.join(', ')}. ` : '', deg.length ? `Degraded: ${deg.join(', ')}.` : '');
}

function stateCount(m, live, ids) {
  const c = { down: 0, degraded: 0 };
  for (const id of ids) { const s = health(m, live, id).state; if (s in c) c[s]++; }
  return c;
}

function alertList(list) {
  if (!list.length) return null;
  return el('section', {}, el('h3', {}, `Firing (${list.length})`),
    el('ul', { class: 'alerts' }, list.map((a) => el('li', { class: a.severity },
      el('b', {}, a.name), ' ', el('span', { class: 'mono' }, a.severity), a.summary ? el('p', {}, a.summary) : null,
      el('p', { class: 'faint' }, `since ${new Date(a.startsAt).toLocaleString()}`)))));
}

/** L0 panel: the realms, the outside, what the projection holds. */
function mapPanel(m, live, v) {
  const land = v.lens === 'land';
  const realms = el('ul', { class: 'list' }, REALMS.map((r) => {
    const realm = m.realms[r], ids = [...realm.groups.values()].flatMap((g) => g.members), c = stateCount(m, live, ids);
    const share = live?.land?.realms?.[r], firing = alertsOn(m, live, { realm: r }).length;
    return el('li', {}, link({ level: 1, realm: r, lens: v.lens },
      el('span', { class: 'swatch', style: null, 'data-realm': r, 'aria-hidden': 'true' }), el('b', {}, REALM[r].label),
      el('span', { class: 'meta' }, `${realm.groups.size} groups · ${realm.count} parts`,
        land ? (share ? ` · cpu ${pct(share.cpu)} ram ${pct(share.mem)} disk ${pct(share.disk)}` : live?.land?.capacity?.cpu ? ' · no pods in the cluster' : '') : '',
        c.down ? ` · ${c.down} down` : '', c.degraded ? ` · ${c.degraded} degraded` : '', firing ? ` · ${firing} firing` : '')));
  }));
  const un = live?.land?.realms?.unattributed;
  return el('div', {},
    el('h2', {}, 'ukubi'),
    el('p', { class: 'meta' }, `${m.nodes.size} nodes · ${m.edges.length} edges · ${m.adrs.length} ADRs · projection ${m.seq}`),
    dayLine(m, live),
    realms,
    land ? el('p', { class: 'meta' }, `Unattributed (namespaces no realm claims): cpu ${pct(un?.cpu)} ram ${pct(un?.mem)} disk ${pct(un?.disk)}. Shares are of allocatable CPU and RAM and of provisioned volume capacity, per pod, by namespace.`) : null,
    el('h3', {}, 'The outside'),
    el('ul', { class: 'list' }, Object.entries(OUTSIDE).map(([k, o]) => el('li', {}, el('b', {}, o.label), el('span', { class: 'meta' }, `${m.outside[k].length} ${KIND[o.kind]}s`)))),
    driftBlock(m),
    alertList(alertsOn(m, live, {})));
}

function driftBlock(m, subject) {
  const d = subject ? m.drift.filter((x) => x.subject === subject) : m.drift;
  if (!d.length) return null;
  const by = new Map();
  for (const x of d) by.set(x.kind, (by.get(x.kind) || 0) + 1);
  return el('details', { class: 'drift' }, el('summary', {}, `Drift: ${[...by].map(([k, n]) => `${n} ${k}`).join(', ')}`),
    el('ul', {}, d.slice(0, 50).map((x) => el('li', {}, el('span', { class: 'mono' }, x.subject), ' ', x.detail))));
}

/** L1 panel: every trade route (no realm), or one realm's groups and routes. */
function empirePanel(m, live, v) {
  const rs = routes(m, live).filter((r) => !v.realm || r.from.startsWith(v.realm + '/') || r.to.startsWith(v.realm + '/'));
  const routeItems = el('ul', { class: 'list' }, rs.map((r) => el('li', {}, el('span', { class: `wire ${r.observed ? 'seen' : 'declared'}`, 'aria-hidden': 'true' }),
    el('span', {}, endpointName(r.from), ' → ', endpointName(r.to)),
    el('span', { class: 'meta' }, `${TRADE[r.kind]} · ${r.count}`, r.kind === 'route' ? (r.observed ? ` · ${rps(r.rps)} req/s` : ' · no traffic measured') : ''))));
  if (!v.realm) {
    return el('div', {}, el('h2', {}, 'Trade routes'),
      el('p', { class: 'meta' }, 'Declared in the repos, weighted by what Traefik measured. Solid: traffic seen; dotted: declared, nothing measured. Pick a realm to focus it.'),
      el('ul', { class: 'list' }, REALMS.map((r) => el('li', {}, link({ level: 1, realm: r, lens: v.lens }, el('b', {}, REALM[r].label))))),
      el('h3', {}, `${rs.length} routes`), routeItems);
  }
  const realm = m.realms[v.realm];
  const groups = el('ul', { class: 'list' }, [...realm.groups.values()].map((g) => {
    const c = stateCount(m, live, g.members), firing = alertsOn(m, live, { realm: v.realm, group: g.id }).length;
    return el('li', {}, link({ ...v, group: g.id, sel: null }, el('b', {}, g.id),
      el('span', { class: 'meta' }, `${g.members.length} parts`, c.down ? ` · ${c.down} down` : '', c.degraded ? ` · ${c.degraded} degraded` : '', firing ? ` · ${firing} firing` : '',
        v.lens === 'land' ? ` · ${liveFacts(m, live, g.members).join(' · ')}` : '')));
  }));
  return el('div', {}, el('h2', {}, REALM[v.realm].label),
    el('p', { class: 'meta' }, `${realm.groups.size} groups · ${realm.count} parts`, realm.groups.size > MAX_GROUPS ? ` · over the ${MAX_GROUPS}-group load rule: ${realm.id === 'code' ? 'atlas folds code itself, so this is a bug' : 'realms.yaml should split it'}` : ''),
    el('p', {}, el('a', { href: format({ level: 2, realm: v.realm, lens: v.lens }), class: 'primary' }, `Enter ${REALM[v.realm].label}`)),
    groups, alertList(alertsOn(m, live, { realm: v.realm })), el('h3', {}, `Routes in and out (${rs.length})`), routeItems);
}

function endpointName(key) {
  const [a, b] = key.split('/');
  return a === 'outside' ? OUTSIDE[b].label : `${REALM[a].label} · ${b}`;
}

/** A group's panel: its members, folded by kind (a real sub-key), each with its health. */
function groupPanel(m, live, v) {
  const g = m.realms[v.realm].groups.get(v.group);
  if (!g) return el('p', {}, 'This group is not in the current projection.');
  return el('div', {}, el('h2', {}, g.id), el('p', { class: 'meta' }, `${REALM[v.realm].label} · ${g.members.length} parts`, ...liveFacts(m, live, g.members).map((f) => ` · ${f}`)),
    fold(m, g.members).map(({ kind, ids }) => el('section', {}, el('h3', {}, `${KIND[kind] || kind} (${ids.length})`),
      el('ul', { class: 'list' }, ids.map((id) => { const h = health(m, live, id); return el('li', {}, link({ ...v, sel: id }, dot(h.state), el('b', {}, nameOf(id)), el('span', { class: 'meta' }, h.why))); })))),
    alertList(alertsOn(m, live, { realm: v.realm, group: v.group })));
}

/** A node's card: what it is, where it was read, what it connects to, which ADRs govern it, what is live on it. */
function card(m, live, v) {
  const n = m.nodes.get(v.sel);
  if (!n) return el('div', {}, el('h2', {}, v.sel), el('p', {}, 'Not in the current projection.'));
  const g = groupOf(m, n.id), h = health(m, live, n.id), url = n.kind === 'repo' ? n.attrs?.url : sourceURL(n);
  const attrs = Object.entries(n.attrs || {}).filter(([k]) => !(n.kind === 'adr' && ['title', 'repo', 'number'].includes(k)));
  const edgeList = (list, dir) => {
    const by = new Map();
    for (const e of list) if (e.kind !== 'governs' || dir === 'in') { const k = e.kind; if (!by.has(k)) by.set(k, []); by.get(k).push(dir === 'out' ? e.to : e.from); }
    return [...by].map(([k, ids]) => el('section', {}, el('h3', {}, `${dir === 'out' ? k : `${k} (in)`} · ${ids.length}`),
      el('ul', { class: 'list' }, ids.slice(0, 30).map((id) => el('li', {}, nodeLink(m, v, id))), ids.length > 30 ? el('li', { class: 'meta' }, `${ids.length - 30} more`) : null)));
  };
  const gov = governs(m, n.id), a = live?.apps?.[n.id];
  return el('div', {},
    el('p', { class: 'kicker' }, KIND[n.kind] || n.kind, g ? ` · ${REALM[g.realm].label} › ${g.group}` : ''),
    el('h2', {}, label(n)),
    n.kind !== 'adr' && n.kind !== 'repo' && n.kind !== 'dns' ? el('p', {}, dot(h.state), ' ', h.why) : null,
    a?.rps ? el('p', { class: 'meta' }, `${rps(a.rps)} req/s through Traefik`) : null,
    signalFacts(live, n.id).length ? el('p', { class: 'meta' }, signalFacts(live, n.id).join(' · ')) : null,
    n.kind === 'app' ? el('p', { class: 'meta' }, liveFacts(m, live, [n.id]).join(' · ') || null, n.attrs?.namespace ? ` (namespace ${n.attrs.namespace})` : '') : null,
    attrs.length ? el('dl', {}, attrs.flatMap(([k, val]) => [el('dt', {}, k), el('dd', { class: 'mono' }, val)])) : null,
    el('p', { class: 'actions' },
      g ? el('a', { href: format({ level: 3, id: n.id, lens: v.lens }), class: 'primary' }, `Enter ${nameOf(n.id)}`) : null,
      docOf(n) ? el('a', { href: format({ level: 4, doc: docOf(n), from: g ? n.id : null, lens: v.lens }) }, n.kind === 'adr' ? 'Read it' : 'Read the file') : null),
    url ? el('p', {}, el('a', { href: url, target: '_blank', rel: 'noopener noreferrer' }, `${n.source.repo}${n.source.path ? '/' + n.source.path : ''}`), n.source.sha ? el('span', { class: 'mono faint' }, ` @${n.source.sha.slice(0, 12)}`) : null) : null,
    alertList(alertsOn(m, live, { id: n.id })),
    gov.length ? el('section', {}, el('h3', {}, `Governed by ${gov.length} ADRs`),
      el('ul', { class: 'list' }, gov.slice(0, 12).map((x) => el('li', {}, nodeLink(m, v, x.id), el('span', { class: 'meta' }, `${x.count} citation${x.count > 1 ? 's' : ''}`))), gov.length > 12 ? el('li', { class: 'meta' }, `${gov.length - 12} more`) : null)) : null,
    edgeList(m.out.get(n.id) || [], 'out'), edgeList(m.inn.get(n.id) || [], 'in'),
    driftBlock(m, n.id));
}

function nodeLink(m, v, id) {
  const n = m.nodes.get(id), g = groupOf(m, id);
  // a placed part opens in its city; anything else (an ADR, a repo, a hostname) as a card where the reader stands
  const to = g ? { level: 2, realm: g.realm, group: g.group, sel: id, lens: v.lens } : v.level >= 3 ? { level: 0, sel: id, lens: v.lens } : { ...v, sel: id };
  return link(to, label(n) || id);
}

/** L2: the city's panel (plan/10 styleboard "L2 city"): districts, the ADRs governing the realm, its share of each
 *  host, its declared routes. The build queue and troops wait for agent-kit and the bridge client (A5): no source yet. */
function cityPanel(m, live, v) {
  const realm = m.realms[v.realm], gov = realmGoverns(m, v.realm).slice(0, 8), tiles = workedTiles(m, live, v.realm);
  const rs = routes(m, live).filter((r) => r.from.startsWith(v.realm + '/') || r.to.startsWith(v.realm + '/'));
  const short = (id) => { const n = m.nodes.get(id); return `${n?.attrs?.repo || ''} ${n?.attrs?.number || nameOf(id)}`.trim(); };
  return el('div', {}, el('p', { class: 'kicker' }, 'L2 · City'), el('h2', {}, REALM[v.realm].label),
    el('p', { class: 'meta' }, `${realm.groups.size} districts · ${realm.count} parts. Height: the ADR citations that govern a building; colour: its health${v.lens === 'land' ? ' (land lens: its share of the cluster)' : ''}.`),
    el('ul', { class: 'list' }, [...realm.groups.values()].map((g) => {
      const c = stateCount(m, live, g.members);
      return el('li', {}, link({ ...v, group: g.id, sel: null }, el('b', {}, g.id), el('span', { class: 'meta' }, `${g.members.length} parts`, c.down ? ` · ${c.down} down` : '', c.degraded ? ` · ${c.degraded} degraded` : '')));
    })),
    gov.length ? el('section', {}, el('h3', {}, 'Governed by · most-cited ADRs'), el('p', { class: 'chips' }, gov.map((x) => link({ ...v, sel: x.id }, `${short(x.id)} · ${x.count}`)))) : null,
    el('section', {}, el('h3', {}, 'Worked tiles · share of each host'),
      tiles.length ? el('ul', { class: 'list' }, tiles.map((t) => el('li', {}, el('b', {}, nameOf(t.host)),
        el('span', { class: 'meta mono' }, `cpu ${pct(t.capCpu ? t.cpu / t.capCpu : null)} · ram ${pct(t.capMem ? t.mem / t.capMem : null)}`))))
        : el('p', { class: 'meta' }, live?.land ? 'None of its pods run in the cluster.' : 'No live data yet.')),
    rs.length ? el('section', {}, el('h3', {}, `Routes · declared (${rs.length})`), el('ul', { class: 'list' }, rs.map((r) => el('li', {}, el('span', {}, endpointName(r.from), ' → ', endpointName(r.to)), el('span', { class: 'meta' }, `${TRADE[r.kind]} · ${r.count}`))))) : null,
    el('p', { class: 'meta' }, 'Build queue (process stages) and troops (live sessions) arrive with agent-kit and the bridge client.'),
    alertList(alertsOn(m, live, { realm: v.realm })));
}

/** L3: a building's cards (≤ MAX_CARDS), each a {key, title, el}: on its walls in CSS3D, flat in the panel on narrow screens.
 *  events: the live events (the "Last events" card). */
export function buildingCards(m, live, events, v) {
  const n = m.nodes.get(v.id);
  if (!n) return [];
  const g = groupOf(m, n.id), h = health(m, live, n.id), a = live?.apps?.[n.id], cards = [];
  const card = (key, title, ...body) => cards.push({ key, title, el: el('article', { class: 'bcard', 'data-card': key }, el('h3', {}, title), ...body) });
  const attrs = Object.entries(n.attrs || {});
  card('facts', 'Facts', el('p', {}, KIND[n.kind] || n.kind, g ? ` · ${REALM[g.realm].label} › ${g.group}` : ''),
    attrs.length ? el('dl', {}, attrs.flatMap(([k, val]) => [el('dt', {}, k), el('dd', { class: 'mono' }, val)])) : null,
    docOf(n) ? el('p', {}, 'Read at ', link({ level: 4, doc: docOf(n), from: n.id, lens: v.lens }, `${n.source.repo}/${n.source.path}`), n.source.sha ? el('span', { class: 'mono faint' }, ` @${n.source.sha.slice(0, 12)}`) : null) : null);
  card('live', 'Live', el('p', {}, dot(h.state), ' ', h.why), a?.rps ? el('p', { class: 'mono' }, `${rps(a.rps)} req/s through Traefik`) : null,
    ...signalFacts(live, n.id).map((f) => el('p', { class: 'mono' }, f)),
    ...liveFacts(m, live, [n.id]).map((f) => el('p', { class: 'mono' }, f)));
  const trade = (list, dir) => list.filter((e) => TRADE[e.kind]).map((e) => el('li', {}, `${TRADE[e.kind]} ${dir === 'out' ? '→' : '←'} `, nodeLink(m, v, dir === 'out' ? e.to : e.from)));
  const ins = trade(m.inn.get(n.id) || [], 'in'), outs = trade(m.out.get(n.id) || [], 'out');
  if (ins.length + outs.length) card('routes', 'Routes', el('ul', { class: 'list' }, ins.slice(0, 8), outs.slice(0, 8)));
  const gov = governs(m, n.id);
  if (gov.length) card('governs', `Governed by · ${gov.length}`, el('ul', { class: 'list' }, gov.slice(0, 6).map((x) => {
    const adr = m.nodes.get(x.id), doc = docOf(adr);
    return el('li', {}, doc ? link({ level: 4, doc, from: n.id, lens: v.lens }, label(adr)) : label(adr), el('span', { class: 'meta' }, ` ${x.count}×`));
  })));
  const firing = alertsOn(m, live, { id: n.id });
  if (firing.length) card('alerts', `Firing · ${firing.length}`, el('ul', { class: 'list' }, firing.slice(0, 5).map((x) => el('li', { class: x.severity }, el('b', {}, x.name), ' ', x.summary || ''))));
  const evs = (events || []).filter((e) => e.node === n.id).slice(-5).reverse();
  if (evs.length) card('events', 'Last events', el('ul', { class: 'list' }, evs.map((e) => el('li', {}, e.text, el('span', { class: 'meta' }, ` · ${new Date(e.ts).toLocaleTimeString()}`)))));
  const drift = m.drift.filter((x) => x.subject === n.id);
  if (drift.length) card('drift', `Drift · ${drift.length}`, el('ul', { class: 'list' }, drift.slice(0, 5).map((x) => el('li', {}, `${x.kind}: ${x.detail}`))));
  return cards.slice(0, MAX_CARDS);
}

/** L3's panel: the building's name and its cards, flat (the dock on narrow screens, the list on wide ones). */
function buildingPanel(m, live, events, v, focused) {
  const n = m.nodes.get(v.id);
  if (!n) return el('div', {}, el('h2', {}, v.id), el('p', {}, 'Not in the current projection.'));
  const cards = buildingCards(m, live, events, v);
  return el('div', {}, el('p', { class: 'kicker' }, 'L3 · Building'), el('h2', {}, label(n)),
    el('p', { class: 'meta' }, 'Click a wall card to face it; Escape goes back to the city.'),
    el('nav', { class: 'cardnav', 'aria-label': 'Cards' }, cards.map((c) => el('button', { type: 'button', 'data-focus': c.key, 'aria-pressed': String(focused === c.key) }, c.title))),
    cards.map((c) => c.el));
}

/** panel is the side panel's content for a view. extra: {events, focused} for L3. */
export function panel(m, live, v, extra = {}) {
  if (v.level === 3) return buildingPanel(m, live, extra.events, v, extra.focused);
  if (v.sel) return card(m, live, v);
  if (v.group) return groupPanel(m, live, v);
  if (v.level === 2) return cityPanel(m, live, v);
  if (v.level === 1) return empirePanel(m, live, v);
  return mapPanel(m, live, v);
}

/** results lists search hits as links. */
export function results(m, v, ids) {
  if (!ids.length) return el('p', { class: 'meta' }, 'Nothing matches.');
  return el('ul', { class: 'list' }, ids.map((id) => el('li', {}, nodeLink(m, v, id), el('span', { class: 'meta' }, KIND[m.nodes.get(id).kind]))));
}

/** minimap: the seven realms as small hexes; the one in view is lit. */
export function minimap(v, still = null) {
  const NS = 'http://www.w3.org/2000/svg', svg = document.createElementNS(NS, 'svg');
  if (still) { // the reader is open: the minimap is a still of the 3D it froze
    const img = el('img', { src: still, alt: 'The 3D view, frozen while you read' });
    return img;
  }
  svg.setAttribute('viewBox', '-12 -7.5 20 15');
  svg.setAttribute('role', 'img');
  svg.setAttribute('aria-label', `Minimap: ${v.realm ? REALM[v.realm].label : 'the whole map'}`);
  const hex = (x, z, r) => Array.from({ length: 6 }, (_, i) => { const a = (i * Math.PI) / 3; return `${(x + r * Math.cos(a)).toFixed(2)},${(z + r * Math.sin(a)).toFixed(2)}`; }).join(' ');
  for (const r of REALMS) {
    const [x, z] = REALM[r].at, a = document.createElementNS(NS, 'a'), p = document.createElementNS(NS, 'polygon');
    a.setAttribute('href', format({ level: 1, realm: r, lens: v.lens }));
    p.setAttribute('points', hex(x, z, 1.9));
    p.setAttribute('class', v.realm === r ? 'on' : v.realm ? 'off' : '');
    p.setAttribute('data-realm', r);
    const t = document.createElementNS(NS, 'title');
    t.textContent = REALM[r].label;
    a.append(t, p);
    svg.append(a);
  }
  for (const o of Object.values(OUTSIDE)) {
    const c = document.createElementNS(NS, 'circle');
    c.setAttribute('cx', o.at[0]); c.setAttribute('cy', o.at[1]); c.setAttribute('r', 0.7); c.setAttribute('class', 'rim');
    svg.append(c);
  }
  return svg;
}

/** status is the status line: stale sources in words, the projection's age, the connection. */
export function status(m, live, conn) {
  const parts = [];
  if (conn !== 'open') parts.push('Live connection lost, reconnecting…');
  for (const [src, since] of Object.entries(live?.stale || {})) parts.push(`${{ prom: 'Prometheus', alerts: 'Alertmanager' }[src] || src} not answering since ${new Date(since).toLocaleTimeString()}`);
  if (m?.at) parts.push(`projection ${m.seq}, ${ago(Date.parse(m.at))}`);
  return parts.join(' · ');
}

export { pct };

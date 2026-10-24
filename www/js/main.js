// atlas (plan/10-understanding.md): the system as a map. L0 is the seven realms as islands, L1 the trade routes
// between their groups; a group opens as a list, a node as a card. The graph comes from GET /api/graph (one
// projection of the repos), the live overlay from /ws. Hash routes make every view a link (router.js).
// Dev (127.0.0.1): window.__atlas exposes the frame stats and the view for www/dev/gate.mjs.
import { buildModel, REALM, REALMS, OUTSIDE, search, groupOf, alertsOn } from './model.js';
import { parse, format, up } from './router.js';
import { connect, gateLapsed } from './socket.js';
import { el, crumbs, panel, results, minimap, status, pct } from './ui.js';

const $ = (id) => document.getElementById(id);
const dev = location.hostname === '127.0.0.1';
const q = new URLSearchParams(location.search);
const coarse = matchMedia('(pointer: coarse)').matches;
const low = q.get('q') === 'low' || (coarse && ((navigator.deviceMemory || 8) <= 4 || (navigator.hardwareConcurrency || 8) <= 4));
const RETRY = 'atlas:reloaded';
const store = (f) => { try { return f(sessionStorage); } catch { return null; } };
if (dev) globalThis.__atlas = { ready: false };

if (!document.createElement('canvas').getContext('webgl2')) {
  $('fallback').hidden = false;
  $('loader').hidden = true;
  $('c').remove();
} else {
  start().then(() => store((s) => s.removeItem(RETRY)), failed);
}

function failed(e) {
  console.error(e);
  if (dev) globalThis.__atlas = { ready: true, error: String(e?.stack || e) };
  const loader = $('loader');
  loader.hidden = false;
  // a deploy retired this page's /v/<build>/ modules: reload once onto the current index
  if (dev || store((s) => s.getItem(RETRY)) || store((s) => (s.setItem(RETRY, '1'), true)) !== true) {
    loader.textContent = 'The map could not be drawn. Reload to try again.';
    return;
  }
  loader.textContent = 'Could not load, retrying…';
  setTimeout(() => location.reload(), 1000);
}

async function fetchGraph() {
  const r = await fetch('/api/graph', { cache: 'no-store' });
  if (r.status === 404) return null;
  if (!r.ok) throw new Error(`/api/graph: ${r.status}`);
  return r.json();
}

async function start() {
  const { createMap, project } = await import('./map.js');
  let model = null, live = null, events = [], conn = 'closed', view = parse(location.hash);
  const mine = import.meta.url.match(/\/v\/([0-9a-f]{12})\//)?.[1];

  // ---- the graph: until the first projection exists, say so and ask again
  let g = await fetchGraph();
  while (!g) {
    $('loader').textContent = 'No projection yet: atlas has not read the repos. Asking again in 20 s…';
    await new Promise((r) => setTimeout(r, 20000));
    g = await fetchGraph();
  }
  model = buildModel(g);
  $('loader').hidden = true;

  const bar = $('top'), fit = () => document.documentElement.style.setProperty('--top', `${bar.offsetHeight}px`);
  new ResizeObserver(fit).observe(bar);
  fit();
  const map = createMap({ canvas: $('c'), low, cover: () => { const pr = $('panel').getBoundingClientRect(), cr = $('c').getBoundingClientRect(); return pr.left > cr.left + cr.width / 2 ? Math.max(0, cr.right - pr.left + 16) : 0; }, onPick: (p) => {
    if (!p) return go(view.realm ? { ...view, group: null, sel: null } : view);
    if (p.outside) return openSearch(`${OUTSIDE[p.outside].kind}:`);
    go({ level: 1, realm: p.realm, group: p.group || null, sel: null, lens: view.lens });
  } });
  map.setModel(model);

  // ---- labels: one per realm, per group (L1, or the realm in focus) and per outside entry
  const layer = $('labels');
  let labels = [];
  function buildLabels() {
    layer.replaceChildren();
    labels = [];
    for (const r of REALMS) {
      const e = el('div', { class: 'lbl realm', 'data-realm': r }, el('b', {}, REALM[r].label), el('span', {}));
      layer.append(e);
      labels.push({ key: r, e, kind: 'realm', realm: r });
      for (const grp of model.realms[r].groups.values()) {
        const ge = el('div', { class: 'lbl grp' }, grp.id);
        layer.append(ge);
        labels.push({ key: `${r}/${grp.id}`, e: ge, kind: 'grp', realm: r });
      }
    }
    for (const [k, o] of Object.entries(OUTSIDE)) {
      const e = el('div', { class: 'lbl outside' }, el('b', {}, o.label), el('span', {}, `${model.outside[k].length} ${o.kind === 'repo' ? 'repos' : 'hostnames'}`));
      layer.append(e);
      labels.push({ key: `outside/${k}`, e, kind: 'outside' });
    }
    paintLabels();
  }
  function paintLabels() {
    for (const l of labels) {
      if (l.kind !== 'realm') continue;
      const realm = model.realms[l.realm], firing = alertsOn(model, live, { realm: l.realm }).length;
      let text = `${realm.groups.size} groups · ${realm.count} parts`;
      if (view.lens === 'land') {
        const s = live?.land?.realms?.[l.realm];
        text = s ? `cpu ${pct(s.cpu)} · ram ${pct(s.mem)} · disk ${pct(s.disk)}` : live?.land?.capacity?.cpu ? 'no pods in the cluster' : 'no land data yet';
      }
      if (firing) text += ` · ${firing} firing`;
      l.e.lastChild.textContent = text;
    }
  }
  map.onFrame(() => {
    const c = $('c');
    for (const l of labels) {
      const show = l.kind !== 'grp' || view.level === 1 || view.realm === l.realm;
      const w = show && map.where(l.key);
      const at = !w ? null : l.kind === 'realm' ? (view.level === 1 ? map.edge(l.realm) : w.clone().setY(1.2)) : l.kind === 'grp' ? w.clone().setY(w.y + 0.45) : w.clone().setY(0.9);
      const p = at && project(map.camera, c, at);
      if (!p) { l.e.hidden = true; continue; }
      l.e.hidden = false;
      l.e.style.transform = `translate(-50%, -100%) translate(${p[0].toFixed(1)}px, ${p[1].toFixed(1)}px)`;
      l.e.classList.toggle('dim', !!view.realm && l.realm !== view.realm && l.kind !== 'outside');
    }
  });
  buildLabels();

  // ---- the view
  function go(v) {
    const h = format(v);
    if (h !== location.hash) location.hash = h;
    else render();
  }
  function render() {
    view = parse(location.hash);
    $('panel').replaceChildren(panel(model, live, view));
    map.resize(); // the panel's width sets the projection centre
    map.setView(view);
    $('level').textContent = view.level === 1 ? 'L1 · EMPIRE' : 'L0 · MAP';
    $('crumbs').replaceChildren(crumbs(model, view));
    $('minimap').replaceChildren(minimap(view));
    $('lens-structure').setAttribute('aria-pressed', String(view.lens !== 'land'));
    $('lens-land').setAttribute('aria-pressed', String(view.lens === 'land'));
    $('level-0').setAttribute('aria-pressed', String(view.level === 0));
    $('level-1').setAttribute('aria-pressed', String(view.level === 1));
    paintLabels();
    paintStatus();
    if (dev) globalThis.__atlas = { ready: true, view, stats: () => map.stats, model: { nodes: model.nodes.size, edges: model.edges.length },
      inject: (f) => onFrame(f), point: (key) => { const w = map.where(key), r = $('c').getBoundingClientRect(), p = w && project(map.camera, $('c'), w); return p && { x: r.left + p[0], y: r.top + p[1] }; } };
  }
  addEventListener('hashchange', render);
  $('lens-structure').addEventListener('click', () => go({ ...view, lens: 'structure' }));
  $('lens-land').addEventListener('click', () => go({ ...view, lens: 'land' }));
  $('level-0').addEventListener('click', () => go({ level: 0, lens: view.lens }));
  $('level-1').addEventListener('click', () => go({ ...view, level: 1 }));

  // ---- search: the way out of a fold
  const box = $('search'), res = $('results');
  function openSearch(text) { box.value = text; box.focus(); showResults(); }
  function showResults() {
    const s = box.value.trim();
    res.hidden = !s;
    if (s) res.replaceChildren(results(model, view, search(model, s, 30)));
  }
  box.addEventListener('input', showResults);
  box.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { res.querySelector('a')?.click(); e.preventDefault(); }
    if (e.key === 'Escape') { box.value = ''; showResults(); box.blur(); }
  });
  res.addEventListener('click', (e) => { if (e.target.closest('a')) { box.value = ''; res.hidden = true; } });

  // ---- Last event (Age of Empires' idle key): fly to it, at the deepest level where it stands alone
  function lastEvent() {
    for (let i = events.length - 1; i >= 0; i--) {
      const e = events[i], g = e.node && groupOf(model, e.node);
      if (g) return go({ level: 1, realm: g.realm, group: g.group, sel: e.node, lens: view.lens });
      if (e.node && model.nodes.has(e.node)) return go({ ...view, sel: e.node });
    }
  }
  $('last').addEventListener('click', lastEvent);
  addEventListener('keydown', (e) => {
    if (e.target.closest?.('input, textarea')) return;
    if (e.key === 'l' || e.key === 'L') lastEvent();
    else if (e.key === '/') { e.preventDefault(); box.focus(); }
    else if (e.key === 'Escape') go(up(view));
  });
  function paintLast() {
    const e = [...events].reverse().find((x) => x.node);
    $('last').disabled = !e;
    $('last').title = e ? `${e.text} (${new Date(e.ts).toLocaleTimeString()})` : 'No event yet';
  }

  function paintStatus() { $('status').textContent = status(model, live, conn); }
  setInterval(paintStatus, 15000);

  // ---- live
  async function refetch(seq) {
    if (seq && seq === model.seq) return;
    const v = await fetchGraph().catch((e) => (console.warn(e), null));
    if (!v) return;
    model = buildModel(v);
    map.setModel(model);
    map.setLive(live);
    buildLabels();
    render();
  }
  const onFrame = (f) => {
    if (f.t === 'hello') {
      if (mine && f.build && f.build !== mine) return failed(new Error(`build ${mine} → ${f.build}`));
      live = f.live;
      events = f.events || [];
      map.setLive(live);
      refetch(f.seq);
    } else if (f.t === 'live') {
      live = f;
      map.setLive(live);
    } else if (f.t === 'ev') {
      events.push(f);
      events = events.slice(-50);
    } else if (f.t === 'graph') {
      refetch(f.seq);
      return;
    } else return;
    paintLast();
    const pane = $('panel'), y = pane.scrollTop;
    pane.replaceChildren(panel(model, live, view));
    pane.scrollTop = y;
    paintLabels();
    paintStatus();
  };
  connect({ onFrame, onStatus: (s) => { conn = s; paintStatus(); }, ...(dev && q.get('ws') ? { url: q.get('ws') } : { probe: () => gateLapsed() }) });

  // ---- who is signed in
  fetch('/auth/me', { cache: 'no-store' }).then((r) => r.json()).then((me) => {
    if (me.name) $('who').textContent = me.name;
    $('signout').hidden = !me.enabled;
  }).catch(() => {});

  render();
  paintLast();
}

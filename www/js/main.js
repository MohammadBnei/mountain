// atlas (plan/10-understanding.md): the system as a map, five steps deep. L0 is the seven realms as islands, L1 the
// trade routes between their groups, L2 a realm as a city (city.js), L3 one part as a building with its cards on the
// walls (building.js), L4 a file in the reader (reader.js). The graph comes from GET /api/graph (one projection of the
// repos), files from GET /api/read, the live overlay from /ws. Hash routes make every view a link (router.js).
// Dev (127.0.0.1): window.__atlas exposes the frame stats and the view for www/dev/gate.mjs.
import { buildModel, REALM, REALMS, OUTSIDE, search, groupOf, alertsOn } from './model.js';
import { parse, format, up } from './router.js';
import { connect, gateLapsed } from './socket.js';
import { el, crumbs, panel, results, minimap, status, pct, buildingCards } from './ui.js';
import { readerView } from './reader.js';

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
  const [{ createMap, project }, { createCity }, { createBuilding }] = await Promise.all([import('./map.js'), import('./city.js'), import('./building.js')]);
  let model = null, live = null, events = [], conn = 'closed', view = parse(location.hash);
  const mine = import.meta.url.match(/\/v\/([0-9a-f]{12})\//)?.[1];
  const narrow = () => matchMedia('(max-width: 600px)').matches; // L3's cards go flat into the dock (iOS, preserve-3d)

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
  const where = (id) => groupOf(model, id);
  const map = createMap({ canvas: $('c'), low, cover: () => { const pr = $('panel').getBoundingClientRect(), cr = $('c').getBoundingClientRect(); return pr.width && pr.left > cr.left + cr.width / 2 ? Math.max(0, cr.right - pr.left + 16) : 0; }, onPick });
  map.setModel(model);

  // ---- picks, per level
  function onPick(p) {
    if (view.level === 3) {
      if (p?.card) return focusCard(p.card);
      if (p?.part) return go({ level: 3, id: p.part, lens: view.lens });
      return focusCard(null);
    }
    if (view.level === 2) {
      if (!p) return go(up(view, where));
      if (p.sel) return p.sel === view.sel ? go({ level: 3, id: p.sel, lens: view.lens }) : go({ ...view, group: p.group, sel: p.sel });
      if (p.group) return go({ ...view, group: p.group, sel: null });
      return go({ ...view, group: null, sel: null });
    }
    if (!p) return go(view.realm ? { ...view, group: null, sel: null } : view);
    if (p.outside) return openSearch(`${OUTSIDE[p.outside].kind}:`);
    if (p.group) return go({ level: 2, realm: p.realm, group: p.group, lens: view.lens });
    if (view.level === 1 && view.realm === p.realm) return go({ level: 2, realm: p.realm, lens: view.lens }); // a second click enters
    go({ level: 1, realm: p.realm, lens: view.lens });
  }

  // ---- the deeper levels: built on entry, disposed on exit (one scene per level, plan/10)
  let mounted = { key: '', mod: null };
  function mountFor(v, keepCamera = false) {
    let key = '', make = null;
    if (v.level === 2) { key = `2:${v.realm}:${model.seq}`; make = () => createCity({ model, realm: v.realm }); }
    if (v.level === 3 && model.nodes.has(v.id)) {
      const flat = narrow();
      key = `3:${v.id}:${model.seq}:${flat}`;
      make = () => createBuilding({ model, id: v.id, flat, cards: buildingCards(model, live, events, v) });
    }
    if (v.level === 4) return; // the reader freezes whatever was mounted
    if (key === mounted.key && !keepCamera) return;
    const focus = mounted.mod?.focused;
    mounted = { key, mod: make ? make() : null };
    if (keepCamera && focus) mounted.mod?.focus?.(focus);
    map.mount(mounted.mod, keepCamera);
    buildLabels();
  }
  function focusCard(key) {
    const b = mounted.mod;
    if (!b?.focus) return;
    b.focus(key);
    map.reaim();
    for (const btn of document.querySelectorAll('#panel [data-focus]')) btn.setAttribute('aria-pressed', String(btn.dataset.focus === key));
  }

  // ---- labels: the map's (realms, groups, the rim) or the mounted level's own
  const layer = $('labels');
  let labels = [];
  function buildLabels() {
    layer.replaceChildren();
    labels = [];
    if (mounted.mod) {
      for (const sp of mounted.mod.labels()) {
        const e = el('div', { class: `lbl ${sp.cls}` }, el('b', {}, sp.text), sp.sub ? el('span', {}, sp.sub) : null);
        layer.append(e);
        labels.push({ ...sp, e, kind: 'spec' });
      }
      return;
    }
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
      let at = null;
      if (l.kind === 'spec') {
        // a city names its districts always, its buildings only in the district in view (the load rule)
        const show = l.cls !== 'bld' || view.level === 3 || (view.group && l.group === view.group) || l.id === view.sel;
        at = show ? l.at : null;
      } else {
        const show = l.kind !== 'grp' || view.level === 1 || view.realm === l.realm;
        const w = show && map.where(l.key);
        at = !w ? null : l.kind === 'realm' ? (view.level === 1 ? map.edge(l.realm) : w.clone().setY(1.2)) : l.kind === 'grp' ? w.clone().setY(w.y + 0.45) : w.clone().setY(0.9);
      }
      const p = at && project(map.camera, c, at);
      if (!p) { l.e.hidden = true; continue; }
      l.e.hidden = false;
      l.e.style.transform = `translate(-50%, -100%) translate(${p[0].toFixed(1)}px, ${p[1].toFixed(1)}px)`;
      l.e.classList.toggle('dim', l.kind !== 'spec' && !!view.realm && l.realm !== view.realm && l.kind !== 'outside');
    }
  });
  buildLabels();

  // ---- the view
  // go renders at once: hashchange fires later, and a key pressed before it (a fast second Escape) must climb from
  // the view it sees, not the one before; hashchange then finds that hash rendered (back/forward and links still go
  // through it).
  let rendered = null;
  function go(v) {
    const h = format(v);
    if (h !== location.hash) location.hash = h;
    render();
  }
  const LEVEL = ['L0 · MAP', 'L1 · EMPIRE', 'L2 · CITY', 'L3 · BUILDING', 'L4 · READ'];
  let reading = '';
  function render() {
    rendered = location.hash;
    view = parse(location.hash);
    if (view.level === 3 && !model.nodes.has(view.id)) view = { level: 0, lens: view.lens };
    document.body.classList.toggle('reading', view.level === 4);
    if (view.level === 4) openReader();
    else {
      reading = '';
      $('reader').hidden = true;
      map.freeze(false);
      mountFor(view);
      $('panel').replaceChildren(panel(model, live, view, { events, focused: mounted.mod?.focused }));
      map.resize(); // the panel's width sets the projection centre
      map.setView(view);
      $('minimap').replaceChildren(minimap(view));
    }
    $('level').textContent = LEVEL[view.level];
    $('crumbs').replaceChildren(crumbs(model, view));
    $('lens-structure').setAttribute('aria-pressed', String(view.lens !== 'land'));
    $('lens-land').setAttribute('aria-pressed', String(view.lens === 'land'));
    $('level-0').setAttribute('aria-pressed', String(view.level === 0));
    $('level-1').setAttribute('aria-pressed', String(view.level === 1));
    paintLabels();
    paintStatus();
    if (dev) globalThis.__atlas = { ready: true, view, stats: () => map.stats, model: { nodes: model.nodes.size, edges: model.edges.length },
      inject: (f) => onFrame(f), focused: () => mounted.mod?.focused || null,
      point: (key) => { const w = map.where(key), r = $('c').getBoundingClientRect(), p = w && project(map.camera, $('c'), w); return p && { x: r.left + p[0], y: r.top + p[1] }; },
      pointAt: (labelKey) => { const l = labels.find((x) => x.key === labelKey), r = $('c').getBoundingClientRect(), p = l && project(map.camera, $('c'), l.hit || l.at); return p && { x: r.left + p[0], y: r.top + p[1] }; },
      pointCard: (key) => { const w = mounted.mod?.standAt?.(key), r = $('c').getBoundingClientRect(), p = w && project(map.camera, $('c'), w); return p && { x: r.left + p[0], y: r.top + p[1] }; } };
  }

  // L4: the 3D freezes to a still (the minimap), the reader takes the stage
  async function openReader() {
    const key = `${view.doc.repo}/${view.doc.path}`;
    if (reading === key) return;
    reading = key;
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))); // a page opened on L4: one frame drawn first
    if (reading !== key) return;
    const still = map.freeze(true) || $('minimap').querySelector('img')?.src || null;
    $('minimap').replaceChildren(minimap(view, still));
    const box = $('reader');
    box.hidden = false;
    box.replaceChildren(el('p', { class: 'meta' }, 'Reading…'));
    let doc = null, error = null;
    try {
      const r = await fetch(`/api/read?repo=${encodeURIComponent(view.doc.repo)}&path=${encodeURIComponent(view.doc.path)}`, { cache: 'no-store' });
      if (r.ok) doc = await r.json();
      else error = { 404: 'atlas does not hold this file at the sha it read (moved, renamed, or a repo it does not read).', 413: 'The file is larger than the reader reads.', 415: 'Not a text file.' }[r.status] || `atlas answered ${r.status}.`;
    } catch (e) { error = `atlas could not be reached: ${e.message}`; }
    if (reading !== key) return; // another document was opened meanwhile
    box.replaceChildren(readerView(model, view, doc, error));
    box.scrollTop = 0;
    const slot = $('readstill');
    if (slot && still) slot.replaceChildren(el('img', { src: still, alt: 'The 3D view, frozen while you read' }));
    box.querySelector('h1, h2, p')?.focus?.();
    if (dev) globalThis.__atlas.read = { doc: doc && { kind: doc.kind, blocks: doc.blocks?.length || 0, lines: doc.lines?.length || 0, redacted: doc.redacted }, error };
  }

  addEventListener('hashchange', () => { if (location.hash !== rendered) render(); });
  addEventListener('resize', () => { if (view.level === 3 && mounted.key.endsWith(`:${!narrow()}`)) render(); });
  $('lens-structure').addEventListener('click', () => go({ ...view, lens: 'structure' }));
  $('lens-land').addEventListener('click', () => go({ ...view, lens: 'land' }));
  $('level-0').addEventListener('click', () => go({ level: 0, lens: view.lens }));
  $('level-1').addEventListener('click', () => go({ level: 1, realm: view.realm || where(view.id || view.from || '')?.realm || null, lens: view.lens }));
  $('panel').addEventListener('click', (e) => { const b = e.target.closest('[data-focus]'); if (b) focusCard(b.dataset.focus === mounted.mod?.focused ? null : b.dataset.focus); });

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
      if (g) return go({ level: 2, realm: g.realm, group: g.group, sel: e.node, lens: view.lens });
      if (e.node && model.nodes.has(e.node)) return go({ level: 0, sel: e.node, lens: view.lens });
    }
  }
  $('last').addEventListener('click', lastEvent);
  addEventListener('keydown', (e) => {
    if (e.target.closest?.('input, textarea')) return;
    if (e.key === 'l' || e.key === 'L') lastEvent();
    else if (e.key === '/') { e.preventDefault(); box.focus(); }
    else if (e.key === 'Escape') {
      if (view.level === 3 && mounted.mod?.focused) return focusCard(null); // first the card, then the building
      go(up(view, where));
    }
  });
  function paintLast() {
    const e = [...events].reverse().find((x) => x.node);
    $('last').disabled = !e;
    $('last').title = e ? `${e.text} (${new Date(e.ts).toLocaleTimeString()})` : 'No event yet';
  }

  function paintStatus() { $('status').textContent = status(model, live, conn); }
  setInterval(paintStatus, 15000);

  // ---- live
  let fetching = 0;
  async function refetch(seq) {
    if (seq && seq === model.seq) return;
    const n = ++fetching;
    const v = await fetchGraph().catch((e) => (console.warn(e), null));
    if (!v || n !== fetching || v.seq === model.seq) return; // a later refetch owns the answer
    if (v.seq < model.seq && !seq) return;
    model = buildModel(v);
    map.setModel(model);
    map.setLive(live);
    mounted = { key: '', mod: mounted.mod }; // rebuilt against the new projection on render
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
    if (view.level === 4) return paintStatus(); // the reader stays still
    if (view.level === 3) mountFor(view, true); // the wall cards carry live facts: rebuilt in place, the camera stays
    const pane = $('panel'), y = pane.scrollTop;
    pane.replaceChildren(panel(model, live, view, { events, focused: mounted.mod?.focused }));
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

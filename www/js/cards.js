// Cards + legend (W5, R6: "no card nowhere, absolutely no relation to the cluster"). Orchestrator-owned host.
//
//   createCards({world, camera, canvas, models, toys}) → {close()}
//
// A tap (pointerup within 6 px of its pointerdown: an orbit drag is not a tap; it also tries rays 14 px around, a finger
// is wider than a crystal) or a 150 ms mouse hover asks every model
// with `pick` (01 contract: pick(ray) → {kind, d, name?, ...raw} | null; d = distance along the ray, kind = a legend.<kind>
// key) and shows the nearest answer that the terrain or the cut face does not hide. A tap pins the card (a dialog: Esc,
// its close button or a tap on empty space closes it); a hover shows it as a tooltip until the pointer leaves the feature.
// While a toy tool is active (toys.active) the pointer is the tool's: no cards. The card is public (user: no internal
// information): a crystal shows its app's name, description and URL (apps.json), any other feature its legend.<kind> line
// ("Title: meaning"); the raw pick fields stay unread. The model that answered gets focus(name, 1 hover | 2 pinned) and
// focus(null, 0) when the card goes (summit: the crystal glows). The legend button lists every legend.<kind> line.
import * as THREE from 'three';
import { pickTerrain } from './toys.js';
import * as cut from './cut.js';
import S from './i18n.json' with { type: 'json' };
import APPS from './apps.json' with { type: 'json' };

const lang = navigator.language?.toLowerCase().startsWith('fr') ? 'fr' : 'en', T = S[lang];
const TAP = 6, HOVER = 150, NEAR = 14; // px: a tap also tries a ring of rays this far out (a crystal is a few px wide)

export function createCards({ world, camera, canvas, models, toys }) {
  const $ = (id) => document.getElementById(id), card = $('card'), q = (s) => card.querySelector(s);
  const kicker = q('.kicker'), title = q('h2'), desc = q('.desc'), go = q('.go'), means = q('.means');
  const ray = new THREE.Raycaster(), ndc = new THREE.Vector2(), ground = new THREE.Vector3(), pickers = models.filter((m) => m.pick);
  let pinned = false, at = null, timer = 0, lit = null; // lit: the card shown {m, name}

  /** The nearest card at client (x, y) that nothing solid hides, else null; fuzzy: then within NEAR px around it. */
  function hitAt(x, y, fuzzy) {
    let c = hitRay(x, y);
    for (let k = 0; !c && fuzzy && k < 8; k++) c = hitRay(x + NEAR * Math.cos(k * Math.PI / 4), y + NEAR * Math.sin(k * Math.PI / 4));
    return c;
  }
  function hitRay(x, y) {
    const r = canvas.getBoundingClientRect();
    ndc.set(((x - r.left) / r.width) * 2 - 1, -((y - r.top) / r.height) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    // the occluder: the terrain where the cut keeps it, else the cut face (an open cut removes the near terrain)
    const [nx, ny, nz, w] = world.clip.plane, g = pickTerrain(world, ray.ray, ground), face = cut.pick(ray.ray, world);
    const kept = g && nx * g.x + ny * g.y + nz * g.z + w >= 0;
    const wall = Math.min(kept ? g.distanceTo(ray.ray.origin) : Infinity, face ? face.distanceTo(ray.ray.origin) : Infinity) + 0.5;
    let best = null;
    for (const m of pickers) {
      const c = m.pick(ray.ray);
      if (c && c.d <= wall && (!best || c.d < best.d)) best = { ...c, m };
    }
    return best;
  }
  function focus(c, lv) {
    if (lit && (lit.m !== c?.m || lit.name !== c?.name)) lit.m.focus?.(null, 0);
    lit = c;
    c?.m.focus?.(c.name, lv);
  }
  function show(c, pin) {
    const app = c.kind === 'summit' ? APPS[c.name] || { name: c.name } : null;
    const [head, ...rest] = (T[`legend.${c.kind}`] || '').split(':'); // "Crystal: one app. …"
    kicker.textContent = T[app ? 'card.app' : 'card.feature'];
    title.textContent = app ? app.name : head.trim();
    desc.textContent = app ? app[lang] || '' : rest.join(':').trim();
    go.hidden = !app?.url;
    if (app?.url) { go.href = app.url; go.textContent = `${new URL(app.url).host} ↗`; go.setAttribute('aria-label', `${T['card.open']} ${app.name}`); }
    means.textContent = app ? T['legend.summit'] : '';
    pinned = pin;
    card.setAttribute('role', pin ? 'dialog' : 'tooltip');
    card.hidden = false;
    focus(c, pin ? 2 : 1);
  }
  const hide = () => { card.hidden = true; focus(null); };
  const close = () => { pinned = false; hide(); };

  canvas.addEventListener('pointerdown', (e) => { at = [e.clientX, e.clientY]; });
  canvas.addEventListener('pointerup', (e) => {
    const tap = at && Math.hypot(e.clientX - at[0], e.clientY - at[1]) < TAP;
    at = null;
    if (!tap || toys.active) return;
    const c = hitAt(e.clientX, e.clientY, true);
    if (c) show(c, true); else close();
  });
  canvas.addEventListener('pointermove', (e) => {
    if (e.pointerType !== 'mouse' || e.buttons || pinned) return;
    clearTimeout(timer);
    timer = setTimeout(() => {
      const c = toys.active ? null : hitAt(e.clientX, e.clientY);
      if (c) show(c, false); else if (!pinned) hide();
    }, HOVER);
  });
  canvas.addEventListener('pointerleave', () => { clearTimeout(timer); if (!pinned) hide(); });
  addEventListener('keydown', (e) => { if (e.key === 'Escape') { close(); legendOpen(false); } });
  $('cardclose').setAttribute('aria-label', T['card.close']);
  $('cardclose').addEventListener('click', close);

  // legend: every legend.<kind> line, in i18n order
  const btn = $('legendbtn'), legend = $('legend');
  btn.setAttribute('aria-label', T['legend.open']);
  btn.title = T['legend.open'];
  $('legendtitle').textContent = T['legend.title'];
  legend.querySelector('dl').replaceChildren(...Object.keys(T).filter((k) => k.startsWith('legend.') && !['legend.open', 'legend.title'].includes(k))
    .map((k) => Object.assign(document.createElement('dd'), { textContent: T[k] })));
  const legendOpen = (on) => { legend.hidden = !on; btn.setAttribute('aria-expanded', String(on)); };
  btn.addEventListener('click', () => legendOpen(legend.hidden));

  return { close };
}

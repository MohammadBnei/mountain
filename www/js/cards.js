// Cards + legend (W5, R6: "no card nowhere, absolutely no relation to the cluster"). Orchestrator-owned host.
//
//   createCards({world, camera, canvas, models, toys}) → {close()}
//
// A tap (pointerup within 6 px of its pointerdown: an orbit drag is not a tap; it also tries rays 14 px around, a finger
// is wider than a crystal) or a 150 ms mouse hover asks every model
// with `pick` (01 contract: pick(ray) → {kind, d, name?, ...raw} | null; d = distance along the ray, kind = a legend.<kind>
// key) and shows the nearest answer that the terrain or the cut face does not hide. A tap pins the card (a dialog: Esc,
// its close button or a tap on empty space closes it); a hover shows it as a tooltip until the pointer leaves the feature.
// While a toy tool is active (toys.active) the pointer is the tool's: no cards. Models return raw, sanitised state
// (world.state / world.sites / metrics) only: the rows are formatted here, card.<field> labels, legend.<kind> the meaning.
// The legend button lists every legend.<kind> line.
import * as THREE from 'three';
import { pickTerrain } from './toys.js';
import * as cut from './cut.js';
import S from './i18n.json' with { type: 'json' };

const T = S[navigator.language?.toLowerCase().startsWith('fr') ? 'fr' : 'en'];
const TAP = 6, HOVER = 150, NEAR = 14; // px: a tap also tries a ring of rays this far out (a crystal is a few px wide)
const fmt = (v) => (typeof v === 'number' ? String(Math.round(v * 100) / 100) : typeof v === 'boolean' ? (v ? '✓' : '✗') : Array.isArray(v) ? v.join(', ') : String(v));

export function createCards({ world, camera, canvas, models, toys }) {
  const $ = (id) => document.getElementById(id), card = $('card'), title = card.querySelector('h2'), rows = card.querySelector('dl'), means = card.querySelector('.means');
  const ray = new THREE.Raycaster(), ndc = new THREE.Vector2(), ground = new THREE.Vector3(), pickers = models.filter((m) => m.pick);
  let pinned = false, at = null, timer = 0;

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
      if (c && c.d <= wall && (!best || c.d < best.d)) best = c;
    }
    return best;
  }
  function show(c, pin) {
    const { kind, d, name, ...raw } = c;
    title.textContent = name != null ? fmt(name) : '';
    rows.replaceChildren(...Object.entries(raw).filter(([, v]) => v != null).flatMap(([k, v]) => [Object.assign(document.createElement('dt'), { textContent: T[`card.${k}`] || k }), Object.assign(document.createElement('dd'), { textContent: fmt(v) })]));
    means.textContent = T[`legend.${kind}`] || '';
    pinned = pin;
    card.setAttribute('role', pin ? 'dialog' : 'tooltip');
    card.hidden = false;
  }
  const close = () => { pinned = false; card.hidden = true; };

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
      if (c) show(c, false); else if (!pinned) card.hidden = true;
    }, HOVER);
  });
  canvas.addEventListener('pointerleave', () => { clearTimeout(timer); if (!pinned) card.hidden = true; });
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

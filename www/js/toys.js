// Visitor toolbar (R7): sign in, then plant something on a free hex (relay toy_plant.go): a tree by day, a flag at
// dawn/dusk, a lantern at night; the relay decides from the real sun, the button only previews it. Admins also get
// an eraser (mark.delete). Plant mode: a tap on a tile plants there (a drag still orbits); a hex ring under the
// pointer says whether the relay will take it. Every answer is said in words (Ukubi: status comes with a word).
import { sun } from './sun.js';
import { hexXZ, worldToHex, ring } from './layout.js';

const ENROLL = 'https://authentik.bnei.dev/if/flow/mountain-enrollment/'; // 08: the brand enrollment_flow is unset on purpose
const itemOf = (elev) => (elev > 6 ? 0 : elev >= -6 ? 1 : 2);
const ITEM = { en: ['Plant a tree', 'Plant a flag', 'Light a lantern'], fr: ['Planter un arbre', 'Planter un drapeau', 'Allumer une lanterne'] };
const DONE = { en: ['Tree planted.', 'Flag planted.', 'Lantern lit.'], fr: ['Arbre planté.', 'Drapeau planté.', 'Lanterne allumée.'] };
const WHY = {
  en: { road: 'Not on a road, a bridge or the square.', plot: 'A building stands there.', taken: 'Someone planted there already.', reach: 'Plant on an island.',
    auth: 'Sign in to plant.', off: 'Planting is paused for now.', rate: 'One at a time: wait a little.', offline: 'Not connected. Try again in a moment.', timeout: 'No answer. Try again.', bad: 'That did not work. Try another tile.', unknown: 'Already removed.' },
  fr: { road: 'Pas sur une route, un pont ou la place.', plot: 'Un bâtiment est déjà là.', taken: 'Quelqu’un a déjà planté ici.', reach: 'Plantez sur une île.',
    auth: 'Connectez-vous pour planter.', off: 'Les plantations sont en pause.', rate: 'Une à la fois : patientez un peu.', offline: 'Hors ligne. Réessayez dans un instant.', timeout: 'Pas de réponse. Réessayez.', bad: 'Cela n’a pas marché. Essayez une autre case.', unknown: 'Déjà retiré.' },
};

export function createToys({ THREE, kit, world, canvas, camera, lang, fly }) {
  const $ = (id) => document.getElementById(id);
  const plantBtn = $('plant'), eraseBtn = $('erase'), signin = $('signin'), status = $('toystatus');
  let role = 'view', mode = null, sayUntil = 0;
  const L = () => lang();

  const split = new URLSearchParams(location.search).has('ws'); // dev: page and relay on two ports, no /auth here
  (split ? Promise.resolve(null) : fetch('/auth/me').then((r) => (r.ok ? r.json() : null))).then((a) => {
    role = a?.role || 'view';
    const pin = location.hostname === '127.0.0.1' && new URLSearchParams(location.search).get('role'); // dev: preview the toolbar
    if (pin) role = pin;
    signin.hidden = !(a?.enabled && role === 'view');
    world.named = !!a?.handle;
    sync();
  }, () => {});
  $('login').href = `/auth/login?return=${encodeURIComponent('/')}`;
  Object.assign($('enroll'), { href: ENROLL });
  addEventListener('keydown', (e) => { if (e.key === 'Escape') { signin.open = false; setMode(null); } });
  addEventListener('pointerdown', (e) => { if (!signin.contains(e.target)) signin.open = false; });

  // the aiming ring: a hex outline on the tile under the pointer, success or danger by the relay's own rule
  const pts = [];
  for (let i = 0; i <= 6; i++) { const a = Math.PI / 6 + (i * Math.PI) / 3; pts.push(new THREE.Vector3(Math.cos(a) * 1.12, 0.04, Math.sin(a) * 1.12)); }
  const ringMat = kit.lineMat(0x2f6b6b);
  const aim = new THREE.Line(new THREE.BufferGeometry().setFromPoints(pts), ringMat);
  aim.visible = false;

  function hexAt(ndc) {
    const ray = new THREE.Raycaster();
    ray.setFromCamera(ndc, camera);
    const p = new THREE.Vector3();
    if (!ray.ray.intersectPlane(new THREE.Plane(new THREE.Vector3(0, 1, 0), 0), p)) return null;
    const [Q, R] = worldToHex(p.x, p.z);
    const i = world.town.islands.findIndex((is) => ring(Q - is.q, R - is.r) <= world.town.radius);
    if (i < 0) return null;
    const is = world.town.islands[i];
    return { h: i + 1, host: is.host, q: Q - is.q, r: R - is.r, x: p.x, z: p.z };
  }
  function free(hx) { // toy_plant.go mirrored: grass or meadow, no building, no plant
    const is = world.town.islands[hx.h - 1], R = world.town.radius;
    let idx = 0, kind = null;
    for (let r = -R; r <= R && kind == null; r++) for (let q = Math.max(-R, -r - R); q <= Math.min(R, -r + R); q++, idx++) if (q === hx.q && r === hx.r) { kind = is.tiles[idx]; break; }
    if (kind !== 'g' && kind !== 'm') return false;
    if (world.state.plots.some((p) => p.host === hx.host && p.q === hx.q && p.r === hx.r)) return false;
    return !world.state.marks.some((m) => m.mark === 'plant' && m.pose.h === hx.h && m.pose.q === hx.q && m.pose.r === hx.r);
  }
  function plantNear(hx) {
    return world.state.marks.find((m) => m.mark === 'plant' && m.pose.h === hx.h && m.pose.q === hx.q && m.pose.r === hx.r);
  }

  const ndcOf = (e) => { const r = canvas.getBoundingClientRect(); return new THREE.Vector2(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1); };
  canvas.addEventListener('pointermove', (e) => {
    if (!mode) return;
    const hx = hexAt(ndcOf(e));
    aim.visible = !!hx;
    if (!hx) return;
    const [x, z] = hexXZ(world.town.islands[hx.h - 1].q + hx.q, world.town.islands[hx.h - 1].r + hx.r);
    aim.position.set(x, world.ready[hx.host] === false ? -0.5 : 0, z);
    const ok = mode === 'plant' ? free(hx) : !!plantNear(hx);
    ringMat.color.set(ok ? 0x2f6b6b : 0xa5402a); // Ukubi success / danger
  });

  /** A tap in plant/erase mode: → true if it was used (main.js then skips its own picking). */
  function tap(e) {
    if (!mode) return false;
    const hx = hexAt(ndcOf(e));
    if (!hx) { say(WHY[L()].reach); return true; }
    if (mode === 'erase') {
      const m = plantNear(hx);
      if (!m) { say(WHY[L()].unknown); return true; }
      world.unmark(m.id).then(() => say(L() === 'fr' ? 'Retiré.' : 'Removed.'), (err) => say(WHY[L()][err.message] || WHY[L()].bad));
      return true;
    }
    // the relay's sun decides (it can differ from the preview near ±6°); its world:mark lands before the ack
    const item = itemOf(sun().elev);
    world.place('plant', { h: hx.h, q: hx.q, r: hx.r }).then((id) => say(DONE[L()][world.state.marks.find((m) => m.id === id)?.pose.item ?? item]),
      (err) => say(WHY[L()][err.message] || WHY[L()].bad));
    return true;
  }

  function say(text) { status.textContent = text; status.hidden = false; sayUntil = Date.now() + 4000; }
  function setMode(m) {
    mode = mode === m ? null : m;
    plantBtn.setAttribute('aria-pressed', String(mode === 'plant'));
    eraseBtn.setAttribute('aria-pressed', String(mode === 'erase'));
    canvas.classList.toggle('aiming', !!mode);
    aim.visible = false;
    if (mode === 'plant') say(L() === 'fr' ? 'Touchez une case d’herbe libre.' : 'Tap a free grass tile.');
  }
  plantBtn.onclick = () => setMode('plant');
  eraseBtn.onclick = () => setMode('erase');

  let lastKey = '';
  function sync() {
    const on = world.metrics?.visitors !== false, can = role === 'visitor' || role === 'admin';
    plantBtn.hidden = !(on && can);
    eraseBtn.hidden = role !== 'admin';
    if (plantBtn.hidden && mode === 'plant') setMode(null);
    const q = world.quota, wait = q ? Math.max(0, Math.ceil(q.at / 1000 + q.next - Date.now() / 1000)) : 0;
    const item = itemOf(sun().elev);
    const label = wait > 0 ? `${Math.floor(wait / 60)}:${String(wait % 60).padStart(2, '0')}` : ITEM[L()][item];
    const key = `${label}|${q?.left}`;
    if (key !== lastKey) {
      lastKey = key;
      plantBtn.querySelector('span').textContent = label;
      plantBtn.disabled = wait > 0 || q?.left === 0;
      plantBtn.title = q ? (L() === 'fr' ? `encore ${q.left} aujourd’hui` : `${q.left} left today`) : '';
    }
    if (sayUntil && Date.now() > sayUntil) { status.hidden = true; sayUntil = 0; }
  }
  world.on('metrics', sync);
  return { aim, tap, tick: sync, get active() { return !!mode; } };
}

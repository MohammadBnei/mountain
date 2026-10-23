// R9 Civ: the herald. A figure at an island's square calls out a decree for 6 s when something lands live: an app's
// successful sync (on its island), new gitops commits ("laws", at the crane yard's island: ArgoCD's), a new ADR
// ("technology", there too). Never during a replay (world.step plays a past day's frames fast). One decree per island at a time, one waiting (the newest wins), none older than 30 s.
// The words are drawn by labels.js (notes()), placed first so building names give way; a polite live region repeats
// them for screen readers.
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { plotXZ } from '../layout.js';
import { decree } from '../civ.js';

const SAY = 6, STALE = 30, SINK = 0.5; // s, s; islands.js: a down island sinks this far
const clock = () => performance.now() / 1000; // wall time: a decree goes stale while the tab is hidden too

export default async function build({ THREE, kit, world, apps, lang = () => 'en' }) {
  const root = new THREE.Group();
  const cell = (g, [c, r]) => { g = g.toNonIndexed(); const uv = new Float32Array(g.attributes.position.count * 2); for (let i = 0; i < uv.length; i += 2) { uv[i] = (c + 0.5) / 8; uv[i + 1] = (r + 0.5) / 4; } g.setAttribute('uv', new THREE.BufferAttribute(uv, 2)); return g; };
  const fig = kit.instanced(mergeGeometries([
    cell(new THREE.CylinderGeometry(0.11, 0.15, 0.42, 7).translate(0, 0.21, 0), kit.CELL.tan),
    cell(new THREE.SphereGeometry(0.09, 7, 5).translate(0, 0.52, 0), kit.CELL.cream),
    cell(new THREE.CylinderGeometry(0.012, 0.012, 0.5, 4).translate(0.16, 0.55, 0), kit.CELL.dark), // a staff
    cell(new THREE.BoxGeometry(0.02, 0.16, 0.22).translate(0.16, 0.7, 0.11), kit.CELL.teal), // its pennant
  ]), kit.shared, 3);
  fig.frustumCulled = false;
  root.add(fig);
  const live = document.getElementById('heraldlive');
  const fr = () => lang() === 'fr';
  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3(1, 1, 1);
  let now = 0;
  const at = {}; // host → {text, t0, next?: {text, t}}

  const nameOf = (app) => apps[app]?.name || app;
  const hostOf = (app) => world.state.plots.find((p) => p.app === app && !p.ruinAt)?.host;
  const capital = () => hostOf('craneyard') || 'h1';
  function call(host, kind, args) {
    if (world.replay || !host) return;
    const text = decree(kind, args, fr());
    const cur = at[host];
    now = clock();
    if (cur && now - cur.t0 < SAY) cur.next = { text, t: now };
    else at[host] = { text, t0: now };
    if (live) live.textContent = text;
  }
  world.on('ev:argo.sync', (f) => { if (f.ok) call(hostOf(f.app), 'sync', { name: nameOf(f.app) }); });
  world.on('ev:gitops.commit', () => call(capital(), 'laws'));
  let top = null; // the highest ADR the page knows: a higher one is a discovery
  const baseline = () => (world.state.tech?.count ? world.state.tech.top : null); // no ADR known yet (a zero tech): no baseline
  top = baseline(); // the first hello landed before this object was built
  world.on('hello', () => { top = baseline(); for (const h of Object.keys(at)) delete at[h]; });
  world.on('ev:tech', (f) => { if (top != null && f.top > top) call(capital(), 'tech', { n: f.top }); top = f.top; });

  let notes = [];
  return {
    root,
    update(dt, t) {
      now = clock();
      if (world.replay) for (const h of Object.keys(at)) delete at[h]; // a scrub: no live decree over a past day
      notes = [];
      let n = 0;
      for (const [host, cur] of Object.entries(at)) {
        if (now - cur.t0 >= SAY) {
          if (cur.next && now - cur.next.t < STALE) at[host] = { text: cur.next.text, t0: now };
          else { delete at[host]; continue; }
        }
        const [cx, cz] = plotXZ(world.town, host, 0, 0), x = cx - 1.25, z = cz - 0.4;
        const k = Math.min(1, (now - at[host].t0) * 4), bob = Math.abs(Math.sin(now * 5)) * 0.05;
        M.compose(P.set(x, bob - (world.ready[host] === false ? SINK : 0), z), Q.setFromEuler(E.set(0, Math.PI * 0.25, 0)), S.set(k, k, k));
        fig.setMatrixAt(n++, M);
        notes.push({ key: `herald@${host}`, host, x, y: 1.0, z, text: at[host].text, down: world.ready[host] === false });
      }
      fig.count = n;
      fig.instanceMatrix.needsUpdate = true;
      return n > 0;
    },
    /** The decrees being called, for labels.js: [{key, host, x, y, z, text}]. */
    notes: () => notes,
  };
}

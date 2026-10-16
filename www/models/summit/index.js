// summit (03-model-summit): one crystal per allowlisted app at world.sites (y from the site only): a cluster of kit
// shard() prisms in ONE InstancedMesh with the kit crystal() material. Height = replicas bucket (world.state.replicas),
// glow = health (aHealth on ramp('health') × aGlow, world.state.health), hue = group (instanceColor on ramp('group')).
// world.js updates sites/state before each emit, so every listened event (and a new state object, or the sites map
// changing under a /api/topology answer) just marks the cluster dirty; step() rebuilds it. pick(ray) → {kind 'summit',
// d, name app, group, health, replicas} of the nearest crystal's bounding box (analytic: no mesh raycast on hover).
// focus(app, lv) (cards.js, W5): lv 1 = hovered, the crystal glows brighter in its health colour; lv 2 = its card is
// pinned, brighter still in sunCore (the card's accent); focus(null, 0) clears.
const MAX = 32, K = 5; // apps (allowlist is 8) × shards per crystal: 160 × 24 tris
const W = 3, SINK = 0.6; // main prism width scale (hex radius 0.9 m), sunk into the slope
const H0 = 1.5, HB = 1.1; // height = H0 + HB × bucket (0..8 → 1.5..10.3 m)
const R = 1.5; // pick box half-width: main prism + tilted satellites
// argo health (sanitize.go healthOK) → [ramp('health') t, aGlow]; no status yet → dark
const HEALTH = { Healthy: [0, 0.3], Progressing: [0, 0.2], Degraded: [0.5, 0.55], Suspended: [0.5, 0.25], Missing: [1, 0.75], Unknown: [0.5, 0.12] }; // day luminance ceiling < 1 (09)
const NONE = [0, 0], FOCUS = [0, 1.2, 3]; // extra aGlow per focus level (> 1 blooms on desktop)

export default async function build({ THREE, kit, world }) {
  const geo = kit.shard(7).translate(0, 0.5, 0); // base at the pivot, so a tilted satellite stays rooted
  const aHealth = new THREE.InstancedBufferAttribute(new Float32Array(MAX * K * 3), 3), aGlow = new THREE.InstancedBufferAttribute(new Float32Array(MAX * K), 1);
  const ok = kit.rampColor('health', 0);
  for (let i = 0; i < MAX * K; i++) aHealth.setXYZ(i, ok.r, ok.g, ok.b); // unused rows stay on the ramp
  geo.setAttribute('aHealth', aHealth);
  geo.setAttribute('aGlow', aGlow);
  const mesh = new THREE.InstancedMesh(geo, kit.crystal(), MAX * K);
  mesh.name = 'crystals';
  mesh.count = 0;
  const root = new THREE.Group();
  root.add(mesh);

  const apps = [], top = new Float32Array(MAX), O = new THREE.Object3D(), C = kit.rampColor('health', 0); // top: pick height per app, 0 = hidden
  const box = new THREE.Box3(), hit = new THREE.Vector3(), SEL = kit.color('sunCore'); // pick scratch; pinned colour
  let hl = null, hlLv = 0; // focused app + level
  const hash = (s) => { let h = 2166136261; for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619); return h >>> 0; };
  function rebuild() {
    apps.length = 0;
    world.sites.forEach((s, app) => apps.length < MAX && apps.push(app)); // ponytail: > 32 apps drop the rest
    apps.sort();
    const health = world.state.health || {}, reps = world.state.replicas || {};
    apps.forEach((app, a) => {
      const s = world.sites.get(app), r = kit.mulberry(hash(app)), H = H0 + HB * (reps[app] ?? 0);
      const [t, glow] = HEALTH[health[app]] || NONE, yaw = r() * Math.PI * 2;
      // not clipped (01), so a site on the cut-away side would float in front of the cut face: hide it while open
      const cp = world.clip.plane, gone = cp[0] * s.x + cp[1] * s.y + cp[2] * s.z + cp[3] < 0;
      top[a] = gone ? 0 : H;
      const lv = app === hl ? hlLv : 0;
      if (lv === 2) C.copy(SEL); else kit.rampColor('health', t, C);
      for (let k = 0; k < K; k++) {
        const i = a * K + k, az = yaw + (k * Math.PI * 2) / (K - 1) + r() * 0.6, tilt = k ? 0.3 + r() * 0.3 : r() * 0.08, h = k ? H * (0.35 + r() * 0.3) : H, w = k ? W * 0.6 : W;
        O.position.set(s.x + (k ? Math.sin(az) * 0.9 : 0), s.y - SINK, s.z + (k ? Math.cos(az) * 0.9 : 0));
        O.rotation.set(Math.cos(az) * tilt, r() * Math.PI, -Math.sin(az) * tilt);
        const g = gone ? 0 : 1;
        O.scale.set(w * g, h * g, w * g);
        O.updateMatrix();
        mesh.setMatrixAt(i, O.matrix);
        kit.setRampAt(mesh, i, s.group === 'platform' ? 0 : 1);
        aHealth.setXYZ(i, C.r, C.g, C.b);
        aGlow.setX(i, glow + FOCUS[lv]);
      }
    });
    mesh.count = apps.length * K;
    mesh.instanceMatrix.needsUpdate = aHealth.needsUpdate = aGlow.needsUpdate = true;
    mesh.computeBoundingSphere(); // culling + raycast see the new crystals
  }

  // dirty check: a listened event, a new state object (hello/replay), or the sites map changed silently (topology)
  let dirty = true, seen = null, sig = NaN, acc = 0;
  const add = (s) => { acc += s.x * 1.3 + s.y * 7.1 + s.z * 3.7 + s.app.length; };
  const sync = () => {
    const cp = world.clip.plane;
    acc = world.sites.size + cp[0] * 5.3 + cp[2] * 11.7 + cp[3] * 0.013; // the cut moving re-hides sites (rebuild per frame only while it animates)
    world.sites.forEach(add);
    if (!dirty && seen === world.state && acc === sig) return;
    dirty = false; seen = world.state; sig = acc;
    rebuild(); // ponytail: rebuild allocates (sort, rng); only on change, ≤ 160 instances
  };
  sync();
  const mark = () => { dirty = true; };

  return {
    root,
    params: {},
    pose() {},
    step: sync,
    on: { 'world:site': mark, 'ev:argo.health': mark, 'ev:replicas': mark, 'ev:app.gone': mark },
    pick(ray) {
      let d = Infinity, app = null;
      for (let i = 0; i < apps.length; i++) {
        const s = world.sites.get(apps[i]);
        if (!s || !top[i]) continue;
        box.min.set(s.x - R, s.y - SINK, s.z - R);
        box.max.set(s.x + R, s.y - SINK + top[i], s.z + R);
        if (!ray.intersectBox(box, hit)) continue;
        const t = hit.distanceTo(ray.origin);
        if (t < d) { d = t; app = apps[i]; }
      }
      if (!app) return null;
      const st = world.state;
      return { kind: 'summit', d, name: app, group: world.sites.get(app).group ?? null, health: st.health?.[app] ?? null, replicas: st.replicas?.[app] ?? null };
    },
    focus(app, lv) { if (app !== hl || lv !== hlLv) { hl = app; hlLv = app ? lv : 0; dirty = true; } },
    busy: () => false,
    dispose() { kit.dispose(root); },
  };
}

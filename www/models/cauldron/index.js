// cauldron (03-model-cauldron): the summit ice cauldron sags under firing alerts. Reads world.state.alerts every
// step (not the ev:alert.* bus), so a reload restored from hello.state sags the same. Sag depth by the worst severity;
// unknown severities (Watchdog's "none", dropped by the relay anyway) never sag. Its own sag ring (lit iceDeep annulus,
// widening and dishing with depth); critical → red under-glow (magma emissive on the ring: no third draw) + a Points
// steam column. A critical alert firing for breakMin → one jokulhlaup per alert.
// Emits cauldron.sag {x, z, depth, sev} on each change, steam {x, z, vol} every STEAM_S while critical.
const X = 0, Z = 0; // the summit of the live seed (layout profile footprint)
const SEV = { __proto__: null, info: 0.75, warning: 2, critical: 5 }; // sag depth, units
const SEG = 32, RAD = 4, RI = 2, LIFT = 1.2, BREAK_MS = 30 * 60e3, STEAM_S = 5, RISE = 40, LIFE = 6;

export default async function build({ THREE, kit, world, rng }) {
  const geo = new THREE.RingGeometry(RI, 6, SEG, RAD); // positions are rewritten in place (XZ plane on the terrain)
  const rp = geo.attributes.position, n0 = rp.count, ux = new Float32Array(n0), uz = new Float32Array(n0), uu = new Float32Array(n0);
  for (let i = 0; i < n0; i++) { const x = rp.getX(i), y = rp.getY(i), r = Math.hypot(x, y); ux[i] = x / r; uz[i] = -y / r; uu[i] = (r - RI) / (6 - RI); }
  const mat = kit.lit('iceDeep', { side: THREE.DoubleSide });
  const ring = new THREE.Mesh(geo, mat);
  ring.name = 'sag';
  ring.frustumCulled = false;

  const N = kit.QUALITY === 'low' ? 150 : 300;
  const sg = new THREE.BufferGeometry(), sp = new Float32Array(N * 3);
  sg.setAttribute('position', new THREE.BufferAttribute(sp, 3));
  const ph = new Float32Array(N), ang = new Float32Array(N), rr = new Float32Array(N);
  for (let i = 0; i < N; i++) { ph[i] = rng(); ang[i] = rng() * Math.PI * 2; rr[i] = 0.3 + rng() * 0.7; }
  const steam = new THREE.Points(sg, kit.points('cloudLit', 3));
  steam.name = 'steam';
  steam.frustumCulled = false;
  const root = new THREE.Group();
  root.add(ring, steam);

  const H = world.heightAt, burst = new Set();
  let depth = 0, target = -1, sev = null, now = Date.now(), steamT = 0;
  const glow = kit.color('magma').multiplyScalar(1.5); // > 1: blooms

  function shape() {
    const ro = 6 + 2 * depth;
    for (let i = 0; i < n0; i++) {
      const u = uu[i], r = RI + (ro - RI) * u, x = X + ux[i] * r, z = Z + uz[i] * r;
      rp.setXYZ(i, x, H(x, z) + LIFT - depth * 0.4 * (1 - u) * (1 - u), z);
    }
    rp.needsUpdate = true; // lit is flatShading: no normals to recompute
  }
  shape();
  ring.visible = steam.visible = false;

  return {
    root,
    params: {},
    pose() {},
    step(dt) {
      now = Math.max(now + dt * 1e3, Date.now()); // sim time in tests, wall time after a hidden tab
      let t = 0, top = null, crit = false;
      const al = world.state.alerts || [];
      for (let k = 0; k < al.length; k++) {
        const a = al[k];
        const d = SEV[a.sev] || 0; // ponytail: guard: Watchdog/"none"/unknown never sag
        if (!d) continue;
        if (d > t) { t = d; top = a.sev; }
        if (a.sev !== 'critical') continue;
        crit = true;
        if (now - a.startsAt >= BREAK_MS && !burst.has(a.fp)) { burst.add(a.fp); world.emit('jokulhlaup', { x: X, z: Z, vol: SEV.critical }); }
      }
      if (t !== target || top !== sev) { target = t; sev = top; world.emit('cauldron.sag', { x: X, z: Z, depth: t, sev }); }
      depth += (target - depth) * Math.min(1, dt / 1.5);
      if (!target && depth < 0.02) depth = 0;
      ring.visible = depth > 0;
      if (ring.visible) shape();
      mat.emissive.copy(glow).multiplyScalar(crit ? Math.min(1, depth / SEV.critical) : 0);

      steam.visible = crit;
      if (!crit) { steamT = 0; return; }
      if ((steamT += dt) >= STEAM_S) { steamT -= STEAM_S; world.emit('steam', { x: X, z: Z, vol: 1 }); }
      const y0 = H(X, Z);
      for (let i = 0; i < N; i++) {
        const p = (ph[i] = (ph[i] + dt / LIFE) % 1), h = p * RISE, s = rr[i] * (1 + h * 0.12), a = ang[i] + p * 2;
        sp[i * 3] = X + Math.cos(a) * s; sp[i * 3 + 1] = y0 + h; sp[i * 3 + 2] = Z + Math.sin(a) * s;
      }
      sg.attributes.position.needsUpdate = true;
    },
    busy: () => depth > 0,
    dispose() { geo.dispose(); mat.dispose(); sg.dispose(); steam.material.dispose(); },
  };
}

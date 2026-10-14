// eruption (03-model-eruption): ev:argo.sync{ok} + the paired world:deposit (same id) → lava blobs run from a
// vent upslope of the deposit down to its cells (steepest descent on heightSolid, pulled to a target cell),
// cool lavaHot → magma → emberRoot, then sink into the land world.js ramps in. ok:false → ash puff, no land.
// Emits lava.heat {x, z, q} when a flow has fully arrived.
import { cellXZ, GRID } from '../../js/world.js';

const MAX = 200, E = 0.8, SPEED = 5, COOL = 5, HOLD = 2;

export default async function build({ THREE, kit, world, rng }) {
  const geo = new THREE.IcosahedronGeometry(1, 0), mat = kit.lava();
  const blobs = new THREE.InstancedMesh(geo, mat, MAX);
  blobs.name = 'lava';
  kit.setRampAt(blobs, 0, 0); // allocates instanceColor for all MAX
  blobs.count = 0;
  kit.lavaHeat(mat, 1.4);
  const ash = new THREE.Mesh(kit.puff(7), kit.lit('stone2'));
  ash.name = 'ash';
  ash.userData.facet = 'cloud';
  ash.visible = false;
  ash.scale.setScalar(1.5); // the puff's start size: puff(7) edges at scale 1 sit under the cloud facet band
  const root = new THREE.Group();
  root.add(blobs, ash);

  // blob state (struct of arrays; step is allocation-free)
  const bx = new Float32Array(MAX), bz = new Float32Array(MAX), tx = new Float32Array(MAX), tz = new Float32Array(MAX);
  const age = new Float32Array(MAX), cool = new Float32Array(MAX), size = new Float32Array(MAX), flow = new Int32Array(MAX);
  const O = new THREE.Object3D();
  const LUT = Array.from({ length: 256 }, (_, i) => kit.rampColor('lava', i / 255)); // lava is a semantic ramp: phase-invariant
  const flows = []; // {x, z, sx, sz, targets:[x,z,...], n, spawned, left}
  let ashAge = -1;

  const H = world.heightAt;
  function start(dep) {
    const { x0, z0, w, h, cells } = dep, targets = [];
    let cx = 0, cz = 0;
    for (let j = 0; j < h; j++) for (let i = 0; i < w; i++) if (cells[j * w + i] > 0) {
      const [x, z] = cellXZ(x0 + i, z0 + j);
      targets.push(x, z); cx += x; cz += z;
    }
    if (!targets.length) return;
    cx /= targets.length / 2; cz /= targets.length / 2;
    let sx = cx, sz = cz; // vent: climb steepest ascent ~15 m from the deposit centre
    for (let k = 0; k < 10; k++) {
      const gx = H(sx + E, sz) - H(sx - E, sz), gz = H(sx, sz + E) - H(sx, sz - E), g = Math.hypot(gx, gz) || 1;
      sx += (gx / g) * 1.5; sz += (gz / g) * 1.5;
    }
    flows.push({ x: cx, z: cz, sx, sz, targets, n: Math.min(60, Math.max(20, targets.length)), spawned: 0, left: 0 });
  }

  // pair ev:argo.sync and world:deposit by id, whichever lands first
  const pend = new Map();
  const pair = (id, k, v) => {
    const p = pend.get(id) || {};
    p[k] = v;
    pend.set(id, p);
    if (pend.size > 32) pend.delete(pend.keys().next().value); // ponytail: drop the oldest unpaired half
    if (!p.ev) return;
    if (!p.ev.ok) { pend.delete(id); return puff(p.ev.app); }
    if (p.dep) { pend.delete(id); start(p.dep); }
  };
  let summit = 0;
  world.fields.heightSolid.forEach((v, i, a) => v > a[summit] && (summit = i));
  function puff(app) {
    const s = world.sites.get(app);
    const [x, z] = s ? [s.x, s.z] : cellXZ(summit % GRID, Math.floor(summit / GRID));
    ash.position.set(x, H(x, z), z);
    ashAge = 0;
    ash.visible = true;
  }

  function kill(i) { // swap-remove blob i
    const l = --blobs.count;
    bx[i] = bx[l]; bz[i] = bz[l]; tx[i] = tx[l]; tz[i] = tz[l]; age[i] = age[l]; cool[i] = cool[l]; size[i] = size[l]; flow[i] = flow[l];
    // the moved blob was already drawn at l this step: draw it at i too, or slot i shows the dead blob a frame
    blobs.instanceMatrix.array.copyWithin(i * 16, l * 16, l * 16 + 16);
    blobs.instanceColor.array.copyWithin(i * 3, l * 3, l * 3 + 3);
  }

  return {
    root,
    params: { speed: SPEED },
    pose() {},
    step(dt) {
      // spawn one blob per step from the oldest flow still spawning
      let fi = 0;
      while (fi < flows.length && flows[fi].spawned >= flows[fi].n) fi++;
      const f = flows[fi];
      if (f && blobs.count < MAX) {
        const i = blobs.count++, k = Math.floor(rng() * (f.targets.length / 2)) * 2;
        bx[i] = f.sx + (rng() - 0.5) * 2; bz[i] = f.sz + (rng() - 0.5) * 2;
        tx[i] = f.targets[k] + (rng() - 0.5) * 1.2; tz[i] = f.targets[k + 1] + (rng() - 0.5) * 1.2;
        age[i] = 0; cool[i] = 0; size[i] = 1.3 + rng() * 0.6; flow[i] = fi;
        f.spawned++; f.left++;
      }
      for (let i = blobs.count - 1; i >= 0; i--) {
        age[i] += dt;
        const dx = tx[i] - bx[i], dz = tz[i] - bz[i], d = Math.hypot(dx, dz);
        if (cool[i] === 0 && d > 0.8 && age[i] < 12) { // flowing: steepest descent + pull to the target cell
          const gx = H(bx[i] + E, bz[i]) - H(bx[i] - E, bz[i]), gz = H(bx[i], bz[i] + E) - H(bx[i], bz[i] - E), g = Math.hypot(gx, gz) || 1;
          const s = Math.min(1, g), vx = dx / d - (gx / g) * s, vz = dz / d - (gz / g) * s, v = Math.hypot(vx, vz) || 1;
          bx[i] += (vx / v) * SPEED * dt; bz[i] += (vz / v) * SPEED * dt;
        } else if (cool[i] === 0) { // arrived
          cool[i] = 1e-4;
          const q = flows[flow[i]];
          if (--q.left === 0 && q.spawned >= q.n) world.emit('lava.heat', { x: q.x, z: q.z, q: q.n / MAX });
        } else cool[i] += dt / COOL;
        if (cool[i] >= 1 + HOLD / COOL) { kill(i); continue; }
        const t = Math.min(1, 0.15 * Math.min(1, age[i] / 4) + cool[i] * 0.85), sink = Math.max(0, cool[i] - 1) * size[i];
        O.position.set(bx[i], H(bx[i], bz[i]) + size[i] * 0.4 - sink, bz[i]);
        O.scale.set(size[i], size[i] * 0.6, size[i]);
        O.rotation.set(0, size[i] * 9, 0);
        O.updateMatrix();
        blobs.setMatrixAt(i, O.matrix);
        blobs.setColorAt(i, LUT[Math.floor(t * 255)]);
      }
      if (!blobs.count && fi === flows.length) flows.length = 0;
      blobs.instanceMatrix.needsUpdate = true;
      blobs.instanceColor.needsUpdate = true;
      if (ashAge >= 0) {
        ashAge += dt;
        const k = ashAge / 3;
        ash.scale.setScalar(1.5 + 3 * k);
        ash.position.y += dt * 2;
        if (k >= 1) { ashAge = -1; ash.visible = false; }
      }
    },
    on: {
      'ev:argo.sync': (ev) => pair(ev.id, 'ev', ev),
      'world:deposit': (d) => pair(d.id, 'dep', d),
    },
    busy: () => blobs.count > 0 || flows.length > 0 || ashAge >= 0,
    dispose() { geo.dispose(); mat.dispose(); ash.geometry.dispose(); ash.material.dispose(); },
  };
}

// shimmer (03-model-shimmer): heat haze from CPU use, uniforms only, 0 draws. Strength = metrics.cpu (eased);
// desktop writes the post haze (kit.U.uHaze) inside kit.U.uMask, a world sphere over the GPU slab (h1, k8s-worker-01)
// that swells with h1's load; the low tier has no post, so it writes kit.U.uWobble (terrain vertex wobble) instead.
// Never uHeat (magma's ember pool, W3c).
import U from '../../js/units.json' with { type: 'json' };

const CELL = U.EXTENT / U.GRID, SPAN = CELL * (U.GRID - 1), SW = SPAN / 3, X0 = -U.EXTENT / 2, YB = -70; // mountain's slabs
const GX = X0 + SW * 0.5, GZ = X0 + SPAN / 2; // h1 = slab 0

export default async function build({ THREE, kit, world }) {
  const params = { haze: 1.5, wobble: 0.35, ease: 2 }; // haze amplitude at cpu 1; wobble metres at cpu 1; ease seconds
  const low = kit.QUALITY === 'low';
  let a = 0, l = 0;

  function step(dt, t) {
    const m = world.metrics, k = Math.min(1, dt / params.ease);
    a += (Math.min(1, Math.max(0, m?.cpu ?? 0)) - a) * k;
    l += (Math.min(1, Math.max(0, m?.hosts?.h1?.load ?? 0)) - l) * k;
    if (a < 0.005 && !(m?.cpu > 0)) a = 0; // exactly 0 at idle: the post haze pass switches off
    if (low) { kit.U.uWobble.value = params.wobble * a * Math.sin(t * 5); return; }
    kit.U.uHaze.value = params.haze * a;
    // ponytail: one sphere from the slab top to the surface above it; cpu elsewhere is not hazed (one mask slot)
    const top = world.heightAt(GX, GZ); // per step (one lookup): deposits and resyncs move the surface
    kit.U.uMask.value.set(GX, (YB + top) / 2, GZ, (top - YB) / 2 * (0.6 + 0.6 * l));
  }

  return {
    root: new THREE.Group(),
    params,
    pose() {},
    step,
    busy: () => Math.abs(a - Math.min(1, Math.max(0, world.metrics?.cpu ?? 0))) > 0.01, // easing
    dispose() { kit.U.uHaze.value = 0; kit.U.uWobble.value = 0; kit.U.uMask.value.set(0, 0, 0, 0); },
  };
}

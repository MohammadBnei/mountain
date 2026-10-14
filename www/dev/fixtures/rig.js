// Gate self-test: step() moves the sun phase (only sky may, via world.rig). Must FAIL (rig untouched).
import cube from './cube.js';
export default async function build(ctx) {
  const m = await cube(ctx);
  return { ...m, step(dt, t) { ctx.kit.setPhase(10 + t, true); } };
}

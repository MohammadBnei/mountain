// Gate self-test: pose(P) accumulates instead of setting. Must FAIL (pose determinism).
import cube from './cube.js';
export default async function build(ctx) {
  const m = await cube(ctx), c = m.root.children[0];
  return { ...m, pose(P) { c.rotation.y += P * 0.3; } };
}

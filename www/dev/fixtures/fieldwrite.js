// Gate self-test: step() pokes a field it does not own (temp belongs to magma). Must FAIL (field writes).
import cube from './cube.js';
export default async function build(ctx) {
  const m = await cube(ctx);
  return { ...m, step() { ctx.world.fields.temp[0] += 1; } };
}

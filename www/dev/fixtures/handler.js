// Gate self-test: a declared ev:argo.sync handler pokes temp (magma's) and wind (socket's) outside step().
// Must FAIL (changed field temp, changed slot wind).
import cube from './cube.js';
export default async function build(ctx) {
  const m = await cube(ctx);
  return { ...m, on: { 'ev:argo.sync'() { ctx.world.fields.temp[5] += 1; ctx.world.wind.x = 99; } } };
}
export const profile = { listens: ['ev:argo.sync'] }; // declared listen (overrides _test)
export async function test({ inject }) { inject('ev:argo.sync', { app: 'x' }); }

// glacier behaviour (03-model-glacier): the hello's glacier is on the shell at build; snowfall thickens the cap; a tongue
// advance reaches further down-slope; a tongue gone (app.gone) calves chunks + one ice.calve, a melted-out one does not;
// ash days thin bands in the ice at their heights; a lower snowline widens the cap; cauldron.sag dishes it and recovers;
// hot temp thins it and emits melt + steam without touching the mass; a new state object (reload) snaps, never calves.
import { ICE } from '../../js/world.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const ice = world.fields.ice, S = 200 / ICE, xz = (k) => [-100 + (k % ICE) * S, -100 + Math.floor(k / ICE) * S];
  const sum = (f = () => true) => { let s = 0; for (let k = 0; k < ice.length; k++) if (f(k)) s += ice[k]; return s; };
  const count = () => ice.reduce((n, v) => n + (v > 0), 0), max = () => ice.reduce((m, v) => Math.max(m, v), 0);
  const near = (k) => { const [x, z] = xz(k); return Math.hypot(x, z) < 8; };
  const got = { 'ice.calve': [], melt: [], steam: [] };
  Object.keys(got).forEach((k) => world.on(k, (p) => got[k].push(p)));
  const glacier = (p) => { inject('world:glacier', p); step(400); };
  const shell = model.root.getObjectByName('shell'), chunks = model.root.getObjectByName('chunks');

  // build: the hello glacier (mass .6, 3 tongues) is on the shell, the tongues reach under the snowline
  const sl = world.snowline, low = () => ice.reduce((n, v, k) => n + (v > 0 && world.heightAt(...xz(k)) < sl - 2), 0);
  const c0 = count(), s0 = sum(), m0 = max();
  assert(c0 > 20 && shell.geometry.attributes.position.count > 0 && low() > 0, `hello glacier on the shell (${c0} iced nodes, ${low()} in tongues below the snowline)`);
  assert(!model.busy(), 'settled at build (no ramp, no chunks)');

  glacier(fixture.snow);
  assert(max() > m0 * 1.3 && sum() > s0, `snowfall thickens the cap (max ${m0.toFixed(2)} → ${max().toFixed(2)})`);
  const c1 = count();
  glacier(fixture.advance);
  assert(count() > c1, `wird's tongue advances 4.5 → 12 m (${c1} → ${count()} iced nodes)`);

  const c2 = count();
  inject('world:glacier', fixture.gone);
  step(2);
  assert(got['ice.calve'].length === 1 && got['ice.calve'][0].size === 3, `app gone → one ice.calve, size = the tongue (${JSON.stringify(got['ice.calve'])})`);
  assert(chunks.count > 0 && model.busy(), `chunks calve (${chunks.count})`);
  step(400);
  assert(chunks.count === 0 && count() < c2, `chunks gone, the tongue's ice too (${c2} → ${count()} nodes)`);
  inject('world:glacier', { ...fixture.gone, tongues: [...fixture.gone.tongues, { app: 'melted', len: 0.7 }] });
  glacier(fixture.gone);
  assert(got['ice.calve'].length === 1, 'a melted-out tongue (< 1 m) does not calve');

  // ash: bands thin the ice at hmax - 1 - age × 0.25
  const before = ice.slice(), bands = [0, 8, 16].map((a) => world.hmax - 1 - a * 0.25);
  glacier(fixture.ash);
  let hit = 0, off = 0;
  for (let k = 0; k < ice.length; k++) if (before[k] - ice[k] > 0.05) (bands.some((y) => Math.abs(world.heightAt(...xz(k)) - y) < 1.7) ? hit++ : off++);
  assert(hit > 0 && off === 0, `ash bands thin the ice at their heights only (${hit} nodes in bands, ${off} elsewhere)`);

  const c3 = count();
  world.write('snowline', 'socket', fixture.snowline);
  step(400);
  assert(count() > c3, `a lower snowline widens the cap (${c3} → ${count()})`);
  world.write('snowline', 'socket', sl);
  step(400);

  const n0 = sum(near);
  inject('cauldron.sag', fixture.sag);
  step(400);
  assert(n0 > 0 && sum(near) < n0 * 0.6, `cauldron.sag dishes the cap (${n0.toFixed(1)} → ${sum(near).toFixed(1)})`);
  inject('cauldron.sag', { ...fixture.sag, depth: 0, sev: null });
  step(400);
  assert(Math.abs(sum(near) - n0) < 0.5, `the sag recovers (${sum(near).toFixed(1)})`);

  const s4 = sum(), mass = world.state.glacier.mass;
  world.write('temp', 'magma', (f) => f.fill(fixture.temp)); // the test plays magma
  step(160 + 400);
  assert(got.melt.length > 0 && got.steam.length > 0 && got.melt[0].vol > 0, `hot ice emits melt + steam (${got.melt.length}, ${got.steam.length})`);
  assert(sum() < s4 * 0.7 && world.state.glacier.mass === mass, `temp thins the ice cosmetically (${s4.toFixed(0)} → ${sum().toFixed(0)}), mass unchanged`);
  world.write('temp', 'magma', (f) => f.fill(0));
  step(160 + 400);
  assert(Math.abs(sum() - s4) < 1, 'cooled: the ice is back');

  // reload: a new state object snaps to its glacier without calving, even with a tongue missing
  world.write('state', 'socket', { ...world.state, glacier: fixture.advance });
  step(1);
  const c5 = count();
  world.write('state', 'socket', { ...world.state, glacier: fixture.gone });
  step(1);
  assert(got['ice.calve'].length === 1 && count() < c5 && !model.busy(), `a reload snaps (${c5} → ${count()} nodes), no calving`);
}

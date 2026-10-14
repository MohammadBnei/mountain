// lightning behaviour (03-model-lightning): corona strength follows total rpsB (none at 0); a spike → exactly one
// bolt from the base of the nearest mass cluster down to the site, ink; errB > 0 → magma; no clouds → the bolt
// starts params.above m over the site.
export default async function test({ model, world, inject, step, fixture, assert }) {
  const halos = model.root.getObjectByName('halos'), arcs = model.root.getObjectByName('corona'), bolt = model.root.getObjectByName('bolt');
  const base = world.metrics, frame = (apps) => world.applyFrame({ ...base, apps });
  for (const s of fixture.sites) inject(s.type, s.payload);
  world.write('clouds', 'sky', fixture.clouds);
  const I = halos.instanceMatrix.array; // instance 0 = the corona core; its scale = |column 0|
  const core = () => { let s = 0; for (let n = 0; n < 20; n++) { step(5); s += Math.hypot(I[0], I[1], I[2]); } return s / 20; };

  frame({});
  step(10);
  assert(!arcs.visible && !halos.visible, 'no traffic: no corona, no halos');
  frame(fixture.calm);
  const calm = core();
  assert(arcs.visible && halos.count === 3, `calm: corona + core + 2 site halos (${arcs.visible}, ${halos.count})`);
  frame(fixture.busy);
  const busy = core();
  assert(busy > calm * 1.5, `corona strength follows total rpsB (core ${calm.toFixed(2)} -> ${busy.toFixed(2)})`);
  assert(halos.count === 7, `rpsB 10: 3 stacked halo quads per site (${halos.count})`);
  step(60);
  assert(!bolt.visible && !model.busy(), 'no spike: no bolt');

  const A = bolt.geometry.attributes.position.array, H = world.heightAt(-50, 5);
  /** Steps 30 times after `apps` lands; → [bolts seen, first bolt's start xyz, main ribbon end xyz, material token]. */
  const watch = (apps) => {
    frame(apps);
    let n = 0, was = false, got = null;
    for (let i = 0; i < 30; i++) {
      step(1);
      if (bolt.visible && !was) { n++; got ??= [[A[0], A[1], A[2]], [A[31 * 6 + 3], A[31 * 6 + 4], A[31 * 6 + 5]], bolt.material.userData.kitToken]; }
      was = bolt.visible;
    }
    return [n, ...(got || [])];
  };
  const near = (a, b, e = 1e-3) => a.every((v, i) => Math.abs(v - b[i]) < e);

  let [n, a, b, tok] = watch(fixture.spike);
  assert(n === 1, `a spike frame strikes exactly one bolt (${n})`);
  assert(Math.abs(a[1] - 100) < 1e-3 && Math.hypot(a[0] + 60, a[2]) <= 10, `bolt starts on the nearest mass base (y 100, within r/2 of (-60, 0)): ${a}`);
  assert(near(b, [-50, H, 5]), `bolt ends at the site's ground (${b} vs -50,${H},5)`);
  assert(tok === 'ink', `errB 0: ink bolt (${tok})`);
  assert(!model.busy(), 'bolt done after ~150 ms');

  [n, , , tok] = watch(fixture.fault);
  assert(n === 1 && tok === 'magma', `errB > 0: one magma bolt (${n}, ${tok})`);

  world.write('clouds', 'sky', []);
  [n, a] = watch(fixture.spike);
  assert(n === 1 && near(a, [-50, H + model.params.above, 5]), `no clouds: bolt from params.above over the site (${a})`);
  frame({});
  step(10);
}

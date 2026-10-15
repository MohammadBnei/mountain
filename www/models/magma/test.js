// magma behaviour (03-model-magma): a dyke per build in flight rises from the chamber toward world.sites[app]
// (running reaches it and emits magma.dyke once, queued stalls part way), retracts when the build ends, comes back
// from world.state.builds alone after a reload; glow and the ember pool follow metrics.heat; temp warms over the
// chamber, along a dyke and at a lava.heat spot; the cut chain composes magma before the mountain's caves.
import { U } from '../../js/kit.js';
import { CUT } from '../../js/shaders.js';

export default async function test({ model, world, inject, step, fixture, assert }) {
  const u = model.field.uniforms, B = u.uMagmaB.value, temp = world.fields.temp, T = (x, z) => temp[world.cell(x, z)];
  const dykes = [], off = world.on('magma.dyke', (p) => dykes.push(p));
  const [wird, mtn] = fixture.sites;
  try {
    // the hello's wird build has no site in the sandbox world: only the feeder dyke
    assert(u.uMagmaN.value === 1, `no site → no build dyke (${u.uMagmaN.value})`);
    world.write('state', 'socket', { ...world.state, builds: {} }); // the test plays socket: a hello with no builds
    step(10);

    for (const s of fixture.sites) inject('world:site', s);
    for (const b of fixture.builds) inject('ev:build', b);
    step(1);
    assert(u.uMagmaN.value === 3, `two builds in flight → feeder + 2 dykes (${u.uMagmaN.value})`);
    step(130); // rise 4 s
    assert(dykes.length === 1 && dykes[0].app === 'wird' && dykes[0].x === wird.x && dykes[0].z === wird.z, `running reaches its site → one magma.dyke (${JSON.stringify(dykes)})`);
    assert(Math.abs(B[1].x - wird.x) < 1e-3 && Math.abs(B[1].y - (wird.y - 1.5)) < 1e-3 && Math.abs(B[1].z - wird.z) < 1e-3, `wird tip under its site (${B[1].toArray()})`);
    const a2 = u.uMagmaA.value[2], f = (B[2].x - a2.x) / (mtn.x - a2.x);
    assert(Math.abs(f - 0.35) < 0.01, `queued stalls at .35 of the way (${f.toFixed(3)})`);
    step(10);
    assert(T(wird.x, wird.z) > 0.9 && T(-95, -95) === 0 && T(0, 10) === 0, `temp: dyke ${T(wird.x, wird.z)}, far ${T(-95, -95)}, idle chamber ${T(0, 10)}`);

    for (const b of fixture.done) inject('ev:build', b);
    step(130);
    assert(u.uMagmaN.value === 1 && !model.busy() && dykes.length === 1, `ended builds retract (${u.uMagmaN.value} capsules, busy ${model.busy()})`);

    // a reload: world.state.builds alone (no ev:build) brings the dyke back
    world.write('state', 'socket', { ...world.state, builds: { wird: 'running' } });
    step(10);
    assert(u.uMagmaN.value === 2 && !model.busy() && dykes.length === 1, `world.state.builds after a resync → dyke, grown, no magma.dyke (${u.uMagmaN.value}, ${dykes.length})`);
    world.write('state', 'socket', { ...world.state, builds: {} });
    step(140);
    assert(u.uMagmaN.value === 1, 'resync without builds → retracted');

    // glow = metrics.heat, eased; the ember pool gets it through kit.setPool
    const m0 = world.metrics;
    world.write('metrics', 'socket', { ...m0, heat: fixture.heat });
    step(150);
    assert(U.uHeat.value > 0.7 && u.uMagmaGlow.value > 1.3 && T(0, 10) > 0.5, `heat ${fixture.heat} → pool uHeat ${U.uHeat.value.toFixed(3)}, glow ${u.uMagmaGlow.value.toFixed(3)}, chamber temp ${T(0, 10)}`);
    world.write('metrics', 'socket', m0);
    step(150);
    assert(U.uHeat.value < 0.1, `heat back to 0 → pool ${U.uHeat.value.toFixed(3)}`);

    // lava.heat warms temp where the flow landed, then it cools
    const { x, z } = fixture.lava, t0 = T(x, z);
    inject('lava.heat', fixture.lava);
    step(10);
    const t1 = T(x, z);
    step(300);
    assert(t0 === 0 && t1 > 0.7 && T(x, z) < t1 && T(x, z) > 0.5, `lava.heat spot ${t0} → ${t1} → ${T(x, z)}`);

    assert(CUT.key === 'cap:magma,caves' && CUT.pars.indexOf('c = magmaField(p, c);') < CUT.pars.indexOf('c = cavesField(p, c);'), `cut chain magma → caves (${CUT.key})`);
  } finally { off(); }
}

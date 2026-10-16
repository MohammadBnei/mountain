// cauldron behaviour (03-model-cauldron): a critical older than 30 min → one jokulhlaup; warning → sag > 0, no steam;
// critical → deeper sag, red under-glow, steam visible, vertex-animated and emitted; resolve → recovers (ring gone,
// sag 0); a Watchdog-style severity never sags; state restored without a bus event (reload) sags too; pick → alerts card.
import * as THREE from 'three';

export default async function test({ model, world, inject, step, fixture, assert }) {
  world.state.alerts = []; // a restored hello may carry firing alerts (a long critical would burst at once): start from none
  const ring = model.root.getObjectByName('sag'), steam = model.root.getObjectByName('steam');
  const got = { 'cauldron.sag': [], jokulhlaup: [], steam: [] };
  const offs = Object.keys(got).map((k) => world.on(k, (p) => got[k].push(p)));
  const last = () => got['cauldron.sag'].at(-1);
  // silent = hello.state replay (state only, no ev:alert.* emit), as on a reload
  const fire = (a, ago = 0, silent = false) => world.applyFrame({ t: 'ev', id: `01JA${a.fp.padStart(22, '0')}`, ts: Date.now(), kind: 'alert.fire', ...a, startsAt: Date.now() - ago }, silent);
  const resolve = (a) => inject('ev:alert.resolve', { fp: a.fp, sev: a.sev, layer: a.layer, durS: 60 });
  const dip = () => { const p = ring.geometry.attributes.position; let m = 1e9; for (let i = 0; i < p.count; i++) m = Math.min(m, p.getY(i) - world.heightAt(p.getX(i), p.getZ(i))); return m; };

  // first, while the model clock still tracks Date.now(): 10 s short of breakMin → nothing, past it → exactly one
  fire(fixture.old, 30 * 60e3 - 10e3);
  step(150);
  assert(got.jokulhlaup.length === 0, `no jokulhlaup before 30 min critical (${got.jokulhlaup.length})`);
  step(300);
  assert(got.jokulhlaup.length === 1, `one jokulhlaup after 30 min critical (${got.jokulhlaup.length})`);
  step(600);
  assert(got.jokulhlaup.length === 1, `still one jokulhlaup (${got.jokulhlaup.length})`);
  resolve(fixture.old);
  step(450);
  assert(!ring.visible && !steam.visible && last().depth === 0, 'resolved: ring and steam gone, sag 0');

  fire(fixture.warning);
  step(90);
  const warn = last();
  assert(warn.sev === 'warning' && warn.depth > 0, `warning sags (${JSON.stringify(warn)})`);
  assert(ring.visible && !steam.visible, 'warning: ring, no steam');
  const warnDip = dip();
  assert(warnDip < 1, `the ring dishes at its inner edge (${warnDip.toFixed(2)} m over the terrain, 1.2 at rest)`);
  const w = ring.material.emissive; // 09 dusk legibility: a cool water lift so the dish reads as deep ice, not a void
  assert(w.b > 0.05 && w.b > w.r, `warning: cool lift, no red glow (${w.r.toFixed(3)}, ${w.g.toFixed(3)}, ${w.b.toFixed(3)})`);
  const down = new THREE.Ray(new THREE.Vector3(0, 200, 0), new THREE.Vector3(0, -1, 0)), card = model.pick(down);
  assert(card?.kind === 'cauldron' && Number.isFinite(card.d) && card.alerts === world.state.alerts.length, `pick the sag → its card (${JSON.stringify(card)})`);
  assert(model.pick(new THREE.Ray(down.origin, new THREE.Vector3(0, 1, 0))) === null, 'pick into the sky → null');

  fire(fixture.critical);
  step(180);
  assert(last().sev === 'critical' && last().depth > warn.depth, `critical sags deeper (${JSON.stringify(last())})`);
  assert(steam.visible && dip() < warnDip, 'critical: steam visible, ring deeper');
  const e = ring.material.emissive;
  assert(e.r > 1 && e.r > e.b, `red under-glow (${e.r.toFixed(2)}, ${e.g.toFixed(2)}, ${e.b.toFixed(2)})`);
  const a = steam.geometry.attributes.position.array.slice();
  step(1);
  assert(steam.geometry.attributes.position.array.some((v, i) => v !== a[i]), 'steam is vertex-animated');
  assert(got.steam.length >= 1, `steam emitted while critical (${got.steam.length})`);

  resolve(fixture.critical);
  resolve(fixture.warning);
  fire(fixture.watchdog);
  step(450);
  assert(!ring.visible && !steam.visible && last().depth === 0, `resolve recovers; Watchdog-style sev never sags (${JSON.stringify(last())})`);
  assert(e.r < e.b, 'no red glow once recovered');
  resolve(fixture.watchdog);

  fire(fixture.warning, 0, true);
  step(90);
  assert(ring.visible && last().sev === 'warning', 'state restored without a bus event sags (reload)');
  resolve(fixture.warning);
  step(450);
  assert(!model.busy(), 'settled');
  offs.forEach((f) => f());
}

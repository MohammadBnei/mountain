// river behaviour (03-model-river): the ribbon runs downhill from under the snowline, on the terrain, never under it;
// metrics.river widens it and quickens the foam (uTime advances faster, the only uniform the model writes); melt,
// aquifer.seep and jokulhlaup each surge it (the jokulhlaup most), then it settles back to the metric's width.
export default async function test({ model, world, inject, step, fixture, assert }) {
  const rib = model.root.getObjectByName('ribbon'), p = rib.geometry.attributes.position, u = rib.material.uniforms, A = 5; // verts per row
  const rows = p.count / A;
  const width = () => { let s = 0; for (let r = 0; r < rows; r++) s += Math.hypot(p.getX(r * A) - p.getX(r * A + A - 1), p.getZ(r * A) - p.getZ(r * A + A - 1)); return s / rows; };
  const speed = (n = 30) => { const t0 = u.uTime.value; step(n); return (u.uTime.value - t0 + 200) % 200 / (n / 30); };
  const river = (v) => { world.applyFrame({ ...world.metrics, river: v }); step(600); };

  // course: downhill from under the snowline, draped over the terrain (never below it, never floating)
  const y = (r) => p.getY(r * A + 2);
  let under = 0, high = 0;
  const tol = globalThis.__quality === 'low' ? 2 : 0.2; // the low body is a coarser 96² mesh than heightSolid (the drape follows it)
  for (let i = 0; i < p.count; i++) { const d = p.getY(i) - world.heightAt(p.getX(i), p.getZ(i)); if (d < -tol) under++; if (d > 2) high++; }
  assert(y(0) < world.snowline && y(0) - y(rows - 1) > 15, `runs downhill from under the snowline (${y(0).toFixed(1)} → ${y(rows - 1).toFixed(1)}, snowline ${world.snowline})`);
  assert(under === 0 && high === 0, `draped on the terrain (${under} vertices under it, ${high} floating > 2 m)`);
  const keys = Object.keys(u).join(), op = u.uOpacity.value;

  river(fixture.quiet);
  const w0 = width(), v0 = speed();
  assert(!model.busy(), 'settled at a steady metric');
  river(fixture.busy);
  const w1 = width(), v1 = speed();
  assert(w1 > w0 * 2.5 && v1 > v0 * 3, `metrics.river 0 → 1 widens (${w0.toFixed(2)} → ${w1.toFixed(2)} m) and quickens it (${v0.toFixed(2)} → ${v1.toFixed(2)} /s)`);

  river(fixture.base);
  const wb = width(), vb = speed(), peak = {};
  for (const [type, k] of [['melt', 'melt'], ['aquifer.seep', 'seep'], ['jokulhlaup', 'jokulhlaup']]) {
    inject(type, fixture[k]);
    step(90);
    peak[type] = width();
    assert(peak[type] > wb + 0.3 && model.busy(), `${type} surges it (${wb.toFixed(2)} → ${peak[type].toFixed(2)} m)`);
    if (type === 'jokulhlaup') assert(speed() > vb * 1.5, 'the jokulhlaup quickens the foam');
    step(1800);
    assert(Math.abs(width() - wb) < 0.05 && !model.busy(), `${type}: back to the metric's width (${width().toFixed(2)})`);
  }
  assert(peak.jokulhlaup > peak.melt + 3 && peak.jokulhlaup > peak['aquifer.seep'] + 3, `the jokulhlaup is the flood (${JSON.stringify(peak)})`);
  assert(Object.keys(u).join() === keys && u.uOpacity.value === op, 'only uTime is written');
}

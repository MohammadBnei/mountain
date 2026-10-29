// The pure geometry of L2 and L3 (no three.js, so core.test.mjs can run it under node): where a city's districts and
// buildings stand, how tall a building is, and where a building's wall cards hang.

const GAP = 1.15,
  PAD = 0.7,
  DISTRICT_GAP = 1.4;

/** layout places districts in rows and each district's buildings on a grid inside it. Pure (core.test.mjs). */
export function layout(groups) {
  const cols = Math.ceil(Math.sqrt(groups.length || 1));
  const plots = groups.map(g => {
    const n = g.members.length,
      c = Math.ceil(Math.sqrt(n));
    return { g, cols: c, w: c * GAP + 2 * PAD, d: Math.ceil(n / c) * GAP + 2 * PAD };
  });
  const rows = [];
  plots.forEach((p, i) => {
    (rows[Math.floor(i / cols)] ||= []).push(p);
  });
  let z = 0,
    width = 0;
  for (const row of rows) {
    let x = 0;
    const depth = Math.max(...row.map(p => p.d));
    for (const p of row) {
      p.x = x;
      p.z = z;
      x += p.w + DISTRICT_GAP;
    }
    width = Math.max(width, x - DISTRICT_GAP);
    z += depth + DISTRICT_GAP;
  }
  const depth = z - DISTRICT_GAP;
  for (const p of plots) {
    // centre the whole city on the origin
    p.x -= width / 2;
    p.z -= depth / 2;
    p.buildings = p.g.members.map((id, i) => ({
      id,
      x: p.x + PAD + (i % p.cols) * GAP + GAP / 2,
      z: p.z + PAD + Math.floor(i / p.cols) * GAP + GAP / 2
    }));
  }
  return { plots, width, depth };
}

export const heightOf = citations => 0.5 + 0.55 * Math.log2(1 + citations);

export const MAX_CARDS = 7,
  RING = 3.4;

/** cardSlots places n cards on a ring facing out, the first one towards the default camera. Pure (core.test.mjs). */
export function cardSlots(n) {
  return Array.from({ length: n }, (_, i) => {
    const a = Math.PI / 2 - (i - (n - 1) / 2) * (n <= 4 ? Math.PI / 4.5 : (Math.PI * 2) / n); // a few face the front; seven go round
    return { x: RING * Math.cos(a), z: RING * Math.sin(a), ry: Math.PI / 2 - a };
  });
}

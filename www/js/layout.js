// Town layout (R7): hex math, the relay's tile map read as world positions, and what each building kind looks like.
// The relay owns the geometry (relay/town, hello.town); nothing here decides where a tile or a plot is.
//
// Hexes are pointy-top axial (q, r), the pi-lab kit.hex grid (KayKit tiles, flat-to-flat 2): x = 2(q + r/2), z = √3·r.
// A plot's local (q, r) is on its host's island; global = island centre + local.
export const SQ3 = Math.sqrt(3);
export const hexXZ = (q, r) => [2 * (q + r / 2), SQ3 * r];

/** Local hexes in the relay's canonical order (relay/town Hexes): r ascending, then q. */
export function hexes(radius) {
  const l = [];
  for (let r = -radius; r <= radius; r++) for (let q = Math.max(-radius, -r - radius); q <= Math.min(radius, -r + radius); q++) l.push([q, r]);
  return l;
}
export const ring = (q, r) => Math.max(Math.abs(q), Math.abs(r), Math.abs(q + r));

/** island → {host, centre:[x, z], tiles:[{q, r, kind, x, z}], tileAt(q, r)}. */
export function islandsOf(town) {
  const list = hexes(town.radius);
  return town.islands.map((is) => {
    const [cx, cz] = hexXZ(is.q, is.r), map = new Map();
    const tiles = list.map(([q, r], i) => {
      const [x, z] = hexXZ(is.q + q, is.r + r);
      const t = { q, r, kind: is.tiles[i], x, z };
      map.set(`${q},${r}`, t);
      return t;
    });
    return { host: is.host, centre: [cx, cz], tiles, tileAt: (q, r) => map.get(`${q},${r}`) };
  });
}

/** World position of local hex (q, r) on host's island. */
export function plotXZ(town, host, q, r) {
  const is = town.islands.find((i) => i.host === host);
  return hexXZ(is.q + q, is.r + r);
}

/** A local hex → world, and back (nearest hex, cube rounding). */
export function worldToHex(x, z) {
  const fr = z / SQ3, fq = x / 2 - fr / 2, fs = -fq - fr;
  let q = Math.round(fq), r = Math.round(fr); const s = Math.round(fs);
  const dq = Math.abs(q - fq), dr = Math.abs(r - fr), ds = Math.abs(s - fs);
  if (dq > dr && dq > ds) q = -r - s; else if (dr > ds) r = -q - s;
  return [q, r];
}

// Building kinds (relay/town Kinds) → KayKit model, scale, and small props merged into the body:
// [model, x, z, rotY, scale] in the building's own frame (front = +z, towards the island square).
export const BUILDINGS = {
  home: { glb: 'building_home_A_blue', s: 2.3 },
  house: { glb: 'building_home_B_blue', s: 2.0 },
  market: { glb: 'building_market_blue', s: 1.28 },
  tavern: { glb: 'building_tavern_blue', s: 1.75, props: [['barrel', 0.75, 0.55, 0, 1.6]] },
  tower: { glb: 'building_tower_A_blue', s: 1.82 },
  church: { glb: 'building_church_blue', s: 2.0 },
  castle: { glb: 'building_castle_blue', s: 1.1 },
  gate: { glb: 'wall_straight_gate', s: 1.28 },
  guardhouse: { glb: 'building_tower_base_blue', s: 2.0 },
  craneyard: { glb: 'building_barracks_blue', s: 1.42, props: [['crate_A_small', 0.8, 0.6, 0.3, 2.2], ['crate_A_small', 0.6, 0.75, -0.2, 2.2]] },
  watchtower: { glb: 'building_tower_A_blue', s: 2.16 },
  warehouse: { glb: 'building_home_B_blue', s: 1.89, props: [['barrel', 0.7, 0.4, 0, 1.8], ['barrel', 0.75, 0.0, 0.5, 1.8], ['sack', -0.7, 0.45, 0.4, 2.2]] },
  vault: { glb: 'building_tower_base_blue', s: 1.69, props: [['crate_A_small', 0.7, 0.5, 0.2, 2]] },
  granary: { glb: 'building_well_blue', s: 2.7, props: [['sack', 0.6, 0.5, 0.4, 2.4], ['sack', -0.6, 0.55, -0.3, 2.4], ['sack', 0.5, -0.6, 1, 2.4]] },
  depot: { glb: 'building_home_A_blue', s: 2.03, props: [['crate_A_small', 0.75, 0.5, 0.1, 2.2], ['crate_A_small', -0.75, 0.45, 0.4, 2.2], ['crate_A_small', 0.72, 0.5, 0.2, 1.8, 0.3]] },
};

// The Ukubi wire of an app or service (apps.json "wire": app | agent | data | network | storage | compute).
export const wireOf = (apps, app) => apps[app]?.wire || 'app';

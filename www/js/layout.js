// Town layout (R7, R8): hex math, the relay's tile map read as world positions, and what each building kind looks like.
// The relay owns the geometry (relay/town, hello.town); nothing here decides where a tile or a plot is.
//
// Hexes are pointy-top axial (q, r), the pi-lab kit.hex grid (KayKit tiles, flat-to-flat 2): x = 2(q + r/2), z = √3·r.
// A plot's local (q, r) is on its host's island; global = island centre + local. R8: each island has its own radius
// (its host's power, relay/town Radii).
export const SQ3 = Math.sqrt(3);
export const hexXZ = (q, r) => [2 * (q + r / 2), SQ3 * r];

/** Local hexes in the relay's canonical order (relay/town Hexes): r ascending, then q. */
export function hexes(radius) {
  const l = [];
  for (let r = -radius; r <= radius; r++) for (let q = Math.max(-radius, -r - radius); q <= Math.min(radius, -r + radius); q++) l.push([q, r]);
  return l;
}
export const ring = (q, r) => Math.max(Math.abs(q), Math.abs(r), Math.abs(q + r));

/** island → {host, radius, centre:[x, z], tiles:[{q, r, kind, x, z}], tileAt(q, r)}. */
export function islandsOf(town) {
  return town.islands.map((is) => {
    const [cx, cz] = hexXZ(is.q, is.r), map = new Map();
    const tiles = hexes(is.radius).map(([q, r], i) => {
      const [x, z] = hexXZ(is.q + q, is.r + r);
      const t = { q, r, kind: is.tiles[i], x, z };
      map.set(`${q},${r}`, t);
      return t;
    });
    return { host: is.host, radius: is.radius, centre: [cx, cz], tiles, tileAt: (q, r) => map.get(`${q},${r}`) };
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
// R8 typology: every kind belongs to a family (relay/town families) that sets where it stands, the ground cap under
// it and its label tier. The styleboard (www/dev/townshots.mjs) is where scales and props are tuned.
export const BUILDINGS = {
  // Defense: security and control
  keep: { glb: 'building_castle_blue', s: 1.1 },
  gate: { glb: 'wall_straight_gate', s: 1.28, props: [['wall_straight', 1.3, 0, 0, 1.28], ['wall_straight', -1.3, 0, 0, 1.28]] },
  guardhouse: { glb: 'building_tower_catapult_blue', s: 1.7 },
  watchtower: { glb: 'building_tower_A_blue', s: 2.16 },
  vault: { glb: 'building_mine_blue', s: 1.3, props: [['fence_stone_straight', 0, 0.95, 0, 1.4]] },
  castle: { glb: 'building_castle_blue', s: 1.1 }, // R7 kinds, kept for replays of older days
  tower: { glb: 'building_tower_A_blue', s: 1.82 },
  // Public: the shared platform
  craneyard: { glb: 'building_tower_B_blue', s: 1.75 },
  depot: { glb: 'building_market_blue', s: 1.28 },
  warehouse: { glb: 'building_home_B_blue', s: 1.89, props: [['barrel', 0.7, 0.4, 0, 1.8], ['barrel', 0.75, 0.0, 0.5, 1.8], ['sack', -0.7, 0.45, 0.4, 2.2]] },
  granary: { glb: 'building_grain', s: 1.0, props: [['sack', 1.0, 0.9, 0.4, 2.2], ['sack', -0.9, 0.95, -0.3, 2.2], ['flag_blue', 1.2, -0.8, 0, 2.4]] }, // a field the size of its hex
  archive: { glb: 'building_tower_base_blue', s: 1.75, props: [['crate_long_A', 0.75, 0.55, 0.2, 1.6]] },
  cellar: { glb: 'building_watermill_blue', s: 1.6 },
  market: { glb: 'building_market_blue', s: 1.28 }, // R7
  // Industry: the infra machines
  hall: { glb: 'building_barracks_blue', s: 1.42 },
  forge: { glb: 'building_blacksmith_blue', s: 1.6, props: [['barrel', 0.8, 0.55, 0, 1.6]] },
  witness: { glb: 'building_windmill_blue', s: 1.6 },
  mill: { glb: 'building_lumbermill_blue', s: 1.5 },
  // Shops: user apps with public traffic (one tavern model, each with its own sign of props)
  church: { glb: 'building_church_blue', s: 2.0 },
  tavern: { glb: 'building_tavern_blue', s: 1.75, props: [['barrel', 0.75, 0.55, 0, 1.6]] },
  inn: { glb: 'building_tavern_blue', s: 1.75, props: [['crate_long_A', 0.8, 0.5, 0.4, 1.5]] },
  alehouse: { glb: 'building_tavern_blue', s: 1.75, props: [['barrel', 0.8, 0.45, 0, 1.6], ['barrel', 0.55, 0.75, 0.4, 1.6]] },
  bookshop: { glb: 'building_tavern_blue', s: 1.75, props: [['crate_A_small', 0.8, 0.55, 0.3, 2.2], ['sack', -0.8, 0.5, 0, 2.2]] },
  playhouse: { glb: 'building_tavern_blue', s: 1.75, props: [['flag_blue', 0.85, 0.5, 0, 2.6], ['flag_blue', -0.85, 0.5, 0, 2.6]] },
  // Houses: small or internal apps
  home: { glb: 'building_home_A_blue', s: 2.3 },
  house: { glb: 'building_home_B_blue', s: 2.0 },
};

/** relay/town families, mirrored: kind → defense | public | industry | shops | houses. */
export const FAMILY = {
  keep: 'defense', gate: 'defense', guardhouse: 'defense', watchtower: 'defense', vault: 'defense', castle: 'defense', tower: 'defense',
  craneyard: 'public', depot: 'public', warehouse: 'public', granary: 'public', archive: 'public', cellar: 'public', market: 'public',
  hall: 'industry', forge: 'industry', witness: 'industry', mill: 'industry',
  church: 'shops', tavern: 'shops', inn: 'shops', alehouse: 'shops', bookshop: 'shops', playhouse: 'shops',
  home: 'houses', house: 'houses',
};
export const familyOf = (kind) => FAMILY[kind] || 'houses';

// The Ukubi wire of an app or service (apps.json "wire": app | agent | data | network | storage | compute).
export const wireOf = (apps, app) => apps[app]?.wire || 'app';

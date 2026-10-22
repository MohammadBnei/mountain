// Islands (R7): one floating hex island per host bucket, from hello.town (relay/town). KayKit hex tiles merged per
// island into one mesh (clay atlas), a lumpy earth cone underneath (pi-lab island.js), a well on the square.
// A host whose nodes are not Ready greys its island (and update() reports it for the buildings on it).
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { islandsOf, ring } from '../layout.js';

export default async function build({ THREE, kit, world }) {
  const root = new THREE.Group();
  const { CELL, recell } = kit;
  const xf = (x, y, z, rotY = 0, s = 1) => new THREE.Matrix4().compose(new THREE.Vector3(x, y, z),
    new THREE.Quaternion().setFromEuler(new THREE.Euler(0, rotY, 0)), new THREE.Vector3(s, s, s));
  const one = async (name) => mergeGeometries(await kit.geometry(name));
  const grass = recell(await one('hex_grass'), CELL.olive, (ny) => (ny > 0.5 ? CELL.sage : CELL.tan));
  const meadow = recell(await one('hex_grass'), CELL.olive, (ny) => (ny > 0.5 ? [3, 3] : CELL.tan)); // a deeper green rim
  const square = recell(await one('hex_grass'), CELL.olive, (ny) => (ny > 0.5 ? [5, 2] : CELL.tan));
  const road = recell(await one('hex_road_A'), CELL.olive, (ny) => (ny > 0.5 ? CELL.sage : CELL.tan)); // its strip runs along +x
  const well = await one('building_well_blue');
  // rotation.y that turns +x onto the axial direction (q, r): always a multiple of 60°, so tiles stay on the grid
  const dirAngle = (q, r) => Math.atan2(-Math.sqrt(3) * r, 2 * (q + r / 2));

  const islands = islandsOf(world.town), meshes = {};
  for (const is of islands) {
    const parts = [];
    for (const t of is.tiles) {
      const g = t.kind === 'm' ? meadow : t.kind === 's' ? square : t.kind === 'r' || t.kind === 'b' ? road : grass;
      // road tiles turn along their spoke: the direction from the square to the tile
      const rot = t.kind === 'r' || t.kind === 'b' ? dirAngle(t.q / ring(t.q, t.r), t.r / ring(t.q, t.r)) : 0;
      parts.push(g.clone().applyMatrix4(xf(t.x, 0, t.z, rot)));
    }
    const [cx, cz] = is.centre;
    parts.push(well.clone().applyMatrix4(xf(cx, 0, cz, 0, 1.6)));
    // earth cone under the tiles, lumpy and faceted, darker toward the tip (pi-lab island.js, scaled to radius 4)
    const top = -0.9, R = 2 * world.town.radius - 0.2; // inside the tiles' outline, so no rim pokes out
    const prof = [[0.4, -7.5], [0.3 * R, -6.4], [0.55 * R, -4.6], [0.8 * R, -2.6], [R, -0.9]].map(([r, y]) => new THREE.Vector2(r, y));
    const cone = new THREE.LatheGeometry(prof, 14).toNonIndexed();
    const cp = cone.attributes.position, cuv = cone.attributes.uv, seed = is.host.charCodeAt(1);
    for (let i = 0; i < cp.count; i++) {
      const x = cp.getX(i), y = cp.getY(i), z = cp.getZ(i), d = (top - y) / (top + 7.5);
      const a = Math.atan2(z, x), f = 1 + (0.1 * Math.sin(a * 3 + seed) + 0.07 * Math.sin(a * 7 + d * 4 + seed)) * d;
      cp.setXYZ(i, cx + x * f, y, cz + z * f);
      cuv.setXY(i, (CELL.tan[0] + 0.5) / 8, (CELL.tan[1] + 0.05 + 0.9 * d) / 4);
    }
    cone.deleteAttribute('normal');
    cone.computeVertexNormals();
    parts.push(cone);
    const mat = kit.variant('shared');
    const m = kit.mesh(mergeGeometries(parts), mat);
    m.userData = { pick: { kind: 'island', host: is.host } };
    meshes[is.host] = m;
    root.add(m);
  }

  const grey = new THREE.Color(0.42, 0.44, 0.5), white = new THREE.Color(1, 1, 1);
  const SINK = 0.5; // a down island sinks a little: its whole town goes quiet and dark
  let last = '';
  return {
    root, islands,
    update() {
      const key = islands.map((is) => world.ready[is.host] !== false).join();
      if (key === last) return false;
      last = key;
      for (const is of islands) {
        const down = world.ready[is.host] === false;
        meshes[is.host].material.color.copy(down ? grey : white);
        meshes[is.host].position.y = down ? -SINK : 0;
      }
      return true;
    },
    pickables: Object.values(meshes),
  };
}

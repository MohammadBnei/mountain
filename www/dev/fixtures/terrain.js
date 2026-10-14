// Gate fixture (not a model): the Go h.base on kit terrain() + cave(), to check the style carry items
// on the real kit render (09 Freeze: dusk caves, rim falloff, ember pool framing). Lane C's mountain replaces it.
export default async function build({ THREE, kit, world }) {
  const G = 128, C = 200 / G, hs = world.fields.heightSolid;
  const g = new THREE.PlaneGeometry(C * (G - 1), C * (G - 1), G - 1, G - 1).rotateX(-Math.PI / 2).translate(-100 + (C * (G - 1)) / 2, 0, -100 + (C * (G - 1)) / 2);
  const p = g.attributes.position, bench = new Float32Array(p.count);
  for (let i = 0; i < p.count; i++) { const k = world.cell(p.getX(i), p.getZ(i)); p.setY(i, hs[k]); bench[i] = world.bench[k] / 255; }
  g.setAttribute('bench', new THREE.BufferAttribute(bench, 1));
  const geo = g.toNonIndexed();
  geo.computeVertexNormals();
  const root = new THREE.Group();
  root.add(new THREE.Mesh(geo, kit.terrain()), kit.cave(world.caves, world.heightAt));
  return { root, params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

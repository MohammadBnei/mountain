// Gate self-test: a kit material whose colour was changed off the palette. Must FAIL (colour membership).
export default async function build({ THREE, kit, plot }) {
  const root = new THREE.Group(), m = kit.lit('rock0');
  const cube = new THREE.Mesh(new THREE.BoxGeometry(2, 2, 2, 2, 2, 2), m);
  cube.position.set(plot.x, 1, plot.z);
  root.add(cube);
  return { root, params: {}, pose() { m.color.setRGB(1, 0, 0, THREE.SRGBColorSpace); }, step() {}, busy: () => false, dispose() {} };
}

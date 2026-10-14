// Gate self-test: a material not from kit. Must FAIL.
export default async function build({ THREE, plot }) {
  const root = new THREE.Group();
  const m = new THREE.Mesh(new THREE.BoxGeometry(2, 2, 2), new THREE.MeshNormalMaterial());
  m.position.set(plot.x, 1, plot.z);
  root.add(m);
  return { root, params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

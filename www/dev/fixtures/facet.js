// Gate self-test: a 3-unit box in the rock band [0.15, 1.2]. Must FAIL (facet band).
export default async function build({ THREE, kit, plot }) {
  const root = new THREE.Group(), box = new THREE.Mesh(new THREE.BoxGeometry(3, 3, 3), kit.lit('rock0'));
  box.position.set(plot.x, 1.5, plot.z);
  root.add(box);
  return { root, params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

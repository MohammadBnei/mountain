// Gate self-test: a correct kit cube. Must PASS.
export default async function build({ THREE, kit, plot }) {
  const root = new THREE.Group();
  const cube = new THREE.Mesh(new THREE.BoxGeometry(2, 2, 2), kit.lit('rock0'));
  cube.position.set(plot.x, 1, plot.z);
  root.add(cube);
  return { root, params: {}, pose(P) { cube.rotation.y = P * 0.3; }, step() {}, busy: () => false, dispose() {} };
}
export async function test({ model, step, assert }) {
  model.pose(1);
  step(3);
  assert(Math.abs(model.root.children[0].rotation.y - 0.3) < 1e-9, 'pose(1) rotates the cube by 0.3');
}

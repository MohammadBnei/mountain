// Gate self-test: an ink panel facing the day sun (kit SUN.day az 20°, el 50°) lands over linear luminance 1.
// Must FAIL (luminance ceiling).
export default async function build({ THREE, kit, plot }) {
  const root = new THREE.Group(), t = new THREE.Mesh(new THREE.PlaneGeometry(4, 4, 4, 4), kit.lit('ink'));
  t.position.set(plot.x, 2, plot.z);
  t.lookAt(t.position.clone().add(new THREE.Vector3(0.25, 0.87, 0.41)));
  root.add(t);
  return { root, params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

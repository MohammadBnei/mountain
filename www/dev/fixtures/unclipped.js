// Gate self-test: a terrain() patch (uClip) under a profile with clipped:false. Must FAIL (clipped flag).
export default async function build({ THREE, kit, plot }) {
  const g = new THREE.PlaneGeometry(3, 3, 6, 6).rotateX(-Math.PI / 2).translate(plot.x, 0.5, plot.z);
  g.setAttribute('bench', new THREE.BufferAttribute(new Float32Array(g.attributes.position.count), 1));
  const root = new THREE.Group();
  root.add(new THREE.Mesh(g.toNonIndexed(), kit.terrain()));
  return { root, params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

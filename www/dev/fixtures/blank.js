// Gate self-test: renders nothing. Must FAIL (coverage).
export default async function build({ THREE }) {
  return { root: new THREE.Group(), params: {}, pose() {}, step() {}, busy: () => false, dispose() {} };
}

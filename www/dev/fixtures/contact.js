// Gate self-test: a real (group 1) and a surface (group 2) body that can collide. Must FAIL (real↔surface).
import * as CANNON from 'cannon-es';
import cube from './cube.js';
export default async function build(ctx) {
  const w = new CANNON.World();
  w.addBody(new CANNON.Body({ mass: 0, shape: new CANNON.Plane(), collisionFilterGroup: 1 }));
  w.addBody(new CANNON.Body({ mass: 1, shape: new CANNON.Sphere(0.2), collisionFilterGroup: 2 }));
  return cube(ctx);
}

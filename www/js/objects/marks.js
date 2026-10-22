// Visitor plants (R7, relay toy_plant.go): world.state.marks of toy "plant", pose {h: 1..3, q, r, item}. What grew
// depends on the sun when the relay accepted it: 0 a tree, 1 a raspberry flag, 2 a lantern (lit at night).
// The town has no scenery trees, so every tree you see is a visitor's.
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import { plotXZ } from '../layout.js';

export default async function build({ THREE, kit, world }) {
  const root = new THREE.Group();
  const one = async (n) => mergeGeometries(await kit.geometry(n));
  const N = 512;
  const tree = kit.instanced(await one('tree_single_A'), kit.shared, N);
  const [pole, cloth] = kit.splitByUV(await one('flag_blue'), kit.inRoof);
  const flagPole = kit.instanced(pole.scale(2.6, 2.6, 2.6), kit.shared, N);
  const flagCloth = kit.instanced(cloth.scale(2.6, 2.6, 2.6), kit.palette.raspberry, N);
  const post = kit.instanced(mergeGeometries([new THREE.BoxGeometry(0.06, 0.62, 0.06).translate(0, 0.31, 0), new THREE.BoxGeometry(0.22, 0.04, 0.04).translate(0.08, 0.6, 0)]), kit.palette.wood, N);
  const lampMat = kit.variant('window');
  const lamp = kit.instanced(new THREE.BoxGeometry(0.14, 0.18, 0.14).translate(0.17, 0.48, 0), lampMat, N);
  const all = [tree, flagPole, flagCloth, post, lamp];
  for (const m of all) { m.frustumCulled = false; root.add(m); }

  const M = new THREE.Matrix4(), Q = new THREE.Quaternion(), E = new THREE.Euler(), P = new THREE.Vector3(), S = new THREE.Vector3();
  let dirty = true, lastNight = -1;
  world.on('state', () => { dirty = true; });
  world.on('metrics', () => { dirty = true; }); // hosts' Ready rides the metrics frame
  const hash = (s) => { let h = 0; for (const c of s) h = (h * 31 + c.charCodeAt(0)) | 0; return h; };
  return {
    root,
    update(night) {
      if (night !== lastNight) { lampMat.emissiveIntensity = 0.15 + 2.2 * night; lastNight = night; }
      if (!dirty) return false;
      dirty = false;
      for (const m of all) m.count = 0;
      for (const mk of world.state.marks) {
        if (mk.mark !== 'plant') continue;
        const { h, q, r, item } = mk.pose, host = `h${h}`;
        if (!world.town.islands.some((i) => i.host === host)) continue;
        const [x, z] = plotXZ(world.town, host, q, r), seed = hash(mk.id);
        // a little off the hex centre, so neighbours don't line up like a grid
        M.compose(P.set(x + ((seed & 7) - 3.5) * 0.08, world.ready[host] === false ? -0.5 : 0, z + (((seed >> 3) & 7) - 3.5) * 0.08), Q.setFromEuler(E.set(0, (seed % 628) / 100, 0)), S.setScalar(item === 0 ? 0.9 + ((seed >> 6) & 3) * 0.08 : 1));
        if (item === 0) tree.setMatrixAt(tree.count++, M);
        else if (item === 1) { flagPole.setMatrixAt(flagPole.count++, M); flagCloth.setMatrixAt(flagCloth.count++, M); }
        else { post.setMatrixAt(post.count++, M); lamp.setMatrixAt(lamp.count++, M); }
      }
      for (const m of all) m.instanceMatrix.needsUpdate = true;
      return true;
    },
  };
}

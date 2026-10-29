// L3, the building (plan/10 "The ladder", "Rendering rules"): one part in 3D, its walls carrying live HTML cards in
// CSS3D. At most 7 cards, at native pixel size; the CSS3D layer keeps pointer-events none and only the focused card
// takes the pointer, so cards never fight orbit, pinch and wheel. A card is focused by a click on its wall (an
// invisible stand-in mesh the raycaster sees) or from the panel's list; the camera then faces it. Below ~600 px the
// cards are not drawn here at all: the panel (the dock) carries them flat (iOS and many preserve-3d layers).
import * as THREE from 'three';
import { CSS3DObject } from 'three/addons/renderers/CSS3DRenderer.js';
import { cardSlots, MAX_CARDS, RING } from './geometry.js';
import { label, REALM } from './model.js';

const SURFACE = new THREE.Color('#25201b'),
  SEA = new THREE.Color('#1c1814');
const CARD_Y = 1.7,
  SCALE = 0.0085; // px → world: a 300 px card is ~2.5 units wide

/**
 * createBuilding: node id, its cards ([{key, title, el}], ≤ MAX_CARDS; el is a DOM element built with textContent),
 * flat = true on narrow screens (no CSS3D). Returns the scene, the CSS3D scene and the hooks the stage calls.
 */
export function createBuilding({ model, id, cards, flat }) {
  const n = model.nodes.get(id),
    realm = n?.realm,
    wire = new THREE.Color(REALM[realm]?.wire || '#9a8a72');
  const scene = new THREE.Scene(),
    css = new THREE.Scene();
  scene.background = SEA;
  scene.add(new THREE.HemisphereLight(0xfff2dd, 0x14110e, 1.4));
  const sun = new THREE.DirectionalLight(0xfff4e6, 1.8);
  sun.position.set(-5, 12, 9);
  scene.add(sun);
  const disposables = [];
  const keep = x => (disposables.push(x), x);
  const ground = new THREE.Mesh(
    keep(new THREE.CylinderGeometry(RING + 1.2, RING + 1.4, 0.3, 6)),
    keep(new THREE.MeshStandardMaterial({ color: SURFACE, roughness: 0.95 }))
  );
  ground.position.y = -0.15;
  ground.rotation.y = Math.PI / 6;
  const tower = new THREE.Mesh(
    keep(new THREE.BoxGeometry(1.6, 3.2, 1.6)),
    keep(
      new THREE.MeshStandardMaterial({
        color: SURFACE.clone().lerp(wire, 0.35),
        emissive: wire,
        emissiveIntensity: 0.12,
        roughness: 0.7
      })
    )
  );
  tower.position.y = 1.6;
  const trim = new THREE.Mesh(
    keep(new THREE.BoxGeometry(1.7, 0.12, 1.7)),
    keep(new THREE.MeshStandardMaterial({ color: wire, emissive: wire, emissiveIntensity: 0.6 }))
  );
  trim.position.y = 3.26;
  scene.add(ground, tower, trim);
  // its parts (nodes whose parent it is) as low annexes behind it
  const parts = [...model.nodes.values()].filter(x => x.parent === id).slice(0, 6);
  const annexGeo = keep(new THREE.BoxGeometry(0.7, 1, 0.7)),
    annexMat = keep(
      new THREE.MeshStandardMaterial({ color: SURFACE.clone().lerp(wire, 0.2), roughness: 0.8 })
    );
  const annexes = parts.map((x, i) => {
    const a = new THREE.Mesh(annexGeo, annexMat),
      ang = Math.PI * (1.25 + 0.5 * (i / Math.max(1, parts.length - 1)));
    a.position.set(1.9 * Math.cos(ang), 0.5, 1.9 * Math.sin(ang));
    a.userData = { part: x.id };
    scene.add(a);
    return a;
  });

  // the cards: CSS3D objects on a ring, each with an invisible stand-in the raycaster can hit
  const shown = cards.slice(0, MAX_CARDS),
    slots = cardSlots(shown.length);
  const standGeo = keep(new THREE.PlaneGeometry(1, 1)),
    standMat = keep(new THREE.MeshBasicMaterial({ visible: false, side: THREE.DoubleSide }));
  const items = shown.map((c, i) => {
    const sl = slots[i],
      stand = new THREE.Mesh(standGeo, standMat);
    stand.position.set(sl.x, CARD_Y, sl.z);
    stand.rotation.y = sl.ry;
    stand.scale.set(300 * SCALE, 220 * SCALE, 1);
    stand.userData = { card: c.key };
    scene.add(stand);
    let obj = null;
    if (!flat) {
      c.el.classList.add('card3d');
      obj = new CSS3DObject(c.el);
      obj.position.copy(stand.position);
      obj.rotation.copy(stand.rotation);
      obj.scale.setScalar(SCALE);
      obj.element.style.pointerEvents = 'none'; // only the focused card takes the pointer
      css.add(obj);
    }
    return { key: c.key, stand, obj, normal: new THREE.Vector3(sl.x, 0, sl.z).normalize() };
  });
  let focused = null;
  function focus(key) {
    focused = items.find(x => x.key === key) || null;
    for (const it of items) {
      if (!it.obj) continue;
      it.obj.element.style.pointerEvents = it === focused ? 'auto' : 'none';
      it.obj.element.classList.toggle('focused', it === focused);
    }
  }

  return {
    scene,
    css: flat ? null : css,
    aim() {
      if (focused) {
        // face the card, close enough to read it at about native size
        const t = focused.stand.position.clone();
        return { target: t, dist: 4.2, dir: focused.normal.clone().setY(0.18).normalize() };
      }
      return {
        target: new THREE.Vector3(0, 1.5, 0),
        dist: 11,
        dir: new THREE.Vector3(0, 0.38, 1).normalize()
      }; // low: walls, not roofs
    },
    pick(ray) {
      const hit = ray.intersectObjects([...items.map(x => x.stand), tower, ...annexes], false)[0];
      if (!hit) return { unfocus: true };
      if (hit.object.userData.card) return { card: hit.object.userData.card };
      if (hit.object.userData.part) return { part: hit.object.userData.part };
      return { unfocus: true };
    },
    focus,
    get focused() {
      return focused?.key || null;
    },
    /**
     * update puts a live frame's cards on the same walls, in place: the scene, the focus and the camera stay. False
     * when the set of cards changed (one more alert card, one fewer): the caller rebuilds the building then.
     */
    update(next) {
      next = next.slice(0, MAX_CARDS);
      if (next.length !== items.length || next.some((c, i) => c.key !== items[i].key)) return false;
      if (!flat) next.forEach((c, i) => items[i].obj.element.replaceChildren(...c.el.childNodes));
      return true;
    },
    /** where a card's wall stands (the gate clicks it) */
    standAt(key) {
      return items.find(x => x.key === key)?.stand.position.clone() || null;
    },
    paint() {},
    labels() {
      return [
        {
          key: `t:${id}`,
          text: label(n),
          sub: n?.kind,
          at: new THREE.Vector3(0, 3.7, 0),
          cls: 'tower'
        },
        ...annexes.map(a => ({
          key: `p:${a.userData.part}`,
          text: label(model.nodes.get(a.userData.part)),
          at: a.position.clone().setY(1.2),
          cls: 'bld'
        }))
      ];
    },
    frame() {},
    dispose() {
      for (const x of disposables) x.dispose();
      for (const it of items)
        if (it.obj) {
          css.remove(it.obj);
          it.obj.element.remove();
        }
    }
  };
}

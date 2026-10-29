#!/usr/bin/env node
// Import KayKit Medieval Hexagon models (CC0) into www/models/ the way the existing GLBs were made (R8): one .glb per
// model, no textures or images (kit.js paints every mesh with its own clay atlas through the model's UVs, and the
// pack's hexagons_medieval.png is the atlas clay.png was repainted from, so no repaint is needed), and
// KHR_mesh_quantization (int16 normalized positions and normals, uint16 normalized UVs).
//
//   GT=<dir with node_modules/@gltf-transform/{core,functions,extensions}@4.5.1> \
//   node www/dev/tools/import-kaykit.mjs <KayKit-Medieval-Hexagon-Pack-1.0 checkout> <name> [<name> …]
//
// <name> is the file stem in the pack (e.g. building_tower_B_blue, tent). Pinned to glTF-Transform 4.5.1, the
// generator of the existing models: `npm i --prefix "$GT" @gltf-transform/{core,functions,extensions}@4.5.1`.
import { readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const [pack, ...names] = process.argv.slice(2);
if (!pack || !names.length || !process.env.GT) {
  console.error(
    'usage: GT=<gltf-transform prefix> import-kaykit.mjs <pack checkout> <name> [<name> …]'
  );
  process.exit(2);
}
const mod = m =>
  import(
    pathToFileURL(join(process.env.GT, 'node_modules', '@gltf-transform', m, 'dist', 'index.js'))
      .href
  );
const { NodeIO } = await mod('core');
const { ALL_EXTENSIONS } = await mod('extensions');
const { prune, quantize, dedup, join: joinMeshes, flatten } = await mod('functions');

const root = join(resolve(pack), 'addons/kaykit_medieval_hexagon_pack/Assets/gltf');
const find = (dir, file) => {
  for (const e of readdirSync(dir)) {
    const p = join(dir, e);
    if (statSync(p).isDirectory()) {
      const f = find(p, file);
      if (f) return f;
    } else if (e === file) return p;
  }
  return null;
};
const out = resolve(new URL('../../models/', import.meta.url).pathname);
const io = new NodeIO().registerExtensions(ALL_EXTENSIONS);
for (const name of names) {
  const src = find(root, `${name}.gltf`);
  if (!src) {
    console.error(`${name}: not in the pack`);
    process.exitCode = 1;
    continue;
  }
  const doc = await io.read(src);
  for (const t of doc.getRoot().listTextures()) t.dispose(); // the atlas comes from kit.js (clay.png); UVs stay (keepAttributes)
  await doc.transform(
    dedup(),
    flatten(),
    joinMeshes(),
    prune({ keepAttributes: true }),
    quantize({ quantizePosition: 16, quantizeNormal: 16, quantizeTexcoord: 16 })
  );
  const dst = join(out, `${name}.glb`);
  await io.write(dst, doc);
  const tris = doc
    .getRoot()
    .listMeshes()
    .flatMap(m => m.listPrimitives())
    .reduce(
      (n, p) => n + (p.getIndices()?.getCount() ?? p.getAttribute('POSITION').getCount()) / 3,
      0
    );
  console.log(`${name}.glb  ${statSync(dst).size} B  ${tris} tris`);
}

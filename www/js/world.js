// World: fields, shared state and the bus (01 "World"). FROZEN W2; bodies may grow in W1a lane B
// behind THIS interface.
//
//   createWorld({dev}) → world
//   world.fields      {'h.base': Int16Array, 'h.land': Int16Array (HEIGHT_Q steps), heightSolid: Float32Array (units),
//                      'h.talus': Float32Array, temp, moisture (GRID²), ice: Float32Array (ICE²)}
//   world.bench       Uint8Array GRID² (0..255);  world.caves [{x, z, y, yaw, seed}];  world.hmax (units)
//   world.metrics world.state world.sites(Map app→site) world.wind world.snowline world.clouds world.sun
//   world.rig {elev, rising} world.clip world.stale   (shared state; writers in WRITERS)
//   world.heightAt(x, z) → units (bilinear on heightSolid);  world.cell(x, z) → index;  cellXZ(i, j)
//   world.on(type, fn) → off;  world.emit(type, payload, emitter);  world.write(slot, writer, valueOrFn)
//   world.as(name) → view whose write()/emit() are bound to name (what models get)
//   decodeI16(b64) / encodeI16(array) / decodeU8(b64): wire grids (little-endian, z outer, x inner)
//   world.applyHello(hello);  world.applyFrame(frame);  world.step(dt);  world.ready → Promise (first hello)
// Dev mode throws on a write by a non-writer and on an emit by a non-emitter or of an unknown bus type.
import U from './units.json' with { type: 'json' };

export const GRID = U.GRID, ICE = U.ICE, CELL = U.EXTENT / U.GRID, HQ = U.HEIGHT_Q;
export const cellXZ = (i, j) => [-U.EXTENT / 2 + i * CELL, -U.EXTENT / 2 + j * CELL];

// One writer per field / shared-state slot.
export const WRITERS = {
  'h.base': 'world', 'h.land': 'world', heightSolid: 'world', 'h.talus': 'scree', ice: 'glacier', temp: 'magma', moisture: 'sky',
  metrics: 'socket', state: 'socket', sites: 'world', wind: 'socket', snowline: 'socket', stale: 'socket',
  clouds: 'sky', sun: 'sky', rig: 'sky', clip: 'story',
};

// Bus registry: type → emitters (listeners are declared per profile in layout.js). Payloads per 01.
export const BUS = {
  'world:deposit': ['world'], 'world:vein': ['world'], 'world:glacier': ['world'], 'world:ash': ['world'], 'world:site': ['world'],
  'world:mark': ['world'], 'world:cloud': ['world'], 'world:rain': ['world'],
  'magma.dyke': ['magma'], 'lava.heat': ['eruption'], 'cauldron.sag': ['cauldron'], jokulhlaup: ['cauldron'],
  melt: ['glacier'], steam: ['glacier', 'cauldron'], 'ice.calve': ['glacier'], 'aquifer.seep': ['aquifers'],
};
export const EV_KINDS = ['argo.sync', 'argo.health', 'pod.churn', 'node.ready', 'quorum', 'alert.fire', 'alert.resolve', 'build', 'gitops.commit', 'adr.merge', 'replicas', 'app.gone'];
const emittersOf = (type) => (type.startsWith('ev:') ? (EV_KINDS.includes(type.slice(3)) ? ['socket', 'world'] : null) : BUS[type]);

const b64 = (s) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
/** int16 grid: base64 of little-endian int16, row-major (z outer, x inner). */
export const decodeI16 = (s) => { const b = b64(s); return new Int16Array(b.buffer, b.byteOffset, b.length >> 1).slice(); };
export const decodeU8 = b64;
export const encodeI16 = (a) => { const b = new Uint8Array(Int16Array.from(a).buffer); let s = ''; for (const x of b) s += String.fromCharCode(x); return btoa(s); };

export function createWorld({ dev = false } = {}) {
  const N = GRID * GRID, subs = new Map();
  let resolveReady;
  const pending = []; // deposits ramping into h.land over 3 s
  const w = {
    dev,
    fields: {
      'h.base': new Int16Array(N), 'h.land': new Int16Array(N), heightSolid: new Float32Array(N),
      'h.talus': new Float32Array(N), temp: new Float32Array(N), moisture: new Float32Array(N), ice: new Float32Array(ICE * ICE),
    },
    bench: new Uint8Array(N), caves: [], hmax: 0, day: null, seed: null,
    metrics: null, state: {}, sites: new Map(), wind: { x: 0, z: 0 }, snowline: 1e4, clouds: [], sun: null,
    rig: { elev: 0, rising: false }, // dusk until sky-full (W3)
    clip: { plane: [0, 1, 0, 1e4], depth: 0 }, stale: null,
    ready: new Promise((r) => (resolveReady = r)),

    cell(x, z) { const i = Math.round((x + 100) / CELL), j = Math.round((z + 100) / CELL); return Math.min(GRID - 1, Math.max(0, j)) * GRID + Math.min(GRID - 1, Math.max(0, i)); },
    heightAt(x, z) {
      const hs = w.fields.heightSolid, fx = Math.min(GRID - 1.001, Math.max(0, (x + 100) / CELL)), fz = Math.min(GRID - 1.001, Math.max(0, (z + 100) / CELL));
      const i = Math.floor(fx), j = Math.floor(fz), u = fx - i, v = fz - j, k = j * GRID + i;
      return (hs[k] * (1 - u) + hs[k + 1] * u) * (1 - v) + (hs[k + GRID] * (1 - u) + hs[k + GRID + 1] * u) * v;
    },

    on(type, fn) { if (!subs.has(type)) subs.set(type, new Set()); subs.get(type).add(fn); return () => subs.get(type).delete(fn); },
    emit(type, payload, emitter) {
      const em = emittersOf(type);
      if (dev && (!em || !em.includes(emitter))) throw new Error(`world: ${emitter} may not emit ${type}${em ? ` (emitters: ${em})` : ' (unknown type)'}`);
      for (const fn of subs.get(type) || []) fn(payload, type);
    },
    write(slot, writer, v) {
      if (dev && WRITERS[slot] !== writer) throw new Error(`world: ${writer} may not write ${slot} (writer: ${WRITERS[slot]})`);
      if (typeof v === 'function') v(slot in w.fields ? w.fields[slot] : w[slot]);
      else if (slot in w.fields) w.fields[slot].set(v);
      else w[slot] = v;
    },
    as(name) {
      const d = {
        write: { value: (slot, v) => w.write(slot, name, v) },
        emit: { value: (type, p) => w.emit(type, p, name) },
      };
      // Plain assignment on a view (`view.metrics = x`) would only shadow the slot; route it through write().
      for (const slot of Object.keys(WRITERS)) if (!(slot in w.fields)) d[slot] = { get: () => w[slot], set: (v) => w.write(slot, name, v) };
      return Object.create(w, d);
    },

    applyHello(h) {
      const s = h.snapshot;
      w.write('h.base', 'world', decodeI16(s.base));
      w.write('h.land', 'world', decodeI16(s.land));
      w.bench = decodeU8(s.bench);
      w.caves = s.caves; w.day = s.day; w.seed = s.seed;
      pending.length = 0;
      solid();
      w.hmax = w.fields.heightSolid.reduce((a, b) => Math.max(a, b), -1e9);
      w.write('state', 'socket', h.state || {});
      for (const d of h.deltas || []) w.applyFrame(d);
      if (h.metrics) w.applyFrame(h.metrics);
      resolveReady(w);
    },
    applyFrame(f) {
      if (f.t === 'hello') return w.applyHello(f);
      if (f.t === 'metrics') {
        w.write('metrics', 'socket', f);
        w.write('wind', 'socket', f.wind);
        w.write('snowline', 'socket', f.snowline);
        const st = Object.values(f.stale || {});
        w.write('stale', 'socket', st.length ? Math.min(...st) : null);
        return;
      }
      if (f.t === 'ev') {
        if (f.kind === 'app.gone') w.write('sites', 'world', (m) => m.delete(f.app));
        return w.emit(`ev:${f.kind}`, f, 'socket');
      }
      if (f.t === 'world') {
        const { t, kind, ...p } = f;
        if (kind === 'deposit') {
          const cells = decodeI16(p.cells);
          pending.push({ ...p, cells, done: new Int16Array(cells.length), age: 0 });
          return w.emit('world:deposit', { ...p, cells }, 'world');
        }
        if (kind === 'site') w.write('sites', 'world', (m) => m.set(p.app, p));
        return w.emit(`world:${kind}`, p, 'world');
      }
    },
    /** Advance the h.land deposit ramps (3 s each). */
    step(dt) {
      for (let n = pending.length - 1; n >= 0; n--) {
        const d = pending[n], k = Math.min(1, (d.age += dt) / 3), land = w.fields['h.land'], hs = w.fields.heightSolid, base = w.fields['h.base'];
        for (let z = 0; z < d.h; z++) for (let x = 0; x < d.w; x++) {
          const c = z * d.w + x, gx = d.x0 + x, gz = d.z0 + z;
          if (gx < 0 || gz < 0 || gx >= GRID || gz >= GRID) continue;
          const want = Math.round(d.cells[c] * k), g = gz * GRID + gx;
          land[g] += want - d.done[c]; d.done[c] = want;
          hs[g] = (base[g] + land[g]) * HQ;
          if (hs[g] > w.hmax) w.hmax = hs[g];
        }
        if (k >= 1) pending.splice(n, 1);
      }
    },
  };
  function solid() { const b = w.fields['h.base'], l = w.fields['h.land'], hs = w.fields.heightSolid; for (let i = 0; i < N; i++) hs[i] = (b[i] + l[i]) * HQ; }
  return w;
}

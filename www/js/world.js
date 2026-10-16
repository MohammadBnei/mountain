// World: fields, shared state and the bus (01 "World"). FROZEN W2 (FROZEN.sha256): interface, bus registry,
// writers, event enum and world.rig change only by an announced orchestrator change.
//
//   createWorld({dev}) → world
//   world.fields      {'h.base': Int16Array, 'h.land': Int16Array (HEIGHT_Q steps), heightSolid: Float32Array (units),
//                      'h.talus': Float32Array, temp, moisture (GRID²), ice: Float32Array (ICE²)}
//   world.bench       Uint8Array GRID² (0..255);  world.caves [{x, z, y, yaw, seed}];  world.hmax (units)
//   world.metrics world.state world.sites(Map app→site) world.wind world.snowline world.clouds world.sun world.moon (W5)
//   world.rig {elev, rising} world.clip world.stale   (shared state; writers in WRITERS)
//   world.state  hello.state, then kept current by the frames, each BEFORE its bus emit (a model reads it at build and on
//                a resync, so a reload keeps what live frames built; the relay's State.apply has the same rules):
//                alerts [{fp, sev, layer, startsAt (unix ms)}]: ev alert.fire adds (startsAt = Alertmanager's, from the ev),
//                alert.resolve removes (cauldron's sag survives a reload);
//                veins [world:vein payload] ≤ 32, oldest first, one per fp (a re-fire keeps a/b, the thicker wins);
//                glacier {mass, tongues:[{app, len}], ash:[day]}: the last world:glacier;
//                ash [{day, title?}] ≤ 32, newest day first, one per day + title (world:ash);
//                health {app: status} (ev argo.health), replicas {app: bucket} (ev replicas), builds {app: 'queued'|'running'}
//                (ev build; success/failure removes); ev app.gone drops the app from all three.
//                W4: marks [world:mark payload] ≤ 2000, oldest first (gone removes the id); clouds [world:cloud payload], one per
//                id (a frame replaces it, state 'gone' removes it), ≤ 256.
//                world.state is a NEW object after every hello or replay: a model that caches it resyncs on identity change
//   world.stale  null | unix ms: the oldest metrics.stale[source] since, or the socket drop time
//   world.clouds sky's clusters [{x, z, y (base, units), r, mass}]: mass = a towering cluster over the summit (lightning's
//                bolt source is the base of the first mass cluster); [] = none yet (lightning falls back above the site)
//   world.rig    {elev (deg), rising}: sky writes it; the host (main.js) applies it with kit.setPhase on change (a model
//                never touches the rig itself: the gate checks the rig is unchanged around step()). Dusk until W3
//   world.heightAt(x, z) → units (bilinear on heightSolid);  world.cell(x, z) → index;  cellXZ(i, j)
//   world.on(type, fn) → off;  world.emit(type, payload, emitter);  world.write(slot, writer, valueOrFn)
//   world.as(name) → view whose write()/emit() are bound to name (what models get)
//   decodeI16(b64) / encodeI16(array) / decodeU8(b64): wire grids (little-endian, z outer, x inner)
//   world.applyHello(hello);  world.applyFrame(frame, silent?) (silent: state only, no bus emit; hello deltas);  world.step(dt);  world.ready → Promise (first hello)
//   world.scrub(day 'YYYY-MM-DD') → Promise<replay frame> (rejects on {error}, with no socket, after 10 s without a reply,
//                or on a hello): sends scrub{day}; from then live frames are dropped (release or a hello resyncs). The replay rebuilds land + state from its snapshot,
//                then step() plays the day's frames, not silent, spread over ≤ 20 s.  world.replay null | {day, frames, metrics, left}
//   world.release() → leaves the replay and resyncs to live (world.resync: a fresh hello)
//   world.send(frame) / world.resync()   wired by socket.connect (absent without a socket)
//   world.place(mark, pose) → Promise<id> (W4): sends {t:'mark', ref, mark, pose, named: world.named}; resolves with the
//                mark id once the relay accepted it (the mark itself arrives as world:mark, to everyone), rejects with
//                Error(reason): off | auth | toy | rate | bad | a toy's reason | timeout (10 s) | offline.  world.named: the
//                toolbar's "sign my marks" (the host sets it).  world.quota null | {next (s), left, at (ms)}: the last
//                accept or rate refusal's budget (W5; set in answer(), the toolbar counts down from at + next).  world.unmark(id) → Promise<id>: admin mark.delete, same answers.
// Dev mode throws on a write by a non-writer and on an emit by a non-emitter or of an unknown bus type.
import U from './units.json' with { type: 'json' };

export const GRID = U.GRID, ICE = U.ICE, CELL = U.EXTENT / U.GRID, HQ = U.HEIGHT_Q;
export const cellXZ = (i, j) => [-U.EXTENT / 2 + i * CELL, -U.EXTENT / 2 + j * CELL];

// One writer per field / shared-state slot.
export const WRITERS = {
  'h.base': 'world', 'h.land': 'world', heightSolid: 'world', 'h.talus': 'scree', ice: 'glacier', temp: 'magma', moisture: 'sky',
  metrics: 'socket', state: 'socket', sites: 'world', wind: 'socket', snowline: 'socket', stale: 'socket',
  clouds: 'sky', sun: 'sky', moon: 'sky', rig: 'sky', clip: 'story',
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
  let play = null; // replay playback: {day, queue, gap, t, timer, done: [resolve, reject]}
  const acks = new Map(); let refN = 0; // W4 place/unmark: ref → [resolve, reject, timer]
  const w = {
    dev,
    fields: {
      'h.base': new Int16Array(N), 'h.land': new Int16Array(N), heightSolid: new Float32Array(N),
      'h.talus': new Float32Array(N), temp: new Float32Array(N), moisture: new Float32Array(N), ice: new Float32Array(ICE * ICE),
    },
    bench: new Uint8Array(N), caves: [], hmax: 0, day: null, seed: null,
    metrics: null, state: { alerts: [] }, sites: new Map(), wind: { x: 0, z: 0 }, snowline: 1e4, clouds: [], sun: null, moon: null,
    rig: { elev: 0, rising: false }, // dusk until sky-full (W3)
    clip: { plane: [0, 1, 0, 1e4], depth: 0 }, stale: null, replay: null, named: false, quota: null,
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
      // Deposit deltas land at full height, silently: they are history, not live lava (no world:deposit replay).
      for (const d of h.deltas || []) if (d.t === 'world' && d.kind === 'deposit') deposit(d, decodeI16(d.cells), 1);
      solid();
      w.write('state', 'socket', { alerts: [], veins: [], glacier: { mass: 0.5, tongues: [], ash: [] }, ash: [], health: {}, replicas: {}, builds: {}, marks: [], clouds: [], ...h.state });
      // Every other delta is history too: its state applies, nothing re-emits as a live event.
      for (const d of h.deltas || []) if (!(d.t === 'world' && d.kind === 'deposit')) apply(d, true);
      if (h.metrics) apply(h.metrics);
      resolveReady(w);
    },
    scrub(day) {
      if (!w.send) return Promise.reject(new Error('world: scrub needs a socket'));
      play?.done[1](new Error('world: superseded'));
      return new Promise((resolve, reject) => {
        const p = play = { day, queue: [], gap: 0, t: 0, done: [resolve, reject] };
        // no reply (lost frame, relay never answers): give up and resync to live
        p.timer = setTimeout(() => { if (play === p) { play = null; w.replay = null; reject(new Error('world: scrub timeout')); w.resync?.(); } }, 10000);
        w.replay = { day, frames: [], metrics: [], left: 0 };
        w.send({ t: 'scrub', day });
      });
    },
    place(mark, pose) { return ask({ t: 'mark', mark, pose, named: !!w.named }); },
    unmark(id) { return ask({ t: 'mark.delete', id }); },
    release() {
      const p = play;
      play = null; w.replay = null;
      p?.done[1](new Error('world: released'));
      w.resync?.();
    },
    // ponytail: live frames during a scrub are dropped, not buffered: release() resyncs through a fresh hello anyway.
    // A hello (reconnect) ends the scrub: the socket has resynced, so the world follows it.
    applyFrame(f, silent = false) {
      if (f.t === 'mark') return answer(f);
      if (f.t === 'replay') return replay(f);
      if (f.t === 'hello' && play) { const p = play; play = null; w.replay = null; clearTimeout(p.timer); p.done[1](new Error('world: resynced')); }
      if (!play) apply(f, silent);
    },
    /** Advance the h.land deposit ramps (3 s each) and the replay playback. */
    step(dt) {
      if (play?.queue.length && (play.t += dt) >= play.gap) {
        for (; play.t >= play.gap && play.queue.length; play.t -= play.gap) apply(play.queue.shift());
        w.replay.left = play.queue.length;
      }
      if (!pending.length) return;
      for (let n = pending.length - 1; n >= 0; n--) {
        const d = pending[n], k = Math.min(1, (d.age += dt) / 3);
        deposit(d, d.cells, k, d.done);
        if (k >= 1) pending.splice(n, 1);
      }
      hmax(); // ponytail: full 128² rescan per ramping step (negative cells can lower the max); cheap at GRID 128
    },
  };
  /** One server frame into the world (applyFrame minus the scrub gate). */
  function apply(f, silent = false) {
    const emit = silent ? () => {} : w.emit;
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
      if (f.kind === 'alert.fire' || f.kind === 'alert.resolve') w.write('state', 'socket', (s) => {
        s.alerts = (s.alerts || []).filter((a) => a.fp !== f.fp);
        if (f.kind === 'alert.fire') s.alerts.push({ fp: f.fp, sev: f.sev, layer: f.layer, startsAt: f.startsAt });
      });
      if (f.kind === 'argo.health') w.write('state', 'socket', (s) => { (s.health ||= {})[f.app] = f.status; });
      if (f.kind === 'replicas') w.write('state', 'socket', (s) => { (s.replicas ||= {})[f.app] = f.bucket; });
      if (f.kind === 'build') w.write('state', 'socket', (s) => { s.builds ||= {}; if (f.state === 'queued' || f.state === 'running') s.builds[f.app] = f.state; else delete s.builds[f.app]; });
      if (f.kind === 'app.gone') w.write('state', 'socket', (s) => { for (const k of ['health', 'replicas', 'builds']) if (s[k]) delete s[k][f.app]; });
      return emit(`ev:${f.kind}`, f, 'socket');
    }
    if (f.t === 'world') {
      const { t, kind, ...p } = f;
      if (kind === 'deposit') {
        const cells = decodeI16(p.cells);
        pending.push({ ...p, cells, done: new Int16Array(cells.length), age: 0 });
        return emit('world:deposit', { ...p, cells }, 'world');
      }
      if (kind === 'site') w.write('sites', 'world', (m) => m.set(p.app, p));
      if (kind === 'vein') w.write('state', 'socket', (s) => {
        const old = (s.veins ||= []).find((v) => v.fp === p.fp);
        s.veins = s.veins.filter((v) => v !== old).concat(old ? { ...p, a: old.a, b: old.b, thick: Math.max(p.thick, old.thick) } : p).slice(-32);
      });
      if (kind === 'glacier') w.write('state', 'socket', (s) => { s.glacier = p; });
      if (kind === 'mark') w.write('state', 'socket', (s) => {
        s.marks = (s.marks || []).filter((m) => m.id !== p.id);
        if (!p.gone) s.marks = s.marks.concat(p).slice(-2000);
      });
      if (kind === 'cloud') w.write('state', 'socket', (s) => {
        const l = (s.clouds || []).filter((c) => c.id !== p.id), i = (s.clouds || []).findIndex((c) => c.id === p.id);
        if (p.state !== 'gone') i < 0 ? l.push(p) : l.splice(i, 0, p);
        s.clouds = l.slice(-256);
      });
      if (kind === 'ash') w.write('state', 'socket', (s) => {
        if ((s.ash ||= []).some((a) => a.day === p.day && a.title === p.title)) return;
        s.ash = s.ash.concat(p).sort((a, b) => (a.day < b.day ? 1 : a.day > b.day ? -1 : 0)).slice(0, 32);
      });
      return emit(`world:${kind}`, p, 'world');
    }
  }
  /** W4: send a mark frame with a fresh ref; settles on the relay's {t:'mark', ref, id | error}. */
  function ask(f) {
    const ref = `r${(++refN).toString(36)}`;
    return new Promise((resolve, reject) => {
      if (!w.send?.({ ...f, ref })) return reject(new Error('offline'));
      acks.set(ref, [resolve, reject, setTimeout(() => { acks.delete(ref); reject(new Error('timeout')); }, 10000)]);
    });
  }
  function answer(f) {
    const a = acks.get(f.ref);
    if (!a) return;
    acks.delete(f.ref); clearTimeout(a[2]);
    if (f.next != null) w.quota = { next: f.next, left: f.left, at: Date.now() };
    if (f.error) a[1](new Error(f.error)); else a[0](f.id);
  }
  /** A replay frame: rebuild land + state from its snapshot, then step() plays its frames (ev/world, live, not silent). */
  function replay(f) {
    if (!play) return; // released before it came
    if (f.day ? f.day !== play.day : !f.error) return; // an earlier, superseded request's reply
    const [ok, no] = play.done;
    clearTimeout(play.timer);
    if (f.error) { play = null; w.release(); return no(new Error(`world: scrub ${f.error}`)); } // live frames were dropped: resync
    w.applyHello({ snapshot: f.snapshot, state: f.state });
    play.queue = f.frames.slice(); play.gap = Math.min(0.25, 20 / Math.max(1, f.frames.length));
    w.replay = { day: f.day, frames: f.frames, metrics: f.metrics, left: f.frames.length };
    ok(f);
  }
  /** Move d's cells to fraction k of their value in h.land and heightSolid (done = what is already applied). */
  function deposit(d, cells, k, done = new Int16Array(cells.length)) {
    const land = w.fields['h.land'], hs = w.fields.heightSolid, base = w.fields['h.base'];
    for (let z = 0; z < d.h; z++) for (let x = 0; x < d.w; x++) {
      const c = z * d.w + x, gx = d.x0 + x, gz = d.z0 + z;
      if (gx < 0 || gz < 0 || gx >= GRID || gz >= GRID) continue;
      const want = Math.round(cells[c] * k), g = gz * GRID + gx;
      land[g] += want - done[c]; done[c] = want;
      hs[g] = (base[g] + land[g]) * HQ;
    }
  }
  function hmax() { let m = -1e9; for (const v of w.fields.heightSolid) if (v > m) m = v; w.hmax = m; }
  function solid() { const b = w.fields['h.base'], l = w.fields['h.land'], hs = w.fields.heightSolid; for (let i = 0; i < N; i++) hs[i] = (b[i] + l[i]) * HQ; hmax(); }
  return w;
}

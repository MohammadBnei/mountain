#!/usr/bin/env node
// world.js + socket.js behaviour check (node >= 22, no deps). Exit 0 = pass.
//   node www/dev/core.test.mjs
import { readFileSync } from 'node:fs';
import { createWorld, encodeI16, GRID, HQ } from '../js/world.js';
import { connect } from '../js/socket.js';
import { sun } from '../js/sun.js';

let bad = 0;
const ok = (c, m) => { console.log(`${c ? 'ok  ' : 'BAD '} ${m}`); if (!c) bad++; };
const throws = (f) => { try { f(); return false; } catch { return true; } };
const hello = JSON.parse(readFileSync(new URL('./fixtures/hello.json', import.meta.url), 'utf8'));

// ---- world: h.base / h.land / heightSolid, 3 s ramp ----
const w = createWorld({ dev: true });
w.applyHello(hello);
const g = 64 * GRID + 64, base0 = w.fields['h.base'][g];
ok(Math.abs(w.fields.heightSolid[g] - base0 * HQ) < 1e-4, 'heightSolid = (h.base + h.land) × HEIGHT_Q after hello');
const seen = [];
w.on('world:deposit', (p) => seen.push(p));
w.applyFrame({ t: 'world', kind: 'deposit', id: 'd1', x0: 64, z0: 64, w: 1, h: 1, cells: encodeI16([1000]) });
ok(seen[0]?.cells instanceof Int16Array && seen[0].cells[0] === 1000, 'world:deposit emitted with decoded int16 cells');
ok(w.fields['h.land'][g] === 0, 'deposit does not land instantly (lava first)');
for (let i = 0; i < 45; i++) w.step(1 / 30);
ok(Math.abs(w.fields['h.land'][g] - 500) <= 1, `h.land at 1.5 s ≈ half (${w.fields['h.land'][g]})`);
for (let i = 0; i < 60; i++) w.step(1 / 30);
ok(w.fields['h.land'][g] === 1000 && Math.abs(w.fields.heightSolid[g] - (base0 + 1000) * HQ) < 1e-4, 'h.land complete at 3 s, heightSolid follows');
ok(w.hmax >= w.fields.heightSolid[g], 'hmax tracks deposits');
w.applyFrame({ t: 'world', kind: 'deposit', id: 'd2', x0: 64, z0: 64, w: 1, h: 1, cells: encodeI16([1000]) });
for (let i = 0; i < 30; i++) w.step(1 / 30);
w.applyHello(hello);
for (let i = 0; i < 120; i++) w.step(1 / 30);
ok(w.fields['h.land'][g] === 0, 'hello resync drops in-flight ramps (the hello land is authoritative)');
seen.length = 0;
w.applyHello({ ...hello, deltas: [{ t: 'world', kind: 'deposit', id: 'd3', x0: 64, z0: 64, w: 1, h: 1, cells: encodeI16([700]) }] });
ok(!seen.length, 'hello deposit deltas are silent (no world:deposit replay on reconnect)');
ok(w.fields['h.land'][g] === 700 && Math.abs(w.fields.heightSolid[g] - (base0 + 700) * HQ) < 1e-4, 'hello deposit deltas land at full height at once');
const top = w.fields.heightSolid.indexOf(w.hmax), tx = top % GRID, tz = (top - tx) / GRID;
w.applyFrame({ t: 'world', kind: 'deposit', id: 'd4', x0: tx, z0: tz, w: 1, h: 1, cells: encodeI16([-20000]) });
for (let i = 0; i < 100; i++) w.step(1 / 30);
ok(w.hmax === Math.max(...w.fields.heightSolid) && w.hmax < w.fields.heightSolid[top] + 20000 * HQ, `hmax drops when the top cell is dug out (${w.hmax})`);

// ---- world: one writer, registered emitters ----
const m = w.as('mountain');
ok(throws(() => m.write('metrics', {})), 'dev: a model may not write metrics');
ok(throws(() => { m.stale = 1; }), 'dev: plain assignment on a view goes through write() and throws');
ok(throws(() => m.emit('lava.heat', {})), 'dev: mountain may not emit lava.heat');
ok(throws(() => m.emit('nope', {})), 'dev: unknown bus type throws');
ok(!throws(() => w.as('eruption').emit('lava.heat', { x: 0, z: 0, q: 1 })), 'eruption may emit lava.heat');
const marks = [];
w.on('world:mark', (p) => marks.push(p));
w.applyFrame({ t: 'world', kind: 'mark', id: 'm1', mark: 'cairn', pose: { x: 1, z: 2 }, weight: 1 });
ok(marks[0]?.mark === 'cairn' && !('kind' in marks[0]) && !('t' in marks[0]), 'world:mark payload {id, mark, pose, weight}');
w.applyFrame({ t: 'world', kind: 'site', app: 'wird', x: 1, z: 2, y: 3, tier: 1, group: 'user' });
ok(w.sites.get('wird')?.x === 1, 'world:site sets world.sites');
w.applyFrame({ t: 'ev', id: 'e1', ts: 0, kind: 'app.gone', app: 'wird' });
ok(!w.sites.has('wird'), 'ev app.gone removes the site');

// ---- world: every hello delta applies silently (carry item 3); alerts in world.state ----
const w3 = createWorld({ dev: true }), live = [];
for (const k of ['world:site', 'world:mark', 'world:vein', 'ev:app.gone', 'ev:alert.fire']) w3.on(k, (p, type) => live.push(type));
const al = { fp: '0123456789abcdef', sev: 'critical', layer: 'k8s' };
w3.applyHello({ ...hello, state: { alerts: [{ ...al, startsAt: 5 }] }, deltas: [
  { t: 'world', kind: 'site', app: 'wird', x: 1, z: 2, y: 3, tier: 1, group: 'user' },
  { t: 'world', kind: 'site', app: 'gone-app', x: 0, z: 0, y: 0, tier: 0, group: 'user' },
  { t: 'world', kind: 'mark', id: 'm1', mark: 'cairn', pose: { x: 1, z: 2 }, weight: 1 },
  { t: 'ev', id: 'e1', ts: 0, kind: 'app.gone', app: 'gone-app' },
  { t: 'ev', id: 'e2', ts: 9, kind: 'alert.fire', fp: 'fedcba9876543210', sev: 'warning', layer: 'apps', startsAt: 7 },
] });
ok(!live.length, `hello deltas emit nothing (got ${live.join() || 'none'})`);
ok(w3.sites.get('wird')?.x === 1 && !w3.sites.has('gone-app'), 'hello deltas still apply their state (site set, app.gone removes)');
ok(w3.state.alerts.map((a) => `${a.fp}@${a.startsAt}`).join() === '0123456789abcdef@5,fedcba9876543210@7', 'hello.state alerts + a fire delta → world.state.alerts');
let seenAlerts;
w3.on('ev:alert.resolve', () => (seenAlerts = w3.state.alerts.length));
w3.applyFrame({ t: 'ev', id: 'e3', ts: 20, kind: 'alert.fire', ...al, startsAt: 5 });
ok(live.join() === 'ev:alert.fire' && w3.state.alerts.length === 2 && w3.state.alerts[1].startsAt === 5, 'live alert.fire emits once, replaces the same fp, keeps Alertmanager startsAt (not ev.ts)');
w3.applyFrame({ t: 'ev', id: 'e4', ts: 30, kind: 'alert.resolve', ...al, durS: 10 });
ok(seenAlerts === 1, 'alert.resolve removes the alert from world.state before ev:alert.resolve fires');
w3.applyHello({ ...hello, state: {} });
ok(Array.isArray(w3.state.alerts) && !w3.state.alerts.length, 'a hello without alerts → world.state.alerts = []');

// ---- world.state W3b: veins, glacier, ash, health, replicas, builds follow the frames (relay State.apply rules) ----
const w6 = createWorld({ dev: true }), order = [];
w6.applyHello({ ...hello, state: {}, deltas: [] });
ok(!w6.state.veins.length && w6.state.glacier.mass === 0.5 && !w6.state.ash.length && !Object.keys(w6.state.health).length, 'a hello without them → empty veins/ash/health, glacier at mass .5');
w6.on('world:vein', () => order.push(w6.state.veins.length));
const vein = (fp, thick, a = [0, 0, 0]) => ({ t: 'world', kind: 'vein', fp, a, b: [1, 1, 1], thick, layer: 'k8s', ref: false });
for (let i = 0; i < 34; i++) w6.applyFrame(vein(i.toString(16).padStart(16, '0'), 0.3));
w6.applyFrame(vein('0000000000000021', 0.2, [9, 9, 9]));
const last = w6.state.veins.at(-1);
ok(order[0] === 1 && w6.state.veins.length === 32 && w6.state.veins[0].fp === '0000000000000002' && last.thick === 0.3 && last.a[0] === 0,
  'world:vein: state before the emit, ≤ 32 oldest dropped, a re-fired fp keeps its seam and the thicker radius');
for (const d of ['2026-09-01', '2026-10-14', '2026-10-01', '2026-10-14']) w6.applyFrame({ t: 'world', kind: 'ash', day: d, ...(d === '2026-10-14' ? { title: 'ADR-0056' } : {}) });
ok(w6.state.ash.map((a) => a.day).join() === '2026-10-14,2026-10-01,2026-09-01' && w6.state.ash[0].title === 'ADR-0056', 'world:ash: newest first, one per day + title');
w6.applyFrame({ t: 'world', kind: 'glacier', mass: 0.6, tongues: [{ app: 'wird', len: 3 }], ash: ['2026-10-14'] });
for (const f of [{ kind: 'argo.health', app: 'wird', status: 'Degraded' }, { kind: 'replicas', app: 'wird', bucket: 3 }, { kind: 'build', app: 'wird', state: 'running' }, { kind: 'build', app: 'mountain', state: 'queued' }, { kind: 'build', app: 'mountain', state: 'success' }])
  w6.applyFrame({ t: 'ev', id: 'x', ts: 0, ...f });
ok(w6.state.glacier.tongues[0].len === 3 && w6.state.health.wird === 'Degraded' && w6.state.replicas.wird === 3 && w6.state.builds.wird === 'running' && !('mountain' in w6.state.builds),
  'world:glacier replaces; argo.health, replicas, build (success clears) land in world.state');
w6.applyFrame({ t: 'ev', id: 'y', ts: 0, kind: 'app.gone', app: 'wird' });
ok(!('wird' in w6.state.health) && !('wird' in w6.state.replicas) && !('wird' in w6.state.builds), 'app.gone drops the app from health, replicas, builds');
// parity with the relay: the fixture hello (dump-hello, before any snapshot) carries every world frame in its deltas,
// so folding them from an empty state must give the relay's own hello.state
const canon = (v) => JSON.stringify(v, (k, x) => (x && typeof x === 'object' && !Array.isArray(x) ? Object.fromEntries(Object.entries(x).sort()) : x));
const w7 = createWorld({ dev: true });
w7.applyHello({ ...hello, state: {} });
for (const k of ['veins', 'glacier', 'ash']) ok(canon(w7.state[k]) === canon(hello.state[k]), `hello deltas fold to the relay's hello.state.${k} (${hello.state[k].length ?? hello.state[k].tongues.length} items)`);

// ---- socket: reconnect with jittered backoff, resync via hello, stale since ----
const socks = [];
globalThis.WebSocket = class {
  constructor(url) { this.url = url; this.out = []; socks.push(this); setTimeout(() => { this.readyState = 1; this.onopen?.(); }, 1); }
  send(s) { this.out.push(s); }
  close() { this.readyState = 3; this.onclose?.(); }
  recv(f) { this.onmessage({ data: typeof f === 'string' ? f : JSON.stringify(f) }); }
};
const fetched = [];
let topo = { sites: [{ app: 'vos-monolith', x: -40.7, z: 1.4 }] }, gate = null;
globalThis.fetch = async (u, o) => { fetched.push(`${u} ${o?.cache}`); await gate; return { json: async () => topo }; };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const w2 = createWorld({ dev: true }), status = [];
Math.random = () => 0; // jitter factor 0.5
const c = connect({ url: 'ws://x/ws', world: w2, onStatus: (s) => status.push(s) });
await sleep(10);
socks[0].recv({ ...hello, t: 'hello' });
ok(w2.stale === null && status.join() === 'open', 'hello applied, not stale');
await sleep(1);
ok(fetched[0] === 'http://x/api/topology no-store' && w2.sites.get('vos-monolith')?.x === -40.7, 'hello fetches /api/topology (no-store) into world.sites');
socks[0].recv({ t: 'world', kind: 'site', app: 'wird', x: 3, z: 0, y: 0, tier: 0, group: 'user' });
let open_; gate = new Promise((r) => (open_ = r));
topo = { sites: [{ app: 'dream-analyst', x: 5 }, { app: 'editable-blog', x: 1 }] };
socks[0].recv({ ...hello, t: 'hello' }); // resync; its topology answer is held back...
socks[0].recv({ t: 'world', kind: 'site', app: 'editable-blog', x: 9, z: 0, y: 0, tier: 0, group: 'user' }); // ...behind a newer site
open_(); gate = null;
await sleep(1);
ok(!w2.sites.has('vos-monolith') && !w2.sites.has('wird') && w2.sites.get('dream-analyst')?.x === 5, 'resync topology replaces world.sites (gone apps drop out)');
ok(w2.sites.get('editable-blog')?.x === 9, 'a world:site after the hello wins over the older topology answer');
const t0 = Date.now();
socks[0].close();
ok(typeof w2.stale === 'number' && w2.stale >= t0 - 5, 'disconnect sets world.stale = since-timestamp');
const was = w2.stale;
await sleep(400);
ok(socks.length === 1, 'no reconnect before the backoff (≥ 500 ms)');
await sleep(500);
ok(socks.length === 2, 'reconnected after 1 s × jitter 0.5');
ok(w2.stale === was, 'stale since kept while not resynced');
socks[1].recv('not json'); // dropped with a warning, the socket keeps going
socks[1].close(); // no hello yet: backoff keeps growing (2 s × 0.5 = 1 s)
await sleep(700);
ok(socks.length === 2, 'backoff doubled when no hello came (accept-then-drop)');
await sleep(500);
ok(socks.length === 3, 'third attempt after 1 s');
socks[2].recv({ ...hello, t: 'hello' });
ok(w2.stale === null, 'hello resync clears stale');
ok(w2.send({ t: 'scrub', day: '2026-10-13' }) && socks[2].out[0] === '{"t":"scrub","day":"2026-10-13"}', 'world.send writes JSON to the open socket');
w2.resync();
ok(socks.length === 4 && socks[2].readyState === 3 && w2.stale === null, 'world.resync reconnects at once, without a stale gap');
socks[3].recv({ ...hello, t: 'hello' });
c.close();
await sleep(1200);
ok(socks.length === 4, 'close() stops reconnecting');

// ---- layout (frozen W2): every model's profile agrees with the world's writers and bus registry; page budget ----
{
  const { profiles } = await import('../js/layout.js'), { WRITERS, BUS, EV_KINDS } = await import('../js/world.js');
  const real = Object.entries(profiles).filter(([k]) => !k.startsWith('_')), bad = [];
  for (const [n, p] of real) {
    for (const s of p.writes) if (WRITERS[s] !== n) bad.push(`${n} writes ${s} (writer ${WRITERS[s]})`);
    for (const t of p.emits) if (!BUS[t]?.includes(n)) bad.push(`${n} emits ${t}`);
    for (const t of p.listens) if (!BUS[t] && !EV_KINDS.includes(t.slice(3))) bad.push(`${n} listens ${t}`);
  }
  for (const [s, wr] of Object.entries(WRITERS)) if (!['world', 'socket', 'story'].includes(wr) && !real.some(([n, p]) => n === wr && p.writes.includes(s))) bad.push(`no profile writes ${s}`);
  for (const [t, em] of Object.entries(BUS)) for (const e of em) if (e !== 'world' && !profiles[e]?.emits.includes(t)) bad.push(`${e} emits ${t} but its profile does not`);
  ok(!bad.length, `layout profiles ⊆ world WRITERS / BUS (${bad.join('; ') || 'all consistent'})`);
  const t = real.reduce((a, [, p]) => a + p.peak.tris, 0), d = real.reduce((a, [, p]) => a + p.peak.draws, 0);
  ok(t <= 200000 && d <= 220, `every profile at peak fits the page budget (${t} tris ≤ 200k, ${d} draws ≤ 220)`);
}

// ---- scrub: replay rebuilds land + state, plays the day's frames, drops live frames; release resyncs ----
const w4 = createWorld({ dev: true }), sent = [], heard = [];
let resyncs = 0;
w4.applyHello(hello);
ok(await w4.scrub('2026-10-13').then(() => false, (e) => /socket/.test(e.message)), 'scrub without a socket rejects');
w4.send = (f) => sent.push(f); w4.resync = () => resyncs++;
for (const k of ['world:deposit', 'ev:alert.fire']) w4.on(k, (p, type) => heard.push(type));
const al4 = { fp: '0123456789abcdef', sev: 'critical', layer: 'k8s', startsAt: 5 };
const replayF = { t: 'replay', day: '2026-10-13', snapshot: { ...hello.snapshot, land: encodeI16(Object.assign(new Int16Array(GRID * GRID), { [g]: 300 })) },
  state: { alerts: [] }, metrics: [], frames: [
    { t: 'world', kind: 'deposit', id: 'r1', x0: 64, z0: 64, w: 1, h: 1, cells: encodeI16([200]) },
    { t: 'ev', id: 'r2', ts: 1, kind: 'alert.fire', ...al4 }] };
const p4 = w4.scrub('2026-10-13');
ok(JSON.stringify(sent) === '[{"t":"scrub","day":"2026-10-13"}]', 'scrub sends {t:scrub, day}');
w4.applyFrame({ t: 'world', kind: 'deposit', id: 'live', x0: 64, z0: 64, w: 1, h: 1, cells: encodeI16([5000]) });
w4.applyFrame(replayF);
const got4 = await p4;
ok(got4 === replayF && w4.replay?.day === '2026-10-13' && w4.replay.left === 2, 'the replay resolves scrub() and sets world.replay');
ok(w4.fields['h.land'][g] === 300 && !heard.length, 'replay land = its snapshot (the live frame during the scrub was dropped)');
for (let i = 0; i < 3 * 30 + 30; i++) w4.step(1 / 30);
ok(w4.fields['h.land'][g] === 500 && w4.state.alerts.length === 1 && heard.join() === 'world:deposit,ev:alert.fire' && w4.replay.left === 0,
  `replay frames play through the bus, not silently (land ${w4.fields['h.land'][g]}, ${heard.join()})`);
w4.applyFrame({ t: 'ev', id: 'l2', ts: 2, kind: 'alert.resolve', ...al4, durS: 1 });
ok(w4.state.alerts.length === 1, 'live frames stay dropped while the replay is held');
w4.release();
ok(w4.replay === null && resyncs === 1, 'release() drops the replay and resyncs');
w4.applyFrame({ t: 'ev', id: 'l3', ts: 3, kind: 'alert.resolve', ...al4, durS: 1 });
ok(!w4.state.alerts.length, 'live frames apply again after release');
const p5 = w4.scrub('2026-01-01');
w4.applyFrame({ t: 'replay', error: 'bad-day' });
ok(await p5.then(() => false, (e) => /bad-day/.test(e.message)) && w4.replay === null && resyncs === 2, 'an error replay rejects scrub() and resyncs');

// ---- scripts/devpod.sh: the dev loop sees alerts (salted fp) and survives a push restart ----
const dp = readFileSync(new URL('../../scripts/devpod.sh', import.meta.url), 'utf8');
ok(/name: MOUNTAIN_FP_SALT, value: \S+/.test(dp), 'devpod: pod env sets MOUNTAIN_FP_SALT (else src_alerts drops every alert)');
ok(/while :; do \$K port-forward [^;]*\|\|/.test(dp), 'devpod: port-forward is looped and its failure tolerated under set -e (a push restart kills it)');

// ---- sun.js: the real Paris sun (sky-full writes world.rig from it) ----
const at = (iso) => sun(Date.parse(iso));
ok(Math.abs(at('2026-06-21T11:52:00Z').elev - 64.6) < 0.3 && Math.abs(at('2026-12-21T11:52:00Z').elev - 17.7) < 0.3, 'sun: solstice noon elevations over Paris (64.6°, 17.7°)');
ok(Math.abs(at('2026-03-20T06:00:00Z').elev) < 0.5 && at('2026-03-20T06:00:00Z').rising && Math.abs(at('2026-03-20T06:00:00Z').az - 90) < 2, 'sun: equinox sunrise ≈ 06:00 UTC, due east, rising');
ok(at('2026-10-15T17:00:00Z').elev < 0 && !at('2026-10-15T17:00:00Z').rising && at('2026-10-15T15:00:00Z').elev > 6, 'sun: an October dusk sets (not rising) after 15:00 UTC daylight');

console.log(bad ? `CORE FAIL (${bad})` : 'CORE PASS');
process.exit(bad ? 1 : 0);

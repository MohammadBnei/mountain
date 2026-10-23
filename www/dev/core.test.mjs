#!/usr/bin/env node
// world.js + socket.js + layout.js + sun.js behaviour check (node >= 22, no deps). Exit 0 = pass.
//   node www/dev/core.test.mjs
import { readFileSync } from 'node:fs';
import { createWorld } from '../js/world.js';
import { connect, gateLapsed } from '../js/socket.js';
import { sun, moon } from '../js/sun.js';

let bad = 0;
const ok = (c, m) => { console.log(`${c ? 'ok  ' : 'BAD '} ${m}`); if (!c) bad++; };
const hello = JSON.parse(readFileSync(new URL('./fixtures/hello.json', import.meta.url), 'utf8'));
const canon = (v) => JSON.stringify(v, (k, x) => (x && typeof x === 'object' && !Array.isArray(x) ? Object.fromEntries(Object.entries(x).sort()) : x));

// ---- world: the hello is the town (Go's State, town geometry); frames fold with the relay's State.apply rules ----
{
  const w = createWorld(), heard = [];
  for (const k of ['state', 'hello', 'world:plot', 'world:mark', 'ev:alert.fire', 'ev:node.ready']) w.on(k, (p, type) => heard.push(type));
  w.applyHello(hello);
  ok(canon(w.state) === canon(hello.state) && w.town === hello.town, 'hello: world.state is the relay\'s hello.state (dump-hello folds the deltas), world.town its geometry');
  ok(w.state.plots.length === 10 && w.state.plots.find((p) => p.app === 'editable-blog').ruinAt > 0, 'plots from the hello, ruins kept');
  ok(w.state.machines.witness === 'down' && w.state.machines.mill === 'down', 'R8: machine states from the hello (the last ev machine wins)');
  w.applyFrame({ t: 'ev', id: '01JA00000000000000000000M3', ts: 1, kind: 'machine', app: 'mill', state: 'up' });
  w.applyFrame({ t: 'ev', id: '01JA00000000000000000000M4', ts: 1, kind: 'app.gone', app: 'witness' });
  ok(w.state.machines.mill === 'up' && !('witness' in w.state.machines), 'R8: ev machine folds; app.gone clears the machine state (relay State.apply)');
  w.applyFrame({ t: 'world', kind: 'plot', app: 'wird', host: 'h2', q: 1, r: 2, role: 'house' });
  ok(w.state.plots.filter((p) => p.app === 'wird').map((p) => `${p.host}${p.q}${p.r}`).join() === 'h212', 'world:plot replaces the app\'s plot (a move)');
  w.applyFrame({ t: 'world', kind: 'plot', app: 'wird', host: 'h2', q: 1, r: 2, role: 'house', gone: true });
  ok(!w.state.plots.some((p) => p.app === 'wird') && !w.state.plots.some((p) => 'gone' in p), 'gone removes it; no plot keeps a gone flag');
  ok(w.state.annexes.map((x) => `${x.app}@${x.host}`).join() === 'wird@h2,wird@h3,gate@h1', 'R8 C: tents from the hello (a gone one dropped)');
  w.applyFrame({ t: 'world', kind: 'annex', app: 'wird', host: 'h2', q: 3, r: -4 });
  w.applyFrame({ t: 'world', kind: 'annex', app: 'gate', host: 'h1', q: 2, r: -6, gone: true });
  ok(w.state.annexes.map((x) => `${x.app}@${x.host}:${x.q}`).join() === 'wird@h3:2,wird@h2:3' && !w.state.annexes.some((x) => 'gone' in x), 'R8 C: world:annex replaces by app@host, gone removes it (relay State.apply)');
  for (let i = 0; i < 60; i++) w.applyFrame({ t: 'world', kind: 'annex', app: `a${i}`, host: 'h1', q: 0, r: 0 });
  ok(w.state.annexes.length === 48 && w.state.annexes[47].app === 'a59', 'R8 C: the last 48 tents kept');
  w.applyFrame({ t: 'ev', kind: 'alert.fire', fp: 'aaaaaaaaaaaaaaaa', sev: 'warning', layer: 'apps', startsAt: 3, app: 'gate', host: 'h2' });
  w.applyFrame({ t: 'ev', kind: 'alert.fire', fp: 'aaaaaaaaaaaaaaaa', sev: 'critical', layer: 'apps', startsAt: 3 });
  ok(w.state.alerts.filter((a) => a.fp === 'aaaaaaaaaaaaaaaa').map((a) => `${a.sev}${a.app ?? '-'}`).join() === 'critical-', 'a re-fire replaces the alert (one per fp), host/app follow the latest');
  w.applyFrame({ t: 'ev', kind: 'alert.resolve', fp: 'aaaaaaaaaaaaaaaa', sev: 'critical', layer: 'apps', durS: 9 });
  ok(!w.state.alerts.some((a) => a.fp === 'aaaaaaaaaaaaaaaa'), 'resolve removes it');
  w.applyFrame({ t: 'ev', kind: 'build', app: 'wird', state: 'success' });
  w.applyFrame({ t: 'ev', kind: 'argo.health', app: 'vos-monolith', status: 'Progressing' });
  w.applyFrame({ t: 'ev', kind: 'app.gone', app: 'dream-analyst' });
  ok(!w.state.builds.wird && w.state.health['vos-monolith'] === 'Progressing' && !('dream-analyst' in w.state.health), 'build success clears it, health follows, app.gone drops the app');
  w.applyFrame({ t: 'ev', kind: 'node.ready', host: 'h3', ready: false });
  ok(w.ready.h3 === false, 'node.ready → world.ready');
  w.applyFrame({ ...hello.metrics, hosts: { h1: { ready: true }, h2: { ready: true }, h3: { ready: true } }, stale: { prom: 1000, alerts: 500 } });
  ok(w.ready.h3 === true && w.stale === 500, 'metrics: hosts\' Ready and the oldest stale source');
  for (let i = 0; i < 2001; i++) w.applyFrame({ t: 'world', kind: 'mark', id: `m${i}`, mark: 'plant', pose: { h: 1, q: 0, r: 4, item: 0 }, weight: 1 });
  ok(w.state.marks.length === 2000 && w.state.marks[0].id !== hello.state.marks[0]?.id, 'marks keep the newest 2000 (the relay cap)');
  w.applyFrame({ t: 'world', kind: 'mark', id: 'm2000', mark: 'plant', pose: {}, weight: 0, gone: true });
  ok(!w.state.marks.some((m) => m.id === 'm2000'), 'a gone mark leaves state.marks');
  ok(heard.includes('world:plot') && heard.includes('ev:alert.fire') && heard.includes('ev:node.ready') && heard.filter((t) => t === 'state').length > 5, 'every frame emits its type, then state');
}

// ---- socket: reconnect with jittered backoff, resync via hello, stale since ----
const socks = [];
globalThis.WebSocket = class {
  constructor(url) { this.url = url; this.out = []; socks.push(this); setTimeout(() => { this.readyState = 1; this.onopen?.(); }, 1); }
  send(s) { this.out.push(s); }
  close() { this.readyState = 3; this.onclose?.(); }
  recv(f) { this.onmessage({ data: typeof f === 'string' ? f : JSON.stringify(f) }); }
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
{
  const w2 = createWorld(), status = [];
  const rnd = Math.random;
  Math.random = () => 0; // jitter factor 0.5
  const c = connect({ url: 'ws://x/ws', world: w2, onStatus: (s) => status.push(s) });
  await sleep(10);
  socks[0].recv(hello);
  ok(w2.stale === null && status.join() === 'open' && w2.state.plots.length === 10, 'hello applied, not stale');
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
  socks[2].recv(hello);
  ok(w2.stale === null, 'hello resync clears stale (its metrics have none)');
  ok(w2.send({ t: 'scrub', day: '2026-10-13' }) && socks[2].out[0] === '{"t":"scrub","day":"2026-10-13"}', 'world.send writes JSON to the open socket');
  w2.resync();
  ok(socks.length === 4 && socks[2].readyState === 3, 'world.resync reconnects at once');
  socks[3].recv(hello);
  c.close();
  await sleep(1200);
  ok(socks.length === 4, 'close() stops reconnecting');
  Math.random = rnd;
}

// ---- the gate (R8 deploy C1): a lapsed forwardAuth session → one reload; anything else → none ----
{
  const store = new Map(), st = { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, v) };
  let reloads = 0;
  const reload = () => reloads++;
  const answer = (r) => async () => r;
  ok(await gateLapsed({ fetchFn: answer({ type: 'opaqueredirect', status: 0 }), reload, store: st, now: 1e9 }) && reloads === 1, 'gate: a redirect reloads once');
  ok(!(await gateLapsed({ fetchFn: answer({ type: 'opaqueredirect', status: 0 }), reload, store: st, now: 1e9 + 60e3 })) && reloads === 1, 'gate: a second redirect within 5 min does not reload again');
  ok(await gateLapsed({ fetchFn: answer({ type: 'basic', status: 401 }), reload, store: st, now: 1e9 + 6 * 60e3 }) && reloads === 2, 'gate: a 401 counts, after the 5 min');
  for (const r of [{ type: 'basic', status: 502 }, { type: 'basic', status: 429 }, { type: 'basic', status: 200 }]) {
    ok(!(await gateLapsed({ fetchFn: answer(r), reload, store: { getItem: () => null, setItem() {} }, now: 2e9 })), `gate: ${r.status} does not reload`);
  }
  ok(!(await gateLapsed({ fetchFn: async () => { throw new Error('offline'); }, reload, store: st, now: 3e9 })) && reloads === 2, 'gate: a network error does not reload');
  // the socket asks once per run of failures, at the second close without a hello
  const w5 = createWorld(), rnd = Math.random;
  Math.random = () => 0;
  let probes = 0;
  const n0 = socks.length;
  const c = connect({ url: 'ws://x/ws', world: w5, probe: () => probes++ });
  await sleep(10);
  socks[n0].recv(hello);
  socks[n0].close();
  ok(probes === 0, 'probe: not at the first close');
  await sleep(600);
  socks[n0 + 1].close();
  ok(probes === 1, 'probe: at the second close without a hello');
  await sleep(1100);
  socks[n0 + 2].close();
  ok(probes === 1, 'probe: once per run of failures');
  await sleep(2100);
  socks[n0 + 3].recv(hello);
  socks[n0 + 3].close();
  await sleep(600);
  socks[n0 + 4].close();
  ok(probes === 2, 'probe: a hello starts a new run');
  c.close();
  Math.random = rnd;
}

// ---- scrub: the replay sets the day's starting town, step() plays its frames, live frames wait; release resyncs ----
{
  const w4 = createWorld();
  w4.applyHello(hello);
  ok(await w4.scrub('2026-10-21').then(() => false, (e) => /offline/.test(e.message)), 'scrub without a socket rejects');
  const sent = []; let resyncs = 0;
  w4.send = (f) => (sent.push(f), true);
  w4.resync = () => { resyncs++; };
  const p = w4.scrub('2026-10-21');
  ok(JSON.stringify(sent) === '[{"t":"scrub","day":"2026-10-21"}]', 'scrub sends {t:scrub, day}');
  const plot = { t: 'world', kind: 'plot', app: 'wird', host: 'h1', q: -2, r: 1, role: 'house' };
  const fire = { t: 'ev', kind: 'alert.fire', fp: 'bbbbbbbbbbbbbbbb', sev: 'info', layer: 'apps', startsAt: 1 };
  w4.applyFrame({ t: 'replay', day: '2026-10-21', snapshot: hello.snapshot, state: { alerts: [], health: {}, replicas: {}, builds: {}, marks: [], plots: [] }, frames: [plot, fire], metrics: [] });
  const r = await p;
  ok(r.day === '2026-10-21' && w4.replay?.left === 2 && !w4.state.plots.length, 'the replay resolves scrub() with the day\'s empty start');
  ok(Array.isArray(w4.state.annexes) && !w4.state.annexes.length, 'R8 C: a replay of a day from before the tents has annexes []');
  w4.applyFrame({ t: 'world', kind: 'plot', app: 'gate', host: 'h2', q: 0, r: -1, role: 'gate' });
  ok(!w4.state.plots.some((x) => x.app === 'gate'), 'a live frame during the replay is dropped');
  w4.step(1);
  ok(w4.state.plots.length === 1 && w4.state.alerts.length === 1 && w4.replay.left === 0, 'step() plays the day\'s frames');
  w4.release();
  ok(w4.replay === null && resyncs === 1, 'release() drops the replay and resyncs');
  const p5 = w4.scrub('2020-01-01');
  w4.applyFrame({ t: 'replay', error: 'bad-day' });
  ok(await p5.then(() => false, (e) => /bad-day/.test(e.message)) && w4.replay === null && resyncs === 2, 'an error replay rejects scrub() and resyncs');
}

// ---- visitor marks: place() and unmark() settle on the relay's ack by ref ----
{
  const w5 = createWorld();
  w5.applyHello(hello);
  ok(await w5.place('plant', {}).then(() => false, (e) => e.message === 'offline'), 'place() without a socket rejects offline');
  const out = [];
  w5.send = (f) => (out.push(f), true);
  w5.named = true;
  const pa = w5.place('plant', { h: 1, q: 4, r: -2 }), pb = w5.place('plant', { h: 1, q: 0, r: 0 });
  ok(out[0].t === 'mark' && out[0].mark === 'plant' && out[0].named === true && out[0].ref !== out[1].ref, 'place() sends {t:mark, ref, mark, pose, named}');
  w5.applyFrame({ t: 'mark', ref: out[1].ref, error: 'road' });
  w5.applyFrame({ t: 'mark', ref: out[0].ref, id: 'X1', next: 60, left: 19 });
  ok(await pa === 'X1' && await pb.then(() => false, (e) => e.message === 'road') && w5.quota?.left === 19, 'acks settle by ref: id resolves (with the quota), error rejects');
  const pd = w5.unmark('X1');
  ok(out[2].t === 'mark.delete' && out[2].id === 'X1', 'unmark() sends mark.delete');
  w5.applyFrame({ t: 'mark', ref: out[2].ref, error: 'auth' });
  ok(await pd.then(() => false, (e) => e.message === 'auth'), 'a refused delete rejects');
}

// ---- layout: the page reads the relay's tile map the way Go wrote it (relay/town Hexes order, pi-lab hex grid) ----
{
  const { hexes, hexXZ, worldToHex, ring, islandsOf } = await import('../js/layout.js');
  const hx = hexes(4);
  ok(hx.length === 61 && hx[0].join() === '0,-4' && hx[60].join() === '0,4', 'hexes(): r ascending, then q (relay town.Hexes)');
  ok(hello.town.islands.map((i) => `${i.host}:${i.radius}:${i.tiles.length}`).join() === 'h1:7:169,h2:5:91,h3:4:61', 'R8: island radii follow the hosts (7/5/4)');
  const isl = islandsOf(hello.town);
  ok(hello.town.bridges.every((b) => isl.find((i) => i.host === b.a).tileAt(b.aq, b.ar).kind === 'b' && isl.find((i) => i.host === b.b).tileAt(b.bq, b.br).kind === 'b'), 'every bridge head is a "b" tile');
  ok(isl.every((i) => i.tileAt(0, 0).kind === 's' && i.tiles.filter((t) => t.kind === 'g').length === 3 * (i.radius - 1) * i.radius - 2 * (i.radius - 1)), 'each island: the square at its centre, its rings 1..r−1 grass but the 2 spokes');
  ok(hx.every(([q, r]) => worldToHex(...hexXZ(q + 7, r - 3)).join() === `${q + 7},${r - 3}`), 'worldToHex inverts hexXZ (picking a tile)');
  ok(ring(-4, 4) === 4 && ring(2, -1) === 2, 'ring() is the hex distance');
}

// ---- scripts/devpod.sh: the dev loop sees alerts (salted fp) and survives a push restart ----
const dp = readFileSync(new URL('../../scripts/devpod.sh', import.meta.url), 'utf8');
ok(/name: MOUNTAIN_FP_SALT, value: \S+/.test(dp), 'devpod: pod env sets MOUNTAIN_FP_SALT (else src_alerts drops every alert)');
ok(/while :; do \$K port-forward [^;]*\|\|/.test(dp), 'devpod: port-forward is looped and its failure tolerated under set -e (a push restart kills it)');

// ---- R9 Civ: the calm fold mirrors relay State.apply (civ_test.go TestCalm); the golden age and turns (civ.js) ----
{
  const { goldenAge, turnOf, GOLDEN_DAYS } = await import('../js/civ.js');
  const w = createWorld();
  w.applyHello({ ...hello, state: { ...hello.state, alerts: [], health: {}, calm: '' } });
  const at = (d) => Date.UTC(2026, 9, d, 12);
  const f = (x) => w.applyFrame({ t: 'ev', id: `c${Math.random()}`, ...x });
  f({ kind: 'argo.health', app: 'wird', status: 'Healthy', ts: at(20) });
  ok(w.state.calm === '2026-10-20', 'calm: a calm frame starts it on its UTC day');
  f({ kind: 'alert.fire', fp: '0000000000000001', sev: 'info', layer: 'apps', startsAt: 1, ts: at(21) });
  f({ kind: 'argo.health', app: 'wird', status: 'Progressing', ts: at(21) });
  ok(w.state.calm === '2026-10-20', 'calm: info alerts and Progressing do not trouble the town');
  f({ kind: 'argo.health', app: 'wird', status: 'Missing', ts: at(22) });
  ok(w.state.calm === '', 'calm: a Missing app ends it');
  f({ kind: 'app.gone', app: 'wird', ts: at(24) });
  ok(w.state.calm === '2026-10-24', 'calm: the troubled app gone ends the trouble');
  const day = (iso) => ({ state: { calm: '2026-10-10' }, replay: { day: iso } });
  ok(!goldenAge(day('2026-10-17')).on && goldenAge(day('2026-10-18')).on && goldenAge(day('2026-10-18')).days === 1,
    `golden age after ${GOLDEN_DAYS} whole calm days (calm's own day never counts), its own day 1 first`);
  ok(turnOf({ replay: { day: '2026-10-20' } }, '2026-10-20') === 1 && turnOf({ replay: { day: '2026-10-23' } }, '2026-10-20') === 4 && turnOf({}, null) === null, 'turns: day one = 1; no start day, no turn');
}

// ---- sun.js: the real Paris sun (the sky, and the plant toy's preview; relay/sun.go is pinned to the same vectors) ----
const at = (iso) => sun(Date.parse(iso));
ok(Math.abs(at('2026-06-21T11:52:00Z').elev - 64.6) < 0.3 && Math.abs(at('2026-12-21T11:52:00Z').elev - 17.7) < 0.3, 'sun: solstice noon elevations over Paris (64.6°, 17.7°)');
ok(Math.abs(at('2026-03-20T06:00:00Z').elev) < 0.5 && at('2026-03-20T06:00:00Z').rising && Math.abs(at('2026-03-20T06:00:00Z').az - 90) < 2, 'sun: equinox sunrise ≈ 06:00 UTC, due east, rising');
ok(Math.abs(at('2026-10-22T16:30:00Z').elev - 2.143272) < 1e-5, 'sun: the vector relay/sun_test.go pins (2026-10-22T16:30Z, 2.143272°)');
{ // SunCalc's own test vector (2013-03-05 UTC, 50.5 N 30.5 E)
  const m = moon(Date.UTC(2013, 2, 5), 50.5, 30.5);
  ok(Math.abs(m.az - (180 - 0.9784 / (Math.PI / 180))) < 0.5 && Math.abs(m.elev - 0.3) < 0.6 && Math.abs(m.frac - 0.4848) < 0.01, `moon: SunCalc test vector (az ${m.az.toFixed(2)}, elev ${m.elev.toFixed(2)}, frac ${m.frac.toFixed(3)})`);
}

console.log(bad ? `CORE FAIL (${bad})` : 'CORE PASS');
process.exit(bad ? 1 : 0);

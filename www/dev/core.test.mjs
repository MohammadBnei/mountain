#!/usr/bin/env node
// world.js + socket.js behaviour check (node >= 22, no deps). Exit 0 = pass.
//   node www/dev/core.test.mjs
import { readFileSync } from 'node:fs';
import { createWorld, encodeI16, GRID, HQ } from '../js/world.js';
import { connect } from '../js/socket.js';

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

// ---- socket: reconnect with jittered backoff, resync via hello, stale since ----
const socks = [];
globalThis.WebSocket = class {
  constructor(url) { this.url = url; socks.push(this); setTimeout(() => this.onopen?.(), 1); }
  close() { this.onclose?.(); }
  recv(f) { this.onmessage({ data: typeof f === 'string' ? f : JSON.stringify(f) }); }
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const w2 = createWorld({ dev: true }), status = [];
Math.random = () => 0; // jitter factor 0.5
const c = connect({ url: 'ws://x/ws', world: w2, onStatus: (s) => status.push(s) });
await sleep(10);
socks[0].recv({ ...hello, t: 'hello' });
ok(w2.stale === null && status.join() === 'open', 'hello applied, not stale');
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
c.close();
await sleep(1200);
ok(socks.length === 3, 'close() stops reconnecting');

console.log(bad ? `CORE FAIL (${bad})` : 'CORE PASS');
process.exit(bad ? 1 : 0);

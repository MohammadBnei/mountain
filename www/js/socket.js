// Socket: the only writer of world.metrics/state/wind/snowline/stale (01). Orchestrator-owned interface;
// body: W1a lane B (stale label, resync details) behind the SAME exports.
//
//   wsURL(loc = location) → 'wss://<host>/ws' (ws:// on http)
//   connect({ url, world, onStatus }) → { close() }
//     every server frame goes to world.applyFrame; reconnects with jittered backoff (1 s → 30 s) and
//     resyncs through the hello the relay sends on connect. onStatus('open'|'closed').
//     Wires world.send(frame) (false while not open) and world.resync() (drop this connection, reconnect at once:
//     the new hello is the resync; world.release uses it).
//
// Body (lane B): backoff resets only once a hello has resynced the world (a relay that accepts then drops
// never gets a 1 s storm). While disconnected world.stale = the since-timestamp of the drop; the hello's
// metrics frame recomputes it on resync. A frame that is not JSON is dropped with a warning.

export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

export function connect({ url = wsURL(), world, onStatus = () => {} }) {
  let ws, closed = false, backoff = 1000, timer, fresh = new Set(); // fresh: apps with a world:site since the last hello
  world.send = (f) => ws?.readyState === 1 && (ws.send(JSON.stringify(f)), true);
  world.resync = () => { if (closed) return; clearTimeout(timer); ws.onclose = ws.onmessage = null; ws.close(); open(); };
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => onStatus('open');
    ws.onmessage = (e) => {
      let f;
      try { f = JSON.parse(e.data); } catch { return console.warn('socket: dropped a non-JSON frame'); }
      world.applyFrame(f);
      if (f.t === 'world' && f.kind === 'site') fresh.add(f.app);
      if (f.t !== 'hello') return;
      backoff = 1000;
      // world.sites = /api/topology + world:site deltas (01): apps with no deposit yet still get a site.
      // A resync REPLACES the sites (gone apps drop out), except those a world:site set after this hello:
      // the answer may be older than them. A newer hello's answer supersedes this one.
      const mine = fresh = new Set();
      fetch(new URL('/api/topology', url.replace(/^ws/, 'http')), { cache: 'no-store' }).then((r) => r.json())
        .then((t) => mine === fresh && world.write('sites', 'world', (m) => {
          for (const k of [...m.keys()]) if (!mine.has(k)) m.delete(k);
          for (const x of t.sites || []) if (!mine.has(x.app)) m.set(x.app, x);
        }))
        .catch((e) => console.warn('socket: /api/topology', e.message));
    };
    ws.onclose = () => {
      onStatus('closed');
      if (closed) return;
      if (world.stale == null) world.write('stale', 'socket', Date.now());
      timer = setTimeout(open, backoff * (0.5 + Math.random()));
      backoff = Math.min(30000, backoff * 2);
    };
  };
  open();
  // a page kept in the back/forward cache would hold its socket (and one of the 4 per IP, 06): drop it on
  // pagehide, reconnect when the page comes back (the hello resyncs)
  globalThis.addEventListener?.('pagehide', (e) => { if (e.persisted && !closed) { clearTimeout(timer); ws.onclose = null; ws.close(); } });
  globalThis.addEventListener?.('pageshow', (e) => { if (e.persisted && !closed) open(); });
  return { close() { closed = true; clearTimeout(timer); ws?.close(); } };
}

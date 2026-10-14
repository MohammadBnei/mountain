// Socket: the only writer of world.metrics/state/wind/snowline/stale (01). Orchestrator-owned interface;
// body: W1a lane B (stale label, resync details) behind the SAME exports.
//
//   wsURL(loc = location) → 'wss://<host>/ws' (ws:// on http)
//   connect({ url, world, onStatus }) → { close() }
//     every server frame goes to world.applyFrame; reconnects with jittered backoff (1 s → 30 s) and
//     resyncs through the hello the relay sends on connect. onStatus('open'|'closed').
//
// Body (lane B): backoff resets only once a hello has resynced the world (a relay that accepts then drops
// never gets a 1 s storm). While disconnected world.stale = the since-timestamp of the drop; the hello's
// metrics frame recomputes it on resync. A frame that is not JSON is dropped with a warning.

export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

export function connect({ url = wsURL(), world, onStatus = () => {} }) {
  let ws, closed = false, backoff = 1000, timer;
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => onStatus('open');
    ws.onmessage = (e) => {
      let f;
      try { f = JSON.parse(e.data); } catch { return console.warn('socket: dropped a non-JSON frame'); }
      world.applyFrame(f);
      if (f.t === 'hello') backoff = 1000;
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
  return { close() { closed = true; clearTimeout(timer); ws?.close(); } };
}

// Socket: the only writer of world.metrics/state/wind/snowline/stale (01). Orchestrator-owned interface;
// body: W1a lane B (stale label, resync details) behind the SAME exports.
//
//   wsURL(loc = location) → 'wss://<host>/ws' (ws:// on http)
//   connect({ url, world, onStatus }) → { close() }
//     every server frame goes to world.applyFrame; reconnects with jittered backoff (1 s → 30 s) and
//     resyncs through the hello the relay sends on connect. onStatus('open'|'closed').

export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

export function connect({ url = wsURL(), world, onStatus = () => {} }) {
  let ws, closed = false, backoff = 1000, timer;
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => { backoff = 1000; onStatus('open'); };
    ws.onmessage = (e) => world.applyFrame(JSON.parse(e.data));
    ws.onclose = () => {
      onStatus('closed');
      if (closed) return;
      timer = setTimeout(open, backoff * (0.5 + Math.random()));
      backoff = Math.min(30000, backoff * 2);
    };
  };
  open();
  return { close() { closed = true; clearTimeout(timer); ws?.close(); } };
}

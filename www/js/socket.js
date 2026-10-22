// Socket: the relay's /ws into the world (from mountain www/js/socket.js, minus /api/topology: plots ride the hello).
//   connect({url, world, onStatus}) → {close()}: every frame to world.applyFrame; jittered backoff 1 s → 30 s, reset
//   by a hello; while down world.stale = the drop time. Wires world.send(frame) and world.resync().
export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

export function connect({ url = wsURL(), world, onStatus = () => {} }) {
  let ws, closed = false, backoff = 1000, timer;
  world.send = (f) => ws?.readyState === 1 && (ws.send(JSON.stringify(f)), true);
  world.resync = () => { if (closed) return; clearTimeout(timer); ws.onclose = ws.onmessage = null; ws.close(); open(); };
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
      if (world.stale == null) { world.stale = Date.now(); world.emit('state', world.state); }
      timer = setTimeout(open, backoff * (0.5 + Math.random()));
      backoff = Math.min(30000, backoff * 2);
    };
  };
  open();
  // the back/forward cache would hold the socket (one of 4 per IP): drop it on pagehide, reconnect on pageshow
  globalThis.addEventListener?.('pagehide', (e) => { if (e.persisted && !closed) { clearTimeout(timer); ws.onclose = null; ws.close(); } });
  globalThis.addEventListener?.('pageshow', (e) => { if (e.persisted && !closed) open(); });
  return { close() { closed = true; clearTimeout(timer); ws?.close(); } };
}

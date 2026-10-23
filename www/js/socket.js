// Socket: the relay's /ws into the world (from mountain www/js/socket.js, minus /api/topology: plots ride the hello).
//   connect({url, world, onStatus, probe?}) → {close()}: every frame to world.applyFrame; jittered backoff 1 s → 30 s,
//   reset by a hello; while down world.stale = the drop time. Wires world.send(frame) and world.resync(). probe: called
//   once per run of failures, at the second close without a hello (gateLapsed, below).
export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

// gateLapsed (R8 deploy C1, infra-bootstrap ADR-0065): the site sits behind a forwardAuth gate whose session can lapse
// under an open tab; every request then answers a redirect to the login wall, which a socket only sees as a failed
// upgrade. /auth/me fetched without following redirects tells: an opaque redirect (or a 401) is the gate, and one
// top-level reload lands on the wall. A network error, a 5xx or a 429 says nothing about the gate: nothing happens.
// At most one reload per 5 min (sessionStorage), so a gate that keeps refusing never loops. → true if it reloaded.
export async function gateLapsed({ fetchFn = globalThis.fetch, reload = () => location.reload(), store = globalThis.sessionStorage, now = Date.now() } = {}) {
  let r;
  try { r = await fetchFn('/auth/me', { redirect: 'manual', cache: 'no-store' }); } catch { return false; }
  if (r.type !== 'opaqueredirect' && r.status !== 401) return false;
  try {
    if (now - Number(store?.getItem('mountain.gate') || 0) < 5 * 60e3) return false;
    store?.setItem('mountain.gate', String(now));
  } catch { /* storage off: still one reload, the gate's own page ends the loop */ }
  reload();
  return true;
}

export function connect({ url = wsURL(), world, onStatus = () => {}, probe = null }) {
  let ws, closed = false, backoff = 1000, timer, fails = 0;
  world.send = (f) => ws?.readyState === 1 && (ws.send(JSON.stringify(f)), true);
  world.resync = () => { if (closed) return; clearTimeout(timer); ws.onclose = ws.onmessage = null; ws.close(); open(); };
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => onStatus('open');
    ws.onmessage = (e) => {
      let f;
      try { f = JSON.parse(e.data); } catch { return console.warn('socket: dropped a non-JSON frame'); }
      world.applyFrame(f);
      if (f.t === 'hello') { backoff = 1000; fails = 0; }
    };
    ws.onclose = () => {
      onStatus('closed');
      if (closed) return;
      if (world.stale == null) { world.stale = Date.now(); world.emit('state', world.state); }
      if (++fails === 2) probe?.();
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

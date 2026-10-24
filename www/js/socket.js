// /ws: the live overlay. connect({onFrame, onStatus, probe}) → {close()}: every frame to onFrame; jittered backoff
// 1 s → 30 s, reset by a hello. probe: called once per run of failures, at the second close without a hello.
export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

// gateLapsed: the site sits behind a forwardAuth gate (infra-bootstrap ADR-0065) whose session can lapse under an
// open tab; every request then answers a redirect to the login wall, which a socket only sees as a failed upgrade.
// /auth/me fetched without following redirects tells: an opaque redirect (or a 401) is the gate, and one top-level
// reload lands on the wall. Anything else says nothing about the gate. At most one reload per 5 min.
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

export function connect({ url = wsURL(), onFrame, onStatus = () => {}, probe = null }) {
  let ws, closed = false, backoff = 1000, timer, fails = 0;
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => onStatus('open');
    ws.onmessage = (e) => {
      let f;
      try { f = JSON.parse(e.data); } catch { return console.warn('socket: dropped a non-JSON frame'); }
      if (f.t === 'hello') { backoff = 1000; fails = 0; }
      onFrame(f);
    };
    ws.onclose = () => {
      onStatus('closed');
      if (closed) return;
      if (++fails === 2) probe?.();
      timer = setTimeout(open, backoff * (0.5 + Math.random()));
      backoff = Math.min(30000, backoff * 2);
    };
  };
  open();
  // the back/forward cache would hold the socket: drop it on pagehide, reconnect on pageshow
  globalThis.addEventListener?.('pagehide', (e) => { if (e.persisted && !closed) { clearTimeout(timer); ws.onclose = null; ws.close(); } });
  globalThis.addEventListener?.('pageshow', (e) => { if (e.persisted && !closed) open(); });
  return { close() { closed = true; clearTimeout(timer); ws?.close(); } };
}

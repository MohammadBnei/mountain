// /ws: the live overlay. connect({onFrame, onStatus, probe}) → {close()}: every frame to onFrame; jittered backoff
// 1 s → 30 s, reset by a hello. probe: called once per run of failures, at the second close without a hello.
export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

// gateLapsed: a sign-in has lapsed under an open tab, and a socket only sees that as a failed upgrade. Two can:
//   - the forwardAuth gate's (infra-bootstrap ADR-0065): every request answers a redirect to the login wall, so
//     /auth/me fetched without following redirects is an opaque redirect (or a 401);
//   - atlas's own 1 h session: the socket closes at its end and /ws answers 403, while /auth/me (an open route)
//     answers {enabled: true, admin: false}.
// Either way one top-level reload signs in again (the page redirects to the sign-in, which is one round trip while
// authentik's session holds). Anything else says nothing about either. At most one reload per 5 min.
export async function gateLapsed({ fetchFn = globalThis.fetch, reload = () => location.reload(), store = globalThis.sessionStorage, now = Date.now() } = {}) {
  let r;
  try { r = await fetchFn('/auth/me', { redirect: 'manual', cache: 'no-store' }); } catch { return false; }
  let lapsed = r.type === 'opaqueredirect' || r.status === 401;
  if (!lapsed && r.ok) { try { const me = await r.json(); lapsed = me.enabled && !me.admin; } catch { /* not JSON: says nothing */ } }
  if (!lapsed) return false;
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
      if (++fails % 6 === 2) probe?.(); // the 2nd failure, then every 6th: a lapsed session is found again after the 5 min guard
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

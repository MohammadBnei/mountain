// /ws: the live overlay. connect({onFrame, onStatus, probe}) → {close()}: every frame to onFrame; jittered backoff
// 1 s → 30 s, reset by a hello. probe: called at the second close without a hello, then every sixth. A sign-out
// ends it for good, onStatus('signed-out') and no reconnect, since a reload would sign straight back in through
// authentik: a close with SIGNED_OUT (the session's own socket, open at the sign-out, maybe in another tab), or a
// probe answering 'signed-out' (a socket that was down then, or a page restored from the back/forward cache).
export const SIGNED_OUT = 4001; // cmd/atlas/hub.go closeSignedOut
export const wsURL = (loc = location) => `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws`;

// gateLapsed: a sign-in has lapsed under an open tab, and a socket only sees that as a failed upgrade. Two can:
//   - the forwardAuth gate's (infra-bootstrap ADR-0065): every request answers a redirect to the login wall, so
//     /auth/me fetched without following redirects is an opaque redirect (or a 401);
//   - atlas's own 1 h session: the socket closes at its end and /ws answers 403, while /auth/me (an open route)
//     answers {enabled: true, admin: false}.
// Either way one top-level reload signs in again (the page redirects to the sign-in, which is one round trip while
// authentik's session holds). Anything else says nothing about either. At most one reload per 5 min. A session
// ended by a sign-out looks the same, so /auth/me says signedOut then (cmd/atlas/auth.go logout): no reload, the
// answer is 'signed-out'.
// signedOut: /auth/me says this browser signed out (and holds no session since): the page stops instead of reloading.
export async function signedOut({ fetchFn = globalThis.fetch } = {}) {
  try {
    const r = await fetchFn('/auth/me', { redirect: 'manual', cache: 'no-store' });
    const me = r.ok ? await r.json() : null;
    return me?.signedOut === true && !me.admin;
  } catch { return false; }
}

export async function gateLapsed({ fetchFn = globalThis.fetch, reload = () => location.reload(), store = globalThis.sessionStorage, now = Date.now() } = {}) {
  let r;
  try { r = await fetchFn('/auth/me', { redirect: 'manual', cache: 'no-store' }); } catch { return false; }
  let lapsed = r.type === 'opaqueredirect' || r.status === 401;
  if (!lapsed && r.ok) {
    let me;
    try { me = await r.json(); } catch { /* not JSON: says nothing */ }
    if (me?.signedOut === true && !me.admin) return 'signed-out';
    lapsed = Boolean(me?.enabled && !me.admin);
  }
  if (!lapsed) return false;
  try {
    if (now - Number(store?.getItem('mountain.gate') || 0) < 5 * 60e3) return false;
    store?.setItem('mountain.gate', String(now));
  } catch { /* storage off: still one reload, the gate's own page ends the loop */ }
  reload();
  return true;
}

export function connect({ url = wsURL(), onFrame, onStatus = () => {}, probe = null }) {
  let ws, closed = false, out = false, backoff = 1000, timer, fails = 0;
  const signedOut = () => { out = closed = true; clearTimeout(timer); ws?.close(); onStatus('signed-out'); };
  const open = () => {
    ws = new WebSocket(url);
    ws.onopen = () => onStatus('open');
    ws.onmessage = (e) => {
      let f;
      try { f = JSON.parse(e.data); } catch { return console.warn('socket: dropped a non-JSON frame'); }
      if (f.t === 'hello') { backoff = 1000; fails = 0; }
      onFrame(f);
    };
    ws.onclose = (e) => {
      if (out) return; // a socket still opening when a probe found the sign-out: its close says nothing new
      if (!closed && e?.code === SIGNED_OUT) return signedOut();
      onStatus('closed');
      if (closed) return;
      if (++fails % 6 === 2) { // the 2nd failure, then every 6th: a lapsed session is found again after the 5 min guard
        Promise.resolve(probe?.()).then((r) => { if (r === 'signed-out' && !closed) signedOut(); });
      }
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

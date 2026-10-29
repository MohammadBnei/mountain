// Shared by gate.mjs and perf.mjs: a static www/ server and a headless Chrome (swiftshader) driven over raw CDP.
// Node >= 22, no dependencies. Ported from pi-lab compose/www/dev/gate.mjs.
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, extname, join, normalize, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

export const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..'); // www/
export const CHROME =
  process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
export const sleep = ms => new Promise(r => setTimeout(r, ms));

const MIME = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.mjs': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png'
};
/** Static server for www/ on 127.0.0.1:<random>. → {origin, close()} */
export async function serveStatic(root = ROOT) {
  const server = createServer(async (req, res) => {
    let path;
    try {
      path = normalize(decodeURIComponent(new URL(req.url, 'http://x').pathname));
    } catch {
      res.writeHead(400).end();
      return;
    }
    const file = join(root, path.endsWith('/') ? path + 'index.html' : path);
    if (!file.startsWith(root + sep) || !existsSync(file)) {
      res.writeHead(404).end();
      return;
    }
    res
      .writeHead(200, {
        'content-type': MIME[extname(file)] || 'application/octet-stream',
        'cache-control': 'no-store'
      })
      .end(await readFile(file));
  });
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  return { origin: `http://127.0.0.1:${server.address().port}`, close: () => server.close() };
}

/**
 * Headless Chrome with its own profile and debugging port (parallel runs never collide), one page attached.
 * args: extra Chrome flags (e.g. the fake microphone).
 * → { s(method, params) (page session), on(fn({method, params})), evalJSON(expr), close() }
 */
export async function launch({ w = 1280, h = 800, mobile = false, dpr = 1, args = [] } = {}) {
  const profileDir = await mkdtemp(join(tmpdir(), 'mountain-chrome-'));
  const chrome = spawn(
    CHROME,
    [
      '--headless=new',
      '--remote-debugging-port=0',
      `--user-data-dir=${profileDir}`,
      '--no-first-run',
      '--no-default-browser-check',
      '--hide-scrollbars',
      '--mute-audio',
      '--use-angle=swiftshader',
      '--enable-unsafe-swiftshader',
      `--window-size=${w},${h}`,
      ...args,
      'about:blank'
    ],
    { stdio: ['ignore', 'ignore', 'pipe'] }
  );
  // stderr is drained (a full pipe would stall Chrome) and its tail kept, to say why a start failed
  let errTail = '',
    exited = null;
  chrome.stderr.on('data', b => {
    errTail = (errTail + b).slice(-2000);
  });
  chrome.on('exit', (code, sig) => {
    exited = sig || code;
  });
  let ws;
  const close = async () => {
    try {
      ws?.close();
    } catch {}
    chrome.kill('SIGKILL');
    await rm(profileDir, { recursive: true, force: true }).catch(() => {});
  };
  let port;
  // up to 30 s: the first start on a fresh CI runner can take more than 10 (an exit ends the wait at once)
  for (let i = 0; i < 300 && !port && exited === null; i++) {
    const f = join(profileDir, 'DevToolsActivePort');
    if (existsSync(f)) port = (await readFile(f, 'utf8')).split('\n')[0].trim();
    if (!port) await sleep(100);
  }
  if (!port) {
    const why = exited === null ? 'no debugging port after 30 s' : `exited ${exited}`;
    await close();
    throw new Error(`Chrome did not start (${CHROME}: ${why}; set CHROME)\n${errTail.trim()}`);
  }
  const { webSocketDebuggerUrl } = await (
    await fetch(`http://127.0.0.1:${port}/json/version`)
  ).json();
  ws = new WebSocket(webSocketDebuggerUrl);
  await new Promise((r, j) => {
    ws.onopen = r;
    ws.onerror = j;
  });
  let nextId = 1;
  const pending = new Map(),
    listeners = [];
  ws.onmessage = ev => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      const { res, rej } = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? rej(new Error(msg.error.message)) : res(msg.result);
    } else if (msg.method) listeners.forEach(l => l(msg));
  };
  const send = (method, params = {}, sessionId) =>
    new Promise((res, rej) => {
      const id = nextId++;
      pending.set(id, { res, rej });
      ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  const s = (m, p) => send(m, p, sessionId);
  await s('Emulation.setDeviceMetricsOverride', {
    width: w,
    height: h,
    deviceScaleFactor: dpr,
    mobile
  });
  const evalJSON = async expr => {
    const r = await s('Runtime.evaluate', {
      expression: `(async () => JSON.stringify(await (${expr})))()`,
      awaitPromise: true,
      returnByValue: true
    });
    if (r.exceptionDetails)
      throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
    return r.result.value ? JSON.parse(r.result.value) : null;
  };
  return { s, on: fn => listeners.push(m => m.sessionId === sessionId && fn(m)), evalJSON, close };
}

/** Wait until evalJSON(expr) is truthy (polls every 100 ms) → value or null on timeout. */
export async function waitFor(page, expr, ms = 30000) {
  for (const t0 = Date.now(); Date.now() - t0 < ms; await sleep(100)) {
    const v = await page.evalJSON(expr).catch(() => null);
    if (v) return v;
  }
  return null;
}

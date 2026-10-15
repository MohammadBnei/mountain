#!/usr/bin/env node
// Perf probe (04): Android throttle profile (393×873, DPR 2.75, mobile UA, touch, CPU ×4) on the live page,
// rAF frame intervals over 8 s plus per-frame WebGL draw calls / triangles counted at the context (any page,
// no hooks). Thresholds exit non-zero. Headless GPU is SwiftShader (software): frame times are pessimistic and
// GPU-bound, so the default frame threshold is a regression guard, not device fps. Ported from pi-lab dev/perf.mjs.
//
//   node www/dev/perf.mjs [url|/path]   http(s) URL as is; a /path is served from www/ (default /index.html?fixture=1#live: the finale camera the budgets were set on)
// Options: --cpu 4  --secs 8  --max-p95 <ms, default 400>  --max-draws 220  --max-tris 200000  --desktop
//          --cut <depth> (opens the cut, main.js dev knob: the cap mesh's fill cost)
// Env: CHROME, DPR. gl_FragDepth: none since W3a (cut.js chose the cap mesh over the back-face cap; numbers in cut.js).
import { launch, serveStatic, sleep } from './cdp.mjs';

const argv = process.argv.slice(2);
const opt = (n, d) => { const i = argv.indexOf(`--${n}`); return i >= 0 ? argv[i + 1] : d; };
const desktop = argv.includes('--desktop');
const lim = { p95: Number(opt('max-p95', 400)), draws: Number(opt('max-draws', 220)), tris: Number(opt('max-tris', 200000)) };
const arg = argv[0]?.startsWith('--') ? null : argv[0], srv = arg?.startsWith('http') ? null : await serveStatic();
let url = srv ? `${srv.origin}${arg || '/index.html?fixture=1#live'}` : arg;
if (opt('cut')) { const u = new URL(url); u.searchParams.set('cut', opt('cut')); url = u.href; } // before the #hash

const page = await launch(desktop ? { w: 1280, h: 800 } : { w: 393, h: 873, mobile: true, dpr: Number(process.env.DPR || 2.75) });
if (!desktop) {
  await page.s('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 });
  await page.s('Emulation.setUserAgentOverride', { userAgent: 'Mozilla/5.0 (Linux; Android 13; Redmi 12C) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Mobile Safari/537.36' });
}
await page.s('Page.enable');
await page.s('Runtime.enable');
const errors = [];
page.on(({ method, params }) => {
  if (method === 'Runtime.exceptionThrown') errors.push(params.exceptionDetails.exception?.description || params.exceptionDetails.text);
  if (method === 'Runtime.consoleAPICalled' && params.type === 'error') errors.push(params.args.map((a) => a.value ?? a.description).join(' '));
});
// Count draws/triangles at the WebGL2 context; our rAF (registered first) closes each frame.
await page.s('Page.addScriptToEvaluateOnNewDocument', { source: `(() => {
  const P = WebGL2RenderingContext.prototype, cur = { d: 0, t: 0 }, out = window.__perf = { frames: [], dts: [], on: false };
  const tri = (mode, n, k = 1) => (mode === 4 ? (n / 3) * k : mode === 5 || mode === 6 ? Math.max(0, n - 2) * k : 0);
  const wrap = (name, f) => { const o = P[name]; P[name] = function (...a) { cur.d++; cur.t += f(a); return o.apply(this, a); }; };
  wrap('drawArrays', (a) => tri(a[0], a[2])); wrap('drawElements', (a) => tri(a[0], a[1]));
  wrap('drawArraysInstanced', (a) => tri(a[0], a[2], a[3])); wrap('drawElementsInstanced', (a) => tri(a[0], a[1], a[4]));
  let last = 0;
  const tick = (now) => { if (out.on) { out.frames.push([cur.d, cur.t]); out.dts.push(now - last); } last = now; cur.d = cur.t = 0; requestAnimationFrame(tick); };
  requestAnimationFrame(tick);
})();` });
await page.s('Page.navigate', { url });
let ready = false;
for (let i = 0; i < 300 && !ready; i++) { await sleep(100); ready = await page.evalJSON('window.__perf && performance.now() > 1000 && document.readyState === "complete"').catch(() => false); }
await sleep(3000); // models build, shaders compile
const quality = await page.evalJSON('globalThis.__quality || null').catch(() => null);
await page.s('Emulation.setCPUThrottlingRate', { rate: Number(opt('cpu', desktop ? 1 : 4)) });
await page.evalJSON('(window.__perf.on = true)');
await sleep(Number(opt('secs', 8)) * 1000);
const r = await page.evalJSON('(window.__perf.on = false, window.__perf)');
await page.close();
srv?.close();

const dts = r.dts.slice(2).sort((a, b) => a - b), pct = (p) => dts[Math.min(dts.length - 1, Math.floor(dts.length * p))];
const draws = Math.max(0, ...r.frames.map((f) => f[0])), tris = Math.max(0, ...r.frames.map((f) => f[1]));
const fails = [];
if (dts.length < 5) fails.push(`only ${dts.length} frames`);
if (!draws) fails.push('no draw calls counted (page not rendering?)');
if (pct(0.95) > lim.p95) fails.push(`frame p95 ${pct(0.95).toFixed(1)} ms > ${lim.p95}`);
if (draws > lim.draws) fails.push(`draws ${draws} > ${lim.draws}`);
if (tris > lim.tris) fails.push(`tris ${tris} > ${lim.tris}`);
errors.forEach((e) => fails.push(`page error: ${String(e).slice(0, 200)}`));
console.log(`perf ${desktop ? 'desktop' : `android cpu ×${opt('cpu', 4)}`} q=${quality}: frames=${dts.length} p50=${pct(0.5)?.toFixed(1)}ms p95=${pct(0.95)?.toFixed(1)}ms max=${dts.at(-1)?.toFixed(0)}ms | peak draws=${draws} tris=${Math.round(tris)} | cut=${opt('cut') || 'closed'}`);
if (fails.length) { console.log('PERF FAIL'); fails.forEach((f) => console.log('  - ' + f)); process.exit(1); }
console.log('PERF PASS');
process.exit(0);

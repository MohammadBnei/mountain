// Boot: picks the quality tier before kit loads, checks WebGL2, then loads main.js.
// ?q=low|high overrides. Low = phones / ≤ 4 cores: Lambert, 96² terrain, no post.
const q = new URLSearchParams(location.search).get('q');
globalThis.__quality = q || (matchMedia('(pointer: coarse)').matches || (navigator.hardwareConcurrency || 8) <= 4 ? 'low' : 'high');

const fallback = (why, text) => {
  const p = document.getElementById('fallback');
  if (text) p.textContent = text;
  p.hidden = false;
  console.warn('mountain: fallback', why);
};
// A failed load (a deploy retired this page's /v/<build>/ assets, a flaky fetch) reloads once onto the
// current index; the sessionStorage flag stops a loop and is cleared once a load succeeds.
const RETRY = 'mountain:reloaded';
const store = (f) => { try { return f(sessionStorage); } catch { return null; } };
if (!document.createElement('canvas').getContext('webgl2')) fallback('no WebGL2');
else import('./main.js').then(() => store((s) => s.removeItem(RETRY)), (e) => {
  console.error(e);
  if (store((s) => s.getItem(RETRY)) || store((s) => (s.setItem(RETRY, '1'), true)) !== true) {
    fallback(String(e), 'Could not load the mountain. Reload to try again.');
    return;
  }
  fallback(String(e), 'Could not load — retrying');
  setTimeout(() => location.reload(), 1000);
});

// Boot: picks the quality tier before kit loads, checks WebGL2, then loads main.js.
// ?q=low|high overrides. Low = phones / ≤ 4 cores: Lambert, 96² terrain, no post.
const q = new URLSearchParams(location.search).get('q');
globalThis.__quality = q || (matchMedia('(pointer: coarse)').matches || (navigator.hardwareConcurrency || 8) <= 4 ? 'low' : 'high');

const fallback = (why) => {
  const p = document.getElementById('fallback');
  p.hidden = false;
  console.warn('mountain: fallback', why);
};
if (!document.createElement('canvas').getContext('webgl2')) fallback('no WebGL2');
else import('./main.js').catch((e) => { console.error(e); fallback(String(e)); });

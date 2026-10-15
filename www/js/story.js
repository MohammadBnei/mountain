// Story (05, R5). Orchestrator-owned. Six sections, P 0 (Hero) … 5 (Live). Previous/Next and ←/→ step one section;
// P eases there over 1.5 s (a press mid-move retargets from the current P; prefers-reduced-motion jumps). No scroll:
// the wheel stays the orbit zoom (main.js). The URL hash names the section (#hero … #live), so a reload or a shared
// link lands on it.
//   view(P, tc, aspect, out) → out.{pos, look}: the keyframe camera (pi-lab CatmullRom keys); at P 5 it is exactly
//                              the live finale sway of layout.js, eased in from P 4
//   clip(P)                  → a world.clip value (cut.at): closed, shallow, aquifers, bands, deepest, closes
//   createStory({world, onStep}) → {tick(now) → P, P, section}: the buttons, keys, hash and panel
import * as THREE from 'three';
import * as cut from './cut.js';
import { finale } from './layout.js';
import S from './i18n.json' with { type: 'json' };

export const sections = ['hero', 'summit', 'water', 'strata', 'bedrock', 'live'];
const TAU = 2 * Math.PI, MOVE = 1500;
// ponytail: placeholder keys; the tuning is human-gated (M BN). [camera azimuth (camera at (sin, ·, cos)), camera
// height, radius, look y, cut depth from the world edge (r 100; 100 = through the summit)]
const keys = [
  [0, 70, 240, 30, 0], //               hero: wide, closed
  [TAU / 4, 75, 165, 50, 40], //        summit: ¼ orbit, shallow
  [TAU / 2, 35, 175, 25, 60], //        water: ¼ orbit, the aquifers
  [(3 * TAU) / 4, 32, 185, 15, 80], //  strata: ¼ orbit, the bands
  [(7 * TAU) / 8, 0, 195, 5, 100], //   bedrock: low, deepest
  [TAU, finale.height, finale.radius, finale.look[1], 0], // live: pull back onto the finale sway, closed
];
const cyl = new THREE.CatmullRomCurve3(keys.map(([a, h, r]) => new THREE.Vector3(a, h, r)), false, 'centripetal');
const lookY = new THREE.CatmullRomCurve3(keys.map((k) => new THREE.Vector3(k[3], 0, 0)), false, 'centripetal');
const v = new THREE.Vector3(), clamp = (x, a, b) => Math.min(b, Math.max(a, x));

export function view(P, tc, aspect, out) {
  P = clamp(P, 0, 5);
  cyl.getPoint(P / 5, v);
  // the finale sway (narrower on portrait: the pool stays in frame), faded in over the last step
  const az = v.x + clamp(P - 4, 0, 1) * finale.sway * Math.min(1, aspect) * Math.sin((TAU * tc) / finale.period);
  out.pos.set(v.z * Math.sin(az), v.y, v.z * Math.cos(az));
  out.look.set(0, lookY.getPoint(P / 5, v).x, 0);
  return out;
}

export function clip(P) {
  P = clamp(P, 0, 5);
  const i = Math.min(4, Math.floor(P)), x = P - i;
  return cut.at(keys[i][4] + (keys[i + 1][4] - keys[i][4]) * x, cyl.getPoint(P / 5, v).x); // faces the keyframe azimuth
}

const lang = navigator.language?.toLowerCase().startsWith('fr') ? 'fr' : 'en', T = S[lang];
const pct = (x) => `${Math.round((x || 0) * 100)} %`;
// Panel facts, mono: only what the page already holds (/api/topology sites, metrics, hello.state). No versions (02).
function facts(i, w) {
  const m = w.metrics || {}, s = w.state || {}, hosts = Object.values(m.hosts || {});
  const health = Object.values(s.health || {});
  return [
    [`${w.sites.size} ${T['fact.apps']}`, `${hosts.length} ${T['fact.hostsN']}`],
    [`${T['fact.snowline']} ${Math.round(w.snowline)} m`, `${health.filter((h) => h === 'Healthy').length}/${health.length} ${T['fact.healthy']}`],
    [`${m.storage?.pools?.length ?? 0} ${T['fact.pools']}`],
    [`${s.ash?.length ?? 0} ${T['fact.ash']}`, `${hosts.filter((h) => h.ready).length}/${hosts.length} ${T['fact.hosts']}`],
    [`${s.veins?.length ?? 0} ${T['fact.veins']}`],
    [`cpu ${pct(m.cpu)} · mem ${pct(m.mem)}`, `${s.alerts?.length ?? 0} ${T['fact.alerts']}`, `${m.buildsInFlight ?? 0} ${T['fact.builds']}`],
  ][i].join(' · ');
}

export function createStory({ world, onStep }) {
  const $ = (id) => document.getElementById(id), prev = $('prev'), next = $('next'), line = $('line'), fact = $('facts'), signin = $('signin');
  const reduce = matchMedia('(prefers-reduced-motion: reduce)');
  document.documentElement.lang = lang;
  prev.setAttribute('aria-label', T['story.prev']);
  next.setAttribute('aria-label', T['story.next']);
  let P = 0, from = 0, to = 0, t0 = -Infinity, factsAt = 0;

  function go(i, jump) {
    i = clamp(i, 0, 5);
    if (i === to && !jump) return;
    from = P; to = i;
    t0 = jump || reduce.matches ? -Infinity : performance.now();
    if (location.hash !== `#${sections[i]}`) history.replaceState(null, '', `#${sections[i]}`);
    prev.disabled = i === 0;
    next.disabled = i === 5;
    line.textContent = T[`story.${sections[i]}`];
    signin.href = `/auth/login?return=${encodeURIComponent(`/#${sections[i]}`)}`; // back to this section
    factsAt = 0;
    onStep?.();
  }
  const fromHash = () => Math.max(0, sections.indexOf(location.hash.slice(1)));
  prev.addEventListener('click', () => go(to - 1));
  next.addEventListener('click', () => go(to + 1));
  addEventListener('keydown', (e) => {
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return; // leave browser shortcuts alone
    if (e.key === 'ArrowLeft') go(to - 1);
    else if (e.key === 'ArrowRight') go(to + 1);
  });
  addEventListener('hashchange', () => go(fromHash()));
  signin.textContent = T['auth.signin'];
  go(fromHash(), true);
  P = to;
  // detail mode (08): the sign-in link only when the relay has OIDC on and this session views (static serving: no link)
  fetch('/auth/me').then((r) => (r.ok ? r.json() : null)).then((a) => { signin.hidden = !(a?.enabled && a.role === 'view'); }, () => {});

  return {
    tick(now) {
      const x = (now - t0) / MOVE;
      P = x >= 1 ? to : from + (to - from) * x * x * (3 - 2 * x);
      if (now > factsAt) { // ponytail: 1 s poll, the bus has no write hook
        factsAt = now + 1000;
        const f = facts(to, world);
        if (fact.textContent !== f) fact.textContent = f;
      }
      return P;
    },
    get P() { return P; },
    get section() { return sections[to]; },
  };
}

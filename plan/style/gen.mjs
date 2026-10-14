// Derives the 4 time keys from the dusk (Aretē) tokens. Writes tokens.json + prints md rows.
// Rule: env tokens scale lightness by (1+a) (darken: RGB multiply; lift: HSL L, capped 0.9) then mix toward a palette tint (t). Fixed tokens never shift.
import { writeFileSync } from 'node:fs';

const dusk = {
  sky0: '#191b38', sky1: '#2a2347', horizon: '#5c3254', cloud: '#232742', ridge: '#3b2a48',
  rock0: '#2a2733', rock1: '#17141b', emberRoot: '#3a1610', stone: '#1b1e25', stone2: '#232730', deep: '#120b0a',
  snow: '#cfe2ea', ice: '#b6d3e0', iceDeep: '#1b2a35',
  cloudLit: '#bab6b6', // = mix(ink, cloud, .25): cumulus lit albedo; `cloud` stays the shadow/underside
  trail: '#423638',    // = mix(rock0, sunMid, .12): worn walkway bench (warm dust, never snow-white)
};
const fixed = {
  gold: '#ddb153', magma: '#e5562f', lavaHot: '#ff8a5c', sunCore: '#ffcf8f', sunMid: '#f2a65e', sunHalo: '#ffd59a',
  elmo: '#d9c8ff', above: '#c9b6ff', slopes: '#9fd0b9', water: '#8fc3df', ink: '#ece6dc',
};
const keys = {
  night: { a: -0.45, tint: fixed.water, t: 0.08 },
  dawn:  { a: 0.8,  tint: fixed.above, t: 0.08 },
  // day relaxes the 10 % tint for the sky group only (R4 style review): sky0/sky1/ridge -> water .5, horizon -> water .6, cloud -> ink .5
  day:   { a: 1.6,  tint: fixed.water, t: 0.10, sky: { sky0: [fixed.water, 0.5], sky1: [fixed.water, 0.5], ridge: [fixed.water, 0.5], horizon: [fixed.water, 0.6], cloud: [fixed.ink, 0.5] }, hemiGround: 'stone' },
  dusk:  { a: 0,     tint: fixed.ink,   t: 0 },
};
// Rig (styleboard-tuned, board.png). Tone mapping: LinearToneMapping, exposure 1 (palette fidelity; lit < 1 by the luminance rule).
// Rig colours are applied at unit max channel (kit normalises: c / max(r,g,b)); intensity carries the energy.
// Phase-shifted: hemiSky (= key sky1), hemiGround (= key emberRoot), fog (= key horizon). Pinned: dir colours (fixed tokens; night = ice@dusk).
const rig = {
  // snowLift: terrain-shader snow term, unlit snow × lift (keeps the cap white-blue under violet hemi). stars: dome star-field weight. pool: idle ember-pool glow floor.
  night: { hemiI: 0.5, dir: '#b6d3e0' /* ice@dusk, pinned */, dirI: 0.8, fogD: 0.0045, exposure: 1, snowLift: 0.12, stars: 1, ember: 1 },
  dawn:  { hemiI: 1.4, dir: fixed.sunHalo, dirI: 2.4, fogD: 0.0035, exposure: 1, snowLift: 0.3, stars: 0.15, ember: 0.5 },
  day:   { hemiI: 2.0, dir: fixed.ink, dirI: 3.2, fogD: 0.0022, exposure: 1.25, snowLift: 0.05, stars: 0, ember: 0.12 },
  dusk:  { hemiI: 2.0, dir: fixed.sunMid, dirI: 1.6, fogD: 0.0035, exposure: 1, snowLift: 0.45, stars: 0.3, ember: 1 },
};

const h2r = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
const r2h = (c) => '#' + c.map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0')).join('');
function rgb2hsl([r, g, b]) {
  const mx = Math.max(r, g, b), mn = Math.min(r, g, b), l = (mx + mn) / 2, d = mx - mn;
  if (!d) return [0, 0, l];
  const s = d / (1 - Math.abs(2 * l - 1));
  const h = mx === r ? ((g - b) / d) % 6 : mx === g ? (b - r) / d + 2 : (r - g) / d + 4;
  return [(h * 60 + 360) % 360, s, l];
}
function hsl2rgb([h, s, l]) {
  const c = (1 - Math.abs(2 * l - 1)) * s, x = c * (1 - Math.abs(((h / 60) % 2) - 1)), m = l - c / 2;
  const [r, g, b] = h < 60 ? [c, x, 0] : h < 120 ? [x, c, 0] : h < 180 ? [0, c, x] : h < 240 ? [0, x, c] : h < 300 ? [x, 0, c] : [c, 0, x];
  return [r + m, g + m, b + m];
}
export const mix = (a, b, t) => r2h(h2r(a).map((v, i) => v + (h2r(b)[i] - v) * t));
function shift(hex, { a, tint, t }) {
  if (a < 0) return mix(r2h(h2r(hex).map((v) => v * (1 + a))), tint, t); // darken: RGB multiply keeps hue + saturation
  const [h, s, l] = rgb2hsl(h2r(hex));
  const l2 = l >= 0.5 ? l : Math.min(0.9, l * (1 + a)); // light tokens (snow, ice) keep L on lift
  return mix(r2h(hsl2rgb([h, s, l2])), tint, t);
}
const unmeasured = (hex) => { const [h, s, l] = rgb2hsl(h2r(hex)); return r2h(hsl2rgb([h, s * 0.3, l * 0.6])); };

const out = {};
for (const [k, p] of Object.entries(keys)) {
  const env = Object.fromEntries(Object.entries(dusk).map(([n, v]) => [n, p.sky?.[n] ? mix(shift(v, p), ...p.sky[n]) : shift(v, p)]));
  const r = rig[k];
  out[k] = { ...env, ...fixed, rig: { hemiSky: env.sky1, hemiGround: env[p.hemiGround || 'emberRoot'], hemiI: r.hemiI, dir: r.dir, dirI: r.dirI, fog: env.horizon, fogD: r.fogD, exposure: r.exposure, tm: 'linear', snowLift: r.snowLift, stars: r.stars, ember: r.ember } };
}
out.semantic = {
  healthOk: fixed.slopes, healthDegraded: fixed.gold, healthDown: fixed.magma,
  groupPlatform: fixed.water, groupUser: fixed.above,
  slabNotReady: dusk.deep, slabLoad0: dusk.stone2, slabLoad50: dusk.emberRoot, slabLoad100: fixed.magma,
  errB: fixed.magma, longhorn: fixed.water, local: mix(fixed.water, dusk.emberRoot, 0.45),
  unmeasuredWater: unmeasured(fixed.water), unmeasuredGold: unmeasured(fixed.gold), unmeasuredLocal: unmeasured(mix(fixed.water, dusk.emberRoot, 0.45)),
  vein: fixed.gold, veinRef: fixed.sunHalo, crystalBody: mix(dusk.ice, dusk.stone2, 0.35),
  lightningCore: fixed.ink, lightningHalo: fixed.elmo,
};
// Ramps: discrete stops [t, token]. semantic = never phase-shift; env = colours follow the key (kit rewrites instanceColor on setPhase).
out.ramps = {
  semantic: {
    slabLoad: [[0, dusk.stone2], [0.5, dusk.emberRoot], [1, fixed.magma]],
    health: [[0, fixed.slopes], [0.5, fixed.gold], [1, fixed.magma]],
    group: [[0, fixed.water], [1, fixed.above]],
    lava: [[0, fixed.lavaHot], [0.5, fixed.magma], [1, dusk.emberRoot]],
  },
  env: {
    rock: [[0, 'emberRoot'], [0.06, 'rock1'], [1, 'rock0']],
    snow: [[0, 'ice'], [1, 'snow']],
    cloud: [[0, 'cloud'], [1, 'cloudLit']], // base -> top of each cumulus
    // rock stop 0 is mix(rock1, emberRoot, rig.ember): the ember root fades by day
  },
};
writeFileSync(new URL('./tokens.json', import.meta.url), JSON.stringify(out, null, 1));
writeFileSync(new URL('./tokens.js', import.meta.url), 'window.T=' + JSON.stringify(out) + ';\n'); // for swatches.html over file://

// md rows
const names = Object.keys(dusk);
console.log('| token | ' + Object.keys(keys).join(' | ') + ' |');
for (const n of names) console.log(`| \`${n}\` | ` + Object.keys(keys).map((k) => `\`${out[k][n]}\``).join(' | ') + ' |');
console.log();
for (const f of ['hemiSky', 'hemiGround', 'hemiI', 'dir', 'dirI', 'fog', 'fogD', 'exposure'])
  console.log(`| ${f} | ` + Object.keys(keys).map((k) => '`' + out[k].rig[f] + '`').join(' | ') + ' |');
console.log(JSON.stringify(out.semantic, null, 1));

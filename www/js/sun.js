// The real sun and moon over Paris (01 world.sun / world.moon, 03-model-sky "full"): SunCalc's low-precision formulas
// (mourner/suncalc, BSD-2), positions + the moon's lit fraction only (no rise/set times). Orchestrator-owned helper; sky
// imports it and writes world.sun / world.moon / world.rig = {elev, rising} from it (the host turns world.rig into
// kit.setPhase and world.sun + world.moon into kit.setSky).
//   sun(ms = now(), lat = PARIS.lat, lon = PARIS.lon)  → {elev (deg), az (deg from north, clockwise), rising}
//   moon(ms = now(), lat = PARIS.lat, lon = PARIS.lon) → {elev, az, frac (0 new .. 1 full)}
//   now() → ms: the wall clock; on a dev host (127.0.0.1) ?at=<ISO date> pins it there (QA of noon, dusk, moon nights)
// rising: before the sun's transit (local solar noon), so dawn vs dusk in kit.keyMix.
const RAD = Math.PI / 180, E = RAD * 23.4397; // obliquity of the ecliptic
export const PARIS = { lat: 48.8566, lon: 2.3522 };

const loc = globalThis.location, pin = loc?.hostname === '127.0.0.1' && Date.parse(new URLSearchParams(loc.search).get('at'));
export const now = () => (pin ? pin : Date.now());

const days = (ms) => ms / 864e5 - 10957.5; // days since J2000 (2000-01-01 12:00 UTC)
const radec = (l, b) => ({ ra: Math.atan2(Math.sin(l) * Math.cos(E) - Math.tan(b) * Math.sin(E), Math.cos(l)), dec: Math.asin(Math.sin(b) * Math.cos(E) + Math.cos(b) * Math.sin(E) * Math.sin(l)) });
function sunRaDec(d) {
  const M = RAD * (357.5291 + 0.98560028 * d); // solar mean anomaly
  return radec(M + RAD * (1.9148 * Math.sin(M) + 0.02 * Math.sin(2 * M) + 0.0003 * Math.sin(3 * M)) + RAD * 102.9372 + Math.PI, 0); // ecliptic longitude
}
function moonRaDec(d) {
  const L = RAD * (218.316 + 13.176396 * d), M = RAD * (134.963 + 13.064993 * d), F = RAD * (93.272 + 13.22935 * d);
  return { ...radec(L + RAD * 6.289 * Math.sin(M), RAD * 5.128 * Math.sin(F)), dist: 385001 - 20905 * Math.cos(M) }; // km
}
/** Horizontal coordinates of (ra, dec) at day d: elevation and azimuth in degrees (from north, clockwise) + hour angle. */
function sky({ ra, dec }, d, lat, lon) {
  const H = RAD * (280.16 + 360.9856235 * d + lon) - ra, phi = RAD * lat; // hour angle
  const elev = Math.asin(Math.sin(phi) * Math.sin(dec) + Math.cos(phi) * Math.cos(dec) * Math.cos(H));
  const az = Math.atan2(Math.sin(H), Math.cos(H) * Math.sin(phi) - Math.tan(dec) * Math.cos(phi)); // from south, west positive
  return { elev: elev / RAD, az: ((az / RAD + 180) % 360 + 360) % 360, H };
}

export function sun(ms = now(), lat = PARIS.lat, lon = PARIS.lon) {
  const { elev, az, H } = sky(sunRaDec(days(ms)), days(ms), lat, lon);
  return { elev, az, rising: Math.sin(H) < 0 };
}

export function moon(ms = now(), lat = PARIS.lat, lon = PARIS.lon) {
  const d = days(ms), s = sunRaDec(d), m = moonRaDec(d), { elev, az } = sky(m, d, lat, lon), SD = 149598000; // sun distance, km
  const phi = Math.acos(Math.sin(s.dec) * Math.sin(m.dec) + Math.cos(s.dec) * Math.cos(m.dec) * Math.cos(s.ra - m.ra));
  const inc = Math.atan2(SD * Math.sin(phi), m.dist - SD * Math.cos(phi));
  return { elev, az, frac: (1 + Math.cos(inc)) / 2 };
}

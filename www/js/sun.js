// The real sun over Paris (01 world.sun, 03-model-sky "full"): SunCalc's low-precision formulas (mourner/suncalc,
// BSD-2), the position only (no rise/set times). Orchestrator-owned helper; sky imports it and writes world.sun /
// world.rig = {elev, rising} from it (the host turns world.rig into kit.setPhase).
//   sun(ms = Date.now(), lat = PARIS.lat, lon = PARIS.lon) → {elev (deg), az (deg from north, clockwise), rising}
// rising: before the sun's transit (local solar noon), so dawn vs dusk in kit.keyMix.
const RAD = Math.PI / 180, E = RAD * 23.4397; // obliquity of the ecliptic
export const PARIS = { lat: 48.8566, lon: 2.3522 };

export function sun(ms = Date.now(), lat = PARIS.lat, lon = PARIS.lon) {
  const d = ms / 864e5 - 10957.5; // days since J2000 (2000-01-01 12:00 UTC)
  const M = RAD * (357.5291 + 0.98560028 * d); // solar mean anomaly
  const L = M + RAD * (1.9148 * Math.sin(M) + 0.02 * Math.sin(2 * M) + 0.0003 * Math.sin(3 * M)) + RAD * 102.9372 + Math.PI; // ecliptic longitude
  const dec = Math.asin(Math.sin(E) * Math.sin(L)), ra = Math.atan2(Math.sin(L) * Math.cos(E), Math.cos(L));
  const H = RAD * (280.16 + 360.9856235 * d + lon) - ra, phi = RAD * lat; // hour angle
  const elev = Math.asin(Math.sin(phi) * Math.sin(dec) + Math.cos(phi) * Math.cos(dec) * Math.cos(H));
  const az = Math.atan2(Math.sin(H), Math.cos(H) * Math.sin(phi) - Math.tan(dec) * Math.cos(phi)); // from south, west positive
  return { elev: elev / RAD, az: ((az / RAD + 180) % 360 + 360) % 360, rising: Math.sin(H) < 0 };
}

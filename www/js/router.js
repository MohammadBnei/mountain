// Hash routes (plan/10 "The load rule"): every view is a link, and back zooms out one step.
//   #/            L0, the map
//   #/L1          L1, every trade route
//   #/L1/<realm>  L1 focused on one realm (its groups listed)
// query (inside the hash): g=<group> (a group's list open), sel=<node id> (its card open), lens=land
import { REALMS } from './model.js';

const ID = /^[a-z]+:[a-z0-9][a-z0-9._/@-]*$/;
const GROUP = /^[a-z0-9][a-z0-9-]*$/;

/** parse reads a location hash into a view; anything it does not understand falls back to the map. */
export function parse(hash = '') {
  const [path, query = ''] = hash.replace(/^#/, '').split('?');
  const q = new URLSearchParams(query), parts = path.split('/').filter(Boolean);
  const v = { level: 0, realm: null, group: null, sel: null, lens: q.get('lens') === 'land' ? 'land' : 'structure' };
  if (parts[0] === 'L1') {
    v.level = 1;
    if (REALMS.includes(parts[1])) v.realm = parts[1];
  }
  if (v.realm && GROUP.test(q.get('g') || '')) v.group = q.get('g');
  if (ID.test(q.get('sel') || '')) v.sel = q.get('sel');
  return v;
}

/** format writes a view as a hash; parse(format(v)) is v. */
export function format(v) {
  let h = '#/';
  if (v.level === 1) h += 'L1' + (v.realm ? `/${v.realm}` : '');
  const q = new URLSearchParams();
  if (v.realm && v.group) q.set('g', v.group);
  if (v.sel) q.set('sel', v.sel);
  if (v.lens === 'land') q.set('lens', 'land');
  const s = q.toString();
  return s ? `${h}?${s}` : h;
}

/** up is one step out: the card, then the group, then the realm, then L1, then the map. */
export function up(v) {
  if (v.sel) return { ...v, sel: null };
  if (v.group) return { ...v, group: null };
  if (v.realm) return { ...v, realm: null };
  if (v.level === 1) return { ...v, level: 0 };
  return v;
}

// Hash routes (plan/10 "The load rule"): every view is a link, and up() zooms out one step.
//   #/                     L0, the map
//   #/L1                   L1, every trade route
//   #/L1/<realm>           L1 focused on one realm (its groups listed, its routes lit)
//   #/L2/<realm>           L2, the realm as a city: its groups are districts, its parts buildings
//   #/L3/<node id>         L3, one part as a building, its walls carrying cards
//   #/L4/<repo>/<path>     L4, the reader (from=<node id>: where back goes)
// query (inside the hash): g=<group> (a district's list open), sel=<node id> (its card open), lens=land
import { REALMS } from './model.js';

const ID = /^[a-z]+:[a-z0-9][a-z0-9._/@-]*$/;
const GROUP = /^[a-z0-9][a-z0-9-]*$/;
const REPO = /^[A-Za-z0-9._-]+$/;
const PATH = /^(?!.*(^|\/)\.\.?(\/|$))[A-Za-z0-9._@+,=~ -][A-Za-z0-9._@+,=~ /-]*$/; // repo-relative, no . or .. segment

/** parse reads a location hash into a view; anything it does not understand falls back to the map. */
export function parse(hash = '') {
  const [raw, query = ''] = hash.replace(/^#/, '').split('?');
  const q = new URLSearchParams(query),
    parts = raw
      .split('/')
      .filter(Boolean)
      .map(p => {
        try {
          return decodeURIComponent(p);
        } catch {
          return '';
        }
      });
  const v = {
    level: 0,
    realm: null,
    group: null,
    sel: null,
    id: null,
    doc: null,
    from: null,
    lens: q.get('lens') === 'land' ? 'land' : 'structure'
  };
  if (
    (parts[0] === 'L1' || parts[0] === 'L2') &&
    (parts[0] === 'L1' || REALMS.includes(parts[1]))
  ) {
    v.level = parts[0] === 'L1' ? 1 : 2;
    if (REALMS.includes(parts[1])) v.realm = parts[1];
  } else if (parts[0] === 'L3' && ID.test(parts.slice(1).join('/'))) {
    v.level = 3;
    v.id = parts.slice(1).join('/');
  } else if (
    parts[0] === 'L4' &&
    REPO.test(parts[1] || '') &&
    PATH.test(parts.slice(2).join('/'))
  ) {
    v.level = 4;
    v.doc = { repo: parts[1], path: parts.slice(2).join('/') };
    if (ID.test(q.get('from') || '')) v.from = q.get('from');
  }
  if (v.realm && v.level >= 1 && GROUP.test(q.get('g') || '')) v.group = q.get('g');
  if (v.level <= 2 && ID.test(q.get('sel') || '')) v.sel = q.get('sel');
  return v;
}

const enc = s =>
  s
    .split('/')
    .map(x => encodeURIComponent(x).replace(/%3A/g, ':').replace(/%40/g, '@'))
    .join('/'); // ids read as written

/** format writes a view as a hash; parse(format(v)) is v. */
export function format(v) {
  let h = '#/';
  const q = new URLSearchParams();
  if (v.level === 1 || v.level === 2) {
    h += `L${v.level}` + (v.realm ? `/${v.realm}` : '');
    if (v.realm && v.group) q.set('g', v.group);
    if (v.sel) q.set('sel', v.sel);
  } else if (v.level === 3) {
    h += `L3/${enc(v.id)}`;
  } else if (v.level === 4) {
    h += `L4/${encodeURIComponent(v.doc.repo)}/${enc(v.doc.path)}`;
    if (v.from) q.set('from', v.from);
  } else if (v.sel) q.set('sel', v.sel);
  if (v.lens === 'land') q.set('lens', 'land');
  const s = q.toString();
  return s ? `${h}?${s}` : h;
}

/**
 * up is one step out: the card, then the district, then the city (to its realm at L1), then L1, then the map; a
 * building goes back to its city's district, the reader to the building it was opened from. where(id) is the node's
 * {realm, group}, or null.
 */
export function up(v, where = () => null) {
  const keep = { lens: v.lens };
  if (v.level === 4) return v.from ? { ...keep, level: 3, id: v.from } : { ...keep, level: 0 };
  if (v.level === 3) {
    const g = where(v.id);
    return g ? { ...keep, level: 2, realm: g.realm, group: g.group } : { ...keep, level: 0 };
  }
  if (v.sel) return { ...v, sel: null };
  if (v.group) return { ...v, group: null };
  if (v.level === 2) return { ...keep, level: 1, realm: v.realm };
  if (v.realm) return { ...v, realm: null };
  if (v.level === 1) return { ...v, level: 0 };
  return v;
}

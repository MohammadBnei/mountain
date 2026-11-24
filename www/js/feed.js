// The feed's page (feed.html, cmd/atlas/feed_page.go): /feed is the timeline, /feed/<id> one thread. It polls /api/feed
// (there is no stream) and never moves what is on screen by itself: a "N new" pill does, when pressed. Everything that
// comes from an event is text (ui.js el: createElement and textContent); a link is built only from a thread's `links`,
// and only from an https URL or a thread id (linkOf).
import { gateLapsed, signedOut } from './socket.js';
import { el } from './ui.js';

export const KINDS = {
  alert: ['Alert', 'Alerts', 'Alert: '],
  deploy: ['Deploy', 'Deploys', 'Deploy: '],
  pr: ['PR', 'PRs', 'PR '],
  session: ['Session', 'Sessions', 'Session: '],
  card: ['Card', 'Cards', 'Card: ']
};
const POLL = 15e3; // between reads while the page is visible; doubled after a failure, to a minute
const STALE = 2 * 3600e3; // no event for this long: the page says so
const NEEDS_SHOWN = 5;
const ENTRIES_SHOWN = 100;

// ---- pure (www/dev/core.test.mjs)

/** clean drops control and format characters (bidi overrides, zero widths) that could reorder or hide text. */
export const clean = s =>
  String(s ?? '').replace(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/gu, c => (c === '\n' || c === '\t' ? c : ''));

/** rel is a short relative time, as X writes it: now, 5m, 3h, 2d. */
export function rel(t, now = Date.now()) {
  const s = Math.round((now - t) / 1000);
  if (!Number.isFinite(s)) return '?';
  if (s < 60) return s < -60 ? 'soon' : 'now';
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

/** route is what a path shows: the timeline, or one thread. */
export function route(path, search = '') {
  const id = /^\/feed\/([a-z]{1,16}-[0-9a-f]{16})$/.exec(path)?.[1];
  const kind = new URLSearchParams(search).get('kind');
  return id ? { id } : { kind: Object.hasOwn(KINDS, kind ?? '') ? kind : '' };
}

/** titleOf is a thread's title without the kind its chip already says. */
export function titleOf(t) {
  const title = clean(t.title),
    prefix = Object.hasOwn(KINDS, t.kind) ? KINDS[t.kind][2] : '';
  return prefix && title.startsWith(prefix) ? title.slice(prefix.length) : title;
}

/** speaker is who an entry is from. A quoted entry is outside's, whoever relayed it, unless an agent or the owner posted it. */
export function speaker(e) {
  const who = clean(e.actor).trim();
  if (who === 'owner') return { who: 'owner', name: 'You', mark: 'Y' };
  if (who !== '' && who !== 'system')
    return { who: 'agent', name: who, mark: [...who][0].toUpperCase() };
  return e.quoted
    ? { who: 'outside', name: 'Outside', mark: '”' }
    : { who: 'system', name: 'System', mark: 'S' };
}

const LINKS = {
  previous: 'Previous episode',
  deploy: 'The rollout it caused',
  pull_request: 'The PR it shipped',
  generator: 'Alert source',
  run: 'CI run',
  revision: 'Revision'
};

/** linkOf is a link as the page may show it: {label, href, out, host, rest}; no href means text only. */
export function linkOf(l) {
  const ref = clean(l.ref),
    label = Object.hasOwn(LINKS, l.kind) ? LINKS[l.kind] : clean(l.kind);
  if (!l.quoted && /^[a-z]{1,16}-[0-9a-f]{16}$/.test(ref)) return { label, href: `/feed/${ref}` };
  if (/^https:\/\//i.test(ref)) {
    try {
      const u = new URL(ref);
      if (u.protocol === 'https:' && !u.username && !u.password)
        return {
          label,
          href: u.href,
          out: true,
          host: u.host,
          rest: u.pathname === '/' ? '' : u.pathname
        };
    } catch {
      /* not a URL: text */
    }
  }
  return { label, rest: l.kind === 'revision' ? ref.slice(0, 12) : ref.slice(0, 200) };
}

export const sig = t => `${t.last_activity_at}|${t.needs_owner}|${t.state}`;

/** fresh counts the threads a new read has that the shown ones lack or show differently. A thread that only slid into
 *  the page (older than the last one shown, when more follow) is not new. */
export function fresh(shown, threads, tail = '') {
  return threads.filter(t => {
    const s = shown.get(t.id);
    return s == null ? Date.parse(t.last_activity_at) >= (Date.parse(tail) || 0) : s !== sig(t);
  }).length;
}

/** lastEvent is when the feed last had something new, in ms; null when it never did. The older of the two ages: after
 *  a restart the replay stores old events now. */
export function lastEvent(age, now = Date.now()) {
  if (age?.last_event_age == null) return null;
  const stored = now - age.last_event_age * 1000,
    newest = Date.parse(age.newest_activity_at ?? '');
  return Number.isNaN(newest) ? stored : Math.min(stored, newest);
}

/** long is whether a text is worth folding. */
export const isLong = s => s.length > 480 || s.split('\n').length > 8;

// ---- the page

const $ = id => document.getElementById(id);
const plural = (n, w) => `${n} ${w}${n === 1 ? '' : 's'}`;

async function get(path) {
  const r = await fetch(path, { cache: 'no-store' });
  if (!r.ok) throw Object.assign(new Error(`${path}: ${r.status}`), { status: r.status });
  return r.json();
}

/** when is a relative time; the absolute one shows on hover and focus. Only a thread's are tab stops: a card's shows
 *  while its link has the focus. */
function when(iso, stop = false) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return el('span', { class: 'when' }, '?');
  const abs = d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' });
  return el(
    'time',
    { class: 'when', datetime: d.toISOString(), 'data-abs': abs, tabindex: stop ? '0' : null },
    el('span', { 'aria-hidden': 'true', 'data-t': d.getTime() }, rel(d.getTime())),
    el('span', { class: 'sr' }, abs)
  );
}

const mark = kind => el('span', { class: 'k', 'data-kind': kind, 'aria-hidden': 'true' });
const kindName = kind => (Object.hasOwn(KINDS, kind) ? KINDS[kind][0] : clean(kind));
const badge = t => el('span', { class: 'badge' }, t.state === 'open' ? 'open' : 'closed');

/** text is a paragraph of an event's words, or a quote of them when they are from outside. */
function text(s, quoted, cls = '') {
  const p = el('p', { class: `text ${cls}`, dir: 'auto' }, clean(s));
  return quoted ? el('blockquote', {}, p) : p;
}

/** folded is text with the rest behind a button, when it is long. */
function folded(s, quoted) {
  const t = clean(s);
  if (!isLong(t)) return text(t, quoted);
  const box = text(t, quoted, 'clamp'),
    p = quoted ? box.firstChild : box;
  const b = el(
    'button',
    { type: 'button', class: 'more', 'aria-expanded': 'false' },
    `Show all ${t.length.toLocaleString()} characters`
  );
  b.addEventListener('click', () => {
    const open = p.classList.toggle('clamp') === false;
    b.setAttribute('aria-expanded', String(open));
    b.textContent = open ? 'Show less' : `Show all ${t.length.toLocaleString()} characters`;
  });
  return [box, b];
}

const title = t =>
  el('span', { class: t.title_quoted ? 'qt text clamp' : 'text clamp', dir: 'auto' }, titleOf(t));

function card(t) {
  const last = t.entries?.at(-1);
  return el(
    'li',
    { class: 'card', 'data-id': t.id },
    el(
      'div',
      { class: 'line' },
      mark(t.kind),
      el('span', {}, kindName(t.kind)),
      badge(t),
      when(t.last_activity_at)
    ),
    el('h3', {}, el('a', { href: `/feed/${encodeURIComponent(t.id)}` }, title(t))),
    last
      ? el(
          'div',
          { class: 'last' },
          ['agent', 'owner'].includes(speaker(last).who)
            ? el('b', { dir: 'auto' }, speaker(last).name.slice(0, 40))
            : el('span', { class: 'sr' }, `${speaker(last).name}: `),
          text(last.text, last.quoted, 'clamp')
        )
      : null
  );
}

function state(head, body, ...more) {
  return el('div', { class: 'state' }, el('b', {}, head), body, ...more.map(m => el('div', {}, m)));
}

function retry(f) {
  const b = el('button', { type: 'button' }, 'Try again');
  b.addEventListener('click', f);
  return b;
}

// ---- the timeline

const list = { shown: new Map(), needs: [], next: '', tail: '', needsOpen: false };

async function readList(kind, cursor = '') {
  const q = new URLSearchParams({ needs: '0', limit: '50' });
  if (kind) q.set('kind', kind);
  if (cursor) q.set('cursor', cursor);
  if (cursor) return { rest: await get(`/api/feed?${q}`) };
  const n = new URLSearchParams({ needs: '1', limit: '200' });
  if (kind) n.set('kind', kind);
  const [needs, rest] = await Promise.all([get(`/api/feed?${n}`), get(`/api/feed?${q}`)]);
  return { needs, rest };
}

function drawList(kind, { needs, rest }) {
  list.shown = new Map([...needs.threads, ...rest.threads].map(t => [t.id, sig(t)]));
  list.needs = needs.threads.map(t => t.id);
  list.next = rest.next_cursor;
  list.tail = list.next ? rest.threads.at(-1)?.last_activity_at : '';
  const n = needs.threads.length,
    many = `${n}${needs.next_cursor ? '+' : ''}`;
  document.title = `${n ? `(${many}) ` : ''}${kind ? KINDS[kind][1] : 'Feed'} · ukubi`;
  const out = [];
  if (n) {
    const ol = el(
      'ol',
      { class: 'cards needs', 'aria-labelledby': 'needs' },
      needs.threads.map(card)
    );
    out.push(el('h2', { id: 'needs' }, `Needs you · ${many}`), ol);
    if (n > NEEDS_SHOWN && !list.needsOpen) {
      const rows = [...ol.children].slice(NEEDS_SHOWN);
      for (const r of rows) r.hidden = true;
      const b = el(
        'button',
        { type: 'button', class: 'wide' },
        `Show ${n - NEEDS_SHOWN} more that need you`
      );
      b.addEventListener('click', () => {
        list.needsOpen = true;
        for (const r of rows) r.hidden = false;
        b.remove();
        rows[0].querySelector('a').focus();
      });
      out.push(b);
    }
  }
  if (rest.threads.length) {
    out.push(
      el('h2', { id: 'latest' }, n ? 'Everything else' : 'Latest'),
      el('ol', { class: 'cards', id: 'rows', 'aria-labelledby': 'latest' }, rest.threads.map(card))
    );
  } else if (!n) {
    out.push(
      state(
        kind ? `No ${KINDS[kind][1].toLowerCase()} yet` : 'Nothing yet',
        'What happens shows up here, newest first.'
      )
    );
  } else {
    out.push(el('p', { class: 'end' }, 'Nothing else.'));
  }
  const foot = el('div', { id: 'foot' });
  out.push(foot);
  $('view').replaceChildren(
    el('h1', { class: 'sr' }, kind ? KINDS[kind][1] : 'Feed'),
    ...out.flat()
  );
  drawFoot(kind, rest);
}

function drawFoot(kind, rest) {
  const foot = $('foot');
  if (list.next) {
    const b = el('button', { type: 'button', class: 'wide' }, 'Load more');
    b.addEventListener('click', async () => {
      b.disabled = true;
      b.textContent = 'Loading…';
      try {
        const { rest: more } = await readList(kind, list.next);
        const at = $('rows').children.length;
        // a thread that moved while paging is already above: it is not drawn twice
        const rows = more.threads.filter(t => !list.shown.has(t.id));
        $('rows').append(...rows.map(card));
        for (const t of rows) list.shown.set(t.id, sig(t));
        list.next = more.next_cursor;
        list.tail = list.next ? more.threads.at(-1)?.last_activity_at : '';
        drawFoot(kind, more);
        $('rows').children[at]?.querySelector('a').focus({ preventScroll: true });
      } catch (e) {
        b.disabled = false;
        b.textContent = 'Could not load more. Try again';
        failed(e);
      }
    });
    return foot.replaceChildren(b);
  }
  const held = rest.dropped
    ? `The feed keeps 30 days at most; ${plural(rest.dropped, 'older event')} made room for newer ones.`
    : 'That is everything. The feed keeps 30 days at most.';
  foot.replaceChildren(list.shown.size ? el('p', { class: 'end' }, held) : '');
}

// ---- one thread

const thread = { shown: 0, all: false, sig: '' };

const keyOf = t => `${sig(t)}|${t.entries.length}|${t.title}|${t.links?.length ?? 0}`;

function entry(e, i) {
  const s = speaker(e);
  return el(
    'li',
    { class: 'entry', 'data-who': s.who, 'data-i': i, tabindex: '-1' },
    el('span', { class: 'tile', 'aria-hidden': 'true' }, s.mark),
    el(
      'div',
      {},
      el(
        'div',
        { class: 'line' },
        el('b', { dir: 'auto' }, s.name),
        s.who === 'agent' ? el('span', { class: 'badge' }, 'agent') : null,
        e.quoted ? el('span', {}, s.who === 'outside' ? 'quoted' : 'quoted text') : null,
        when(e.time, true)
      ),
      el('div', { class: 'body' }, folded(e.text, e.quoted))
    )
  );
}

function linkItem(l) {
  const k = linkOf(l);
  const parts = [
    el('b', { dir: 'auto' }, k.label),
    k.host ? el('span', { class: 'mono host' }, k.host) : null,
    k.rest ? el('span', { class: 'mono faint' }, k.rest) : null
  ];
  if (!k.href) return el('li', {}, el('span', { class: 'ref' }, parts));
  return el(
    'li',
    {},
    k.out
      ? el(
          'a',
          { class: 'out', href: k.href, target: '_blank', rel: 'noopener noreferrer' },
          parts,
          el('span', { 'aria-hidden': 'true' }, '↗'),
          el('span', { class: 'sr' }, '(opens another site)')
        )
      : el('a', { href: k.href }, parts)
  );
}

function drawThread(t) {
  const es = t.entries ?? [],
    from = thread.all ? 0 : Math.max(0, es.length - ENTRIES_SHOWN);
  const old = thread.shown;
  thread.shown = es.length;
  thread.sig = keyOf(t);
  document.title = `${t.needs_owner ? '(!) ' : ''}${titleOf(t).slice(0, 80)} · Feed`;
  crumbs(`${kindName(t.kind)} thread`);
  const rows = es.slice(from).map((e, i) => entry(e, from + i));
  if (old) for (const r of rows) if (Number(r.dataset.i) >= old) r.classList.add('fresh');
  let earlier = null;
  if (from) {
    earlier = el('button', { type: 'button', class: 'wide' }, `Show ${from} earlier`);
    earlier.addEventListener('click', () => {
      thread.all = true;
      drawThread(t);
      $('view').querySelector('.entry')?.focus();
    });
  }
  const ttl = titleOf(t);
  $('view').replaceChildren(
    el('a', { class: 'back', href: '/feed' }, '← Feed'),
    el(
      'header',
      { class: 'thead' },
      el(
        'div',
        { class: 'line' },
        mark(t.kind),
        el('span', {}, kindName(t.kind)),
        badge(t),
        t.needs_owner ? el('span', { class: 'badge hot' }, 'needs you') : null,
        el('span', { class: 'right' }, 'opened ', when(t.opened_at, true))
      ),
      el('h1', {}, title(t)),
      ttl.length > 200
        ? el(
            'details',
            {},
            el('summary', {}, `Whole title, ${ttl.length.toLocaleString()} characters`),
            text(ttl, t.title_quoted)
          )
        : null,
      t.links?.length
        ? el('ul', { class: 'linked', 'aria-label': 'Links' }, t.links.map(linkItem))
        : null
    ),
    earlier ?? '',
    el('h2', { class: 'sr' }, 'Entries'),
    rows.length
      ? el('ol', { class: 'entries' }, rows)
      : state('No entries', 'This thread holds nothing yet.'),
    // slice 5 puts the reply box here
    el(
      'section',
      { id: 'reply', 'aria-label': 'Reply' },
      'Read only for now: replies come here. Sessions and cards are answered in the ',
      el(
        'a',
        { href: 'https://fleet.bnei.dev/', target: '_blank', rel: 'noopener noreferrer' },
        'fleet console ↗'
      ),
      '.'
    )
  );
}

// ---- both

const at = globalThis.location ? route(location.pathname, location.search) : {};
let pending = null, // what the last read found and the page does not show yet
  age = null, // the last answer's age, and when it came
  wait = POLL,
  timer = 0,
  down = null, // the read failure on show
  over = false, // signed out: nothing more is read
  drawn = false;

function crumbs(here) {
  const ol = el('ol', { class: 'crumbs' });
  const items = [
    el('a', { href: '/' }, 'ukubi'),
    here ? el('a', { href: '/feed' }, 'feed') : el('span', { class: 'here' }, 'feed'),
    here ? el('span', { class: 'here' }, here) : null
  ].filter(Boolean);
  items.forEach((c, i) => {
    ol.append(el('li', {}, i ? el('span', { class: 'sep', 'aria-hidden': 'true' }, '›') : null, c));
  });
  $('crumbs').replaceChildren(ol);
}

function kinds() {
  const tab = (k, name) =>
    el(
      'li',
      {},
      el(
        'a',
        { href: k ? `/feed?kind=${k}` : '/feed', 'aria-current': at.kind === k ? 'page' : null },
        k ? mark(k) : null,
        name
      )
    );
  $('kinds').replaceChildren(
    el(
      'ul',
      {},
      tab('', 'All'),
      Object.entries(KINDS).map(([k, v]) => tab(k, v[1]))
    )
  );
}

const span = ms =>
  ms < 3600e3
    ? plural(Math.max(1, Math.round(ms / 60e3)), 'minute')
    : ms < 48 * 3600e3
      ? plural(Math.round(ms / 3600e3), 'hour')
      : plural(Math.round(ms / 86400e3), 'day');

/** banner says what makes the page less than current: a failed read, or a feed that has had nothing for long. Its text
 *  changes by the minute at most, so a screen reader hears it once. */
function banner() {
  const now = Date.now(),
    last = age && lastEvent({ ...age.v, last_event_age: aged(now) }, now);
  let msg = '';
  if (down && drawn)
    msg = `Not updating: the last answer came ${span(now - age.at)} ago. Trying again.`;
  else if (age && last === null)
    msg =
      'The feed has had no event since it started. It may still be reading, or the bridge may be out of reach.';
  else if (age && now - last > STALE)
    msg = `No event for ${span(now - last)}. This may be out of date: the bridge may have stopped sending.`;
  if ($('banner').dataset.msg !== msg) {
    $('banner').dataset.msg = msg;
    $('banner').replaceChildren(msg ? el('p', {}, msg) : '');
  }
  $('fresh').textContent = last ? `Last event ${rel(last, now)} ago`.replace('now ago', 'now') : '';
  for (const s of document.querySelectorAll('[data-t]'))
    s.textContent = rel(Number(s.dataset.t), now);
}

const aged = now =>
  age.v?.last_event_age == null ? null : age.v.last_event_age + (now - age.at) / 1000;

function failed(e) {
  console.warn('feed:', e.message);
  down = e;
  banner();
}

/** lapsed: the sign-in ended under the page. One reload signs in again (socket.js); a sign-out stops the page. */
async function lapsed(e) {
  if (e.status && e.status !== 403 && e.status !== 401) return false;
  const r = await gateLapsed();
  if (r !== 'signed-out' && !(e.status === 403 && (await signedOut()))) return r;
  over = true;
  $('view').replaceChildren(state('Signed out', 'Reload to sign in again.'));
  $('pill').hidden = true;
  return true;
}

function pill(n, words = `${n} new`) {
  $('pill').textContent = words;
  $('pill').hidden = false;
}

function show() {
  if (!pending) return;
  const d = pending;
  pending = null;
  $('pill').hidden = true;
  if (at.id) {
    const old = thread.shown;
    drawThread(d);
    const first = $('view').querySelector(`.entry[data-i="${old}"]`);
    first?.focus({ preventScroll: true });
    first?.scrollIntoView({ block: 'center' });
    return;
  }
  drawList(at.kind, d);
  scrollTo(0, 0);
  $('view').querySelector('.card h3 a')?.focus({ preventScroll: true });
}

async function read() {
  clearTimeout(timer);
  if (over) return;
  try {
    const d = at.id ? await get(`/api/feed/${at.id}`) : await readList(at.kind);
    age = { v: at.id ? d.age : d.rest.age, at: Date.now() };
    down = null;
    wait = POLL;
    if (!drawn) {
      if (at.id) drawThread(d);
      else drawList(at.kind, d);
      drawn = true;
    } else if (at.id) {
      const n = d.entries.length - thread.shown;
      pending = keyOf(d) === thread.sig ? null : d;
      if (!pending) $('pill').hidden = true;
      else pill(n, n < 1 ? 'Changed · show' : `${n} new · show`); // n < 1: only its head, or older entries evicted
    } else {
      const now = new Set([...d.needs.threads, ...d.rest.threads].map(t => t.id));
      const n =
        fresh(list.shown, d.needs.threads) +
        fresh(list.shown, d.rest.threads, list.tail || '') +
        list.needs.filter(id => !now.has(id)).length; // one that needed the owner and went further down
      pending = n ? d : null;
      if (!n) $('pill').hidden = true;
      else if (list.shown.size) pill(n);
      else show(); // an empty page has nothing to move
    }
    banner();
  } catch (e) {
    if (await lapsed(e)) return;
    wait = Math.min(60e3, wait * 2);
    failed(e);
    if (e.status === 404 && at.id) {
      over = true;
      drawn = true;
      down = null;
      banner();
      crumbs('thread');
      $('view').replaceChildren(
        state(
          'Not in the feed',
          'This thread does not exist, or is older than what the feed keeps.',
          el('a', { href: '/feed' }, 'Back to the feed')
        )
      );
      return;
    }
    if (!drawn) {
      $('view').replaceChildren(
        state('Could not read the feed', 'Trying again by itself.', retry(read))
      );
    }
  }
  if (!document.hidden) timer = setTimeout(read, wait);
}

/** keys: X's. j and k walk the cards or entries, . shows what is new, u goes back; Enter is the link's own. */
function key(e) {
  if (e.metaKey || e.ctrlKey || e.altKey || e.target.closest?.('input, textarea, select')) return;
  if (e.key === '.' && pending) return show();
  if (e.key === 'u' && at.id) return location.assign('/feed');
  if (e.key !== 'j' && e.key !== 'k') return;
  const all = [...document.querySelectorAll(at.id ? '.entry' : '.card:not([hidden]) h3 a')];
  const i = all.findIndex(x => x === document.activeElement || x.contains(document.activeElement));
  const to = all[i < 0 ? 0 : Math.min(all.length - 1, Math.max(0, i + (e.key === 'j' ? 1 : -1)))];
  to?.focus({ preventScroll: true });
  to?.scrollIntoView({ block: 'nearest' });
}

function boot() {
  document.body.classList.toggle('inthread', Boolean(at.id));
  crumbs(at.id ? 'thread' : '');
  kinds();
  $('kinds')
    .querySelector('[aria-current]')
    ?.scrollIntoView({ block: 'nearest', inline: 'center' });
  $('pill').addEventListener('click', show);
  document.addEventListener('keydown', key);
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) clearTimeout(timer);
    else read();
  });
  addEventListener('pageshow', e => e.persisted && read());
  setInterval(banner, 30e3);
  fetch('/auth/me', { cache: 'no-store' })
    .then(r => r.json())
    .then(me => {
      if (me.name) $('who').textContent = me.name;
      $('signout').hidden = !me.enabled;
    })
    .catch(() => {});
  read();
}
if (globalThis.document) boot();

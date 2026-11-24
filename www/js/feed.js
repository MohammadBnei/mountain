// The feed's page (feed.html, cmd/atlas/feed_page.go): a timeline of posts and a post's thread, used as X is and drawn
// as the game's strategy view is (feed.css). One document: a post opens its thread in place (history.pushState) and back
// finds the timeline where it was. It polls /api/feed (there is no stream) and never moves what is on screen by itself:
// a "N new posts" pill does, when pressed. Everything that comes from an event is text (ui.js el: createElement and
// textContent; feed-md.js for a session's Markdown); a link is built only from a thread's `links`, a claim's ref and a
// post's own Markdown links, and only to an https URL or a thread id.
import { httpsURL, markdown } from './feed-md.js';
import { gateLapsed, signedOut } from './socket.js';
import { el } from './ui.js';

// kind: its word, its tab, the prefix its titles carry, and the system's account that posts about it (name, handle, icon)
export const KINDS = {
  laptop: ['Laptop session', 'Laptop', 'Laptop: ', 'Laptop', 'laptop', 'person'],
  alert: ['Alert', 'Alerts', 'Alert: ', 'Alerts', 'alerts', 'bell'],
  deploy: ['Deploy', 'Deploys', 'Deploy: ', 'Deploys', 'deploys', 'sync'],
  pr: ['PR', 'PRs', 'PR ', 'Forge', 'forge', 'repo'],
  session: ['Session', 'Sessions', 'Session: ', 'Fleet', 'fleet', 'person'],
  card: ['Card', 'Cards', 'Card: ', 'Fleet', 'fleet', 'inbox']
};
// ukubi-game's line icons (cmd/game/websrc/icons.ts): one path each, on a 20 px grid
const ICONS = {
  bell: 'M5 14V9a5 5 0 0 1 10 0v5l1 1H4zM8 17h4',
  sync: 'M15 7a6 6 0 0 0-10 1M5 13a6 6 0 0 0 10-1M15 3v4h-4M5 17v-4h4',
  repo: 'M7 6l-4 4 4 4M13 6l4 4-4 4M11 4l-2 12',
  person: 'M10 3a3 3 0 1 0 0 6a3 3 0 1 0 0-6M4 17a6 6 0 0 1 12 0',
  inbox: 'M3 11h4l1 2h4l1-2h4M5 4h10l2 7v6H3v-6z',
  logs: 'M4 5h12M4 9h12M4 13h8M4 17h10',
  town: 'M3 9l7-5 7 5v8H3z',
  door: 'M5 17V3h10v14M3 17h14M12 10h0',
  back: 'M12 5l-5 5 5 5'
};
const FLEET = 'https://fleet.bnei.dev/';
const POLL = 15e3; // between reads while the page is visible; doubled after a failure, to a minute
const STALE = 6 * 3600e3; // no event for this long: the page says so
const SIDE_SHOWN = 5;
const POSTS_SHOWN = 60;
const VIEWS_KEPT = 20;

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

/** route is what a path shows: one thread, or a tab of the timeline (needs, all or a kind). */
export function route(path, search = '') {
  const id = /^\/feed\/([a-z]{1,16}-[0-9a-f]{16})$/.exec(path)?.[1];
  if (id) return { id };
  const q = new URLSearchParams(search),
    kind = q.get('kind') ?? '';
  return { tab: Object.hasOwn(KINDS, kind) ? kind : q.get('tab') === 'needs' ? 'needs' : 'all' };
}

const pathOf = tab =>
  tab === 'all' ? '/feed' : tab === 'needs' ? '/feed?tab=needs' : `/feed?kind=${tab}`;
const kindOf = id => id.split('-')[0];
const kindWord = kind => (Object.hasOwn(KINDS, kind) ? KINDS[kind][0] : clean(kind));

/** titleOf is a thread's title without the kind its account already says. */
export function titleOf(t) {
  const title = clean(t.title),
    prefix = Object.hasOwn(KINDS, t.kind) ? KINDS[t.kind][2] : '';
  return prefix && title.startsWith(prefix) ? title.slice(prefix.length) : title;
}

/** speaker is who posted entry e of thread t: the system's account for the kind, an agent by its position's name, the
 *  owner, or a laptop session (a Markdown entry: its own words). Quoted text has no speaker: it shows inside the post
 *  that relays it. */
export function speaker(e, t) {
  const who = clean(e.actor).trim(),
    first = s => ([...s][0] ?? '?').toUpperCase();
  if (e.markdown) {
    const name = clean(e.by).trim() || titleOf(t) || 'Session';
    return { who: 'session', name, handle: 'session', mark: first(name) };
  }
  if (who === 'owner') return { who, name: 'You', handle: 'owner', mark: 'Y' };
  if (who !== '' && who !== 'system')
    return { who: 'agent', name: who, handle: 'agent', mark: first(who) };
  const [, , , name, handle, icon] = Object.hasOwn(KINDS, t.kind)
    ? KINDS[t.kind]
    : ['', '', '', 'System', 'system', 'logs'];
  return { who: 'system', name, handle, icon };
}

/** posts groups a thread's entries: a quoted entry joins the system's post of the same moment it came with (its quoted
 *  card), else it is a post of the system's account holding only that card. A session's words and a claim stand alone.
 *  A login shows once: on the words when there are words. */
export function posts(t) {
  const out = [];
  (t.entries ?? []).forEach((e, i) => {
    const last = out[out.length - 1],
      system = !e.actor || e.actor === 'system';
    if (e.claim || e.markdown) out.push({ i, e, quotes: [] });
    else if (
      e.quoted &&
      system &&
      last &&
      !last.e.claim &&
      !last.e.markdown &&
      last.system &&
      last.e.time === e.time
    )
      last.quotes.push(e);
    else if (e.quoted) out.push({ i, e: { ...e, text: '', by: '' }, quotes: [e], system });
    else out.push({ i, e, quotes: [], system });
  });
  for (const p of out) if (p.quotes.some(q => q.by && q.by === p.e.by)) p.e = { ...p.e, by: '' };
  return out;
}

const LINKS = {
  previous: 'Previous episode',
  deploy: 'The rollout it caused',
  pull_request: 'The PR it shipped',
  generator: 'Alert source',
  run: 'CI run',
  revision: 'Revision'
};

/** linkOf is a link as the page may show it: {label, href, title, quoted, out, host, rest}; no href means text only. */
export function linkOf(l) {
  const ref = clean(l.ref),
    label = Object.hasOwn(LINKS, l.kind) ? LINKS[l.kind] : clean(l.kind);
  if (!l.quoted && /^[a-z]{1,16}-[0-9a-f]{16}$/.test(ref)) {
    const title = l.title ? titleOf({ kind: kindOf(ref), title: l.title }) : '';
    return { label, href: `/feed/${ref}`, title, quoted: Boolean(l.title_quoted) };
  }
  const u = httpsURL(ref);
  if (u)
    return {
      label,
      href: u.href,
      out: true,
      host: u.host,
      rest: u.pathname === '/' ? '' : u.pathname
    };
  return { label, rest: l.kind === 'revision' ? ref.slice(0, 12) : ref.slice(0, 200) };
}

/** said is a system line as a post says it after its subject: "health: Progressing -> Degraded" is "degraded". */
export const said = text => {
  const m = /^(reopened: )?health: \w+ -> (\w+)$/.exec(text);
  return m ? `${m[1] ? 'reopened, ' : ''}${m[2].toLowerCase()}` : text;
};

/** wholeOf is the text of entry e in its whole thread t: the same post, else the one of the same moment. */
export function wholeOf(t, e) {
  const id = e.post_id,
    es = t.entries ?? [];
  const f = id
    ? es.find(x => x.post_id === id && !x.claim)
    : es.find(x => x.time === e.time && x.text?.startsWith(e.text.slice(0, 200)));
  return f?.text ?? e.text;
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

/** isLong is whether a text is worth folding. */
export const isLong = s => s.length > 480 || s.split('\n').length > 8;

// ---- the page's parts

const $ = id => document.getElementById(id);
const plural = (n, w) => `${n} ${w}${n === 1 ? '' : 's'}`;

async function get(path) {
  const r = await fetch(path, { cache: 'no-store' });
  if (!r.ok) throw Object.assign(new Error(`${path}: ${r.status}`), { status: r.status });
  return r.json();
}

function icon(name) {
  const ns = 'http://www.w3.org/2000/svg',
    s = document.createElementNS(ns, 'svg'),
    p = document.createElementNS(ns, 'path');
  s.setAttribute('viewBox', '0 0 20 20');
  s.setAttribute('class', 'icon');
  s.setAttribute('aria-hidden', 'true');
  p.setAttribute('d', ICONS[name]);
  s.append(p);
  return s;
}

/** when is a relative time; the absolute one shows on hover and, in a thread, on focus or a tap. */
function when(iso, stop = false) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return el('span', { class: 'when' }, '?');
  const abs = d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' });
  const t = el(
    'time',
    { class: 'when', datetime: d.toISOString(), 'data-abs': abs, tabindex: stop ? '0' : null },
    el('span', { 'aria-hidden': 'true', 'data-t': d.getTime() }, rel(d.getTime())),
    el('span', { class: 'sr' }, abs)
  );
  if (stop) t.addEventListener('click', () => t.focus()); // a tap does not focus it by itself on iOS
  return t;
}

const avatar = s =>
  el(
    'span',
    { class: 'avatar', 'data-who': s.who, 'data-handle': s.handle, 'aria-hidden': 'true' },
    s.icon ? icon(s.icon) : s.mark
  );
const chip = (words, cls = '') => el('span', { class: `tag ${cls}` }, words);
const sr = words => el('span', { class: 'sr' }, words);
/** quotedBy is a login, in quotation marks: it is from outside too. */
const quotedBy = by =>
  by ? [' by ', el('q', { class: 'by', dir: 'auto' }, clean(by).slice(0, 64))] : null;

/** folded is text with the rest behind a button, when it is long. */
function folded(s, whole) {
  const t = clean(s),
    p = el('p', { class: 'text', dir: 'auto' }, t);
  if (!isLong(t)) return p;
  p.classList.add('clamp');
  return whole ? [p, more(p, 'clamp', `Show all ${t.length.toLocaleString()} characters`)] : p;
}

/** more is the button that unfolds a node: X's "Show more". */
function more(node, cls, words = 'Show more') {
  const b = el('button', { type: 'button', class: 'more', 'aria-expanded': 'false' }, words);
  b.addEventListener('click', e => {
    e.preventDefault();
    const open = node.classList.toggle(cls) === false;
    b.setAttribute('aria-expanded', String(open));
    b.textContent = open ? 'Show less' : words;
  });
  return b;
}

/** quoteCard is X's quoted post: what came from outside, as a quote, under whose words it is. */
function quoteCard(head, quotes, whole, by) {
  return el(
    'blockquote',
    { class: 'quoted' },
    el(
      'p',
      { class: 'qhead' },
      head,
      by ? [' · ', el('q', { class: 'by', dir: 'auto' }, clean(by).slice(0, 64))] : null
    ),
    quotes.map(q => folded(q, whole))
  );
}

/** words is a post's body: the system's line, a session's Markdown (its first lines in a timeline), an agent's text,
 *  and the quoted card of what came with it. In a list the server may have cut the text (has_more): "Show more" reads
 *  the thread for the whole of it. */
function words(p, whole, t) {
  const { e } = p,
    out = [];
  const body = (text, all) =>
    e.markdown
      ? [el('blockquote', { class: all ? 'own' : 'own fold' }, markdown(clean(text)))]
      : [folded(text, all)].flat();
  if (e.text) {
    const text = clean(e.text),
      cut = !whole && e.has_more;
    if (whole || !(cut || (e.markdown && (text.length > 320 || text.split('\n').length > 6))))
      out.push(...body(e.text, true));
    else {
      const box = el('div', {}, body(e.text, false));
      let full = cut ? null : body(e.text, true);
      const b = el(
        'button',
        { type: 'button', class: 'more', 'aria-expanded': 'false' },
        'Show more'
      );
      b.addEventListener('click', async ev => {
        ev.preventDefault();
        if (b.getAttribute('aria-expanded') === 'true') {
          box.replaceChildren(...body(e.text, false));
          b.setAttribute('aria-expanded', 'false');
          b.textContent = 'Show more';
          return;
        }
        try {
          full ??= body(wholeOf(await get(`/api/feed/${encodeURIComponent(t.id)}`), e), true);
        } catch (err) {
          b.textContent = 'Could not read it. Try again';
          return failed(err);
        }
        box.replaceChildren(...full);
        b.setAttribute('aria-expanded', 'true');
        b.textContent = 'Show less';
      });
      out.push(box, b);
    }
    if (e.by && out[0].tagName === 'P') out[0].append(...quotedBy(e.by));
  }
  if (p.quotes.length)
    out.push(
      quoteCard(
        'Quoted',
        p.quotes.map(q => q.text),
        whole,
        p.quotes.find(q => q.by)?.by
      )
    );
  return out;
}

/** subject is what a post is about: its thread's title, bold, in quotation marks (and said to be quoted) when it is
 *  from outside. */
function subject(t, max = 160) {
  const full = titleOf(t),
    title = full.length > max ? `${full.slice(0, max)}…` : full;
  return t.title_quoted
    ? [sr('quoted: '), el('q', { class: 'subject', dir: 'auto' }, title)]
    : el('strong', { class: 'subject', dir: 'auto' }, title);
}

/** postRef is the id of another post of a thread, as a link may name it. */
const postRef = id => (/^[\w-]{1,64}$/.test(clean(id)) ? clean(id) : '');
export const asks = e => Boolean(e.asks_owner || e.needs_list?.length);

/** mark is a chip, a link to another post of thread t when it names one. */
function mark(words, cls, t, id) {
  const to = postRef(id);
  return to
    ? el('a', { class: `tag ${cls}`, href: `/feed/${encodeURIComponent(t.id)}#${to}` }, words)
    : chip(words, cls);
}

function marks(e, t) {
  return [
    e.markdown ? chip('laptop session') : null,
    asks(e) ? chip('asks you', 'hot') : null,
    e.correction ? mark('correction', 'fix', t, e.corrects) : null,
    e.corrected_by ? mark('corrected', 'fix', t, e.corrected_by) : null
  ];
}

/** asked is who asks the owner, and what for, as the session itself puts it; on a later post of a thread whose ask
 *  still stands (needs_by), who asks. */
function asked(e, s, stands = false) {
  if (!asks(e) && !stands) return null;
  const list = Array.isArray(e.needs_list) ? e.needs_list.slice(0, 20) : [];
  return el(
    'div',
    { class: 'asked' },
    el(
      'p',
      {},
      'asked by ',
      el('q', { class: 'by', dir: 'auto' }, s.name.slice(0, 80)),
      ', in its own words',
      list.length ? ':' : ''
    ),
    list.length
      ? el(
          'ul',
          {},
          list.map(n => el('li', { dir: 'auto' }, clean(n).slice(0, 300)))
        )
      : null
  );
}

/** head is a post's first line: name, handle, time, and what is marked on it. */
function head(s, time, tags) {
  return el(
    'div',
    { class: 'head' },
    el('h3', { class: 'name', dir: 'auto' }, s.name.slice(0, 80)),
    s.who === 'session' ? sr(', a laptop session,') : null,
    el('span', { class: 'handle' }, `@${s.handle}`),
    el('span', { class: 'dotsep', 'aria-hidden': 'true' }, '·'),
    time,
    tags
  );
}

/** post is a thread in the timeline: its newest post, the whole of it opening the thread (through the time's link, as
 *  on X), but for its own links and buttons. */
function post(t) {
  const p = posts({ ...t, entries: (t.entries ?? []).slice(-1) })[0] ?? {
      e: { actor: 'system', text: '' },
      quotes: []
    },
    s = speaker(p.e, t),
    title = titleOf(t);
  const open = el(
    'a',
    { class: 'open', href: `/feed/${encodeURIComponent(t.id)}` },
    sr(`Open ${kindWord(t.kind)} ${title.slice(0, 120)}, `),
    when(t.last_activity_at)
  );
  // it leads with what it is about: "DiskFillingUp firing"; a session's own words are about themselves
  const stands = t.needs_by === 'session' && !p.e.claim,
    body = [];
  if (p.e.claim)
    body.push(el('p', { class: 'text' }, subject(t), ' mentioned by a laptop session'));
  else if (s.who === 'system') {
    const line = clean(said(p.e.text ?? ''));
    body.push(
      el(
        'p',
        { class: 'text', dir: 'auto' },
        subject(t),
        line ? ` ${line.slice(0, 400)}` : '',
        quotedBy(p.e.by)
      ),
      ...words({ ...p, e: { ...p.e, text: '' } }, false, t)
    );
  } else {
    if (s.who !== 'session') body.push(el('p', { class: 'text', dir: 'auto' }, subject(t)));
    body.push(...words(p, false, t));
  }
  return el(
    'li',
    { class: 'post', 'data-id': t.id, 'data-who': s.who },
    avatar(s),
    el(
      'div',
      { class: 'body' },
      head(s, open, [
        t.needs_owner && !asks(p.e) && !stands ? chip('needs you', 'hot') : null,
        ...marks(p.e, t)
      ]),
      asked(p.e, s, stands),
      body
    )
  );
}

/** reply is one post of a thread's page. */
function reply(p, t, isNew) {
  const { e } = p;
  if (e.claim) {
    const to = clean(e.thread),
      ref = /^laptop-[0-9a-f]{16}$/.test(to)
        ? `/feed/${to}${postRef(e.post_id) ? `#${postRef(e.post_id)}` : ''}`
        : null,
      by = e.by
        ? el('q', { class: 'by', dir: 'auto' }, clean(e.by).slice(0, 80))
        : 'a laptop session';
    return el(
      'li',
      { class: 'post claim', 'data-i': p.i, tabindex: '-1' },
      el('span', { class: 'bullet', 'aria-hidden': 'true' }),
      el(
        'p',
        { class: 'text' },
        'mentioned by ',
        ref ? el('a', { href: ref }, by) : by,
        ' ',
        when(e.time, true)
      )
    );
  }
  const s = speaker(e, t);
  return el(
    'li',
    {
      class: `post${isNew ? ' fresh' : ''}`,
      'data-who': s.who,
      'data-i': p.i,
      'data-post': postRef(e.post_id),
      tabindex: '-1'
    },
    avatar(s),
    el(
      'div',
      { class: 'body' },
      head(s, when(e.time, true), [s.who === 'agent' ? chip('agent') : null, ...marks(e, t)]),
      asked(e, s),
      words(p, true, t)
    )
  );
}

function linkCard(l) {
  const k = linkOf(l),
    title = k.title
      ? [
          k.quoted ? sr('quoted: ') : null,
          el('span', { class: k.quoted ? 'to qt' : 'to', dir: 'auto' }, k.title)
        ]
      : null;
  const inner = [
    el('span', { class: 'what', dir: 'auto' }, k.label),
    title,
    k.host ? el('span', { class: 'host' }, k.host) : null,
    k.rest ? el('span', { class: 'rest' }, k.rest) : null
  ];
  if (!k.href) return el('li', {}, el('span', { class: 'ref' }, inner));
  return el(
    'li',
    {},
    k.out
      ? el(
          'a',
          { class: 'ref', href: k.href, target: '_blank', rel: 'noopener noreferrer' },
          inner,
          sr(' (opens another site)'),
          el('span', { class: 'out', 'aria-hidden': 'true' }, '↗')
        )
      : el('a', { class: 'ref', href: k.href }, inner)
  );
}

function state(title, body, ...extra) {
  return el(
    'div',
    { class: 'state' },
    el('b', {}, title),
    el('span', {}, body),
    ...extra.map(m => el('div', {}, m))
  );
}

function button(words, f, cls = 'chip') {
  const b = el('button', { type: 'button', class: cls }, words);
  b.addEventListener('click', f);
  return b;
}

// ---- views: each route's own element and state, kept so that back finds it as it was

const views = new Map(); // path and search -> view
let cur = null;

function viewOf(url) {
  const key = url.pathname + url.search;
  let v = views.get(key);
  if (!v) {
    v = {
      key,
      ...route(url.pathname, url.search),
      el: el('div', { class: 'view' }),
      y: 0,
      pending: null,
      drawn: false,
      over: false
    };
    v.el.append(el('p', { class: 'state' }, 'Reading…'));
    if (v.tab) Object.assign(v, { shown: new Map(), next: '', tail: '' });
    else Object.assign(v, { n: 0, sig: '', all: false, name: '' });
  }
  views.delete(key); // the newest last
  views.set(key, v);
  for (const k of views.keys()) {
    if (views.size <= VIEWS_KEPT) break;
    views.delete(k);
  }
  return v;
}

// ---- a tab of the timeline

function listPath(v, cursor = '') {
  const q = new URLSearchParams({ limit: '50' });
  if (v.tab === 'needs') q.set('needs', '1');
  else if (v.tab !== 'all') q.set('kind', v.tab);
  if (cursor) q.set('cursor', cursor);
  return `/api/feed?${q}`;
}

const tabName = tab => (tab === 'needs' ? 'Needs you' : tab === 'all' ? 'All' : KINDS[tab][1]);

function drawList(v, d) {
  v.shown = new Map(d.threads.map(t => [t.id, sig(t)]));
  v.next = d.next_cursor;
  v.tail = v.next ? d.threads[d.threads.length - 1]?.last_activity_at : '';
  v.drawn = true;
  if (!d.threads.length) {
    const what =
      v.tab === 'needs'
        ? state('Nothing needs you', 'What waits for your answer shows up here.')
        : state(
            v.tab === 'all' ? 'Nothing yet' : `No ${tabName(v.tab).toLowerCase()} yet`,
            'What happens shows up here, newest first.'
          );
    return v.el.replaceChildren(what);
  }
  const ol = el(
      'ol',
      { class: 'posts', role: 'list', 'aria-label': `Posts: ${tabName(v.tab)}` },
      d.threads.map(post)
    ),
    foot = el('div', { class: 'foot' });
  v.el.replaceChildren(ol, foot);
  drawFoot(v, ol, foot, d);
}

function drawFoot(v, ol, foot, d) {
  if (!v.next) {
    return foot.replaceChildren(
      el(
        'p',
        {},
        d.dropped
          ? `The feed keeps 30 days at most; ${plural(d.dropped, 'older event')} made room for newer ones.`
          : 'That is everything. The feed keeps 30 days at most.'
      )
    );
  }
  const b = button('Load more', async () => {
    b.disabled = true;
    b.textContent = 'Loading…';
    try {
      const m = await get(listPath(v, v.next)),
        at = ol.children.length,
        rows = m.threads.filter(t => !v.shown.has(t.id)); // one that moved while paging is already above
      ol.append(...rows.map(post));
      for (const t of rows) v.shown.set(t.id, sig(t));
      v.next = m.next_cursor;
      v.tail = v.next ? m.threads[m.threads.length - 1]?.last_activity_at : '';
      drawFoot(v, ol, foot, m);
      ol.children[at]?.querySelector('.open').focus({ preventScroll: true });
    } catch (e) {
      b.disabled = false;
      b.textContent = 'Could not load more. Try again';
      failed(e);
    }
  });
  foot.replaceChildren(b);
}

// ---- a thread

/** jump goes to the post the address names (#id): a correction's, or the corrected one. */
function jump(v) {
  const id = postRef(location.hash.slice(1));
  if (!id || !v.t) return;
  const at = () => [...v.el.querySelectorAll('.post')].find(p => p.dataset.post === id);
  if (!at() && !v.all && (v.t.entries ?? []).some(e => e.post_id === id)) {
    v.all = true;
    drawThread(v, v.t);
  }
  at()?.focus({ preventScroll: true });
  at()?.scrollIntoView({ block: 'center' });
}

const keyOf = t => `${sig(t)}|${(t.entries ?? []).length}|${t.title}|${t.links?.length ?? 0}`;

function drawThread(v, t) {
  const ps = posts(t),
    from = v.all ? 0 : Math.max(0, ps.length - POSTS_SHOWN),
    old = v.n;
  v.n = (t.entries ?? []).length;
  v.sig = keyOf(t);
  v.t = t;
  v.drawn = true;
  v.name = `${t.needs_owner ? '(!) ' : ''}${titleOf(t).slice(0, 80)}`;
  const isSession = ps.some(p => p.e.markdown),
    s = speaker({ actor: 'system', markdown: isSession }, t),
    title = titleOf(t),
    long = title.length > 160;
  const name = el('p', { class: 'text about', dir: 'auto' }, subject(t, Infinity));
  if (long) name.classList.add('clamp');
  const main = el('div', {});
  main.append(
    name,
    ...(long
      ? [more(name, 'clamp', `Whole title, ${title.length.toLocaleString()} characters`)]
      : [])
  );
  let earlier = '';
  if (from) {
    earlier = button(
      `Show ${from} earlier`,
      () => {
        v.all = true;
        drawThread(v, t);
        v.el.querySelector('.thread .post')?.focus();
      },
      'chip wide'
    );
  }
  const links = [...(t.links ?? [])].map(linkCard);
  if (t.kind === 'session' || t.kind === 'card')
    links.push(linkCard({ kind: 'Fleet console', ref: FLEET, quoted: false }));
  const opened = new Date(t.opened_at);
  v.el.replaceChildren(
    el(
      'article',
      { class: 'main', 'aria-label': `${kindWord(t.kind)} thread` },
      el(
        'div',
        { class: 'mainhead' },
        avatar(s),
        el(
          'div',
          {},
          el('h2', { class: 'name', dir: 'auto' }, s.name.slice(0, 80)),
          el('span', { class: 'handle' }, `@${s.handle}`)
        ),
        el(
          'span',
          { class: 'tags' },
          isSession ? chip('laptop session') : null,
          t.needs_owner ? chip('needs you', 'hot') : null,
          chip(t.state === 'open' ? 'open' : 'closed')
        )
      ),
      isSession ? null : main,
      el(
        'p',
        { class: 'abs' },
        Number.isNaN(opened.getTime())
          ? 'opened ?'
          : el(
              'time',
              { datetime: opened.toISOString() },
              `${opened.toLocaleTimeString(undefined, { timeStyle: 'short' })} · ${opened.toLocaleDateString(undefined, { dateStyle: 'medium' })}`
            ),
        ` · ${kindWord(t.kind)}`
      ),
      links.length ? el('ul', { class: 'links', role: 'list', 'aria-label': 'Links' }, links) : null
    ),
    // where the reply box goes (slice 5)
    el(
      'div',
      { class: 'replybox', role: 'group', 'aria-label': 'Reply', 'aria-disabled': 'true' },
      avatar({ who: 'owner', handle: 'owner', mark: 'Y' }),
      el('span', {}, 'Replies come later')
    ),
    earlier,
    ps.length
      ? el(
          'ol',
          { class: 'posts thread', role: 'list', 'aria-label': 'Posts in this thread' },
          ps.slice(from).map(p => reply(p, t, old > 0 && p.i >= old))
        )
      : state('No posts', 'This thread holds nothing yet.')
  );
}

// ---- the frame: tabs, destinations, the side, the words about freshness

let age = null, // the last answer's age, and when it came
  wait = POLL,
  timer = 0,
  down = null, // the read failure on show
  stopped = false, // signed out: nothing more is read
  needs = null; // the last list of what needs the owner

function needsCount() {
  return needs ? `${needs.threads.length}${needs.next_cursor ? '+' : ''}` : '';
}

function frame() {
  const n = needsCount(),
    some = n !== '' && n !== '0';
  const tab = k => {
    const a = el(
      'a',
      { href: pathOf(k), 'aria-current': cur.tab === k ? 'page' : null },
      tabName(k)
    );
    if (k === 'needs' && some) a.append(el('span', { class: 'count' }, n));
    return el('li', {}, a);
  };
  $('tabs').hidden = Boolean(cur.id);
  if (!cur.id) {
    const at = $('tabs').scrollLeft,
      first = !$('tabs').firstChild;
    $('tabs').replaceChildren(
      el('ul', { role: 'list' }, ['needs', 'all', ...Object.keys(KINDS)].map(tab))
    );
    $('tabs').scrollLeft = at;
    const now = $('tabs').querySelector('[aria-current]');
    if ((first || $('tabs').dataset.tab !== cur.tab) && now)
      $('tabs').scrollLeft = now.offsetLeft - ($('tabs').clientWidth - now.offsetWidth) / 2;
    $('tabs').dataset.tab = cur.tab;
  }
  const dest = (href, words, ic, here, out) =>
    el(
      'a',
      {
        href,
        'aria-current': here ? 'page' : null,
        target: out ? '_blank' : null,
        rel: out ? 'noopener noreferrer' : null
      },
      el('span', { class: 'pill' }, icon(ic)),
      el('span', { class: 'label' }, words),
      words === 'Needs you' && some ? el('span', { class: 'badge' }, n) : null
    );
  $('dest').replaceChildren(
    dest('/feed', 'Feed', 'logs', cur.tab && cur.tab !== 'needs'),
    dest('/feed?tab=needs', 'Needs you', 'inbox', cur.tab === 'needs'),
    dest('/', 'Map', 'town', false),
    dest(FLEET, 'Fleet', 'door', false, true)
  );
  $('title').textContent = cur.id ? 'Post' : 'Feed';
  $('back').hidden = !cur.id;
  document.title = cur.id
    ? `${cur.name || 'Post'} · Feed`
    : `${some ? `(${n}) ` : ''}${cur.tab === 'all' ? 'Feed' : tabName(cur.tab)} · ukubi`;
  // the side: what needs the owner, but on its own tab
  const side = [];
  if (needs && cur.tab !== 'needs') {
    side.push(el('h2', {}, 'Needs you'));
    if (some) {
      side.push(
        el(
          'ul',
          { role: 'list' },
          needs.threads
            .slice(0, SIDE_SHOWN)
            .map(t =>
              el(
                'li',
                {},
                el(
                  'a',
                  { href: `/feed/${encodeURIComponent(t.id)}` },
                  t.title_quoted ? sr('quoted: ') : null,
                  el(
                    'span',
                    { class: t.title_quoted ? 'qt clamp' : 'clamp', dir: 'auto' },
                    titleOf(t)
                  ),
                  el('small', {}, kindWord(t.kind), ' · ', rel(Date.parse(t.last_activity_at)))
                )
              )
            )
        )
      );
      if (needs.threads.length > SIDE_SHOWN)
        side.push(el('a', { class: 'all', href: '/feed?tab=needs' }, `Show all ${n}`));
    } else side.push(el('p', { class: 'unknown' }, 'nothing needs you'));
  }
  $('summary').replaceChildren(...side);
}

const span = ms =>
  ms < 3600e3
    ? plural(Math.max(1, Math.round(ms / 60e3)), 'minute')
    : ms < 48 * 3600e3
      ? plural(Math.round(ms / 3600e3), 'hour')
      : plural(Math.round(ms / 86400e3), 'day');

const aged = now =>
  age.v?.last_event_age == null ? null : age.v.last_event_age + (now - age.at) / 1000;

/** banner says what makes the page less than current: a failed read, a feed with nothing in it, one with nothing new
 *  for long. Its text changes by the minute at most, so a screen reader hears it once. */
function banner() {
  const now = Date.now(),
    last = age && lastEvent({ ...age.v, last_event_age: aged(now) }, now);
  let msg = '',
    quiet = false;
  if (down && cur.drawn)
    msg = `Not updating: the last answer came ${span(now - age.at)} ago. Trying again.`;
  else if (age && last === null)
    msg =
      'The feed has had no event since it started. It may still be reading, or the bridge may be out of reach.';
  else if (age && now - last > STALE) {
    msg = `Last event ${span(now - last)} ago`;
    quiet = true;
  }
  if ($('banner').dataset.msg !== msg) {
    $('banner').dataset.msg = msg;
    $('banner').replaceChildren(msg ? el('p', { class: quiet ? 'quiet' : '' }, msg) : '');
  }
  $('fresh').textContent =
    last && now - last <= STALE ? `Last event ${rel(last, now)} ago`.replace('now ago', 'now') : '';
  for (const s of document.querySelectorAll('[data-t]'))
    s.textContent = rel(Number(s.dataset.t), now);
}

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
  stopped = true;
  $('view').replaceChildren(state('Signed out', 'Reload to sign in again.'));
  $('pill').hidden = true;
  return true;
}

function pill(words) {
  $('pill').textContent = words;
  $('pill').hidden = !words;
}

/** show draws what is waiting behind the pill: a timeline from its top, a thread at its first new post. */
function show() {
  const v = cur,
    d = v.pending;
  if (!d) return;
  v.pending = null;
  pill('');
  if (v.id) {
    const old = v.n;
    drawThread(v, d);
    const at = [...v.el.querySelectorAll('.thread .post')].find(p => Number(p.dataset.i) >= old);
    at?.focus({ preventScroll: true });
    at?.scrollIntoView({ block: 'center' });
  } else {
    drawList(v, d);
    toTop();
    v.el.querySelector('.open')?.focus({ preventScroll: true });
  }
  frame();
}

const toTop = () =>
  scrollTo({
    top: 0,
    behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'auto'
  });

async function read() {
  clearTimeout(timer);
  if (stopped || !cur || cur.over) return;
  const v = cur;
  try {
    const needsPath = '/api/feed?needs=1&limit=50';
    const [d, n] = await Promise.all([
      get(v.id ? `/api/feed/${v.id}` : listPath(v)),
      v.tab === 'needs' ? null : get(needsPath)
    ]);
    if (v !== cur) return; // the reader went on: that view reads for itself
    needs = n ?? d;
    age = { v: d.age, at: Date.now() };
    down = null;
    wait = POLL;
    if (!v.drawn) {
      if (v.id) {
        drawThread(v, d);
        jump(v);
      } else drawList(v, d);
    } else if (v.id) {
      const more = (d.entries ?? []).length - v.n;
      v.pending = keyOf(d) === v.sig ? null : d;
      pill(!v.pending ? '' : more > 0 ? plural(more, 'new post') : 'Changed · show'); // else only its head, or older entries gone
    } else {
      const now = new Set(d.threads.map(t => t.id)),
        n = fresh(v.shown, d.threads, v.tail || ''),
        gone =
          v.tab === 'needs' ? [...v.shown.keys()].filter(id => !now.has(id) && !v.next).length : 0;
      v.pending = n + gone ? d : null;
      if (v.pending && !v.shown.size)
        show(); // an empty page has nothing to move
      else pill(n ? plural(n, 'new post') : gone ? 'Changed · show' : '');
    }
    frame();
    banner();
  } catch (e) {
    if (v !== cur || (await lapsed(e))) return;
    wait = Math.min(60e3, wait * 2);
    failed(e);
    if (e.status === 404 && v.id) {
      Object.assign(v, { over: true, drawn: true });
      down = null;
      banner();
      v.el.replaceChildren(
        state(
          'Not in the feed',
          'This thread does not exist, or is older than what the feed keeps.',
          el('a', { class: 'chip', href: '/feed' }, 'Back to the feed')
        )
      );
      return;
    }
    if (!v.drawn)
      v.el.replaceChildren(
        state('Could not read the feed', 'Trying again by itself.', button('Try again', read))
      );
  }
  if (!document.hidden) timer = setTimeout(read, wait);
}

// ---- going from one view to another, without a page load

/** go shows url. On the way back the view is where it was, and the post that was opened has the focus. */
function go(url, back = false) {
  const from = cur;
  if (from) from.y = scrollY;
  cur = viewOf(url);
  if (!back) {
    cur.y = 0;
    if (cur.tab && cur.pending) drawList(cur, cur.pending); // coming anew, not back: what was waiting shows
    cur.pending = null;
  }
  $('view').replaceChildren(cur.el);
  pill('');
  document.body.classList.toggle('inthread', Boolean(cur.id));
  frame();
  scrollTo({ top: cur.y, behavior: 'instant' });
  if (from) {
    const opened = back && from.id && cur.el.querySelector(`[data-id="${from.id}"] .open`);
    (opened || $('title')).focus({ preventScroll: true });
  }
  read();
}

function onClick(e) {
  if (e.defaultPrevented || e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const a = e.target.closest?.('a[href]');
  if (!a || a.target) return;
  const url = new URL(a.href);
  if (url.origin !== location.origin || !/^\/feed(\/|$)/.test(url.pathname)) return;
  e.preventDefault();
  if (url.pathname + url.search === cur.key && url.hash) {
    history.replaceState(history.state, '', url);
    return jump(cur);
  }
  if (url.pathname + url.search === cur.key) {
    // the active tab, as on X: to the top, with what is new
    toTop();
    return cur.pending ? show() : read();
  }
  history.pushState({ in: true }, '', url);
  go(url);
}

/** keys: X's. j and k walk the posts, . shows what is new, u goes back; Enter is the link's own. */
function key(e) {
  if (e.metaKey || e.ctrlKey || e.altKey || e.target.closest?.('input, textarea, select')) return;
  if (e.key === '.' && cur.pending) return show();
  if (e.key === 'u' && cur.id) return goBack(e);
  if (e.key !== 'j' && e.key !== 'k') return;
  const all = [...cur.el.querySelectorAll(cur.id ? '.thread .post' : '.post .open')];
  const i = all.findIndex(x => x === document.activeElement || x.contains(document.activeElement));
  const to = all[i < 0 ? 0 : Math.min(all.length - 1, Math.max(0, i + (e.key === 'j' ? 1 : -1)))];
  to?.focus({ preventScroll: true });
  (to?.closest('.post') ?? to)?.scrollIntoView({ block: 'nearest' });
}

function goBack(e) {
  if (!history.state?.in) return e.type === 'click' ? undefined : $('back').click();
  e.preventDefault();
  history.back();
}

function boot() {
  history.scrollRestoration = 'manual';
  $('back').append(icon('back'));
  $('back').addEventListener('click', goBack);
  $('pill').addEventListener('click', show);
  document.addEventListener('click', onClick);
  document.addEventListener('keydown', key);
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) clearTimeout(timer);
    else read();
  });
  addEventListener('popstate', () => go(new URL(location.href), true));
  addEventListener('pageshow', e => e.persisted && read());
  setInterval(banner, 30e3);
  fetch('/auth/me', { cache: 'no-store' })
    .then(r => r.json())
    .then(me => {
      if (me.name) $('who').textContent = me.name;
      $('signout').hidden = !me.enabled;
    })
    .catch(() => {});
  go(new URL(location.href));
}
if (globalThis.document) boot();

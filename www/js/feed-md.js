// A small Markdown subset for a session's posts (feed.js), from text that is hostile until shown otherwise. Built with
// createElement and textContent only (ui.js el), so nothing in it is ever HTML: paragraphs, bold, italic, code spans and
// fences, lists (one nested level), tables, blockquotes, headings (a bold line), rules, and [text](https://…) links with
// their host in sight. Images, raw HTML, bare URLs and anything else stay the text they are. Every step is bounded, so
// the time is linear in the text: depth is capped (QUOTES, DEPTH), a link's two parts have a length, a code span's
// closing run is looked up, never searched for again.
import { el } from './ui.js';

const DEPTH = 6, // open bold and italic marks at once; more are text
  QUOTES = 2, // blockquotes in blockquotes
  COLS = 12; // a table row's cells: what follows stays in the last one

/** httpsURL is ref as a URL the page may link to: https, a host, no user info; else null. */
export function httpsURL(ref) {
  if (!/^https:\/\//i.test(ref)) return null;
  try {
    const u = new URL(ref);
    return u.protocol === 'https:' && !u.username && !u.password ? u : null;
  } catch {
    return null;
  }
}

// ---- inline: one pass, a stack of open marks

const LINK = /\[([^[\]\n]{1,200})\]\(([^()\s<>]{1,500})\)/y;
const word = c => c !== undefined && /[\p{L}\p{N}_]/u.test(c);

/** inline is s as a tree of {tag, kids} and strings. Pure, so it is tested without a DOM. */
export function inline(s) {
  const root = { tag: '', kids: [] },
    stack = [root];
  let text = '';
  const top = () => stack[stack.length - 1];
  const flush = () => {
    if (text) top().kids.push(text);
    text = '';
  };
  // unopened: a mark that never closed is its own characters, its content its parent's
  const spill = () => {
    flush();
    const f = stack.pop();
    top().kids.push(f.mark, ...f.kids);
  };
  let runs = null; // code runs: per length, where each starts, in order
  const closing = (from, n) => {
    if (!runs) {
      runs = new Map();
      for (const m of s.matchAll(/`+/g)) {
        const l = m[0].length;
        if (!runs.has(l)) runs.set(l, { at: [], i: 0 });
        runs.get(l).at.push(m.index);
      }
    }
    const r = runs.get(n);
    while (r.i < r.at.length && r.at[r.i] < from) r.i++;
    return r.i < r.at.length ? r.at[r.i] : -1;
  };
  for (let i = 0; i < s.length; ) {
    const c = s[i];
    if (c === '\\' && i + 1 < s.length && /[\\`*_[\]()#|>!~-]/.test(s[i + 1])) {
      text += s[i + 1];
      i += 2;
      continue;
    }
    if (c === '`') {
      let n = 1;
      while (s[i + n] === '`') n++;
      const end = closing(i + n, n);
      if (end >= 0) {
        flush();
        top().kids.push({ tag: 'code', kids: [s.slice(i + n, end).trim() || s.slice(i + n, end)] });
        i = end + n;
      } else {
        text += s.slice(i, i + n);
        i += n;
      }
      continue;
    }
    if (c === '[') {
      LINK.lastIndex = i;
      const m = LINK.exec(s);
      if (m) {
        const u = s[i - 1] === '!' ? null : httpsURL(m[2]);
        if (u) {
          flush();
          top().kids.push({ tag: 'a', href: u.href, host: u.host, kids: [m[1]] });
        } else text += m[0];
        i += m[0].length;
        continue;
      }
    }
    if (c === '*' || c === '_') {
      const n = s[i + 1] === c ? 2 : 1,
        mark = s.slice(i, i + n),
        before = s[i - 1],
        after = s[i + n];
      const canClose =
        before !== undefined && !/\s/.test(before) && !(c === '_' && word(after)) && i > 0;
      const canOpen = after !== undefined && !/\s/.test(after) && !(c === '_' && word(before));
      let at = -1;
      if (canClose)
        for (let k = stack.length - 1; k > 0 && at < 0; k--) if (stack[k].mark === mark) at = k;
      if (at > 0 && (stack[at].kids.length || text)) {
        while (stack.length - 1 > at) spill();
        flush();
        const f = stack.pop();
        top().kids.push({ tag: f.tag, kids: f.kids });
      } else if (canOpen && stack.length <= DEPTH) {
        flush();
        stack.push({ tag: n === 2 ? 'strong' : 'em', mark, kids: [] });
      } else text += mark;
      i += n;
      continue;
    }
    text += c;
    i++;
  }
  while (stack.length > 1) spill();
  flush();
  return root.kids;
}

// ---- blocks: one pass over the lines

const FENCE = /^ {0,3}(`{3,}|~{3,})/,
  HEAD = /^ {0,3}#{1,6}\s+(.*)$/,
  RULE = /^ {0,3}(?:(?:-\s?){3,}|(?:\*\s?){3,}|(?:_\s?){3,})$/,
  ITEM = /^( {0,9})(?:([-*+])|(\d{1,9})[.)])\s+(.*)$/,
  QUOTE = /^ {0,3}>\s?(.*)$/,
  DIVIDER = /^[\s|:-]+$/;

const isRule = l => l.length < 100 && RULE.test(l.trimEnd()); // long ones are items or text

const divider = l => l?.includes('|') && l.includes('-') && DIVIDER.test(l) && l.length < 400;

/** cells is a table row's cells. An escaped or coded bar is not a separator's job here: a bar always separates. */
function cells(l) {
  const out = l.trim().replace(/^\|/, '').replace(/\|$/, '').split('|');
  return out.length > COLS
    ? [...out.slice(0, COLS - 1), out.slice(COLS - 1).join('|')].map(c => c.trim())
    : out.map(c => c.trim());
}

/** blocks is s as a list of blocks: {t: p|h|hr|code|quote|list|table, …}. Pure. */
export function blocks(s, depth = 0) {
  const ls = s.split('\n'),
    out = [];
  let para = [];
  const end = () => {
    if (para.length) out.push({ t: 'p', text: para.join('\n') });
    para = [];
  };
  for (let i = 0; i < ls.length; i++) {
    const l = ls[i],
      fence = FENCE.exec(l),
      head = HEAD.exec(l),
      quote = QUOTE.exec(l),
      item = ITEM.exec(l);
    if (!l.trim()) {
      end();
    } else if (fence) {
      end();
      const mark = fence[1][0],
        n = fence[1].length,
        body = [];
      for (i++; i < ls.length; i++) {
        const f = FENCE.exec(ls[i]);
        if (f && f[1][0] === mark && f[1].length >= n && ls[i].trim().length === f[1].length) break;
        body.push(ls[i]);
      }
      out.push({ t: 'code', text: body.join('\n') });
    } else if (head) {
      end();
      out.push({ t: 'h', text: head[1] });
    } else if (isRule(l)) {
      end();
      out.push({ t: 'hr' });
    } else if (quote && depth < QUOTES) {
      end();
      const body = [quote[1]];
      for (; i + 1 < ls.length; i++) {
        const q = QUOTE.exec(ls[i + 1]);
        if (!q) break;
        body.push(q[1]);
      }
      out.push({ t: 'quote', kids: blocks(body.join('\n'), depth + 1) });
    } else if (l.includes('|') && divider(ls[i + 1])) {
      end();
      const cols = cells(l),
        rows = [];
      i++;
      while (i + 1 < ls.length && ls[i + 1].includes('|') && ls[i + 1].trim()) {
        rows.push(cells(ls[i + 1]));
        i++;
      }
      out.push({ t: 'table', head: cols, rows });
    } else if (item) {
      end();
      const list = { t: 'list', ordered: !item[2], start: Number(item[3] ?? 1), items: [] };
      const base = item[1].length;
      for (; i < ls.length; i++) {
        const n = ITEM.exec(ls[i]);
        const last = list.items[list.items.length - 1];
        if (n && n[1].length > base + 1 && last) {
          last.sub ??= { t: 'list', ordered: !n[2], start: Number(n[3] ?? 1), items: [] };
          last.sub.items.push({ text: [n[4]] });
        } else if (n && !isRule(ls[i])) {
          if (!n[2] !== list.ordered) break; // the other sort of list starts here
          list.items.push({ text: [n[4]] });
        } else if (ls[i].trim() && /^\s{2,}/.test(ls[i]) && last) {
          (last.sub ? last.sub.items[last.sub.items.length - 1] : last).text.push(ls[i].trim());
        } else break;
      }
      i--;
      out.push(list);
    } else para.push(l);
  }
  end();
  return out;
}

// ---- the DOM

const spans = s =>
  inline(s).map(function node(n) {
    if (typeof n === 'string') return n;
    if (n.tag === 'a')
      return el(
        'a',
        { href: n.href, target: '_blank', rel: 'noopener noreferrer' },
        ...n.kids,
        el('span', { class: 'host' }, ` ${n.host}`)
      );
    return el(n.tag, {}, n.kids.map(node));
  });

const listOf = b =>
  el(
    b.ordered ? 'ol' : 'ul',
    { role: 'list', start: b.ordered && b.start !== 1 ? b.start : null },
    b.items.map(it => el('li', {}, spans(it.text.join('\n')), it.sub ? listOf(it.sub) : null))
  );

const row = (tag, cs) =>
  el(
    'tr',
    {},
    cs.map(c => el(tag, { dir: 'auto' }, spans(c)))
  );

function blockOf(b) {
  if (b.t === 'p') return el('p', { dir: 'auto' }, spans(b.text));
  if (b.t === 'h') return el('p', { class: 'mdh', dir: 'auto' }, el('strong', {}, spans(b.text)));
  if (b.t === 'hr') return el('hr');
  if (b.t === 'code')
    return el('pre', { tabindex: '0', 'aria-label': 'Code' }, el('code', {}, b.text));
  if (b.t === 'quote') return el('blockquote', {}, b.kids.map(blockOf));
  if (b.t === 'list') return listOf(b);
  // wider than the post: it scrolls inside it, by touch and by keyboard
  return el(
    'div',
    { class: 'mdtable', tabindex: '0', role: 'region', 'aria-label': 'Table' },
    el(
      'table',
      {},
      el('thead', {}, row('th', b.head)),
      el(
        'tbody',
        {},
        b.rows.map(r => row('td', r))
      )
    )
  );
}

/** markdown is s (already cleaned of control characters) as elements. */
export const markdown = s => el('div', { class: 'md' }, blocks(s).map(blockOf));

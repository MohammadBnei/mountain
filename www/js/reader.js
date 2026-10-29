// L4, the reader (plan/10 "The ladder", "Rendering rules"): a file atlas holds, at the sha it read it, in a full
// reading view. GET /api/read hands back blocks and inlines (markdown, parsed and redacted by atlas) or lines (any
// other text); this module builds them with createElement and textContent only, links included: an in-atlas link is a
// hash route, an external one opens elsewhere with no referrer, and there is no third kind. Highlighting is by class
// names only (the CSP is style-src 'self').

import { adrOfDoc, groupOf, label } from './model.js';
import { format } from './router.js';
import { el } from './ui.js';

const KEYWORDS = new Set(
  (
    'func package import return if else for range switch case default type struct interface map chan go defer var const ' +
    'nil true false let function export class new await async try catch throw def from as in not and or None True False ' +
    'apiVersion kind metadata spec'
  ).split(' ')
);

// A line longer than this is shown plain: the string pattern backtracks on an unterminated quote, quadratic in the
// line (a 160 K line of escaped quotes took 14 s), and nobody reads a 2000-character line for its colours.
const MAX_HIGHLIGHT = 2000;

/** tokens splits a line of code into [class, text] pairs: comments, strings, numbers, keywords. Pure. */
export function tokens(line, lang) {
  if (line.length > MAX_HIGHLIGHT) return [['', line]];
  const out = [],
    hashComment = ['yaml', 'sh', 'py', 'toml', 'ini', 'docker', 'hcl'].includes(lang);
  const re =
    /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`[^`]*`)|(\/\/.*$|#.*$)|\b(\d+(?:\.\d+)?)\b|\b([A-Za-z_][A-Za-z0-9_]*)\b/g;
  let last = 0,
    m;
  for (m = re.exec(line); m; m = re.exec(line)) {
    if (m[2] && m[2].startsWith('#') && !hashComment) {
      continue;
    }
    if (m[2] && m[2].startsWith('//') && hashComment && lang !== 'hcl') {
      continue;
    }
    if (m.index > last) out.push(['', line.slice(last, m.index)]);
    if (m[1]) out.push(['s', m[1]]);
    else if (m[2]) {
      out.push(['c', line.slice(m.index)]);
      last = line.length;
      break;
    } else if (m[3]) out.push(['n', m[3]]);
    else out.push([KEYWORDS.has(m[4]) ? 'k' : '', m[4]]);
    last = m.index + m[0].length;
  }
  if (last < line.length) out.push(['', line.slice(last)]);
  return out;
}

function codeEl(text, lang) {
  return el(
    'pre',
    { class: `code${lang ? ` lang-${lang.replace(/[^a-z0-9-]/gi, '')}` : ''}` },
    el(
      'code',
      {},
      text
        .split('\n')
        .map((line, i) => [
          i ? '\n' : null,
          ...tokens(line, lang).map(([c, t]) => (c ? el('span', { class: `t-${c}` }, t) : t))
        ])
    )
  );
}

function inlines(list, ctx) {
  return (list || []).map(x => {
    switch (x.t) {
      case 'text':
        return x.text;
      case 'br':
        return el('br');
      case 'code':
        return el('code', {}, x.text);
      case 'strong':
        return el('strong', {}, inlines(x.in, ctx));
      case 'em':
        return el('em', {}, inlines(x.in, ctx));
      case 'del':
        return el('del', {}, inlines(x.in, ctx));
      case 'link': {
        if (x.doc) {
          const i = x.doc.indexOf('/');
          return el(
            'a',
            {
              href: format({
                level: 4,
                doc: { repo: x.doc.slice(0, i), path: x.doc.slice(i + 1) },
                from: ctx.from,
                lens: ctx.lens
              })
            },
            inlines(x.in, ctx)
          );
        }
        if (/^https?:\/\//.test(x.href || ''))
          return el(
            'a',
            { href: x.href, target: '_blank', rel: 'noopener noreferrer' },
            inlines(x.in, ctx)
          );
        return el('span', {}, inlines(x.in, ctx));
      }
      default:
        return null;
    }
  });
}

function blocks(list, ctx) {
  return (list || []).map(b => {
    switch (b.t) {
      case 'h':
        return el(`h${Math.min(6, Math.max(1, b.level))}`, {}, inlines(b.in, ctx));
      case 'p':
        return el('p', {}, inlines(b.in, ctx));
      case 'list':
        return el(
          b.ordered ? 'ol' : 'ul',
          {},
          (b.items || []).map(it => el('li', {}, blocks(it, ctx)))
        );
      case 'code':
        return codeEl(b.text || '', b.lang);
      case 'quote':
        return el('blockquote', {}, blocks(b.blocks, ctx));
      case 'hr':
        return el('hr');
      case 'table':
        return el(
          'div',
          { class: 'tablewrap' },
          el(
            'table',
            {},
            (b.rows || []).map((row, i) =>
              el(
                'tr',
                {},
                row.map(cell => el(i ? 'td' : 'th', {}, inlines(cell, ctx)))
              )
            )
          )
        );
      default:
        return null;
    }
  });
}

const NOT_HELD =
  'atlas does not hold this file at the sha it read (moved, renamed, or a repo it does not read).';
const READ_SAYS = {
  'read.repo': NOT_HELD,
  'read.file': NOT_HELD,
  'read.too_large': 'The file is larger than the reader reads.',
  'read.not_text': 'Not a text file.'
};

/** readError is what the reader says for a failed /api/read: by the key of atlas's error body (cmd/atlas/errkeys.go),
 * else by its status (a body that is not the model's: a proxy's page, the gate's plain-text 403). */
export function readError(status, body) {
  return (
    (Object.hasOwn(READ_SAYS, body?.key ?? '') && READ_SAYS[body.key]) ||
    (status === 404 && NOT_HELD) ||
    `atlas answered ${status}.`
  );
}

/**
 * view builds the reader for a fetched doc (or an error), with the side column: where it was read, what governs or
 * is governed by it, and the way back.
 */
export function readerView(m, v, doc, error) {
  const ctx = { from: v.from, lens: v.lens };
  const back = v.from
    ? el(
        'a',
        { href: format({ level: 3, id: v.from, lens: v.lens }), class: 'back' },
        `Back to ${label(m.nodes.get(v.from)) || v.from}`
      )
    : el('a', { href: format({ level: 0, lens: v.lens }), class: 'back' }, 'Back to the map');
  if (error)
    return el(
      'div',
      { class: 'reader' },
      el('article', {}, el('h1', {}, v.doc.path), el('p', {}, error)),
      el('aside', {}, back)
    );
  const adr = adrOfDoc(m, doc);
  const body =
    doc.kind === 'md' ? blocks(doc.blocks, ctx) : codeEl((doc.lines || []).join('\n'), doc.lang);
  const cited = adr
    ? (m.inn.get(adr.id) || [])
        .filter(e => e.kind === 'governs')
        .sort((a, b) => Number(b.attrs?.count || 1) - Number(a.attrs?.count || 1))
    : [];
  return el(
    'div',
    { class: 'reader' },
    el(
      'article',
      {},
      el(
        'p',
        { class: 'where mono' },
        `${doc.repo}/${doc.path} @ ${doc.sha.slice(0, 12)}`,
        doc.redacted ? ` · ${doc.redacted} redacted` : ''
      ),
      body
    ),
    el(
      'aside',
      {},
      el('div', { class: 'still', id: 'readstill' }),
      adr
        ? el(
            'section',
            {},
            el('h3', {}, 'Governs'),
            cited.length
              ? el(
                  'ul',
                  { class: 'list' },
                  cited
                    .slice(0, 12)
                    .map(e =>
                      el(
                        'li',
                        {},
                        el(
                          'a',
                          { href: hrefFor(m, v, e.from) },
                          label(m.nodes.get(e.from)) || e.from
                        ),
                        el('span', { class: 'meta' }, ` ${e.attrs?.count || 1}×`)
                      )
                    )
                )
              : el(
                  'p',
                  { class: 'meta' },
                  'No file cites it: a decision with no code atlas can see.'
                )
          )
        : null,
      el(
        'p',
        {},
        el(
          'a',
          {
            href: `https://github.com/MohammadBnei/${encodeURIComponent(doc.repo)}/blob/${doc.sha}/${doc.path.split('/').map(encodeURIComponent).join('/')}`,
            target: '_blank',
            rel: 'noopener noreferrer'
          },
          'On GitHub'
        )
      ),
      back
    )
  );
}

function hrefFor(m, v, id) {
  const g = groupOf(m, id);
  return format(g ? { level: 3, id, lens: v.lens } : { level: 0, sel: id, lens: v.lens });
}

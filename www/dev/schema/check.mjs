#!/usr/bin/env node
// Validates frames against www/dev/schema/*.json (no deps: a validator for the JSON Schema subset used here).
//   node www/dev/schema/check.mjs [frame.json ...]
// Default: fixtures/hello.json (from `relay dump-hello`) must pass, fixtures/frames.json valid[] must pass and
// invalid[] must each fail. Exit 0 = pass. Usable as a module: import { validate } from './check.mjs'.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const DIR = dirname(fileURLToPath(import.meta.url));
const schemas = Object.fromEntries(['hello', 'ev', 'metrics', 'world'].map((n) => [n, JSON.parse(readFileSync(join(DIR, `${n}.json`), 'utf8'))]));
const typeOf = (v) => (v === null ? 'null' : Array.isArray(v) ? 'array' : Number.isInteger(v) ? 'integer' : typeof v);

function check(s, v, root, path, errs) {
  if (s.$ref) {
    const [file, frag] = s.$ref.split('#');
    const r = file ? schemas[file.replace('.json', '')] : root;
    check(frag ? frag.split('/').slice(1).reduce((o, k) => o[k], r) : r, v, r, path, errs);
  }
  const t = typeOf(v), at = (m) => errs.push(`${path || '/'}: ${m}`);
  if (s.type && ![].concat(s.type).some((x) => x === t || (x === 'number' && t === 'integer'))) return at(`type ${t}, want ${s.type}`);
  if ('const' in s && v !== s.const) at(`${JSON.stringify(v)} != const ${JSON.stringify(s.const)}`);
  if (s.enum && !s.enum.includes(v)) at(`${JSON.stringify(v)} not in enum`);
  if (t === 'string') {
    if (s.pattern && !new RegExp(s.pattern, 'u').test(v)) at(`"${v.slice(0, 40)}" !~ ${s.pattern}`);
    if (s.minLength !== undefined && v.length < s.minLength) at(`length ${v.length} < ${s.minLength}`);
    if (s.maxLength !== undefined && v.length > s.maxLength) at(`length ${v.length} > ${s.maxLength}`);
  }
  if ((t === 'number' || t === 'integer') && ((s.minimum !== undefined && v < s.minimum) || (s.maximum !== undefined && v > s.maximum))) at(`${v} out of [${s.minimum}, ${s.maximum}]`);
  if (t === 'array') {
    if (s.minItems !== undefined && v.length < s.minItems) at(`${v.length} items < ${s.minItems}`);
    if (s.maxItems !== undefined && v.length > s.maxItems) at(`${v.length} items > ${s.maxItems}`);
    if (s.items) v.forEach((x, i) => check(s.items, x, root, `${path}/${i}`, errs));
  }
  if (t === 'object') {
    for (const k of s.required || []) if (!(k in v)) at(`missing ${k}`);
    for (const [k, x] of Object.entries(v)) {
      if (s.propertyNames) check(s.propertyNames, k, root, `${path}/${k} (name)`, errs);
      if (s.properties?.[k]) check(s.properties[k], x, root, `${path}/${k}`, errs);
      else if (s.additionalProperties === false) at(`extra property ${k}`);
      else if (typeof s.additionalProperties === 'object') check(s.additionalProperties, x, root, `${path}/${k}`, errs);
    }
  }
  if (s.oneOf) {
    const ok = s.oneOf.filter((b) => { const e = []; check(b, v, root, path, e); return !e.length; }).length;
    if (ok !== 1) at(`matches ${ok} of oneOf (want 1)`);
  }
}

/** validate(frame) → [] or error strings; the schema is picked by frame.t. */
export function validate(f) {
  const s = schemas[f?.t];
  if (!s) return [`unknown frame t=${f?.t}`];
  const errs = [];
  check(s, f, s, '', errs);
  return errs;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  let bad = 0;
  const report = (name, errs, wantOk = true) => {
    const ok = wantOk ? !errs.length : errs.length > 0;
    if (!ok) bad++;
    console.log(`${ok ? 'ok  ' : 'BAD '} ${name}${wantOk ? '' : ' (must fail)'}${errs.length ? ': ' + errs.slice(0, 3).join('; ') : ''}`);
  };
  const files = process.argv.slice(2);
  if (files.length) for (const f of files) report(f, validate(JSON.parse(readFileSync(f, 'utf8'))));
  else {
    const fx = join(DIR, '../fixtures');
    const hello = JSON.parse(readFileSync(join(fx, 'hello.json'), 'utf8'));
    report('fixtures/hello.json', validate(hello));
    const { decodeI16, createWorld } = await import('../../js/world.js');
    const base = decodeI16(hello.snapshot.base);
    const al = { fp: '0123456789abcdef', sev: 'critical', layer: 'k8s', startsAt: 1760450000000 };
    report('hello.state with a firing alert', validate({ ...hello, state: { alerts: [al] } }));
    report('hello.state with a raw fingerprint', validate({ ...hello, state: { alerts: [{ ...al, fp: 'a1b2c3d4e5f6a7b8c9d0' }] } }), false);
    report('hello.base decodes to 128² int16', base.length === 128 * 128 && Math.max(...base) > 3000 ? [] : [`len ${base.length}`]);
    const { valid, invalid } = JSON.parse(readFileSync(join(fx, 'frames.json'), 'utf8'));
    valid.forEach((f, i) => report(`frames.valid[${i}] ${f.t}:${f.kind || ''}`, validate(f)));
    invalid.forEach((f, i) => report(`frames.invalid[${i}] ${f.t}:${f.kind || ''}`, validate(f), false));
    const dw = createWorld({ dev: true }), seen = []; // dev world throws on an unknown bus type
    for (const k of ['deposit', 'site', 'mark']) dw.on(`world:${k}`, (p, type) => seen.push(type));
    for (const f of valid.filter((f) => f.t === 'world')) { try { dw.applyFrame(f); } catch (e) { seen.push(e.message); } }
    report('valid world frames → world:<kind> bus', seen.join() === 'world:deposit,world:site,world:mark' ? [] : [seen.join()]);
  }
  console.log(bad ? `SCHEMA FAIL (${bad})` : 'SCHEMA PASS');
  process.exit(bad ? 1 : 0);
}

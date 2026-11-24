# Spike: the ui-core bet (Preact + signals, statechart + surface table)

> Status: history (2026-11-24). Record of 2026-11-14 measurements.

Worktree (kept, detached at origin/main, never pushed): (session scratchpad)
Bun 1.4.4 (local) and 1.4.2 (CI's pin, via npm `bun@1.4.2`). Chromium via Playwright (headless shell 1234).

## Verdicts

| Bet | Verdict |
|---|---|
| 1a. Byte-identical dist under Bun 1.4 with Preact + signals | PASS |
| 1b. Runs clean under `style-src 'self'` | PASS |
| 2. Surface table covers 9 fixture states + strategy/thread, "no escape hatches" | FAIL as worded; PASS if amended (see below) |
| Size/speed (not a bet, measured) | gz +10.1 KB (top of the 5-10 KB estimate); ~10 us per fight tick |

## Versions (exact)
- preact 11.2.0 (what `bun add` resolved; Preact 10.x also satisfies signals' peer range, not tried)
- @preact/signals 2.11.4 (pulls @preact/signals-core)
- bun 1.4.4 and 1.4.2 give identical output; typescript 6.0.3, biome 2.6.4 (unchanged)
- package.json: `bun add -E preact @preact/signals` put them in `dependencies` (should be devDependencies, since the repo bundles everything); lockfile updated.

## 1. Bun dist identity
Baseline check first: a clean `bun build` of world.ts on this checkout is byte-identical to the committed dist/world.js (sha 1c30cda4...).
With the spike component imported by world.ts (3 bundles, the exact three `bun build` lines of package.json `build`), built into two dirs, plus a third with bun 1.4.2:

    57f1dc2f...b3fb29753d  app.js    (a == b == c)
    363b12b9...e5c4c4  boot.js   (a == b == c)
    465096b6...a4982   world.js  (a == b == c)

So: deterministic across two builds and across 1.4.2 vs 1.4.4. (app.js and boot.js are unchanged vs baseline since they do not import Preact.)

**Bun and JSX:** native. No plugin, no Babel. `bun build` reads tsconfig `jsx` + `jsxImportSource` from tsconfig.json and emits `preact/jsx-runtime` calls. Files must be `.tsx`.

**Config needed (the whole list):**
- tsconfig.json: add `"jsx": "react-jsx"`, `"jsxImportSource": "preact"`, and `"cmd/game/websrc/**/*.tsx"` to `include`. `tsc -p tsconfig.json` is clean. (tsconfig.test.json needs the same jsx lines if a test imports a tsx file; bun test itself needs nothing.)
- biome: no config change. Biome 2.6.4 parses TSX and lints JSX with the existing rules (`biome ci` passes on the new files; the 3 warnings in the repo are old noUnusedImports). Its formatter reflowed my JSX; `html.formatter` off is irrelevant to JSX.
- `"types": []` in tsconfig stays; preact ships its own types, and the `style={{'--e': ...}}` custom property type-checked.
- Build script: unchanged lines; only the entry list changes if a new bundle is wanted.

**Size (world.js):**
- raw 2,166,665 -> 2,202,328 B (+35.7 KB)
- gzip -9: 509,655 -> 520,014 B = **+10,359 B (+10.1 KB)**. That is preact 11 + signals + hooks + the dock component (~1.2 KB of that, the TSX). The ADR's 5-10 KB estimate is at its upper edge; budget 10-11 KB.

## 2. CSP (real Chromium, game's exact CSP string from gate.go)
Script: dev/ui-spike-csp.mjs (copies hud-fixture's server + CSP approach; needs `NODE_PATH=<dir holding playwright>`; use any directory with playwright installed).
Rendered the dock from signals, drove it into the fight face (HP 40, slots, ult 50%).
- Result: **no console error, no pageerror, no securitypolicyviolation event.**
- `style={{ width: '40%' }}` -> element.style (CSSOM). Applied: inline `style` attr shows `width: 40%;`, computed width non-auto. Allowed.
- `style={{ '--e': '50%' }}` (custom property) -> works the same (Preact uses style.setProperty).
- String `style="width: 33px; --k: 7px"` in JSX: also applied (Preact assigns through CSSOM, not setAttribute) and no violation.
- Negative control: `insertAdjacentHTML('<i style="width:77px">')` was BLOCKED in the same page ("Applying inline style violates ... style-src 'self'", width stayed auto). So the CSP was really enforcing and Preact's path is not that path.
- Inline scripts: none. Preact needs no eval and no `new Function`. Signals neither.
Caveat: a future component must never use `dangerouslySetInnerHTML` with style attributes, or set `innerHTML` with style. An AST guard could ban both.

## 3. Statechart and surface table
Files: cmd/game/websrc/ui-chart.ts (machine + table), ui-chart.test.ts (4 tests, all pass).

Code shape:

    interface S { mode: 'explore'|'strategy'; place: 'outside'|'inside'|'flying'; fight: 'calm'|'fighting'|'down'; panel: 'none'|`open:${PanelKind}` }
    step(s, ev, facts): S | null            // null = refused; guards read other regions and facts
      case 'ENTER':  return s.place==='outside' && calm ? {...s, place:'inside'} : null
      case 'OPEN':   if (kind==='thread' && subject not app/component) return null; ...
    const TABLE: Row[] = [ // ordered, first match wins; a row names what is SHOWN, the rest is away
      { name:'fight-inside', when:k=>fightFace(k)&&k.place==='inside', show:['menu','dock:fight','leave'], touch:['pad'] },
      { name:'thread-strategy', when:k=>k.panel==='open:thread'&&k.mode==='strategy', show:['menu','strategy','thread'] }, ...
    surfacesOf(s, facts, ctx) -> { row, show }   // touch adds row.touch; visitor drops OWNER_ONLY (I7)

All 11 states are rows (15 rows total, since down/flying/strategy/thread-strategy and the touch/visitor variants fall out):
- rest, focus, fight (touch adds pad, mouse does not), menu (radial), panel (people), overview (+find), inside, public, settings: each asserted against dev/hud-fixture.mjs show lists in the test.
- public = the focus row + role filter (the visitor's owner surfaces are removed). It needed no row of its own.
- NEW strategy at app subject, thread open: row `thread-strategy` = menu + strategy + thread. OK.
- NEW phone fight inside a building: row `fight-inside` = menu + dock:fight + leave + pad. OK, but see hatch 3.
- I9 check: BFS over events x all fact combos reaches 78 region states; every one, in every (focus, line, foot, subject, input, role) combination, matches a row, and all 15 rows get hit. No catch-all row (the last row has an explicit guard), so the enumeration is a real exhaustiveness test.
- I2 holds by construction (regions independent; MODE keeps panel and thread).

**Where it needed more than "4 regions + context" (why strict FAIL):**
1. **Facts outside the regions drive the table**: `focus` (target in reach), `line` (a dock line is up, timed by a queue), `foot` (on foot vs bike), `subject`. The key must be regions + these 4 facts. Easy to declare, but it is not "keyed by the chart".
2. **Panel needs child states.** Region value `none | open` is not enough; the rows need the open panel's kind (`open:people`, `open:thread`, ...). Fine as a compound state, but the ADR text should say so.
3. **Phone fight inside a building** has no way to leave: the Leave button lives on the focus face, which the fight face replaces. I added a `leave` surface to that row. This is a design decision, not a table problem (alternative: guard HIT so a fight cannot start inside; ENTER is already refused while fighting). Candidate decision record.
4. **`bike`**: Place = outside | inside | flying has no "riding"; today's `foot` fact does that job. Either add `ride` to Place or keep `foot`.
5. **Public visitor**: the fixture's "gone" (never in the DOM) vs "away" (hidden). The table says shown/not shown only; role needed a post-filter (OWNER_ONLY) and a mounted-vs-shown distinction if the fixture's `gone` check is to be derived from the table.
6. **`down`** is a Fight value in the ADR but has no fixture state; I9 forced a new row (`down` -> respawn card) and a new fixture row. Good: that is the invariant working.
7. **Row order = `covered()`**: panel rows precede fight/line rows (a panel covers the dock even mid-fight, as dockOf does today). Order is semantics; the test pins it.
8. **Fight timing**: today's `fight` = inFight || hurt window || a skill ready. The machine gets it only as scene events (HIT, CALM by a timer); the timer lives outside the chart.
9. The table says which surfaces, not where. The four-size geometry checks (44 px targets, no overlap) still come from the fixture and CSS; not a table matter.
Not modeled: talk and the sheet (today's `covered()` also counts them) -- they would be more Panel kinds.

**Size estimate** (non-comment, non-blank lines): machine (types + step with 11 guarded events) 56; table (15 rows + filter) 35; test 60. About 90 lines for machine+table, ~1.5 lines per row. Real version: the 3 more fact types, talk/sheet panels, the strategy subjects and the reconnecting state will roughly double it, ~200 lines.

## 4. Size and speed (micro-bench, Chromium, 20k iterations each, us per change, 3 runs agree within 5%)
Script: dev/ui-spike-bench.mjs (bundles ui-bench.tsx; the real makeDock, the real town-util `keyed`).

| Change | today `makeDock.show` (direct writes) | `keyed` rebuild (JSON sig + replaceChildren) | Preact dock, sync flush per change |
|---|---|---|---|
| fight tick (HP, cooldown, ult change) | 2.5 | 6.4 | 9.8 |
| new line | 3.0 | 1.5 | 5.1 |
| state flip (line <-> fight, batched) | 3.1 | 4.7 | 12.1 |

Notes: Preact re-renders the whole Dock component on any signal read in render (diffs ~40 nodes); a signal bound directly into a text/prop would skip the component render, not tried. Sync flush is the worst case (one render per change); the default microtask debounce batches. 10 us is 0.06% of a 16.7 ms frame; even 3x worse is irrelevant. It is ~4x the hand-written direct writes and ~1.5x a keyed rebuild: **cost is fine, and consistent with the ADR's I6 (fast values bind per frame outside the store)**.

## Fallbacks
Bet 1 holds, so no need for the no-JSX or Lit fallback. Bet 2: widen the table's key to regions + declared facts, make Panel a compound state, and list `leave`-while-fighting as a decision record. Do not narrow the table to mode and form layout.

## Files in the worktree (all untracked or modified, none committed)
package.json, bun.lock, tsconfig.json (jsx), cmd/game/websrc/world.ts (+1 import, size measure only), ui-dock.tsx, ui-spike.ts, ui-spike-page.tsx, ui-chart.ts, ui-chart.test.ts, ui-bench.tsx, dev/ui-spike-csp.mjs, dev/ui-spike-bench.mjs. Build dirs next to the worktree: ../dist-a ../dist-b ../dist-c ../base-dist, ../build.sh, ../bun142.
Run: `NODE_PATH=<dir holding playwright> node dev/ui-spike-csp.mjs` (and ui-spike-bench.mjs); `bun test cmd/game/websrc/ui-chart.test.ts`.

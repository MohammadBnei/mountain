# Quick fixes: ship first, apart

The dev rules say "a fix found during a refactor ships first, apart" (`plan/16-dev-rules.md:81`). These three fixes
come out of the audit (`../01-baseline/audit.md`). Each one is its own small PR in ukubi-game, done on the main thread
with the usual loop: a short plan, a doubt cycle, the proof, the build, the checks, a review, the PR, then a merge on
the owner's word. Each section below is the starting point for that short plan, not the plan itself.

## Q1. Delete the TS copy of the land's height and regions

- **What:** `websrc/world-land.ts:292 regionAt` and `:314 heightAt` copy Go's `land.go:904 regionAt` and `:939 heightOn`.
  Nothing checks the copy any more.
- **Live readers of `regionAt`:** the terrain tint (`world.ts:452`), the cover (`:456`) and the minimap (`:2256`). They
  call it once for every point they colour, so a WASM call per point may cost too much.
- **Open question:** the shape of the replacement.
  - The zone per hex: the hex land already knows it through `zoneOf`, `land/hex.go:828`.
  - Or a batched WASM export, like `heights` (`internal/physwasm/bridge.go:201`).
- **Readers of `heightAt`:** the default argument of `rayLand` and the gate probe (`world.ts:3353`).
- **Proof:**
  - the dist diff;
  - the terrain, cover and minimap pixels unchanged over the gate's views;
  - the land hash unchanged.

## Q2. Assemble the building set once, on the server

- **What:** the extra buildings (the 8 info buildings, the yard's hut and pieces) are assembled in TS
  (`websrc/world.ts:349-366`, `world-capital.ts:138`, `world-yard.ts:215`) and again in Go
  (`internal/realm/land.go:131-170`). The `_(blue|green|red|yellow)$` regex appears on both sides. Plan/21 already
  says "one land, built from the public town" (`plan/21-the-shared-world.md:87-92`).
- **Shape:** the server's `/api/town` (and the public snapshot) sends every building already placed. The client and
  the realm both read that list.
- **Proof:**
  - the land hash is the same for every seed in the realm golden (`dev/realm-land-golden.ts`);
  - the sim frames golden is unchanged;
  - the dist diff.

## Q3. The server's move check applies the land's step rule

- **What:** `World.ResolveFall` (`internal/realm/realm.go:850-905`) never calls `land.StepOK`, the hex step rule the
  client's `Step` obeys. A claimed move over a cliff edge or into water passes as long as no hard collider is in the way.
- **Shape:** add `StepOK` to the sweep in `ResolveFall`. The tolerances (`Desync`, `Slack`) stay.
- **Proof:**
  - a realm test that a cliff walk is refused and a legal terrace step is accepted;
  - a mutation that drops the call and turns the test red;
  - the existing fall tests unchanged.
- **Behaviour change:** yes, a cheating or desynced client is snapped back sooner. This is listed in the PR.

## Q4. Cache and compress the static files; shorten the loader fade

- **What:** the load profile (`../01-baseline/load-profile.md`) found that `dist/*`, the CSS, the fonts and
  `world.html` are served by `http.FileServerFS` over the embedded files with no ETag, no Last-Modified and no
  Cache-Control.
  - Every reload downloads about 3.0 MB again: `world.js` is 2.17 MB and the wasm 569 KB.
  - The 89 kit GLBs are revalidated on every reload.
  - Nothing is compressed: `world.js` is 511 KB gzipped, the wasm 234 KB.
  - A fixed 450 ms fade (`FADE_MS`) runs before `ready`.
- **Shape:**
  - Content-hashed (or build-token) URLs with `Cache-Control: immutable`, or at least an ETag from the embedded
    file's hash.
  - gzip or brotli, precompressed at build time so the dist stays byte-identical.
  - The fade overlaps the first frames instead of delaying `ready`.
  - First, check what the production ingress already compresses.
- **Expected gain:** 0.5–2.5 s on every warm reload over a real link, about 2 MB less on a cold load, and 450 ms
  everywhere. This is likely most of the owner's "few seconds".
- **Proof:**
  - response headers asserted in a gate test (`internal/gate`);
  - a reload in the browser shows 304 or a cache hit for every static file;
  - `ready` measured before and after.

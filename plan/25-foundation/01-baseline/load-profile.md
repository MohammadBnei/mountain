# World load profile (ukubi-game main @67bd436, local stack)

> Status: history (2026-11-24). Record of 2026-11-14 measurements.

## Method
- Stack: `MOUNTAIN=../mountain REALM=<built cmd/realm> dev/stack-realm.sh <built cmd/game> <out>` (atlas --dev fixtures, 89-node founded world, realm, owner on :7962). Binaries built. Stopped afterwards; no tracked files touched, no worktree needed.
- Browser: Playwright-core driving Google Chrome, `--use-angle=metal --enable-gpu --ignore-gpu-blocklist`, 1440x900, `/world.html?hour=12`. The measuring scripts lived in a session scratchpad and were not kept.
- Cold = fresh context (empty cache) x3; warm = reload in same context x3. Marks: `__world.ready`, first rAF, FCP, longtasks, wrapped WebAssembly.instantiate and WebSocket, resource timing, CDP CPU profile of one warm reload. Localhost, so network time is ~0: real-link cost is computed from bytes.

## Static boot path
world.html -> boot.js, world.js (2.17 MB single bundle, loaded at once) -> `world.ts`: `Promise.all([kit/manifest.json, api/town, loadPhysics()])` (wasm fetch + instantiate, no streaming) -> snapDisc -> `landOf` -> `hexLand` (`hexMs`, exposed as `__world.land().ms`) -> holo boxes, `drawBuildings` (89 GLB fetches) -> buildTerrain/tunnels/minimap/cover -> `pollOnce` (api/tech, state, live) -> `renderer.setAnimationLoop` -> await terrain.ready, buildingsReady, under.ready -> `await loader.finish()` (FADE_MS = 450 fixed) -> `ready`. Realm ticket + WebSocket start after ~0.9 s and do not gate `ready`.
Existing timings: only `hexMs`. Gates (dev/*-gate.mjs) just `waitForFunction(__world.ready, 60 s)`; no load budget anywhere. world-land.test.ts asserts landOf < 200 ms.

## Raw numbers (ms)
| run | ready | first rAF | FCP | hexLand | wasm instantiate | longtask sum before ready (n, max) | ws open |
|---|---|---|---|---|---|---|---|
| cold 0 | 5650 | 1553 | 2788 | 285 | 5 | 4701 (4, 2716) | 6266 |
| cold 1 | 1592 | 13 | 176 | 283 | 1 | 1065 (2, 902) | 1315 |
| cold 2 | 1528 | 29 | 64 | 281 | 1 | 1009 (2, 826) | 1266 |
| warm 0 | 1358 | 16 | 28 | 254 | 1 | 896 (2, 746) | 1086 |
| warm 1 | 1411 | 15 | 36 | 264 | 1 | 935 (2, 774) | 1147 |
| warm 2 | 1401 | 30 | 56 | 260 | 1 | 918 (2, 755) | 1140 |

Cold 0 (5.6 s, 2.7 s longtask) is the first launch of a fresh Chrome: GPU/Metal shader cache cold. Later "cold" runs have an empty HTTP cache but a warm shader cache, so they are not a true first-ever visit.

Bytes: 139 requests, 4.9 MB: world.js 2.17 MB, models 89 GLB 1.40 MB, wasm 569 KB, css/fonts ~200 KB, /api/town 112 KB, manifest 28 KB, other kit/api ~150 KB.
Warm reload still transfers 3.15 MB: world.js, wasm, css, fonts, town (no validators, see below). Models revalidate (89 x 304, 26 KB).

## Timeline, warm reload (ms)
- 0-57: html, css, world.js download+parse (localhost, tiny).
- 57-831: ONE 774 ms main-thread task (sync startup). Profile inclusive: hexLand 237 (wasm 233, of which land.stageTerraces 68), buildTunnels 167 (inFloor/inRuns/segDist2 ~145 self), buildTerrain 135, buildMinimap 61, rest landOf/placeDisc/holo.
- 68-540: 89 model GLB fetches (HTTP/1.1, 6 at a time), overlapping the task above.
- 955-1116: second longtask, first frames: WebGLRenderer.render ~324 incl, setProgram 168, getProgramInfoLog 106 (shader compile/link).
- ~960-1410: fixed 450 ms loader fade, then `ready`. Realm ticket at 926, ws open ~1140 (after ready was set or concurrent; not on the critical path).

## Breakdown of warm ready (1400 ms)
| stage | ms | % |
|---|---|---|
| land + hex + tunnels + terrain + minimap (sync JS/WASM) | ~775 | 55 |
| first-frame render, shader compile | ~160-330 | ~15 |
| loader fade (fixed) | 450 | 32 |
| town fetch / wasm instantiate / models (local) | <5 / 1 / hidden under the sync task | ~0 |
| realm handshake | off critical path | 0 |
(Stages overlap, so percentages are approximate.)

## Caching headers (the key finding)
- `dist/*`, `*.css`, fonts, `world.html`: embed.FS via `http.FileServerFS` (internal/play/play.go:238, internal/public/routes.go:69): no ETag, no Last-Modified (embed has zero mtime), no Cache-Control, no compression. Every reload refetches 3.0 MB in full. (`Cache-Control: no-cache` + ETag only appears on /kit, served by another handler, so 304 for 89 models.) Behind the gate with sign-in: `private, no-cache` and still no validator.
- No gzip/brotli on the game: world.js 2.17 MB -> 511 KB gzip -9, wasm 569 -> 234 KB, so ~2.0 MB less over the wire (ingress may compress in prod; unverified).
- Real-link estimate for the 3.0 MB uncached assets: ~0.5 s at 50 Mbit/s, ~2.4 s at 10 Mbit/s, ~1 s+ more with 100 ms RTT for the 89 GLB waves. This is on top of the 1.4 s above, so on a real network most of the owner's "few seconds" is probably here.

## Candidate fixes (expected gain from the numbers)
1. Content-hash dist filenames + `Cache-Control: public, max-age=31536000, immutable` (or at least ETag from the embed hash + `no-cache`): warm reload skips 3.0 MB. Saves 0.5-2.5 s on a real link, 0 locally. Small change in one handler. Highest value.
2. gzip/br for js/css/wasm/json: -2.0 MB cold; ~0.3-1.3 s cold on 10-50 Mbit/s. Cheap (precompressed or middleware).
3. Kit models: `immutable` + one pack (or HTTP/2): removes 89 revalidations (~0.1-0.5 s of waves on real RTT).
4. Drop or overlap the 450 ms loader fade (set `ready` when the world is drawn, fade over it): -450 ms (32%) everywhere, trivial. Check gates relying on "nothing reads through it".
5. Land build (775 ms sync): cache `hexLand` output by land hash in IndexedDB (saves ~260 ms hexLand, maybe 400+ with tunnels/terrain geometry if cached too; needs a stable hash, serialisation cost to measure); or move landOf/hexLand/tunnels to a Worker (wasm instance there; frees the main thread but wall time stays ~same unless parallelised with model fetch, which already overlaps). Optimise buildTunnels first (167 ms, pure JS scan with inFloor/segDist2; a spatial index likely cuts most).
6. Progressive near-spawn build / chunk streaming: terrain already streams detail; building only the spawn-radius first could cut the 775 ms task to ~300 ms but is the largest change.
7. Shader precompile (`renderer.compileAsync` during the loader): moves ~100-300 ms of getProgramInfoLog out of the first frames; first-ever visit (cold GPU cache) saw a 2.7 s task, where this matters most.
8. Instrumentation: add `performance.mark` per loader stage and a ready budget to world-gate so regressions show.

## Caveats
Localhost, Apple GPU, desktop viewport; phones will multiply the CPU stages (the code comments say 3-4x). Prod compression/CDN unverified. One genuinely cold-GPU sample only.

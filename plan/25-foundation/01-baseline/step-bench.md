# Shared movement core: server CPU, trig divergence, WASM size

Repo ukubi-game @67bd436, Apple M1 Pro, Go 1.27.2, single thread, `-benchmem`. Throwaway worktree (removed); bench sources, the pure-trig package and call-site patch are in `step-bench-artifacts/` beside this file.

## 1. Server CPU
Commands: `go test ./internal/physics -run XXX -bench 'BenchmarkX|BenchmarkSweep|BenchmarkPath' -benchmem`, same for `./internal/realm` (`ResolveFall`) and `./internal/mobs` (`Step|Tick50`, existing benches). World = `traceWorld` (land fixture `cmd/game/websrc/world-land.fixture.json` + decor grid + walls); each trace's recorded inputs replayed in a loop, so ns/op is per Step, jumps/slides included. "rec" = recorded dt 1/60, "20Hz" = same inputs at dt 0.05.

| case | ns/op rec | ns/op 20Hz | allocs |
|---|---|---|---|
| open ground (double, drift-bike, swim) | 3190-3490 | 3210-3890 | 0 |
| trunk / fence | 3270 / 3430 | 3340 / 3420 | 0 |
| building | 3650 | 3970 | 0 |
| terraces: step / cliff | 5680 / 5270 | 6350 / 8030 | 0 |
| bump | 3650 | 6700 | 0 |
| `SweepMotion` standalone (0.67-unit move) building / open | 264 / 307 | | 0 |
| `World.ResolveFall` (realm fixture; home / rim) | 596 / 1235 | | 2 / 11 (128 B / 1184 B) |
| mob `mobs.Step`, no replan | 76 | | 0 |
| realm-sized mob tick, 50 chasing mobs (replans incl.) | 21 461 per tick (430/mob) | | 0 |
| `Walk.Path` replan, 32 / 80 / 160-unit window, wall in the way | 81 000 / 383 000 / 426 000 | | 0 |

Derived (20 Hz, one Step per move): typical 3.5 us x 20 = 70 us/s per player = 0.007% of a core, about 14 000 players per core; worst measured (cliff at 20 Hz, 8 us) = 160 us/s, about 6 000 per core. With a 3x derate for a slower node and 50% headroom: roughly 1 000-2 000 players per core, i.e. Step is not the budget. Replacing `ResolveFall` (0.6-1.2 us, plus allocs) with Step is about +2.5-7 us per move: +50-140 us/s per player. Mobs: a chasing mob replans every 500 ms (2/s): 2 x 80-430 us = 0.16-0.85 ms/s per mob, so path replans (about 1 000-6 000 chasing mobs per core) dominate everything else by 10x or more.

## 2. Trig divergence
Call sites outside tests: physics 22 (Sin 8, Cos 10, Atan2 4; `moves.go` 5, `grid.go` 4, `motion.go` 9, `step.go` 1, `physics.go` 1, `collide.go` 4 lines), mobs 3 (Cos 1, Atan2 2); plus Exp 4 and Hypot 41, which are not imported from JS (pure Go in TinyGo). Only sin/cos/atan2 are `env` imports in the current wasm (verified from the import section).

Method: rewrote the 22 physics call sites to hooks; replayed every trace (room traces skipped), looping its inputs to 60 s at dt 1/60 (open loop, no server correction); reference = exact Go math; running max of position distance up to 1/10/60 s.
- Perturbation A (1 ULP, sign keyed on input bits, the realistic V8-vs-Go gap): worst-case 1 s 3.5e-12 (bike-slow), 10 s 0.033 (bump), 60 s 0.034 (bump). Every other trace stays below 1e-11 for 60 s; 7 of 19 are bit-exact (no trig-sensitive path). The 0.033 is one discrete collision-branch flip (bump trace), not growth.
- Perturbation B (results rounded to float32, a roughly 10^8x harsher stress): 1 s 1.8e-4, 10 s 0.055, 60 s 1.57 (drift-bike, a branchy slide/drift loop).
Reading: smooth paths differ by about 1e-12 and do not grow; the risk is branch flips at wall/edge contact, worth 3-5 cm once, which the 20 Hz server correction would absorb or flag as a desync. Existing traces already pass at 1e-9 vs the real V8 (`traceTol`).

Pure-Go trig: TinyGo wasm-unknown links math.Sin/Cos/Atan2 to libm, hence the imports. I copied Go's `sin.go`, `atan.go`, `atan2.go`, `trig_reduce.go` into `internal/ptrig` (about 330 lines stdlib plus a 35-line shim), pointed the 25 call sites at it. `go test` physics, mobs, physwasm all pass (traces within 1e-9). Built with `dev/wasm/build-in-docker.sh` (Docker available, cached toolchains, 62 s): the module has zero imports; 568 978 to 572 398 bytes (+3 420, +0.6%; gzip 234 382 to 235 746, +1 364). Effort: about half a day including a call-site lint (codecheck) so nobody calls `math.Sin` directly again. Not measured: wasm speed of pure-Go vs JS Math (host call overhead likely favours pure Go; unmeasured).

## 3. WASM today
`cmd/game/web/dist/physics.wasm` = 568 978 bytes (234 382 gzip); 33 exports (`memory`, `_initialize`, `syscall.seek`, `fminimum`, `fminimumf`, `fmaximum`, `fmaximumf` plus 26 `//go:wasmexport` functions incl. `step`, `air`, `stand`, `route`); 3 imports (env.sin, cos, atan2).

## Caveats
- Bit-identical client/server needs identical float ops: the repo wraps products in `float64()` to stop arm64 FMA fusion; the copied stdlib trig lacks those casts, so a native arm64 server would fuse inside the polynomials and differ by ULPs from wasm (amd64 does not fuse). Add casts or build the server amd64.
- Bench is single-thread, warm cache, one rider; real tick adds snapshot/proto work (`BenchmarkTick` not run). ResolveFall's bench used the realm fixture, Step's the physics fixture (different lands).
- Drift test is open loop and 1-ULP is an assumption about V8 (fdlibm-derived, under 1 ULP); no real V8 vs Go diff was taken beyond the existing traces.

# ADR: sim-world, one Go core for the world and everything that moves on it

Status: proposed 2026-11-15, after an architect round and one doubt round (`../05-doubt/sim-world.md`). The owner said
yes to writing it; approval is pending the reviewed page.

## Context

The MVP is done. The goal now is a world stable enough to build on with trust and speed. The pain is growth.
- **The world will grow:** the disc about 2× in radius (`DiscR` 54.66, `town/disc.go:35`), plus many spaces linked by
  portals: building interiors, the underside, sky and space, dungeons, and later endless procedural realms.
- **Every space is shared and realm-authoritative**, for explore and strategy alike.
- **Load:** 20 to 50 players in a busy space, up to about 30 spaces live, 20 Hz, a result final within about 150 ms.
- **Changes land on the next reload.** The lore is not written yet; the owner will move things, rarely.

What stands in the way today:
- The realm holds one `World` (`internal/realm/land.go:18`) behind one pointer, one lock and one land hash. A client
  whose land hash differs is refused at hello ("another land: reload the page", `internal/realm/realm.go:529`), so a
  land change makes every reconnect fail until the page reloads.
- The layout is history-dependent state: villages and lots are logged as `layout.stepped` events
  (`game/layout.go:12-19`) and each new layout is stepped from the last (`LayOut`, `town/village.go:792`). The log is
  the source of truth, so the same graph can give two towns; the stepper itself is not the problem.
- Most land stages are whole-world passes: massifs, pits, tunnels, caves, roads, lairs, the guild hall and village
  placement (doubt finding F3). Chunk-local generation is not possible for them as written.
- Bit-exactness is claimed, not proven: `math.Exp` (FMA-gated assembly) and `math.Hypot` (assembly) take architecture
  paths on amd64 that pure Go and WASM do not. `town/`, `internal/combat` and the realm (`place.go`, `fight.go`,
  `build.go`, ...) use `math.Sin/Cos/Atan2` outside the lint, and no test compares the gc and WASM builds bit for bit.
- Movement is a pose claim checked by `World.ResolveFall` (defined `realm.go:886`, called on each move at `:700`). The
  client steps at a variable dt; the ship's flight model lives only in TS (`world-ship.ts`, Go has only its hull
  collision, `physics.ShipLand`), and the lunge and auto-hop live outside `Step`.
- Snapshots are full protojson every tick: every body to every page, mobs already filtered per cell
  (`realm.go:421-429`).
- Interiors and the underside are private: the page sends no pose there (`world.ts`, plan/20 T4).
- Combat, mobs and placement live in `internal/realm` with Postgres and sockets, so solo mode cannot run them.

## Decision

**A bounded space is generated whole on the server and shipped as pinned, content-addressed chunks. One pure Go sim
package runs every mover, in the realm, in client prediction and in solo mode.**

- **The authored layer.** Spaces, portals, zones, regions, placements (villages, lots, the hall, massif and lair sites)
  and generator parameters are versioned files in git, reviewed by PR, pinned by commit sha. Atlas projects them like
  any other repo. A placement changes only when the owner edits it; a new graph node gets a default slot written as a
  proposal, and nothing placed ever moves on its own. The stepper stays as a pure projection tool: it turns the graph
  into proposed placements, and its output becomes a reviewed file, never a logged event. A new, enhanced placement
  system grows from it. The `layout.stepped` log, the old district layout and the town page retire. The master control room comes later and writes these files.
- **The world model.** A space is a pure function of (pinned files, graph snapshot, seed, generator version). The server
  builds it whole and cuts it into chunks: the existing binary `WriteHex` form (`internal/land/wire.go`), one file per
  chunk, cached forever. A chunk's id hashes only what the chunk reads. The client streams chunks and no longer builds
  land. Decor node ids become (chunk, index).
- **Spaces in the realm.** One instance per (space, version), each with its own tick, lock, bodies and mobs, loaded
  while someone is in it or strategy watches it. A new version starts a new instance; the old one drains. The welcome
  names the version; a portal admits to a version the page holds or can fetch. Interiors and the underside become
  shared instances with their own colliders.
- **The sim core.** A pure `sim` package: movement (`Step`), fight, mobs and placement, pulled out of `internal/realm`,
  built by the realm and the WASM. Its ABI is `step(state, inputs) → state` over plain structs. Mover profiles (rider,
  ship, mob, boss, resident, critter, animal, carrier) share one two-level nav: chunk or hex graph first, the 0.5-unit
  grid only near the mover, replans only when the goal moves a cell.
- **Who the realm simulates.** Reactive movers: players, ships, mobs, bosses, an agent that walks to you. Residents,
  critters and carriers stay scripted from the shared clock (already the same on every page) until one needs to react.
- **Movement by input replay.** The client sends tick-stamped inputs; the realm steps them in tick order at 20 Hz; the
  client predicts, then rewinds and replays on a correction. `ResolveFall`'s pose check goes.
- **Bit-exact, proven.** Exp and Hypot join `internal/ptrig`; the lint bans every `math` function with assembly; `town/`
  and placement join the exact set; a CI test runs the same inputs through gc and the TinyGo WASM and compares
  `Float64bits`.
- **Snapshots.** Binary protobuf, quantized positions, deltas, interest filtering for bodies as for mobs.
- **Solo mode.** The client runs the same `sim` package for the movers near the player (plan/21's promise kept).
- **Kept state.** Inventory and progression stay realm events in its Postgres (plan/21). Player-built pieces go. Mobs
  and bosses reset like dungeons. Operations, threads and rooms live in atlas, fleet and operations.
- **Language.** Go. The cost is path replans (81 to 426 µs against a ~4 µs `Step`), not the language; the pure ABI
  keeps a later port contained.

## Order

1. **Proof of determinism:** Exp, Hypot, the wider lint, the gc-against-WASM golden.
2. **Authored files and the server-built land:** placements as files (proposed by the new placement system, the stepper as a pure function), `WriteHex` chunks by content id, the client
   streams instead of building. The town page and old layout retire.
3. **The `sim` package:** fight, mobs and placement extracted; the ship, lunge and auto-hop in `Step`.
4. **Spaces:** one realm instance per (space, version), shared interiors and underside, binary delta snapshots.
5. **Input replay:** fixed client step, tick-stamped inputs, rewind and replay, substeps or swept contact for bikes and
   ships, a lag-compensation rule. A two-phone race and PvP test comes first.
6. **Later:** endless realms with their own local-rules generator; the master control room.

## Alternatives considered

| Option | Why not |
|---|---|
| A: whole world per space, built in the client | 2× radius is 4× area: today's 775 ms startup task becomes ~3 s on desktop, 9 to 12 s on a phone |
| B: small bounded spaces only | No large continent; a portal pause everywhere |
| C: every chunk generated locally | Most land stages are global (F3); endless realms need a different generator anyway |
| D: sharded sim processes | Ops cost with no need at 20 to 50 players |
| E: thin client, no WASM sim | Loses prediction, solo mode and feel |
| A Rust engine | Language is not the bottleneck; one Go code for realm and WASM is already bit-exact |
| Lore as atlas rows | Breaks atlas's rule that projections rebuild from repos; no dev persistence, review or revert |

## Consequences

- Growth is a file edit and a reload; nothing moves unless the owner moves it.
- The client stops building land, which removes the 775 ms startup task and the phone's 3 to 4× of it.
- The realm becomes many instances; plan/21's single world and single hash end.
- The ship, lunge and auto-hop move into Go; the TS movement copies are deleted.
- Residents stay cheap; making one reactive is a profile change, not a new system.

## The bets

1. Building a bounded space whole on the server stays affordable: the land build (`hexLand`, ~285 ms today,
   `01-baseline/load-profile.md`) grows with area to about 1.1 s for the 2× disc, once per version. The page's render
   geometry (tunnels, terrain meshes) is built per chunk as it streams in.
2. 20 Hz input replay with substeps feels right for racing and PvP. The two-phone test settles it before the switch.

## Not doing

- Endless procedural realms in this foundation.
- Realm-simulated residents, critters and carriers before one needs to react.
- A Rust port.
- Live rebuild of a space mid-session.
- Sharding spaces across processes.

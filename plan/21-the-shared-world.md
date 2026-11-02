# 21 · The shared world: one job per part, one physics core

Status: draft 1, 2026-11-02. Written from the owner's research request, an intent interview, two architecture
interviews (the physics core, then the shared world) and two doubt cycles. The owner approved this plan on
2026-11-02.

The disc becomes an MMO-lite action game:
- farming mobs and bosses;
- opt-in PvP and third-person battles;
- picking up and placing objects;
- building;
- gathering resources (wood, iron).

The goal is not to bolt a game server onto the side. It is to **simplify**: each part of the system gets one
conceptually independent job, and data flows one way.

## Where we start

All collision lives in the browser, in TypeScript:
- `world-collide.ts` holds the colliders and the grid;
- `world-motion.ts` holds the rider's body;
- `world-shot.ts` holds the cosmetic shot;
- `world-targets.ts` decides what the pointer is on.

The collision is 2.5D: circles and oriented boxes on the ground plane, each with a base and a height. The colliders
carry no ids, layers or masks. The grid is built once. There are no area queries and no hit events.

The server knows nothing about the land: `hexLand` and `decorOf` run only in the client.

## Intent (confirmed by the owner, 2026-11-02)

- **Scale:** 2–20 friends in one shared world, on the owner's cluster.
- **Combat:**
  - guns are hitscan;
  - spells are projectiles that can be dodged;
  - melee swings an arc during its active frames;
  - boss attacks are telegraphed ground areas.
- **PvP:** opt-in only (a duel or an arena, after the first slice).
- **Mobs:**
  - they roam, aggro, path with A* round player walls, chase, and leash home;
  - bosses have phases;
  - mobs and bosses are **pure game**: they are not tied to infra or incidents, and no reward touches goals or work
    (plan/12's director rule holds).
- **Physics:**
  - no tumbling or rigid-body simulation;
  - no digging;
  - carried things are kinematic, and thrown things fall and settle.
- **Building:**
  - small pieces inside a hex first, whole buildings later;
  - players build only in **build zones** that the town layout never touches (the yard's slots and a reserved
    ring).
- **Persistence:**
  - pieces, inventory and progression persist for signed-in players;
  - mobs, HP and projectiles are transient;
  - guests stay page-local and never build (plan/18).
- **Authority:**
  - the server owns the truth (HP, mobs, items, nodes, pieces, inventory);
  - the client claims its own hits, and the server checks range, cooldown and line of sight;
  - the client moves its own rider, and the server clamps the pose within a tolerance;
  - no lag compensation.

## The job split (ADR)

| Part | Its one job |
|---|---|
| `internal/land` (library, new) | the public town layout → `HexLand`: land, trees, fences, steps |
| `internal/physics` (library, new) | bodies, masked `Ray`/`Overlap`/`Sweep`, the rider step, the walk grid with A*. Runs on the server, and in the client as TinyGo WASM |
| sim / atlas | what the cluster **is** (unchanged) |
| town (`cmd/game`, owner pod) | what it **means**: buildings, cards, the residents' schedule data, the snapshot (unchanged) |
| edge (`cmd/game --public`) | the **door**: pages, sign-in, the snapshot, yard talk, letters; mints realm tickets |
| visitors | **who**: name, lots, bike, letters (unchanged) |
| **realm** (`cmd/realm`, new) | what is **shared and dynamic**: player bodies, mobs, bosses, combat, items, nodes, pieces, inventory |
| client | eyes and hands: render, input, prediction through the WASM physics; the residents' rounds on one fixed world clock |

- **Flow:**
  - Data runs town → public snapshot → realm. The realm reads the layout only, to build the land.
  - The realm never writes to infra, the town or visitors, so there is no cycle.
  - Clients get meaning from the edge (the owner pod for the owner) and place from the realm. The realm sends ids,
    never meaning.
- **Residents stay in the client:**
  - Their rounds are deterministic: they read the resident's id, its building and the hour.
  - The world clock uses one fixed zone, not the viewer's.
  - A shot resident becomes a realm event, and each client plays it as a reaction.
- **Lots and the bike stay in visitors:**
  - A lot is meaning (archetype, name, pitch, colours, a sanity check). Its place is the layout's slot.
  - The realm learns a player's slot from the realm ticket.
- **One land:**
  - The realm, the owner and friends all build the land from the **public** town at age 1.
  - `landOf`'s road tie-break sorts on the reduced road kind, so the owner's raw road vias and the public reduced
    ones give the same land.
  - A Go test proves that the owner input and `PublicOf` give the same `LandSpec`.
  - The realm handshake refuses a client whose land hash differs.
- **Auth:**
  - The owner signs in on `game.bnei.dev` and friends on `world.bnei.dev`. The `__Host-` session cookies are
    host-only.
  - Each pod mints realm tickets with its own Ed25519 key. A ticket holds the subject, role, slot and expiry.
  - The realm accepts the role `owner` only from the owner pod's key, and `friend` only from the public pod's.
    The internet-facing pod can never mint an owner.
- **Host and wire:**
  - The realm gets its own host, `realm.bnei.dev`, because the shared app chart routes by host, not by path. The
    pages' CSP `connect-src` names `wss://realm.bnei.dev`.
  - The wire is a websocket carrying protojson. One `.proto` schema serves Go and TypeScript, and binary encoding
    is a later switch.
  - The handshake carries the protocol version, the land hash and the ticket.
- **Failure:** the realm is additive. When it is down, the world plays solo exactly as today, with a banner, and
  the shared features pause.
- **Durable state:**
  - Pieces, inventory and progression are events in the realm's own Postgres database, folded on boot.
  - Guards run when a command is accepted, never during replay (the standard's fold rule).
  - Mobs, HP and projectiles are never logged.
- **Layers and attacks are data:**
  - Every query takes a layer mask (world, rider, mob, node, item, piece, projectile, boss area).
  - Attacks are rows in a registry: `{id, delivery: hitscan|projectile|arc|area, shape, speed, damage, cooldown,
    mask, telegraphMs}`.
  - Damage flows through hit events into health; the physics never applies damage.
  - Mob and boss AI are lifecycles the game owns, so a state machine is allowed (plan/15).
- **One logic:**
  - The physics is written once, in Go.
  - The client swaps to the WASM build right after the library is proven, before any realm feature, so the
    server and the client never run two copies.
  - Client and server results are not claimed bit-exact (gc uses assembly for `exp` and `hypot` and fuses FMA on
    arm64; clients step at their own frame rate), so the server's pose check uses a tolerance.

### Alternatives considered

- **The realm absorbing lots or residents:** meaning leaks into the realm, and the realm → town → snapshot → realm
  loop forms.
- **The realm merged into the public pod:** the door and the place would share one process.
- **The realm relaying the town's meaning:** the realm would do two jobs.
- **Connect server-stream plus unary calls:** browsers cannot do Connect bidi. Unary calls would mean about 400
  requests a second at full load, and two channels with no order between them.
- **CRUD tables:** they break the fold rule and give no audit.
- **Pieces anchored to hexes and returned on a layout step, or a land frozen per season:** reserved build zones
  are simpler.
- **Bosses spawned from incidents:** mobs and bosses are game, not ops.
- **TypeScript and Go twins checked by shared goldens:** two copies drift.
- **Rapier or Jolt:** no Go side, tumbling is not needed, and Jolt's prebuilt packages are not deterministic.
- **Full server authority with rewind:** too heavy for 20 friends.
- **One shared HMAC key:** the public pod could mint owner tickets.

### Consequences

- A new stateful service with its own host, database and secret, as one replica with `Recreate`.
- The CSP gains `'wasm-unsafe-eval'` (plan/13 amended).
- The client's collision, motion and land code is replaced by the WASM core.
- The layout gains the build-zone rule.
- Plan/19's "no mobs, no PvP" is reversed (plan/19 amended).

### Not doing

- tumbling physics;
- digging or voxels;
- more than 20 players, sharding or horizontal scaling;
- the realm writing to infra;
- lag compensation;
- the PvP arena before the first slice;
- placing whole buildings;
- plan/20's tunnels (`Land` stays an interface, so a second ground surface fits later).

## Phases

Each phase is a PR through the dev rules (plan/16): plan, doubt, proof, build, review, merge on the owner's word.
The owner plays every visible change before it merges.

0. **Docs:** this plan; the plan/19 and plan/13 amendments.
   - **0b. Infra (infra-bootstrap, stacked, never merged without the owner):**
     - the `ukubi-realm` registry entry and the `realm.bnei.dev` route;
     - its database and secret;
     - the network policy to the public snapshot;
     - the friends' sign-in turned on, which is a precondition for tickets.
1. **One land, then `internal/land`:**
   - the road tie-break by kind (TypeScript first, with the pins re-recorded on the owner's word);
   - the world clock's fixed zone;
   - the Go port of the whole `hexLand` chain (`world-land`, `world-hex`, `world-carts` ways, `world-fun`, the
     grooves, `phi`, `pure`);
   - gates: the existing `pin.spec` and `pin.hash`, a decor, fence and step golden, and the owner-versus-public
     `LandSpec` test.
2. **`internal/physics`:**
   - shapes, a dynamic grid, masked queries;
   - the rider step, with the land composition (walls, swimming, the air peak) as data rather than callbacks;
   - the walk grid and A*;
   - gates: rider traces recorded from today's TypeScript, compared on outcome within a tolerance, and zero
     allocations per step.
3. **The WASM client swap:**
   - the CSP change, TinyGo pinned, and a `build:wasm` step;
   - CI builds the wasm and checks its bytes match between darwin and linux;
   - the client makes one batched call per frame, never a callback per sample;
   - the TypeScript cores are deleted;
   - the owner checks the feel.
4. **Realm skeleton and presence:**
   - `proto/realm/v1`, `cmd/realm` and `internal/realm` (a 20 Hz tick, the event log and its fold, the snapshot
     poller, the land and its hash);
   - tickets minted in both pods;
   - remote riders interpolated in the client, and the banner when the realm is down.
5. **Combat, mobs and a boss:**
   - the attack registry, hit events and health;
   - the hitscan claim check, and projectiles and arcs simulated by the realm;
   - the mob state machine, a spawn-zone registry, and a scheduled boss with phases and telegraphs.
6. **Resources and carrying:**
   - trees, ore and barrels become nodes, each with a stable id, health, and a respawn or reset timer;
   - pick, carry, drop and throw;
   - inventory events.
7. **Building pieces:**
   - a sub-grid inside the build zones, a ghost checked by `Overlap`, a support rule, and the zone owner taken from
     the ticket;
   - a placed piece makes the mobs repath.
   - **The exit slice:** the owner and a friend, on one disc, farm one mob type, chop wood and mine iron, place a
     wall that the mobs path round, and fight one boss.

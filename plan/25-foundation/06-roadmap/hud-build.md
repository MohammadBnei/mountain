# HUD build: round 4 on ui-data (plan/25 part 2)

Builds the HUD direction (03-interview/round-4-hud.md, H-1 to H-14) on the ui-data layers
(04-architecture/ui-data.md, spike in ui-data-spike.md). Checked against ukubi-game main 07cc9306 (after startup S1,
#318), mountain main b90e127, agent-fleet main 666dbdcc, and the startup plan (mountain branch
`plan/25-startup-stream` 30a96e2, local only). A fresh doubt read found 20 issues; all are folded in, and the owner
settled the calls they raised (2026-11-19, below).

## Owner decisions (2026-11-19)

| # | Decision |
|---|---|
| HB-1 | Merge each phase on green CI plus a fresh code review; the visual and feel phases (H1, H2, H3, H6) only after the owner has looked or played. Infra PRs stay drafts for the owner to apply (as plan/21). |
| HB-2 | world.ts gets a code-splitting build, so the owner chunk shares Preact, the signals and the store with world.js. Every dist file is its own `bun build` entry today with no `--splitting` (package.json:8), and the one lazy chunk, mermaid, is loaded through a URL the bundler does not follow and hands back plain data (world.ts:3650-3652, ui-mermaid.tsx:9-13); a second entry importing Preact or the store would carry its own copies. |
| HB-3 | I1's test becomes "every `work` action is reachable in both views", not "opens the same panel" (ui-invariants.contract.test.ts:99-124). A new D-record supersedes D-12. |
| HB-4 | A resident's Talk in strategy opens the Thread face, through an adapter over the resident's lines; the store keeps ids only (I4). |
| HB-5 | The land screen draws mermaid's SVG onto a canvas with a small walker (paths through `Path2D`, rects, circles, text), then a `CanvasTexture` on a plane. The owner pointed at three's `HTMLMesh` (three 0.186.1 `examples/jsm/interactive/HTMLMesh.js`): it paints DOM to a canvas itself, so `img-src 'self'` does not block it, but it draws only text, boxes, `<img>` and `<canvas>`, not SVG shapes. The walker is the same idea for SVG. No CSP change. |
| HB-6 | The dialogue strip ships before agent-fleet's reply-choice and short-line fields; they join when fleet-work ships them. |
| HB-7 | Explore keeps `panel:thread` until the strip lands (H6); strategy gets the Thread face in H3. |

## Constraints from other work

- **Startup (the efficient-loading session).** S2 is in progress (kit.ts, public Cache-Control); S3 and S4 wait for an
  owner checkpoint. S4 moves control to the near ring in `world.ts` (between `space.ready` and `boot.span('playable')`),
  builds `world-terrain.ts` per chunk and moves the minimap's land layer (startup-stream.md:94-112;
  world-minimap.ts:452, 476). So:
  - this build does not change `world.ts`'s boot order; new surfaces mount through the surface table;
  - anything that reads the whole land (the tracker's distances, edge arrows, ◆ marks) reads the entity index after
    `space.ready`, never before `playable`;
  - shared files, edited by both: `world.ts` (the `say()` wiring, the scene feed), `world-scene-api.ts` (the new
    `fast` source for anchors), `world-minimap.ts`. Each HUD PR that touches them rebases on the startup work, not the
    other way round; the minimap marks land after S4 or in their own PR;
  - HB-2's split, agreed with the efficient-loading session (2026-11-19): it lands after S2 (ukubi-game #319); the
    build line in package.json and `dev/budgets.sh` are this build's. S2 needs no change.
- **world.js budget.** The committed `dist/world.js` at 07cc9306 is 2,476,983 of 2,510,000 bytes, about 33 KB; S2 adds
  about 1 KB; S3 about 1-2 KB and S4 perhaps 5-10 KB (the startup session's guess, not measured; both wait on an
  owner checkpoint). The split moves owner-only UI out of world.js; each PR
  reports its world.js delta, and H1 reports its own before H2 starts. A budget raise is the owner's call, never a
  lane's. The deletions that free most room (world-control 17.4 KB, town-card 15.7, town-shell 11.3, world-chrome 9.9,
  town-lenses 9.2, cockpit-tabs 6.6, cockpit 3.7) happen in H3.
- **Fleet-work's files.** No HUD phase changes `internal/play/{agents,residents,cockpit,needs}.go` or `fleet/`.
  Residents' talk (`/api/resident/say`, `/api/voice/visit`, world-talk.ts:1-12) and the needs feed stay as served; a
  resident's reply choices or short line is a request to fleet-work, not an H6 edit. The telemetry forward is a new
  `internal/play/telemetry.go`.
- ukubi-game #317's join-request dock line (src `party`) is repointed in H2.

## Phases

One PR each (H2 is two), in the agreed shape: build fan-out on disjoint files, one integration, gates by angle, one fix
agent per round, PR, fresh review, merge. Lean checks (unit tests, dist identity, the full Go suite, CI, review);
gates before the owner's hands-on only. Visual and feel phases are played on desktop and on a phone emulator (long
routes, real taps) before the owner sees them.

| # | Phase | Repo | Depends on | Owner looks |
|---|---|---|---|---|
| H0 | Contracts and the split | game | the split agreed with startup | no |
| H1 | Layers on the fixture | game | H0 | yes, fixture screenshots |
| H2a | Explore, desktop | game | H1 | yes, plays |
| H3 | Strategy | game | H1 | yes, plays |
| H2b | Explore, phone | game | H2a, H3, the portrait board | yes, plays |
| H4 | Telemetry path | mountain, infra, game | H0 | no (data) |
| H5 | Fleet on generated clients | game | H4's chunk | no |
| H6 | Talk and the land screen | game | H2b, H3 | yes, plays |

H4 runs beside H1 to H3: it is in other repos until its last PR.

### H0: contracts and the split (one opus agent, behaviour-neutral)

- **The split (HB-2):** `bun build world.ts --splitting` with fixed `--chunk-naming`, an owner entry
  (`ui-owner.ts`) loaded by `import()` only on an owner's page, `dev/budgets.sh` lines per chunk, dist identity kept
  (bun 1.4.2). The public page loads only world.js and the shared chunk.
  - Caching: a file is immutable only when asked as `?v=<sha256[:12]>` (`internal/play/immutable.go`), and the server
    adds `?v=` only to html links (pageLinks) and kit manifest paths (kitLinks). A chunk reached by `import()` or a
    static import gets none and is revalidated on every load; `world-voice-lk.js` has the same gap today
    (world.ts:3651). Fixed chunk names plus a `jsLinks` rewrite that adds `?v=` to `./chunk-*.js` specifiers in
    served .js files, one versioning rule; it fixes the voice chunk too.
  - `world.html` carries `<link rel="modulepreload">` for the shared chunk (versioned by pageLinks), so the split adds
    no cold round trip (~150 ms on h3) to the boot path.
- `ui-data.ts`: the UI types (`Series`, `Stat`, `LogLines`, `SpanTree`, `Thread`, `Decision`, `Line`, `Character`,
  `EntityRef`, `Unknown`) and the bundle shape an adapter returns.
- `ui-layout.ts`: the layout data's types (grid slot, anchor with its three targets, accepted types per slot, default
  component) and an empty registry per surface and form.
- `ui-contracts.ts`, in one pass so later phases do not each edit it: the new `SurfaceId`s (tracker, marks, edge
  arrows, prompt, updates, strip, land screen, the zones, the rail, the bottom bar) and their MOUNT rows; the owner
  surfaces in `OWNER_ONLY` (ui-surfaces.ts:48-52); the store's `face` field (kept by I2), the hidden-HUD flag for `\`,
  the talk-mode state (H-11); an `alert` subject level for H-7's default face (levels are cluster, town, app,
  component today). PanelKinds stay until the phase that retires them.
- Tests: I10 (slots name existing types with components) and I11 (AST guard: no component imports generated code or
  a client). I12 moves to H1, I13 to H4. I9's region-count test (ui-invariants:721-723) gets a fixture state per new
  value. The I4 test passes untouched.

### H1: the layers on the fixture, no backend

- Components per type: sparkline and axis chart (`Series`), stat tile, log list with level and text filter and tail
  (`LogLines`), waterfall (`SpanTree`), thread (`Thread`, reusing `ui-thread.tsx` and `ui-markdown.tsx`), decision
  row, line, character chip, `Unknown` with age or reason. Owner-only components sit in the owner chunk.
- The faces' limits as layout defaults: Metrics at most 4 series and windows 15 min, 1 h and 24 h; Logs the last 200
  lines; Spans the last 20 traces and one waterfall; the "Open in Grafana ›" link rendered from the frame's URL.
- The view chip per slot (choice in browser storage, try/catch).
- The anchor layer on `ui-fast.ts`: components render a bounded candidate set as `data-fast` markers; `ui-fast`
  picks the nearest 10 and writes the distances per frame; nothing in the store (I6, I12). A new `SceneApi.fast`
  source feeds it (`world-scene-api.ts`, shared with startup). The anchor rule per `FocusKind` (town-types.ts:499),
  with a surface or edge fallback for `carried`, `inside` and `water`.
- Layout data for desktop and phone, grid areas only, no re-parenting (I5).
- I12's test.
- Proof: the HUD fixture at its 4 sizes, every component and every face on canned data, screenshots in the PR; the
  world.js delta.

### H2a: explore, desktop

- The prompt at the thing ("E Talk", hold E for the radial). `dock:focus` and `dock:line` leave MOUNT at rest on
  desktop; the fight face stays.
- The slide-in updates take the whole `makeLines` queue for every role: the `say()` sites (world.ts:1073, 1128, 1137,
  2369, 2447, 2757-2767, 3554), `fight.say` (1423, 1525, 1669-1735), chat, mail and party news, the guest's
  `VISIT_HINT`, character speech lines (world-characters-scene.ts), with D-09's `mine` lane kept. #317's join-request
  line goes there.
- The Needs you tracker under the map (a click points the map), the light on the switch, amber ◆ marks and edge
  arrows: owner-only (`OWNER_ONLY`; guest and signed fixture rows show them gone).
- Amber means "needs you" only, in 2D and 3D: the minimap's problem beacons become red triangles; in the world,
  `BEACONS.degraded` (world-beacons.ts:16-17) turns red and the at-risk plot (town-looks.ts:117-123) a dashed red
  outline.
- Leave goes in the inside radial (round-4-hud.md:85); D-01's `leave` beside the fight face stays.
- One small Menu button with the sign-in chip; `\` hides the HUD (banner, reconnecting and respawn stay).
- Keys `\` (and H3's) get catalogue rows (ui-catalogue.contract.test.ts) and stay out of `RIDE_KEYS`
  (world-view.ts:201, 1216-1219).
- Tests this phase edits: dev/hud-fixture.mjs:97 (`dock:focus`), ui-surfaces.test.ts and ui-dock.test.ts (dock rows),
  the `line-over` layer (ui-surfaces.ts:159); D-05 and D-10 become superseded records.

### H3: strategy (desktop and phone)

- It opens with an inventory: every item of `panel:cockpit`, `panel:control`, `panel:fleet`, `panel:cluster`,
  `panel:card`'s owner data (round-4-hud.md:89) and the console's commands (town-console.ts:23-40: Fleet, Open repo,
  Logs, Sync, Rollback) and the control room's Pins panel (W2-b), each mapped to a zone, a face or "dropped"; and
  whether the console commands become `work` actions, with their explore presentation (I1).
- Desktop (H-5 to H-8): left zone one list in groups (Needs you, Council, Places); centre the map with lenses; right
  zone faces as tabs (Thread, Logs, Metrics, Spans), the default face by subject (a character Thread, a building
  Metrics, an alert Logs). Zone keys 1/2/3 with no field focused, Ctrl+[ and Ctrl+] (H-14), catalogue rows.
- Phone (H-9): the rail in landscape (Needs you with a badge, Pick, Map, Subject, Explore), the bottom bar in
  portrait.
- Talk: a character's opens the Thread face; a resident's too, through the adapter of HB-4. A thread switch takes 2
  taps (H-13). Explore keeps `panel:thread` (HB-7).
- Opening strategy inside a building (H-1's bridge, `overview` with a subject and a face): four edits, the chart's
  refusal (ui-chart.ts:221), the view's (world-view.ts:2098), the table's `place !== 'inside'` (ui-surfaces.ts:100)
  and the action's `when` (ui-actions.ts:280); the camera stays in the hall; an I9 fixture row `strategy-inside`.
- Faces show `Unknown` ("not wired") in production until H4; canned frames only in the fixture and the sim.
- The old panels and their files are deleted here, freeing world.js room (above).
- Tests this phase edits: I1's test (HB-3, the new D-record), I9's name list (ui-invariants:683-719: `overview`,
  `strategy`, `thread`).

### H2b: explore, phone

First, the portrait explore board (map corner, tracker line, pad) for the owner to pick.
- The prompt at the thing is a tappable primary on coarse input (no key caps, ui-contracts.ts `coarse`), the same for
  engage on mobs, the boss and nodes (D-11); `dock:focus` leaves MOUNT on the phone once that holds.
- The "◆ N need you" line opens strategy on Needs you (H3's destination); the radial's primary act in its centre
  (D-11); the map tap still enlarges; the strategy switch as a Menu row.
- Portrait explore per the picked board.
- H-13's "a far character picked from a list walks toward the owner": needs a summon in world-wander.ts (none today);
  here or in H6.

### H4: the telemetry path (cross-repo, in order)

1. mountain: the protos module (`api/telemetry/v1`), buf pinned, `buf breaking` in CI; atlas's `TelemetryService`
   behind its gate. `Read` marked `idempotency_level = NO_SIDE_EFFECTS` (served as a Connect GET) and `Watch` (POST,
   ending before 90 s) each named in `machineRoutes`, which is method-scoped (machine.go:46), so the principal stays
   read-only; the joins through `contract`; the Grafana Explore URL in each frame; a read-duration metric per source;
   the sim's canned frames per subject.
2. infra-bootstrap: the monitoring network policy lets mountain reach Loki (:3100) and Tempo (:3200). Draft PR; the
   owner applies it.
3. ukubi-game: re-pin mountain; `buf.gen.yaml` (`include_imports`) generating outside websrc's guarded tree (or
   excluded from the I3/I8/CSP guards and `biome ci`), committed, and a CI check that the output matches its pin; the forward handler in `internal/play/telemetry.go` behind the owner gate; the
   generated client and adapters in the owner chunk; `Watch`'s resume through `follow`, with its I8 allow-list entry
   (ui-invariants:520-540).
- I13's test.
- Proof: the faces in the sim and on the cluster for `building:repo:agent-fleet`; atlas's frame poll before and after,
  from the new metric.

### H5: fleet on the generated clients

- `buf generate` from agent-fleet at a pinned main SHA into the owner chunk; `fleet-client.ts` keeps `follow` and
  swaps its hand-rolled transport; I8's allow-list unchanged.
- Proof: the characters' watch, a thread read and a decision answered, on the cluster.

### H6: talk and the land screen

- The dialogue strip (H-10) replaces `panel:talk` for residents and characters, every role, and `panel:thread` in
  explore (HB-7); free text always; open decisions as choices; the model's choices and short line when fleet ships
  them (HB-6).
- The soft talk mode (H-11): the camera frames both, walking off or Esc ends it.
- The land screen (H-12, HB-5): the SVG walker to a `CanvasTexture` on a plane at the character, fading when the talk
  ends; "Keep in strategy" copies it to the right zone.
- The bridge in the Capital hall (H-1): a room per the hall's plan (world-interior.ts, world-capital.ts; W7-c) and a
  door that is the owner's only (I7).
- Tests this phase edits: I1's test drops `'talk'` as a panel; the journey `ride-enter-talk-leave` (dev/journeys.mjs:
  354-406, `#talk`); I8's allow-list entry for `world-talk.ts` (ui-invariants:511) if that file goes.

## Open

- H-13's walk toward the owner: H2b or H6.

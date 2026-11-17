# C1-C2: fleet characters in the game (plan/25 part 2, step S8)

The game side of agent-fleet ADR-0084 §7 (long-lived characters) and ADR-0086 (cards). There is no plan/28: the
fleet contract is those two ADRs plus the amendment the fleet session is writing (below). S8 runs before S7 (owner,
2026-11-17): the strategy lenses need the data S8 brings. Its slices are C1 and C2 (ui-core-build.md already uses
S7/S8 for other steps).

## Owner decisions (2026-11-17)

| # | Decision |
|---|---|
| S8-1 | Characters are the owner's only and all trusted. Guests never reach the fleet (the game's own voice path stays theirs). |
| S8-2 | The persona lives in the agent definition, a reviewed block: `persona {display_name, role, traits[], relations[{to, kind}], portrait, home}`. No separate persona project exists. |
| S8-3 | Characters move freely in the world. Movement is game-side; the fleet gives only `home` (`world`, `village:<id>`, `building:<id>`), the spawn point and wander anchor. |
| S8-4 | Start and Stop are exposed on AgentService, owner-only, so the game's menu runs them. |
| S8-5 | Order errand shows greyed "not yet": today a spawn goes through talk, gated by the character's `spawns.allow` and budget. |
| S8-6 | The game MCP and multi-character scenes are later (ADR-0084 §10). |

## The contract (fleet side, ADR-0084 amendment, owner-only, admin origin, browser to core)

- `ListCharacters` / `WatchCharacters`: per character `name`, `definition {name, sha}`, `persona`, `state`
  (`stopped | starting | idle | running | held | stopping`) and `why`, `threads [{key, title, running, last_at}]`
  (title: the thread's first owner line, 60 chars), the running turn, `budget {turns_today, limit, held}`, `spawns
  {used, limit}`, `can_speak`, `open_cards` (cards whose subject is the character: `CardSubject.kind = character`
  plus its definition name, a proto addition the amendment carries; today `CardSubject.kind` allows pr, issue, alert,
  repo and alert_group only, transcript.proto:131).
- `StartCharacter` / `StopCharacter`: return the new state at once; the watch pushes every transition. This amends
  ADR-0086:234-235 ("steer and stop stay in the console") as well as ADR-0084 §7; `DashboardService.StartStanding`
  and `StopStanding` exist (console).
- `PostToCharacter(character, text, thread_key)` and `WatchSpeech {character, text, face}`: on main today. Speech is
  live only: core stores no line and drops one nobody watches (ADR-0084:55-58), so a line spoken during a reconnect
  is lost; the transcript (`ReadCharacterThread`) is the record.
- `ReadCharacterThread(name, thread, since)` / `WatchCharacterThread`: the console's entry shapes.
- The game server has no connection to characters: the page calls core with core's own cookie, as the card deck
  does (`world-fleet.ts`). The 17 `game-<repo>` residents stay workers on the game's `fleet/` client.

## Game side (ukubi-game, TS; the fleet session keeps `internal/play/{agents,residents,cockpit,needs}.go` and `fleet/`)

1. **One fleet client.** `world-fleet.ts`'s `call()` and `ui-live.ts`'s `cards()` copy become one module. Watches
   are Connect server streams read with fetch (enveloped `application/connect+json`); core ends a stream at
   `streamMaxAge`, so reconnecting is routine, and a dropped list watch falls back to the 20 s poll (in ui-live.ts,
   invariant I8). 401 keeps "Sign in to fleet"; 403 warns once; a stream error arrives in the end frame (HTTP 200).
   Speech and thread text never enter `UiState` or ui-contracts (I4): they stay in the scene and the panel's reader.
2. **Characters as a second source inside world-people.** A `Placed` source next to the residents with its own route
   (spawn at `home`, wander the land's ways within a radius, page-local, the same path for a seed and day), so the
   nearest-8 draw cap (`MAX_DRAWN`, world-people.ts:865-920) covers residents and characters together. A stopped
   character stands at home. `at()` returns the wander position even when the figure is not drawn, so the council and
   focus can reach a far character (world-entities.ts:18). Registered in the entity index (`kind:'character'`,
   `live:true`); `KindId`, `FocusKind` and a `TargetKind` with `owner:true` (world-targets.ts:56-67, 406-408) gain
   `character`, so `people.hitOf` does not pick it as a resident.
3. **Looks and portraits, no new art.** A character wears a rider class body and the wardrobe (world-avatar.ts) in
   its persona colour; `persona.portrait` names the class and colour. The portrait is the figure rendered once to a
   small canvas (the game's own pixels, `img-src 'self'` unchanged). `face` is ignored until the kit has expressions.
4. **The CK menu.** `FOCUS_ROWS` rows for `character`, all `roles: OWNER`: Talk, Ask about, Inspect run, Start,
   Stop (two rows: labels are static, ui-contracts.ts:241), Order errand (greyed "not yet"). Character state reaches
   `when`/`why` through new `ActionDeps` and `SceneApi` reads. Ask about prefills only the owner's own buildings,
   villages and residents, quoted as data, never a name another player chose (the message is trusted T0).
5. **Speech.** In explore a line is the figure's bubble and the dock's talk line when the figure is drawn, else a
   notice; the open transcript shows it too. Lines lost in a reconnect are not recovered (the transcript is the record).
6. **The transcript.** The existing covering `thread` panel (`COVERING`, ui-chart.ts:49-58) reads and watches the
   character's thread (`Read/WatchCharacterThread`), one tap from the bubble or the dock line (P2-9).
7. **Needs.** A character card opens in the existing `card` panel with the character's portrait and name; other
   cards are unchanged. A new card while the owner plays is a notice, never a modal.
8. **Council, in the overview.** The strategy overview (ui-overview.tsx, plan/24 §7) gains a council section: the
   world character as ruler, village characters as councillors (from `home` and `relations`), each with state, budget
   and open cards; selecting one focuses the figure. No new panel kind (one component family).
9. **Touch points.** `world.ts` registrations only (the entity register at ~2350, the hover and pointer kind checks
   ~2303 and ~2713, the new `SceneApi` reads), no composition-root moves, edit windows agreed with the fleet session;
   world-people.ts; world-targets.ts; ui-actions.ts and ui-contracts.ts; ui-overview.tsx; hud-fixture states for a
   held character's greyed rows and the council section. Tests that change: ui-actions.test.ts `KINDS` and `ORDER`,
   world-targets.test.ts (the kinds list), and a test that the public page's `kindsFor` has no `character`.

## Slices (each a PR: claims checked at file:line, a doubt round, a mutation per new test, hands-on at phone size)

- **C0, a spike (hours):** a browser fetch reading `WatchSpeech` from fleet.bnei.dev with credentials over the real
  proxy (Cloudflare idle timeout, buffering), so C1 knows whether streams hold or poll is the norm.
- **C1, after a dev core accepts the local origin** (`FLEET_ADMIN_ORIGINS`) and ops-lead is started with a `speak`
  grant: the one fleet client; the character source, wander and entity kind for ops-lead with a hard-coded `home`;
  speech bubbles; a Talk row (PostToCharacter) and the transcript panel read.
- **C2, after the amendment lands on core main:** List/WatchCharacters, the remaining rows with Start/Stop, the
  character cards in `card`, the council section, portraits, fixture states.

## Proofs

- The wander, by properties (no position goldens): the same seed and day give the same path; the figure stays on
  ways, within its radius of `home`, off water; a stopped character stands at home.
- The menu: each row's `when`/`why` per state (stopped, starting, idle, running, held, stopping); every row is
  owner-only (a friend's and a guest's chart show none: I7).
- The client: a fake core (recorded Connect frames) for list, watch, stream end and reconnect, poll fallback, 401, 403.
- Hands-on, phone emulator, real taps: tap ops-lead, open its menu, Talk, see the bubble and the transcript; stop
  core mid-stream and see it recover; sign out of the fleet and see "Sign in to fleet"; the public page shows no
  character. Browsers closed and stacks stopped after.

## Security

Characters are the owner's: `api/fleet` is owner-only (main.go:172), the public pod refuses `--fleet-web`, core checks
the caller on every RPC, and characters are page-local, so the realm never sends them to anyone. A friend beside the
owner sees the owner talking to nobody. Every check beyond the hidden rows is core's.

## Open (to settle with the fleet session in its doubt pass)

- Connect server streaming from the browser over HTTP/1.1 (core's Connect handler must serve `application/connect+json`
  streams to the admin origin with credentials); else the watches are polled.
- `home` values for ops-lead (world), and how village ids match the game's village ids.
- Thread titles: the 60-char first line is enough for now; explicit titles later if the panel needs them.

# S8: fleet characters in the game (plan/25 part 2)

The game side of agent-fleet ADR-0084 §7 (long-lived characters) and ADR-0086 (cards). There is no plan/28: the
fleet contract is those two ADRs plus the amendment the fleet session is writing (below). S8 runs before S7 (owner,
2026-11-17): the strategy lenses need the data S8 brings.

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
  {used, limit}`, `can_speak`, `open_cards` (cards with subject `standing/<name>`).
- `StartCharacter` / `StopCharacter`: return the new state at once; the watch pushes every transition.
- `PostToCharacter(character, text, thread_key)` and `WatchSpeech {character, text, face}`: on main today.
- `ReadCharacterThread(name, thread, since)` / `WatchCharacterThread`: the console's entry shapes.
- The game server has no connection to characters: the page calls core with core's own cookie, as the card deck
  does (`world-fleet.ts`). The 17 `game-<repo>` residents stay workers on the game's `fleet/` client.

## Game side (ukubi-game, TS; the fleet session keeps `internal/play/{agents,residents,cockpit,needs}.go` and `fleet/`)

1. **One fleet client.** `world-fleet.ts`'s `call()` and `ui-live.ts`'s `cards()` copy become one module, plus a
   Connect server-stream reader for the three watches, with a poll fallback (the deck's 20 s poll) when a stream
   drops. 401 keeps "Sign in to fleet"; 403 warns once.
2. **Characters as a figure source.** A small wander module: a character spawns at its `home` and walks the land's
   ways near it (`roundOf`'s ways, not the residents' building round), page-local and deterministic per day like
   residents. Drawn within the shared `MAX_DRAWN`, a distinct look (not the tinted barbarian). Registered in the
   entity index: `kind:'character'`, `verbs:'character'`, `live:true`; `KindId`/`FocusKind` gain `character`, so
   pick, focus and follow work as for residents. A stopped character stands still at home, greyed.
3. **The CK menu.** `FOCUS_ROWS` rows for `character`, all `roles: OWNER`: Talk (opens the thread panel on a new
   or the last thread, `PostToCharacter`), Ask about (Talk prefilled with the focused thing's name), Inspect run
   (the thread panel on the running or last thread), Stop / Start (one act, `when` by state, `why` from the
   fleet), Order errand (greyed "not yet"). Each greyed row shows the fleet's `why`.
4. **Speech.** `WatchSpeech` lines become the figure's bubble (`People.bubble` pattern) and, while the thread
   panel shows that character, its live line; `face` picks the figure's expression where the look has one.
5. **The thread panel (P2-9).** `ThreadRef` panels read and watch the character's thread (`Read/WatchCharacterThread`):
   bubbles in explore, the side panel in strategy and for runs.
6. **Needs as event windows.** Cards with subject `standing/<name>` open as a window tied to the character (its
   portrait, the card's options); other cards keep the deck (`panel:card`). A new card while the owner plays
   surfaces as a notice, never a modal.
7. **Council.** A new owner panel `council`: the world character as ruler, village characters as councillors
   (from `home` and `relations`), each with state, budget and open cards; selecting one focuses the figure.
8. **Portraits.** `persona.portrait` is a key into the game's own portrait registry (images served by the game,
   `img-src 'self'` unchanged); a missing key falls back to an initial on the character's colour.
9. **Surfaces.** `PanelKind` gains `council` and `character`; hud-fixture states for each (owner; a held character
   with its `why`; a stopped one), copied from the `thread` and `card` states.

## Slices (each a PR: plan claims checked at file:line, a doubt round, mutation per new test, hands-on at phone size)

- **S8a, no fleet change needed:** the one fleet client with stream reader; `WatchSpeech` and `PostToCharacter`
  wired to a hard-coded owner test character behind a dev flag; the figure source and wander; the entity kind.
- **S8b, after the amendment lands on core main:** `ListCharacters/WatchCharacters`, the menu rows, Start/Stop,
  the thread panel reads, need windows, council, portraits, fixture states.

## Proofs

- The wander is deterministic: a golden of positions over a day per seed.
- The menu: each row's `when`/`why` per state (stopped, starting, idle, running, held, stopping) in `ui-actions.test.ts`;
  every row is owner-only (a friend's and a guest's chart show none: I7).
- The client: a fake core (recorded Connect frames) for list, watch, stream drop then poll fallback, 401, 403.
- Hands-on: ops-lead (the first character) on a local stack against a dev core, at phone size; screenshots of the
  figure, the menu, a bubble, the council.

## Open (to settle with the fleet session in its doubt pass)

- Connect server streaming from the browser over HTTP/1.1 (core's Connect handler must serve `application/connect+json`
  streams to the admin origin with credentials); else the watches are polled.
- `home` values for ops-lead (world), and how village ids match the game's village ids.
- Thread titles: the 60-char first line is enough for now; explicit titles later if the panel needs them.

# Interview round 3 (2026-11-14, after the baseline numbers)

- **Quick fixes.** Answer: all four now, "via workflows with appropriate model strength and a single gate at the end",
  using what we learned about workflow settings.
- **Determinism.** Answer: **bit-exact**. Trig is ported to pure Go, a lint bans `math.Sin`, `math.Cos` and
  `math.Atan2`, and the server replays inputs exactly. The trial port showed the cost: +3,420 bytes of WASM and the
  tests green.
- **Strategy view.** The owner's words: "I am thinking about Civilisation 5/7, or Age of Empires 3, with a sprinkle of
  Crusader Kings. But I think we'll go with a clean separate mode to make them free of each other, reusing the same
  3D though. Unsure about that, maybe we need a specific round on this subject." So the strategy view gets its own
  round, below.
- **What the shared sim core covers in v1.** Answer: the rider and the ship, mobs, residents, and critters, animals
  and carriers. The owner adds: "physics too, to enable racing, PvP and boss fighting?" So the core must carry
  authoritative contact between players and with bosses, not only movement.

## Round 4 (to come): the strategy view (settled in round-4-hud.md, 2026-11-18)
- The references: Civilization V and VII (hex map, the yields read on the map, lenses), Age of Empires III (a home city,
  a real-time economy) and Crusader Kings (characters, relations, intrigue).
- The working bet: a separate mode on the same 3D scene, with its own HUD, kept free of explore.
- To settle:
  - what the strategy layer is for in ukubi's terms (the cluster console, the town, the guilds);
  - what carries over from each reference;
  - how you enter and leave the mode;
  - phone against desktop.

### Round 4, first answers (2026-11-14)
- **What it is for:** running the cluster, growing the town, and people and relations. Military and the realm are left
  out for now.
- **Feel:** "CK first, but Civ lenses just behind". Characters and relations lead; the Civ map with its lenses comes
  second.
- **Who:** the owner and admins only. Public visitors stay explore-only (ukubi-game #242).
- **Phone:** the full mode, touch-first.
- **More context:** a sibling session (the "scene interview") is working on this subject in parallel. Its findings
  feed this round before `04-architecture/ui-core.md`.

### Input from the scene research (session ukubi-9f, 2026-11-14)
That session is in a pure research phase. Its positions are owner-confirmed research, not a frozen spec, and its
architecture v2 was not adopted. The log and the files are in `~/Code/ukubi/design-2026-11/` (`doubt-decisions.md`
holds the Q numbers below).

What its research positions say about the strategy layer:
- **The game is the real work:** improvements, feature development and alert handling (Q27). The owner authors or
  triggers most of it (Q28).
- **Reward:** the realm grows from real deeds, with no points (Q29). A feature becomes a house and roads, an
  improvement mends a stratum, and a resolved alert becomes a saga.
- **The unit is an Operation over agent sessions** (Q34–37).
  - It is done when the graph verifies that reality changed.
  - Questions wait with nudges; nothing approves itself.
  - Risk is shown as zones that mirror the real enforcement.
- **A "War table"** is the live operations view (Q48). The 3D Hall loaded too slowly on the phone (25–40 s), so a flat
  view was recommended. The owner has not decided that.
- **Projection-based and schematic** (Q59): "if we do this that way, here is the possible result". The visual grammar
  takes after lot-map: pulses, fading routes and clock hands (Q68).
- **An inbox for every agent's decision cards** (Q64–66).
- **Current state is never "fine" by default** (Q73–82). It shows live data, else the last observation and its age,
  else "unknown".

Still open there:
- the projection interface;
- whether the War table is flat or 3D;
- the quiet-day loop (prototypes A to C, Q55);
- where combat fits.

**What this means for ui-core.** The strategy mode should be the screen for operations, characters and projections.
It may well be flat or schematic over the shared 3D scene, not a camera mode. The ui-core round must keep both open,
and it must not fix a shape before the scene research does.

### Owner requirement on the two modes (2026-11-14)
The owner's words: "I still want somehow to be able to switch between explore or strategic, and have access to the
same possibilities."

What this binds `ui-core.md` to:
- **One switch.** Explore and strategy are two presentations of one state. You can switch at any time, and the
  selection, focus and open work carry over.
- **The same possibilities.** Every action is defined once, in one action layer that knows nothing of the view. Each
  mode only presents those actions:
  - explore through the dock, the radial and the panels;
  - strategy through the board, the cards and the inbox.
  - Round 4 (round-4-hud.md, "The principle") replaces these two presentation lines; the rule itself stands.
- **A check.** A test lists the actions reachable in each mode and fails when the two lists differ, the same way the
  router's AST guard checks keys today.
- **Left open:** how the strategy mode looks (flat or schematic, or the same 3D). That waits for the scene research
  (above).

### Open plan/24 HUD points, handed over by session ukubi-73 (2026-11-14)
The HUD shipped in ukubi-game #252 (`783426e`). These points are open:
1. **Phone Settings.** It opens with `showModal()` (`world-settings.ts:52`), so while the panel is open the Menu's ×
   at the bottom is visible but does nothing. The panel's own × and Esc still close it.
2. **Owner page load.** The page loads in the overview, where the Menu reads "Close". So the first Menu press closes
   the overview instead of opening the radial.
3. **Overview pick.** A pick shows in the overview panel's selection view. The details drawer opens only through
   `#portrait .details` (changed in `3c98e1d`, because on phones the drawer, opened by itself, covered the Menu).

Points 2 and 3 sit in the overview, which plan/25 supersedes (plan/24 rows 8–9), so the new strategy mode settles them.
Point 1 is an input for ui-core's rule that a modal panel and the Menu's close stay consistent.

**Merge order agreed with ukubi-73:**
- #253 merges first.
- plan/22 P3–P5 (mail, voice, party; `internal/realm`, `world-realm*.ts`, `world-people-panel.ts`, `world-dock.ts`,
  `world-voice.ts`) rebases onto it.
- The ui-core migration comes after plan/22's UI lands.

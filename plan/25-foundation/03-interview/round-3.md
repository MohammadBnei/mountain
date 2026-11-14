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

## Round 4 (to come): the strategy view
- The references: Civilization V and VII (hex map, the yields read on the map, lenses), Age of Empires III (a home city,
  a real-time economy) and Crusader Kings (characters, relations, intrigue).
- The working bet: a separate mode on the same 3D scene, with its own HUD, kept free of explore.
- To settle:
  - what the strategy layer is for in ukubi's terms (the cluster console, the town, the guilds);
  - what carries over from each reference;
  - how you enter and leave the mode;
  - phone against desktop.

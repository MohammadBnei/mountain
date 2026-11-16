# Interview rounds 1 and 2, and the doubt round (2026-11-14)

Each question was asked with options. The research behind it is in `../01-baseline/audit.md`.

## Round 1
- **What must "well engineered" buy first?** Options: cheap change, fewer regressions, room to grow, all equally.
  Answer: all equally.
- **How bold?** Options: seams in place, new cores, let the research decide. Answer: new cores.
- **Which order?** Options: UI first, world model first, physics and agents first, research all and then pick.
  Answer: research all, then pick.
- **Where do the folders live?** Answer: `mountain/plan/25-foundation/`.

## Round 2
- **UI core.** Answer: a small reactive library is allowed.
- **The four views.** Options: one layout with variants, keep four layouts, rethink the strategy view. Answer:
  rethink the strategy view.
- **Authority.** Options: a shared fixed-step core, client-trusted with better checks, fully server-authoritative.
  Answer: a shared fixed-step core.
- **Who owns the placed world.** Options: the server sends the placed world, a shared Go library on both sides,
  streaming and chunks too. Answer: streaming and chunks.

## After the doubt round
- **Supersede plan/24 rows 8–9 now?** Answer: yes, now.
- **Streaming target?** The owner's words: "The loading of the world is a few seconds. With streaming, we could break
  that down? It also loads on each reload of the page." The real target is load and reload time, so it is measured
  first.
- **The old district layout.** The owner's words: "With the new implementation cleaning up the engineering, we can map
  out the land much more efficiently! Retire the town page."

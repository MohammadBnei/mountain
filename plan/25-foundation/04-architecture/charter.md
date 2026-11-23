# The builders' charter (ui-core)

Read this before touching code. It lets you decide close calls yourself. Write a decision record
(`../06-roadmap/decisions.md`) only when the persona who owns the surface can't settle the question, or when the door
is one-way: a data shape, the wire, or removing a feature.

## What we are building

ukubi is a game world that is also the owner's workplace. One state, shown two ways:

- **Explore.** You ride, walk, meet the agents in their buildings, interview them, and play. This mode is for
  discovery, ideation, interviews and `play`.
- **Strategy.** The same state on a map that zooms from the cluster to a town to an app. It is CK first (characters,
  relations, who waits on whom), with Civ lenses behind. This mode runs the real work: operations, arbitration,
  cluster-wide changes.

You can switch between the modes at any moment. Switching keeps the subject, the selection, the open thread and the
operation.

The shared story is the workshop loop, in `../03-interview/scenarios.md`. It is built from four primitives:
- **subject:** cluster, town, app, component;
- **architecture model:** atlas facts, plus what an agent has read;
- **character:** an agent with a persona and a place;
- **thread and operation:** the conversation (kept in agent-fleet core) and the lifecycle of its work.

## The personas: ask what makes sense for them

- **Mo, the architect and admin.** The owner, a senior engineer who runs the cluster and ships features through
  agents.
  - **Where he works.** On desktop, in strategy, for real work, with long threads. In explore, to wander and interview.
    On the phone, to answer a card or check an alert.
  - **What he hates.** Clutter, slow paths to the work, and anything fake: rooms that lie, data that pretends all is
    fine.
  - **What he loves.** A calm screen, the game feel while exploring, and a world that grows from real work.
- **Sam, a recruiter and a guest.** Public and explore-only, usually on a phone. Sam is exploring Mo's work: what Mo
  built, how, and how well. Sam must understand it within seconds and never sees an owner action.
- **Nico, the player.** The MMO side, explore-only: combat, racing, riding, building, guilds. Nico wants a one-thumb
  action-RPG on the phone and a keyboard flow on desktop.

**Who wins a tie, by surface:**
- Mo on owner and `work` surfaces;
- Sam on public surfaces;
- Nico on `play` surfaces.

## The owner's standing design rules (decided; don't reopen)

**The HUD** (plan/24):
- At rest, the screen shows only a small map corner and one Menu.
- One bottom dock shows context: the focus (a name, the primary action, hold for more), the fight (attack, skills,
  ultimate), and incoming lines that slide in.
  Superseded by round 4 (03-interview/round-4-hud.md, 2026-11-18): no dock at rest; the prompt at the thing, the
  radial, the slide-in updates and the Needs you tracker take its content; the fight face stays.
- Social lives in panels.

**Calm and coherence:**
- Use fewer pieces, merged and grouped.
- Use one component family: one glass style, one radius scale, one button size per tier.
- Judge every change against the whole screen, never one element alone.

**The look** (`ui-theme-intent`):
- Neon glass in the Arcane (violet and teal) and Cockpit (blue and amber) themes, each dark and light.
- Jost, with JetBrains Mono for numbers.
- Restrained glow, only on interactive things. Legibility wins over spectacle.

**Input** (plan/23):
- One router and context stack, with no listener races.
- A tap goes by what the thing offers (plan/25 P2-2'): a thing whose only verb is Go onto is walked to; a building, or a thing with several verbs, opens its radial at the screen centre; a mob, the boss and a node are selected first, the second tap engages; the ground walks. A hold opens the radial too. A verb that cannot run now is shown shut with its reason.
- Esc, M, Enter (chat while exploring outside), Tab and ? are globals.
- Keyboard shortcuts keep working for everything that is removed.

**The phone:**
- Tap only. Tap to walk, with the pathfinder.
- Select-then-engage for an enemy out of range.
- Prompts open on a tap, never by themselves.
- The left half and the centre stay free for the camera.
- Touch targets are at least 44 px.

**Movement:**
- Running is the default.
- The bike and the ship keep their own controls.

**Who sees what:**
- The public world is explore-only (ukubi-game #242).
- The owner surfaces need sign-in.

## The invariants (never trade these)

I1–I9 in `ui-core.md`:
- work parity;
- the state kept across the switch;
- one form module;
- no conversation held by the game;
- one DOM;
- no store change per frame;
- the public view a subset;
- no polling;
- every state has a table row and a fixture row.

Security and the CSP (`style-src 'self'`, no inline script) are never traded either.

## When rules pull apart, in order

1. The invariants and security.
2. The owner's explicit decisions: the plan rows, the interviews, this charter.
3. What the persona who owns the surface needs.
4. A calm screen, then consistency.
5. The smallest code.

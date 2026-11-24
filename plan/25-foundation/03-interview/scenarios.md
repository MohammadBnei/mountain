# Scenarios: the workshop loop

> Status: history (2026-11-24). Interview record.

Status: the owner's story, 2026-11-14. It came out of an ideation round (`idea-refine`) after interview round 3. It is
the shared picture `../04-architecture/ui-core.md` and `world-model.md` must serve. It is not a test list yet.

## Problem statement

How might we move the owner's real feature work onto ukubi? That work is plan, interview, doubt, build, gates, review
and merge. Explore would carry the discovery, the interviews and the sketches. Strategy would run the work. Both would
show one shared state, so the owner can switch at any moment.

## Who and why

This is for the owner first, and later the admins. Most of the owner's work is feature development, with alerts and
incidents a close second. Alerts too usually lead to an upgrade, which is a feature.

## The roles of the two modes (owner, 2026-11-14)

- **Strategy** is for real, advanced work: running operations, arbitration, cluster-wide changes. Its map zooms from
  the cluster to a town (a logical group, still to define) to an app, like Civilization. The characters are
  CK-style portraits.
- **Explore** is for exploration, ideation, interviews and, later, projection.
- **The switch** keeps the subject, the thread and the operation. Each action is defined once and is reachable from
  both modes.
- **The phone** comes later, step by step: first a simple stack of decision cards with answers, full parity later.

## The loop

1. **Ride to the app (explore).** You enter app X's building, and its architecture is laid out as rooms. Atlas facts
   (pods, services, data stores, the links between them) are drawn solid. The agent's reading of the repo is drawn as
   chalk, marked "read by agent".
2. **Meet its lead dev (explore).** This is a discussion close to a session with Claude Code, not decision cards.
   - **The persona.** The agent is the app's lead dev, with a name, a manner and a memory per app. The persona is
     derived from the repo (its CLAUDE.md, ADRs and history), and the owner tunes it once. The frame is the company's
     architect visiting the dev of the app.
   - **It knows its whereabouts.** Each turn carries:
     - the subject;
     - the room you both stand in;
     - what is in your view;
     - what you just pointed at.
   - **Its tools are grounded in the place:**
     - the repo;
     - atlas live facts;
     - its memory of past threads here;
     - the world's own tools: walk to a room, mark a wall, pull a live gauge onto a wall.
   - **The talk.** Text streams, with markdown and code, and long answers when needed. Questions come in prose, with
     option chips you can tap or ignore.
   - **The space joins in.** A tap on a room or a pipe enters the thread as "this". When the agent cites a file, its
     wall lights. "Let me show you" means it walks, and you can follow. The wait for a reply reads as the agent
     turning to the wall to read.
   - **Voice comes later:** push-to-talk, speech in and text out. The text thread stays the record.
3. **See the change while you talk (explore).** There are two levels from the start:
   - **Point** (always, free): you tap, the agent highlights, and you both look at the same thing.
   - **Sketch** (during the talk, the agent's choice, cheap): chalk marks show what changes, for example three
     buckets drawn beside a pipe.
4. **Switch to strategy.** It opens on app X at the app level of the map. The same thread is open. The plan's steps
   sit on the schematic.
5. **Run it (strategy).** You approve. Doubt, build, gates and review run as a visible operation, and only your
   decisions block. They show as "waiting on you" in both modes.
6. **Go cluster-wide (strategy).** If the change needs the whole cluster, you go to the cluster agent's portrait. A
   child operation starts, linked to the first.
7. **Merge (either mode).** The rooms are rebuilt from the new atlas facts, and the town grows.

**A sample meeting**

> **You** (in the queue room, tap the pipe to `notify`): Why does this go straight through? I'd want it batched.
> **Mira, lead dev of app X** (walks to the pipe; the wall lights `jobs/dispatch.go:88`): Because notifications were
> rare when we built it. Atlas says 1.2k a minute now at peak. Two ways: batch in the dispatcher (small, local) or put
> a stream between us and notify (bigger, and the cluster has NATS already). Which do you lean to? Mostly I worry
> about ordering: notify assumes order per user.
> **You**: Batch, but keep order per user.
> **Mira**: Then per-user buckets, flushed every 500 ms. Let me draft it. *(turns to the wall; a chalk sketch appears
> beside the pipe: three buckets)*

## Later: projection (an add-on, not needed from the start)

Projection is triggered only when a plan is approved, or when you ask for it. Drawing the ghost is instant, because it
is a local change on the architecture model. The slow part is the agent turning its plan into a structured change
(add, move, remove), about 10 to 60 seconds, which runs in the background. When it is ready, you walk the after-state
as ghost rooms, and strategy shows a before and after. What you correct while walking goes back into the thread.

## The four primitives both modes render

- **Subject.** What you are looking at. It nests: cluster, town, app, component.
- **Architecture model.** The atlas skeleton plus the agent's additions, each part marked with where it came from.
- **Character.** An agent with an identity, a persona, relations and a place.
- **Thread and operation.** The session, and the lifecycle of the work it starts. Neither is tied to a mode, and an
  operation can have child operations.

## Assumptions to check, and how

- [ ] **The meeting is as good as the CLI.** Spike: one agent-fleet session in a game thread panel. The owner uses it
      for one real small feature.
- [ ] **The rooms are true enough to point at.** Spike: build the model for ukubi-game itself, walk it, and point at
      three things.
- [ ] **Spatial context helps the agent.** Run the same question with and without the subject, room and pointing
      context, and compare the answers.
- [ ] **The owner's 10-step shape fits one operation lifecycle.** Write the state machine and replay one past PR
      (#252) through it.

## Not doing yet, and why

| Item | Why |
|---|---|
| Projection | It is a nice add-on, after point and sketch |
| Voice to the agent | Text quality comes first |
| CK diplomacy between agents | It needs agents that negotiate; it comes after one agent works well |
| Full phone parity | A card stack comes first |
| Guests and realm play | They are not the owner's real work |
| Proposals from the graph | The owner authors most work (scene research Q28) |

## Open questions

- What a "town" is at the map's middle level.
- The agent-fleet seam the thread uses: a live session, its permissions, and what an agent may do from inside the
  game.
- How this joins the scene research's Operation and War table (ukubi-9f), so the two do not diverge.

## Inputs for world-model.md

- **mountain #99** (draft, ukubi-9f, branch `door/f-slice1`) adds `graph.Kinds` in `internal/graph`: a map from each
  of the 13 kinds to a one-line meaning. `Valid`'s id rule is built from it. The PR also adds two new kinds, `pg:`
  (a Patroni member) and `endpoint:` (a VIP or a redirect Service). This is the registry that maps a kind to a room.
- #99 merged to main as `0e212bf` (2026-11-14). No ukubi-game re-pin needed.

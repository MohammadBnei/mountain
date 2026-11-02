# 19 · The storyboard: work can be played

Status: draft 2, 2026-11-01. Written from the owner's ideation, interview and first doubt cycle; the second doubt
cycle's answers are in § After the second doubt cycle, which wins over the sections above where they differ.

Plan/18 built the map: the capital, the bazaar, the villages, the control room. This plan gives the world its story,
and the one quest every visitor plays. It replaces plan/18's "plan 19" row (quests, lore, mobs, progress store, LLM):
the general quest engine is dropped, and lore shrinks to what the story needs. (Mobs, dropped here at first, are back
  with plan/21, 2026-11-02.)

## Intent (confirmed by the owner, 2026-11-01)

- **Premise:** the disc is a self-portrait of the owner: the owner's work, the cities the owner lived in or loves, the owner's readings
  (Pratchett's Discworld gives the turtle, the elephants and the disc). It is also a working thesis, and the next piece
  of the owner's career: **make work fun, literally**. The land, the towns and the roads are the owner's real systems, read
  acted on and evolved through this game.
- **What a visitor leaves with:** "I tried his apps", "I get what he built", "I want to come back", "this is fun", "I
  want to learn how he did it", and above all the question: *what if I could play my work too?*
- **Users, in order:** friends first, then engineers and LinkedIn contacts. Recruiters and engineers will not play for
  hours: the impact lands within minutes.
- **The owner's role:** the keeper, living in the same world. Players online at the same time meet the keeper in
  person (plan 20), and the keeper answers their letters from the deck anywhere.
- **Coherence:** the owner's admin world and the players' world are one place seen from two sides. Real events reach
  players only as mood.

## Owner decisions

1. **Stall speech is LLM-driven.** The stallkeeper voices the owner's fact card, the app's public stats, the lore,
   and a condensed memory of the player (signed-in players only). It never invents an app's features.
2. **The builder's LLM is small and guided.** The options are deterministic (a fixed list of archetypes); the LLM
   gently steers the player to the closest one and dresses it (name, pitch, colours). The steering is invisible to
   the player; the final card says how it worked.
3. **Pieces come from the owner's real stack only.** Where a textbook would choose differently, the card says so:
   "a textbook would use X; in this realm we build with what the realm has."
4. **No intro flyover.** The visitor spawns on foot, as plan/18 V3 built it.
5. **The fork comes before the climax.** A guest builds and connects the app, then chooses: roam free on foot, or
   sign in to finish the challenge (the crowd and the pattern) and get the bike.
6. **Everyone hears the question**, guests too, as they leave or finish.
7. **Visitor data is written by the owner pod**, relayed by the public pod (see § The write path).
8. **The cluster deploys everything.** Cluster down means nothing faces users: no off-cluster fallback, no static
   page. Plan 19 ships when the cluster is back.
9. **LLM budget:** guests about 6 calls a session, then scripted lines; signed-in players uncapped, under one global
   daily cap the owner sets in the control room.
10. **The core lasts 5 to 10 minutes**, cut down through tests with real people.

## The core quest

| # | scene | time | who |
|---|---|---|---|
| 1 | Arrival: on foot in the spawn square, lanterns and the beacon. The spawn stone reads "This world runs on real systems I built" and links to the blueprint page. | ~30 s | all |
| 2 | The bazaar: one stall, the stallkeeper talks (picked, never on its own: plan/18 decision 4), the player opens the real app. | 1–2 min | all |
| 3 | The builder's yard: a short talk with the builder steers the player to an archetype; they place the app and connect it to Postgres. | 2–3 min | all |
| 4 | The fork: roam free on foot, or sign in to finish. | — | guests |
| 5 | The challenge: a crowd arrives, the road jams, the player picks a pattern; a wrong pick jams visibly, a right one flows. | 1–2 min | signed in |
| 6 | Joining the disc: the lot stays under the player's chosen name, the camera pulls out wide, the bike is theirs. | ~30 s | signed in |
| 7 | The question: the keeper's note, "This is how I work. What would yours look like?", the owner's contact, the letterbox (signed in), and the card "how this was built". | ~30 s | all |

A guest who roams free still gets scene 7, as a note that finds them on the road. Quest 1 of plan/18 (reach the
bazaar, open an app) becomes scenes 1–2. The bike moves from quest 2 to scene 6; a visitor whose browser already holds
the bike keeps it.

## The builder's yard

- **Where:** its own area beside the capital, reserved by the server like the info lots (town.InfoLots), outside the
  real layout's input. Player lots never move, crowd or rename a real building, and never enter the layout event.
- **Archetypes:** a fixed table, about 12, owned by the owner (an open question below). Each holds: an id, a building
  model, the pieces it needs (an app, Postgres, sometimes S3 or a cache), its crowd and the pattern that fixes it, and
  three pitch seeds.
- **Pieces:** a catalogue from the real stack, each with its real name, a plain line for friends ("where the app
  remembers things"), and where relevant the textbook note. Only what the owner runs.
- **The builder's talk:** the visitor types freely. The server sends the archetype list and the visitor's lines to the
  small model and asks for JSON: `{archetype, name, pitch, colours}`. The server validates it: the archetype must be
  in the table, the name and pitch pass voice.Clean and length caps, the colours come from a palette. Anything else
  falls back to the closest archetype by keyword, then to buttons.
- **The challenge:** an outcome table, not the sim. Each archetype's crowd has one right pattern and one or two wrong
  ones; each pick maps to a fixed animation (carriers flow, or queue and jam). Deterministic, cheap, the same for
  every player. The sim stays the owner's.
- **Persistence:** a signed-in player's lot (archetype, name, pitch, colours, display name) lives in the visitor store.
  A guest's lot lives in the browser and fades when they leave.

## Stalls

- **The fact card** (per stall, edited in the control room's Showcase editor, which grows these fields): what the app
  does, who it is for, its URL, two or three facts, what it must never claim, and which public stats it may cite.
- **The stallkeeper's line:** the model gets the card, the allowed stats, a lore snippet and the player's memory, and
  answers in the keeper's character. Fallback when the budget is gone or the model is down: a line read from the card
  itself, never a generic canned line (today's voice.canned picks by hash and knows no facts).
- **Memory:** signed-in players only, condensed by the model to a few lines after a talk, stored in the visitor store,
  removed by deletion. Guests have none: client-held memory would be prompt input from the browser.
- **Shutters:** as plan/18 V3. Voco'steroid lives on its own domain; its up or down comes from the same coarse
  probe as the others (an open question if it runs off the cluster).

## The keeper

- The owner plays in the same world. Owner-only data shows on the owner's screen only.
- The keeper is never on any map or minimap. When the owner is online, a lantern lights on the clock tower. Players
  who come near see the keeper (plan 20: presence needs a realtime channel).
- **Letters:** signed-in players write from the letterbox; the owner reads and answers from the deck anywhere in the
  world, and from the control room. A reply waits for the player's next visit.
- **The settlers page** in the control room: lots by archetype, letters, the steering log (what the builder picked for
  whom), the funnel by scene.

## Mood

- Real events reach players only through the coarse states the public snapshot already carries (plan/17's
  allowlist): weather, shutters.
- **Rumours** are canned lines, chosen by those coarse states. No model ever sees a real event, so none can leak a
  host, a time or a cause.

## The write path (owner decision 7)

Today the public pod holds no credential and no state, and the owner pod is never reached from the internet
(ukubi-game internal/public/poll.go:1-3). Visitor data written by the owner pod changes that, so the path stays
narrow:

- The public pod relays a small typed set of writes: letter, lot, display name, memory, deletion. It relays only for a
  signed-in session, and rate-limits each by the session's subject.
- The writes travel on an internal-only route of the owner pod, with a shared token, closed off by a network policy
  so only the public pod can reach it (an infra PR, stacked). The owner pod gets no public address.
- The owner pod validates every write again, field by field: a compromised public pod can only send what the API
  allows.
- **The visitor store is not the game's event log.** The log is append-only and replayed; personal data in it could
  never be deleted. Visitor data goes into its own store (a gamedb table), keyed by the OIDC subject, deleted for real.
- A ban (plan/18 V7) and an account deletion remove the player's lot, letters and memory.
- The public pod reads back what it shows (lots, replies for a session) on its poll, through the same typed snapshot
  allowlist, extended with the settlers' quarter: display name, archetype, name, pitch, colours. Never an email.

## The blueprint page

A single page linked from the spawn stone, for anyone who won't play: what the world is, the stack, how a building
maps to a real app, and links to the real plans and repos the owner chooses to make public. Written by the owner
with an agent, like the lore.

## Privacy

- New personal data: the display name (chosen, shown publicly on the player's lot), letters, memory, the lot. The
  privacy page (plan/18 V3) lists each, its purpose, and deletion through users@bnei.dev or the admin.
- The display name is opt-in, defaults to none (the lot shows "a settler"), and is never the email.

## Slices

| slice | what | depends on |
|---|---|---|
| S0 | this plan in mountain | none |
| S1 | the write path: the owner pod's internal route and the visitor store, the public pod's relay, deletion and ban cascade, the snapshot's settlers fields; network policy as a stacked infra PR | S0 |
| S2 | stalls: fact-card fields in the Showcase editor, the stallkeeper's prompt, card fallback, guest session cap and the global daily cap, memory | S1 for memory |
| S3 | the builder's yard: the reserved area, the archetype table and piece catalogue, the builder's talk with validation and fallbacks, placing and connecting | S0 |
| S4 | the fork, the challenge (outcome table and animation), sign-in at the fork, joining the disc (persistence, display name, pull-out), the bike moved | S1, S3 |
| S5 | the question: the keeper's note, the contact, the "how this was built" card, the letterbox and the owner's replies from the deck, the settlers page | S1, S4 |
| S6 | mood: canned rumours by coarse state; the spawn stone; the blueprint page | S0 |
| S7 | tests with people: the funnel by scene, timing, then cuts to the core | S2–S6 |
| plan 20 | presence: the keeper and players seen live, the clock-tower lantern | S1 |

The build can run on the local stack while the cluster is down; nothing ships to users before it returns.

## After the second doubt cycle

The second review read this plan against ukubi-game's main. Each finding and its answer:

1. **The lot comes from the browser.** A guest builds before signing in, and sign-in is a redirect, so the build
   must survive in the browser. Answer: the public pod signs the builder's validated output (archetype, name, pitch,
   colours) into a short-lived build token (HMAC, the pod's own key), kept in localStorage across the redirect. The
   lot write carries the token; the owner pod accepts only text the token signed. The challenge's pass is not trusted
   either: the lot is kept whatever the pick (the challenge teaches, it does not gate). Public player text needs a
   moderation answer: open question 7.
2. **Read-back through the broadcast snapshot.** The snapshot listener (:7411) is open to any pod and serves one
   document to everyone (helm/values.yaml:25-32). Answer: only the settlers' quarter (public by design: display name,
   archetype, name, pitch, colours) rides the snapshot. Per-player data (replies, memory) is read by the public pod
   per subject, on demand, from the write listener below, never broadcast.
3. **A route can't be closed off by a NetworkPolicy.** Answer: the writes and per-player reads get their own
   listener on the owner pod (a third port), with its own NetworkPolicy admitting only the public pod, and a shared
   token. The chart's `networkPolicy.ingressFrom` limit (values.yaml:30-32) is fixed in the same infra PR. The open
   funnel POST on :7411 stays as plan/18 V7 left it.
4. **Writes fail when the owner pod is down.** It runs one replica with Recreate, and is down often. Answer: the
   public pod keeps a bounded, in-memory outbox per write kind (the funnel's add-back pattern, public/funnel.go),
   retried on each poll. A restart of the public pod loses it, and the player is told. Scene 6 never waits on the
   write: the lot shows from the build token at once, marked "the courier is on the road" until the owner pod
   confirms it.
5. **Subjects already sit in the event log (bans).** control.banned events hold the sub (game/showcase.go). Answer:
   a ban is kept on purpose, to stop abuse, and the privacy page says so. Deletion removes everything else. New
   events never carry a sub. The owner pod checks bans on every write, so writes don't rely on the public pod's
   fail-open list.
6. **Memory has no read path.** Answer: the per-subject read in 2 carries memory; the owner pod checks its shape and
   length only, since it is the public pod's own model output.
7. **The LLM caps don't match the code.** Answer:
   - Guests get a signed anonymous session cookie (random id, no personal data, listed on the privacy page). The
     ~6-call cap keys on it, and the per-IP limit stays as a backstop, raised for offices.
   - Signed-in players are "uncapped" within reason: a high per-subject daily cap (about 100) under the global one,
     so a few throwaway accounts can't drain it.
   - The caps reuse the existing settings `llm.visitor` and `llm.total` (game/showcase.go:36-37). They reach the
     public pod through the snapshot, which grows the fields. The counter stays in the pod's memory; the provider's
     spend cap is still the hard stop.
8. **The yard isn't outside the layout if reserved like the info lots.** Answer: the yard is a fixed grid the server
   places once, beside the capital, before the lotter runs and outside newLotter (town/village.go). The owner pod
   assigns each new lot the next free slot. A deleted lot's slot stays empty until the next new lot, so no lot ever
   moves. A full yard shows "the yard is full" and the lot lives in the browser only, until the owner widens the
   grid.
9. **The privacy page misses data.** Answer:
   - the steering log joins the list and the deletion cascade;
   - Anthropic is named as the processor of the talk texts, and "nothing is shared with third parties" is
     corrected;
   - deletion by email needs the owner to look up the subject in authentik, and deleting the authentik account is
     part of the cascade (a manual step).
10. **voice can't do JSON or card fallbacks.** Answer: new seams, each a narrow interface its consumer owns:
    - a dresser returning the builder's JSON;
    - a stallkeeper whose fallback is read from the card;
    - an output check (length, palette, archetype membership, and none of the card's "never claim" terms) apart from
      voice.Clean, which stays the input check.
    This plan overrides plan/18 decision 2 ("a live LLM only adds tone") for the stalls and the builder; lore stays
    written by the owner.
11. **The database role can't delete.** Answer: S1 adds the visitor tables with a role that may delete them and
    only them, applied by hand like 0001. The local stack gets a file-backed store behind the same seam.
12. **The time budget skips sign-in, and the funnel steps change.** Answer: sign-in (redirect, consent, reload)
    counts in scene 4's time and is measured in S7. S7 renames the funnel steps by scene in town/funnel.go, the
    privacy page and the cockpit tab together.
13. **The bike stays on one device.** Answer: the visitor store holds the bike too, and the browser's copy migrates
    on first sign-in.
14. **The keeper's lantern broadcasts when the owner is online.** It is a real fact, not mood. Open question 8.

## Not doing (and why)

- **Outage replays** (real incidents replayed in the sim as puzzles): a later engineer track; costly, and friends come
  first.
- **A general quest engine:** one quest serves the thesis; more quests come when tests ask for them.
- **Guest multiplayer:** plan/18's decision stands.
- **Voice chat:** nothing asks for it; moderation cost.

Reversed by plan/21 (2026-11-02): mobs, and opt-in PvP, which this list once dropped.
- **A static fallback when the cluster is down:** owner decision 8.

## Owner answers (2026-11-01, night)

- **Public player text** (lot name, pitch, display name) goes public, after a quick LLM sanity check: a one-word
  verdict from the small model. Until it passes (the model down, the budget gone) the lot shows its archetype's
  default name and "a settler". A rejected text keeps the defaults; the player may try again.
- **The clock-tower lantern stays**, lit while the owner is online.
- Questions 1 to 6 below have defaults, built as data the owner edits in the control room:
  1. a drafted archetype table (see S3), reviewed by the owner later;
  2. drafted words for the spawn stone and the keeper's note, as Showcase fields;
  3. the contact: a Showcase field, empty until the owner fills it (the note hides the line while empty);
  4. stats: none until the owner picks them per stall (the card alone);
  5. Voco'steroid: its shutters come from the existing probe, unchanged;
  6. the blueprint page's links: a Showcase list, starting empty.

## Open questions for the owner

1. The archetype list: which 12 apps can a visitor build (and which of the owner's pieces each needs)?
2. The keeper's note and the spawn stone: the exact words.
3. The contact at the end: LinkedIn, email, or both?
4. Which public stats each stall may cite.
5. Voco'steroid: on the cluster, or off it (where its shutters come from)?
6. The blueprint page: which repos and plans are public?
7. Public player text (lot name, pitch, display name) is shown to every visitor. Either the names and pitches are
   assembled from fixed parts the model only picks (no moderation needed) and the display name waits for the owner's
   approval on the settlers page, or free text goes public with only an automatic check.
8. The clock-tower lantern tells everyone when the owner is online. Keep it, delay it, or drop it?

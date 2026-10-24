# 09 · Playing the town (research, 2026-10-23)

> Status: **research**. The owner interview (what, how) and the architect interview are done, followed by a doubt
> cycle with three reviews. Nothing here is built. Each fleet or infra change below gets its own ADR, interview and
> doubt cycle before any code.

## Why
The town only watches. The owner wants to **play** it like Civilization, with agent-fleet agents doing real work:
improving apps, building features, observing. The binding rule still holds: every image is true and tied to
something real. Game state is real data too (orders, answers, goals), but it is never drawn as cluster health.

## What the owner wants (interview)
- **Core moments, by excitement:**
  1. reacting to events;
  2. growing a strategy;
  3. directing work;
  4. exploring.
- **Waiting for you** (turn notifications):
  - agents blocked on you;
  - trouble in the cluster, each with a ready fix proposal;
  - opportunities.

  Open PRs are not notifications: review and merge follow the merge flow below.
- **Opportunities** come from:
  - the fleet's existing proposals and scheduled audits;
  - drift and hygiene scouts;
  - a goal's next step;
  - patterns in the town's data;
  - alerts turned into opportunities.
- **Strategy:**
  - choose research;
  - progress over time;
  - goals per building: a feature, a quality bar or a free-form intent. On the map a goal shows as a small
    `step/total` mark on top of the building, with the detail on its card.
- **Levels come from goals reached**, not from sync activity. A goal closes only on a checkable signal (a PR merged
  plus a check, or an SLO held for N days). Levels never drop. Until goals exist, Civ ships without levels.
- **Players:**
  - the owner;
  - trusted players, in authentik groups `mountain-players` + `mountain-play-<building>` declared in git (their own
    blueprint file). A player must also be in `mountain-visitors` or `platform-admins`.
- **Watchers** (invited visitors) see:
  - agent units as working or idle per building, never a count and never a prompt;
  - the era and a bucketed score;
  - goal *kinds* from a catalogue. A custom title is shown only after the owner approves it.
- **Tempo:** real time, plus a digest on arrival. It is a Civ-style turn summary when there is news, else a
  persistent to-do queue by urgency. The digest and the last-visit mark live in the game's database, never in
  mountain's events table or its replay.
- **Orders:** a menu of order templates per building, plus free text. How involved the player stays depends on the
  order.
- **Agent conversation:** inside the town, a panel over the map at the building. On the phone, the panel covers
  plans and questions; a diff opens on GitHub. Phone and desktop count equally.
- **Score** is made of calm days, goals reached and knowledge (research, ADRs, investigations). It does not count
  traffic. Latency counts only once a latency histogram is polled.
- **Pressure** is game-only:
  - a neglected building decays (an open item left unanswered, never plain inactivity);
  - the score drains while something waits on you; any answer stops the drain, including a reject or a snooze.
  - It has its own look, never smoke, fading, grey or rubble, and it does not spread to neighbouring buildings.
- **Achievements and eras** come from real data.
- **Research:** an agent interviews the player in the town's panel, then drafts an ADR. The tech tree and its
  unlocks are **parked** for a later session.
- **Exploring:**
  - a building's story;
  - ask a scout (waits on the fleet's headless invoke, agent-fleet ADR-0070);
  - live internals for players;
  - agent memory. The fleet journal is never shown to watchers, and only through a scoped read.

## Architecture (after the doubt cycle)
- **A game service** owns the game state: goals, score, digest, achievements and who-played-what.
  - It is a second Go binary in this repo (`game/`), with its own Deployment.
  - It runs in its **own namespace**, because the fleet's `:9091` fence admits whole namespaces, and has its own
    Pigsty database.
  - It is ClusterIP only. There is no `game.bnei.dev`.
- **One authentik OAuth sign-in, with the token kept server-side.**
  - The relay is already the town's OIDC client. It keeps the player's authentik tokens, so the page only ever
    talks to the relay.
  - The relay calls the game in-cluster with the player's token. The game validates it and checks the player's
    groups per building.
  - The game forwards the token to agent-fleet as a registered client, and the fleet records the real person. This
    needs a fleet ADR: today a client's author is always `client:<name>`.
- **The fleet does the work.**
  - Orders open fleet sessions.
  - Answers and approvals use AgentService's decision calls.
  - The panel is mountain UI fed by the game, relaying the transcript. It copies the fleet island's sanitising
    (DOMPurify, raw HTML off) and its XSS corpus test.
- **Merge flow:** agents work to completion, and a PR is deployment synchronisation:
  1. open the PR;
  2. a code review cycle (review, then fix or response);
  3. a merge synchronised to the right time.

  The timing rules are **parked** for their own interview. The flow needs fleet PR mode, which is unbuilt: in push
  mode an approved push already is the merge.
- **Events:** ukubi-bridge (infra-bootstrap ADR-0060) is the central point.
  - The game publishes "needs you" events.
  - Push is a separate bridge subscriber.
  - Until the bridge lands, an owner-first interim queue reads the fleet's own stream and API, behind one interface
    that the bridge replaces.
- **Push:** a self-hosted **ntfy**.
  - It is public, with its own users, deny-all by default.
  - Payloads stay generic: "an agent needs you" plus a link, never an app, host or prompt.
  - It needs its own infra-bootstrap ADR, because it sits outside the authentik tiers.
- **The editable worker setup per building** is **parked** for its own interview. Today a fleet client opens only
  definitions, each pinned to one repo with a reviewed ceiling (ADR-0062/0065). Live context packs (0063) are the
  closest editable precedent.

## Changes this needs elsewhere (each its own ADR)
**agent-fleet:**
- a forwarded user token: every decision is recorded under the person (amends 0062 §6 and 0066 §3);
- capacity: today `MAX_EXTERNAL_SESSIONS=1` and the definition pool is 2. This means a per-player cap, a scout pool,
  and the 0066 §8 sweep (a session waiting on a human is never swept);
- PR mode and the merge flow (amends ADR-0005 and the "nothing merges itself" row);
- a scoped read of proposals and the journal for a client, where accepting an opportunity opens a definition
  thread;
- a "waiting on a human" event from core.

**infra-bootstrap:**
- an ADR-0060 amendment: app publish/subscribe, trust and `actor` for app events, and no definition subscribes to
  needs-you;
- ntfy;
- the game's registry entry and namespace;
- the client token in its own Infisical project (a recorded exception to ADR-0049).

## Roadmap
These run **in parallel**:
- the bridge, with its amendment;
- the owner-first interim queue, with ntfy push.

Then:
1. orders;
2. goals and levels;
3. score and pressure.

**Civ ships first, without levels.**

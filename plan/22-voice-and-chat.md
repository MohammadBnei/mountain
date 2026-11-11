# 22 · Voice, chat, parties and mail: the realm as the social hub

Status: approved, 2026-11-11. Written from a handoff (session ukubi-2a's intent interview), three doubt cycles and an
architecture interview with the owner. Amends plan/19 ("Not doing": voice chat) and plan/21 (the realm gains social
state: a roster, chat, mail, blocks, timeouts).

## Context
Players in the shared world (`ukubi-game`, `cmd/realm`, about 20 players, single realm replica) need to talk. The intent was confirmed with the owner on 2026-11-11: a handoff from ukubi-2a, then the interview and the reconcile choices made in this session.

**Who is in the world:** friends mixed with strangers. GitHub and Google sign-ins are auto-approved, and everyone is equal from day one. Moderation therefore reacts after the fact, and **every rule holds against a modded client**.

**What is built:**
- **Proximity voice** on self-hosted LiveKit, never recorded.
  - Push-to-talk and open mic, switched by a one-tap HUD chip beside the mic. Push-to-talk is the default.
  - Phone: a mic button near Jump. Desktop: `Z` (`B` is Build).
- **Ad-hoc parties:** invite, accept, up to 8 members. Members hear each other anywhere. Held in realm memory.
- **Text chat** in a bottom log with a show/hide toggle:
  - **World:** 24 h history, caught up on join.
  - **Party:** live only.
- **Mail:** DMs and the old visitor→owner letters merge into **one realm mail**.
- **Moderation:**
  - Mute is client-side; block is server-side.
  - **Report** goes to the owner's inbox.
  - Reports from **any 3 distinct accounts within 1 h** time the player out of voice and chat for 24 h, until the owner reviews it. The owner chose simple: misuse is undone after the fact.
  - The owner's ban kicks a player from voice at once.
- Plan/22 reverses `mountain/plan/19-storyboard.md:226`, which lists voice chat under "Not doing".

**Accepted trade-offs:**
- While the realm is down, there is no social: no chat, no voice, and the keeper shows "post office closed". Ban evasion with new GitHub accounts is open; the owner bans again.
- Players on strict networks with no UDP and no TCP 7881 get no voice.

**Cut or deferred:**
- Cut: head bubbles, the speaking ring, and the Controls-panel voice row (the HUD chip only).
- Deferred: guilds and a guild channel, a Nearby text channel, TURN, vouching, word filters, an LLM check on chat.

## Shape: the realm is the hub; LiveKit only carries audio

### 1. Identity
- **Ticket:** a new claim `name` (`internal/ticket/ticket.go:38`).
  - The owner pod fills it with the session name.
  - The public pod uses `visitors.Record.Display` when `DisplayChecked`, otherwise `Rider <rider id>`, which the realm fills in.
  - Both minters fill it: the owner one (`cmd/game/main.go:172-177`) and the public `Who` (`:426-441`, which returns `(sub, slot, name, err)`).
- **Voice id:** `hex(HMAC-SHA256(UKUBI_REALM_ID_SECRET, sub))[:16]`. Stable across restarts and opaque. It is the LiveKit participant identity.
- **Roster:** table `realm_roster(voice_id PK, sub, name, seen_at)`, upserted on each join. It maps a voice id back to a subject for mail, block and report. It is also the offline mail picker: anyone seen in the last 30 days.
- **Rider ids** restart at 0 when the realm restarts (`realm.go:507`). So `block`, `report` and `party` messages always carry `voice_id`, never a rider id.
- **Owner identity:** one source only, `UKUBI_REALM_OWNER_SUB`. It is used for owner actions and the timeout exemption, whichever pod the owner's ticket came from.
- **The `Who` map:** a new `ServerMsg.who{map<rider, {name, voice_id}>}` covering **every** rider, sent on join and on change. It is separate from `Looks`, which leaves out riders with no look (`internal/realm/look.go:72-80`) and is client-settable.
- **Subjects never leave the realm.** Clients name players by rider id (online) or voice id (offline). The one exception is the owner: report mail carries the subject so that ban works (§6).

### 2. Voice: enforcement by publisher permissions (`internal/talk`)
Self-hosted LiveKit cannot enforce subscriptions from the server. `canSubscribe` blocks server subscribes too, `autoSubscribe` is the client's own choice, `MoveParticipant` is Cloud-only, and tokens can't be revoked. So **each speaker's own client restricts who may subscribe to its track**, and the SFU enforces that.
- **Allowlist:** the realm sends each speaker `ServerMsg.hear{voice_ids[]}`. That client calls `setTrackSubscriptionPermissions(false, [{participantIdentity, allowAll:true}…])`.
  - A modded listener cannot get past it.
  - A modded speaker can only overshare its own voice.
  - Block holds, because the person who blocks controls their own allowlist.
- **Hearing rule:** recomputed on the tick, at most every 500 ms, with only changes sent. A speaker is heard by:
  - riders within 30 m, plus party members;
  - minus blocks, in both directions;
  - minus timed-out riders.
  
  A rider in a private place (`p.left != nil`: building, ship, tunnels; `internal/realm/silence.go:13-21`) is heard by their party only, and hears only their party.
- **Listener side:** `ServerMsg.hear` also carries the listener's own **audible set**, and the client connects with `autoSubscribe:false`. It subscribes only to tracks that are **both** in its audible set **and** allowed by the speaker.
  - This stops a banned, timed-out or blocked modded speaker who keeps rejoining on a refreshed token (each LiveKit token is valid for at least 10 min, `pkg/service/roommanager.go:62`). Honest listeners drop that speaker the moment the realm removes it from their set.
  - Volume is cosmetic (`setVolume`): full within 8 m, falling linearly to 0 at 30 m, and full for party members.
- **Closed by default:** the client calls `setTrackSubscriptionPermissions(false, [])` **before connect and before publish**. The SDK default is allow-all (`LocalParticipant.ts:144`). Check the state after a full reconnect in S1.
- **Hysteresis:** enter at 28 m, leave at 32 m, so no flapping every 500 ms at the edge.
- **Accepted leak:** LiveKit sends speaker and mute updates to every participant (`rtc/room.go:1571`). Any client can see that "voice X is talking now", even inside private places. Content never leaks. This is recorded as a decision; no spike.
- **Token:** sent with `ServerMsg.voice{url, token}` after `join` (`internal/realm/realm.go:471`).
  - HS256, signed with stdlib `crypto/hmac`.
  - Grants: room `world`, `canPublish` with `canPublishSources:[microphone]`, `canSubscribe:true`, **`canPublishData:false`**, `canUpdateOwnMetadata:false`. TTL 10 min.
  - No re-minting: LiveKit refreshes tokens itself.
- **Kick:** on ban, timeout or socket close, the realm runs these steps **against the participant SID it recorded for that socket**, never against the identity. On `REASON_REPLACED` (a second tab or a reconnect, `realm.go:996`) it does not kick: LiveKit already evicts the duplicate identity (`rtc/room.go:2214`). Steps, in order:
  1. `UpdateParticipant` with all permissions off (`canPublish:false`, `canSubscribe:false`). The refreshed token then carries that empty grant.
  2. `RemoveParticipant`.
  3. Every 10 s, compare `ListParticipants` with open sockets. Remove anyone who has no socket or is banned or timed out.
  
  This also covers a realm restart: on start, the first pass removes everyone until they rejoin.
- **Off the lock:** LiveKit calls run in their own goroutine with a queue and backoff, **never under `r.mu`** (the convention at `look.go:51`). If LiveKit is down, voice is down and the realm keeps going.
- **Spikes** (first step of P4):
  - **S1:** confirm that publisher permissions block a modded `setSubscribed(true)`. Confirm that the closed-by-default permission survives a reconnect and a republish. Confirm the SID-based kick leaves a new tab alone. Find the paths the SDK needs (for the IngressRoute and CSP), e.g. `/rtc/validate`.
  - **S2:** Freebox UDP hairpin from the LAN. If it fails, test a second (LAN) candidate. If Pion's 1:1 NAT allows only one, fall back to split-horizon `voice.bnei.lan` with a LAN-only LiveKit `node_ip`. This one is unverified.
- **Settings:** `UKUBI_REALM_LIVEKIT_URL` (internal API), `_PUBLIC_URL` (wss), `_KEY` and `_SECRET`, plus `UKUBI_REALM_ID_SECRET`. All go in `envVars` (`cmd/realm/config.go:30`) and the README table.
  - The dev keys `devkey`/`secret` are refused outside `--dev`, the same guard as the ticket dev keys (`config.go:~88-92`).

### 3. Parties (realm memory only)
- Messages: `ClientMsg.party{invite|accept|leave, rider}` and `ServerMsg.party{id, members[]}`.
- At most 8 members. A party dissolves when its last member leaves, and is lost if the realm restarts.
- Party membership feeds the hearing rule and the Party channel.

### 4. Chat
- Messages: `ClientMsg.say{channel: world|party, text}` (oneof 16) and `ServerMsg.chat{lines[]}` (oneof 12).
- `talk.Line` cleans each line: trim, strip control and format runes but keep `<` and `>`, at most 280 runes. The realm does not import `internal/voice`. Copy `voice.Limit` (about 40 lines) into `talk`.
- Rate limit: 5 lines per 10 s per subject. Lines over the limit are dropped, and the sender sees "slow down".
- **World history:** table `realm_chat` (§8), pruned every hour of rows older than 24 h. Join sends the last 24 h, up to 200 lines. `--dev` keeps it in memory.
- **Party lines:** fanned out to members only, never stored.
- **Block:** lines are filtered per receiver, in both directions.
- **Delivery:** each player has a `chat` channel with a buffer of 64. When the buffer is full, the oldest line is dropped. The socket is never closed for this. `ponytail: 20 players × 5 lines/10 s never fills 64; add gap fetch if it ever does.`

### 5. Mail: one system in the realm
- **Table:** `realm_mail(id, from_sub, to_sub, at, kind dm|letter|report, text, read_at, nonce)`, with a unique key on `(from_sub, to_sub, nonce)`. Nonces are chosen by the client, so two visitors can share one.
- **Messages:** `ClientMsg.mail{send{to_voice_id, text, nonce} | read{id}}` and `ServerMsg.mail{items[], unread}`.
  - The inbox (last 50) is sent on join. New mail is pushed live when the recipient is online.
  - There is no Sent box and no delete; mail ages out under the caps.
- **Caps:**
  - 2000 runes per mail.
  - 20 sends per hour per subject.
  - 200 mails per inbox; the oldest read mail is dropped first. With 200 unread, the inbox refuses new mail.
  - Exempt: the **owner's inbox and `report` mail**. Their cap is per sender (at most 5 unread to the owner per sender).
- **Checks:** block and ban are checked in the realm. Decide under `r.mu`, **write to the database after releasing it**.
- **Recipients:** chosen from the roster (§1): online riders, anyone seen in the last 30 days, and the owner. The keeper becomes "write to the owner".
- **Owner inbox:** the in-world inbox panel. It replaces `GET /api/control/letters` and the reply route (`internal/play/letters.go`). The cockpit row "Letters (n)" (`world.ts:2015`) opens it.
- **Import** (one-time, P3): `cmd/realm --import-letters <file>` reads the visitors JSON. That is the `FileStore` file, or `SELECT body FROM game_visitors` exported by the owner, which is a listed owner step.
  - Each letter and its reply become two `letter` rows, idempotent on `(from_sub, to_sub, nonce)`.
  - The owner subject comes from `UKUBI_REALM_OWNER_SUB` (a new env var; also used to exempt the owner).
  - Run the import **after** the new routes are deployed, so no letter is lost between the export and the switch.
- **Retired:** `/api/visitor/letter`, `/visitors/write/letter` and the keeper's letterbox call the realm. The old visitor JSON fields stay readable for one release.

### 6. Moderation
- **Block:** table `realm_blocks`, set with `ClientMsg.block{rider|voice_id, on}`. It applies at once to chat, mail and the hearing rule.
- **Mute:** client-side, in localStorage `ukubi-muted` keyed by voice id.
- **Report:** `ClientMsg.report{rider, why}`. At most 5 per hour per subject.
  - Creates a `report` mail to the owner with the target's last 20 World and Party lines, **plus the target subject** (owner-only).
  - Auto-mutes the target for the reporter.
  - Reports from 3 distinct subjects within 1 h create a row in `realm_timeouts(sub, until)` for 24 h: chat and mail are refused, and voice gets the kick sequence (§2). **The owner is never timed out.**
- **Owner actions:** `ClientMsg.moderate{report_id, lift|ban}`, accepted only from `OWNER_SUB`.
  - `lift` deletes the timeout.
  - `ban` makes the realm kick that player at once (socket plus voice), with a local ban held until the poll confirms it. The owner's page then POSTs `/api/control/ban {sub}` to the owner pod itself (`internal/play/control.go:185`, `gate.Owner`): it is signed in already, and report mail carries the subject. So no realm→owner route, no token, and the snapshot port stays public.
- **Ban poll:** the realm polls `/api/public/bans` every 30 s. That catches bans made from the cockpit: kick the socket and voice, refuse at join. All of this is new P1 code: the realm has no ban check today.
  - Until the first poll succeeds after a cold start, **all social is closed** (chat, mail, voice), while movement still works. The last list is kept in memory after that.
  - The owner pod being down at a cold start therefore means no social. This is accepted alongside "realm down".
- **Forget:** the owner pod adds forgotten subjects as hashes next to the bans it already serves (`/api/public/bans` gains `forgotten[]`). The realm already polls that list. On a new hash, it deletes the subject's roster, chat, mail (both directions), blocks (both directions) and timeouts. It also scrubs report mail: the quoted lines **and** the target subject.
  - There is no new listener, token, fence or retry loop. A realm that is down catches up on its next poll.
  - The hash lives as long as the ban list does.

### 7. HUD and client
- **Mic:** `data-pad="mic"` in the jump `.side` group (`cmd/game/web/world.html:97`), wired in `bindPad` (`world-input.ts:278`), using `hold()` for push-to-talk.
  - Beside it, a **PTT/OPEN chip** stored in `ukubi-voice-mode`, using the `ukubi-camera` pattern (`world-bindings.ts:301`).
  - Desktop: `ptt` added to `KeyAction`/`KEYS`, default `KeyZ`.
  - The first press asks for mic permission and arms the mic. `room.startAudio()` runs on the first tap on the page (iOS).
- **Player menu:** tapping a rider opens Invite to party, Message, Mute, Block and Report. Remote riders live in `world-realm.ts`.
- **Chat log:** bottom-left, with World and Party chips. It shows the last 5 lines and fades them after 10 s.
  - A deck toggle (`ukubi-chat-shown`) hides it and keeps an unread count while hidden.
  - Enter, or the chat button on phones, opens the input.
- **Inbox panel:** inbox and write. The unread badge sits on the deck. Reports show in a different style, with Lift and Ban buttons for the owner.
- **SDK:** `livekit-client` from npm, which connects when `voice` arrives. Measure its bundle and cold-load cost in P4, and report it in the PR.
- **CSP:** a new game-pod setting `--voice-origin` / `UKUBI_GAME_VOICE_ORIGIN`, on both the owner and public pods, fed to `CSPFor` (`internal/gate/gate.go:247`). Add it to the README table.

### 8. Migrations: one per phase, each an owner step before its deploy
The runtime role gets exactly the grants listed.

| Migration | Phase | Tables | Grants |
|---|---|---|---|
| `0002` | P1 | `realm_roster`, `realm_blocks` | SELECT, INSERT, UPDATE, DELETE (forget) |
| `0003` | P2 | `realm_chat` | SELECT, INSERT, DELETE |
| `0004` | P3 | `realm_mail` | SELECT, INSERT, UPDATE(`read_at`, `text` for the scrub), DELETE |
| `0005` | P5 | `realm_timeouts` | SELECT, INSERT, DELETE |

## Infra (infra-bootstrap: PRs only, the OWNER merges)
- **LiveKit as a registry app:** an entry in `gitops/apps/registry.yaml`, copied into `apps.applicationset.yaml`.
  - Values come from `helm/livekit.yaml` in ukubi-game, the same as `helm/realm.yaml`.
  - common-app-chart with 1 replica and Recreate. The config is rendered by an InfisicalSecret template (pattern: `netbird-config-secret.yaml`).
  - Config: `rtc.udp_port 7882`, `rtc.tcp_port 7881`, `node_ip` per S2. No Redis.
- **Signal:** `voice.bnei.dev` IngressRoute with **`PathPrefix(/rtc)` only**, so the Twirp admin API is not public, plus any path S1 shows the SDK needs. The Cloudflare proxy is on.
- **Media:** an `extraManifests` LoadBalancer on `192.168.1.236`, with `externalTrafficPolicy: Local`, UDP 7882 and TCP 7881.
- **Fences:**
  - `livekit` ingress: Traefik on 7880, the world on 7881/7882, `ukubi-realm` on 7880.
  - `livekit` egress: the world on UDP (ICE checks) and DNS (copy the ADR-0054 STUN precedent).
  - `ukubi-realm` egress: add `livekit:7880`. Nothing else changes.
- **Secrets:** they go in the `ukubi-realm-secret.yaml` template. Hand edits are overwritten within 60 s, and the template refuses to render with a missing key, which blocks the realm. So **the secrets land per phase, before that phase's realm deploy:**
  - **Infra PR A (before P1):** `_ID_SECRET` and `_OWNER_SUB`.
  - **Infra PR B (before P4):** the LiveKit app, the exposure, the `_LIVEKIT_*` keys, and `UKUBI_GAME_VOICE_ORIGIN` in the game values.
- **ADR:** records the new exposure. ICE candidates reveal the WAN IP that ADR-0038's origin lock hides, the same as ADR-0054's STUN.
- **Owner, by hand, per phase:**
  - Infisical keys, then merge that phase's infra PR, then apply its migration (§8), then the realm deploy.
  - P3: export `game_visitors` for the import.
  - P4: Freebox forwards, UDP 7882 and TCP 7881 to `.236`.

## Local dev
- Add `livekit-server --dev` to `dev/Procfile` and `dev/stack-realm.sh`. The realm's `--dev` uses its keys.
- Chat and mail use the in-memory store.
- Blocks and mail now live in the realm, so `stack-realm.sh` exercises them.

## Phases (one PR each; ukubi-game merges on green CI plus a fresh code review)
- **P0:** plan/22 in mountain, plus the ADR draft in infra-bootstrap.
- **P1:** `Who` map, name claim (both minters), voice id, roster, `OWNER_SUB`, ban poll with the `forgotten[]` hashes, socket kick, cold-start social gate, `realm_blocks`, player menu (Mute and Block). Migration 0002, after infra PR A.
- **P2:** chat. Proto, `talk.Line` plus the limit, `realm_chat` with prune, log UI and toggle, block filtering. Migration 0003.
- **P3:** mail. `realm_mail`, inbox panel, owner inbox, keeper rewire, `--import-letters`, forget extended to mail, retire the old letter routes. Migration 0004.
- **P4:** voice. Spikes S1 and S2, token, hearing rule and `hear` allowlist, kick sequence and reconcile, mic, chip, `KeyZ`, volume, CSP setting, LiveKit in the dev stack.
- **P5:** parties and reports. Party proto and menu, Party channel, party hearing, report, timeout, `moderate`. Migration 0005.
- **Infra PRs A and B:** owner-merged. A comes before P1; B runs in parallel with P2–P3 and comes before the P4 deploy. The voice end-to-end check waits for B.
- **P5** forget also covers `realm_timeouts`, so the forget code grows with each phase's tables.

Base on main at `0dc697d` or later: #239 and #244 are merged, and no branches are open in `world-view.ts`, `world-input.ts` or `world.css`. The pad now has Run beside Jump and Go at the left thumb. Before P4, re-check where the mic button fits in the jump `.side` group.

## Verification
- **Go:** full `go test ./...` before any merge. Tests cover:
  - name claim; voice id is stable and opaque; `Who` covers riders who have no look;
  - JWT against a known vector, with grants including `canPublishData:false`; dev keys refused outside `--dev`;
  - hearing rule: distance, party, block both ways, timeout, private place;
  - kick order (`UpdateParticipant`, then `RemoveParticipant`) by SID and reconcile, against a fake Twirp server; REPLACED skips the kick; a down LiveKit does not stall the tick;
  - audible set and allowlist both honour block, ban and timeout; hysteresis at 28/32 m;
  - `talk.Line`, rate drop, chat buffer overflow drops the oldest line, prune;
  - mail caps and owner/report exemptions, nonce idempotency, import round trip;
  - a forgotten hash deletes and scrubs everything, including the report target subject;
  - 3 reports cause a timeout, `OWNER_SUB` is exempt, `moderate ban` kicks at once;
  - ban poll kicks a live socket; all social stays closed before the first poll; roster maps voice id to subject;
  - `wire_test.go` pins the new TS wire.
- **TS:** `bun test` covers the volume curve; permissions closed before connect and publish; applying the `hear` allowlist to `setTrackSubscriptionPermissions`; subscribing only to the audible set intersected with what is allowed; the voice-mode chip, the chat log fade/toggle/unread count, and the player-menu actions.
- **Hands-on:** in the phone emulator (390×844×3 touch) on `dev/stack-realm.sh` plus LiveKit dev, **one Chrome page** (the phone). Player 2 is a **headless** Playwright context, with no extra Chrome page, using `--use-fake-device-for-media-stream`. Check each case:
  1. **Enforcement:** Player 2, modded, calls `setSubscribed(true)` on a speaker 50 m away and gets no audio. A banned modded Player 2 who rejoins on its old token is not heard by the phone. A second tab for the phone's subject keeps its voice.
  2. The phone walks within 30 m of Player 2 and hears them; walks away and loses them.
  3. Player 2 enters a building and goes silent to outsiders; after joining a party, the party hears them.
  4. Push-to-talk on hold, then the open toggle.
  5. World chat survives a reload; party lines are not stored.
  6. Mail to an offline player arrives on next join.
  7. Block hides chat, mail and voice both ways.
  8. 3 reports cause a timeout; the owner lifts it; the owner bans from the report and the player drops from voice at once.
- **Infra:** after the owner merges, `voice.bnei.dev` connects from a phone on mobile data and from the home LAN (S2).

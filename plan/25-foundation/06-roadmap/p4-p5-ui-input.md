# P4 voice and P5 party: what the ui-core bang wires (from ukubi-73, 2026-11-14)

#256 (P4, `67bcefd`) and #257 (P5, `59bac64`) are merged. Both ship the server, the wire and the transport only. The UI
for reference lives on the branches `realm/p4-voice-ui` and `realm/p5-party-ui`.

## Action ids

`cmd/game/websrc/world-social-actions.ts` holds the names only, with no surface yet:
- `voice.ptt`, `voice.openMic`;
- `party.invite`, `party.accept`, `party.leave`;
- `player.report`;
- `owner.lift`, `owner.ban`.

## Wire

| Direction | Field | Message | Notes |
|---|---|---|---|
| ClientMsg | 18 | mail: `MailOp{send{to_voice_id,text,nonce} \| read{id}}` | |
| ClientMsg | 19 | party: `PartyOp{INVITE\|ACCEPT\|LEAVE, voice_id}` | |
| ClientMsg | 20 | report: `Report{voice_id, why}` | |
| ClientMsg | 21 | moderate: `Moderate{report_id, LIFT\|BAN}` | Owner only. A BAN must also be POSTed to `/api/control/ban {sub, ban:true}`, or a realm restart loses it |
| ServerMsg | 15 | mail: `Mail{items, unread}` | |
| ServerMsg | 16 | voice: `Voice{url, token}` | |
| ServerMsg | 17 | hear: `Hear{voice_ids}` | An empty Hear arrives as `{}` |
| ServerMsg | 18 | party: `Party{id, members, invites, refused}` | |
| ServerMsg | 19 | timeout: `Timeout{until}` | ms since the epoch; 0 means none |
| ServerMsg | 20 | reports: `Reports{items[...]}` | Owner only |

## world-voice.ts

- **Shape.** `makeVoice(deps)` returns `on('state'|'speaking'|'peers', cb)`, which returns an unsubscribe function.
- **Commands:** `connect`, `ptt(on)`, `openMic(on)`, `setVolume(id, v)`.
- **Plumbing:**
  - `offer`, `hear`, `remute`, `drop`, `state`, `talking`;
  - `unlock()`: a user gesture;
  - `frame()`: the per-frame distance volume, a fast binding.
- **Its deps:** `room`, `at`, `me`, `muted`, `mate?`, `mount(el)`, `unmount(el)`, `warn`.
- **What it leaves out.** It has no timers and no DOM, and it isn't wired into `world.ts` yet. The LiveKit SDK loads
  lazily as `dist/world-voice-lk.js`.

## Plan

This is a follow-up lane on `feat/ui-core`, after the integration:
- rebase onto the main that has #256 and #257;
- fill S0's placeholder slice with the voice and party store slices;
- register the actions;
- port the reference UI as Preact surfaces: the mic in the pad, the ptt key, the Settings voice tab, the Party tab
  and the radial entries.

GitHub Actions was billing-blocked again on 2026-11-14 (CI has since moved to Forgejo Actions), so nothing has deployed since `914d878`.

# 05-story — Story

| P | Section | Cut | Camera |
|---|---|---|---|
| 0 | Hero | closed | wide |
| 1 | Summit | shallow | ¼ orbit |
| 2 | Water | aquifers | ¼ orbit |
| 3 | Strata | bands | ¼ orbit |
| 4 | Bedrock | deepest | low |
| ≥5 | Live | closes | pull back, orbit/zoom unlocked |

- **Mechanism:** pi-lab `story.js` CatmullRom keyframes; `cut(P)` writes `world.clip`.
- **Agent acceptance:** the mechanism with **placeholder i18n copy keys** (EN + FR). The final Aretē lines and keyframe tuning are **human-gated** (M BN).
- **Panels:** an Aretē line, then mono facts. Facts come from the allowlisted `/api/topology` + metrics; versions only as static copy.
- **Scrub:** the finale timeline bar.
  - While dragging, `step` pauses, physics is skipped, live frames are buffered, and the replay is capped per day.
  - On release, the client re-requests `hello`.
- **Low tier:** same story, half-size pools, frozen `t`, cannon cap halved, no composer.


# 05-story — Story

| P | Section | Cut | Camera |
|---|---|---|---|
| 0 | Hero | closed | wide |
| 1 | Summit | shallow | ¼ orbit |
| 2 | Water | aquifers | ¼ orbit |
| 3 | Strata | bands | ¼ orbit |
| 4 | Bedrock | deepest | low |
| ≥5 | Live | closes | pull back, then the finale sway |

- **Mechanism:** pi-lab `story.js` CatmullRom keyframes; `cut(P)` writes `world.clip`.
- **Input (R5): buttons, not scroll.**
  - Previous/Next buttons (bottom centre, ≥ 44 px targets, `aria-label`s, i18n keys) and the ←/→ keys step P by one section. P animates to the target over ≈ 1.5 s (eased); a press during a transition retargets from the current P.
  - No scroll hijack: the page does not scroll, and the wheel stays the orbit zoom.
  - **The mouse (and touch) always orbits** (OrbitControls, PR #7) around the current section's look point, within the same distance/polar limits. A step, or 45 s idle, glides the camera back onto the keyframe.
  - Previous is disabled at Hero, Next at Live. The section index is in the URL hash (`#live`, …) so a reload or a shared link lands on it; the default is Hero.
  - Reduced motion (`prefers-reduced-motion`): the step jumps, no animation.
- **Agent acceptance:** the mechanism with **placeholder i18n copy keys** (EN + FR). The final Aretē lines and keyframe tuning are **human-gated** (M BN).
- **Panels:** an Aretē line, then mono facts. Facts come from the allowlisted `/api/topology` + metrics; versions only as static copy.
- **Scrub:** the finale timeline bar.
  - While dragging, `step` pauses, physics is skipped, live frames are buffered, and the replay is capped per day.
  - On release, the client re-requests `hello`.
- **Low tier:** same story, half-size pools, frozen `t`, cannon cap halved, no composer.


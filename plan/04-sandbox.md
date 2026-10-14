# 04-sandbox — Sandbox

- `dev/sandbox.html?m=a,b&seed=&script=&snapshot=` is grown from pi-lab `object.html`. It uses the **kit rig** (never `object.html`'s white lights or shadows).
- Controls: P slider, play/step, dt multiplier, `params` sliders, injectors for every `ev`, `world:*` and `mark`, field heatmaps, bus log, tier toggle.
- Hooks: `__setP`, `__inject`, `__gate`.
- `gate.mjs` per model checks:
  - budget + peak
  - `stepMs`
  - writes, emits and listens
  - the clipped flag
  - `pose` determinism on the fixture snapshot
  - `test.js` passes
  - no foreign materials
  - **style (R4, 09):** every mesh material is a kit factory; lit means flatShading; no `map` outside glow/points/fields; colours ∈ kit tokens/ramps; no shadows; median edge length (world space, incl. instance matrices) within the profile's `facet:[min,max]`; luminance ceiling < 1 and dusk floor p50 > 0.004 on the gate's own HalfFloat RT; static source scan; rig/dome unchanged around `step()` (full table: 09 "Gate checks")
  - **the CSP header** (set by the Go relay: importmap hash, `connect-src wss://mountain.bnei.dev`, no third-party origins)
  - no console errors
  - no real↔surface contact
- **Relay mode (R4):** `gate.mjs page` runs against the real relay binary (`relay --dev --source=fixture:<file>`), so the CSP has one source.
- **Hostile fixture (R4):** the fixture timeline carries IPs, namespaces, hostnames, versions and non-allowlisted apps. A Go/websocat sanitisation script asserts none reach `/ws`.
- `perf.mjs` gets an Android throttle profile, reads the `CHROME` env var, and measures the `gl_FragDepth` cost. **Thresholds exit non-zero (R4):** frame time, draws ≤ 220, tris ≤ 200k at peak.


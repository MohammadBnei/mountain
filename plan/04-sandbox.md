# 04-sandbox — Sandbox

- `dev/sandbox.html?m=a,b&seed=&script=&snapshot=` is grown from pi-lab `object.html`.
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
  - **the CSP header** (set by the Go relay: importmap hash, `connect-src wss://mountain.bnei.dev`, no third-party origins)
  - no console errors
  - no real↔surface contact
- `perf.mjs` gets an Android throttle profile and measures the `gl_FragDepth` cost.


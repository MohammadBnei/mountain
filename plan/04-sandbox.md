# 04-sandbox — Sandbox

- `dev/sandbox.html?m=a,b&seed=&script=&snapshot=` is grown from pi-lab `object.html`. It uses the **kit rig** (never `object.html`'s white lights or shadows).
- Controls: P slider, play/step, dt multiplier, `params` sliders, injectors for every `ev`, `world:*` and `mark`, field heatmaps, bus log, tier toggle, **scrub** (W2: `live` connects socket.js to the relay serving the sandbox, a day picker, `scrub`, `release`; the finale timeline bar is W3).
- Hooks: `__setP`, `__inject`, `__gate`, `__world` (dev scripts).
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
- **CI (`app.yml`):** runs `node www/dev/core.test.mjs` (node only). The self-hosted runner has no Chrome and CI does not `sudo apt` one, so `gate.mjs selftest` runs in CI only if a Chrome is already on the runner; otherwise **`node www/dev/gate.mjs selftest` (and `gate.mjs <model>` for touched models) is a required local pre-merge step**.
- **Runner Chrome (checked 2026-10-14, W1b seam):** `ukubi-build` is an **unprivileged Debian 13 LXC** (4 cores, 4 GB, `nesting=1`; infra-bootstrap `terraform/build-runner.tf`). `ansible/playbooks/build-runner-configure.yml` installs only podman, buildah, skopeo, curl, jq, git, sudo, acl, libicu76: **no Chrome/Chromium and no system node**, and nothing in infra-bootstrap docs plans one. So the gate stays a local pre-merge step. Two ways to CI it later, neither done: (a) an infra-bootstrap PR adding `chromium` (+ fonts) to that apt list, Chrome then needs `--no-sandbox` or user namespaces inside the unprivileged LXC; (b) no infra change: run the gate in a podman container on the runner (`podman run … node + chromium`, `CHROME=/usr/bin/chromium`). Both render WebGL2 on SwiftShader (no GPU): the HalfFloat readback and stepMs budgets must be re-checked there before the gate becomes required in CI.
- `perf.mjs` gets an Android throttle profile, reads the `CHROME` env var, and measures the `gl_FragDepth` cost. **Thresholds exit non-zero (R4):** frame time, draws ≤ 220, tris ≤ 200k at peak.


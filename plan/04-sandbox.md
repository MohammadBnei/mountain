# 04-sandbox — Dev loop and gate

**Dev loop:**
- `go run ./relay --dev --source=fixture:www/dev/fixtures/relay-town.json` serves the page on `http://127.0.0.1:8080`
  with a scripted Prometheus/Alertmanager/GitHub timeline.
- `--dev` allows an `http://127.0.0.1:*` Origin and adds `ws://127.0.0.1:*` to the CSP.
- `?q=low|high` forces a tier and `?lang=fr` the French copy.
- `window.__town` holds the dev hooks.

**`gate.mjs page`** (the real relay binary, so the CSP has one source):
- the CSP header: importmap hash, `connect-src wss://mountain.bnei.dev`, no third-party origins;
- every asset under `/v/<build>/`;
- no console error, exception, failed request or request to another origin;
- the hello arrives;
- the budget, ≤ 220 draws and ≤ 200k tris with the shadow pass included, on desktop and on phone-low (coarse
  pointer, low tier);
- labels on both tiers;
- resync: the relay restarts on the same port; the page says "Stale since" while it is down, then resyncs through
  the next hello.

**`gate.mjs selftest`:**
- the page on the hostile fixture (`relay-hostile.json`);
- plus a leak scan of the hello for node IPs and names, `bnei.lan`, the service product names, private apps and
  markup.
- The frame-level sanitising (every frame, not only the hello) is covered by the Go tests below.

**Other checks:**
- **`townshots.mjs`:** the styleboard, with screenshots of every building state and of the town at day, dusk and night
  on desktop and phone. The owner signed it off on 2026-10-22.
- **`core.test.mjs`** (node only):
  - the world fold against the Go fixtures (`frames.json`, `hello.json`);
  - layout hex math against `relay/town`;
  - the sun vectors.
- **`schema/check.mjs`:** every fixture frame against the v2 schemas.

**Hostile fixture:** the timeline carries:
- IPs, namespaces, hostnames, versions, pod names and non-allowlisted apps;
- plant marks on roads, bridge heads and plots;
- forged poses, an unknown toy, text in a pose;
- anonymous and lapsed-role sessions, a hostile handle, rate spam, and oversized and malformed frames.

Go tests (`sanitize_test`, `validators_test`, `plots_test`) and the selftest assert that none of it reaches `/ws`.

**CI (since mountain#50, 2026-10-27):**
- **A PR is tested on GitHub-hosted runners, whatever state the cluster is in.** `app.yml`'s `test` (promtool, go vet
  and test, the schema check, `core.test.mjs`, and `gate.mjs selftest` with the image's Chrome) and `db-test`
  (Postgres 17 in a service container) both run on `ubuntu-latest`. A PR goes green or red on its own, and is
  merged on that.
- **Only `build-push` needs ukubi** (the `build-runner` LXC: buildah and the LAN registry). It runs on a push to
  main only, so while the cluster is down it sits queued after the merge, and the image is built and deployed when
  the runner comes back (plan/11). A queued `build-push` on main never blocks a PR.
- `gate.mjs page` is still a local step. Locally as root, Chrome needs `CHROME=<wrapper adding --no-sandbox>`.

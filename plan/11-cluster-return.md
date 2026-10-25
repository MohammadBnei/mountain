# 11 · When the cluster is back (owner checklist, written 2026-10-24)

The cluster has been offline since 2026-10-23 (internet outage at home, back around 2026-10-26). Code merged while it was
down has never run. This file is the whole list of what to do when it returns, in order, with what each step should
show. Steps marked **you** are yours (the cluster, Infisical, pigsty, authentik); the rest are mine (I write the PRs and
check them) once you tell me the cluster is up.

This file covers both sessions' work: mine (mountain/atlas, the bridge's I-steps and B1b) and the other session's
("Fleet and infra assessment": the agent-fleet and ukubi-bridge releases), which it read and confirmed on 2026-10-24.
"Me" in step 3 means that session; everywhere else it means this one.

**The order at a glance** (hard constraints in bold; everything else may run in parallel):
1. Platform up (1), then free the build box (3.0).
2. mountain build (3, after 4 if you can), ADR-0067 restart (2), atlas checks (5): independent of the rest.
3. **Pigsty role and DB → 3a (bridge deploy) → I2 → B1b**.
4. **3b (agent-fleet 6.25.0) → B1b** (core 6.24.0 has no bridge client, so restarting it means nothing before 3b), and
   3b → I1 if one firing alert must give one proposal from the first alert (the dedup ships in 6.25.0).
5. I3 last, then 3e after 3b.

## 1. The platform is up (you)
- [ ] The 5 k8s nodes are `Ready`: `kubectl get nodes`.
- [ ] ArgoCD: the `bootstrap` Application and every app are `Synced` and `Healthy`, or catching up.
- [ ] Pigsty: Patroni has a leader (`pg-proxmox`), `postgres.bnei.lan:5432` answers.
- [ ] authentik answers at https://authentik.bnei.dev.
- [ ] The `build-runner` LXC is up and its runner instances are online (GitHub → each repo → Settings → Actions →
      Runners: `Idle`). `build-runner-ops` skill if one is offline.

## 2. The gate narrows to platform-admins (infra-bootstrap #373, ADR-0067)
ArgoCD applies the two changed blueprint ConfigMaps on its own; authentik does not reload them by itself
(`docs/runbook-authentik-identity.md` §9.1).
- [ ] **you:** check the files are mounted in the worker, then restart it:
      `ssh k9s kubectl exec -n authentik deploy/platform-authentik-worker -- find /blueprints/mounted -maxdepth 2 -name 'mountain*'`
      then `ssh k9s kubectl rollout restart deploy/platform-authentik-worker -n authentik`.
- [ ] **you:** the bindings: `psql -d authentik -tAc 'SELECT target_id, group_id, enabled FROM authentik_policies_policybinding'`.
      The `mountain-visitors` rows on `mountain` and `mountain-gate` are gone; the `platform-admins` rows remain.
- [ ] Later, your call: remove the inert invitation flow and the `mountain-visitors` group (ADR-0067 rollout step 3).

## 3. Re-run the builds that queued while the runner was down
GitHub cancels a job that waited more than 24 h for a runner, so these runs are almost certainly cancelled: start them
again by hand (Actions → the workflow → Run workflow, branch `main`).
- [ ] **mountain** `app` on `main` (the latest main: A1, A2 and A3 at `2b5eaf9`, plus whatever merged since). It tests, builds, pushes the image and bumps
      `helm/values.yaml`'s tag, and ArgoCD rolls atlas out. One run is enough. Do step 4 first if you can.

From the other session (it runs 3a, 3b and 3e; you click Run where it can't):
- [ ] **3.0 (you, before any re-run): free the build box's disk.** agent-fleet's `build-push` failed once on "no space
      left" before the outage. On `root@192.168.1.111`: `buildah rmi --prune`, then clear `/var/tmp/buildah-cache-0`
      (the `build-runner-ops` skill). Check that both runner instances, agent-fleet and ukubi-bridge, show `Idle`.
- [ ] **3a. ukubi-bridge, deploying #8, #9 and #10.** After step 6's Pigsty role and DB if `bridge` is really missing;
      **before I2 and B1b** (B1b pins the cert into `BRIDGE_OIDC_CERT`, which only #9's code reads).
      - Run: re-run `app.yml` on main (run 40102366773, for `914bd61`) or Run workflow on `main`. It tests, builds,
        pushes, then `deploy` bumps `helm/values.yaml` and ArgoCD rolls it out.
      - Check:
        - the pod is Ready;
        - one log line `BRIDGE_CLIENTS is ignored` (expected until I3);
        - the API refuses every call (expected until B1b);
        - GitHub webhook deliveries still get 2xx;
        - the hourly retention job logs a run.
- [ ] **3b. agent-fleet, releasing #321 and everything since 6.24.0.** Any time after 3.0; it does not depend on the
      bridge (core logs one Info line, "bridge client unconfigured", until I2 and B1b).
      - Run: re-run main's `docker.yml` (run 40105751702, for `32f31fe`). release-it commits the version and tag
        (expect `6.25.0`), `build-push` builds all six images, `deploy` commits "pin images for X", ArgoCD runs core's
        PreSync migrate Job, rolls out core, and the provisioner follows from main.
      - Check:
        - the migrate Job is Succeeded and `schema_migrations` reads 22, not dirty (000019–000022; 000022 drops
          `scheduled_audits`);
        - core is Ready and the console loads;
        - warming one repo session reaches a PR (the golden path);
        - the dispatcher is idle (`bridge.yaml` lists no definition).
      - Rollback: migrate down to 21 recreates `scheduled_audits` empty; nothing reads it.
      - Live checks after 3b (you look or warm, the other session reads the logs with you):
        - a. The Grafana panels "Sessions by live state" and "Warm headroom" render with no `pending` series (#321
          renamed them).
        - b. Sessions whose pods died during the outage are reconciled within about 60 s: `SELECT id, pod_phase FROM
          sessions WHERE archived_at IS NULL` (through core's DB access) shows none in a live phase without a pod.
        - c. The golden path above proves more than core: the provisioner's 5-min owner-vault refresh re-minting the 1 h
          App token (every pre-outage token has expired), Agent Vault back on its longhorn SQLite, and the sidecar's
          egress router. **A 401/403 on push: look at the provisioner's vault refresh first.**
        - d. Optional: warm one definition session (`opus-workspace`) to check the vault proxy and the definition
          pods' `CiliumNetworkPolicy` egress.
- [ ] **3c. hq #2**: nothing to re-run (docs only, no `app/**`). Check that no hq run is left queued.
- [ ] **3d. infra-bootstrap #365**: nothing to re-run (docs, plus one YAML comment ArgoCD applies as a no-op).
- [ ] **3e. After 3b** (the other session, a small PR): bump infra-bootstrap's `agent-fleet` submodule to the new tag.
- [ ] **You, any time:** revoke `UKUBI_BOT_GH_PAT` at GitHub **before 2026-10-28**, then delete it from Infisical.
      Optional: the "now" rulesets (no delete or force-push on main, immutable agent-fleet tags, no numeric branches);
      they never block CI.

## 4. atlas's repo token (you, before or right after the mountain build)
- [ ] Infisical project `mountain-8c-cz`, key `MOUNTAIN_GITHUB_TOKEN`: give the token **Contents: read** and
      **Metadata: read** on `agent-fleet`, `ukubi-bridge` and `pi-lab` too (it already covers infra-bootstrap, mountain
      and the app repos). Same token, edited in GitHub's fine-grained token settings; the value does not change.
- [ ] If you replaced the token instead: update the key, then restart the pod (env is read at start):
      `kubectl -n mountain rollout restart deploy/mountain`.

## 5. atlas is live (you check, I read the logs with you)
- [ ] ArgoCD `mountain`: `Synced`, `Healthy`; the image tag in `helm/values.yaml` is main's latest sha (`2b5eaf9…` or later).
- [ ] The log: `db: applied migration 001_atlas.sql` once, then `projector: projection 1: 5 repos, … nodes`. A line
      `projector: <repo>@main: not found (no access)` means step 4 is not done for that repo.
- [ ] Anonymous: `curl -sI https://mountain.bnei.dev/` answers 302 to authentik.
- [ ] Signed in as a platform admin: the map draws; the status line says `projection N, … ago`; the land lens shows
      percentages; Trade routes shows solid (measured) routes into the apps Traefik serves; a card opens on a click.
- [ ] Metrics (Prometheus): `mountain_source_last_success` for `prom`, `alerts`, `github`, `projection`, `db` all
      recent; `mountain_drift{kind="unassigned"}` is 0. MountainUnassigned firing = a component realms.yaml does not
      place: tell me, it is a one-line PR.
- [ ] Rollback, if needed: revert #29, then `DROP TABLE atlas_migrations, atlas_drift, atlas_edge_ver, atlas_node_ver,
      atlas_vector, atlas_projection` in `mountaindb` before the relay image starts (`10-understanding.md` "A2 as built").

## 6. The event bridge, finished (the plan of 2026-10-23; code merged, never run)
- [ ] **you:** (before 3a) the bridge's Pigsty role and database (`docs/secrets.md`: "not yet created"), from the hub
      (`docs/runbook-k9s-ops-hub.md`): `./pgsql-user.yml -l pg-proxmox -e username=dbuser_bridge`, then
      `./pgsql-db.yml -l pg-proxmox -e dbname=bridge`. The bridge pod then goes `Ready`.
- [ ] **I1** (me, a PR; after 3b for one proposal per alert from the first): alerts also go to the bridge. The token rides the existing `alertmanager-thot-token` Secret (a
      new Secret's missing mount would block Alertmanager and Grafana from starting); a second `webhook_configs` entry
      in receiver `thot`, `send_resolved: true`; Grafana posts to `/webhook/grafana`. Check: one firing alert gives one
      proposal, whichever path lands first.
- [ ] **you:** check no `bridge-*` user exists in authentik, then create `BRIDGE_CORE_APP_PASSWORD` in Infisical.
- [ ] **I2** (me, a PR; after 3a): the authentik provider (`client_id: ukubi-bridge-api`), the `bridge-core` service account,
      the reserved game/push accounts (no token), the reserved name prefixes in every enrollment flow, username
      changes off. Checked first against the pinned authentik image locally, then a real token decoded live.
- [ ] ~~M1~~ (the relay's handle check): gone with the relay; atlas has no handles. Nothing to do.
- [ ] **B1b** (me, a PR; **after 3a, 3b and I2**): pin the live signing cert in the bridge, then restart core (6.25.0,
      which reads the `bridge-core` Secret I2 delivers). ADR-0066 §1 gets its one-line
      amendment (the pinned cert is the trust anchor).
- [ ] **I3** (me, a PR): remove the `BRIDGE_CLIENTS` template line, then **you** delete `BRIDGE_CLIENTS` and
      `BRIDGE_CORE_TOKEN` from Infisical.
- [ ] Not verifiable before: a live token's `iss`; Alertmanager reaching the bridge; core's grant; one PR event opening
      a session end to end. I check each with you as its step lands.

## 6b. Start the recorder (you; R11, `12-world-and-game.md`)
- [ ] Once `ukubi-game` exists and its recorder is merged: run `record` where the cluster's Prometheus and
      Alertmanager are reachable (the k9s hub, or a dev pod in `mountain-test` with `scripts/devpod.sh`'s pattern).
      It is read-only (range queries, `/api/v2/alerts`). Recording is forward only, so each day it is not running is
      lost history. Also run `record capture` once, for the contract shape tests.

## 7. Still open from earlier rounds (only if not done before the outage)
- [ ] ADR-0065's live checks (R8 deploy C1), if you never ran them: the header-forged `curl` to
      `/outpost.goauthentik.io/auth/traefik` redirects with a `redirect_uri` on mountain.bnei.dev.
- [ ] B0 (ukubi-bridge's release workflow) no longer waits on the release App, deferred to Track 4 (owner,
      2026-10-24): main pushes stay on `GITHUB_TOKEN` in push-to-main jobs, built the way agent-fleet's `docker.yml`
      is. Code, not a cluster step.

## 8. Owner, not tied to the cluster (any time)
- [ ] **Before 2026-10-28:** revoke `UKUBI_BOT_GH_PAT` at GitHub, then delete it from Infisical (also in step 3).
- [ ] Create the public `agent-kit` repo and install the Claude, `ukubi-fleet` and `ukubi-bridge` Apps on it: that
      starts the other session's Track 2.
- [ ] agent-fleet and ukubi-bridge, Settings → Actions: "Workflow permissions" read-only, and "Allow GitHub Actions to
      create and approve pull requests" off. The workflows declare their own permissions; this covers a future job
      without a block.
- [ ] infra-bootstrap `protect-main`: switch the admin bypass to pull-request-only, or correct the VISION line. #365
      merged on the admin bypass with zero reviews.
- [ ] Optional: the "now" rulesets (no delete or force-push on main, immutable agent-fleet tags, no numeric branches).

## After all that
A4 (Hubble metrics, the ServiceMonitor, the Alloy traces Deployment, OTEL in atlas and the bridge) needs the cluster
for every step, and A5 (the `ukubi-atlas` App, `bridge-atlas`) needs you to create the App and its Infisical project.
I prepare both PRs during the outage where code allows; their rollout steps land in this file.

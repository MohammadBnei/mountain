> **Landed (R8):** infra-bootstrap ADR-0064 (`docs/adr/0064-mountain-town-roles-and-guest-probes.md`, PR #366) folds
> this draft in, together with R8's machines and guest probes. ADR-0064 is canonical; this draft is kept for history.

# Draft: amendment to infra-bootstrap ADR-0056 (public telemetry)

For M BN to land in infra-bootstrap. It records a decision already made for the town (R7, 2026-10-22): platform
services become public, under generic role names.

## Context
ADR-0056 makes mountain's telemetry public by allowlist only: allowlisted apps, host buckets h1..h3, bucketed values,
PromQL only. Platform namespaces were "known, never public". The town rewrite (R7) draws every app **and platform
service** as a building on the island of the host its pods run on, and the owner chose to show services in full.

## Decision (amendment)
- These platform namespaces are public, each under a **role key** (never the product name) in `relay/config.yaml`:

  | Namespace | Public key |
  |---|---|
  | traefik | gate |
  | authentik | guardhouse |
  | argocd | craneyard |
  | monitoring | watchtower |
  | longhorn-system | warehouse |
  | infisical | vault |
  | garage | granary |
  | zot | depot |

- They are treated like apps: ArgoCD sync and health (when the Argo Application name matches the namespace), pod
  churn, bucketed replicas and Traefik traffic, storage pools, and the host bucket (h1..h3) their pods run on.
- Which host an app or service runs on is now public, as a bucket only. Pod names, node names, IPs and exact pod
  counts never leave the relay.
- Alerts may carry `host` (from the `node` label) and `app` (from the `namespace` label) when they map.
- Storage pools say what kind of storage they are, never the product: class `replicated` (the replicated block store)
  or `local` (node-local paths). Since R7.1 the always-null `garage` and `pigsty` keys are gone from the metrics frame.
- Two names of the mechanism stay public, on purpose:
  - the event kinds `argo.sync` and `argo.health`: they name how deploys are observed, not a service of the cluster;
  - the sign-up link to the identity provider (`authentik.bnei.dev/if/flow/mountain-enrollment/`): a visitor is sent
    there to sign in anyway.
- Still private: agent-fleet, hq, has-journal, wird-adminweb, pgweb, searxng, thot, ente, proxmox, actions-runner,
  kube-system and every namespace not listed.

## Consequences
- An observer learns the cluster's service roles, their health and rough load, and their host bucket. The product
  names of the services stay off the frames (pools included); guessing them from the roles is accepted. The two
  mechanism names above are the exception.
- The tests hold it:
  - `relay/src_prom_test.go` replays the hostile fixture and checks every frame against a leak list: traefik,
    authentik, argocd, longhorn, infisical, garage, zot, pigsty, prometheus, grafana, plus node names, IPs, private
    apps and namespaces.
  - `relay/plots_test.go` runs the same check on plot frames.
  - `www/dev/gate.mjs selftest` scans every `/ws` frame of a 20 s fixture run.

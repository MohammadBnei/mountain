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
- Still private: agent-fleet, hq, has-journal, wird-adminweb, pgweb, searxng, thot, ente, proxmox, actions-runner,
  kube-system and every namespace not listed.

## Consequences
- An observer learns the cluster's service roles, their health and rough load, and their host bucket. The product
  names stay off the wire; guessing them from the roles is accepted.
- `relay/plots_test.go` asserts that no product name, pod name, node name or IP reaches a frame.

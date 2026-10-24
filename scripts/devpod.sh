#!/usr/bin/env bash
# Dev loop against the real cluster, without an image build or the registry.
# The atlas binary (www embedded) is cross-compiled and copied into a plain
# alpine pod in a throwaway namespace; it reads the real Prometheus and
# Alertmanager. Reached only through kubectl port-forward (no ingress), so
# --dev origins (http://127.0.0.1:*) are the only ones that matter. It listens
# on the pod's loopback with sign-in unset, the one way --dev admits everyone
# (port-forward connects there; nothing else in the cluster can).
#
#   scripts/devpod.sh up       create ns + pod, push binary, port-forward 18080/19090
#   scripts/devpod.sh push     rebuild + hot-swap the binary (atlas restarts in ~1 s)
#   scripts/devpod.sh logs     follow atlas logs
#   scripts/devpod.sh down     delete the namespace
#
#   ARGS='--prom-url=http://invalid' scripts/devpod.sh push     extra atlas flags (reset on every push/up)
#   GRAPH=www/dev/fixtures/graph.json scripts/devpod.sh push   copied to /app/graph.json, adds --graph-fixture
#   (no GitHub token in the pod: without GRAPH the page waits for a projection that never comes)
#   (a pod from before atlas (named relay): `down` + `up` once)
#
# ponytail: one pod, no Service/Deployment; the real chart is tested by the
# ArgoCD deploy, this only shortens the code -> live-data loop.
set -euo pipefail
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/ukubi-admin.conf}"
NS="${NS:-mountain-test}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${TMPDIR:-/tmp}/mountain-atlas-linux-amd64"
K="kubectl -n $NS"

build() { (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BIN" ./cmd/atlas); }

push() {
  build
  $K cp "$BIN" atlas:/app/atlas.new -c atlas
  args="${ARGS:-}"
  if [ -n "${GRAPH:-}" ]; then $K cp "$GRAPH" atlas:/app/graph.json -c atlas; args="$args --graph-fixture /app/graph.json"; fi
  printf '%s\n' "$args" | $K exec -i atlas -c atlas -- sh -c 'cat > /app/args'
  $K exec atlas -c atlas -- sh -c 'chmod +x /app/atlas.new && mv /app/atlas.new /app/atlas && { p=$(pidof atlas) && kill $p || true; }'
  echo "pushed $(shasum -a 256 "$BIN" | cut -c1-12) args: ${args:-(none)}"
}

up() {
  kubectl create ns "$NS" --dry-run=client -o yaml | kubectl label --local -f - purpose=dev -o yaml | kubectl apply -f -
  $K apply -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata: { name: atlas, labels: { app: mountain-atlas-dev } }
spec:
  securityContext: { runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532, fsGroup: 65532 }
  containers:
    - name: atlas
      image: docker.io/library/alpine:3.20
      # Supervisor loop: pkill atlas => restart with the new binary. Loopback only: see the header.
      command: [sh, -c, 'while true; do [ -x /app/atlas ] && /app/atlas --dev --listen 127.0.0.1:8080 --metrics 127.0.0.1:9090 $(cat /app/args 2>/dev/null); sleep 1; done']
      env: [{ name: ATLAS_CACHE, value: /app/cache }]
      ports: [{ containerPort: 8080 }, { containerPort: 9090 }]
      resources: { requests: { cpu: 20m, memory: 64Mi }, limits: { memory: 512Mi } }
      securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: [ALL] } }
      volumeMounts: [{ name: app, mountPath: /app }]
  volumes: [{ name: app, emptyDir: {} }]
EOF
  $K wait --for=condition=Ready pod/atlas --timeout=120s
  push
  echo "port-forward: http://127.0.0.1:18080  metrics http://127.0.0.1:19090/metrics"
  while :; do $K port-forward pod/atlas 18080:8080 19090:9090 || sleep 1; done # push restarts atlas: re-forward
}

case "${1:-}" in
  up) up ;;
  push) push ;;
  logs) $K logs -f atlas -c atlas ;;
  down) kubectl delete ns "$NS" --wait=false ;;
  *) sed -n 2,19p "$0"; exit 2 ;;
esac

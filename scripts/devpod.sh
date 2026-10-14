#!/usr/bin/env bash
# Dev loop against the real cluster, without an image build or the registry.
# The relay binary (www embedded) is cross-compiled and copied into a plain
# alpine pod in a throwaway namespace; it reads the real Prometheus and
# Alertmanager. Reached only through kubectl port-forward (no ingress), so
# --dev origins (http://127.0.0.1:*) are the only ones that matter.
#
#   scripts/devpod.sh up       create ns + pod, push binary, port-forward 18080/19090
#   scripts/devpod.sh push     rebuild + hot-swap the binary (relay restarts in ~1 s)
#   scripts/devpod.sh logs     follow relay logs
#   scripts/devpod.sh down     delete the namespace
#
#   ARGS='--prom-url=http://invalid' scripts/devpod.sh push     extra relay flags (reset on every push/up)
#   CONFIG=path/config.yaml scripts/devpod.sh push              copied to /app/config.yaml, adds --config
#   (a pod from before ARGS / MOUNTAIN_FP_SALT support lacks them: `down` + `up` once)
#
# ponytail: one pod, no Service/Deployment; the real chart is tested by the
# ArgoCD deploy, this only shortens the code -> live-data loop.
set -euo pipefail
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/ukubi-admin.conf}"
NS="${NS:-mountain-test}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${TMPDIR:-/tmp}/mountain-relay-linux-amd64"
K="kubectl -n $NS"

build() { (cd "$ROOT" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BIN" ./relay); }

push() {
  build
  $K cp "$BIN" relay:/app/relay.new -c relay
  args="${ARGS:-}"
  if [ -n "${CONFIG:-}" ]; then $K cp "$CONFIG" relay:/app/config.yaml -c relay; args="$args --config /app/config.yaml"; fi
  printf '%s\n' "$args" | $K exec -i relay -c relay -- sh -c 'cat > /app/args'
  $K exec relay -c relay -- sh -c 'chmod +x /app/relay.new && mv /app/relay.new /app/relay && { p=$(pidof relay) && kill $p || true; }'
  echo "pushed $(shasum -a 256 "$BIN" | cut -c1-12) args: ${args:-(none)}"
}

up() {
  kubectl create ns "$NS" --dry-run=client -o yaml | kubectl label --local -f - purpose=dev -o yaml | kubectl apply -f -
  $K apply -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata: { name: relay, labels: { app: mountain-relay-dev } }
spec:
  securityContext: { runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532, fsGroup: 65532 }
  containers:
    - name: relay
      image: docker.io/library/alpine:3.20
      # Supervisor loop: pkill relay => restart with the new binary.
      command: [sh, -c, 'while true; do [ -x /app/relay ] && /app/relay --dev --listen :8080 --metrics :9090 $(cat /app/args 2>/dev/null); sleep 1; done']
      # Dev-only salt (reached only via port-forward): without one src_alerts drops every alert.
      env: [{ name: MOUNTAIN_FP_SALT, value: dev-pod-salt }]
      ports: [{ containerPort: 8080 }, { containerPort: 9090 }]
      resources: { requests: { cpu: 20m, memory: 32Mi }, limits: { memory: 256Mi } }
      securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: [ALL] } }
      volumeMounts: [{ name: app, mountPath: /app }]
  volumes: [{ name: app, emptyDir: {} }]
EOF
  $K wait --for=condition=Ready pod/relay --timeout=120s
  push
  echo "port-forward: http://127.0.0.1:18080  metrics http://127.0.0.1:19090/metrics"
  while :; do $K port-forward pod/relay 18080:8080 19090:9090 || sleep 1; done # push restarts the relay: re-forward
}

case "${1:-}" in
  up) up ;;
  push) push ;;
  logs) $K logs -f relay -c relay ;;
  down) kubectl delete ns "$NS" --wait=false ;;
  *) sed -n 2,17p "$0"; exit 2 ;;
esac

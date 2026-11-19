# Spike: ui-data's bet and chunk (2026-11-19)

Measured for ui-data.md. Read-only on the cluster, nothing committed. Subject `building:repo:agent-fleet`.

## Telemetry query cost

Through an ssh tunnel and `kubectl port-forward` to Prometheus (:9090), Loki (:3100) and Tempo (:3200), client-side
timings, 20 runs each (p95 is the second-worst run). In-cluster latency is lower; atlas's own path is still blocked by
the network policy, so it is untested.

| Face query | p50 | p95 | Bytes |
|---|---|---|---|
| Metrics, 4 range series, 15 min | 55 ms | 77 ms | 11.4 KB |
| Metrics, 4 range series, 1 h | 52 ms | 56 ms | 10.8 KB |
| Metrics, 4 range series, 24 h | 73 ms | 92 ms | 10.9 KB |
| Logs, last 200 lines, 1 h | 24 ms | 28 ms | 68 KB |
| Logs, 200 lines, `detected_level="info"` | 53 ms | 825 ms (one outlier) | 68 KB |
| Logs, 200 lines, `\|= "dashboard"` | 39 ms | 47 ms | 69 KB |
| Tempo, last 20 traces | 30 ms | 43 ms | 13.4 KB |
| Tempo, one trace | 34 ms | 59 ms | 917 B |

Selectors: Traefik series `exported_service=~"agent-fleet-agent-fleet-core-.*"`, pods from
`kube_pod_status_phase{namespace="agent-fleet"}`; Loki `{namespace="agent-fleet",service_name="agent-fleet-core"}`;
Tempo `resource.service.name="agent-fleet-core"`.

atlas's frame today (`cmd/atlas/live.go`): one poll every 15 s (`liveEvery`), 12 contract queries plus the signals due;
the 5 day-window signals every 300 s. No read-duration metric is exported. Replayed: a full 25-query poll p50 517 ms,
p95 581 ms.

Under load (5 parallel loops of full face reads for 120 s, about 75 requests/s, 0 errors), the 25-query poll went to
p50 547 ms, p95 806 ms: inside the 15 s tick and the 1.5 s per-source budget. One viewer reads one face at a time,
far below this.

Headroom: atlas (deployment `mountain`) uses 2m CPU and 48 Mi; Prometheus 47-61m CPU and 1.49 GB of its 2 Gi limit
(74 %, the tightest spot); Loki 15m, Tempo 9m, both without limits; worker-02 at 15 % CPU.

## Owner-only chunk

bun 1.4.4, `@connectrpc/connect` and `connect-web` 2.3.0, `@bufbuild/protobuf` and `protoc-gen-es` 2.17.0, buf 1.75.0.

| Chunk | Minified | gzip | Unminified |
|---|---|---|---|
| Telemetry client | 125,967 B | 34.6 KB | 245,069 B |
| Fleet `AgentService` client | 141,790 B | 39.9 KB | 261,107 B |
| Both | 143,747 B | 40.6 KB | 263,348 B |

The runtime is about 110 KB minified; fleet's generated code about 16 KB, telemetry's about 2 KB. world.js rebuilt
byte-identical (2,475,643 B, no connect or protobuf code in it), 34 KB under budget: the chunk loads by dynamic import
only.

## Verdict

The bet holds on cost. Left to check in production: atlas's in-cluster path once the policy opens, through the
read-duration metric atlas gains with the service.

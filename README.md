# mountain

mountain holds two things:

- **atlas**, the cluster understanding surface served at mountain.bnei.dev (admins only): the system graph projected
  from the repos, a live overlay from Prometheus and Alertmanager, and the page that draws both. It is one Go service
  (`cmd/atlas`) with the front end embedded from `www/`. Design: `plan/10-understanding.md`.
- **The master plans** in `plan/`: the design of the world, the game and the change loop, for every ukubi repo. Start
  at `plan/README.md`. How a change is made is `plan/16-dev-rules.md`.

It also exports what other repos read: `contract/` and `realms/` (the wire contracts and the realm definitions), and
`api/` (atlas's protos, generated with buf).

## Run atlas locally

Needs Go (see `go.mod`) and no cluster:

```sh
go run ./cmd/atlas --dev --listen 127.0.0.1:8080 \
  --graph-fixture www/dev/fixtures/graph.json \
  --live-fixture www/dev/fixtures/live.json
```

It serves the page on `http://127.0.0.1:8080`. `--dev` admits everyone only when `--listen` names a loopback address (the default `:8080` binds every interface, so the page refuses everyone) and allows
`http://127.0.0.1:*` origins; the fixtures are `--dev` only. Without fixtures, atlas reads a checkout directory with
`--repos <dir>`, or polls the forge when `MOUNTAIN_FORGE_TOKEN` is set. The flags and environment are listed in the doc
comment of `cmd/atlas/main.go`.

The owner's feed (`/feed`) is off unless `FEED_ENABLED=true`. Locally, add `--feed-fixture www/dev/fixtures/feed.json` to
see it without a bridge; `node www/dev/feed-gate.mjs` checks the page in Chrome and takes its screenshots.

## Checks

```sh
go vet ./... && go test ./...      # root tests run promtool when on PATH (CI requires it); DB tests need MOUNTAIN_TEST_DSN
node www/dev/core.test.mjs         # front-end core test
buf lint && buf generate           # api/ must match its generation
```

CI runs these (plus `biome ci .` and golangci-lint) as Forgejo Actions on the Talos runners; the workflow files are in
`.github/workflows/`.

## Layout

| Path | What |
|---|---|
| `cmd/atlas/` | The service: serve, `project`, `resolve`, `state` |
| `internal/` | `extract` (repo extractors), `graph`, `bridgev1` |
| `www/` | The page (`js/`, `models/`, `vendor/`) and its dev tools (`dev/`) |
| `contract/`, `realms/`, `api/` | What other repos read |
| `helm/` | Chart values and alert-rule tests |
| `plan/` | The master plans |

## Plans

| Area | Files |
|---|---|
| Decisions and rules | `plan/00-decisions.md`, `plan/15-architecture.md` (code standard), `plan/16-dev-rules.md` (change loop) |
| atlas | `plan/10-understanding.md`, `plan/11-cluster-return.md` |
| World and game | `plan/12`, `14`, `17`, `18`, `19`, `21`, `22`, `23`, `24`, `plan/25-foundation/`, `29` |
| Retired town (history) | `plan/01`-`04`, `06`-`08` |
| Parked and superseded | `plan/09` (research, parked), `plan/13` (superseded game town) |
| Stubs | `plan/20`, `plan/26`-`28` |

The full list with a status per file is `plan/README.md`.

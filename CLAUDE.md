# mountain

atlas (the cluster understanding surface, `cmd/atlas`) and the master plans (`plan/`). See `README.md` for the layout
and the run commands. Workspace rules load from the root `CLAUDE.md`.

## Facts

- Go (version in `go.mod`), a static binary with `www/` embedded (`embed.go`). The image is built by
  `.github/workflows/app.yml` from reviewed main.
- Tests: `go vet ./... && go test ./...`, `node www/dev/core.test.mjs`, `buf lint && buf generate` (the committed `api/`
  must match). Root tests run `promtool` when it is on PATH (CI requires it); `TestDB*` need `MOUNTAIN_TEST_DSN`. Biome is pinned in CI.
- The repo, namespace and image stay `mountain` (atlas is the code name): a rename breaks the CI identity's bound claims
  and the runner registration.
- mountain is public. `contract/` and `realms/` are read by ukubi-game as a Go module (proxy.golang.org, no secret).
- No trailers, no session links, no co-author in any commit or PR text: `trailers.yml` scans every commit message and
  the PR's title and body, so write "claude links" and "model names" in words when describing the rule.
- Plans cite code by path, so renaming a package or file under `cmd/` or `internal/` means grepping `plan/`.
- plan/16 is the change loop. The plan index is `plan/README.md`.

# 01-architecture — Architecture

## Repo
```
mountain/
  go.mod embed.go       root package `mountain`, `//go:embed www`, imported by relay
  FROZEN.sha256 freeze_test.go   sha256 of every frozen file; CI fails on an unannounced change
  Dockerfile helm/values.yaml .github/workflows/app.yml   CI pins :sha in helm/values.yaml
  relay/
    main.go hub.go ws.go config.yaml sanitize.go sources.go metrics.go
    town/               the town geometry (islands, tiles, bridges, Slot): the only implementation, sent in hello.town
    plots.go            the placer: pods → plots (world:plot), ruins, plantAt
    src_prom.go src_alerts.go src_github.go fixture.go
    store.go scrub.go migrations/ rollback/
    validators.go toy_plant.go sun.go   visitor marks: RegisterToy, rate limits, kill switch; the plant toy and its sun
    oidc.go
  www/
    index.html style.css apps.json fonts/ models/ (KayKit GLBs + clay.png, CC0) vendor/ (three r186.1, OrbitControls,
                        GLTFLoader, BufferGeometryUtils, SkeletonUtils; manifest.json)
    js/main.js          renderer, lights, sun → sky, camera (overview + fly-in), cards, legend, stale, quality ladder
    js/world.js layout.js kit.js   FROZEN: the state fold + bus, hex math + building kinds, the clay kit
    js/socket.js sun.js labels.js toys.js
    js/objects/{islands,bridges,buildings,people,marks}.js
    dev/gate.mjs cdp.mjs townshots.mjs core.test.mjs schema/*.json (FROZEN) fixtures/
  plan/
```

## World (`world.js`, FROZEN)
`world.js` mirrors the relay's state fold (`relay/sources.go State.apply`), so a reload, a resync and a replay show
the same town.

| Slot | Writer | Source |
|---|---|---|
| `world.town` | world.js | `hello.town` (static geometry from `relay/town`) |
| `world.state` | world.js | `hello.state` (`alerts`, `health`, `replicas`, `builds`, `marks`, `plots`), then every frame, **before** its emit |
| `world.metrics` | world.js | the last metrics frame (02) |
| `world.ready` | world.js | host → bool (`ev:node.ready`, `metrics.hosts`) |
| `world.quorum` | world.js | `[hosts]` or null (`ev:quorum`) |
| `world.stale` | world.js | null or unix ms (the oldest stale source, or the socket drop) |
| `world.replay` | world.js | null or `{day, left}` while a scrub replay is held (06) |

**Bus:**
- `world.on(type, fn)` returns an `off` function.
- Types:
  - `hello`;
  - `metrics`;
  - `ev:<kind>` (02 enum);
  - `world:plot` and `world:mark`;
  - `state` (emitted after any change).
- Calls: `world.place(mark, pose)`, `world.unmark(id)`, `world.scrub(day)`, `world.release()`, `world.step(dt)`.

## Objects (`js/objects/*.js`, see 03)
```js
// build({THREE, kit, world, layout, ...}) → { root, update(dt, t) → busy, on?, pick?(ray) → {kind, key, d}, dispose }
```
- An object's steady look is recomputed from `world.state` alone, so a reload and a replay agree.
- Transient acts (a drop, a hop) are tweens started only by live frames.
- One InstancedMesh per GLB kind keeps the draw count flat however many apps there are.

## Kit (`kit.js`, FROZEN)
- pi-lab's clay kit:
  - KayKit GLBs loaded once and recelled onto the clay atlas (`models/clay.png`, 8×4 cells);
  - `kit.instanced`, `kit.splitByUV`;
  - `kit.variant(name, overrides)`;
  - `kit.glowing()`: emissive × vColor, so a roof glows in its own wire colour at night.
- Every mesh is tagged `userData.kit`, and the gate rejects any other material.
- The quality tier is read from `globalThis.__quality`:
  - `low` is a coarse pointer with ≤ 4 GB or ≤ 4 cores, or `?q=low`;
  - on low the shadow pass is off and contact blobs stand in.

## Style
The **Ukubi design system** (00):
- Colour tokens in `style.css`: Office by day, Underground·Glow at night.
- Roofs are tinted per system wire, read from `apps.json` `wire`.
- Self-hosted Readex Pro and JetBrains Mono (`www/fonts/`, OFL).
- One LED, lit while alerts fire.
- Status is always said in words, with 1.5 px line icons.

## Budgets
- **≤ 220 draws and ≤ 200k tris** at peak, shadow pass included, on desktop and on phone-low. `gate.mjs page`
  enforces it.
- At sign-off, desktop drew 103 draws / 99k tris and phone-low 53 draws.
- Low tier: no antialias, a pixel ratio of at most 1.25 (2 on high), no shadow pass.

## Wire
- JSON Schema v2 for `hello`, `ev`, `metrics`, `world`, `replay`, `scrub` and `mark` lives in `www/dev/schema/*.json`
  and is frozen. The Go and JS tests both validate frames against it.
- The hello fixture (`www/dev/fixtures/hello.json`) comes from `relay dump-hello --events …`, never by hand.

## CI (`.github/workflows/app.yml`)
| Item | Rule |
|---|---|
| Triggers | `push` to main on `relay/**`, `www/**`, `embed.go`, `go.mod`, `go.sum`, `Dockerfile`, `app.yml`; `workflow_dispatch`; PRs to main run the tests only (build-push and deploy are guarded) |
| Excluded paths | `helm/**`, `plan/**`, so the tag-bump commit cannot retrigger CI |
| Checks | `test`: promtool, go vet + test, the schema check on a Go-generated hello, the core test and the gate selftest if Chrome is present; `db-test` (Postgres 17) |
| Tag bump | CI writes the image `:sha` into `helm/values.yaml` and pushes with `GITHUB_TOKEN` |
| Gate | `gate.mjs page` and `selftest` need Chrome, which the self-hosted runner lacks, so they are a **required local pre-merge step** (04) |

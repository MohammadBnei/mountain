# 03-model-summit

Layer: **real**. Contract, world, bus and budgets: see `01-architecture.md`. Ship `models/summit/{index.js, fixture.json, test.js}` and pass `gate.mjs summit`.

## Wave
W3

## Rules
One crystal per allowlisted app at `world.sites` (**y from sites only**). Height = replicas bucket, glow = health, hue = group

## Technique
kit `shard(seed)`, **one InstancedMesh**, kit `crystal()` material (`instanceColor` = `ramp('group')` body tint; `aHealth` vec3 ∈ `ramp('health')` × `aGlow` scalar drives emissive), `instanceId` picking. Shader owned by the orchestrator

## Clip / cost
not clipped / 2 draws, 7k

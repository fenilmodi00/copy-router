# cmd — AGENTS

> **Mirror notice.** Verbatim sync with [CLAUDE.md](CLAUDE.md). **Update both together** — divergence = bug.

Composition root. Only place that constructs concrete adapters + wires them together. Read [root CLAUDE.md](../CLAUDE.md) first.

## Rules

- **`cmd/router/main.go` is the only file that imports concrete `internal/providers/*` adapters and `internal/postgres`.** No other place wires things.
- Keep `main.go` focused on wiring. Today's helpers:
  - `buildClusterScorer` — per-version Scorer assembly + embedder warmup
  - `buildExploringRouter` — optionally wraps the cluster router in `banditexplore` (env-flag gated; off by default)
  - `buildSemanticCache` — response-cache assembly
  - `buildOtelEmitter` — OTel span exporter
  - `runSessionPinSweep` — TTL sweep loop
  - `resolveHardPinModel` / `resolveDefaultBaselineModel` / `resolveAvailableModels` — boot-time model resolution
  - `registerDeploymentKeyedProvider` — shared "resolve key → build client → log" registration; this deploy uses it for **aiand only** (`AIAND_API_KEY` / `AIAND_API_URL`)
  - `buildAiandCatalogHandler` — dashboard live ai& catalog (`GET /admin/v1/aiand/models`)
  - small env parsers (e.g. `envVarHint`, `parseEnvInt`, `parseEnvFloat`, `parseEnvDurationMs`)
- **No more heuristic-fallback router.** If cluster routing fails to boot, `main.go` panics. Misconfiguration must abort the process rather than silently degrade.
- **Never introduce DI container, reflection-based wiring, or service locator.** Composition = plain Go function calls.
- **`panic` is reserved for startup-time fail-fast** (`config.MustGet`, cluster-scorer boot failure, invalid `ROUTER_DEPLOYMENT_MODE`). Never panic on request path.

## Deployment-mode dispatch

`ROUTER_DEPLOYMENT_MODE` is read at boot (`selfhosted` | `selfserve` | `managed`; any other value → panic):

- `selfhosted` (default): mounts dashboard at `/ui/*`, `/admin/v1/*`, operator-password cookie auth. `AIAND_API_KEY` is the deploy baseline.
- `selfserve`: mounts dashboard + `/account/v1/*` aiand-key login (`KeyVerifier`). Per-user BYOK bills Models/Playground; deploy `AIAND_API_KEY` optional.
- `managed`: dashboard + `/admin/v1/*` not mounted. aiand still registered; BYOK-only when billing is unwired.

Provider registration (this fork):

- Composition root registers **`providers.ProviderAiand` only** into `providerMap` / `envKeyedProviders`.
- `envKeyedProviders` tracks whether the deploy-level `AIAND_API_KEY` is set so hard-pin resolution knows what is safe to pin to.
- Other `Provider*` constants stay in `provider.go` for wire-family / BYOK / tests — they are not boot-wired here.

Single source of truth for provider→env-var mapping = `providers.APIKeyEnvVars` in [`../internal/providers/provider.go`](../internal/providers/provider.go). Admin `/config` view reads it so it can't drift from actual wiring.

# cmd — CLAUDE

> **Mirror notice.** Verbatim sync with [AGENTS.md](AGENTS.md). **Update both together** — divergence = bug.

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
  - `buildAiandCatalogHandler` — dashboard live ai& catalog (`GET /v1/aiand/models`)
  - small env parsers (e.g. `envVarHint`, `parseEnvInt`, `parseEnvFloat`, `parseEnvDurationMs`)
- **No more heuristic-fallback router.** If cluster routing fails to boot, `main.go` panics. Misconfiguration must abort the process rather than silently degrade.
- **Never introduce DI container, reflection-based wiring, or service locator.** Composition = plain Go function calls.
- **`panic` is reserved for startup-time fail-fast** (`config.MustGet`, cluster-scorer boot failure). Never panic on request path.

## Hosted mode (single mode)

The router runs one "hosted" mode: the dashboard is mounted at `/ui/*` and its
data plane at `/v1/*`; the account routes (`POST /account/v1/login`,
`POST /account/v1/logout`, `GET /account/v1/me`) do aiand-key login. There is
no operator password and no separate admin API.

Provider registration (this fork):

- Composition root registers **`providers.ProviderAiand` only** into `providerMap` / `envKeyedProviders`.
- `envKeyedProviders` tracks whether the deploy-level `AIAND_API_KEY` is set so hard-pin resolution knows what is safe to pin to.
- Other `Provider*` constants stay in `provider.go` for wire-family / BYOK / tests — they are not boot-wired here.

Single source of truth for provider→env-var mapping = `providers.APIKeyEnvVars` in [`../internal/providers/provider.go`](../internal/providers/provider.go). The `/config` view reads it so it can't drift from actual wiring.

# internal/providers — AGENTS

> **Mirror notice.** Verbatim sync with [CLAUDE.md](CLAUDE.md). **Update both together** — divergence = bug.

Provider `Client` interface + canonical `Provider*` name constants + the OpenAI-compat adapter used for aiand. Read [root CLAUDE.md](../../CLAUDE.md) first.

## Layout

- `provider.go` — `Client` interface, `Provider*` constants, `APIKeyEnvVars` map (single source of truth for provider→env-var mapping).
- `openaicompat/` — concrete adapter for OpenAI Chat Completions upstreams (aiand is the deployment-wired one).
- `httputil/` — shared transport + streaming helpers.

Former native `anthropic/`, `openai/`, and `google/` adapter packages are gone. Claude Code still speaks Anthropic wire on `/v1/messages`; `internal/translate` converts to OpenAI-compat before `openaicompat` dispatches to aiand.

## Inward-pointing import (intentional)

Provider adapters (`internal/providers/<name>/`) import `internal/proxy` for the `OnUpstreamMeta` callback so streaming responses record usage/headers back to proxy. This is one of the few inward-pointing adapter→inner-ring imports and is intentional.

## Adding a new `providers.Client` adapter

1. **Prefer `openaicompat`.** For OpenAI-compatible upstreams, pass a base URL into `openaicompat.NewClient` / `NewClientWithModelIDMap`. Only add a new adapter package when the wire format is not Chat Completions.
2. **Implement `Proxy` and `Passthrough`.** Build the client with `httputil.NewClient(httputil.NewTransport(...))` so redirects are refused and the managed-mode destination policy applies, and use `httputil.StreamBody`. Call `proxy.OnUpstreamMeta` when usage/headers are observed. Do not leak provider-specific types across the package boundary.
3. **Add compile-time check:** `var _ providers.Client = (*Client)(nil)`.
4. **Add a canonical name constant** to [`provider.go`](provider.go) (the `Provider*` block) + register the matching env-var name in `APIKeyEnvVars` **and** a `ProviderFamilies` entry (see the "THREE-map edit" comment above the `Provider*` block in `provider.go`). Deployment today wires `"aiand"` only. Other `Provider*` names remain for wire-family dispatch, BYOK/custom bindings, and tests. Skipping the `ProviderFamilies` entry silently 502s at request time — `families_test.go`'s `TestEveryProviderHasFamilyAndEnvVar` catches map drift for constants that appear in `ProviderFamilies`.
5. **Check the non-family-based dispatch switches.** Most cross-format dispatch in `internal/proxy` keys off `providers.FamilyFor`. Two switches still key off literal `Provider*` constants:
   - [`internal/translate/emit_openai.go`](../translate/emit_openai.go)'s `applySessionAffinity` — prompt-cache stickiness per provider. Unlisted OpenAI-compat providers fall through to the generic `x-session-affinity` header (correct for aiand).
   - [`internal/router/rl/mapping.go`](../router/rl/mapping.go)'s `rosterIDFor` — RL roster slug mapping (training keep; unlisted providers fall through to bare model ID).
6. **Wire in `../../cmd/router/main.go`.** Only place that imports the provider package directly. Today only `ProviderAiand` is registered into `providerMap` / `envKeyedProviders`.

## Aiand-only routing

Composition root registers aiand alone. Cluster scorer filters `model_registry.json` to entries whose `provider` is in `availableProviders`, so a deploy with only aiand cannot emit a deleted vendor.

- `openaicompat.AiandBaseURL` (`https://api.aiand.com/v1`) is the default; override with `AIAND_API_URL`.
- Catalog bindings for v0.76 resolve to `ProviderAiand`.
- Anthropic and OpenAI HTTP ingress stay mounted so Claude Code and OpenAI-shaped clients reach aiand through translate.

## What is load-bearing

- **The training script is the only writer of `rankings.json`.** Hand-editing breaks the cluster geometry guarantee. Re-run training after touching `model_registry.json` + commit the regenerated artifact.
- **Cluster scorer is availability-aware at boot, not request time.** Filter happens in `NewScorer`; empty filtered set = hard boot error.

## What to NOT do

- **Don't bypass the provider filter.** If you need another upstream, register it — don't special-case around `availableProviders`.
- **Don't reintroduce native Anthropic/OpenAI/Google adapter packages** for the aiand-only deploy. Translate + openaicompat covers Claude Code → aiand.
- **Don't add per-installation provider preference yet.** Deploy-time env + cluster argmax cover v1.
- **Don't leak provider-specific types** across the package boundary.

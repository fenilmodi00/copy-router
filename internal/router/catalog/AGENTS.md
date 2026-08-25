# internal/router/catalog — AGENTS

> **Mirror notice.** Verbatim sync with [CLAUDE.md](CLAUDE.md). **Update both together** — divergence = bug.

Single source of truth for per-model data: capability tier, ordered list of provider bindings, per-binding pricing + upstream model ID. Read [root CLAUDE.md](../../../CLAUDE.md) first.

## What's here

- `Model` — one struct per logical model. Fields: `ID`, `Tier`, `ContextWindow`, `ReasoningEfforts`, `Providers []ProviderBinding`.
- `ProviderBinding` — one `(Provider, UpstreamID, Price)` tuple. A model's bindings are ordered: the first whose `Provider` name is in the deploy's available set wins.
- `Pricing` — per-binding input / output / cache-read pricing.
- `Tier` — Low / Mid / High (capability bucket for routing, distinct from effort).
- `ReasoningEfforts` — ordered ai& `reasoning_effort` menu for the model (`none` / `low` / `high` / `max`, plus `medium` on a few OpenAI-compat rows). Empty = no effort parameter. Looked up via `CapabilitiesFor` / `ReasoningEffortsFor`.
- `ContextWindow` — model's total input+output token budget in tokens. 0 falls back to `DefaultContextWindow` (128K).
- Lookup helpers: `ByID`, `ResolveBinding`, `PriceFor(provider, id)`, `PrimaryPriceFor(id)`, `TierFor`, `IsAtOrBelow`, `AllowedAtOrBelow`, `AllPrimaryPricing`, `ValidateDeployed`, `ContextWindowFor`, `CapabilitiesFor`, `ReasoningEffortsFor`.
- Cost math: `EffectiveInputCost`, `EffectiveOutputCost` — the OTel emitter, telemetry write path, and billing debit hook all funnel through these.

## Adding a model

1. Append one `Model{}` struct literal to `Models` in `catalog.go`.
2. If the model is a routing target, list it in the cluster bundle's `model_registry.json` (this catalog says how to price/dispatch a model; the registry says which version routes to it).
3. Run `go run ./cmd/genprices` to regenerate `install/install.sh` + `install/cc-statusline.sh`.

That's it. Nothing else needs editing — the planner, scorer, OTel emitter, install scripts, and provider modelIDMaps all flow from this file.

## aiand-only bindings

This deploy's catalog keeps only models with an `ProviderAiand` binding. `ResolveBinding` / `ValidateDeployed` accept catalog IDs or binding `UpstreamID`s so v0.76 registry fields (`deepseek-ai/...`, `zai-org/...`) resolve without renaming rows. Provider adapter packages for other vendors may still exist until later strip PRs; they must not appear in `Models`.

The cluster scorer resolves each routable model's binding at boot via `ResolveBinding(id, availableProviders)`. The chosen binding's `Provider` becomes the `router.Decision.Provider`; the planner then uses `catalog.PriceFor(provider, id)` so STAY-vs-SWITCH EV math is correct.

## Invariants

- **No I/O.** Pure data + accessors. Adding HTTP, DB, or FS calls = layering violation.
- **`Models` is the only writer.** `ByID` / `PrimaryPriceFor` / `TierFor` are read-only views over it.
- **Every binding's `Provider` is one of the `providers.Provider*` constants.** Tested in `catalog_test.go`.
- **Every binding has positive input + output prices.** Tested.
- **No duplicate `Model.ID`s.** Tested.
- **`Providers` is never empty.** Tested.

## What to NOT do

- **Don't read pricing from a parallel table.** The OTel emitter, planner, billing hook, and install-script generator all funnel through this package. A second price table guarantees drift.
- **Don't add a runtime mutation API.** The catalog is compile-time data; per-deploy filtering happens through `ResolveBinding(id, available)`, not by mutating `Models`.
- **Don't duplicate ReasoningEfforts in `internal/router/model.go`.** ai& menus live on `Model.ReasoningEfforts`; `CapabilitiesFor` builds the CapReasoning `ModelSpec`. Keep Anthropic/OpenAI/Gemini adaptive specs in `model.go`'s registry.

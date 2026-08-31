# Router documentation

Index of Markdown documentation in the `router/` repo.

This tree is an **ai& (aiand)-exclusive** deploy: composition root registers
`providers.ProviderAiand` only; catalog bindings and `AIAND_*` env are the
upstream path. See root [README](../README.md) and [AGENTS.md](../AGENTS.md).

| Doc | What it covers |
|---|---|
| [SEMANTICS.md](SEMANTICS.md) | **Canonical terminology** for sessions, rounds, turns, actions, and steps. Read this first before other docs. |
| [CONFIGURATION.md](CONFIGURATION.md) | Environment variables (`AIAND_*` first), model-string resolution, peripheral gateway/BYOK appendix, OTel, cluster artifacts, selfserve. |
| [adding-glm-5-3.md](adding-glm-5-3.md) | How the six-model ai& roster was cut in (catalog + cluster overlay recipe). |
| [HOST_WSL_SUPABASE.md](HOST_WSL_SUPABASE.md) | Host `make setup` / `make dev` against Supabase session pooler (no Compose Postgres; `PUBSUB_DISABLED=true`). |
| [ANALYTICS_EXPORT.md](ANALYTICS_EXPORT.md) | `/v1/analytics/*` raw routing-decision export: read-only keys, cursor paging, row grain, field reference. |
| [SMOKE.md](SMOKE.md) | Record/replay smoke suite against aiand. |
| [POLICY_ROUTER_HARNESS.md](POLICY_ROUTER_HARNESS.md) | Contract for out-of-process policy sidecars. |
| [HMM_GO_SELECTION.md](HMM_GO_SELECTION.md) | Architecture, `policy_router_v3` split, and rollback story for Go-owned HMM roster ownership and deterministic arm selection. |
| [TRANSLATION_COMPATIBILITY.md](TRANSLATION_COMPATIBILITY.md) | Cross-format translation requirements and rollout modes. |
| [aiand-api-reference.md](aiand-api-reference.md) | ai& API notes used by this deploy. |
| [aiand-live-catalog.md](aiand-live-catalog.md) | Live catalog endpoint / dashboard wiring. |
| [aiand-dashboard-features.md](aiand-dashboard-features.md) | Dashboard feature notes for the ai& fork. |

For engineering conventions (layer model, package layout, recipes), see the
root [`CLAUDE.md`](../CLAUDE.md) (and its mirror [`AGENTS.md`](../AGENTS.md)).
Glossary: [`CONTEXT.md`](../CONTEXT.md).

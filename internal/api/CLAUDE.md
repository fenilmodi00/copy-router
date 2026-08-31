# internal/api — CLAUDE

> **Mirror notice.** Verbatim sync with [AGENTS.md](AGENTS.md). **Update both together** — divergence = bug.

Presentation layer. Handlers adapt HTTP ↔ Service. Read [root CLAUDE.md](../../CLAUDE.md) first.

## Subpackages

- `admin/` — operational endpoints: `/health`, `/validate`, `/admin/v1/*`
- `anthropic/` — Anthropic Messages surface (`/v1/messages`, passthrough, `/v1/route`)
- `openai/` — OpenAI surfaces: `/v1/chat/completions` (lead) and `/v1/responses` (OpenAI Responses ingress, translated internally to chat — the surface Codex CLI requires), plus `/v1/models` passthrough helper.
- `analytics/` — read-only routing-decision export (`/v1/analytics/routing-decisions`, `/models`, `/schema`). Authed by `ra_` analytics keys via `middleware.WithAnalyticsKey` **only** — no `WithAuth`, no balance check, no spend cap, since nothing here can route or spend.
- `feedback/` — `/router-feedback` in-chat command surface (rating + note persist to `router.router_feedback`, short-circuits routing, emits `router.feedback.command` span). The HTTP `/f/<token>` pages were **removed** — `ROUTER_FEEDBACK_*` env vars are ignored and must not be re-wired; the [`internal/feedback`](../feedback) signer stays for token mint/verify on remaining internal paths. Do not add auth-free HTTP routes here on the old page's behalf.

## Import rules

- May import `internal/auth` (Service handle + middleware-context types) and `internal/proxy` (routing/dispatch service handle).
- May import `internal/observability` for logging, `internal/providers` for shared sentinel errors, `internal/router/cluster` for `ErrClusterUnavailable` sentinel + `DeployedModelsSource` interface, `internal/analytics` for the export Service handle + row/schema types.
- **Must not import** `internal/postgres`, any concrete `internal/providers/*` adapter, or `internal/translate` directly.
- Concrete instances reach presentation only via constructor params from composition root.

## Adding an HTTP endpoint

1. **Decide timeout budget.** Cheap auth-only ops use `validateTimeout` / `healthTimeout` (1 s). Provider calls get own constant in [`../server/server.go`](../server/server.go) — pick budget + justify in comment.
2. **Decide auth.** Routes needing valid `rk_` bearer go through `middleware.WithAuth(authSvc)`. Admin endpoints use `WithAdminOrAuth` (admin cookie OR bearer) or `WithAdminOnly` (admin cookie only). Unauthed routes (e.g. `/health`) attach no auth middleware.
3. **Decide if dashboard data-plane surface.** The dashboard data-plane routes (`/admin/v1/*` metrics, keys, provider-keys, config, excluded-models, content-capture) mount from a single shared route table in [`../../internal/server/dashboard_routes.go`](../server/dashboard_routes.go) for both `selfhosted` and `selfserve` modes; `managed` skips them. New dashboard data-plane endpoints are added as a row in `dashboardRoutes` there, not as a new route inside a mode block. The login surfaces (`/admin/v1/auth/*` operator password in selfhosted, `/account/v1/*` aiand-key in selfserve) stay in `Register` — they are genuinely mode-specific. Product-surface endpoints (`/v1/*`, `/health`, `/validate`) stay outside the table so they're available in `managed` mode too. **Do not** add a new `/admin/v1/*` route inside a `Register` mode block — add it as a table row, or it will be missed by the other dashboard mode.
4. **Pick (or create) the right subpackage.** Operational → `admin/`; Anthropic Messages → `anthropic/`; OpenAI → `openai/`; feedback-command surface → `feedback/`. New surfaces get their own subpackage.
5. **Use `observability.FromGin(c)` for request-scoped logger.** For authed installation: `middleware.InstallationFrom(c)` (nil if `WithAuth` not applied — handler should be on authed group). For BYOK secrets: there's no gin-context accessor — `WithAuth` stashes them on the *request* context via `context.WithValue(ctx, proxy.ExternalAPIKeysContextKey{}, externalKeys)` (see [`../server/middleware/auth.go`](../server/middleware/auth.go)). Handlers don't read this directly; they forward `c.Request.Context()` into `*proxy.Service` calls, which pull the keys back out internally via `ctx.Value(proxy.ExternalAPIKeysContextKey{})`.
6. **Pick the right service.** Identity-only ops → `*auth.Service`. Routing/dispatch/translate → `*proxy.Service`. Don't touch repositories, router, providers, planner/handover/cache packages from a handler. Handler adapts HTTP ↔ service; service does the work.
7. **Test with in-memory fakes + gin testing harness** (`httptest.NewRequest`/`ResponseRecorder`). No real DB for handler tests — use fakes from [`../auth/service_test.go`](../auth/service_test.go) and [`../proxy/service_test.go`](../proxy/service_test.go) as model.

## History

`internal/router/heuristic` and `internal/router/evalswitch` previously lived in the API ring; both removed when heuristic fallback retired in favor of `cluster.ErrClusterUnavailable` → HTTP 503.

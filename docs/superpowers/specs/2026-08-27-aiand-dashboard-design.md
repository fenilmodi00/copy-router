# ai& Exclusive Dashboard — Spec

> Triage label: `ready-for-agent`
> Approach locked: **Lean Aggregator Console** (Approach A)
> Scope locked: aiand-exclusive — no other upstream providers. Features are a UI surface over ai&'s API + the router's existing metrics.

---

## Problem Statement

The router's existing `/dashboard` page shows only one side of the operational truth: what the router *decided* (chosen model, savings vs the requested model, latency, total tokens). It says nothing about *what's offered*.

The router's only upstream — ai& (aiand.com) — exposes a lot more ground truth that nobody in the router console has a window into:

- A live model catalog (per-organization) with pricing, capability flags (reasoning, tool_calling, vision, video, document), context windows, reasoning-effort menus, prompt-cache rate, and currency.
- Aggregate analytics: per-period request counts, input/output tokens, cost, error counts, average latency.
- Per-model breakdowns (which models the user's traffic actually lands on).
- Per-request usage data including cache creation vs cache read token counts.
- Rate-limit headers (`X-RateLimit-Limit/Remaining/Reset`, `X-RateLimit-Policy`, `Retry-After`).
- Org tier (`tier_0` / `tier_1`) and currency.
- API key management (list / create / rotate / delete) with 8-char prefix + 4-char suffix.

The result, from the dashboard user's perspective: when they're trying to decide which model is the best fit, they have to bounce between ai&'s marketing page, the ai& docs, and the router's existing dashboard. They can't compare models side-by-side, can't see which models are actually consuming the most spend on their install, can't pick models from a unified view grounded in their real usage, and can't see their prompt-cache economics. The dashboard shows the decisions but not the offerings — and it can't show a single ranked view across all the underlying providers (DeepSeek, OpenAI, Google, Qwen, Moonshot/Kimi, Zhipu/GLM, Motif) the way an aggregator-style console should.

## Solution

Re-tilt the dashboard's center of gravity from "router cost & savings" toward "ai& catalog surface." Three things change:

1. Replace the existing savings-charts grid with an Overview that retains the cost-saving story but re-anchors it: four sparkline KPI cards (Tokens / Requests / Actual cost / Cache hit rate), a cross-provider popularity leaderboard, and a "Top models by spend" table.
2. Add a new Models section with three routes: a sortable/filterable catalog explorer, a per-model detail page with the per-model charts that today sit on `/dashboard`, and an up-to-four side-by-side comparison view.
3. Wire the catalog + prompt-cache-into-rollups so the dashboard renders literally what ai& knows, plus what the router decided. The cross-provider popularity leaderboard — ranked tokens-processed across DeepSeek/OpenAI/Google/Qwen/Kimi/GLM — is the only view ai&'s own console can't offer. That's the differentiator.

The existing `/settings/*` pages (Router keys, Provider keys, Models-and-routing admin) are untouched. The settings "Models & routing" page is the excluded-models admin surface, not the catalog explorer — they answer different questions.

## User Stories

1. As a router dashboard user, I want to see a single ranked view of which models I've actually been using, so that I can decide whether my traffic is on the right models.
2. As a router dashboard user, I want to see the live ai& model catalog (the models I can route to, with their pricing and capabilities), so that I can choose models without bouncing to ai&'s own console.
3. As a router dashboard user, I want to filter the model catalog by capability (vision, video, document, reasoning, tool_calling), so that I can answer "which models can handle images?" without reading the docs.
4. As a router dashboard user, I want to filter the model catalog by provider, so that I can answer "which models come from Google?" at a glance.
5. As a router dashboard user, I want to filter the model catalog by tier (Low / Mid / High), so that I can find strong-capability models.
6. As a router dashboard user, I want to sort the model catalog by total tokens processed on my install, so that the most-used models rise to the top.
7. As a router dashboard user, I want to sort the model catalog by input or output price, so that I can find the cheapest models at my target tier.
8. As a router dashboard user, I want to sort the model catalog by context window, so that I can find models that fit my largest prompts.
9. As a router dashboard user, I want a model detail page that shows price, capabilities, context, and the reasoning-effort menu, so that I can pick a model knowing what I'm getting.
10. As a router dashboard user, I want to see per-model request counts, spend, and cost-per-1K over the recent period, so that I can spot which model is the expensive one.
11. As a router dashboard user, I want to see per-model 24h / 7d / 30d mini-statistics, so that I can tell whether a model is doing a lot this week vs historically.
12. As a router dashboard user, I want to click a model anywhere on the dashboard and land on its detail page, so that I can drill in without navigating from memory.
13. As a router dashboard user, I want to compare up to four models side-by-side on price, context, capabilities, and tier, so that I can pick between them in one screen.
14. As a router dashboard user, I want the comparison view to compute the cost of a sample 50K-token request shape per model, so that I can see the real-shapes-cost.
15. As a router dashboard user, I want the comparison view to compute the cost of that same shape under a 70% prompt-cache-hit assumption, so that I can see how caching changes the picture.
16. As a router dashboard user, I want my comparison selection to survive a refresh, so that I'm not re-picking models every time I reload.
17. As a router dashboard user, I want to add a model to my comparison basket from the model detail page, so that I can build up a comparison as I browse.
18. As a router dashboard user, I want the cache hit rate prominently displayed as a KPI, so that I can tell whether my prompts are taking advantage of caching.
19. As a router dashboard user, I want the cache hit rate to render as `—%` when there's no cached usage yet, so that I don't see a fake percentage on a fresh install.
20. As a router dashboard user, I want KPI sparklines plus period-over-period deltas on the overview, so that I can spot trends and inflection points at a glance.
21. As a router dashboard user, I want to filter the existing savings / cost stories by date range, so that I can investigate specific incidents.
22. As a router dashboard user, I want the popularity leaderboard to rank models by actual tokens processed on this install, so that the rank reflects *my* usage — not industry aggregations.
23. As a router dashboard user, I want the popularity leaderboard to be clickable — each row leads to the model's detail page, so that I can drill from "popular this week" to "and here's why."
24. As a router dashboard user, I want to see a Top-models-by-spend table, so that I can see who's eating the budget.
25. As a router dashboard user, I want the catalog to show reasoning-effort options per model (`none / low / medium / high / max`), so that I know which models accept which efforts.
26. As a router dashboard user, I want the catalog to show the prompt-cache input price when a model supports caching, so that I can price in caching when comparing.
27. As a router dashboard user, I want the catalog to show token currency, so that I can confirm whether I'm seeing USD or JPY.
28. As a router dashboard user, I want a clear message when ai&'s catalog isn't reachable (no AIAND_API_KEY or upstream 502), so that I know whether the empty table is real or a config issue.
29. As a router dashboard user, I want the catalog page to render even when the catalog is empty, with an empty-state instead of a hard error, so that I'm not bounced to a broken page on a fresh install.
30. As a router dashboard user, I want the model id to render as a tooltip that shows the raw upstream id, so that I can copy and paste the exact id for documentation.
31. As a router dashboard user, I want model detail to suggest "compare with…: three other models in the same tier", so that peer comparisons are one click away.
32. As a router dashboard user, I want the dashboard's existing settings pages (Router keys, Provider keys, Models & routing) to remain reachable exactly where they are, so that I don't lose muscle memory.
33. As a router dashboard user, I want the new Models route to be siblings to the existing /settings/models route, not in conflict, so that the explorer and the admin surface both work.
34. As a router dashboard user, I want the dashboard to function in managed mode just like today (no changes there), so that nothing regresses for managed deployments.
35. As a router dashboard user, I want the dashboard to keep working when ai& is unreachable by serving a stale cached catalog for up to a minute, so that an ai& blip doesn't make the entire Models page unavailable.
36. As a router dashboard user, I want the catalog to refresh from the live ai& endpoint at least once a minute, so that a deprecation or new release takes effect within a minute.
37. As a router dashboard user, I want to see caching economics visualized as both a percentage (hit rate) and a dollars figure (cache savings = cache_tokens × (input - cached) per 1M rate), so that the dollar impact is visible.
38. As a router dashboard user, I want the boolean "supports caching" indicator on each catalog entry, so that I know whether a model even has a caching story.
39. As a router dashboard user, I want to be able to dismiss a model from the comparison basket without leaving the comparison page, so that quick re-picks don't require navigation.
40. As a router dashboard user, I want the dashboard to mount in selfhosted mode only — the existing `Router keys` and `/settings/*` contract — so that the surface parity with today's router-keys and provider admin hold.

## Implementation Decisions

### Backend additions

- **A new authenticated read-only backend endpoint serves the live ai& catalog to the dashboard.** It uses the deployment's `AIAND_API_KEY` to call ai& and forwards the response shape unchanged, so the frontend's TypeScript types mirror ai&'s canonical model row 1:1. The endpoint sits behind the same auth posture as the existing admin metrics endpoints and is mounted only in selfhosted mode (matching today's `/admin/v1/*` mount convention). `AIAND_API_KEY` is read via existing helpers at composition-root construction time; if absent, the endpoint is not registered (boot fails closed, not per-request 5xx).
- **An in-process 60-second TTL on the upstream response** absorbs dashboard-tab fan-out without hammering ai&. A second call within the window is served from cache and does not produce a second upstream request.
- **Three SUM columns added to existing metrics rollups** (summary, timeseries, model-breakdown): `cache_write_tokens` and `cache_read_tokens` SUM-mirrored from existing per-row metrics data into the per-period rollups. No new tables, no migration.
- **No new errors, middleware, permissions, cross-layer imports, or layer-test seams.** `internal/api/admin` already imports the inner-ring packages this needs (a typed `Client` in `internal/providers/openaicompat` is a deliberate adapter-to-inner-ring inward-pointing import that we further co-use here to keep the catalog call shape consistent with how the rest of the system already calls aiand-shape endpoints).
- **The router's compile-time catalog** — the source of truth for routing pricing and the dispatcher's per-model provider bindings — is **untouched**. The new endpoint serves as a *display source-of-truth* for the dashboard; per-model chart queries already filter on routing-decision strings, not on FK-like catalog ids, so an ai& rename can't produce a 500 even mid-period.

### Frontend work

- **Three new routes under the (app) group**: a catalog explorer, a per-model detail page (the new home of today's two `ModelBreakdownChart`s, expressed as one chart with a `requests ↔ spend ↔ cost-per-1K` metric toggle), and a 4-cap side-by-side compare page reachable directly via a deep-link.
- **The existing `/dashboard` route is re-scoped, not removed.** The big savings charts (savings rate, cumulative savings, cost breakdown) move to the model-detail page; the Overview keeps the four KPI cards, the popularity leaderboard, and a Top-models-by-spend table. The existing `DashboardPageFilters` continue to drive the date range.
- **Sidebar nav gains a Models item** alongside the existing Overview; the settings sidebar and footer are unchanged.
- **Shared, domain-aware atoms**: capability / tier badge variants, a Sparkline component (zero recharts overhead), a small segmented-control MetricToggle, and a color-token palette for capability / tier badges centralized into a single lib helper.
- **Two new chart components**: `PopularityLeaderboard` (top-N models by tokens processed in the selected range, horizontal bars) and `CacheHitGauge` (cache hit rate as a percent, branch-aware zero-state).
- **A small client-side compare-basket store** with a hard cap at four items, persisted to `localStorage`, and reads from a `?ids=a,b,c,d` URL on mount so a comparable URL is shareable. The store caps at 4 on add even if the URL or store pre-loaded more.
- **The frontend doesn't introduce new cross-layer imports.** Every page composes existing primitives plus the new atoms + the new endpoint; `lib/api.ts` gains one new method (`api.aiandModels.list()`).
- **The existing `/settings/models` route stays** — it's an admin surface (excluded models), distinct from the public catalog explorer. Both pages are siblings; no rerouting.

### Deployment mode + secrets

- Selfhosted-only mount, exactly like today's `/admin/v1/metrics/*`. Managed mode is untouched.
- `AIAND_API_KEY` access concentrated in composition root; no plaintext leakage to browser, no log line containing the key, no key logged at any level.
- Logging follows existing patterns: cache transitions get a structured Debug; upstream failures get an Error with `err` and an upstream-status code; never log raw payloads.

## Testing Decisions

- **Good test = asserts external behavior that breaks when prod code is deleted, not implementation.** Example: asserting the cache-hit-gauge renders `—%` for an empty cache_read vs asserting that the React component tree has a specific className.
- **For the new catalog endpoint**: an `httptest` upstream proves the verbatim-200 forward, the 502-on-upstream-down branch, the second-call-within-TTLP-doesn't-re-hit-upstream counter, and the empty-data-array-is-200-not-502 branch.
- **For the Postgres rollup SUMs**: extend existing fixtures with rows carrying non-zero `cache_*_tokens` and assert round-trip SUMs match the input. No new table tests.
- **On the frontend**, the following earn tests because they each encode a contract a future refactor can break:
  - format helpers (`formatUSD`'s distinct branches for `0 / NaN / abs(v)<0.001`; `formatNumber`'s 1K / 1M / 1B boundaries; `formatContext`'s small / large).
  - The compare-page verdict math (sample shape and 70%-cache-hit shape) — these encode the spec's pricing formula.
  - The cache-hit gauge's **denominator-is-zero** branch — encodes the `—%` contract.
  - The compare-basket store's **5th-add-rejected** branch and **localStorage hydration**.

- **What's worth NOT testing** (and so won't have tests): that the catalog table renders nine rows by default; that the popularity leaderboard has 5 entries; that the comparison page clamps to 4 from a 5-item URL is worth testing but the cosmetic-only render of each badge variant is not.

## Out of Scope

- **Approach B features** (spend progress bar with multi-threshold alert markers; rate-limit dashboard with per-model RPM/TPM/concurrency gauges) — deferred to v1.1.
- **Approach C features** (request logs, playground with streaming + per-token logprob visualization, billing/credits balance + auto-reload, key-page polish, app attribution, cache prefix flamegraph) — deferred indefinitely.
- **Adding non-ai& upstreams** (Fireworks, Groq, Together, DeepInfra, OpenRouter, Gemini, xAI) — explicitly locked: the router stays aiand-exclusive. The dashboard surface draws on ai&'s catalog and the existing router metrics only.
- **A new OTel span catalog** for the dashboard surface — v1 uses the existing observability group.

## Further Notes

- **Why aiand-exclusive matters for the differentiation.** ai& itself can't render a cross-provider popularity view — it sees its own /analytics/metrics and a flat list of (provider, model) pairs without a totals-by-provider dimension. Because the router sees every routed request and records which `decision_model` was used, we have a unique position to render "this install's tokens-processed by `deepseek-ai/*` vs `openai/*` vs `google/*` vs `qwen/*` vs `moonshotai/*` vs `zai-org/*` vs `motif-technologies/*`". The leaderboard is the surface to ship first; everything else is supporting cast.
- **Why no new persistence for v1.** The catalog is read fresh from ai& on cache miss + 60 s TTL. Per-model metrics are already aggregated; we just need three more SUMs. Pending costs: a single new component, one new Postgres SUM clause per existing rollup, one new handler, ~2 lines of composition-root wiring.
- **Seam check.** Highest seam used for behavior change is the dashboard route group on Next.js (one new sub-tree under the (app) group). Backend seam is at the existing `/admin/v1` admin group. No deeper seams needed.
- **Issue tracker.** Spec written to `docs/superpowers/specs/`. No linear / GitHub issue created automatically; tell me which tracker to file on if you want this published.

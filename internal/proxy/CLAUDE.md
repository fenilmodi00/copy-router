# internal/proxy — CLAUDE

> **Mirror notice.** Verbatim sync with [AGENTS.md](AGENTS.md). **Update both together** — divergence = bug.

Routing/dispatch service. Per-action orchestrator that composes scorer, planner, handover, cache, sessionpin, catalog, turntype, usage, billing, providers, and translate. Read [root CLAUDE.md](../../CLAUDE.md) first.

## Surface

`*proxy.Service` in [`service.go`](service.go) exposes:

- `Route` — returns `router.Decision`
- `ProxyMessages` — Anthropic Messages surface
- `ProxyOpenAIChatCompletion` — OpenAI Chat Completions

Plus the action loop, handover adapter, cache writer, and session-key derivation in sibling files.

## Adding a method to `*proxy.Service`

1. **Define method on `*proxy.Service`.** No I/O directly here — push into a provider adapter or repo. Inner-ring imports (`router`, `providers`, `translate`, `observability`, `internal/router/*`, `internal/proxy/usage`) + small utility libs are fine.
2. **If you need new repo methods**, surface them as an interface in the inner-ring package, implement in `internal/postgres/`. Example: `sessionpin.Store` in [`../router/sessionpin/store.go`](../router/sessionpin/store.go), implemented by `postgres.SessionPinRepository`.
3. **Update `service_test.go` fakes** to satisfy the expanded interface. Real assertions on return values, not "mock called with X".

## Per-action flow (cache-aware action routing)

The per-action flow is more than "scorer → dispatch". Pinned session, planner verdict, optional handover summary, and the semantic response cache all sit between the inbound request and the upstream provider. Packages are intentionally small + single-purpose so each is unit-testable without the others. ("Action" = one upstream API request; see [../../docs/SEMANTICS.md](../../docs/SEMANTICS.md). The `Stage` column below is a pipeline stage *within* one action, not a router step.)

| Stage | Package | Notes |
|---|---|---|
| Turn-type classification | [`../router/turntype`](../router/turntype) | MainLoop / ToolResult / SubAgentDispatch / Compaction / Probe |
| Session-pin lookup | [`../router/sessionpin`](../router/sessionpin) | Sticky `(api_key_id, session_key, role)` pin |
| Fresh routing decision | [`../router/cluster`](../router/cluster) | Cluster scorer argmax |
| STAY vs SWITCH | [`../router/planner`](../router/planner) | Cache-aware EV policy |
| Handover summary on SWITCH | [`../router/handover`](../router/handover) | Bounds switch-action input cost |
| Semantic response cache | [`../router/cache`](../router/cache) | Cross-request, non-streaming only |
| Subscription strict pass-through gate | [`usage`](usage) | See [`usage/CLAUDE.md`](usage/CLAUDE.md) |

The provider-backed `Summarizer` implementation for handover lives in [`handover.go`](handover.go); the inner-ring `handover` package only defines the contract. On summarizer timeout or error, proxy keeps the full prior history unchanged (it does **not** trim) — a pricier switch action beats silently dropping the conversation the switched-to model needs.

## Proactive context-window compaction

`ProxyMessages` / `ProxyOpenAIChatCompletion` call [`maybeCompact`](compaction.go) **before** routing so an over-long session is compacted rather than dead-ending in the scorer with no eligible provider. It engages when the estimate reaches `ROUTER_COMPACTION_PCT` (default 0.85) of the largest eligible model's window and runs Claude Code's tiered cascade: (1) `ClearOldToolResults` — local, clears stale tool results; (2) structured 9-section summary via a **window-aware** Anthropic-family summarizer (`SummarizeForCompaction`; haiku when the history fits, `claude-fable-5` for larger) rewritten with `RewriteForCompaction(summary, recentTurns)`; (3) progressive `TrimLastNMessages` rescue. If even the trimmed floor overflows, it returns `ErrContextWindowExceeded` → HTTP 413 (distinct from the "no provider keys" `ErrNoEligibleProvider`). The summary call is billed as a `_precompaction_summary` ledger row. Trigger below the window (not at overflow) is load-bearing: a summarizer can only ingest a history that still fits *some* model.

## Model-restriction layers

Three distinct restrictions compose, and the layering is deliberate — do not
collapse them.

| Layer | Scope | Polarity | Where enforced |
|---|---|---|---|
| `allowed_models` | org (installation) | fail-closed | desugared into `ExcludedModels` by `excludedModelsForRequest` |
| `excluded_models` | org (installation) | fail-closed | scorer + policy resolver |
| `cluster_model_lists` | API key (org default) | fail-open | `policy.ApplyClusterArmOverrides` |
| `model_router_user_cluster_model_lists` | router user | fail-open | same, after `mergeClusterOverrides` |

**The allowlist is desugared, not separately filtered.** `excludedModelsForRequest`
adds every routable model absent from a non-empty allowlist to the exclusion
set, so all six existing enforcement sites honor it with no new filter loops.
`router.Request.AllowedModels` exists only so errors and diagnostics can name
the allowlist instead of dumping the desugared exclusion list.

**A wholly non-routable allowlist is rejected at the admin API.** Membership
validation for `PUT /admin/v1/allowed-models` is catalog-wide on purpose —
force-model and hard-pin reach rows the router never scores — but the
desugaring only excludes over `routableUniverse()`. A list naming *only*
non-routable rows (an unbound provider, or a tierless passthrough-only row)
therefore excludes the entire pool and 400s every routed request with
`ErrAllowlistEmptiesPool`. The handler calls `Service.RoutableModels()` and
refuses to save such a list, deferring to the unknown-model error first so a
typo is not reported as a routability problem. Keep that accessor and the
desugaring reading the same universe. `availableModels` is the generic
`RoutingTargetSet` plus `HMMRoutingTargetSet` when an HMM sidecar is wired,
so an HMM-only allowlist is not rejected as emptying the pool.

**The fail-open/fail-closed asymmetry is load-bearing.** An org allowlist is a
compliance control (a breach is worse than an outage); a user's per-cluster
selection is a preference (hard-failing a turn because a personal pick went
stale is a terrible trade). This composes safely ONLY because the allowlist
binds upstream in the resolver, while the preference binds downstream in the
override layer — falling open there falls back to a selection that is already
allowlist-constrained. **Never enforce the allowlist in the override layer**;
fail-open would silently defeat it.

**Per-cluster lists intersect, never override.** `mergeClusterOverrides`
intersects a user's selection with the API-key-scoped list. A plain override
would let an individual re-admit a model the org deliberately removed —
privilege escalation through an admin control.

**Two paths deliberately bypass `excluded_models` and need explicit allowlist
handling:** `usageBypassEngaged` (consults `SafetyExcludedModels`, since
exclusions are a preference the bypass may override — but an allowlist is not)
and `ROUTER_EXCLUDED_MODELS` (an operator escape hatch that short-circuits
`excludedModelsForRequest` entirely; intentional, so an operator debugging a
deployment is not constrained by one org's config).

## Translation

`proxy.Service` is the **only caller of [`../translate`](../translate)**. Keep providers ignorant of cross-format concerns. See [translate/CLAUDE.md](../translate/CLAUDE.md) for the recipe.

## Runtime provider fallback

This deploy's catalog rows are **single-binding `ProviderAiand`**. Cross-binding failover in [`dispatchWithFallback`](fallback.go) is dormant until a second binding appears; same-binding retry and sibling-model failover still matter.

`dispatchWithFallback` walks the ordered binding list from [`catalog.Model.Providers`](../router/catalog/catalog.go), retries on `providers.IsRetryable` errors (5xx/408/429 buffered responses, transport errors, `httputil.ErrUpstreamIdleTimeout`), and on exhaustion writes the final upstream error envelope via a format-specific renderer (`flushUpstreamErrorAsAnthropic` for ProxyMessages, `flushBufferedIfPresent` for ProxyOpenAIChatCompletion).

**Model-not-found 404 → cross-binding failover (only).** A buffered upstream 404 (`providers.IsUpstreamModelNotFound`) means the chosen provider doesn't serve the model — a stale/wrong upstream id or a provider with no active endpoints. It is deliberately *not* in `IsRetryable`: re-hitting the same provider is futile (so it never triggers same-binding retry), but a different provider binding may carry the model, so it triggers one cross-binding hop. On the last binding the 404 still flushes.

**Billing-blocked 402 → cross-binding failover (only).** Same shape as the 404 (`providers.IsUpstreamProviderBillingBlocked`): the provider refuses this account (credits exhausted, plan gate). Same-binding retry would re-bill the same rejection, so it walks to the next binding and flushes on the last one.

**Same-cluster model failover ([`sibling_failover.go`](sibling_failover.go)).** When every binding is exhausted pre-commit on a retryable/404/402 fault, `ProxyMessages` re-dispatches a *different* model drawn from `Decision.Metadata.CandidateModels` — the pool the policy already scored for this turn — preferring a candidate on a provider other than the one that just failed. On the aiand-only catalog this is usually another aiand model in the quality band. Gated by `ROUTER_SIBLING_FAILOVER` (default on) and skipped for `/force-model`, shadow mode, BYOK (`shouldFailover`), and subscription-only balances.

**Single-binding same-binding retry.** Catalog models here carry one binding, so cross-binding failover has nowhere to walk. `dispatchWithFallback` retries the *same* binding in place up to `maxSameBindingRetries` (2) with exponential backoff (`sameBindingBackoff`: 250ms, 500ms), pre-commit only, abortable on ctx cancel (`sleepWithContext`). Multi-binding models (if reintroduced) skip in-place retry and fail over to the next provider. Tests inject `Service.retrySleep` to keep the backoff instant.

**Retries are bounded twice: count AND wall-clock.** `maxSameBindingRetries` caps how many attempts; `sameBindingRetryBudget` (10s) caps how much time they may consume in total. Cheap failures (5xx in milliseconds) still get the full attempt count. The budget stopping a retry logs at WARN with spent_ms/budget_ms. Tests inject `Service.now` (alongside `retrySleep`) to simulate a slow attempt without burning real time.

`preludeBuffer` wraps the client writer on every request so the eager SSE Prelude doesn't commit the response to the client before the upstream produces its first byte. The buffer absorbs pre-Seal writes (Prelude's status + `message_start`), commits on the first post-Seal write (= first upstream chunk), and `Discard()`s pre-commit state between attempts so a retry begins with a pristine writer. `Committed()` is the retry gate: once it flips, the response is on the wire and no further retry is allowed.

Per-attempt body rebuild: each closure constructs `EmitOptions` with `TargetProvider = d.Provider` so provider-specific gates in [`emit_openai.go`](../translate/emit_openai.go) (session-affinity headers, reasoning quirks) follow the attempt's target. aiand uses the generic OpenAI-compat path.

**Invariants:**
- Unconditional wrap: `preludeBuffer` engages on every request (single- and multi-binding alike). TTFB cost is ~200B of buffered SSE released the moment the upstream's first byte arrives. `Committed()` is the retry gate for both cross-binding failover and single-binding in-place retry.
- Retry gated on `preludeBuf.Committed() == false`. Once committed (first upstream byte flushed through the chain), switching providers mid-stream would interleave two model outputs.
- Per-attempt `Prepare*` + translator construction. Translators are stateful; a retry must rebuild the chain from scratch.
- BYOK and inbound-client-credential requests skip failover entirely (`shouldFailover()` returns false) — those keys bind to one provider and would 401 elsewhere.
- Cancel/deadline classified as non-retryable: client disconnect or per-request budget elapse must not waste a second upstream call.
- After dispatch, `actPricing` is re-resolved against the WINNING binding via `catalog.PriceFor(finalProvider, decision.Model)` so debits and OTel `cost.actual_*` reflect the actually-served provider's per-1M rate (the catalog's `PrimaryPriceFor` would otherwise always return the primary's).

## Client-facing SSE keepalive

Clients time a stream out on received BYTES, not on semantic events: Claude Code
aborts a first-party stream (which includes any `ANTHROPIC_BASE_URL` override,
so all routed traffic) after **180s** of byte silence. A long reasoning phase
translates to zero client-facing frames, so a healthy turn reads as a dead
connection — prod 2026-08-24, three `gpt-5.6-luna` turns that each spent their
whole 64K output budget reasoning for 320-360s and were killed client-side at
exactly 180s while the router went on to complete them successfully.

None of the upstream watchdogs can cover this. They measure the *upstream* leg:
byte-idle keeps getting reset by Responses bookkeeping frames, and the
output-stall budget (240s) is longer than the client's 180s by design. The gap
is the *client* leg, so `ProxyMessages` wraps the client writer in
[`sse.KeepaliveWriter`](../sse/keepalive.go), which emits `anthropicPingFrame`
after `ROUTER_SSE_KEEPALIVE_INTERVAL_SECONDS` (default 15s, 0 disables) of
silence. This is what direct-to-Anthropic already gets for free — Anthropic's own
API emits `ping` for exactly this reason (see `anthropic_footer.go`, which
forwards them).

**Invariants:**
- **Innermost wrap.** The keepalive sits closest to the socket, below the
  feedback footer, so it observes the bytes that actually reach the client
  rather than what the proxy handed to the next translator.
- **Arms on first byte, never before.** For a `preludeBuffer`-wrapped chain the
  first byte through is the commit point, so a keepalive can never commit a
  response the router still wants to retry on another binding.
- **Record-boundary only.** A frame goes out only when the last write ended on a
  blank line, so a keepalive can never land inside a partially written event.
- **`Close()` before the handler returns** (deferred at the wrap site) and it
  blocks on any in-flight frame, so no keepalive outlives the response.

Only the Anthropic Messages surface is wrapped today — that is where the failure
was observed. `KeepaliveWriter` takes the frame as a parameter, so adding the
OpenAI surface is a wiring change plus their own frame.

## `OnUpstreamMeta` callbacks

Provider adapters call back into `proxy.OnUpstreamMeta` so streaming responses record usage/headers back to proxy without coupling provider packages to proxy internals. The catalog / planner stack depends on per-action token counts being recorded promptly — **don't add a provider that forgets to call the callback.**

## What NOT to do

- **Don't move provider-call logic into planner.** Planner must remain pure so EV math is provable. Anything network-touching goes in `proxy.Service`.
- **Don't add a handover path that doesn't time out.** `Summarizer` contract says implementations MUST respect the context deadline. On timeout/error the proxy keeps the full prior history unchanged — do NOT reintroduce a silent trim-to-last-N fallback (it lobotomized switched-to models; see the handover-fallback fix).
- **Don't cache streaming responses.** Streaming bypasses cache on purpose — captured bytes would be post-translation SSE frames, and lookup latency budget is hostile to first-token-time. If you think this should change, write a doc first.

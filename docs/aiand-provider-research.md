# ai& (aiand.com) — Provider Research

> Research notes for adding **ai&** as an upstream provider in this router.
> All claims cite a primary source (ai&'s own docs at `docs.aiand.com` or its
> marketing site `aiand.com`). Anything not verified from a primary source is
> marked **unverified**.

## Summary

ai& (branded "ai&", domain `aiand.com`, operator **ai& Inc.**) is a
vertically-integrated AI inference company based in Yokohama, Japan that
serves **open-weight LLMs from its own Japan-resident infrastructure** through
**OpenAI- and Anthropic-compatible APIs** with per-token credit billing
[docs overview](https://docs.aiand.com/) · [about](https://www.aiand.com/about).
It is, for this router's purposes, an **OpenAI-compatible inference provider**
in the same category as OpenRouter / Together / Fireworks — drop-in via a
`base_url` override, plus a parallel Anthropic-shaped surface.

- **Operator:** ai& Inc., founded March 2026, Queen's Tower A, 2-3-1
  Minatomirai, Nishi-ku, Yokohama-shi, Kanagawa, Japan. Co-Founders: David
  Bennett (CEO) and Shimpei Hara (President). $50M seed + >$2B committed data
  center capital. [about](https://www.aiand.com/about)
- **What they offer:** hosted inference of frontier open-weight models
  (Kimi, DeepSeek, GLM, Qwen, Gemma, gpt-oss) on heterogeneous GPU infra in
  Japan, data-resident with zero cross-border egress. [inference](https://www.aiand.com/inference)
- **API compatibility:** OpenAI Chat Completions / Responses / Models +
  Anthropic Messages, on one base URL. [docs](https://docs.aiand.com/) ·
  [migrating-openai](https://docs.aiand.com/migrating-openai/) ·
  [migrating-anthropic](https://docs.aiand.com/migrating-anthropic/)
- **OpenAI-compatible?** Yes — confirmed wire-compatible, not just similar.
  [migrating-openai](https://docs.aiand.com/migrating-openai/)

## API surface

### Base URL & authentication

| Item | Value | Source |
|---|---|---|
| API base URL | `https://api.aiand.com` | [docs overview](https://docs.aiand.com/) ("Base URL") |
| SDK `base_url` (OpenAI/Anthropic SDK) | `https://api.aiand.com/v1` | [quick-start](https://docs.aiand.com/getting-started/) · [openai-sdk](https://docs.aiand.com/sdks/openai/) · [migrating-anthropic](https://docs.aiand.com/migrating-anthropic/) |
| Auth scheme | Bearer token in `Authorization` header: `Authorization: Bearer sk-your-api-key` | [authentication](https://docs.aiand.com/authentication/) |
| Key format | API keys start with `sk-` (not `sk-ant-`), shown once at creation | [quick-start](https://docs.aiand.com/getting-started/) · [migrating-anthropic](https://docs.aiand.com/migrating-anthropic/) |
| Secondary auth | JWT (session auth) for browser/console flows; pass `X-Org-ID` header. API keys carry their org automatically. | [migrating-openai](https://docs.aiand.com/migrating-openai/) · [authentication](https://docs.aiand.com/authentication/) |
| Required for all requests | Yes — "Authentication is required for all requests." | [docs overview](https://docs.aiand.com/) |

Two auth methods are supported: **API Key (recommended)** and **JWT (session
auth)**. The router should use API keys exclusively
[authentication](https://docs.aiand.com/authentication/).

### Endpoints

ai& mirrors three wire formats. The `model` parameter is the same literal
model ID across all of them.

| Endpoint | Method | Wire format mirrored | Notes | Source |
|---|---|---|---|---|
| `/v1/chat/completions` | POST | OpenAI Chat Completions | "Fully compatible." Full parity. | [chat-completions](https://docs.aiand.com/api/chat-completions/) · [migrating-openai](https://docs.aiand.com/migrating-openai/) |
| `/v1/messages` | POST | Anthropic Messages | Accepts Anthropic format; **translated internally to OpenAI format**, response translated back to Anthropic shape. Streaming + tool use + vision supported. | [messages](https://docs.aiand.com/api/messages/) · [migrating-anthropic](https://docs.aiand.com/migrating-anthropic/) |
| `/v1/responses` | POST | OpenAI Responses API | Compatible; supports `previous_response_id`, `store`, `instructions`. | [responses](https://docs.aiand.com/api/responses/) · [migrating-openai](https://docs.aiand.com/migrating-openai/) |
| `/v1/completions` | POST | OpenAI Completions (legacy) | Listed under "Inference API" nav. | [docs nav](https://docs.aiand.com/) |
| `/v1/models` | GET | OpenAI Models | Returns models the org has access to, with pricing + metadata. | [models](https://docs.aiand.com/api/models/) |

**No Gemini-native surface.** ai& exposes OpenAI and Anthropic shapes only —
no `/v1beta/models/:action` Gemini endpoint is documented
[docs nav](https://docs.aiand.com/). For this router, ai& routes through the
OpenAI-compatible adapter (`internal/providers/openaicompat`), not a native
Gemini path.

### Model name resolution (provider quirk)

The `model` value is resolved as: **(1) exact case-insensitive match** against
a model `id`, else **(2) prefix match** to the highest-priority model whose
`id` starts with the value. Unmatched → `400` pointing at `GET /v1/models`.
Usage is always billed against the resolved canonical `id`
[models](https://docs.aiand.com/api/models/).

> Router implication: when forwarding, send the **exact catalog `id`** (e.g.
> `zai-org/glm-5.2`), not a bare alias like `glm`, to avoid ambiguous prefix
> resolution across versions.

## Model catalog

**This table reflects the LIVE `GET /v1/models` snapshot (2026-08-24).** The
static marketing catalog page drifts and was already stale (it listed
`kimi-k2.6` / `glm-5.1` as current and Qwen as free). See
[`docs/aiand-live-catalog.md`](aiand-live-catalog.md) for the current models,
new/deprecated changes, and cached-input pricing, and
[`docs/aiand-model-catalog.json`](aiand-model-catalog.json) for the verbatim
endpoint dump. **Always refresh from the endpoint before wiring catalog
bindings** — the list is dynamic per-org
[catalog](https://docs.aiand.com/models/catalog/) ·
[models](https://docs.aiand.com/api/models/).

Current models (USD per 1M tokens; `cached` = prompt-cache input):

| Model ID (`model` value) | provider | Context | In/1M | Out/1M | Cached/1M |
|---|---|---|---|---|---|
| `deepseek-ai/deepseek-v4-flash` | deepseek-ai | 1M | $0.15 | $0.25 | $0.08 |
| `openai/gpt-oss-120b` | openai | 131K | $0.15 | $0.60 | $0.08 |
| `google/gemma-4-31b-it` | google | 262K | $0.20 | $0.50 | $0.05 |
| `qwen/qwen3.6-27b` | qwen | 262K | $0.32 | $3.20 | $0.20 |
| `motif-technologies/motif-3` | Motif-Technologies | 262K | $0.50 | $2.00 | $0.20 |
| `moonshotai/kimi-k2.7-code` | moonshotai | 262K | $0.75 | $3.50 | $0.20 |
| `deepseek-ai/deepseek-v4-pro` | deepseek-ai | 1M | $1.00 | $2.50 | $0.25 |
| `zai-org/glm-5.2` | zai-org | 1M | $1.00 | $4.00 | $0.30 |
| `moonshotai/kimi-k3` | moonshotai | 1M | $3.00 | $12.50 | $0.50 |

> Deprecated since the static doc was written: `moonshotai/kimi-k2.6` and
> `zai-org/glm-5.1` are no longer returned by `/v1/models`. Qwen is not free
> (now $0.32/$3.20).

Billing currency is per-org (`usd` or `jpy`), set at org creation
[catalog](https://docs.aiand.com/models/catalog/) ·
[models](https://docs.aiand.com/api/models/).

Open-weight model families served: **Qwen, DeepSeek, Gemma (Google),
gpt-oss (OpenAI open weights), Kimi (Moonshot), GLM (Zhipu/zai-org)** = live
catalog. Notably **no Llama or Mistral** appear in the current catalog
[`docs/aiand-live-catalog.md`](aiand-live-catalog.md).

### `/v1/models` response shape (OpenAI surface)

Returns an OpenAI-shaped `{"object":"list","data":[...]}` with extra fields
[models](https://docs.aiand.com/api/models/) · [catalog](https://docs.aiand.com/models/catalog/):

```json
{
  "object": "list",
  "data": [{
    "id": "openai/gpt-oss-120b",
    "object": "model",
    "owned_by": "ai&",
    "provider": "openai",
    "context_window": 131072,
    "capabilities": ["reasoning", "tool_calling"],
    "reasoning_efforts": ["low", "medium", "high"],
    "reasoning_effort_default": "medium",
    "description": "OpenAI GPT OSS 120B",
    "currency": "usd",
    "input_per_1m": "0.150000",
    "output_per_1m": "0.600000",
    "created": 1775474514
  }]
}
```

With an `anthropic-version` header, `/v1/models` returns the **Anthropic
shape** (`data[].display_name`, `created_at`, `has_more`, `first_id`,
`last_id`) — but pricing/capabilities are only on the OpenAI surface
[catalog](https://docs.aiand.com/models/catalog/).

## Streaming

All generative endpoints support streaming via SSE with `stream: true`
[streaming](https://docs.aiand.com/capabilities/streaming/).

| Shape | Format | Source |
|---|---|---|
| OpenAI | `data: {...}` events with `choices[].delta` chunks; terminal `data: [DONE]`. Use `stream_options.include_usage = true` for the final `usage` block. | [streaming](https://docs.aiand.com/capabilities/streaming/) · [chat-completions](https://docs.aiand.com/api/chat-completions/) |
| Anthropic | Typed events: `message_start`, `content_block_delta`, `message_delta`, `message_stop`. Usage carried in `message_delta`. | [streaming](https://docs.aiand.com/capabilities/streaming/) |

**Provider quirk — metrics trailer event.** ai& never modifies the model's
SSE byte stream. With the opt-in request header `X-Aiand-Metrics: true`, it
appends a single trailing `event: metrics` SSE event (after the terminal
message) carrying token counts, cost, and currency. The stream above the
trailer is byte-identical to the source API, so official SDKs work unmodified
[streaming](https://docs.aiand.com/capabilities/streaming/) ·
[streaming-events](https://docs.aiand.com/reference/streaming-events/) ·
[response-headers](https://docs.aiand.com/reference/response-headers/).

> Router implication: the existing `internal/sse` framing + OpenAI-compatible
> streaming path handles ai& unmodified. The `metrics` trailer is an extra
> named event after `[DONE]`; ignore it unless we want to reconcile cost
> server-side (we already compute cost from `usage`).

## Pricing, billing & rate limits

### Pricing

Per-token, deducted from a **prepaid credit balance** after each request
completes; failed requests are not billed. Minimum top-up $1
[pricing](https://docs.aiand.com/models/pricing/) · [quick-start](https://docs.aiand.com/getting-started/).

```
cost = (input_tokens / 1_000_000) * input_per_1m
     + (output_tokens / 1_000_000) * output_per_1m
```
Source: [pricing](https://docs.aiand.com/models/pricing/).

**Prompt caching** (provider quirk): applies to prompts ≥1024 tokens, counted
in 128-token increments; org-scoped; ~10 min inactivity TTL. Models with a
cached input rate report `cached_tokens`; others omit the field and bill all
input at the standard rate [pricing](https://docs.aiand.com/models/pricing/).
Cached input rates observed on the marketing page
[inference](https://www.aiand.com/inference):

| Model | Standard input / 1M | Cached input / 1M |
|---|---|---|
| GLM-5.2 | $0.80 (marketing) / $1.00 (catalog) | $0.30 |
| DeepSeek V4 Flash | $0.15 | $0.08 |
| DeepSeek V4 Pro | $1.00 | $0.25 |
| gpt-oss-120b | $0.15 | $0.08 |

> Note: the GLM-5.2 standard input rate differs between the marketing
> (`$0.80`) and the catalog (`$1.00`) pages. Treat `GET /v1/models` /
> `docs.aiand.com/models/catalog/` as authoritative.

**Per-request cost reporting** (opt-in, header `X-Aiand-Metrics: true`):
non-streaming → `X-Cost` + `X-Cost-Currency` + `X-Inference-Ms` response
headers; streaming → the `metrics` trailer event
[pricing](https://docs.aiand.com/models/pricing/) ·
[response-headers](https://docs.aiand.com/reference/response-headers/).

### Rate limits

Per-organization, tier-based, applied to inference endpoints (management APIs
have looser limits) [rate-limits](https://docs.aiand.com/reference/rate-limits/).
Buckets: `rpm`, `global_rpm`, `input_tpm`, `output_tpm`, `concurrency`,
`global_concurrency`. On `429`, two extra headers:

| Header | Meaning |
|---|---|
| `X-RateLimit-Policy` | Which bucket denied the request |
| `Retry-After` | Seconds until capacity (omitted for `concurrency`/`global_concurrency` rejects — finish/cancel in-flight instead) |

Recommended: exponential backoff with jitter
[rate-limits](https://docs.aiand.com/reference/rate-limits/). Exact tier
thresholds: **unverified** (the "Tiers" section content was not captured).

## Error shapes

The dedicated "Error Codes" page appears in the docs nav but its URL could not
be retrieved (`/reference/error-codes/` and variants returned 404) —
**unverified as a standalone page**. Error behavior gathered from
cross-referenced pages:

| Status | Meaning / trigger | Source |
|---|---|---|
| `400` | `model_capability_mismatch` — e.g. vision content sent to a non-vision model | [chat-completions](https://docs.aiand.com/api/chat-completions/) |
| `400` | Unmatched `model` value — lists `GET /v1/models` as source of valid names | [models](https://docs.aiand.com/api/models/) |
| `400` | `reasoning_effort` value the model doesn't accept | [chat-completions](https://docs.aiand.com/api/chat-completions/) · [catalog](https://docs.aiand.com/models/catalog/) |
| `401` | Missing or invalid credentials | [authentication](https://docs.aiand.com/authentication/) |
| `402` | Insufficient credits in the billing account | [authentication](https://docs.aiand.com/authentication/) |
| `403` | Valid credentials but insufficient permissions | [authentication](https://docs.aiand.com/authentication/) |
| `404` | `previous_response_id` from another user/org or non-existent | [responses](https://docs.aiand.com/api/responses/) |
| `429` | Rate-limited (see headers above) | [rate-limits](https://docs.aiand.com/reference/rate-limits/) |

The full error response body format is documented on the (unretrievable)
"Error Codes" page; the auth page confirms these status meanings
[authentication](https://docs.aiand.com/authentication/).

## SDK compatibility

Confirmed drop-in for both major SDKs with a `base_url` override:

**OpenAI SDK** — wire-compatible with Chat Completions, Responses, and Models
APIs. Change base URL + API key; streaming, tool calls, structured outputs
work as-is [migrating-openai](https://docs.aiand.com/migrating-openai/) ·
[openai-sdk](https://docs.aiand.com/sdks/openai/).

```python
from openai import OpenAI
client = OpenAI(
    base_url="https://api.aiand.com/v1",
    api_key="sk-your-aiand-api-key",
)
```

**Anthropic SDK** — the `anthropic-version` header the SDK sends tells ai& to
return Anthropic-shape payloads. Same key/org/billing as the OpenAI surface
[migrating-anthropic](https://docs.aiand.com/migrating-anthropic/).

```python
from anthropic import Anthropic
client = Anthropic(
    base_url="https://api.aiand.com/v1",
    api_key="sk-your-aiand-api-key",
)
```

Both SDKs can be used in the same project against ai&, hitting different
endpoints but sharing the key, org, and billing
[migrating-anthropic](https://docs.aiand.com/migrating-anthropic/).

## Integration notes for this router

Per `internal/providers/CLAUDE.md` and the AGENTS.md "How do I add new
OpenAI-compatible upstream?" recipe, ai& is an **OpenAI-compatible upstream**
— no new adapter package. Integration steps:

1. **Provider constant.** Add `providers.ProviderAiand` ("aiand") to
   `internal/providers/provider.go` alongside the existing
   `ProviderOpenRouter` / `ProviderFireworks` constants.
2. **Base URL + env var.** Add an `*BaseURL` constant
   (`https://api.aiand.com/v1`) and env-var entry (`AIAND_API_KEY`) to
   `internal/providers/openaicompat`, mirroring the OpenRouter pattern.
3. **Registration.** Add a registration block in `cmd/router/main.go` that
   wires the `openaicompat` client with the ai& base URL + key.
4. **Catalog.** Add the model IDs above to `internal/router/catalog/` with
   tier + provider bindings + pricing from `GET /v1/models`. Send **exact
   catalog IDs** (e.g. `zai-org/glm-5.2`), not bare aliases, to avoid the
   prefix-resolution ambiguity.
5. **Auth header.** Standard `Authorization: Bearer <key>` — already handled
   by the `openaicompat` client. No special headers required (the
   `X-Aiand-Metrics` / `X-Org-ID` headers are opt-in and not needed for
   proxying).
6. **Streaming.** The OpenAI SSE path (`internal/sse` + `internal/proxy`)
   works unmodified; the `metrics` trailer event after `[DONE]` can be
   ignored (cost is reconciled from `usage`).
7. **Anthropic surface.** Optional: ai& also speaks `/v1/messages` natively
   (Anthropic shape, internally translated). If we want to route Anthropic
   clients to ai& open-weights without our own translation, we could add an
   Anthropic-compatible client path — but the router already translates
   Anthropic↔OpenAI, so the OpenAI surface is sufficient.

### Quirks to encode

- **Model ID namespace** is `<lab>/<model>` (e.g. `zai-org/glm-5.2`), not bare
  names. Catalog IDs use `owner/` prefixes from the upstream open-weight lab.
  [catalog](https://docs.aiand.com/models/catalog/)
- **Prefix resolution** means a bare `glm` resolves to the highest-priority GLM
  version — pin exact IDs in the catalog to avoid drift
  [models](https://docs.aiand.com/api/models/).
- **`thinking` block unsupported** on the Anthropic surface — `budget_tokens`
  / `type` don't map to ai&'s effort scale; reasoning falls back to the
  model's default [messages](https://docs.aiand.com/api/messages/).
- **`reasoning_effort`** is per-model (validated against
  `reasoning_efforts` from `GET /v1/models`); an unsupported value → `400`
  [catalog](https://docs.aiand.com/models/catalog/).
- **`/v1/messages` is translated**, not native Anthropic — request → OpenAI
  format → inference → Anthropic format. Parameters with no equivalent are
  silently ignored [messages](https://docs.aiand.com/api/messages/).
- **Cached-token billing** (`cached_tokens` field) only on models that support
  it; affects cost math if we reconcile from `usage`
  [pricing](https://docs.aiand.com/models/pricing/).

## Cited sources (primary)

All factual claims above trace to these first-party ai& sources:

| Source URL | What it verifies |
|---|---|
| https://docs.aiand.com/ | Overview, base URL, endpoint list, "OpenAI- and Anthropic-compatible" |
| https://docs.aiand.com/getting-started/ | Key creation, $1 min top-up, first-call examples, streaming quickstart |
| https://docs.aiand.com/authentication/ | Bearer auth scheme, key format, 401/402/403 meanings, JWT method |
| https://docs.aiand.com/api/chat-completions/ | `/v1/chat/completions` params, usage, `stream:true`, `400 model_capability_mismatch` |
| https://docs.aiand.com/api/messages/ | `/v1/messages` Anthropic shape, internal translation, `thinking` unsupported |
| https://docs.aiand.com/api/responses/ | `/v1/responses` params, `404` on cross-org response_id |
| https://docs.aiand.com/api/models/ | `GET /v1/models`, model object fields, prefix name resolution, cost formula |
| https://docs.aiand.com/models/catalog/ | Exact model IDs, capabilities, context, pricing, `reasoning_efforts`, Anthropic surface |
| https://docs.aiand.com/models/pricing/ | Per-token billing, prompt caching rules, `X-Aiand-Metrics` reporting |
| https://docs.aiand.com/capabilities/streaming/ | SSE streaming, OpenAI vs Anthropic event shapes, metrics trailer |
| https://docs.aiand.com/reference/rate-limits/ | Per-org tiers, rate-limit buckets, `429` headers, backoff |
| https://docs.aiand.com/reference/response-headers/ | `X-Request-ID`, opt-in cost/timing headers |
| https://docs.aiand.com/reference/streaming-events/ | `metrics` trailer event semantics |
| https://docs.aiand.com/migrating-openai/ | OpenAI wire-compatibility, supported endpoints, `X-Org-ID` |
| https://docs.aiand.com/migrating-anthropic/ | Anthropic SDK base_url override, `anthropic-version` shape selection, `sk-` keys |
| https://docs.aiand.com/sdks/openai/ | OpenAI SDK drop-in setup |
| https://www.aiand.com/about | Operator (ai& Inc.), founders, funding, Japan incorporation |
| https://www.aiand.com/inference | Frontier models, Japan-hosted/data-resident, cached input pricing |

## Gaps / unverified

- **Error Codes page** — listed in the docs nav ("Reference → Error Codes")
  but the page URL could not be retrieved (`/reference/error-codes/` and
  variants 404). Error status meanings are covered by cross-references
  (auth, chat-completions, models, responses, rate-limits), but the **full
  error response body format** is unverified from a primary page.
- **Rate-limit tier thresholds** — the "Tiers" section content on
  `/reference/rate-limits/` was not captured; only the bucket names and header
  semantics are verified. Exact numeric limits: **unverified**.
- **GLM-5.2 standard input pricing** — marketing page (`$0.80`) and catalog
  (`$1.00`) disagree. `GET /v1/models` / catalog treated as authoritative;
  reconcile at integration time.
- **No Llama/Mistral** in the current catalog — confirmed absent as of this
  research, but the catalog is dynamic ("each organization sees only what it
  has access to"), so availability may differ per org.
- **Gemini-native surface** — no `/v1beta/models/:action` endpoint is
  documented; ai& exposes OpenAI + Anthropic shapes only.

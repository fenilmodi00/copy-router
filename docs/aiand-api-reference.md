# AIand API Reference — Data Available via API Key

> **Source**: Comprehensive crawl of `https://docs.aiand.com` (firecrawl) + Tavily deep research (2026-08-26).
> 
> This document catalogs all API endpoints, data structures, capabilities, and pricing available when authenticating with an `AIAND_API_KEY`. Use this to plan dashboard features and router integration.

---

## 1. Base URLs

| Purpose | URL |
|---------|-----|
| API endpoint base | `https://api.aiand.com` |
| OpenAI-compatible chat | `https://api.aiand.com/v1/chat/completions` |
| Responses API | `https://api.aiand.com/v1/responses` |
| Anthropic Messages (compatible) | `https://api.aiand.com/v1/messages` |
| Model catalog listing | `https://api.aiand.com/v1/models` |
| File/Upload API | `https://api.aiand.com/v1/files` |
| Key management | `https://api.aiand.com/api/v1/keys` |
| Analytics | `https://api.aiand.com/analytics/metrics` |

---

## 2. Authentication

### API Key (Recommended for programmatic access)

- **Header**: `Authorization: Bearer sk-<api-key>`
- **Key format**: `sk-` prefix, shown **once** at creation — must be stored securely
- **Organization scope**: Key auto-resolves associated billing account
- **Cannot be retrieved again** after creation page is closed

### JWT Session Auth (browser-based only)

- Access token: 15-min TTL as `access_token` cookie
- Refresh token: 30-day TTL as `refresh_token` cookie
- Requires `X-Org-ID` header for organization context

> **Note**: For dashboard/routing use, API key auth is preferred. JWT is for web console only.

---

## 3. All API Endpoints (601+ unique URLs)

### 3.1 Chat Completions (`POST /v1/chat/completions`)

**Primary inference endpoint — OpenAI-compatible**

| Parameter | Type | Description |
|-----------|------|-------------|
| `model` | string | Model ID from catalog (e.g., `deepseek-ai/deepseek-v4-flash`) |
| `messages` | array | Message array with text, image, video, file references |
| `stream` | boolean | Enable SSE streaming |
| `temperature` | number | Sampling temperature (0-2) |
| `top_p` | number | Nucleus sampling (0-1) |
| `max_completion_tokens` | integer | Max tokens in response |
| `stop` | array[string] | Stop sequences |
| `frequency_penalty` | number | Frequency penalty (-2 to 2) |
| `presence_penalty` | number | Presence penalty (-2 to 2) |
| `logprobs` | boolean | Return log probabilities |
| `top_logprobs` | integer | Number of top logprobs to return |
| `logit_bias` | object | Token bias modifications |
| `response_format` | object | Structured output format |
| `parallel_tool_calls` | boolean | Call tools in parallel |
| `tool_choice` | string \| enum | Tool selection mode |
| `tools` | array | Tool definitions for function calling |
| `reasoning_effort` | string | `low` \| `medium` \| `high` (model-dependent) |
| `top_k` | integer | Top-k sampling |
| `min_p` | number | Min-p sampling |
| `repetition_penalty` | number | Repetition penalty |
| `user` | string | User ID for tracing |

**Messages format:**

| Role | Content Types |
|------|--------------|
| `text` | `{"type": "text", "text": "..."}` |
| `image` | `{"type": "image_url", "image_url": {"url": "..."}}` (via `file_id`) |
| `video` | `{"type": "video_url", "video_url": {"url": "..."}}` (via `file_id`) |
| `audio` | `{"type": "input_audio", "input_audio": {"transcript": "..."}}` |
| `file` | `{"type": "file", "file_id": "..."}` (references uploaded file) |

**Non-streaming response:**

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "deepseek-ai/deepseek-v4-flash",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "..."},
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 150,
    "completion_tokens": 230,
    "total_tokens": 380
  }
}
```

**Streaming**: SSE format with `data: {...}` events, final `data: [DONE]`. Optional `event: metrics` trailer with token counts + cost + currency.

---

### 3.2 Responses API (`POST /v1/responses`)

**OpenAI Responses API-compatible endpoint**

| Parameter | Type | Description |
|-----------|------|-------------|
| `model` | string | Model ID |
| `input` | string \| array | Input text or array of items |
| `instructions` | string | System instructions |
| `stream` | boolean | Enable streaming |
| `temperature` | number | Sampling temperature |
| `top_p` | number | Nucleus sampling |
| `max_output_tokens` | integer | Max tokens in response |
| `tools` | array | Tool definitions |
| `tool_choice` | string \| enum | Tool selection |
| `parallel_tool_calls` | boolean | Parallel tool calls |
| `reasoning` | object \| boolean | Enable reasoning (model-dependent) |
| `truncation` | object | Truncation strategy |
| `previous_response_id` | string | Continue previous response |
| `store` | boolean | Store in memory |
| `metadata` | object | Additional metadata |
| `seed` | integer | Random seed |
| `stop` | string \| array | Stop sequences |
| `top_k` | integer | Top-k sampling |
| `repetition_penalty` | number | Repetition penalty |

**Input item types:**

| Type | Format |
|------|--------|
| `input_text` | Simple string or array of content blocks |
| `input_image` | `{"image_url": {"url": "..."}}` (via `file_id`) |
| `input_file` | `{"file_id": "..."}` (resolves uploaded file) |

**Response:**

```json
{
  "id": "resp-...",
  "object": "response",
  "status": "completed",  // completed/failed/in_progress/incomplete/cancelled/queued
  "model": "deepseek-ai/deepseek-v4-flash",
  "output": [{"role": "assistant", "content": [{"type": "text", "text": "..."}]}],
  "usage": {
    "input_tokens": 150,
    "output_tokens": 230,
    "total_tokens": 380
  }
}
```

**Streaming**: Same SSE format with optional `event: metrics` trailer.

---

### 3.3 Model Catalog (`GET /v1/models`)

Lists all available models with full metadata:

**Response format:**

```json
{
  "object": "list",
  "data": [{
    "id": "deepseek-ai/deepseek-v4-flash",
    "object": "model",
    "created": 1725784500,
    "owned_by": "ai&",
    "provider": "deepseek-ai",
    "context_window": 1000000,
    "capabilities": ["reasoning", "tool_calling"],
    "reasoning_efforts": ["low", "medium", "high"],
    "reasoning_effort_default": "medium",
    "currency": "usd",
    "input_per_1m": "0.15",
    "output_per_1m": "0.25"
  }, { ... next model ... }]
}
```

**All 9 current models (from live catalog):**

| Model ID | Provider | Context | Capabilities | Input/1M | Output/1M | Notes |
|---|---|---|---|---|---|---|
| `deepseek-ai/deepseek-v4-flash` | deepseek-ai | 1M | reasoning, tool_calling | $0.15 | $0.25 | Flash tier |
| `openai/gpt-oss-120b` | openai | 131K | reasoning, tool_calling | $0.15 | $0.60 | Open-weight |
| `google/gemma-4-31b-it` | google | 262K | reasoning, tool_calling, **vision**, **video**, document | $0.20 | $0.50 | Full multimodal |
| `qwen/qwen3.6-27b` | qwen | 262K | reasoning, tool_calling, **vision**, video, document | high | high | 262K context |
| `motif-technologies/motif-3` | Motif-Technologies | 262K | chat, reasoning, tool_calling | $0.50 | $2.00 | MoE 13.2B active |
| `moonshotai/kimi-k2.7-code` | moonshotai | 262K | reasoning, tool_calling, **vision**, document | $0.75 | $3.50 | Code + vision |
| `moonshotai/kimi-k3` | moonshotai | 1M | reasoning, tool_calling, vision, document | $3.00 | $12.50 | Frontier, 1M ctx |
| `zai-org/glm-5.2` | zai-org | 1M | reasoning, tool_calling | $1.00 | $4.00 | High-capability |
| `zai-org/glm-5.1` | zai-org | 203K | reasoning, tool_calling | $1.40 | $4.40 | Predecessor |

**Capabilities matrix (per model):**

| Capability | deepseek-v4-flash | gpt-oss-120b | gemma-4-31b-it | qwen3.6-27b | motif-3 | kimi-k2.7-code | kimi-k3 | glm-5.2 | glm-5.1 |
|---|---|---|---|---|---|---|---|---|---|
| `reasoning` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `tool_calling` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `vision` | ❌ | ❌ | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ |
| `video` | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ |
| `document` | ❌ | ❌ | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ |

---

### 3.4 File Upload API (`POST /v1/files`)

**Upload files for vision/video support**

| Parameter | Type | Description |
|-----------|------|-------------|
| `file` | file | Binary file content ( multipart/form-data ) |
| `purpose` | string | `vision` or `video` |

**Response:**

```json
{
  "id": "file-...",
  "object": "file",
  "bytes": 12345,
  "purpose": "vision",
  "filename": "example.png",
  "created": 1725784500
}
```

**Usage**: Reference by `file_id` in `messages[].file` or `input.file` fields.

---

### 3.5 Upload Chunks (`POST /api/uploads`)

**Chunked upload for large files (up to 8GB)**

| Parameter | Type | Description |
|-----------|------|-------------|
| `file` | binary | Chunked file data |
| `purpose` | string | `vision` or `video` |

**Responses**: Returns upload session with `upload_id`, supports resumable upload.

---

### 3.6 API Key Management (`GET/POST/PATCH/DELETE /api/v1/keys`)

**Organization-scoped key management** (requires JWT auth + `X-Org-ID`)

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/keys` | List all keys for organization |
| `POST /api/v1/keys` | Create new API key (shown once) |
| `PATCH /api/v1/keys/{id}` | Rotate/revoke key |
| `DELETE /api/v1/keys/{id}` | Delete key |

**Response (create):**

```json
{
  "id": "key-...",
  "organization_id": "org-...",
  "key": "sk-live-...",  // Shown once only
  "created_at": "2026-08-26T...",
  "expires_at": null  // or timestamp
}
```

---

### 3.7 Analytics & Metrics (`GET /analytics/metrics`)

**Usage and cost querying** (API key auth)

| Parameter | Type | Description |
|-----------|------|-------------|
| `range` | string | Time range: `24h` \| `7d` \| `30d` \| `custom` |
| `start_time` | integer | Unix timestamp (for custom range) |
| `end_time` | integer | Unix timestamp (for custom range) |
| `model` | string | Filter by model ID |
| `granularity` | string | `hour` \| `day` \| `week` |

**Response:**

```json
{
  "data": [{
    "period": "2026-08-25T00:00:00Z",
    "requests": 1250,
    "input_tokens": 150000,
    "output_tokens": 75000,
    "cost": 22.50,
    "currency": "usd",
    "errors": 3,
    "average_latency_ms": 850
  }, { ... next period ... }]
}
```

---

### 3.8 User/Organization Context (`GET /api/v1/me`)

**Get current user/organization context** (API key auth)

```json
{
  "id": "user-...",
  "organization_id": "org-...",
  "plan": "tier_0" | "tier_1",
  "endpoint": "https://api.aiand.com",
  "currency": "usd"
}
```

---

### 3.9 Rate Limit Headers (automatic on all responses)

| Header | Meaning |
|--------|---------|
| `X-RateLimit-Limit` | Effective RPM cap for this request |
| `X-RateLimit-Remaining` | Requests remaining in current window |
| `X-RateLimit-Reset` | Epoch seconds when limit resets |
| `429` response specific: | |
| `X-RateLimit-Policy` | Which bucket was exhausted (`rpm`, `global_rpm`, `input_tpm`, `output_tpm`, `concurrency`, `global_concurrency`) |
| `Retry-After` | Seconds to wait before retry |

---

## 4. Error Codes

| Status | Meaning |
|--------|---------|
| `400` | Bad request — model capability mismatch, invalid params |
| `401` | Missing or invalid API key |
| `402` | Insufficient credits / quota exceeded |
| `403` | Valid key but insufficient permissions |
| `429` | Rate limited — see `X-RateLimit-Policy` + `Retry-After` |
| `500` | Server error |
| `503` | Service unavailable |

**Error response format:**

```json
{
  "type": "error",
  "message": "Human readable error description",
  "param": "optional field name",
  "code": "machine-readable code"
}
```

---

## 5. Prompt Caching (Feature)

- **Automatic**: System detects repeated prompt prefixes
- **1024+ token prefix**: Counted in 128-token increments
- **10-minute reuse window**: Same prefix reused within 10 minutes gets cached rate
- **Only models with cached input rate**: Report `cached_tokens` in usage
- **Cost savings**: Cached input tokens charged at reduced rate (model-dependent)

---

## 6. Pricing Formula

```
 cost = (input_tokens / 1_000_000 × input_per_1m) + (output_tokens / 1_000_000 × output_per_1m)
```

- Prices in USD per 1M tokens (strings for precision)
- Credits deducted after **successful** requests only
- Failed requests (4xx/5xx) are **not billed**
- Currency can be `usd` or `jpy` (set at org creation)
- Approximate JPY rate: ¥160 ≈ $1 USD

---

## 7. Tiers

| Tier | Description |
|------|-------------|
| **Tier 0** | Evaluation tier, lower per-minute caps, suitable for development |
| **Tier 1** | Production tier, higher caps, access to higher-throughput models (promoted after first successful payment) |

Tier is visible via `GET /api/v1/me` (`plan` field) and affects:
- Per-model RPM/TPM caps
- Concurrent request limits
- Access to certain models

---

## 8. Feature Matrix for Dashboard Planning

Based on the full API surface, these are the key features to consider for the router/dashboard:

| Feature | API Endpoint | Data Available | Priority |
|---|---|---|---|
| **Model catalog browsing** | `GET /v1/models` | Full model list with pricing, capabilities, context windows | 🔴 High |
| **Real-time usage metrics** | `GET /analytics/metrics` | Per-period requests, tokens, cost, errors, latency | 🔴 High |
| **API key management** | `GET/POST/PATCH/DELETE /api/v1/keys` | Key creation, list, rotation, deletion | 🟡 Medium |
| **User/org context** | `GET /api/v1/me` | Plan tier, currency, organization info | 🟡 Medium |
| **Prompt caching status** | Usage reporting | Cached token counts, automatic detection | 🟡 Medium |
| **Rate limit monitoring** | Response headers | `X-RateLimit-Limit`, `Remaining`, `Policy` | 🟢 Low |
| **File/video upload** | `POST /v1/files` / `POST /api/uploads` | Upload files for vision/video support | 🟢 Low |
| **Streaming metrics** | SSE `event: metrics` | Per-request token counts + cost during streaming | 🟢 Low |
| **Historical trend analysis** | `/analytics/metrics` with ranges | Time-series cost/token trends | 🟢 Low |
| **Model capability filtering** | Catalog + request validation | Which models support vision, tool_calling, reasoning | 🟢 Low |

---

## 9. Integration Notes for Router

### Import Rules (from CLAUDE.md/AGENTS.md)

- **No cross-layer imports**: `internal/api/*` may import `internal/auth`, `internal/proxy`, `internal/router` — but NOT `internal/postgres` or concrete provider adapters
- **Catalog data** lives in `internal/router/catalog/` — use as single source of truth
- **Model names** must use named constants from `internal/providers` (no magic strings)
- **Pricing math** should use `internal/timing` value types for per-request latency stamps

### Recommended Router Integration

1. **Catalog ingestion**: Periodically fetch `GET /v1/models` and update `internal/router/catalog/` model rows
2. **Pricing lookup**: Map model IDs → `input_per_1m` / `output_per_1m` for cost calculation
3. **Capability gating**: Check `capabilities` field before routing to model
4. **Usage tracking**: Post-inference, call `GET /analytics/metrics` or use SSE `event: metrics` for per-request cost
5. **Key rotation**: Dashboard UI should call `POST /api/v1/keys` for key management

### API Key Handling in Dashboard

- Store `sk-` prefixed keys securely (never expose full token in logs)
- Use 8-char prefix + 4-char suffix for display (`KeyPrefix`/`KeySuffix` pattern)
- Keys are organization-scoped — no need for separate `X-Org-ID` with API key auth
- Rotate/revoke via `/api/v1/keys` endpoints when needed

---

## 10. Quick Reference: All Model IDs

```
deepseek-ai/deepseek-v4-flash
openai/gpt-oss-120b
google/gemma-4-31b-it
qwen/qwen3.6-27b
motif-technologies/motif-3
moonshotai/kimi-k2.7-code
moonshotai/kimi-k3
zai-org/glm-5.2
zai-org/glm-5.1
```

---

*Document generated 2026-08-26 from live AIand API documentation crawl and Tavily deep research.*
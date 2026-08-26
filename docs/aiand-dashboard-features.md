# AIand Dashboard Feature Planning

> Based on comprehensive API documentation research (2026-08-26) from:
> - Firecrawl crawl of `https://docs.aiand.com` (100 pages, 601+ endpoints)
> - Tavily deep research on `aiand.com API documentation`

This document lists all data and endpoints available via `AIAND_API_KEY` that can inform dashboard feature planning for the router.

---

## 🔴 HIGH PRIORITY — Core Routing & Cost Features

### 1. Model Catalog Browsing
**Endpoint**: `GET /v1/models`

**Data available**:
- Full model list (9 current models) with:
  - Model ID (e.g., `deepseek-ai/deepseek-v4-flash`)
  - Provider (`deepseek-ai`, `openai`, `google`, `qwen`, `Motif-Technologies`, `moonshotai`, `zai-org`)
  - Context window size (131K - 1M tokens)
  - Capabilities array (reasoning, tool_calling, vision, video, document)
  - Reasoning efforts menu (none/low/high/medium/max)
  - Default reasoning effort per model
  - Pricing: `input_per_1m`, `output_per_1m` (USD per 1M tokens)
  - Currency per organization (`usd` or `jpy`)
  - Prompt caching support (`cached_input_per_1m`)

**Dashboard use**:
- Model selector/selector with capability filtering
- Pricing display per model
- Context window visualization
- Capability badges (vision, video, document, tool_calling, reasoning)

### 2. Real-time Usage Metrics
**Endpoint**: `GET /analytics/metrics`

**Parameters**:
- `range`: `24h` \| `7d` \| `30d` \| `custom`
- `start_time` / `end_time` (Unix timestamps, for custom range)
- `model`: filter by model ID
- `granularity`: `hour` \| `day` \| `week`

**Data returned per period**:
- Request count
- Input tokens consumed
- Output tokens consumed
- Cost in USD (with currency)
- Error count
- Average latency (ms)

**Dashboard use**:
- Cost trends over time
- Token usage breakdown (input vs output)
- Error rate monitoring
- Latency tracking per model
- Per-model performance charts

### 3. API Key Management
**Endpoints**: `GET/POST/PATCH/DELETE /api/v1/keys`

**Data available**:
- List all organization API keys
- Create new keys (shown once at creation)
- Key rotation/revocation
- Key metadata (organization_id, created_at, expires_at)

**Dashboard use**:
- Key inventory display
- Key rotation UI
- Expiration warnings
- Key usage statistics per key

### 4. Organization Context
**Endpoint**: `GET /api/v1/me`

**Data returned**:
- User/organization ID
- Plan tier (`tier_0` evaluation or `tier_1` production)
- Currency setting (`usd` or `jpy`)
- API endpoint base URL

**Dashboard use**:
- Plan tier display (upgrade/downgrade CTAs)
- Currency selector
- Account status overview

---

## 🟡 MEDIUM PRIORITY — Advanced Features

### 5. Prompt Caching Status
**Data**: Automatic detection via usage reporting

**How it works**:
- System detects repeated prompt prefixes automatically
- 1024+ token prefix counted in 128-token increments
- 10-minute reuse window for cached prompts
- Only models with cached input rate report `cached_tokens` in usage

**Dashboard use**:
- Cache hit ratio display
- Cost savings from caching
- Cache window visualizer
- Model-specific cache status

### 6. Rate Limit Monitoring
**Headers on all responses**:
- `X-RateLimit-Limit`: Effective RPM cap
- `X-RateLimit-Remaining`: Requests remaining
- `X-RateLimit-Reset`: Epoch seconds when limit resets
- `429` responses include:
  - `X-RateLimit-Policy`: Which bucket was exhausted
  - `Retry-After`: Seconds to wait before retry

**Dashboard use**:
- Real-time rate limit status
- Historical rate limit violations
- Per-bucket capacity visualization
- Recommended backoff times

### 7. File/Video Upload Status
**Endpoints**: `POST /v1/files`, `POST /api/uploads`

**Data available**:
- File upload for vision/video support
- Purposes: `vision` or `video`
- Max file size: 8GB (chunked upload)
- Supported MIME types: png, jpeg, webp, gif, mp4, webm, quicktime

**Dashboard use**:
- Upload progress tracking
- Stored files list
- Vision/video capabilities per model
- File reference management

### 8. Streaming Metrics
**SSE format**: `event: metrics` trailer after terminal message

**Data in metrics event**:
- Prompt token count
- Completion token count
- Total token count
- Cost in currency
- Model ID

**Dashboard use**:
- Real-time token counting during streaming
- Live cost accrual display
- Per-token cost breakdown

---

## 🟢 LOW PRIORITY — Peripheral Features

### 9. Model Capability Filtering
**Data**: From `GET /v1/models` `capabilities` field

**Capabilities per model** (from live catalog):

| Model | reasoning | tool_calling | vision | video | document |
|-------|-----------|-------------|--------|-------|----------|
| deepseek-v4-flash | ✅ | ✅ | ❌ | ❌ | ❌ |
| gpt-oss-120b | ✅ | ✅ | ❌ | ❌ | ❌ |
| gemma-4-31b-it | ✅ | ✅ | ✅ | ✅ | ✅ |
| qwen3.6-27b | ✅ | ✅ | ✅ | ✅ | ✅ |
| motif-3 | ✅ | ✅ | ❌ | ❌ | ❌ |
| kimi-k2.7-code | ✅ | ✅ | ✅ | ❌ | ✅ |
| kimi-k3 | ✅ | ✅ | ✅ | ✅ | ✅ |
| glm-5.2 | ✅ | ✅ | ❌ | ❌ | ❌ |
| glm-5.1 | ✅ | ✅ | ❌ | ❌ | ❌ |

**Dashboard use**:
- Capability-aware model filtering
- Feature comparison tables
- UI disable/enable based on model capabilities

### 10. Error & Success Tracking
**Endpoints**: Usage metrics + error codes

**Error codes mapping**:
- `400`: Model capability mismatch, invalid params
- `401`: Missing/invalid API key
- `402`: Insufficient credits/quota
- `403`: Valid key but insufficient permissions
- `429`: Rate limited
- `500`: Server error
- `503`: Service unavailable

**Dashboard use**:
- Error rate per error type
- 402 (quota) tracking for upgrade prompts
- 429 (rate limit) pattern analysis
- Success/failure ratios per model

### 11. Tier Awareness
**Data**: From `GET /api/v1/me` (`plan` field) and catalog tier assignments

**Tier assignments** (from catalog):
- `TierLow`: qwen3.6-27b, gpt-oss-120b, gemma-4-31b-it, glm-5.1, glm-5.2
- `TierMid`: deepseek-v4-pro, motif-3
- `TierHigh`: deepseek-v4-flash, kimi-k2.7-code, kimi-k3

**Dashboard use**:
- Tier-gated feature access
- Upgrade prompts from Tier0 → Tier1
- Model availability per tier

### 12. Cost Calculation Reference
**Formula**: `cost = (input_tokens / 1_000_000 × input_per_1m) + (output_tokens / 1_000_000 × output_per_1m)`

**Per-model pricing** (USD per 1M tokens):
| Model | Input | Output |
|-------|-------|--------|
| qwen3.6-27b | Free | Free |
| deepseek-v4-flash | $0.15 | $0.25 |
| gpt-oss-120b | $0.15 | $0.60 |
| gemma-4-31b-it | $0.20 | $0.50 |
| motif-3 | $0.50 | $2.00 |
| kimi-k2.7-code | $0.75 | $3.50 |
| kimi-k3 | $3.00 | $12.50 |
| glm-5.2 | $1.00 | $4.00 |
| glm-5.1 | $1.40 | $4.40 |

**Dashboard use**:
- Per-request cost estimation
- Budget tracking and alerts
- Cost breakdown (input vs output)
- Prompt caching cost reduction display

---

## 📊 Data Flow Summary for Dashboard

```
API Key Auth (AIAND_API_KEY)
      │
      ▼
┌─────────────────┐
│ GET /v1/models  │──→ Model catalog (9 models + pricing/capabilities)
│ GET /analytics/metrics │──→ Usage trends (tokens, cost, errors, latency)
│ GET /api/v1/me  │──→ Org context (tier, currency)
│ POST/PATCH/DELETE /api/v1/keys │──→ Key management
└─────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────┐
│ Dashboard Components:                               │
│ • Model selector with capability filters            │
│ • Cost/trend charts (input/output tokens)           │
│ • Key management UI                                 │
│ • Plan tier display & upgrade CTAs                 │
│ • Rate limit status indicators                      │
│ • Prompt cache hit ratio                            │
│ • Per-model pricing breakdown                       │
│ • Streaming live metrics (if enabled)               │
└─────────────────────────────────────────────────────┘
```

---

## 🛠️ Integration Notes for Implementation

### Code Layering (per CLAUDE.md/AGENTS.md)

- **`internal/api/*`** can import: `internal/auth`, `internal/proxy`, `internal/router`, `internal/observability`
- **`internal/router/catalog/`** is the single source of truth for model data — use it for pricing/capability lookups
- **No cross-layer imports**: Don't import `internal/postgres` or concrete provider adapters from API handlers
- **Model names** must use `providers.ProviderAiand` constant (no magic strings)
- **Pricing math** flows through `catalog.PriceFor(provider, id)` — do not duplicate

### Recommended Implementation Pattern

1. **Periodic catalog refresh**: Background job that fetches `GET /v1/models` and updates `internal/router/catalog/Models`
2. **Usage aggregation**: Post-inference, tally tokens + call `GET /analytics/metrics` or use SSE `event: metrics` for per-request cost
3. **Key rotation UI**: Dashboard calls `POST /api/v1/keys` for key creation, `PATCH` for rotation
4. **Tier-aware gating**: Check `catalog.TierFor(id)` + `GET /api/v1/me` plan tier for feature access

### API Key Security in Dashboard

- Display 8-char prefix + 4-char suffix only (never full token)
- Use `KeyPrefix`/`KeySuffix` pattern from auth package
- Keys shown once at creation — store securely, cannot retrieve later
- Rotation via `/api/v1/keys` endpoints only
- Never log raw bearer tokens or full API keys

---

*Document generated 2026-08-26 from live AIand API documentation research.*
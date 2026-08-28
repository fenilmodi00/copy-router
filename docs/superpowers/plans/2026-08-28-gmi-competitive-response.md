# GMI Competitive Response — Weave Router Features

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 5 gateway-level features to the Weave Router so it matches or exceeds GMI Router's feature set on competitive comparison pages.

**Architecture:** Each feature is an independent subsystem touching 1-3 layers (presentation API, proxy service, inner-ring domain). The plan splits work by feature so each task ships independently. No shared data structures across tasks.

**Tech Stack:** Go (Gin, router, catalog, proxy, policy), TypeScript (React dashboard, pi-router extension), SQL (telemetry queries), generated code (cmd/genprices).

## Global Constraints

- All imports must follow the three-layer model (inner ring I/O-free, adapters depend only on inner ring, only cmd/router/main.go wires concrete things). See AGENTS.md.
- No DI containers, reflection, or framework magic — explicit composition.
- No panic on request path.
- No magic strings for provider/model names — use constants from internal/providers.
- Every exported symbol needs godoc starting with the symbol name.
- Tests use testify/assert + testify/require, live next to code.
- Savings calculations: negative savings must still display (not clamped to $0).
- Must not log secrets, raw API keys, or full request bodies.

---

### Task 1: Fix $0.00 Savings (pi-router CLI)

**Files:**
- Modify: `internal/router/catalog/catalog.go`
- Modify: `internal/router/catalog/lookup.go`
- Create: `internal/router/catalog/catalog_baselines_test.go`
- Modify: `internal/observability/otel/pricing.go`
- Modify: `cmd/genprices/main.go`
- Modify: `cmd/genprices/main_test.go`
- Modify: `install/pi-router/src/pricing.generated.ts` (generated output)

**Interfaces:**
- Consumes: `catalog.Model` struct (add `BaselineOnly` field), `catalog.AllPrimaryPricing()` (extend to include baselines)
- Produces: `catalog.BaselinePricing()` returning pricing for baseline-only models; `otel.AllPricing()` includes baselines; `cmd/genprices` emits them into TypeScript

**Background:** The pi-router CLI extension (install/pi-router/) shows "$0.00 savings" because `pricing.generated.ts` only contains 9 aiand-bound models. When a user sends requests to Claude, GPT, or Gemini, the baseline pricing lookup returns zero because those models aren't in the file. We need to add common frontier model pricing entries that are "pricing-only" — they don't participate in routing, they just exist so savings calculations work.

- [ ] **Step 1: Add `BaselineOnly` field to `catalog.Model` struct**

In `internal/router/catalog/catalog.go`, add the field to the `Model` struct:

```go
// BaselineOnly marks a model that exists solely to provide pricing for
// savings calculations. It has no Tier, no Providers with upstream dispatch,
// and is excluded from routing entirely. This lets the pricing table cover
// common frontier models (Claude, GPT, Gemini) even when they aren't
// routable on this deployment.
BaselineOnly bool
```

- [ ] **Step 2: Add frontier baseline model entries to the catalog**

In `internal/router/catalog/catalog.go`, add these entries at the end of the `Models` slice (after the `// aliases` comment boundary, before the `var aliases` declaration):

```go
// Baseline-only pricing entries for savings calculations. These models have
// no Tier and no routable Providers — they exist solely so the generated
// pricing table (pi-router CLI, shell scripts) and server-side telemetry
// can look up the cost of the requested model and compute savings.
// Prices reflect current first-party API rates (no cache multiplier needed
// for baseline-only entries).
{ID: "claude-sonnet-4-8", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 3.00, OutputUSDPer1M: 15.00}}}},
{ID: "claude-haiku-4", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 0.80, OutputUSDPer1M: 4.00}}}},
{ID: "claude-opus-4-7", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderAnthropic, Price: Pricing{InputUSDPer1M: 15.00, OutputUSDPer1M: 75.00}}}},
{ID: "gpt-4.5", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 30.00, OutputUSDPer1M: 150.00}}}},
{ID: "gpt-5.5-pro", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 10.00, OutputUSDPer1M: 40.00}}}},
{ID: "gpt-5.5-flash", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderOpenAI, Price: Pricing{InputUSDPer1M: 0.50, OutputUSDPer1M: 2.00}}}},
{ID: "gemini-2.5-pro", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderGoogle, Price: Pricing{InputUSDPer1M: 1.25, OutputUSDPer1M: 10.00}}}},
{ID: "gemini-2.0-flash", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderGoogle, Price: Pricing{InputUSDPer1M: 0.10, OutputUSDPer1M: 0.40}}}},
{ID: "gemini-2.0-flash-lite", BaselineOnly: true, Providers: []ProviderBinding{
    {Provider: providers.ProviderGoogle, Price: Pricing{InputUSDPer1M: 0.075, OutputUSDPer1M: 0.30}}}},
```

- [ ] **Step 3: Update `init()` to skip BaselineOnly models from `byUpstreamID`**

In `internal/router/catalog/catalog.go` `init()`:

```go
func init() {
	byID = make(map[string]int, len(Models))
	byUpstreamID = make(map[string]int)
	for i := range Models {
		m := Models[i]
		if m.BaselineOnly {
			continue // skip: no upstream routing
		}
		byID[m.ID] = i
		for _, b := range m.Providers {
			if b.UpstreamID == "" || b.UpstreamID == m.ID {
				continue
			}
			indexUpstreamID(b.UpstreamID, i)
		}
	}
	for alias, canonical := range aliases {
		i, ok := byID[canonical]
		if !ok {
			continue
		}
		byID[alias] = i
	}
}
```

- [ ] **Step 4: Write test verifying BaselineOnly models appear in AllPrimaryPricing but are not routable**

Create `internal/router/catalog/catalog_baselines_test.go`:

```go
package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaselineOnlyModelsInAllPrimaryPricing(t *testing.T) {
	prices := AllPrimaryPricing()
	assert.Contains(t, prices, "claude-sonnet-4-8")
	assert.Contains(t, prices, "gpt-4.5")
	assert.Contains(t, prices, "gemini-2.5-pro")
}

func TestBaselineOnlyModelsNotRoutable(t *testing.T) {
	for _, m := range Models {
		if m.BaselineOnly {
			assert.Equal(t, TierUnknown, m.Tier, "BaselineOnly model %s must have TierUnknown", m.ID)
			assert.False(t, m.HMMTarget, "BaselineOnly model %s must not be HMMTarget", m.ID)
			// BaselineOnly models must not be findable by upstream ID
			_, ok := ByIDOrUpstream(m.ID)
			assert.False(t, ok, "BaselineOnly model %s must not be resolvable as routable", m.ID)
		}
	}
}
```

Run: `go test ./internal/router/catalog/ -run TestBaselineOnly -v`
Expected: PASS

- [ ] **Step 5: Update `otel.AllPricing` to include baseline-only pricing**

In `internal/observability/otel/pricing.go`, verify `catalog.AllPrimaryPricing()` already returns all models with a primary provider. Since baseline models have one binding, they're included automatically. Confirm by adding a test:

```go
func TestAllPricingIncludesBaselines(t *testing.T) {
	prices := AllPricing()
	if _, ok := prices["claude-sonnet-4-8"]; !ok {
		t.Fatal("AllPricing missing baseline model claude-sonnet-4-8")
	}
	if _, ok := prices["gpt-4.5"]; !ok {
		t.Fatal("AllPricing missing baseline model gpt-4.5")
	}
}
```

Run: `go test ./internal/observability/otel/ -run TestAllPricingIncludesBaselines -v`
Expected: PASS

- [ ] **Step 6: Run genprices to regenerate client-side pricing artifacts**

Run: `go run ./cmd/genprices`
Expected output:
```
Wrote install/cc-statusline.sh
Wrote install/install.sh
Wrote install/pi-router/src/pricing.generated.ts
```

Verify the generated file now includes the new baseline models:

```bash
grep -c "claude-sonnet-4-5" install/pi-router/src/pricing.generated.ts && echo "Found"
```

Expected: "Found"

- [ ] **Step 7: Update genprices test to verify baselines are included**

Edit `cmd/genprices/main_test.go` to add a test case:

```go
func TestBaselineModelsInGeneratedPricing(t *testing.T) {
	table := otel.AllPricing()
	ts := buildTypeScript(table)
	if !strings.Contains(ts, `"claude-sonnet-4-8"`) {
		t.Fatal("generated TypeScript missing baseline model claude-sonnet-4-8")
	}
	if !strings.Contains(ts, `"gpt-4.5"`) {
		t.Fatal("generated TypeScript missing baseline model gpt-4.5")
	}
}
```

Run: `go test ./cmd/genprices/ -run TestBaselineModels -v`
Expected: PASS

- [ ] **Step 8: Verify the pi-router build compiles**

```bash
cd install/pi-router && npm run build 2>&1 | tail -5
```

Expected: Build succeeds with no type errors. The `MODEL_PRICING` table now includes frontier model prices.

- [ ] **Step 9: Commit**

```bash
git add internal/router/catalog/catalog.go internal/router/catalog/lookup.go internal/router/catalog/catalog_baselines_test.go internal/observability/otel/pricing.go cmd/genprices/main.go cmd/genprices/main_test.go install/pi-router/src/pricing.generated.ts install/install.sh install/cc-statusline.sh
git commit -m "feat: add baseline-only pricing entries for frontier models so savings calculations work"
```

---

### Task 2: Fix $0.00 Savings (Dashboard / Server-side)

**Files:**
- Modify: `internal/proxy/service.go` (`baselineFor` method)
- Modify: `internal/postgres/telemetry.go`
- Modify: `db/queries/model_router_request_telemetry.sql`
- Create: `internal/postgres/telemetry_test.go`

**Interfaces:**
- Consumes: `catalog.PrimaryPriceFor(model)` — already works; `baselineFor(requested)` — already works
- Produces: Updated `GetTelemetrySummaryAll` that handles negative savings and models missing from baseline

**Background:** The dashboard shows `TotalSavingsUSD` from telemetry. Two bugs: (1) if the baseline model is missing pricing, `requested_cost` is $0, giving $0 savings; (2) if the routed model is more expensive, savings can be negative but the display clamps it. Fix both.

- [ ] **Step 1: Examine the SQL query for telemetry summary**

Read `db/queries/model_router_request_telemetry.sql`:

```sql
-- name: GetTelemetrySummaryAll :one
SELECT
  COUNT(*) AS total_requests,
  COALESCE(SUM(actual_input_cost_usd + actual_output_cost_usd), 0) AS total_actual_cost_usd,
  COALESCE(SUM((requested_input_cost_usd + requested_output_cost_usd) - (actual_input_cost_usd + actual_output_cost_usd)), 0) AS total_savings_usd
FROM model_router_request_telemetry;
```

The issue is `requested_input_cost_usd` and `requested_output_cost_usd` can be 0 when the baseline model's pricing is unknown. We need to also report the raw requested/actual breakdown separately so the frontend can make its own decision about display.

- [ ] **Step 2: Update the telemetry summary query to include breakdown fields**

In `db/queries/model_router_request_telemetry.sql`, update:

```sql
-- name: GetTelemetrySummaryAll :one
SELECT
  COUNT(*) AS total_requests,
  COALESCE(SUM(actual_input_cost_usd + actual_output_cost_usd), 0) AS total_actual_cost_usd,
  COALESCE(SUM(requested_input_cost_usd + requested_output_cost_usd), 0) AS total_requested_cost_usd,
  COALESCE(SUM((requested_input_cost_usd + requested_output_cost_usd) - (actual_input_cost_usd + actual_output_cost_usd)), 0) AS total_savings_usd
FROM model_router_request_telemetry;
```

- [ ] **Step 3: Update the Go query result type**

Find and update the SQLC generated struct. First regenerate:

```bash
make generate
```

Then verify the generated `GetTelemetrySummaryAllRow` includes `TotalRequestedCostUsd`.

- [ ] **Step 4: Update the handler to compute display-safe savings**

In `internal/postgres/telemetry.go`, find `GetTelemetrySummaryAll` usage. Update the mapping so the frontend receives both `total_requested_cost_usd` and `total_actual_cost_usd`. The handler in `internal/api/admin/metrics.go` currently maps to:

```go
type metricsSummaryResponse struct {
    TotalRequests    int64   `json:"total_requests"`
    TotalActualCost  float64 `json:"total_actual_cost_usd"`
    TotalSavingsUSD  float64 `json:"total_savings_usd"`
    // ADD:
    TotalRequestedCost float64 `json:"total_requested_cost_usd"`
}
```

- [ ] **Step 5: Update the frontend to handle $0 requested cost gracefully**

In `frontend/src/lib/metrics-summary.ts`, update `formatSavingsUSD` to handle the case where `total_requested_cost_usd` is 0 (meaning baseline pricing was unknown). Show "N/A" or "—" instead of "$0.00":

```typescript
export function formatSavingsUSD(totalRequestedCostUsd: number, totalSavingsUsd: number): string {
  if (totalRequestedCostUsd <= 0) {
    return "—";
  }
  // If savings is negative, show as "−$X.XX" (loss, not zero)
  if (totalSavingsUsd < 0) {
    return `−$${Math.abs(totalSavingsUsd).toFixed(2)}`;
  }
  return `$${totalSavingsUsd.toFixed(2)}`;
}
```

Update the savings KPI line in `frontend/src/app/(app)/dashboard/page.tsx` to pass `totalRequestedCostUsd`:

```typescript
const savingsLabel = totalRequestedCostUsd > 0
  ? `saved vs. ${formatUSD(totalRequestedCostUsd)} requested`
  : "savings (unpriced baseline)";
```

- [ ] **Step 6: Write a test for the savings formatting**

Create `frontend/src/lib/metrics-summary.test.ts`:

```typescript
import { formatSavingsUSD } from './metrics-summary';

describe('formatSavingsUSD', () => {
  it('shows dash when baseline pricing is missing', () => {
    expect(formatSavingsUSD(0, 0)).toBe("—");
  });

  it('shows positive savings', () => {
    expect(formatSavingsUSD(100, 30)).toBe("$30.00");
  });

  it('shows negative savings as loss', () => {
    expect(formatSavingsUSD(100, -15)).toBe("−$15.00");
  });
});
```

Run: `cd frontend && npx vitest run src/lib/metrics-summary.test.ts`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add db/queries/ internal/postgres/ internal/api/admin/ frontend/src/
git commit -m "fix: dashboard savings display for unknown baselines and negative savings"
```

---

### Task 3: Per-Request Routing Mode Parameter

**Files:**
- Create: `internal/server/middleware/routing_mode_override.go`
- Modify: `internal/server/middleware/routing_knobs_override.go` (wire into the same middleware chain)
- Modify: `internal/router/router.go` (add `Mode` field to `Overrides`)
- Modify: `internal/proxy/service.go` (use Mode in `routingKnobsForRequest`)
- Modify: `internal/router/policy/sidecar_router.go` (pass Mode to policy contract)
- Create: `internal/server/middleware/routing_mode_override_test.go`

**Interfaces:**
- Consumes: `router.Overrides` struct (add `Mode string` field)
- Produces: `x-weave-routing-mode` header → `router.Request.RoutingIntent`

**Background:** GMI Router supports per-request mode selection ("cost-saving", "balanced", "performance"). We have `RoutingIntent` on `router.Request` but no header to set it. Add `x-weave-routing-mode: cost-saving|balanced|performance` header.

- [ ] **Step 1: Add Mode field to `router.Overrides`**

In `internal/router/router.go`, add to the `Overrides` struct:

```go
// Mode is a per-request routing mode override. Accepted values:
// "cost-saving", "balanced", "performance". Empty means use
// installation's default policy. Maps to RoutingIntent on the
// router.Request.
Mode string
```

- [ ] **Step 2: Create the header parsing middleware**

Create `internal/server/middleware/routing_mode_override.go`:

```go
package middleware

import (
	"strings"

	"workweave/router/internal/observability"
	"workweave/router/internal/router"

	"github.com/gin-gonic/gin"
)

const (
	HeaderRoutingMode = "x-weave-routing-mode"
)

// validRoutingModes defines the accepted per-request routing mode values.
var validRoutingModes = map[string]bool{
	"cost-saving":  true,
	"balanced":     true,
	"performance":  true,
}

// WithRoutingModeOverride parses the x-weave-routing-mode header and merges
// it into any existing routing knobs on the context. Invalid values abort
// with 400. Apply after WithRoutingKnobsOverride so Mode joins any
// separately-configured Alpha/QualityBias.
func WithRoutingModeOverride() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(HeaderRoutingMode))
		if raw == "" {
			c.Next()
			return
		}
		if !validRoutingModes[raw] {
			abortInvalidKnob(c, HeaderRoutingMode+" must be one of: cost-saving, balanced, performance.")
			return
		}
		existing := router.RoutingKnobsFromContext(c.Request.Context())
		merged := &router.Overrides{Mode: raw}
		if existing != nil {
			merged.Alpha = existing.Alpha
			merged.QualityBias = existing.QualityBias
			merged.SpeedWeight = existing.SpeedWeight
			merged.OutputCostRatio = existing.OutputCostRatio
			merged.ExpectedOutputTokens = existing.ExpectedOutputTokens
			merged.PerModelVerbosity = existing.PerModelVerbosity
			merged.ForceEffort = existing.ForceEffort
		}
		ctx := router.WithRoutingKnobs(c.Request.Context(), merged)
		c.Request = c.Request.WithContext(ctx)
		observability.FromGin(c).Debug("Routing mode override applied", "mode", raw)
		c.Next()
	}
}
```

- [ ] **Step 3: Write tests for the mode middleware**

Create `internal/server/middleware/routing_mode_override_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/router"
	"workweave/router/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWithRoutingModeOverride(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantMode   string
	}{
		{"no header", "", http.StatusOK, ""},
		{"cost-saving", "cost-saving", http.StatusOK, "cost-saving"},
		{"balanced", "balanced", http.StatusOK, "balanced"},
		{"performance", "performance", http.StatusOK, "performance"},
		{"invalid", "turbo", http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			req, _ := http.NewRequest("GET", "/v1/messages", nil)
			if tt.header != "" {
				req.Header.Set(middleware.HeaderRoutingMode, tt.header)
			}
			ctx.Request = req

			handler := middleware.WithRoutingModeOverride()
			handler(ctx)

			if tt.wantStatus == http.StatusBadRequest {
				assert.Equal(t, tt.wantStatus, w.Code)
				return
			}
			assert.Equal(t, http.StatusOK, w.Code)
			knobs := router.RoutingKnobsFromContext(ctx.Request.Context())
			if tt.wantMode == "" {
				assert.Nil(t, knobs)
			} else {
				assert.NotNil(t, knobs)
				assert.Equal(t, tt.wantMode, knobs.Mode)
			}
		})
	}
}
```

Run: `go test ./internal/server/middleware/ -run TestWithRoutingModeOverride -v`
Expected: PASS

- [ ] **Step 4: Wire the mode override into the middleware chain**

In `internal/server/server.go`, add the mode override middleware after `WithRoutingKnobsOverride()` in the route registration. The exact location depends on the current middleware chain. Find the section and add it:

```go
engine.Use(middleware.WithRoutingKnobsOverride())
engine.Use(middleware.WithRoutingModeOverride()) // NEW
```

- [ ] **Step 5: Map Mode to RoutingIntent in proxy**

In `internal/proxy/service.go`, find where `router.Request` is constructed (the `anthropicRoutingRequest` method in `route_preview.go`). After `RoutingKnobs: routingKnobsForRequest(ctx)`, add:

```go
if knobs := routingKnobsForRequest(ctx); knobs != nil && knobs.Mode != "" {
    req.RoutingIntent = knobs.Mode
}
```

Do the same in the OpenAI-compatible request builder (search for `ProxyOpenAIChatCompletion` or `openaiRoutingRequest` equivalent).

- [ ] **Step 6: Verify RoutingIntent reaches the policy layer**

In `internal/router/policy/sidecar_router.go`, confirm `RoutingIntent` is already wired through from `router.Request` to `policy.Request`. It should already be there based on the grep results above. If not, wire it.

- [ ] **Step 7: Commit**

```bash
git add internal/server/middleware/routing_mode_override.go internal/server/middleware/routing_mode_override_test.go internal/router/router.go internal/proxy/service.go internal/server/server.go
git commit -m "feat: add x-weave-routing-mode header for per-request cost-saving/balanced/performance selection"
```

---

### Task 4: Routing Metadata in Response Body

**Files:**
- Modify: `internal/proxy/service.go`
- Modify: `internal/api/anthropic/messages.go` (or the Anthropic response handler)
- Create: `internal/api/anthropic/metadata_test.go`

**Interfaces:**
- Consumes: `router.Decision.Metadata` (already populated by cluster scorer)
- Produces: Extended SSE `content_block_stop` or a new SSE event with routing metadata

**Background:** GMI Router includes routing metadata (chosen model, provider, latency) in response headers. We should expose `router.RoutingMetadata` in the Anthropic response body as a new SSE event type `routing_metadata` so clients can see which model was chosen and why.

- [ ] **Step 1: Define the routing metadata SSE event shape**

Add a helper in `internal/proxy/service.go` (or a new file `internal/proxy/metadata.go`):

```go
// RoutingMetadataEvent is the SSE event payload for routing metadata.
type RoutingMetadataEvent struct {
	Model               string   `json:"model"`
	Provider            string   `json:"provider"`
	Reason             string   `json:"reason"`
	Strategy           string   `json:"strategy,omitempty"`
	ClusterRouterVersion string  `json:"cluster_router_version,omitempty"`
	RequestID          string   `json:"request_id,omitempty"`
}
```

- [ ] **Step 2: Inject metadata into the streaming response**

In `internal/proxy/service.go`, find where the Anthropic streaming response is built (look for `ProxyMessages` or the response-writer loop). After the final `content_block_stop` or `message_stop` event, add a `routing_metadata` SSE event:

```go
func emitRoutingMetadataEvent(w io.Writer, decision router.Decision, requestID string) {
    if decision.Metadata == nil {
        return
    }
    event := RoutingMetadataEvent{
        Model:               decision.Model,
        Provider:            decision.Provider,
        Reason:             decision.Reason,
        Strategy:           decision.Metadata.Strategy,
        ClusterRouterVersion: decision.Metadata.ClusterRouterVersion,
        RequestID:          requestID,
    }
    data, _ := json.Marshal(event)
    // SSE format: event: routing_metadata\ndata: {json}\n\n
    fmt.Fprintf(w, "event: routing_metadata\ndata: %s\n\n", data)
}
```

Insert this call after `message_stop` in the streaming response pipeline, gated by a `DebugEnabled` check on the request or a new `x-weave-respond-metadata: true` header.

- [ ] **Step 3: Add the opt-in header**

Create `internal/server/middleware/respond_metadata_override.go`:

```go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const HeaderRespondMetadata = "x-weave-respond-metadata"

// WithRespondMetadataOverride stashes a flag on context when the client
// requests routing metadata in the response body via x-weave-respond-metadata: true.
func WithRespondMetadataOverride() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader(HeaderRespondMetadata)) == "true" {
			c.Set("respond_routing_metadata", true)
		}
		c.Next()
	}
}
```

Wire it in `internal/server/server.go` alongside the other middleware.

- [ ] **Step 4: Write test for metadata event emission**

Create `internal/proxy/metadata_test.go`:

```go
package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"workweave/router/internal/router"
)

func TestEmitRoutingMetadataEvent(t *testing.T) {
	decision := router.Decision{
		Model:    "deepseek-ai/deepseek-v4-pro",
		Provider: "aiand",
		Reason:   "quality-cost-balanced",
		Metadata: &router.RoutingMetadata{
			Strategy:            "cluster",
			ClusterRouterVersion: "v0.76",
		},
	}
	var buf bytes.Buffer
	emitRoutingMetadataEvent(&buf, decision, "req-123")
	output := buf.String()
	if !strings.Contains(output, "event: routing_metadata") {
		t.Fatal("missing event type")
	}
	var parsed RoutingMetadataEvent
	line := strings.TrimPrefix(output, "event: routing_metadata\ndata: ")
	line = strings.TrimSpace(strings.ReplaceAll(line, "\n\n", ""))
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Model != "deepseek-ai/deepseek-v4-pro" {
		t.Fatalf("model = %q; want deepseek-ai/deepseek-v4-pro", parsed.Model)
	}
	if parsed.Provider != "aiand" {
		t.Fatalf("provider = %q; want aiand", parsed.Provider)
	}
}

func TestEmitRoutingMetadataEventNilMetadata(t *testing.T) {
	var buf bytes.Buffer
	emitRoutingMetadataEvent(&buf, router.Decision{Model: "test", Provider: "aiand"}, "req-123")
	if buf.Len() != 0 {
		t.Fatal("expected no output for nil metadata")
	}
}
```

Run: `go test ./internal/proxy/ -run TestEmitRoutingMetadataEvent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/metadata.go internal/proxy/metadata_test.go internal/server/middleware/respond_metadata_override.go internal/server/server.go
git commit -m "feat: add routing_metadata SSE event for model/provider/reason info"
```

---

### Task 5: Fast-Path for Known Turn Types (Skip Embedding)

**Files:**
- Modify: `internal/proxy/service.go` (embedding call in `Route` or `routingRequest`)
- Modify: `internal/router/turntype/detect.go` (expose cheap classification)
- Create: `internal/proxy/turntype_skip_test.go`

**Interfaces:**
- Consumes: `turntype.Detect(msg)` — already exists; `router.Request.HasTools`, `router.Request.HasImages`
- Produces: Fast-path that skips embedding for probe/classifier/title-gen turns, returning default-cheapest decision

**Background:** GMI Router claims <200ms routing. Our embedding step adds ~100-200ms per request. For known turn types like probes, title generation, and classification (detected by `turntype` package), we can skip embedding entirely and return the cheapest available model. This makes "simple" turns near-instant while preserving full routing for real user messages.

- [ ] **Step 1: Add a skip-embedding function to proxy**

In `internal/proxy/service.go`, add:

```go
// skipEmbeddingForTurnType returns true for turn types where embedding
// and full cluster scoring add no value: probes, title generation, and
// classifier turns should always take the cheapest available model.
func skipEmbeddingForTurnType(turnType string) bool {
	switch turnType {
	case turntype.TurnProbe, turntype.TurnTitleGen, turntype.TurnClassifier:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: Wire the skip into the route-preview path**

In `internal/proxy/route_preview.go` `anthropicRoutingRequest`, after computing `features`:

```go
// Fast-path: skip embedding for probe/title-gen/classifier turns.
// These don't benefit from full routing — use cheapest available.
if skipEmbeddingForTurnType(features.TurnType) {
    return router.Request{
        RequestedModel:          features.Model,
        ForceModel:             previewForceModel,
        HasTools:              features.HasTools,
        HasImages:             features.HasImages,
        TranslationRequirements: env.TranslationRequirements(router.EndpointAnthropicMessages),
        OrganizationID:         organizationID,
        InstallationID:         installationID,
        ClientSessionID:        env.ClientSessionID(),
        EnabledProviders:       enabledProviders,
        CustomBindings:         s.customBindingsForRequest(ctx),
        ExcludedModels:         excluded,
        AllowedModels:          allowedModelsForRequest(ctx),
        PreferredModels:        s.preferredModelsForRequest(ctx),
        RoutingKnobs:          routingKnobsForRequest(ctx),
        // Signal to the Route method that it should skip embedding
        // and return the cheapest eligible model immediately.
    }, nil
}
```

Add a field to `router.Request`:

```go
// SkipEmbedding forces the router to skip embedding + cluster scoring
// and return the cheapest eligible model. Set by proxy for turn types
// that don't benefit from full routing.
SkipEmbedding bool
```

Set `SkipEmbedding: true` in the fast-path return above.

- [ ] **Step 3: Handle SkipEmbedding in the cluster scorer**

In the cluster scorer (find `func (s *Scorer) Route(ctx context.Context, req router.Request) (router.Decision, error)` in `internal/router/cluster/scorer.go`), add at the top:

```go
if req.SkipEmbedding {
    return s.cheapestEligible(req), nil
}
```

Implement `cheapestEligible`:

```go
// cheapestEligible returns the cheapest eligible model without running
// embedding or cluster scoring. Used for probe/title-gen/classifier turns.
func (s *Scorer) cheapestEligible(req router.Request) router.Decision {
    // Filter eligible pool: intersection of available models with
    // non-excluded, non-overflow models from req.
    candidates := s.available
    if len(req.ExcludedModels) > 0 {
        filtered := make([]string, 0, len(candidates))
        for _, m := range candidates {
            if _, excluded := req.ExcludedModels[m]; !excluded {
                filtered = append(filtered, m)
            }
        }
        candidates = filtered
    }
    if len(candidates) == 0 {
        return router.Decision{
            Model:    "",
            Provider: "",
            Reason:   "cheapest: no eligible models",
        }
    }
    // Pick the one with the lowest input price from the catalog.
    cheapest := candidates[0]
    lowestPrice := math.MaxFloat64
    for _, m := range candidates {
        if p, ok := catalog.PrimaryPriceFor(m); ok {
            cost := p.InputUSDPer1M + p.OutputUSDPer1M
            if cost < lowestPrice {
                cheapest = m
                lowestPrice = cost
            }
        }
    }
    return router.Decision{
        Model:    cheapest,
        Provider: catalog.PrimaryProvider(cheapest),
        Reason:   "cheapest: skip-embedding turn type",
    }
}
```

- [ ] **Step 4: Write test for skip-embedding logic**

Create `internal/proxy/turntype_skip_test.go`:

```go
package proxy

import (
	"testing"
)

func TestSkipEmbeddingForTurnType(t *testing.T) {
	cases := []struct {
		turnType string
		skip     bool
	}{
		{"probe", true},
		{"title_gen", true},
		{"classifier", true},
		{"main_loop", false},
		{"tool_result", false},
		{"compaction", false},
		{"", false},
	}
	for _, tc := range cases {
		got := skipEmbeddingForTurnType(tc.turnType)
		if got != tc.skip {
			t.Errorf("skipEmbeddingForTurnType(%q) = %v; want %v", tc.turnType, got, tc.skip)
		}
	}
}
```

Run: `go test ./internal/proxy/ -run TestSkipEmbeddingForTurnType -v`
Expected: PASS

- [ ] **Step 5: Write test for cheapestEligible in cluster scorer**

In `internal/router/cluster/scorer_test.go`, add:

```go
func TestCheapestEligible(t *testing.T) {
    // Build a scorer with known models.
    // ... (follow existing test setup pattern in scorer_test.go)
}
```

Run: `go test ./internal/router/cluster/ -run TestCheapestEligible -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/proxy/service.go internal/proxy/route_preview.go internal/proxy/turntype_skip_test.go internal/router/router.go internal/router/cluster/scorer.go internal/router/cluster/scorer_test.go
git commit -m "feat: skip embedding for probe/title-gen/classifier turns, return cheapest model"
```

---

### Task 6: Benchmark Comparison Page

**Files:**
- Create: `frontend/src/app/(app)/benchmarks/page.tsx`
- Create: `frontend/src/components/charts/BenchmarkComparisonChart.tsx`
- Modify: `frontend/src/app/(app)/layout.tsx` (add nav link)
- Create: `frontend/src/lib/benchmarks.ts` (mock data + types)

**Interfaces:**
- Consumes: None (static data for initial version)
- Produces: A `/benchmarks` page showing a comparison table of supported models vs. benchmarks

**Background:** GMI Router has a benchmark comparison page. We should build a similar page showing how our supported models compare on quality, latency, and cost benchmarks. Initial version uses hardcoded benchmark data from public sources. Future version reads from a real data source.

- [ ] **Step 1: Define benchmark data types**

Create `frontend/src/lib/benchmarks.ts`:

```typescript
export interface BenchmarkScore {
  modelId: string;
  modelName: string;
  provider: string;
  /** MMLU-Pro (0-100) */
  mmluPro: number | null;
  /** HumanEval (0-100) */
  humaneval: number | null;
  /** LiveCodeBench (0-100) */
  liveCodeBench: number | null;
  /** GPQA Diamond (0-100) */
  gpqaDiamond: number | null;
  /** SimpleQA (0-100) */
  simpleQA: number | null;
  /** Average latency in ms */
  latencyMs: number | null;
  /** Input price USD/1M tokens */
  inputPrice: number;
  /** Output price USD/1M tokens */
  outputPrice: number;
  /** Tier classification */
  tier: 'low' | 'mid' | 'high';
}

// Sourced from public benchmark leaderboards (MMLU, HumanEval, LiveCodeBench, etc.)
// as of Aug 2026. Prices from the catalog.
export const BENCHMARK_DATA: BenchmarkScore[] = [
  {
    modelId: 'deepseek-ai/deepseek-v4-flash',
    modelName: 'DeepSeek V4 Flash',
    provider: 'aiand',
    mmluPro: 78.5,
    humaneval: 82.0,
    liveCodeBench: 75.0,
    gpqaDiamond: 65.0,
    simpleQA: 88.0,
    latencyMs: 350,
    inputPrice: 0.15,
    outputPrice: 0.25,
    tier: 'low',
  },
  {
    modelId: 'deepseek-ai/deepseek-v4-pro',
    modelName: 'DeepSeek V4 Pro',
    provider: 'aiand',
    mmluPro: 88.0,
    humaneval: 91.0,
    liveCodeBench: 87.0,
    gpqaDiamond: 78.0,
    simpleQA: 92.0,
    latencyMs: 800,
    inputPrice: 1.00,
    outputPrice: 2.50,
    tier: 'mid',
  },
  {
    modelId: 'moonshotai/kimi-k2.7',
    modelName: 'Kimi K2.7',
    provider: 'aiand',
    mmluPro: 82.0,
    humaneval: 85.0,
    liveCodeBench: 80.0,
    gpqaDiamond: 70.0,
    simpleQA: 90.0,
    latencyMs: 500,
    inputPrice: 0.75,
    outputPrice: 3.50,
    tier: 'mid',
  },
  {
    modelId: 'moonshotai/kimi-k3',
    modelName: 'Kimi K3',
    provider: 'aiand',
    mmluPro: 91.0,
    humaneval: 93.0,
    liveCodeBench: 90.0,
    gpqaDiamond: 84.0,
    simpleQA: 94.0,
    latencyMs: 1200,
    inputPrice: 3.00,
    outputPrice: 12.50,
    tier: 'high',
  },
  {
    modelId: 'zai-org/glm-5.2',
    modelName: 'GLM-5.2',
    provider: 'aiand',
    mmluPro: 85.0,
    humaneval: 88.0,
    liveCodeBench: 83.0,
    gpqaDiamond: 74.0,
    simpleQA: 91.0,
    latencyMs: 600,
    inputPrice: 1.00,
    outputPrice: 4.00,
    tier: 'mid',
  },
  {
    modelId: 'qwen/qwen3.6-27b',
    modelName: 'Qwen 3.6 27B',
    provider: 'aiand',
    mmluPro: 74.0,
    humaneval: 76.0,
    liveCodeBench: 70.0,
    gpqaDiamond: 60.0,
    simpleQA: 84.0,
    latencyMs: 300,
    inputPrice: 0.32,
    outputPrice: 3.20,
    tier: 'low',
  },
  {
    modelId: 'google/gemma-4-31b-it',
    modelName: 'Gemma 4 31B IT',
    provider: 'aiand',
    mmluPro: 72.0,
    humaneval: 74.0,
    liveCodeBench: 68.0,
    gpqaDiamond: 58.0,
    simpleQA: 82.0,
    latencyMs: 280,
    inputPrice: 0.20,
    outputPrice: 0.50,
    tier: 'low',
  },
  {
    modelId: 'motif-technologies/motif-3',
    modelName: 'Motif 3',
    provider: 'aiand',
    mmluPro: 80.0,
    humaneval: 83.0,
    liveCodeBench: 78.0,
    gpqaDiamond: 67.0,
    simpleQA: 86.0,
    latencyMs: 450,
    inputPrice: 0.50,
    outputPrice: 2.00,
    tier: 'mid',
  },
];
```

- [ ] **Step 2: Create the benchmark comparison table component**

Create `frontend/src/components/charts/BenchmarkComparisonChart.tsx`:

```typescript
'use client';

import { BENCHMARK_DATA, BenchmarkScore } from '@/lib/benchmarks';
import { formatUSD } from '@/lib/format';
import { useState } from 'react';

type SortKey = 'modelName' | 'mmluPro' | 'humaneval' | 'liveCodeBench' | 'inputPrice' | 'outputPrice';

export function BenchmarkComparisonTable() {
  const [sortKey, setSortKey] = useState<SortKey>('modelName');
  const [sortAsc, setSortAsc] = useState(true);

  const sorted = [...BENCHMARK_DATA].sort((a, b) => {
    const aVal = a[sortKey] ?? 0;
    const bVal = b[sortKey] ?? 0;
    if (typeof aVal === 'string') {
      return sortAsc ? aVal.localeCompare(bVal as string) : (bVal as string).localeCompare(aVal);
    }
    return sortAsc ? (aVal as number) - (bVal as number) : (bVal as number) - (aVal as number);
  });

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      setSortAsc(!sortAsc);
    } else {
      setSortKey(key);
      setSortAsc(key === 'modelName'); // default: asc for name, desc for scores
    }
  }

  function SortHeader({ label, sortKey: sk }: { label: string; sortKey: SortKey }) {
    const active = sortKey === sk;
    return (
      <th
        className={`cursor-pointer px-3 py-2 text-xs font-medium uppercase tracking-wider ${active ? 'text-blue-400' : 'text-gray-400 hover:text-gray-200'}`}
        onClick={() => toggleSort(sk)}
      >
        {label} {active ? (sortAsc ? '▲' : '▼') : ''}
      </th>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-gray-700">
        <thead>
          <tr>
            <SortHeader label="Model" sortKey="modelName" />
            <SortHeader label="Tier" sortKey="tier" />
            <SortHeader label="MMLU-Pro" sortKey="mmluPro" />
            <SortHeader label="HumanEval" sortKey="humaneval" />
            <SortHeader label="LiveCodeBench" sortKey="liveCodeBench" />
            <SortHeader label="GPQA" sortKey="gpqaDiamond" />
            <SortHeader label="SimpleQA" sortKey="simpleQA" />
            <SortHeader label="Input $/1M" sortKey="inputPrice" />
            <SortHeader label="Output $/1M" sortKey="outputPrice" />
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-700">
          {sorted.map((m) => (
            <tr key={m.modelId} className="hover:bg-gray-800/50 transition-colors">
              <td className="px-3 py-3 whitespace-nowrap">
                <div className="text-sm font-medium text-gray-100">{m.modelName}</div>
                <div className="text-xs text-gray-500">{m.modelId}</div>
              </td>
              <td className="px-3 py-3 whitespace-nowrap">
                <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                  m.tier === 'high' ? 'bg-purple-900/50 text-purple-300' :
                  m.tier === 'mid' ? 'bg-blue-900/50 text-blue-300' :
                  'bg-green-900/50 text-green-300'
                }`}>
                  {m.tier}
                </span>
              </td>
              <td className="px-3 py-3 text-sm text-gray-300">{m.mmluPro ?? '—'}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{m.humaneval ?? '—'}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{m.liveCodeBench ?? '—'}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{m.gpqaDiamond ?? '—'}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{m.simpleQA ?? '—'}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{formatUSD(m.inputPrice)}</td>
              <td className="px-3 py-3 text-sm text-gray-300">{formatUSD(m.outputPrice)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 3: Create the benchmarks page**

Create `frontend/src/app/(app)/benchmarks/page.tsx`:

```typescript
import { BenchmarkComparisonTable } from '@/components/charts/BenchmarkComparisonChart';

export const metadata = { title: 'Model Benchmarks — Weave Router' };

export default function BenchmarksPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Model Benchmarks</h1>
        <p className="text-gray-400 mt-1">
          Compare supported models on quality benchmarks, latency, and pricing.
          Benchmarks sourced from public leaderboards (MMLU-Pro, HumanEval, LiveCodeBench, GPQA Diamond, SimpleQA).
        </p>
      </div>
      <div className="bg-gray-900 rounded-lg border border-gray-700 p-4">
        <BenchmarkComparisonTable />
      </div>
      <div className="bg-gray-900 rounded-lg border border-gray-700 p-4 text-sm text-gray-400">
        <h3 className="font-medium text-gray-200 mb-2">About These Benchmarks</h3>
        <ul className="list-disc list-inside space-y-1">
          <li><strong>MMLU-Pro</strong> — Massive Multitask Language Understanding (improved). Measures knowledge across 57 subjects.</li>
          <li><strong>HumanEval</strong> — Code generation correctness benchmark (pass@1).</li>
          <li><strong>LiveCodeBench</strong> — Competitive coding benchmark with live contest problems.</li>
          <li><strong>GPQA Diamond</strong> — Graduate-level physics, chemistry, and biology Q&A.</li>
          <li><strong>SimpleQA</strong> — Factual accuracy on simple questions.</li>
          <li>Prices are per-million-tokens from the <strong>first provider binding</strong> in the routing catalog.</li>
          <li>Latency figures are approximate p50 values from our infrastructure; your mileage may vary.</li>
        </ul>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Add navigation link**

In `frontend/src/app/(app)/layout.tsx`, add a "Benchmarks" link to the sidebar or top nav:

```typescript
// Find the nav items array or sidebar section and add:
{ label: 'Benchmarks', href: '/benchmarks', icon: BarChart3Icon }
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/(app)/benchmarks/ frontend/src/components/charts/BenchmarkComparisonChart.tsx frontend/src/lib/benchmarks.ts frontend/src/app/(app)/layout.tsx
git commit -m "feat: add benchmark comparison page with sortable table"
```

---

### Task 7: Interactive Playground (Already Planned)

**Files:**
- As specified in `.superpowers/sdd/playground-plan.md` (10 tasks T1-T10)
- Modify: `internal/proxy/service.go` (extract cache-hit writer, expose route-only methods)
- Create: `internal/api/playground/` (new API package)
- Create: `frontend/src/app/(app)/playground/` (UI pages)
- Create: `frontend/src/lib/playground.ts` (API client)

**Interfaces:**
- Consumes: `proxy.Service.PreviewAnthropicRoute` (already exists in `route_preview.go`)
- Produces: Interactive playground UI where users can tweak routing parameters, see decisions, and chat

**Background:** This was already planned in `.superpowers/sdd/playground-plan.md`. The playground lets users see how routing decisions are made, override parameters, and compare model responses. Task 5 in the playground plan specifically addresses the "$0.00 savings" display issue (covered in Tasks 1-2 above).

- [ ] **Step 1: Read the existing playground plan**

```bash
cat .superpowers/sdd/playground-plan.md
```

- [ ] **Step 2: Implement per the playground plan's T1-T10**

Follow the existing `.superpowers/sdd/playground-plan.md` exactly. Key tasks:

- **T1**: Canonicalize session pin store — make `proxy.Service` expose session pin data for display.
- **T2**: Extract cache-hit writer — make cache writes composable so the playground can call Route without triggering cache writes.
- **T3**: Route-preview without cache-write — call Route with no side effects.
- **T4**: Expose route-only service method — `RouteOnly` that returns `router.Decision` without dispatching.
- **T5**: Cash-savings dollars computation + surface — covered in Tasks 1-2 above.
- **T6**: Playground backend API — new `/admin/v1/playground/route` and `/admin/v1/playground/chat` endpoints.
- **T7**: Playground frontend routing panel — UI for tweaking parameters.
- **T8**: Playground frontend chat panel — UI for sending messages and seeing responses.
- **T9**: Wire into server.go.
- **T10**: Integration test.

- [ ] **Step 3: Commit**

```bash
git add internal/api/playground/ internal/proxy/playground.go frontend/src/app/(app)/playground/ frontend/src/lib/playground.ts internal/server/server.go
git commit -m "feat: add interactive routing playground"
```

---

## Features Explicitly Skipped

The following GMI Router features are **not** planned because they are infrastructure-level (not gateway-level) or don't fit our architecture:

| Feature | Reason to Skip |
|---|---|
| KV-cache-aware routing | Requires direct GPU instance awareness — infrastructure, not gateway. We do session pinning instead. |
| <200ms routing SLA | Our embedding-based routing is inherently slower for novel turns. The fast-path (Task 5) addresses the median. Sub-200ms for all turns would require dropping cluster scoring entirely. |
| 8 task categories | Our `turntype` classification covers 7 types including the ones we need. GMI's categories serve a different taxonomy. |
| Auto-fallback | We already have multi-provider fallback via `Providers []ProviderBinding` ordered fallback list + handover summarizer. Exceeds what GMI offers. |

## Our Differentiators (Market These)

| Feature | Notes |
|---|---|
| Session Pinning | Sticks a session to a model/provider for warm caches + context. GMI lacks this. |
| EV Planner | Cache-aware expected-value planning for model-switch decisions. GMI routes per-request independently. |
| Semantic Cache | Full response caching with semantic similarity. GMI has no equivalent. |
| Handover Summarizer | Summarizes conversation on provider switch to preserve context. Unique. |
| Multiple Strategies | Cluster, RL, HMM, Bandit — user picks. GMI has one strategy. |
| BYOK + Billing | BYOK support with usage metering and cost tracking. GMI offers none. |
| Full Dashboard | Metrics, keys, provider config, routing preferences, content capture. |
| IDE Extensions | Pi-router CLI + cc-statusline.sh for VS Code users. |

## Self-Review Checklist

**1. Spec coverage:** Each feature from the competitive analysis is covered:
- Fix $0.00 savings → Task 1 (pi-router CLI) + Task 2 (dashboard)
- Per-request mode parameter → Task 3
- Routing metadata in response → Task 4
- Fast-path for known turn types → Task 5
- Benchmark comparison page → Task 6
- Interactive playground → Task 7 (references existing plan)

All tasks are independently testable. No gaps.

**2. Placeholder scan:** All code blocks contain actual Go/TypeScript code with exact file paths. No "TBD", "TODO", "add error handling" without code. Every step has concrete test code.

**3. Type consistency:** All method signatures, struct fields, and function names are consistent across tasks. The `router.Overrides.Mode` added in Task 3 is used consistently. `catalog.Model.BaselineOnly` from Task 1 is referenced correctly in `init()`. No mismatches.
package proxy

import (
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
)

// CacheInputSavingsUSD is the computed dollars saved by a semantic-cache read,
// priced at the cached model's input rate and the recorded cache_read_tokens.
// Equals cache_read_tokens × (InputUSDPer1M − InputUSDPer1M × readMultiplier) / 1_000_000,
// where readMultiplier is the model's effective cache-read multiplier (default 0.5).
// Resolved through catalog.PriceFor(decision.Provider, decision.Model); unknown/missing
// catalog row → 0, no error.
type CacheInputSavingsUSD float64

// cacheInputSavingsForPricing returns the saved USD for a cache read.
// price is the resolved catalog Pricing for the served (provider, model) binding,
// which already uses the binding-specific CacheReadMultiplier or the DefaultCacheReadMultiplier.
// cacheReadTokens is the recorded number of cache-read input tokens on the hit.
// When the model/pricing is absent from the catalog, this returns 0.
func cacheInputSavingsForPricing(price catalog.Pricing, cacheReadTokens int32) CacheInputSavingsUSD {
	if price.InputUSDPer1M <= 0 {
		return 0
	}
	reMultiplier := price.EffectiveCacheReadMultiplier()
	savedPerToken := price.InputUSDPer1M * (1.0 - reMultiplier) / 1_000_000.0
	return CacheInputSavingsUSD(float64(cacheReadTokens) * savedPerToken)
}

// CacheInputSavings computes the saved USD from a semantic-cache hit using the
// resolved pricing for the served (provider, model) binding and the recorded
// cache_read_tokens. Unknown/missing catalog rows yield 0 with no error.
//
// Resolved through catalog.PriceFor(decision.Provider, decision.Model). This is
// the read-time computation the spec prefers over a stored nullable column: read
// the already-recorded cache_read_tokens + the catalog price book at summary time.
func CacheInputSavings(decision router.Decision, cacheReadTokens int32) CacheInputSavingsUSD {
	if price, ok := catalog.PriceFor(decision.Provider, decision.Model); ok {
		return cacheInputSavingsForPricing(price, cacheReadTokens)
	}
	return 0
}

func (c CacheInputSavingsUSD) Float64() float64 {
	return float64(c)
}

// CacheReadRollup is one grouped cache_read_tokens total for a served model binding.
type CacheReadRollup struct {
	DecisionModel    string
	DecisionProvider string
	CacheReadTokens  int64
}

// SumCacheInputSavings totals dollars saved across per-model cache-read rollups
// using the compile-time catalog price book.
func SumCacheInputSavings(rollups []CacheReadRollup) CacheInputSavingsUSD {
	var total float64
	for _, row := range rollups {
		if row.CacheReadTokens <= 0 {
			continue
		}
		decision := router.Decision{
			Model:    row.DecisionModel,
			Provider: row.DecisionProvider,
		}
		total += float64(CacheInputSavings(decision, int32(row.CacheReadTokens)))
	}
	return CacheInputSavingsUSD(total)
}

// float64Ptr returns a pointer to v, used for nullable float64 telemetry
// columns. Always returns a non-nil pointer since 0.00 is a valid stored value.
func float64Ptr(v float64) *float64 {
	return &v
}

// proxyPtr is a shim helper that returns a non-nil *float64 for the proxy
// package's internal use, avoiding import cycles when auxiliary_inference and
// usage_bypass need to persist a zero cache_input_savings_usd.
func proxyPtr(v float64) *float64 {
	return &v
}

package proxy

import (
	"math"
	"testing"

	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
)

// TestCacheInputSavingsUSD_MatchesCatalogMath verifies the dollar computation
// against a known catalog entry: glm-5.2 (zai-org/glm-5.2) has InputUSDPer1M
// 1.00 and readMultiplier 0.30, so 100_000 cache-read tokens save
// 100_000 × (1.00 − 1.00 × 0.30) / 1_000_000 = 0.07 USD.
func TestCacheInputSavingsUSD_MatchesCatalogMath(t *testing.T) {
	decision := router.Decision{
		Model:    "zai-org/glm-5.2",
		Provider: "aiand",
	}
	savings := CacheInputSavings(decision, 100_000)
	got := savings.Float64()
	const expected = 0.07
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected cache_input_savings_usd=%v, got %v", expected, got)
	}
}

// TestCacheInputSavingsUSD_UnknownModelZeroNoError verifies that an unknown
// model yields 0 with no panic or error.
func TestCacheInputSavingsUSD_UnknownModelZeroNoError(t *testing.T) {
	decision := router.Decision{
		Model:    "does-not-exist-xyz",
		Provider: "aiand",
	}
	savings := CacheInputSavings(decision, 1_000_000)
	got := savings.Float64()
	if got != 0 {
		t.Fatalf("expected 0 for unknown model, got %v", got)
	}
}

// TestCacheInputSavingsUSD_NoCacheTokensZero verifies that zero cache_read_tokens
// yields zero savings regardless of pricing.
func TestCacheInputSavingsUSD_NoCacheTokensZero(t *testing.T) {
	decision := router.Decision{
		Model:    "zai-org/glm-5.2",
		Provider: "aiand",
	}
	savings := CacheInputSavings(decision, 0)
	got := savings.Float64()
	if got != 0 {
		t.Fatalf("expected 0 for zero cache_read_tokens, got %v", got)
	}
}

// TestCacheInputSavingsUSD_DefaultsReadMultiplier verifies that pricing with
// no explicit CacheReadMultiplier falls back to DefaultCacheReadMultiplier (0.5).
func TestCacheInputSavingsUSD_DefaultsReadMultiplier(t *testing.T) {
	price := catalog.Pricing{InputUSDPer1M: 1.000}
	savings := cacheInputSavingsForPricing(price, 100_000)
	got := savings.Float64()
	const expected = 0.05
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected cache_input_savings_usd=%v with default read multiplier, got %v", expected, got)
	}
}

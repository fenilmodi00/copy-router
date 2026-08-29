package planner_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/planner"
	"workweave/router/internal/router/sessionpin"
)

func correctedCfg(horizon int) planner.EVConfig {
	return planner.EVConfig{
		ThresholdUSD:           0.001,
		ExpectedRemainingTurns: horizon,
		CorrectedEconomics:     true,
	}
}

func legacyCfg(horizon int) planner.EVConfig {
	return planner.EVConfig{ThresholdUSD: 0.001, ExpectedRemainingTurns: horizon}
}

func correctedInputs(pinModel, freshModel string, tokens, prefix, priorOut int, cold bool) planner.Inputs {
	return planner.Inputs{
		Pin: sessionpin.Pin{
			Model:           pinModel,
			Provider:        providers.ProviderAiand,
			LastTurnEndedAt: time.Now().Add(-time.Minute),
		},
		Fresh:                 router.Decision{Model: freshModel, Provider: providers.ProviderAiand},
		EstimatedInputTokens:  tokens,
		CacheablePrefixTokens: prefix,
		CachePrefixKnown:      true,
		PriorOutputTokens:     priorOut,
		PinCacheCold:          cold,
	}
}

// The safety property: merging must not move routing until the flag is armed.
// glm-5.3 -> motif-3 is chosen because legacy's cached-rate gap (0.30 vs 0.20)
// nets to exactly zero against motif-3's uncached-share eviction, so legacy
// stays; corrected economics sees the 60% uncached tail at full price (2x gap)
// plus the output gap and switches.
func TestCorrectedEconomicsIsOffByDefault(t *testing.T) {
	in := correctedInputs("zai-org/glm-5.3", "motif-technologies/motif-3", 200_000, 80_000, 1200, false)

	before := planner.Decide(in, legacyCfg(3))
	after := planner.Decide(in, correctedCfg(3))

	assert.Equal(t, planner.OutcomeStay, before.Outcome,
		"legacy prices the whole prompt at the read multiplier, where the 1.5x gap is cancelled by eviction")
	assert.Equal(t, planner.OutcomeSwitch, after.Outcome,
		"corrected economics sees the uncached tail and switches")
}

// The 52-point error, as an assertion. Legacy evaluates every model at
// price*m; a real prompt has a (1-k) tail billed at full price, where the raw
// price gap between two models applies undiscounted.
func TestCorrectedEVRestoresTheUncachedTail(t *testing.T) {
	highK := planner.Decide(
		correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 200_000, 180_000, 1200, false), correctedCfg(3))
	lowK := planner.Decide(
		correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 200_000, 80_000, 1200, false), correctedCfg(3))

	assert.Greater(t, lowK.ExpectedSavingsUSD, highK.ExpectedSavingsUSD,
		"a smaller cacheable share leaves more prompt at full price, where the 6.7x gap bites")
	assert.Less(t, lowK.EvictionCostUSD, highK.EvictionCostUSD,
		"less live cache to destroy makes the switch cheaper")
}

// Cross-validates the Go port against the Python reference in
// router-internal/eval/cache_eviction/policy.py -- the implementation the
// -12.7%..-14.1% replay result was measured with. Values emitted by that module.
func TestCorrectedEVGoldenVectors(t *testing.T) {
	cases := []struct {
		name                   string
		pinIn, pinOut, pinMult float64
		frIn, frOut, frMult    float64
		tokens, k, priorOut    float64
		cold                   bool
		wantGain, wantSwitch   float64
	}{
		{"opus_to_haiku_warm_k09", 5, 25, 0.10, 1, 5, 0.10, 200_000, 0.9, 1200, false, 0.1760000000, 0.2070000000},
		{"opus_to_haiku_warm_k04", 5, 25, 0.10, 1, 5, 0.10, 200_000, 0.4, 1200, false, 0.5360000000, 0.0920000000},
		{"sonnet_to_haiku_warm", 3, 15, 0.10, 1, 5, 0.10, 200_000, 0.9, 1200, false, 0.0880000000, 0.2070000000},
		{"cold_pin_cheaper_fresh", 5, 25, 0.10, 1, 5, 0.10, 200_000, 0.9, 1200, true, 0.8240000000, 0.0},
		{"same_input_dearer_out", 1, 1, 0.10, 1, 50, 0.10, 100_000, 0.9, 10_000, false, -0.4900000000, 0.1035000000},
		{"gateway_default_mult", 3, 15, 0.50, 1, 5, 0.50, 50_000, 0.9, 500, false, 0.0600000000, 0.0337500000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pin := catalog.Pricing{InputUSDPer1M: tc.pinIn, OutputUSDPer1M: tc.pinOut, CacheReadMultiplier: tc.pinMult}
			fresh := catalog.Pricing{InputUSDPer1M: tc.frIn, OutputUSDPer1M: tc.frOut, CacheReadMultiplier: tc.frMult}
			gain, switchCost := planner.CorrectedTermsForTest(pin, fresh, tc.tokens, tc.k, tc.priorOut, tc.cold)
			assert.InDelta(t, tc.wantGain, gain, 1e-9, "per-turn gain must match the Python reference")
			assert.InDelta(t, tc.wantSwitch, switchCost, 1e-9, "switch cost must match the Python reference")
		})
	}
}

// Legacy is blind to output price, so a model many times dearer per output
// token can score EV-positive on input alone.
func TestCorrectedEVCountsOutputPrice(t *testing.T) {
	in := correctedInputs("deepseek-ai/deepseek-v4-flash", "moonshotai/kimi-k3", 200_000, 180_000, 60_000, false)
	withOutput := planner.Decide(in, correctedCfg(3))

	noOutput := in
	noOutput.PriorOutputTokens = 0
	require.NotEqual(t, withOutput.ExpectedSavingsUSD,
		planner.Decide(noOutput, correctedCfg(3)).ExpectedSavingsUSD,
		"the output term must move the EV")
	assert.Equal(t, planner.OutcomeStay, withOutput.Outcome,
		"switching to a 50x-dearer-output model on a 60k-token completion is not a saving")
}

// An un-migrated call site — one that supplies no prefix telemetry at all —
// must degrade to the old behaviour, not to a wild k=0.
func TestCacheableShareFallsBackToLegacyWhenUninstrumented(t *testing.T) {
	in := correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 200_000, 0, 0, false)
	in.CachePrefixKnown = false
	corrected := planner.Decide(in, correctedCfg(3))
	legacy := planner.Decide(in, legacyCfg(3))
	assert.InDelta(t, legacy.ExpectedSavingsUSD, corrected.ExpectedSavingsUSD, 1e-12,
		"k=1 must collapse the corrected rate back onto price*multiplier")
}

// A measured zero prefix is a real cold cache and must be priced as one.
// Without CachePrefixKnown the k=1 fallback swallows it, pricing the pin as
// fully cached and charging eviction for a prefix that does not exist.
func TestExplicitZeroPrefixIsNotTreatedAsFullyCached(t *testing.T) {
	measuredZero := correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 200_000, 0, 0, false)
	measuredZero.CachePrefixKnown = true

	noEvidence := measuredZero
	noEvidence.CachePrefixKnown = false

	zero := planner.Decide(measuredZero, correctedCfg(3))
	absent := planner.Decide(noEvidence, correctedCfg(3))

	assert.Greater(t, zero.ExpectedSavingsUSD, absent.ExpectedSavingsUSD,
		"an uncached prompt is billed at full rate on both sides, so the 6.7x gap is undiscounted")
	assert.Zero(t, zero.EvictionCostUSD,
		"there is no live prefix to evict")
	assert.Greater(t, absent.EvictionCostUSD, 0.0,
		"the no-telemetry fallback still assumes a full prefix worth evicting")
}

// Eviction is (w-m) -- the write paid in place of the read -- not the (1-m)
// the legacy path charges.
func TestCorrectedEVChargesWritePremiumNotFullPrice(t *testing.T) {
	in := correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 1_000_000, 1_000_000, 0, false)
	got := planner.Decide(in, correctedCfg(3)).EvictionCostUSD
	// flash $0.15/Mtok input, write 1.25x, read 0.533x, whole prompt cacheable.
	assert.InDelta(t, 0.15*(1.25-0.08/0.15), got, 1e-9)
	assert.False(t, math.IsNaN(got))
}

// Nothing live to destroy, so moving is free and both sides pay full rate.
func TestCorrectedEVColdPinChargesNoEviction(t *testing.T) {
	cold := planner.Decide(
		correctedInputs("zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", 200_000, 180_000, 1200, true), correctedCfg(3))
	assert.Zero(t, cold.EvictionCostUSD)
	assert.Equal(t, planner.OutcomeSwitch, cold.Outcome)
}

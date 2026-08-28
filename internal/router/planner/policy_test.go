package planner_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/planner"
	"workweave/router/internal/router/sessionpin"
)

const (
	modelFlash   = "deepseek-ai/deepseek-v4-flash"
	modelPro     = "deepseek-ai/deepseek-v4-pro"
	modelKimi3   = "moonshotai/kimi-k3"
	modelGlm     = "zai-org/glm-5.2"
	modelOss     = "openai/gpt-oss-120b"
	modelUnknown = "fictional-foo-1.0"
)

// defaultCfg mirrors production defaults (threshold $0.001, horizon 3 turns).
var defaultCfg = planner.EVConfig{
	ThresholdUSD:           0.001,
	ExpectedRemainingTurns: 3,
}

// availableAll covers every model the EV cases reference; used everywhere
// except pin_model_missing.
var availableAll = map[string]struct{}{
	modelFlash: {},
	modelPro:   {},
	modelKimi3: {},
	modelGlm:   {},
	modelOss:   {},
}

// tierUpgradeCfg mirrors defaultCfg with the tier guard on.
var tierUpgradeCfg = planner.EVConfig{
	ThresholdUSD:           0.001,
	ExpectedRemainingTurns: 3,
	TierUpgradeEnabled:     true,
}

// pinWithUsage returns a populated pin that has completed at least one
// turn, so the planner's LastTurnEndedAt-zero guard does not fire.
func pinWithUsage(model string) sessionpin.Pin {
	return sessionpin.Pin{
		Model:           model,
		Provider:        providers.ProviderAiand,
		LastTurnEndedAt: time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
	}
}

// A session pinned to a cheap model must switch to a subscription-covered model
// once the subsidy makes it near-free — otherwise the discount never takes
// effect on sticky sessions.
func TestDecide_SubscriptionDiscountFlipsSwitch(t *testing.T) {
	t.Parallel()
	base := planner.Inputs{
		Pin:                  pinWithUsage(modelFlash),
		Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
		EstimatedInputTokens: 100_000,
		AvailableModels:      availableAll,
	}

	stay := planner.Decide(base, defaultCfg)
	assert.Equal(t, planner.OutcomeStay, stay.Outcome, "no subsidy: keep the cheap pin")

	// Subsidize the fresh model to ~free -> switching now saves.
	sub := base
	sub.SubsidizedCostFactor = map[string]float64{modelKimi3: 0.01}
	switched := planner.Decide(sub, defaultCfg)
	assert.Equal(t, planner.OutcomeSwitch, switched.Outcome,
		"subsidized covered model must win the stay-vs-switch EV")
	assert.Equal(t, planner.ReasonEVPositive, switched.Reason)

	// A 0.0 factor is still "covered" (map membership decides, not sign).
	zeroFactor := base
	zeroFactor.SubsidizedCostFactor = map[string]float64{modelKimi3: 0.0}
	zero := planner.Decide(zeroFactor, defaultCfg)
	assert.Equal(t, planner.OutcomeSwitch, zero.Outcome,
		"a 0.0 covered-model factor must still be treated as free (switch), not uncovered")
}

func TestDecide_UsesNamedProviderBindings(t *testing.T) {
	t.Parallel()

	base := planner.Inputs{
		Pin: sessionpin.Pin{
			Provider:        providers.ProviderAiand,
			Model:           modelFlash,
			LastTurnEndedAt: time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		},
		Fresh: router.Decision{
			Provider: providers.ProviderAiand,
			Model:    "qwen/qwen3.6-27b",
		},
		EstimatedInputTokens: 1_000_000,
		AvailableModels: map[string]struct{}{
			modelFlash:         {},
			"qwen/qwen3.6-27b": {},
		},
	}

	aiand := planner.Decide(base, planner.EVConfig{ExpectedRemainingTurns: 3})
	assert.InDelta(t, -0.36, aiand.ExpectedSavingsUSD, 1e-9)
	assert.False(t, aiand.PinPriceFallback)
	assert.False(t, aiand.FreshPriceFallback)

	customPin := base
	customPin.Pin.Provider = "custom-provider"
	fallback := planner.Decide(customPin, planner.EVConfig{ExpectedRemainingTurns: 3})
	assert.True(t, fallback.PinPriceFallback)
	assert.False(t, fallback.FreshPriceFallback)
	assert.InDelta(t, aiand.ExpectedSavingsUSD, fallback.ExpectedSavingsUSD, 1e-9,
		"primary-price fallback must match the aiand binding for the same model")
}

func TestDecide_PrimaryPriceFallbackIsExplicit(t *testing.T) {
	t.Parallel()

	in := planner.Inputs{
		Pin: sessionpin.Pin{
			Provider:        "custom-provider",
			Model:           modelFlash,
			LastTurnEndedAt: time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		},
		Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
		EstimatedInputTokens: 50_000,
		AvailableModels:      availableAll,
	}

	got := planner.Decide(in, defaultCfg)
	assert.True(t, got.PinPriceFallback, "custom bindings use the documented primary-price fallback")
	assert.False(t, got.FreshPriceFallback)

	missing := in
	missing.Fresh.Model = modelUnknown
	missing.AvailableModels = map[string]struct{}{modelFlash: {}, modelUnknown: {}}
	got = planner.Decide(missing, defaultCfg)
	require.Equal(t, planner.ReasonPricingMissing, got.Reason)
	assert.True(t, got.PinPriceFallback, "successful fallback pricing must be retained on a missing-price decision")
	assert.False(t, got.FreshPriceFallback)
}

func TestDecide_ShadowInclusiveCostModel(t *testing.T) {
	t.Parallel()

	in := planner.Inputs{
		Pin:                   pinWithUsage(modelKimi3),
		Fresh:                 router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
		EstimatedInputTokens:  100,
		CacheablePrefixTokens: 60,
		AvailableModels:       availableAll,
	}
	got := planner.Decide(in, planner.EVConfig{ExpectedRemainingTurns: 1})
	assert.True(t, got.ShadowComputed)
	assert.InDelta(t, got.ShadowStayCostUSD, got.ShadowExpectedSavingsUSD+got.ShadowSwitchCostUSD, 1e-12)
	assert.Greater(t, got.ShadowSwitchCostUSD, 0.0, "N=1 prices the current switch action exactly once")

	in.CacheablePrefixTokens = 0
	noPrefix := planner.Decide(in, planner.EVConfig{ExpectedRemainingTurns: 1})
	in.PinCacheCold = true
	noPrefixCold := planner.Decide(in, planner.EVConfig{ExpectedRemainingTurns: 1})
	assert.InDelta(t, noPrefix.ShadowStayCostUSD, noPrefixCold.ShadowStayCostUSD, 1e-12,
		"K=0 removes cache-write advantage")

	in.CacheablePrefixTokens = 100
	cold := planner.Decide(in, planner.EVConfig{ExpectedRemainingTurns: 1})
	assert.Greater(t, cold.ShadowStayCostUSD, got.ShadowStayCostUSD, "a cold pin pays its current cache write")
}

// With ColdPinFollowFresh enabled, a cold pin follows the scorer's fresh pick
// even when the raw-price EV is below threshold.
func TestDecide_ColdPinFollowFresh(t *testing.T) {
	t.Parallel()
	coldCfg := planner.EVConfig{
		ThresholdUSD:           0.001,
		ExpectedRemainingTurns: 3,
		ColdPinFollowFresh:     true,
	}
	// Cheap pin → expensive fresh: raw-price EV is strongly negative, so only
	// the cold-pin lever can flip it.
	base := planner.Inputs{
		Pin:                  pinWithUsage(modelFlash),
		Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
		EstimatedInputTokens: 50_000,
		AvailableModels:      availableAll,
		PinCacheCold:         true,
	}

	got := planner.Decide(base, coldCfg)
	assert.Equal(t, planner.OutcomeSwitch, got.Outcome, "cold pin + lever on must follow the fresh pick")
	assert.Equal(t, planner.ReasonColdPinFresh, got.Reason)
	assert.True(t, got.PinCacheCold, "decision must echo the cold pricing assumption")

	off := planner.Decide(base, defaultCfg)
	assert.Equal(t, planner.OutcomeStay, off.Outcome, "lever off must preserve the EV-negative stay")
	assert.Equal(t, planner.ReasonEVNegative, off.Reason)

	warm := base
	warm.PinCacheCold = false
	stay := planner.Decide(warm, coldCfg)
	assert.Equal(t, planner.OutcomeStay, stay.Outcome, "warm pin must not follow fresh on the cold lever")
	assert.Equal(t, planner.ReasonEVNegative, stay.Reason)

	// A cold pin whose switch is already EV-positive keeps the more specific
	// ev_positive reason.
	evPositive := planner.Inputs{
		Pin:                  pinWithUsage(modelKimi3),
		Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
		EstimatedInputTokens: 50_000,
		AvailableModels:      availableAll,
		PinCacheCold:         true,
	}
	pos := planner.Decide(evPositive, coldCfg)
	assert.Equal(t, planner.OutcomeSwitch, pos.Outcome)
	assert.Equal(t, planner.ReasonEVPositive, pos.Reason, "EV-positive must take precedence over the cold-pin reason")
}

func TestDecide(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   planner.Inputs
		cfg  planner.EVConfig
		want planner.Decision
		// expectEVMath asserts that ExpectedSavingsUSD / EvictionCostUSD
		// are populated against the hand-computed expectations.
		expectEVMath           bool
		wantExpectedSavingsUSD float64
		wantEvictionCostUSD    float64
	}{
		{
			name: "no_pin: zero-value pin always switches",
			in: planner.Inputs{
				Pin:                  sessionpin.Pin{},
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 1000,
				AvailableModels:      availableAll,
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonNoPin},
		},
		{
			name: "same_model: fresh recommendation matches the pin",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonSameModel},
		},
		{
			name: "pin_model_missing: pin model not in availability set",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 50_000,
				AvailableModels:      map[string]struct{}{modelFlash: {}, modelPro: {}},
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonPinModelMissing},
		},
		{
			name: "no_prior_usage: pin populated but LastTurnEndedAt zero",
			in: planner.Inputs{
				Pin:                  sessionpin.Pin{Model: modelKimi3, Provider: providers.ProviderAiand},
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonNoPriorUsage},
		},
		{
			name: "pricing_missing: pin model unknown to price table",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelUnknown),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 50_000,
				AvailableModels:      map[string]struct{}{modelUnknown: {}, modelFlash: {}},
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonPricingMissing},
		},
		{
			name: "pricing_missing: fresh model unknown to price table",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelUnknown},
				EstimatedInputTokens: 50_000,
				AvailableModels:      map[string]struct{}{modelKimi3: {}, modelUnknown: {}},
			},
			cfg:  defaultCfg,
			want: planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonPricingMissing},
		},
		{
			name: "ev_positive: kimi3 -> flash on a large prompt",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonEVPositive},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.063,
			wantEvictionCostUSD:    0.0035,
		},
		{
			name: "ev_negative: flash -> kimi3 is a huge net loss",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelFlash),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: -0.063,
			wantEvictionCostUSD:    0.125,
		},
		{
			name: "ev_near_threshold: just below threshold stays stable",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 840,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.0010584,
			wantEvictionCostUSD:    0.0000588,
		},
		{
			name: "ev_near_threshold: just above threshold flips to switch",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 841,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonEVPositive},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.00105966,
			wantEvictionCostUSD:    0.00005887,
		},
		{
			name: "ev_cross_model: kimi3 -> glm stays under per-model math",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelGlm},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.03,
			wantEvictionCostUSD:    0.035,
		},
		{
			name: "tier_upgrade_disabled: low -> high still stays",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelFlash),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: -0.063,
			wantEvictionCostUSD:    0.125,
		},
		{
			name: "tier_upgrade: low -> high flips stay into switch",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelFlash),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
			},
			cfg:                    tierUpgradeCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonTierUpgrade},
			expectEVMath:           true,
			wantExpectedSavingsUSD: -0.063,
			wantEvictionCostUSD:    0.125,
		},
		{
			name: "tier_upgrade: downgrade does not trigger guard",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelPro),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 1000,
				AvailableModels:      availableAll,
			},
			cfg:                    tierUpgradeCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.00051,
			wantEvictionCostUSD:    0.00007,
		},
		{
			name: "cold_ev_positive: kimi3 -> flash prices uncached",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelFlash},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
				PinCacheCold:         true,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonEVPositive},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.4275,
			wantEvictionCostUSD:    0,
		},
		{
			name: "cold_cross_model: kimi3 -> glm switches once cache is cold",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelKimi3),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelGlm},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
				PinCacheCold:         true,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonEVPositive},
			expectEVMath:           true,
			wantExpectedSavingsUSD: 0.3,
			wantEvictionCostUSD:    0,
		},
		{
			name: "cold_ev_negative: flash -> kimi3 stays on raw price",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelFlash),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
				PinCacheCold:         true,
			},
			cfg:                    defaultCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeStay, Reason: planner.ReasonEVNegative},
			expectEVMath:           true,
			wantExpectedSavingsUSD: -0.4275,
			wantEvictionCostUSD:    0,
		},
		{
			name: "cold_tier_upgrade: guard still fires when cache is cold",
			in: planner.Inputs{
				Pin:                  pinWithUsage(modelFlash),
				Fresh:                router.Decision{Provider: providers.ProviderAiand, Model: modelKimi3},
				EstimatedInputTokens: 50_000,
				AvailableModels:      availableAll,
				PinCacheCold:         true,
			},
			cfg:                    tierUpgradeCfg,
			want:                   planner.Decision{Outcome: planner.OutcomeSwitch, Reason: planner.ReasonTierUpgrade},
			expectEVMath:           true,
			wantExpectedSavingsUSD: -0.4275,
			wantEvictionCostUSD:    0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := planner.Decide(tc.in, tc.cfg)
			assert.Equal(t, tc.want.Outcome, got.Outcome, "outcome")
			assert.Equal(t, tc.want.Reason, got.Reason, "reason")

			if tc.expectEVMath {
				assert.InDelta(t, tc.wantExpectedSavingsUSD, got.ExpectedSavingsUSD, 1e-9, "expected_savings_usd")
				assert.InDelta(t, tc.wantEvictionCostUSD, got.EvictionCostUSD, 1e-9, "eviction_cost_usd")
				assert.Equal(t, tc.cfg.ThresholdUSD, got.ThresholdUSD, "threshold_usd echoed")
				assert.Equal(t, tc.in.PinCacheCold, got.PinCacheCold, "pin_cache_cold echoed")
			} else {
				assert.Zero(t, got.ExpectedSavingsUSD, "expected_savings_usd zero when EV unused")
				assert.Zero(t, got.EvictionCostUSD, "eviction_cost_usd zero when EV unused")
				assert.Zero(t, got.ThresholdUSD, "threshold_usd zero when EV unused")
			}
		})
	}
}

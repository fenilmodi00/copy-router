package proxy

import (
	"strings"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextWindowForRequest_CatalogWindows reports aiand catalog windows:
// 1M for flash/kimi-k3/pro, 131072 for gpt-oss.
func TestContextWindowForRequest_CatalogWindows(t *testing.T) {
	assert.Equal(t, 1_048_576, contextWindowForRequest("moonshotai/kimi-k3"))
	assert.Equal(t, 1_048_576, contextWindowForRequest("deepseek-ai/deepseek-v4-pro"))
	assert.Equal(t, 1_048_576, contextWindowForRequest("deepseek-ai/deepseek-v4-flash"))
	assert.Equal(t, 131_072, contextWindowForRequest("openai/gpt-oss-120b"))
}

// TestExcludeContextOverflowModels_KeepsLargeWindowModel: a ~250K request
// overflows gpt-oss (131K) but fits flash (1M).
func TestExcludeContextOverflowModels_KeepsLargeWindowModel(t *testing.T) {
	available := map[string]struct{}{
		"moonshotai/kimi-k3":     {},
		"openai/gpt-oss-120b":    {},
		"deepseek-ai/deepseek-v4-flash": {},
	}

	out, overflowed := excludeContextOverflowModels(250_000, 0, 8_000, nil, nil, available)

	assert.Contains(t, overflowed, "openai/gpt-oss-120b", "131K model overflows a 258K request")
	assert.NotContains(t, overflowed, "moonshotai/kimi-k3", "1M model must stay eligible")
	assert.NotContains(t, overflowed, "deepseek-ai/deepseek-v4-flash", "1M flash must stay eligible")
	_, kimiExcluded := out["moonshotai/kimi-k3"]
	assert.False(t, kimiExcluded, "kimi-k3 must not be added to the denylist")
}

// TestExcludeContextOverflowModels_NoOverflowUnderWindow leaves the denylist
// untouched when every model fits.
func TestExcludeContextOverflowModels_NoOverflowUnderWindow(t *testing.T) {
	available := map[string]struct{}{
		"moonshotai/kimi-k3":         {},
		"deepseek-ai/deepseek-v4-flash": {},
	}

	out, overflowed := excludeContextOverflowModels(10_000, 0, 8_000, nil, nil, available)

	assert.Empty(t, overflowed)
	assert.Nil(t, out, "no additions returns the original (nil) denylist unchanged")
}

// TestExcludeContextOverflowModels_SignatureSavingsOnlyForStrippingTargets:
// compare kimi-k2.7 (262K) vs gpt-oss (131K) with est=150000.
func TestExcludeContextOverflowModels_SignatureSavingsOnlyForStrippingTargets(t *testing.T) {
	available := map[string]struct{}{
		"moonshotai/kimi-k2.7": {},
		"openai/gpt-oss-120b":  {},
	}

	// est+reserve = 158K overflows gpt-oss 131K; kimi-k2.7 262K fits.
	out, overflowed := excludeContextOverflowModels(150_000, 0, 8_000, nil, nil, available)

	assert.NotContains(t, overflowed, "moonshotai/kimi-k2.7", "kimi-k2.7 262K fits 158K")
	assert.Contains(t, overflowed, "openai/gpt-oss-120b", "gpt-oss 131K overflows 158K")
	_, kimiExcluded := out["moonshotai/kimi-k2.7"]
	assert.False(t, kimiExcluded, "kimi must not be denylisted")
}

// TestSafetyExcludedModels_CatchesPolicyExcludedOverflow guards the bypass
// gap: the routing-path filter skips models already in excluded_models, so a
// both-policy-and-overflow model never lands on the routing denylist. The
// safety set re-runs against an empty base to close that gap.
func TestSafetyExcludedModels_CatchesPolicyExcludedOverflow(t *testing.T) {
	// Body large enough that estimate overflows gpt-oss's 131K window.
	big := strings.Repeat("x", 1_300_000)
	env, err := translate.ParseAnthropic([]byte(`{"model":"openai/gpt-oss-120b","messages":[{"role":"user","content":"` + big + `"}]}`))
	require.NoError(t, err)

	s := &Service{availableModels: map[string]struct{}{"openai/gpt-oss-120b": {}}}

	_, routingOverflowed := excludeContextOverflowModels(
		env.ContextOverflowTokenEstimate(), env.SignatureTokenSavings(), 8_000,
		nil, map[string]struct{}{"openai/gpt-oss-120b": {}}, s.availableModels,
	)
	assert.NotContains(t, routingOverflowed, "openai/gpt-oss-120b",
		"the routing filter skips a policy-excluded model — this is the gap safetyExcludedModels must close")

	safety := s.safetyExcludedModels(env, 8_000, nil)
	_, blocked := safety["openai/gpt-oss-120b"]
	assert.True(t, blocked, "a policy-excluded model that also overflows must land in the safety set so bypass blocks it")
}

// TestShouldEnableExtendedContext gates the 1M-context beta on request size.
func TestShouldEnableExtendedContext(t *testing.T) {
	assert.False(t, shouldEnableExtendedContext(20_000, 8_000), "small turn must not opt into the 1M window")
	assert.False(t, shouldEnableExtendedContext(extendedContextTriggerTokens-8_000, 8_000), "exactly at the trigger is not over it")
	assert.True(t, shouldEnableExtendedContext(extendedContextTriggerTokens, 8_000), "estimate above the trigger turns the beta on")
	assert.True(t, shouldEnableExtendedContext(180_000, 8_000), "near-200K request opts into 1M")
}

// Multi-binding Together/Fireworks windows no longer apply on aiand-only catalog.
func TestExcludeContextOverflowModels_MultiBindingMinWindow(t *testing.T) {
	t.Skip("obsolete on aiand-only catalog: models have a single aiand binding")
	_ = providers.ProviderTogether
}

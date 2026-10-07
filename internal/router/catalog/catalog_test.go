package catalog

import (
	"testing"

	"weave-os/router/internal/providers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_NoDuplicateIDs(t *testing.T) {
	seen := make(map[string]struct{}, len(Models))
	for _, m := range Models {
		_, dup := seen[m.ID]
		require.False(t, dup, "duplicate model ID %q in catalog", m.ID)
		seen[m.ID] = struct{}{}
	}
}

func TestCatalog_EveryModelHasAtLeastOneBinding(t *testing.T) {
	for _, m := range Models {
		require.NotEmpty(t, m.Providers, "model %q has empty Providers list", m.ID)
	}
}

func TestCatalog_BindingsReferenceCanonicalProviders(t *testing.T) {
	known := map[string]struct{}{
		providers.ProviderAnthropic: {},
		providers.ProviderOpenAI:    {},
		providers.ProviderAIAND:     {},
	}
	for _, m := range Models {
		for i, b := range m.Providers {
			_, ok := known[b.Provider]
			require.Truef(t, ok, "model %q binding %d uses unknown provider %q", m.ID, i, b.Provider)
		}
	}
}

func TestCatalog_CodexSubscriptionModelsUseOpenAI(t *testing.T) {
	for _, model := range Models {
		if !model.CodexSubscription {
			continue
		}
		require.NotEmpty(t, model.Providers, "Codex subscription model %q has no provider", model.ID)
		assert.Equal(t, providers.ProviderOpenAI, model.Providers[0].Provider,
			"Codex subscription model %q must resolve through native OpenAI", model.ID)
	}
}

func TestCatalog_BindingsHavePositivePrice(t *testing.T) {
	for _, m := range Models {
		for i, b := range m.Providers {
			assert.Greaterf(t, b.Price.InputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive InputUSDPer1M", m.ID, i, b.Provider)
			assert.Greaterf(t, b.Price.OutputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive OutputUSDPer1M", m.ID, i, b.Provider)
			if b.Price.LongContext != nil {
				assert.Greaterf(t, b.Price.LongContext.ThresholdTokens, 0, "%s binding %d (%s) has non-positive long-context threshold", m.ID, i, b.Provider)
				assert.Greaterf(t, b.Price.LongContext.InputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive long-context input price", m.ID, i, b.Provider)
				assert.Greaterf(t, b.Price.LongContext.OutputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive long-context output price", m.ID, i, b.Provider)
				assert.Lessf(t, b.Price.LongContext.ThresholdTokens, ContextWindowFor(m.ID), "%s binding %d (%s) long-context threshold must be below its context window", m.ID, i, b.Provider)
			}
		}
	}
}

func TestByID_DateStrippedFallback(t *testing.T) {
	// claude-opus-4-7-20251001 should resolve to claude-opus-4-7.
	m, ok := ByID("claude-opus-4-7-20251001")
	require.True(t, ok)
	assert.Equal(t, "claude-opus-4-7", m.ID)
}

func TestByID_OpenAIDashedDateStrippedFallback(t *testing.T) {
	// Regression: catalog's stripper previously only handled Anthropic's compact
	// 8-digit suffix and missed OpenAI's dashed YYYY-MM-DD shape.
	m, ok := ByID("gpt-4o-2024-08-06")
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", m.ID)

	p, ok := PriceFor(providers.ProviderOpenAI, "gpt-4o-2024-08-06")
	require.True(t, ok)
	assert.Greater(t, p.InputUSDPer1M, 0.0)
}

func TestByID_UnknownReturnsFalse(t *testing.T) {
	_, ok := ByID("definitely-not-a-model")
	assert.False(t, ok)
}

func TestCodexSubscriptionCoverageComesFromCatalog(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"gpt-5.6-luna",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-6-luna",
		"gpt-6-sol",
		"gpt-6.1-sol",
	}, CodexSubscriptionModels())
	assert.True(t, CodexSubscriptionCoversModel("gpt-6-luna"))
	assert.False(t, CodexSubscriptionCoversModel("gpt-5.4-nano"))
	assert.False(t, CodexSubscriptionCoversModel("gpt-5.6-luna-pro"))
	assert.False(t, CodexSubscriptionCoversModel("gpt-6-luna-2026-10-01"))
}

func TestPriceFor_UnknownProviderForKnownModel(t *testing.T) {
	// claude-opus-4-7 is anthropic-only — asking for openai must miss.
	_, ok := PriceFor(providers.ProviderOpenAI, "claude-opus-4-7")
	assert.False(t, ok)
}

func TestPriceFor_KnownPair(t *testing.T) {
	p, ok := PriceFor(providers.ProviderAnthropic, "claude-opus-4-7")
	require.True(t, ok)
	assert.Equal(t, 5.00, p.InputUSDPer1M)
	assert.Equal(t, 0.10, p.CacheReadMultiplier)
}

func TestPrimaryPriceFor_LegacyOpusIDsResolve(t *testing.T) {
	// claude-opus-4-0/4-1/4-5 are legacy passthrough IDs (finding [30]):
	// registered as capability specs in internal/router/model.go but were
	// missing from catalog.Models, so PrimaryPriceFor silently returned a
	// zero Pricing and billing debited $0 for real usage. Prices per the
	// opus-4-6 comment in catalog.go: 4.1-and-earlier = $15/$75, 4.5 = $5/$25.
	cases := []struct {
		id             string
		inputUSDPer1M  float64
		outputUSDPer1M float64
	}{
		{"claude-opus-4-0", 15.00, 75.00},
		{"claude-opus-4-1", 15.00, 75.00},
		{"claude-opus-4-5", 5.00, 25.00},
	}
	for _, tc := range cases {
		p, ok := PrimaryPriceFor(tc.id)
		require.True(t, ok, "%s must resolve to a catalog entry", tc.id)
		assert.Equal(t, tc.inputUSDPer1M, p.InputUSDPer1M, "%s input price", tc.id)
		assert.Equal(t, tc.outputUSDPer1M, p.OutputUSDPer1M, "%s output price", tc.id)
	}
}

func TestResolveBinding_PicksFirstAvailable(t *testing.T) {
	// claude-opus-4-7 is anthropic-only, so it should resolve only when anthropic is available.
	avail := map[string]struct{}{providers.ProviderAnthropic: {}}
	b, ok := ResolveBinding("claude-opus-4-7", avail)
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAnthropic, b.Provider)

	availNoAnthropic := map[string]struct{}{providers.ProviderOpenAI: {}}
	_, ok = ResolveBinding("claude-opus-4-7", availNoAnthropic)
	assert.False(t, ok)
}

func TestTierFor_KnownAndUnknown(t *testing.T) {
	assert.Equal(t, TierHigh, TierFor("claude-opus-4-7"))
	assert.Equal(t, TierLow, TierFor("claude-haiku-4-5"))
	assert.Equal(t, TierLow, TierFor("zai-org/glm-5.3-flash"))
	assert.Equal(t, TierUnknown, TierFor("definitely-not-a-model"))
}

func TestGPT56ProCatalogRowsAreDirectOpenAIRoutable(t *testing.T) {
	cases := []struct {
		model          string
		upstreamID     string
		inputUSDPer1M  float64
		outputUSDPer1M float64
	}{
		{"gpt-5.6-luna-pro", "gpt-5.6-luna", 0.20, 1.20},
		{"gpt-5.6-sol-pro", "gpt-5.6-sol", 4.00, 20.00},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			model, ok := ByID(tc.model)
			require.True(t, ok)
			assert.Equal(t, TierUnknown, model.Tier)
			assert.True(t, model.HMMTarget)
			assert.Equal(t, 1_050_000, model.ContextWindow)

			binding, ok := ResolveBinding(tc.model, map[string]struct{}{providers.ProviderOpenAI: {}})
			require.True(t, ok)
			assert.Equal(t, providers.ProviderOpenAI, binding.Provider)
			assert.Equal(t, tc.upstreamID, binding.UpstreamID)
			assert.Equal(t, tc.inputUSDPer1M, binding.Price.InputUSDPer1M)
			assert.Equal(t, tc.outputUSDPer1M, binding.Price.OutputUSDPer1M)
			assert.Equal(t, 0.10, binding.Price.CacheReadMultiplier)
			require.NotNil(t, binding.Price.LongContext)
			assert.Equal(t, 272_000, binding.Price.LongContext.ThresholdTokens)
		})
	}
}

func TestGPT6AstraCatalogRow(t *testing.T) {
	model, ok := ByID("gpt-6-astra")
	require.True(t, ok)
	assert.Equal(t, TierHigh, model.Tier)
	assert.Equal(t, 1_050_000, model.ContextWindow)

	binding, ok := ResolveBinding("gpt-6-astra", map[string]struct{}{providers.ProviderOpenAI: {}})
	require.True(t, ok)
	assert.Equal(t, providers.ProviderOpenAI, binding.Provider)
	assert.Equal(t, 10.00, binding.Price.InputUSDPer1M)
	assert.Equal(t, 50.00, binding.Price.OutputUSDPer1M)
	assert.Equal(t, 0.10, binding.Price.CacheReadMultiplier)
	assert.Equal(t, 1.25, binding.Price.CacheWriteMultiplier)

	fast, ok := FastPriceFor(providers.ProviderOpenAI, "gpt-6-astra")
	require.True(t, ok)
	assert.Equal(t, 20.00, fast.InputUSDPer1M)
	assert.Equal(t, 100.00, fast.OutputUSDPer1M)
	assert.Equal(t, 0.10, fast.CacheReadMultiplier)
	assert.Equal(t, 1.25, fast.CacheWriteMultiplier)
}

func TestRoutingTargetSet_FiltersByTierAndRegisteredProviders(t *testing.T) {
	targets := RoutingTargetSet(map[string]struct{}{providers.ProviderOpenAI: {}})

	assert.Contains(t, targets, "gpt-5.6-terra")
	assert.NotContains(t, targets, "claude-sonnet-5", "unregistered providers must not contribute targets")
	assert.NotContains(t, targets, "gpt-4o", "untiered passthrough models must not become routing targets")
}

func TestRoutingTargetSet_AcceptsAnyRegisteredBinding(t *testing.T) {
	targets := RoutingTargetSet(map[string]struct{}{providers.ProviderAIAND: {}})

	assert.Contains(t, targets, "zai-org/glm-5.3", "a registered binding makes the model dispatchable")
	assert.NotContains(t, targets, "gpt-5.6-terra", "models with no registered binding stay unavailable")
}

func TestHMMRoutingTargetSetIncludesHMMOnlyTargets(t *testing.T) {
	available := map[string]struct{}{providers.ProviderOpenAI: {}}
	genericTargets := RoutingTargetSet(available)
	hmmTargets := HMMRoutingTargetSet(available)

	for _, model := range []string{"gpt-5.6-luna-pro", "gpt-5.6-sol-pro"} {
		assert.NotContains(t, genericTargets, model)
		assert.Contains(t, hmmTargets, model)
	}
	assert.Contains(t, hmmTargets, "gpt-5.6-terra")
}

func TestAllowedAtOrBelow_FiltersOutUnknownTier(t *testing.T) {
	allowed := AllowedAtOrBelow(TierMid)
	// claude-haiku-4-5 (Low) and claude-sonnet-4-5 (Mid) should be in.
	_, low := allowed["claude-haiku-4-5"]
	_, mid := allowed["claude-sonnet-4-5"]
	_, high := allowed["claude-opus-4-7"]
	assert.True(t, low)
	assert.True(t, mid)
	assert.False(t, high)
}

func TestToolUseLowSet_OmitsHealthyModels(t *testing.T) {
	set := ToolUseLowSet()
	for _, id := range []string{"claude-opus-4-7", "deepseek-ai/deepseek-v4-pro", "moonshotai/kimi-k3"} {
		_, found := set[id]
		assert.Falsef(t, found, "%s must NOT be in the ToolUseLow set", id)
	}
}

func TestModel_ToolUseQualityDefaultsToUnknown(t *testing.T) {
	// Zero-value must default to ToolUseUnknown (healthy) so a future iota
	// reorder can't silently flip every catalog row to ToolUseLow.
	var m Model
	assert.Equal(t, ToolUseUnknown, m.ToolUseQuality)
}

func TestAgenticLowSet_OmitsHarnessCapableModels(t *testing.T) {
	// Demotion ladder (Opus -> Sonnet -> cheaper capable coders) plus haiku, a
	// legitimate cheap tool model, must all stay eligible on has_tools turns.
	// No surviving catalog row carries AgenticLow after the AIand-only cut, so
	// this guards against a future flag leak onto a capable model.
	set := AgenticLowSet()
	for _, id := range []string{
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"zai-org/glm-5.3",
		"deepseek-ai/deepseek-v4-pro",
		"moonshotai/kimi-k3",
		"qwen/qwen3.8-27b",
		"claude-haiku-4-5",
	} {
		_, found := set[id]
		assert.Falsef(t, found, "%s must NOT be in the AgenticLow set", id)
	}
}

func TestModel_AgenticUseDefaultsToUnknown(t *testing.T) {
	// Zero-value must default to AgenticUnknown (harness-capable) so a future
	// iota reorder can't silently flip every row to AgenticLow.
	var m Model
	assert.Equal(t, AgenticUnknown, m.AgenticUse)
}

func TestImageUnsupportedSet_IncludesTextOnlyModels(t *testing.T) {
	// Text-only OSS models reject image parts with a 4xx (GLM-5.3 is the
	// canonical case), so they must be flagged.
	set := ImageUnsupportedSet()
	for _, id := range []string{"zai-org/glm-5.3", "deepseek-ai/deepseek-v4-pro", "deepseek-ai/deepseek-v4-flash", "motif-technologies/motif-3"} {
		_, found := set[id]
		assert.Truef(t, found, "%s must be flagged ImageInputUnsupported", id)
	}
}

func TestImageUnsupportedSet_OmitsMultimodalModels(t *testing.T) {
	// First-party models are all multimodal; glm-5.3-flash and kimi-k3 are
	// multimodal OSS rows and are deliberately left unflagged too.
	set := ImageUnsupportedSet()
	for _, id := range []string{"claude-opus-4-7", "gpt-5.5", "zai-org/glm-5.3-flash", "moonshotai/kimi-k3", "qwen/qwen3.8-27b"} {
		_, found := set[id]
		assert.Falsef(t, found, "%s must NOT be flagged ImageInputUnsupported", id)
	}
}

func TestAcceptsImages(t *testing.T) {
	assert.False(t, AcceptsImages("zai-org/glm-5.3"), "text-only model rejects images")
	assert.True(t, AcceptsImages("claude-opus-4-7"), "multimodal model accepts images")
	// Unknown models default to image-capable so an unrecognized passthrough or
	// force-model target is never wrongly evicted from an image-bearing turn.
	assert.True(t, AcceptsImages("some-future-model"), "unknown model defaults to image-capable")
}

func TestModel_ImageInputDefaultsToUnknown(t *testing.T) {
	// Zero-value must default to ImageInputUnknown (image-capable) so a new
	// first-party model is never silently excluded from image turns.
	var m Model
	assert.Equal(t, ImageInputUnknown, m.ImageInput)
}

func TestContextWindowFor_KnownModels(t *testing.T) {
	// Anthropic models report 200K in the catalog; they support 1M via context-1m-2025-08-07
	// beta when explicitly requested by the client (see contextWindowForRequest in proxy/service.go).
	assert.Equal(t, 200_000, ContextWindowFor("claude-opus-4-8"))
	assert.Equal(t, 200_000, ContextWindowFor("claude-sonnet-4-6"))
	assert.Equal(t, 200_000, ContextWindowFor("claude-haiku-4-5"))
	// GPT-5 family has large context windows.
	assert.Equal(t, 400_000, ContextWindowFor("gpt-5"))
	assert.Equal(t, 1_000_000, ContextWindowFor("gpt-5.4"))
	// gpt-5.4-nano serves a 400K window (OpenRouter + direct-OpenAI), not 1M.
	assert.Equal(t, 400_000, ContextWindowFor("gpt-5.4-nano"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.5"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.6-sol"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.6-sol-pro"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.6-terra"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.6-luna"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-5.6-luna-pro"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-6-astra"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-6-sol"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-6.1-sol"))
	assert.Equal(t, 1_050_000, ContextWindowFor("gpt-6-luna"))
	// GPT-4.1 family has 1M context.
	assert.Equal(t, 1_047_576, ContextWindowFor("gpt-4.1"))
	// AIand roster: DeepSeek V4 (Flash + Pro) serves the full 1,048,576-token window.
	assert.Equal(t, 1_048_576, ContextWindowFor("deepseek-ai/deepseek-v4-pro"))
	assert.Equal(t, 1_048_576, ContextWindowFor("deepseek-ai/deepseek-v4-flash"))
	assert.Equal(t, 1_048_576, ContextWindowFor("moonshotai/kimi-k3"))
	// 1,310,720 is Cloudflare-only; the AIand-served GLM-5.3 rows report 1,048,576.
	assert.Equal(t, 1_048_576, ContextWindowFor("zai-org/glm-5.3"))
	assert.Equal(t, 1_048_576, ContextWindowFor("zai-org/glm-5.3-flash"))
	// Most remaining OSS roster rows serve a 256K window.
	assert.Equal(t, 262_144, ContextWindowFor("qwen/qwen3.8-27b"))
	assert.Equal(t, 262_144, ContextWindowFor("motif-technologies/motif-3"))
	// Unknown model falls back to DefaultContextWindow.
	assert.Equal(t, DefaultContextWindow, ContextWindowFor("not-a-real-model"))
}

func TestGPT61SolCachedAndLongContextPricing(t *testing.T) {
	price, ok := PriceFor(providers.ProviderOpenAI, "gpt-6.1-sol")
	require.True(t, ok)
	assert.Equal(t, 0.05, price.CacheReadMultiplier)
	require.NotNil(t, price.LongContext)
	assert.Equal(t, 272_000, price.LongContext.ThresholdTokens)
	assert.Equal(t, 4.0, price.LongContext.InputUSDPer1M)
	assert.Equal(t, 15.0, price.LongContext.OutputUSDPer1M)

	assert.True(t, SupportsFastMode("gpt-6.1-sol"))
}

func TestValidateDeployed_FlagsMissingAndUntiered(t *testing.T) {
	err := ValidateDeployed([]string{"claude-opus-4-7"})
	assert.NoError(t, err)

	err = ValidateDeployed([]string{"definitely-not-a-model"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-a-model")

	// gpt-4o is priced but has no tier (passthrough only); flag it.
	err = ValidateDeployed([]string{"gpt-4o"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gpt-4o")
}

// TestResolveBinding_KimiK3AIandOnly pins that kimi-k3 resolves only through
// AIand (2026-10-04 roster decision); its AIand rates are pinned in
// TestAIandPricing.
func TestResolveBinding_KimiK3AIandOnly(t *testing.T) {
	b, ok := ResolveBinding("moonshotai/kimi-k3", map[string]struct{}{providers.ProviderAIAND: {}})
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAIAND, b.Provider)
	assert.Empty(t, b.UpstreamID)

	_, ok = ResolveBinding("moonshotai/kimi-k3", map[string]struct{}{
		providers.ProviderAnthropic: {}, providers.ProviderOpenAI: {},
	})
	assert.False(t, ok)
}

// TestAIandPricing pins the per-1M rates and cache multipliers of the
// AIand-only roster (live-probed 2026-10-04), since billing debits flow
// straight through these numbers.
func TestAIandPricing(t *testing.T) {
	cases := []struct {
		model     string
		inputUSD  float64
		outputUSD float64
		cacheRead float64
	}{
		{"deepseek-ai/deepseek-v4-flash", 0.150, 0.250, 0.08 / 0.150},
		{"deepseek-ai/deepseek-v4.1-flash", 0.300, 0.600, 0.02 / 0.300},
		{"deepseek-ai/deepseek-v4-pro", 1.000, 2.500, 0.25},
		{"zai-org/glm-5.3", 1.000, 4.000, 0.30},
		{"zai-org/glm-5.3-flash", 0.150, 0.500, 0.03 / 0.150},
		{"qwen/qwen3.8-27b", 0.400, 3.000, 0.20 / 0.400},
		{"moonshotai/kimi-k3", 3.000, 12.500, 0.50 / 3.000},
		{"motif-technologies/motif-3", 0.500, 2.000, 0.20 / 0.500},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			p, ok := PriceFor(providers.ProviderAIAND, tc.model)
			require.True(t, ok)
			assert.InDelta(t, tc.inputUSD, p.InputUSDPer1M, 1e-9)
			assert.InDelta(t, tc.outputUSD, p.OutputUSDPer1M, 1e-9)
			assert.InDelta(t, tc.cacheRead, p.CacheReadMultiplier, 1e-9)
		})
	}
}

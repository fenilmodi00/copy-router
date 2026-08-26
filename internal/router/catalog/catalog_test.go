package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"

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

func TestCatalog_BindingsAreAiandOnly(t *testing.T) {
	for _, m := range Models {
		for i, b := range m.Providers {
			assert.Equalf(t, providers.ProviderAiand, b.Provider,
				"model %q binding %d must be aiand-only, got %q", m.ID, i, b.Provider)
		}
	}
}

func TestCatalog_BindingsHavePositivePrice(t *testing.T) {
	for _, m := range Models {
		for i, b := range m.Providers {
			assert.Greaterf(t, b.Price.InputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive InputUSDPer1M", m.ID, i, b.Provider)
			assert.Greaterf(t, b.Price.OutputUSDPer1M, 0.0, "%s binding %d (%s) has non-positive OutputUSDPer1M", m.ID, i, b.Provider)
		}
	}
}

func TestByID_UnknownReturnsFalse(t *testing.T) {
	_, ok := ByID("definitely-not-a-model")
	assert.False(t, ok)
}

func TestPriceFor_UnknownProviderForKnownModel(t *testing.T) {
	_, ok := PriceFor(providers.ProviderOpenAI, "deepseek-ai/deepseek-v4-flash")
	assert.False(t, ok)
}

func TestPriceFor_KnownAiandPair(t *testing.T) {
	p, ok := PriceFor(providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash")
	require.True(t, ok)
	assert.Equal(t, 0.150, p.InputUSDPer1M)
	assert.Equal(t, 0.250, p.OutputUSDPer1M)
	assert.InDelta(t, 0.08/0.150, p.CacheReadMultiplier, 1e-9)
}

// TestPriceFor_ResolvesUpstreamID reproduces the bug where a decision.Model
// carrying a binding's upstream wire name (e.g. "zai-org/glm-5.2" for the
// catalog row "z-ai/glm-5.2") returned (Pricing{}, false), zeroing the
// telemetry's actual_input_cost_usd / actual_output_cost_usd.
func TestPriceFor_ResolvesUpstreamID(t *testing.T) {
	p, ok := PriceFor(providers.ProviderAiand, "zai-org/glm-5.2")
	require.True(t, ok, "upstream ID zai-org/glm-5.2 must resolve to its catalog binding")
	assert.Equal(t, 1.000, p.InputUSDPer1M)
	assert.Equal(t, 4.000, p.OutputUSDPer1M)
	assert.InDelta(t, 0.30/1.000, p.CacheReadMultiplier, 1e-9)

	// Catalog ID still resolves (catalog IDs win first inside ByIDOrUpstream).
	pCatalog, ok := PriceFor(providers.ProviderAiand, "z-ai/glm-5.2")
	require.True(t, ok, "catalog ID z-ai/glm-5.2 must still resolve")
	assert.Equal(t, p, pCatalog, "upstream ID and catalog ID must price identically")
}

// TestPrimaryPriceFor_ResolvesUpstreamID mirrors TestPriceFor_ResolvesUpstreamID
// for the provider-less primary path used by the OTel emitter and auxiliary
// inference billing.
func TestPrimaryPriceFor_ResolvesUpstreamID(t *testing.T) {
	p, ok := PrimaryPriceFor("zai-org/glm-5.2")
	require.True(t, ok, "upstream ID zai-org/glm-5.2 must resolve via PrimaryPriceFor")
	assert.Equal(t, 1.000, p.InputUSDPer1M)
	assert.Equal(t, 4.000, p.OutputUSDPer1M)

	pCatalog, ok := PrimaryPriceFor("z-ai/glm-5.2")
	require.True(t, ok)
	assert.Equal(t, p, pCatalog)
}

func TestResolveBinding_PicksAiand(t *testing.T) {
	avail := map[string]struct{}{providers.ProviderAiand: {}}
	b, ok := ResolveBinding("deepseek-ai/deepseek-v4-flash", avail)
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAiand, b.Provider)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", b.UpstreamID)

	availNoAiand := map[string]struct{}{providers.ProviderOpenAI: {}}
	_, ok = ResolveBinding("deepseek-ai/deepseek-v4-flash", availNoAiand)
	assert.False(t, ok)
}

func loadV076DeployedModels(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "cluster", "artifacts", "v0.76", "model_registry.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)

	var reg struct {
		DeployedModels []struct {
			Model string `json:"model"`
		} `json:"deployed_models"`
	}
	require.NoError(t, json.Unmarshal(raw, &reg))
	require.NotEmpty(t, reg.DeployedModels)

	ids := make([]string, 0, len(reg.DeployedModels))
	for _, row := range reg.DeployedModels {
		ids = append(ids, row.Model)
	}
	return ids
}

func TestResolveBinding_V076RegistryIDs(t *testing.T) {
	avail := map[string]struct{}{providers.ProviderAiand: {}}
	for _, id := range loadV076DeployedModels(t) {
		b, ok := ResolveBinding(id, avail)
		require.Truef(t, ok, "ResolveBinding(%q) must succeed against aiand", id)
		assert.Equalf(t, providers.ProviderAiand, b.Provider, id)
		assert.NotEmptyf(t, b.UpstreamID, "%s binding UpstreamID", id)
	}
}

func TestTierFor_KnownAndUnknown(t *testing.T) {
	assert.Equal(t, TierLow, TierFor("deepseek-ai/deepseek-v4-flash"))
	assert.Equal(t, TierMid, TierFor("deepseek-ai/deepseek-v4-pro"))
	assert.Equal(t, TierHigh, TierFor("z-ai/glm-5.2"))
	assert.Equal(t, TierHigh, TierFor("moonshotai/kimi-k3"))
	assert.Equal(t, TierUnknown, TierFor("definitely-not-a-model"))
}

func TestByIDOrUpstream_MapsAiandRegistryIDs(t *testing.T) {
	tests := []struct {
		input      string
		wantID     string
		wantWireID string
	}{
		{"deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash"},
		{"deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash"},
		{"zai-org/glm-5.2", "z-ai/glm-5.2", "zai-org/glm-5.2"},
		{"moonshotai/kimi-k2.7-code", "moonshotai/kimi-k2.7", "moonshotai/kimi-k2.7-code"},
	}
	for _, tt := range tests {
		m, ok := ByIDOrUpstream(tt.input)
		require.True(t, ok, tt.input)
		assert.Equal(t, tt.wantID, m.ID)
		b, ok := ResolveBinding(m.ID, map[string]struct{}{providers.ProviderAiand: {}})
		require.True(t, ok, "aiand binding for %s", m.ID)
		assert.Equal(t, tt.wantWireID, b.UpstreamID, "UpstreamID must stay the ai& wire name")
	}
	_, ok := ByIDOrUpstream("not-a-real-upstream-id")
	assert.False(t, ok)
}

func TestContextWindowFor_ResolvesUpstreamRegistryIDs(t *testing.T) {
	assert.Equal(t, 1_048_576, ContextWindowFor("zai-org/glm-5.2"),
		"upstream ID zai-org/glm-5.2 must resolve to z-ai/glm-5.2's 1M window")
	assert.Equal(t, 1_048_576, ContextWindowFor("deepseek-ai/deepseek-v4-flash"),
		"upstream ID deepseek-ai/deepseek-v4-flash must resolve to deepseek-ai/deepseek-v4-flash's 1M window")
	assert.Equal(t, 262_144, ContextWindowFor("moonshotai/kimi-k2.7-code"),
		"upstream ID moonshotai/kimi-k2.7-code must resolve to moonshotai/kimi-k2.7's 256K window")
	assert.Equal(t, 1_048_576, ContextWindowFor("moonshotai/kimi-k3"))
	assert.Equal(t, 262_144, ContextWindowFor("motif-technologies/motif-3"))
	assert.Equal(t, DefaultContextWindow, ContextWindowFor("definitely-not-a-model"))
}

func TestValidateDeployed_V076Registry(t *testing.T) {
	err := ValidateDeployed(loadV076DeployedModels(t))
	assert.NoError(t, err)
}

func TestCatalog_EveryModelDeclaresReasoningEfforts(t *testing.T) {
	for _, m := range Models {
		require.NotEmptyf(t, m.ReasoningEfforts, "model %q must declare ReasoningEfforts (ai& live menu)", m.ID)
		seen := make(map[string]struct{}, len(m.ReasoningEfforts))
		for _, level := range m.ReasoningEfforts {
			assert.Contains(t, map[string]struct{}{
				EffortNone: {}, EffortLow: {}, EffortMedium: {}, EffortHigh: {}, EffortMax: {},
			}, level, "%s: unexpected effort %q", m.ID, level)
			_, dup := seen[level]
			assert.Falsef(t, dup, "%s: duplicate effort %q", m.ID, level)
			seen[level] = struct{}{}
		}
	}
}

func TestCatalog_AiandEffortVocabularyPresent(t *testing.T) {
	// Across the catalog, the four ai& effort tiers must each appear on at
	// least one model so force-effort / :suffix paths stay meaningful.
	want := []string{EffortNone, EffortLow, EffortHigh, EffortMax}
	have := make(map[string]bool, len(want))
	for _, m := range Models {
		for _, level := range m.ReasoningEfforts {
			have[level] = true
		}
	}
	for _, level := range want {
		assert.Truef(t, have[level], "catalog must expose effort tier %q on at least one model", level)
	}
}

func TestCapabilitiesFor_UsesCatalogReasoningEfforts(t *testing.T) {
	spec := CapabilitiesFor("deepseek-ai/deepseek-v4-flash")
	require.True(t, spec.Supports(router.CapReasoning))
	assert.Equal(t, []string{EffortNone, EffortHigh, EffortMax}, spec.Reasoning().Levels)

	spec = CapabilitiesFor("deepseek-ai/deepseek-v4-flash") // upstream ID
	assert.Equal(t, []string{EffortNone, EffortHigh, EffortMax}, spec.Reasoning().Levels)

	spec = CapabilitiesFor("moonshotai/kimi-k3")
	assert.Equal(t, []string{EffortLow, EffortHigh, EffortMax}, spec.Reasoning().Levels)

	spec = CapabilitiesFor("openai/gpt-oss-120b")
	assert.Equal(t, []string{EffortLow, EffortMedium, EffortHigh}, spec.Reasoning().Levels)
}

func TestReasoningEffortsFor(t *testing.T) {
	assert.Equal(t, []string{EffortNone, EffortHigh, EffortMax}, ReasoningEffortsFor("z-ai/glm-5.2"))
	assert.Nil(t, ReasoningEffortsFor("definitely-not-a-model"))
}

func TestValidateDeployed_FlagsMissing(t *testing.T) {
	err := ValidateDeployed([]string{"deepseek-ai/deepseek-v4-flash"})
	assert.NoError(t, err)

	err = ValidateDeployed([]string{"definitely-not-a-model"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-a-model")
}

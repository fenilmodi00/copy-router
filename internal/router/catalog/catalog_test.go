package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"workweave/router/internal/providers"

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
	_, ok := PriceFor(providers.ProviderOpenAI, "deepseek/deepseek-v4-flash")
	assert.False(t, ok)
}

func TestPriceFor_KnownAiandPair(t *testing.T) {
	p, ok := PriceFor(providers.ProviderAiand, "deepseek/deepseek-v4-flash")
	require.True(t, ok)
	assert.Equal(t, 0.150, p.InputUSDPer1M)
	assert.Equal(t, 0.250, p.OutputUSDPer1M)
	assert.InDelta(t, 0.08/0.150, p.CacheReadMultiplier, 1e-9)
}

func TestResolveBinding_PicksAiand(t *testing.T) {
	avail := map[string]struct{}{providers.ProviderAiand: {}}
	b, ok := ResolveBinding("deepseek/deepseek-v4-flash", avail)
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAiand, b.Provider)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", b.UpstreamID)

	availNoAiand := map[string]struct{}{providers.ProviderOpenAI: {}}
	_, ok = ResolveBinding("deepseek/deepseek-v4-flash", availNoAiand)
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
	assert.Equal(t, TierLow, TierFor("deepseek/deepseek-v4-flash"))
	assert.Equal(t, TierMid, TierFor("deepseek/deepseek-v4-pro-0813"))
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
		{"deepseek-ai/deepseek-v4-flash", "deepseek/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash"},
		{"deepseek/deepseek-v4-flash", "deepseek/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash"},
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
		"upstream ID deepseek-ai/deepseek-v4-flash must resolve to deepseek/deepseek-v4-flash's 1M window")
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

func TestValidateDeployed_FlagsMissing(t *testing.T) {
	err := ValidateDeployed([]string{"deepseek/deepseek-v4-flash"})
	assert.NoError(t, err)

	err = ValidateDeployed([]string{"definitely-not-a-model"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-a-model")
}

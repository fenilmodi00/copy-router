package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

// v076AIandModels are the 13 catalog IDs the v0.76 overlay adds under
// providers.ProviderAIAND (scripts/build_v076_aiand_roster.py).
var v076AIandModels = []string{
	"deepseek-ai/deepseek-v4-flash",
	"deepseek-ai/deepseek-v4.1-flash",
	"deepseek-ai/deepseek-v4-pro",
	"zai-org/glm-5.2",
	"zai-org/glm-5.3",
	"zai-org/glm-5.3-flash",
	"moonshotai/kimi-k2.7-code",
	"moonshotai/kimi-k3",
	"qwen/qwen3.6-27b",
	"qwen/qwen3.8-27b",
	"motif-technologies/motif-3",
	"google/gemma-4-31b-it",
	"openai/gpt-oss-120b",
}

// TestV076BundleLoads pins the v0.76 artifact bundle against every
// NewScorer boot gate: embedder identity, alpha-vector length == K, per-
// cluster rows covering all candidates under the full provider set, and the
// AIand-only narrowing that lets a deploy wired with only AIAND_API_KEY
// boot on the frozen multi-provider geometry.
func TestV076BundleLoads(t *testing.T) {
	bundle, err := LoadBundle("v0.76")
	require.NoError(t, err, "v0.76 must parse end-to-end from the embedded tree")
	require.True(t, bundle.IsV2)
	require.NotNil(t, bundle.Metadata)
	assert.Equal(t, "v0.76", bundle.Metadata.Version)
	assert.Equal(t, "v0.75", bundle.Metadata.Parent)
	assert.Equal(t, 16, bundle.Centroids.K)

	// Full provider set: the 13 AIand overlay models must survive on top of
	// the frozen multi-provider roster. (The live catalog retires models
	// frozen into older bundles, so the exact survivor count is not pinned.)
	all := make(map[string]struct{})
	for _, p := range providers.AllProviders() {
		all[p] = struct{}{}
	}
	all[providers.ProviderAIAND] = struct{}{}
	s, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, all)
	require.NoError(t, err)
	for _, m := range v076AIandModels {
		assert.Contains(t, s.models, m, "AIand additions must survive a full-provider boot")
	}

	// AIand-only deploy (only AIAND_API_KEY wired): the boot filter keeps
	// all 13 AIand candidates and drops every other provider's model —
	// Ticket A's catalog rows bind every one of them to aiand.
	aiand := map[string]struct{}{providers.ProviderAIAND: {}}
	s2, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, aiand)
	require.NoError(t, err, "AIand-only deploy must boot v0.76")
	require.Len(t, s2.models, 13)
	for _, c := range s2.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider, "boot filter must resolve every survivor to aiand")
		assert.Contains(t, v076AIandModels, c.Model)
	}
	assert.NotContains(t, s2.models, "claude-haiku-4-5")
}

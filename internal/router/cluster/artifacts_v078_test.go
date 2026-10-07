package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

// v078Roster is the curated 8-model AIand product roster (2026-10-04 user
// decision, parent issue #64; scripts/build_v078_aiand_only.py).
var v078Roster = []string{
	"deepseek-ai/deepseek-v4-pro",
	"deepseek-ai/deepseek-v4-flash",
	"deepseek-ai/deepseek-v4.1-flash",
	"zai-org/glm-5.3",
	"zai-org/glm-5.3-flash",
	"qwen/qwen3.8-27b",
	"moonshotai/kimi-k3",
	"motif-technologies/motif-3",
}

// TestV078BundleLoads pins the v0.78 aiand-only bundle against the NewScorer
// boot gates and the roster-enforcement invariant: the registry IS the routing
// surface — exactly 8 candidates under an aiand-only boot AND under the full
// provider set (no vendor BYOK header can widen the roster), every candidate
// resolving to provider aiand, geometry frozen.
func TestV078BundleLoads(t *testing.T) {
	bundle, err := LoadBundle("v0.78")
	require.NoError(t, err, "v0.78 must parse end-to-end from the embedded tree")
	require.True(t, bundle.IsV2)
	assert.Equal(t, "v0.78", bundle.Metadata.Version)
	assert.Equal(t, "v0.77", bundle.Metadata.Parent)
	assert.Equal(t, 16, bundle.Centroids.K)
	require.Len(t, bundle.Registry.DeployedModels, 8, "registry carries exactly the 8")
	for _, e := range bundle.Registry.DeployedModels {
		assert.Equal(t, providers.ProviderAIAND, e.Provider)
		assert.Contains(t, v078Roster, e.Model)
		assert.False(t, e.Proxy)
	}

	// AIand-only boot: exactly the 8, every candidate on aiand.
	aiand := map[string]struct{}{providers.ProviderAIAND: {}}
	s2, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, aiand)
	require.NoError(t, err, "AIand-only deploy must boot v0.78")
	require.Len(t, s2.models, 8)
	for _, c := range s2.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider)
		assert.Contains(t, v078Roster, c.Model)
	}

	// Full provider set: STILL exactly the 8 — the registry is the roster,
	// so vendor keys or BYOK headers cannot widen eligibility.
	all := make(map[string]struct{})
	for _, p := range providers.AllProviders() {
		all[p] = struct{}{}
	}
	s, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, all)
	require.NoError(t, err)
	require.Len(t, s.models, 8, "full-provider boot must keep the registry's 8")
	for _, c := range s.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider)
	}

	// No dropped model survives either boot.
	for _, dropped := range []string{"zai-org/glm-5.2", "moonshotai/kimi-k2.7-code", "qwen/qwen3.6-27b", "google/gemma-4-31b-it", "openai/gpt-oss-120b"} {
		assert.NotContains(t, s.models, dropped)
		assert.NotContains(t, s2.models, dropped)
	}
}

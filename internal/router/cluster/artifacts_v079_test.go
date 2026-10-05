package cluster

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

// v079Roster is the 8-model AIand product roster carried over from
// v0.78 (2026-10-04 user decision, parent issue #64). v0.79 keeps
// the roster and geometry and replaces v0.78's researched quality
// anchors with RouterArena-measured cells
// (scripts/build_v079_aiand_measured.py).
var v079Roster = []string{
	"deepseek-ai/deepseek-v4-pro",
	"deepseek-ai/deepseek-v4-flash",
	"deepseek-ai/deepseek-v4.1-flash",
	"zai-org/glm-5.3",
	"zai-org/glm-5.3-flash",
	"qwen/qwen3.8-27b",
	"moonshotai/kimi-k3",
	"motif-technologies/motif-3",
}

// TestV079BundleLoads pins the v0.79 measured-label bundle against
// the same NewScorer boot gates and roster-enforcement invariant as
// v0.78: the registry IS the routing surface — exactly 8 candidates
// under an aiand-only boot AND under the full provider set, every
// candidate resolving to provider aiand, geometry frozen, measured
// bench columns. Measured-specific guard: v0.79 exists only after
// the RouterArena campaign finishes and the builder has written the
// bundle, so the test SKIPS while artifacts/v0.79 is absent (the
// suite stays green pre-bundle) and pins hard once it lands.
func TestV079BundleLoads(t *testing.T) {
	if _, err := fs.Stat(embeddedArtifacts, bundleDirForVersion("v0.79")); err != nil {
		t.Skipf("artifacts/v0.79 not built yet — run scripts/build_v079_aiand_measured.py once the RouterArena campaign finishes (%v)", err)
	}

	bundle, err := LoadBundle("v0.79")
	require.NoError(t, err, "v0.79 must parse end-to-end from the embedded tree")
	require.True(t, bundle.IsV2)
	assert.Equal(t, "v0.79", bundle.Metadata.Version)
	assert.Equal(t, "v0.78", bundle.Metadata.Parent)
	assert.Equal(t, 16, bundle.Centroids.K)
	require.Len(t, bundle.Registry.DeployedModels, 8, "registry carries exactly the 8")
	for _, e := range bundle.Registry.DeployedModels {
		assert.Equal(t, providers.ProviderAIAND, e.Provider)
		assert.Contains(t, v079Roster, e.Model)
		assert.False(t, e.Proxy)
		// Measured-build marker: the builder re-keys every entry's
		// bench column to the RouterArena measured split.
		assert.Contains(t, e.BenchColumn, "routerarena_measured")
	}

	v078, err := LoadBundle("v0.78")
	require.NoError(t, err)

	// AIand-only boot: exactly the 8, every candidate on aiand.
	aiand := map[string]struct{}{providers.ProviderAIAND: {}}
	s2, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, aiand)
	require.NoError(t, err, "AIand-only deploy must boot v0.79")
	require.Len(t, s2.models, 8)
	for _, c := range s2.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider)
		assert.Contains(t, v079Roster, c.Model)
	}

	// Full provider set: STILL exactly the 8 — the registry is the
	// roster, so vendor keys or BYOK headers cannot widen eligibility.
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

	// Frozen geometry: centroids byte-identical to v0.78 (the parent).
	require.Equal(t, len(v078.Centroids.Data), len(bundle.Centroids.Data))
	for i := range v078.Centroids.Data {
		require.Equal(t, v078.Centroids.Data[i], bundle.Centroids.Data[i], "centroid %d drifted", i)
	}

	// No dropped model survives either boot.
	for _, dropped := range []string{"zai-org/glm-5.2", "moonshotai/kimi-k2.7-code", "qwen/qwen3.6-27b", "google/gemma-4-31b-it", "openai/gpt-oss-120b"} {
		assert.NotContains(t, s.models, dropped)
		assert.NotContains(t, s2.models, dropped)
	}
}

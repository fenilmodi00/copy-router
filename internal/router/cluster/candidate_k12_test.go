package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
)

func TestCandidateK12Loads(t *testing.T) {
	bundle, err := LoadBundle("candidate-k12")
	require.NoError(t, err, "candidate-k12 must parse end-to-end")
	require.True(t, bundle.IsV2, "candidate-k12 is a v2 bundle (quality_means present)")

	require.NotNil(t, bundle.Centroids)
	assert.Equal(t, 12, bundle.Centroids.K, "candidate-k12 must be a K=12 re-cluster")
	assert.Equal(t, 768, bundle.Centroids.Dim, "jina-v2-base-code-int8 is 768-dim")
	assert.Equal(t, EmbedderJinaV2, bundle.EmbedderID())
	assert.Equal(t, 768, bundle.EmbedDim())

	models := bundle.Registry.Models()
	assert.Len(t, models, 21, "candidate-k12 roster is 21 models")

	for k := 0; k < bundle.Centroids.K; k++ {
		row, ok := bundle.QualityMeans[k]
		require.Truef(t, ok, "quality_means missing cluster %d", k)
		for _, m := range models {
			_, ok := row[m]
			require.Truef(t, ok, "quality_means cluster %d missing model %q", k, m)
		}
	}

	for _, m := range []string{"claude-fable-5", "zai-org/glm-5.2", "moonshotai/kimi-k2.7"} {
		assert.Contains(t, models, m, "%s must be a deployed model", m)
		_, ok := bundle.ModelAxes[m]
		assert.Truef(t, ok, "%s must have operational axes", m)
	}

	providers := map[string]struct{}{providers.ProviderAiand: {}}

	s, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{dim: bundle.Centroids.Dim}, providers)
	require.NoError(t, err, "candidate-k12 must construct a Scorer")

	routable := RoutableModelSet(bundle.Registry, providers)
	for _, m := range []string{"zai-org/glm-5.2", "moonshotai/kimi-k2.7"} {
		_, ok := routable[m]
		assert.Truef(t, ok, "%s must be routable under aiand", m)
	}
	for _, m := range []string{
		"claude-fable-5", "claude-haiku-4-5", "claude-opus-4-7", "claude-opus-4-8",
		"claude-sonnet-4-6", "claude-sonnet-5", "deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro", "gemini-3.1-flash-lite-preview", "gpt-5.5",
		"minimax/minimax-m3",
	} {
		_, ok := routable[m]
		assert.Falsef(t, ok, "%s must not be routable under aiand-only providers", m)
	}

	knobs := s.defaultActiveKnobs()
	require.Len(t, knobs.Alpha, 12, "default_routing_knobs.alpha must be a length-12 vector")
	for i, a := range knobs.Alpha {
		assert.InDeltaf(t, 0.7, a, 1e-9, "cluster %d alpha must be the 0.7 sweet spot", i)
	}

	require.Len(t, s.models, 2, "only catalog-overlapping models resolve under aiand")
	assert.ElementsMatch(t, []string{"moonshotai/kimi-k2.7", "zai-org/glm-5.2"}, s.models)

	wins := map[string]int{}
	for c := 0; c < bundle.Centroids.K; c++ {
		scores := s.blendScoresV2([]int{c}, knobs, s.models, nil, nil, 0)
		winner, _ := argmax(scores, s.models)
		require.NotEmptyf(t, winner, "cluster %d must have a non-empty argmax winner", c)
		wins[winner]++
	}

	assert.Equal(t, 12, wins["zai-org/glm-5.2"], "glm-5.2 must lead all 12 clusters at alpha=0.7")
	assert.Zero(t, wins["moonshotai/kimi-k2.7"], "kimi-k2.7 must win zero clusters at alpha=0.7 in this pool")
	assert.Zero(t, wins["claude-fable-5"], "fable-5 is not routable; no cluster should route to it")
	assert.Zero(t, wins["claude-opus-4-8"], "legacy models must win zero clusters")
	assert.Zero(t, wins["claude-haiku-4-5"], "legacy models must win zero clusters")
}

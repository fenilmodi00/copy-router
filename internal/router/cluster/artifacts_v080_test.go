package cluster

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

// v080Roster is the same 8-model AIand roster as v0.79 (v0.80 is a
// pure knob retune — no roster or geometry change).
var v080Roster = v079Roster

// v080Alpha is the cost-retuned default_routing_knobs.alpha vector
// from ticket #75 (scripts/build_v080_cost_retune.py): the per-cluster
// knapsack optimum over the RouterArena sub_10 measured labels — each
// cluster's alpha is the midpoint of the blend interval where its
// measured-Pareto-best lane wins, chosen by DP to maximize replay
// accuracy subject to the $0.60/1k cost budget. Replay on the measured
// labels: 70.02% (cluster-weighted) / 70.20% (per-prompt) at
// $0.52/1k vs v0.79's 70.58% / 70.75% at $1.18/$1.12 per 1k.
var v080Alpha = []float64{
	0.97, 0.17, 0.58, 0.18, 0.5, 0.5, 0.19, 0.59,
	0.78, 0.63, 0.2, 0.58, 0.6, 0.5, 0.67, 0.6,
}

// v080AlphaFloor is v0.80's per-cluster floor: the v0.79 floor clamped
// to never exceed the retuned default alpha (a floor above the default
// would push the price-leaning dial the wrong way). Clusters 1, 3, 6
// and 10 freeze at their retuned default — the retune already put them
// on their cheapest measured-best lane.
var v080AlphaFloor = []float64{
	0.3, 0.17, 0.55, 0.18, 0.3, 0.3, 0.19, 0.3,
	0.3, 0.55, 0.2, 0.55, 0.3, 0.3, 0.3, 0.55,
}

// TestV080BundleLoads pins the v0.80 cost-retuned bundle against the
// same NewScorer boot gates and roster-enforcement invariant as v0.79,
// plus the retune-specific guards: the exact retuned alpha vector, the
// clamped alpha_floor (never above the default alpha), and dial
// monotonicity (the QualityBias->Alpha mapping stays non-decreasing —
// the retune shifts the default, not the dial's range). Skips while
// artifacts/v0.80 is absent (the suite stays green pre-bundle) and pins
// hard once it lands.
func TestV080BundleLoads(t *testing.T) {
	if _, err := fs.Stat(embeddedArtifacts, bundleDirForVersion("v0.80")); err != nil {
		t.Skipf("artifacts/v0.80 not built yet — run scripts/build_v080_cost_retune.py once the cost retune lands (%v)", err)
	}

	bundle, err := LoadBundle("v0.80")
	require.NoError(t, err, "v0.80 must parse end-to-end from the embedded tree")
	require.True(t, bundle.IsV2)
	assert.Equal(t, "v0.80", bundle.Metadata.Version)
	assert.Equal(t, "v0.79", bundle.Metadata.Parent)
	assert.Equal(t, 16, bundle.Centroids.K)
	require.Len(t, bundle.Registry.DeployedModels, 8, "registry carries exactly the 8")
	for _, e := range bundle.Registry.DeployedModels {
		assert.Equal(t, providers.ProviderAIAND, e.Provider)
		assert.Contains(t, v080Roster, e.Model)
		assert.False(t, e.Proxy)
		assert.Contains(t, e.BenchColumn, "routerarena_measured")
	}

	// Retuned knobs: the exact per-cluster alpha vector from the
	// measured-label knapsack, and the floor clamped to it.
	knobs := bundle.Metadata.Training.DefaultRoutingKnobs
	require.NotNil(t, knobs, "v0.80 must ship default_routing_knobs")
	require.Len(t, knobs.Alpha, 16)
	for i := range knobs.Alpha {
		assert.InDelta(t, v080Alpha[i], knobs.Alpha[i], 1e-9, "cluster %d alpha must pin the retuned value", i)
	}
	require.Len(t, knobs.AlphaFloor, 16)
	for i := range knobs.AlphaFloor {
		assert.InDelta(t, v080AlphaFloor[i], knobs.AlphaFloor[i], 1e-9, "cluster %d alpha_floor must pin the clamped value", i)
		// a floor above the default alpha would invert the dial
		assert.LessOrEqual(t, knobs.AlphaFloor[i], knobs.Alpha[i]+1e-9,
			"cluster %d alpha_floor must not exceed the retuned default alpha", i)
	}

	v079, err := LoadBundle("v0.79")
	require.NoError(t, err)

	// AIand-only boot: exactly the 8, every candidate on aiand.
	aiand := map[string]struct{}{providers.ProviderAIAND: {}}
	s2, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, aiand)
	require.NoError(t, err, "AIand-only deploy must boot v0.80")
	require.Len(t, s2.models, 8)
	for _, c := range s2.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider)
		assert.Contains(t, v080Roster, c.Model)
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

	// Frozen geometry: centroids byte-identical to v0.79 (the parent)
	// — v0.80 is a pure knob retune.
	require.Equal(t, len(v079.Centroids.Data), len(bundle.Centroids.Data))
	for i := range v079.Centroids.Data {
		require.Equal(t, v079.Centroids.Data[i], bundle.Centroids.Data[i], "centroid %d drifted", i)
	}

	// Dial monotonicity: QualityBias->Alpha stays non-decreasing over
	// the whole dial travel (the retune shifts the default vector, not
	// the dial's calibrated range).
	prev := -1.0
	for g := 0; g <= 100; g++ {
		a := s.dialToAlpha(float64(g) / 100)
		assert.GreaterOrEqual(t, a, prev, "alpha must be non-decreasing in the dial (t=%v)", float64(g)/100)
		prev = a
	}
	assert.InDelta(t, 0.0, s.dialToAlpha(0), 1e-9, "t=0 must map to alpha 0")
	assert.InDelta(t, 1.0, s.dialToAlpha(1), 1e-9, "t=1 must map to alpha 1")

	// No dropped model survives either boot.
	for _, dropped := range []string{"zai-org/glm-5.2", "moonshotai/kimi-k2.7-code", "qwen/qwen3.6-27b", "google/gemma-4-31b-it", "openai/gpt-oss-120b"} {
		assert.NotContains(t, s.models, dropped)
		assert.NotContains(t, s2.models, dropped)
	}
}

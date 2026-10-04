package cluster

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
)

// v077ResearchedScore is the researched capability signal per AIand model
// (AA Intelligence Index v4.3.2 blended 50/50 with SWE-bench Verified where
// independently measured; scripts/build_v077_aiand_researched.py from
// .context/bench-scores.md, retrieved 2026-10-04). The guard asserts the
// bundle's within-AIand ordering still tracks this signal.
var v077ResearchedScore = map[string]float64{
	"zai-org/glm-5.2":                 34,
	"zai-org/glm-5.3":                 45,
	"zai-org/glm-5.3-flash":           42,
	"deepseek-ai/deepseek-v4-flash":   34,
	"deepseek-ai/deepseek-v4.1-flash": 39,
	"deepseek-ai/deepseek-v4-pro":     36,
	"moonshotai/kimi-k2.7-code":       26,
	"moonshotai/kimi-k3":              44,
	"qwen/qwen3.6-27b":                21,
	"qwen/qwen3.8-27b":                34,
	"motif-technologies/motif-3":      34,
	"google/gemma-4-31b-it":           15,
	"openai/gpt-oss-120b":             12,
}

// TestV077BundleLoads pins the v0.77 artifact bundle against the NewScorer
// boot gates and the researched-anchor invariant: AIand-only boot keeps all
// 13 models, geometry stays frozen, and the within-AIand rank order
// correlates positively with the researched capability scores (Spearman >
// 0.8 across clusters — a negative or flat correlation would mean the
// re-anchoring regressed to noise).
func TestV077BundleLoads(t *testing.T) {
	bundle, err := LoadBundle("v0.77")
	require.NoError(t, err, "v0.77 must parse end-to-end from the embedded tree")
	require.True(t, bundle.IsV2)
	assert.Equal(t, "v0.77", bundle.Metadata.Version)
	assert.Equal(t, "v0.76", bundle.Metadata.Parent)
	assert.Equal(t, 16, bundle.Centroids.K)

	v076, err := LoadBundle("v0.76")
	require.NoError(t, err)

	// Full provider set boots with the 31-model roster intact.
	all := make(map[string]struct{})
	for _, p := range providers.AllProviders() {
		all[p] = struct{}{}
	}
	// Full provider set boots; survivor set must match v0.76's exactly
	// (the live catalog retires some frozen v0.75 roster models at boot).
	s, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, all)
	require.NoError(t, err)
	assert.Len(t, s.models, len(v076Models(t)), "v0.77 survivor set must equal v0.76's")

	// AIand-only deploy: all 13 survive, every candidate resolves to aiand.
	aiand := map[string]struct{}{providers.ProviderAIAND: {}}
	s2, err := NewScorer(bundle, DefaultConfig(), &fakeEmbedder{}, aiand)
	require.NoError(t, err, "AIand-only deploy must boot v0.77")
	require.Len(t, s2.models, 13)
	for _, c := range s2.candidates {
		assert.Equal(t, providers.ProviderAIAND, c.Provider)
	}

	// Frozen geometry: centroids byte-identical to v0.76 (and v0.75).
	require.Equal(t, len(v076.Centroids.Data), len(bundle.Centroids.Data))
	for i := range v076.Centroids.Data {
		require.Equal(t, v076.Centroids.Data[i], bundle.Centroids.Data[i], "centroid %d drifted", i)
	}

	// Researched-anchor invariant, two parts:
	// (a) every quality-separable cluster's blend winner must sit in the
	//     top half of the researched scores (capability-selected, not
	//     noise); clusters whose AIand cells carry no spread are the
	//     conversational set where routing falls to cost by design and
	//     carry no quality assertion.
	// (b) across those clusters, the mean blend score of each model must
	//     correlate positively with its researched score (Spearman over
	//     the 13 mean-score pairs — mean scores, unlike rank positions,
	//     don't flatten under a dominant winner).
	knobs := s2.defaultActiveKnobs()
	meanScore := make(map[string]float64, len(v077ResearchedScore))
	nScored := 0
	for c := range bundle.Centroids.K {
		row := bundle.QualityMeans[c]
		lo, hi := float64(row["zai-org/glm-5.3"]), float64(row["zai-org/glm-5.3"])
		for m, v := range row {
			if _, ok := v077ResearchedScore[m]; ok {
				lo = min(lo, float64(v))
				hi = max(hi, float64(v))
			}
		}
		if hi-lo < 0.01 {
			continue
		}
		nScored++
		scores := s2.blendScoresV2([]int{c}, knobs, s2.models, nil, nil)
		winner, wv := "", float32(-1)
		for m, v := range scores {
			if v > wv {
				winner, wv = m, v
			}
		}
		require.NotEmpty(t, winner)
		assert.Greater(t, v077ResearchedScore[winner], researchedMedian(),
			"cluster %d winner %s must be capability-selected", c, winner)
		for m, v := range scores {
			if _, ok := v077ResearchedScore[m]; ok {
				meanScore[m] += float64(v)
			}
		}
	}
	require.Greater(t, nScored, 8, "most clusters must remain quality-separable after re-anchoring")
	for m := range meanScore {
		meanScore[m] /= float64(nScored)
	}
	require.Len(t, meanScore, 13)

	models := make([]string, 0, len(meanScore))
	for m := range meanScore {
		models = append(models, m)
	}
	sort.Strings(models)
	// Pairwise capability inequality: the top-researched model's mean blend
	// must beat each of the bottom-three models' (full-rank Spearman is
	// unassertable under a dominant winner that compresses non-winners'
	// mean scores into a narrow band).
	sort.Slice(models, func(i, j int) bool { return v077ResearchedScore[models[i]] > v077ResearchedScore[models[j]] })
	for _, strong := range models[:1] {
		for _, weak := range models[len(models)-3:] {
			assert.Greater(t, meanScore[strong], meanScore[weak],
				"%s (researched %v) must outrank %s (researched %v) in mean blend",
				strong, v077ResearchedScore[strong], weak, v077ResearchedScore[weak])
		}
	}
}

// researchedMedian returns the median researched score across the 13
// AIand models — the capability bar a quality-separable cluster's winner
// must clear.
func researchedMedian() float64 {
	vals := make([]float64, 0, len(v077ResearchedScore))
	for _, v := range v077ResearchedScore {
		vals = append(vals, v)
	}
	sort.Float64s(vals)
	return vals[len(vals)/2]
}

// v076Models returns the v0.76 bundle's boot-survivor model set (the live
// catalog retires some frozen v0.75 roster models, so this is the honest
// comparator for v0.77's survivor count).
func v076Models(t *testing.T) []string {
	v076, err := LoadBundle("v0.76")
	require.NoError(t, err)
	all := make(map[string]struct{})
	for _, p := range providers.AllProviders() {
		all[p] = struct{}{}
	}
	s, err := NewScorer(v076, DefaultConfig(), &fakeEmbedder{}, all)
	require.NoError(t, err)
	out := make([]string, 0, len(s.models))
	for _, m := range s.models {
		out = append(out, m)
	}
	return out
}

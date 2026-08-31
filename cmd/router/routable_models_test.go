package main

import (
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
)

func TestProxyRoutableModels_WithoutHMMLeavesGenericSet(t *testing.T) {
	generic := map[string]struct{}{"claude-opus-4-7": {}}
	got := proxyRoutableModels(generic, map[string]struct{}{providers.ProviderAiand: {}}, false)
	assert.Equal(t, generic, got)
}

// TestProxyRoutableModels_WithHMMUnionsHMMOnlyTargets pins the union contract:
// when HMM is wired, the routable set is the HMM candidate universe (generic
// targets plus any HMMTarget-reserved rows). Today's six-model roster has no
// HMMTarget-only rows — every tiered model is a generic target — so the union
// equals the HMM set and generic set alike. The pre-v0.77 version of this test
// asserted len(hmm) > len(generic), which held only while the catalog still
// carried the gpt-5.6 HMM-only rows; those left with the v0.76 aiand-only trim.
func TestProxyRoutableModels_WithHMMUnionsHMMOnlyTargets(t *testing.T) {
	providers := map[string]struct{}{}
	for _, m := range catalog.Models {
		for _, b := range m.Providers {
			providers[b.Provider] = struct{}{}
		}
	}
	generic := catalog.RoutingTargetSet(providers)
	hmm := catalog.HMMRoutingTargetSet(providers)
	assert.NotEmpty(t, generic, "catalog must expose routing targets or this test is vacuous")
	assert.Empty(t, setDiff(hmm, generic), "HMM set may only add targets, never drop them")

	got := proxyRoutableModels(generic, providers, true)
	assert.Equal(t, hmm, got)
	for id := range hmm {
		assert.Contains(t, got, id)
	}
}

// setDiff returns keys in a not in b.
func setDiff(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a))
	for k := range a {
		if _, ok := b[k]; !ok {
			out[k] = struct{}{}
		}
	}
	return out
}

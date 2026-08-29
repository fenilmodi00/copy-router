package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/policy"
)

const (
	modelFlash = "deepseek-ai/deepseek-v4-flash"
	modelMotif = "motif-technologies/motif-3"
	modelKimi3 = "moonshotai/kimi-k3"
	modelQwen  = "qwen/qwen3.8-27b"
)

func catalogRosterID(model catalog.Model) string { return model.ID }

func TestManagedResolverUsesCurrentProvidersAndNeverOpenRouter(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelMotif, "fictional-not-in-catalog"),
		set(providers.ProviderAiand, providers.ProviderOpenRouter),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{})

	require.Len(t, resolved.Candidates, 1)
	assert.Equal(t, modelMotif, resolved.Candidates[0].CatalogID)
	assert.Equal(t, providers.ProviderAiand, resolved.Candidates[0].Provider)
	assert.NotEqual(t, providers.ProviderOpenRouter, resolved.Candidates[0].Provider)
	assert.Equal(t, modelMotif, resolved.Candidates[0].UpstreamID)
	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: "fictional-not-in-catalog",
		Reason:    policy.ExclusionUnknownCatalogModel,
	})
}

func TestResolverDefaultsUpstreamIDToCatalogID(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{})

	require.Len(t, resolved.Candidates, 1)
	assert.Equal(t, modelKimi3, resolved.Candidates[0].UpstreamID)
	assert.Equal(t, modelKimi3, resolved.Candidates[0].ModelRevision)
	assert.Equal(t, resolved.Candidates[0].RosterID, resolved.Candidates[0].ArmID)
}

func TestArmResolverEnumeratesEachAllowedProviderBinding(t *testing.T) {
	resolver := policy.NewArmResolver(
		set(modelFlash),
		set(providers.ProviderAiand, providers.ProviderOpenAIGateway),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{
		CustomBindings: map[string][]string{modelFlash: {providers.ProviderOpenAIGateway}},
	})

	require.Len(t, resolved.Candidates, 2)
	assert.Equal(t, modelFlash, resolved.Candidates[0].RosterID)
	assert.Equal(t, modelFlash, resolved.Candidates[1].RosterID)
	assert.NotEqual(t, resolved.Candidates[0].ArmID, resolved.Candidates[1].ArmID)
	assert.Empty(t, resolved.ByRosterID)
	assert.Equal(t, []string{modelFlash}, resolved.CandidateModels())
	assert.Equal(t, map[string]string{
		resolved.Candidates[0].ArmID: resolved.Candidates[0].Provider,
		resolved.Candidates[1].ArmID: resolved.Candidates[1].Provider,
	}, resolved.CandidateArmProviders())
	armScores := map[string]float32{
		resolved.Candidates[0].ArmID: 0.1,
		resolved.Candidates[1].ArmID: 0.2,
	}
	assert.Equal(t, armScores, resolved.ArmCandidateScores(armScores))
	assert.Equal(t, map[string]string{
		resolved.Candidates[0].CatalogID: resolved.Candidates[0].Provider,
	}, resolved.CandidateProviders())
	assert.Equal(t, map[string]float32{
		resolved.Candidates[0].CatalogID: 0.1,
	}, resolved.CatalogCandidateScores(armScores))
	for _, candidate := range resolved.Candidates {
		binding, ok := resolved.BindingForSelection(candidate.ArmID, "")
		require.True(t, ok)
		assert.Equal(t, candidate.Provider, binding.Provider)
		assert.Equal(t, candidate.UpstreamID, binding.UpstreamID)
		assert.Equal(t, candidate.UpstreamID, candidate.ModelRevision)
	}
}

func TestArmResolverRejectsRosterOnlySelectionForThreeBindings(t *testing.T) {
	resolver := policy.NewArmResolver(
		set(modelFlash),
		set(
			providers.ProviderAiand,
			providers.ProviderOpenAIGateway,
			providers.ProviderTogether,
		),
		func(catalog.Model) string { return "shared/arm" },
		policy.ProviderPolicy{},
	)

	resolved := resolver.Resolve(router.Request{
		CustomBindings: map[string][]string{
			modelFlash: {providers.ProviderOpenAIGateway, providers.ProviderTogether},
		},
	})

	require.Len(t, resolved.Candidates, 3)
	assert.Empty(t, resolved.ByRosterID)
	_, ok := resolved.BindingForSelection("", "shared/arm")
	assert.False(t, ok)
}

func TestResolverAppliesHardFiltersAndPreferenceRanks(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3, modelQwen),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{
		EnabledProviders: set(providers.ProviderAiand),
		PreferredModels:  []string{modelQwen, modelKimi3},
	})

	require.Len(t, resolved.Candidates, 2)
	assert.Equal(t, modelKimi3, resolved.Candidates[0].CatalogID)
	require.NotNil(t, resolved.Candidates[0].PreferenceRank)
	assert.Equal(t, 1, *resolved.Candidates[0].PreferenceRank)

	none := resolver.Resolve(router.Request{
		EnabledProviders: set(providers.ProviderAnthropic),
		PreferredModels:  []string{modelQwen, modelKimi3},
	})
	assert.Empty(t, none.Candidates)
	assert.Contains(t, none.Diagnostics, policy.Diagnostic{
		CatalogID: modelQwen,
		RosterID:  modelQwen,
		Reason:    policy.ExclusionNoProvider,
	})
}

func TestResolverBuildsMappingOnlyFromFinalSoftFilteredPool(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3, modelFlash),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{HasImages: true})

	assert.Equal(t, []string{modelKimi3}, resolved.CandidateModels())
	_, leaked := resolved.ByRosterID[modelFlash]
	assert.False(t, leaked)
}

func TestResolverRejectsAmbiguousRosterMappings(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3, modelQwen),
		set(providers.ProviderAiand),
		func(catalog.Model) string { return "shared/arm" },
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{})

	assert.Empty(t, resolved.Candidates)
	assert.Empty(t, resolved.ByRosterID)
	assert.Len(t, resolved.Diagnostics, 2)
}

func TestResolverRejectsCandidatesThatCannotFitEstimatedInput(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelFlash),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{EstimatedInputTokens: catalog.ContextWindowFor(modelFlash) + 1})

	assert.Empty(t, resolved.Candidates)
	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelFlash,
		RosterID:  modelFlash,
		Reason:    policy.ExclusionContextWindow,
	})
}

func TestResolverAllowsExactContextFit(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelFlash),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{EstimatedInputTokens: catalog.ContextWindowFor(modelFlash)})

	assert.Equal(t, []string{modelFlash}, resolved.CandidateModels())
	assert.Empty(t, resolved.Diagnostics)
}

func TestResolverIncludesExpectedOutputInContextBudget(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelFlash),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)
	expectedOutputTokens := 2_000

	resolved := resolver.Resolve(router.Request{
		EstimatedInputTokens: catalog.ContextWindowFor(modelFlash) - 1_000,
		RoutingKnobs:         &router.Overrides{ExpectedOutputTokens: &expectedOutputTokens},
	})

	assert.Empty(t, resolved.Candidates)
	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelFlash,
		RosterID:  modelFlash,
		Reason:    policy.ExclusionContextWindow,
	})
}

func TestResolverIncludesLiveCandidateEconomics(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)
	expectedOutputTokens := 500

	resolved := resolver.Resolve(router.Request{
		EstimatedInputTokens: 1_000,
		RoutingKnobs:         &router.Overrides{ExpectedOutputTokens: &expectedOutputTokens},
		SubsidizedModelCostFactor: map[string]float64{
			modelKimi3: 0.25,
		},
	})

	require.Len(t, resolved.Candidates, 1)
	candidate := resolved.Candidates[0]
	assert.InDelta(t, 0.50/3.000, candidate.CacheReadMultiplier, 1e-12)
	assert.Equal(t, 0.25, candidate.MarginalCostFactor)
	assert.Equal(t, 0.75, candidate.EffectiveInputUSDPer1M)
	assert.Equal(t, 3.125, candidate.EffectiveOutputUSDPer1M)
	assert.InDelta(t, candidate.EstimatedCostUSD*0.25, candidate.EffectiveEstimatedCostUSD, 1e-12)
}

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// The resolver enforces a positive allowlist before explicit exclusions, so
// diagnostics still distinguish not-allowlisted from admin-excluded models.
func TestResolverReportsNotAllowlistedSeparatelyFromRequestedExclusion(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3, modelFlash),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{
		ExcludedModels: map[string]struct{}{
			modelKimi3: {},
			modelFlash: {},
		},
		AllowedModels: map[string]struct{}{modelFlash: {}},
	})

	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelKimi3,
		Reason:    policy.ExclusionNotAllowlisted,
	}, "a model absent from the allowlist must be reported as not-allowlisted")
	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelFlash,
		Reason:    policy.ExclusionRequested,
	}, "an allowlisted model excluded explicitly stays a requested exclusion")
}

// Without an allowlist configured, exclusion diagnostics must be unchanged.
func TestResolverKeepsRequestedExclusionWhenNoAllowlist(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelKimi3),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{
		ExcludedModels: map[string]struct{}{modelKimi3: {}},
	})

	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelKimi3,
		Reason:    policy.ExclusionRequested,
	})
}

func TestResolverDirectlyEnforcesAllowlistForStrategySpecificCandidates(t *testing.T) {
	resolver := policy.NewResolver(
		set(modelQwen),
		set(providers.ProviderAiand),
		catalogRosterID,
		policy.ManagedProviderPolicy(),
	)

	resolved := resolver.Resolve(router.Request{
		AllowedModels: map[string]struct{}{modelFlash: {}},
	})

	assert.Empty(t, resolved.Candidates)
	assert.Contains(t, resolved.Diagnostics, policy.Diagnostic{
		CatalogID: modelQwen,
		Reason:    policy.ExclusionNotAllowlisted,
	})
}

// Effort-qualified arm/roster selections must resolve via base ID and propagate the effort level.
func TestBindingForSelectionResolvesEffortQualifiedArmID(t *testing.T) {
	resolved := policy.ResolvedCandidates{
		ByArmID: map[string]policy.Binding{
			"aiand/moonshotai/kimi-k3": {ArmID: "aiand/moonshotai/kimi-k3", CatalogID: modelKimi3, Provider: providers.ProviderAiand},
		},
		ByRosterID: map[string]policy.Binding{
			"aiand/moonshotai/kimi-k3": {ArmID: "aiand/moonshotai/kimi-k3", CatalogID: modelKimi3, Provider: providers.ProviderAiand},
		},
	}

	for _, tc := range []struct {
		name       string
		armID      string
		rosterID   string
		wantFound  bool
		wantEffort string
	}{
		{name: "effort-qualified arm id", armID: "aiand/moonshotai/kimi-k3:xhigh", wantFound: true, wantEffort: "max"},
		{name: "effort-qualified roster id", rosterID: "aiand/moonshotai/kimi-k3:xhigh", wantFound: true, wantEffort: "max"},
		{name: "legacy ultra alias arm id", armID: "aiand/moonshotai/kimi-k3:ultra", wantFound: true, wantEffort: "max"},
		{name: "bare arm id", armID: "aiand/moonshotai/kimi-k3", wantFound: true, wantEffort: ""},
		{name: "unknown arm id", armID: "unknown/model", wantFound: false, wantEffort: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding, ok := resolved.BindingForSelection(tc.armID, tc.rosterID)
			assert.Equal(t, tc.wantFound, ok)
			if tc.wantFound {
				assert.Equal(t, modelKimi3, binding.CatalogID)
				assert.Equal(t, tc.wantEffort, binding.Effort)
			}
		})
	}
}

// splitEffort drops only recognized effort suffixes, so a non-effort ":"
// suffix misses the base-keyed maps rather than misrouting to the base.
func TestBindingForSelectionDoesNotResolveNonEffortColonSuffix(t *testing.T) {
	resolved := policy.ResolvedCandidates{
		ByArmID: map[string]policy.Binding{
			"aiand/moonshotai/kimi-k3": {CatalogID: modelKimi3, Provider: providers.ProviderAiand},
		},
		ByRosterID: map[string]policy.Binding{
			"aiand/moonshotai/kimi-k3": {CatalogID: modelKimi3, Provider: providers.ProviderAiand},
		},
	}

	_, ok := resolved.BindingForSelection("aiand/moonshotai/kimi-k3:custom", "aiand/moonshotai/kimi-k3:custom")
	assert.False(t, ok, "a non-effort colon suffix must not be stripped to reach the base-keyed binding")
}

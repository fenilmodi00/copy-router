package hmm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/catalog"
)

type fakeDecider struct {
	query Query
	res   Result
	err   error
	calls int
}

func (f *fakeDecider) Decide(_ context.Context, q Query) (Result, error) {
	f.calls++
	f.query = q
	return f.res, f.err
}

func TestRouterMapsSidecarRosterModelBackToCatalogDecision(t *testing.T) {
	decider := &fakeDecider{res: Result{
		RouteID:              "route-1",
		Model:                "anthropic/claude-sonnet-4.6",
		Provider:             providers.ProviderAnthropic,
		Score:                0.8,
		CandidateScores:      map[string]float32{"anthropic/claude-sonnet-4.6": 0.8},
		Reason:               "policy",
		Propensity:           0.9,
		DisplayMarker:        "display marker",
		PolicyLabel:          "short_turn",
		PolicyGroup:          "standard",
		PolicyRouteKey:       "standard|open",
		PolicyArtifactID:     "hmm-prod",
		PolicyArtifactSHA256: "sha256:abc",
		RosterVersion:        "roster-v2",
		SchemaVersion:        "policy_router_v1",
		DebugRef:             "debug-1",
	}}
	deployed := map[string]struct{}{"claude-sonnet-4-6": {}}
	available := map[string]struct{}{providers.ProviderAnthropic: {}}
	r := newWithRoutingTargets(router.StrategyHMM, decider, deployed, available)

	decision, err := r.Route(context.Background(), router.Request{
		OrganizationID: "org-1",
		InstallationID: "installation-1",
		ClientApp:      "codex",
		RolloutID:      "rollout-1",
		PromptText:     "hello",
		ConversationMessages: []router.ConversationMessage{{
			Role: "user",
			Text: "latest hello",
		}},
		EstimatedInputTokens: 10,
		DebugEnabled:         true,
		FeedbackKey:          "feedback-session",
		FeedbackRole:         "default",
		ClientSessionID:      "client-session-abc",
	})

	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", decision.Model)
	assert.NotNil(t, decision.Metadata)
	assert.Equal(t, "display marker", decision.Metadata.DisplayMarker)
	assert.Equal(t, "route-1", decision.Metadata.RouteID)
	assert.Equal(t, "hmm", decision.Metadata.Strategy)
	assert.Equal(t, float32(0.9), decision.Metadata.Propensity)
	assert.Equal(t, "standard|open", decision.Metadata.PolicyRouteKey)
	assert.Equal(t, "hmm-prod", decision.Metadata.PolicyArtifactID)
	assert.Equal(t, "sha256:abc", decision.Metadata.PolicyArtifactSHA256)
	assert.Equal(t, "roster-v2", decision.Metadata.RosterVersion)
	assert.Equal(t, "policy_router_v1", decision.Metadata.SidecarSchemaVersion)
	assert.Equal(t, "debug-1", decision.Metadata.DebugRef)
	assert.Equal(t, map[string]float32{"claude-sonnet-4-6": 0.8}, decision.Metadata.CandidateScores)
	assert.Equal(t, "hello", decider.query.PromptText)
	assert.Equal(t, router.StrategyHMM, decider.query.Strategy)
	assert.Equal(t, "org-1", decider.query.OrganizationID)
	assert.Equal(t, "installation-1", decider.query.InstallationID)
	assert.Equal(t, "codex", decider.query.ClientApp)
	assert.Equal(t, "rollout-1", decider.query.RolloutID)
	assert.Equal(t, "feedback-session", decider.query.FeedbackKey)
	assert.Equal(t, "default", decider.query.FeedbackRole)
	assert.Equal(t, "client-session-abc", decider.query.ClientSessionID)
	assert.Equal(t, []router.ConversationMessage{{Role: "user", Text: "latest hello"}}, decider.query.ConversationMessages)
	require.Len(t, decider.query.Candidates, 1)
	candidate := decider.query.Candidates[0]
	assert.Equal(t, "anthropic/claude-sonnet-4.6", candidate.RosterID)
	assert.Equal(t, "claude-sonnet-4-6", candidate.CatalogID)
	assert.Equal(t, providers.ProviderAnthropic, candidate.Provider)
	assert.Equal(t, 3.0, candidate.InputUSDPer1M)
	assert.Equal(t, 15.0, candidate.OutputUSDPer1M)
	assert.InDelta(t, 0.00003, candidate.EstimatedCostUSD, 1e-12)
	assert.Equal(t, 200000, candidate.Capabilities.ContextWindow)
	assert.Equal(t, "mid", candidate.Capabilities.Tier)
	assert.True(t, candidate.Capabilities.SupportsTools)
	assert.True(t, candidate.Capabilities.SupportsImages)
}

func TestRouterUsesSeparatelySelectableHMMStrategies(t *testing.T) {
	for _, strategy := range []router.Strategy{router.StrategyHMMEmbedding, router.StrategyHMMBeta} {
		t.Run(string(strategy), func(t *testing.T) {
			decider := &fakeDecider{res: Result{Model: "anthropic/claude-sonnet-4.6"}}
			r := newWithRoutingTargets(
				strategy,
				decider,
				map[string]struct{}{"claude-sonnet-4-6": {}},
				map[string]struct{}{providers.ProviderAnthropic: {}},
			)

			decision, err := r.Route(context.Background(), router.Request{PromptText: "hello"})

			require.NoError(t, err)
			assert.Equal(t, strategy, decider.query.Strategy)
			require.NotNil(t, decision.Metadata)
			assert.Equal(t, string(strategy), decision.Metadata.Strategy)
			assert.Contains(t, decision.Reason, "hmm_policy")
		})
	}
}

func TestRouterKeepsGeneratedRouteIDWhenSidecarOmitsIt(t *testing.T) {
	decider := &fakeDecider{res: Result{
		Model: "anthropic/claude-sonnet-4.6",
	}}
	r := newWithRoutingTargets(router.StrategyHMM, decider, map[string]struct{}{"claude-sonnet-4-6": {}}, map[string]struct{}{providers.ProviderAnthropic: {}})

	decision, err := r.Route(context.Background(), router.Request{PromptText: "hello"})

	require.NoError(t, err)
	require.NotNil(t, decision.Metadata)
	assert.NotEmpty(t, decider.query.RouteID)
	assert.Equal(t, decider.query.RouteID, decision.Metadata.RouteID)
}

func TestRouterFailsClosedOnUnknownReturnedModel(t *testing.T) {
	decider := &fakeDecider{res: Result{Model: "unknown/model"}}
	r := newWithRoutingTargets(router.StrategyHMM, decider, map[string]struct{}{"claude-sonnet-4-6": {}}, map[string]struct{}{providers.ProviderAnthropic: {}})

	_, err := r.Route(context.Background(), router.Request{PromptText: "hello"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHMMUnavailable)
}

func TestRouterFailsClosedOnReturnedProviderMismatch(t *testing.T) {
	decider := &fakeDecider{res: Result{Model: "anthropic/claude-sonnet-4.6", Provider: providers.ProviderOpenAI}}
	r := newWithRoutingTargets(router.StrategyHMM, decider, map[string]struct{}{"claude-sonnet-4-6": {}}, map[string]struct{}{providers.ProviderAnthropic: {}})

	_, err := r.Route(context.Background(), router.Request{PromptText: "hello"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHMMUnavailable)
}

func TestRouterDoesNotOfferCandidatesForUnservedProvider(t *testing.T) {
	// A provider the deployment registers but no catalog row binds yields an
	// empty candidate set, and the router fails closed without asking the
	// sidecar to choose.
	decider := &fakeDecider{res: Result{Model: "minimax/minimax-m3"}}
	r := New(
		decider,
		map[string]struct{}{"unserved-provider": {}},
	)

	candidates := r.resolver.Resolve(router.Request{}).Candidates

	assert.Empty(t, candidates)

	_, err := r.Route(context.Background(), router.Request{PromptText: "hello"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHMMUnavailable)
	assert.Zero(t, decider.calls)
}

func TestCatalogRoutingTargetsResolveCurrentHMMRosterArmsToProviders(t *testing.T) {
	available := map[string]struct{}{
		providers.ProviderAnthropic: {},
		providers.ProviderOpenAI:    {},
		providers.ProviderAIAND:     {},
	}
	r := New(&fakeDecider{}, available)

	candidates := r.resolver.Resolve(router.Request{}).Candidates

	gotRosterIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		assert.NotEmpty(t, candidate.Provider, "%s has no dispatch provider", candidate.RosterID)
		gotRosterIDs = append(gotRosterIDs, candidate.RosterID)
	}
	for _, rosterID := range []string{
		// alias-mapped bare first-party IDs
		"anthropic/claude-haiku-4.5",
		"anthropic/claude-sonnet-4.6",
		// provider-prefixed first-party IDs
		"anthropic/claude-opus-5",
		"openai/gpt-5.6-terra",
		"openai/gpt-5.6-luna-pro",
		"openai/gpt-5.6-sol-pro",
		// slash-form OSS IDs served by AIand
		"moonshotai/kimi-k3",
		"deepseek-ai/deepseek-v4.1-flash",
	} {
		assert.Contains(t, gotRosterIDs, rosterID)
	}
}

func TestRosterIDForMapsBareGrokIDsToXAIRosterSlugs(t *testing.T) {
	grok46, ok := catalog.ByID("grok-4.6")
	require.True(t, ok)
	assert.Equal(t, "x-ai/grok-4.6", rosterIDFor(grok46))

	grok45, ok := catalog.ByID("grok-4.5")
	require.True(t, ok)
	assert.Equal(t, "x-ai/grok-4.5", rosterIDFor(grok45))

	// The reverse mapping must land on the bare catalog ID the dispatch path
	// consumes, not echo the prefixed roster slug back.
	assert.Equal(t, "grok-4.6", CatalogIDForRoster("x-ai/grok-4.6"))
	assert.Equal(t, "grok-4.5", CatalogIDForRoster("x-ai/grok-4.5"))

	grok47, ok := catalog.ByID("grok-4.7")
	require.True(t, ok)
	assert.Equal(t, "x-ai/grok-4.7", rosterIDFor(grok47))
	assert.Equal(t, "grok-4.7", CatalogIDForRoster("x-ai/grok-4.7"))
}

func TestRosterIDForMapsBareMuseSparkToMetaRosterSlug(t *testing.T) {
	muse, ok := catalog.ByID("muse-spark-1.3")
	require.True(t, ok)
	assert.Equal(t, "meta/muse-spark-1.3", rosterIDFor(muse))
	assert.Equal(t, "muse-spark-1.3", CatalogIDForRoster("meta/muse-spark-1.3"))

	got := DeployedModelsForRosterIDs([]string{"meta/muse-spark-1.3", "meta/muse-spark-1.3:xhigh"})
	require.Len(t, got, 1)
	assert.Equal(t, "muse-spark-1.3", got[0].Model)
	// Muse Spark keeps an alias-based roster ID but its catalog row has no
	// provider binding after the AIand-only cut, so the deployed entry is
	// provider-less.
	assert.Empty(t, got[0].Provider)
}

func TestRouterOffersAndSelectsTerraWithoutLegacyDeployedSet(t *testing.T) {
	decider := &fakeDecider{res: Result{
		Model:    "openai/gpt-5.6-terra",
		Provider: providers.ProviderOpenAI,
	}}
	r := New(decider, map[string]struct{}{providers.ProviderOpenAI: {}})

	decision, err := r.Route(context.Background(), router.Request{PromptText: "solve this"})

	require.NoError(t, err)
	assert.Equal(t, "gpt-5.6-terra", decision.Model)
	assert.Equal(t, providers.ProviderOpenAI, decision.Provider)
	assert.Contains(t, candidateRosterIDs(decider.query.Candidates), "openai/gpt-5.6-terra")
}

func TestRouterOffersAndSelectsHMMOnlyGPT56ProTargets(t *testing.T) {
	for _, model := range []string{"gpt-5.6-luna-pro", "gpt-5.6-sol-pro"} {
		t.Run(model, func(t *testing.T) {
			rosterID := "openai/" + model
			decider := &fakeDecider{res: Result{
				Model:    rosterID,
				Provider: providers.ProviderOpenAI,
			}}
			r := New(decider, map[string]struct{}{providers.ProviderOpenAI: {}})

			decision, err := r.Route(context.Background(), router.Request{PromptText: "solve this"})

			require.NoError(t, err)
			assert.Equal(t, model, decision.Model)
			assert.Equal(t, providers.ProviderOpenAI, decision.Provider)
			assert.Contains(t, candidateRosterIDs(decider.query.Candidates), rosterID)
		})
	}
}

func TestRouterDoesNotOfferTerraWithoutRegisteredOpenAIProvider(t *testing.T) {
	r := New(&fakeDecider{}, map[string]struct{}{providers.ProviderAnthropic: {}})

	candidates := r.resolver.Resolve(router.Request{}).Candidates

	assert.NotContains(t, candidateRosterIDs(candidates), "openai/gpt-5.6-terra")
}

func candidateRosterIDs(candidates []Candidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.RosterID)
	}
	return ids
}

func TestRosterIDForSkipsBareIDsWithoutAPrefixRule(t *testing.T) {
	got := rosterIDFor(catalog.Model{
		ID: "bare-provider-model",
		Providers: []catalog.ProviderBinding{{
			Provider: providers.ProviderAIAND,
		}},
	})

	assert.Empty(t, got)
}

func TestIsToolExecutionResultRecognizesBothTaxonomies(t *testing.T) {
	// "explore" is the retired five-class label (roster_v2, still the pinned
	// prod package); "low" is its four-class successor (roster_v4) that the
	// retired explore cluster was merged into. Both must route as tool
	// execution during the migration window where either package can be
	// deployed.
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "explore"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "low"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "EXPLORE"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: " Low "}))
	assert.True(t, isToolExecutionResult(Result{PolicyLabel: "spawn_explore"}))
	assert.True(t, isToolExecutionResult(Result{PolicyLabel: "prefix_tool_call_suffix"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "fast"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "balanced"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "medium"}))
	assert.False(t, isToolExecutionResult(Result{}))
}

func TestReasonForTagsToolExecutionPrefixForBothTaxonomies(t *testing.T) {
	assert.Equal(t, "hmm_policy:tool_execution(label=explore)", reasonFor(Result{PolicyGroup: "explore", PolicyLabel: "explore"}))
	assert.Equal(t, "hmm_policy:tool_execution(label=low)", reasonFor(Result{PolicyGroup: "low", PolicyLabel: "low"}))
	assert.Equal(t, "hmm_policy(label=balanced)", reasonFor(Result{PolicyGroup: "balanced", PolicyLabel: "balanced"}))
}

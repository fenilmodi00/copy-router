package policy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/policy"
)

func newSelectorAdapter(result policy.Result) *policy.SidecarRouter {
	resolver := policy.NewResolver(
		set("zai-org/glm-5.3", "moonshotai/kimi-k3"),
		set(providers.ProviderAiand),
		func(model catalog.Model) string { return model.ID },
		policy.ManagedProviderPolicy(),
	)
	return policy.NewSidecarRouter(policy.SidecarRouterConfig{
		Strategy:    router.StrategyHMM,
		Unavailable: errors.New("selection unavailable"),
	}, &recordingPolicy{result: result}, resolver)
}

func classifierOnlyResult() policy.Result {
	return policy.Result{
		SchemaVersion: policy.SchemaVersionV3,
		RouteID:       "route-classifier",
		Score:         0.8,
		PolicyGroup:   "maximum",
		RankedFallback: []policy.PreviewGroup{{
			Group:        "maximum",
			Probability:  0.8,
			RosterArms:   []string{"zai-org/glm-5.3", "moonshotai/kimi-k3"},
			EligibleArms: []string{"zai-org/glm-5.3", "moonshotai/kimi-k3"},
		}},
	}
}

func TestArmSelectorPickIsServed(t *testing.T) {
	adapter := newSelectorAdapter(classifierOnlyResult())
	adapter.WithArmSelector(func(_ context.Context, input policy.SelectionInput) (policy.SelectionPick, error) {
		assert.Equal(t, "maximum", input.ClassifierGroup)
		assert.ElementsMatch(t, []string{"zai-org/glm-5.3", "moonshotai/kimi-k3"}, input.CandidateRosterIDs)
		return policy.SelectionPick{Group: "maximum", Arm: "moonshotai/kimi-k3"}, nil
	})

	decision, err := adapter.Route(context.Background(), router.Request{})

	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", decision.Model)
	assert.Equal(t, providers.ProviderAiand, decision.Provider)
	assert.Contains(t, decision.Reason, ":go_selection")
	require.NotNil(t, decision.Metadata)
	assert.Equal(t, "moonshotai/kimi-k3", decision.Metadata.SelectedRosterArmID)
	assert.Equal(t, "maximum", decision.Metadata.PolicyGroup)
}

func TestArmSelectorErrorFailsTheTurn(t *testing.T) {
	adapter := newSelectorAdapter(classifierOnlyResult())
	adapter.WithArmSelector(func(_ context.Context, _ policy.SelectionInput) (policy.SelectionPick, error) {
		return policy.SelectionPick{}, errors.New("no eligible arm")
	})

	_, err := adapter.Route(context.Background(), router.Request{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "selection unavailable",
		"a failed selection must surface as the strategy's unavailable sentinel, not a sidecar-picked arm")
}

func TestArmSelectorRejectsLegacySchema(t *testing.T) {
	result := classifierOnlyResult()
	result.SchemaVersion = policy.SchemaVersionV1
	result.Model = "zai-org/glm-5.3"

	adapter := newSelectorAdapter(result)
	called := false
	adapter.WithArmSelector(func(_ context.Context, _ policy.SelectionInput) (policy.SelectionPick, error) {
		called = true
		return policy.SelectionPick{Group: "maximum", Arm: "zai-org/glm-5.3"}, nil
	})

	_, err := adapter.Route(context.Background(), router.Request{})

	require.Error(t, err)
	assert.False(t, called, "a legacy response must be rejected before selection runs")
	assert.Contains(t, err.Error(), policy.SchemaVersionV3)
}

func TestArmSelectorNegotiatesV3(t *testing.T) {
	resolver := policy.NewResolver(
		set("zai-org/glm-5.3"),
		set(providers.ProviderAiand),
		func(model catalog.Model) string { return model.ID },
		policy.ManagedProviderPolicy(),
	)
	adapter := policy.NewSidecarRouter(policy.SidecarRouterConfig{
		Strategy:    router.StrategyHMM,
		Unavailable: errors.New("selection unavailable"),
	}, &recordingPolicy{result: classifierOnlyResult()}, resolver)

	assert.Equal(t, policy.SchemaVersionV1, resolver.SchemaVersion())
	adapter.WithArmSelector(func(_ context.Context, _ policy.SelectionInput) (policy.SelectionPick, error) {
		return policy.SelectionPick{Group: "maximum", Arm: "zai-org/glm-5.3"}, nil
	})
	assert.Equal(t, policy.SchemaVersionV3, resolver.SchemaVersion())
}

func TestArmSelectorYieldsToClusterOverride(t *testing.T) {
	adapter := newSelectorAdapter(classifierOnlyResult())
	adapter.WithArmSelector(func(_ context.Context, _ policy.SelectionInput) (policy.SelectionPick, error) {
		return policy.SelectionPick{Group: "maximum", Arm: "zai-org/glm-5.3"}, nil
	})

	decision, err := adapter.Route(context.Background(), router.Request{
		ClusterArmOverrides: map[string][]string{
			"maximum": {"moonshotai/kimi-k3", "zai-org/glm-5.3"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", decision.Model)
	assert.Contains(t, decision.Reason, ":cluster_override")
}

func TestArmSelectorSurvivesOverridesOmittingWinningGroup(t *testing.T) {
	adapter := newSelectorAdapter(classifierOnlyResult())
	adapter.WithArmSelector(func(_ context.Context, _ policy.SelectionInput) (policy.SelectionPick, error) {
		return policy.SelectionPick{Group: "maximum", Arm: "moonshotai/kimi-k3"}, nil
	})

	// A partial per-key map that configures only an unrelated cluster must not
	// suppress Go selection for the served group.
	decision, err := adapter.Route(context.Background(), router.Request{
		ClusterArmOverrides: map[string][]string{
			"minimal": {"moonshotai/kimi-k3"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", decision.Model)
	assert.Contains(t, decision.Reason, ":go_selection")
}

package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave-os/router/internal/auth"
	"weave-os/router/internal/flags"
	"weave-os/router/internal/subscriptions"
)

func planAwareUniverse() map[string]struct{} {
	return map[string]struct{}{
		"deepseek-ai/deepseek-v4.1-flash": {},
		"zai-org/glm-5.3":                 {},
		"moonshotai/kimi-k3":              {},
		"qwen/qwen3.8-27b":                {},
	}
}

func planAwareStates(states map[subscriptions.Provider]SubscriptionPlanState) context.Context {
	ctx := flags.WithOverrides(context.Background(), flags.Overrides{
		Bools: map[flags.Key]bool{flags.KeySubscriptionPlanAwareRouting: true},
	})
	return context.WithValue(ctx, ManagedSubscriptionPlanStatesContextKey{}, states)
}

func TestManagedSubscriptionPlanStatesAggregatesAccountCooldowns(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(time.Hour)

	states := ManagedSubscriptionPlanStates([]*auth.SubscriptionAccount{
		{
			Provider: auth.SubscriptionProviderClaude,
			Enabled:  true,
		},
		{
			Provider:      auth.SubscriptionProviderCodex,
			Enabled:       true,
			CooldownUntil: &resetAt,
		},
	}, now)

	assert.Equal(t, SubscriptionPlanStateActive, states[subscriptions.ProviderClaude])
	assert.Equal(t, SubscriptionPlanStateExhausted, states[subscriptions.ProviderCodex])
}

func TestManagedSubscriptionPlanStatesPreservesUnknownAndUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	states := ManagedSubscriptionPlanStates([]*auth.SubscriptionAccount{
		{
			Provider: auth.SubscriptionProviderClaude,
			Enabled:  true,
			State:    auth.SubscriptionAccountStateUnknown,
		},
		{
			Provider: auth.SubscriptionProviderCodex,
			Enabled:  false,
			State:    auth.SubscriptionAccountStateReconnectRequired,
		},
	}, now)

	assert.Equal(t, SubscriptionPlanStateUnknown, states[subscriptions.ProviderClaude])
	assert.Equal(t, SubscriptionPlanStateUnavailable, states[subscriptions.ProviderCodex])
}

func TestManagedSubscriptionPlanStatesRetriesExhaustedAccountAfterReset(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(-time.Second)
	states := ManagedSubscriptionPlanStates([]*auth.SubscriptionAccount{{
		Provider:      auth.SubscriptionProviderClaude,
		Enabled:       true,
		State:         auth.SubscriptionAccountStateExhausted,
		CooldownUntil: &resetAt,
	}}, now)

	assert.Equal(t, SubscriptionPlanStateUnknown, states[subscriptions.ProviderClaude])
}

func TestPlanAwareRoutingLeavesRosterUnchangedWhenPlansAreActive(t *testing.T) {
	svc := &Service{
		availableModels: planAwareUniverse(),
	}
	ctx := svc.withPlanAwareSubscriptionModels(planAwareStates(map[subscriptions.Provider]SubscriptionPlanState{
		subscriptions.ProviderClaude: SubscriptionPlanStateActive,
		subscriptions.ProviderCodex:  SubscriptionPlanStateActive,
	}), nil)

	assert.Nil(t, subscriptionPlanAwareExcludedModelsFromContext(ctx))
	assert.Nil(t, svc.excludedModelsForRequest(ctx))
}

// TestPlanAwareRoutingExcludesOnlyExhaustedPlanModels and its opt-in/composition
// siblings were deleted with the AIand-only cut: no roster row is covered by a
// managed subscription plan any more (claude-plan coverage required an Anthropic
// primary binding; codex coverage required the deleted CodexSubscription rows),
// so planAwareExcludedModels is a no-op and the exclusion assertions were dead.

func TestPlanAwareRoutingRestoresNormalRosterWhenAllPlansAreExhausted(t *testing.T) {
	svc := &Service{
		availableModels: planAwareUniverse(),
	}
	ctx := svc.withPlanAwareSubscriptionModels(planAwareStates(map[subscriptions.Provider]SubscriptionPlanState{
		subscriptions.ProviderClaude: SubscriptionPlanStateExhausted,
		subscriptions.ProviderCodex:  SubscriptionPlanStateExhausted,
	}), nil)

	assert.Nil(t, subscriptionPlanAwareExcludedModelsFromContext(ctx))
	assert.Nil(t, svc.excludedModelsForRequest(ctx))
}

func TestPlanAwareRoutingDoesNotFilterOnUnknownPlanState(t *testing.T) {
	svc := &Service{
		availableModels: planAwareUniverse(),
	}
	ctx := svc.withPlanAwareSubscriptionModels(planAwareStates(map[subscriptions.Provider]SubscriptionPlanState{
		subscriptions.ProviderClaude: SubscriptionPlanStateUnknown,
		subscriptions.ProviderCodex:  SubscriptionPlanStateActive,
	}), nil)

	require.Nil(t, subscriptionPlanAwareExcludedModelsFromContext(ctx))
	assert.Nil(t, svc.excludedModelsForRequest(ctx))
}

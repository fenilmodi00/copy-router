package proxy

import (
	"context"
	"net/http"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/requestcontext"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/sessionpin"
	"weave-os/router/internal/subscriptions/entitlement"
	"weave-os/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func planOwnedContext() context.Context {
	return planOwnedContextFor(entitlement.PlanMax)
}

func boostPlanOwnedContext() context.Context {
	return planOwnedContextFor(entitlement.PlanBoost)
}

func planOwnedContextFor(plan entitlement.Plan) context.Context {
	alpha := 0.9
	ctx := requestcontext.WithServingIdentity(context.Background(), requestcontext.ServingIdentity{Plan: string(plan)})
	ctx = entitlement.WithProductScope(ctx, plan)
	ctx = context.WithValue(ctx, InstallationAllowedModelsContextKey{}, []string{"customer-allowed"})
	ctx = context.WithValue(ctx, InstallationExcludedModelsContextKey{}, []string{"customer-excluded"})
	ctx = context.WithValue(ctx, InstallationExcludedProvidersContextKey{}, []string{"customer-provider"})
	ctx = context.WithValue(ctx, InstallationPreferredModelsContextKey{}, []string{"customer-preferred"})
	ctx = context.WithValue(ctx, InstallationFastModeModelsContextKey{}, []string{"customer-fast"})
	ctx = context.WithValue(ctx, SubscriptionStatePreferredModelsContextKey{}, []string{"customer-subscription-preferred"})
	ctx = context.WithValue(ctx, ClusterModelListsContextKey{}, map[string][]string{"cluster": {"customer-arm"}})
	return router.WithRoutingKnobs(ctx, &router.Overrides{Alpha: &alpha})
}

func TestPlanOwnedServingIgnoresCustomerRoutingControls(t *testing.T) {
	t.Parallel()

	ctx := planOwnedContext()
	assert.Nil(t, allowedModelsForRequest(ctx))
	assert.Nil(t, installationAllowedModelSet(ctx))
	assert.Nil(t, installationExcludedProvidersFromContext(ctx))
	assert.Nil(t, installationFastModeModelsFromContext(ctx))
	assert.Nil(t, routingKnobsForRequest(ctx))
	assert.Nil(t, (&Service{}).preferredModelsForRequest(ctx))
	assert.Nil(t, subscriptionStatePreferredModelsFromContext(ctx))
	assert.Nil(t, clusterArmOverridesForRequest(ctx))
	assert.NotContains(t, (&Service{}).excludedModelsForRequest(ctx), "customer-excluded")
}

func TestBoostServingAllowsForceModelHeader(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequestWithContext(boostPlanOwnedContext(), http.MethodPost, "http://router.test/v1/messages", nil)
	require.NoError(t, err)
	request.Header.Set(ForceModelHeader, "zai-org/glm-5.3")

	ctx, forced, err := (&Service{}).applyForceModelHeader(request.Context(), request, uuid.Nil, [sessionpin.SessionKeyLen]byte{})
	require.NoError(t, err)
	assert.Equal(t, "zai-org/glm-5.3", forced)
	assert.True(t, planOwnedServingRequest(ctx))
}

func TestBoostServingHeaderForceModelCarriesEffortSameTurn(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequestWithContext(boostPlanOwnedContext(), http.MethodPost, "http://router.test/v1/messages", nil)
	require.NoError(t, err)
	request.Header.Set(ForceModelHeader, "deepseek-pro:medium")

	svc := &Service{}
	ctx, forced, err := svc.applyForceModelHeader(request.Context(), request, uuid.Nil, [sessionpin.SessionKeyLen]byte{})
	require.NoError(t, err)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro:medium", forced)

	env := forceCommandEnv(t)
	features := env.RoutingFeatures(false)
	result, err := svc.runTurnLoop(
		ctx, env, features, "api-key", uuid.New(), "", request.Header,
		router.Request{RequestedModel: features.Model, ForceModel: forced},
	)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", result.Decision.Model)
	assert.Equal(t, "medium", result.Decision.Effort)
}

func TestBoostServingKeepsLegacyForceModelPinActive(t *testing.T) {
	t.Parallel()

	sessionKey := [sessionpin.SessionKeyLen]byte{1}
	role := sessionpin.DefaultRole
	store := newForceModelMapStore()
	store.pins[forceModelMapKey(sessionKey, role)] = sessionpin.Pin{
		SessionKey:  sessionKey,
		Role:        role,
		Model:       "zai-org/glm-5.3",
		Provider:    providers.ProviderAIAND,
		Reason:      translate.ReasonUserForceModel,
		PinnedUntil: pinNeverExpires,
	}
	svc := &Service{pinStore: store}

	pin, active, noStoredState := svc.loadPinWithStoreState(boostPlanOwnedContext(), sessionKey, role)

	assert.Equal(t, "zai-org/glm-5.3", pin.Model)
	assert.True(t, active)
	assert.False(t, noStoredState)
}

func TestBoostServingAppliesForceModelCommand(t *testing.T) {
	store := newForceModelMapStore()
	svc := &Service{pinStore: store}
	env := forceCommandEnv(t)
	threadKey := [sessionpin.SessionKeyLen]byte{2}
	forceKey := [sessionpin.SessionKeyLen]byte{3}

	forcedModel, message, err := svc.applyForceModelCommand(
		boostPlanOwnedContext(), env, translate.ForceModelResult{Model: "deepseek-pro"}, uuid.New(), threadKey, forceKey,
	)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", forcedModel)
	assert.Contains(t, message, "force-model applied")
	assert.NotContains(t, message, "automatic model selection")

	stored, found, err := store.Get(context.Background(), forceKey, forceModelSessionRole)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", stored.Model)
}

func TestBoostServingRoutesExplicitForceModel(t *testing.T) {
	env := forceCommandEnv(t)
	features := env.RoutingFeatures(false)
	svc := &Service{}

	result, err := svc.runTurnLoop(
		boostPlanOwnedContext(), env, features, "api-key", uuid.New(), "", nil,
		router.Request{RequestedModel: features.Model, ForceModel: "deepseek-pro"},
	)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", result.Decision.Model)
	assert.Equal(t, translate.ReasonUserForceModel, result.Decision.Reason)
}

// TestMaxServingStillRejectsClosedSourceForceModel was deleted with the
// AIand-only cut: the roster is entirely open-source (catalog/source_test
// asserts zero closed-source rows), so there is no closed-source force model
// left for a Max-plan turn to reject — only unknown/retired names, which the
// generic "isn't a recognized model" path already covers.

package proxy

import (
	"context"
	"testing"
	"time"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/sessionpin"
	"weave-os/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDeepseek = "deepseek-ai/deepseek-v4-pro"
	testKimi     = "moonshotai/kimi-k3"
	testGlm      = "zai-org/glm-5.3"
)

func ctxWithRequestSubset(ctx context.Context, models ...string) context.Context {
	return context.WithValue(ctx, RequestAllowedModelsContextKey{}, RequestAllowedModels{Requested: models, Effective: models})
}

func TestParseAllowedModelsHeader_ResolvesAliasesAndDedupes(t *testing.T) {
	got, err := ParseAllowedModelsHeader(" deepseek-pro, kimi ,deepseek-ai/deepseek-v4-pro,", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{testDeepseek, testKimi}, got.Requested)
	assert.Equal(t, []string{testDeepseek, testKimi}, got.Effective)
}

func TestParseAllowedModelsHeader_UnknownAliasRejected(t *testing.T) {
	_, err := ParseAllowedModelsHeader("glm,not-a-model", nil)
	var headerErr *AllowedModelsHeaderError
	require.ErrorAs(t, err, &headerErr)
	assert.Contains(t, headerErr.Reason, "not-a-model")
}

func TestParseAllowedModelsHeader_BlankRejected(t *testing.T) {
	_, err := ParseAllowedModelsHeader(" , ", nil)
	var headerErr *AllowedModelsHeaderError
	require.ErrorAs(t, err, &headerErr)
}

func TestParseAllowedModelsHeader_IntersectsInstallationAllowlist(t *testing.T) {
	got, err := ParseAllowedModelsHeader("deepseek-pro,kimi", []string{testDeepseek, testGlm})
	require.NoError(t, err)
	assert.Equal(t, []string{testDeepseek, testKimi}, got.Requested)
	assert.Equal(t, []string{testDeepseek}, got.Effective)
}

func TestParseAllowedModelsHeader_EmptyIntersectionFailsClosed(t *testing.T) {
	_, err := ParseAllowedModelsHeader("kimi", []string{testDeepseek})
	var headerErr *AllowedModelsHeaderError
	require.ErrorAs(t, err, &headerErr)
	assert.Contains(t, headerErr.Reason, testKimi)
}

func TestAllowedModelsForRequest_SubsetNarrowsPolicyAllowlist(t *testing.T) {
	ctx := ctxWithRequestSubset(ctxWithAllowedModels("a", "b"), "b", "c")
	assert.Equal(t, map[string]struct{}{"b": {}}, allowedModelsForRequest(ctx))
	assert.Equal(t, map[string]struct{}{"a": {}, "b": {}}, installationAllowedModelSet(ctx))
}

func TestAllowedModelsForRequest_SubsetAloneRestricts(t *testing.T) {
	ctx := ctxWithRequestSubset(context.Background(), "b")
	assert.Equal(t, map[string]struct{}{"b": {}}, allowedModelsForRequest(ctx))
	assert.Nil(t, installationAllowedModelSet(ctx))
}

func TestExcludedModelsForRequest_SubsetExcludesComplementButPolicySetDoesNot(t *testing.T) {
	s := &Service{availableModels: map[string]struct{}{"a": {}, "b": {}, "c": {}}}
	ctx := ctxWithRequestSubset(ctxWithAllowedModels("a", "b"), "b")

	got := s.excludedModelsForRequest(ctx)
	assert.NotContains(t, got, "b")
	assert.Contains(t, got, "a")
	assert.Contains(t, got, "c")

	policy := s.policyExcludedModels(ctx)
	assert.NotContains(t, policy, "a")
	assert.NotContains(t, policy, "b")
	assert.Contains(t, policy, "c")
}

func TestModelPermittedByAllowlist_IgnoresRequestSubset(t *testing.T) {
	ctx := ctxWithRequestSubset(ctxWithAllowedModels("a", "b"), "b")
	assert.True(t, modelPermittedByAllowlist(ctx, "a"))
	assert.False(t, modelPermittedByAllowlist(ctx, "c"))
}

func TestTelemetryDecisionReason_PrefixesOnlyWithSubset(t *testing.T) {
	assert.Equal(t, "cluster_argmax", telemetryDecisionReason(context.Background(), "cluster_argmax"))
	ctx := ctxWithRequestSubset(context.Background(), "b")
	assert.Equal(t, AllowlistOverrideReasonPrefix+"cluster_argmax", telemetryDecisionReason(ctx, "cluster_argmax"))
	assert.Equal(t, translate.ReasonUserForceModel, telemetryDecisionReason(ctx, translate.ReasonUserForceModel))
}

func TestRequestedAllowedModelsForTelemetry(t *testing.T) {
	assert.Nil(t, requestedAllowedModelsForTelemetry(context.Background()))
	ctx := context.WithValue(context.Background(), RequestAllowedModelsContextKey{}, RequestAllowedModels{
		Requested: []string{testKimi, testDeepseek},
		Effective: []string{testDeepseek},
	})
	assert.Equal(t, []string{testDeepseek, testKimi}, requestedAllowedModelsForTelemetry(ctx))
}

func TestReadmitForcedModel_LiftsSubsetOnlyExclusion(t *testing.T) {
	s := &Service{availableModels: map[string]struct{}{testDeepseek: {}, testKimi: {}, testGlm: {}}}
	ctx := ctxWithRequestSubset(context.Background(), testDeepseek)
	env, err := translate.ParseAnthropic([]byte(`{"model":"zai-org/glm-5.3","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	req := router.Request{ExcludedModels: s.excludedModelsForRequest(ctx)}
	require.Contains(t, req.ExcludedModels, testKimi)

	pin := sessionpin.Pin{Model: testKimi, Provider: providers.ProviderAIAND}
	got := s.readmitForcedModel(ctx, req, env, translate.RoutingFeatures{MaxTokens: 16}, pin)
	assert.NotContains(t, got, testKimi)
	assert.Contains(t, got, testGlm)
	assert.Contains(t, req.ExcludedModels, testKimi, "input map must not be mutated")
}

func TestReadmitForcedModel_KeepsPolicyExclusion(t *testing.T) {
	s := &Service{availableModels: map[string]struct{}{testDeepseek: {}, testKimi: {}, testGlm: {}}}
	ctx := ctxWithRequestSubset(ctxWithAllowedModels(testDeepseek, testKimi), testDeepseek)
	env, err := translate.ParseAnthropic([]byte(`{"model":"zai-org/glm-5.3","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	req := router.Request{ExcludedModels: s.excludedModelsForRequest(ctx)}

	pin := sessionpin.Pin{Model: testGlm, Provider: providers.ProviderAIAND}
	got := s.readmitForcedModel(ctx, req, env, translate.RoutingFeatures{MaxTokens: 16}, pin)
	assert.Contains(t, got, testGlm)
}

func TestReadmitForcedModel_NoSubsetIsNoOp(t *testing.T) {
	s := &Service{availableModels: map[string]struct{}{testDeepseek: {}, testKimi: {}}}
	req := router.Request{ExcludedModels: map[string]struct{}{testKimi: {}}}
	got := s.readmitForcedModel(context.Background(), req, nil, translate.RoutingFeatures{}, sessionpin.Pin{Model: testKimi})
	assert.Contains(t, got, testKimi)
}

func TestModelInRequestSubset(t *testing.T) {
	assert.True(t, modelInRequestSubset(context.Background(), testGlm))
	ctx := ctxWithRequestSubset(context.Background(), testDeepseek)
	assert.True(t, modelInRequestSubset(ctx, testDeepseek))
	assert.False(t, modelInRequestSubset(ctx, testGlm))
}

func TestForcedModelBinding_IgnoresRequestSubset(t *testing.T) {
	s := &Service{availableModels: map[string]struct{}{testDeepseek: {}, testKimi: {}, testGlm: {}}}
	ctx := ctxWithRequestSubset(ctxWithAllowedModels(testDeepseek, testKimi), testDeepseek)

	binding, reason := s.forcedModelBinding(ctx, testKimi, providers.ProviderAIAND)
	assert.Empty(t, reason)
	assert.Equal(t, providers.ProviderAIAND, binding)

	_, reason = s.forcedModelBinding(ctx, testGlm, providers.ProviderAIAND)
	assert.NotEmpty(t, reason, "installation allowlist still binds a forced model")
}

// A sticky pin outside the request subset must reroute inside it; a pin
// inside the subset still sticks.
func TestTurnLoop_StickyPinOutsideRequestSubsetReroutes(t *testing.T) {
	newSvc := func(fr *tierProbeRouter) *Service {
		store := &overwritingPinStore{pin: sessionpin.Pin{
			Provider:    providers.ProviderAIAND,
			Model:       testGlm,
			Reason:      "cluster:v0.2",
			PinnedUntil: time.Now().Add(time.Hour),
		}, found: true}
		return NewService(fr, nil, nil, false, nil, store, false,
			providers.ProviderAIAND, "zai-org/glm-5.3-flash", nil).
			WithDeploymentKeyedProviders(keyed(providers.ProviderAIAND, providers.ProviderAIAND))
	}
	env := forceCommandEnv(t)
	feats := env.RoutingFeatures(false)

	fr := &tierProbeRouter{available: map[string]struct{}{testGlm: {}, "zai-org/glm-5.3-flash": {}}}
	svc := newSvc(fr)
	ctx := ctxWithRequestSubset(context.Background(), "zai-org/glm-5.3-flash")
	res, err := svc.runTurnLoop(ctx, env, feats, "key-1", uuid.New(), "", nil,
		router.Request{RequestedModel: feats.Model, ExcludedModels: svc.excludedModelsForRequest(ctx)})
	require.NoError(t, err)
	assert.Equal(t, "zai-org/glm-5.3-flash", res.Decision.Model)
	assert.False(t, res.StickyHit)

	fr = &tierProbeRouter{available: map[string]struct{}{testGlm: {}, "zai-org/glm-5.3-flash": {}}}
	svc = newSvc(fr)
	ctx = ctxWithRequestSubset(context.Background(), testGlm, "zai-org/glm-5.3-flash")
	res, err = svc.runTurnLoop(ctx, env, feats, "key-1", uuid.New(), "", nil,
		router.Request{RequestedModel: feats.Model, ExcludedModels: svc.excludedModelsForRequest(ctx)})
	require.NoError(t, err)
	assert.Equal(t, testGlm, res.Decision.Model)
}

func TestRequestAllowedModelsPresent(t *testing.T) {
	assert.False(t, requestAllowedModelsPresent(context.Background()))
	assert.True(t, requestAllowedModelsPresent(
		ctxWithRequestSubset(context.Background(), testDeepseek)))
}

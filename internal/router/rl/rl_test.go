package rl_test

import (
	"context"
	"errors"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/rl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	modelFlash = "deepseek-ai/deepseek-v4-flash"
	modelKimi3 = "moonshotai/kimi-k3"
)

// fakeDecider records the query it received and returns a canned result/error.
type fakeDecider struct {
	got    rl.Query
	result rl.Result
	err    error
}

func (f *fakeDecider) Decide(_ context.Context, q rl.Query) (rl.Result, error) {
	f.got = q
	return f.result, f.err
}

func deployed(ids ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func enabled(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[n] = struct{}{}
	}
	return out
}

// allProviders is the deployment's keyed-provider set used to resolve dispatch
// bindings on the unrestricted (nil EnabledProviders) path.
var allProviders = enabled(providers.ProviderAiand)

func TestRouteMapsRosterChoiceBackToCatalogModel(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3, Score: 1.5, ScoreLabel: "DPO score", StateLabel: "implementing"}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	decision, err := r.Route(context.Background(), router.Request{
		PromptText:       "refactor the auth module",
		EnabledProviders: enabled(providers.ProviderAiand),
	})
	require.NoError(t, err)
	assert.Equal(t, modelKimi3, decision.Model)
	assert.Equal(t, providers.ProviderAiand, decision.Provider)
	assert.Contains(t, decision.Reason, "DPO score")
	assert.Contains(t, decision.Reason, "implementing")

	rosterIDs := make(map[string]string, len(dec.got.Candidates))
	for _, c := range dec.got.Candidates {
		rosterIDs[c.RosterID] = c.Provider
	}
	assert.Equal(t, providers.ProviderAiand, rosterIDs[modelKimi3])
	assert.Equal(t, providers.ProviderAiand, rosterIDs[modelFlash])
}

func TestRouteOmitsModelsWithNoEnabledProvider(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: enabled(providers.ProviderAnthropic),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, rl.ErrPolicyUnavailable))
	assert.Empty(t, dec.got.Candidates, "Decide must not run when no binding is enabled")
}

func TestRouteExcludesRequestedExclusions(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelFlash}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: enabled(providers.ProviderAiand),
		ExcludedModels:   map[string]struct{}{modelKimi3: {}},
	})
	require.NoError(t, err)
	for _, c := range dec.got.Candidates {
		assert.NotEqual(t, modelKimi3, c.RosterID)
	}
}

func TestRouteNoEligibleCandidatesIsUnavailable(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3}}
	r := rl.New(dec, deployed(modelKimi3), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: enabled(providers.ProviderOpenAI),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, rl.ErrPolicyUnavailable))
}

func TestRouteDeciderErrorIsUnavailable(t *testing.T) {
	dec := &fakeDecider{err: errors.New("sidecar down")}
	r := rl.New(dec, deployed(modelKimi3), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: enabled(providers.ProviderAiand),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, rl.ErrPolicyUnavailable))
}

func TestRouteNilEnabledProvidersIsUnrestricted(t *testing.T) {
	// nil EnabledProviders means "unrestricted" (router.Request contract); the
	// policy must still be offered the deployed models via their primary
	// provider, not an empty set.
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	decision, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, modelKimi3, decision.Model)
	assert.Equal(t, providers.ProviderAiand, decision.Provider)
	assert.NotEmpty(t, dec.got.Candidates, "nil providers must not empty the candidate set")
}

func TestRouteToolTurnDoesNotDropAgenticLow(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "use a tool",
		HasTools:         true,
		EnabledProviders: nil,
	})
	require.NoError(t, err)
	rosterIDs := make(map[string]struct{}, len(dec.got.Candidates))
	for _, c := range dec.got.Candidates {
		rosterIDs[c.RosterID] = struct{}{}
	}
	assert.Contains(t, rosterIDs, modelFlash)
	assert.Contains(t, rosterIDs, modelKimi3)
}

func TestRouteImageTurnDropsImageUnsupported(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: modelKimi3}}
	r := rl.New(dec, deployed(modelKimi3, modelFlash), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "what is in this image",
		HasImages:        true,
		EnabledProviders: nil,
	})
	require.NoError(t, err)
	for _, c := range dec.got.Candidates {
		assert.NotEqual(t, modelFlash, c.RosterID,
			"image-unsupported model must not be offered on an image turn")
	}
}

func TestRouteUnknownReturnedModelIsUnavailable(t *testing.T) {
	dec := &fakeDecider{result: rl.Result{Model: "openai/gpt-oss-120b"}}
	r := rl.New(dec, deployed(modelKimi3), allProviders)

	_, err := r.Route(context.Background(), router.Request{
		PromptText:       "hi",
		EnabledProviders: enabled(providers.ProviderAiand),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, rl.ErrPolicyUnavailable))
}

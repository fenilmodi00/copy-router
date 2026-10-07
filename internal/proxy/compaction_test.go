package proxy

import (
	"context"
	"testing"
	"time"
	"weave-os/router/internal/dispatch"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/catalog"
	"weave-os/router/internal/router/policy"
	"weave-os/router/internal/router/sessionpin"
	"weave-os/router/internal/router/turntype"
	"weave-os/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrecompactionPolicyReviewsEveryCascadeCandidate(t *testing.T) {
	spec, ok := policy.DefaultRegistry().Spec(policy.PurposePrecompactionSummary)
	require.True(t, ok)
	assert.Contains(t, spec.FixedCatalogModels, policy.PrecompactionDefaultModel)
	assert.Contains(t, spec.FixedCatalogModels, policy.PrecompactionLargeWindowModel)
	// Every reviewed summarizer must be servable on the AIand-only roster at a
	// non-low tier: a warm session pin resolves onto one of these.
	for _, m := range spec.FixedCatalogModels {
		model, known := catalog.ByID(m)
		require.True(t, known, "summarizer %s must be a catalog model", m)
		assert.NotEqual(t, catalog.TierLow, model.Tier, "summarizer %s must not be low-tier", m)
		aiand := false
		for _, b := range model.Providers {
			aiand = aiand || b.Provider == providers.ProviderAIAND
		}
		assert.True(t, aiand, "summarizer %s must carry an AIand binding", m)
	}
}

func TestCompactionHardPin(t *testing.T) {
	s := &Service{compactionHardPinEnabled: true}
	var key [sessionpin.SessionKeyLen]byte
	ctx := context.Background()

	p, m, source, ok := s.compactionHardPin(ctx, key, "", router.Request{})
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAIAND, p)
	assert.Equal(t, policy.PrecompactionDefaultModel, m, "no pin → the deployment's mid/high-tier summarizer")
	assert.Equal(t, policy.OverrideSourceDeployment, source, "the deployment's compaction model fixed the turn")

	_, _, _, ok = s.compactionHardPin(ctx, key, "", router.Request{EnabledProviders: map[string]struct{}{providers.ProviderOpenAI: {}}})
	assert.False(t, ok, "AIand disabled for the tenant → fall back to generic hard-pin")

	_, _, _, ok = s.compactionHardPin(ctx, key, "", router.Request{GatewayProviders: map[string]struct{}{providers.ProviderAIAND: {}}})
	assert.False(t, ok, "gateway-exclusive tenant → fall back to generic hard-pin")

	_, _, _, ok = s.compactionHardPin(ctx, key, "", router.Request{ExcludedModels: map[string]struct{}{policy.PrecompactionDefaultModel: {}}})
	assert.False(t, ok, "excluded default with no pin → fall back to generic hard-pin")

	unavailable := &Service{compactionHardPinEnabled: true, availableModels: map[string]struct{}{"claude-haiku-4-5": {}}}
	_, _, _, ok = unavailable.compactionHardPin(ctx, key, "", router.Request{})
	assert.False(t, ok, "default not routable in this deployment → fall back to generic hard-pin")
}

// rolePinStore serves a distinct pin per role so the thread pin and the
// _hmm_history row can disagree.
type rolePinStore struct {
	stubPinStore
	byRole map[string]sessionpin.Pin
}

func (s *rolePinStore) Get(_ context.Context, _ [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
	pin, found := s.byRole[role]
	return pin, found, nil
}

func TestCompactionHardPin_CodexKeepsNonAnthropicSessionModel(t *testing.T) {
	var key [sessionpin.SessionKeyLen]byte
	ctx := context.Background()
	live := time.Now().Add(time.Hour)
	openAIProviders := map[string]providers.Client{providers.ProviderOpenAI: nil, providers.ProviderAnthropic: nil, providers.ProviderAIAND: nil}
	codex := func(req router.Request) router.Request {
		req.ClientApp = ClientAppCodex
		return req
	}

	// A Codex thread the HMM has been serving on gpt-5.6-sol: its compaction
	// turn stays in the Sol family (upgraded to its newest version) instead of
	// crossing to the Anthropic summarizer.
	hmmServed := &rolePinStore{byRole: map[string]sessionpin.Pin{
		hmmHistoryRole(sessionpin.DefaultRole): {Provider: providers.ProviderOpenAI, LastServedModel: "gpt-5.6-sol", LastTurnEndedAt: time.Now(), PinnedUntil: live},
	}}
	s := &Service{compactionHardPinEnabled: true, pinStore: hmmServed, clients: dispatch.NewClients(openAIProviders)}
	p, m, source, ok := s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{}))
	require.True(t, ok)
	assert.Equal(t, providers.ProviderOpenAI, p)
	assert.Equal(t, "gpt-6.1-sol", m)
	assert.Equal(t, policy.OverrideSourceSession, source, "the session's own model fixed the turn")

	// Claude Code's compaction turn is Anthropic-format: the same history keeps
	// the deployment's AIand summarizer.
	p, m, source, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, router.Request{ClientApp: ClientAppClaudeCode})
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAIAND, p)
	assert.Equal(t, policy.PrecompactionDefaultModel, m)
	assert.Equal(t, policy.OverrideSourceDeployment, source)

	// The most recently served model wins when the thread pin and HMM history disagree.
	switched := &rolePinStore{byRole: map[string]sessionpin.Pin{
		sessionpin.DefaultRole:                 {Provider: providers.ProviderOpenAI, Model: "gpt-5.6-terra", LastServedModel: "gpt-5.6-terra", LastTurnEndedAt: time.Now().Add(-time.Minute), PinnedUntil: live},
		hmmHistoryRole(sessionpin.DefaultRole): {Provider: providers.ProviderOpenAI, LastServedModel: "gpt-5.6-sol", LastTurnEndedAt: time.Now(), PinnedUntil: live},
	}}
	s = &Service{compactionHardPinEnabled: true, pinStore: switched, clients: dispatch.NewClients(openAIProviders)}
	_, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{}))
	require.True(t, ok)
	assert.Equal(t, "gpt-6.1-sol", m)

	// An expired thread pin no longer speaks for the session.
	expired := &rolePinStore{byRole: map[string]sessionpin.Pin{
		sessionpin.DefaultRole: {Provider: providers.ProviderOpenAI, Model: "gpt-5.6-sol", LastServedModel: "gpt-5.6-sol", LastTurnEndedAt: time.Now(), PinnedUntil: time.Now().Add(-time.Hour)},
	}}
	s = &Service{compactionHardPinEnabled: true, pinStore: expired, clients: dispatch.NewClients(openAIProviders)}
	p, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{}))
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAIAND, p)
	assert.Equal(t, policy.PrecompactionDefaultModel, m)

	// A tenant that turned OpenAI off cannot keep the thread there.
	s = &Service{compactionHardPinEnabled: true, pinStore: switched, clients: dispatch.NewClients(openAIProviders)}
	_, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{EnabledProviders: map[string]struct{}{providers.ProviderAIAND: {}}}))
	require.True(t, ok)
	assert.Equal(t, policy.PrecompactionDefaultModel, m, "served vendor disabled → the AIand summarizer")

	// Org exclusions and the deployment-wide automatic disable still apply.
	solFamily := map[string]struct{}{"gpt-5.6-sol": {}, "gpt-6-sol": {}, "gpt-6.1-sol": {}}
	_, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{ExcludedModels: solFamily}))
	require.True(t, ok)
	assert.Equal(t, policy.PrecompactionDefaultModel, m)
	_, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{AutomaticExcludedModels: solFamily}))
	require.True(t, ok)
	assert.Equal(t, policy.PrecompactionDefaultModel, m)

	// An older low-tier model may have a newer mid-tier successor.
	lowServed := &rolePinStore{byRole: map[string]sessionpin.Pin{
		hmmHistoryRole(sessionpin.DefaultRole): {Provider: providers.ProviderOpenAI, LastServedModel: "gpt-4.1-mini", LastTurnEndedAt: time.Now(), PinnedUntil: live},
	}}
	s = &Service{compactionHardPinEnabled: true, pinStore: lowServed, clients: dispatch.NewClients(openAIProviders)}
	p, m, _, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{}))
	require.True(t, ok)
	assert.Equal(t, providers.ProviderOpenAI, p)
	assert.Equal(t, "gpt-5.5-mini", m)

	// An AIand-served Codex session keeps its own model: the summarizer lane is
	// the same vendor that ran the thread.
	aiandServed := &rolePinStore{byRole: map[string]sessionpin.Pin{
		sessionpin.DefaultRole: {Provider: providers.ProviderAIAND, Model: "zai-org/glm-5.3", LastServedModel: "zai-org/glm-5.3", LastTurnEndedAt: time.Now(), PinnedUntil: live},
	}}
	s = &Service{compactionHardPinEnabled: true, pinStore: aiandServed, clients: dispatch.NewClients(openAIProviders)}
	p, m, source, ok = s.compactionHardPin(ctx, key, sessionpin.DefaultRole, codex(router.Request{}))
	require.True(t, ok)
	assert.Equal(t, providers.ProviderAIAND, p)
	assert.Equal(t, "zai-org/glm-5.3", m)
	assert.Equal(t, policy.OverrideSourceSession, source, "the session's own model fixed the turn")
}

func TestCompactionHardPin_CodexEffortQualifiedSessionModel(t *testing.T) {
	const sessionModel = "gpt-5.6-luna"
	store := &rolePinStore{byRole: map[string]sessionpin.Pin{
		hmmHistoryRole(sessionpin.DefaultRole): {
			Provider: providers.ProviderOpenAI, LastServedModel: sessionModel + ":xhigh",
			LastTurnEndedAt: time.Now(), PinnedUntil: time.Now().Add(time.Hour),
		},
	}}
	s := &Service{
		compactionHardPinEnabled: true,
		pinStore:                 store,
		clients:                  dispatch.NewClients(map[string]providers.Client{providers.ProviderOpenAI: nil}),
		availableModels:          map[string]struct{}{sessionModel: {}},
	}

	provider, model, source, ok := s.compactionHardPin(context.Background(), [sessionpin.SessionKeyLen]byte{}, sessionpin.DefaultRole, router.Request{ClientApp: ClientAppCodex})
	require.True(t, ok)
	assert.Equal(t, providers.ProviderOpenAI, provider)
	assert.Equal(t, sessionModel, model)
	assert.Equal(t, policy.OverrideSourceSession, source)
}

func TestCompactionHardPin_FamilyUpgradeHonorsRestrictions(t *testing.T) {
	const olderModel = "gpt-6-sol"
	const newerModel = "gpt-6.1-sol"
	ctx := context.Background()
	store := &rolePinStore{byRole: map[string]sessionpin.Pin{
		hmmHistoryRole(sessionpin.DefaultRole): {
			Provider: providers.ProviderOpenAI, LastServedModel: olderModel,
			LastTurnEndedAt: time.Now(), PinnedUntil: time.Now().Add(time.Hour),
		},
	}}
	s := &Service{pinStore: store, clients: dispatch.NewClients(map[string]providers.Client{providers.ProviderOpenAI: nil})}
	for _, test := range []struct {
		name string
		req  router.Request
		want string
	}{
		{"upgrade", router.Request{}, newerModel},
		{"org exclusion", router.Request{ExcludedModels: map[string]struct{}{newerModel: {}}}, olderModel},
		{"automatic exclusion", router.Request{AutomaticExcludedModels: map[string]struct{}{newerModel: {}}}, olderModel},
		{"provider disabled", router.Request{EnabledProviders: map[string]struct{}{providers.ProviderAIAND: {}}}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, model, ok := s.compactionSessionModel(ctx, [sessionpin.SessionKeyLen]byte{}, sessionpin.DefaultRole, test.req)
			assert.Equal(t, test.want != "", ok)
			assert.Equal(t, test.want, model)
		})
	}
	s.availableModels = map[string]struct{}{olderModel: {}}
	_, model, ok := s.compactionSessionModel(ctx, [sessionpin.SessionKeyLen]byte{}, sessionpin.DefaultRole, router.Request{})
	require.True(t, ok)
	assert.Equal(t, olderModel, model)
}

func TestCompactionHardPin_CodexKeepsUntieredSessionFamily(t *testing.T) {
	for _, sessionModel := range []string{"gpt-4o", "gpt-5-chat"} {
		t.Run(sessionModel, func(t *testing.T) {
			store := &rolePinStore{byRole: map[string]sessionpin.Pin{
				hmmHistoryRole(sessionpin.DefaultRole): {
					Provider: providers.ProviderOpenAI, LastServedModel: sessionModel,
					LastTurnEndedAt: time.Now(), PinnedUntil: time.Now().Add(time.Hour),
				},
			}}
			s := &Service{
				pinStore:        store,
				clients:         dispatch.NewClients(map[string]providers.Client{providers.ProviderOpenAI: nil}),
				availableModels: map[string]struct{}{sessionModel: {}},
			}
			provider, model, _, ok := s.compactionHardPin(context.Background(), [sessionpin.SessionKeyLen]byte{}, sessionpin.DefaultRole, router.Request{ClientApp: ClientAppCodex})
			require.True(t, ok)
			assert.Equal(t, providers.ProviderOpenAI, provider)
			assert.Equal(t, sessionModel, model)
		})
	}
}

func TestTurnLoop_CompactionReadsClientIdentityBeforeHardPin(t *testing.T) {
	const sessionModel = "gpt-6-sol"
	store := &rolePinStore{byRole: map[string]sessionpin.Pin{
		hmmHistoryRole(roleForTier(catalog.TierMid)): {
			Provider: providers.ProviderOpenAI, LastServedModel: sessionModel,
			LastTurnEndedAt: time.Now(), PinnedUntil: time.Now().Add(time.Hour),
		},
	}}
	s := NewService(nil, map[string]providers.Client{providers.ProviderOpenAI: nil, providers.ProviderAnthropic: nil}, nil, false, nil, store, false, providers.ProviderAnthropic, policy.PrecompactionDefaultModel, nil)
	s.compactionHardPinEnabled = true
	env, err := translate.ParseOpenAI([]byte(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"You are performing a CONTEXT CHECKPOINT COMPACTION. Create a summary."}],"max_tokens":4096}`))
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), ClientIdentityContextKey{}, ClientIdentity{ClientApp: ClientAppCodex})
	features := env.RoutingFeatures(false)
	turn, err := s.runTurnLoop(ctx, env, features, "test-key", uuid.Nil, "", nil, router.Request{RequestedModel: features.Model})
	require.NoError(t, err)
	assert.Equal(t, turntype.Compaction, turn.TurnType)
	assert.Equal(t, providers.ProviderOpenAI, turn.Decision.Provider)
	assert.Equal(t, "gpt-6.1-sol", turn.Decision.Model)
}

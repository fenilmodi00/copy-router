package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/cache"
	"weave-os/router/internal/router/turntype"
	"weave-os/router/internal/translate"
)

func TestContextEstimateHeadersProtocols(t *testing.T) {
	fixtures := []struct {
		name  string
		parse func([]byte) (*translate.RequestEnvelope, error)
		body  string
	}{
		{"anthropic", translate.ParseAnthropic, `{"model":"zai-org/glm-5.3","max_tokens":16000,"messages":[{"role":"user","content":"inspect the build failure"}]}`},
		{"openai", translate.ParseOpenAI, `{"model":"deepseek-ai/deepseek-v4-pro","max_tokens":16000,"messages":[{"role":"user","content":"inspect the build failure"}]}`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			env, err := fixture.parse([]byte(fixture.body))
			require.NoError(t, err)
			headers := http.Header{}
			setContextEstimateHeaders(headers, env.ContextOverflowTokenEstimate(), 16000)
			require.Equal(t, "16000", headers.Get(HeaderRouterContextReserve))
			require.Equal(t, "approximate", headers.Get(HeaderRouterContextEstimateKind))
			require.Equal(t, "1", headers.Get(HeaderRouterContextVersion))
			require.NotEmpty(t, headers.Get(HeaderRouterContextEstimate))
			require.NotEqual(t, "0", headers.Get(HeaderRouterContextEstimate))
		})
	}
}

func TestCachedContextEstimateUsesLiveRequest(t *testing.T) {
	env, err := translate.ParseAnthropic([]byte(`{"model":"deepseek-ai/deepseek-v4-pro","max_tokens":32,"messages":[{"role":"user","content":"a fresh request"}]}`))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	setContextEstimateHeaders(rec.Header(), env.ContextOverflowTokenEstimate(), 8000)
	estimate := rec.Header().Get(HeaderRouterContextEstimate)
	stale := http.Header{}
	stale.Set(HeaderRouterContextEstimate, "999999")
	stale.Set(HeaderRouterContextReserve, "1")
	stale.Set(HeaderRouterContextEstimateKind, "exact")
	stale.Set(HeaderRouterContextWindow, "1")
	provenance := cache.NewProvenance(cache.ProvenanceScope{
		CredentialSubject: "subject-test",
		Product:           cache.ProductLegacy,
		Model:             "deepseek-ai/deepseek-v4-pro",
		Provider:          providers.ProviderAIAND,
		UpstreamScope:     "aiand",
	})
	(&Service{}).writeCachedResponse(rec, cache.CachedResponse{Provenance: provenance, Headers: stale, Body: []byte(`{"ok":true}`)}, router.Decision{Model: "deepseek-ai/deepseek-v4-pro", Provider: providers.ProviderAIAND})
	require.Equal(t, estimate, rec.Result().Header.Get(HeaderRouterContextEstimate))
	require.Equal(t, "8000", rec.Result().Header.Get(HeaderRouterContextReserve))
	require.Equal(t, "approximate", rec.Result().Header.Get(HeaderRouterContextEstimateKind))
	require.NotEqual(t, "1", rec.Result().Header.Get(HeaderRouterContextWindow))
	require.Equal(t, "1048576", rec.Result().Header.Get(HeaderRouterContextWindow))
	require.Equal(t, "hit", rec.Result().Header.Get(HeaderRouterCache))
}

func TestContextSnapshotFreshness(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	snapshot := ContextSnapshot{Version: 1, EstimateKind: ContextEstimateApproximate, EstimateTokens: 72000, ContextWindow: 128000, OutputReserveTokens: 8000, ServedModel: "deepseek-ai/deepseek-v4-pro", RequestID: "request-test", RequestedAt: now.Add(-time.Minute), RecordedAt: now}
	require.True(t, snapshot.Fresh(now))
	require.False(t, snapshot.Fresh(now.Add(ContextSnapshotTTL+time.Second)))
	require.False(t, snapshot.Fresh(now.Add(-time.Second)))
	snapshot.EstimateKind = "exact"
	require.False(t, snapshot.Fresh(now))
	snapshot.EstimateKind = ContextEstimateApproximate
	snapshot.EstimateTokens = 0
	require.False(t, snapshot.Fresh(now))
	require.Nil(t, ParseContextSnapshot([]byte(`{"version":`)))
}

func TestContextSnapshotUsesFinalServedModel(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	headers := http.Header{}
	setContextEstimateHeaders(headers, 72000, 8000)
	headers.Set(HeaderRouterContextWindow, "262144")
	headers.Set(HeaderRouterModel, "qwen/qwen3.8-27b")
	snapshot := contextSnapshotJSONForDecision(headers, "request-final", "qwen/qwen3.8-27b", "deepseek-ai/deepseek-v4-pro", providers.ProviderAIAND, now, now)
	parsed := ParseContextSnapshot(snapshot)
	require.NotNil(t, parsed)
	require.Equal(t, "deepseek-ai/deepseek-v4-pro", parsed.ServedModel)
	require.Equal(t, 1_048_576, parsed.ContextWindow)
}

func TestSetContextEstimateHeadersOmitsUnavailable(t *testing.T) {
	headers := http.Header{}
	headers.Set(HeaderRouterContextEstimate, "999999")
	setContextEstimateHeaders(headers, 0, 8000)
	require.Empty(t, headers.Get(HeaderRouterContextEstimate), "an unavailable estimate is omitted, never zero")
	require.Empty(t, headers.Get(HeaderRouterContextReserve))
	require.Empty(t, headers.Get(HeaderRouterContextVersion))
}

func TestConversationContextTurn(t *testing.T) {
	for _, kind := range []string{"main_loop", "tool_result", "compaction"} {
		require.True(t, conversationContextTurn(turntype.TurnType(kind)), kind)
	}
	for _, kind := range []string{"", "probe", "title_generation", "classifier", "recap", "subagent"} {
		require.False(t, conversationContextTurn(turntype.TurnType(kind)), kind)
	}
}

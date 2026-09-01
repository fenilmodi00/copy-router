package proxy_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"workweave/router/internal/feedback"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
	"workweave/router/internal/router/cache"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// embeddingFixture returns a deterministic L2-normalized vector keyed by seed.
func embeddingFixture(seed float32) []float32 {
	v := []float32{seed, 1, 0, 0, 0, 0, 0, 0}
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	norm := float32(math.Sqrt(float64(sum)))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

// anthropicBody returns a minimal valid Anthropic Messages body. Includes a
// stub tool so the request stays classified as MainLoop — without it the
// turntype detector would fingerprint it as Classifier (small max_tokens,
// no tools, short message list) and hard-pin past the semantic cache.
func anthropicBody(prompt string, stream bool) []byte {
	streamLit := "false"
	if stream {
		streamLit = "true"
	}
	return []byte(`{
		"model":"moonshotai/kimi-k3",
		"max_tokens":256,
		"stream":` + streamLit + `,
		"tools":[{"name":"noop","description":"placeholder","input_schema":{"type":"object"}}],
		"messages":[{"role":"user","content":"` + prompt + `"}]
	}`)
}

// decisionWithEmbedding builds a routing decision with metadata needed for cache eligibility.
func decisionWithEmbedding(emb []float32, clusterIDs []int) router.Decision {
	return router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "deepseek-ai/deepseek-v4-flash",
		Reason:   "test",
		Metadata: &router.RoutingMetadata{
			Embedding:  emb,
			ClusterIDs: clusterIDs,
		},
	}
}

// proxyContextWithExternalID wires the per-tenant ID; without it the cache is bypassed.
func proxyContextWithExternalID(t *testing.T, externalID string) context.Context {
	t.Helper()
	ctx := context.Background()
	if externalID != "" {
		ctx = context.WithValue(ctx, proxy.ExternalIDContextKey{}, externalID)
	}
	return ctx
}

// responsesCacheUpstream answers a text-only Responses stream whose terminal
// completion carries the given text. The cache tests' tool-bearing bodies
// promote onto /v1/responses (reasoning model + tools), so their fake
// upstreams must speak the Responses wire.
func responsesCacheUpstream(text string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"type":"response.created","response":{"id":"resp_cache","status":"in_progress"}}`,
			`{"type":"response.output_text.delta","output_index":0,"delta":"` + text + `"}`,
			`{"type":"response.completed","response":{"id":"resp_cache","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":10,"output_tokens":2}}}`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte("data: " + f + "\n\n"))
		}
	}
}

func TestService_Cache_HitShortCircuitsProvider(t *testing.T) {
	emb := embeddingFixture(1)
	provider := &fakeProvider{proxyResponse: responsesCacheUpstream("hi")}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0, 1, 2, 3})}
	c := cache.New(cache.DefaultConfig())
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := proxyContextWithExternalID(t, "tenant-1")
	body := anthropicBody("ping", false)

	rec1 := httptest.NewRecorder()
	httpReq1 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httpReq1))
	require.Len(t, provider.proxyBodies, 1, "first call must hit the provider")

	rec2 := httptest.NewRecorder()
	httpReq2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httpReq2))
	assert.Len(t, provider.proxyBodies, 1, "cache hit must not invoke provider a second time")

	assert.Contains(t, rec2.Body.String(), "hi", "cached turn must replay the served text")
	assert.Equal(t, proxy.RouterCacheHit, rec2.Header().Get(proxy.HeaderRouterCache))
}

func TestService_Cache_StreamingBypasses(t *testing.T) {
	emb := embeddingFixture(2)
	provider := &fakeProvider{proxyResponse: responsesCacheUpstream("stream-payload")}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0})}
	c := cache.New(cache.DefaultConfig())
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := proxyContextWithExternalID(t, "tenant-1")
	body := anthropicBody("streaming please", true)

	rec1 := httptest.NewRecorder()
	httpReq1 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httpReq1))

	rec2 := httptest.NewRecorder()
	httpReq2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httpReq2))

	assert.Len(t, provider.proxyBodies, 2, "streaming requests must always hit the provider — no caching")
	assert.Empty(t, rec2.Header().Get(proxy.HeaderRouterCache), "streaming responses carry no x-router-cache marker")
}

func TestService_Cache_HeuristicDecisionBypasses(t *testing.T) {
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"id":"x"}`)) },
	}
	// Decision with no Metadata — what the heuristic router produces.
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAiand, Model: "deepseek-ai/deepseek-v4-flash", Reason: "heuristic"}}
	c := cache.New(cache.DefaultConfig())
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := proxyContextWithExternalID(t, "tenant-1")
	body := anthropicBody("ask", false)

	rec1 := httptest.NewRecorder()
	httpReq1 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httpReq1))

	rec2 := httptest.NewRecorder()
	httpReq2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httpReq2))

	assert.Len(t, provider.proxyBodies, 2, "decisions without RoutingMetadata must not be cached")
}

func TestService_Cache_MissingExternalIDBypasses(t *testing.T) {
	emb := embeddingFixture(3)
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"id":"y"}`)) },
	}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0})}
	c := cache.New(cache.DefaultConfig())
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	body := anthropicBody("ask", false)

	// No externalID → cache bypassed (per-tenant scope is the only isolation).
	ctx := context.Background()
	rec1 := httptest.NewRecorder()
	httpReq1 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httpReq1))

	rec2 := httptest.NewRecorder()
	httpReq2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httpReq2))

	assert.Len(t, provider.proxyBodies, 2, "without externalID the cache must not store or replay")
}

// TestService_Cache_HitOmitsFeedbackLink guards two properties of the
// semantic-cache hit path: (1) the miss response carries a feedback link, and
// (2) the cache hit carries none — replaying the cached request's link would
// attribute a new client's rating to the wrong request_id.
func TestService_Cache_HitOmitsFeedbackLink(t *testing.T) {
	emb := embeddingFixture(7)
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) {
			// Echo a feedback header into the stored response to prove it is
			// not replayed from cache on the hit.
			w.Header().Set(proxy.HeaderRouterFeedbackURL, "https://router.example.com/f/STALE")
			responsesCacheUpstream("hi")(w)
		},
	}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0, 1, 2, 3})}
	c := cache.New(cache.DefaultConfig())
	signer := feedback.NewSigner("cache-secret", time.Hour)
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
		WithFeedback(nil, signer, "https://router.example.com")

	ctx := context.WithValue(proxyContextWithExternalID(t, "tenant-1"), proxy.InstallationIDContextKey{}, uuid.New().String())
	body := anthropicBody("ping", false)

	rec1 := httptest.NewRecorder()
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))))
	require.NotEmpty(t, rec1.Header().Get(proxy.HeaderRouterFeedbackURL), "miss path must emit a feedback link")

	rec2 := httptest.NewRecorder()
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))))
	require.Len(t, provider.proxyBodies, 1, "second call must be a cache hit")
	require.Equal(t, proxy.RouterCacheHit, rec2.Header().Get(proxy.HeaderRouterCache))
	assert.Empty(t, rec2.Header().Get(proxy.HeaderRouterFeedbackURL), "cache hit must not emit a feedback link (never replay the cached one)")
}

func TestService_Cache_HitRecordsSemanticCacheTelemetry(t *testing.T) {
	emb := embeddingFixture(3)
	provider := &fakeProvider{proxyResponse: responsesCacheUpstream("hi")}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0, 1})}
	c := cache.New(cache.DefaultConfig())
	tel := newCaptureTelemetry()
	instID := uuid.New().String()
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", tel).
		WithFeedback(nil, feedback.NewSigner("cache-secret", time.Hour), "https://router.example.com")

	ctx := context.WithValue(proxyContextWithExternalID(t, "tenant-telemetry"), proxy.InstallationIDContextKey{}, instID)
	body := anthropicBody("telemetry ping", false)

	rec1 := httptest.NewRecorder()
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))))
	require.Len(t, provider.proxyBodies, 1)

	rec2 := httptest.NewRecorder()
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))))
	require.Equal(t, proxy.RouterCacheHit, rec2.Header().Get(proxy.HeaderRouterCache))

	require.Eventually(t, func() bool {
		return tel.semanticCacheHitRow() != nil
	}, 3*time.Second, 10*time.Millisecond, "cache hit must emit telemetry with semantic_cache_hit")

	hitRow := tel.semanticCacheHitRow()
	require.NotNil(t, hitRow)
	assert.Equal(t, instID, hitRow.InstallationID)
	assert.Equal(t, "router.upstream", hitRow.SpanType)
}

func TestService_Cache_DisabledByNilCache(t *testing.T) {
	emb := embeddingFixture(4)
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"id":"z"}`)) },
	}
	fr := &fakeRouter{decision: decisionWithEmbedding(emb, []int{0})}
	// nil cache equivalent to ROUTER_SEMANTIC_CACHE_ENABLED=false.
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAiand: provider}, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := proxyContextWithExternalID(t, "tenant-1")
	body := anthropicBody("ask", false)

	rec1 := httptest.NewRecorder()
	httpReq1 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec1, httpReq1))

	rec2 := httptest.NewRecorder()
	httpReq2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(ctx, body, rec2, httpReq2))

	assert.Len(t, provider.proxyBodies, 2, "nil cache must be a transparent passthrough")
	assert.Empty(t, rec2.Header().Get(proxy.HeaderRouterCache))
}

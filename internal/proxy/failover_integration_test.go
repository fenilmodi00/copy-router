package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/providers/openaicompat"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProxyMessages_SingleBindingPreservesEagerPrelude asserts that
// single-binding requests (every Anthropic-native model today) still
// fire translator.Prelude eagerly to the client writer — preserving
// main #220's TTFB win. The preludeBuffer is not engaged because
// resolveBindingsForDispatch returns a single-element slice.
func TestProxyMessages_SingleBindingPreservesEagerPrelude(t *testing.T) {
	// An Anthropic-shape upstream that emits SSE chunks. We don't assert
	// the chunks here; we assert that the response is committed (200) and
	// the client sees message_start before message_stop — i.e. the
	// translator's Prelude wasn't swallowed by an inadvertent buffer.
	anth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Minimal valid Anthropic-shape stream.
		for _, c := range []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_x\",\"role\":\"assistant\",\"content\":[],\"model\":\"deepseek-ai/deepseek-v4-flash\"}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		} {
			_, _ = w.Write([]byte(c))
		}
	}))
	defer anth.Close()

	// deepseek-ai/deepseek-v4-flash is single-binding (Anthropic only).
	svc := makeProxyService(
		router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"},
		map[string]providers.Client{
			providers.ProviderAnthropic: &fakeProvider{
				proxyResponse: func(w http.ResponseWriter) {
					// Mirror the translator's expected SSE shape.
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_x\",\"role\":\"assistant\",\"content\":[],\"model\":\"deepseek-ai/deepseek-v4-flash\"}}\n\n"))
					_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
				},
			},
		},
	).WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderAnthropic: {}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"deepseek-ai/deepseek-v4-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, req))
	assert.Equal(t, http.StatusOK, rec.Code)
	respBody := rec.Body.String()
	assert.Contains(t, respBody, "message_start")
	assert.Contains(t, respBody, "message_stop")
	// No fallback header for single-binding requests.
	assert.Empty(t, rec.Header().Get(proxy.HeaderRouterFallbackFrom))
}

// TestProxyMessages_SingleBindingStreamingPreCommitError asserts the fixed
// behavior: when a single-binding cross-format streaming request gets an
// upstream error BEFORE any upstream byte arrives, the preludeBuffer
// discards the buffered prelude and the client receives a clean
// Anthropic-shape JSON error envelope at the upstream's status — not a
// stranded `message_start` text-only turn that Claude Code would reject
// for missing tool_use. This is the v0.58 SWE-bench bake-off regression
// fix.
func TestProxyMessages_SingleBindingStreamingPreCommitError(t *testing.T) {
	// Stub upstream OpenAI-compat provider that 503s on every request.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream unavailable","type":"upstream_error"}}`))
	}))
	defer stub.Close()

	// gpt-5 is single-binding to openai in catalog; route there from an
	// inbound Anthropic Messages request so the cross-format
	// AnthropicSSETranslator + Prelude path runs.
	svc := proxy.NewService(
		&fakeRouter{decision: router.Decision{Provider: providers.ProviderOpenAI, Model: "gpt-5"}},
		map[string]providers.Client{
			providers.ProviderOpenAI: openaicompat.NewClient("test-key", stub.URL),
		},
		nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil,
	).WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderOpenAI: {}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"gpt-5","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	_ = svc.ProxyMessages(context.Background(), body, rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"pre-commit upstream error surfaces upstream's status, not a stranded HTTP 200")
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"),
		"pre-commit error is a clean JSON envelope, not a half-emitted SSE stream")

	respBody := rec.Body.String()
	assert.NotContains(t, respBody, "event: message_start",
		"prelude bytes were buffered and discarded — no stranded marker on the wire")
	assert.NotContains(t, respBody, "✦ **Weave Router**",
		"routing marker discarded with the prelude buffer")
	assert.Contains(t, respBody, `"type":"error"`, "Anthropic-shape error envelope")
	assert.Contains(t, respBody, "upstream unavailable", "translated upstream message reaches the client")
}

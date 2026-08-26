package proxy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type fakeRouter struct {
	decision    router.Decision
	err         error
	capturedReq *router.Request
	routeCalls  int
}

type fakePreviewRouter struct {
	previewResult policy.PreviewResult
	previewReq    *router.Request
	previewCalls  int
	routeCalls    int
}

func (f *fakePreviewRouter) Route(context.Context, router.Request) (router.Decision, error) {
	f.routeCalls++
	return router.Decision{}, errors.New("serving route must not run during preview")
}

func (f *fakePreviewRouter) PreviewRoute(_ context.Context, req router.Request) (policy.PreviewResult, error) {
	f.previewCalls++
	f.previewReq = &req
	return f.previewResult, nil
}

func (f *fakeRouter) Route(ctx context.Context, req router.Request) (router.Decision, error) {
	f.capturedReq = &req
	f.routeCalls++
	return f.decision, f.err
}

type fakeProvider struct {
	proxyBodies    [][]byte
	proxyEndpoints []providers.Endpoint
	proxyResponse  func(w http.ResponseWriter)
	proxyErr       error
	// proxyCreds records the resolved credential per dispatch; nil means
	// deployment-key fallback (no credential set).
	proxyCreds []*proxy.Credentials
	// passthroughCreds records the resolved credential per Passthrough call.
	passthroughCreds []*proxy.Credentials
}

func (f *fakeProvider) Proxy(ctx context.Context, decision router.Decision, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	saved := make([]byte, len(prep.Body))
	copy(saved, prep.Body)
	f.proxyBodies = append(f.proxyBodies, saved)
	f.proxyEndpoints = append(f.proxyEndpoints, prep.Endpoint)
	f.proxyCreds = append(f.proxyCreds, proxy.CredentialsFromContext(ctx))
	if f.proxyResponse != nil {
		f.proxyResponse(w)
	}
	return f.proxyErr
}

func TestService_BaselineFailoverSkipsUnconfiguredProvider(t *testing.T) {
	aiandProvider := &fakeProvider{proxyErr: &providers.UpstreamStatusError{Status: http.StatusServiceUnavailable}}
	telemetry := newCaptureTelemetry()
	svc := proxy.NewService(&fakeRouter{
		decision: router.Decision{
			Model:    "deepseek-ai/deepseek-v4-flash",
			Provider: providers.ProviderAiand,
			Reason:   "cluster:v0.76 test",
		},
	}, map[string]providers.Client{
		providers.ProviderAiand: aiandProvider,
	}, nil, false, nil, nil, false, "", "deepseek-ai/deepseek-v4-pro", telemetry).
		WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderAiand: {}}).
		WithAvailableModels(map[string]struct{}{"deepseek-ai/deepseek-v4-flash": {}})

	body := []byte(`{"model":"deepseek-ai/deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"max_tokens":512}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ctx := context.WithValue(context.Background(), proxy.InstallationIDContextKey{}, uuid.New().String())
	_ = svc.ProxyMessages(ctx, body, rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code, "the routed model's 503 surfaces as 502; no doomed anthropic rescue runs")
	row := telemetry.firstRow(t)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", row.DecisionModel, "telemetry must attribute the routed model, not the unconfigured baseline")
}

func TestService_ProxyOpenAIResponses_CustomToolUsesNativeOpenAIFamily(t *testing.T) {
	provider := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","object":"response","output":[]}`)
	}}
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderOpenAI, Model: "openai/gpt-oss-120b", Reason: "test"}}
	svc := proxy.NewService(fr, map[string]providers.Client{
		providers.ProviderAnthropic: &fakeProvider{},
		providers.ProviderOpenAI:    provider,
	}, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	body := []byte(`{"model":"openai/gpt-oss-120b","input":"apply a patch","reasoning":{"effort":"high"},"tools":[{"type":"custom","name":"apply_patch"}]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))
	require.NoError(t, svc.ProxyOpenAIResponses(context.Background(), body, rec, req))

	require.NotNil(t, fr.capturedReq)
	originalEnvelope, err := translate.ParseOpenAI(body)
	require.NoError(t, err)
	assert.Equal(t, originalEnvelope.ReasoningConfigurationSHA256(), fr.capturedReq.ReasoningConfigurationSHA256)
	assert.Equal(t, originalEnvelope.ToolConfigurationSHA256(), fr.capturedReq.ToolConfigurationSHA256)
	assert.Equal(t, map[string]struct{}{providers.ProviderOpenAI: {}}, fr.capturedReq.EnabledProviders)
	require.Len(t, provider.proxyBodies, 1)
	assert.JSONEq(t, `{"model":"openai/gpt-oss-120b","input":"apply a patch","reasoning":{"effort":"high"},"tools":[{"type":"custom","name":"apply_patch"}]}`, string(provider.proxyBodies[0]))
	assert.Equal(t, providers.EndpointResponses, provider.proxyEndpoints[0])
	assert.JSONEq(t, `{"id":"resp_1","object":"response","output":[]}`, rec.Body.String())
}

func TestService_ProxyOpenAIResponses_CodexPassthroughUsesChatForOpenAICompatProvider(t *testing.T) {
	openRouter := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`)
	}}
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderOpenRouter, Model: "deepseek/deepseek-chat", Reason: "test"}}
	svc := proxy.NewService(fr, map[string]providers.Client{
		providers.ProviderOpenAI:     &fakeProvider{},
		providers.ProviderOpenRouter: openRouter,
	}, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := context.WithValue(context.Background(), proxy.OpenAISubscriptionContextKey{}, "eyJhbGciOiJSUzI1NiJ9.codex.sig")
	ctx = context.WithValue(ctx, proxy.OpenAIAccountIDContextKey{}, "acct-123")
	body := []byte(`{"model":"openai/gpt-oss-120b","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))
	require.NoError(t, svc.ProxyOpenAIResponses(ctx, body, rec, req))

	require.Len(t, openRouter.proxyBodies, 1)
	assert.Equal(t, providers.EndpointChatCompletions, openRouter.proxyEndpoints[0])
	assert.Contains(t, string(openRouter.proxyBodies[0]), `"messages"`)
	assert.NotContains(t, string(openRouter.proxyBodies[0]), `"input_text"`)
}

func TestService_ProxyOpenAIResponses_CodexPortableBridgeKeepsHMMProvidersAndRestoresCustomTool(t *testing.T) {
	fireworks := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_new","type":"function","function":{"name":"exec","arguments":"{\"input\":\"return tools.apply_patch({});\"}"}}]},"finish_reason":null}]}`+"\n\n"+
				`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n"+
				"data: [DONE]\n\n")
	}}
	fr := &fakeRouter{decision: router.Decision{
		Provider: providers.ProviderFireworks,
		Model:    "moonshotai/kimi-k2.7",
		Reason:   "hmm:test",
	}}
	svc := proxy.NewService(fr, map[string]providers.Client{
		providers.ProviderOpenAI:    &fakeProvider{},
		providers.ProviderAnthropic: &fakeProvider{},
		providers.ProviderFireworks: fireworks,
		providers.ProviderGoogle:    &fakeProvider{},
		providers.ProviderTogether:  &fakeProvider{},
	}, nil, false, nil, nil, false, providers.ProviderOpenAI, "openai/gpt-oss-120b", nil).
		WithDeploymentKeyedProviders(map[string]struct{}{
			providers.ProviderOpenAI:    {},
			providers.ProviderAnthropic: {},
			providers.ProviderFireworks: {},
			providers.ProviderGoogle:    {},
			providers.ProviderTogether:  {},
		})

	ctx := context.WithValue(context.Background(), proxy.OpenAISubscriptionContextKey{}, "eyJhbGciOiJSUzI1NiJ9.codex.sig")
	ctx = context.WithValue(ctx, proxy.OpenAIAccountIDContextKey{}, "acct-123")
	ctx = context.WithValue(ctx, proxy.ClientIdentityContextKey{}, proxy.ClientIdentity{ClientApp: proxy.ClientAppCodex})
	body := []byte(`{
			"model":"openai/gpt-oss-120b",
			"stream":true,
			"service_tier":"priority",
			"prompt_cache_key":"codex-cache-key",
			"text":{"verbosity":"low"},
			"input":[
			{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque"},
			{"type":"custom_tool_call","id":"ctc_old","call_id":"call_old","name":"exec","input":"return tools.read({});"},
			{"type":"custom_tool_call_output","call_id":"call_old","output":[{"type":"input_text","text":"first"},{"type":"input_text","text":"second"}]},
			{"type":"function_call","id":"fc_old","call_id":"call_send","namespace":"collaboration","name":"send_message","arguments":"{\"target\":\"/root\"}"},
			{"type":"function_call_output","call_id":"call_send","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		],
		"tools":[{"type":"namespace","name":"functions","tools":[
			{"type":"custom","name":"exec","description":"Run code mode","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}},
				{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","parameters":{"type":"object","$defs":{"target":{"type":"string"}},"properties":{"target":{"$ref":"#/$defs/target"}}}}]}
		]}]
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))

	require.NoError(t, svc.ProxyOpenAIResponses(ctx, body, rec, req))
	require.NotNil(t, fr.capturedReq)
	assert.True(t, fr.capturedReq.HasTools)
	assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderOpenAI)
	assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderFireworks,
		"portable Codex turns must reach the ordinary deployed HMM provider roster")
	assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderAnthropic)
	assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderGoogle)
	assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderTogether)
	require.Len(t, fireworks.proxyBodies, 1)
	chatBody := fireworks.proxyBodies[0]
	assert.Equal(t, "moonshotai/kimi-k2.7", gjson.GetBytes(chatBody, "model").Str)
	assert.Equal(t, "exec", gjson.GetBytes(chatBody, "tools.0.function.name").Str)
	assert.Equal(t, "collaboration__send_message", gjson.GetBytes(chatBody, "tools.1.function.name").Str)
	assert.Equal(t, "string", gjson.GetBytes(chatBody, "tools.1.function.parameters.properties.target.type").Str)
	assert.False(t, gjson.GetBytes(chatBody, "tools.1.function.parameters.$defs").Exists())
	assert.NotContains(t, gjson.GetBytes(chatBody, "tools.1.function.parameters").Raw, `"$ref"`)
	assert.False(t, gjson.GetBytes(chatBody, "service_tier").Exists())
	assert.False(t, gjson.GetBytes(chatBody, "prompt_cache_key").Exists())
	assert.Equal(t, "Keep the response concise and focused.", gjson.GetBytes(chatBody, "messages.0.content").Str)
	assert.Contains(t, string(chatBody), `"tool_call_id":"call_old"`)
	assert.Contains(t, string(chatBody), "first")
	assert.Contains(t, string(chatBody), "second")
	assert.Contains(t, rec.Body.String(), `"type":"custom_tool_call"`)
	assert.Contains(t, rec.Body.String(), `"name":"exec"`)
	assert.Contains(t, rec.Body.String(), `"input":"return tools.apply_patch({});"`)
	assert.NotContains(t, rec.Body.String(), `"type":"function_call","status":"completed","call_id":"call_new"`)
}

func (f *fakeProvider) Passthrough(ctx context.Context, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	f.passthroughCreds = append(f.passthroughCreds, proxy.CredentialsFromContext(ctx))
	return nil
}

func makeProxyService(decision router.Decision, p map[string]providers.Client) *proxy.Service {
	return proxy.NewService(&fakeRouter{decision: decision}, p, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)
}

// TestService_PassthroughToNamedProvider_ResolvesBYOKCredential: passthrough
// must resolve credential precedence so a BYOK key wins over the deployment key.
func TestService_PassthroughToNamedProvider_ResolvesBYOKCredential(t *testing.T) {
	provider := &fakeProvider{}
	svc := makeProxyService(router.Decision{}, map[string]providers.Client{providers.ProviderAnthropic: provider})

	ctx := context.WithValue(context.Background(), proxy.ExternalAPIKeysContextKey{}, []*auth.ExternalAPIKey{
		{Provider: providers.ProviderAnthropic, Plaintext: []byte("sk-ant-byok"), BaseURL: "https://byok.example.com"},
	})
	httpReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	err := svc.PassthroughToNamedProvider(ctx, providers.ProviderAnthropic, nil, rec, httpReq)

	require.NoError(t, err)
	require.Len(t, provider.passthroughCreds, 1)
	require.NotNil(t, provider.passthroughCreds[0], "BYOK credential must be resolved onto ctx before Passthrough dispatches")
	assert.Equal(t, []byte("sk-ant-byok"), provider.passthroughCreds[0].APIKey)
	assert.Equal(t, "https://byok.example.com", provider.passthroughCreds[0].BaseURL)
}

// TestService_PassthroughToProvider_CountTokensLocalFallback verifies that a
// gateway-only deployment (no Anthropic credential) answers count_tokens locally.
func TestService_PassthroughToProvider_CountTokensLocalFallback(t *testing.T) {
	anthropicProvider := &fakeProvider{}
	gatewayProvider := &fakeProvider{}
	svc := makeProxyService(router.Decision{}, map[string]providers.Client{
		providers.ProviderAnthropic:        anthropicProvider,
		providers.ProviderAnthropicGateway: gatewayProvider,
	}).WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderAnthropicGateway: {}})

	body := []byte(`{"model":"deepseek-ai/deepseek-v4-pro","messages":[{"role":"user","content":"hello world"}]}`)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(""))
	rec := httptest.NewRecorder()

	require.NoError(t, svc.PassthroughToProvider(context.Background(), body, rec, httpReq))
	assert.Empty(t, anthropicProvider.passthroughCreds, "no upstream dispatch when no Anthropic credential is reachable")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("content-type"))
	tokens := gjson.GetBytes(rec.Body.Bytes(), "input_tokens")
	require.True(t, tokens.Exists())
	assert.Positive(t, tokens.Int())
}

// TestService_PassthroughToProvider_CountTokensForwardsWithCredential verifies
// that a reachable Anthropic credential keeps count_tokens on the real upstream.
func TestService_PassthroughToProvider_CountTokensForwardsWithCredential(t *testing.T) {
	body := []byte(`{"model":"deepseek-ai/deepseek-v4-pro","messages":[{"role":"user","content":"hello"}]}`)

	t.Run("deployment key", func(t *testing.T) {
		provider := &fakeProvider{}
		svc := makeProxyService(router.Decision{}, map[string]providers.Client{providers.ProviderAnthropic: provider}).
			WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderAnthropic: {}})
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(""))
		rec := httptest.NewRecorder()

		require.NoError(t, svc.PassthroughToProvider(context.Background(), body, rec, httpReq))
		assert.Len(t, provider.passthroughCreds, 1, "deployment-keyed Anthropic must forward upstream")
	})

	t.Run("BYOK key", func(t *testing.T) {
		provider := &fakeProvider{}
		svc := makeProxyService(router.Decision{}, map[string]providers.Client{providers.ProviderAnthropic: provider}).
			WithDeploymentKeyedProviders(map[string]struct{}{})
		ctx := context.WithValue(context.Background(), proxy.ExternalAPIKeysContextKey{}, []*auth.ExternalAPIKey{
			{Provider: providers.ProviderAnthropic, Plaintext: []byte("sk-ant-byok")},
		})
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(""))
		rec := httptest.NewRecorder()

		require.NoError(t, svc.PassthroughToProvider(ctx, body, rec, httpReq))
		assert.Len(t, provider.passthroughCreds, 1, "BYOK Anthropic must forward upstream")
	})
}

// TestService_PassthroughToProvider_LocalMetadataWithoutAnthropic covers the
// ai&-only deployment: no Anthropic provider registered, so the metadata
// pre-flight calls are answered locally instead of failing.
func TestService_PassthroughToProvider_LocalMetadataWithoutAnthropic(t *testing.T) {
	deployed := []string{"moonshotai/kimi-k3", "zai-org/glm-5.2"}
	aiandProvider := &fakeProvider{}
	newSvc := func() *proxy.Service {
		return makeProxyService(router.Decision{}, map[string]providers.Client{providers.ProviderAiand: aiandProvider}).
			WithLocalModelList(func() []string { return deployed })
	}

	t.Run("count_tokens answered locally", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-ai/deepseek-v4-pro","messages":[{"role":"user","content":"hello world"}]}`)
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(""))
		rec := httptest.NewRecorder()

		require.NoError(t, newSvc().PassthroughToProvider(context.Background(), body, rec, httpReq))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Positive(t, gjson.GetBytes(rec.Body.Bytes(), "input_tokens").Int())
		assert.Empty(t, aiandProvider.passthroughCreds, "no upstream dispatch for metadata calls")
	})

	t.Run("models list served from deployed registry", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		rec := httptest.NewRecorder()

		require.NoError(t, newSvc().PassthroughToProvider(context.Background(), nil, rec, httpReq))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("content-type"))
		data := gjson.GetBytes(rec.Body.Bytes(), "data")
		require.True(t, data.IsArray(), "response must carry the Anthropic model-list shape, got %s", rec.Body.String())
		ids := []string{}
		for _, e := range data.Array() {
			ids = append(ids, e.Get("id").Str)
			assert.Equal(t, "model", e.Get("type").Str)
		}
		assert.Equal(t, deployed, ids)
		assert.False(t, gjson.GetBytes(rec.Body.Bytes(), "has_more").Bool())
	})

	t.Run("single model entry", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/v1/models/moonshotai/kimi-k3", nil)
		rec := httptest.NewRecorder()

		require.NoError(t, newSvc().PassthroughToProvider(context.Background(), nil, rec, httpReq))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "moonshotai/kimi-k3", gjson.GetBytes(rec.Body.Bytes(), "id").Str)
	})

	t.Run("unknown model 404s in the Anthropic error shape", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/v1/models/nope", nil)
		rec := httptest.NewRecorder()

		require.NoError(t, newSvc().PassthroughToProvider(context.Background(), nil, rec, httpReq))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "not_found_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").Str)
	})

	t.Run("listing disabled without a model list source", func(t *testing.T) {
		svc := makeProxyService(router.Decision{}, map[string]providers.Client{providers.ProviderAiand: &fakeProvider{}})
		httpReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		rec := httptest.NewRecorder()

		err := svc.PassthroughToProvider(context.Background(), nil, rec, httpReq)
		assert.ErrorIs(t, err, proxy.ErrProviderNotConfigured)
	})
}

func TestService_ProxyMessages_PropagatesUpstreamStatusError(t *testing.T) {
	upstreamErr := &providers.UpstreamStatusError{Status: 400}
	provider := &fakeProvider{proxyErr: upstreamErr}
	svc := makeProxyService(
		router.Decision{Provider: "anthropic", Model: "deepseek-ai/deepseek-v4-flash"},
		map[string]providers.Client{"anthropic": provider},
	)

	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`)

	err := svc.ProxyMessages(context.Background(), body, rec, httpReq)

	var got *providers.UpstreamStatusError
	require.ErrorAs(t, err, &got, "must surface the typed UpstreamStatusError")
	assert.Equal(t, 400, got.Status)
}

// TestService_ProxyMessages_CrossFormatUpstreamErrorBodyReachesClient guards a
// regression: a cross-format upstream non-2xx (e.g. OpenRouter 402) buffered
// the body inside AnthropicSSETranslator but never flushed it, because
// Finalize was skipped on any non-nil proxyErr. Both the translated body and
// the typed UpstreamStatusError must reach the client/handler.
func TestService_ProxyMessages_CrossFormatUpstreamErrorBodyReachesClient(t *testing.T) {
	const upstreamBody = `{"error":{"message":"OpenRouter: insufficient credits","code":402,"type":"invalid_request_error"}}`
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = io.WriteString(w, upstreamBody)
		},
		proxyErr: &providers.UpstreamStatusError{Status: http.StatusPaymentRequired},
	}
	svc := makeProxyService(
		router.Decision{Provider: providers.ProviderOpenRouter, Model: "deepseek/deepseek-chat"},
		map[string]providers.Client{providers.ProviderOpenRouter: provider},
	)

	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`)

	err := svc.ProxyMessages(context.Background(), body, rec, httpReq)

	var got *providers.UpstreamStatusError
	require.ErrorAs(t, err, &got, "UpstreamStatusError must still propagate for telemetry")
	assert.Equal(t, http.StatusPaymentRequired, got.Status)

	assert.Equal(t, http.StatusPaymentRequired, rec.Code, "upstream status must reach the client")
	respBody := rec.Body.String()
	require.NotEmpty(t, respBody, "translated upstream error body must reach the client")
	assert.Contains(t, respBody, "insufficient credits", "upstream error message must survive translation")
	assert.Contains(t, respBody, `"type":"error"`, "body must be in Anthropic error envelope shape")
}

// TestService_ProxyMessages_StripsRoutingMarkerFromInboundHistory guards
// service.go's hygiene fix: the routing-marker text injected on prior
// cross-format responses (✦ **Weave Router** → ...) must not survive into the
// upstream body, or it round-trips and pollutes context on every later turn.
func TestService_ProxyMessages_StripsRoutingMarkerFromInboundHistory(t *testing.T) {
	const markerSentinel = "Weave Router"
	body := []byte(`{
		"model":"moonshotai/kimi-k3",
		"messages":[
			{"role":"user","content":"first prompt"},
			{"role":"assistant","content":[
				{"type":"text","text":"✦ **Weave Router** → deepseek-ai/deepseek-v4-pro (openrouter) · reason: top scorer\n\n"},
				{"type":"text","text":"real assistant reply"}
			]},
			{"role":"user","content":[
				{"type":"text","text":"</summary>\n<result>✦ **Weave Router** → deepseek-ai/deepseek-v4-flash (anthropic) · reason: tool-result follow-up\n\n</result>"}
			]}
		]
	}`)

	provider := &fakeProvider{}
	svc := makeProxyService(
		router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"},
		map[string]providers.Client{providers.ProviderAnthropic: provider},
	)

	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

	require.Len(t, provider.proxyBodies, 1)
	upstream := string(provider.proxyBodies[0])
	assert.NotContains(t, upstream, markerSentinel, "routing marker must not reach upstream")
	assert.Contains(t, upstream, "real assistant reply", "non-marker assistant content must survive")

	// The last user message's content is promoted to an array with a cache_control
	// marker; unmarshal to verify the wrapper text survived.
	var upstreamJSON map[string]any
	require.NoError(t, json.Unmarshal(provider.proxyBodies[0], &upstreamJSON))
	msgs, _ := upstreamJSON["messages"].([]any)
	lastMsg, _ := msgs[len(msgs)-1].(map[string]any)
	blocks, _ := lastMsg["content"].([]any)
	lastBlock, _ := blocks[len(blocks)-1].(map[string]any)
	assert.Contains(t, lastBlock["text"], "</result>", "wrapper text around an embedded marker must survive")
}

func TestService_ProxyMessages_EmbedOnlyUserMessageFlag(t *testing.T) {
	const firstUserPrompt = "Walk every Go file under router/internal/ and produce a one-paragraph summary of each."
	const secondUserPrompt = "Now narrow it to handlers under internal/api/."
	// embedOnlyUserMessage must keep both user prompts, drop system text,
	// assistant tool_use, and tool_result blocks.
	body := []byte(`{
		"model":"moonshotai/kimi-k3",
		"system":"You are Claude Code. CLAUDE.md says: do not use emojis...",
		"messages":[
			{"role":"user","content":"` + firstUserPrompt + `"},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"go.mod"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"module workweave/router\n\ngo 1.23\n\nrequire (\n\tgithub.com/gin-gonic/gin v1.10.0\n)"}]},
			{"role":"user","content":"` + secondUserPrompt + `"}
		]
	}`)

	t.Run("flag off uses concatenated stream", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: "anthropic", Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr,
			map[string]providers.Client{providers.ProviderAnthropic: &fakeProvider{}},
			nil,
			false,
			nil,
			nil,
			false,
			providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash",
			nil,
		)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		got := fr.capturedReq.PromptText
		assert.Contains(t, got, "You are Claude Code", "flag=off keeps system prompt")
		assert.Contains(t, got, firstUserPrompt, "flag=off keeps first user message text")
	})

	t.Run("flag on concatenates user-role text and drops everything else", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: "anthropic", Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr,
			map[string]providers.Client{providers.ProviderAnthropic: &fakeProvider{}},
			nil,
			true,
			nil,
			nil,
			false,
			providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash",
			nil,
		)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		got := fr.capturedReq.PromptText
		assert.Equal(t, firstUserPrompt+"\n"+secondUserPrompt, got,
			"flag=on emits user-role text only (no system, no assistant tool_use, no tool_result)")
	})
}

func TestService_ProxyMessages_EmbedOnlyUserMessageContextOverride(t *testing.T) {
	const userPrompt = "Find the race condition in main.go"
	body := []byte(`{
		"model":"moonshotai/kimi-k3",
		"system":"You are Claude Code preamble...",
		"messages":[{"role":"user","content":"` + userPrompt + `"}]
	}`)

	cases := []struct {
		name           string
		startupFlag    bool
		ctxOverride    *bool
		wantPromptText string
	}{
		{
			name:           "ctx=true overrides startup=false",
			startupFlag:    false,
			ctxOverride:    boolPtr(true),
			wantPromptText: userPrompt,
		},
		{
			name:           "ctx=false overrides startup=true",
			startupFlag:    true,
			ctxOverride:    boolPtr(false),
			wantPromptText: "You are Claude Code preamble...\n" + userPrompt,
		},
		{
			name:           "no ctx override falls back to startup=true",
			startupFlag:    true,
			ctxOverride:    nil,
			wantPromptText: userPrompt,
		},
		{
			name:           "no ctx override falls back to startup=false",
			startupFlag:    false,
			ctxOverride:    nil,
			wantPromptText: "You are Claude Code preamble...\n" + userPrompt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRouter{decision: router.Decision{Provider: "anthropic", Model: "deepseek-ai/deepseek-v4-flash"}}
			svc := proxy.NewService(fr,
				map[string]providers.Client{"anthropic": &fakeProvider{}},
				nil,
				tc.startupFlag,
				nil,
				nil,
				false,
				providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash",
				nil,
			)

			ctx := context.Background()
			if tc.ctxOverride != nil {
				ctx = context.WithValue(ctx, proxy.EmbedOnlyUserMessageContextKey{}, *tc.ctxOverride)
			}

			rec := httptest.NewRecorder()
			httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
			require.NoError(t, svc.ProxyMessages(ctx, body, rec, httpReq))

			require.NotNil(t, fr.capturedReq)
			assert.Equal(t, tc.wantPromptText, fr.capturedReq.PromptText,
				"context override (%v) must beat startup flag (%v)", tc.ctxOverride, tc.startupFlag)
		})
	}
}

func boolPtr(b bool) *bool { return &b }

// TestService_ProxyMessages_NoPinStoreRunsScorerEveryTurn verifies that
// without a pin store, every turn re-runs the cluster scorer.
func TestService_ProxyMessages_NoPinStoreRunsScorerEveryTurn(t *testing.T) {
	body := []byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`)
	fr := &fakeRouter{decision: router.Decision{Provider: "anthropic", Model: "deepseek-ai/deepseek-v4-flash"}}
	svc := proxy.NewService(fr,
		map[string]providers.Client{providers.ProviderAnthropic: &fakeProvider{}},
		nil,
		false,
		nil,
		nil, // pinStore disabled
		false,
		providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash",
		nil,
	)

	ctx := context.WithValue(context.Background(), proxy.APIKeyIDContextKey{}, "key-1")
	for range 2 {
		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(ctx, body, rec, httpReq))
	}
	assert.Equal(t, 2, fr.routeCalls, "without a pin store, both turns must consult the scorer")
}

func TestService_ProxyOpenAIChatCompletion_AnthropicCrossFormat(t *testing.T) {
	anthropicResp := `{"id":"msg_abc","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"moonshotai/kimi-k3","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`

	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(anthropicResp))
		},
	}
	svc := makeProxyService(
		router.Decision{Provider: "anthropic", Model: "moonshotai/kimi-k3", Reason: "test"},
		map[string]providers.Client{"anthropic": provider},
	)

	openAIReq := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(openAIReq))

	err := svc.ProxyOpenAIChatCompletion(context.Background(), []byte(openAIReq), rec, httpReq)
	require.NoError(t, err)

	require.Len(t, provider.proxyBodies, 1)
	var translated map[string]any
	require.NoError(t, json.Unmarshal(provider.proxyBodies[0], &translated))
	assert.Equal(t, float64(100), translated["max_tokens"], "max_tokens preserved on translated body")
	msgs, _ := translated["messages"].([]any)
	require.Len(t, msgs, 1)

	var openAIOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &openAIOut))
	assert.Equal(t, "chat.completion", openAIOut["object"])
	choices, _ := openAIOut["choices"].([]any)
	require.Len(t, choices, 1)
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	assert.Equal(t, "Hello!", message["content"])
	assert.Equal(t, "stop", choice["finish_reason"])
}

func TestService_ProxyOpenAIChatCompletion_AnthropicProxyError_PropagatesError(t *testing.T) {
	upstreamErr := errors.New("dial tcp: connection refused")
	provider := &fakeProvider{
		proxyErr: upstreamErr,
	}
	svc := makeProxyService(
		router.Decision{Provider: "anthropic", Model: "moonshotai/kimi-k3", Reason: "test"},
		map[string]providers.Client{"anthropic": provider},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	err := svc.ProxyOpenAIChatCompletion(context.Background(), []byte(body), rec, httpReq)

	require.ErrorIs(t, err, upstreamErr, "upstream Proxy error must propagate")
	assert.NotContains(t, rec.Body.String(), "translation failed",
		"Proxy error must not be masked by Finalize's translation failure body")
}

func TestService_ProxyOpenAIChatCompletion_NativeOpenAI(t *testing.T) {
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion"}`)
		},
	}
	svc := makeProxyService(
		router.Decision{Provider: "openai", Model: "gpt-4o", Reason: "test"},
		map[string]providers.Client{"openai": provider},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	err := svc.ProxyOpenAIChatCompletion(context.Background(), []byte(body), rec, httpReq)
	require.NoError(t, err)

	require.Len(t, provider.proxyBodies, 1)
	var got map[string]any
	require.NoError(t, json.Unmarshal(provider.proxyBodies[0], &got))
	assert.Equal(t, "gpt-4o", got["model"], "envelope rewrites model to decision.Model")
	msgs, _ := got["messages"].([]any)
	require.Len(t, msgs, 1)
	assert.Contains(t, rec.Body.String(), `"chat.completion"`)
}

// OpenRouter speaks OpenAI Chat Completions natively, so an OpenAI-format
// inbound landing on an OpenRouter decision must take the no-translation path.
// Regression: eval harness v0.27 hit "no translation path defined".
func TestService_ProxyOpenAIChatCompletion_NativeOpenRouter(t *testing.T) {
	provider := &fakeProvider{
		proxyResponse: func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion"}`)
		},
	}
	svc := makeProxyService(
		router.Decision{Provider: providers.ProviderOpenRouter, Model: "qwen/qwen3.6-27b", Reason: "test"},
		map[string]providers.Client{providers.ProviderOpenRouter: provider},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	err := svc.ProxyOpenAIChatCompletion(context.Background(), []byte(body), rec, httpReq)
	require.NoError(t, err)

	require.Len(t, provider.proxyBodies, 1)
	var got map[string]any
	require.NoError(t, json.Unmarshal(provider.proxyBodies[0], &got))
	assert.Equal(t, "qwen/qwen3.6-27b", got["model"], "envelope rewrites model to decision.Model")
	assert.Contains(t, rec.Body.String(), `"chat.completion"`)
}

// Bedrock, Makora, and Together are direct providers served by the
// openaicompat client and must route through the OpenAI-emission case, not
// the default "no translation path" branch. Regression: Makora/Together
// (DeepSeek-V4 primaries) were missing from the old literal dispatch list and
// 502'd in prod. Keying dispatch off the translation family fixes all of
// them.
func TestService_ProxyMessages_DispatchesBedrockMakoraTogether(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
	}{
		{"bedrock", providers.ProviderBedrock, "moonshotai/kimi-k2.5"},
		{"makora", providers.ProviderMakora, "deepseek-ai/deepseek-v4-flash"},
		{"together", providers.ProviderTogether, "deepseek-ai/deepseek-v4-pro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProvider{
				proxyResponse: func(w http.ResponseWriter) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
				},
			}
			svc := makeProxyService(
				router.Decision{Provider: tc.provider, Model: tc.model, Reason: "test"},
				map[string]providers.Client{tc.provider: p},
			)
			// Tools + opus keeps this out of the classifier-hard-pin path,
			// so the test exercises the widened switch, not the fallback.
			body := []byte(`{"model":"moonshotai/kimi-k3","max_tokens":16,"tools":[{"name":"calc","description":"add","input_schema":{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}}],"messages":[{"role":"user","content":"What is 7+5? Use calc."}]}`)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
			require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, req))
			require.Len(t, p.proxyBodies, 1, "%s must reach the upstream", tc.provider)
		})
	}
}

func TestService_ProxyOpenAIChatCompletion_DispatchesBedrockMakoraTogether(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
	}{
		{"bedrock", providers.ProviderBedrock, "qwen/qwen3.6-27b-next"},
		{"makora", providers.ProviderMakora, "deepseek-ai/deepseek-v4-flash"},
		{"together", providers.ProviderTogether, "deepseek-ai/deepseek-v4-pro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProvider{
				proxyResponse: func(w http.ResponseWriter) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion"}`)
				},
			}
			svc := makeProxyService(
				router.Decision{Provider: tc.provider, Model: tc.model, Reason: "test"},
				map[string]providers.Client{tc.provider: p},
			)
			body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			require.NoError(t, svc.ProxyOpenAIChatCompletion(context.Background(), []byte(body), rec, req))
			require.Len(t, p.proxyBodies, 1, "%s must reach the upstream", tc.provider)
		})
	}
}

// TestService_WithByokOnly_FiltersUnauthedProvidersFromScorer: with BYOK-only,
// providers without per-request creds must be excluded, or argmax 402s.
func TestService_WithByokOnly_FiltersUnauthedProvidersFromScorer(t *testing.T) {
	body := []byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`)
	providerMap := map[string]providers.Client{
		providers.ProviderAnthropic:  &fakeProvider{},
		providers.ProviderOpenRouter: &fakeProvider{},
	}

	t.Run("byok-off keeps every registered provider eligible (selfhost baseline)", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderAnthropic)
		assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderOpenRouter)
	})

	t.Run("byok-on with no creds yields empty eligible set", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
			WithByokOnly(true)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		assert.Empty(t, fr.capturedReq.EnabledProviders, "BYOK-only: registered providers ineligible without creds")
	})

	t.Run("byok-on Anthropic surface with x-api-key enables Anthropic only", func(t *testing.T) {
		// A client x-api-key on the Anthropic surface is a legitimate passthrough
		// credential and enables Anthropic, but must not leak into OpenRouter or
		// other OpenAI-compat upstreams on a different inbound surface.
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
			WithByokOnly(true)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		httpReq.Header.Set("x-api-key", "sk-ant-customer-key")
		_ = svc.ProxyMessages(context.Background(), body, rec, httpReq)

		require.NotNil(t, fr.capturedReq)
		assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderAnthropic,
			"client-supplied x-api-key on the Anthropic surface enables Anthropic")
		assert.NotContains(t, fr.capturedReq.EnabledProviders, providers.ProviderOpenRouter,
			"client header on the Anthropic surface must not leak credentials into OpenAI-compat upstreams")
	})

	t.Run("byok-on Anthropic surface with inbound subscription Bearer enables Anthropic only", func(t *testing.T) {
		// A Claude subscription OAuth bearer is legitimate Anthropic auth and
		// enables Anthropic, but must never enable OpenRouter or other
		// OpenAI-compat upstreams — that cross-provider leak was the 2026-05-13
		// prod incident (argmax picked OpenRouter, 401'd with no OpenRouter key).
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
			WithByokOnly(true)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		httpReq.Header.Set("Authorization", "Bearer sk-ant-oat01-claude-code-token")
		_ = svc.ProxyMessages(context.Background(), body, rec, httpReq)

		require.NotNil(t, fr.capturedReq)
		assert.Contains(t, fr.capturedReq.EnabledProviders, providers.ProviderAnthropic,
			"a Claude subscription bearer is valid Anthropic auth and enables Anthropic")
		assert.NotContains(t, fr.capturedReq.EnabledProviders, providers.ProviderOpenRouter,
			"inbound Bearer on the Anthropic surface must never leak into OpenRouter (2026-05-13 incident)")
	})
}

// Model exclusion flows from installation context or env override into
// the router.Request that the scorer consumes. Env override wins.
func TestService_ExcludedModelsThroughRequest(t *testing.T) {
	body := []byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`)
	providerMap := map[string]providers.Client{providers.ProviderAnthropic: &fakeProvider{}}

	t.Run("no override and no installation list → nil", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(context.Background(), body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		assert.Nil(t, fr.capturedReq.ExcludedModels)
	})

	t.Run("installation list populates request", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

		ctx := context.WithValue(context.Background(), proxy.InstallationExcludedModelsContextKey{}, []string{"moonshotai/kimi-k3", "gpt-5"})
		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(ctx, body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		assert.Contains(t, fr.capturedReq.ExcludedModels, "moonshotai/kimi-k3")
		assert.Contains(t, fr.capturedReq.ExcludedModels, "gpt-5")
	})

	t.Run("env override replaces installation list", func(t *testing.T) {
		fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: "deepseek-ai/deepseek-v4-flash"}}
		svc := proxy.NewService(fr, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
			WithExcludedModelsOverride([]string{"gpt-4o"})

		// Installation list says one thing; override says another. Override wins.
		ctx := context.WithValue(context.Background(), proxy.InstallationExcludedModelsContextKey{}, []string{"moonshotai/kimi-k3"})
		rec := httptest.NewRecorder()
		httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
		require.NoError(t, svc.ProxyMessages(ctx, body, rec, httpReq))

		require.NotNil(t, fr.capturedReq)
		assert.Contains(t, fr.capturedReq.ExcludedModels, "gpt-4o")
		assert.NotContains(t, fr.capturedReq.ExcludedModels, "moonshotai/kimi-k3")
		assert.True(t, svc.HasExcludedModelsOverride())
		assert.Equal(t, []string{"gpt-4o"}, svc.ExcludedModelsOverride())
	})
}

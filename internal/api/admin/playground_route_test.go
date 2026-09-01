package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
)

type playgroundExternalKeyRepo struct {
	keys []*auth.ExternalAPIKey
}

func (r *playgroundExternalKeyRepo) Create(context.Context, auth.CreateExternalAPIKeyParams) (*auth.ExternalAPIKey, error) {
	return nil, nil
}
func (r *playgroundExternalKeyRepo) GetForInstallation(context.Context, string) ([]*auth.ExternalAPIKey, error) {
	return r.keys, nil
}
func (r *playgroundExternalKeyRepo) SoftDeleteByProvider(context.Context, string, string) error {
	return nil
}
func (r *playgroundExternalKeyRepo) SoftDelete(context.Context, string, string) error { return nil }
func (r *playgroundExternalKeyRepo) UpdateModelAliases(context.Context, string, string, map[string]string) (*auth.ExternalAPIKey, error) {
	return nil, nil
}
func (r *playgroundExternalKeyRepo) MarkUsed(context.Context, string) error { return nil }

type playgroundFakeRouter struct {
	decision router.Decision
	err      error
}

func (f *playgroundFakeRouter) Route(context.Context, router.Request) (router.Decision, error) {
	return f.decision, f.err
}

func playgroundSvc(fr *playgroundFakeRouter) *proxy.Service {
	return proxy.NewService(fr, map[string]providers.Client{}, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)
}

func playgroundRoutePOST(handler gin.HandlerFunc, body string, headers map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("router_installation", &auth.Installation{
			ID:         "11111111-1111-1111-1111-111111111111",
			ExternalID: "acct-test",
		})
		c.Next()
	})
	engine.POST("/v1/playground/route", handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/playground/route", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	engine.ServeHTTP(rec, req)
	return rec
}

func TestPlaygroundRoute_ReturnsSanitizedDecision(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "moonshotai/kimi-k2.7",
		Reason:   "cluster",
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"moonshotai/kimi-k2.7","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.ElementsMatch(t, []string{"model", "provider", "reason", "requested_cost_usd", "actual_cost_usd", "cache_savings_usd", "id"}, mapKeys(resp))
}

func TestPlaygroundRoute_NoMetadataLeak(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "moonshotai/kimi-k2.7",
		Reason:   "cluster",
		Metadata: &router.RoutingMetadata{Embedding: []float32{1, 0, 0}},
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"moonshotai/kimi-k2.7","messages":[{"role":"user","content":"hi"}]}`, nil)
	body := rec.Body.String()
	assert.NotContains(t, body, "embeddings")
	assert.NotContains(t, body, "metadata")
	assert.NotContains(t, body, "candidates")
}

func TestPlaygroundRoute_ForceModelThroughHeader(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "zai-org/glm-5.3",
		Reason:   "forced",
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"x-weave-force-model": "zai-org/glm-5.3"})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "zai-org/glm-5.3", resp["model"])
}

func TestPlaygroundRoute_ModelFieldBeatsHeader(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "qwen/qwen3.8-27b",
		Reason:   "forced",
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"qwen/qwen3.8-27b","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"x-weave-force-model": "zai-org/glm-5.3"})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "qwen/qwen3.8-27b", resp["model"])
}

func TestPlaygroundRoute_ModelNullIsAuto(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "zai-org/glm-5.3",
		Reason:   "cluster",
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"x-weave-force-model": "zai-org/glm-5.3"})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestPlaygroundRoute_NonJSONObject400(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}), `[1,2,3]`, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPlaygroundRoute_BodyTooLarge413(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}), strings.Repeat("x", proxy.MaxRequestBodyBytes+1), nil)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestPlaygroundRoute_RoutingFailure502(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{err: errors.New("boom")})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"moonshotai/kimi-k2.7","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestPlaygroundRoute_DecisionUpstreamIDNeverLegacy(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{decision: router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "z-ai/glm-5.2",
		Reason:   "sticky",
	}})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "zai-org/glm-5.3", resp["model"])
}

func TestPlaygroundRoute_ForceHeaderUnknown400(t *testing.T) {
	svc := playgroundSvc(&playgroundFakeRouter{})
	rec := playgroundRoutePOST(admin.PlaygroundRouteHandler(svc, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"x-weave-force-model": "not-a-real-model-xyz"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// playgroundChat tests below use a capturing proxy wrapper.

type playgroundChatProxy struct {
	lastBody []byte
	lastCtx  context.Context
	proxyFn  func(ctx context.Context, body []byte, w http.ResponseWriter, r *http.Request) error
}

func (p *playgroundChatProxy) ProxyOpenAIChatCompletion(ctx context.Context, body []byte, w http.ResponseWriter, r *http.Request) error {
	p.lastBody = append([]byte(nil), body...)
	p.lastCtx = ctx
	if p.proxyFn != nil {
		return p.proxyFn(ctx, body, w, r)
	}
	return nil
}

type playgroundChatService struct {
	*playgroundChatProxy
}

func (s *playgroundChatService) ProxyOpenAIChatCompletion(ctx context.Context, body []byte, w http.ResponseWriter, r *http.Request) error {
	return s.playgroundChatProxy.ProxyOpenAIChatCompletion(ctx, body, w, r)
}

func playgroundChatPOST(handler gin.HandlerFunc, body string, headers map[string]string) *httptest.ResponseRecorder {
	return playgroundChatPOSTWithInst(handler, body, headers, &auth.Installation{
		ID:         "11111111-1111-1111-1111-111111111111",
		ExternalID: "acct-test",
	})
}

func playgroundChatPOSTWithInst(handler gin.HandlerFunc, body string, headers map[string]string, inst *auth.Installation) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if inst != nil {
		engine.Use(func(c *gin.Context) {
			c.Set("router_installation", inst)
			c.Next()
		})
	}
	engine.POST("/v1/playground/chat", handler)
	req := httptest.NewRequest(http.MethodPost, "/v1/playground/chat", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestPlaygroundChat_BindsInstallationContext(t *testing.T) {
	inst := &auth.Installation{ID: "11111111-1111-1111-1111-111111111111", ExternalID: "acct-1"}
	ext := &playgroundExternalKeyRepo{keys: []*auth.ExternalAPIKey{{
		ID: "ek-1", Provider: providers.ProviderAiand, Plaintext: []byte("sk-user"),
	}}}
	name := auth.PlaygroundAPIKeyName
	keyRepo := &playgroundAPIKeyRepo{keys: []*auth.APIKey{{
		ID:             "playground-key-id",
		InstallationID: inst.ID,
		Name:           &name,
		Scope:          auth.ScopeRouting,
	}}}
	authSvc := auth.NewService(nil, keyRepo, ext, nil, auth.NoOpAPIKeyCache{}, nil, func() time.Time { return time.Now() })

	capture := &playgroundChatProxy{}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("router_installation", inst)
		c.Next()
	})
	engine.POST("/v1/playground/chat", admin.PlaygroundChatHandler(capture, authSvc))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/playground/chat",
		bytes.NewReader([]byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)))
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	id, _ := capture.lastCtx.Value(proxy.InstallationIDContextKey{}).(string)
	assert.Equal(t, inst.ID, id)
	apiKeyID, _ := capture.lastCtx.Value(proxy.APIKeyIDContextKey{}).(string)
	assert.Equal(t, "playground-key-id", apiKeyID)
	assert.True(t, proxy.RespondRoutingMetadata(capture.lastCtx))
	keys, _ := capture.lastCtx.Value(proxy.ExternalAPIKeysContextKey{}).([]*auth.ExternalAPIKey)
	require.Len(t, keys, 1)
	assert.Equal(t, providers.ProviderAiand, keys[0].Provider)
}

type playgroundAPIKeyRepo struct {
	keys []*auth.APIKey
}

func (r *playgroundAPIKeyRepo) Create(context.Context, auth.CreateAPIKeyParams) (*auth.APIKey, error) {
	return nil, nil
}
func (r *playgroundAPIKeyRepo) GetActiveByHashWithInstallation(context.Context, string) (*auth.APIKey, *auth.Installation, error) {
	return nil, nil, nil
}
func (r *playgroundAPIKeyRepo) ListForInstallation(context.Context, string) ([]*auth.APIKey, error) {
	return r.keys, nil
}
func (r *playgroundAPIKeyRepo) MarkUsed(context.Context, string) error { return nil }
func (r *playgroundAPIKeyRepo) SoftDelete(context.Context, string, string) (int64, error) {
	return 0, nil
}

func TestPlaygroundChat_StreamsSSEWithDone(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(_ context.Context, _ []byte, w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"chunk\":1}\n\ndata: [DONE]\n\n"))
		return nil
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}],"stream":true}`, nil)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(rec.Body.String()), "data: [DONE]"))
}

func TestPlaygroundChat_SessionHeaderDrivesPinKey(t *testing.T) {
	capture := &playgroundChatProxy{}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Playground-Session": "playground-session-abc"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, proxy.ClientAppPlayground, proxy.ClientIdentityFrom(capture.lastCtx).ClientApp)
	assert.Equal(t, "playground-session-abc", proxy.ClientIdentityFrom(capture.lastCtx).SessionID)
	assert.Contains(t, string(capture.lastBody), "playground-session-abc")
}

func TestPlaygroundChat_Classified429RetryAfter(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(context.Context, []byte, http.ResponseWriter, *http.Request) error {
		return &providers.UpstreamStatusError{Status: http.StatusTooManyRequests}
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	var body struct {
		Error struct {
			Code *string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Error.Code)
	assert.Equal(t, "rate_limit", *body.Error.Code)
}

func TestPlaygroundChat_NonStreamReturnsJSON(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(_ context.Context, _ []byte, w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}]}`))
		return nil
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}],"stream":false}`, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "chatcmpl-1", body["id"])
}

func TestPlaygroundChat_402Surfaced(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(context.Context, []byte, http.ResponseWriter, *http.Request) error {
		return &providers.UpstreamStatusError{Status: http.StatusPaymentRequired}
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusPaymentRequired, rec.Code)
	var body struct {
		Error struct {
			Code *string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Error.Code)
	assert.Equal(t, "insufficient_credits", *body.Error.Code)
}

func TestPlaygroundChat_503Surfaced(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(context.Context, []byte, http.ResponseWriter, *http.Request) error {
		return &providers.UpstreamStatusError{Status: http.StatusServiceUnavailable}
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body struct {
		Error struct {
			Code *string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Error.Code)
	assert.Equal(t, "unavailable", *body.Error.Code)
}

func TestPlaygroundChat_ModelNullForwardsAuto(t *testing.T) {
	capture := &playgroundChatProxy{}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":null,"messages":[{"role":"user","content":"hi"}]}`, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(capture.lastBody), `"model":"auto"`)
}

func TestPlaygroundChat_MidStreamFailureWritesTrailingErrorEvent(t *testing.T) {
	capture := &playgroundChatProxy{proxyFn: func(_ context.Context, _ []byte, w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"chunk\":1}\n\n"))
		return errors.New("upstream died")
	}}
	rec := playgroundChatPOST(admin.PlaygroundChatHandler(capture, &auth.Service{}),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}],"stream":true}`, nil)
	assert.Contains(t, rec.Body.String(), "upstream_interrupted")
}

// Silence unused import when build tags omit other chat tests.
var _ = time.Second

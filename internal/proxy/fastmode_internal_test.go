package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The AIand roster exposes no fast tier: no binding carries FastPrice, so the
// installation fast-mode opt-in is inert for every model the router can serve.
// These tests pin that inertness at the proxy layer; catalog/fastmode_test.go
// owns the pricing-table half.
const (
	fastRosterModel   = "zai-org/glm-5.3"
	fastRosterDefault = "zai-org/glm-5.3-flash"
)

func fastModeCtx(models ...string) context.Context {
	return context.WithValue(context.Background(), InstallationFastModeModelsContextKey{}, models)
}

func newFastModeService(decision router.Decision, upstream providers.Client) (*Service, *bypassCaptureTelemetry) {
	telemetry := newBypassCaptureTelemetry()
	svc := NewService(staticRouter{decision: decision}, map[string]providers.Client{
		providers.ProviderAIAND: upstream,
	}, nil, false, nil, nil, false, providers.ProviderAIAND, fastRosterDefault, telemetry)
	return svc, telemetry
}

func TestFastModeForAttempt(t *testing.T) {
	t.Run("off when the installation did not opt the model in", func(t *testing.T) {
		assert.False(t, fastModeForAttempt(fastModeCtx("deepseek-ai/deepseek-v4-pro"), fastRosterModel, providers.ProviderAIAND))
		assert.False(t, fastModeForAttempt(context.Background(), fastRosterModel, providers.ProviderAIAND))
	})
	t.Run("still off for an opted-in binding with no fast tier", func(t *testing.T) {
		assert.False(t, fastModeForAttempt(fastModeCtx(fastRosterModel), fastRosterModel, providers.ProviderAIAND))
		assert.False(t, fastModeForAttempt(fastModeCtx("moonshotai/kimi-k3"), "moonshotai/kimi-k3", providers.ProviderAIAND))
	})
	t.Run("off when the turn is served on a subscription", func(t *testing.T) {
		ctx := context.WithValue(fastModeCtx(fastRosterModel), CredentialsContextKey{}, &Credentials{
			APIKey: []byte("sk-ant-oat-token"),
			Source: credSourceSubscription,
			OAuth:  true,
		})
		assert.False(t, fastModeForAttempt(ctx, fastRosterModel, providers.ProviderAIAND))
	})
}

func TestServedPricing(t *testing.T) {
	base, ok := catalog.PriceFor(providers.ProviderAIAND, "qwen/qwen3.8-27b")
	require.True(t, ok)
	got, ok := servedPricing(providers.ProviderAIAND, "qwen/qwen3.8-27b", false)
	require.True(t, ok)
	assert.Equal(t, base, got)

	got, ok = servedPricing(providers.ProviderAIAND, "qwen/qwen3.8-27b", true)
	require.True(t, ok, "a binding without a fast tier must still price at its list rate")
	assert.Equal(t, base, got, "the fast flag cannot invent a fast tier the roster does not have")
}

func TestProxyMessages_FastModeNoFastTierNeverGetsFastFields(t *testing.T) {
	upstream := &bypassFakeProvider{respBody: `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"` + fastRosterModel + `","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`}
	svc, _ := newFastModeService(router.Decision{Provider: providers.ProviderAIAND, Model: fastRosterModel}, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"` + fastRosterModel + `","messages":[{"role":"user","content":"hi"}]}`)
	require.NoError(t, svc.ProxyMessages(fastModeCtx(fastRosterModel), body, rec, req))

	require.Equal(t, 1, upstream.dispatches)
	assert.False(t, gjson.GetBytes(upstream.capturedBody, "service_tier").Exists())
	assert.False(t, gjson.GetBytes(upstream.capturedBody, "speed").Exists())
}

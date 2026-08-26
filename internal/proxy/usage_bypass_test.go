package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/proxy/usage"
	"workweave/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bypassSubToken      = "sk-ant-oat01-subscription-token"
	bypassRequestedMdl  = "deepseek-ai/deepseek-v4-pro"
	bypassScorerPickMdl = "deepseek-ai/deepseek-v4-flash"
)

// bypassFixture builds a service whose fake scorer would route to
// bypassScorerPickMdl, with the subscription usage observer wired and a fake
// Anthropic provider that returns a minimal valid Messages response so a routed
// turn completes. seedUtil >= 0 pre-records an observation at that utilization
// under the subscription token; seedUtil < 0 leaves the observer cold.
func bypassFixture(t *testing.T, seedUtil float64) (*proxy.Service, *fakeRouter, *fakeProvider) {
	t.Helper()
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: bypassScorerPickMdl}}
	p := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + bypassScorerPickMdl + `","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}}
	obs := usage.NewObserver([]byte("salt"), 10*time.Minute, time.Now)
	if seedUtil >= 0 {
		obs.Record(obs.Key([]byte(bypassSubToken)), usage.Snapshot{
			Primary: usage.Window{UsedPercent: seedUtil, WindowMinutes: 300},
		})
	}
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAnthropic: p}, nil, false, nil, nil, false, providers.ProviderAiand, bypassScorerPickMdl, nil).
		WithSubscriptionAwareRouting(obs, 0.05, 2.0)
	return svc, fr, p
}

// bypassCtx returns a ctx carrying a Claude subscription token plus a
// per-installation usage-bypass config at the given threshold.
func bypassCtx(threshold float64) context.Context {
	ctx := context.WithValue(context.Background(), proxy.AnthropicSubscriptionContextKey{}, bypassSubToken)
	return context.WithValue(ctx, proxy.InstallationUsageBypassContextKey{}, proxy.UsageBypassConfig{
		Enabled:   true,
		Threshold: &threshold,
	})
}

func bypassRequest(t *testing.T) (*httptest.ResponseRecorder, *http.Request, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"` + bypassRequestedMdl + `","messages":[{"role":"user","content":"hi"}]}`)
	return rec, req, body
}

// TestSubscriptionExhausted_NoDeploymentKey_KeepsSubscription guards the
// safety rail: with no deployment / BYOK Anthropic key to fall through to,
// dropping the subscription would leave the turn with no credential (a 400,
// worse than the 429). So the subscription is kept even when exhausted.
func TestSubscriptionExhausted_NoDeploymentKey_KeepsSubscription(t *testing.T) {
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: bypassScorerPickMdl}}
	p := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + bypassScorerPickMdl + `","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}}
	obs := usage.NewObserver([]byte("salt"), 10*time.Minute, time.Now)
	obs.Record(obs.Key([]byte(bypassSubToken)), usage.Snapshot{
		Secondary: usage.Window{UsedPercent: 1.0, WindowMinutes: 10080},
	})
	// No WithDeploymentKeyedProviders — passthrough-only Anthropic.
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAnthropic: p}, nil, false, nil, nil, false, providers.ProviderAiand, bypassScorerPickMdl, nil).
		WithSubscriptionAwareRouting(obs, 0.05, 2.0)

	rec, req, body := bypassRequest(t)
	require.NoError(t, svc.ProxyMessages(bypassCtx(0.80), body, rec, req))

	require.Len(t, p.proxyCreds, 1)
	creds := p.proxyCreds[0]
	require.NotNil(t, creds, "with no fallback key the subscription must still be used")
	assert.True(t, creds.OAuth,
		"no deployment/BYOK key to fall through to — keep the subscription rather than 400")
}

// bypassStreamResponse writes a minimal valid Anthropic SSE stream so the
// subscription-only warning marker (which only injects on a streaming response)
// has a stream to prepend to.
func bypassStreamResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_up\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"" + bypassRequestedMdl + "\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

// swapErrProvider wraps a fakeProvider and returns `first` as the proxyErr on
// the first dispatch, `second` on every dispatch thereafter. Used to simulate a
// bypass 429 followed by a successful routed dispatch against the same fake.
type swapErrProvider struct {
	first, second error
	inner         *fakeProvider
	calls         int
}

func (s *swapErrProvider) Proxy(ctx context.Context, decision router.Decision, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	s.calls++
	err := s.first
	if s.calls > 1 {
		err = s.second
	}
	orig := s.inner.proxyErr
	s.inner.proxyErr = err
	defer func() { s.inner.proxyErr = orig }()
	return s.inner.Proxy(ctx, decision, prep, w, r)
}

func (s *swapErrProvider) Passthrough(ctx context.Context, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	return s.inner.Passthrough(ctx, prep, w, r)
}
